package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// seedDuplicateProficiencyFixtures installs a Science-Nin-shaped class
// ("Select any three Toolkits") and a Genius-shaped background ("One
// Toolkit of your choice") plus four real toolkits — the minimal fixture
// reproducing the reported bug: a toolkit picked at the class step was
// still offered (and selectable) by the background step's own open
// toolkit choice.
func seedDuplicateProficiencyFixtures(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 6, 10)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_proficiencies (class_slug, kind, value) VALUES
		 ('class/science-nin', 'tool', 'Select any three Toolkits')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO backgrounds (slug, name, description) VALUES
		 ('background/genius', 'Genius', 'A prodigy.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO background_proficiencies (background_slug, kind, value) VALUES
		 ('background/genius', 'tool', 'One Toolkit of your choice')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT OR IGNORE INTO equipment (slug, name, kind) VALUES
		 ('toolkit/hackers-kit', 'Hackers Kit', 'toolkit'),
		 ('toolkit/trackers-kit', 'Trackers Kit', 'toolkit'),
		 ('toolkit/security-kit', 'Security Kit', 'toolkit'),
		 ('toolkit/poison-kit', 'Poison Kit', 'toolkit')`); err != nil {
		t.Fatal(err)
	}
}

// TestBackgroundToolkitExcludesClassStepPick is the exact reported bug:
// Science-Nin's three class-step toolkit picks (including Hackers Kit) were
// still all offered again by the Genius background's own "toolkit of your
// choice" grant, letting a player double up on a proficiency they already
// had. Covers both halves of the fix — the option not being rendered at
// all, and a hand-built POST naming it directly still being rejected.
func TestBackgroundToolkitExcludesClassStepPick(t *testing.T) {
	s := testServer(t)
	seedDuplicateProficiencyFixtures(t, s)

	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Duplicate Test')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	postClass := func(form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", idStr)
		w := httptest.NewRecorder()
		s.handleCreateClass(w, req)
		return w
	}

	w := postClass(url.Values{
		"class_slug": {"class/science-nin"},
		"toolkit_0":  {"Hackers Kit"},
		"toolkit_1":  {"Trackers Kit"},
		"toolkit_2":  {"Security Kit"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("class step POST status = %d, body:\n%s", w.Code, w.Body.String())
	}

	// Render-time exclusion: the background step's own dropdown must not
	// offer any of the three toolkits already granted by the class step.
	req := httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/create/background?background=background/genius", nil)
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateBackground(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("background step GET status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, held := range []string{"Hackers Kit", "Trackers Kit", "Security Kit"} {
		if strings.Contains(body, `value="`+held+`"`) {
			t.Errorf("background step still offers %q, already picked at the class step", held)
		}
	}
	if !strings.Contains(body, `value="Poison Kit"`) {
		t.Error("background step wrongly excluded Poison Kit, which was never picked anywhere")
	}

	// Submit-time defense in depth: a hand-built POST naming the
	// already-held toolkit directly (bypassing the dropdown's own
	// filtering above) must be rejected, not silently granted a second
	// time.
	bgForm := url.Values{"background_slug": {"background/genius"}, "choice_0_0": {"Hackers Kit"}}
	req = httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/background", strings.NewReader(bgForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateBackground(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("duplicate background POST status = %d, want 200 re-render with error, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "already have proficiency") {
		t.Errorf("duplicate background toolkit POST did not surface the expected error:\n%s", w.Body.String())
	}

	var toolCount int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = ? AND kind = 'tool' AND value = 'Hackers Kit'`, id,
	).Scan(&toolCount); err != nil {
		t.Fatal(err)
	}
	if toolCount != 1 {
		t.Errorf("Hackers Kit proficiency rows = %d, want exactly 1 (no duplicate granted)", toolCount)
	}
}

// TestClassStepRevisitExcludesLaterBackgroundPick covers going back to an
// earlier step after a later one already saved a pick: re-rendering the
// class step (after the background step has already granted Hackers Kit)
// must not offer Hackers Kit again, and a hand-built resubmission naming it
// must be rejected the same way the background step's own submit is.
func TestClassStepRevisitExcludesLaterBackgroundPick(t *testing.T) {
	s := testServer(t)
	seedDuplicateProficiencyFixtures(t, s)

	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Revisit Test')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	postClass := func(form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", idStr)
		w := httptest.NewRecorder()
		s.handleCreateClass(w, req)
		return w
	}

	// Class step first, deliberately leaving Hackers Kit unpicked here.
	w := postClass(url.Values{
		"class_slug": {"class/science-nin"},
		"toolkit_0":  {"Trackers Kit"},
		"toolkit_1":  {"Security Kit"},
		"toolkit_2":  {"Poison Kit"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("class step POST status = %d, body:\n%s", w.Code, w.Body.String())
	}

	// Background grants Hackers Kit.
	bgForm := url.Values{"background_slug": {"background/genius"}, "choice_0_0": {"Hackers Kit"}}
	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/background", strings.NewReader(bgForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateBackground(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("background step POST status = %d, body:\n%s", w.Code, w.Body.String())
	}

	// Revisiting the class step must not re-offer Hackers Kit, even though
	// it renders fine otherwise (the three already-chosen slots stay
	// visible/selected).
	req = httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/create/class?class=class/science-nin", nil)
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("class step revisit GET status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `value="Hackers Kit"`) {
		t.Error("revisited class step still offers Hackers Kit, already granted by the background step")
	}

	// And a resubmission naming it directly is rejected server-side too.
	w = postClass(url.Values{
		"class_slug": {"class/science-nin"},
		"toolkit_0":  {"Hackers Kit"},
		"toolkit_1":  {"Security Kit"},
		"toolkit_2":  {"Poison Kit"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("resubmit with an already-held toolkit status = %d, want 200 re-render with error, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "already have proficiency") {
		t.Errorf("resubmit with an already-held toolkit did not surface the expected error:\n%s", w.Body.String())
	}
}

// TestClassStepRejectsSameSlotDuplicateToolkit covers same-step
// self-collision: create-equipment.js already disables a sibling slot's
// selected option client-side, but a hand-built POST isn't bound by that —
// slot 2 repicking what slot 1 already chose (or slot 3 repicking either)
// must be rejected server-side too, not silently grant the same toolkit
// proficiency twice over.
func TestClassStepRejectsSameSlotDuplicateToolkit(t *testing.T) {
	s := testServer(t)
	seedDuplicateProficiencyFixtures(t, s)

	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Self Collision Test')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	form := url.Values{
		"class_slug": {"class/science-nin"},
		"toolkit_0":  {"Hackers Kit"},
		"toolkit_1":  {"Hackers Kit"},
		"toolkit_2":  {"Security Kit"},
	}
	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("same-slot duplicate POST status = %d, want 200 re-render with error, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Pick a different toolkit for each slot") {
		t.Errorf("same-slot duplicate POST did not surface the expected error:\n%s", w.Body.String())
	}

	var classCount int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`, id).Scan(&classCount); err != nil {
		t.Fatal(err)
	}
	if classCount != 0 {
		t.Errorf("class was saved despite the rejected duplicate toolkit pick (%d rows)", classCount)
	}
}
