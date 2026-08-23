package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the two subclass tracker popups converted from
// Genjutsu Specialist's own subclass-gated Core-sheet sections: "Twisted
// Casting" (Beguiler only, 10th level) and "Psyche Breaker" (Corrupt
// Thoughts only, 10th level) — mirroring subclass_tracker_popup_new_test.
// go's own Awakened Scroll EmptyHint + render/round-trip + sidebar-gating
// shape, since both of these popups share Awakened Scroll's own hand-rolled
// "jutsu_id" picker shape (sourced from the character's own known Genjutsu,
// not a rules-database catalog). getPopup/postPopupForm/knownJutsuIDFromBody
// are shared helpers already defined in subclass_tracker_popup_test.go/
// subclass_tracker_popup_new_test.go.

// seedGenjutsuPledgesCatalog seeds enough rules.db content for both
// Twisted Casting (Beguiler) and Psyche Breaker (Corrupt Thoughts) to
// render: the class itself and its Genjutsu Pledges subclass chain —
// twistedCastingCap/psycheBreakerCap are both hand-curated by level, no
// class_level_resources chart needed.
func seedGenjutsuPledgesCatalog(t *testing.T, s *server) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/genjutsu-specialist', 'Genjutsu Specialist', 8, 8)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges', 'class/genjutsu-specialist', 'Genjutsu Pledges')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges/beguiler', 'class/genjutsu-specialist/group/genjutsu-pledges', 'Beguiler'),
		('class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts', 'class/genjutsu-specialist/group/genjutsu-pledges', 'Corrupt Thoughts')`)
	mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/test-genjutsu', 'Test Genjutsu', 'Genjutsu', 'C', '1 Action', '30 ft', '1 Minute', 'HS', 'Cost: 2', 'Genjutsu, Visual',
		        'A test genjutsu.')`)
}

// seedGenjutsuPledgeCharacter inserts a character with the given Genjutsu
// Specialist level, already 2nd-level-subclassed into the given Pledge,
// plus one known Genjutsu for Twisted Casting/Psyche Breaker's own picker
// to offer.
func seedGenjutsuPledgeCharacter(t *testing.T, s *server, name, pledgeSlug string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 10, 10, 16)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/genjutsu-specialist', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, ?, 2)`,
		id, pledgeSlug,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (?, 'jutsu/test-genjutsu')`, id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedPlainGenjutsuCharacter inserts a plain Genjutsu Specialist character
// with no subclass chosen — used by the EmptyHint tests, which only need
// the base class seeded without triggering either Pledge's own gate.
func seedPlainGenjutsuCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 10, 10, 16)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/genjutsu-specialist', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// --- Twisted Casting -------------------------------------------------------

func TestTwistedCastingPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedGenjutsuPledgesCatalog(t, s)
	id := seedPlainGenjutsuCharacter(t, s, "Ino", 1) // no subclass -> Cap 0

	w := getPopup(t, s, twistedCastingPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "10th-level (or higher) Beguiler Genjutsu Specialist") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestTwistedCastingPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedGenjutsuPledgesCatalog(t, s)
	id := seedGenjutsuPledgeCharacter(t, s, "Ino", "class/genjutsu-specialist/group/genjutsu-pledges/beguiler", 10)
	popupPath := twistedCastingPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Genjutsu") {
		t.Errorf("initial render missing the available known Genjutsu option:\n%s", body)
	}

	jutsuIDStr := knownJutsuIDFromBody(t, body, "Test Genjutsu")

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"jutsu_id": {jutsuIDStr}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListGenjutsuJutsuPicks(s.charDB, id, charstore.GenjutsuPickTwistedCasting)
	if err != nil {
		t.Fatal(err)
	}
	jutsuID, _ := strconv.ParseInt(jutsuIDStr, 10, 64)
	if len(picks) != 1 || picks[0] != jutsuID {
		t.Fatalf("picks after add = %+v, want one row for jutsu id %d", picks, jutsuID)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Genjutsu") {
		t.Errorf("popup after add missing Test Genjutsu in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"jutsu_id": {jutsuIDStr}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListGenjutsuJutsuPicks(s.charDB, id, charstore.GenjutsuPickTwistedCasting)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Psyche Breaker ----------------------------------------------------

func TestPsycheBreakerPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedGenjutsuPledgesCatalog(t, s)
	id := seedPlainGenjutsuCharacter(t, s, "Sasuke", 1) // no subclass -> Cap 0

	w := getPopup(t, s, psycheBreakerPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "10th-level (or higher) Corrupt Thoughts Genjutsu Specialist") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestPsycheBreakerPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedGenjutsuPledgesCatalog(t, s)
	id := seedGenjutsuPledgeCharacter(t, s, "Sasuke", "class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts", 10)
	popupPath := psycheBreakerPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Genjutsu") {
		t.Errorf("initial render missing the available known Genjutsu option:\n%s", body)
	}

	jutsuIDStr := knownJutsuIDFromBody(t, body, "Test Genjutsu")

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"jutsu_id": {jutsuIDStr}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListGenjutsuJutsuPicks(s.charDB, id, charstore.GenjutsuPickPsycheBreaker)
	if err != nil {
		t.Fatal(err)
	}
	jutsuID, _ := strconv.ParseInt(jutsuIDStr, 10, 64)
	if len(picks) != 1 || picks[0] != jutsuID {
		t.Fatalf("picks after add = %+v, want one row for jutsu id %d", picks, jutsuID)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Genjutsu") {
		t.Errorf("popup after add missing Test Genjutsu in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"jutsu_id": {jutsuIDStr}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListGenjutsuJutsuPicks(s.charDB, id, charstore.GenjutsuPickPsycheBreaker)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Sidebar buttons for both new popups --------------------------------

// TestTwistedCastingPsycheBreakerSidebarButtons covers character_sheet.
// html's own gating for the 2 sidebar buttons this conversion added, and
// confirms the old inline pickers/routes they replaced are gone from the
// Core sheet — the same two checks
// TestOperativeTrapsAwakenedScrollCombatMedicSidebarButtons already makes
// for its own 3 popups. Twisted Casting/Psyche Breaker both still keep
// their original Core-sheet POST routes alive (/sheet/genjutsu-twisted-
// casting, /sheet/genjutsu-psyche-breaker — see genjutsu_twisted_casting_
// popup.go's own header doc), so goneAddText checks the removed inline
// <select>-plus-button markup itself rather than a removed route.
func TestTwistedCastingPsycheBreakerSidebarButtons(t *testing.T) {
	cases := []struct {
		name       string
		seed       func(t *testing.T, s *server) int64
		popupPath  func(int64) string
		buttonText string
	}{
		{"Twisted Casting", func(t *testing.T, s *server) int64 {
			seedGenjutsuPledgesCatalog(t, s)
			return seedGenjutsuPledgeCharacter(t, s, "Ino", "class/genjutsu-specialist/group/genjutsu-pledges/beguiler", 10)
		}, twistedCastingPopupPath, `tracked in the "Twisted Casting" popup`},
		{"Psyche Breaker", func(t *testing.T, s *server) int64 {
			seedGenjutsuPledgesCatalog(t, s)
			return seedGenjutsuPledgeCharacter(t, s, "Sasuke", "class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts", 10)
		}, psycheBreakerPopupPath, `tracked in the "Psyche Breaker" popup`},
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
