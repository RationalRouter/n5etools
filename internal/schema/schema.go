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
// This is the one signal internal/autoupdate needs to detect "this binary's
// own parser/schema logic has moved on since a book was last ingested, even
// though the book's own Drive copy hasn't changed" — every change that adds
// a table needing a content backfill ships as a new migration file, so the
// latest filename already embeds a real, zero-maintenance version number
// with no separate constant to remember to bump.
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
