// Package autoupdate is the unattended startup check that keeps the
// player-facing app's rules database current with the official sourcebooks
// on Google Drive, without a human ever running n5e-ingest by hand.
//
// For each internal/sources.OfficialBooks entry with a registered parser,
// two signals can force a download:
//
//   - Drive's copy changed: a HEAD request's Last-Modified
//     (internal/fetch.HeadLastModified) is newer than the stored
//     drive_last_modified baseline. This is the only network call on a
//     normal startup; nothing is downloaded when Drive reports no change.
//   - This build needs the book re-parsed: the stored ingest_schema_version
//     sorts below schema.RequiredIngestVersion for that book. Only a
//     migration carrying a "-- reingest:" marker for the book moves that,
//     for a parser change whose effect cannot be backfilled in SQL. Schema
//     changes and data fixes that backfill themselves never force one.
//
// A never-stamped baseline counts as stale, so a book that was never
// ingested on this install is ingested once.
//
// When Run is given a library directory, each book's downloaded PDF is kept
// there (as <slug base>.pdf) for the app's in-app sourcebook reader instead
// of being deleted after its ingest attempt, and a book whose library copy
// is missing is downloaded even when neither signal above fired. The usual
// hash check still applies, so an install that already has current rules
// content only fetches the file.
//
// Once either signal says "stale," the PDF is downloaded. If only the Drive
// timestamp moved and the file's SHA-256 matches the one recorded at the
// last ingest, the content is identical and nothing is re-ingested.
// Otherwise the file is handed to the matching internal/ingest function,
// the same parse-correct-load pipeline cmd/n5e-ingest's subcommands use. Every per-book step is isolated
// so one book's failure (network, parser) can never stop the others from
// being checked, matching this codebase's established "one bad row never
// blocks the batch" philosophy already used inside internal/store's LoadX
// functions.
package autoupdate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RationalRouter/n5e/internal/fetch"
	"github.com/RationalRouter/n5e/internal/ingest"
	"github.com/RationalRouter/n5e/internal/schema"
	"github.com/RationalRouter/n5e/internal/sources"
	"github.com/RationalRouter/n5e/internal/store"
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
	"book/core":              ingest.Core,
	"book/jutsu-compendium":  ingest.Jutsu,
	"book/bingo-book-pack-1": ingest.BingoBookPack1,
	"book/bingo-book":        ingest.BingoBook,
	// Disabled with its sources.OfficialBooks entry; see the comment there.
	// Enabling it needs an internal/ingest entry point wrapping what
	// cmd/n5e-ingest's adversary-builder-sheet subcommand does today.
	// "book/adversary-builder-sheet": ingest.AdversaryBuilderSheet,
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
	downloadTimeout = 30 * time.Minute
)

// Run checks every book in books against its canonical Drive location and
// updates db in place for anything that changed. db must already be a
// writable, schema.Apply'd rules database; dbPath is its on-disk path,
// needed only to key internal/ingest's LanguageTool correction cache
// sidecar file. cacheDir holds each book's downloaded PDF for the duration
// of its own check — created if missing, and each file removed again right
// after that book's dispatch attempt, successful or not. libraryDir, when
// not empty, replaces cacheDir for book downloads: each successfully
// downloaded PDF is staged under libraryDir/pending and then moved to
// LibraryPath, where it stays (see the package doc). client may be nil
// (internal/fetch's own sane defaults apply); passing one in is mainly for
// tests that need to redirect Drive's hardcoded hostname to a local
// httptest server.
func Run(ctx context.Context, db *sql.DB, dbPath, cacheDir, libraryDir string, client *http.Client, books []sources.BookSource) []Result {
	dir := cacheDir
	if libraryDir != "" {
		dir = filepath.Join(libraryDir, pendingDir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// Every book fails the same way if the cache dir can't be made —
		// still report one Result per book rather than a bare error return,
		// so callers only ever need to walk one shape of output.
		results := make([]Result, len(books))
		for i, b := range books {
			results[i] = Result{Slug: b.Slug, Title: b.Title, Action: "error",
				Err: fmt.Errorf("creating download dir %s: %w", dir, err)}
		}
		return results
	}

	// The version stamped on each book after a successful ingest. A read
	// failure (of this binary's own embedded migrations) is fanned out as a
	// per-book error, same shape as the cache-dir failure above.
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
		results = append(results, checkOne(ctx, db, dbPath, cacheDir, libraryDir, client, currentVersion, book))
	}
	return results
}

// pendingDir is the libraryDir subdirectory downloads are staged in, so a
// partial file never sits under a book's real library name.
const pendingDir = "pending"

// readySuffix marks a staged download as complete. PromotePending only
// promotes staged files that carry one.
const readySuffix = ".ready"

// LibraryFileName is the file name a book's PDF is kept under in the
// library directory, e.g. "core.pdf" for book/core.
func LibraryFileName(slug string) string {
	return filepath.Base(slug) + ".pdf"
}

// LibraryPath is where a book's kept PDF lives inside libraryDir.
func LibraryPath(libraryDir, slug string) string {
	return filepath.Join(libraryDir, LibraryFileName(slug))
}

// promote moves a completed staged download over its library file. The
// rename is retried briefly because Windows refuses to replace a file that
// another handle (an in-progress read by the sourcebook reader) has open.
// A rename that still fails leaves the staged file and its ready marker for
// the next startup's PromotePending.
func promote(staged, final string) error {
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		if err = os.Rename(staged, final); err == nil {
			os.Remove(staged + readySuffix)
			return nil
		}
		time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
	}
	return err
}

// PromotePending finishes any library download a previous run completed but
// could not move into place, and discards staged files that were never
// completed (the process ended mid-download). Call it at startup, before
// anything reads the library.
func PromotePending(libraryDir string) error {
	dir := filepath.Join(libraryDir, pendingDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, readySuffix) {
			continue
		}
		staged := filepath.Join(dir, name)
		if _, err := os.Stat(staged + readySuffix); err != nil {
			os.Remove(staged)
			continue
		}
		if err := promote(staged, filepath.Join(libraryDir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	// A marker whose file is gone has nothing left to promote.
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, readySuffix) {
			if _, err := os.Stat(filepath.Join(dir, strings.TrimSuffix(name, readySuffix))); errors.Is(err, fs.ErrNotExist) {
				os.Remove(filepath.Join(dir, name))
			}
		}
	}
	return errors.Join(errs...)
}

func checkOne(ctx context.Context, db *sql.DB, dbPath, cacheDir, libraryDir string, client *http.Client, currentVersion string, book sources.BookSource) Result {
	res := Result{Slug: book.Slug, Title: book.Title}

	libraryMissing := false
	if libraryDir != "" {
		if _, err := os.Stat(LibraryPath(libraryDir, book.Slug)); errors.Is(err, fs.ErrNotExist) {
			libraryMissing = true
		}
	}

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
		required, err := schema.RequiredIngestVersion(schema.Rules, book.Slug)
		if err != nil {
			res.Action, res.Err = "error", fmt.Errorf("determining required ingest version: %w", err)
			return res
		}
		versionStale = !foundVersion || storedVersion < required
	}

	headCtx, cancel := context.WithTimeout(ctx, headTimeout)
	remote, err := fetch.HeadLastModified(headCtx, client, book.DriveURL)
	cancel()
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("checking for updates: %w", err)
		return res
	}

	if foundLast && !remote.After(last) && !versionStale && !libraryMissing {
		res.Action = "unchanged"
		return res
	}

	destPath := filepath.Join(cacheDir, LibraryFileName(book.Slug))
	if libraryDir != "" {
		destPath = filepath.Join(libraryDir, pendingDir, LibraryFileName(book.Slug))
	}
	dlCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	sha, err := fetch.Download(dlCtx, client, book.DriveURL, destPath)
	cancel()
	if err != nil {
		os.Remove(destPath)
		res.Action, res.Err = "error", fmt.Errorf("downloading: %w", err)
		return res
	}
	if libraryDir == "" {
		defer os.Remove(destPath)
	} else {
		// The file is the current Drive copy whatever happens to the ingest
		// below, so it is kept for reading even when parsing fails. The
		// marker is written before the deferred move so a failed move can be
		// finished by PromotePending on the next startup.
		if err := os.WriteFile(destPath+readySuffix, nil, 0o644); err != nil {
			os.Remove(destPath)
			res.Action, res.Err = "error", fmt.Errorf("marking download complete: %w", err)
			return res
		}
		defer promote(destPath, LibraryPath(libraryDir, book.Slug))
	}

	// Drive's Last-Modified can move without the file's bytes changing. When
	// the downloaded file hashes identically to the one last ingested and this
	// build's ingest logic hasn't changed, the stored content is already
	// current: re-baseline the timestamp and skip the re-ingest.
	if hasParser && !versionStale {
		storedSHA, foundSHA, err := store.GetSourceBookSHA256(db, book.Slug)
		if err != nil {
			res.Action, res.Err = "error", fmt.Errorf("reading stored sha256: %w", err)
			return res
		}
		if foundSHA && storedSHA == sha {
			if err := store.SetSourceBookLastModified(db, book.Slug, remote); err != nil {
				res.Action, res.Err = "error", fmt.Errorf("stamping last-modified: %w", err)
				return res
			}
			res.Action = "unchanged"
			return res
		}
	}

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
