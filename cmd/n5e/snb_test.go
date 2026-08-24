package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// seedSNBRules inserts a minimal Science-Nin class, its S.N.B Specialist
// subclass, and the subclass_features rows loadSNBReference reads through
// v_subclass_features (Combat Programming, Combo Attack, Improved Servos,
// Artificial Sentience, Regenerative Armor, The Future of Shinobi: S.N.B) —
// enough for a real render without needing the full seeded rules.db, the
// same "minimal fixture, real query shape" approach seedTitanRules
// (titan_test.go) already uses for Mech Crafter.
func seedSNBRules(t *testing.T, s *server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 8)`,
		`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
			('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`,
		`INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'class/science-nin/group/scientific-inquiry', 'S.N.B Specialist')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + snbComboAttackFeatureSlug + `', 'class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'Combo Attack', 6, 'You and your S.N.B can combine attacks.')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + snbCombatProgrammingFeatureSlug + `', 'class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'Combat Programming', 6, 'Choose one of the following: Striker, Caster, Defensive, Lurker.')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + snbImprovedServosFeatureSlug + `', 'class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'Improved Servos', 9, 'Your S.N.B''s servos are improved.')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + snbArtificialSentienceFeatureSlug + `', 'class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'Artificial Sentience', 14, 'Your S.N.B gains a rudimentary sentience.')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + snbRegenerativeArmorFeatureSlug + `', 'class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'Regenerative Armor', 17, 'Your S.N.B can spend Hit Die to heal itself.')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + scienceNinFutureOfShinobiSNBFeatureSlug + `', 'class/science-nin/group/scientific-inquiry/s-n-b-specialist',
			 'The Future of Shinobi: S.N.B', 20, 'Your S.N.B reaches its final form.')`,
	}
	for _, stmt := range stmts {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("seed rules: %v\n%s", err, stmt)
		}
	}
}

// seedSNBCharacter inserts a character with the given Science-Nin level and
// a fixed Intelligence score (16, +3 modifier) — snbMaxHP/snbBiteAttack's
// own "Your Intelligence modifier" reads as the PLAYER's, so a non-zero,
// distinctive modifier here (rather than the flat 10/+0 other companion
// fixtures use) actually exercises that term instead of masking a wrong
// lookup behind a zero.
func seedSNBCharacter(t *testing.T, s *server, level int) {
	t.Helper()
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kabuto', 10, 10, 10, 16, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/science-nin', ?, 0)`,
		level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/science-nin/group/scientific-inquiry/s-n-b-specialist', 3)`); err != nil {
		t.Fatal(err)
	}
}

// TestSNBAC pins "10 + Your Proficiency Bonus (Natural Armor)", plus Combat
// Programming's own Defensive option folding in a further "1/3rd your
// Proficiency Bonus, rounded down" once picked, and Armored Exterior's own
// base-value override (12/13/14 by level) once known.
func TestSNBAC(t *testing.T) {
	if got := snbAC(2, 1, "", false); got != 12 {
		t.Errorf("snbAC(2, 1, \"\", false) = %d, want 12 (10+2, no pick)", got)
	}
	if got := snbAC(2, 1, "Striker", false); got != 12 {
		t.Errorf("snbAC(2, 1, Striker, false) = %d, want 12 (non-Defensive pick adds nothing)", got)
	}
	if got := snbAC(6, 6, "Defensive", false); got != 18 {
		t.Errorf("snbAC(6, 6, Defensive, false) = %d, want 18 (10+6+floor(6/3)=2)", got)
	}
	if got := snbAC(4, 4, "Defensive", false); got != 15 {
		t.Errorf("snbAC(4, 4, Defensive, false) = %d, want 15 (10+4+floor(4/3)=1)", got)
	}
	if got := snbAC(2, 6, "", true); got != 14 {
		t.Errorf("snbAC(2, 6, \"\", true) = %d, want 14 (Armored Exterior base 12+2, below 9th)", got)
	}
	if got := snbAC(2, 9, "", true); got != 15 {
		t.Errorf("snbAC(2, 9, \"\", true) = %d, want 15 (Armored Exterior base 13+2, at 9th)", got)
	}
	if got := snbAC(2, 17, "", true); got != 16 {
		t.Errorf("snbAC(2, 17, \"\", true) = %d, want 16 (Armored Exterior base 14+2, at 17th)", got)
	}
}

// TestSNBMaxHP pins "Your Intelligence modifier + your Science-Nin level
// times 5" read by plain left-to-right precedence (IntMod + Level*5, not
// (IntMod+Level)*5 — see snbMaxHP's own doc), the floor of 1 for a
// pathological negative-modifier case, and Sturdy's own additional "5 times
// your Science-Nin level" term once known.
func TestSNBMaxHP(t *testing.T) {
	cases := []struct {
		level, intMod int
		hasSturdy     bool
		want          int
	}{
		{1, 3, false, 8},    // 3 + 1*5
		{6, 3, false, 33},   // 3 + 6*5 — NOT (3+6)*5=45
		{1, -5, false, 1},   // 1*5-5=0, floored to 1
		{20, -1, false, 99}, // -1 + 20*5
		{6, 3, true, 63},    // 3 + 6*5 + 6*5 (Sturdy's own extra level*5 term)
	}
	for _, c := range cases {
		if got := snbMaxHP(c.level, c.intMod, c.hasSturdy); got != c.want {
			t.Errorf("snbMaxHP(%d, %d, %v) = %d, want %d", c.level, c.intMod, c.hasSturdy, got, c.want)
		}
	}
}

// TestSNBBiteDieForLevel pins "1d6 ... increases to a d8 at Level 9 and
// then a d10 at Level 14."
func TestSNBBiteDieForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 6}, {8, 6}, {9, 8}, {13, 8}, {14, 10}, {20, 10},
	}
	for _, c := range cases {
		if got := snbBiteDieForLevel(c.level); got != c.want {
			t.Errorf("snbBiteDieForLevel(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// TestSNBBiteAttack pins Bite's own "Your Intelligence modifier + Your Prof
// Bonus to hit ... 1d6 + Your Intelligence modifier piercing damage", with
// the Intelligence modifier read as the PLAYER's own (same reasoning as
// snbMaxHP's doc), not the S.N.B's own fixed -1 Int baseline, plus Improved
// Accuracy's own to-hit bonus once known.
func TestSNBBiteAttack(t *testing.T) {
	row := snbBiteAttack(9, 3, 4, 0)
	if row.Name != "Bite" {
		t.Errorf("Name = %q, want Bite", row.Name)
	}
	if row.AttackTotal != 3+4 {
		t.Errorf("AttackTotal = %d, want 7 (player int mod + prof)", row.AttackTotal)
	}
	if row.DamageTotal != 3 {
		t.Errorf("DamageTotal = %d, want 3 (player int mod only)", row.DamageTotal)
	}
	if row.DamageDice() != "1d8" {
		t.Errorf("DamageDice() = %q, want 1d8 (level 9 die size)", row.DamageDice())
	}
	if row.DamageType != "piercing" {
		t.Errorf("DamageType = %q, want piercing", row.DamageType)
	}

	withAccuracy := snbBiteAttack(9, 3, 4, 2)
	if withAccuracy.AttackTotal != 3+4+2 {
		t.Errorf("AttackTotal with Improved Accuracy = %d, want 9 (player int mod + prof + accuracy bonus)", withAccuracy.AttackTotal)
	}
}

// TestSNBEffectiveAbilityModifier covers the fallback-to-base-stat-block
// behavior a brand-new S.N.B (every ability score field blank) needs for
// Passive Perception to be sane, and confirms an explicitly-set companion
// score always wins over the baseline.
func TestSNBEffectiveAbilityModifier(t *testing.T) {
	blank := charstore.Companion{}
	if got := snbEffectiveAbilityModifier(blank, "wis"); got != -3 {
		t.Errorf("blank companion wis modifier = %d, want -3 (from the 5 base baseline)", got)
	}
	if got := snbEffectiveAbilityModifier(blank, "dex"); got != 2 {
		t.Errorf("blank companion dex modifier = %d, want 2 (from the 14 base baseline)", got)
	}

	set := charstore.Companion{Wis: sql.NullInt64{Int64: 20, Valid: true}}
	if got := snbEffectiveAbilityModifier(set, "wis"); got != 5 {
		t.Errorf("explicit wis=20 modifier = %d, want 5 (explicit score must win over baseline)", got)
	}
}

// TestPrefillSNBStatDefaultsOnCreation covers the fix that closed S.N.B's
// version of the same auto-compute-at-creation gap Nin-Dog/Titan had: a
// brand-new S.N.B must reach its first render already carrying AC/HP/Speed/
// ability scores computed from the base stat block, not sitting blank until
// the player clicks every Sync button by hand. Exercises the real HTTP
// handler (handleSheetCompanionAdd), not prefillSNBStatDefaults directly, so
// this also pins that the creation handler's own kind=="snb" branch
// actually calls it.
func TestPrefillSNBStatDefaultsOnCreation(t *testing.T) {
	s := testServer(t)
	seedSNBRules(t, s)
	// Level 6: past Combat Programming's own gate, non-trivial enough that a
	// wrong level/Int-modifier lookup would actually be caught.
	seedSNBCharacter(t, s, 6)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/companions",
		strings.NewReader(url.Values{"name": {"Unit-7"}, "kind": {"snb"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetCompanionAdd(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add snb companion: status %d, body %s", w.Code, w.Body.String())
	}

	companions, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(companions) != 1 {
		t.Fatalf("ListCompanions = %d rows, want 1", len(companions))
	}
	c := companions[0]

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	playerIntMod := sheet.Abilities["int"].Modifier // base_int 16 => +3

	wantAC := int64(snbAC(sheet.ProficiencyBonus, 6, "", false)) // no Combat Programming pick, no upgrades yet
	wantHP := int64(snbMaxHP(6, playerIntMod, false))
	wantSpeed := int64(snbBaseSpeed)

	if !c.AC.Valid || c.AC.Int64 != wantAC {
		t.Errorf("AC = %+v, want %d", c.AC, wantAC)
	}
	if !c.HPCurrent.Valid || c.HPCurrent.Int64 != wantHP {
		t.Errorf("HPCurrent = %+v, want %d", c.HPCurrent, wantHP)
	}
	if !c.HPMax.Valid || c.HPMax.Int64 != wantHP {
		t.Errorf("HPMax = %+v, want %d", c.HPMax, wantHP)
	}
	if !c.Speed.Valid || c.Speed.Int64 != wantSpeed {
		t.Errorf("Speed = %+v, want %d", c.Speed, wantSpeed)
	}
	for key, dest := range map[string]sql.NullInt64{
		"str": c.Str, "dex": c.Dex, "con": c.Con, "int": c.Int, "wis": c.Wis, "cha": c.Cha,
	} {
		want := int64(snbBaseAbilityScores[key])
		if !dest.Valid || dest.Int64 != want {
			t.Errorf("%s = %+v, want %d (base stat block baseline)", key, dest, want)
		}
	}

	// loadSNBReference now recomputes and unconditionally overwrites AC (and
	// every other formula field) on every render — see the identical note
	// in nindog_test.go's TestPrefillNinDogStatDefaultsOnCreation. Pinning
	// via character_companion_overrides (migration 0079) is what protects a
	// value across a re-render now, not a bare column edit.
	if err := charstore.SetCompanionOverride(s.charDB, c.ID, "ac", "99"); err != nil {
		t.Fatal(err)
	}
	if err := s.prefillSNBStatDefaults(1, c.ID); err != nil {
		t.Fatal(err)
	}
	after, err := charstore.GetCompanion(s.charDB, 1, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.AC.Valid || after.AC.Int64 != 99 {
		t.Errorf("re-running the prefill overwrote a pinned AC: got %+v, want 99 preserved", after.AC)
	}
}

// TestLoadSNBReferenceFeatureLocking covers the level-gated Features list:
// Combo Attack/Combat Programming (6th), Improved Servos (9th), Artificial
// Sentience (14th), Regenerative Armor (17th), and The Future of Shinobi:
// S.N.B (20th) must each report Locked in lockstep with the character's own
// total level, the same "shown either way, Locked flag set" treatment
// loadNinDogReference/loadTitanReference already give their own feature
// lists.
func TestLoadSNBReferenceFeatureLocking(t *testing.T) {
	s := testServer(t)
	seedSNBRules(t, s)
	seedSNBCharacter(t, s, 6)
	companionID, err := charstore.AddCompanion(s.charDB, 1, "snb", "Unit-7")
	if err != nil {
		t.Fatal(err)
	}
	companion, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.loadSNBReference(1, companion, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil {
		t.Fatal("loadSNBReference returned nil, want a populated panel (unlike Titan, an S.N.B is never level-gated to nil)")
	}

	wantLocked := map[string]bool{
		"Combo Attack":                 false, // 6th, met
		"Improved Servos":              true,  // 9th, not yet
		"Artificial Sentience":         true,  // 14th, not yet
		"Regenerative Armor":           true,  // 17th, not yet
		"The Future of Shinobi: S.N.B": true,  // 20th, not yet
	}
	seen := map[string]bool{}
	for _, f := range ref.Features {
		seen[f.Name] = true
		want, ok := wantLocked[f.Name]
		if !ok {
			t.Errorf("unexpected feature %q in Features", f.Name)
			continue
		}
		if f.Locked != want {
			t.Errorf("feature %q Locked = %v, want %v (character level %d, feature level %d)", f.Name, f.Locked, want, sheet.Level, f.Level)
		}
	}
	for name := range wantLocked {
		if !seen[name] {
			t.Errorf("feature %q missing from Features", name)
		}
	}

	if !ref.CombatProgrammingUnlocked {
		t.Error("CombatProgrammingUnlocked = false at level 6, want true")
	}

	// Below the 6th-level gate: Combat Programming and Combo Attack both
	// lock, and CombatProgrammingUnlocked flips false.
	if _, err := s.charDB.Exec(`UPDATE character_classes SET levels = 1 WHERE character_id = 1`); err != nil {
		t.Fatal(err)
	}
	sheetLow, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	refLow, err := s.loadSNBReference(1, companion, sheetLow)
	if err != nil {
		t.Fatal(err)
	}
	if refLow.CombatProgrammingUnlocked {
		t.Error("CombatProgrammingUnlocked = true at level 1, want false")
	}
	for _, f := range refLow.Features {
		if f.Name == "Combo Attack" && !f.Locked {
			t.Error("Combo Attack Locked = false at level 1, want true")
		}
	}

	// Base traits (Artificial Intelligence, Bio-Organic Identifier,
	// Internal Radio Link) are always shown, ungated by level.
	if len(ref.Traits) != 3 {
		t.Errorf("Traits = %d entries, want 3 (always-on base traits)", len(ref.Traits))
	}
}

// TestSNBAttacksIncludeComputedBite covers the fix that closed S.N.B's own
// version of the typed-free-text-Attacks gap Nin-Dog/Titan already had
// closed (companionStructuredAttackKinds' own doc): both loadSummonsTabData
// (Companions tab) and handleCompanionSheet (standalone popup) must append
// the computed Bite row (snbBiteAttack) after any player-added rows, the
// same "structured Attacks plus one computed baseline row" shape
// TestNinDogBiteAttack/TestTitanBashAttack already pin at the unit level —
// this is the integration-level confirmation that the append actually
// happens at both real call sites, not just that the function itself is
// correct in isolation. See companions_test.go's own
// TestSummonCustomAttacksLoadStructured for the sibling summon/custom case,
// where no baseline row is ever appended.
func TestSNBAttacksIncludeComputedBite(t *testing.T) {
	s := testServer(t)
	seedSNBRules(t, s)
	seedSNBCharacter(t, s, 9) // level 9: Bite's own die size steps up to 1d8

	snbID, err := charstore.AddCompanion(s.charDB, 1, "snb", "Unit-7")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionAttack(s.charDB, 1, snbID, charstore.CompanionAttack{
		Name: "Taser Jab", AttackAbility: "int", DamageCount: 1, DamageSides: 4, DamageAbility: "int", DamageType: "lightning",
	}); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := s.loadSummonsTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	var got []companionAttackRow
	for _, v := range tab.Companions {
		if v.Companion.ID == snbID {
			got = v.Attacks
		}
	}
	if len(got) != 2 || got[0].Name != "Taser Jab" || got[1].Name != "Bite" {
		t.Fatalf("Companions-tab snb Attacks = %+v, want [Taser Jab, Bite] (player row, then the computed baseline)", got)
	}
	if got[1].DamageDice() != "1d8" {
		t.Errorf("computed Bite row DamageDice() = %q, want 1d8 (level 9 die size)", got[1].DamageDice())
	}

	cid := strconv.FormatInt(snbID, 10)
	req := httptest.NewRequest(http.MethodGet, "/characters/1/companions/"+cid, nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handleCompanionSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("open snb popup: status %d, body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Taser Jab") || !strings.Contains(body, "Bite") {
		t.Errorf("snb popup missing one of the player row or the computed Bite row:\n%s", body)
	}
	if strings.Contains(body, `name="attacks"`) {
		t.Errorf("snb popup fell back to the freeform attacks textarea instead of structured Attacks")
	}
}

// TestSNBCombatProgrammingPick covers the actual persisted-choice route
// (handleSNBCombatProgrammingPick): a valid option name at or above 6th
// level is accepted and reflected back on the next loadSNBReference call
// (including AC's own Defensive-pick bonus), an invalid option name is
// rejected, and the pick is unreachable below 6th level even with a valid
// option name — the same one-shot-pick shape
// handleNinDogHijutsuTraitPick's own tests already cover for Beast Master's
// bonus Hijutsu pick.
func TestSNBCombatProgrammingPick(t *testing.T) {
	s := testServer(t)
	seedSNBRules(t, s)
	seedSNBCharacter(t, s, 6)
	companionID, err := charstore.AddCompanion(s.charDB, 1, "snb", "Unit-7")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	post := func(optionName string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/snb-combat-programming",
			strings.NewReader(url.Values{"option_name": {optionName}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleSNBCombatProgrammingPick(w, req)
		return w
	}

	if w := post("Not A Real Option"); w.Code != http.StatusBadRequest {
		t.Errorf("invalid option: status %d, want 400", w.Code)
	}

	if w := post("Defensive"); w.Code != http.StatusOK {
		t.Fatalf("valid option: status %d, body %s", w.Code, w.Body.String())
	}

	pick, err := charstore.GetFeatureCompanionChoice(s.charDB, 1, snbCombatProgrammingFeatureSlug, companionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pick != "Defensive" {
		t.Errorf("persisted pick = %q, want Defensive", pick)
	}

	companion, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.loadSNBReference(1, companion, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if ref.CombatProgrammingPick != "Defensive" {
		t.Errorf("ref.CombatProgrammingPick = %q, want Defensive", ref.CombatProgrammingPick)
	}
	wantAC := snbAC(sheet.ProficiencyBonus, sheet.Level, "Defensive", false)
	if ref.ExpectedAC != wantAC {
		t.Errorf("ExpectedAC after Defensive pick = %d, want %d (Defensive's own AC bonus folded in)", ref.ExpectedAC, wantAC)
	}

	// Below the 6th-level gate, the pick route itself must refuse — not
	// just the template hiding the picker form.
	if _, err := s.charDB.Exec(`UPDATE character_classes SET levels = 1 WHERE character_id = 1`); err != nil {
		t.Fatal(err)
	}
	if w := post("Striker"); w.Code != http.StatusBadRequest {
		t.Errorf("pick below level 6: status %d, want 400", w.Code)
	}

	// A wrong companion kind must also be rejected server-side.
	otherID, err := charstore.AddCompanion(s.charDB, 1, "custom", "Not An SNB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`UPDATE character_classes SET levels = 6 WHERE character_id = 1`); err != nil {
		t.Fatal(err)
	}
	otherCid := strconv.FormatInt(otherID, 10)
	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+otherCid+"/snb-combat-programming",
		strings.NewReader(url.Values{"option_name": {"Striker"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", otherCid)
	w := httptest.NewRecorder()
	s.handleSNBCombatProgrammingPick(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("pick on a non-snb companion: status %d, want 400", w.Code)
	}
}
