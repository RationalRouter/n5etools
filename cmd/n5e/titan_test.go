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

// seedTitanRules inserts a minimal Science-Nin class, its Mech Crafter
// subclass, and the Ordnance Training feature (3rd level) — enough to
// satisfy loadTitanUpgradesData's own has[scienceNinOrdnanceTrainingFeatureSlug]
// gate without needing the full seeded rules.db, the same "minimal fixture,
// real query shape" approach seedPuppetMasterRules (companions_test.go)
// already uses for Puppet Master. Also seeds one Titan Specialization
// option (class_options, list_name="Titan") so a reference-panel render has
// something real to pick from.
func seedTitanRules(t *testing.T, s *server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 8)`,
		`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
			('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`,
		`INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/science-nin/group/scientific-inquiry/mech-crafter',
			 'class/science-nin/group/scientific-inquiry', 'Mech Crafter')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + scienceNinOrdnanceTrainingFeatureSlug + `',
			 'class/science-nin/group/scientific-inquiry/mech-crafter',
			 'Ordnance Training', 3, 'Craft a Titan.')`,
		`INSERT INTO class_options (slug, class_slug, list_name, name, description) VALUES
			('class/science-nin/group/scientific-inquiry/mech-crafter/option/titan/ronin-specialization',
			 'class/science-nin', 'Titan', 'Ronin Titan', 'A swift, independent Titan.')`,
	}
	for _, stmt := range stmts {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("seed rules: %v\n%s", err, stmt)
		}
	}
}

// seedTitanCharacter inserts a character with the given Science-Nin level,
// already past Ordnance Training's own 3rd-level gate and with the Mech
// Crafter subclass chosen — the common setup TestPrefillTitanStatDefaults
// OnCreation and TestTitanStructuredAttacksReachable both need.
func seedTitanCharacter(t *testing.T, s *server, level int) {
	t.Helper()
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Sasori', 10, 10, 10, 10, 10, 10)`); err != nil {
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
		VALUES (1, 'class/science-nin/group/scientific-inquiry/mech-crafter', 3)`); err != nil {
		t.Fatal(err)
	}
}

// TestPrefillTitanStatDefaultsOnCreation covers the fix that closed Titan's
// version of the same auto-compute-at-creation gap Nin-Dog had: a brand-new
// Titan must reach its first render already carrying AC/HP/Speed/Barrier/
// ability scores computed from Ordnance Training's own baseline (mirroring
// prefillPuppetStatDefaults for kind="puppet" and prefillNinDogStatDefaults
// for kind="nin-dog"), not sitting blank until the player clicks every Sync
// button by hand. AC uses titanAC's own "12 + Intelligence Modifier + half
// Proficiency Bonus" formula — see titan.go's header doc for why this was
// once believed absent from the source entirely (a PDF-extraction gap, not
// a true absence). Exercises the real HTTP handler
// (handleSheetCompanionAdd), not prefillTitanStatDefaults directly, so this
// also pins that the creation handler's own kind=="titan" branch actually
// calls it.
func TestPrefillTitanStatDefaultsOnCreation(t *testing.T) {
	s := testServer(t)
	seedTitanRules(t, s)
	// Level 6: past Ordnance Training's own 3rd-level gate, and non-trivial
	// enough that a wrong level/CON-modifier lookup would actually be
	// caught (unlike level 1, where Barrier Max is 2 either way).
	seedTitanCharacter(t, s, 6)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/companions",
		strings.NewReader(url.Values{"name": {"Iron Titan"}, "kind": {"titan"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetCompanionAdd(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add titan companion: status %d, body %s", w.Code, w.Body.String())
	}

	companions, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(companions) != 1 {
		t.Fatalf("ListCompanions = %d rows, want 1", len(companions))
	}
	c := companions[0]

	// Base stat block's own +1 CON modifier (13 baseline) — the same
	// fallback titanEffectiveAbilityModifier uses for a companion whose own
	// ability score fields are still blank at the moment
	// prefillTitanStatDefaults runs (right after AddCompanion, before this
	// call ever writes them).
	wantHP := int64(titanMaxHP(6, 1))
	wantBarrier := int64(titanBarrierMax(6))
	wantSpeed := int64(titanSpeedForSpecialization(""))

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Base stat block's own -3 Intelligence modifier (5 baseline) — same
	// "companion's own fields still blank, base stat block baseline used"
	// reasoning as wantHP's own +1 CON modifier above.
	wantAC := int64(titanAC(charsheet.AbilityModifier(titanBaseAbilityScores["int"]), sheet.ProficiencyBonus))
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
	if !c.BarrierCurrent.Valid || c.BarrierCurrent.Int64 != wantBarrier {
		t.Errorf("BarrierCurrent = %+v, want %d", c.BarrierCurrent, wantBarrier)
	}
	if !c.BarrierMax.Valid || c.BarrierMax.Int64 != wantBarrier {
		t.Errorf("BarrierMax = %+v, want %d", c.BarrierMax, wantBarrier)
	}
	for key, dest := range map[string]sql.NullInt64{
		"str": c.Str, "dex": c.Dex, "con": c.Con, "int": c.Int, "wis": c.Wis, "cha": c.Cha,
	} {
		want := int64(titanBaseAbilityScores[key])
		if !dest.Valid || dest.Int64 != want {
			t.Errorf("%s = %+v, want %d (base stat block baseline)", key, dest, want)
		}
	}

	// The same COALESCE-based safety prefillNinDogStatDefaults already
	// relies on: re-running the prefill (as a future per-render backfill
	// would) must never clobber a field the player has since edited by
	// hand.
	if err := charstore.SetCompanionIntField(s.charDB, 1, c.ID, "hp_max", sql.NullInt64{Int64: 999, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.prefillTitanStatDefaults(1, c.ID); err != nil {
		t.Fatal(err)
	}
	after, err := charstore.GetCompanion(s.charDB, 1, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.HPMax.Valid || after.HPMax.Int64 != 999 {
		t.Errorf("re-running the prefill overwrote a manually-set HP Max: got %+v, want 999 preserved", after.HPMax)
	}
}

// TestPrefillTitanStatDefaultsNoOrdnanceTrainingLeavesBlank covers the
// no-op path: a character with no Ordnance Training yet (no Science-Nin
// levels at all here) must not have any stat prefilled — loadTitanReference
// returns nil in that case (same gate loadTitanUpgradesData applies), and
// prefillTitanStatDefaults must treat that as "nothing to compute yet"
// rather than erroring the whole companion-add request.
func TestPrefillTitanStatDefaultsNoOrdnanceTrainingLeavesBlank(t *testing.T) {
	s := testServer(t)
	seedTitanRules(t, s)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('No Ordnance Yet', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/companions",
		strings.NewReader(url.Values{"name": {"Blank Titan"}, "kind": {"titan"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetCompanionAdd(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add titan companion: status %d, body %s", w.Code, w.Body.String())
	}

	companions, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(companions) != 1 {
		t.Fatalf("ListCompanions = %d rows, want 1", len(companions))
	}
	c := companions[0]
	if c.HPMax.Valid || c.Speed.Valid || c.BarrierMax.Valid || c.Str.Valid {
		t.Errorf("companion with no Ordnance Training = %+v, want every stat still blank", c)
	}
}

// TestTitanStructuredAttacksReachable covers the fix that closed Titan's
// own version of the typed-free-text-Attacks bug Nin-Dog also had
// (companionStructuredAttackKinds used to whitelist only puppet/nin-dog): a
// titan companion can now add and delete a structured attack through the
// same real HTTP handlers puppet/nin-dog already use, responding with the
// Companions-tab fragment (companionAttacksFragment's own "every non-puppet
// kind lands on sheet_summon_tab" rule).
func TestTitanStructuredAttacksReachable(t *testing.T) {
	s := testServer(t)
	seedTitanRules(t, s)
	seedTitanCharacter(t, s, 6)
	companionID, err := charstore.AddCompanion(s.charDB, 1, "titan", "Iron Titan")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	addReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/attacks",
		strings.NewReader(url.Values{"name": {"Missile Barrage"}, "damage_count": {"3"}, "damage_sides": {"6"}}.Encode()))
	addReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addReq.Header.Set("X-Requested-With", "fetch")
	addReq.SetPathValue("id", "1")
	addReq.SetPathValue("cid", cid)
	addW := httptest.NewRecorder()
	s.handlePuppetAttackAdd(addW, addReq)
	if addW.Code != http.StatusOK {
		t.Fatalf("add titan attack: status %d, body %s", addW.Code, addW.Body.String())
	}
	if !strings.Contains(addW.Body.String(), "Missile Barrage") {
		t.Errorf("add titan attack: response fragment missing the new row, got %s", addW.Body.String())
	}

	attacks, err := charstore.ListCompanionAttacks(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 1 || attacks[0].Name != "Missile Barrage" {
		t.Fatalf("ListCompanionAttacks = %+v, want one Missile Barrage row", attacks)
	}
	aid := strconv.FormatInt(attacks[0].ID, 10)

	delReq := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/attacks/"+aid+"/delete", nil)
	delReq.Header.Set("X-Requested-With", "fetch")
	delReq.SetPathValue("id", "1")
	delReq.SetPathValue("cid", cid)
	delReq.SetPathValue("aid", aid)
	delW := httptest.NewRecorder()
	s.handlePuppetAttackDelete(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("delete titan attack: status %d, body %s", delW.Code, delW.Body.String())
	}
	attacks, err = charstore.ListCompanionAttacks(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 0 {
		t.Errorf("ListCompanionAttacks after delete = %+v, want none", attacks)
	}
}

// TestTitanSpecializationCommitMarkup covers the fix that closed Titan
// Specialization's own version of the preview-vs-commit lock bug Nin-Dog
// Breed had: the select must be a .companion-commit-select (opted out of
// the generic whole-form autosave-on-blur), with its own explicit "Set
// Specialization" .companion-commit-btn — not a plain .companion-field,
// which would silently commit the permanent pick on an ordinary field blur.
// Also covers the locked-after-commit state: once TitanSpecialization is
// set, the select is disabled and the commit button is gone, matching the
// Nin-Dog breed picker's own already-correct locked-state rendering.
func TestTitanSpecializationCommitMarkup(t *testing.T) {
	s := testServer(t)
	seedTitanRules(t, s)
	seedTitanCharacter(t, s, 6)
	companionID, err := charstore.AddCompanion(s.charDB, 1, "titan", "Iron Titan")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	render := func() string {
		req := httptest.NewRequest(http.MethodGet, "/characters/1/companions/"+cid, nil)
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleCompanionSheet(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("open titan companion popup: status %d, body %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	before := render()
	if !strings.Contains(before, `name="titan_specialization" form="companion-form" class="companion-commit-select`) {
		t.Errorf("titan_specialization select is not a .companion-commit-select before a pick is made:\n%s", before)
	}
	if strings.Contains(before, `name="titan_specialization"`) && !strings.Contains(before, "Set Specialization") {
		t.Errorf("no 'Set Specialization' commit button rendered before a pick is made:\n%s", before)
	}
	if strings.Contains(before, `name="titan_specialization" form="companion-form" class="companion-commit-select sheet-option-tooltip-select" data-tooltip-for="titan-spec-tooltip-`+cid+`" disabled`) {
		t.Errorf("titan_specialization select is disabled before any pick has been made:\n%s", before)
	}

	if err := charstore.SetCompanionFields(s.charDB, 1, companionID, "Iron Titan", "",
		sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{}, sql.NullInt64{},
		"", "", "", "", false, "", "",
		"class/science-nin/group/scientific-inquiry/mech-crafter/option/titan/ronin-specialization",
	); err != nil {
		t.Fatal(err)
	}

	after := render()
	if !strings.Contains(after, `name="titan_specialization" form="companion-form" class="companion-commit-select sheet-option-tooltip-select" data-tooltip-for="titan-spec-tooltip-`+cid+`" disabled`) {
		t.Errorf("titan_specialization select is not disabled once a specialization is locked in:\n%s", after)
	}
	if strings.Contains(after, "Set Specialization") {
		t.Errorf("commit button still rendered after the specialization is already locked in:\n%s", after)
	}
	if !strings.Contains(after, "Chosen permanently") {
		t.Errorf("no permanent-lock hint rendered once a specialization is locked in:\n%s", after)
	}
}

// TestTitanMaxHP pins titan_unit_card's own verbatim formula "20+[2*Titan's
// Constitution Modifer x Science Nin level]", plus the floor of 1 for a
// pathological negative-modifier case.
func TestTitanMaxHP(t *testing.T) {
	cases := []struct {
		level, conMod int
		want          int
	}{
		{1, 1, 22},  // 20 + 2*1*1
		{12, 1, 44}, // 20 + 2*1*12
		{1, -3, 14}, // 20 + 2*-3*1 (the base block's own -3 CON baseline)
		{20, -5, 1}, // would go negative without the floor
	}
	for _, c := range cases {
		if got := titanMaxHP(c.level, c.conMod); got != c.want {
			t.Errorf("titanMaxHP(%d, %d) = %d, want %d", c.level, c.conMod, got, c.want)
		}
	}
}

// TestTitanBarrierMax pins Battery Powered Barrier's own "maximum number of
// hit points equal to twice your Science-Nin level".
func TestTitanBarrierMax(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 2}, {3, 6}, {20, 40},
	}
	for _, c := range cases {
		if got := titanBarrierMax(c.level); got != c.want {
			t.Errorf("titanBarrierMax(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// TestTitanSizeForLevel pins Gradual Expansion's own "starts off as Large,
// becoming Huge at 14th level".
func TestTitanSizeForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "Large"}, {13, "Large"}, {14, "Huge"}, {20, "Huge"},
	}
	for _, c := range cases {
		if got := titanSizeForLevel(c.level); got != c.want {
			t.Errorf("titanSizeForLevel(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

// TestTitanSpeedForSpecialization pins the base 30 ft. (Legion/Monarch both
// restate it, and "" means no specialization chosen yet) against Ronin's own
// "35 feet".
func TestTitanSpeedForSpecialization(t *testing.T) {
	cases := []struct {
		slug string
		want int
	}{
		{"", 30},
		{"class/science-nin/group/scientific-inquiry/mech-crafter/option/titan/legion-specialization", 30},
		{"class/science-nin/group/scientific-inquiry/mech-crafter/option/titan/monarch-specialization", 30},
		{"class/science-nin/group/scientific-inquiry/mech-crafter/option/titan/ronin-specialization", 35},
	}
	for _, c := range cases {
		if got := titanSpeedForSpecialization(c.slug); got != c.want {
			t.Errorf("titanSpeedForSpecialization(%q) = %d, want %d", c.slug, got, c.want)
		}
	}
}

// TestTitanExoSuitCap pins Endless Work's own "You can add second upgrade to
// your Exo-Suit at level 14."
func TestTitanExoSuitCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{6, 1}, {13, 1}, {14, 2}, {20, 2},
	}
	for _, c := range cases {
		if got := titanExoSuitCap(c.level); got != c.want {
			t.Errorf("titanExoSuitCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// TestTitanEffectiveAbilityModifier covers the fallback-to-base-stat-block
// behavior a brand-new Titan (every ability score field blank) needs for its
// very first Sync Max HP hint to be sane, and confirms an explicitly-set
// companion score always wins over the baseline.
func TestTitanEffectiveAbilityModifier(t *testing.T) {
	blank := charstore.Companion{}
	if got := titanEffectiveAbilityModifier(blank, "con"); got != 1 {
		t.Errorf("blank companion con modifier = %d, want 1 (from the 13 base baseline)", got)
	}
	if got := titanEffectiveAbilityModifier(blank, "int"); got != -3 {
		t.Errorf("blank companion int modifier = %d, want -3 (from the 5 base baseline)", got)
	}

	set := charstore.Companion{Con: sql.NullInt64{Int64: 20, Valid: true}}
	if got := titanEffectiveAbilityModifier(set, "con"); got != 5 {
		t.Errorf("explicit con=20 modifier = %d, want 5 (explicit score must win over baseline)", got)
	}
}

// TestTitanEffectiveUpgradeCost pins Specialist Crafting's own "Upgrades of
// that keyword cost 2 less creation points (min. 1)" discount — applied only
// to the designated keyword, floored at 1, and a no-op with no keyword
// designated.
func TestTitanEffectiveUpgradeCost(t *testing.T) {
	minor := titanUpgradeOption{Cost: 2, Keyword: "mech"}
	if got := titanEffectiveUpgradeCost(minor, ""); got != 2 {
		t.Errorf("no specialist keyword: cost = %d, want 2 (unchanged)", got)
	}
	if got := titanEffectiveUpgradeCost(minor, "mech"); got != 1 {
		t.Errorf("mech-specialized minor (2 CP): cost = %d, want 1 (floored, not -0)", got)
	}
	if got := titanEffectiveUpgradeCost(minor, "weapon"); got != 2 {
		t.Errorf("weapon-specialized against a mech upgrade: cost = %d, want 2 (unchanged)", got)
	}

	mastercraft := titanUpgradeOption{Cost: 32, Keyword: "mech"}
	if got := titanEffectiveUpgradeCost(mastercraft, "mech"); got != 30 {
		t.Errorf("mech-specialized Bijuu Slayer (32 CP): cost = %d, want 30", got)
	}
}

// TestParseTitanUpgradeStatLine covers the ordinary "Cost: N Creation Points
// Keyword: Mech|Weapon Drain: N CCD Chakra" shape, Sturdy Frame's own
// no-Drain variant, and the Mastercraft tier's own "BIJUU SLAYER " name
// prefix before the stat line.
func TestParseTitanUpgradeStatLine(t *testing.T) {
	cost, drain, keyword, rest := parseTitanUpgradeStatLine(
		"Cost: 2 Creation Points Keyword: Weapon Drain: 5 CCD Chakra A large katana-like blade.")
	if cost != 2 || drain != 5 || keyword != "weapon" || rest != "A large katana-like blade." {
		t.Errorf("Ion Sword-shaped line: got cost=%d drain=%d keyword=%q rest=%q", cost, drain, keyword, rest)
	}

	cost, drain, keyword, rest = parseTitanUpgradeStatLine(
		"Cost: 8 Creation Points Keyword: Mech Your Titan is exceptionally durable.")
	if cost != 8 || drain != 0 || keyword != "mech" || rest != "Your Titan is exceptionally durable." {
		t.Errorf("Sturdy Frame-shaped line (no Drain): got cost=%d drain=%d keyword=%q rest=%q", cost, drain, keyword, rest)
	}

	cost, drain, keyword, rest = parseTitanUpgradeStatLine(
		"BIJUU SLAYER Cost: 32 Creation Points Keyword: Mech Drain: 30 CCD Chakra You fortify your Titan's framework.")
	if cost != 32 || drain != 30 || keyword != "mech" || rest != "You fortify your Titan's framework." {
		t.Errorf("Mastercraft-shaped line (name prefix before Cost:): got cost=%d drain=%d keyword=%q rest=%q", cost, drain, keyword, rest)
	}
}

// TestTitanAC pins "12 + Intelligence Modifier + half your Proficiency
// Bonus" — verified directly against the source PDF page image (see
// titan.go's header doc), including the floored-half-proficiency
// convention internal/charsheet.ArmorClass's own "PROF" armor-ability term
// already uses.
func TestTitanAC(t *testing.T) {
	cases := []struct {
		intMod, profBonus, want int
	}{
		{-3, 2, 10}, // base stat block's own -3 INT, PB 2 (floors to 1)
		{0, 3, 13},  // odd proficiency bonus floors down
		{5, 6, 20},
	}
	for _, c := range cases {
		if got := titanAC(c.intMod, c.profBonus); got != c.want {
			t.Errorf("titanAC(%d, %d) = %d, want %d", c.intMod, c.profBonus, got, c.want)
		}
	}
}

// TestTitanSpecializationAbilityBonuses pins Monarch's flat +4 Constitution,
// Ronin's flat +4 Dexterity, and Legion's own two player-chosen abilities at
// +2 each — including the same ability picked for both Legion slots
// correctly stacking to +4 rather than one bonus overwriting the other.
func TestTitanSpecializationAbilityBonuses(t *testing.T) {
	if got := titanSpecializationAbilityBonuses("", "", ""); len(got) != 0 {
		t.Errorf("no specialization: bonuses = %+v, want empty", got)
	}
	if got := titanSpecializationAbilityBonuses("option/titan/monarch-specialization", "", ""); got["con"] != 4 {
		t.Errorf("monarch: bonuses = %+v, want con=4", got)
	}
	if got := titanSpecializationAbilityBonuses("option/titan/ronin-specialization", "", ""); got["dex"] != 4 {
		t.Errorf("ronin: bonuses = %+v, want dex=4", got)
	}

	got := titanSpecializationAbilityBonuses("option/titan/legion-specialization", "str", "wis")
	if got["str"] != 2 || got["wis"] != 2 {
		t.Errorf("legion (distinct picks): bonuses = %+v, want str=2 wis=2", got)
	}
	got = titanSpecializationAbilityBonuses("option/titan/legion-specialization", "con", "con")
	if got["con"] != 4 {
		t.Errorf("legion (same ability picked twice): bonuses = %+v, want con=4 (stacked)", got)
	}
	got = titanSpecializationAbilityBonuses("option/titan/legion-specialization", "str", "")
	if got["str"] != 2 || len(got) != 1 {
		t.Errorf("legion (only slot 1 chosen): bonuses = %+v, want just str=2", got)
	}
}

// TestTitanBashAttack pins Bash's own "1d6 + Str + Dex" formula (both the
// companion's Strength AND Dexterity modifiers feeding both the attack roll
// and the damage, plus the owning character's proficiency bonus on the
// attack roll only — see titanBashAttack's own doc for why, since no
// separate "+X to hit" is stated anywhere in the source), and Ronin
// Specialization's own +1 critical threat range.
func TestTitanBashAttack(t *testing.T) {
	companion := charstore.Companion{
		Str: sql.NullInt64{Int64: 17, Valid: true}, // +3
		Dex: sql.NullInt64{Int64: 15, Valid: true}, // +2
	}
	row := titanBashAttack(companion, 4)
	if row.Name != "Bash" {
		t.Errorf("Name = %q, want Bash", row.Name)
	}
	if row.AttackTotal != 3+2+4 {
		t.Errorf("AttackTotal = %d, want 9 (str+dex+prof)", row.AttackTotal)
	}
	if row.DamageTotal != 3+2 {
		t.Errorf("DamageTotal = %d, want 5 (str+dex, no prof)", row.DamageTotal)
	}
	if row.DamageDice() != "1d6" {
		t.Errorf("DamageDice() = %q, want 1d6", row.DamageDice())
	}
	if row.DamageType != "bludgeoning" {
		t.Errorf("DamageType = %q, want bludgeoning", row.DamageType)
	}
	if row.CritRangeThreshold != 20 {
		t.Errorf("CritRangeThreshold = %d, want 20 (no Ronin)", row.CritRangeThreshold)
	}

	companion.TitanSpecialization = "option/titan/ronin-specialization"
	row = titanBashAttack(companion, 4)
	if row.CritRangeThreshold != 19 {
		t.Errorf("Ronin CritRangeThreshold = %d, want 19", row.CritRangeThreshold)
	}
}
