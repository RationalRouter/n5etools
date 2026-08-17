// Package validate produces the human-facing report that closes out an
// ingestion pass: a plain summary of everything that needs a maintainer's
// eyes. It never touches parsed data — only reads what the loaders already
// wrote, plus (for the Sheet1 cross-check) the community Mastersheet's own
// flat entity lists as an independent second opinion.
package validate

import (
	"database/sql"
	"fmt"
	"sort"
)

// TableCount is one content table's needs_review tally.
type TableCount struct {
	Table string
	Count int
}

// Report is the full validation pass.
type Report struct {
	NeedsReview      []TableCount // only tables with count > 0
	TotalNeedsReview int
	CrossCheck       []CrossCheckResult
}

// needsReviewTables is every content table with a detection_status column —
// queried from the schema itself (not hand-maintained) so a forgotten entry
// here can't silently hide a table's needs_review rows.
func needsReviewTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT m.name FROM sqlite_master m
		JOIN pragma_table_info(m.name) p ON p.name = 'detection_status'
		WHERE m.type = 'table'
		ORDER BY m.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// Run produces the full report: needs_review counts across every content
// table, plus (when sheetPath is non-empty) the Sheet1 name cross-check.
func Run(db *sql.DB, sheetPath string) (*Report, error) {
	tables, err := needsReviewTables(db)
	if err != nil {
		return nil, fmt.Errorf("listing content tables: %w", err)
	}

	r := &Report{}
	for _, table := range tables {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM ` + table + ` WHERE detection_status = 'needs_review'`,
		).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting needs_review in %s: %w", table, err)
		}
		if n > 0 {
			r.NeedsReview = append(r.NeedsReview, TableCount{Table: table, Count: n})
		}
		r.TotalNeedsReview += n
	}
	sort.Slice(r.NeedsReview, func(i, j int) bool { return r.NeedsReview[i].Table < r.NeedsReview[j].Table })

	if sheetPath != "" {
		cc, err := CrossCheckSheet1(db, sheetPath)
		if err != nil {
			return nil, fmt.Errorf("Sheet1 cross-check: %w", err)
		}
		r.CrossCheck = cc
	}

	return r, nil
}
