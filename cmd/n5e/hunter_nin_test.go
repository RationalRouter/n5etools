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
