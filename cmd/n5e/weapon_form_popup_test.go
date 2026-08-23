package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the "Weapon Form" subclass tracker popup (weapon_form_
// popup.go), mirroring science_nin_more_subclass_popups_test.go's own
// EmptyHint + render/add/delete round-trip shape. Slayer Form is used as the
// seeded Form throughout since it's the only one of the 8 with its own extra
// Stalking Predator sub-section, letting one seed exercise every section
// shape this popup renders (auto-granted Techniques, cap-gated Styles,
// single re-selectable Stalking Predator, cap-gated base-class Superior
// Weapon Flurry). getPopup/postPopupForm are shared helpers already defined
// in subclass_tracker_popup_test.go.

const weaponFormTestStyleSlug = "class/weapon-specialist/group/weapon-forms/slayer-form/option/styles/test-style"

// seedSlayerFormCatalog seeds enough rules.db content for the Weapon Form
// popup to render every section: the class itself, a "Styles Known"
// class_level_resources row, Slayer Form's own subclass chain, its 3rd-level
// Slayer Techniques auto-grant feature (weaponFormTechniqueAutoGrants' own
// key for Slayer Form), and a one-entry Styles catalog. Stalking Predator
// (6th level) and Superior Weapon Flurry (14th level) both use hardcoded Go
// catalogs (stalkingPredatorOptions, superiorWeaponFlurryCatalog) — neither
// needs its own rules.db seeding.
func seedSlayerFormCatalog(t *testing.T, s *server, level, stylesCap int) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/weapon-specialist', 'Weapon Specialist', 10, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/weapon-specialist', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/weapon-specialist', ?, 'Styles Known', ?)`, level, strconv.Itoa(stylesCap))
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/weapon-specialist/group/weapon-forms', 'class/weapon-specialist', 'Weapon Forms')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/weapon-specialist/group/weapon-forms/slayer-form', 'class/weapon-specialist/group/weapon-forms', 'Slayer Form')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/weapon-specialist/group/weapon-forms/slayer-form/feature/slayer-techniques-changed',
		 'class/weapon-specialist/group/weapon-forms/slayer-form', 'Slayer Techniques', 3, 'You learn two Flurry Techniques.', 1)`)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		(?, 'class/weapon-specialist', 'class/weapon-specialist/group/weapon-forms/slayer-form', 'Slayer Form Styles', 'Test Style', 'A test Style.', 1)`,
		weaponFormTestStyleSlug)
}

// seedSlayerFormCharacter inserts a character with the given Weapon
// Specialist level, already 3rd-level-subclassed into Slayer Form.
func seedSlayerFormCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 14, 14, 12, 10, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/weapon-specialist', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/weapon-specialist/group/weapon-forms/slayer-form', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestWeaponFormPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	id := int64(1)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Nobody', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	w := getPopup(t, s, weaponFormPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Weapon Form chosen yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestWeaponFormPopupRenderAddDelete covers the round trip through the real
// registered routes for all four section shapes this popup introduces:
// Techniques (read-only, auto-granted), Styles (a cap-gated catalog pick,
// same shape as every other subclassTrackerSection user), Stalking
// Predator (a single freely re-editable <select>, hand-rolled outside
// subclassTrackerSection), and Superior Weapon Flurry (a second cap-gated
// catalog pick, base-class rather than Form-specific). Also confirms the
// mixed subclass-scope split: Techniques/Styles/Stalking Predator render
// inside the nested subclass-scope wrapper, Superior Weapon Flurry does
// not (see character_weapon_form.html's own doc comment).
func TestWeaponFormPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedSlayerFormCatalog(t, s, 14, 4)
	id := seedSlayerFormCharacter(t, s, "Zabuza", 14)
	popupPath := weaponFormPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("Weapon Form popup's outer div must NOT carry subclass-scope directly (it mixes base-class content) — only a nested div should:\n%s", body)
	}
	if !strings.Contains(body, `<div class="subclass-scope">`) {
		t.Errorf("Weapon Form popup missing its nested subclass-scope wrapper around Form-specific content:\n%s", body)
	}
	if !strings.Contains(body, "Studied Crippling") || !strings.Contains(body, "Studied Strike") {
		t.Errorf("initial render missing Slayer Form's own auto-granted Techniques:\n%s", body)
	}
	if !strings.Contains(body, "Test Style") {
		t.Errorf("initial render missing the available Style option:\n%s", body)
	}
	if !strings.Contains(body, "Vipers Tongue") || !strings.Contains(body, "Apex Glare") {
		t.Errorf("initial render missing the Stalking Predator options:\n%s", body)
	}
	if !strings.Contains(body, "Enhanced Deflection") {
		t.Errorf("initial render missing an available Superior Weapon Flurry benefit:\n%s", body)
	}
	// Superior Weapon Flurry must render after the subclass-scope div
	// closes, not inside it — found by counting nested <div>/</div> tags
	// from the wrapper's own opening tag to its matching close, rather
	// than a naive first-</div> search that would stop at an inner
	// .puppet-upgrade-pick's own closing tag instead.
	scopeStart := strings.Index(body, `<div class="subclass-scope">`)
	if scopeStart < 0 {
		t.Fatalf("subclass-scope wrapper not found:\n%s", body)
	}
	scopeCloseOffset := matchingDivClose(t, body[scopeStart:])
	flurryIdx := strings.Index(body, "Superior Weapon Flurry")
	if flurryIdx < 0 || flurryIdx < scopeStart+scopeCloseOffset {
		t.Errorf("Superior Weapon Flurry should render outside the subclass-scope div:\n%s", body)
	}

	styleAddW := postPopupForm(t, s, popupPath+"/style/add", url.Values{"option_slug": {weaponFormTestStyleSlug}})
	if styleAddW.Code != http.StatusSeeOther {
		t.Fatalf("style add: status %d, body %q", styleAddW.Code, styleAddW.Body.String())
	}
	styles, err := charstore.ListWeaponFormStyles(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(styles) != 1 || styles[0] != weaponFormTestStyleSlug {
		t.Fatalf("styles after add = %+v, want one row for %q", styles, weaponFormTestStyleSlug)
	}
	afterStyleAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterStyleAdd, `<span class="puppet-upgrade-pick-name">Test Style</span>`) {
		t.Errorf("known Style should render as plain text (no static detail route exists for this catalog):\n%s", afterStyleAdd)
	}

	predatorW := postPopupForm(t, s, popupPath+"/stalking-predator", url.Values{"value": {"apex-glare"}})
	if predatorW.Code != http.StatusSeeOther {
		t.Fatalf("stalking predator set: status %d, body %q", predatorW.Code, predatorW.Body.String())
	}
	afterPredator := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterPredator, `value="apex-glare" data-description="Skill: Intimidation.`) {
		t.Errorf("Stalking Predator select should show apex-glare pre-selected:\n%s", afterPredator)
	}

	flurryAddW := postPopupForm(t, s, popupPath+"/superior-weapon-flurry/add", url.Values{"option_slug": {"enhanced-deflection-duration"}})
	if flurryAddW.Code != http.StatusSeeOther {
		t.Fatalf("superior weapon flurry add: status %d, body %q", flurryAddW.Code, flurryAddW.Body.String())
	}
	flurry, err := charstore.ListSuperiorWeaponFlurryBenefits(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(flurry) != 1 || flurry[0] != "enhanced-deflection-duration" {
		t.Fatalf("superior weapon flurry picks after add = %+v, want one row", flurry)
	}

	styleDelW := postPopupForm(t, s, popupPath+"/style/delete", url.Values{"option_slug": {weaponFormTestStyleSlug}})
	if styleDelW.Code != http.StatusSeeOther {
		t.Fatalf("style delete: status %d, body %q", styleDelW.Code, styleDelW.Body.String())
	}
	styles, err = charstore.ListWeaponFormStyles(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(styles) != 0 {
		t.Fatalf("styles after delete = %+v, want none", styles)
	}

	flurryDelW := postPopupForm(t, s, popupPath+"/superior-weapon-flurry/delete", url.Values{"option_slug": {"enhanced-deflection-duration"}})
	if flurryDelW.Code != http.StatusSeeOther {
		t.Fatalf("superior weapon flurry delete: status %d, body %q", flurryDelW.Code, flurryDelW.Body.String())
	}
	flurry, err = charstore.ListSuperiorWeaponFlurryBenefits(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(flurry) != 0 {
		t.Fatalf("superior weapon flurry picks after delete = %+v, want none", flurry)
	}
}

// matchingDivClose returns the byte offset, within html (which must start
// with an opening "<div...>" tag), of the "</div>" that closes that very
// first div — counting nested "<div" opens against "</div>" closes rather
// than just returning the first "</div>" found, which would stop at an
// inner element's own close instead of the outer wrapper's.
func matchingDivClose(t *testing.T, html string) int {
	t.Helper()
	depth := 0
	pos := 0
	for pos < len(html) {
		openIdx := strings.Index(html[pos:], "<div")
		closeIdx := strings.Index(html[pos:], "</div>")
		if closeIdx < 0 {
			t.Fatalf("no matching </div> found")
		}
		if openIdx >= 0 && openIdx < closeIdx {
			depth++
			pos += openIdx + len("<div")
			continue
		}
		depth--
		pos += closeIdx + len("</div>")
		if depth == 0 {
			return pos
		}
	}
	t.Fatalf("no matching </div> found")
	return -1
}
