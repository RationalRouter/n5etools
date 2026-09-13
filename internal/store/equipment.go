// Loading the curated weapon and armor rows into the equipment table.
//
// These are the clan/class-specific weapons and armor that appear in no
// printed price table, so no book parser produces them; internal/classcharts
// carries them as this repo's own data. Cost, where a row has it, came from
// the core book's own OCR pass (see migration 0015) and is carried along
// here so a from-scratch rebuild reproduces it.
//
// This loader only ever sets the columns it owns; a core-book load fills in
// anything else on the same slug without touching what this wrote, the same
// override-safe contract as everywhere else.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/classcharts"
	"github.com/sergio/n5e/internal/parse"
)

// LoadCuratedEquipment upserts the curated armor and weapon rows.
func LoadCuratedEquipment(db *sql.DB, book SourceBook, items []classcharts.Item) (*LoadReport, error) {
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
	for _, it := range items {
		if other, dup := seen[it.Slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", it.Slug, other, it.Name))
			continue
		}
		seen[it.Slug] = it.Name

		outcome, err := upsertCuratedItem(tx, book, it)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", it.Slug, err)
		}
		report.count(outcome, it.Slug)

		if it.Kind == "weapon" {
			var raw string
			if it.Properties != nil {
				raw = *it.Properties
			}
			if err := syncEquipmentProperties(tx, it.Slug, raw, report); err != nil {
				return nil, fmt.Errorf("%s properties: %w", it.Slug, err)
			}
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

func upsertCuratedItem(tx *sql.Tx, book SourceBook, it classcharts.Item) (rowOutcome, error) {
	status := it.DetectionStatus
	if status == "" {
		status = "auto"
	}

	var old struct {
		name, kind                          string
		damageDice, damageType, properties  sql.NullString
		weaponCategory, armorCategory       sql.NullString
		ability1, ability2                  sql.NullString
		costRyo, bulk, maxMod               sql.NullFloat64
		acBonus, strengthReq                sql.NullInt64
		status                              string
	}
	err := tx.QueryRow(`
		SELECT name, kind, damage_dice, damage_type, properties, weapon_category,
		       armor_category, armor_ability_1, armor_ability_2,
		       cost_ryo, bulk, armor_max_mod, ac_bonus, strength_req,
		       detection_status
		FROM equipment WHERE slug = ?`, it.Slug).Scan(
		&old.name, &old.kind, &old.damageDice, &old.damageType, &old.properties,
		&old.weaponCategory, &old.armorCategory, &old.ability1, &old.ability2,
		&old.costRyo, &old.bulk, &old.maxMod, &old.acBonus, &old.strengthReq,
		&old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO equipment (slug, name, kind, damage_dice, damage_type,
			                       properties, weapon_category, armor_category,
			                       armor_ability_1, armor_ability_2, cost_ryo,
			                       bulk, armor_max_mod, ac_bonus, strength_req,
			                       source_book, source_version, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			it.Slug, it.Name, it.Kind, str(it.DamageDice), str(it.DamageType),
			str(it.Properties), str(it.WeaponCategory), str(it.ArmorCategory),
			str(it.ArmorAbility1), str(it.ArmorAbility2), flt(it.CostRyo),
			flt(it.Bulk), flt(it.ArmorMaxMod), num(it.ACBonus), num(it.StrengthReq),
			book.Slug, book.Version, status)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name != it.Name || old.kind != it.Kind ||
		!nullableStrEq(old.damageDice, it.DamageDice) ||
		!nullableStrEq(old.damageType, it.DamageType) ||
		!nullableStrEq(old.properties, it.Properties) ||
		!nullableStrEq(old.weaponCategory, it.WeaponCategory) ||
		!nullableStrEq(old.armorCategory, it.ArmorCategory) ||
		!nullableStrEq(old.ability1, it.ArmorAbility1) ||
		!nullableStrEq(old.ability2, it.ArmorAbility2) ||
		!nullableFloatEq(old.costRyo, it.CostRyo) ||
		!nullableFloatEq(old.bulk, it.Bulk) ||
		!nullableFloatEq(old.maxMod, it.ArmorMaxMod) ||
		!nullableIntEq(old.acBonus, it.ACBonus) ||
		!nullableIntEq(old.strengthReq, it.StrengthReq)

	newStatus, outcome := decideStatus(old.status, status, changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE equipment
		SET name = ?, kind = ?, damage_dice = ?, damage_type = ?, properties = ?,
		    weapon_category = ?, armor_category = ?, armor_ability_1 = ?,
		    armor_ability_2 = ?, cost_ryo = ?, bulk = ?, armor_max_mod = ?,
		    ac_bonus = ?, strength_req = ?, source_book = ?, source_version = ?,
		    detection_status = ?
		WHERE slug = ?`,
		it.Name, it.Kind, str(it.DamageDice), str(it.DamageType), str(it.Properties),
		str(it.WeaponCategory), str(it.ArmorCategory), str(it.ArmorAbility1),
		str(it.ArmorAbility2), flt(it.CostRyo), flt(it.Bulk), flt(it.ArmorMaxMod),
		num(it.ACBonus), num(it.StrengthReq), book.Slug, book.Version, newStatus,
		it.Slug)
	return outcome, err
}

// str/num/flt turn a nil pointer into a SQL NULL rather than a zero value —
// a missing ac_bonus must not be stored as 0.
func str(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func num(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func flt(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// syncEquipmentProperties re-derives a weapon's normalized property rows
// from its raw properties text (see internal/parse/weaponprops.go) on every
// load, regardless of whether the row itself changed, so a change to the
// canonicalization map alone (with no change to the raw text) still gets
// picked up on the next ingest. Unrecognized tokens are surfaced via
// report.Skipped rather than silently dropped.
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
