package main

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// TestParseFeatAbilityIncrease pins known real-corpus cases directly,
// including the Hive Minded bug report this mechanism was built to fix.
func TestParseFeatAbilityIncrease(t *testing.T) {
	cases := []struct {
		name        string
		slug        string
		description string
		wantAbil    []string
		wantAmount  int
		wantOK      bool
	}{
		{
			name: "Hive Minded — two named abilities",
			slug: "feat/aburame/hive-minded",
			description: "Your insects treat you and your body as a hive, it's queen, and its army. You gain the following benefits; " +
				"• Increase your Intelligence or Wisdom score by 1, to a maximum of 20. " +
				"• When you would make a check to maintain concentration on an Aburame clan Jutsu, you may instead make an Intelligence (Chakra Control) check.",
			wantAbil:   []string{"int", "wis"},
			wantAmount: 1,
			wantOK:     true,
		},
		{
			name:        "single fixed ability",
			slug:        "feat/acrobat",
			description: "You become nimbler, gaining the following benefits: • Increase your Dexterity score by 1, to a maximum of 20. • You gain something else.",
			wantAbil:    []string{"dex"},
			wantAmount:  1,
			wantOK:      true,
		},
		{
			name:        "override — free choice of any ability",
			slug:        "feat/advanced-study",
			description: "• Increase one Ability score by 1, to a maximum of 20 • You learn an additional jutsu...",
			wantAbil:    []string{"str", "dex", "con", "int", "wis", "cha"},
			wantAmount:  1,
			wantOK:      true,
		},
		{
			name:        "excluded — raises a summoned Puppet's stat, not the character's",
			slug:        "feat/class/puppeteer-training",
			description: "This Puppet increases one ability score by +2, or two ability scores by +1.",
			wantOK:      false,
		},
		{
			name:        "excluded — Speed increase false positive from the loose prefilter",
			slug:        "feat/class/swifter-response",
			description: "During the first round of initiative, increase your speed by 15 feet. If you score a hit against a creature...",
			wantOK:      false,
		},
		{
			name:        "no clause at all",
			slug:        "feat/some-other-feat",
			description: "You gain proficiency in Stealth.",
			wantOK:      false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotAbil, gotAmount, gotOK := parseFeatAbilityIncrease(c.slug, c.description)
			if gotOK != c.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, c.wantOK)
			}
			if !gotOK {
				return
			}
			if gotAmount != c.wantAmount {
				t.Errorf("amount = %d, want %d", gotAmount, c.wantAmount)
			}
			if len(gotAbil) != len(c.wantAbil) {
				t.Fatalf("abilities = %v, want %v", gotAbil, c.wantAbil)
			}
			for i, a := range c.wantAbil {
				if gotAbil[i] != a {
					t.Errorf("abilities = %v, want %v", gotAbil, c.wantAbil)
					break
				}
			}
		})
	}
}

// TestFeatAbilityIncreaseCorpusCoverage runs the parser over every feat in
// the real rules database whose description contains an "Increase...score"
// clause (the same loose prefilter used during development) and asserts
// every one of them either parses successfully or is a known, hand-verified
// exclusion — catching new feats added to the sourcebook with a clause shape
// the parser doesn't yet handle, rather than silently granting nothing.
//
// Skips when the maintainer-built database isn't present, matching the
// whole-book regression tests elsewhere in this repo (see
// internal/charstore/clanasi_test.go).
func TestFeatAbilityIncreaseCorpusCoverage(t *testing.T) {
	const dbPath = "../../out/rules.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("out/rules.db not built; skipping whole-database feat check")
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT slug, description FROM feats WHERE description LIKE '%Increase%score%' ORDER BY slug`)
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
		abilities, amount, ok := parseFeatAbilityIncrease(slug, description)
		switch {
		case ok:
			matched++
			if len(abilities) == 0 {
				t.Errorf("%s: parsed OK but no abilities returned", slug)
			}
			if amount < 1 {
				t.Errorf("%s: parsed amount %d, want >= 1", slug, amount)
			}
		case featAbilityIncreaseExcluded[slug]:
			// Known non-ability-score clause; correctly unmatched.
		default:
			t.Errorf("%s: description has an ability-increase-shaped clause but the parser didn't match it and it's not in featAbilityIncreaseExcluded — either add a case to the parser or add it to the exclusion list: %s", slug, description)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no candidate feats read from the database")
	}
	t.Logf("%d/%d candidate feats matched (rest are known exclusions)", matched, checked)
}
