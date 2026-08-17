package main

import "testing"

// Every description below is a verbatim class_equipment_options.description
// value from rules.db. The count matters as much as the category: the row for
// "any two simple weapons" carries quantity 1, so a reading that trusted the
// quantity column would render one dropdown where the book grants two.
func TestWeaponChoiceOption(t *testing.T) {
	cases := []struct {
		desc     string
		category string
		count    int
		ok       bool
	}{
		{"1 Simple Weapon", "simple", 1, true},
		{"1 Simple weapon", "simple", 1, true},
		{"One Simple Weapon", "simple", 1, true},
		{"One Simple weapon", "simple", 1, true},
		{"2 Simple Weapons", "simple", 2, true},
		{"any two simple weapons", "simple", 2, true},
		{"1 Martial Weapon", "martial", 1, true},

		// Not weapon-category choices — these must fall through to the
		// handling they already had.
		{"2 Kits of your Choice", "", 0, false},
		{"No Armor", "", 0, false},
		{"a Hand crossbow and one stack of bolts", "", 0, false},
		{"Ninjutsu Scroll (D-Rank)", "", 0, false},
		{"Cooking Tools, Flash Tag, Paper Bomb", "", 0, false},
		{"", "", 0, false},
	}

	for _, tc := range cases {
		category, count, ok := weaponChoiceOption(tc.desc)
		if ok != tc.ok || category != tc.category || count != tc.count {
			t.Errorf("weaponChoiceOption(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tc.desc, category, count, ok, tc.category, tc.count, tc.ok)
		}
	}
}

// Guards rules migration 0022: the dropdowns are only as good as the category
// backfill behind them, and a migration that silently stopped applying would
// show up here as empty lists rather than as a wrong-looking creation page.
func TestWeaponCategoriesArePopulated(t *testing.T) {
	srv := testServer(t)

	for _, category := range []string{"simple", "martial", "exotic"} {
		options, err := srv.loadWeaponOptions(category)
		if err != nil {
			t.Fatalf("loadWeaponOptions(%q): %v", category, err)
		}
		if len(options) == 0 {
			t.Errorf("no %s weapons — migration 0022 did not apply", category)
		}
		for _, o := range options {
			if o.Name == "" || o.Slug == "" {
				t.Errorf("%s weapon with empty slug/name: %+v", category, o)
			}
		}
	}

	// The versatile-grip duplicates are the same object as the weapon they
	// duplicate, so offering both as separate picks would read as two
	// different weapons.
	simple, err := srv.loadWeaponOptions("simple")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range simple {
		if o.Slug == "weapon/spear-two-hands" || o.Slug == "weapon/quarterstaff-two-hands" {
			t.Errorf("two-handed duplicate %q offered as its own choice", o.Slug)
		}
	}
}
