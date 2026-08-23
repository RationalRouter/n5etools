package main

import "testing"

// TestBeyondTheVeilBulkBonus covers Elemental Innovationist's 6th-level
// Beyond the Veil: a character who has reached the feature gets the Aether
// Connector's flat +100 max-bulk bonus; one who hasn't (wrong subclass, or
// not yet 6th level in it) gets nothing. Mirrors
// TestLoadGrantedFeaturesIncludesSubclass's own rules.db seeding shape
// (subclass_groups -> subclasses -> subclass_features, gated by the parent
// class's real character_classes.levels row).
func TestBeyondTheVeilBulkBonus(t *testing.T) {
	elementalInnovationist := "class/science-nin/group/scientific-inquiry/elemental-innovationist"
	grenadier := "class/science-nin/group/scientific-inquiry/grenadier"

	cases := []struct {
		name          string
		subclassSlug  string
		scienceNinLvl int
		wantBonus     float64
	}{
		{"Elemental Innovationist at 6th grants +100", elementalInnovationist, 6, 100},
		{"Elemental Innovationist above 6th still grants +100", elementalInnovationist, 9, 100},
		{"Elemental Innovationist below 6th grants nothing", elementalInnovationist, 5, 0},
		{"no subclass picked grants nothing", "", 6, 0},
		{"different subclass at 6th grants nothing", grenadier, 6, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)

			mustExecRules := func(query string, args ...any) {
				t.Helper()
				if _, err := s.rulesDB.Exec(query, args...); err != nil {
					t.Fatalf("seed rules: %v (%s)", err, query)
				}
			}
			mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
			mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
				('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
			mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
				('class/science-nin/group/scientific-inquiry/elemental-innovationist', 'class/science-nin/group/scientific-inquiry', 'Elemental Innovationist')`)
			mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
				('class/science-nin/group/scientific-inquiry/grenadier', 'class/science-nin/group/scientific-inquiry', 'Grenadier')`)
			mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
				('class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/beyond-the-veil',
				 'class/science-nin/group/scientific-inquiry/elemental-innovationist', 'Beyond the Veil', 6,
				 'Starting at 6th level, you gain access to a new SNT known as the Aether Connector.', 1)`)

			res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
				VALUES ('Beyond the Veil Test', 10, 10, 10, 10, 10, 10)`)
			if err != nil {
				t.Fatal(err)
			}
			characterID, _ := res.LastInsertId()
			if _, err := s.charDB.Exec(
				`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
				characterID, c.scienceNinLvl,
			); err != nil {
				t.Fatal(err)
			}
			if c.subclassSlug != "" {
				if _, err := s.charDB.Exec(
					`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, ?, 3)`,
					characterID, c.subclassSlug,
				); err != nil {
					t.Fatal(err)
				}
			}

			got, err := s.beyondTheVeilBulkBonus(characterID)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.wantBonus {
				t.Errorf("beyondTheVeilBulkBonus = %v, want %v", got, c.wantBonus)
			}
		})
	}
}
