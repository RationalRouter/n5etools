// Loading internal/parse/bingobook_freeform.go's slices into the rules
// database: the Adversary Pack 1 tables and glossaries clean enough to
// parse automatically (see that file's package comment for what's
// deliberately excluded and hand-curated instead).
package store

import (
	"database/sql"
	"fmt"

	"github.com/RationalRouter/n5e/internal/parse"
)

// LoadFreeformStrikeTable fully replaces adversary_freeform_strike_table —
// pure derived data, same convention as adversary_freeform_slots.
func LoadFreeformStrikeTable(db *sql.DB, book SourceBook, rows []parse.FreeformStrikeRow) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM adversary_freeform_strike_table`); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.Exec(
			`INSERT INTO adversary_freeform_strike_table (level_min, low, medium, high) VALUES (?, ?, ?, ?)`,
			r.LevelMin, r.Low, r.Medium, r.High); err != nil {
			return fmt.Errorf("level %d: %w", r.LevelMin, err)
		}
	}
	return tx.Commit()
}

// LoadFreeformJutsuDamageTable fully replaces
// adversary_freeform_jutsu_damage_table — pure derived data.
func LoadFreeformJutsuDamageTable(db *sql.DB, book SourceBook, rows []parse.FreeformJutsuDamageRow) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM adversary_freeform_jutsu_damage_table`); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.Exec(
			`INSERT INTO adversary_freeform_jutsu_damage_table
			 (tier, level_min, very_low, low, medium, high, very_high) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.Tier, r.LevelMin, r.VeryLow, r.Low, r.Medium, r.High, r.VeryHigh); err != nil {
			return fmt.Errorf("%s level %d: %w", r.Tier, r.LevelMin, err)
		}
	}
	return tx.Commit()
}

// LoadClassificationModifiers fully replaces
// adversary_classification_modifiers — pure derived data, exactly one row
// per kind (iconic/epic).
func LoadClassificationModifiers(db *sql.DB, book SourceBook, mods []parse.ClassificationModifier) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM adversary_classification_modifiers`); err != nil {
		return err
	}
	for _, m := range mods {
		if _, err := tx.Exec(
			`INSERT INTO adversary_classification_modifiers
			 (kind, hp_chakra_modifier, ac_bonus, attack_save_bonus, ability_points, initiative_bonus)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			m.Kind, m.HPChakraModifier, m.ACBonus, m.AttackSaveBonus, m.AbilityPoints, m.InitiativeBonus); err != nil {
			return fmt.Errorf("%s: %w", m.Kind, err)
		}
	}
	return tx.Commit()
}

// LoadAdversaryClarifications upserts the fixed 7-term CLARIFICATIONS
// glossary, same upsert-by-slug/detection_status shape as
// adversary_role_traits.
func LoadAdversaryClarifications(db *sql.DB, book SourceBook, terms []parse.AdversaryClarification) (*LoadReport, error) {
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
	for _, t := range terms {
		slug := "adversary-clarification/" + Slugify(t.Term)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, t.Term))
			continue
		}
		seen[slug] = t.Term
		outcome, err := upsertAdversaryClarification(tx, book, slug, t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "adversary_clarifications", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM adversary_clarifications WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertAdversaryClarification(tx *sql.Tx, book SourceBook, slug string, t parse.AdversaryClarification) (rowOutcome, error) {
	var old struct {
		term, description sql.NullString
		status            string
	}
	err := tx.QueryRow(`
		SELECT term, description, detection_status
		FROM adversary_clarifications WHERE slug = ?`, slug).Scan(&old.term, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO adversary_clarifications (slug, term, description, source_book, source_version,
			                                      source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, 'auto')`,
			slug, t.Term, t.Description, book.Slug, book.Version, t.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.term.String != t.Term || old.description.String != t.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE adversary_clarifications
		SET term = ?, description = ?, source_book = ?, source_version = ?,
		    source_page = ?, detection_status = ?
		WHERE slug = ?`,
		t.Term, t.Description, book.Slug, book.Version, t.SourcePage, newStatus, slug)
	return outcome, err
}

// LoadTransformationBuffs upserts the fixed 8-option Transformation Buffs
// menu, same upsert-by-slug/detection_status shape as adversary_role_traits.
func LoadTransformationBuffs(db *sql.DB, book SourceBook, buffs []parse.TransformationBuff) (*LoadReport, error) {
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
	for _, b := range buffs {
		slug := "adversary-transformation-buff/" + Slugify(b.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, b.Name))
			continue
		}
		seen[slug] = b.Name
		outcome, err := upsertTransformationBuff(tx, book, slug, b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "adversary_transformation_buffs", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM adversary_transformation_buffs WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertTransformationBuff(tx *sql.Tx, book SourceBook, slug string, b parse.TransformationBuff) (rowOutcome, error) {
	var old struct {
		name, description sql.NullString
		sortOrder         sql.NullInt64
		status            string
	}
	err := tx.QueryRow(`
		SELECT name, description, sort_order, detection_status
		FROM adversary_transformation_buffs WHERE slug = ?`, slug).Scan(
		&old.name, &old.description, &old.sortOrder, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO adversary_transformation_buffs (slug, name, description, sort_order,
			                                            source_book, source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, b.Name, b.Description, b.SortOrder, book.Slug, book.Version, b.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != b.Name || old.description.String != b.Description ||
		old.sortOrder.Int64 != int64(b.SortOrder)
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE adversary_transformation_buffs
		SET name = ?, description = ?, sort_order = ?, source_book = ?, source_version = ?,
		    source_page = ?, detection_status = ?
		WHERE slug = ?`,
		b.Name, b.Description, b.SortOrder, book.Slug, book.Version, b.SourcePage, newStatus, slug)
	return outcome, err
}
