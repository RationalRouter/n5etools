// Loading the per-class level progression charts into class_levels.
//
// These numbers exist only as images in the class compendium PDF, so no
// parser produces them; internal/classcharts is the project's own curated
// copy and the sole authority for them. Same contract as every loader:
// parsed columns only, *_override and human statuses preserved, demote on
// change.
package store

import (
	"database/sql"
	"fmt"

	"github.com/RationalRouter/n5e/internal/classcharts"
)

// LoadClassLevels upserts one chart per class (20 rows each) and upserts its
// per-level resource rows. Classes must already be loaded from the class
// compendium — a chart naming an unknown class is skipped loudly.
func LoadClassLevels(db *sql.DB, book SourceBook, charts []classcharts.Class) (*LoadReport, error) {
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
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM classes WHERE slug = ?`,
			chart.Slug).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == 0 {
			report.Skipped = append(report.Skipped, fmt.Sprintf(
				"%s: chart names unknown class — load classes first?", chart.Slug))
			continue
		}

		// Resource rows are upserted per (level, resource_name), not rebuilt
		// from a blanket delete. Several columns here exist in no printed
		// table and are carried as 'manual' rows (Puppet Master's upgrade
		// tier chart is the bulk of them), and v_class_level_resources reads
		// value_override in preference to value — a delete-then-reinsert
		// pass silently destroyed both.
		seen := make(map[string]bool, len(chart.Levels)*2)
		for _, lr := range chart.Levels {
			outcome, err := upsertClassLevel(tx, chart.Slug, lr)
			if err != nil {
				return nil, fmt.Errorf("%s level %d: %w", chart.Slug, lr.Level, err)
			}
			report.count(outcome, fmt.Sprintf("%s/level/%d", chart.Slug, lr.Level))
			for _, res := range lr.Resources {
				seen[resourceKey(lr.Level, res.Name)] = true
				outcome, err := upsertClassLevelResource(tx, chart.Slug, lr.Level, res)
				if err != nil {
					return nil, fmt.Errorf("%s level %d resource %s: %w",
						chart.Slug, lr.Level, res.Name, err)
				}
				report.count(outcome, fmt.Sprintf("%s/level/%d/%s",
					chart.Slug, lr.Level, res.Name))
			}
		}

		// Rows the chart has stopped listing are reported, never deleted —
		// the same contract findVanished applies to every other table.
		if err := findVanishedResources(tx, chart.Slug, seen, report); err != nil {
			return nil, err
		}
	}

	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM class_levels WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func resourceKey(level int, name string) string {
	return fmt.Sprintf("%d\x00%s", level, name)
}

// upsertClassLevelResource writes one charted resource cell. Only `value` is
// ever written: value_override and a human detection_status are this row's
// own, and survive every re-ingest. A row's curated status ('manual' for the
// columns no printed table carries) is used as the incoming status, so it
// stays marked as hand-maintained rather than being reset to 'auto'.
func upsertClassLevelResource(tx *sql.Tx, classSlug string, level int, res classcharts.Resource) (rowOutcome, error) {
	status := res.Status
	if status == "" {
		status = "auto"
	}

	var old struct {
		value  string
		status string
	}
	err := tx.QueryRow(`
		SELECT value, detection_status FROM class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = ?`,
		classSlug, level, res.Name).Scan(&old.value, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO class_level_resources (class_slug, level, resource_name,
			                                   value, detection_status)
			VALUES (?, ?, ?, ?, ?)`,
			classSlug, level, res.Name, res.Value, status)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	newStatus, outcome := decideStatus(old.status, status, old.value != res.Value)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE class_level_resources SET value = ?, detection_status = ?
		WHERE class_slug = ? AND level = ? AND resource_name = ?`,
		res.Value, newStatus, classSlug, level, res.Name)
	return outcome, err
}

// findVanishedResources reports this class's stored resource rows that the
// incoming chart no longer lists. Reported, never deleted.
func findVanishedResources(tx *sql.Tx, classSlug string, seen map[string]bool, report *LoadReport) error {
	rows, err := tx.Query(`
		SELECT level, resource_name FROM class_level_resources
		WHERE class_slug = ? ORDER BY level, resource_name`, classSlug)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var level int
		var name string
		if err := rows.Scan(&level, &name); err != nil {
			return err
		}
		if !seen[resourceKey(level, name)] {
			report.Vanished = append(report.Vanished,
				fmt.Sprintf("%s/level/%d/%s", classSlug, level, name))
		}
	}
	return rows.Err()
}

// upsertClassLevel writes the effective chart values into the base columns.
// The *_override columns belong to whoever set them and are never written
// here — on an existing database an override still wins in v_class_levels,
// so writing the same effective value underneath it changes no behavior.
func upsertClassLevel(tx *sql.Tx, classSlug string, lr classcharts.Level) (rowOutcome, error) {
	var profBonus, jutsuKnown, highestRank, featuresText any
	if lr.ProficiencyBonus != nil {
		profBonus = *lr.ProficiencyBonus
	}
	if lr.JutsuKnown != nil {
		jutsuKnown = *lr.JutsuKnown
	}
	if lr.HighestRankKnown != nil {
		highestRank = *lr.HighestRankKnown
	}
	if lr.FeaturesText != nil {
		featuresText = *lr.FeaturesText
	}

	var old struct {
		profBonus, jutsuKnown    sql.NullInt64
		highestRank, featuresTxt sql.NullString
		status                   string
	}
	err := tx.QueryRow(`
		SELECT proficiency_bonus, jutsu_known, highest_rank_known,
		       features_text, detection_status
		FROM class_levels WHERE class_slug = ? AND level = ?`,
		classSlug, lr.Level).Scan(&old.profBonus, &old.jutsuKnown,
		&old.highestRank, &old.featuresTxt, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO class_levels (class_slug, level, proficiency_bonus,
			                          jutsu_known, highest_rank_known,
			                          features_text, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, 'auto')`,
			classSlug, lr.Level, profBonus, jutsuKnown, highestRank, featuresText)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := !nullableIntEq(old.profBonus, lr.ProficiencyBonus) ||
		!nullableIntEq(old.jutsuKnown, lr.JutsuKnown) ||
		!nullableStrEq(old.highestRank, lr.HighestRankKnown) ||
		!nullableStrEq(old.featuresTxt, lr.FeaturesText)
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE class_levels
		SET proficiency_bonus = ?, jutsu_known = ?, highest_rank_known = ?,
		    features_text = ?, detection_status = ?
		WHERE class_slug = ? AND level = ?`,
		profBonus, jutsuKnown, highestRank, featuresText, newStatus,
		classSlug, lr.Level)
	return outcome, err
}
