package store

import (
	"database/sql"
)

// OCRAnomaly is one page where an ingest run's independent OCR reading
// disagreed with the text-layer parse enough to flag for review. Mirrors
// internal/ingest.OCRAnomaly's shape; kept as its own type here (rather than
// importing internal/ingest, which already imports internal/store) to avoid
// an import cycle.
type OCRAnomaly struct {
	Page     int
	TextLine string
	OCRLine  string
	Ratio    float64
}

// WriteOCRAnomalies appends an ingest run's OCR cross-validation results to
// the ocr_anomalies audit table. Same contract as WriteTextCorrections: an
// audit log, not authoritative content, in its own transaction, safe to
// re-run against unchanged content via INSERT OR IGNORE on the table's
// UNIQUE constraint.
func WriteOCRAnomalies(db *sql.DB, book SourceBook, anomalies []OCRAnomaly) error {
	if len(anomalies) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO ocr_anomalies
			(source_book, source_version, page, text_excerpt, ocr_excerpt, similarity)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range anomalies {
		if _, err := stmt.Exec(book.Slug, book.Version, a.Page, a.TextLine, a.OCRLine, a.Ratio); err != nil {
			return err
		}
	}
	return tx.Commit()
}
