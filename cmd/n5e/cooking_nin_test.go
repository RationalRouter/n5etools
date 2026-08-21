package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

func TestCookingToolInfusionLevel(t *testing.T) {
	cases := []struct {
		name  string
		level int
		arch  cookingArchetypeFeats
		want  int
	}{
		{"real level wins outright", 6, cookingArchetypeFeats{}, 6},
		{"real level wins even over a higher archetype tier", 3, cookingArchetypeFeats{Specialist: true}, 3},
		{"no level, no archetype feat", 0, cookingArchetypeFeats{}, 0},
		{"Chef Trainee -> as though 1st", 0, cookingArchetypeFeats{Trainee: true}, 1},
		{"Chefs Expert -> as though 6th", 0, cookingArchetypeFeats{Trainee: true, Expert: true}, 6},
		{"Chefs Specialist -> as though 11th", 0, cookingArchetypeFeats{Trainee: true, Expert: true, Specialist: true}, 11},
	}
	for _, c := range cases {
		if got := cookingToolInfusionLevel(c.level, c.arch); got != c.want {
			t.Errorf("%s: cookingToolInfusionLevel(%d, %+v) = %d, want %d", c.name, c.level, c.arch, got, c.want)
		}
	}
}

func TestFilterCookingToolPropertyL6Options(t *testing.T) {
	// Returning is offered only once the 1st-level pick is Thrown.
	withoutThrown := filterCookingToolPropertyL6Options("blocking", "")
	for _, o := range withoutThrown {
		if o.Value == "returning" {
			t.Errorf("Returning must not be offered without the Thrown property: %+v", withoutThrown)
		}
	}
	withThrown := filterCookingToolPropertyL6Options("thrown", "")
	found := false
	for _, o := range withThrown {
		if o.Value == "returning" {
			found = true
		}
	}
	if !found {
		t.Errorf("Returning must be offered once the 1st-level pick is Thrown: %+v", withThrown)
	}
	// A character who already holds Returning keeps seeing it even if the
	// 1st-level pick was changed away from Thrown afterward.
	stillShown := filterCookingToolPropertyL6Options("blocking", "returning")
	found = false
	for _, o := range stillShown {
		if o.Value == "returning" {
			found = true
		}
	}
	if !found {
		t.Errorf("an already-picked Returning must stay visible even if Thrown was un-picked: %+v", stillShown)
	}
}

func TestFilterCookingToolPropertyL11Options(t *testing.T) {
	withoutCritical := filterCookingToolPropertyL11Options("light", "")
	for _, o := range withoutCritical {
		if o.Value == "critical-2" {
			t.Errorf("Critical 2 must not be offered without the Critical property: %+v", withoutCritical)
		}
	}
	withCritical := filterCookingToolPropertyL11Options("critical", "")
	found := false
	for _, o := range withCritical {
		if o.Value == "critical-2" {
			found = true
		}
	}
	if !found {
		t.Errorf("Critical 2 must be offered once the 6th-level pick is Critical: %+v", withCritical)
	}
}

// seedCookingToolInfusionResources inserts the real Cooking Die chart
// (1d4 at 1st-4th, stepping up through 1d12 at 17th+, confirmed against
// the shipped rules.db). The implement catalog itself needs no seeding —
// schema.Apply already ran 0059_cooking_tool_infusion_implements.sql, so
// the real weapon/cooking-tool-* rows are already present on a fresh
// testServer().
func seedCookingToolInfusionResources(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/cooking-nin', 'Cooking-Nin', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_levels (class_slug, level) VALUES
		('class/cooking-nin', 1), ('class/cooking-nin', 6), ('class/cooking-nin', 11)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/cooking-nin', 1, 'Cooking Die', '1d4'),
		('class/cooking-nin', 6, 'Cooking Die', '1d6'),
		('class/cooking-nin', 11, 'Cooking Die', '1d8')`,
	); err != nil {
		t.Fatal(err)
	}
}

// TestLoadCookingToolInfusionView confirms the implement catalog loads off
// the equipment table, and the 6th/11th property rows only unlock at their
// own level.
func TestLoadCookingToolInfusionView(t *testing.T) {
	s := testServer(t)
	seedCookingToolInfusionResources(t, s)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Chouji', 10, 10, 14, 12, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/cooking-nin', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	view, err := s.loadCookingToolInfusionView(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.ImplementOptions) < 20 {
		t.Errorf("ImplementOptions = %d entries, want at least 20 (the catalog 0059 seeds)", len(view.ImplementOptions))
	}
	if view.PropertyL6Available || view.PropertyL11Available {
		t.Errorf("at 1st level neither the 6th nor 11th property row should be available: %+v", view)
	}

	view, err = s.loadCookingToolInfusionView(1, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !view.PropertyL6Available || view.PropertyL11Available {
		t.Errorf("at 6th level only the 6th property row should be available: %+v", view)
	}

	view, err = s.loadCookingToolInfusionView(1, 11)
	if err != nil {
		t.Fatal(err)
	}
	if !view.PropertyL6Available || !view.PropertyL11Available {
		t.Errorf("at 11th level both the 6th and 11th property rows should be available: %+v", view)
	}
}

// TestHandleCookingToolImplementEquipsInventory confirms picking an
// implement adds it as an equipped inventory row, switching implements
// removes the stale row rather than leaving a duplicate, and clearing the
// pick (empty value) drops it entirely.
func TestHandleCookingToolImplementEquipsInventory(t *testing.T) {
	s := testServer(t)
	seedCookingToolInfusionResources(t, s)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Chouji', 10, 10, 14, 12, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/cooking-nin', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	if err := s.syncCookingToolInfusionInventory(1, "weapon/cooking-tool-frying-pan"); err != nil {
		t.Fatal(err)
	}
	inv, err := s.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv) != 1 || inv[0].Slug != "weapon/cooking-tool-frying-pan" || !inv[0].Equipped {
		t.Fatalf("after picking Frying Pan: inventory = %+v, want one equipped Frying Pan row", inv)
	}

	// Switching implements must not leave the old row behind.
	if err := s.syncCookingToolInfusionInventory(1, "weapon/cooking-tool-ladle"); err != nil {
		t.Fatal(err)
	}
	inv, err = s.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv) != 1 || inv[0].Slug != "weapon/cooking-tool-ladle" || !inv[0].Equipped {
		t.Fatalf("after switching to Ladle: inventory = %+v, want one equipped Ladle row, no stale Frying Pan", inv)
	}

	// Clearing the pick drops the row entirely.
	if err := s.syncCookingToolInfusionInventory(1, ""); err != nil {
		t.Fatal(err)
	}
	inv, err = s.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv) != 0 {
		t.Fatalf("after clearing the pick: inventory = %+v, want empty", inv)
	}
}

// TestBuildAttacksCookingToolInfusionOverrides confirms buildAttacks
// applies the Cooking Tool's own always-on bonuses only to the equipped
// item matching the character's implement pick: Intelligence in place of
// Strength for both attack and damage, the level-scaling Cooking Die as
// the effective damage dice (not the catalog row's own inert 1d4), the
// player's chosen damage type, and a widened crit-threat range once
// Critical is known.
func TestBuildAttacksCookingToolInfusionOverrides(t *testing.T) {
	s := testServer(t)
	seedCookingToolInfusionResources(t, s)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Chouji', 10, 10, 14, 18, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/cooking-nin', 6, 0)`); err != nil {
		t.Fatal(err)
	}
	if err := s.syncCookingToolInfusionInventory(1, "weapon/cooking-tool-frying-pan"); err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetFeatureChoice(s.charDB, 1, cookingToolInfusionFeatureSlug, cookingToolChoiceImplement, "weapon/cooking-tool-frying-pan"); err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetFeatureChoice(s.charDB, 1, cookingToolInfusionFeatureSlug, cookingToolChoiceDamageType, "Bludgeoning"); err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetFeatureChoice(s.charDB, 1, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL6, "critical"); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	attacks, err := s.buildAttacks(1, inv, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 1 {
		t.Fatalf("got %d attack rows, want 1", len(attacks))
	}
	row := attacks[0]
	if row.Ability != "INT" {
		t.Errorf("Ability = %q, want INT", row.Ability)
	}
	if row.DamageDice != "1d6" {
		t.Errorf("DamageDice = %q, want 1d6 (the 6th-level Cooking Die, not the catalog's own 1d4)", row.DamageDice)
	}
	if row.DamageType != "Bludgeoning" {
		t.Errorf("DamageType = %q, want Bludgeoning", row.DamageType)
	}
	if row.CritRangeThreshold != 19 {
		t.Errorf("CritRangeThreshold = %d, want 19 (Critical's own +1 crit-threat rank)", row.CritRangeThreshold)
	}
}

// postCookingTool posts one Cooking Tool Infusion pick directly to the
// given handler, mirroring feature_choices_test.go's postASI shape.
func postCookingTool(t *testing.T, handler func(http.ResponseWriter, *http.Request), value string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"value": {value}}
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/cooking-tool", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// TestHandleCookingToolPicksLockOnceChosen confirms each of the 5 Cooking
// Tool Infusion picks locks after its first non-empty value: a second POST,
// whether attempting a different value or a clear, is rejected and the
// stored value is left untouched. See the doc comment above
// cookingToolChoiceImplement's own const block for why these picks follow
// Titan Specialization/Armor Chassis's "silence means permanent" precedent
// rather than Food For the Soul/Fast and Furious's free re-editability.
func TestHandleCookingToolPicksLockOnceChosen(t *testing.T) {
	s := testServer(t)
	seedCookingToolInfusionResources(t, s)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Chouji', 10, 10, 14, 12, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/cooking-nin', 11, 0)`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		handler   func(http.ResponseWriter, *http.Request)
		first     string
		second    string
		choiceIdx int
	}{
		{"implement", s.handleCookingToolImplement, "weapon/cooking-tool-frying-pan", "weapon/cooking-tool-ladle", cookingToolChoiceImplement},
		{"damage type", s.handleCookingToolDamageType, "Slashing", "Piercing", cookingToolChoiceDamageType},
		{"1st-level property", s.handleCookingToolPropertyL1, "deadly", "hidden", cookingToolChoicePropertyL1},
		{"6th-level property", s.handleCookingToolPropertyL6, "light", "critical", cookingToolChoicePropertyL6},
		{"11th-level property", s.handleCookingToolPropertyL11, "reach-3", "tactical", cookingToolChoicePropertyL11},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if w := postCookingTool(t, c.handler, c.first); w.Code != http.StatusOK {
				t.Fatalf("first pick (%s): status = %d, body = %s", c.first, w.Code, w.Body)
			}
			if w := postCookingTool(t, c.handler, c.second); w.Code == http.StatusOK {
				t.Fatalf("second pick (%s) after already choosing %s: status = %d, want a rejection", c.second, c.first, w.Code)
			}
			if w := postCookingTool(t, c.handler, ""); w.Code == http.StatusOK {
				t.Fatalf("clearing after already choosing %s: status = %d, want a rejection", c.first, w.Code)
			}
			choices, err := features.LoadFeatureChoices(s.charDB, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := choices[cookingToolChoiceKey(c.choiceIdx)]; got != c.first {
				t.Errorf("stored value = %q, want the original pick %q to survive both rejected attempts", got, c.first)
			}
		})
	}
}
