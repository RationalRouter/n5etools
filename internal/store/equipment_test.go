package store

import (
	"testing"

	"github.com/sergio/n5e/internal/sheet"
)

func TestLoadWeapArmorCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	maxMod := 7.0
	armors := []sheet.Armor{
		{Name: "Padded Cloth", CalcType: "Light Armor", Ability1: "DEX", Ability2: "NONE",
			MaxMod: &maxMod, ArmorBonus: 1, Bulk: 1},
		{Name: "Skin", CalcType: "Custom", Ability1: "DEX", Ability2: "NONE"},
	}
	weapons := []sheet.Weapon{
		{Name: "Kunai", DamageDice: "1d4", DamageType: "Piercing", Qualities: "Thrown (20/60), Finesse, Light"},
	}
	r, err := LoadWeapArmor(db, mastersheet, armors, weapons)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 3 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadWeapArmor(db, mastersheet, armors, weapons)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 3 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var kind, category, ability1 string
	var maxModOut float64
	if err := db.QueryRow(`SELECT kind, armor_category, armor_ability_1, armor_max_mod
		FROM equipment WHERE slug = 'armor/padded-cloth'`).
		Scan(&kind, &category, &ability1, &maxModOut); err != nil {
		t.Fatal(err)
	}
	if kind != "armor" || category != "light" || ability1 != "DEX" || maxModOut != 7.0 {
		t.Errorf("padded cloth row: kind=%s cat=%s ab1=%s max=%v", kind, category, ability1, maxModOut)
	}

	var dmgDice string
	if err := db.QueryRow(`SELECT damage_dice FROM equipment
		WHERE slug = 'weapon/kunai'`).Scan(&dmgDice); err != nil {
		t.Fatal(err)
	}
	if dmgDice != "1d4" {
		t.Errorf("kunai damage dice = %q", dmgDice)
	}
}

// Equipment has no *_override columns (unlike the rest of the schema) —
// it predates that pattern and a human-blessed row has nothing but its
// detection_status to protect. This test only covers the part that still
// applies: a verified row survives a no-op reload, then demotes when the
// sheet's content actually changes.
func TestLoadWeapArmorVerifiedDemotesOnChange(t *testing.T) {
	db := testDB(t)
	armors := []sheet.Armor{{Name: "Combat Armor", CalcType: "Heavy Armor", Ability1: "NONE",
		Ability2: "NONE", ArmorBonus: 6, Bulk: 6}}
	if _, err := LoadWeapArmor(db, mastersheet, armors, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE equipment SET detection_status = 'verified'
		WHERE slug = 'armor/combat-armor'`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWeapArmor(db, mastersheet, armors, nil); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow(`SELECT detection_status FROM equipment
		WHERE slug = 'armor/combat-armor'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "verified" {
		t.Errorf("after no-op reload: status=%s", status)
	}

	changed := []sheet.Armor{{Name: "Combat Armor", CalcType: "Heavy Armor", Ability1: "NONE",
		Ability2: "NONE", ArmorBonus: 8, Bulk: 6}}
	r, err := LoadWeapArmor(db, mastersheet, changed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("demoted = %v", r.Demoted)
	}
	if err := db.QueryRow(`SELECT detection_status FROM equipment
		WHERE slug = 'armor/combat-armor'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" {
		t.Errorf("after content change: status=%s", status)
	}
}
