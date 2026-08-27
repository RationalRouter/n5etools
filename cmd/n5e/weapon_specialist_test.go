package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// seedWeaponFocusPair inserts one of the four Versatile weapon pairs
// (migration 0021: Quarterstaff, Spear, Taichi, Tetsubo) as two equipment
// rows, plus one unrelated control weapon — the minimal fixture
// loadWeaponFocusTabData/weaponFocusBonusSet actually read from.
func seedWeaponFocusPair(t *testing.T, s *server) {
	t.Helper()
	for _, stmt := range []string{
		`INSERT INTO equipment (slug, name, kind) VALUES ('weapon/tetsubo', 'Tetsubo', 'weapon')`,
		`INSERT INTO equipment (slug, name, kind) VALUES ('weapon/tetsubo-two-hands', 'Tetsubo (Two Hands)', 'weapon')`,
		`INSERT INTO equipment (slug, name, kind) VALUES ('weapon/kunai', 'Kunai', 'weapon')`,
	} {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// TestLoadWeaponFocusTabDataExcludesTwoHandsFromAvailable reproduces the
// audit finding: offering both grip rows of one physical weapon as separate
// Weapon Focus picks reads as two different weapons and lets a player waste
// a slot on one they already have covered.
func TestLoadWeaponFocusTabDataExcludesTwoHandsFromAvailable(t *testing.T) {
	s := testServer(t)
	seedWeaponFocusPair(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Thor')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/weapon-specialist', 1, 0)`,
	); err != nil {
		t.Fatal(err)
	}

	data, err := s.loadWeaponFocusTabData(1, &charsheet.Sheet{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range data.Available {
		if o.Slug == "weapon/tetsubo-two-hands" {
			t.Errorf("Available still offers the (Two Hands) duplicate row %q alongside its one-handed sibling", o.Slug)
		}
	}
	var sawOneHanded bool
	for _, o := range data.Available {
		if o.Slug == "weapon/tetsubo" {
			sawOneHanded = true
		}
	}
	if !sawOneHanded {
		t.Error("Available dropped the one-handed Tetsubo row too — the fix should only exclude the (Two Hands) duplicate, not the weapon itself")
	}

	// A legacy pick recorded against the (Two Hands) slug (as an existing
	// character's data may already be) must still resolve and count as
	// Known, not silently vanish because it's no longer offered fresh.
	if err := charstore.AddWeaponFocus(s.charDB, 1, "weapon/tetsubo-two-hands"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadWeaponFocusTabData(1, &charsheet.Sheet{})
	if err != nil {
		t.Fatal(err)
	}
	if data.Used != 1 {
		t.Errorf("Used = %d, want 1 — a legacy (Two Hands) pick must still count as a used slot", data.Used)
	}
	var sawKnown bool
	for _, o := range data.Known {
		if o.Slug == "weapon/tetsubo-two-hands" {
			sawKnown = true
		}
	}
	if !sawKnown {
		t.Error("a legacy (Two Hands) pick no longer resolves in Known at all")
	}
}

// TestWeaponFocusBonusSetCoversBothGrips reproduces the other half of the
// finding: a Weapon Focus pick recorded against one grip's slug must apply
// its bonus to the equipped item regardless of which of the two grip rows
// is actually in the character's inventory, since they're the same
// physical weapon.
func TestWeaponFocusBonusSetCoversBothGrips(t *testing.T) {
	s := testServer(t)
	seedWeaponFocusPair(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Thor')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/weapon-specialist', 1, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	if err := charstore.AddWeaponFocus(s.charDB, 1, "weapon/tetsubo-two-hands"); err != nil {
		t.Fatal(err)
	}

	set, bonus, err := s.weaponFocusBonusSet(1, &charsheet.Sheet{})
	if err != nil {
		t.Fatal(err)
	}
	if bonus != 1 {
		t.Errorf("bonus = %d, want 1 at level 1", bonus)
	}
	if !set["weapon/tetsubo-two-hands"] {
		t.Error("the picked slug itself is missing from the bonus set")
	}
	if !set["weapon/tetsubo"] {
		t.Error("the sibling one-handed grip slug does not get the bonus — equipping the other grip of the same physical weapon should still be covered")
	}
	if set["weapon/kunai"] {
		t.Error("an unrelated weapon slug incorrectly picked up the bonus")
	}
}

// seedSamuraiFormEnhancedStrikes inserts Samurai Form's Enhanced Strikes
// (13th level) as a subclass_features row — the minimal fixture
// weaponSpecialistEnhancedStrikesDice/buildAttacks read from.
func seedSamuraiFormEnhancedStrikes(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/weapon-specialist', 'Weapon Specialist', 10, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/weapon-specialist/group/weapon-forms', 'class/weapon-specialist', 'Weapon Forms')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/weapon-specialist/group/weapon-forms/samurai-form', 'class/weapon-specialist/group/weapon-forms', 'Samurai Form')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/weapon-specialist/group/weapon-forms/samurai-form/feature/enhanced-strikes',
		 'class/weapon-specialist/group/weapon-forms/samurai-form', 'Enhanced Strikes', 13,
		 'Whenever you hit a creature with a weapon attack using a weapon you have as your Weapon Focus, the creature takes extra damage equal 1 flurry die.', 1)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, damage_dice, damage_type) VALUES
		('weapon/katana', 'Katana', 'weapon', '1d8', 'Slashing'),
		('weapon/kunai', 'Kunai', 'weapon', '1d4', 'Piercing')`,
	); err != nil {
		t.Fatal(err)
	}
}

// TestWeaponSpecialistEnhancedStrikesDice pins Enhanced Strikes' own die
// progression (Weapon Flurry's die size at the character's Weapon
// Specialist class level, per the feature's "equal [to] 1 flurry die"
// text) and its level gate (13th, same as the feature's own row-level in
// subclass_features — no separate MinLevel needed since loadGrantedFeatures
// already enforces the row-level before this function is ever consulted).
func TestWeaponSpecialistEnhancedStrikesDice(t *testing.T) {
	s := testServer(t)
	seedSamuraiFormEnhancedStrikes(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Musashi')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/weapon-specialist', 12, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/weapon-specialist/group/weapon-forms/samurai-form', 3)`); err != nil {
		t.Fatal(err)
	}

	dice, err := s.weaponSpecialistEnhancedStrikesDice(1, &charsheet.Sheet{Level: 12})
	if err != nil {
		t.Fatal(err)
	}
	if dice != "" {
		t.Errorf("dice at 12th level = %q, want \"\" (Enhanced Strikes isn't granted until 13th)", dice)
	}

	if _, err := s.charDB.Exec(`UPDATE character_classes SET levels = 13 WHERE character_id = 1`); err != nil {
		t.Fatal(err)
	}
	dice, err = s.weaponSpecialistEnhancedStrikesDice(1, &charsheet.Sheet{Level: 13})
	if err != nil {
		t.Fatal(err)
	}
	if dice != "1d10" {
		t.Errorf("dice at 13th level = %q, want \"1d10\" (Weapon Flurry's own die size at 13th)", dice)
	}
}

// TestBuildAttacksEnhancedStrikes confirms buildAttacks attaches Enhanced
// Strikes' bonus damage die only to the equipped weapon(s) matching the
// character's own Weapon Focus pick, never to an equipped weapon of a
// different type.
func TestBuildAttacksEnhancedStrikes(t *testing.T) {
	s := testServer(t)
	seedSamuraiFormEnhancedStrikes(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Musashi')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/weapon-specialist', 13, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/weapon-specialist/group/weapon-forms/samurai-form', 3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES
		(1, 'weapon/katana', 1, 1),
		(1, 'weapon/kunai', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := charstore.AddWeaponFocus(s.charDB, 1, "weapon/katana"); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	attacks, err := s.buildAttacks(1, inv, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 2 {
		t.Fatalf("got %d attack rows, want 2", len(attacks))
	}
	for _, row := range attacks {
		switch row.Slug {
		case "weapon/katana":
			if row.BonusDamageDice != "1d10" {
				t.Errorf("Katana (the Weapon Focus pick) BonusDamageDice = %q, want \"1d10\"", row.BonusDamageDice)
			}
			if row.BonusDamageCount != 1 || row.BonusDamageSides != 10 {
				t.Errorf("Katana BonusDamageCount/Sides = %d/%d, want 1/10", row.BonusDamageCount, row.BonusDamageSides)
			}
			if row.BonusDamageLabel != "Enhanced Strikes" {
				t.Errorf("Katana BonusDamageLabel = %q, want \"Enhanced Strikes\"", row.BonusDamageLabel)
			}
		case "weapon/kunai":
			if row.BonusDamageDice != "" {
				t.Errorf("Kunai (not the Weapon Focus pick) BonusDamageDice = %q, want \"\"", row.BonusDamageDice)
			}
		default:
			t.Errorf("unexpected attack row slug %q", row.Slug)
		}
	}
}
