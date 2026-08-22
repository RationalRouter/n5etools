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
