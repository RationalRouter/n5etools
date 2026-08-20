package main

import (
	"database/sql"
	"os"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	_ "modernc.org/sqlite"
)

// TestParseFeatSkillProficiency pins known real-corpus cases.
func TestParseFeatSkillProficiency(t *testing.T) {
	cases := []struct {
		name        string
		slug        string
		description string
		want        string
		wantOK      bool
	}{
		{
			name:        "period-terminated, with skill suffix",
			description: "You gain proficiency in the Acrobatics skill. If you are already proficient, you instead gain Mastery.",
			want:        "Acrobatics",
			wantOK:      true,
		},
		{
			name:        "'proficiency with' preposition, no article",
			description: "You gain proficiency with performance. If you are already proficient, you instead gain Mastery.",
			want:        "Performance",
			wantOK:      true,
		},
		{
			name: "feat/master-of-disguise: skill clause follows an unrelated tool clause",
			slug: "feat/master-of-disguise",
			description: "You gain proficiency with the disguise kit. " +
				"You gain proficiency with performance. If you are already proficient, you instead gain Mastery.",
			want:   "Performance",
			wantOK: true,
		},
		{
			name:        "'proficiency with' a tool/kit is still excluded",
			description: "You gain proficiency with the Alchemist Kit. If you are already proficient, you instead gain Mastery.",
			wantOK:      false,
		},
		{
			name:        "feat/crafter override: Crafting grant buried in a kit-choice sentence",
			slug:        "feat/crafter",
			description: "You gain proficiency in Crafting and either Weaponsmiths or Armorsmith Kit (Pick one). If you are already proficient, you instead gain Mastery.",
			want:        "Crafting",
			wantOK:      true,
		},
		{
			name:        "period-terminated, no skill suffix",
			description: "You gain Proficiency in Illusions. You learn one Malleable Mirage that you qualify for.",
			want:        "Illusions",
			wantOK:      true,
		},
		{
			name:        "comma-terminated variant",
			description: "You gain proficiency in Perception, or +1 ranks of Mastery if already proficient.",
			want:        "Perception",
			wantOK:      true,
		},
		{
			name:        "lowercase mid-word normalizes to canonical casing",
			description: "You gain proficiency in the Chakra control skill. When making a Charisma (Intimidation) check...",
			want:        "Chakra Control",
			wantOK:      true,
		},
		{
			name:        "multi-word skill name",
			description: "You gain proficiency in the Sleight of Hand skill. You can perform a Sleight of hand check as a bonus action.",
			want:        "Sleight of Hand",
			wantOK:      true,
		},
		{
			name:        "excluded — mixed skill+weapon clause",
			description: "You gain proficiency in Martial Weapons and the Martial Arts skill.",
			wantOK:      false,
		},
		{
			name:        "feat/konjiki/steel-weapon-manifestation override: Martial Arts grant buried in a weapon-proficiency sentence",
			slug:        "feat/konjiki/steel-weapon-manifestation",
			description: "You gain proficiency in Martial Weapons and the Martial Arts skill.",
			want:        "Martial Arts",
			wantOK:      true,
		},
		{
			name:        "feat/konjiki/steel-smith override: Crafting grant buried in a tool+Mastery sentence",
			slug:        "feat/konjiki/steel-smith",
			description: "You gain proficiency in Crafting and Weaponsmith kits, gaining +1 ranks of Mastery if already proficient.",
			want:        "Crafting",
			wantOK:      true,
		},
		{
			name:        "excluded — parenthetical tool-or-skill choice",
			description: "You gain proficiency in Investigation or the Forensics Kit (Pick one). If you are already proficient, you instead gain Mastery.",
			wantOK:      false,
		},
		{
			name:        "excluded — tool kit, not a skill",
			description: "You gain Proficiency in Cooking Tools. You gain a Cooking Dice Equal to 1d4.",
			wantOK:      false,
		},
		{
			name:        "excluded — saving throw proficiency",
			description: "You gain proficiency in Strength saving throws while you are submerged in water.",
			wantOK:      false,
		},
		{
			name:        "excluded — choose N skills",
			description: "You may select two skills to gain proficiency in.",
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
			got, ok := parseFeatSkillProficiency(c.slug, c.description)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, c.wantOK, got)
			}
			if ok && got != c.want {
				t.Errorf("skill = %q, want %q", got, c.want)
			}
		})
	}
}

// TestFeatSkillProficiencyMasteryRe pins the three real-corpus phrasings of
// the Mastery-upgrade clause, plus the feat/elite-* shape that must NOT
// match (a separate "You gain Mastery in one X skill... which you already
// have proficiency in" sentence — a player-choice Mastery grant, not a
// conditional upgrade of a proficiency clause this package parses).
func TestFeatSkillProficiencyMasteryRe(t *testing.T) {
	cases := []struct {
		name        string
		description string
		want        bool
	}{
		{
			name:        "majority phrasing: already proficient, you instead gain Mastery",
			description: "You gain proficiency in Acrobatics. If you are already proficient, you instead gain Mastery.",
			want:        true,
		},
		{
			name:        "feat/vesper/echo-location: reversed word order",
			description: "You gain proficiency in Perception, or +1 ranks of Mastery if already proficient.",
			want:        true,
		},
		{
			name:        "feat/yamada/asayama-pills: reversed word order, skill named twice",
			description: "You gain proficiency in Medicine, or +1 ranks of Mastery in Medicine if already proficient.",
			want:        true,
		},
		{
			name:        "feat/konjiki/steel-smith: reversed word order, 'gaining' instead of 'gain'",
			description: "You gain proficiency in Crafting and Weaponsmith kits, gaining +1 ranks of Mastery if already proficient.",
			want:        true,
		},
		{
			name:        "feat/class/witches-training: 'already have Proficiency ... one rank of Mastery'",
			description: "You gain Proficiency in Nature. If you already have Proficiency, you instead gain one rank of Mastery.",
			want:        true,
		},
		{
			name:        "feat/elite-* shape: 'already have proficiency in' is a trailing relative clause, not an upgrade of THIS grant",
			description: "You gain Mastery in one Wisdom skill which you already have proficiency in.",
			want:        false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := featSkillProficiencyMasteryRe.MatchString(c.description); got != c.want {
				t.Errorf("match = %v, want %v", got, c.want)
			}
		})
	}
}

// TestFeatSkillProficiencyCorpusCoverage runs the parser over every feat in
// the real rules database whose description mentions granting proficiency,
// and asserts that every match resolves to a real charsheet.SkillAbility
// key. Unlike the ability-score parser's corpus test, most candidates here
// are expected NOT to match (weapon/tool/saving-throw/choice-of-N clauses
// are deliberately out of scope — see the plan doc) — this test only
// catches a match that resolves to something that isn't a real skill name,
// which would mean a mis-applied proficiency.
//
// Skips when the maintainer-built database isn't present, matching the
// whole-book regression tests elsewhere in this repo (see
// internal/charstore/clanasi_test.go).
func TestFeatSkillProficiencyCorpusCoverage(t *testing.T) {
	const dbPath = "../../out/rules.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("out/rules.db not built; skipping whole-database feat check")
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT slug, description FROM feats
		WHERE description LIKE '%roficiency in%' OR description LIKE '%roficiency with%'
		ORDER BY slug`)
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
		skill, ok := parseFeatSkillProficiency(slug, description)
		if !ok {
			continue
		}
		matched++
		if _, valid := charsheet.SkillAbility[skill]; !valid {
			t.Errorf("%s: parsed skill %q is not a real charsheet.SkillAbility key", slug, skill)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no candidate feats read from the database")
	}
	if matched == 0 {
		t.Fatal("no feats matched at all — regex is likely broken")
	}
	t.Logf("%d/%d candidate feats matched a clean fixed-skill grant (rest are out-of-scope clause shapes, documented in FEAT_AUDIT.md)", matched, checked)
}
