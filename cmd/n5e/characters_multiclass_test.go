package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// seedMulticlassClasses inserts two real classes (matching charstore's
// hand-curated multiclass tables by slug, so MeetsMulticlassPrereq/
// MulticlassGrantChoices actually have entries for them) plus a subclass
// group on the primary class, exercising the "Your Classes" panel's
// subclass-picker branch too.
func seedMulticlassClasses(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		 ('class/weapon-specialist', 'Weapon Specialist', 10, 8),
		 ('class/ninjutsu-specialist', 'Ninjutsu Specialist', 8, 10),
		 ('class/science-nin', 'Science-Nin', 6, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name, selection_levels) VALUES
		 ('class/weapon-specialist/group/weapon-forms', 'class/weapon-specialist', 'Weapon Forms', '3')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		 ('class/weapon-specialist/group/weapon-forms/iaijutsu', 'class/weapon-specialist/group/weapon-forms', 'Iaijutsu')`); err != nil {
		t.Fatal(err)
	}
}

func TestCreateClassMulticlassFlow(t *testing.T) {
	s := testServer(t)
	seedMulticlassClasses(t, s)

	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Multi')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	post := func(form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", idStr)
		w := httptest.NewRecorder()
		s.handleCreateClass(w, req)
		return w
	}
	get := func(class string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/create/class?class="+class, nil)
		req.SetPathValue("id", idStr)
		w := httptest.NewRecorder()
		s.handleCreateClass(w, req)
		return w
	}

	// First class: Weapon Specialist at level 5 — the original, untouched
	// first-class flow (already covered elsewhere), just establishing state.
	w := post(url.Values{"class_slug": {"class/weapon-specialist"}, "level": {"5"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("first class POST status = %d, body:\n%s", w.Code, w.Body.String())
	}

	// Give the character the ability scores to multiclass at all: the book
	// requires meeting BOTH the new class's prerequisite AND the primary
	// class's own (Weapon Specialist: str/dex 14), plus what Ninjutsu
	// Specialist needs (con/int 14) — but NOT what Science-Nin needs
	// (int>=16), used below to check the unmet-prereq rejection path.
	if _, err := s.charDB.Exec(
		`UPDATE characters SET base_str = 14, base_dex = 14, base_con = 14, base_int = 14 WHERE id = ?`, id,
	); err != nil {
		t.Fatal(err)
	}

	// Browsing a second, not-yet-held class renders the multiclass-add
	// form (a real render, not just a status check — this is what would
	// have caught the AlreadyHeld/primary-class regression this feature
	// introduced during development).
	w = get("class/ninjutsu-specialist")
	if w.Code != http.StatusOK {
		t.Fatalf("GET multiclass-add status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"Weapon Specialist", "Level 5", // Your Classes panel shows the primary
		"Add Ninjutsu Specialist as an additional class", // the multiclass-add form
	} {
		if !strings.Contains(body, want) {
			t.Errorf("multiclass-add page missing %q\nbody:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Ability score prerequisites not currently met") {
		t.Error("prereq warning shown even though con/int 14 were set")
	}
	// Regression: the multiclass-add level <select> must default to 1, not
	// silently pre-select the PRIMARY class's own current level (5) just
	// because renderClassStep computes one "current level" for the whole
	// page — caught by rendering this specific select's markup, not just
	// checking the page contains the number 5 somewhere (it legitimately
	// does, in the Your Classes panel's own Weapon Specialist row).
	if start := strings.Index(body, `id="mc-level"`); start >= 0 {
		end := strings.Index(body[start:], "</select>")
		if end < 0 {
			t.Fatal("mc-level select has no closing </select>")
		}
		mcLevelSelect := body[start : start+end]
		if !strings.Contains(mcLevelSelect, `value="1" selected`) {
			t.Errorf("multiclass-add level select does not default to 1:\n%s", mcLevelSelect)
		}
		if strings.Contains(mcLevelSelect, `value="5" selected`) {
			t.Errorf("multiclass-add level select leaked the primary class's own level (5):\n%s", mcLevelSelect)
		}
	} else {
		t.Fatal("multiclass-add page missing the mc-level select entirely")
	}

	// Confirm adding it.
	w = post(url.Values{"class_slug": {"class/ninjutsu-specialist"}, "level": {"3"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("add multiclass POST status = %d, body:\n%s", w.Code, w.Body.String())
	}

	var classCount int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`, id).Scan(&classCount); err != nil {
		t.Fatal(err)
	}
	if classCount != 2 {
		t.Fatalf("character_classes rows = %d, want 2", classCount)
	}
	var ninjaLevel, ninjaOrder int
	if err := s.charDB.QueryRow(
		`SELECT levels, order_index FROM character_classes WHERE character_id = ? AND class_slug = 'class/ninjutsu-specialist'`, id,
	).Scan(&ninjaLevel, &ninjaOrder); err != nil {
		t.Fatal(err)
	}
	if ninjaLevel != 3 || ninjaOrder != 1 {
		t.Errorf("ninjutsu-specialist levels=%d order_index=%d, want 3/1", ninjaLevel, ninjaOrder)
	}
	assertProficiency(t, s.charDB, id, "class/ninjutsu-specialist", "skill", "Ninshou")
	assertProficiency(t, s.charDB, id, "class/ninjutsu-specialist", "skill", "Chakra Control")

	// Revisiting the step with two classes held renders "Your Classes" with
	// both rows, including the subclass picker on the primary class and a
	// Remove button — the your_classes_panel template's real shape once
	// there's more than one row, not just the single-class case already
	// covered elsewhere.
	w = get("")
	if w.Code != http.StatusOK {
		t.Fatalf("GET class list status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body = w.Body.String()
	for _, want := range []string{"Weapon Specialist", "Ninjutsu Specialist", "Iaijutsu", "Remove"} {
		if !strings.Contains(body, want) {
			t.Errorf("Your Classes panel missing %q\nbody:\n%s", want, body)
		}
	}

	// Adding a THIRD class whose prerequisite (int>=16) is not met must be
	// rejected — re-rendered with an error, not silently accepted, and the
	// database must not gain a row for it.
	w = post(url.Values{"class_slug": {"class/science-nin"}, "level": {"1"}})
	if w.Code != http.StatusOK {
		t.Fatalf("unmet-prereq POST status = %d, want 200 re-render, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Ability score prerequisites not met for: Science-Nin") {
		t.Errorf("expected an unmet-prerequisite error naming Science-Nin, body:\n%s", w.Body.String())
	}
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`, id).Scan(&classCount); err != nil {
		t.Fatal(err)
	}
	if classCount != 2 {
		t.Fatalf("character_classes rows after rejected add = %d, want still 2", classCount)
	}

	// Set the subclass pick on the primary class through the creation
	// flow's own route (setCharacterSubclass), the "not even character
	// creation asks for one" gap this feature closes.
	subReq := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class/subclass", strings.NewReader(
		url.Values{"class_slug": {"class/weapon-specialist"}, "subclass_slug": {"class/weapon-specialist/group/weapon-forms/iaijutsu"}}.Encode()))
	subReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	subReq.SetPathValue("id", idStr)
	subW := httptest.NewRecorder()
	s.handleCreateClassSubclass(subW, subReq)
	if subW.Code != http.StatusSeeOther {
		t.Fatalf("set subclass status = %d, body:\n%s", subW.Code, subW.Body.String())
	}
	var subclassSlug string
	if err := s.charDB.QueryRow(`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, id).Scan(&subclassSlug); err != nil {
		t.Fatal(err)
	}
	if subclassSlug != "class/weapon-specialist/group/weapon-forms/iaijutsu" {
		t.Errorf("subclass_slug = %q, want the Iaijutsu pick", subclassSlug)
	}

	// Removing the secondary class cleans up its rows and drops the
	// character back to one class.
	remReq := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class/remove", strings.NewReader(
		url.Values{"class_slug": {"class/ninjutsu-specialist"}}.Encode()))
	remReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	remReq.SetPathValue("id", idStr)
	remW := httptest.NewRecorder()
	s.handleCreateClassRemove(remW, remReq)
	if remW.Code != http.StatusSeeOther {
		t.Fatalf("remove class status = %d, body:\n%s", remW.Code, remW.Body.String())
	}
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`, id).Scan(&classCount); err != nil {
		t.Fatal(err)
	}
	if classCount != 1 {
		t.Errorf("character_classes rows after remove = %d, want 1", classCount)
	}
}

// TestSheetClassMulticlassFlow exercises the sheet's own side of
// multiclassing: handleSheetClass (the Add a Class page), handleSheetClassLevel/
// Subclass/Remove, and — the part most likely to break silently, since
// go build/vet can't catch a template field mistake — a real render of the
// full character sheet for an already-multiclassed character, checking the
// redesigned sheet_level_row shows a read-only character-level total, a
// per-class level <select>, and the Add a Class link.
func TestSheetClassMulticlassFlow(t *testing.T) {
	s := testServer(t)
	seedMulticlassClasses(t, s)

	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Sheet Multi', 14, 14, 14, 14, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/weapon-specialist', 5, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}

	// The sheet's own Add a Class page renders the same picker as
	// creation's Class step (buildClassStepData reused verbatim) — a real
	// render, not just a status check.
	getReq := httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/sheet/class?class=class/ninjutsu-specialist", nil)
	getReq.SetPathValue("id", idStr)
	getW := httptest.NewRecorder()
	s.handleSheetClass(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET sheet add-class status = %d, body:\n%s", getW.Code, getW.Body.String())
	}
	if !strings.Contains(getW.Body.String(), "Add Ninjutsu Specialist as an additional class") {
		t.Errorf("sheet add-class page missing the multiclass-add form\nbody:\n%s", getW.Body.String())
	}

	postForm := func(path string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", idStr)
		w := httptest.NewRecorder()
		switch path {
		case "/sheet/class":
			s.handleSheetClass(w, req)
		case "/sheet/class/remove":
			s.handleSheetClassRemove(w, req)
		case "/sheet/class/subclass":
			s.handleSheetClassSubclass(w, req)
		case "/sheet/class/level":
			s.handleSheetClassLevel(w, req)
		default:
			t.Fatalf("unhandled path %s", path)
		}
		return w
	}

	// Confirm adding Ninjutsu Specialist (con/int 14 both meet its own and
	// Weapon Specialist's own retroactive prerequisite — str/dex 14).
	w := postForm("/sheet/class", url.Values{"class_slug": {"class/ninjutsu-specialist"}, "level": {"2"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("add multiclass from sheet status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/characters/"+idStr {
		t.Errorf("add multiclass redirect = %q, want the sheet itself", loc)
	}

	// The full sheet now renders both classes with per-class level
	// selects, a read-only character-level total, and the Add a Class link.
	sheetReq := httptest.NewRequest(http.MethodGet, "/characters/"+idStr, nil)
	sheetReq.SetPathValue("id", idStr)
	sheetW := httptest.NewRecorder()
	s.handleCharacterSheet(sheetW, sheetReq)
	if sheetW.Code != http.StatusOK {
		t.Fatalf("GET sheet status = %d, body:\n%s", sheetW.Code, sheetW.Body.String())
	}
	body := sheetW.Body.String()
	for _, want := range []string{
		"Character Level 7", // 5 + 2
		"Weapon Specialist", "Ninjutsu Specialist",
		`class="sheet-class-level-select"`,
		`href="/characters/` + idStr + `/sheet/class">+ Add a Class</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet missing %q\nbody:\n%s", want, body)
		}
	}

	// Change Weapon Specialist's own level in place through the sheet's
	// per-class level control.
	w = postForm("/sheet/class/level", url.Values{"class_slug": {"class/weapon-specialist"}, "level": {"6"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("sheet class level status = %d, body:\n%s", w.Code, w.Body.String())
	}
	var weaponLevel int
	if err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = 'class/weapon-specialist'`, id,
	).Scan(&weaponLevel); err != nil {
		t.Fatal(err)
	}
	if weaponLevel != 6 {
		t.Errorf("weapon-specialist level = %d, want 6", weaponLevel)
	}

	// Set the subclass pick through the sheet's own Add a Class route.
	w = postForm("/sheet/class/subclass", url.Values{
		"class_slug": {"class/weapon-specialist"}, "subclass_slug": {"class/weapon-specialist/group/weapon-forms/iaijutsu"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("sheet class subclass status = %d, body:\n%s", w.Code, w.Body.String())
	}
	var subclassSlug string
	if err := s.charDB.QueryRow(`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, id).Scan(&subclassSlug); err != nil {
		t.Fatal(err)
	}
	if subclassSlug != "class/weapon-specialist/group/weapon-forms/iaijutsu" {
		t.Errorf("subclass_slug = %q, want the Iaijutsu pick", subclassSlug)
	}

	// Remove the secondary class through the sheet's own route.
	w = postForm("/sheet/class/remove", url.Values{"class_slug": {"class/ninjutsu-specialist"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("sheet class remove status = %d, body:\n%s", w.Code, w.Body.String())
	}
	var classCount int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`, id).Scan(&classCount); err != nil {
		t.Fatal(err)
	}
	if classCount != 1 {
		t.Errorf("character_classes rows after sheet remove = %d, want 1", classCount)
	}
}

func assertProficiency(t *testing.T, charDB *sql.DB, characterID int64, sourceRef, kind, value string) {
	t.Helper()
	var n int
	if err := charDB.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = ? AND source_ref = ? AND kind = ? AND value = ?`,
		characterID, sourceRef, kind, value,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected exactly one %s/%s proficiency from %s, got %d", kind, value, sourceRef, n)
	}
}
