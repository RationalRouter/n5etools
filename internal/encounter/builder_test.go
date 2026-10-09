package encounter

import (
	"reflect"
	"testing"
)

// builderFixture holds the migration 0082 rows the tests touch: levels 3, 8
// and 12, every Rank, and the roles used below.
func builderFixture() BuilderTables {
	return BuilderTables{
		Levels: map[int]LevelStats{
			3:  {Level: 3, Prof: 3, AttackBonus: 5, DamagePerAction: 16, StandardMods: [6]int{2, 1, 1, 0, -1, -3}, EliteMods: [6]int{3, 2, 1, 0, -1, -2}},
			8:  {Level: 8, Prof: 5, AttackBonus: 8, DamagePerAction: 30, StandardMods: [6]int{3, 2, 2, 1, -1, -2}, EliteMods: [6]int{4, 3, 2, 1, 0, -1}},
			12: {Level: 12, Prof: 6, AttackBonus: 10, DamagePerAction: 40, StandardMods: [6]int{4, 3, 3, 2, 0, -1}, EliteMods: [6]int{6, 4, 2, 1, 1, 0}},
		},
		TraitCounts: map[int][4]int{3: {1, 2, 4, 6}, 8: {1, 2, 4, 6}, 12: {2, 3, 6, 8}},
		Ranks: map[string]RankTier{
			"E": {Rank: "E", HitDie: 4, HitAvg: 3, ChakraDie: 4, ChakraAvg: 3, ToHit: -2, SaveDC: -3},
			"D": {Rank: "D", HitDie: 6, HitAvg: 4, ChakraDie: 6, ChakraAvg: 4, ToHit: -1, SaveDC: -2},
			"C": {Rank: "C", HitDie: 8, HitAvg: 5, ChakraDie: 8, ChakraAvg: 5, SaveDC: -1},
			"B": {Rank: "B", HitDie: 10, HitAvg: 6, ChakraDie: 10, ChakraAvg: 6, Defensive: 1, BonusDamage: 5},
			"A": {Rank: "A", HitDie: 12, HitAvg: 7, ChakraDie: 12, ChakraAvg: 7, Defensive: 2, ToHit: 1, SaveDC: 1, BonusDamage: 10},
			"S": {Rank: "S", HitDie: 20, HitAvg: 11, ChakraDie: 20, ChakraAvg: 11, Defensive: 3, ToHit: 2, SaveDC: 2, BonusDamage: 15},
		},
		Roles: map[string]BuilderRole{
			"adversary-builder-role/generalist": {Slug: "adversary-builder-role/generalist", Tag: "Generalist", AC: 1, Save: 1, HP: 1, CP: 1, DamageMult: 1},
			"adversary-builder-role/defender":   {Slug: "adversary-builder-role/defender", Tag: "Defender", AC: -1, Save: 1, HP: 1.4, CP: 0.8, DamageMult: 0.7, Speed: 10, TraitRoleSlug: "adversary-role/defender"},
			"adversary-builder-role/lurker":     {Slug: "adversary-builder-role/lurker", Tag: "Lurker", AC: -2, Save: 1, HP: 0.7, CP: 1.2, DamageMult: 1.3, TraitRoleSlug: "adversary-role/lurker"},
		},
		Clans: map[string]ClanMods{"Uchiha": {Tag: "Uchiha"}},
		AutoBonuses: []AutoBonus{
			{Tag: "Defender", Stat: "dr", Value: 2},
			{Tag: "Lurker", Stat: "speed_flat", Value: 25},
			{Tag: "Lurker", Stat: "note", Note: "Crit range for a Lurkers attacks is increased by 1"},
			{Tag: "Generalist", Stat: "init", Value: 2},
			{Tag: "Generalist", Stat: "all_skills", Value: 2},
		},
		Classes: map[string]Rank{
			"adversary-rank/minion":   {Slug: "adversary-rank/minion", InitBonus: -2},
			"adversary-rank/standard": {Slug: "adversary-rank/standard"},
			"adversary-rank/elite":    {Slug: "adversary-rank/elite", ACBonus: 1, SaveBonus: 1, SaveDCBonus: 1, InitBonus: 2},
			"adversary-rank/solo":     {Slug: "adversary-rank/solo", ACBonus: 2, SaveBonus: 2, SaveDCBonus: 2, InitBonus: 3},
		},
		ClassHP: map[string]ClassHPMod{
			"adversary-rank/minion":   {MinionFlat: true},
			"adversary-rank/standard": {},
			"adversary-rank/elite":    {Base: 1.25},
			"adversary-rank/solo":     {Base: 1.5, PerPlayer: 0.5},
		},
		JutsuKnown: []JutsuKnownRow{
			{"adversary-rank/minion", "*", 1, 1}, {"adversary-rank/minion", "*", 4, 2},
			{"adversary-rank/solo", "D", 1, 2}, {"adversary-rank/solo", "D", 5, 4},
			{"adversary-rank/solo", "C", 5, 2}, {"adversary-rank/solo", "C", 9, 3},
			{"adversary-rank/solo", "B", 6, 1}, {"adversary-rank/solo", "B", 9, 2},
			{"adversary-rank/solo", "A", 9, 1},
		},
		Classifications: []ClassificationModifier{
			{Kind: "iconic", HPChakraModifier: 1, ACBonus: 1, AttackSaveBonus: 1, AbilityPoints: 2, InitiativeBonus: 1},
		},
	}
}

func intp(n int) *int { return &n }

// The Adversary Builder sheet's own example (Adversary tab): a C-Rank Uchiha
// Generalist Solo, level 12, four players, light armor. With the sheet's
// class values in place of Pack 1's, the engine must reproduce the sheet's
// numbers exactly.
func TestComputeMatchesSheetExample(t *testing.T) {
	tb := builderFixture()
	tb.Classes["adversary-rank/solo"] = Rank{Slug: "adversary-rank/solo", ACBonus: 4, InitBonus: 4}
	tb.ClassHP["adversary-rank/solo"] = ClassHPMod{PerPlayer: 0.75}
	// The sheet does not apply the Rank's to-hit or DC bonuses; C-Rank's
	// DC bonus is the only non-zero one involved.
	c := tb.Ranks["C"]
	c.SaveDC = 0
	tb.Ranks["C"] = c

	sb := Compute(BuildChoices{
		ClassSlug: "adversary-rank/solo", Rank: "C", Level: 12, Players: 4,
		RoleSlug: "adversary-builder-role/generalist", ClanTag: "Uchiha",
		AffiliationTag: "Factionless", Armor: "light",
		Abilities: [6]*int{intp(6), intp(4), intp(2), intp(1), intp(1), intp(0)},
		SaveProfs: []string{"dex", "cha"},
	}, tb)

	checks := []struct {
		name      string
		got, want int
	}{
		{"HP", sb.HPMax, 334}, {"CP", sb.CPMax, 334}, {"AC", sb.AC, 25}, {"Init", sb.Init, 13},
		{"DR", sb.DR, 2}, {"Speed", sb.Speed, 30}, {"Prof", sb.Prof, 6},
		{"STR save", sb.Saves[0], 10}, {"DEX save", sb.Saves[1], 11}, {"CON save", sb.Saves[2], 6},
		{"Ninjutsu attack", sb.Attack["ninjutsu"], 7}, {"Weapon attack", sb.WeaponAttack, 12},
		{"Trait slots", sb.TraitSlots, 8}, {"Tenacity", sb.TenacityMax, 12},
	}
	for _, ch := range checks {
		if ch.got != ch.want {
			t.Errorf("%s = %d, want %d", ch.name, ch.got, ch.want)
		}
	}
	if !reflect.DeepEqual(sb.HPPhases, []int{200, 100}) {
		t.Errorf("HP phases = %v, want [200 100]", sb.HPPhases)
	}
}

// Pack 1 class values, worked by hand: Elite C-Rank Defender, level 8,
// heavy armor, abilities auto-assigned (Con 4, Str 3, Dex 2, Wis 1, Int 0,
// Cha -1).
func TestComputePack1Elite(t *testing.T) {
	sb := Compute(BuildChoices{
		ClassSlug: "adversary-rank/elite", Rank: "C", Level: 8, Players: 4,
		RoleSlug: "adversary-builder-role/defender", ClanTag: "Non_Clan", Armor: "heavy",
	}, builderFixture())

	if want := [6]int{3, 2, 4, 0, 1, -1}; sb.Abilities != want {
		t.Fatalf("abilities = %v, want %v", sb.Abilities, want)
	}
	checks := []struct {
		name      string
		got, want int
	}{
		// 10 + 4x8 + 5x8x(1 + 1.4 + 1.25)
		{"HP", sb.HPMax, 188},
		// 10 + 4x8 + 5x8x(1 + 0.8 + 1.25)
		{"CP", sb.CPMax, 164},
		// 10 + 2 + heavy 6 + bracket 1 - 1 role + 1 class
		{"AC", sb.AC, 19},
		// 2 + Dex 2 + Elite 2
		{"Init", sb.Init, 6},
		// 2 + Defender 3 + automatic 2
		{"DR", sb.DR, 7},
		{"Speed", sb.Speed, 40},
		// Con proficient: 4 + 5 + role 1 + class 1
		{"CON save", sb.Saves[2], 11},
		// Dex not proficient: 2 + 2 + 1 + 1
		{"DEX save", sb.Saves[1], 6},
		// round(30 x 0.7 x 1.1)
		{"Damage/action", sb.DamagePerAction, 23},
		{"Ninjutsu DC", sb.DC["ninjutsu"], 10 + 5 + 0 + 1 - 1},
		{"Trait slots", sb.TraitSlots, 4},
		{"Tenacity", sb.TenacityMax, 4},
	}
	for _, ch := range checks {
		if ch.got != ch.want {
			t.Errorf("%s = %d, want %d", ch.name, ch.got, ch.want)
		}
	}
	if !reflect.DeepEqual(sb.HPPhases, []int{112, 56}) {
		t.Errorf("HP phases = %v, want [112 56]", sb.HPPhases)
	}
}

func TestComputeMinionHasFlatPools(t *testing.T) {
	sb := Compute(BuildChoices{
		ClassSlug: "adversary-rank/minion", Rank: "D", Level: 3, Players: 4,
		RoleSlug: "adversary-builder-role/lurker", Armor: "light",
	}, builderFixture())
	if sb.HPMax != 13 || sb.CPMax != 13 {
		t.Errorf("Minion HP/CP = %d/%d, want 13/13", sb.HPMax, sb.CPMax)
	}
	if sb.HPPhases != nil || sb.TenacityMax != 0 {
		t.Errorf("Minion has phases %v / tenacity %d, want none", sb.HPPhases, sb.TenacityMax)
	}
	if sb.Speed != 55 || len(sb.Notes) != 1 {
		t.Errorf("Lurker speed %d notes %v, want 55 and one note", sb.Speed, sb.Notes)
	}
	if !reflect.DeepEqual(sb.Jutsu, []JutsuQuota{{"D", 1}}) {
		t.Errorf("Minion jutsu = %v, want [{D 1}]", sb.Jutsu)
	}
}

func TestComputeAppliesRankBonuses(t *testing.T) {
	choices := BuildChoices{
		ClassSlug: "adversary-rank/standard", Level: 12, Players: 4,
		RoleSlug: "adversary-builder-role/generalist", Armor: "light",
	}
	choices.Rank = "C"
	c := Compute(choices, builderFixture())
	choices.Rank = "S"
	s := Compute(choices, builderFixture())
	if d := s.AC - c.AC; d != 3 {
		t.Errorf("S vs C AC difference = %d, want 3", d)
	}
	if d := s.WeaponAttack - c.WeaponAttack; d != 2 {
		t.Errorf("S vs C attack difference = %d, want 2", d)
	}
	if d := s.DC["genjutsu"] - c.DC["genjutsu"]; d != 3 {
		t.Errorf("S vs C DC difference = %d, want 3", d)
	}
	if d := s.DamagePerAction - c.DamagePerAction; d != 15 {
		t.Errorf("S vs C damage difference = %d, want 15", d)
	}
	if s.HPMax <= c.HPMax {
		t.Errorf("S-Rank HP %d not above C-Rank %d", s.HPMax, c.HPMax)
	}
}

func TestComputeClassificationAndAdjustments(t *testing.T) {
	choices := BuildChoices{
		ClassSlug: "adversary-rank/standard", Rank: "C", Level: 8, Players: 4,
		RoleSlug: "adversary-builder-role/generalist", Armor: "light",
	}
	plain := Compute(choices, builderFixture())
	choices.Classification = "iconic"
	choices.Adjust = Adjustments{HP: 5, AC: -1, Speed: 10}
	iconic := Compute(choices, builderFixture())
	if d := iconic.AC - plain.AC; d != 0 {
		t.Errorf("Iconic +1 AC with -1 adjust changed AC by %d, want 0", d)
	}
	if d := iconic.Speed - plain.Speed; d != 30 {
		t.Errorf("Iconic speed +20 plus adjust 10 changed speed by %d, want 30", d)
	}
	// One more point of multiplier adds Rank Die avg x level = 40, plus 5.
	if d := iconic.HPMax - plain.HPMax; d != 45 {
		t.Errorf("Iconic HP difference = %d, want 45", d)
	}
	for _, d := range Disciplines {
		if iconic.Attack[d]-plain.Attack[d] != 1 || iconic.DC[d]-plain.DC[d] != 1 {
			t.Errorf("Iconic %s attack/DC +%d/+%d, want +1/+1", d, iconic.Attack[d]-plain.Attack[d], iconic.DC[d]-plain.DC[d])
		}
	}
}

func TestAssignAbilitiesKeepsPinnedValues(t *testing.T) {
	got := AssignAbilities([6]*int{nil, nil, nil, nil, nil, intp(4)}, [6]int{4, 3, 3, 2, 0, -1}, "adversary-builder-role/defender")
	// Cha pinned to 4; the rest follow Defender priority Con, Str, Dex, Wis, Int.
	if want := [6]int{3, 2, 3, -1, 0, 4}; got != want {
		t.Errorf("AssignAbilities = %v, want %v", got, want)
	}
}

func TestJutsuQuotasFoldHigherRanksDown(t *testing.T) {
	got := jutsuQuotas(builderFixture().JutsuKnown, "adversary-rank/solo", "C", 12)
	// D 4; C 3 + B 2 + A 1 counted at C, the adversary's own Rank.
	if want := []JutsuQuota{{"D", 4}, {"C", 6}}; !reflect.DeepEqual(got, want) {
		t.Errorf("jutsuQuotas = %v, want %v", got, want)
	}
}

func TestRankForLevel(t *testing.T) {
	for level, want := range map[int]string{1: "D", 4: "D", 5: "C", 12: "B", 16: "A", 17: "S", 30: "S"} {
		if got := RankForLevel(level); got != want {
			t.Errorf("RankForLevel(%d) = %s, want %s", level, got, want)
		}
	}
}
