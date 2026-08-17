package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/sergio/n5e/internal/embedded"
	"github.com/sergio/n5e/internal/schema"
)

// openRulesDB opens (or creates) a persistent, writable rules.db next to
// the executable — the same "portable folder" pattern characters.db already
// uses. On first run (no such file yet) it's seeded from the embedded
// snapshot (internal/embedded.RulesDB, frozen at build time); an
// already-updated on-disk copy from a previous run, or a previous app
// version, is never overwritten.
//
// This replaces an earlier design that extracted the embedded bytes to a
// throwaway temp file and opened it read-only/immutable on every launch.
// That worked fine for a purely reference-only rules database, but left
// nowhere for the startup auto-updater (internal/autoupdate) to actually
// persist anything it downloads and re-ingests — a writable, on-disk
// rules.db is what makes that possible.
func openRulesDB(rulesDBPath string) (*sql.DB, error) {
	if _, err := os.Stat(rulesDBPath); os.IsNotExist(err) {
		if err := os.WriteFile(rulesDBPath, embedded.RulesDB, 0o644); err != nil {
			return nil, fmt.Errorf("seeding %s: %w", rulesDBPath, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("checking %s: %w", rulesDBPath, err)
	}

	// busy_timeout: same reasoning as characters.db's own sql.Open in
	// main.go — every request handler only ever reads rulesDB today, but
	// the background auto-update goroutine now writes to it, so a request
	// and an in-progress update can genuinely overlap.
	db, err := sql.Open("sqlite", rulesDBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", rulesDBPath, err)
	}
	// Idempotent, same as every other schema.Apply call in this codebase —
	// brings an older on-disk rules.db (from a previous app version) up to
	// date with any new migrations on every startup.
	if err := schema.Apply(db, schema.Rules); err != nil {
		db.Close()
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	return db, nil
}
