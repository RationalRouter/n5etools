package main

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// TestParseFeatWeaponProficiency pins known real-corpus cases.
func TestParseFeatWeaponProficiency(t *testing.T) {
	cases := []struct {
		name        string
		description string
		want        string
		wantOK      bool
	}{
		{
			name:        "compound clause, category before a joined skill grant",
			description: "You gain proficiency in Martial Weapons and the Martial Arts skill.",
			want:        "Martial Weapons",
			wantOK:      true,
		},
		{
			name:        "period-terminated, no trailing clause",
			description: "You gain proficiency in Simple Weapons. You are considered trained.",
			want:        "Simple Weapons",
			wantOK:      true,
		},
		{
			name:        "lowercase mid-word normalizes to canonical casing",
			description: "You gain proficiency in exotic weapons of your homeland.",
			want:        "Exotic Weapons",
			wantOK:      true,
		},
		{
			name:        "excluded — comma-separated list of individual weapons",
			description: "You gain proficiency in Kunai, Shuriken, and Senbon, if you were not already. These Weapons become known as Assassin Weapons.",
			wantOK:      false,
		},
		{
			name:        "excluded — choice between named equipment sets",
			description: "You gain proficiency in one set of equipment as these weapons become known as Bullseye Weapons.",
			wantOK:      false,
		},
		{
			name:        "excluded — N weapons of your choice, uses proficiency with",
			description: "You gain proficiency with four weapons of your choice.",
			wantOK:      false,
		},
		{
			name:        "excluded — companion grant, not the character's own",
			description: "Your Nin-Dog gains proficiency in simple and martial weapons.",
			wantOK:      false,
		},
		{
			name:        "excluded — skill proficiency, not a weapon",
			description: "You gain proficiency in the Acrobatics skill. If you are already proficient, you instead gain Mastery.",
			wantOK:      false,
		},
		{
			name:        "no clause at all",
			description: "Increase your Dexterity score by 1, to a maximum of 20.",
			wantOK:      false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseFeatWeaponProficiency(c.description)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, c.wantOK, got)
			}
			if ok && got != c.want {
				t.Errorf("category = %q, want %q", got, c.want)
			}
		})
	}
}

// TestFeatWeaponProficiencyCorpusCoverage runs the parser over every feat in
// the real rules database whose description mentions granting proficiency
// alongside a weapon-related word, and asserts that every match resolves to
// a real weaponCategoryLookup value. Like the skill parser's own corpus
// test, most candidates here are expected NOT to match — a named-weapon
// list, an equipment-set choice, and "N weapons of your choice" are all
// deliberately out of scope (see FEAT_AUDIT.md's weapon-set follow-up
// candidate) — this test only catches a match that resolves to something
// that isn't a real category, which would mean a mis-applied proficiency.
//
// Skips when the maintainer-built database isn't present, matching
// TestFeatSkillProficiencyCorpusCoverage's own precedent.
func TestFeatWeaponProficiencyCorpusCoverage(t *testing.T) {
	const dbPath = "../../out/rules.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("out/rules.db not built; skipping whole-database feat check")
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT slug, description FROM feats WHERE description LIKE '%roficiency in%' AND description LIKE '%eapon%' ORDER BY slug`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var checked, matched int
	for rows.Next() {
		var slug, description string
		if err := rows.Scan(&slug, &description); err != nil {
			t.Fatal(err)
		}
		checked++
		category, ok := parseFeatWeaponProficiency(description)
		if !ok {
			continue
		}
		matched++
		valid := false
		for _, canon := range weaponCategoryLookup {
			if canon == category {
				valid = true
				break
			}
		}
		if !valid {
			t.Errorf("%s: parsed category %q is not a real weaponCategoryLookup value", slug, category)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no candidate feats read from the database")
	}
	t.Logf("%d/%d candidate feats matched a clean fixed-weapon-category grant (rest are out-of-scope clause shapes, documented in FEAT_AUDIT.md)", matched, checked)
}
