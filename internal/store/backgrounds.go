// Loading core book Chapter 3 backgrounds.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sergio/n5e/internal/parse"
)

// LoadBackgrounds upserts the parsed backgrounds and rebuilds their
// proficiency rows.
func LoadBackgrounds(db *sql.DB, book SourceBook, backgrounds []parse.Background) (*LoadReport, error) {
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
	for _, b := range backgrounds {
		slug := "background/" + Slugify(b.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, b.Name))
			continue
		}
		seen[slug] = b.Name
		outcome, err := upsertBackground(tx, book, slug, b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
		if err := rebuildBackgroundProfs(tx, slug, b); err != nil {
			return nil, fmt.Errorf("%s proficiencies: %w", slug, err)
		}
	}

	if err := findVanished(tx, "backgrounds", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM backgrounds WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertBackground(tx *sql.Tx, book SourceBook, slug string, b parse.Background) (rowOutcome, error) {
	var old struct {
		description, featureName, featureText, asiText sql.NullString
		equipment, equipmentPack                       sql.NullString
		status                                         string
	}
	err := tx.QueryRow(`
		SELECT description, feature_name, feature_text, asi_text,
		       equipment_text, equipment_pack_text, detection_status
		FROM backgrounds WHERE slug = ?`, slug).Scan(
		&old.description, &old.featureName, &old.featureText, &old.asiText,
		&old.equipment, &old.equipmentPack, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO backgrounds (slug, name, description, feature_name,
			                         feature_text, asi_text, equipment_text,
			                         equipment_pack_text, source_book,
			                         source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, b.Name, b.Description, b.FeatureName, b.FeatureText, b.ASIText,
			b.Equipment, b.EquipmentPack, book.Slug, book.Version, b.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.description.String != b.Description ||
		old.featureName.String != b.FeatureName ||
		old.featureText.String != b.FeatureText ||
		old.asiText.String != b.ASIText ||
		old.equipment.String != b.Equipment ||
		old.equipmentPack.String != b.EquipmentPack
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE backgrounds
		SET name = ?, description = ?, feature_name = ?, feature_text = ?,
		    asi_text = ?, equipment_text = ?, equipment_pack_text = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		b.Name, b.Description, b.FeatureName, b.FeatureText, b.ASIText,
		b.Equipment, b.EquipmentPack, book.Slug, book.Version, b.SourcePage,
		newStatus, slug)
	return outcome, err
}

// rebuildBackgroundProfs replaces the proficiency rows. Plain comma lists
// split into one row per value; choice text ("Choose two from …", "One of
// your choice", "History and Deception or Persuasion") stays as one verbatim
// row for the creation flow to present.
func rebuildBackgroundProfs(tx *sql.Tx, slug string, b parse.Background) error {
	if _, err := tx.Exec(
		`DELETE FROM background_proficiencies WHERE background_slug = ?`, slug); err != nil {
		return err
	}
	insert := func(kind, raw string) error {
		raw = strings.TrimSuffix(strings.TrimSpace(raw), ".")
		if raw == "" {
			return nil
		}
		values := []string{raw}
		if !strings.Contains(raw, "Choose") && !strings.Contains(raw, "choice") &&
			!strings.Contains(raw, " or ") {
			values = strings.Split(raw, ",")
		}
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO background_proficiencies (background_slug, kind, value)
				VALUES (?, ?, ?)`, slug, kind, v); err != nil {
				return err
			}
		}
		return nil
	}
	if err := insert("skill", b.SkillProfs); err != nil {
		return err
	}
	return insert("tool", b.ToolProfs)
}
