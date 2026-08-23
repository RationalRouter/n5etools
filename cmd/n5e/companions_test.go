package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// TestCompanionKindLabel covers the one shared kind-to-display-label
// mapping every companion-kind badge render site (the Core tab's
// Companions box, the Companions tab's per-companion card header, the
// standalone companion popup) now goes through, for all six kinds this
// app creates plus a defensive fallback for anything not in the table.
// "custom" must map to "Other" specifically — matching the Add Companion
// dropdown's own option text, not a generic Title Case of the raw value —
// and "titan" must resolve correctly here even though this pass's own
// main focus is nin-dog, since a later Titan-specific stage relies on this
// helper already being correct for every kind, not just the ones a given
// stage happens to be working on.
func TestCompanionKindLabel(t *testing.T) {
	cases := []struct{ kind, want string }{
		{"puppet", "Puppet"},
		{"summon", "Summon"},
		{"nin-dog", "Nin-Dog"},
		{"titan", "Titan"},
		{"snb", "S.N.B"},
		{"custom", "Other"},
		{"some-future-kind", "some-future-kind"}, // fallback: raw value, not blank
	}
	for _, c := range cases {
		if got := companionKindLabel(c.kind); got != c.want {
			t.Errorf("companionKindLabel(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}

// TestCompanionSupportsStructuredAttacks pins the whitelist gating the
// structured/rollable Attacks presentation, both for the template data a
// popup/tab card is given and for the attack add/delete handlers'
// server-side guard. Titan, summon, custom, and snb each had (or, for snb,
// would otherwise have) the identical free-text bug and fix shape as
// Nin-Dog (see companionStructuredAttackKinds' own doc) and are now all in
// the whitelist — every kind this app can actually create today — with a
// defensive fallback still covering any future kind that hasn't been added
// yet.
func TestCompanionSupportsStructuredAttacks(t *testing.T) {
	cases := []struct {
		kind string
		want bool
	}{
		{"puppet", true},
		{"nin-dog", true},
		{"titan", true},
		{"summon", true},
		{"custom", true},
		{"snb", true},
		{"some-future-kind", false},
	}
	for _, c := range cases {
		if got := companionSupportsStructuredAttacks(c.kind); got != c.want {
			t.Errorf("companionSupportsStructuredAttacks(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}

// TestCompanionAttacksFragment pins which sheet fragment a structured
// attack add/delete responds with per kind: puppet-kind companions only
// ever render structured Attacks on the Puppets tab, every other
// structured-attacks kind (nin-dog, titan) renders on the Companions tab.
func TestCompanionAttacksFragment(t *testing.T) {
	if got := companionAttacksFragment("puppet"); got != "sheet_puppet_tab" {
		t.Errorf("companionAttacksFragment(puppet) = %q, want sheet_puppet_tab", got)
	}
	if got := companionAttacksFragment("nin-dog"); got != "sheet_summon_tab" {
		t.Errorf("companionAttacksFragment(nin-dog) = %q, want sheet_summon_tab", got)
	}
	if got := companionAttacksFragment("titan"); got != "sheet_summon_tab" {
		t.Errorf("companionAttacksFragment(titan) = %q, want sheet_summon_tab", got)
	}
	if got := companionAttacksFragment("snb"); got != "sheet_summon_tab" {
		t.Errorf("companionAttacksFragment(snb) = %q, want sheet_summon_tab", got)
	}
}

// TestSummonCustomAttacksLoadStructured pins the actual data-loading behavior
// companionStructuredAttackKinds' extension to summon/custom depends on —
// TestCompanionSupportsStructuredAttacks above only covers the whitelist
// lookup itself, not that loadSummonsTabData (Companions tab) and
// handleCompanionSheet (standalone popup) actually populate structured
// Attacks for these two kinds now, with no baseline attack appended (see
// loadSummonsTabData's own doc — neither has a computed natural weapon the
// way Nin-Dog Bite/Titan Bash/S.N.B Bite do; snb_test.go's own
// TestSNBAttacksIncludeComputedBite covers that third, structurally
// different case instead of being folded in here).
func TestSummonCustomAttacksLoadStructured(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Attacker', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	summonID, err := charstore.AddCompanion(s.charDB, 1, "summon", "Fluffy")
	if err != nil {
		t.Fatal(err)
	}
	customID, err := charstore.AddCompanion(s.charDB, 1, "custom", "Sidekick")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionAttack(s.charDB, 1, summonID, charstore.CompanionAttack{
		Name: "Claw Swipe", AttackAbility: "str", DamageCount: 1, DamageSides: 6, DamageAbility: "str", DamageType: "slashing",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionAttack(s.charDB, 1, customID, charstore.CompanionAttack{
		Name: "Improvised Bonk", AttackAbility: "str", DamageCount: 1, DamageSides: 4, DamageAbility: "str", DamageType: "bludgeoning",
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
	byID := map[int64]summonCompanionView{}
	for _, v := range tab.Companions {
		byID[v.Companion.ID] = v
	}
	if got := byID[summonID].Attacks; len(got) != 1 || got[0].Name != "Claw Swipe" {
		t.Errorf("summon Attacks = %+v, want exactly the one player-added row, nothing appended", got)
	}
	if got := byID[customID].Attacks; len(got) != 1 || got[0].Name != "Improvised Bonk" {
		t.Errorf("custom Attacks = %+v, want exactly the one player-added row, nothing appended", got)
	}

	for _, tc := range []struct {
		id         int64
		attackName string
	}{
		{summonID, "Claw Swipe"},
		{customID, "Improvised Bonk"},
	} {
		cid := strconv.FormatInt(tc.id, 10)
		req := httptest.NewRequest(http.MethodGet, "/characters/1/companions/"+cid, nil)
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleCompanionSheet(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("open companion %d popup: status %d, body %s", tc.id, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "companion-attack-row") || !strings.Contains(body, tc.attackName) {
			t.Errorf("companion %d popup: expected a structured companion-attack-row for %q, got %s", tc.id, tc.attackName, body)
		}
		if strings.Contains(body, `name="attacks"`) {
			t.Errorf("companion %d popup: fell back to the freeform attacks textarea instead of structured Attacks", tc.id)
		}
	}
}

func TestHighestRankForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "D"}, {4, "D"}, {5, "C"}, {8, "C"}, {9, "B"}, {12, "B"},
		{13, "A"}, {16, "A"}, {17, "S"}, {20, "S"},
	}
	for _, c := range cases {
		if got := highestRankForLevel(c.level); got != c.want {
			t.Errorf("highestRankForLevel(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

// seedPuppetMasterRules inserts a minimal Puppet Master class with one
// subclass and two level-gated features — enough to exercise
// loadCharacterReference's real query shape without needing the full
// seeded rules.db.
func seedPuppetMasterRules(t *testing.T, s *server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 8)`,
		`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
			('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`,
		`INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/puppet-master/group/puppet-techniques/blue-technique-warmaster',
			 'class/puppet-master/group/puppet-techniques', 'Blue Technique ~ Warmaster')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('class/puppet-master/group/puppet-techniques/blue-technique-warmaster/feature/puppet-weapon-types',
			 'class/puppet-master/group/puppet-techniques/blue-technique-warmaster',
			 'Puppet Weapon Types', 2, 'Select a Puppet Weapon Type.')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('class/puppet-master/group/puppet-techniques/blue-technique-warmaster/feature/transformer',
			 'class/puppet-master/group/puppet-techniques/blue-technique-warmaster',
			 'Transformer', 14, 'Select a second Puppet Weapon Type for your Puppet Tool.')`,
	}
	for _, stmt := range stmts {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("seed rules: %v\n%s", err, stmt)
		}
	}
}

func TestLoadCharacterReferenceNoClass(t *testing.T) {
	s := testServer(t)
	seedPuppetMasterRules(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('No Class')`); err != nil {
		t.Fatal(err)
	}
	ref, err := s.loadCharacterReference(1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ref.Features) != 0 {
		t.Errorf("Features = %v, want none for a character with no class levels at all", ref.Features)
	}
}

func TestLoadCharacterReferenceNoSubclassYet(t *testing.T) {
	s := testServer(t)
	seedPuppetMasterRules(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Fresh Puppeteer')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/puppet-master', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	ref, err := s.loadCharacterReference(1, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ref.Features {
		if f.IsSubclass {
			t.Errorf("Features = %v, want no subclass entries with no subclass chosen", ref.Features)
		}
	}
}

// TestLoadCharacterReferenceLevelGating is the concrete scenario from the
// feature request: Blue Technique ~ Warmaster's Transformer feature (level
// 14) grants a second Puppet Weapon Type, and must show as locked below
// level 14 and unlocked at or above it — the same level-gating
// loadGrantedFeatures already applies to the main sheet's Features panel,
// reused here for the Class Reference popup.
func TestLoadCharacterReferenceLevelGating(t *testing.T) {
	s := testServer(t)
	seedPuppetMasterRules(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Kankuro')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/puppet-master', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/puppet-master/group/puppet-techniques/blue-technique-warmaster', 2)`); err != nil {
		t.Fatal(err)
	}

	ref, err := s.loadCharacterReference(1, "")
	if err != nil {
		t.Fatal(err)
	}
	var subFeatures []referenceFeatureEntry
	for _, f := range ref.Features {
		if f.IsSubclass {
			subFeatures = append(subFeatures, f)
		}
	}
	if len(subFeatures) != 2 {
		t.Fatalf("subclass Features = %v, want 2 entries", subFeatures)
	}
	wantSource := "Puppet Master — Blue Technique ~ Warmaster"
	for _, f := range subFeatures {
		if f.SourceLabel != wantSource {
			t.Errorf("SourceLabel = %q, want %q", f.SourceLabel, wantSource)
		}
	}
	// Merged list is sorted by Level, so Puppet Weapon Types (2) must sort
	// before Transformer (14) — the actual "interleaved, not stacked"
	// behavior this whole restructure exists to guarantee.
	if subFeatures[0].Name != "Puppet Weapon Types" || subFeatures[1].Name != "Transformer" {
		t.Errorf("subclass Features order = [%s, %s], want [Puppet Weapon Types, Transformer]", subFeatures[0].Name, subFeatures[1].Name)
	}
	byName := map[string]referenceFeatureEntry{}
	for _, f := range subFeatures {
		byName[f.Name] = f
	}
	if byName["Puppet Weapon Types"].Locked {
		t.Errorf("Puppet Weapon Types (level 2) should be unlocked at character level 10")
	}
	if !byName["Transformer"].Locked {
		t.Errorf("Transformer (level 14) should be locked at character level 10")
	}

	// Level up to 14: Transformer should unlock.
	if _, err := s.charDB.Exec(`UPDATE character_classes SET levels = 14 WHERE character_id = 1`); err != nil {
		t.Fatal(err)
	}
	ref, err = s.loadCharacterReference(1, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ref.Features {
		if f.Name == "Transformer" && f.Locked {
			t.Errorf("Transformer should be unlocked at character level 14")
		}
	}
}

// seedBearTribeRules inserts a minimal summon tribe with one D-Rank and one
// A-Rank feature, so rank-gating can be exercised the same way
// TestLoadPuppetMasterReferenceLevelGating exercises class-level gating.
func seedBearTribeRules(t *testing.T, s *server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO summon_tribes (slug, name, summon_type) VALUES ('summon/bear', 'Bear', 'Carnivoran')`,
		`INSERT INTO summon_tribe_features (slug, tribe_slug, name, rank, description, sort_order) VALUES
			('summon/bear/feature/claw-flex', 'summon/bear', 'Claw Flex', 'D', 'A basic claw trick.', 1)`,
		`INSERT INTO summon_tribe_features (slug, tribe_slug, name, rank, description, sort_order) VALUES
			('summon/bear/feature/apex-predator', 'summon/bear', 'Apex Predator', 'A', 'A powerful late-game trick.', 2)`,
		`INSERT INTO summon_tribe_attacks (tribe_slug, name, description) VALUES
			('summon/bear', 'Claws', 'Melee Weapon Attack.')`,
	}
	for _, stmt := range stmts {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("seed rules: %v\n%s", err, stmt)
		}
	}
}

func TestLoadSummonTribeReferenceRankGating(t *testing.T) {
	s := testServer(t)
	seedBearTribeRules(t, s)

	// Character level 3 -> highestRankForLevel D: only Claw Flex unlocked.
	ref, err := s.loadSummonTribeReference("summon/bear", 3)
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil {
		t.Fatal("loadSummonTribeReference returned nil for a tribe that exists")
	}
	byName := map[string]companionFeatureRef{}
	for _, f := range ref.Features {
		byName[f.Name] = f
	}
	if byName["Claw Flex"].Locked {
		t.Errorf("Claw Flex (D-Rank) should be unlocked at character level 3")
	}
	if !byName["Apex Predator"].Locked {
		t.Errorf("Apex Predator (A-Rank) should be locked at character level 3")
	}
	if len(ref.Attacks) != 1 || ref.Attacks[0].Name != "Claws" {
		t.Errorf("Attacks = %v, want the tribe's one natural weapon, always shown", ref.Attacks)
	}

	// Character level 15 -> highestRankForLevel A: both unlocked.
	ref, err = s.loadSummonTribeReference("summon/bear", 15)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ref.Features {
		if f.Locked {
			t.Errorf("%s should be unlocked at character level 15, rank order caught up", f.Name)
		}
	}
}

func TestLoadSummonTribeReferenceUnknownSlug(t *testing.T) {
	s := testServer(t)
	seedBearTribeRules(t, s)
	ref, err := s.loadSummonTribeReference("summon/does-not-exist", 5)
	if err != nil {
		t.Fatal(err)
	}
	if ref != nil {
		t.Errorf("loadSummonTribeReference should return nil for an unknown tribe slug, got %+v", ref)
	}
}

// TestCompanionSheetEndToEnd covers the full add -> open -> autosave ->
// delete lifecycle through the real HTTP handlers, one companion per kind,
// the same shape TestHandleSheetRyo uses for the main sheet's own POST
// handlers.
func TestCompanionSheetEndToEnd(t *testing.T) {
	s := testServer(t)
	seedPuppetMasterRules(t, s)
	seedBearTribeRules(t, s)
	// seedPuppetMasterRules only seeds Blue Technique Warmaster; Purple
	// Technique Juggernaut needs its own row here since the character
	// below picks it (armor_chassis save below is Purple-exclusive).
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut',
			 'class/puppet-master/group/puppet-techniques', 'Purple Technique ~ Juggernaut')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kankuro', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/puppet-master', 14, 0)`); err != nil {
		t.Fatal(err)
	}
	// Purple Technique Juggernaut, not an arbitrary subclass: this test
	// exercises the whole-form save's armor_chassis field below, which is
	// Purple's own exclusive Armor Chassis feature (see
	// handleCompanionSave's gate) — any other subclass would have that
	// field silently dropped.
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 2)`); err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{"puppet", "summon", "custom", "snb"} {
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/companions",
			strings.NewReader(url.Values{"name": {"Test " + kind}, "kind": {kind}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetCompanionAdd(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("add %s companion: status %d, body %s", kind, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Test "+kind) {
			t.Errorf("add %s companion: fragment missing the new row, got %s", kind, w.Body.String())
		}
	}

	companions, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(companions) != 4 {
		t.Fatalf("ListCompanions = %d rows, want 4", len(companions))
	}

	for _, c := range companions {
		cid := strconv.FormatInt(c.ID, 10)
		req := httptest.NewRequest(http.MethodGet, "/characters/1/companions/"+cid, nil)
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleCompanionSheet(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("open %s companion popup: status %d, body %s", c.Kind, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Test "+c.Kind) {
			t.Errorf("open %s companion popup: page missing its own name, got %s", c.Kind, w.Body.String())
		}
	}

	puppet := companions[0]
	if puppet.Kind != "puppet" {
		t.Fatalf("expected the first added companion to be the puppet, got %s", puppet.Kind)
	}
	puppetID := strconv.FormatInt(puppet.ID, 10)

	// AC has its own dedicated endpoint (handleCompanionIntField), same
	// shape as handleCompanionHP — set it there, not through the whole-form
	// save, which no longer carries "ac" at all (see the regression test
	// below for why that matters).
	acReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+puppetID+"/ac",
		strings.NewReader(url.Values{"value": {"15"}}.Encode()))
	acReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	acReq.SetPathValue("id", "1")
	acReq.SetPathValue("cid", puppetID)
	acW := httptest.NewRecorder()
	s.handleCompanionIntField("ac")(acW, acReq)
	if acW.Code != http.StatusOK || acW.Body.String() != "15" {
		t.Fatalf("set AC: status %d, body %q", acW.Code, acW.Body.String())
	}

	saveReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+puppetID,
		strings.NewReader(url.Values{
			"name":          {"Karasu"},
			"attacks":       {"Iron Fan Strike +6, 1d8+3 slashing"},
			"armor_chassis": {"Warforged"}, "is_armor_form": {"1"},
		}.Encode()))
	saveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	saveReq.SetPathValue("id", "1")
	saveReq.SetPathValue("cid", puppetID)
	saveW := httptest.NewRecorder()
	s.handleCompanionSave(saveW, saveReq)
	if saveW.Code != http.StatusNoContent {
		t.Fatalf("save companion: status %d, body %s", saveW.Code, saveW.Body.String())
	}
	saved, err := charstore.GetCompanion(s.charDB, 1, puppet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Karasu" || !saved.AC.Valid || saved.AC.Int64 != 15 || saved.Attacks != "Iron Fan Strike +6, 1d8+3 slashing" {
		t.Errorf("saved companion = %+v, want Karasu/AC15 (still, from its own endpoint)/the attack line", saved)
	}
	if saved.ArmorChassis != "Warforged" || !saved.IsArmorForm {
		t.Errorf("saved companion armor fields = %q/%v, want Warforged/true", saved.ArmorChassis, saved.IsArmorForm)
	}

	// A later whole-form save that omits is_armor_form (the checkbox left
	// unchecked, which browsers simply exclude from the POST body) must
	// turn it back off, not leave the earlier true value stuck — same
	// "the form's own field set is authoritative" rule SetCompanionFields
	// already follows for every other field.
	uncheckReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+puppetID,
		strings.NewReader(url.Values{
			"name":          {"Karasu"},
			"attacks":       {"Iron Fan Strike +6, 1d8+3 slashing"},
			"armor_chassis": {"Warforged"},
		}.Encode()))
	uncheckReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	uncheckReq.SetPathValue("id", "1")
	uncheckReq.SetPathValue("cid", puppetID)
	uncheckW := httptest.NewRecorder()
	s.handleCompanionSave(uncheckW, uncheckReq)
	if uncheckW.Code != http.StatusNoContent {
		t.Fatalf("save companion (unchecked): status %d, body %s", uncheckW.Code, uncheckW.Body.String())
	}
	saved, err = charstore.GetCompanion(s.charDB, 1, puppet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.IsArmorForm {
		t.Errorf("IsArmorForm = true, want false after a save without the checkbox field")
	}
	// The regression this whole section guards against: AC must still be 15
	// after TWO whole-form saves that never once carried "ac" — see
	// SetCompanionFields' own doc for the real bug this caught live (AC/
	// hp_max were still being read and unconditionally overwritten here
	// even after their own delta-editable fields were split out of this
	// form, silently nulling them on the very next unrelated field's blur).
	if !saved.AC.Valid || saved.AC.Int64 != 15 {
		t.Errorf("AC = %+v after whole-form saves with no \"ac\" field, want it to still be 15", saved.AC)
	}

	delReq := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/companions/"+puppetID+"/delete", nil)
	delReq.Header.Set("X-Requested-With", "fetch")
	delReq.SetPathValue("id", "1")
	delReq.SetPathValue("cid", puppetID)
	delW := httptest.NewRecorder()
	s.handleSheetCompanionDelete(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("delete companion: status %d, body %s", delW.Code, delW.Body.String())
	}
	remaining, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 3 {
		t.Errorf("after delete, ListCompanions = %d rows, want 3", len(remaining))
	}
}

// TestHandleCompanionSaveDropsArmorChassisForNonPurple is the server-side
// half of the Armor Chassis gate (see TestPuppetArmorChassisGatedToPurpleTechnique
// for the AC-formula half): the popup's own picker only ever offers Armor
// Chassis to a Purple Technique character, but that's UI convenience, not
// enforcement — a raw POST to the same endpoint (exactly what the audit's
// live reproduction did) must not be able to give a Blue Technique
// character's Puppet Tool a stored armor_chassis value at all.
func TestHandleCompanionSaveDropsArmorChassisForNonPurple(t *testing.T) {
	s := testServer(t)
	seedPuppetMasterRules(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Kankuro')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/puppet-master', 5, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/puppet-master/group/puppet-techniques/blue-technique-warmaster', 2)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid,
		strings.NewReader(url.Values{"name": {"Karasu"}, "armor_chassis": {"Steel Fortress"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handleCompanionSave(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("save companion: status %d, body %s", w.Code, w.Body.String())
	}
	saved, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ArmorChassis != "" {
		t.Errorf("ArmorChassis = %q, want empty — a Blue Technique character has no Purple-exclusive Armor Chassis to set", saved.ArmorChassis)
	}
}

// TestHandleCompanionHP covers the companion popup's HP box: a leading
// +/- adjusts hp_current (floored at 0), a bare number sets it outright,
// and blank clears it — the same dual-purpose input shape as the main
// sheet's Ryo box, split into its own endpoint/form so a leftover "+3"
// never gets silently resubmitted by an unrelated field's autosave (see
// handleCompanionHP's doc comment).
func TestHandleCompanionHP(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Kankuro')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	post := func(value string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/hp",
			strings.NewReader(url.Values{"value": {value}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleCompanionHP(w, req)
		return w.Code, w.Body.String()
	}

	if code, body := post("20"); code != http.StatusOK || body != "20" {
		t.Fatalf("set 20: status %d, body %q", code, body)
	}
	if code, body := post("-6"); code != http.StatusOK || body != "14" {
		t.Fatalf("-6 from 20: status %d, body %q, want 14", code, body)
	}
	if code, body := post("+3"); code != http.StatusOK || body != "17" {
		t.Fatalf("+3 from 14: status %d, body %q, want 17", code, body)
	}
	// Floored at 0, not negative, mirroring the main sheet's SetHP.
	if code, body := post("-100"); code != http.StatusOK || body != "0" {
		t.Fatalf("-100 from 17: status %d, body %q, want floored 0", code, body)
	}
	if code, body := post(""); code != http.StatusOK || body != "" {
		t.Fatalf("clear: status %d, body %q, want empty", code, body)
	}
	saved, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.HPCurrent.Valid {
		t.Errorf("after clearing, HPCurrent = %+v, want NULL", saved.HPCurrent)
	}
	// A delta against a never-set (NULL) HP treats it as 0, not an error.
	if code, body := post("+5"); code != http.StatusOK || body != "5" {
		t.Fatalf("+5 from cleared/NULL: status %d, body %q, want 5", code, body)
	}
}

// TestCompanionHPSurvivesWhileFormSaveDoesNotIncludeIt is the regression
// case for a real bug caught live: hp_current used to be one of
// SetCompanionFields' columns, which means it was in #companion-form's
// field set too — a blur on ANY other field (Notes, an ability score,
// anything) resubmitted the whole form, and since the whole form no longer
// carries hp_current at all (it moved to its own <form>/endpoint), that
// resubmit sent an empty hp_current and silently wiped whatever HP had just
// been set via handleCompanionHP. Fixed by dropping hp_current from
// SetCompanionFields' columns entirely; this test posts to
// handleCompanionSave (the whole-form save) AFTER setting HP via
// handleCompanionHP and asserts HP survives it.
func TestCompanionHPSurvivesWhileFormSaveDoesNotIncludeIt(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Kankuro')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	hpReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/hp",
		strings.NewReader(url.Values{"value": {"20"}}.Encode()))
	hpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hpReq.SetPathValue("id", "1")
	hpReq.SetPathValue("cid", cid)
	hpW := httptest.NewRecorder()
	s.handleCompanionHP(hpW, hpReq)
	if hpW.Code != http.StatusOK {
		t.Fatalf("set hp: status %d", hpW.Code)
	}

	// Simulate blurring an unrelated field (e.g. Notes), which resubmits
	// the whole #companion-form — hp_current is not one of its fields.
	saveReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid,
		strings.NewReader(url.Values{"name": {"Karasu"}, "notes": {"some note"}}.Encode()))
	saveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	saveReq.SetPathValue("id", "1")
	saveReq.SetPathValue("cid", cid)
	saveW := httptest.NewRecorder()
	s.handleCompanionSave(saveW, saveReq)
	if saveW.Code != http.StatusNoContent {
		t.Fatalf("save companion: status %d, body %s", saveW.Code, saveW.Body.String())
	}

	saved, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.HPCurrent.Valid || saved.HPCurrent.Int64 != 20 {
		t.Errorf("HPCurrent after an unrelated whole-form save = %+v, want still 20", saved.HPCurrent)
	}
	if saved.Notes != "some note" {
		t.Errorf("Notes = %q, want %q (whole-form save itself should still work)", saved.Notes, "some note")
	}
}

// TestCompanionsPersistThroughLevelUp confirms nothing in the level-up path
// (charstore.SetLevel) touches character_companions — a companion is a
// plain row scoped to character_id with no other write path that could
// clear it (see charstore/companions.go's DeleteCompanion, the only
// place any code deletes one), so it should already survive normal sheet
// operations by construction. This test exists to keep it that way.
func TestCompanionsPersistThroughLevelUp(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kankuro', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/puppet-master', 1, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}

	if err := charstore.SetLevel(s.charDB, 1, 10); err != nil {
		t.Fatal(err)
	}

	companions, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(companions) != 1 || companions[0].ID != companionID || companions[0].Name != "Karasu" {
		t.Errorf("companions after level up = %+v, want Karasu still present", companions)
	}
}
