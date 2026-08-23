package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/features"
)

// This file covers the "Stancer" / "Passionate Flame" / "Ruin" subclass
// tracker popups (taijutsu_stancer_popup.go, taijutsu_passionate_flame_
// popup.go, taijutsu_ruin_popup.go), mirroring science_nin_more_subclass_
// popups_test.go's own EmptyHint + render/round-trip shape. All three hand-
// roll their own content (no subclassTrackerSection catalog picker — every
// pick here is one or two freely re-editable <select>s), same shape
// character_science_nin_technobi.html's own S.E.N.T select already
// establishes test coverage for. seedTaijutsuLevelResources (taijutsu_
// test.go, same package) seeds the shared base-class rows every case below
// builds on. getPopup/postPopupForm are shared helpers already defined in
// subclass_tracker_popup_test.go.

// seedTaijutsuStyleGroup seeds the shared subclass_groups parent row every
// Taijutsu Style subclass below hangs off.
func seedTaijutsuStyleGroup(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/taijutsu-specialist/group/taijutsu-style', 'class/taijutsu-specialist', 'Taijutsu Style')`); err != nil {
		t.Fatal(err)
	}
}

// seedTaijutsuStanceCatalog seeds two Taijutsu Stance rows (stance_type=
// 'taijutsu') for Stancer's own Mixed Martial Arts/Stance Blending pickers.
func seedTaijutsuStanceCatalog(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO fighting_stances (slug, name, stance_type, description) VALUES
		('stance/iron-fist-stance', 'Iron Fist Stance', 'taijutsu', 'Your unarmed strikes deal extra damage.'),
		('stance/whirlwind-stance', 'Whirlwind Stance', 'taijutsu', 'You gain extra reach on unarmed strikes.')`); err != nil {
		t.Fatal(err)
	}
}

func seedPlainTaijutsuCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 10, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/taijutsu-specialist', ?, 0)`, id, level); err != nil {
		t.Fatal(err)
	}
	return id
}

// --- Stancer -------------------------------------------------------------

func seedStancerCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	seedTaijutsuStyleGroup(t, s)
	seedTaijutsuStanceCatalog(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/taijutsu-specialist/group/taijutsu-style/stancer', 'class/taijutsu-specialist/group/taijutsu-style', 'Stancer')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/taijutsu-specialist/group/taijutsu-style/stancer', 'Mixed Martial Arts', 3, 'Learn one more Taijutsu Stance.', 1),
		(?, 'class/taijutsu-specialist/group/taijutsu-style/stancer', 'Stance Blending', 9, 'Learn one more Taijutsu Stance.', 2)`,
		mixedMartialArtsFeatureSlug, stanceBlendingFeatureSlug); err != nil {
		t.Fatal(err)
	}
	id := seedPlainTaijutsuCharacter(t, s, name, level)
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/taijutsu-specialist/group/taijutsu-style/stancer', 3)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStancerPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)
	seedTaijutsuStyleGroup(t, s)
	id := seedPlainTaijutsuCharacter(t, s, "Genin", 1)

	w := getPopup(t, s, stancerPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Stancer stance picks available yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestStancerPopupRenderSet covers the round trip through the real
// registered routes for both Mixed Martial Arts (3rd level) and Stance
// Blending (9th level) at once, at a level (9th) where both are present.
func TestStancerPopupRenderSet(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)
	id := seedStancerCharacter(t, s, "Ne", 9)
	popupPath := stancerPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Mixed Martial Arts") || !strings.Contains(body, "Stance Blending") {
		t.Errorf("initial render missing both stance sections at 9th level:\n%s", body)
	}
	if !strings.Contains(body, "Iron Fist Stance") || !strings.Contains(body, "Whirlwind Stance") {
		t.Errorf("initial render missing the Taijutsu Stance options:\n%s", body)
	}

	mmaW := postPopupForm(t, s, popupPath+"/mixed-martial-arts", url.Values{"stance_slug": {"stance/iron-fist-stance"}})
	if mmaW.Code != http.StatusSeeOther {
		t.Fatalf("mixed martial arts set: status %d, body %q", mmaW.Code, mmaW.Body.String())
	}
	sbW := postPopupForm(t, s, popupPath+"/stance-blending", url.Values{"stance_slug": {"stance/whirlwind-stance"}})
	if sbW.Code != http.StatusSeeOther {
		t.Fatalf("stance blending set: status %d, body %q", sbW.Code, sbW.Body.String())
	}

	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := choices[features.ChoiceKey{FeatureSlug: mixedMartialArtsFeatureSlug, ChoiceIndex: 0}]; got != "stance/iron-fist-stance" {
		t.Errorf("mixed martial arts stance = %q, want stance/iron-fist-stance", got)
	}
	if got := choices[features.ChoiceKey{FeatureSlug: stanceBlendingFeatureSlug, ChoiceIndex: 0}]; got != "stance/whirlwind-stance" {
		t.Errorf("stance blending stance = %q, want stance/whirlwind-stance", got)
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, `value="stance/iron-fist-stance" data-description="Your unarmed strikes deal extra damage." selected`) {
		t.Errorf("Mixed Martial Arts select should show its pick pre-selected:\n%s", after)
	}
	if !strings.Contains(after, `value="stance/whirlwind-stance" data-description="You gain extra reach on unarmed strikes." selected`) {
		t.Errorf("Stance Blending select should show its pick pre-selected:\n%s", after)
	}
}

// --- Passionate Flame ------------------------------------------------------

func seedPassionateFlameCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	seedTaijutsuStyleGroup(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/taijutsu-specialist/group/taijutsu-style/passionate-flame', 'class/taijutsu-specialist/group/taijutsu-style', 'Passionate Flame')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/taijutsu-specialist/group/taijutsu-style/passionate-flame', 'Fists of Iron', 3, 'Bonus damage.', 1)`,
		fistsOfIronFeatureSlug); err != nil {
		t.Fatal(err)
	}
	id := seedPlainTaijutsuCharacter(t, s, name, level)
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/taijutsu-specialist/group/taijutsu-style/passionate-flame', 3)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPassionateFlamePopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)
	seedTaijutsuStyleGroup(t, s)
	id := seedPlainTaijutsuCharacter(t, s, "Genin", 1)

	w := getPopup(t, s, passionateFlamePopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "a Passionate Flame Taijutsu Specialist") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestPassionateFlamePopupRenderSet(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)
	id := seedPassionateFlameCharacter(t, s, "Rock Lee", 9)
	popupPath := passionateFlamePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Fists of Iron: +2 Martial Dice bonus") {
		t.Errorf("initial render missing the Fists of Iron readout at 9th level:\n%s", body)
	}
	if !strings.Contains(body, "Mighty Blows") {
		t.Errorf("initial render missing the Hand Wraps of Passion options:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/hand-wraps-of-passion", url.Values{"value": {"mighty-guards"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("hand wraps of passion set: status %d, body %q", w.Code, w.Body.String())
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := choices[features.ChoiceKey{FeatureSlug: handWrapsOfPassionFeatureSlug, ChoiceIndex: 0}]; got != "mighty-guards" {
		t.Errorf("hand wraps of passion = %q, want mighty-guards", got)
	}
	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, `value="mighty-guards"`) || !strings.Contains(after, "selected") {
		t.Errorf("Hand Wraps of Passion select should show its pick pre-selected:\n%s", after)
	}
}

// --- Ruin --------------------------------------------------------------

func seedRuinCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	seedTaijutsuStyleGroup(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/taijutsu-specialist/group/taijutsu-style/ruin', 'class/taijutsu-specialist/group/taijutsu-style', 'Ruin')`); err != nil {
		t.Fatal(err)
	}
	id := seedPlainTaijutsuCharacter(t, s, name, level)
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/taijutsu-specialist/group/taijutsu-style/ruin', 3)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRuinPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)
	seedTaijutsuStyleGroup(t, s)
	id := seedPlainTaijutsuCharacter(t, s, "Genin", 1)

	w := getPopup(t, s, ruinPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "a Ruin Taijutsu Specialist") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestRuinPopupRenderSet(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)
	id := seedRuinCharacter(t, s, "Karin", 3)
	popupPath := ruinPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Sensory") || !strings.Contains(body, "Fuinjutsu") {
		t.Errorf("initial render missing the Anti-Chakra Wavelength keyword options:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/anti-chakra-wavelength", url.Values{"keyword1": {"Sensory"}, "keyword2": {"Fuinjutsu"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("anti-chakra wavelength set: status %d, body %q", w.Code, w.Body.String())
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := choices[features.ChoiceKey{FeatureSlug: antiChakraWavelengthFeatureSlug, ChoiceIndex: 0}]; got != "Sensory" {
		t.Errorf("keyword1 = %q, want Sensory", got)
	}
	if got := choices[features.ChoiceKey{FeatureSlug: antiChakraWavelengthFeatureSlug, ChoiceIndex: 1}]; got != "Fuinjutsu" {
		t.Errorf("keyword2 = %q, want Fuinjutsu", got)
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, `value="Sensory" selected`) || !strings.Contains(after, `value="Fuinjutsu" selected`) {
		t.Errorf("keyword selects should show both picks pre-selected:\n%s", after)
	}
}

// --- Sidebar buttons for Weapon Form / Stancer / Passionate Flame / Ruin --

// TestWeaponFormAndTaijutsuSubclassTrackerSidebarButtons covers character_
// sheet.html's own gating for these four new sidebar buttons, and confirms
// the old inline pickers they replaced are gone from the Core sheet — same
// two checks TestLastSubclassTrackerSidebarButtons already makes for the
// nine Science-Nin subclasses.
func TestWeaponFormAndTaijutsuSubclassTrackerSidebarButtons(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, s *server) int64
		popupPath   func(int64) string
		buttonText  string
		goneAddText string // text from the old inline picker form — must not leak into the Core sheet, since that picker is gone
	}{
		{"Weapon Form", func(t *testing.T, s *server) int64 {
			seedSlayerFormCatalog(t, s, 14, 4)
			return seedSlayerFormCharacter(t, s, "Zabuza", 14)
		}, weaponFormPopupPath, `tracked in the "Weapon Form" popup`, "Learn Style"},
		{"Stancer", func(t *testing.T, s *server) int64 {
			seedTaijutsuLevelResources(t, s)
			return seedStancerCharacter(t, s, "Ne", 9)
		}, stancerPopupPath, `tracked in the "Stancer" popup`, "Holding its benefit alongside your primary Stance"},
		{"Passionate Flame", func(t *testing.T, s *server) int64 {
			seedTaijutsuLevelResources(t, s)
			return seedPassionateFlameCharacter(t, s, "Rock Lee", 9)
		}, passionateFlamePopupPath, `tracked in the "Passionate Flame" popup`, "Choose an enhancement above"},
		{"Ruin", func(t *testing.T, s *server) int64 {
			seedTaijutsuLevelResources(t, s)
			return seedRuinCharacter(t, s, "Karin", 3)
		}, ruinPopupPath, `tracked in the "Ruin" popup`, "Choose 2 keywords"},
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
