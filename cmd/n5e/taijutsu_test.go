package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

func TestMartialDieSize(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "d4"}, {8, "d4"}, {9, "d6"}, {16, "d6"}, {17, "d8"}, {20, "d8"},
	}
	for _, c := range cases {
		if got := martialDieSize(c.level); got != c.want {
			t.Errorf("martialDieSize(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestFistsOfIronBonusDice(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{3, 1}, {8, 1}, {9, 2}, {16, 2}, {17, 3}, {20, 3},
	}
	for _, c := range cases {
		if got := fistsOfIronBonusDice(c.level); got != c.want {
			t.Errorf("fistsOfIronBonusDice(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestUnarmedDamageDieSize(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{0, ""}, {1, "d6"}, {5, "d6"}, {6, "d8"}, {10, "d8"}, {11, "d10"}, {20, "d10"},
	}
	for _, c := range cases {
		if got := unarmedDamageDieSize(c.level, nil, ""); got != c.want {
			t.Errorf("unarmedDamageDieSize(%d, nil, \"\") = %q, want %q", c.level, got, c.want)
		}
	}
}

// TestUnarmedDamageDieSizeStanceOverride exercises the stance-conditional
// override stanceUnarmedDieGrants adds on top of the level-based
// progression above: a matching "X Fist Expert" feat only applies while the
// character's CURRENT Fighting Stance matches, and never lowers a
// level-based die that's already bigger.
func TestUnarmedDamageDieSizeStanceOverride(t *testing.T) {
	dragonFistExpert := []grantedFeatureRow{{Slug: "feat/dragon-fist-expert"}}

	cases := []struct {
		name       string
		level      int
		features   []grantedFeatureRow
		stanceSlug string
		want       string
	}{
		{"no real level, no feat, no stance: hidden", 0, nil, "", ""},
		{"no real level, feat but wrong stance selected: hidden", 0, dragonFistExpert, "stance/wolf-fist-stance", ""},
		{"no real level, feat and matching stance: d8", 0, dragonFistExpert, "stance/dragon-fist-stance", "d8"},
		{"level 1 (base d6), feat and matching stance: overridden to d8", 1, dragonFistExpert, "stance/dragon-fist-stance", "d8"},
		{"level 11 (base d10), feat and matching stance: d10 wins (larger)", 11, dragonFistExpert, "stance/dragon-fist-stance", "d10"},
		{"level 6 (base d8), feat but no stance selected: base d8 only", 6, dragonFistExpert, "", "d8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unarmedDamageDieSize(c.level, c.features, c.stanceSlug); got != c.want {
				t.Errorf("unarmedDamageDieSize(%d, %v, %q) = %q, want %q", c.level, c.features, c.stanceSlug, got, c.want)
			}
		})
	}
}

// seedTaijutsuLevelResources inserts the base class_level_resources chart
// for Taijutsu Specialist ("Martial Die": 1@2-5/2@6-9/3@10-13/4@14-18/5@19-20,
// "Martial Techniques": 4/5@7/6@13/7@19) — the real values, confirmed
// against the shipped rules.db, not placeholders.
func seedTaijutsuLevelResources(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/taijutsu-specialist', 'Taijutsu Specialist', 12, 6)`); err != nil {
		t.Fatal(err)
	}
	dieByLevel := map[int]int{}
	for lvl := 2; lvl <= 5; lvl++ {
		dieByLevel[lvl] = 1
	}
	for lvl := 6; lvl <= 9; lvl++ {
		dieByLevel[lvl] = 2
	}
	for lvl := 10; lvl <= 13; lvl++ {
		dieByLevel[lvl] = 3
	}
	for lvl := 14; lvl <= 18; lvl++ {
		dieByLevel[lvl] = 4
	}
	for lvl := 19; lvl <= 20; lvl++ {
		dieByLevel[lvl] = 5
	}
	techCap := func(lvl int) int {
		switch {
		case lvl >= 19:
			return 7
		case lvl >= 13:
			return 6
		case lvl >= 7:
			return 5
		default:
			return 4
		}
	}
	for lvl := 1; lvl <= 20; lvl++ {
		if _, err := s.rulesDB.Exec(
			`INSERT INTO class_levels (class_slug, level) VALUES ('class/taijutsu-specialist', ?)`, lvl,
		); err != nil {
			t.Fatal(err)
		}
		if d, ok := dieByLevel[lvl]; ok {
			if _, err := s.rulesDB.Exec(
				`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
				 ('class/taijutsu-specialist', ?, 'Martial Die', ?)`, lvl, d,
			); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.rulesDB.Exec(
			`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
			 ('class/taijutsu-specialist', ?, 'Martial Techniques', ?)`, lvl, techCap(lvl),
		); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMartialDiceMax(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)

	noOverlay := []grantedFeatureRow{}
	withUnnaturalTalent := []grantedFeatureRow{{Slug: unnaturalTalentFeatureSlug}}

	cases := []struct {
		name     string
		features []grantedFeatureRow
		level    int
		want     int
	}{
		{"base level 5, no overlay", noOverlay, 5, 1},
		{"base level 9, no overlay", noOverlay, 9, 2},
		{"unnatural talent below 3rd: no bonus yet", withUnnaturalTalent, 2, 1},
		{"unnatural talent at 5th (its own 3rd+ tier): +1", withUnnaturalTalent, 5, 2},
		{"unnatural talent at 10th: +2", withUnnaturalTalent, 10, 5},
		{"unnatural talent at 17th: +3", withUnnaturalTalent, 17, 7},
		{"unnatural talent at 20th: +3 (capped, same tier as 17th)", withUnnaturalTalent, 20, 8},
		{"martial arts training, no real level: base 1 die", []grantedFeatureRow{{Slug: martialArtsTrainingFeatSlug}}, 0, 1},
		{"training + expert, no real level: 2 dice", []grantedFeatureRow{{Slug: martialArtsTrainingFeatSlug}, {Slug: martialArtsExpertFeatSlug}}, 0, 2},
		{"training + expert + specialist, no real level: 3 dice", []grantedFeatureRow{{Slug: martialArtsTrainingFeatSlug}, {Slug: martialArtsExpertFeatSlug}, {Slug: martialArtsSpecialistFeatSlug}}, 0, 3},
		{"enthusiast alone grants no dice", []grantedFeatureRow{{Slug: martialArtsEnthusiastFeatSlug}}, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := s.martialDiceMax(c.features, c.level)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("martialDiceMax(level=%d) = %d, want %d", c.level, got, c.want)
			}
		})
	}
}

func TestMartialTechniqueCap(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)

	noOverlay := []grantedFeatureRow{}
	withTalentedLegacy := []grantedFeatureRow{{Slug: talentedLegacyFeatureSlug}}

	cases := []struct {
		name     string
		features []grantedFeatureRow
		level    int
		want     int
	}{
		{"base level 1, no overlay", noOverlay, 1, 4},
		{"base level 13, no overlay", noOverlay, 13, 6},
		{"talented legacy below 14th: no bonus yet", withTalentedLegacy, 13, 6},
		{"talented legacy at 14th: +1", withTalentedLegacy, 14, 7},
		{"talented legacy at 19th (still +1, base already 7)", withTalentedLegacy, 19, 8},
		{"talented legacy at 20th: +2 total", withTalentedLegacy, 20, 9},
		{"martial arts training, no real level: base cap 1", []grantedFeatureRow{{Slug: martialArtsTrainingFeatSlug}}, 0, 1},
		{"training + expert + specialist, no real level: cap 3", []grantedFeatureRow{{Slug: martialArtsTrainingFeatSlug}, {Slug: martialArtsExpertFeatSlug}, {Slug: martialArtsSpecialistFeatSlug}}, 0, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := s.martialTechniqueCap(c.features, c.level)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("martialTechniqueCap(level=%d) = %d, want %d", c.level, got, c.want)
			}
		})
	}
}

// TestMartialTechniqueAutoGrantsComplete guards against a typo silently
// dropping one of the 16 bespoke technique names (8 subclasses x 2 each) —
// a plain data-shape sanity check, not a rules assertion.
func TestMartialTechniqueAutoGrantsComplete(t *testing.T) {
	if len(martialTechniqueAutoGrants) != 8 {
		t.Fatalf("martialTechniqueAutoGrants has %d entries, want 8 (one per subclass)", len(martialTechniqueAutoGrants))
	}
	for slug, names := range martialTechniqueAutoGrants {
		if len(names) != 2 {
			t.Errorf("%s grants %d techniques, want 2", slug, len(names))
		}
		for _, n := range names {
			if n == "" {
				t.Errorf("%s has an empty technique name", slug)
			}
		}
	}
}

// TestLoadMartialTechniquesTabData exercises the full picker end-to-end: a
// level-9 Passionate Flame character learns one of two available class-wide
// techniques, and the auto-granted Enhanced Flurry pair (Chakra Enhanced
// Blows/Dynamic Set-Up) shows up as Known+Granted without ever being added
// to character_martial_techniques or counting against the cap.
func TestLoadMartialTechniquesTabData(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)

	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/taijutsu-specialist/group/taijutsu-style', 'class/taijutsu-specialist', 'Taijutsu Style')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/taijutsu-specialist/group/taijutsu-style/passionate-flame',
		 'class/taijutsu-specialist/group/taijutsu-style', 'Passionate Flame')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/taijutsu-specialist/group/taijutsu-style/passionate-flame/feature/fists-of-iron',
		 'class/taijutsu-specialist/group/taijutsu-style/passionate-flame', 'Fists of Iron', 3, 'Bonus damage.', 1),
		('class/taijutsu-specialist/group/taijutsu-style/passionate-flame/feature/enhanced-flurry',
		 'class/taijutsu-specialist/group/taijutsu-style/passionate-flame', 'Enhanced Flurry', 3, 'Grants two techniques.', 2)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO fighting_stances (slug, name, stance_type, description) VALUES
		('stance/iron-fist-stance', 'Iron Fist Stance', 'taijutsu', 'Your unarmed strikes deal extra damage.'),
		('stance/blade-stance', 'Blade Stance', 'bukijutsu', 'Not a Taijutsu Stance — should never be offered here.')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/taijutsu-specialist/option/martial-techniques/brutal-taijutsu', 'class/taijutsu-specialist', NULL,
		 'Martial Techniques', 'Brutal Taijutsu', 'Spend martial die for bonus damage.', 1),
		('class/taijutsu-specialist/option/martial-techniques/precise-taijutsu', 'class/taijutsu-specialist', NULL,
		 'Martial Techniques', 'Precise Taijutsu', 'Spend martial die for a bonus to hit.', 2)`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Rock Lee', 15, 14, 13, 8, 12, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/taijutsu-specialist', 9, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/taijutsu-specialist/group/taijutsu-style/passionate-flame', 3)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.loadMartialTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadMartialTechniquesTabData returned nil for a real Taijutsu Specialist")
	}
	if data.Cap != 5 {
		t.Errorf("Cap = %d, want 5 (level 9 base chart)", data.Cap)
	}
	if data.Used != 0 {
		t.Errorf("Used = %d, want 0 before any pick", data.Used)
	}
	if len(data.Known) != 2 {
		t.Fatalf("Known = %+v, want the 2 auto-granted Enhanced Flurry techniques only", data.Known)
	}
	for _, k := range data.Known {
		if !k.Granted {
			t.Errorf("known entry %+v should be Granted (auto-granted, not a player pick)", k)
		}
	}
	if len(data.Available) != 2 {
		t.Fatalf("Available = %+v, want both class-wide techniques before any pick", data.Available)
	}
	if data.PassionateFlame == nil {
		t.Fatal("PassionateFlame view is nil for a Passionate Flame character")
	}
	if data.PassionateFlame.FistsOfIronDice != 2 {
		t.Errorf("FistsOfIronDice = %d, want 2 (level 9 tier)", data.PassionateFlame.FistsOfIronDice)
	}
	if data.Ruin != nil {
		t.Error("Ruin view should be nil for a Passionate Flame character")
	}
	if data.UnarmedDamageDie != "d8" {
		t.Errorf("UnarmedDamageDie = %q, want d8 (level 9 tier)", data.UnarmedDamageDie)
	}
	if data.Stance == nil {
		t.Fatal("Stance view is nil for a Taijutsu Specialist")
	}
	if len(data.Stance.Options) != 1 {
		t.Fatalf("Stance.Options = %+v, want only the 1 taijutsu-type stance, not the bukijutsu one", data.Stance.Options)
	}
	if data.Stance.Current != "" {
		t.Errorf("Stance.Current = %q, want unset before any pick", data.Stance.Current)
	}

	// Pick a Fighting Stance.
	if err := charstore.SetFeatureChoice(s.charDB, 1, unarmedTechniqueFeatureSlug, 0, "stance/iron-fist-stance"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadMartialTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.Stance.Current != "stance/iron-fist-stance" {
		t.Errorf("Stance.Current = %q, want stance/iron-fist-stance after picking it", data.Stance.Current)
	}
	if data.Stance.CurrentName != "Iron Fist Stance" {
		t.Errorf("Stance.CurrentName = %q, want Iron Fist Stance", data.Stance.CurrentName)
	}

	// Learn one of the two available class-wide techniques.
	if err := charstore.AddMartialTechnique(s.charDB, 1, "class/taijutsu-specialist/option/martial-techniques/brutal-taijutsu"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadMartialTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.Used != 1 {
		t.Errorf("Used = %d, want 1 after learning one technique", data.Used)
	}
	if len(data.Known) != 3 {
		t.Fatalf("Known = %+v, want 2 granted + 1 learned", data.Known)
	}
	if len(data.Available) != 1 {
		t.Fatalf("Available = %+v, want 1 remaining after learning the other", data.Available)
	}
}

// TestLoadMartialTechniquesTabDataArchetypeFeatsNoRealLevel exercises the
// class-level gate widened to also open for the archetype-training feats —
// a character with zero real Taijutsu Specialist levels but Martial Arts
// Training + Martial Arts Expert + Iron Fist Expert must still get a
// rendered Martial Techniques box (2 Martial Dice, cap 2, a Fighting Stance
// picker stored under Martial Arts Training's own slug rather than Unarmed
// Technique's), and picking the matching Iron Fist Stance must apply Iron
// Fist Expert's stance-conditional Unarmed Damage die override even though
// this character has no real Unarmed Technique feature at all.
func TestLoadMartialTechniquesTabDataArchetypeFeatsNoRealLevel(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)

	if _, err := s.rulesDB.Exec(`
		INSERT INTO fighting_stances (slug, name, stance_type, description) VALUES
		('stance/iron-fist-stance', 'Iron Fist Stance', 'taijutsu', 'Your unarmed strikes deal extra damage.'),
		('stance/blade-stance', 'Blade Stance', 'bukijutsu', 'Not a Taijutsu Stance — should never be offered here.')`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Guy', 16, 14, 14, 8, 10, 12)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_feats (character_id, feat_slug, source) VALUES
		(1, ?, 'other'), (1, ?, 'other'), (1, 'feat/iron-fist-expert', 'other')`,
		martialArtsTrainingFeatSlug, martialArtsExpertFeatSlug,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	dice, err := s.loadMartialDice(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if dice == nil {
		t.Fatal("loadMartialDice returned nil for a character with archetype-training feats and no real Taijutsu Specialist levels")
	}
	if dice.Max != 2 {
		t.Errorf("Martial Dice Max = %d, want 2 (1 from Training + 1 from Expert)", dice.Max)
	}

	data, err := s.loadMartialTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadMartialTechniquesTabData returned nil for a character with archetype-training feats and no real Taijutsu Specialist levels")
	}
	if data.Cap != 2 {
		t.Errorf("Cap = %d, want 2 (1 from Training + 1 from Expert)", data.Cap)
	}
	if data.Stance == nil {
		t.Fatal("Stance view is nil for a character with Martial Arts Training")
	}
	if data.UnarmedDamageDie != "" {
		t.Errorf("UnarmedDamageDie = %q, want \"\" before any stance is picked", data.UnarmedDamageDie)
	}

	// Pick the Iron Fist Stance — stored under Martial Arts Training's own
	// slug (taijutsuStanceFeatureSlug), not Unarmed Technique's, since this
	// character has no real Taijutsu Specialist levels.
	if err := charstore.SetFeatureChoice(s.charDB, 1, martialArtsTrainingFeatSlug, 0, "stance/iron-fist-stance"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadMartialTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.Stance.Current != "stance/iron-fist-stance" {
		t.Errorf("Stance.Current = %q, want stance/iron-fist-stance after picking it", data.Stance.Current)
	}
	if data.UnarmedDamageDie != "d8" {
		t.Errorf("UnarmedDamageDie = %q, want d8 (Iron Fist Expert's own stance-conditional grant)", data.UnarmedDamageDie)
	}
}

// TestLoadMartialTechniquesTabDataNoLevelNoFeat confirms the box stays
// hidden (nil) for a character with neither real Taijutsu Specialist levels
// nor any archetype-training feat — the widened gate must not open
// unconditionally.
func TestLoadMartialTechniquesTabDataNoLevelNoFeat(t *testing.T) {
	s := testServer(t)
	seedTaijutsuLevelResources(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Ino', 8, 12, 10, 12, 14, 15)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	dice, err := s.loadMartialDice(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if dice != nil {
		t.Errorf("loadMartialDice = %+v, want nil for a character with no levels and no archetype feat", dice)
	}

	data, err := s.loadMartialTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Errorf("loadMartialTechniquesTabData = %+v, want nil for a character with no levels and no archetype feat", data)
	}
}
