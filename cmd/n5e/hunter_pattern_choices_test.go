package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// seedHunterPatternChoiceOptions inserts the two proficiency-choice Hunters
// Patterns not already seeded by seedHunterNinLevelResources (hunter_nin_
// test.go) — kleptomaniac IS seeded there, habitual-researcher and
// practiced-combatant are not, so both are added here rather than widening
// the shared helper and perturbing its own AvailablePatterns count
// assertions in TestLoadHunterTechniquesTabData.
func seedHunterPatternChoiceOptions(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/hunter-nin/option/hunters-patterns/habitual-researcher', 'class/hunter-nin', NULL,
		 'Hunters Patterns', 'Habitual Researcher', 'Select two skills. You gain proficiency in the given skills.', 3),
		('class/hunter-nin/option/hunters-patterns/practiced-combatant', 'class/hunter-nin', NULL,
		 'Hunters Patterns', 'Practiced Combatant', 'You gain one Taijutsu or Weapon stance from Chapter 13.', 4)`,
	); err != nil {
		t.Fatal(err)
	}
}

func seedHunterPatternChoiceCharacter(t *testing.T, s *server) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Pattern Test', 10, 12, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/hunter-nin', 2, 0)`, id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBuildPendingHunterPatternChoiceRowsKleptomaniac(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	id := seedHunterPatternChoiceCharacter(t, s)

	rows, err := s.buildPendingHunterPatternChoiceRows(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("pending rows before any Pattern pick = %+v, want none", rows)
	}

	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickPattern, kleptomaniacOptionSlug); err != nil {
		t.Fatal(err)
	}
	rows, err = s.buildPendingHunterPatternChoiceRows(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PatternSlug != kleptomaniacOptionSlug {
		t.Fatalf("pending rows after picking Kleptomaniac = %+v, want exactly one Kleptomaniac row", rows)
	}
	if len(rows[0].Fields) != 1 || len(rows[0].Fields[0].Options) != 2 {
		t.Fatalf("Kleptomaniac row fields = %+v, want one field with 2 options", rows[0].Fields)
	}
}

// TestHandleSheetHunterPatternChoiceKleptomaniac resolves Kleptomaniac's own
// pick toward the tool option (Security Kits) and confirms: the granted
// proficiency shows up via loadCharacterProficiencyValues (the same reader
// backing the Tool Proficiencies panel, with no source_kind filter — see
// ApplyHunterPatternProficiencies' own doc comment) and the pending row
// disappears from a fresh build once resolved.
func TestHandleSheetHunterPatternChoiceKleptomaniac(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	id := seedHunterPatternChoiceCharacter(t, s)
	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickPattern, kleptomaniacOptionSlug); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/hunter-patterns/choice", strings.NewReader(url.Values{
		"pattern_slug": {kleptomaniacOptionSlug},
		"choice":       {"tool:Security Kits"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetHunterPatternChoice(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("resolve kleptomaniac choice: status %d, body %q", w.Code, w.Body.String())
	}

	tools, err := s.loadCharacterProficiencyValues(id, "tool")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0] != "Security Kits" {
		t.Fatalf("tool proficiencies after resolving Kleptomaniac = %+v, want [Security Kits]", tools)
	}

	rows, err := s.buildPendingHunterPatternChoiceRows(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("pending rows after resolving Kleptomaniac = %+v, want none", rows)
	}
}

// TestHandleSheetHunterPatternChoiceHabitualResearcher resolves Habitual
// Researcher's two-skill pick and confirms both land as real skill
// proficiencies (charsheet.Compute's own skill loader, no source_kind
// filter), and that picking the same skill for both fields is rejected.
func TestHandleSheetHunterPatternChoiceHabitualResearcher(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	seedHunterPatternChoiceOptions(t, s)
	id := seedHunterPatternChoiceCharacter(t, s)
	habitualResearcherSlug := "class/hunter-nin/option/hunters-patterns/habitual-researcher"
	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickPattern, habitualResearcherSlug); err != nil {
		t.Fatal(err)
	}

	// Same skill picked twice must be rejected — the book's own "select two
	// skills" implies two distinct ones.
	badReq := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/hunter-patterns/choice", strings.NewReader(url.Values{
		"pattern_slug": {habitualResearcherSlug},
		"choice_1":     {"skill:Stealth"},
		"choice_2":     {"skill:Stealth"},
	}.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.SetPathValue("id", "1")
	badW := httptest.NewRecorder()
	s.handleSheetHunterPatternChoice(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("resolve habitual researcher with duplicate skills: status %d, want 400", badW.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/hunter-patterns/choice", strings.NewReader(url.Values{
		"pattern_slug": {habitualResearcherSlug},
		"choice_1":     {"skill:Stealth"},
		"choice_2":     {"skill:Perception"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetHunterPatternChoice(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("resolve habitual researcher choice: status %d, body %q", w.Code, w.Body.String())
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range []string{"Stealth", "Perception"} {
		found := false
		for _, sk := range sheet.Skills {
			if sk.Name == skill {
				found = sk.Proficient
			}
		}
		if !found {
			t.Errorf("%s not proficient after resolving Habitual Researcher", skill)
		}
	}
}

// TestHandleHunterPickDeleteRetractsPatternProficiency confirms unpicking a
// resolved Kleptomaniac retracts its granted proficiency instead of leaving
// an orphaned row behind with no pick to justify it.
func TestHandleHunterPickDeleteRetractsPatternProficiency(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	id := seedHunterPatternChoiceCharacter(t, s)
	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickPattern, kleptomaniacOptionSlug); err != nil {
		t.Fatal(err)
	}
	if err := charstore.ApplyHunterPatternProficiencies(s.charDB, id, kleptomaniacOptionSlug,
		[]charstore.HunterPatternProficiency{{Kind: "skill", Value: "Sleight of Hand"}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/hunter-patterns/delete", strings.NewReader(url.Values{
		"option_slug": {kleptomaniacOptionSlug},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleHunterPickDelete(charstore.HunterPickPattern)(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("delete kleptomaniac pick: status %d, body %q", w.Code, w.Body.String())
	}

	skills, err := s.loadCharacterProficiencyValues(id, "skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		if sk == "Sleight of Hand" {
			t.Fatal("Sleight of Hand proficiency survived unpicking Kleptomaniac")
		}
	}
}

// TestHunterPracticedCombatantStance exercises Practiced Combatant's own
// Taijutsu-or-Weapon stance grant: hidden until the Pattern is picked,
// rejects a stance pick before that, and stores/reflects a valid one
// afterward via loadHunterTechniquesTabData (the same reader the sheet
// template renders from).
func TestHunterPracticedCombatantStance(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	seedHunterPatternChoiceOptions(t, s)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO fighting_stances (slug, stance_type, name, description) VALUES
		('stance/taijutsu/iron-fist', 'taijutsu', 'Iron Fist', 'A hard-hitting stance.'),
		('stance/bukijutsu/steel-guard', 'bukijutsu', 'Steel Guard', 'A defensive weapon stance.')`,
	); err != nil {
		t.Fatal(err)
	}
	id := seedHunterPatternChoiceCharacter(t, s)

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.PracticedCombatantStance != nil {
		t.Fatalf("PracticedCombatantStance = %+v, want nil before the Pattern is picked", data.PracticedCombatantStance)
	}

	// Rejected before the Pattern is picked.
	badReq := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/hunter-patterns/practiced-combatant-stance", strings.NewReader(url.Values{
		"stance_slug": {"stance/taijutsu/iron-fist"},
	}.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.SetPathValue("id", "1")
	badW := httptest.NewRecorder()
	s.handleHunterPracticedCombatantStance(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("set stance before picking Practiced Combatant: status %d, want 400", badW.Code)
	}

	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickPattern, hunterNinPracticedCombatantOptionSlug); err != nil {
		t.Fatal(err)
	}

	data, err = s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.PracticedCombatantStance == nil {
		t.Fatal("PracticedCombatantStance = nil, want a view once the Pattern is picked")
	}
	if len(data.PracticedCombatantStance.Options) != 2 {
		t.Fatalf("PracticedCombatantStance.Options = %+v, want both seeded stances (taijutsu + bukijutsu)", data.PracticedCombatantStance.Options)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/hunter-patterns/practiced-combatant-stance", strings.NewReader(url.Values{
		"stance_slug": {"stance/bukijutsu/steel-guard"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleHunterPracticedCombatantStance(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("set practiced combatant stance: status %d, body %q", w.Code, w.Body.String())
	}

	data, err = s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.PracticedCombatantStance.Current != "stance/bukijutsu/steel-guard" {
		t.Errorf("PracticedCombatantStance.Current = %q, want stance/bukijutsu/steel-guard", data.PracticedCombatantStance.Current)
	}
}
