package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

func TestHuntersPatternsCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 1}, {8, 1}, {9, 2}, {14, 2}, {15, 3}, {20, 3},
	}
	for _, c := range cases {
		if got := huntersPatternsCap(c.level); got != c.want {
			t.Errorf("huntersPatternsCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestHuntersExploitsCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 2}, {9, 2}, {10, 3}, {16, 3}, {17, 4}, {20, 4},
	}
	for _, c := range cases {
		if got := huntersExploitsCap(c.level); got != c.want {
			t.Errorf("huntersExploitsCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestDefensiveTacticsCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {5, 0}, {6, 1}, {10, 1}, {11, 2}, {16, 2}, {17, 3}, {20, 3},
	}
	for _, c := range cases {
		if got := defensiveTacticsCap(c.level); got != c.want {
			t.Errorf("defensiveTacticsCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// seedHunterNinLevelResources inserts Lethal Attack's own chart (the real
// values, confirmed against the shipped rules.db) plus a minimal Grave
// Stalker/Blade Warden subclass and exploit catalog for the exclusive-grant
// and subclass-lock filtering tests below.
func seedHunterNinLevelResources(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/hunter-nin', 'Hunter-Nin', 8, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO class_levels (class_slug, level) VALUES ('class/hunter-nin', 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/hunter-nin', 10, 'Lethal Attack', '5d8')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/hunter-nin/group/hunters-creeds', 'class/hunter-nin', 'Hunters Creeds')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/hunter-nin/group/hunters-creeds/grave-stalker', 'class/hunter-nin/group/hunters-creeds', 'Grave Stalker'),
		('class/hunter-nin/group/hunters-creeds/blade-warden', 'class/hunter-nin/group/hunters-creeds', 'Blade Warden')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/hunter-nin/group/hunters-creeds/grave-stalker/feature/shadow-stalker',
		 'class/hunter-nin/group/hunters-creeds/grave-stalker', 'Shadow Stalker', 3, 'Blindsight and exclusive access to Shadow Step.', 1)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/hunter-nin/option/hunters-patterns/martial-student', 'class/hunter-nin', NULL,
		 'Hunters Patterns', 'Martial Student', 'Use Dex for Taijutsu/Bukijutsu.', 1),
		('class/hunter-nin/option/hunters-patterns/kleptomaniac', 'class/hunter-nin', NULL,
		 'Hunters Patterns', 'Kleptomaniac', 'Sleight of hand shenanigans.', 2),
		('class/hunter-nin/option/hunters-exploits/aim', 'class/hunter-nin', NULL,
		 'Hunters Exploits', 'Aim', 'A generic, unlocked exploit.', 1),
		('class/hunter-nin/option/hunters-exploits/shadow-step', 'class/hunter-nin', NULL,
		 'Hunters Exploits', 'Shadow Step', 'Requirements: Grave Stalker Subclass. Teleport through darkness.', 2),
		('class/hunter-nin/option/hunters-exploits/wardens-assault', 'class/hunter-nin', NULL,
		 'Hunters Exploits', 'Wardens Assault', 'Requirements: Blade Warden Subclass. Double Lethal Attack trigger.', 3)`,
	); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHunterTechniquesTabData(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Sakura', 8, 14, 12, 10, 15, 13)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/hunter-nin', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/hunter-nin/group/hunters-creeds/grave-stalker', 3)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.loadHunterTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadHunterTechniquesTabData returned nil for a real Hunter-Nin")
	}
	if data.LethalAttackDice != "5d8" {
		t.Errorf("LethalAttackDice = %q, want 5d8 (level 10 chart)", data.LethalAttackDice)
	}
	if data.PatternsCap != 2 {
		t.Errorf("PatternsCap = %d, want 2 (level 10)", data.PatternsCap)
	}
	if len(data.AvailablePatterns) != 2 {
		t.Fatalf("AvailablePatterns = %+v, want both seeded patterns before any pick", data.AvailablePatterns)
	}
	if data.ExploitsCap != 3 {
		t.Errorf("ExploitsCap = %d, want 3 (level 10)", data.ExploitsCap)
	}
	if data.DefensiveTacticsCap != 1 {
		t.Errorf("DefensiveTacticsCap = %d, want 1 (level 10)", data.DefensiveTacticsCap)
	}
	if len(data.AvailableDefensiveTactics) != 4 {
		t.Errorf("AvailableDefensiveTactics = %+v, want all 4 hand-curated options before any pick", data.AvailableDefensiveTactics)
	}

	// Grave Stalker's own Shadow Stalker feature grants Shadow Step for
	// free, outside the cap, and Shadow Step must not appear in Available.
	if len(data.KnownExploits) != 1 || data.KnownExploits[0].Name != "Shadow Step" || !data.KnownExploits[0].Granted {
		t.Fatalf("KnownExploits = %+v, want exactly Shadow Step, Granted", data.KnownExploits)
	}
	if data.ExploitsUsed != 0 {
		t.Errorf("ExploitsUsed = %d, want 0 — the free grant must not count against the cap", data.ExploitsUsed)
	}
	for _, o := range data.AvailableExploits {
		if o.Name == "Shadow Step" {
			t.Error("Shadow Step should not appear in AvailableExploits once auto-granted")
		}
		if o.Name == "Wardens Assault" {
			t.Error("Wardens Assault (Blade Warden-exclusive) should never appear for a Grave Stalker")
		}
	}
	if len(data.AvailableExploits) != 1 || data.AvailableExploits[0].Name != "Aim" {
		t.Fatalf("AvailableExploits = %+v, want only the unlocked Aim exploit", data.AvailableExploits)
	}

	// Learn a Hunters Pattern and confirm the known/available split updates.
	if err := charstore.AddHunterNinPick(s.charDB, 1, charstore.HunterPickPattern, "class/hunter-nin/option/hunters-patterns/kleptomaniac"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadHunterTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.PatternsUsed != 1 || len(data.KnownPatterns) != 1 || len(data.AvailablePatterns) != 1 {
		t.Errorf("after learning Kleptomaniac: PatternsUsed=%d Known=%+v Available=%+v", data.PatternsUsed, data.KnownPatterns, data.AvailablePatterns)
	}
}

func TestHuntersArchetypeGrants(t *testing.T) {
	cases := []struct {
		name           string
		slugs          []string
		wantSlots      int
		wantLethalDice string
	}{
		{"no archetype feats", nil, 0, ""},
		// Enthusiast alone grants neither a slot nor a die — this shape
		// never occurs under the real prerequisite chain (Enthusiast
		// requires Expert, which requires Training), but the function must
		// still report "nothing" for it so the caller's gate stays closed.
		{"enthusiast alone", []string{huntersEnthusiastFeatSlug}, 0, ""},
		{"training only", []string{huntersTrainingFeatSlug}, 1, "2d6"},
		{"training + expert", []string{huntersTrainingFeatSlug, huntersExpertFeatSlug}, 2, "4d6"},
		{
			"training + expert + specialist",
			[]string{huntersTrainingFeatSlug, huntersExpertFeatSlug, hunterSpecialistFeatSlug},
			3, "6d6",
		},
		{
			"full chain including enthusiast",
			[]string{huntersTrainingFeatSlug, huntersExpertFeatSlug, hunterSpecialistFeatSlug, huntersEnthusiastFeatSlug},
			3, "6d6",
		},
		{
			"expert + enthusiast, no specialist",
			[]string{huntersTrainingFeatSlug, huntersExpertFeatSlug, huntersEnthusiastFeatSlug},
			2, "4d6",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rows []grantedFeatureRow
			for _, slug := range c.slugs {
				rows = append(rows, grantedFeatureRow{Slug: slug})
			}
			slots, dice := huntersArchetypeGrants(rows)
			if slots != c.wantSlots || dice != c.wantLethalDice {
				t.Errorf("huntersArchetypeGrants(%v) = (%d, %q), want (%d, %q)",
					c.slugs, slots, dice, c.wantSlots, c.wantLethalDice)
			}
		})
	}
}

// TestLoadHunterTechniquesTabDataArchetypeFeatsNoRealLevel exercises the
// class-level gate widened to also open for the 4 archetype-training
// feats — a character with zero real Hunter-Nin levels but Hunters
// Training + Hunters Expert must still get a rendered Hunter Techniques
// box: Lethal Attack fixed at 4d6 (Expert's tier), 2 Exploit slots (one
// each from Training and Expert), and neither Patterns nor Defensive
// Tactics (neither feat touches those).
func TestLoadHunterTechniquesTabDataArchetypeFeatsNoRealLevel(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kakashi', 8, 15, 12, 10, 14, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_feats (character_id, feat_slug, source) VALUES
		(1, ?, 'asi'), (1, ?, 'asi')`, huntersTrainingFeatSlug, huntersExpertFeatSlug,
	); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.loadHunterTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadHunterTechniquesTabData returned nil for a character with archetype-training feats and no real Hunter-Nin levels")
	}
	if data.LethalAttackDice != "4d6" {
		t.Errorf("LethalAttackDice = %q, want 4d6 (Hunters Expert's flat, non-scaling tier)", data.LethalAttackDice)
	}
	if data.ExploitsCap != 2 {
		t.Errorf("ExploitsCap = %d, want 2 (1 from Hunters Training + 1 from Hunters Expert)", data.ExploitsCap)
	}
	if data.PatternsCap != 0 {
		t.Errorf("PatternsCap = %d, want 0 — neither feat grants Hunters Patterns", data.PatternsCap)
	}
	if data.DefensiveTacticsCap != 0 {
		t.Errorf("DefensiveTacticsCap = %d, want 0 — neither feat grants Defensive Tactics", data.DefensiveTacticsCap)
	}
	// Of the 3 seeded exploits, Shadow Step and Wardens Assault are each
	// locked to a specific Hunter's Creed subclass this feat-only character
	// (no subclass at all) doesn't have — only the unlocked Aim exploit
	// should be open, same filtering a real subclassless Hunter-Nin gets.
	if len(data.AvailableExploits) != 1 || data.AvailableExploits[0].Name != "Aim" {
		t.Errorf("AvailableExploits = %+v, want only the unlocked Aim exploit", data.AvailableExploits)
	}
}

// TestLoadHunterTechniquesTabDataNoLevelNoFeat confirms the box still stays
// hidden (nil) for a character with neither real Hunter-Nin levels nor any
// archetype-training feat — the widened gate must not open unconditionally.
func TestLoadHunterTechniquesTabDataNoLevelNoFeat(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Iruka', 8, 12, 12, 12, 12, 8)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadHunterTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Errorf("loadHunterTechniquesTabData = %+v, want nil for a character with no Hunter-Nin levels and no archetype feat", data)
	}
}

func TestWardenWeaponCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 1}, {9, 1}, {10, 1}, {20, 1},
	}
	for _, c := range cases {
		if got := wardenWeaponCap(c.level); got != c.want {
			t.Errorf("wardenWeaponCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestSubclassTechniqueCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 1}, {9, 1}, {10, 2}, {20, 2},
	}
	for _, c := range cases {
		if got := subclassTechniqueCap(c.level); got != c.want {
			t.Errorf("subclassTechniqueCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestArsenalItemCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 4}, {9, 4}, {10, 6}, {20, 6},
	}
	for _, c := range cases {
		if got := arsenalItemCap(c.level); got != c.want {
			t.Errorf("arsenalItemCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestProstheticAttachmentCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 2}, {9, 2}, {10, 4}, {20, 4},
	}
	for _, c := range cases {
		if got := prostheticAttachmentCap(c.level); got != c.want {
			t.Errorf("prostheticAttachmentCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestWolfTechniqueCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {9, 0}, {10, 1}, {17, 1}, {20, 1},
	}
	for _, c := range cases {
		if got := wolfTechniqueCap(c.level); got != c.want {
			t.Errorf("wolfTechniqueCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestStepWeaponDie(t *testing.T) {
	cases := []struct {
		dice string
		want string
	}{
		{"1d4", "1d6"},
		{"1d6", "1d8"},
		{"1d8", "1d10"},
		{"1d10", "1d12"},
		{"1d12", "1d12"}, // no defined next step — returned unchanged
		{"2d6", "2d8"},   // multi-die count is preserved
		{"", ""},         // malformed input returned unchanged, not guessed at
		{"not-dice", "not-dice"},
	}
	for _, c := range cases {
		if got := stepWeaponDie(c.dice); got != c.want {
			t.Errorf("stepWeaponDie(%q) = %q, want %q", c.dice, got, c.want)
		}
	}
}

// seedBladeWardenSubclassFeatures adds the 3 real Blade Warden
// subclass_features rows this file's own bonuses key off of (Warden's
// Proficiency at 3rd, Aggressive Attack at 7th, Blade's Aggression at
// 10th) plus one simple, non-Two-Handed/non-Heavy weapon so
// loadWardenWeaponCatalog has a real candidate. Callers must already have
// called seedHunterNinLevelResources, which seeds the blade-warden
// subclass row itself.
func seedBladeWardenSubclassFeatures(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/hunter-nin/group/hunters-creeds/blade-warden/feature/wardens-proficiency',
		 'class/hunter-nin/group/hunters-creeds/blade-warden', 'Wardens Proficiency', 3, 'Choose a Warden Weapon.', 1),
		('class/hunter-nin/group/hunters-creeds/blade-warden/feature/aggressive-attack',
		 'class/hunter-nin/group/hunters-creeds/blade-warden', 'Aggressive Attack', 7,
		 'Add both Strength and Dexterity to damage with your Warden Weapon.', 2),
		('class/hunter-nin/group/hunters-creeds/blade-warden/feature/blades-aggression',
		 'class/hunter-nin/group/hunters-creeds/blade-warden', 'Blades Aggression', 10,
		 'Your Warden Weapons damage die is increased by 1 step.', 3)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, weapon_category, damage_dice, damage_type, properties)
		VALUES ('weapon/kunai', 'Kunai', 'weapon', 'simple', '1d4', 'Piercing', 'Light, Finesse, Thrown (30/60), Multiattack, Ammunition')`,
	); err != nil {
		t.Fatal(err)
	}
}

// TestLoadHunterTechniquesTabDataBladeWardenPicks confirms the Warden
// Weapon TYPE and Warden Weapon Property sub-sections only ever appear for
// a Blade Warden, and that every other subclass-exclusive sub-section
// (e.g. Medical Assassination Technique, Arsenalist only) stays hidden for
// a Blade Warden the same way DefensiveTacticsCap et al. already stay
// hidden for the wrong level.
func TestLoadHunterTechniquesTabDataBladeWardenPicks(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	seedBladeWardenSubclassFeatures(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Zabuza', 16, 14, 12, 10, 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/hunter-nin', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/hunter-nin/group/hunters-creeds/blade-warden', 3)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadHunterTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.WardenWeaponCap != 1 {
		t.Errorf("WardenWeaponCap = %d, want 1", data.WardenWeaponCap)
	}
	if len(data.AvailableWardenWeapon) != 1 || data.AvailableWardenWeapon[0].Slug != "weapon/kunai" {
		t.Errorf("AvailableWardenWeapon = %+v, want just the seeded Kunai", data.AvailableWardenWeapon)
	}
	if data.WardenWeaponPropertyCap != 2 {
		t.Errorf("WardenWeaponPropertyCap = %d, want 2 (level 10)", data.WardenWeaponPropertyCap)
	}
	if len(data.AvailableWardenWeaponProperty) != 3 {
		t.Errorf("AvailableWardenWeaponProperty = %+v, want all 3 (Flurry Attack/Mortal/Fatal)", data.AvailableWardenWeaponProperty)
	}
	// Every OTHER subclass-exclusive sub-section must stay hidden for a
	// Blade Warden.
	if data.MedicalTechniqueCap != 0 || data.ShadowTechniqueCap != 0 || data.ArsenalItemCap != 0 ||
		data.ToxicTechniqueCap != 0 || data.ViceTechniqueCap != 0 || data.VoidTechniqueCap != 0 ||
		data.ProstheticAttachmentCap != 0 {
		t.Errorf("a Blade Warden must not see any other Creed's own sub-section: %+v", data)
	}

	// Learn the Kunai as the Warden Weapon and confirm the known/available
	// split updates.
	if err := charstore.AddHunterNinPick(s.charDB, 1, charstore.HunterPickWardenWeapon, "weapon/kunai"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadHunterTechniquesTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.WardenWeaponUsed != 1 || len(data.KnownWardenWeapon) != 1 || len(data.AvailableWardenWeapon) != 0 {
		t.Errorf("after learning Kunai as Warden Weapon: Used=%d Known=%+v Available=%+v",
			data.WardenWeaponUsed, data.KnownWardenWeapon, data.AvailableWardenWeapon)
	}
}

// TestBuildAttacksWardenWeaponBonuses confirms buildAttacks applies both
// Blade Warden bonuses only to the equipped weapon matching the character's
// own Warden Weapon pick: Aggressive Attack (7th level) swaps the single-
// ability damage sum for both Strength AND Dexterity, and Blade's
// Aggression (10th) separately steps the damage die up by 1.
func TestBuildAttacksWardenWeaponBonuses(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	seedBladeWardenSubclassFeatures(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Zabuza', 16, 14, 12, 10, 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/hunter-nin', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/hunter-nin/group/hunters-creeds/blade-warden', 3)`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'weapon/kunai', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := res.LastInsertId(); err != nil {
		t.Fatal(err)
	}
	if err := charstore.AddHunterNinPick(s.charDB, 1, charstore.HunterPickWardenWeapon, "weapon/kunai"); err != nil {
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

	// Str 16 (+3) + Dex 14 (+2) = +5, instead of Finesse's usual "better of
	// the two" single modifier.
	if row.DamageBonus != 5 {
		t.Errorf("DamageBonus = %d, want 5 (Str +3 and Dex +2 both applied by Aggressive Attack)", row.DamageBonus)
	}
	// Kunai's own 1d4 stepped up once by Blade's Aggression.
	if row.DamageDice != "1d6" {
		t.Errorf("DamageDice = %q, want 1d6 (Kunai's 1d4 stepped up by Blade's Aggression)", row.DamageDice)
	}
	if row.DamageCount != 1 || row.DamageSides != 6 {
		t.Errorf("DamageCount/DamageSides = %d/%d, want 1/6", row.DamageCount, row.DamageSides)
	}
}

// TestBuildAttacksArsenalWeaponBonus confirms buildAttacks applies
// Arsenalist's own Arsenal Weapons clause (the tail of Arsenal's
// Proficiency, 3rd level) to any equipped weapon whose properties match a
// picked Arsenal keyword ("thrown"/"multiattack"/"light"/"finesse"): the
// damage die steps up by 1 and a flat, level-gated damage bonus (+2/+4/+6
// at 3rd/10th/17th) is added. The Hidden Weapon property tag and the
// Lethal-Attack trigger override stay manual/narrated — only the die-step
// and flat bonus are exercised here.
func TestBuildAttacksArsenalWeaponBonus(t *testing.T) {
	s := testServer(t)
	seedHunterNinLevelResources(t, s)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, weapon_category, damage_dice, damage_type, properties)
		VALUES ('weapon/kunai', 'Kunai', 'weapon', 'simple', '1d4', 'Piercing', 'Light, Finesse, Thrown (30/60), Multiattack, Ammunition')`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Sasori', 10, 16, 12, 10, 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/hunter-nin', 10, 0)`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'weapon/kunai', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := res.LastInsertId(); err != nil {
		t.Fatal(err)
	}
	if err := charstore.AddHunterNinPick(s.charDB, 1, charstore.HunterPickArsenalItem, "thrown-weapons"); err != nil {
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

	// Kunai is Finesse; Dex 16 (+3) beats Str 10 (+0), plus the flat +4
	// Arsenal Weapons bonus at 10th Hunter-Nin level.
	if row.DamageBonus != 7 {
		t.Errorf("DamageBonus = %d, want 7 (Dex +3 plus Arsenal Weapons' +4 at 10th level)", row.DamageBonus)
	}
	// Kunai's own 1d4 stepped up once by Arsenal Weapons.
	if row.DamageDice != "1d6" {
		t.Errorf("DamageDice = %q, want 1d6 (Kunai's 1d4 stepped up by Arsenal Weapons)", row.DamageDice)
	}
	if row.DamageCount != 1 || row.DamageSides != 6 {
		t.Errorf("DamageCount/DamageSides = %d/%d, want 1/6", row.DamageCount, row.DamageSides)
	}
}
