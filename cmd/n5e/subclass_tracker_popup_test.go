package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// getPopup issues a GET against a real subclass-tracker-popup route through
// s.routes() — the same full-mux dispatch postSheetForm (science_nin_
// creation_points_test.go) already uses for POSTs, so path-parameter
// extraction ({id}) is exercised through the real registered pattern
// (server.go) rather than a hand-set httptest.Request.SetPathValue.
func getPopup(t *testing.T, s *server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

// postPopupForm issues a POST against a real subclass-tracker-popup mutation
// route — deliberately WITHOUT the "X-Requested-With: fetch" header
// postSheetForm sets for the Core sheet's own AJAX routes, since this
// pattern's own routes (subclass_tracker_popup.go's header doc) are plain
// POST-and-redirect, not fetch-and-swap, and never branch on that header.
func postPopupForm(t *testing.T, s *server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

// seedSNBFutureOfShinobiFeature adds the 20th-level "Future of Shinobi:
// S.N.B" subclass feature row seedScienceNinCPRules doesn't itself seed
// (that helper only covers the 3rd-level S.N.B Upgrades grant) — needed to
// exercise the popup's own Future of Shinobi: S.N.B section
// (snbUpgradesPermanentSection, snb_upgrades_popup.go).
func seedSNBFutureOfShinobiFeature(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/s-n-b-specialist', 'The Future of Shinobi: S.N.B', 20,
		 'Three known S.N.B Upgrades become permanent — they no longer take an upgrade slot or cost Creation Points.', 2)`,
		scienceNinFutureOfShinobiSNBFeatureSlug); err != nil {
		t.Fatalf("seed Future of Shinobi: S.N.B feature: %v", err)
	}
}

// seedTitanEndlessWorkFeature adds the 6th-level "Endless Work" subclass
// feature row seedScienceNinCPRules doesn't itself seed — needed to
// exercise the popup's own Exo-Suit section (titanExoSuitSection,
// titan_slots_popup.go).
func seedTitanEndlessWorkFeature(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/mech-crafter', 'Endless Work', 6,
		 'A second, Mech-keyword-only Cost 8 or lower slot, usable outside the Titan.', 2)`,
		titanEndlessWorkFeatureSlug); err != nil {
		t.Fatalf("seed Endless Work feature: %v", err)
	}
}

// seedTitanSpecialistCraftingFeature adds the 14th-level "Specialist
// Crafting" subclass feature row — needed to exercise the popup's own
// Specialist Crafting keyword picker (character_titan_slots.html).
func seedTitanSpecialistCraftingFeature(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/mech-crafter', 'Specialist Crafting', 14,
		 'Designate Mech or Weapon; upgrades of that keyword cost 2 less Creation Points (min. 1).', 3)`,
		titanSpecialistCraftingFeatureSlug); err != nil {
		t.Fatalf("seed Specialist Crafting feature: %v", err)
	}
}

// TestSNBUpgradesPopupEmptyHint covers a character with no S.N.B Specialist
// levels — reachable only by visiting the popup URL directly, since the
// sidebar button that links here is itself gated on the same condition
// (character_sheet.html's own {{if and .ScienceNin .ScienceNin.SNBSpecialist}}).
func TestSNBUpgradesPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 20)
	id := seedScienceNinCPCharacter(t, s, "Kabuto", 6, "") // no subclass chosen

	w := getPopup(t, s, "/characters/"+strconv.FormatInt(id, 10)+"/snb-upgrades")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "no S.N.B Upgrades yet") {
		t.Errorf("body missing empty hint:\n%s", body)
	}
	if strings.Contains(body, "Creation Points") {
		t.Errorf("empty-hint page should not show the Creation Points tile:\n%s", body)
	}
}

// TestSNBUpgradesPopupRenderAddDelete exercises the full round trip through
// the real registered routes: an initial render showing the available
// option and an empty Creation Points tile, installing it (moving it into
// Known and spending its Cost from the shared budget), then removing it
// again — covering both subclassTrackerSection's Known/Available rendering
// and subclassTrackerPopupDelete's generic delete route.
func TestSNBUpgradesPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 20)
	id := seedScienceNinCPCharacter(t, s, "Kabuto", 6, "class/science-nin/group/scientific-inquiry/s-n-b-specialist")
	idStr := strconv.FormatInt(id, 10)
	popupPath := "/characters/" + idStr + "/snb-upgrades"

	w := getPopup(t, s, popupPath)
	if w.Code != http.StatusOK {
		t.Fatalf("initial render: status %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper this project's blue subclass-specific convention needs:\n%s", body)
	}
	if !strings.Contains(body, "CP Test Beast Plating") {
		t.Errorf("initial render missing the available S.N.B Upgrade option:\n%s", body)
	}
	if !strings.Contains(body, "0/20") {
		t.Errorf("initial Creation Points tile should read 0/20:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"option_slug": {cpTestSNBUpgradeSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q, want %d redirect", addW.Code, addW.Body.String(), http.StatusSeeOther)
	}
	if loc := addW.Header().Get("Location"); loc != popupPath {
		t.Errorf("add redirect Location = %q, want %q", loc, popupPath)
	}

	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickSNBUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != cpTestSNBUpgradeSlug {
		t.Fatalf("picks after add = %+v, want one row for %q", picks, cpTestSNBUpgradeSlug)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "6/20") {
		t.Errorf("Creation Points tile after a 6-cost install should read 6/20:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"option_slug": {cpTestSNBUpgradeSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickSNBUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// TestSNBUpgradesPopupDeleteMissingSlug covers subclassTrackerPopupDelete's
// own generic validation (subclass_tracker_popup.go) — shared by every
// category this pattern's delete routes use, so one test here covers every
// current and future caller's own missing-slug case.
func TestSNBUpgradesPopupDeleteMissingSlug(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 20)
	id := seedScienceNinCPCharacter(t, s, "Kabuto", 6, "class/science-nin/group/scientific-inquiry/s-n-b-specialist")
	w := postPopupForm(t, s, "/characters/"+strconv.FormatInt(id, 10)+"/snb-upgrades/delete", url.Values{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete with no option_slug: status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestSNBUpgradesPopupNotFound covers a nonexistent character id — the
// popup must 404, matching every other character-scoped route rather than
// rendering an empty/zero-value page.
func TestSNBUpgradesPopupNotFound(t *testing.T) {
	s := testServer(t)
	w := getPopup(t, s, "/characters/999/snb-upgrades")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestSNBUpgradesPopupPermanentSection covers Future of Shinobi: S.N.B —
// only already-Known S.N.B Upgrades become candidates for AvailablePermanent
// (scienceNinSNBSpecialistData's own doc, science_nin_subclasses.go), so
// this installs one ordinary upgrade first, confirms the section only then
// appears, and makes it permanent through the popup's own dedicated route.
func TestSNBUpgradesPopupPermanentSection(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 20, 40)
	seedSNBFutureOfShinobiFeature(t, s)
	id := seedScienceNinCPCharacter(t, s, "Kabuto", 20, "class/science-nin/group/scientific-inquiry/s-n-b-specialist")
	idStr := strconv.FormatInt(id, 10)
	popupPath := "/characters/" + idStr + "/snb-upgrades"

	beforeAdd := getPopup(t, s, popupPath).Body.String()
	if strings.Contains(beforeAdd, "Future of Shinobi: S.N.B") {
		t.Errorf("Future of Shinobi: S.N.B section should stay hidden until a known upgrade exists to make permanent:\n%s", beforeAdd)
	}

	if w := postPopupForm(t, s, popupPath+"/add", url.Values{"option_slug": {cpTestSNBUpgradeSlug}}); w.Code != http.StatusSeeOther {
		t.Fatalf("install ordinary upgrade first: status %d, body %q", w.Code, w.Body.String())
	}

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Future of Shinobi: S.N.B") {
		t.Fatalf("popup missing Future of Shinobi: S.N.B section once a known upgrade exists:\n%s", body)
	}

	permW := postPopupForm(t, s, popupPath+"/permanent/add", url.Values{"option_slug": {cpTestSNBUpgradeSlug}})
	if permW.Code != http.StatusSeeOther {
		t.Fatalf("make permanent: status %d, body %q", permW.Code, permW.Body.String())
	}
	perm, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickSNBUpgradePermanent)
	if err != nil {
		t.Fatal(err)
	}
	if len(perm) != 1 || perm[0].OptionSlug != cpTestSNBUpgradeSlug {
		t.Fatalf("permanent picks = %+v, want one row for %q", perm, cpTestSNBUpgradeSlug)
	}
}

// TestTitanSlotsPopupEmptyHint covers a character with no Ordnance Training
// yet (below 3rd-level Mech Crafter) — reachable only by visiting the popup
// URL directly, since ShowTitanSlotsPopup gates the sidebar button on the
// same granted-feature check.
func TestTitanSlotsPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 1, 20)
	id := seedScienceNinCPCharacter(t, s, "Kankuro", 1, "") // below Ordnance Training's own 3rd-level gate

	w := getPopup(t, s, "/characters/"+strconv.FormatInt(id, 10)+"/titan-slots")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "no Titan Slots yet") {
		t.Errorf("body missing empty hint:\n%s", body)
	}
}

// TestTitanSlotsPopupRenderAddDelete covers the main Titan Slots section's
// own round trip, mirroring TestSNBUpgradesPopupRenderAddDelete's shape for
// the second (Mech Crafter) target of this pattern.
func TestTitanSlotsPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 20)
	id := seedScienceNinCPCharacter(t, s, "Kankuro", 6, "class/science-nin/group/scientific-inquiry/mech-crafter")
	idStr := strconv.FormatInt(id, 10)
	popupPath := "/characters/" + idStr + "/titan-slots"

	w := getPopup(t, s, popupPath)
	if w.Code != http.StatusOK {
		t.Fatalf("initial render: status %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "CP Test Titan Plating") {
		t.Errorf("initial render missing the available Titan Upgrade option:\n%s", body)
	}
	if !strings.Contains(body, "0/20") {
		t.Errorf("initial Creation Points tile should read 0/20:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"option_slug": {cpTestTitanUpgradeSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTitanUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != cpTestTitanUpgradeSlug {
		t.Fatalf("picks after add = %+v, want one row for %q", picks, cpTestTitanUpgradeSlug)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "6/20") {
		t.Errorf("Creation Points tile after a 6-cost install should read 6/20:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"option_slug": {cpTestTitanUpgradeSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTitanUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// TestTitanSlotsPopupExoSuit covers Endless Work's own separate Exo-Suit
// section (titanExoSuitSection, titan_slots_popup.go) — a distinct pick
// category from the ordinary Titan Slots section above, still spending from
// the same shared Creation Points budget.
func TestTitanSlotsPopupExoSuit(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 20)
	seedTitanEndlessWorkFeature(t, s)
	id := seedScienceNinCPCharacter(t, s, "Kankuro", 6, "class/science-nin/group/scientific-inquiry/mech-crafter")
	idStr := strconv.FormatInt(id, 10)
	popupPath := "/characters/" + idStr + "/titan-slots"

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Exo-Suit") {
		t.Fatalf("popup missing Exo-Suit section once Endless Work is granted:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/exosuit/add", url.Values{"option_slug": {cpTestTitanUpgradeSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("exosuit add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTitanExosuitUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != cpTestTitanUpgradeSlug {
		t.Fatalf("exosuit picks after add = %+v, want one row for %q", picks, cpTestTitanUpgradeSlug)
	}

	delW := postPopupForm(t, s, popupPath+"/exosuit/delete", url.Values{"option_slug": {cpTestTitanUpgradeSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("exosuit delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTitanExosuitUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("exosuit picks after delete = %+v, want none", picks)
	}
}

// TestTitanSlotsPopupSpecialistCrafting covers the 14th-level Specialist
// Crafting keyword picker — a single "Mech or Weapon" <select>, not a
// catalog picker, so it's hand-rolled in character_titan_slots.html rather
// than routed through subclass_tracker_section.
func TestTitanSlotsPopupSpecialistCrafting(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 14, 20)
	seedTitanSpecialistCraftingFeature(t, s)
	id := seedScienceNinCPCharacter(t, s, "Kankuro", 14, "class/science-nin/group/scientific-inquiry/mech-crafter")
	idStr := strconv.FormatInt(id, 10)
	popupPath := "/characters/" + idStr + "/titan-slots"

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Specialist Crafting") {
		t.Fatalf("popup missing Specialist Crafting section once granted:\n%s", body)
	}

	setW := postPopupForm(t, s, popupPath+"/specialist-crafting", url.Values{"keyword": {"mech"}})
	if setW.Code != http.StatusSeeOther {
		t.Fatalf("set specialist crafting keyword: status %d, body %q", setW.Code, setW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTitanSpecialistCraftingKeyword)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != "mech" {
		t.Fatalf("specialist crafting picks = %+v, want one row for \"mech\"", picks)
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, `value="mech" selected`) {
		t.Errorf("Specialist Crafting <select> should show mech pre-selected after setting it:\n%s", after)
	}
}

// TestSubclassTrackerPopupSidebarButtons covers the sidebar gating itself
// (character_sheet.html): each button must appear only for the subclass it
// belongs to, never for a character with neither subclass, and the old
// inline pickers these buttons replaced must be gone from the Core
// sheet/Companions tab rather than duplicated alongside the popup.
func TestSubclassTrackerPopupSidebarButtons(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 20)

	snbID := seedScienceNinCPCharacter(t, s, "Kabuto", 6, "class/science-nin/group/scientific-inquiry/s-n-b-specialist")
	snbBody := getPopup(t, s, "/characters/"+strconv.FormatInt(snbID, 10)).Body.String()
	if !strings.Contains(snbBody, `href="/characters/`+strconv.FormatInt(snbID, 10)+`/snb-upgrades"`) {
		t.Errorf("S.N.B Specialist sheet missing the S.N.B Upgrades sidebar button:\n%s", snbBody)
	}
	if strings.Contains(snbBody, `href="/characters/`+strconv.FormatInt(snbID, 10)+`/titan-slots"`) {
		t.Errorf("S.N.B Specialist sheet should not show the Titan Slots sidebar button")
	}
	// The old inline picker forms must be gone, replaced by a pointer to
	// the popup — not duplicated alongside it.
	if strings.Contains(snbBody, `action="/characters/`+strconv.FormatInt(snbID, 10)+`/sheet/science-nin-snb-upgrade"`) {
		t.Errorf("Core sheet still renders the old inline S.N.B Upgrades picker form; it should only point at the popup now")
	}
	if !strings.Contains(snbBody, `S.N.B Upgrades and Future of Shinobi: S.N.B are tracked in the "S.N.B Upgrades" popup`) {
		t.Errorf("Core sheet missing the pointer text to the S.N.B Upgrades popup:\n%s", snbBody)
	}

	mechID := seedScienceNinCPCharacter(t, s, "Kankuro", 6, "class/science-nin/group/scientific-inquiry/mech-crafter")
	mechBody := getPopup(t, s, "/characters/"+strconv.FormatInt(mechID, 10)).Body.String()
	if !strings.Contains(mechBody, `href="/characters/`+strconv.FormatInt(mechID, 10)+`/titan-slots"`) {
		t.Errorf("Mech Crafter sheet missing the Titan Slots sidebar button:\n%s", mechBody)
	}
	if strings.Contains(mechBody, `href="/characters/`+strconv.FormatInt(mechID, 10)+`/snb-upgrades"`) {
		t.Errorf("Mech Crafter sheet should not show the S.N.B Upgrades sidebar button")
	}

	plainID := seedScienceNinCPCharacter(t, s, "Genin", 6, "")
	plainBody := getPopup(t, s, "/characters/"+strconv.FormatInt(plainID, 10)).Body.String()
	if strings.Contains(plainBody, "/snb-upgrades\"") || strings.Contains(plainBody, "/titan-slots\"") {
		t.Errorf("a Science-Nin with neither subclass should show neither subclass tracker popup button")
	}
}
