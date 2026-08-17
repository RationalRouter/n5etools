// Loading the Mastersheet's class progression charts into class_levels.
//
// These numbers exist only as images in the class compendium PDF; the
// community Mastersheet is the agreed authority for them. Same contract as
// every loader: parsed columns only, *_override and human statuses
// preserved, demote on change.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/sheet"
)

// LoadClassLevels upserts one chart per class (20 rows each) and rebuilds
// the per-level resource rows. Classes must already be loaded from the
// class compendium — a chart naming an unknown class is skipped loudly.
func LoadClassLevels(db *sql.DB, book SourceBook, charts []sheet.ClassChart) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	report := &LoadReport{}
	for _, chart := range charts {
		classSlug := "class/" + Slugify(chart.Name)
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM classes WHERE slug = ?`,
			classSlug).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == 0 {
			report.Skipped = append(report.Skipped, fmt.Sprintf(
				"%s: chart names unknown class %q — load classes first?", classSlug, chart.Name))
			continue
		}

		// The chart also states the class dice — cross-check against what
		// the PDF prose parser loaded; a mismatch means one source changed.
		var hitDie, chakraDie sql.NullInt64
		if err := tx.QueryRow(`SELECT hit_die, chakra_die FROM classes WHERE slug = ?`,
			classSlug).Scan(&hitDie, &chakraDie); err != nil {
			return nil, err
		}
		if int(hitDie.Int64) != chart.HitDie || int(chakraDie.Int64) != chart.ChakraDie {
			report.Skipped = append(report.Skipped, fmt.Sprintf(
				"%s: NOT skipped, but dice disagree — book d%d/d%d vs sheet d%d/d%d",
				classSlug, hitDie.Int64, chakraDie.Int64, chart.HitDie, chart.ChakraDie))
		}

		if _, err := tx.Exec(`DELETE FROM class_level_resources WHERE class_slug = ?`,
			classSlug); err != nil {
			return nil, err
		}
		for _, lr := range chart.Levels {
			outcome, err := upsertClassLevel(tx, classSlug, lr)
			if err != nil {
				return nil, fmt.Errorf("%s level %d: %w", classSlug, lr.Level, err)
			}
			report.count(outcome, fmt.Sprintf("%s/level/%d", classSlug, lr.Level))
			for _, res := range lr.Resources {
				if _, err := tx.Exec(`
					INSERT INTO class_level_resources (class_slug, level, resource_name, value)
					VALUES (?, ?, ?, ?)`,
					classSlug, lr.Level, res.Name, res.Value); err != nil {
					return nil, fmt.Errorf("%s level %d resource %s: %w",
						classSlug, lr.Level, res.Name, err)
				}
			}
		}
	}

	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM class_levels WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertClassLevel(tx *sql.Tx, classSlug string, lr sheet.LevelRow) (rowOutcome, error) {
	var jutsuKnown any
	if lr.JutsuKnown != nil {
		jutsuKnown = *lr.JutsuKnown
	}

	var old struct {
		profBonus, jutsuKnown sql.NullInt64
		status                string
	}
	err := tx.QueryRow(`
		SELECT proficiency_bonus, jutsu_known, detection_status
		FROM class_levels WHERE class_slug = ? AND level = ?`,
		classSlug, lr.Level).Scan(&old.profBonus, &old.jutsuKnown, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO class_levels (class_slug, level, proficiency_bonus,
			                          jutsu_known, detection_status)
			VALUES (?, ?, ?, ?, 'auto')`,
			classSlug, lr.Level, lr.ProfBonus, jutsuKnown)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := int(old.profBonus.Int64) != lr.ProfBonus ||
		!nullableIntEq(old.jutsuKnown, lr.JutsuKnown)
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE class_levels
		SET proficiency_bonus = ?, jutsu_known = ?, detection_status = ?
		WHERE class_slug = ? AND level = ?`,
		lr.ProfBonus, jutsuKnown, newStatus, classSlug, lr.Level)
	return outcome, err
}
