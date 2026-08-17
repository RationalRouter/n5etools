package main

import (
	"database/sql"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// puppetGeneralizedSkillCap reads directly off Generalized Skill's own
// printed text: "select a number of skills equal to 2 + your Intelligence
// Modifier," granted starting at 5th level.
func TestPuppetGeneralizedSkillCap(t *testing.T) {
	sheet := &charsheet.Sheet{Abilities: map[string]charsheet.AbilityScore{
		"int": {Score: 16, Modifier: 3},
	}}
	lowIntSheet := &charsheet.Sheet{Abilities: map[string]charsheet.AbilityScore{
		"int": {Score: 6, Modifier: -2},
	}}
	cases := []struct {
		name  string
		sheet *charsheet.Sheet
		level int
		want  int
	}{
		{"below level 5", sheet, 4, 0},
		{"level 5, +3 int", sheet, 5, 5},
		{"level 20, +3 int", sheet, 20, 5},
		{"floored at 0 for a very low Int", lowIntSheet, 5, 0},
	}
	for _, c := range cases {
		if got := puppetGeneralizedSkillCap(c.sheet, c.level); got != c.want {
			t.Errorf("%s: puppetGeneralizedSkillCap() = %d, want %d", c.name, got, c.want)
		}
	}
}

// resolvePuppetSkills returns every skill in sheet.Skills (not just ones a
// grant or pick touches — "generally, a puppet takes its skill statistics
// from the Puppet Master himself"), defaulting to the character's own
// SkillEntry as-is, with a Generalized Skill pick only improving the NUMBER
// (using the companion's own ability score, with prof bonus added only if
// already proficient — RAW's "may use your skill bonus... though they may
// calculate using their own statistics, if higher" doesn't grant new
// proficiency by itself) and a flat-grant upgrade (Ghillie Coating, etc.)
// making the companion proficient using the CHARACTER's own ability score.
func firstSkill(groups []puppetSkillGroupRow) puppetSkillRow {
	return groups[0].Skills[0]
}

func TestResolvePuppetSkills(t *testing.T) {
	sheet := &charsheet.Sheet{
		ProficiencyBonus: 2,
		Abilities:        map[string]charsheet.AbilityScore{"dex": {Score: 12, Modifier: 1}},
		Skills: []charsheet.SkillEntry{
			{Name: "Stealth", Ability: "dex", Proficient: false, Modifier: 1}, // character's own Dex mod, not proficient
		},
	}
	// Companion's own Dex score of 18 (+4) beats the character's own +1.
	companion := charstore.Companion{Dex: sql.NullInt64{Int64: 18, Valid: true}}

	groups := resolvePuppetSkills(sheet, companion, []string{"Stealth"}, nil, nil)
	sk := firstSkill(groups)
	if sk.Name != "Stealth" {
		t.Fatalf("resolvePuppetSkills() = %+v, want a Stealth row", groups)
	}
	// Not proficient (no flat grant here), so the companion's own raw Dex
	// mod (+4) is compared with NO proficiency bonus added — same as how
	// the character's own un-proficient Stealth (+1, no prof bonus) reads.
	if sk.Proficient {
		t.Errorf("Stealth Proficient = true, want false (Generalized Skill alone grants no new proficiency)")
	}
	if want := 4; sk.Modifier != want {
		t.Errorf("Stealth modifier = %d, want %d (companion's own Dex mod, un-proficient)", sk.Modifier, want)
	}

	// Flat-grant upgrades make the companion proficient using the
	// CHARACTER's own ability score (dex mod +1) plus Proficiency Bonus.
	groups = resolvePuppetSkills(sheet, companion, nil, []string{"class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/ghillie-coating"}, nil)
	sk = firstSkill(groups)
	if sk.Name != "Stealth" || !sk.Proficient {
		t.Fatalf("resolvePuppetSkills() with Ghillie Coating = %+v, want a proficient Stealth row", groups)
	}
	if want := 1 + 2; sk.Modifier != want {
		t.Errorf("Stealth modifier with Ghillie Coating = %d, want %d (character's own Dex mod + Proficiency Bonus)", sk.Modifier, want)
	}

	// Both together: flat grant makes it proficient, THEN Generalized Skill
	// compares against the companion's own Dex mod WITH prof bonus (now
	// that it's proficient) — 4+2=6 beats the flat-grant-only 1+2=3.
	groups = resolvePuppetSkills(sheet, companion, []string{"Stealth"}, []string{"class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/ghillie-coating"}, nil)
	sk = firstSkill(groups)
	if want := 4 + 2; sk.Modifier != want {
		t.Errorf("Stealth modifier with both = %d, want %d", sk.Modifier, want)
	}
}

func TestResolvePuppetSkillsNilSheet(t *testing.T) {
	if got := resolvePuppetSkills(nil, charstore.Companion{}, nil, nil, nil); got != nil {
		t.Errorf("resolvePuppetSkills(nil, ...) = %+v, want nil", got)
	}
}
