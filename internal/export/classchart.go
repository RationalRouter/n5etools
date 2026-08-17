// Package export generates spreadsheet-friendly output from the rules
// database — deliverables the friend's Google Sheet currently produces by
// hand, now derived straight from parsed book data.
package export

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// classChartHeader matches the Mastersheet's own ClassChartMaster columns,
// with per-class resource columns (which vary class to class — Genjutsu
// Specialist has "Malleable Mirages", Hunter-Nin has "Lethal Attack", ...)
// folded into one uniform "Resources" column instead of ragged per-class
// column blocks, so the whole export opens as one clean table in any
// spreadsheet tool rather than 11 differently-shaped blocks.
var classChartHeader = []string{"Class", "Level", "Proficiency Bonus", "Features", "Resources", "Jutsu Known"}

// WriteClassChart writes one CSV row per class per level (1-20) — the
// ClassChartMaster export the friend's Google Sheet currently requires
// manual formula upkeep to produce.
func WriteClassChart(db *sql.DB, w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(classChartHeader); err != nil {
		return err
	}

	classes, err := db.Query(`SELECT slug, name FROM classes ORDER BY name`)
	if err != nil {
		return err
	}
	defer classes.Close()

	type class struct{ slug, name string }
	var all []class
	for classes.Next() {
		var c class
		if err := classes.Scan(&c.slug, &c.name); err != nil {
			return err
		}
		all = append(all, c)
	}
	if err := classes.Err(); err != nil {
		return err
	}

	for _, c := range all {
		// Collect the level rows fully before running any nested query —
		// querying db again while this cursor is still open risks landing
		// on a different pooled connection, which for an in-memory test DB
		// means a second, completely empty database (file-backed DBs like
		// the real rules.db don't have this problem, but the code shouldn't
		// depend on that).
		levelRows, err := db.Query(`
			SELECT level, proficiency_bonus, jutsu_known
			FROM class_levels WHERE class_slug = ? ORDER BY level`, c.slug)
		if err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
		type levelRow struct {
			level                 int
			profBonus, jutsuKnown sql.NullInt64
		}
		var levels []levelRow
		for levelRows.Next() {
			var lr levelRow
			if err := levelRows.Scan(&lr.level, &lr.profBonus, &lr.jutsuKnown); err != nil {
				levelRows.Close()
				return fmt.Errorf("%s: %w", c.name, err)
			}
			levels = append(levels, lr)
		}
		if err := levelRows.Err(); err != nil {
			levelRows.Close()
			return fmt.Errorf("%s: %w", c.name, err)
		}
		levelRows.Close()

		for _, lr := range levels {
			features, err := classFeaturesAtLevel(db, c.slug, lr.level)
			if err != nil {
				return fmt.Errorf("%s L%d: %w", c.name, lr.level, err)
			}
			resources, err := resourcesAtLevel(db, c.slug, lr.level)
			if err != nil {
				return fmt.Errorf("%s L%d: %w", c.name, lr.level, err)
			}
			row := []string{
				c.name, strconv.Itoa(lr.level), nullIntStr(lr.profBonus),
				features, resources, nullIntStr(lr.jutsuKnown),
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}

		// Some class_features carry no level at all — not a gap, but a real
		// shape in the data: choice sub-options nested under a level-gated
		// parent (Medical-Nin's Doctrine picks, Genjutsu Specialist's
		// Actualized-* choices, ...). Rather than silently drop them from
		// the export, list them in one trailing row per class.
		unleveled, err := classFeaturesAtLevel(db, c.slug, -1)
		if err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
		if unleveled != "" {
			row := []string{c.name, "(unleveled)", "", unleveled, "", ""}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}

	cw.Flush()
	return cw.Error()
}

// classFeaturesAtLevel returns the comma-joined feature names for one
// level, or (when level is -1) the names of features with no level at all —
// see the "(unleveled)" row in WriteClassChart for why those are surfaced
// rather than dropped.
func classFeaturesAtLevel(db *sql.DB, classSlug string, level int) (string, error) {
	var rows *sql.Rows
	var err error
	if level == -1 {
		rows, err = db.Query(`
			SELECT name FROM class_features
			WHERE class_slug = ? AND level IS NULL ORDER BY sort_order`, classSlug)
	} else {
		rows, err = db.Query(`
			SELECT name FROM class_features
			WHERE class_slug = ? AND level = ? ORDER BY sort_order`, classSlug, level)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", err
		}
		names = append(names, n)
	}
	return strings.Join(names, ", "), rows.Err()
}

func resourcesAtLevel(db *sql.DB, classSlug string, level int) (string, error) {
	rows, err := db.Query(`
		SELECT resource_name, value FROM class_level_resources
		WHERE class_slug = ? AND level = ? ORDER BY resource_name`, classSlug, level)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return "", err
		}
		parts = append(parts, name+": "+value)
	}
	return strings.Join(parts, "; "), rows.Err()
}

func nullIntStr(n sql.NullInt64) string {
	if !n.Valid {
		return ""
	}
	return strconv.FormatInt(n.Int64, 10)
}
