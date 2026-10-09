// Package schema owns the SQL schema for both N5E databases and applies it.
//
// There are two independent migration series:
//
//   - migrations/rules      → the read-only game-content database built by
//     n5e-ingest and embedded into the player app.
//   - migrations/characters → characters.db, created next to the executable
//     on first run.
//
// Migrations are plain .sql files applied in filename order (0001_..., 0002_...).
// Applied filenames are recorded in a schema_migrations table so re-opening an
// existing database only applies what is new. Never edit an already-shipped
// migration file — add a new one.
package schema

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/rules/*.sql migrations/characters/*.sql
var migrationFS embed.FS

// Series identifies which database's migration series to apply.
type Series string

const (
	Rules      Series = "rules"
	Characters Series = "characters"
)

// Apply brings db up to date with the given migration series. It is safe to
// call on every startup; already-applied migrations are skipped.
func Apply(db *sql.DB, series Series) error {
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enabling foreign keys: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	names, err := migrationNames(series)
	if err != nil {
		return err
	}

	for _, name := range names {
		var applied int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		body, err := migrationFS.ReadFile("migrations/" + string(series) + "/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for %s: %w", name, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (filename) VALUES (?)`, name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}
	}
	return nil
}

// LatestMigration returns the newest (highest-sorted) filename in a series,
// or "" if the series has no migrations at all (never happens for either
// series in practice, but handled rather than panicking on an empty slice).
//
// internal/autoupdate stamps it on a book after each successful ingest, and
// the translation bundles are named by it. Whether a book needs re-ingesting
// is decided by RequiredIngestVersion, not by this.
func LatestMigration(series Series) (string, error) {
	names, err := migrationNames(series)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	return names[len(names)-1], nil
}

// legacyReingestCutoff is the last rules migration from before re-ingest
// became opt-in. Every migration up to and including it still counts as
// requiring a re-ingest of every parsed book, so an install whose books were
// last ingested before one of those migrations still catches up.
const legacyReingestCutoff = "0080_bingo_book_npc_templates.sql"

// reingestMarker opens the header line a migration uses to declare that its
// change can only reach existing installs through a real re-parse of a
// sourcebook: "-- reingest: book/jutsu-compendium, book/clan-compendium", or
// "-- reingest: all". A migration without it (a schema change, or a data fix
// that backfills itself in SQL) never triggers a download or re-ingest.
const reingestMarker = "-- reingest:"

// RequiredIngestVersion returns the newest migration filename that requires
// bookSlug to be re-ingested: the latest migration naming it (or "all") in
// a reingest marker, or legacyReingestCutoff if none newer does. A book
// whose stored ingest_schema_version sorts below this is stale. Filenames
// are zero-padded, so string order is apply order.
func RequiredIngestVersion(series Series, bookSlug string) (string, error) {
	names, err := migrationNames(series)
	if err != nil {
		return "", err
	}
	required := ""
	for _, name := range names {
		if name <= legacyReingestCutoff {
			required = name
			continue
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+string(series)+"/"+name)
		if err != nil {
			return "", fmt.Errorf("reading migration %s: %w", name, err)
		}
		if migrationReingests(string(body), bookSlug) {
			required = name
		}
	}
	return required, nil
}

// migrationReingests reports whether a migration's reingest marker names
// bookSlug or "all".
func migrationReingests(body, bookSlug string) bool {
	for _, line := range strings.Split(body, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), reingestMarker)
		if !ok {
			continue
		}
		for _, slug := range strings.Split(rest, ",") {
			if s := strings.TrimSpace(slug); s == "all" || s == bookSlug {
				return true
			}
		}
	}
	return false
}

// migrationNames lists a series' .sql files in apply order.
func migrationNames(series Series) ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations/"+string(series))
	if err != nil {
		return nil, fmt.Errorf("listing %s migrations: %w", series, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
