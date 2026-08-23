package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the "Scouting Technique" subclass tracker popup
// (scout_nin_scouting_technique_popup.go, character_scout_nin_scouting_
// technique.html) — mirroring hunter_creed_popup_test.go's own EmptyHint +
// render/round-trip + sidebar-gating shape. Like Hunter's Creed, this is
// ONE popup covering all 9 Scouting Techniques, so the render/round-trip
// tests below each seed a different Technique and hit the same popup path
// — the popup itself decides which sections to show off the character's
// own subclass.

// seedScoutNinLevelResources seeds the shared class/subclass_groups rows
// every case below builds on — mirrors seedHunterNinLevelResources
// (hunter_nin_test.go) exactly.
func seedScoutNinLevelResources(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/scout-nin', 'Scout-Nin', 8, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/scout-nin/group/scouting-technique', 'class/scout-nin', 'Scouting Technique')`); err != nil {
		t.Fatal(err)
	}
}

// seedScoutNinCharacter creates a character with the given class level and
// (if non-empty) Scouting Technique subclass. Callers must already have
// called seedScoutNinLevelResources and inserted their own "subclasses" row
// for subclassSlug.
func seedScoutNinCharacter(t *testing.T, s *server, name, subclassSlug string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 14, 12, 10, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/scout-nin', ?, 0)`, id, level); err != nil {
		t.Fatal(err)
	}
	if subclassSlug != "" {
		if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, ?, 3)`, id, subclassSlug); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// --- Empty hints -------------------------------------------------------

func TestScoutingTechniquePopupEmptyHintNoLevels(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Genin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	w := getPopup(t, s, scoutingTechniquePopupPath(1))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "hasn&#39;t chosen a Scouting Technique yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestScoutingTechniquePopupEmptyHintNoTechniqueChosen covers a real
// Scout-Nin below 3rd level (loadScoutNinTabData returns non-nil, but
// TechniqueName stays "").
func TestScoutingTechniquePopupEmptyHintNoTechniqueChosen(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	id := seedScoutNinCharacter(t, s, "Genin", "", 1)

	w := getPopup(t, s, scoutingTechniquePopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "hasn&#39;t chosen a Scouting Technique yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// --- Assault Scout (Maneuvers-only Technique) ---------------------------

// TestScoutingTechniquePopupAssaultScoutManeuvers covers the umbrella
// Maneuvers Known section shared by all 9 Techniques, against a subclass
// with no OTHER subclass-exclusive pick at all (assault-scout — confirmed
// against scout_nin.go, no Cap function switches on this slug beyond
// scoutNinManeuversKnownCap itself).
func TestScoutingTechniquePopupAssaultScoutManeuvers(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/scout-nin/group/scouting-technique/assault-scout', 'class/scout-nin/group/scouting-technique', 'Assault Scout')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/scout-nin/option/assault-maneuvers/power-attack', 'class/scout-nin', 'class/scout-nin/group/scouting-technique/assault-scout',
		 'Assault Maneuvers', 'Power Attack', 'Deal extra damage on a hit.', 1)`); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Gaara", assaultScoutSubclassSlug, 3)
	popupPath := scoutingTechniquePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Assault Maneuvers (") {
		t.Errorf("initial render missing the Maneuvers Known section:\n%s", body)
	}
	if !strings.Contains(body, "Power Attack") {
		t.Errorf("initial render missing the seeded Maneuver option:\n%s", body)
	}
	// No other Technique's own section may leak in.
	if strings.Contains(body, "Tactical Superiority") || strings.Contains(body, "Signature Maneuver") ||
		strings.Contains(body, "Supreme Clones") || strings.Contains(body, "Mobile Savant") {
		t.Errorf("Assault Scout popup must not show another Technique's own section:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/maneuvers/add", url.Values{"option_slug": {"class/scout-nin/option/assault-maneuvers/power-attack"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("maneuvers add: status %d, body %q", w.Code, w.Body.String())
	}
	picks, err := charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickManeuvers)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0] != "class/scout-nin/option/assault-maneuvers/power-attack" {
		t.Errorf("maneuvers picks = %v, want [power-attack]", picks)
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, "1/3 known") {
		t.Errorf("popup should show the used count updated (cap 3 at 3rd level):\n%s", after)
	}

	dw := postPopupForm(t, s, popupPath+"/maneuvers/delete", url.Values{"option_slug": {"class/scout-nin/option/assault-maneuvers/power-attack"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("maneuvers delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	picks, err = charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickManeuvers)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Errorf("maneuvers picks after delete = %v, want none", picks)
	}
}

// --- Tactical Scout (Maneuvers + Tactical Superiority + Signature Maneuver) --

// TestScoutingTechniquePopupTacticalScout covers all three of Tactical
// Scout's own sections in a single popup render — Signature Maneuver's own
// candidates are drawn from the character's already-known "Keyword:
// Tactical" Maneuvers (so it starts empty until one is learned via the
// Maneuvers section first), while Tactical Superiority's candidates are
// drawn from OTHER subclasses' own catalogs (loadScoutNinManeuverCatalogForSubclasses)
// independent of what the character already knows.
func TestScoutingTechniquePopupTacticalScout(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/scout-nin/group/scouting-technique/tactical-scout', 'class/scout-nin/group/scouting-technique', 'Tactical Scout'),
		('class/scout-nin/group/scouting-technique/assault-scout', 'class/scout-nin/group/scouting-technique', 'Assault Scout')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/scout-nin/option/tactical-maneuvers/feint', 'class/scout-nin', 'class/scout-nin/group/scouting-technique/tactical-scout',
		 'Tactical Maneuvers', 'Feint', 'Keyword: Tactical. Trick a foe into leaving an opening.', 1),
		('class/scout-nin/option/tactical-maneuvers/rally', 'class/scout-nin', 'class/scout-nin/group/scouting-technique/tactical-scout',
		 'Tactical Maneuvers', 'Rally', 'Bolster an ally, no keyword.', 2),
		('class/scout-nin/option/assault-maneuvers/power-attack', 'class/scout-nin', 'class/scout-nin/group/scouting-technique/assault-scout',
		 'Assault Maneuvers', 'Power Attack', 'Deal extra damage on a hit.', 1)`); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Shikamaru", tacticalScoutSubclassSlug, 20)
	popupPath := scoutingTechniquePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Tactical Maneuvers (") {
		t.Errorf("initial render missing the Maneuvers Known section:\n%s", body)
	}
	if !strings.Contains(body, "Feint") || !strings.Contains(body, "Rally") {
		t.Errorf("initial render missing both Maneuvers options:\n%s", body)
	}
	if !strings.Contains(body, "Tactical Superiority (") {
		t.Errorf("initial render missing the Tactical Superiority section:\n%s", body)
	}
	if !strings.Contains(body, "Power Attack") {
		t.Errorf("initial render missing Tactical Superiority's cross-subclass option:\n%s", body)
	}
	if !strings.Contains(body, "Signature Maneuver (") {
		t.Errorf("initial render missing the Signature Maneuver section:\n%s", body)
	}
	if strings.Contains(body, "Supreme Clones") || strings.Contains(body, "Mobile Savant") {
		t.Errorf("Tactical Scout popup must not show another Technique's own section:\n%s", body)
	}

	// Learn Feint (the Keyword: Tactical Maneuver) first — Signature
	// Maneuver's own candidates are drawn from already-known Maneuvers.
	mw := postPopupForm(t, s, popupPath+"/maneuvers/add", url.Values{"option_slug": {"class/scout-nin/option/tactical-maneuvers/feint"}})
	if mw.Code != http.StatusSeeOther {
		t.Fatalf("maneuvers add: status %d, body %q", mw.Code, mw.Body.String())
	}

	sw := postPopupForm(t, s, popupPath+"/signature-maneuver/add", url.Values{"option_slug": {"class/scout-nin/option/tactical-maneuvers/feint"}})
	if sw.Code != http.StatusSeeOther {
		t.Fatalf("signature maneuver add: status %d, body %q", sw.Code, sw.Body.String())
	}
	sigPicks, err := charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickSignatureManeuver)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigPicks) != 1 || sigPicks[0] != "class/scout-nin/option/tactical-maneuvers/feint" {
		t.Errorf("signature maneuver picks = %v, want [feint]", sigPicks)
	}

	tw := postPopupForm(t, s, popupPath+"/tactical-superiority/add", url.Values{"option_slug": {"class/scout-nin/option/assault-maneuvers/power-attack"}})
	if tw.Code != http.StatusSeeOther {
		t.Fatalf("tactical superiority add: status %d, body %q", tw.Code, tw.Body.String())
	}
	tsPicks, err := charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickTacticalSuperiority)
	if err != nil {
		t.Fatal(err)
	}
	if len(tsPicks) != 1 || tsPicks[0] != "class/scout-nin/option/assault-maneuvers/power-attack" {
		t.Errorf("tactical superiority picks = %v, want [power-attack]", tsPicks)
	}

	dw := postPopupForm(t, s, popupPath+"/tactical-superiority/delete", url.Values{"option_slug": {"class/scout-nin/option/assault-maneuvers/power-attack"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("tactical superiority delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	tsPicks, err = charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickTacticalSuperiority)
	if err != nil {
		t.Fatal(err)
	}
	if len(tsPicks) != 0 {
		t.Errorf("tactical superiority picks after delete = %v, want none", tsPicks)
	}
}

// --- Cloning Scout (Maneuvers + Supreme Clones) -------------------------

func TestScoutingTechniquePopupCloningScoutSupremeClones(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/scout-nin/group/scouting-technique/cloning-scout', 'class/scout-nin/group/scouting-technique', 'Cloning Scout')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/scout-nin/group/scouting-technique/cloning-scout', 'Supreme Clones', 20,
		 'Mark a Maneuver your clones can use for free once per turn.', 5)`,
		supremeClonesFeatureSlug); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/scout-nin/option/cloning-maneuvers/split-strike', 'class/scout-nin', 'class/scout-nin/group/scouting-technique/cloning-scout',
		 'Cloning Maneuvers', 'Split Strike', 'Coordinate an attack with a clone.', 1)`); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Naruto", cloningScoutSubclassSlug, 20)
	popupPath := scoutingTechniquePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Cloning Maneuvers (") {
		t.Errorf("initial render missing the Maneuvers Known section:\n%s", body)
	}
	if !strings.Contains(body, "Supreme Clones (") {
		t.Errorf("initial render missing the Supreme Clones section:\n%s", body)
	}
	if !strings.Contains(body, "clones can perform the marked Maneuver") {
		t.Errorf("Supreme Clones section missing its own hint text:\n%s", body)
	}

	mw := postPopupForm(t, s, popupPath+"/maneuvers/add", url.Values{"option_slug": {"class/scout-nin/option/cloning-maneuvers/split-strike"}})
	if mw.Code != http.StatusSeeOther {
		t.Fatalf("maneuvers add: status %d, body %q", mw.Code, mw.Body.String())
	}
	scw := postPopupForm(t, s, popupPath+"/supreme-clones/add", url.Values{"option_slug": {"class/scout-nin/option/cloning-maneuvers/split-strike"}})
	if scw.Code != http.StatusSeeOther {
		t.Fatalf("supreme clones add: status %d, body %q", scw.Code, scw.Body.String())
	}
	scPicks, err := charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickSupremeClones)
	if err != nil {
		t.Fatal(err)
	}
	if len(scPicks) != 1 || scPicks[0] != "class/scout-nin/option/cloning-maneuvers/split-strike" {
		t.Errorf("supreme clones picks = %v, want [split-strike]", scPicks)
	}

	dw := postPopupForm(t, s, popupPath+"/supreme-clones/delete", url.Values{"option_slug": {"class/scout-nin/option/cloning-maneuvers/split-strike"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("supreme clones delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	scPicks, err = charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickSupremeClones)
	if err != nil {
		t.Fatal(err)
	}
	if len(scPicks) != 0 {
		t.Errorf("supreme clones picks after delete = %v, want none", scPicks)
	}
}

// --- Pathfinder Scout (Maneuvers + Mobile Savant) -----------------------

func TestScoutingTechniquePopupPathfinderScoutMobileSavant(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/scout-nin/group/scouting-technique/pathfinder-scout', 'class/scout-nin/group/scouting-technique', 'Pathfinder Scout')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/scout-nin/option/pathfinder-maneuvers/trailblazer', 'class/scout-nin', 'class/scout-nin/group/scouting-technique/pathfinder-scout',
		 'Pathfinder Maneuvers', 'Trailblazer', 'Move ahead of the group.', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/wind-release-zephyr-strike', 'Zephyr Strike', 'Ninjutsu', 'C', '1 Action', '30 feet', 'Instant', 'HS', 'Cost: 3 Chakra', 'Wind', 'test jutsu')`,
	); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Kiba", pathfinderScoutSubclassSlug, 3)
	popupPath := scoutingTechniquePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Pathfinder Maneuvers (") {
		t.Errorf("initial render missing the Maneuvers Known section:\n%s", body)
	}
	if !strings.Contains(body, "Mobile Savant (") {
		t.Errorf("initial render missing the Mobile Savant section:\n%s", body)
	}
	if !strings.Contains(body, "Zephyr Strike") {
		t.Errorf("initial render missing the Mobile Savant jutsu option:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/mobile-savant/add", url.Values{"option_slug": {"jutsu/wind-release-zephyr-strike"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("mobile savant add: status %d, body %q", w.Code, w.Body.String())
	}
	picks, err := charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickMobileSavant)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0] != "jutsu/wind-release-zephyr-strike" {
		t.Errorf("mobile savant picks = %v, want [zephyr-strike]", picks)
	}

	dw := postPopupForm(t, s, popupPath+"/mobile-savant/delete", url.Values{"option_slug": {"jutsu/wind-release-zephyr-strike"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("mobile savant delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	picks, err = charstore.ListScoutNinPicks(s.charDB, id, charstore.ScoutNinPickMobileSavant)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Errorf("mobile savant picks after delete = %v, want none", picks)
	}
}

// --- Trickster Scout (Change of Heart, a freely re-editable <select>) --

func TestScoutingTechniquePopupTricksterScoutChangeOfHeart(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/scout-nin/group/scouting-technique/trickster-scout', 'class/scout-nin/group/scouting-technique', 'Trickster Scout')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/scout-nin/group/scouting-technique/trickster-scout', 'Change of Heart', 17,
		 'Choose Messiah, Izanagi, or Satanael.', 6)`,
		changeOfHeartFeatureSlug); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Sasuke", tricksterScoutSubclassSlug, 18)
	popupPath := scoutingTechniquePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Change of Heart") {
		t.Errorf("initial render missing the Change of Heart section:\n%s", body)
	}
	if !strings.Contains(body, "Izanagi") {
		t.Errorf("initial render missing Change of Heart's own options:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/change-of-heart", url.Values{"value": {"izanagi"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("change of heart set: status %d, body %q", w.Code, w.Body.String())
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, `value="izanagi" data-description="Izanagi`) && !strings.Contains(after, `value="izanagi"`) {
		t.Errorf("popup should still show the Izanagi option after setting it:\n%s", after)
	}
	if !strings.Contains(after, "selected") {
		t.Errorf("popup should mark the chosen option selected:\n%s", after)
	}
}

// --- Arbiter Scout (Paragon's Presence, a freely re-editable <select>) --

func TestScoutingTechniquePopupArbiterScoutParagonsPresence(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/scout-nin/group/scouting-technique/arbiter-scout', 'class/scout-nin/group/scouting-technique', 'Arbiter Scout')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/scout-nin/group/scouting-technique/arbiter-scout', 'Paragon''s Presence', 14,
		 'Choose Berserk, Charmed, or Frightened.', 4)`,
		paragonsPresenceFeatureSlug); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Killer Bee", arbiterScoutSubclassSlug, 14)
	popupPath := scoutingTechniquePopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Paragon's Presence") {
		t.Errorf("initial render missing the Paragon's Presence section:\n%s", body)
	}
	if !strings.Contains(body, "Frightened") {
		t.Errorf("initial render missing Paragon's Presence's own options:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/paragons-presence", url.Values{"value": {"berserk"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("paragons presence set: status %d, body %q", w.Code, w.Body.String())
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, "selected") {
		t.Errorf("popup should mark the chosen option selected:\n%s", after)
	}
}

// --- Sidebar button + inline pointer text --------------------------------

// TestScoutingTechniqueSidebarButton covers character_sheet.html's own
// gating for the single "Scouting Technique" sidebar button (shared by all
// 9 Techniques), and confirms the old inline picker text it replaced is
// gone from the Core sheet — same two checks TestHunterCreedSidebarButton
// already makes for Hunter's Creed.
func TestScoutingTechniqueSidebarButton(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, s *server) int64
		goneAddText string // text from the old inline picker form — must not leak into the Core sheet
	}{
		{"Assault Scout", func(t *testing.T, s *server) int64 {
			seedScoutNinLevelResources(t, s)
			if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
				('class/scout-nin/group/scouting-technique/assault-scout', 'class/scout-nin/group/scouting-technique', 'Assault Scout')`); err != nil {
				t.Fatal(err)
			}
			return seedScoutNinCharacter(t, s, "Gaara", assaultScoutSubclassSlug, 3)
		}, "Learn Maneuver"},
		{"Cloning Scout", func(t *testing.T, s *server) int64 {
			seedScoutNinLevelResources(t, s)
			if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
				('class/scout-nin/group/scouting-technique/cloning-scout', 'class/scout-nin/group/scouting-technique', 'Cloning Scout')`); err != nil {
				t.Fatal(err)
			}
			return seedScoutNinCharacter(t, s, "Naruto", cloningScoutSubclassSlug, 20)
		}, "Mark Supreme Clone"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			id := c.seed(t, s)
			idStr := strconv.FormatInt(id, 10)
			sheetBody := getPopup(t, s, "/characters/"+idStr).Body.String()
			wantHref := "/characters/" + idStr + "/scout-nin/scouting-technique"
			if !strings.Contains(sheetBody, `href="`+wantHref+`"`) {
				t.Errorf("Core sheet missing its own sidebar button (href=%q):\n%s", wantHref, sheetBody)
			}
			if !strings.Contains(sheetBody, `tracked in the "Scouting Technique" popup`) {
				t.Errorf("Core sheet missing pointer text:\n%s", sheetBody)
			}
			if strings.Contains(sheetBody, c.goneAddText) {
				t.Errorf("Core sheet still contains inline picker text %q; it should only point at the popup now", c.goneAddText)
			}
		})
	}
}

// TestScoutingTechniqueBaseClassPicksStayInline confirms Shinobi Adept and
// Jack of All, Master of None still render directly on the Core sheet even
// with NO Scouting Technique chosen yet — both take no subclassSlug
// parameter of their own (jackOfAllCap/shinobiAdeptCap, scout_nin.go), so
// they must not have been swept into the Scouting Technique popup along
// with the genuinely subclass-exclusive picks. Also confirms the sidebar
// button/pointer text stay absent, since TechniqueName is still "".
func TestScoutingTechniqueBaseClassPicksStayInline(t *testing.T) {
	s := testServer(t)
	seedScoutNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/scout-nin/feature/combat', 'class/scout-nin', 'Combat', 5, 'A generic Generalization option.'),
		('class/scout-nin/feature/shinobis-tactics', 'class/scout-nin', 'Shinobi''s Tactics', 2, 'A generic Shinobi Adept option.')`); err != nil {
		t.Fatal(err)
	}
	id := seedScoutNinCharacter(t, s, "Genin", "" /* no Scouting Technique chosen */, 5)
	idStr := strconv.FormatInt(id, 10)

	sheetBody := getPopup(t, s, "/characters/"+idStr).Body.String()
	if !strings.Contains(sheetBody, "Jack of All, Master of None") {
		t.Errorf("Core sheet missing Jack of All despite no subclass chosen (base-class feature):\n%s", sheetBody)
	}
	if !strings.Contains(sheetBody, "Shinobi Adept") {
		t.Errorf("Core sheet missing Shinobi Adept despite no subclass chosen (base-class feature):\n%s", sheetBody)
	}
	wantHref := "/characters/" + idStr + "/scout-nin/scouting-technique"
	if strings.Contains(sheetBody, `href="`+wantHref+`"`) {
		t.Errorf("Core sheet should not show the Scouting Technique sidebar button with no Technique chosen:\n%s", sheetBody)
	}
	if strings.Contains(sheetBody, `tracked in the "Scouting Technique" popup`) {
		t.Errorf("Core sheet should not show the popup pointer text with no Technique chosen:\n%s", sheetBody)
	}
}
