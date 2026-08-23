package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the 5 Science-Nin subclass tracker popups wired up after
// S.N.B Specialist/Mech Crafter (subclass_tracker_popup_test.go) — Elemental
// Innovationist, Grenadier, Mad Scientist, Ninjaneer, and Shinobi-Ware — one
// EmptyHint test plus one render/add/delete round trip per popup, mirroring
// TestSNBUpgradesPopupEmptyHint/TestSNBUpgradesPopupRenderAddDelete's own
// shape. Grenadier and Shinobi-Ware reuse existing seed helpers
// (science_nin_bim_test.go, ever_evolving_test.go) directly rather than
// duplicating them.

// --- Elemental Innovationist -----------------------------------------------

const (
	eiTestEIPSlug = "class/science-nin/option/e-i-ps/minor/entry/test-eip"
	eiTestWoWSlug = "class/science-nin/option/wow/entry/test-wow"
)

// seedElementalInnovationistCatalog seeds enough rules.db content for an
// Elemental Innovationist's own Exoskeleton/E.I.Ps/W.O.W picks to render:
// the class itself, a Creation Points chart row, Elemental Innovationist's
// own subclass chain, its 3rd-level Exoskeleton and E.I.Ps granting
// features, a one-entry E.I.Ps catalog (tiered, via class_option_entries —
// loadScienceNinEntryCatalog's own shape), and a one-row W.O.W catalog (flat,
// via loadScienceNinFlatCatalog — the class_options row's own slug IS the
// pickable option).
func seedElementalInnovationistCatalog(t *testing.T, s *server, level int) {
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
		('class/science-nin/group/scientific-inquiry/elemental-innovationist', 'class/science-nin/group/scientific-inquiry', 'Elemental Innovationist')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/elemental-innovationist', 'Exoskeleton', 3, 'Test.', 1),
		(?, 'class/science-nin/group/scientific-inquiry/elemental-innovationist', 'Elemental Infused Perks (E.I.Ps)', 3, 'Test.', 2)`,
		scienceNinExoskeletonFeatureSlug, scienceNinEIPFeatureSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/e-i-ps/minor', 'class/science-nin', NULL, 'E.I.Ps', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/e-i-ps/minor', 'Test E.I.P', 'A test elemental perk.', 1)`, eiTestEIPSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		(?, 'class/science-nin', NULL, 'W.O.W (Weapons of Wonder)', 'Test W.O.W', 'A test wonder weapon.', 1)`, eiTestWoWSlug)
}

// seedElementalInnovationistCharacter inserts a character with the given
// Science-Nin level, already 3rd-level-subclassed into Elemental
// Innovationist.
func seedElementalInnovationistCharacter(t *testing.T, s *server, name string, level int) int64 {
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
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/elemental-innovationist', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestElementalInnovationistPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedElementalInnovationistCatalog(t, s, 1) // below Exoskeleton/E.I.Ps' own 3rd-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Karui", 1)

	w := getPopup(t, s, elementalInnovationistPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "no Elemental Innovationist picks yet") {
		t.Errorf("body missing empty hint:\n%s", body)
	}
}

// TestElementalInnovationistPopupRenderAddDelete covers the round trip
// through the real registered routes for both E.I.Ps (a catalog pick) and
// Exoskeleton (a bare toggle) — the two section shapes this popup
// introduces beyond the generic Known/Available pattern SNB/Titan already
// cover.
func TestElementalInnovationistPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedElementalInnovationistCatalog(t, s, 6)
	id := seedElementalInnovationistCharacter(t, s, "Karui", 6)
	popupPath := elementalInnovationistPopupPath(id)

	w := getPopup(t, s, popupPath)
	if w.Code != http.StatusOK {
		t.Fatalf("initial render: status %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test E.I.P") {
		t.Errorf("initial render missing the available E.I.P option:\n%s", body)
	}
	if !strings.Contains(body, "Exoskeleton: Doffed") {
		t.Errorf("initial render missing the Doffed Exoskeleton toggle:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/eip/add", url.Values{"option_slug": {eiTestEIPSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("eip add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickEIP)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != eiTestEIPSlug {
		t.Fatalf("picks after eip add = %+v, want one row for %q", picks, eiTestEIPSlug)
	}

	toggleW := postPopupForm(t, s, popupPath+"/exoskeleton", url.Values{"on": {"1"}})
	if toggleW.Code != http.StatusSeeOther {
		t.Fatalf("exoskeleton toggle: status %d, body %q", toggleW.Code, toggleW.Body.String())
	}
	afterToggle := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterToggle, "Exoskeleton: Donned") {
		t.Errorf("popup after toggling on should read Donned:\n%s", afterToggle)
	}

	delW := postPopupForm(t, s, popupPath+"/eip/delete", url.Values{"option_slug": {eiTestEIPSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("eip delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickEIP)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after eip delete = %+v, want none", picks)
	}
}

// seedNinjaneerlessScienceNinCharacter inserts a plain Science-Nin character
// with no subclass chosen — used by every EmptyHint test in this file that
// only needs the base class seeded (via one of this file's own *Catalog
// helpers) without triggering that catalog's own subclass gate.
func seedNinjaneerlessScienceNinCharacter(t *testing.T, s *server, name string, level int) int64 {
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
	return id
}

// --- Grenadier ---------------------------------------------------------
// Reuses seedGrenadierBIMCatalog/seedGrenadierCharacter (science_nin_bim_
// test.go) directly — B.I.M's own data-layer behavior (repeat picks,
// Barrier B.I.M's own exception, the cap) is already covered there; this
// only needs to confirm the popup route itself renders and round-trips.

func TestGrenadierPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedGrenadierBIMCatalog(t, s, 1) // below B.I.M's own 3rd-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Deidara", 1)

	w := getPopup(t, s, grenadierPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no B.I.M") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestGrenadierPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedGrenadierBIMCatalog(t, s, 10)
	id := seedGrenadierCharacter(t, s, "Deidara", 10)
	popupPath := grenadierPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Timer B.I.M") || !strings.Contains(body, "Barrier B.I.M") {
		t.Errorf("initial render missing both available B.I.M options:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/bim/add", url.Values{"option_slug": {timerBIMSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("bim add: status %d, body %q", addW.Code, addW.Body.String())
	}
	gr := loadGrenadierData(t, s, id)
	if gr.BIMUsed != 1 || len(gr.KnownBIM) != 1 || gr.KnownBIM[0].Slug != timerBIMSlug {
		t.Fatalf("Grenadier data after popup add = %+v, want one known Timer B.I.M", gr)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Timer B.I.M") {
		t.Errorf("popup after add missing Timer B.I.M in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/bim/delete", url.Values{"option_slug": {timerBIMSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("bim delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	gr = loadGrenadierData(t, s, id)
	if gr.BIMUsed != 0 {
		t.Fatalf("Grenadier data after popup delete = %+v, want no B.I.M held", gr)
	}
}

// --- Mad Scientist -------------------------------------------------------

// madScientistTestSerumSlug is deliberately dual-pool (no "Mending"/
// "Maiming" word in its Drain line, so inversionSerumFixedPool returns "")
// — exercising inversionSerumSection's own Slug+"|mending"/Slug+"|maiming"
// splitting, the one piece of this popup's own logic
// TestScienceNinInversionSerumUnaffectedByBIMChange (science_nin_bim_test.
// go, a FIXED-pool serum) doesn't already cover.
const madScientistTestSerumSlug = "class/science-nin/option/inversion-serums/minor/entry/test-dual-serum"

func seedMadScientistCatalog(t *testing.T, s *server, level int) {
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
		('class/science-nin/group/scientific-inquiry/mad-scientist', 'class/science-nin/group/scientific-inquiry', 'Mad Scientist')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/mad-scientist', 'Inversion Serums', 3,
		 'You can hold a number of Serums equal to your Intelligence modifier.', 1)`, scienceNinInversionSerumsFeatureSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/inversion-serums/minor', 'class/science-nin', NULL, 'Inversion Serums', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/inversion-serums/minor', 'Test Dual Serum', 'Drain: 2 CCD Chakra. A dual-pool test serum.', 1)`,
		madScientistTestSerumSlug)
}

func seedMadScientistCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 16, 10, 10)`, name) // Int 16 -> +3 mod -> SerumCap 3
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
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/mad-scientist', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMadScientistPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedMadScientistCatalog(t, s, 1) // below Inversion Serums' own 3rd-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Kabuto", 1)

	w := getPopup(t, s, madScientistPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Inversion Serums yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestMadScientistPopupDualPoolRenderAddDelete covers the dual-pool split:
// a Serum with no fixed pool must offer BOTH a Mending and a Maiming pick
// through the popup's Available list, installing one must record that pool,
// and deleting must use the plain (non-pool-suffixed) slug the Known list
// renders.
func TestMadScientistPopupDualPoolRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedMadScientistCatalog(t, s, 6)
	id := seedMadScientistCharacter(t, s, "Kabuto", 6)
	popupPath := madScientistPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `value="`+madScientistTestSerumSlug+`|mending"`) {
		t.Errorf("dual-pool serum missing its Mending radio option:\n%s", body)
	}
	if !strings.Contains(body, `value="`+madScientistTestSerumSlug+`|maiming"`) {
		t.Errorf("dual-pool serum missing its Maiming radio option:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/inversion-serum/add", url.Values{"option_slug": {madScientistTestSerumSlug + "|mending"}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("inversion serum add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickInversionSerum)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != madScientistTestSerumSlug || picks[0].Pool != "mending" {
		t.Fatalf("picks after add = %+v, want one Mending-pool row for %q", picks, madScientistTestSerumSlug)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Dual Serum") {
		t.Errorf("popup after add missing Test Dual Serum in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/inversion-serum/delete", url.Values{"option_slug": {madScientistTestSerumSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("inversion serum delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickInversionSerum)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Ninjaneer -----------------------------------------------------------
// TestNinjaneerWeaponDesignationTemplateRendersTooltip (ninjaneer_test.go)
// only confirms the three weapon-designation pickers render inside this
// popup — it never posts to any of the popup's own add/delete routes.
// TestNinjaneerPopupWeaponDesignationRenderAddDelete below is what actually
// exercises Enhanced Weapon/Legendary Weapon/Perfected Weapon Mark's add+
// delete round trip through ninjaneerWeaponDesignationPopupAdd/Delete
// (ninjaneer.go), the same way TestNinjaneerFutureOfShinobiWeaponsDamageBonus
// already does for the older Core-sheet routes those functions share their
// validation with. This file's own TestNinjaneerPopupArsenalModRenderAddDelete
// still separately covers the one section this doesn't: Arsenal
// Modifications' own catalog pick.

const ninjaneerTestArsenalModSlug = "class/science-nin/option/arsenal-modifications/minor/entry/test-mod"

// seedNinjaneerArsenalModCatalog adds an Arsenal Modifications catalog on
// top of seedNinjaneerCatalog, which grants the gating Enhanced Arsenal
// feature (scienceNinArsenalFeatureSlug) but seeds no catalog of its own.
func seedNinjaneerArsenalModCatalog(t *testing.T, s *server) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/arsenal-modifications/minor', 'class/science-nin', NULL, 'Arsenal Modifications', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/arsenal-modifications/minor', 'Test Arsenal Mod', 'A test arsenal modification.', 1)`,
		ninjaneerTestArsenalModSlug)
}

func TestNinjaneerPopupArsenalModRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 3)
	seedNinjaneerArsenalModCatalog(t, s)
	id := seedNinjaneerCharacter(t, s, "Karin", 3, 14)
	popupPath := ninjaneerPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Arsenal Mod") {
		t.Errorf("initial render missing the available Arsenal Mod option:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/arsenal-mod/add", url.Values{"option_slug": {ninjaneerTestArsenalModSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("arsenal mod add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickArsenalMod)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].OptionSlug != ninjaneerTestArsenalModSlug {
		t.Fatalf("picks after add = %+v, want one row for %q", picks, ninjaneerTestArsenalModSlug)
	}

	delW := postPopupForm(t, s, popupPath+"/arsenal-mod/delete", url.Values{"option_slug": {ninjaneerTestArsenalModSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("arsenal mod delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickArsenalMod)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// TestNinjaneerPopupWeaponDesignationRenderAddDelete covers all three
// single-slot weapon designations (Enhanced Weapon, Legendary Weapon,
// Perfected Weapon Mark) through the Ninjaneer popup's own routes: each must
// render its picker when nothing is designated, accept an add that shows the
// designated weapon in Known, and accept a delete that clears it back to the
// picker — the same pick/discard round trip every other subclass tracker
// popup category gets, for the one shape (an inventory-row designation, not
// a catalog Slug) unique to this popup.
func TestNinjaneerPopupWeaponDesignationRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 20) // 20th level: Enhanced Arsenal, Warrior of Science, and A Weapon to Surpass are all granted
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 20, 16)
	finesseInv := addInventoryWeapon(t, s, id, "weapon/test-finesse")
	heavyInv := addInventoryWeapon(t, s, id, "weapon/test-heavy")
	popupPath := ninjaneerPopupPath(id)

	cases := []struct {
		tier       string // route segment
		category   charstore.ScienceNinSubclassPickCategory
		addLabel   string
		inventory  int64
		weaponName string
	}{
		{"enhanced-weapon", charstore.ScienceNinPickEnhancedWeapon, "Designate Enhanced Weapon", finesseInv, "Test Finesse Blade"},
		{"legendary-weapon", charstore.ScienceNinPickLegendaryWeapon, "Designate Legendary Weapon", heavyInv, "Test Heavy Blade"},
		{"perfected-weapon-mark", charstore.ScienceNinPickPerfectedWeaponMark, "Mark Perfected Weapon", finesseInv, "Test Finesse Blade"},
	}

	for _, c := range cases {
		t.Run(c.tier, func(t *testing.T) {
			before := getPopup(t, s, popupPath).Body.String()
			if !strings.Contains(before, c.addLabel) {
				t.Fatalf("popup missing %q picker before designating anything:\n%s", c.addLabel, before)
			}

			addW := postPopupForm(t, s, popupPath+"/"+c.tier+"/add", url.Values{"option_slug": {strconv.FormatInt(c.inventory, 10)}})
			if addW.Code != http.StatusSeeOther {
				t.Fatalf("%s add: status %d, body %q", c.tier, addW.Code, addW.Body.String())
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, c.category)
			if err != nil {
				t.Fatal(err)
			}
			if len(picks) != 1 || picks[0].OptionSlug != strconv.FormatInt(c.inventory, 10) {
				t.Fatalf("%s picks after add = %+v, want one row for inventory id %d", c.tier, picks, c.inventory)
			}
			afterAdd := getPopup(t, s, popupPath).Body.String()
			if !strings.Contains(afterAdd, c.weaponName) {
				t.Errorf("%s popup after add missing designated weapon %q in Known:\n%s", c.tier, c.weaponName, afterAdd)
			}

			delW := postPopupForm(t, s, popupPath+"/"+c.tier+"/delete", url.Values{"option_slug": {strconv.FormatInt(c.inventory, 10)}})
			if delW.Code != http.StatusSeeOther {
				t.Fatalf("%s delete: status %d, body %q", c.tier, delW.Code, delW.Body.String())
			}
			picks, err = charstore.ListScienceNinSubclassPicks(s.charDB, id, c.category)
			if err != nil {
				t.Fatal(err)
			}
			if len(picks) != 0 {
				t.Fatalf("%s picks after delete = %+v, want none", c.tier, picks)
			}
			afterDelete := getPopup(t, s, popupPath).Body.String()
			if !strings.Contains(afterDelete, c.addLabel) {
				t.Errorf("%s popup after delete should show the picker again:\n%s", c.tier, afterDelete)
			}
		})
	}
}

// --- Shinobi-Ware ----------------------------------------------------------
// Reuses seedEverEvolvingRules/seedEverEvolvingCharacter (ever_evolving_
// test.go) directly — Ever Evolving's own data-layer behavior (budget
// enforcement, single-slot cap, refund on delete) is already covered there
// through the Core-sheet's own route; this only needs to confirm the same
// mutations also work through the popup's own routes and render correctly.

func TestShinobiWarePopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedEverEvolvingRules(t, s, 1, 20) // below Ever Evolving's own 14th-level gate
	id := seedNinjaneerlessScienceNinCharacter(t, s, "Kabuto", 1)

	w := getPopup(t, s, shinobiWarePopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Shinobi-Ware Upgrades yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestShinobiWarePopupEverEvolvingRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedEverEvolvingRules(t, s, 14, 20)
	id := seedEverEvolvingCharacter(t, s, "Kabuto", 14)
	popupPath := shinobiWarePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Minor Test Seal") {
		t.Errorf("initial render missing an available Ever Evolving seal option:\n%s", body)
	}
	// Shinobi-Ware Upgrades' own section must also render (0/0 slots, since
	// Edge Runner isn't granted by this helper), confirming the top-level
	// section doesn't error out when its own gating feature is absent.
	if !strings.Contains(body, "Shinobi-Ware Upgrades (0/0 slots)") {
		t.Errorf("popup missing the always-present Shinobi-Ware Upgrades section header:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/ever-evolving-seal/add", url.Values{"option_slug": {"seal/armor/test/minor-test-seal"}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("ever evolving seal add: status %d, body %q", addW.Code, addW.Body.String())
	}
	sw := loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal == nil || sw.EverEvolvingSeal.Slug != "seal/armor/test/minor-test-seal" {
		t.Fatalf("Shinobi-Ware data after popup add = %+v, want Minor Test Seal applied", sw.EverEvolvingSeal)
	}

	delW := postPopupForm(t, s, popupPath+"/ever-evolving-seal/delete", url.Values{"option_slug": {"seal/armor/test/minor-test-seal"}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("ever evolving seal delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	sw = loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal != nil {
		t.Fatalf("Shinobi-Ware data after popup delete = %+v, want no seal applied", sw.EverEvolvingSeal)
	}
}

// --- Sidebar buttons for all 5 new popups -----------------------------

// TestNewSubclassTrackerSidebarButtons covers character_sheet.html's own
// gating for the 5 sidebar buttons this session's conversion added, and
// confirms the old inline pickers they replaced are gone from the Core
// sheet — the same two checks TestSubclassTrackerPopupSidebarButtons already
// makes for S.N.B Specialist/Mech Crafter, extended to the rest of
// Science-Nin's subclasses.
func TestNewSubclassTrackerSidebarButtons(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, s *server) int64
		popupPath   func(int64) string
		buttonText  string
		goneAddText string // the popup's own catalog-pick AddLabel — must not leak into the Core sheet, since the inline picker it replaced is gone
	}{
		{"Elemental Innovationist", func(t *testing.T, s *server) int64 {
			seedElementalInnovationistCatalog(t, s, 6)
			return seedElementalInnovationistCharacter(t, s, "Karui", 6)
		}, elementalInnovationistPopupPath, `tracked in the "Elemental Innovationist" popup`, "Fit Perk"},
		{"Grenadier", func(t *testing.T, s *server) int64 {
			seedGrenadierBIMCatalog(t, s, 6)
			return seedGrenadierCharacter(t, s, "Deidara", 6)
		}, grenadierPopupPath, `tracked in the "Grenadier" popup`, "Load Bandolier"},
		{"Mad Scientist", func(t *testing.T, s *server) int64 {
			seedMadScientistCatalog(t, s, 6)
			return seedMadScientistCharacter(t, s, "Kabuto", 6)
		}, madScientistPopupPath, `tracked in the "Mad Scientist" popup`, "Brew Serum"},
		{"Ninjaneer", func(t *testing.T, s *server) int64 {
			seedNinjaneerCatalog(t, s, 6)
			seedNinjaneerArsenalModCatalog(t, s)
			return seedNinjaneerCharacter(t, s, "Karin", 6, 14)
		}, ninjaneerPopupPath, `tracked in the "Ninjaneer" popup`, "Install Modification"},
		{"Shinobi-Ware", func(t *testing.T, s *server) int64 {
			seedEverEvolvingRules(t, s, 14, 20)
			return seedEverEvolvingCharacter(t, s, "Kabuto", 14)
		}, shinobiWarePopupPath, `tracked in the "Shinobi-Ware" popup`, "Install Upgrade"},
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
