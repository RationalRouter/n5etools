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

// seedSENTResources seeds the minimal rules content S.E.N.Ts' own gating
// needs: a Science-Nin class reaching 3rd level, its Technobi subclass, and
// the S.E.N.Ts subclass feature row itself — same shape
// seedCookingToolInfusionResources (cooking_nin_test.go) establishes for
// Cooking Tool Infusion.
func seedSENTResources(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/science-nin', 'Science-Nin', 8, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_levels (class_slug, level) VALUES
		('class/science-nin', 1), ('class/science-nin', 2), ('class/science-nin', 3)`,
	); err != nil {
		t.Fatal(err)
	}
	// loadScienceNinTabData (science_nin.go) treats a zero Creation Points
	// cap as "not really a Science-Nin for sheet purposes" and bails out to
	// a nil *scienceNinToolsTabData — needed here so the sheet_science_nin
	// fragment render in TestHandleSENTPickFreelyReeditable doesn't hit a
	// nil-pointer template error unrelated to what that test is checking.
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', 3, 'Creation Points', '3')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/technobi', 'class/science-nin/group/scientific-inquiry', 'Technobi')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/technobi', 'S.E.N.Ts', 3, 'S.E.N.Ts description')`,
		scienceNinSENTsFeatureSlug,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, properties) VALUES
		('weapon/kunai', 'Kunai', 'weapon', '1d4', 'Piercing', 'Light, Finesse, Thrown (30/60), Multiattack, Ammunition'),
		('weapon/longbow', 'Longbow', 'weapon', '1d8', 'Piercing', 'Range (120/180), Finesse, Two-Handed, Ammunition'),
		('weapon/shuriken', 'Shuriken', 'weapon', '1d4', 'Slashing', 'Thrown (30/120), Multiattack, Ammunition'),
		('weapon/fuma-shuriken', 'Fuma-Shuriken', 'weapon', '1d8', 'Slashing', 'Heavy, Two-Handed, Thrown (30/60), Returning'),
		('weapon/blowgun', 'Blowgun', 'weapon', '1', 'Piercing', 'Range (25/100), Ammunition, Loading')`,
	); err != nil {
		t.Fatal(err)
	}
}

// makeTechnobiCharacter inserts a character with the given Science-Nin
// level and, if withTechnobi, the Technobi subclass pick at 3rd level —
// enough for loadMergedGrantedFeatures to actually grant S.E.N.Ts once
// level 3 is reached.
func makeTechnobiCharacter(t *testing.T, s *server, id int64, level int, withTechnobi bool) {
	t.Helper()
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (id, name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 'Kabuto', 10, 12, 14, 18, 10, 10)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (?, 'class/science-nin', ?, 0)`, id, level); err != nil {
		t.Fatal(err)
	}
	if withTechnobi {
		if _, err := s.charDB.Exec(`
			INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
			VALUES (?, 'class/science-nin/group/scientific-inquiry/technobi', 3)`, id); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLoadSENTWeaponCatalog confirms the catalog reads real equipment rows
// by slug and excludes non-matching ammo weapons (Fuma-Shuriken's Returning
// property means it isn't a consumable stack; Blowgun isn't one of the
// feature's own named ammo types).
func TestLoadSENTWeaponCatalog(t *testing.T) {
	s := testServer(t)
	seedSENTResources(t, s)
	catalog, err := s.loadSENTWeaponCatalog()
	if err != nil {
		t.Fatal(err)
	}
	// 0019_missing_weapons.sql's own migration-seeded weapon/light-crossbow
	// is present in every rulesDB regardless of this test's own seeding, so
	// this checks containment rather than an exact count.
	got := map[string]bool{}
	for _, o := range catalog {
		got[o.Value] = true
	}
	for _, want := range []string{"weapon/kunai", "weapon/longbow", "weapon/shuriken"} {
		if !got[want] {
			t.Errorf("catalog missing %s: %v", want, catalog)
		}
	}
	for _, excluded := range []string{"weapon/fuma-shuriken", "weapon/blowgun"} {
		for _, o := range catalog {
			if o.Value == excluded {
				t.Errorf("catalog wrongly includes %s", excluded)
			}
		}
	}
}

// TestSentAttackOverridesGatedOnFeature confirms the pick only ever
// resolves once the character actually holds S.E.N.Ts (Technobi, 3rd
// level) — a character with no subclass chosen, or below 3rd level, gets
// no override even if a pick happens to be stored.
func TestSentAttackOverridesGatedOnFeature(t *testing.T) {
	s := testServer(t)
	seedSENTResources(t, s)

	makeTechnobiCharacter(t, s, 1, 3, false) // no subclass chosen at all
	if err := charstore.SetFeatureChoice(s.charDB, 1, scienceNinSENTsFeatureSlug, 0, "weapon/kunai"); err != nil {
		t.Fatal(err)
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.sentAttackOverrides(1, sheet); err != nil {
		t.Fatal(err)
	} else if got != "" {
		t.Errorf("character without Technobi: sentAttackOverrides = %q, want empty", got)
	}

	makeTechnobiCharacter(t, s, 2, 3, true)
	if err := charstore.SetFeatureChoice(s.charDB, 2, scienceNinSENTsFeatureSlug, 0, "weapon/kunai"); err != nil {
		t.Fatal(err)
	}
	sheet2, err := charsheet.Compute(s.rulesDB, s.charDB, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.sentAttackOverrides(2, sheet2); err != nil {
		t.Fatal(err)
	} else if got != "weapon/kunai" {
		t.Errorf("Technobi character at 3rd level: sentAttackOverrides = %q, want weapon/kunai", got)
	}
}

// TestBuildAttacksSENTOverrides confirms the equipped S.E.N.T weapon gets
// Intelligence in place of its own default ability, and its damage dice
// becomes 1d12 (the feature's "d10 stack, damage die increases by a step"
// text) — while a second, non-picked equipped weapon is untouched.
func TestBuildAttacksSENTOverrides(t *testing.T) {
	s := testServer(t)
	seedSENTResources(t, s)
	makeTechnobiCharacter(t, s, 1, 3, true)
	if err := charstore.SetFeatureChoice(s.charDB, 1, scienceNinSENTsFeatureSlug, 0, "weapon/kunai"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES
		(1, 'weapon/kunai', 1, 1),
		(1, 'weapon/longbow', 1, 1)`); err != nil {
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
	if len(attacks) != 2 {
		t.Fatalf("got %d attack rows, want 2", len(attacks))
	}
	var kunai, longbow *attackRow
	for i := range attacks {
		switch attacks[i].Slug {
		case "weapon/kunai":
			kunai = &attacks[i]
		case "weapon/longbow":
			longbow = &attacks[i]
		}
	}
	if kunai == nil || longbow == nil {
		t.Fatalf("missing expected attack rows: %+v", attacks)
	}
	if kunai.Ability != "INT" {
		t.Errorf("Kunai (the S.E.N.T pick) Ability = %q, want INT", kunai.Ability)
	}
	if kunai.DamageDice != "1d12" {
		t.Errorf("Kunai (the S.E.N.T pick) DamageDice = %q, want 1d12 (1d10 stack, stepped once)", kunai.DamageDice)
	}
	if longbow.Ability == "INT" {
		t.Errorf("Longbow (not the S.E.N.T pick) wrongly got the S.E.N.T ability override")
	}
	if longbow.DamageDice != "1d8" {
		t.Errorf("Longbow (not the S.E.N.T pick) DamageDice = %q, want its own catalog 1d8, untouched", longbow.DamageDice)
	}
}

// postSENTPick posts one S.E.N.T pick directly to the handler.
func postSENTPick(t *testing.T, handler func(http.ResponseWriter, *http.Request), value string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"value": {value}}
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/sent-pick", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// TestHandleSENTPickFreelyReeditable confirms the pick can be changed
// repeatedly — unlike Cooking Tool Implement's permanent-once-chosen lock,
// S.E.N.Ts' own "during a Short Rest you can work on a stack" text implies
// no such lock.
func TestHandleSENTPickFreelyReeditable(t *testing.T) {
	s := testServer(t)
	seedSENTResources(t, s)
	makeTechnobiCharacter(t, s, 1, 3, true)

	if w := postSENTPick(t, s.handleSENTPick, "weapon/kunai"); w.Code != http.StatusOK {
		t.Fatalf("first pick: status %d, body %s", w.Code, w.Body.String())
	}
	if w := postSENTPick(t, s.handleSENTPick, "weapon/longbow"); w.Code != http.StatusOK {
		t.Fatalf("re-pick: status %d, body %s", w.Code, w.Body.String())
	}
	choices, err := features.LoadFeatureChoices(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := choices[sentChoiceKey]; got != "weapon/longbow" {
		t.Errorf("after re-picking: stored value = %q, want weapon/longbow", got)
	}

	if w := postSENTPick(t, s.handleSENTPick, "weapon/not-a-real-weapon"); w.Code != http.StatusBadRequest {
		t.Errorf("invalid weapon slug: status %d, want 400", w.Code)
	}
}
