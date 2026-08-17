// Loading core book multiclassing rules into class_multiclass_rules.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadMulticlassRules upserts one row per class. A rule naming a class not
// yet loaded from the class compendium is skipped loudly rather than
// creating an orphan row (class_slug is a foreign key).
func LoadMulticlassRules(db *sql.DB, book SourceBook, rules []parse.MulticlassRule) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	report := &LoadReport{}
	seen := map[string]string{}
	for _, r := range rules {
		classSlug := "class/" + Slugify(r.ClassName)
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM classes WHERE slug = ?`,
			classSlug).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == 0 {
			report.Skipped = append(report.Skipped, fmt.Sprintf(
				"%s: names unknown class %q — load classes first?", classSlug, r.ClassName))
			continue
		}
		seen[classSlug] = r.ClassName
		outcome, err := upsertMulticlassRule(tx, book, classSlug, r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", classSlug, err)
		}
		report.count(outcome, classSlug)
	}

	// findVanished assumes a "slug" column; this table's key is class_slug
	// (one row per class, no separate synthetic slug needed), so the scan
	// is inlined rather than widening the shared helper for one caller.
	rows, err := tx.Query(
		`SELECT class_slug FROM class_multiclass_rules WHERE source_book = ?`, book.Slug)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := seen[slug]; !ok {
			report.Vanished = append(report.Vanished, slug)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM class_multiclass_rules WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertMulticlassRule(tx *sql.Tx, book SourceBook, classSlug string, r parse.MulticlassRule) (rowOutcome, error) {
	var old struct {
		ability, profs, jutsu sql.NullString
		status                string
	}
	err := tx.QueryRow(`
		SELECT ability_prereq_text, proficiencies_text, jutsu_per_level_text, detection_status
		FROM class_multiclass_rules WHERE class_slug = ?`, classSlug).Scan(
		&old.ability, &old.profs, &old.jutsu, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO class_multiclass_rules (class_slug, ability_prereq_text,
			                                    proficiencies_text, jutsu_per_level_text,
			                                    source_book, source_version, source_page,
			                                    detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'auto')`,
			classSlug, r.AbilityPrereq, r.ProficienciesGained, r.JutsuPerLevel,
			book.Slug, book.Version, r.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.ability.String != r.AbilityPrereq ||
		old.profs.String != r.ProficienciesGained ||
		old.jutsu.String != r.JutsuPerLevel
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE class_multiclass_rules
		SET ability_prereq_text = ?, proficiencies_text = ?, jutsu_per_level_text = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE class_slug = ?`,
		r.AbilityPrereq, r.ProficienciesGained, r.JutsuPerLevel,
		book.Slug, book.Version, r.SourcePage, newStatus, classSlug)
	return outcome, err
}
