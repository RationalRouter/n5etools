package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the "Hunter's Creed" subclass tracker popup
// (hunter_creed_popup.go, character_hunter_creed.html) — mirroring
// taijutsu_subclass_popups_test.go's own EmptyHint + render/round-trip
// shape, plus a sidebar-gating test. Unlike that file's four popups (one
// popup per subclass), Hunter's Creed is ONE popup covering all 8 Creeds,
// so the render/round-trip tests below each seed a different Creed and hit
// the same popup path — the popup itself decides which sections to show
// off the character's own subclass, same as weapon_form_popup_test.go's
// own multi-Form coverage. seedHunterNinLevelResources/
// seedBladeWardenSubclassFeatures (hunter_nin_test.go, same package) seed
// the shared base-class rows every case below builds on.

// seedArsenalistCharacter creates a level-10 Arsenalist Hunter-Nin. Callers
// must already have called seedHunterNinLevelResources for the shared
// class/class_levels rows.
func seedArsenalistCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/hunter-nin/group/hunters-creeds/arsenalist', 'class/hunter-nin/group/hunters-creeds', 'Arsenalist')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 14, 12, 10, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/hunter-nin', ?, 0)`, id, level); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/hunter-nin/group/hunters-creeds/arsenalist', 3)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedWolvesLegacyCharacter creates a level-10 Wolves Legacy Hunter-Nin (so
// both Prosthetic Attachments, 3rd level, and Wolf Techniques, 10th level,
// are open). Callers must already have called seedHunterNinLevelResources.
// wolfTechniqueOptions itself is a hand-curated Go catalog, not DB-backed
// (see its own doc comment), so no jutsu row needs seeding here — matches
// TestHunterNinWolfTechniqueGrantedJutsu's own precedent of picking a Wolf
// Technique with no backing jutsu row present.
func seedWolvesLegacyCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/hunter-nin/group/hunters-creeds/wolves-legacy', 'class/hunter-nin/group/hunters-creeds', 'Wolves Legacy')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 14, 12, 10, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/hunter-nin', ?, 0)`, id, level); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/hunter-nin/group/hunters-creeds/wolves-legacy', 3)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// --- Empty hints -----------------------------------------------------------

// TestHunterCreedPopupEmptyHintNoLevels covers a character with no
// Hunter-Nin levels at all.
func TestHunterCreedPopupEmptyHintNoLevels(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Genin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	w := getPopup(t, s, hunterCreedPopupPath(1))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "hasn&#39;t chosen a Hunter&#39;s Creed yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestHunterCreedPopupEmptyHintNoCreedChosen covers a real Hunter-Nin who
// hasn't picked a Creed yet (below 3rd level) — loadHunterTechniquesTabData
// returns non-nil (Lethal Attack alone), but CreedName stays "".
func TestHunterCreedPopupEmptyHintNoCreedChosen(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Genin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/hunter-nin', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	w := getPopup(t, s, hunterCreedPopupPath(1))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "hasn&#39;t chosen a Hunter&#39;s Creed yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// --- Grave Stalker (single-section Creed) -----------------------------------

func TestHunterCreedPopupRenderSetGraveStalker(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kagero', 10, 14, 12, 10, 12, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/hunter-nin', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (1, 'class/hunter-nin/group/hunters-creeds/grave-stalker', 3)`); err != nil {
		t.Fatal(err)
	}
	popupPath := hunterCreedPopupPath(1)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Shadow Assassination Technique") {
		t.Errorf("initial render missing the Shadow Assassination Technique section:\n%s", body)
	}
	if !strings.Contains(body, "Shadow Sword") {
		t.Errorf("initial render missing the Shadow Technique options:\n%s", body)
	}
	// Every other Creed's own section must stay absent.
	if strings.Contains(body, "Warden Weapon") || strings.Contains(body, "Arsenal (") || strings.Contains(body, "Wolf Techniques") {
		t.Errorf("Grave Stalker popup must not show another Creed's own section:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/shadow-technique/add", url.Values{"option_slug": {"shadow-sword"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("shadow technique add: status %d, body %q", w.Code, w.Body.String())
	}
	picks, err := charstore.ListHunterNinPicks(s.charDB, 1, charstore.HunterPickShadowTechnique)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0] != "shadow-sword" {
		t.Errorf("ListHunterNinPicks = %v, want [shadow-sword]", picks)
	}

	after := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(after, "Shadow Sword") || !strings.Contains(after, "1/2 chosen") {
		t.Errorf("popup should show the pick known and the used count updated:\n%s", after)
	}

	dw := postPopupForm(t, s, popupPath+"/shadow-technique/delete", url.Values{"option_slug": {"shadow-sword"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("shadow technique delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	picks, err = charstore.ListHunterNinPicks(s.charDB, 1, charstore.HunterPickShadowTechnique)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Errorf("ListHunterNinPicks after delete = %v, want none", picks)
	}
}

// --- Blade Warden (dual-section Creed) --------------------------------------

func TestHunterCreedPopupRenderSetBladeWarden(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	seedBladeWardenSubclassFeatures(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Zabuza', 16, 14, 12, 10, 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/hunter-nin', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (1, 'class/hunter-nin/group/hunters-creeds/blade-warden', 3)`); err != nil {
		t.Fatal(err)
	}
	popupPath := hunterCreedPopupPath(1)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Warden Weapon (") || !strings.Contains(body, "Warden Weapon Property (") {
		t.Errorf("initial render missing both Warden Weapon sections:\n%s", body)
	}
	if !strings.Contains(body, "Kunai") {
		t.Errorf("initial render missing the seeded Kunai option:\n%s", body)
	}
	if !strings.Contains(body, "Flurry Attack") {
		t.Errorf("initial render missing the Warden Weapon Property options:\n%s", body)
	}

	ww := postPopupForm(t, s, popupPath+"/warden-weapon/add", url.Values{"option_slug": {"weapon/kunai"}})
	if ww.Code != http.StatusSeeOther {
		t.Fatalf("warden weapon add: status %d, body %q", ww.Code, ww.Body.String())
	}
	wwp := postPopupForm(t, s, popupPath+"/warden-weapon-property/add", url.Values{"option_slug": {"flurry-attack"}})
	if wwp.Code != http.StatusSeeOther {
		t.Fatalf("warden weapon property add: status %d, body %q", wwp.Code, wwp.Body.String())
	}

	weaponPicks, err := charstore.ListHunterNinPicks(s.charDB, 1, charstore.HunterPickWardenWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if len(weaponPicks) != 1 || weaponPicks[0] != "weapon/kunai" {
		t.Errorf("warden weapon picks = %v, want [weapon/kunai]", weaponPicks)
	}
	propPicks, err := charstore.ListHunterNinPicks(s.charDB, 1, charstore.HunterPickWardenWeaponProperty)
	if err != nil {
		t.Fatal(err)
	}
	if len(propPicks) != 1 || propPicks[0] != "flurry-attack" {
		t.Errorf("warden weapon property picks = %v, want [flurry-attack]", propPicks)
	}

	dw := postPopupForm(t, s, popupPath+"/warden-weapon/delete", url.Values{"option_slug": {"weapon/kunai"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("warden weapon delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	weaponPicks, err = charstore.ListHunterNinPicks(s.charDB, 1, charstore.HunterPickWardenWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if len(weaponPicks) != 0 {
		t.Errorf("warden weapon picks after delete = %v, want none", weaponPicks)
	}
}

// --- Arsenalist (bespoke Arsenal Item add core) -----------------------------

func TestHunterCreedPopupArsenalItemAdd(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	id := seedArsenalistCharacter(t, s, "Kankuro", 10)
	popupPath := hunterCreedPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Arsenal (") {
		t.Errorf("initial render missing the Arsenal section:\n%s", body)
	}
	if !strings.Contains(body, "Paper Bombs") {
		t.Errorf("initial render missing the Arsenal options:\n%s", body)
	}

	w := postPopupForm(t, s, popupPath+"/arsenal-item/add", url.Values{"option_slug": {"paper-bombs"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("arsenal item add: status %d, body %q", w.Code, w.Body.String())
	}
	picks, err := charstore.ListHunterNinPicks(s.charDB, id, charstore.HunterPickArsenalItem)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0] != "paper-bombs" {
		t.Errorf("arsenal item picks = %v, want [paper-bombs]", picks)
	}

	dw := postPopupForm(t, s, popupPath+"/arsenal-item/delete", url.Values{"option_slug": {"paper-bombs"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("arsenal item delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	picks, err = charstore.ListHunterNinPicks(s.charDB, id, charstore.HunterPickArsenalItem)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Errorf("arsenal item picks after delete = %v, want none", picks)
	}
}

// --- Wolves Legacy (dual-section, bespoke Wolf Technique add core) ---------

func TestHunterCreedPopupWolvesLegacyAdd(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	id := seedWolvesLegacyCharacter(t, s, "Kidomaru", 10)
	popupPath := hunterCreedPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, "Prosthetic Attachments (") || !strings.Contains(body, "Wolf Techniques (") {
		t.Errorf("initial render missing both Wolves Legacy sections:\n%s", body)
	}
	if !strings.Contains(body, "Sabimaru") {
		t.Errorf("initial render missing the Prosthetic Attachment options:\n%s", body)
	}
	if !strings.Contains(body, "Weapon Deflect") {
		t.Errorf("initial render missing the Wolf Technique options:\n%s", body)
	}
	if !strings.Contains(body, "Special Resources on the Core tab") {
		t.Errorf("Prosthetic Attachments section missing its Core-tab cross-reference hint:\n%s", body)
	}

	pa := postPopupForm(t, s, popupPath+"/prosthetic-attachment/add", url.Values{"option_slug": {"sabimaru"}})
	if pa.Code != http.StatusSeeOther {
		t.Fatalf("prosthetic attachment add: status %d, body %q", pa.Code, pa.Body.String())
	}
	wt := postPopupForm(t, s, popupPath+"/wolf-technique/add", url.Values{"option_slug": {"jutsu/weapon-deflect"}})
	if wt.Code != http.StatusSeeOther {
		t.Fatalf("wolf technique add: status %d, body %q", wt.Code, wt.Body.String())
	}

	paPicks, err := charstore.ListHunterNinPicks(s.charDB, id, charstore.HunterPickProstheticAttachment)
	if err != nil {
		t.Fatal(err)
	}
	if len(paPicks) != 1 || paPicks[0] != "sabimaru" {
		t.Errorf("prosthetic attachment picks = %v, want [sabimaru]", paPicks)
	}
	wtPicks, err := charstore.ListHunterNinPicks(s.charDB, id, charstore.HunterPickWolfTechnique)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtPicks) != 1 || wtPicks[0] != "jutsu/weapon-deflect" {
		t.Errorf("wolf technique picks = %v, want [jutsu/weapon-deflect]", wtPicks)
	}

	dw := postPopupForm(t, s, popupPath+"/wolf-technique/delete", url.Values{"option_slug": {"jutsu/weapon-deflect"}})
	if dw.Code != http.StatusSeeOther {
		t.Fatalf("wolf technique delete: status %d, body %q", dw.Code, dw.Body.String())
	}
	wtPicks, err = charstore.ListHunterNinPicks(s.charDB, id, charstore.HunterPickWolfTechnique)
	if err != nil {
		t.Fatal(err)
	}
	if len(wtPicks) != 0 {
		t.Errorf("wolf technique picks after delete = %v, want none", wtPicks)
	}
}

// --- Sidebar button ----------------------------------------------------

// TestHunterCreedSidebarButton covers character_sheet.html's own gating for
// the single "Hunter's Creed" sidebar button (shared by all 8 Creeds), and
// confirms the old inline picker text it replaced is gone from the Core
// sheet — same two checks TestWeaponFormAndTaijutsuSubclassTrackerSidebarButtons
// already makes for Weapon Form/Stancer/Passionate Flame/Ruin.
func TestHunterCreedSidebarButton(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, s *server) int64
		goneAddText string // text from the old inline picker form — must not leak into the Core sheet
	}{
		{"Grave Stalker", func(t *testing.T, s *server) int64 {
			seedHunterNinLevelResources(t, s)
			res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
				VALUES ('Kagero', 10, 14, 12, 10, 12, 10)`)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := res.LastInsertId()
			if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/hunter-nin', 10, 0)`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/hunter-nin/group/hunters-creeds/grave-stalker', 3)`, id); err != nil {
				t.Fatal(err)
			}
			return id
		}, "Learn Technique"},
		{"Wolves Legacy", func(t *testing.T, s *server) int64 {
			seedHunterNinLevelResources(t, s)
			return seedWolvesLegacyCharacter(t, s, "Kidomaru", 10)
		}, "Spent from your Prosthetic Attachments pool"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			id := c.seed(t, s)
			idStr := strconv.FormatInt(id, 10)
			sheetBody := getPopup(t, s, "/characters/"+idStr).Body.String()
			wantHref := "/characters/" + idStr + "/hunter-nin/creed"
			if !strings.Contains(sheetBody, `href="`+wantHref+`"`) {
				t.Errorf("Core sheet missing its own sidebar button (href=%q):\n%s", wantHref, sheetBody)
			}
			if !strings.Contains(sheetBody, `tracked in the "Hunter's Creed" popup`) {
				t.Errorf("Core sheet missing pointer text:\n%s", sheetBody)
			}
			if strings.Contains(sheetBody, c.goneAddText) {
				t.Errorf("Core sheet still contains inline picker text %q; it should only point at the popup now", c.goneAddText)
			}
		})
	}
}
