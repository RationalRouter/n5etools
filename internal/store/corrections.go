package store

import (
	"database/sql"

	"github.com/sergio/n5e/internal/correct"
)

// WriteTextCorrections appends a Sweep's Records to the text_corrections
// audit table. It is an audit log, not authoritative content, so it runs
// in its own transaction separate from whatever LoadXxx call already
// committed the corrected entities themselves. Re-running against
// unchanged text logs the same rows again harmlessly — INSERT OR IGNORE
// against the table's UNIQUE constraint.
func WriteTextCorrections(db *sql.DB, book SourceBook, records []correct.Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO text_corrections
			(source_book, source_version, entity_type, entity_path, field, tool, rule_id, original, corrected)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		var ruleID any
		if r.RuleID != "" {
			ruleID = r.RuleID
		}
		if _, err := stmt.Exec(book.Slug, book.Version, r.EntityType, r.EntityPath, r.Field, r.Tool, ruleID, r.Original, r.Corrected); err != nil {
			return err
		}
	}
	return tx.Commit()
}
