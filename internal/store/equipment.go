// Loading the Mastersheet's WeapArmor tab into the equipment table.
//
// Cost and weight are not in this sheet — they come from the core book's
// own Chapter 5 prose (a separate, not-yet-written parser). This loader
// only ever sets the columns it owns (the armor-calc and weapon-stat
// columns); a later core-book load fills cost/weight in on the same slug
// without touching what this loader wrote, same override-safe contract as
// everywhere else.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sergio/n5e/internal/parse"
	"github.com/sergio/n5e/internal/sheet"
)

// armorCategory maps the sheet's "Calculation Type" to the schema's
// armor_category free-text convention.
func armorCategory(calcType string) string {
	switch calcType {
	case "Light Armor":
		return "light"
	case "Medium Armor":
		return "medium"
	case "Heavy Armor":
		return "heavy"
	default:
		return strings.ToLower(strings.ReplaceAll(calcType, " ", "-")) // "Custom" / "Ability Mod"
	}
}

// LoadWeapArmor upserts the sheet's official armor and weapon rows.
func LoadWeapArmor(db *sql.DB, book SourceBook, armors []sheet.Armor, weapons []sheet.Weapon) (*LoadReport, error) {
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
	for _, a := range armors {
		slug := "armor/" + Slugify(a.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, a.Name))
			continue
		}
		seen[slug] = a.Name
		outcome, err := upsertArmor(tx, book, slug, a)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}
	for _, w := range weapons {
		slug := "weapon/" + Slugify(w.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, w.Name))
			continue
		}
		seen[slug] = w.Name
		outcome, err := upsertWeapon(tx, book, slug, w)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
		if err := syncEquipmentProperties(tx, slug, w.Qualities, report); err != nil {
			return nil, fmt.Errorf("%s properties: %w", slug, err)
		}
	}

	if err := findVanished(tx, "equipment",
		"kind IN ('armor','weapon')", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM equipment WHERE detection_status = 'needs_review'
		 AND kind IN ('armor','weapon')`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertArmor(tx *sql.Tx, book SourceBook, slug string, a sheet.Armor) (rowOutcome, error) {
	var old struct {
		name, category, ability1, ability2 sql.NullString
		maxMod                             sql.NullFloat64
		acBonus, bulk                      sql.NullFloat64
		status                             string
	}
	err := tx.QueryRow(`
		SELECT name, armor_category, armor_ability_1, armor_ability_2,
		       armor_max_mod, ac_bonus, bulk, detection_status
		FROM equipment WHERE slug = ?`, slug).Scan(
		&old.name, &old.category, &old.ability1, &old.ability2,
		&old.maxMod, &old.acBonus, &old.bulk, &old.status)

	category := armorCategory(a.CalcType)
	var maxMod any
	if a.MaxMod != nil {
		maxMod = *a.MaxMod
	}

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO equipment (slug, name, kind, armor_category, armor_ability_1,
			                       armor_ability_2, armor_max_mod, ac_bonus, bulk,
			                       source_book, source_version, detection_status)
			VALUES (?, ?, 'armor', ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, a.Name, category, a.Ability1, a.Ability2, maxMod,
			a.ArmorBonus, a.Bulk, book.Slug, book.Version)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != a.Name ||
		old.category.String != category ||
		old.ability1.String != a.Ability1 ||
		old.ability2.String != a.Ability2 ||
		!nullableFloatEq(old.maxMod, a.MaxMod) ||
		old.acBonus.Float64 != a.ArmorBonus ||
		old.bulk.Float64 != a.Bulk
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE equipment
		SET name = ?, armor_category = ?, armor_ability_1 = ?, armor_ability_2 = ?,
		    armor_max_mod = ?, ac_bonus = ?, bulk = ?, source_book = ?,
		    source_version = ?, detection_status = ?
		WHERE slug = ?`,
		a.Name, category, a.Ability1, a.Ability2, maxMod, a.ArmorBonus, a.Bulk,
		book.Slug, book.Version, newStatus, slug)
	return outcome, err
}

// syncEquipmentProperties re-derives a weapon's normalized property rows
// from its raw properties text (see internal/parse/weaponprops.go) on every
// load, regardless of whether the row itself changed — same
// delete-then-reinsert idempotent pattern classlevels.go uses for
// class_level_resources, so a change to the canonicalization map alone
// (with no change to the raw text) still gets picked up on the next
// ingest. Unrecognized tokens are surfaced via report.Skipped rather than
// silently dropped.
func syncEquipmentProperties(tx *sql.Tx, slug, rawProperties string, report *LoadReport) error {
	if _, err := tx.Exec(`DELETE FROM equipment_properties WHERE equipment_slug = ?`, slug); err != nil {
		return err
	}
	props, anomalies := parse.ParseEquipmentProperties(rawProperties, slug)
	for _, a := range anomalies {
		report.Skipped = append(report.Skipped, fmt.Sprintf("%s: %s", slug, a.Problem))
	}
	for _, p := range props {
		if p.PropertySlug == "" {
			continue // unrecognized token, already reported above
		}
		var detail any
		if p.Detail != "" {
			detail = p.Detail
		}
		if _, err := tx.Exec(`
			INSERT INTO equipment_properties (equipment_slug, property_slug, detail)
			VALUES (?, ?, ?)`,
			slug, p.PropertySlug, detail); err != nil {
			return fmt.Errorf("property %s: %w", p.PropertySlug, err)
		}
	}
	return nil
}

func upsertWeapon(tx *sql.Tx, book SourceBook, slug string, w sheet.Weapon) (rowOutcome, error) {
	var old struct {
		name, damageDice, damageType, properties sql.NullString
		status                                   string
	}
	err := tx.QueryRow(`
		SELECT name, damage_dice, damage_type, properties, detection_status
		FROM equipment WHERE slug = ?`, slug).Scan(
		&old.name, &old.damageDice, &old.damageType, &old.properties, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO equipment (slug, name, kind, damage_dice, damage_type,
			                       properties, source_book, source_version, detection_status)
			VALUES (?, ?, 'weapon', ?, ?, ?, ?, ?, 'auto')`,
			slug, w.Name, w.DamageDice, w.DamageType, w.Qualities, book.Slug, book.Version)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != w.Name ||
		old.damageDice.String != w.DamageDice ||
		old.damageType.String != w.DamageType ||
		old.properties.String != w.Qualities
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE equipment
		SET name = ?, damage_dice = ?, damage_type = ?, properties = ?,
		    source_book = ?, source_version = ?, detection_status = ?
		WHERE slug = ?`,
		w.Name, w.DamageDice, w.DamageType, w.Qualities, book.Slug, book.Version,
		newStatus, slug)
	return outcome, err
}
