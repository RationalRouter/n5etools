// Sheet1 cross-check: the community Mastersheet's Sheet1 holds six flat,
// independently-maintained entity-name lists (see internal/sheet package
// docs for why they're flat, not relational). Comparing our parsed names
// against them is a cheap second opinion — a name in one source and not the
// other is worth a human's eyes, even though it isn't proof of an error on
// either side (the two sources are maintained independently).
//
// The comparison is deliberately approximate, not a strict gate: several
// Sheet1 blocks are catch-alls that mix categories our schema keeps
// separate (its "Clan Features" list mixes leveled features with trait
// labels like "Recommended Ability Score Increase (Aburame)" that we store
// in dedicated columns, not as named rows; its "Class Features" list also
// covers the jutsu-creation cost catalog we deliberately haven't parsed —
// see project notes). Jutsu has its own known noise source: Sheet1 strips
// the "Category: " prefix from compound jutsu names ("Transformation: Bat"
// prints there as just "BAT"), so a chunk of jutsu mismatches are this
// systematic naming difference, not missing data on either side. Feats and
// Latents should line up closely with little to no expected noise.
package validate

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

// CrossCheckResult is one category's comparison against a Sheet1 column.
type CrossCheckResult struct {
	Category       string
	SheetCount     int
	DBCount        int
	InDBNotSheet   []string // capped sample — a DB entity Sheet1 doesn't have
	InSheetNotDB   []string // capped sample — a Sheet1 entity we haven't loaded
	TruncatedDB    bool
	TruncatedSheet bool
}

const sampleCap = 25

var normalizeStripRe = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeName folds a name for comparison: lowercase, strip everything
// but letters/digits. This absorbs the two sources' independent
// transcription differences (apostrophe style, hyphen vs space, punctuation)
// without needing to know every specific variant.
func normalizeName(s string) string {
	return normalizeStripRe.ReplaceAllString(strings.ToLower(s), "")
}

// sheetColumn reads one Sheet1 column as a set of normalized names, keyed
// back to the first original spelling seen.
func sheetColumn(f *excelize.File, col int) (map[string]string, error) {
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for r := 2; r < len(rows); r++ { // row 3 (index 2) is the first data row
		if col >= len(rows[r]) {
			continue
		}
		v := strings.TrimSpace(rows[r][col])
		if v == "" {
			continue
		}
		if n := normalizeName(v); n != "" {
			if _, seen := names[n]; !seen {
				names[n] = v
			}
		}
	}
	return names, nil
}

// dbNames runs a query returning one name per row and normalizes the set,
// same shape as sheetColumn so the two can be diffed directly.
func dbNames(db *sql.DB, query string) (map[string]string, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if n := normalizeName(v); n != "" {
			if _, seen := names[n]; !seen {
				names[n] = v
			}
		}
	}
	return names, rows.Err()
}

// diff compares two normalized-name sets and returns capped samples of the
// entries unique to each side.
func diff(category string, sheet, db map[string]string) CrossCheckResult {
	r := CrossCheckResult{Category: category, SheetCount: len(sheet), DBCount: len(db)}
	for n, orig := range db {
		if _, ok := sheet[n]; !ok {
			if len(r.InDBNotSheet) >= sampleCap {
				r.TruncatedDB = true
				continue
			}
			r.InDBNotSheet = append(r.InDBNotSheet, orig)
		}
	}
	for n, orig := range sheet {
		if _, ok := db[n]; !ok {
			if len(r.InSheetNotDB) >= sampleCap {
				r.TruncatedSheet = true
				continue
			}
			r.InSheetNotDB = append(r.InSheetNotDB, orig)
		}
	}
	return r
}

// Sheet1's flat entity-list columns (0-based), from the sheet's own header
// row — see internal/sheet package docs / project notes for the full layout.
const (
	sheet1ColJutsuName    = 0  // A: "Jutsu Names"
	sheet1ColFeatTitle    = 12 // M: "Feat Titles"
	sheet1ColClanFeature  = 15 // P: Clan Features "Title"
	sheet1ColClassFeature = 17 // R: Class Features "Title"
	sheet1ColLatentName   = 23 // X: "Name of Latent Feature"
)

// CrossCheckSheet1 diffs our loaded entity names against Sheet1's five
// populated flat lists (the sixth, "Class Mod Features & Jutsu", is an
// unused column group in this workbook — nothing to compare).
func CrossCheckSheet1(db *sql.DB, sheetPath string) ([]CrossCheckResult, error) {
	f, err := excelize.OpenFile(sheetPath)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", sheetPath, err)
	}
	defer f.Close()

	type category struct {
		name    string
		col     int
		dbQuery string
	}
	categories := []category{
		{"Jutsu", sheet1ColJutsuName, `SELECT name FROM jutsu`},
		{"Feats & Stances", sheet1ColFeatTitle,
			`SELECT name FROM feats UNION ALL SELECT name FROM fighting_stances`},
		{"Clan Features", sheet1ColClanFeature, `SELECT name FROM clan_features`},
		{"Class/Subclass Features & Options", sheet1ColClassFeature,
			`SELECT name FROM class_features UNION ALL SELECT name FROM subclass_features
			 UNION ALL SELECT name FROM class_options`},
		{"Bloodline Latents", sheet1ColLatentName, `SELECT name FROM bloodline_latents`},
	}

	var results []CrossCheckResult
	for _, c := range categories {
		sheetSet, err := sheetColumn(f, c.col)
		if err != nil {
			return nil, fmt.Errorf("%s: reading Sheet1 column: %w", c.name, err)
		}
		dbSet, err := dbNames(db, c.dbQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: querying DB: %w", c.name, err)
		}
		results = append(results, diff(c.name, sheetSet, dbSet))
	}
	return results, nil
}
