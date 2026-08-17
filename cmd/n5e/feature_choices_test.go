package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// seedASITestCharacter builds a level-4 Hunter-Nin (the book's own ASI
// breakpoint) plus one feat with no prerequisites (always eligible) and one
// with a prerequisite this character fails, so both the Feat branch's
// success and rejection paths can be exercised.
func seedASITestCharacter(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/hunter-nin', 'Hunter-Nin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/hunter-nin/feature/ability-score-improvement-feat', 'class/hunter-nin', 'Ability Score Improvement/Feat', NULL, 'pick an ability')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO feats (slug, name, category, description) VALUES
		('feat/nature-release', 'Nature Release', 'general', 'Pick a nature release.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO feats (slug, name, category, prerequisites, description) VALUES
		('feat/high-level-only', 'High Level Only', 'general', 'Level 20+', 'Not available yet.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (id, name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (1, 'ASI Test', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/hunter-nin', 4, 0)`,
	); err != nil {
		t.Fatal(err)
	}
}

func postASI(t *testing.T, s *server, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/asi", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetASI(w, req)
	return w
}

func TestHandleSheetASISplit(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"asi"}, "ability1": {"str"}, "ability2": {"dex"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	rows, err := s.charDB.Query(`SELECT ability, amount FROM character_ability_bonuses WHERE character_id = 1 AND source_ref = 'class/hunter-nin@4'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var ab string
		var amt int
		if err := rows.Scan(&ab, &amt); err != nil {
			t.Fatal(err)
		}
		got[ab] = amt
	}
	if got["str"] != 1 || got["dex"] != 1 || len(got) != 2 {
		t.Fatalf("got %v, want str:1 dex:1", got)
	}
}

func TestHandleSheetASIDouble(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"asi"}, "ability1": {"con"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var ability string
	var amount int
	if err := s.charDB.QueryRow(
		`SELECT ability, amount FROM character_ability_bonuses WHERE character_id = 1 AND source_ref = 'class/hunter-nin@4'`,
	).Scan(&ability, &amount); err != nil {
		t.Fatal(err)
	}
	if ability != "con" || amount != 2 {
		t.Fatalf("got %s:%d, want con:2", ability, amount)
	}
}

func TestHandleSheetASIRejectsSameAbilityTwice(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"asi"}, "ability1": {"str"}, "ability2": {"str"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a fake 'split' naming the same ability twice; body %s", w.Code, w.Body.String())
	}
}

func TestHandleSheetASIRejectsUnknownRef(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@99"}, "mode": {"asi"}, "ability1": {"str"}, "ability2": {"dex"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a ref the character doesn't qualify for; body %s", w.Code, w.Body.String())
	}
}

func TestHandleSheetASIFeatBranch(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"feat"}, "feat_slug": {"feat/nature-release"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var count int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1 AND feat_slug = 'feat/nature-release'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected the feat to be granted, got count %d", count)
	}
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_asi_feat_choices WHERE character_id = 1 AND ref = 'class/hunter-nin@4'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected the linkage row, got count %d", count)
	}

	// The breakpoint is now resolved, so re-posting the ability branch for
	// the same ref must be rejected the same way any already-resolved slot
	// would be (Compute no longer reports it as pending).
	w2 := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"asi"}, "ability1": {"str"}, "ability2": {"dex"}})
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 once the breakpoint is already resolved via the Feat branch; body %s", w2.Code, w2.Body.String())
	}
}

func TestHandleSheetASIFeatBranchRejectsUnqualifiedFeat(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"feat"}, "feat_slug": {"feat/high-level-only"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a feat this level-4 character doesn't qualify for; body %s", w.Code, w.Body.String())
	}
}

func TestHandleSheetASIFeatBranchRejectsUnknownFeat(t *testing.T) {
	s := testServer(t)
	seedASITestCharacter(t, s)

	w := postASI(t, s, url.Values{"ref": {"class/hunter-nin@4"}, "mode": {"feat"}, "feat_slug": {"feat/does-not-exist"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a nonexistent feat slug; body %s", w.Code, w.Body.String())
	}
}

// seedRaisedMaxASITestCharacter builds a level-4 Weapon Specialist, Samurai
// Form, with Master of Focus already granted (str/con base 20 -> 22, and per
// Item 11 the max raised right along with it to 22) — the end-to-end case
// that exercises sheet.AbilityMax flowing all the way through handleSheetASI
// via features.ValidASIPicks, not just the package-level unit tests.
func seedRaisedMaxASITestCharacter(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/weapon-specialist', 'Weapon Specialist', 10, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description) VALUES
		('class/weapon-specialist/feature/ability-score-improvement-feat', 'class/weapon-specialist', 'Ability Score Improvement/Feat', NULL, 'pick an ability')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/weapon-specialist/group/weapon-forms', 'class/weapon-specialist', 'Weapon Forms')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/weapon-specialist/group/weapon-forms/samurai-form', 'class/weapon-specialist/group/weapon-forms', 'Samurai Form')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		('class/weapon-specialist/group/weapon-forms/samurai-form/feature/master-of-focus',
		 'class/weapon-specialist/group/weapon-forms/samurai-form', 'Master of Focus', 3,
		 'Your Strength and Constitution scores increase by 2. Your maximum for these scores increases by 2.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (id, name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (1, 'Raised Max ASI Test', 20, 15, 20, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/weapon-specialist', 4, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (1, 'class/weapon-specialist/group/weapon-forms/samurai-form', 3)`,
	); err != nil {
		t.Fatal(err)
	}
}

func TestHandleSheetASIRejectsPickPastRaisedMax(t *testing.T) {
	s := testServer(t)
	seedRaisedMaxASITestCharacter(t, s)

	// Str already sits at 22 (20 base + Master of Focus's +2), the same as
	// its raised max — a further +1 must be rejected even though 22 is
	// above the old flat-20 cap.
	w := postASI(t, s, url.Values{"ref": {"class/weapon-specialist@4"}, "mode": {"asi"}, "ability1": {"str"}, "ability2": {"dex"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a str pick already at its raised max of 22; body %s", w.Code, w.Body.String())
	}
}

func TestHandleSheetASIAllowsUnaffectedAbilityPastOldFlatCap(t *testing.T) {
	s := testServer(t)
	seedRaisedMaxASITestCharacter(t, s)

	// Dex and Wis were never touched by Master of Focus, so they're still
	// governed by the ordinary flat-20 max and this split is legal.
	w := postASI(t, s, url.Values{"ref": {"class/weapon-specialist@4"}, "mode": {"asi"}, "ability1": {"dex"}, "ability2": {"wis"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 for a dex/wis split unaffected by the raised max; body %s", w.Code, w.Body.String())
	}
}
