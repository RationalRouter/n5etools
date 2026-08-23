package charsheet

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
	"github.com/sergio/n5e/internal/schema"
)

func TestAbilityModifier(t *testing.T) {
	cases := map[int]int{
		1: -5, 6: -2, 7: -2, 8: -1, 9: -1, 10: 0, 11: 0,
		12: 1, 13: 1, 14: 2, 15: 2, 16: 3, 17: 3, 18: 4, 20: 5,
	}
	for score, want := range cases {
		if got := AbilityModifier(score); got != want {
			t.Errorf("AbilityModifier(%d) = %d, want %d", score, got, want)
		}
	}
}

func TestCheckModifier(t *testing.T) {
	if got := CheckModifier(3, 5, true); got != 8 {
		t.Errorf("proficient: got %d, want 8", got)
	}
	if got := CheckModifier(3, 5, false); got != 3 {
		t.Errorf("not proficient: got %d, want 3", got)
	}
}

// TestSavingThrowModifier is the one formula most likely to be silently
// "corrected" back to standard 5e (zero bonus when not proficient) by a
// future editor who doesn't know this ruleset deviates — pinned here with
// the exact numbers from the core book's own worked example isn't
// available, so these are hand-verified against the confirmed rule text
// directly (half proficiency bonus, rounded down, for non-proficient
// saves).
func TestSavingThrowModifier(t *testing.T) {
	if got := SavingThrowModifier(2, 6, true); got != 8 {
		t.Errorf("proficient: got %d, want 8 (2 + 6)", got)
	}
	if got := SavingThrowModifier(2, 6, false); got != 5 {
		t.Errorf("not proficient: got %d, want 5 (2 + 3, half of 6)", got)
	}
	// Odd proficiency bonus (5) must round the half down, not up.
	if got := SavingThrowModifier(0, 5, false); got != 2 {
		t.Errorf("not proficient, odd prof bonus: got %d, want 2 (0 + 2, half of 5 rounded down)", got)
	}
	if got := SavingThrowModifier(-1, 3, false); got != 0 {
		t.Errorf("negative ability mod: got %d, want 0 (-1 + 1, half of 3 rounded down)", got)
	}
}

func TestArmorClass(t *testing.T) {
	scores := map[string]int{"dex": 16, "int": 12} // mod +3, +1
	profBonus := 6

	// Light armor: 1 ability term (Dex), uncapped.
	if got := ArmorClass(2, "DEX", "", nil, scores, profBonus); got != 15 {
		t.Errorf("light armor: got %d, want 15 (10 + 2 + 3)", got)
	}

	// Two ability-derived terms, capped.
	max := 2.0
	if got := ArmorClass(3, "DEX", "INT", &max, scores, profBonus); got != 15 {
		t.Errorf("capped two-term: got %d, want 15 (10 + 3 + min(3+1, 2)=2)", got)
	}

	// A 'PROF' term contributes half proficiency bonus, same rule as
	// SavingThrowModifier — this is the case most likely to be gotten
	// wrong (full prof bonus instead of half).
	if got := ArmorClass(0, "PROF", "", nil, scores, profBonus); got != 13 {
		t.Errorf("PROF term: got %d, want 13 (10 + 0 + 3, half of prof bonus 6)", got)
	}

	// Heavy armor stores the literal string "NONE" for an unused ability
	// slot (confirmed against out/rules.db), not "". Must be treated as
	// zero the same as "" — regression test for a real bug where this fell
	// through to AbilityModifier(scores["none"]) = AbilityModifier(0) = -5.
	if got := ArmorClass(6, "NONE", "NONE", nil, scores, profBonus); got != 16 {
		t.Errorf("NONE/NONE: got %d, want 16 (10 + 6 + 0)", got)
	}
}

func testDBs(t *testing.T) (rulesDB, charDB *sql.DB) {
	t.Helper()
	rulesDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rulesDB.Close() })
	if err := schema.Apply(rulesDB, schema.Rules); err != nil {
		t.Fatal(err)
	}

	charDB, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { charDB.Close() })
	if err := schema.Apply(charDB, schema.Characters); err != nil {
		t.Fatal(err)
	}
	return rulesDB, charDB
}

func TestComputeFreshDraftCharacter(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Test Genin', 10, 14, 12, 8, 13, 15)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if sheet.Name != "Test Genin" {
		t.Errorf("Name = %q", sheet.Name)
	}
	if sheet.Level != 0 {
		t.Errorf("Level = %d, want 0 (no character_classes rows yet)", sheet.Level)
	}
	if sheet.ProficiencyBonus != 2 {
		t.Errorf("ProficiencyBonus = %d, want 2 (fallback for a level-0 draft)", sheet.ProficiencyBonus)
	}
	if got := sheet.Abilities["dex"].Modifier; got != 2 {
		t.Errorf("dex modifier = %d, want 2 (score 14)", got)
	}
	if sheet.MaxHP != 0 || sheet.MaxChakra != 0 {
		t.Errorf("MaxHP/MaxChakra = %d/%d, want 0/0 (no level_gains rows yet)", sheet.MaxHP, sheet.MaxChakra)
	}
	// Unarmored is a real armor class, not an absent one: 10 + Dex. The
	// sheet used to render "—" here, which every character saw, because
	// there was no way to mark anything equipped in the first place.
	if sheet.AC == nil {
		t.Error("AC = nil, want 12 (unarmored: 10 + Dex 14's +2)")
	} else if *sheet.AC != 12 {
		t.Errorf("AC = %d, want 12 (unarmored: 10 + Dex 14's +2)", *sheet.AC)
	}
	// Every skill must be present even with zero proficiencies granted.
	if len(sheet.Skills) != len(SkillAbility) {
		t.Errorf("got %d skills, want %d", len(sheet.Skills), len(SkillAbility))
	}
	if len(sheet.Saves) != 6 {
		t.Errorf("got %d saves, want 6", len(sheet.Saves))
	}
}

func TestComputeWithClassLevelsProficienciesAndBonuses(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/test', 'Test Class', 10, 8)`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Leveled Character', 10, 10, 14, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 5, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	// Con bonus from a clan, auditable via character_ability_bonuses.
	if _, err := charDB.Exec(
		`INSERT INTO character_ability_bonuses (character_id, source_kind, source_ref, ability, amount)
		 VALUES (?, 'clan', 'clan/test', 'con', 2)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref) VALUES
		 (?, 'skill', 'Stealth', 'class', 'class/test'),
		 (?, 'saving_throw', 'Constitution', 'class', 'class/test')`, id, id,
	); err != nil {
		t.Fatal(err)
	}
	// Raw die values only — no Con modifier baked in, per charsheet.go's
	// documented convention (Max HP/Chakra adds level*currentConMod at
	// read time so ability-score changes retroactively apply for free).
	if _, err := charDB.Exec(`
		INSERT INTO character_level_gains (character_id, class_slug, class_level, hp_gain, chakra_gain, method) VALUES
		(?, 'class/test', 1, 10, 8, 'fixed'),
		(?, 'class/test', 2, 6, 5, 'fixed'),
		(?, 'class/test', 3, 6, 5, 'fixed'),
		(?, 'class/test', 4, 6, 5, 'fixed'),
		(?, 'class/test', 5, 6, 5, 'fixed')`, id, id, id, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if sheet.Level != 5 {
		t.Errorf("Level = %d, want 5", sheet.Level)
	}
	var wantProfBonus int
	if err := rulesDB.QueryRow(`SELECT proficiency_bonus FROM xp_levels WHERE level = 5`).Scan(&wantProfBonus); err != nil {
		t.Fatal(err)
	}
	if sheet.ProficiencyBonus != wantProfBonus {
		t.Errorf("ProficiencyBonus = %d, want %d (from xp_levels)", sheet.ProficiencyBonus, wantProfBonus)
	}
	if got := sheet.Abilities["con"].Score; got != 16 {
		t.Errorf("con score = %d, want 16 (base 14 + clan bonus 2)", got)
	}

	conMod := sheet.Abilities["con"].Modifier // +3
	wantHP := 10 + 6 + 6 + 6 + 6 + 5*conMod
	if sheet.MaxHP != wantHP {
		t.Errorf("MaxHP = %d, want %d", sheet.MaxHP, wantHP)
	}
	wantChakra := 8 + 5 + 5 + 5 + 5 + 5*conMod
	if sheet.MaxChakra != wantChakra {
		t.Errorf("MaxChakra = %d, want %d", sheet.MaxChakra, wantChakra)
	}

	var stealth SkillEntry
	for _, sk := range sheet.Skills {
		if sk.Name == "Stealth" {
			stealth = sk
		}
	}
	if !stealth.Proficient {
		t.Error("Stealth should be proficient")
	}
	wantStealth := sheet.Abilities["dex"].Modifier + sheet.ProficiencyBonus
	if stealth.Modifier != wantStealth {
		t.Errorf("Stealth modifier = %d, want %d", stealth.Modifier, wantStealth)
	}

	var conSave SaveEntry
	for _, sv := range sheet.Saves {
		if sv.Ability == "con" {
			conSave = sv
		}
	}
	if !conSave.Proficient {
		t.Error("Constitution save should be proficient (granted via character_proficiencies)")
	}
	if conSave.Modifier != conMod+sheet.ProficiencyBonus {
		t.Errorf("con save modifier = %d, want %d (full prof bonus, proficient)", conSave.Modifier, conMod+sheet.ProficiencyBonus)
	}
	// A non-proficient save must still get half proficiency bonus, not zero.
	var strSave SaveEntry
	for _, sv := range sheet.Saves {
		if sv.Ability == "str" {
			strSave = sv
		}
	}
	if strSave.Proficient {
		t.Error("Strength save should not be proficient")
	}
	wantStrSave := sheet.Abilities["str"].Modifier + sheet.ProficiencyBonus/2
	if strSave.Modifier != wantStrSave {
		t.Errorf("str save modifier = %d, want %d (half prof bonus despite no proficiency)", strSave.Modifier, wantStrSave)
	}
}

// TestComputeAppliesGrantedFeatureProficiencyAndACSwap exercises
// internal/features' curated grant tables end-to-end through Compute — not
// just that ResolveProficiencyGrants/ResolveACSwapAbility return the right
// values in isolation (internal/features/grants_test.go already covers
// that), but that Compute actually folds them into the real Skills/AC it
// returns. Uses the Hoshigaki clan, which conveniently grants both a fixed
// skill proficiency (Brute Strength -> Intimidation) and an AC ability swap
// (Shark-Skinned Predator -> Constitution for Dexterity), both level 1.
func TestComputeAppliesGrantedFeatureProficiencyAndACSwap(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/test', 'Test Class', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO clans (slug, name) VALUES ('clan/hoshigaki', 'Hoshigaki Clan')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO clan_features (slug, clan_slug, name, level, description) VALUES
		('clan/hoshigaki/feature/brute-strength', 'clan/hoshigaki', 'Brute Strength', 1, 'grants Intimidation'),
		('clan/hoshigaki/feature/shark-skinned-predator', 'clan/hoshigaki', 'Shark-Skinned Predator', 1, 'Con for Dex AC')`); err != nil {
		t.Fatal(err)
	}

	// Con (mod +3) higher than Dex (mod +0) — proves the AC swap actually
	// takes the better number rather than just applying unconditionally.
	res, err := charDB.Exec(`
		INSERT INTO characters (name, clan_slug, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kisame Test', 'clan/hoshigaki', 10, 10, 16, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 1, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	var intimidation SkillEntry
	for _, sk := range sheet.Skills {
		if sk.Name == "Intimidation" {
			intimidation = sk
		}
	}
	if !intimidation.Proficient {
		t.Error("Intimidation should be proficient via Hoshigaki's Brute Strength — feature grant never applied")
	}

	if sheet.AC == nil {
		t.Fatal("AC is nil, want a computed value")
	}
	// Unarmored with the Dex swap would be 10 + Con mod(+3) = 13, versus the
	// unswapped 10 + Dex mod(+0) = 10 — the swap must win.
	if *sheet.AC != 13 {
		t.Errorf("AC = %d, want 13 (unarmored, Constitution substituted for Dexterity via Shark-Skinned Predator)", *sheet.AC)
	}
}

// TestComputeAppliesGrantedFeatureSpeedBonus exercises the Speed side of
// the same wiring, via Namikaze's Supernatural Speed (level 1: +5 feet).
func TestComputeAppliesGrantedFeatureSpeedBonus(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/test', 'Test Class', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO clans (slug, name, speed_feet) VALUES ('clan/namikaze', 'Namikaze Clan', 30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO clan_features (slug, clan_slug, name, level, description) VALUES
		('clan/namikaze/feature/supernatural-speed', 'clan/namikaze', 'Supernatural Speed', 1, 'speed increases')`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, clan_slug, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Minato Test', 'clan/namikaze', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 1, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.Speed != 35 {
		t.Errorf("Speed = %d, want 35 (clan base 30 + Supernatural Speed's +5)", sheet.Speed)
	}
}

// TestComputeGatesMobilitySpeedBonusOnPick guards against the bug found
// auditing Scout-Nin: Mobility's class_features row (one of Jack of All,
// Master of None's 5 Generalizations) is blanket-granted to every
// 5th-level Scout-Nin for Features & Traits display, the same as
// Combat/Control/Skill/Support — but unlike those 4, Mobility feeds a real
// computed field (Speed), so its bonus must only apply once the player has
// actually picked Mobility via the Jack of All cap+catalog choice
// (charstore.ScoutNinPickJackOfAll), not merely by reaching 5th level.
func TestComputeGatesMobilitySpeedBonusOnPick(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/scout-nin', 'Scout-Nin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/scout-nin/feature/mobility', 'class/scout-nin', 'Mobility', 5, '+10 bonus to your speed'),
		('class/scout-nin/feature/combat', 'class/scout-nin', 'Combat', 5, 'attack and damage bonus')`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Mobility Test', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/scout-nin', 5, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.Speed != 30 {
		t.Errorf("Speed with no Jack of All pick made = %d, want 30 (book default, no Mobility bonus without the pick)", sheet.Speed)
	}

	if err := charstore.AddScoutNinPick(charDB, id, charstore.ScoutNinPickJackOfAll, "class/scout-nin/feature/combat"); err != nil {
		t.Fatal(err)
	}
	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute after picking Combat instead: %v", err)
	}
	if sheet.Speed != 30 {
		t.Errorf("Speed after picking Combat (not Mobility) = %d, want 30 (still no Mobility bonus)", sheet.Speed)
	}

	if err := charstore.AddScoutNinPick(charDB, id, charstore.ScoutNinPickJackOfAll, "class/scout-nin/feature/mobility"); err != nil {
		t.Fatal(err)
	}
	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute after picking Mobility: %v", err)
	}
	if sheet.Speed != 40 {
		t.Errorf("Speed after picking Mobility = %d, want 40 (book default 30 + Mobility's own +10 at 5th level)", sheet.Speed)
	}
}

// jutsuAttackModifier finds one AttackKinds entry's Modifier by kind, for
// tests that only care about a delta before/after a pick rather than the
// full absolute number (which also depends on ability scores and
// proficiency bonus).
func jutsuAttackModifier(sheet *Sheet, kind string) int {
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == kind {
			return a.Modifier
		}
	}
	return 0
}

func skillModifier(sheet *Sheet, name string) int {
	for _, s := range sheet.Skills {
		if s.Name == name {
			return s.Modifier
		}
	}
	return 0
}

func saveModifier(sheet *Sheet, ability string) int {
	for _, s := range sheet.Saves {
		if s.Ability == ability {
			return s.Modifier
		}
	}
	return 0
}

// TestComputeGatesCombatSkillMobilitySaveBonusesOnPick guards against the
// same "blanket-granted for Features & Traits display, not gated on the
// actual Jack of All pick" bug TestComputeGatesMobilitySpeedBonusOnPick
// already guards for Mobility's Speed bonus, extended to Combat's jutsu
// attack/damage bonus, Skill's skill-check bonus, and Mobility's OWN
// saving-throw bonus (a second, separate clause from its Speed bonus).
// Each must apply only once its own Generalization is picked, and each
// must step from its 5th-level tier to its 11th-level tier as the
// character's own level crosses that threshold.
func TestComputeGatesCombatSkillMobilitySaveBonusesOnPick(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/scout-nin', 'Scout-Nin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/scout-nin/feature/combat', 'class/scout-nin', 'Combat', 5, 'attack and damage bonus'),
		('class/scout-nin/feature/skill', 'class/scout-nin', 'Skill', 5, 'skill check bonus'),
		('class/scout-nin/feature/mobility', 'class/scout-nin', 'Mobility', 5, 'speed and save bonus')`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Jack of All Test', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/scout-nin', 5, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	before, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute before any pick: %v", err)
	}
	if before.JackOfAllCombatBonus != 0 {
		t.Errorf("JackOfAllCombatBonus before picking Combat = %d, want 0", before.JackOfAllCombatBonus)
	}

	for _, slug := range []string{"class/scout-nin/feature/combat", "class/scout-nin/feature/skill", "class/scout-nin/feature/mobility"} {
		if err := charstore.AddScoutNinPick(charDB, id, charstore.ScoutNinPickJackOfAll, slug); err != nil {
			t.Fatal(err)
		}
	}

	at5, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute at 5th level after picking all three: %v", err)
	}
	if at5.JackOfAllCombatBonus != 1 {
		t.Errorf("JackOfAllCombatBonus at 5th level = %d, want 1", at5.JackOfAllCombatBonus)
	}
	for _, kind := range []string{"Ninjutsu", "Genjutsu", "Taijutsu", "Bukijutsu"} {
		want := jutsuAttackModifier(before, kind) + 1
		if got := jutsuAttackModifier(at5, kind); got != want {
			t.Errorf("%s attack modifier at 5th level = %d, want %d (+1 Combat bonus)", kind, got, want)
		}
	}
	for name := range SkillAbility {
		want := skillModifier(before, name) + 2
		if got := skillModifier(at5, name); got != want {
			t.Errorf("%s skill modifier at 5th level = %d, want %d (+2 Skill bonus)", name, got, want)
			break
		}
	}
	for _, ab := range Abilities {
		want := saveModifier(before, ab) + 1
		if got := saveModifier(at5, ab); got != want {
			t.Errorf("%s save modifier at 5th level = %d, want %d (+1 Mobility save bonus)", ab, got, want)
		}
	}

	if _, err := charDB.Exec(`UPDATE character_classes SET levels = 11 WHERE character_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	// A fresh, same-level "no picks" baseline is needed here rather than
	// reusing the 5th-level "before" sheet: ProficiencyBonus itself can
	// change between 5th and 11th level, and SavingThrowModifier folds in
	// HALF that bonus even for saves the character isn't proficient in —
	// comparing across levels would conflate that unrelated shift with the
	// tier-2 Jack of All bonus this assertion is actually testing for.
	for _, slug := range []string{"class/scout-nin/feature/combat", "class/scout-nin/feature/skill", "class/scout-nin/feature/mobility"} {
		if err := charstore.RemoveScoutNinPick(charDB, id, charstore.ScoutNinPickJackOfAll, slug); err != nil {
			t.Fatal(err)
		}
	}
	before11, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute at 11th level, no picks: %v", err)
	}
	for _, slug := range []string{"class/scout-nin/feature/combat", "class/scout-nin/feature/skill", "class/scout-nin/feature/mobility"} {
		if err := charstore.AddScoutNinPick(charDB, id, charstore.ScoutNinPickJackOfAll, slug); err != nil {
			t.Fatal(err)
		}
	}
	at11, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute at 11th level, with picks: %v", err)
	}
	if at11.JackOfAllCombatBonus != 2 {
		t.Errorf("JackOfAllCombatBonus at 11th level = %d, want 2", at11.JackOfAllCombatBonus)
	}
	for _, kind := range []string{"Ninjutsu", "Genjutsu", "Taijutsu", "Bukijutsu"} {
		want := jutsuAttackModifier(before11, kind) + 2
		if got := jutsuAttackModifier(at11, kind); got != want {
			t.Errorf("%s attack modifier at 11th level = %d, want %d (+2 Combat bonus)", kind, got, want)
		}
	}
	if got := skillModifier(at11, "Stealth") - skillModifier(before11, "Stealth"); got != 3 {
		t.Errorf("Stealth skill bonus at 11th level = %d, want 3", got)
	}
	if got := saveModifier(at11, "dex") - saveModifier(before11, "dex"); got != 2 {
		t.Errorf("dex save bonus at 11th level = %d, want 2", got)
	}
}

// TestComputeMartialStudentUsesDexForNonLethalPrecisionJutsuType exercises
// Martial Student (Hunters Pattern, class/hunter-nin/option/hunters-
// patterns/martial-student): once Lethal Precision has picked Taijutsu,
// Martial Student mirrors its own Dexterity override onto Bukijutsu (the
// type Lethal Precision did NOT pick) — and does nothing to Taijutsu itself,
// which already uses Dexterity via Lethal Precision alone.
func TestComputeMartialStudentUsesDexForNonLethalPrecisionJutsuType(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/hunter-nin', 'Hunter-Nin', 8, 6)`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Martial Student Test', 10, 18, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/hunter-nin', 1, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	before, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute before any pick: %v", err)
	}
	if got, want := jutsuAttackModifier(before, "Bukijutsu"), before.Abilities["str"].Modifier+before.ProficiencyBonus; got != want {
		t.Errorf("Bukijutsu attack modifier before any pick = %d, want %d (default Strength)", got, want)
	}

	if err := charstore.AddHunterNinPick(charDB, id, charstore.HunterPickLethalPrecision, "taijutsu"); err != nil {
		t.Fatal(err)
	}
	if err := charstore.AddHunterNinPick(charDB, id, charstore.HunterPickPattern, "class/hunter-nin/option/hunters-patterns/martial-student"); err != nil {
		t.Fatal(err)
	}

	after, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute after picks: %v", err)
	}
	if got, want := jutsuAttackModifier(after, "Taijutsu"), after.Abilities["dex"].Modifier+after.ProficiencyBonus; got != want {
		t.Errorf("Taijutsu attack modifier (Lethal Precision's own pick) = %d, want %d (Dexterity)", got, want)
	}
	if got, want := jutsuAttackModifier(after, "Bukijutsu"), after.Abilities["dex"].Modifier+after.ProficiencyBonus; got != want {
		t.Errorf("Bukijutsu attack modifier (Martial Student's mirrored pick) = %d, want %d (Dexterity)", got, want)
	}
}

// TestComputeAppliesChoiceGatedFeatureGrant exercises the choice-gated side
// of the same wiring, via Scout-Nin's Canny ("Choose any one skill. You gain
// proficiency in this skill."): unresolved, it must show up in
// PendingFeatureChoices and grant nothing; once a pick is stored in
// character_feature_choices, Compute must apply it and stop listing it as
// pending.
func TestComputeAppliesChoiceGatedFeatureGrant(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/scout-nin', 'Scout-Nin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/scout-nin/feature/canny', 'class/scout-nin', 'Canny', 1, 'choose a skill')`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Canny Test', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/scout-nin', 1, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if len(sheet.PendingFeatureChoices) != 1 {
		t.Fatalf("PendingFeatureChoices = %+v, want exactly Canny's unresolved slot", sheet.PendingFeatureChoices)
	}
	for _, sk := range sheet.Skills {
		if sk.Name == "Stealth" && sk.Proficient {
			t.Fatal("Stealth should not be proficient before Canny's pick is resolved")
		}
	}

	if _, err := charDB.Exec(
		`INSERT INTO character_feature_choices (character_id, feature_slug, choice_index, value)
		 VALUES (?, 'class/scout-nin/feature/canny', 0, 'Stealth')`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute after pick: %v", err)
	}
	if len(sheet.PendingFeatureChoices) != 0 {
		t.Errorf("PendingFeatureChoices = %+v, want none once Canny is resolved", sheet.PendingFeatureChoices)
	}
	var stealthProficient bool
	for _, sk := range sheet.Skills {
		if sk.Name == "Stealth" {
			stealthProficient = sk.Proficient
		}
	}
	if !stealthProficient {
		t.Error("Stealth should be proficient once Canny's pick names it — resolved choice grant never applied")
	}
}

// TestComputeAppliesFeatureAbilityScoresAndSaveMastery covers the two
// feature-granted mechanisms added by the Puppet Master re-audit follow-up,
// via Purple Technique's Intelligent Design (level 10: a pick-one +2
// Strength-or-Dexterity, and always-on Mastery on Strength saves) plus
// Master of the Purple Technique (level 20: a fixed +2 Intelligence): the
// choice must show as pending and grant nothing until resolved, the save
// Mastery must fold into the Strength save's modifier, and the fixed
// capstone increase must apply with no pick at all.
func TestComputeAppliesFeatureAbilityScoresAndSaveMastery(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut',
		 'class/puppet-master/group/puppet-techniques', 'Purple Technique ~ Juggernaut')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design',
		 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 'Intelligent Design', 10, 'str skills, str save mastery, +2 str or dex'),
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/master-of-the-purple-technique',
		 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 'Master of the Purple Technique', 20, '+2 int')`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Juggernaut Test', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/puppet-master', 10, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 2)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	// The +2 pick is pending, not applied.
	foundPending := false
	for _, s := range sheet.PendingFeatureChoices {
		if s.Kind == features.ChoiceAbilityScoreIncrease {
			foundPending = true
		}
	}
	if !foundPending {
		t.Error("Intelligent Design's ability pick should be pending at level 10")
	}
	if got := sheet.Abilities["str"].Score; got != 10 {
		t.Errorf("str before pick = %d, want 10", got)
	}
	// Nothing has resolved yet, so every ability's max sits at the flat
	// default of 20 — an unresolved choice slot contributes no RaisesMax
	// bonus.
	for _, ab := range Abilities {
		if got := sheet.AbilityMax[ab]; got != 20 {
			t.Errorf("AbilityMax[%s] before pick = %d, want 20", ab, got)
		}
	}
	// Save Mastery applies regardless of the pick: str save = SavingThrow
	// Modifier(mod 0, prof 6 at level 10, not proficient → half = 3) +
	// MasteryBonus(rank 1) = 3 + 2 = 5, and every other save stays
	// Mastery-free.
	for _, sv := range sheet.Saves {
		switch sv.Ability {
		case "str":
			if sv.MasteryRank != 1 {
				t.Errorf("str save MasteryRank = %d, want 1", sv.MasteryRank)
			}
			if sv.Modifier != 5 {
				t.Errorf("str save modifier = %d, want 5 (half of prof 6 + Mastery 2)", sv.Modifier)
			}
		default:
			if sv.MasteryRank != 0 {
				t.Errorf("%s save MasteryRank = %d, want 0", sv.Ability, sv.MasteryRank)
			}
		}
	}

	// Resolve the pick: +2 Strength applies and the slot stops pending.
	if _, err := charDB.Exec(
		`INSERT INTO character_feature_choices (character_id, feature_slug, choice_index, value)
		 VALUES (?, 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design', 0, 'str')`, id,
	); err != nil {
		t.Fatal(err)
	}
	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute after pick: %v", err)
	}
	if got := sheet.Abilities["str"].Score; got != 12 {
		t.Errorf("str after pick = %d, want 12", got)
	}
	// Intelligent Design's choice slot carries RaisesMax — the pick raises
	// str's max right along with its current score.
	if got := sheet.AbilityMax["str"]; got != 22 {
		t.Errorf("AbilityMax[str] after pick = %d, want 22", got)
	}
	if got := sheet.AbilityMax["dex"]; got != 20 {
		t.Errorf("AbilityMax[dex] after pick = %d, want 20 (unaffected)", got)
	}
	for _, s := range sheet.PendingFeatureChoices {
		if s.Kind == features.ChoiceAbilityScoreIncrease {
			t.Errorf("ability pick still pending after being resolved: %+v", s)
		}
	}

	// At level 20 the capstone's fixed +2 Intelligence applies with no pick.
	if _, err := charDB.Exec(`UPDATE character_classes SET levels = 20 WHERE character_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute at level 20: %v", err)
	}
	if got := sheet.Abilities["int"].Score; got != 12 {
		t.Errorf("int at level 20 = %d, want 12 (Master of the Purple Technique's fixed +2)", got)
	}
	// The fixed grant also raises int's max, stacking independently of the
	// earlier choice-gated str max raise.
	if got := sheet.AbilityMax["int"]; got != 22 {
		t.Errorf("AbilityMax[int] at level 20 = %d, want 22 (Master of the Purple Technique's fixed +2 also raises the max)", got)
	}
	if got := sheet.AbilityMax["str"]; got != 22 {
		t.Errorf("AbilityMax[str] at level 20 = %d, want 22 (still carried from the earlier Intelligent Design pick)", got)
	}
}

// TestComputeAppliesChassisPropertyBonuses covers Item 4: a worn Armor
// Chassis's own Powerful Build (Str +2, capped at 22) and Mobile (+5 Speed)
// properties, keyed purely off character_companions.is_armor_form/
// armor_chassis — no Puppet Master class/feature setup is needed for these,
// since the bonus is a straight equipment-property translation, the same
// "caller resolves runtime state" shape SM-A/SM-B use elsewhere in this
// file.
func TestComputeAppliesChassisPropertyBonuses(t *testing.T) {
	cases := []struct {
		name      string
		chassis   string
		baseStr   int
		wantStr   int
		wantSpeed int
	}{
		{"Steel Fortress (Powerful Build) raises Str by 2", "Steel Fortress", 20, 22, 30},
		{"Steel Fortress caps at 22, not a full +2 past it", "Steel Fortress", 21, 22, 30},
		{"Steel Fortress grants nothing more once already at 22", "Steel Fortress", 22, 22, 30},
		{"Weaved Mail (Mobile) grants +5 Speed, no Str change", "Weaved Mail", 10, 10, 35},
		{"Wooden Suit (Mobile) grants +5 Speed, no Str change", "Wooden Suit", 10, 10, 35},
		{"Iron Shell has neither property", "Iron Shell", 10, 10, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rulesDB, charDB := testDBs(t)
			res, err := charDB.Exec(`
				INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
				VALUES ('Chassis Property Test', ?, 10, 10, 10, 10, 10)`, c.baseStr)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := res.LastInsertId()
			if _, err := charDB.Exec(
				`INSERT INTO character_companions (character_id, kind, name, armor_chassis, is_armor_form, sort_order)
				 VALUES (?, 'puppet', 'Juggernaut Armor', ?, 1, 0)`, id, c.chassis,
			); err != nil {
				t.Fatal(err)
			}
			sheet, err := Compute(rulesDB, charDB, id)
			if err != nil {
				t.Fatalf("Compute: %v", err)
			}
			if got := sheet.Abilities["str"].Score; got != c.wantStr {
				t.Errorf("str = %d, want %d", got, c.wantStr)
			}
			if got := sheet.Speed; got != c.wantSpeed {
				t.Errorf("Speed = %d, want %d", got, c.wantSpeed)
			}
		})
	}
}

// TestComputeAppliesMasterOfThePurpleTechniqueConditionalInt covers Item 1:
// the feature's SECOND +2 Intelligence only applies while a puppet is
// currently worn as the character's Juggernaut Armor
// (character_companions.is_armor_form), and unlike the base +2, does not
// raise the Intelligence max (see ResolveConditionalAbilityScoreGrants'
// own comment for why).
func TestComputeAppliesMasterOfThePurpleTechniqueConditionalInt(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut',
		 'class/puppet-master/group/puppet-techniques', 'Purple Technique ~ Juggernaut')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/master-of-the-purple-technique',
		 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 'Master of the Purple Technique', 20, '+2 int, +2 more while worn as armor')`); err != nil {
		t.Fatal(err)
	}
	// "Steel Fortress" is one of the 4 real chassis rows the rules.db
	// migrations seed — reused rather than inserting a duplicate, since
	// testDBs applies the real Rules schema (data included), not just its
	// table shape.
	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Master of the Purple Technique Test', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/puppet-master', 20, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 2)`, id,
	); err != nil {
		t.Fatal(err)
	}

	// No puppet marked as worn armor yet: only the base, unconditional +2
	// applies.
	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute (no armor worn): %v", err)
	}
	if got := sheet.Abilities["int"].Score; got != 12 {
		t.Errorf("int with no armor worn = %d, want 12 (base +2 only)", got)
	}
	if got := sheet.AbilityMax["int"]; got != 22 {
		t.Errorf("AbilityMax[int] with no armor worn = %d, want 22 (base grant still raises the max)", got)
	}

	// Mark a puppet as worn armor, with its chassis chosen and no AC
	// entered yet — is_armor_form alone must be enough; a blank AC must
	// not silently drop the conditional bonus (see puppetWornAsArmorAC's
	// own leniency, which this resolver deliberately does not share).
	if _, err := charDB.Exec(
		`INSERT INTO character_companions (character_id, kind, name, armor_chassis, is_armor_form, sort_order)
		 VALUES (?, 'puppet', 'Juggernaut Armor', 'Steel Fortress', 1, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute (armor worn): %v", err)
	}
	if got := sheet.Abilities["int"].Score; got != 14 {
		t.Errorf("int with armor worn = %d, want 14 (base +2, conditional +2)", got)
	}
	// The conditional half is deliberately NOT max-raising.
	if got := sheet.AbilityMax["int"]; got != 22 {
		t.Errorf("AbilityMax[int] with armor worn = %d, want 22 (conditional +2 must not raise the max)", got)
	}
}

// TestComputeAppliesResolvedASI proves an Ability Score Improvement pick
// actually raises the computed ability score, and that PendingASISlots
// drops it once resolved — the ability-score half needs no dedicated
// Compute wiring beyond what sumAbilityBonuses already does (it sums every
// character_ability_bonuses row regardless of source_kind), but
// PendingASISlots itself does.
func TestComputeAppliesResolvedASI(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/hunter-nin', 'Hunter-Nin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/hunter-nin/feature/ability-score-improvement-feat', 'class/hunter-nin', 'Ability Score Improvement/Feat', NULL, 'pick an ability')`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('ASI Test', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/hunter-nin', 4, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if len(sheet.PendingASISlots) != 1 || sheet.PendingASISlots[0].Ref != "class/hunter-nin@4" {
		t.Fatalf("PendingASISlots = %+v, want exactly the level-4 slot", sheet.PendingASISlots)
	}
	if got := sheet.Abilities["str"].Score; got != 10 {
		t.Fatalf("str before pick = %d, want 10", got)
	}

	if _, err := charDB.Exec(
		`INSERT INTO character_ability_bonuses (character_id, source_kind, source_ref, ability, amount)
		 VALUES (?, 'asi', 'class/hunter-nin@4', 'str', 1)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err = Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute after pick: %v", err)
	}
	if len(sheet.PendingASISlots) != 0 {
		t.Errorf("PendingASISlots = %+v, want none once resolved", sheet.PendingASISlots)
	}
	if got := sheet.Abilities["str"].Score; got != 11 {
		t.Errorf("str after pick = %d, want 11", got)
	}
}

func TestComputeWithEquippedArmor(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, ac_bonus, armor_ability_1, armor_ability_2, armor_max_mod)
		VALUES ('armor/test-jacket', 'Test Jacket', 'armor', 4, 'DEX', 'PROF', 3)`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Armored', 10, 16, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, equipped) VALUES (?, 'armor/test-jacket', 1)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.AC == nil {
		t.Fatal("AC is nil, want a computed value")
	}
	// dex mod +3, PROF term = profBonus(2)/2 = 1, sum = 4, capped at 3 -> AC = 10+4+3 = 17
	if *sheet.AC != 17 {
		t.Errorf("AC = %d, want 17", *sheet.AC)
	}
}

func TestComputeACOverride(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Overridden', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_overrides (character_id, field, value, note) VALUES (?, 'ac', '99', 'magic item not modeled yet')`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.AC == nil || *sheet.AC != 99 {
		t.Errorf("AC override not applied, got %v", sheet.AC)
	}
}

func TestComputeAppliesPuppetWornAsArmorAC(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, ac_bonus, armor_ability_1, armor_ability_2, armor_max_mod)
		VALUES ('armor/test-jacket', 'Test Jacket', 'armor', 4, 'DEX', 'PROF', 3)`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Juggernaut', 10, 16, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	// Equipped armor too, to prove the puppet's AC replaces it rather than
	// stacking — this character would otherwise compute to 17, same as
	// TestComputeWithEquippedArmor.
	if _, err := charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, equipped) VALUES (?, 'armor/test-jacket', 1)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_companions (character_id, kind, name, ac, is_armor_form) VALUES (?, 'puppet', 'Steel Fortress', 22, 1)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.AC == nil || *sheet.AC != 22 {
		t.Errorf("AC = %v, want the worn puppet's own AC (22), not the equipped jacket's", sheet.AC)
	}
}

func TestComputeIgnoresPuppetNotWornAsArmor(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, ac_bonus, armor_ability_1, armor_ability_2, armor_max_mod)
		VALUES ('armor/test-jacket', 'Test Jacket', 'armor', 4, 'DEX', 'PROF', 3)`); err != nil {
		t.Fatal(err)
	}

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Puppeteer', 10, 16, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, equipped) VALUES (?, 'armor/test-jacket', 1)`, id,
	); err != nil {
		t.Fatal(err)
	}
	// Has an AC entered, but the box isn't checked — should have no effect.
	if _, err := charDB.Exec(
		`INSERT INTO character_companions (character_id, kind, name, ac, is_armor_form) VALUES (?, 'puppet', 'Steel Fortress', 22, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.AC == nil || *sheet.AC != 17 {
		t.Errorf("AC = %v, want the normal equipped-armor AC (17) since is_armor_form is unchecked", sheet.AC)
	}
}

// levelledCharacter is a level-1 character of a d10-hit-die / d8-chakra-die
// class with a Constitution modifier of exactly +1, set up the way
// charstore.SetClass leaves one: a single character_classes row and the one
// level-1 gain row, and nothing for any level above that.
func levelledCharacter(t *testing.T, rulesDB, charDB *sql.DB) int64 {
	t.Helper()
	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/test', 'Test Class', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Growing Character', 10, 10, 12, 14, 16, 18)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 1, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(`
		INSERT INTO character_level_gains (character_id, class_slug, class_level, hp_gain, chakra_gain, method)
		VALUES (?, 'class/test', 1, 10, 8, 'fixed')`, id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFixedGain(t *testing.T) {
	if got := FixedGain(10, true); got != 10 {
		t.Errorf("first level of a d10: got %d, want 10 (the die's maximum)", got)
	}
	if got := FixedGain(10, false); got != 6 {
		t.Errorf("later level of a d10: got %d, want 6 (10/2 + 1)", got)
	}
	if got := FixedGain(0, true); got != 0 {
		t.Errorf("no class yet: got %d, want 0", got)
	}
}

// TestComputeGrantsHPAndChakraForUnrecordedLevels is the part-6 bug pinned
// down: raising a character's level has to raise their hit dice, HP and
// chakra, without any level_gains row being written for the new levels.
func TestComputeGrantsHPAndChakraForUnrecordedLevels(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	if _, err := charDB.Exec(
		`UPDATE character_classes SET levels = 7 WHERE character_id = ?`, id); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.Level != 7 {
		t.Fatalf("Level = %d, want 7", sheet.Level)
	}
	conMod := sheet.Abilities["con"].Modifier
	if conMod != 1 {
		t.Fatalf("con modifier = %d, want 1 (the fixture's whole point)", conMod)
	}
	// Level 1 takes the die's max, levels 2-7 the fixed 10/2+1 = 6 each,
	// plus the Con modifier once per level.
	wantHP := 10 + 6*6 + 7*conMod
	if sheet.MaxHP != wantHP {
		t.Errorf("MaxHP = %d, want %d (level 1 max + six fixed gains + level*conMod)", sheet.MaxHP, wantHP)
	}
	wantChakra := 8 + 6*5 + 7*conMod
	if sheet.MaxChakra != wantChakra {
		t.Errorf("MaxChakra = %d, want %d", sheet.MaxChakra, wantChakra)
	}
	if sheet.MaxHPPinned || sheet.MaxChakraPinned {
		t.Error("nothing was pinned by hand, but the sheet says otherwise")
	}
	if sheet.MaxHPAuto != wantHP || sheet.MaxChakraAuto != wantChakra {
		t.Errorf("Auto maxima = %d/%d, want %d/%d", sheet.MaxHPAuto, sheet.MaxChakraAuto, wantHP, wantChakra)
	}
}

// TestComputeStoredGainWinsOverFixed guards the fallback from swallowing a
// recorded roll: a level WITH a row must use that row's number, not the
// fixed value, or rolling for HP would silently do nothing.
func TestComputeStoredGainWinsOverFixed(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	if _, err := charDB.Exec(
		`UPDATE character_classes SET levels = 3 WHERE character_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	// A lucky roll at level 2; level 3 has no row and falls back to 6.
	if _, err := charDB.Exec(`
		INSERT INTO character_level_gains (character_id, class_slug, class_level, hp_gain, chakra_gain, method)
		VALUES (?, 'class/test', 2, 9, 7, 'rolled')`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	conMod := sheet.Abilities["con"].Modifier
	if want := 10 + 9 + 6 + 3*conMod; sheet.MaxHP != want {
		t.Errorf("MaxHP = %d, want %d (10 max + 9 rolled + 6 fixed)", sheet.MaxHP, want)
	}
	if want := 8 + 7 + 5 + 3*conMod; sheet.MaxChakra != want {
		t.Errorf("MaxChakra = %d, want %d", sheet.MaxChakra, want)
	}
}

func TestComputeMaximaOverrides(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	if _, err := charDB.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note) VALUES
		(?, 'maxhp', '77', 'rolled by hand'),
		(?, 'maxchakra', 'not a number', 'junk')`, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sheet.MaxHP != 77 || !sheet.MaxHPPinned {
		t.Errorf("MaxHP = %d (pinned %v), want 77 pinned", sheet.MaxHP, sheet.MaxHPPinned)
	}
	// A malformed override is ignored rather than zeroing the field it pins.
	if sheet.MaxChakraPinned {
		t.Error("a non-numeric maxchakra override was treated as a pin")
	}
	if sheet.MaxChakra != sheet.MaxChakraAuto {
		t.Errorf("MaxChakra = %d, want the computed %d", sheet.MaxChakra, sheet.MaxChakraAuto)
	}
	// The automatic number stays visible so the sheet can offer it back.
	if sheet.MaxHPAuto == 77 {
		t.Error("MaxHPAuto should still hold the computed total, not the pin")
	}
}

// Initiative is Dex + HALF proficiency bonus, rounded down — not a plain Dex
// check. It rendered as one until that was corrected, and the two only differ
// by a number that looks plausible either way, so this pins the formula.
func TestComputeInitiative(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	dexMod := sheet.Abilities["dex"].Modifier
	want := dexMod + sheet.ProficiencyBonus/2
	if sheet.Initiative != want {
		t.Errorf("Initiative = %+d, want %+d (dex %+d + half prof %d/2)",
			sheet.Initiative, want, dexMod, sheet.ProficiencyBonus)
	}
	if sheet.ProficiencyBonus >= 2 && sheet.Initiative == dexMod {
		t.Errorf("Initiative = %+d, the bare Dex modifier — half of the +%d "+
			"proficiency bonus is missing", sheet.Initiative, sheet.ProficiencyBonus)
	}
	if sheet.InitiativeAbility != "dex" || sheet.InitiativeProf != ProfHalf || sheet.InitiativeBonus != 0 {
		t.Errorf("defaults = %s/%s/%+d, want dex/half/+0",
			sheet.InitiativeAbility, sheet.InitiativeProf, sheet.InitiativeBonus)
	}
}

// The initiative picker's three overrides, which is the only way a character
// deviates from Dex + half proficiency.
func TestComputeInitiativeOverrides(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	if _, err := charDB.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note) VALUES
		(?, 'initiative_ability', 'wis', 'feature'),
		(?, 'initiative_prof', 'full', 'feature'),
		(?, 'initiative_bonus', '2', 'feature')`, id, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	want := sheet.Abilities["wis"].Modifier + sheet.ProficiencyBonus + 2
	if sheet.Initiative != want {
		t.Errorf("Initiative = %+d, want %+d (wis %+d + full prof %d + 2)",
			sheet.Initiative, want, sheet.Abilities["wis"].Modifier, sheet.ProficiencyBonus)
	}
	if sheet.InitiativeAbility != "wis" || sheet.InitiativeProf != ProfFull || sheet.InitiativeBonus != 2 {
		t.Errorf("parts = %s/%s/%+d, want wis/full/+2",
			sheet.InitiativeAbility, sheet.InitiativeProf, sheet.InitiativeBonus)
	}
}

// A junk override must fall back to the default, exactly like the jutsu attack
// abilities do — never compute off ability score 0 or drop the prof share.
func TestComputeInitiativeJunkOverrides(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	if _, err := charDB.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note) VALUES
		(?, 'initiative_ability', 'luck', 'junk'),
		(?, 'initiative_prof', 'double', 'junk'),
		(?, 'initiative_bonus', 'lots', 'junk')`, id, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	want := sheet.Abilities["dex"].Modifier + sheet.ProficiencyBonus/2
	if sheet.Initiative != want {
		t.Errorf("Initiative = %+d, want the default %+d", sheet.Initiative, want)
	}
}

func TestInitiativeModifier(t *testing.T) {
	// Half is the N5E default and rounds down; full and none exist because
	// features move it. The flat bonus is added on top of whichever.
	cases := []struct {
		ability, prof int
		mode          string
		flat, want    int
	}{
		{0, 3, ProfHalf, 0, 1}, // +3 prof is N5E's level-1 bonus; half rounds to 1
		{3, 3, ProfHalf, 0, 4},
		{-1, 3, ProfHalf, 0, 0},
		{2, 5, ProfHalf, 0, 4}, // 5/2 rounds down to 2
		{2, 5, ProfFull, 0, 7},
		{2, 5, ProfNone, 0, 2},
		{2, 5, ProfHalf, 3, 7},  // flat bonus stacks
		{2, 5, ProfNone, -1, 1}, // and can be negative
		// An unrecognised mode must fall back to the default, not to zero: a
		// stale override should never silently cost a player their bonus.
		{2, 5, "", 0, 4},
		{2, 5, "nonsense", 0, 4},
	}
	for _, c := range cases {
		got := InitiativeModifier(c.ability, c.prof, c.mode, c.flat)
		if got != c.want {
			t.Errorf("InitiativeModifier(%d, %d, %q, %d) = %d, want %d",
				c.ability, c.prof, c.mode, c.flat, got, c.want)
		}
	}
}

func TestComputeJutsuAttacks(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB) // int 14 (+2), wis 16 (+3), str 10 (+0), cha 18 (+4)

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	prof := sheet.ProficiencyBonus
	want := map[string]struct {
		ability string
		mod     int
	}{
		"Ninjutsu": {"int", 2 + prof},
		"Genjutsu": {"wis", 3 + prof},
		"Taijutsu": {"str", 0 + prof},
		// Derived: same ability and modifier as Taijutsu, by definition.
		"Bukijutsu": {"str", 0 + prof},
	}
	if len(sheet.JutsuAttacks) != len(want) {
		t.Fatalf("got %d attack kinds, want %d", len(sheet.JutsuAttacks), len(want))
	}
	for _, a := range sheet.JutsuAttacks {
		w, ok := want[a.Kind]
		if !ok {
			t.Errorf("unexpected attack kind %q", a.Kind)
			continue
		}
		if a.Ability != w.ability || a.Modifier != w.mod {
			t.Errorf("%s attack = %s %+d, want %s %+d", a.Kind, a.Ability, a.Modifier, w.ability, w.mod)
		}
	}
}

func TestComputeJutsuAttackAbilityOverride(t *testing.T) {
	rulesDB, charDB := testDBs(t)
	id := levelledCharacter(t, rulesDB, charDB)

	if _, err := charDB.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note) VALUES
		(?, 'genjutsu_ability', 'cha', 'class feature'),
		(?, 'taijutsu_ability', 'nonsense', 'junk')`, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	byKind := map[string]JutsuAttack{}
	for _, a := range sheet.JutsuAttacks {
		byKind[a.Kind] = a
	}
	gen := byKind["Genjutsu"]
	if gen.Ability != "cha" || gen.Modifier != 4+sheet.ProficiencyBonus {
		t.Errorf("Genjutsu = %s %+d, want cha %+d", gen.Ability, gen.Modifier, 4+sheet.ProficiencyBonus)
	}
	// An unrecognised ability falls back to the default rather than
	// computing off a score of zero (which would read as -5).
	tai := byKind["Taijutsu"]
	if tai.Ability != "str" {
		t.Errorf("Taijutsu = %s, want the str default", tai.Ability)
	}
}

// TestComputeGenjutsuAbilityCruelAngelsThesis pins Spyware's 3rd-level Cruel
// Angel's Thesis ("You can use your Intelligence Modifier as your Genjutsu
// ability modifier"): granted, it swaps Genjutsu's default ability to
// Intelligence; absent, Genjutsu still defaults to Wisdom; and a player's
// own manual genjutsu_ability override still wins over the feature, the
// same override-beats-default precedence TestComputeJutsuAttackAbilityOverride
// pins for the other Genjutsu-ability-swapping features.
func TestComputeGenjutsuAbilityCruelAngelsThesis(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/spyware', 'class/science-nin/group/scientific-inquiry', 'Spyware')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		('class/science-nin/group/scientific-inquiry/spyware/feature/cruel-angels-thesis',
		 'class/science-nin/group/scientific-inquiry/spyware', 'Cruel Angel''s Thesis', 3,
		 'You can use your Intelligence Modifier as your Genjutsu ability modifier.')`); err != nil {
		t.Fatal(err)
	}

	// int 16 (+3), wis 10 (+0) — deliberately different, to pin which
	// ability actually drives the computed modifier.
	newSpywareCharacter := func(name string) int64 {
		t.Helper()
		res, err := charDB.Exec(`
			INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
			VALUES (?, 10, 10, 10, 16, 10, 10)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := charDB.Exec(
			`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', 3, 0)`, id,
		); err != nil {
			t.Fatal(err)
		}
		return id
	}
	genjutsuOf := func(sheet *Sheet) JutsuAttack {
		for _, a := range sheet.JutsuAttacks {
			if a.Kind == "Genjutsu" {
				return a
			}
		}
		t.Fatal("no Genjutsu entry in JutsuAttacks")
		return JutsuAttack{}
	}

	// Granted: Genjutsu computes off Intelligence.
	withFeature := newSpywareCharacter("Spyware Test")
	if _, err := charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/spyware', 3)`, withFeature,
	); err != nil {
		t.Fatal(err)
	}
	sheet, err := Compute(rulesDB, charDB, withFeature)
	if err != nil {
		t.Fatalf("Compute (Cruel Angel's Thesis granted): %v", err)
	}
	prof := sheet.ProficiencyBonus
	if gen := genjutsuOf(sheet); gen.Ability != "int" || gen.Modifier != 3+prof {
		t.Errorf("Genjutsu (Cruel Angel's Thesis granted) = %s %+d, want int %+d", gen.Ability, gen.Modifier, 3+prof)
	}

	// Absent: same ability scores, no Spyware subclass — Genjutsu still
	// defaults to Wisdom.
	withoutFeature := newSpywareCharacter("No Spyware Test")
	sheet, err = Compute(rulesDB, charDB, withoutFeature)
	if err != nil {
		t.Fatalf("Compute (no subclass): %v", err)
	}
	if gen := genjutsuOf(sheet); gen.Ability != "wis" {
		t.Errorf("Genjutsu (no Cruel Angel's Thesis) = %s, want the wis default", gen.Ability)
	}

	// Granted, but with a manual override on top: the override still wins,
	// the same precedence TestComputeJutsuAttackAbilityOverride pins for
	// genjutsuExpertiseFeatSlug/mentalBoonsFeatSlug/gaseousHazeFeatureSlug.
	withOverride := newSpywareCharacter("Spyware Override Test")
	if _, err := charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/spyware', 3)`, withOverride,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_overrides (character_id, field, value, note) VALUES (?, 'genjutsu_ability', 'str', 'manual override')`, withOverride,
	); err != nil {
		t.Fatal(err)
	}
	sheet, err = Compute(rulesDB, charDB, withOverride)
	if err != nil {
		t.Fatalf("Compute (Cruel Angel's Thesis + manual override): %v", err)
	}
	if gen := genjutsuOf(sheet); gen.Ability != "str" {
		t.Errorf("Genjutsu (feature + manual override) = %s, want the str override to win", gen.Ability)
	}
}

func TestComputeClashChecks(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	// str 10 (+0), dex 16 (+3) — deliberately higher than str, to pin the
	// "whichever's better" branch of the Taijutsu/Bukijutsu formula. int 14
	// (+2), wis 12 (+1).
	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Clasher', 10, 16, 10, 14, 12, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := charDB.Exec(`
		INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref) VALUES
		(?, 'skill', 'Ninshou', 'other', 'test'),
		(?, 'skill', 'Martial Arts', 'other', 'test')`, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	prof := sheet.ProficiencyBonus

	want := map[string]int{
		"Ninjutsu":  2 + prof, // int, proficient (Ninshou)
		"Genjutsu":  1,        // wis, not proficient (Illusions) — no book-confirmed formula, shipped as the best available default
		"Taijutsu":  3 + prof, // dex (better than str), proficient (Martial Arts)
		"Bukijutsu": 3 + prof, // identical to Taijutsu by design, not its own Attack-ability override
	}
	if len(sheet.ClashChecks) != len(want) {
		t.Fatalf("got %d clash checks, want %d", len(sheet.ClashChecks), len(want))
	}
	for _, c := range sheet.ClashChecks {
		w, ok := want[c.Discipline]
		if !ok {
			t.Errorf("unexpected clash discipline %q", c.Discipline)
			continue
		}
		if c.Modifier != w {
			t.Errorf("%s clash = %+d, want %+d", c.Discipline, c.Modifier, w)
		}
	}

	var taiMod, bukiMod int
	for _, c := range sheet.ClashChecks {
		if c.Discipline == "Taijutsu" {
			taiMod = c.Modifier
		}
		if c.Discipline == "Bukijutsu" {
			bukiMod = c.Modifier
		}
	}
	if taiMod != bukiMod {
		t.Errorf("Taijutsu clash (%+d) and Bukijutsu clash (%+d) should always match", taiMod, bukiMod)
	}
}

func TestComputeClashChecksNoProficiencies(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	// str 14 (+2) > dex 10 (+0) here, to pin the other branch of "whichever's
	// better" than TestComputeClashChecks does. No proficiencies anywhere,
	// so every Clash modifier should be a bare ability modifier.
	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Untrained', 14, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	for _, c := range sheet.ClashChecks {
		var want int
		if c.Discipline == "Taijutsu" || c.Discipline == "Bukijutsu" {
			want = 2 // str modifier, the higher of str/dex here
		}
		if c.Modifier != want {
			t.Errorf("%s clash = %+d, want %+d (no proficiency, no bonus)", c.Discipline, c.Modifier, want)
		}
	}
}

func TestComputeClashChecksAbilityOverride(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	// str 10 (+0), dex 12 (+1) — dex is the "whichever's better" default for
	// Taijutsu/Bukijutsu here. int 14 (+2), cha 18 (+4). No proficiencies:
	// every modifier below is a bare ability modifier, so the override is
	// the only thing moving the number.
	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Overridden Clasher', 10, 12, 10, 14, 10, 18)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if _, err := charDB.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note) VALUES
		(?, '`+ClashAbilityField("Genjutsu")+`', 'cha', 'test override'),
		(?, '`+ClashAbilityField("Taijutsu")+`', 'nonsense', 'junk')`, id, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	byDiscipline := map[string]ClashCheck{}
	for _, c := range sheet.ClashChecks {
		byDiscipline[c.Discipline] = c
	}

	gen := byDiscipline["Genjutsu"]
	if gen.Ability != "cha" || gen.Modifier != 4 {
		t.Errorf("Genjutsu clash = %s %+d, want cha +4", gen.Ability, gen.Modifier)
	}
	// An unrecognised ability falls back to the default (dex, the better of
	// str/dex here) rather than computing off a score of zero.
	tai := byDiscipline["Taijutsu"]
	if tai.Ability != "dex" || tai.Modifier != 1 {
		t.Errorf("Taijutsu clash = %s %+d, want the dex default", tai.Ability, tai.Modifier)
	}
	// Bukijutsu shares Taijutsu's default but was never itself overridden —
	// confirms the two overrides are independent, not linked.
	buki := byDiscipline["Bukijutsu"]
	if buki.Ability != "dex" || buki.Modifier != 1 {
		t.Errorf("Bukijutsu clash = %s %+d, want the dex default", buki.Ability, buki.Modifier)
	}
	// Untouched by any override.
	nin := byDiscipline["Ninjutsu"]
	if nin.Ability != "int" || nin.Modifier != 2 {
		t.Errorf("Ninjutsu clash = %s %+d, want the int default", nin.Ability, nin.Modifier)
	}
}

// TestMasteryRankCap locks in the book's own level-gated ceiling (Chapter
// 6, page 49): "Between levels 1-6, you can only gain 1 Rank of Mastery.
// Between levels 7-11, up to 2 Ranks. When you reach levels 12+, up to 3
// Ranks."
func TestMasteryRankCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 1}, {6, 1}, {7, 2}, {11, 2}, {12, 3}, {20, 3},
	}
	for _, c := range cases {
		if got := MasteryRankCap(c.level); got != c.want {
			t.Errorf("MasteryRankCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// TestMasteryBonus locks in the book's own worked bonuses: "One Source:
// Rank 1 Mastery (+2), Two Sources: Rank 2 Mastery (+4), Three Sources:
// Rank 3 Mastery (+6)", plus its own hard ceiling: "If you have more than
// three sources of mastery, the maximum possible bonus you can have is +6."
func TestMasteryBonus(t *testing.T) {
	cases := []struct {
		rank, want int
	}{
		{0, 0}, {1, 2}, {2, 4}, {3, 6}, {4, 6}, {9, 6}, {-1, 0},
	}
	for _, c := range cases {
		if got := MasteryBonus(c.rank); got != c.want {
			t.Errorf("MasteryBonus(%d) = %d, want %d", c.rank, got, c.want)
		}
	}
}

// TestEffectiveMasteryRank covers the level clamp a stored rank passes
// through on the way to the sheet: the cap is a standing limit on what a
// character can have, so a rank stored at a higher level applies at the
// reduced rank while their level is lower, and never above Rank 3.
func TestEffectiveMasteryRank(t *testing.T) {
	cases := []struct {
		rank, level, want int
	}{
		{1, 1, 1},   // within the level-1 cap
		{2, 5, 1},   // stored Rank 2, character now level 5
		{3, 5, 1},   //   "        Rank 3
		{3, 9, 2},   // levels 7-11 allow Rank 2
		{3, 12, 3},  // levels 12+ allow the full Rank 3
		{4, 20, 3},  // never above the book's own ceiling
		{0, 20, 0},  // no entry, no rank
		{-1, 20, 0}, // nonsense stored value can't produce a negative bonus
	}
	for _, c := range cases {
		if got := EffectiveMasteryRank(c.rank, c.level); got != c.want {
			t.Errorf("EffectiveMasteryRank(rank %d, level %d) = %d, want %d", c.rank, c.level, got, c.want)
		}
	}
}

// TestMasteryRankLabel locks in the book's own Shinobi Rank vocabulary:
// "No Proficiency: Not Proficient. Proficiency: Genin Mastery. Rank 1
// Mastery: Chunin Mastery. Rank 2 Mastery: Jonin Mastery. Rank 3 Mastery:
// Sanin or Kage Mastery."
func TestMasteryRankLabel(t *testing.T) {
	cases := []struct {
		rank       int
		proficient bool
		want       string
	}{
		{0, false, "Not Proficient"},
		{0, true, "Genin"},
		{1, true, "Chunin"},
		{2, true, "Jonin"},
		{3, true, "Sanin or Kage"},
	}
	for _, c := range cases {
		if got := MasteryRankLabel(c.rank, c.proficient); got != c.want {
			t.Errorf("MasteryRankLabel(%d, %v) = %q, want %q", c.rank, c.proficient, got, c.want)
		}
	}
}

// TestComputeAppliesMasteryToSkillModifier confirms a stored Mastery entry
// actually reaches the computed sheet, not just the pure formula funcs
// above — Acrobatics (dex, +1 here), proficient, plus a Rank 2 Mastery
// entry (+4) on a level-7 character (the lowest level Rank 2 is legal at),
// should read ability(+1) + proficiency + Mastery(+4).
func TestComputeAppliesMasteryToSkillModifier(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Masterful', 10, 12, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 7, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref) VALUES (?, 'skill', 'Acrobatics', 'other', 'test')`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_mastery (character_id, skill_name, rank) VALUES (?, 'Acrobatics', 2)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	mod, proficient := SkillModifier(sheet, "Acrobatics")
	if !proficient {
		t.Fatal("Acrobatics should be proficient")
	}
	want := 1 + sheet.ProficiencyBonus + 4 // dex mod + proficiency + Rank 2 Mastery
	if mod != want {
		t.Errorf("Acrobatics modifier = %d, want %d (dex +1, proficiency +%d, Mastery Rank 2 +4)", mod, want, sheet.ProficiencyBonus)
	}
	if got := sheet.MasteryRanks["Acrobatics"]; got != 2 {
		t.Errorf("sheet.MasteryRanks[Acrobatics] = %d, want 2", got)
	}
}

// TestComputeClampsMasteryToLevelCap is the same setup one level lower:
// at level 6 the book allows only Rank 1, so a stored Rank 2 entry must
// apply as +2, not +4 — the cap is a standing limit on what a character can
// have, not a one-time gate at the moment the rank was recorded.
func TestComputeClampsMasteryToLevelCap(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Demoted', 10, 12, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 6, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref) VALUES (?, 'skill', 'Acrobatics', 'other', 'test')`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_mastery (character_id, skill_name, rank) VALUES (?, 'Acrobatics', 2)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	mod, _ := SkillModifier(sheet, "Acrobatics")
	want := 1 + sheet.ProficiencyBonus + 2 // Rank 2 stored, Rank 1 applied at level 6
	if mod != want {
		t.Errorf("Acrobatics modifier = %d, want %d (Rank 2 stored but capped to Rank 1 at level 6)", mod, want)
	}
	if got := sheet.MasteryRanks["Acrobatics"]; got != 1 {
		t.Errorf("sheet.MasteryRanks[Acrobatics] = %d, want 1 (clamped)", got)
	}
}

// TestComputeAppliesMasteryToToolkit confirms a Mastery entry on a toolkit
// reaches the sheet too: the book grants Mastery "with a given skill or
// toolkit", and a toolkit's own row is composed outside this package (see
// buildProficiencyRows), so Sheet.MasteryRanks is the only thing carrying
// the rank across — a name that isn't one of the 21 skills must not be
// dropped on the way out of Compute.
func TestComputeAppliesMasteryToToolkit(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	res, err := charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Tinker', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if _, err := charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/test', 12, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(
		`INSERT INTO character_mastery (character_id, skill_name, rank) VALUES (?, 'Armorsmith kit', 3)`, id,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := Compute(rulesDB, charDB, id)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got := sheet.MasteryRanks["Armorsmith kit"]; got != 3 {
		t.Errorf("sheet.MasteryRanks[Armorsmith kit] = %d, want 3", got)
	}
}

// TestComputeAppliesElementalInnovationistPermaPerkBonuses covers Perma
// Perk's (14th-level Elemental Innovationist) one always-on numeric clause
// for each of the four Perma-Perk-eligible E.I.Ps this package models
// (Speed/Stamina/Juggernaut/Vulture — Razor E.I.P's own crit-range bonus is
// applied in cmd/n5e/characters.go instead, since attack rows are built
// there). Also confirms a designated Perma Perk grants nothing before 14th
// level, the same "recheck the granting condition" guard
// TestComputeGatesMobilitySpeedBonusOnPick already establishes for a
// different feature.
func TestComputeAppliesElementalInnovationistPermaPerkBonuses(t *testing.T) {
	rulesDB, charDB := testDBs(t)

	if _, err := rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/elemental-innovationist', 'class/science-nin/group/scientific-inquiry', 'Elemental Innovationist')`); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		('class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/perma-perk',
		 'class/science-nin/group/scientific-inquiry/elemental-innovationist', 'Perma Perk', 14,
		 'Choose 1 E.I.P you know, gaining its benefits without Exoskeleton armor and without needing to spend CCD chakra to activate.')`); err != nil {
		t.Fatal(err)
	}

	// int 16 (+3) — pins Vulture E.I.P's Passive-Perception bonus to a
	// non-zero, easily-distinguished value.
	newCharacter := func(name string, level int) int64 {
		t.Helper()
		res, err := charDB.Exec(`
			INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
			VALUES (?, 10, 10, 10, 16, 10, 10)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := charDB.Exec(
			`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`, id, level,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := charDB.Exec(
			`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/elemental-innovationist', 3)`, id,
		); err != nil {
			t.Fatal(err)
		}
		return id
	}

	initiativeAbilityOf := func(sheet *Sheet) string { return sheet.InitiativeAbility }

	cases := []struct {
		name     string
		permaEIP string
		check    func(t *testing.T, sheet *Sheet, baseSpeed, baseMaxHP, basePassivePerception int)
	}{
		{
			name:     "Speed E.I.P swaps Initiative to Intelligence",
			permaEIP: elementalInnovationistSpeedEIPSlug,
			check: func(t *testing.T, sheet *Sheet, _, _, _ int) {
				if got := initiativeAbilityOf(sheet); got != "int" {
					t.Errorf("InitiativeAbility = %s, want int", got)
				}
			},
		},
		{
			name:     "Stamina E.I.P adds +5 Speed",
			permaEIP: elementalInnovationistStaminaEIPSlug,
			check: func(t *testing.T, sheet *Sheet, baseSpeed, _, _ int) {
				if sheet.Speed != baseSpeed+5 {
					t.Errorf("Speed = %d, want %d (base + Stamina E.I.P's +5)", sheet.Speed, baseSpeed+5)
				}
			},
		},
		{
			name:     "Juggernaut E.I.P adds 10 + Science-Nin level to Max HP",
			permaEIP: elementalInnovationistJuggernautEIPSlug,
			check: func(t *testing.T, sheet *Sheet, _, baseMaxHP, _ int) {
				want := baseMaxHP + 10 + 14
				if sheet.MaxHPAuto != want {
					t.Errorf("MaxHPAuto = %d, want %d (base + 10 + level 14)", sheet.MaxHPAuto, want)
				}
			},
		},
		{
			name:     "Vulture E.I.P adds Intelligence modifier to Passive Perception",
			permaEIP: elementalInnovationistVultureEIPSlug,
			check: func(t *testing.T, sheet *Sheet, _, _, basePassivePerception int) {
				want := basePassivePerception + 3
				if sheet.PassivePerception != want {
					t.Errorf("PassivePerception = %d, want %d (base + Int modifier +3)", sheet.PassivePerception, want)
				}
			},
		},
	}

	// Baseline at 14th level, no Perma Perk pick made yet — establishes what
	// "unboosted" looks like for each of the three additive fields above,
	// and confirms nothing is granted merely by having Perma Perk available.
	baseline := newCharacter("Baseline", 14)
	baseSheet, err := Compute(rulesDB, charDB, baseline)
	if err != nil {
		t.Fatalf("Compute (baseline, no pick): %v", err)
	}
	if got := initiativeAbilityOf(baseSheet); got != "dex" {
		t.Errorf("baseline InitiativeAbility = %s, want the dex default", got)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := newCharacter(tc.name, 14)
			if err := charstore.AddScienceNinSubclassPick(charDB, id, charstore.ScienceNinPickPermaPerk, tc.permaEIP, ""); err != nil {
				t.Fatal(err)
			}
			sheet, err := Compute(rulesDB, charDB, id)
			if err != nil {
				t.Fatalf("Compute (%s designated): %v", tc.permaEIP, err)
			}
			tc.check(t, sheet, baseSheet.Speed, baseSheet.MaxHPAuto, baseSheet.PassivePerception)
		})
	}

	// Below 14th level: the SAME Speed E.I.P designation grants nothing —
	// charstore.SetLevel/the level column here can be lowered after a pick
	// was made, and Perma Perk's own gate must be rechecked live rather than
	// trusting the stored pick's mere existence.
	underleveled := newCharacter("Underleveled", 13)
	if err := charstore.AddScienceNinSubclassPick(charDB, underleveled, charstore.ScienceNinPickPermaPerk, elementalInnovationistSpeedEIPSlug, ""); err != nil {
		t.Fatal(err)
	}
	sheet, err := Compute(rulesDB, charDB, underleveled)
	if err != nil {
		t.Fatalf("Compute (13th level, Speed E.I.P designated): %v", err)
	}
	if got := initiativeAbilityOf(sheet); got != "dex" {
		t.Errorf("13th-level InitiativeAbility = %s, want the dex default (Perma Perk not yet active)", got)
	}
}
