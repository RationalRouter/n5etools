package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// seedVoidSoulRules inserts a minimal Scout-Nin class, its Trickster Scout
// subclass, and the Void Soul Awakening feature (3rd level) — enough to
// satisfy hasFeature(granted, voidSoulAwakeningFeatureSlug)'s own gate in
// voidSoulJutsuAddCore/loadVoidSoulReference without needing the full
// seeded rules.db, mirroring seedTitanRules' identical shape for Ordnance
// Training.
func seedVoidSoulRules(t *testing.T, s *server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/scout-nin', 'Scout-Nin', 10, 8)`,
		`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
			('class/scout-nin/group/scouting-technique', 'class/scout-nin', 'Scouting Technique')`,
		`INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/scout-nin/group/scouting-technique/trickster-scout',
			 'class/scout-nin/group/scouting-technique', 'Trickster Scout')`,
		`INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
			('` + voidSoulAwakeningFeatureSlug + `',
			 'class/scout-nin/group/scouting-technique/trickster-scout', 'Void Soul Awakening', 3,
			 'Summon a Void Soul.')`,
		`INSERT INTO class_casting (class_slug, discipline, ability) VALUES
			('class/scout-nin', 'ninjutsu', 'int'),
			('class/scout-nin', 'genjutsu', 'wis'),
			('class/scout-nin', 'taijutsu', 'str')`,
	}
	for _, stmt := range stmts {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("seed rules: %v\n%s", err, stmt)
		}
	}
}

// seedVoidSoulCharacter inserts a Scout-Nin character past Void Soul
// Awakening's own 3rd-level gate, with the Trickster Scout subclass chosen,
// and a kind="void-soul" companion row already attached — the common setup
// every test below needs. baseCha sets the character's own Charisma score,
// which drives the ability-point budget (Charisma Modifier x 3).
func seedVoidSoulCharacter(t *testing.T, s *server, baseCha int) (characterID, companionID int64) {
	t.Helper()
	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Void Soul Tester', 10, 10, 10, 10, 10, ?)`, baseCha)
	if err != nil {
		t.Fatal(err)
	}
	characterID, _ = res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/scout-nin', 3, 0)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/scout-nin/group/scouting-technique/trickster-scout', 3)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}
	res, err = s.charDB.Exec(
		`INSERT INTO character_companions (character_id, kind, name) VALUES (?, 'void-soul', 'Test Void Soul')`,
		characterID,
	)
	if err != nil {
		t.Fatal(err)
	}
	companionID, _ = res.LastInsertId()
	return characterID, companionID
}

// postVoidSoulAbilityPoint exercises handleVoidSoulAbilityPoint directly,
// mirroring TestPrefillTitanStatDefaultsOnCreation's own "drive the real
// HTTP handler, not the underlying logic" approach — this pins the route's
// own path-value wiring (parseCharacterAndCompanionID) along with the
// budget/cap math.
func postVoidSoulAbilityPoint(t *testing.T, s *server, characterID, companionID int64, ability, delta string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/1/void-soul-ability-point",
		strings.NewReader(url.Values{"ability": {ability}, "delta": {delta}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", strconv.FormatInt(characterID, 10))
	req.SetPathValue("cid", strconv.FormatInt(companionID, 10))
	w := httptest.NewRecorder()
	s.handleVoidSoulAbilityPoint(w, req)
	return w
}

// TestVoidSoulAbilityPointRespectsBudgetAndPerAbilityCap covers Void Soul
// Awakening's own point-buy allocator: "You gain a number of points to
// spend, equal to your Charisma Modifier times 3... The maximum value of
// any modifier is +6." Charisma 16 here is a +3 modifier, so the budget is
// 9 points total.
func TestVoidSoulAbilityPointRespectsBudgetAndPerAbilityCap(t *testing.T) {
	s := testServer(t)
	seedVoidSoulRules(t, s)
	characterID, companionID := seedVoidSoulCharacter(t, s, 16)

	// Spend all 9 points on STR: +6 (the per-ability ceiling) should
	// succeed, but the 7th point overall must be rejected once STR is
	// already at its own +6 cap, not because the budget ran out (9 > 6).
	for i := 0; i < voidSoulMaxModifier; i++ {
		if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "str", "1"); w.Code != http.StatusOK {
			t.Fatalf("increase STR (%d/%d): status %d, body %s", i+1, voidSoulMaxModifier, w.Code, w.Body.String())
		}
	}
	if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "str", "1"); w.Code == http.StatusOK {
		t.Fatalf("increasing STR past +%d modifier should be rejected (per-ability ceiling), got 200", voidSoulMaxModifier)
	}

	companion, err := charstore.GetCompanion(s.charDB, characterID, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if !companion.Str.Valid || companionScoreModifier(companion.Str) != voidSoulMaxModifier {
		t.Fatalf("STR modifier = %+v, want %d", companion.Str, voidSoulMaxModifier)
	}

	// 6 of the 9-point budget is spent on STR; 3 remain. Spending them on
	// DEX should succeed, but the budget (not the per-ability ceiling) must
	// reject a 4th point on DEX.
	for i := 0; i < 3; i++ {
		if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "dex", "1"); w.Code != http.StatusOK {
			t.Fatalf("increase DEX (%d/3): status %d, body %s", i+1, w.Code, w.Body.String())
		}
	}
	if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "dex", "1"); w.Code == http.StatusOK {
		t.Fatal("increasing DEX past the character's own total budget should be rejected, got 200")
	}

	// Decreasing STR by one frees a point back up for DEX.
	if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "str", "-1"); w.Code != http.StatusOK {
		t.Fatalf("decrease STR: status %d, body %s", w.Code, w.Body.String())
	}
	if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "dex", "1"); w.Code != http.StatusOK {
		t.Fatalf("increase DEX after freeing a point: status %d, body %s", w.Code, w.Body.String())
	}

	// A modifier can never go negative — "the following" plainly can't be
	// decreased below its own un-spent floor.
	if w := postVoidSoulAbilityPoint(t, s, characterID, companionID, "cha", "-1"); w.Code == http.StatusOK {
		t.Fatal("decreasing an already-+0 modifier below zero should be rejected, got 200")
	}
}

// TestVoidSoulJutsuAddRespectsKnownCap covers the companion-scoped known-
// jutsu picker's own cap: "equal to your Proficiency Bonus" — a 3rd-level
// character's Proficiency Bonus is +3, so a 4th pick must be rejected
// regardless of whether it would otherwise be eligible.
func TestVoidSoulJutsuAddRespectsKnownCap(t *testing.T) {
	s := testServer(t)
	seedVoidSoulRules(t, s)
	characterID, _ := seedVoidSoulCharacter(t, s, 16)

	// Four Bukijutsu D-rank jutsu with no elemental keyword — Bukijutsu is
	// admitted unconditionally for every class (classJutsuPredicate's own
	// doc), so all four are eligible on class origin alone.
	slugs := []string{"jutsu/one", "jutsu/two", "jutsu/three", "jutsu/four"}
	for i, slug := range slugs {
		if _, err := s.rulesDB.Exec(
			`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
				VALUES (?, ?, 'Bukijutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Bukijutsu', 'test jutsu')`,
			slug, "Test Jutsu "+strconv.Itoa(i),
		); err != nil {
			t.Fatalf("seed jutsu %s: %v", slug, err)
		}
	}

	for i, slug := range slugs[:3] {
		status, msg := s.voidSoulJutsuAddCore(characterID, slug)
		if status != http.StatusOK {
			t.Fatalf("add jutsu %d/%s: status %d, msg %s", i, slug, status, msg)
		}
	}
	status, msg := s.voidSoulJutsuAddCore(characterID, slugs[3])
	if status == http.StatusOK {
		t.Fatal("a 4th pick beyond the Proficiency Bonus cap should be rejected, got 200")
	}
	if msg != "known jutsu cap reached" {
		t.Errorf("rejection message = %q, want the cap-specific message", msg)
	}

	known, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickVoidSoulJutsu)
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 3 {
		t.Fatalf("known picks = %d, want 3", len(known))
	}
}

// TestVoidSoulJutsuAddRejectsIneligibleAndDuplicatePicks covers two of
// voidSoulJutsuAddCore's own rejections that aren't about the cap: a jutsu
// the character has no class/clan/affinity claim on at all (Fire Release,
// no affinity granted), and re-adding a slug that's already known.
func TestVoidSoulJutsuAddRejectsIneligibleAndDuplicatePicks(t *testing.T) {
	s := testServer(t)
	seedVoidSoulRules(t, s)
	characterID, _ := seedVoidSoulCharacter(t, s, 16)

	if _, err := s.rulesDB.Exec(
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description) VALUES
			('jutsu/fire-test', 'Fire Test', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Fire Release, Ninjutsu', 'needs Fire affinity'),
			('jutsu/combo-test', 'Combo Test', 'Bukijutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Combination, Bukijutsu', 'a Combination jutsu'),
			('jutsu/known-test', 'Known Test', 'Bukijutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Bukijutsu', 'plain eligible jutsu')`,
	); err != nil {
		t.Fatal(err)
	}

	if status, msg := s.voidSoulJutsuAddCore(characterID, "jutsu/fire-test"); status == http.StatusOK {
		t.Fatal("a jutsu needing an affinity the character doesn't have should be rejected, got 200")
	} else if msg != "not a valid pick" {
		t.Errorf("rejection message = %q, want %q", msg, "not a valid pick")
	}

	// "Your Void Soul cannot cast Combination Jutsu."
	if status, _ := s.voidSoulJutsuAddCore(characterID, "jutsu/combo-test"); status == http.StatusOK {
		t.Fatal("a Combination jutsu should always be rejected, got 200")
	}

	if status, msg := s.voidSoulJutsuAddCore(characterID, "jutsu/known-test"); status != http.StatusOK {
		t.Fatalf("add an eligible jutsu: status %d, msg %s", status, msg)
	}
	if status, _ := s.voidSoulJutsuAddCore(characterID, "jutsu/known-test"); status == http.StatusOK {
		t.Fatal("re-adding an already-known jutsu should be rejected, got 200")
	}
}
