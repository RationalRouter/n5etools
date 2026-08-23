package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// seedKitChoiceFixtures installs a Hunter-Nin-shaped class — a fixed tool
// grant ("Forgery Kit", auto-proficiency, the same shape as the class's own
// other fixed proficiencies) plus a "2 Kits of your Choice" starting
// equipment option — and three real toolkits. This is the minimal fixture
// reproducing the reported bug: the Starting Equipment step's own kit
// dropdowns offered a toolkit the character already had proficiency with
// from the class itself, and let the same toolkit be picked twice across
// the one grant's own two slots.
func seedKitChoiceFixtures(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/hunter-nin', 'Hunter-Nin', 8, 8)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_proficiencies (class_slug, kind, value) VALUES
		 ('class/hunter-nin', 'tool', 'Forgery Kit')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_equipment_options (class_slug, group_idx, choice_idx, description, quantity) VALUES
		 ('class/hunter-nin', 0, 0, '2 Kits of your Choice', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT OR IGNORE INTO equipment (slug, name, kind) VALUES
		 ('toolkit/forgery-kit', 'Forgery Kit', 'toolkit'),
		 ('toolkit/poison-kit', 'Poison Kit', 'toolkit'),
		 ('toolkit/trappers-kit', 'Trappers Kit', 'toolkit')`); err != nil {
		t.Fatal(err)
	}
}

// newKitChoiceCharacter creates a bare character and gives it the
// Hunter-Nin class via the real class-step handler, so the class's fixed
// "Forgery Kit" tool proficiency lands in character_proficiencies exactly
// the way a real playthrough would produce it.
func newKitChoiceCharacter(t *testing.T, s *server, name string) string {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES (?)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	form := url.Values{"class_slug": {"class/hunter-nin"}}
	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("class step POST status = %d, body:\n%s", w.Code, w.Body.String())
	}
	return idStr
}

// TestEquipmentKitChoiceExcludesClassGrantedToolkit is the reported bug's
// render-time half: the equipment step's "2 Kits of your Choice" dropdowns
// must not offer Forgery Kit, which Hunter-Nin already grants as a fixed
// tool proficiency, while still offering toolkits the character has no
// proficiency in at all.
func TestEquipmentKitChoiceExcludesClassGrantedToolkit(t *testing.T) {
	s := testServer(t)
	seedKitChoiceFixtures(t, s)
	idStr := newKitChoiceCharacter(t, s, "Kit Choice Render Test")

	req := httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/create/equipment", nil)
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateEquipment(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("equipment step GET status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `value="toolkit/forgery-kit"`) {
		t.Error("equipment step still offers Forgery Kit, already granted by the class itself")
	}
	for _, want := range []string{"toolkit/poison-kit", "toolkit/trappers-kit"} {
		if !strings.Contains(body, `value="`+want+`"`) {
			t.Errorf("equipment step wrongly excluded %q, which the character has no proficiency in", want)
		}
	}
}

// TestEquipmentKitChoiceRejectsHeldToolkit is the submit-time defense in
// depth: a hand-built POST naming the already-held toolkit directly
// (bypassing both the dropdown's own render-time filtering and
// create-equipment.js's client-side exclusion) must be rejected, not
// silently granted as a second, redundant kit.
func TestEquipmentKitChoiceRejectsHeldToolkit(t *testing.T) {
	s := testServer(t)
	seedKitChoiceFixtures(t, s)
	idStr := newKitChoiceCharacter(t, s, "Kit Choice Held Test")

	form := url.Values{
		"group_0":   {"0"},
		"kit_0_0_0": {"toolkit/forgery-kit"},
		"kit_0_0_1": {"toolkit/poison-kit"},
	}
	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/equipment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateEquipment(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("held-toolkit equipment POST status = %d, want 200 re-render with error, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "already have proficiency") {
		t.Errorf("held-toolkit equipment POST did not surface the expected error:\n%s", w.Body.String())
	}

	id, _ := strconv.ParseInt(idStr, 10, 64)
	var n int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_inventory WHERE character_id = ? AND notes = 'creation-equipment'`, id,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("equipment was saved despite the rejected held-toolkit pick (%d inventory rows)", n)
	}
}

// TestEquipmentKitChoiceRejectsSameSlotDuplicateToolkit covers the other
// half of the grant's own shape: two slots from the SAME "2 Kits of your
// Choice" pick must not resolve to the same toolkit twice over, even
// though create-equipment.js already disables a sibling slot's current
// pick client-side — a hand-built POST isn't bound by that.
func TestEquipmentKitChoiceRejectsSameSlotDuplicateToolkit(t *testing.T) {
	s := testServer(t)
	seedKitChoiceFixtures(t, s)
	idStr := newKitChoiceCharacter(t, s, "Kit Choice Duplicate Test")

	form := url.Values{
		"group_0":   {"0"},
		"kit_0_0_0": {"toolkit/poison-kit"},
		"kit_0_0_1": {"toolkit/poison-kit"},
	}
	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/equipment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateEquipment(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("same-slot duplicate equipment POST status = %d, want 200 re-render with error, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Pick a different toolkit for each slot") || !strings.Contains(w.Body.String(), "chosen twice") {
		t.Errorf("same-slot duplicate equipment POST did not surface the expected error:\n%s", w.Body.String())
	}
}

// TestEquipmentKitChoiceAcceptsTwoDistinctUnheldToolkits is the control
// case: two different toolkits the character has no proficiency in must
// still save cleanly, one inventory row each.
func TestEquipmentKitChoiceAcceptsTwoDistinctUnheldToolkits(t *testing.T) {
	s := testServer(t)
	seedKitChoiceFixtures(t, s)
	idStr := newKitChoiceCharacter(t, s, "Kit Choice Valid Test")

	form := url.Values{
		"group_0":   {"0"},
		"kit_0_0_0": {"toolkit/poison-kit"},
		"kit_0_0_1": {"toolkit/trappers-kit"},
	}
	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/equipment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateEquipment(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("valid equipment POST status = %d, want 303 redirect, body:\n%s", w.Code, w.Body.String())
	}

	id, _ := strconv.ParseInt(idStr, 10, 64)
	rows, err := s.charDB.Query(
		`SELECT item_slug, quantity FROM character_inventory WHERE character_id = ? AND notes = 'creation-equipment' ORDER BY item_slug`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var slug string
		var qty int
		if err := rows.Scan(&slug, &qty); err != nil {
			t.Fatal(err)
		}
		got[slug] = qty
	}
	want := map[string]int{"toolkit/poison-kit": 1, "toolkit/trappers-kit": 1}
	if len(got) != len(want) {
		t.Fatalf("saved inventory = %+v, want %+v", got, want)
	}
	for slug, qty := range want {
		if got[slug] != qty {
			t.Errorf("saved inventory[%q] = %d, want %d", slug, got[slug], qty)
		}
	}
}
