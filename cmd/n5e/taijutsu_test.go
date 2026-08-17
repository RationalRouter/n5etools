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
		{1, "d6"}, {5, "d6"}, {6, "d8"}, {10, "d8"}, {11, "d10"}, {20, "d10"},
	}
	for _, c := range cases {
		if got := unarmedDamageDieSize(c.level); got != c.want {
			t.Errorf("unarmedDamageDieSize(%d) = %q, want %q", c.level, got, c.want)
		}
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
