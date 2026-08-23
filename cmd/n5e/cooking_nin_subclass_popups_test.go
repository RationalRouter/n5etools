package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the four subclass tracker popups converted from
// Cooking-Nin's own subclass-gated Core-sheet sections: "Bonus Tool
// Infusion: Pipe" (Herbalist), "Expert Combatant" (Battle Cook), "Fast and
// Furious" (Entremetier Chef), and "Nature's Blend Enhancements"
// (Gastrochemist) — mirroring genjutsu_subclass_popups_test.go's own
// EmptyHint + render/round-trip + sidebar-gating shape.
// getPopup/postPopupForm/knownJutsuIDFromBody are shared helpers already
// defined in subclass_tracker_popup_test.go/subclass_tracker_popup_new_
// test.go. The Pipe implement catalog (weapon/cooking-pipe-*, migration
// 0064) and Expert Combatant's simple/martial weapon catalog (e.g.
// weapon/torinawa, one of the few base weapons a migration inserts
// outright rather than only patching content a real ingest run adds
// later — most base weapons, e.g. Kunai, only appear via UPDATE
// statements and so don't exist on a bare schema.Apply'd test database)
// are both already present on a fresh testServer() — unlike Genjutsu's
// own hand-rolled "jutsu_id" pickers, neither of these two catalogs needs
// its own seed data.

// seedCookingNinSubclassCatalog seeds enough rules.db content for all four
// Cooking Focus subclasses to render: the class itself, its Cooking Focus
// subclass_groups row, all four subclasses, each one's own gating feature
// row (matched by hasGrantedFeature via loadMergedGrantedFeatures), and one
// Fire-Release jutsu for Gastrochemist's own Nature's Blend Enhancement
// picker to offer.
func seedCookingNinSubclassCatalog(t *testing.T, s *server) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/cooking-nin', 'Cooking-Nin', 8, 8)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/cooking-nin/group/cooking-focus', 'class/cooking-nin', 'Cooking Focus')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/cooking-nin/group/cooking-focus/herbalist', 'class/cooking-nin/group/cooking-focus', 'Herbalist'),
		('class/cooking-nin/group/cooking-focus/battle-cook', 'class/cooking-nin/group/cooking-focus', 'Battle Cook'),
		('class/cooking-nin/group/cooking-focus/entremetier-chef', 'class/cooking-nin/group/cooking-focus', 'Entremetier Chef'),
		('class/cooking-nin/group/cooking-focus/gastrochemist', 'class/cooking-nin/group/cooking-focus', 'Gastrochemist')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		(?, 'class/cooking-nin/group/cooking-focus/herbalist', 'Bonus Tool Infusion: Pipe', 3, 'Pipe description'),
		(?, 'class/cooking-nin/group/cooking-focus/battle-cook', 'Expert Combatant', 2, 'Expert Combatant description'),
		(?, 'class/cooking-nin/group/cooking-focus/entremetier-chef', 'Fast and Furious', 2, 'Fast and Furious description'),
		(?, 'class/cooking-nin/group/cooking-focus/gastrochemist', 'Nature''s Blend', 2, 'Nature''s Blend description')`,
		bonusToolInfusionPipeFeatureSlug, expertCombatantFeatureSlug, fastAndFuriousFeatureSlug, gastrochemistNaturesBlendFeatureSlug)
	mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/test-fire-blend', 'Test Fire Blend Jutsu', 'Ninjutsu', 'C', '1 Action', '30 ft', 'Instantaneous', 'HS', 'Cost: 2', 'Fire Release, Offensive',
		        'A test jutsu of the Fire release.')`)
}

// seedCookingNinSubclassCharacter inserts a character with the given
// Cooking-Nin level, already 2nd-level-subclassed into the given Cooking
// Focus.
func seedCookingNinSubclassCharacter(t *testing.T, s *server, name, subclassSlug string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 16, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/cooking-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, ?, 2)`,
		id, subclassSlug,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedGastrochemistCharacter mirrors seedCookingNinSubclassCharacter for
// the Gastrochemist subclass specifically, additionally seeding one known
// Fire-release jutsu and the character's own Nature's Blend release
// element — both required for loadKnownBlendJutsu to offer anything on the
// Available list.
func seedGastrochemistCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	id := seedCookingNinSubclassCharacter(t, s, name, "class/cooking-nin/group/cooking-focus/gastrochemist", level)
	if _, err := s.charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (?, 'jutsu/test-fire-blend')`, id,
	); err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetElementalAffinity(s.charDB, id, "natures-blend", "Fire"); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedPlainCookingNinCharacter inserts a plain Cooking-Nin character with
// no subclass chosen — used by the EmptyHint tests, which only need the
// base class seeded without triggering any Cooking Focus's own gate.
func seedPlainCookingNinCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 10, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/cooking-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// --- Bonus Tool Infusion: Pipe (Herbalist) ---------------------------------

func TestPipePopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedPlainCookingNinCharacter(t, s, "Chouji", 1) // no subclass -> ungranted

	w := getPopup(t, s, pipePopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Herbalist grants this at 3rd level") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestPipePopupRenderSetRoundTrip(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedCookingNinSubclassCharacter(t, s, "Chouji", "class/cooking-nin/group/cooking-focus/herbalist", 11)
	popupPath := pipePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Tobacco Pipe") {
		t.Errorf("initial render missing an implement option:\n%s", body)
	}

	sets := []struct {
		field string
		value string
	}{
		{"implement", "weapon/cooking-pipe-tobacco"},
		{"damage-type", "Bludgeoning"},
		{"property-l3", "deep-breath"},
		{"property-l6", "inhaled-herb"},
		{"property-l11", "constant-smoke"},
	}
	for _, set := range sets {
		w := postPopupForm(t, s, popupPath+"/"+set.field, url.Values{"value": {set.value}})
		if w.Code != http.StatusSeeOther {
			t.Fatalf("set %s=%s: status %d, body %q", set.field, set.value, w.Code, w.Body.String())
		}
	}

	view, err := s.loadCookingToolInfusionPipeView(id, 11)
	if err != nil {
		t.Fatal(err)
	}
	if view.Implement != "weapon/cooking-pipe-tobacco" {
		t.Errorf("Implement = %q, want weapon/cooking-pipe-tobacco", view.Implement)
	}
	if view.DamageType != "Bludgeoning" {
		t.Errorf("DamageType = %q, want Bludgeoning", view.DamageType)
	}
	if view.PropertyL3 != "deep-breath" || view.PropertyL6 != "inhaled-herb" || view.PropertyL11 != "constant-smoke" {
		t.Errorf("properties = %q/%q/%q, want deep-breath/inhaled-herb/constant-smoke", view.PropertyL3, view.PropertyL6, view.PropertyL11)
	}

	afterSet := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterSet, "Tobacco Pipe") {
		t.Errorf("popup after set missing the chosen implement's name:\n%s", afterSet)
	}
}

// --- Expert Combatant (Battle Cook) ----------------------------------------

func TestBattleCookExpertCombatantPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedPlainCookingNinCharacter(t, s, "Ino", 1) // no subclass -> ungranted

	w := getPopup(t, s, battleCookExpertCombatantPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Battle Cook grants this at 2nd level") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestBattleCookExpertCombatantPopupRenderSet(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedCookingNinSubclassCharacter(t, s, "Ino", "class/cooking-nin/group/cooking-focus/battle-cook", 5)
	popupPath := battleCookExpertCombatantPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Torinawa") {
		t.Errorf("initial render missing a weapon option:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/weapon", url.Values{"value": {"weapon/torinawa"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("set: status %d, body %q", w.Code, w.Body.String())
	}

	view, err := s.loadExpertCombatantView(id)
	if err != nil {
		t.Fatal(err)
	}
	if view.Weapon != "weapon/torinawa" {
		t.Errorf("Weapon = %q, want weapon/torinawa", view.Weapon)
	}

	afterSet := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterSet, "Torinawa") {
		t.Errorf("popup after set missing the chosen weapon's name:\n%s", afterSet)
	}
}

// --- Fast and Furious (Entremetier Chef) -----------------------------------

func TestFastAndFuriousPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedPlainCookingNinCharacter(t, s, "Sasuke", 1) // no subclass -> ungranted

	w := getPopup(t, s, fastAndFuriousPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Entremetier Chef grants this at 2nd level") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestFastAndFuriousPopupRenderSet(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedCookingNinSubclassCharacter(t, s, "Sasuke", "class/cooking-nin/group/cooking-focus/entremetier-chef", 5)
	popupPath := fastAndFuriousPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/set", url.Values{"value": {"int"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("set: status %d, body %q", w.Code, w.Body.String())
	}

	// Freely re-editable, unlike every lock-once-chosen pick above.
	w = postPopupForm(t, s, popupPath+"/set", url.Values{"value": {"cha"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("re-set: status %d, body %q", w.Code, w.Body.String())
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil || data.FastAndFurious == nil || data.FastAndFurious.Current != "cha" {
		t.Fatalf("FastAndFurious after re-set = %+v, want Current=cha", data.FastAndFurious)
	}
}

// --- Nature's Blend Enhancements (Gastrochemist) ---------------------------

func TestBlendEnhancementsPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedPlainCookingNinCharacter(t, s, "Choza", 1) // no subclass -> Cap 0

	w := getPopup(t, s, blendEnhancementsPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Gastrochemist grants this at 2nd level") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestBlendEnhancementsPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedCookingNinSubclassCatalog(t, s)
	id := seedGastrochemistCharacter(t, s, "Choza", 5) // cap 3 at 5th level
	popupPath := blendEnhancementsPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Fire Blend Jutsu") {
		t.Errorf("initial render missing the available known jutsu option:\n%s", body)
	}

	jutsuIDStr := knownJutsuIDFromBody(t, body, "Test Fire Blend Jutsu")

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"jutsu_id": {jutsuIDStr}, "enhancement_type": {"texture"}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListCookingNinBlendEnhancementPicks(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	jutsuID, _ := strconv.ParseInt(jutsuIDStr, 10, 64)
	if len(picks) != 1 || picks[0].JutsuID != jutsuID || picks[0].EnhancementType != "texture" {
		t.Fatalf("picks after add = %+v, want one row for jutsu id %d, type texture", picks, jutsuID)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Fire Blend Jutsu") {
		t.Errorf("popup after add missing Test Fire Blend Jutsu in Known:\n%s", afterAdd)
	}
	if !strings.Contains(afterAdd, "Enhance Texture") {
		t.Errorf("popup after add missing the chosen Enhancement label:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"jutsu_id": {jutsuIDStr}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListCookingNinBlendEnhancementPicks(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Sidebar buttons for all four popups -----------------------------------

// TestCookingNinSubclassPopupSidebarButtons covers character_sheet.html's
// own gating for the 4 sidebar buttons this conversion added, and confirms
// each popup's own pointer text is present on the Core sheet — the same
// two checks TestTwistedCastingPsycheBreakerSidebarButtons already makes
// for its own 2 popups.
func TestCookingNinSubclassPopupSidebarButtons(t *testing.T) {
	cases := []struct {
		name       string
		seed       func(t *testing.T, s *server) int64
		popupPath  func(int64) string
		buttonText string
	}{
		{"Pipe", func(t *testing.T, s *server) int64 {
			seedCookingNinSubclassCatalog(t, s)
			return seedCookingNinSubclassCharacter(t, s, "Chouji", "class/cooking-nin/group/cooking-focus/herbalist", 11)
		}, pipePopupPath, `tracked in the "Bonus Tool Infusion: Pipe" popup`},
		{"Expert Combatant", func(t *testing.T, s *server) int64 {
			seedCookingNinSubclassCatalog(t, s)
			return seedCookingNinSubclassCharacter(t, s, "Ino", "class/cooking-nin/group/cooking-focus/battle-cook", 5)
		}, battleCookExpertCombatantPopupPath, `tracked in the "Expert Combatant (Battle Cook)" popup`},
		{"Fast and Furious", func(t *testing.T, s *server) int64 {
			seedCookingNinSubclassCatalog(t, s)
			return seedCookingNinSubclassCharacter(t, s, "Sasuke", "class/cooking-nin/group/cooking-focus/entremetier-chef", 5)
		}, fastAndFuriousPopupPath, `tracked in the "Fast and Furious" popup`},
		{"Nature's Blend Enhancements", func(t *testing.T, s *server) int64 {
			seedCookingNinSubclassCatalog(t, s)
			return seedGastrochemistCharacter(t, s, "Choza", 5)
		}, blendEnhancementsPopupPath, `tracked in the "Nature's Blend Enhancements" popup`},
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
		})
	}
}
