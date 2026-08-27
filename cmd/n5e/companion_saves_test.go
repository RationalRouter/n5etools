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

// TestCompanionSaves covers companionSaves' own modifier math for every
// combination this app actually renders: an ordinary companion (nin-dog)
// with both a positive and a negative ability score, proficient and not,
// and S.N.B Specialist's own "negative modifiers are treated as +0" floor,
// which applies to the ABILITY MODIFIER before proficiency is added, not
// the final total — a proficient S.N.B with a below-10 score still gets the
// full proficiency bonus rather than proficiency being "wasted" topping up
// an already-floored 0.
func TestCompanionSaves(t *testing.T) {
	c := charstore.Companion{
		Kind:              "nin-dog",
		Str:               sql.NullInt64{Int64: 16, Valid: true}, // +3
		Dex:               sql.NullInt64{Int64: 8, Valid: true},  // -1
		SaveProficiencies: "str",
		// Con/Int/Wis/Cha left NULL — must read as modifier 0, not error or
		// worst-case.
	}
	view := companionSaves(c, &charsheet.Sheet{ProficiencyBonus: 4, Level: 5})
	byAbility := map[string]companionSaveRow{}
	for _, row := range view.Rows {
		byAbility[row.Ability] = row
	}

	if got := byAbility["str"]; !got.Proficient || got.Modifier != 3+4 {
		t.Errorf("str = %+v, want proficient, modifier 7 (3 ability + 4 prof)", got)
	}
	if got := byAbility["dex"]; got.Proficient || got.Modifier != -1 {
		t.Errorf("dex = %+v, want not proficient, modifier -1 (no floor for a non-S.N.B kind)", got)
	}
	if got := byAbility["con"]; got.Proficient || got.Modifier != 0 {
		t.Errorf("con (unset score) = %+v, want not proficient, modifier 0", got)
	}
	if view.Used != 1 {
		t.Errorf("Used = %d, want 1 (only str is proficient)", view.Used)
	}
	if view.Cap != 0 {
		t.Errorf("Cap = %d, want 0 (no cap for nin-dog)", view.Cap)
	}
	if len(view.Rows) != 6 {
		t.Fatalf("Rows = %d entries, want 6 (one per ability, canonical order)", len(view.Rows))
	}

	// S.N.B Specialist's own floor: dex (-1 modifier) reads as 0 whether or
	// not it's proficient — proficient still adds the FULL proficiency
	// bonus on top of the floored 0, not a reduced amount.
	snb := charstore.Companion{
		Kind:              "snb",
		Dex:               sql.NullInt64{Int64: 8, Valid: true}, // -1, floored to 0
		Str:               sql.NullInt64{Int64: 8, Valid: true}, // -1, floored to 0
		SaveProficiencies: "dex",
	}
	snbView := companionSaves(snb, &charsheet.Sheet{ProficiencyBonus: 3, Level: 6})
	snbByAbility := map[string]companionSaveRow{}
	for _, row := range snbView.Rows {
		snbByAbility[row.Ability] = row
	}
	if got := snbByAbility["dex"]; !got.Proficient || got.Modifier != 3 {
		t.Errorf("S.N.B proficient dex = %+v, want modifier 3 (floored 0 + prof bonus 3)", got)
	}
	if got := snbByAbility["str"]; got.Proficient || got.Modifier != 0 {
		t.Errorf("S.N.B non-proficient str = %+v, want modifier 0 (floored, not proficient)", got)
	}
	if snbView.Cap != 2 {
		t.Errorf("S.N.B Cap at level 6 = %d, want 2", snbView.Cap)
	}
}

// TestCompanionSaveCap pins S.N.B Specialist's own "Proficient in 2 of your
// choice (3 at 14th level)" cap, and that every other kind has no cap at
// all (0 — meaning "not enforced", see companionSaveCap's own doc).
func TestCompanionSaveCap(t *testing.T) {
	cases := []struct {
		kind  string
		level int
		want  int
	}{
		{"snb", 1, 2},
		{"snb", 13, 2},
		{"snb", 14, 3},
		{"snb", 20, 3},
		{"nin-dog", 20, 0},
		{"titan", 20, 0},
		{"summon", 1, 0},
		{"custom", 1, 0},
		{"puppet", 1, 0},
	}
	for _, c := range cases {
		if got := companionSaveCap(c.kind, c.level); got != c.want {
			t.Errorf("companionSaveCap(%q, %d) = %d, want %d", c.kind, c.level, got, c.want)
		}
	}
}

// TestParseJoinSaveProficiencies covers the stored comma-separated column's
// own round trip: canonical ability order regardless of insertion order
// (so the stored value stays stable/diffable across saves), and a
// defensively-dropped unknown token (a hand-edited or stale row).
func TestParseJoinSaveProficiencies(t *testing.T) {
	set := parseSaveProficiencies("dex, str , bogus,con")
	if len(set) != 3 || !set["str"] || !set["dex"] || !set["con"] {
		t.Errorf("parseSaveProficiencies = %+v, want exactly str/dex/con (bogus dropped)", set)
	}
	if got := joinSaveProficiencies(set); got != "str,dex,con" {
		t.Errorf("joinSaveProficiencies = %q, want canonical ability order \"str,dex,con\"", got)
	}
	if got := parseSaveProficiencies(""); len(got) != 0 {
		t.Errorf("parseSaveProficiencies(\"\") = %+v, want empty set", got)
	}
	if got := joinSaveProficiencies(map[string]bool{}); got != "" {
		t.Errorf("joinSaveProficiencies(empty) = %q, want empty string", got)
	}
}

// TestHandleCompanionSavingThrowToggleOrdinaryKind covers the no-cap path
// (every kind but snb): turning a save on/off round-trips through the
// database with no budget check at all, so no character class/level setup
// is needed for this one.
func TestHandleCompanionSavingThrowToggleOrdinaryKind(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kiba', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "nin-dog", "Akamaru")
	if err != nil {
		t.Fatal(err)
	}

	toggle := func(ability string, on bool) *httptest.ResponseRecorder {
		t.Helper()
		onValue := "0"
		if on {
			onValue = "1"
		}
		cid := strconv.FormatInt(companionID, 10)
		req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/saving-throw",
			strings.NewReader(url.Values{"ability": {ability}, "on": {onValue}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleCompanionSavingThrowToggle(w, req)
		return w
	}

	if w := toggle("str", true); w.Code != http.StatusOK {
		t.Fatalf("toggle str on: status %d, body %s", w.Code, w.Body.String())
	}
	c, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveProficiencies != "str" {
		t.Errorf("SaveProficiencies after toggling str on = %q, want \"str\"", c.SaveProficiencies)
	}

	if w := toggle("con", true); w.Code != http.StatusOK {
		t.Fatalf("toggle con on: status %d, body %s", w.Code, w.Body.String())
	}
	c, err = charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveProficiencies != "str,con" {
		t.Errorf("SaveProficiencies after toggling con on = %q, want \"str,con\" (canonical order, no cap for nin-dog)", c.SaveProficiencies)
	}

	if w := toggle("str", false); w.Code != http.StatusOK {
		t.Fatalf("toggle str off: status %d, body %s", w.Code, w.Body.String())
	}
	c, err = charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveProficiencies != "con" {
		t.Errorf("SaveProficiencies after toggling str back off = %q, want \"con\"", c.SaveProficiencies)
	}

	// A bad ability value never reaches the database.
	if w := toggle("wisdom", true); w.Code != http.StatusBadRequest {
		t.Errorf("toggle bad ability: status %d, want 400", w.Code)
	}

	// A companion id that doesn't exist (or belongs to another character)
	// 404s rather than silently doing nothing.
	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/9999/saving-throw",
		strings.NewReader(url.Values{"ability": {"str"}, "on": {"1"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", "9999")
	w := httptest.NewRecorder()
	s.handleCompanionSavingThrowToggle(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("toggle on a nonexistent companion: status %d, want 404", w.Code)
	}
}

// TestHandleCompanionSavingThrowToggleSNBCap covers S.N.B Specialist's own
// budget enforcement — this is the one kind handleCompanionSavingThrowToggle
// actually gates, and the cap itself changes at 14th level, so this needs a
// real class/level on the character (companionSaveCap's cap check calls
// charsheet.Compute).
func TestHandleCompanionSavingThrowToggleSNBCap(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kabuto', 10, 10, 10, 16, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/science-nin', 6, 0)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "snb", "S.N.B")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	toggleOn := func(ability string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/saving-throw",
			strings.NewReader(url.Values{"ability": {ability}, "on": {"1"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", cid)
		w := httptest.NewRecorder()
		s.handleCompanionSavingThrowToggle(w, req)
		return w
	}

	// Level 6: cap is 2. The first two picks succeed...
	if w := toggleOn("str"); w.Code != http.StatusOK {
		t.Fatalf("toggle str on (1st pick): status %d, body %s", w.Code, w.Body.String())
	}
	if w := toggleOn("dex"); w.Code != http.StatusOK {
		t.Fatalf("toggle dex on (2nd pick): status %d, body %s", w.Code, w.Body.String())
	}
	// ...and a third is rejected, not silently allowed past the cap.
	w := toggleOn("con")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("toggle con on (3rd pick, over cap): status %d, want 400, body %s", w.Code, w.Body.String())
	}
	c, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveProficiencies != "str,dex" {
		t.Errorf("SaveProficiencies after the rejected 3rd pick = %q, want unchanged \"str,dex\"", c.SaveProficiencies)
	}

	// Bump to 14th level: the cap widens to 3, and the same pick that was
	// just rejected now succeeds.
	if _, err := s.charDB.Exec(`UPDATE character_classes SET levels = 14 WHERE character_id = 1`); err != nil {
		t.Fatal(err)
	}
	if w := toggleOn("con"); w.Code != http.StatusOK {
		t.Fatalf("toggle con on at 14th level (cap now 3): status %d, body %s", w.Code, w.Body.String())
	}
	c, err = charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveProficiencies != "str,dex,con" {
		t.Errorf("SaveProficiencies at 14th level = %q, want \"str,dex,con\"", c.SaveProficiencies)
	}
}

// TestCompanionSavesVoidSoulMirrorsPlayerProficiencies covers companionSaves'
// third shape (companionSaveCap's own doc): a void-soul companion's stored
// save_proficiencies column is ignored outright, even when it holds a
// deliberately stale/wrong value, in favor of sheet.Saves — "Uses your
// saving throw proficiencies" is not a player choice for this kind, unlike
// Nin-Dog/Titan's own fixed-but-trusted columns.
func TestCompanionSavesVoidSoulMirrorsPlayerProficiencies(t *testing.T) {
	c := charstore.Companion{
		Kind:              "void-soul",
		SaveProficiencies: "dex", // deliberately stale/wrong — must be ignored
		Str:               sql.NullInt64{Int64: 16, Valid: true}, // +3
		Dex:               sql.NullInt64{Int64: 16, Valid: true}, // +3
		Con:               sql.NullInt64{Int64: 16, Valid: true}, // +3
	}
	sheet := &charsheet.Sheet{
		ProficiencyBonus: 4,
		Level:            5,
		Saves: []charsheet.SaveEntry{
			{Ability: "str", Proficient: true},
			{Ability: "dex", Proficient: false},
			{Ability: "con", Proficient: true},
		},
	}
	view := companionSaves(c, sheet)
	byAbility := map[string]companionSaveRow{}
	for _, row := range view.Rows {
		byAbility[row.Ability] = row
	}

	if got := byAbility["str"]; !got.Proficient || got.Modifier != 3+4 {
		t.Errorf("str = %+v, want proficient (mirrors sheet.Saves), modifier 7", got)
	}
	if got := byAbility["dex"]; got.Proficient || got.Modifier != 3 {
		t.Errorf("dex = %+v, want NOT proficient (sheet.Saves wins over the stale stored \"dex\" column), modifier 3", got)
	}
	if got := byAbility["con"]; !got.Proficient || got.Modifier != 3+4 {
		t.Errorf("con = %+v, want proficient (mirrors sheet.Saves), modifier 7", got)
	}
	if view.Cap != 0 {
		t.Errorf("Cap = %d, want 0 (no cap for void-soul)", view.Cap)
	}
}

// TestHandleCompanionSavingThrowToggleVoidSoulRejected covers the server-side
// half of "a disabled control is a convenience, not the real enforcement"
// for void-soul companions: a direct POST to the saving-throw toggle
// endpoint must be rejected (companion_fields.html's own template never
// renders the toggle form for this kind in the first place, but that's a UI
// nicety, not the actual guard).
func TestHandleCompanionSavingThrowToggleVoidSoulRejected(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Void', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "void-soul", "Void Soul")
	if err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetCompanionSaveProficiencies(s.charDB, 1, companionID, "dex"); err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/saving-throw",
		strings.NewReader(url.Values{"ability": {"str"}, "on": {"1"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handleCompanionSavingThrowToggle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("toggle a void-soul companion's saving throw: status %d, want 400, body %s", w.Code, w.Body.String())
	}

	c, err := charstore.GetCompanion(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveProficiencies != "dex" {
		t.Errorf("SaveProficiencies after the rejected toggle = %q, want unchanged \"dex\"", c.SaveProficiencies)
	}
}

// TestParseCollapsedCompanionIDs covers the "summons:collapsed" UI-state
// blob's own decode: a valid JSON array, malformed/absent input reading as
// "nothing collapsed" rather than erroring the whole tab render.
func TestParseCollapsedCompanionIDs(t *testing.T) {
	if got := parseCollapsedCompanionIDs(""); got != nil {
		t.Errorf("parseCollapsedCompanionIDs(\"\") = %+v, want nil", got)
	}
	if got := parseCollapsedCompanionIDs("not json"); got != nil {
		t.Errorf("parseCollapsedCompanionIDs(garbage) = %+v, want nil, not an error", got)
	}
	got := parseCollapsedCompanionIDs("[3,5]")
	if len(got) != 2 || !got[3] || !got[5] {
		t.Errorf("parseCollapsedCompanionIDs(\"[3,5]\") = %+v, want {3:true, 5:true}", got)
	}
}

// TestSheetSummonTabCollapseRoundTrips exercises the full server-side half
// of the minimize feature end to end: saving "summons:collapsed" via the
// same generic UI-state endpoint sheet-companion-collapse.js POSTs to, then
// confirming a fresh render of sheet_summon_tab honors it (the collapsed
// companion's own <details> loses its "open" attribute, every other
// companion's own keeps it) — client-side persistence (the JS file itself)
// isn't unit-testable here, but everything server-side it depends on is.
func TestSheetSummonTabCollapseRoundTrips(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kiba', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	keptOpenID, err := charstore.AddCompanion(s.charDB, 1, "nin-dog", "Akamaru")
	if err != nil {
		t.Fatal(err)
	}
	collapsedID, err := charstore.AddCompanion(s.charDB, 1, "summon", "Gamakichi")
	if err != nil {
		t.Fatal(err)
	}

	body := url.Values{"key": {"summons:collapsed"}, "data": {"[" + strconv.FormatInt(collapsedID, 10) + "]"}}
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/ui-state", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetUIStateSave(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("save collapsed state: status %d, body %s", w.Code, w.Body.String())
	}

	renderW := httptest.NewRecorder()
	s.renderSheetFragment(renderW, 1, "sheet_summon_tab")
	if renderW.Code != http.StatusOK {
		t.Fatalf("render sheet_summon_tab: status %d, body %s", renderW.Code, renderW.Body.String())
	}
	html := renderW.Body.String()

	keptOpenTag := `data-companion-id="` + strconv.FormatInt(keptOpenID, 10) + `" open>`
	if !strings.Contains(html, keptOpenTag) {
		t.Errorf("expected the never-collapsed companion's <details> to still carry open, got:\n%s", html)
	}
	collapsedTag := `data-companion-id="` + strconv.FormatInt(collapsedID, 10) + `">`
	if !strings.Contains(html, collapsedTag) {
		t.Errorf("expected the collapsed companion's <details> to have no open attribute, got:\n%s", html)
	}

	// Regression check for the <details>/</div> mismatch this feature was
	// built on top of: every <details> on this fragment (each companion's
	// own companion-card wrapper, plus each one's nested "Add an attack"
	// disclosure) must be closed by a real </details>, not a stray </div> a
	// browser's HTML parser would silently drop (leaving every companion
	// after the first nested inside the previous one's <details>, breaking
	// collapse for all but the outermost card). Counts every <details>, not
	// just companion-card's own, since the nested "Add an attack" ones
	// would otherwise mask an unbalanced companion-card count in the total.
	if opens, closes := strings.Count(html, "<details"), strings.Count(html, "</details>"); opens != closes || opens == 0 {
		t.Errorf("<details>/</details> count mismatch: %d open, %d close (want equal, and at least 1)", opens, closes)
	}
}

// TestSheetSummonTabRendersSavingThrowsToggles covers the actual UI wiring
// this file's other tests only exercise from the server side: the
// Companions tab's own card renders a real toggle form (not a read-only
// dot) for a non-puppet companion, and renders no Saving Throws box at all
// for a puppet-kind companion (companion_fields.html's own guard — Puppet
// Tools show a fixed class-text readout elsewhere on the same card
// instead).
func TestSheetSummonTabRendersSavingThrowsToggles(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kiba', 16, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	ninDogID, err := charstore.AddCompanion(s.charDB, 1, "nin-dog", "Akamaru")
	if err != nil {
		t.Fatal(err)
	}
	// A puppet-kind companion can show up in the Companions ("Summons") tab
	// list too (loadSummonsTabData's own no-kind-filter doc) — it must not
	// get a Saving Throws box there, matching the Puppets tab's own
	// class-text-only treatment.
	if _, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu"); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.renderSheetFragment(w, 1, "sheet_summon_tab")
	if w.Code != http.StatusOK {
		t.Fatalf("render sheet_summon_tab: status %d, body %s", w.Code, w.Body.String())
	}
	html := w.Body.String()

	cid := strconv.FormatInt(ninDogID, 10)
	if !strings.Contains(html, `action="/characters/1/companions/`+cid+`/saving-throw"`) {
		t.Errorf("expected a saving-throw toggle form for the nin-dog companion, got:\n%s", html)
	}
	if !strings.Contains(html, "Saving Throws") {
		t.Errorf("expected a Saving Throws heading somewhere on the tab, got:\n%s", html)
	}

	// The nin-dog's own card has 6 toggle forms (one per ability); the
	// puppet's own card has none — so exactly 6 saving-throw toggle forms
	// should exist on the whole tab, not 12.
	if got := strings.Count(html, "/saving-throw\""); got != 6 {
		t.Errorf("saving-throw toggle form count = %d, want exactly 6 (nin-dog only, none for the puppet)", got)
	}
}

// TestCompanionSheetPopupShowsReadOnlySaves covers the popup's own
// read-only half: Saves is computed and shown (a dot, not a toggle
// button), since the popup's bare layout never loads sheet-toggles.js — a
// toggle form rendered there would silently fall through to a real,
// unhandled page navigation on click.
func TestCompanionSheetPopupShowsReadOnlySaves(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kiba', 16, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "nin-dog", "Akamaru")
	if err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetCompanionSaveProficiencies(s.charDB, 1, companionID, "str"); err != nil {
		t.Fatal(err)
	}

	cid := strconv.FormatInt(companionID, 10)
	req := httptest.NewRequest(http.MethodGet, "/characters/1/companions/"+cid, nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handleCompanionSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("open companion popup: status %d, body %s", w.Code, w.Body.String())
	}
	html := w.Body.String()

	if !strings.Contains(html, "Saving Throws") {
		t.Errorf("expected a Saving Throws box in the popup, got:\n%s", html)
	}
	if strings.Contains(html, "/saving-throw\"") {
		t.Errorf("popup must not render an editable saving-throw toggle form (sheet-toggles.js isn't loaded there), got:\n%s", html)
	}
}
