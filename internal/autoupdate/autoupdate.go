// Package autoupdate is the unattended startup check that keeps the
// player-facing app's rules database current with the official sourcebooks
// on Google Drive, without a human ever running n5e-ingest by hand.
//
// For each internal/sources.OfficialBooks entry with a registered parser,
// TWO independent staleness signals can force a real download+ingest even
// when the other one says "unchanged": the stored drive_last_modified
// baseline (internal/store.GetSourceBookLastModified) vs. a cheap HEAD
// request (internal/fetch.HeadLastModified) — "has the PDF itself changed
// on Drive" — and the stored ingest_schema_version baseline
// (internal/store.GetSourceBookIngestVersion) vs. this binary's own
// schema.LatestMigration(schema.Rules) — "has THIS BUILD's own
// parsing/schema logic moved on since the last successful ingest,
// regardless of whether Drive's copy changed at all." The second signal is
// what closes a real gap found 2026-07-27: a migration adding a table that
// needs a content backfill (jutsu_upcast_rules) applies structurally on
// every startup regardless, but its content only gets populated by a real
// ingest — which, before this signal existed, only ever ran when Drive's
// own timestamp moved, so an existing install's newly-added-but-empty table
// could sit empty forever if nothing about the upstream PDF happened to
// change afterward. Both baselines are nullable/never-stamped on every
// pre-existing row, which is exactly what makes this self-healing on
// upgrade: an old install force-ingests every parsed book exactly once on
// its first launch after picking up this change, then settles back into the
// normal cheap-check cadence.
//
// Once either signal says "stale," the PDF is downloaded and handed to the
// matching internal/ingest function, the same parse-correct-load pipeline
// cmd/n5e-ingest's subcommands already use. Every per-book step is isolated
// so one book's failure (network, parser) can never stop the others from
// being checked, matching this codebase's established "one bad row never
// blocks the batch" philosophy already used inside internal/store's LoadX
// functions.
package autoupdate

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sergio/n5e/internal/fetch"
	"github.com/sergio/n5e/internal/ingest"
	"github.com/sergio/n5e/internal/schema"
	"github.com/sergio/n5e/internal/sources"
	"github.com/sergio/n5e/internal/store"
)

// Result is what happened when checking one official sourcebook.
type Result struct {
	Slug, Title string
	// Action is one of "unchanged", "updated", "no parser yet", "error".
	Action string
	Report *ingest.Report // set only when Action == "updated"
	Err    error          // set only when Action == "error"
}

// ingestFunc is the common shape every internal/ingest entry point reduces
// to once its book-specific extra arguments (Clans/Classes' verbose bool)
// are folded in by ingestDispatch below.
type ingestFunc func(db *sql.DB, dbPath, pdfPath, version string) (*ingest.Report, error)

// ingestDispatch is the "does this book have a registered parser" table —
// a package var, not a literal switch, specifically so tests can register a
// fake entry and exercise the full updated/error paths without needing a
// real sourcebook PDF on disk. A slug with no entry here (today: only the
// Kage Guide, which has no parser at all) is exactly what "no parser yet"
// means; it is not an error.
var ingestDispatch = map[string]ingestFunc{
	"book/core":             ingest.Core,
	"book/jutsu-compendium": ingest.Jutsu,
	"book/bingo-book":       ingest.BingoBook,
	"book/clan-compendium": func(db *sql.DB, dbPath, pdfPath, version string) (*ingest.Report, error) {
		return ingest.Clans(db, dbPath, pdfPath, version, false)
	},
	"book/class-compendium": func(db *sql.DB, dbPath, pdfPath, version string) (*ingest.Report, error) {
		return ingest.Classes(db, dbPath, pdfPath, version, false)
	},
}

// headTimeout/downloadTimeout bound each book's own network phase
// independently (via context.WithTimeout derived from the caller's ctx),
// rather than sharing one small budget across every book checked in a Run
// call — otherwise one slow or stalled book's HEAD/download could exhaust a
// shared deadline and starve every book checked after it of a fair chance,
// even though their own network calls would have been fine. There is no
// timeout at all on the ingest step below that: internal/ingest's exported
// functions (Stage 1) take no context.Context parameter and call
// internal/correct.Sweep with context.Background() internally, so once a
// download succeeds and parsing/correction starts, it runs to completion or
// error exactly like it always has for a maintainer's hand-run
// `n5e-ingest -db`, uncancellable by anything Run does here.
const (
	headTimeout     = 15 * time.Second
	downloadTimeout = 3 * time.Minute
)

// Run checks every book in books against its canonical Drive location and
// updates db in place for anything that changed. db must already be a
// writable, schema.Apply'd rules database; dbPath is its on-disk path,
// needed only to key internal/ingest's LanguageTool correction cache
// sidecar file. cacheDir holds each book's downloaded PDF for the duration
// of its own check — created if missing, and each file removed again right
// after that book's dispatch attempt, successful or not. client may be nil
// (internal/fetch's own sane defaults apply); passing one in is mainly for
// tests that need to redirect Drive's hardcoded hostname to a local
// httptest server.
func Run(ctx context.Context, db *sql.DB, dbPath, cacheDir string, client *http.Client, books []sources.BookSource) []Result {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		// Every book fails the same way if the cache dir can't be made —
		// still report one Result per book rather than a bare error return,
		// so callers only ever need to walk one shape of output.
		results := make([]Result, len(books))
		for i, b := range books {
			results[i] = Result{Slug: b.Slug, Title: b.Title, Action: "error",
				Err: fmt.Errorf("creating cache dir %s: %w", cacheDir, err)}
		}
		return results
	}

	// Computed once per Run, not per book: this binary's own current version
	// of the "does an ingested book's content need to be regenerated" signal
	// (see the package doc comment). A read failure here is essentially
	// impossible in practice — it's reading this same binary's own embedded
	// migration files, not touching disk or network — but is fanned out as a
	// per-book error result rather than a bare panic/return, same shape as
	// the cache-dir failure just above, so every book still gets exactly one
	// Result.
	currentVersion, err := schema.LatestMigration(schema.Rules)
	if err != nil {
		results := make([]Result, len(books))
		for i, b := range books {
			results[i] = Result{Slug: b.Slug, Title: b.Title, Action: "error",
				Err: fmt.Errorf("determining current schema version: %w", err)}
		}
		return results
	}

	results := make([]Result, 0, len(books))
	for _, book := range books {
		results = append(results, checkOne(ctx, db, dbPath, cacheDir, client, currentVersion, book))
	}
	return results
}

func checkOne(ctx context.Context, db *sql.DB, dbPath, cacheDir string, client *http.Client, currentVersion string, book sources.BookSource) Result {
	res := Result{Slug: book.Slug, Title: book.Title}

	last, foundLast, err := store.GetSourceBookLastModified(db, book.Slug)
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("reading stored last-modified: %w", err)
		return res
	}

	// Only books with a registered parser have ingested content that can go
	// stale relative to this binary's own logic — a no-parser book (the Kage
	// Guide today) never has anything to backfill, so it must stay on the
	// pure Drive-timestamp cadence below or it would re-download on every
	// single startup forever (its ingest_schema_version is never stamped,
	// since nothing ever ingests it).
	fn, hasParser := ingestDispatch[book.Slug]
	versionStale := false
	if hasParser {
		storedVersion, foundVersion, err := store.GetSourceBookIngestVersion(db, book.Slug)
		if err != nil {
			res.Action, res.Err = "error", fmt.Errorf("reading stored ingest version: %w", err)
			return res
		}
		versionStale = !foundVersion || storedVersion != currentVersion
	}

	headCtx, cancel := context.WithTimeout(ctx, headTimeout)
	remote, err := fetch.HeadLastModified(headCtx, client, book.DriveURL)
	cancel()
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("checking for updates: %w", err)
		return res
	}

	if foundLast && !remote.After(last) && !versionStale {
		res.Action = "unchanged"
		return res
	}

	destPath := filepath.Join(cacheDir, filepath.Base(book.Slug)+".pdf")
	dlCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	sha, err := fetch.Download(dlCtx, client, book.DriveURL, destPath)
	cancel()
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("downloading: %w", err)
		return res
	}
	defer os.Remove(destPath)

	// Honest provenance, not a fabricated printed version string: there is
	// no human present to type the book's real "3.12"-style version, so the
	// Drive Last-Modified date stands in for it.
	version := remote.Format("2006-01-02") + " (auto, drive)"

	if !hasParser {
		if err := store.RecordSourceBookProvenance(db, store.SourceBook{
			Slug: book.Slug, Title: book.Title, Version: version,
			FileName: filepath.Base(destPath), FileSHA256: sha,
		}); err != nil {
			res.Action, res.Err = "error", fmt.Errorf("recording provenance: %w", err)
			return res
		}
		if err := store.SetSourceBookLastModified(db, book.Slug, remote); err != nil {
			res.Action, res.Err = "error", fmt.Errorf("stamping last-modified: %w", err)
			return res
		}
		res.Action = "no parser yet"
		return res
	}

	report, err := fn(db, dbPath, destPath, version)
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("ingesting: %w", err)
		return res
	}
	if err := store.SetSourceBookIngestVersion(db, book.Slug, currentVersion); err != nil {
		res.Action, res.Err = "error", fmt.Errorf("stamping ingest version: %w", err)
		return res
	}
	if err := store.SetSourceBookLastModified(db, book.Slug, remote); err != nil {
		res.Action, res.Err = "error", fmt.Errorf("stamping last-modified: %w", err)
		return res
	}

	res.Action = "updated"
	res.Report = report
	return res
}
