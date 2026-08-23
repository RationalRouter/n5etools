package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the last 3 of Science-Nin's 9 subclass tracker popups —
// Spyware, Storm Rider, and Technobi — completing the conversion started by
// subclass_tracker_popup_test.go (S.N.B Specialist/Mech Crafter) and
// science_nin_more_subclass_popups_test.go (Elemental Innovationist,
// Grenadier, Mad Scientist, Ninjaneer, Shinobi-Ware). One EmptyHint test
// plus one render/add/delete round trip per popup, mirroring those files'
// own shape; getPopup/postPopupForm and seedNinjaneerlessScienceNinCharacter
// are shared helpers already defined there. None of these three
// subclasses' own catalogs (Spyware Programs, A.T. Enhancements, Technobi
// Mechanizations) had any prior test coverage of their own, so this file
// also seeds their rules.db content from scratch, same shape
// seedGrenadierBIMCatalog/seedElementalInnovationistCatalog already
// establish for the earlier five.

// --- Spyware -----------------------------------------------------------

const spywareTestProgramSlug = "class/science-nin/option/spyware-programs/minor/entry/test-program"

// seedSpywareCatalog seeds enough rules.db content for a Spyware's own
// Spyware Programs/Quick Hack box to render: the class itself, a Creation
// Points chart row, Spyware's own subclass chain, its 3rd-level Cruel
// Angel's Thesis (Spyware Programs) and 14th-level Netrunner (Quick Hack)
// granting features, and a one-entry Spyware Programs catalog.
func seedSpywareCatalog(t *testing.T, s *server, level int) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/science-nin', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', ?, 'Creation Points', '20')`, level)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/spyware', 'class/science-nin/group/scientific-inquiry', 'Spyware')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/spyware', 'Cruel Angel''s Thesis', 3, 'Test.', 1),
		(?, 'class/science-nin/group/scientific-inquiry/spyware', 'Netrunner', 14, 'Test.', 2)`,
		scienceNinCruelAngelsThesisFeatureSlug, scienceNinNetrunnerFeatureSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/spyware-programs/minor', 'class/science-nin', NULL, 'Spyware Programs', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/spyware-programs/minor', 'Test Program', 'A test spyware program.', 1)`, spywareTestProgramSlug)
}

// seedSpywareCharacter inserts a character with the given Science-Nin
// level, already 3rd-level-subclassed into Spyware.
func seedSpywareCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/spyware', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSpywarePopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedSpywareCatalog(t, s, 1) // below Cruel Angel's Thesis' own 3rd-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Kabuto", 1)

	w := getPopup(t, s, spywarePopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Spyware Programs yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestSpywarePopupRenderAddDelete covers the round trip through the real
// registered routes for both Spyware Programs (a flat-cap catalog pick)
// and Quick Hack (a single designation restricted to already-known
// Programs) — the two section shapes this popup introduces.
func TestSpywarePopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedSpywareCatalog(t, s, 14)
	id := seedSpywareCharacter(t, s, "Kabuto", 14)
	popupPath := spywarePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Program") {
		t.Errorf("initial render missing the available Spyware Program option:\n%s", body)
	}
	if strings.Contains(body, "Quick Hack") {
		t.Errorf("Quick Hack section should stay hidden until a known Program exists to designate:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/program/add", url.Values{"option_slug": {spywareTestProgramSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("program add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickSpywareProgram)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != spywareTestProgramSlug {
		t.Fatalf("picks after add = %+v, want one row for %q", picks, spywareTestProgramSlug)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Quick Hack") {
		t.Fatalf("popup missing Quick Hack section once a known Program exists:\n%s", afterAdd)
	}

	hackW := postPopupForm(t, s, popupPath+"/quick-hack/add", url.Values{"option_slug": {spywareTestProgramSlug}})
	if hackW.Code != http.StatusSeeOther {
		t.Fatalf("quick hack add: status %d, body %q", hackW.Code, hackW.Body.String())
	}
	hackPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickQuickHack)
	if err != nil {
		t.Fatal(err)
	}
	if len(hackPicks) != 1 || hackPicks[0].OptionSlug != spywareTestProgramSlug {
		t.Fatalf("quick hack picks after add = %+v, want one row for %q", hackPicks, spywareTestProgramSlug)
	}

	hackDelW := postPopupForm(t, s, popupPath+"/quick-hack/delete", url.Values{"option_slug": {spywareTestProgramSlug}})
	if hackDelW.Code != http.StatusSeeOther {
		t.Fatalf("quick hack delete: status %d, body %q", hackDelW.Code, hackDelW.Body.String())
	}

	delW := postPopupForm(t, s, popupPath+"/program/delete", url.Values{"option_slug": {spywareTestProgramSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("program delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickSpywareProgram)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Storm Rider ---------------------------------------------------------

const stormRiderTestEnhancementSlug = "class/science-nin/option/air-treck-enhancements/minor/entry/test-enhancement"

// seedStormRiderCatalog seeds enough rules.db content for a Storm Rider's
// own A.T. Enhancements/Regalia box to render: the class itself, a
// Creation Points chart row, Storm Rider's own subclass chain, its
// 3rd-level Air Trecks (A.T. Enhancements) and 14th-level King's Road
// (Regalia) granting features, and a one-entry Air Treck Enhancements
// catalog. Regalia's own 8 options are hand-curated in
// scienceNinRegaliaOptions (science_nin_subclasses.go), not database-driven,
// so nothing further needs seeding for that section.
func seedStormRiderCatalog(t *testing.T, s *server, level int) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/science-nin', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', ?, 'Creation Points', '20')`, level)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/storm-rider', 'class/science-nin/group/scientific-inquiry', 'Storm Rider')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/storm-rider', 'Air Trecks', 3, 'Test.', 1),
		(?, 'class/science-nin/group/scientific-inquiry/storm-rider', 'King''s Road', 14, 'Test.', 2)`,
		scienceNinAirTrecksFeatureSlug, scienceNinKingsRoadFeatureSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/air-treck-enhancements/minor', 'class/science-nin', NULL, 'Air Treck Enhancements', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/air-treck-enhancements/minor', 'Test Enhancement', 'A test A.T. enhancement.', 1)`, stormRiderTestEnhancementSlug)
}

func seedStormRiderCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/storm-rider', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStormRiderPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedStormRiderCatalog(t, s, 1) // below Air Trecks' own 3rd-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Karin", 1)

	w := getPopup(t, s, stormRiderPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no A.T. Enhancements yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestStormRiderPopupRenderAddDelete covers the round trip through the
// real registered routes for both A.T. Enhancements (a flat-cap database
// catalog) and Regalia (a flat-cap hand-curated catalog, only shown once
// King's Road is granted) — the two section shapes this popup introduces.
func TestStormRiderPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedStormRiderCatalog(t, s, 14)
	id := seedStormRiderCharacter(t, s, "Karin", 14)
	popupPath := stormRiderPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Enhancement") {
		t.Errorf("initial render missing the available A.T. Enhancement option:\n%s", body)
	}
	if !strings.Contains(body, "Gem Regalia") {
		t.Errorf("initial render missing an available Regalia option once King's Road is granted:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/enhancement/add", url.Values{"option_slug": {stormRiderTestEnhancementSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("enhancement add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickAirTreckEnhancement)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != stormRiderTestEnhancementSlug {
		t.Fatalf("enhancement picks after add = %+v, want one row for %q", picks, stormRiderTestEnhancementSlug)
	}

	regaliaSlug := "science-nin/regalia/gem"
	regaliaW := postPopupForm(t, s, popupPath+"/regalia/add", url.Values{"option_slug": {regaliaSlug}})
	if regaliaW.Code != http.StatusSeeOther {
		t.Fatalf("regalia add: status %d, body %q", regaliaW.Code, regaliaW.Body.String())
	}
	regaliaPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickRegalia)
	if err != nil {
		t.Fatal(err)
	}
	if len(regaliaPicks) != 1 || regaliaPicks[0].OptionSlug != regaliaSlug {
		t.Fatalf("regalia picks after add = %+v, want one row for %q", regaliaPicks, regaliaSlug)
	}

	regaliaDelW := postPopupForm(t, s, popupPath+"/regalia/delete", url.Values{"option_slug": {regaliaSlug}})
	if regaliaDelW.Code != http.StatusSeeOther {
		t.Fatalf("regalia delete: status %d, body %q", regaliaDelW.Code, regaliaDelW.Body.String())
	}

	delW := postPopupForm(t, s, popupPath+"/enhancement/delete", url.Values{"option_slug": {stormRiderTestEnhancementSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("enhancement delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickAirTreckEnhancement)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("enhancement picks after delete = %+v, want none", picks)
	}
}

// --- Technobi ------------------------------------------------------------

const technobiTestMechanizationSlug = "class/science-nin/option/technobi-mechanizations/minor/entry/test-mechanization"

// seedTechnobiCatalog seeds enough rules.db content for a Technobi's own
// Technobi Mechanizations/S.E.N.T box to render: the class itself, a
// Creation Points chart row, Technobi's own subclass chain, its 3rd-level
// The Best Laid Trap (Mechanizations) and S.E.N.Ts (weapon pick) granting
// features (both 3rd level), a one-entry Technobi Mechanizations catalog,
// and one S.E.N.T-eligible weapon row (same shape seedSENTResources,
// sents_test.go, already establishes for S.E.N.Ts alone).
func seedTechnobiCatalog(t *testing.T, s *server, level int) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/science-nin', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', ?, 'Creation Points', '20')`, level)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/technobi', 'class/science-nin/group/scientific-inquiry', 'Technobi')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/technobi', 'The Best Laid Trap', 3, 'Test.', 1),
		(?, 'class/science-nin/group/scientific-inquiry/technobi', 'S.E.N.Ts', 3, 'Test.', 2)`,
		scienceNinBestLaidTrapFeatureSlug, scienceNinSENTsFeatureSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/technobi-mechanizations/minor', 'class/science-nin', NULL, 'Technobi Mechanizations', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/technobi-mechanizations/minor', 'Test Mechanization', 'A test mechanization.', 1)`, technobiTestMechanizationSlug)
	mustExecRules(`INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, properties) VALUES
		('weapon/kunai', 'Kunai', 'weapon', '1d4', 'Piercing', 'Light, Finesse, Thrown (30/60), Multiattack, Ammunition')`)
}

func seedTechnobiCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/technobi', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTechnobiPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedTechnobiCatalog(t, s, 1) // below The Best Laid Trap's own 3rd-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Sasori", 1)

	w := getPopup(t, s, technobiPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Technobi Mechanizations yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestTechnobiPopupRenderAddDelete covers the round trip through the real
// registered routes for both Technobi Mechanizations (a flat-cap catalog
// pick) and S.E.N.T (a single freely re-editable <select>, hand-rolled
// outside subclassTrackerSection) — the two section shapes this popup
// introduces.
func TestTechnobiPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedTechnobiCatalog(t, s, 3)
	id := seedTechnobiCharacter(t, s, "Sasori", 3)
	popupPath := technobiPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Mechanization") {
		t.Errorf("initial render missing the available Technobi Mechanization option:\n%s", body)
	}
	if !strings.Contains(body, "Kunai") {
		t.Errorf("initial render missing the S.E.N.T weapon select once S.E.N.Ts is granted:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/mechanization/add", url.Values{"option_slug": {technobiTestMechanizationSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("mechanization add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTechnobiMechanization)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != technobiTestMechanizationSlug {
		t.Fatalf("mechanization picks after add = %+v, want one row for %q", picks, technobiTestMechanizationSlug)
	}

	sentW := postPopupForm(t, s, popupPath+"/sent", url.Values{"value": {"weapon/kunai"}})
	if sentW.Code != http.StatusSeeOther {
		t.Fatalf("sent set: status %d, body %q", sentW.Code, sentW.Body.String())
	}
	afterSent := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterSent, `value="weapon/kunai" data-description="" selected`) {
		t.Errorf("S.E.N.T <select> should show weapon/kunai pre-selected after setting it:\n%s", afterSent)
	}

	delW := postPopupForm(t, s, popupPath+"/mechanization/delete", url.Values{"option_slug": {technobiTestMechanizationSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("mechanization delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTechnobiMechanization)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("mechanization picks after delete = %+v, want none", picks)
	}
}

// --- Sidebar buttons for all 3 popups ------------------------------------

// TestLastSubclassTrackerSidebarButtons covers character_sheet.html's own
// gating for the Spyware/Storm Rider/Technobi sidebar buttons, and confirms
// the old inline pickers they replaced are gone from the Core sheet — the
// same two checks TestNewSubclassTrackerSidebarButtons already makes for
// the previous five, completing coverage for all nine Science-Nin
// subclasses.
func TestLastSubclassTrackerSidebarButtons(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, s *server) int64
		popupPath   func(int64) string
		buttonText  string
		goneAddText string // the popup's own catalog-pick AddLabel — must not leak into the Core sheet, since the inline picker it replaced is gone
	}{
		{"Spyware", func(t *testing.T, s *server) int64 {
			seedSpywareCatalog(t, s, 3)
			return seedSpywareCharacter(t, s, "Kabuto", 3)
		}, spywarePopupPath, `tracked in the "Spyware" popup`, "Install Program"},
		{"Storm Rider", func(t *testing.T, s *server) int64 {
			seedStormRiderCatalog(t, s, 3)
			return seedStormRiderCharacter(t, s, "Karin", 3)
		}, stormRiderPopupPath, `tracked in the "Storm Rider" popup`, "Install Enhancement"},
		{"Technobi", func(t *testing.T, s *server) int64 {
			seedTechnobiCatalog(t, s, 3)
			return seedTechnobiCharacter(t, s, "Sasori", 3)
		}, technobiPopupPath, `tracked in the "Technobi" popup`, "Install Mechanization"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			id := c.seed(t, s)
			idStr := strconv.FormatInt(id, 10)
			sheetBody := getPopup(t, s, "/characters/"+idStr).Body.String()
			if !strings.Contains(sheetBody, `href="`+c.popupPath(id)+`"`) {
				t.Errorf("Core sheet missing its own sidebar button (href=%q):\n%s", c.popupPath(id), sheetBody)
			}
			if !strings.Contains(sheetBody, c.buttonText) {
				t.Errorf("Core sheet missing pointer text %q:\n%s", c.buttonText, sheetBody)
			}
			if strings.Contains(sheetBody, c.goneAddText) {
				t.Errorf("Core sheet still contains inline picker text %q; it should only point at the popup now", c.goneAddText)
			}
		})
	}
}
