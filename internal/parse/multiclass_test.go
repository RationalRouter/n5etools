package parse

import (
	"strings"
	"testing"
)

// Fixture mirroring the real tables' quirks: a value that wraps onto its
// own line ("Constitution" / "14"), the "Intelligence"/"Operative" name
// split (present in more than one table), the "Cooking Nin"/"Cooking-Nin"
// spelling inconsistency, and the caps sub-headings that separate the
// tables from surrounding prose.
func TestParseMulticlassRulesFixture(t *testing.T) {
	lines := mkLines(179,
		"MULTICLASSING",
		"Multiclassing lets you gain levels in multiple classes.",
		"PREREQUISITES",
		"To qualify you must meet ability score prerequisites.",
		"Class Ability Score Minimum",
		"Genjutsu Specialist Wisdom & Charisma 14",
		"Hunter-Nin Dexterity 14 & Intelligence 14",
		"Intelligence Operative Intelligence 14 & Charisma 14",
		"Medical-Nin Intelligence 14 & Wisdom 14",
		"Ninjutsu Specialist Constitution 14 & Intelligence 14",
		"Scout-Nin Strength 13 & Intelligence 13 & Wisdom 13",
		"Taijutsu Specialist Strength or Dexterity 14 & Constitution",
		"14",
		"Weapon Specialist Strength 14 & Dexterity 14",
		"Puppet Master Constitution & Intelligence 14",
		"Cooking Nin Intelligence 14 & Charisma 14",
		"Science Nin Intelligence 16",
		"Class Proficiencies Gained",
		"Genjutsu Specialist Deception and Illusions",
		"Hunter-Nin Light armor, trackers kit.",
		"Intelligence",
		"Operative Light and medium armor; One Ninja Tool Kit.",
		"Medical-Nin Light armor; Medicine Kit; Medicine skill",
		"Ninjutsu Specialist Ninshou and Chakra Control Skill",
		"Scout-Nin Light and Medium armor, one skill.",
		"Taijutsu Specialist Combat Bracers, Iron Claw.",
		"Weapon Specialist All Armors; Simple & Martial Weapons",
		"Puppet Master Light Armor, Weaponsmith kit.",
		"Cooking Nin Intelligence 14 & Charisma 14",
		"Science Nin Light Armor, Ninshou skill, one tool kit",
		"CLASS FEATURES",
		"A few features have additional rules when multiclassing.",
		"JUTSU MODIFIERS",
		"You only follow the Jutsu Save DC of your highest-level class.",
		"Class Jutsu Gained Per level",
		"Genjutsu Specialist +1 Every 2 Levels.",
		"Hunter-Nin +1 Every 3 Levels.",
		"Intelligence Operative +1 Every 2 Levels.",
		"Medical-Nin +1 Every Level.",
		"Ninjutsu Specialist +1 Every Level.",
		"Scout-Nin +1 Every 2 Levels.",
		"Taijutsu Specialist +1 Every 3 Levels.",
		"Weapon Specialist +1 Every 3 Levels.",
		"Puppet Master +1 Every 2 Levels.",
		"Cooking-Nin +1 Every 2 Levels.",
		"Science-Nin +1 Every 2 Levels.",
	)
	rules, anomalies := ParseMulticlassRules(lines)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(rules) != 11 {
		t.Fatalf("got %d rules, want 11: %+v", len(rules), rules)
	}

	byName := map[string]MulticlassRule{}
	for _, r := range rules {
		byName[r.ClassName] = r
	}

	taijutsu := byName["Taijutsu Specialist"]
	if taijutsu.AbilityPrereq != "Strength or Dexterity 14 & Constitution 14" {
		t.Errorf("wrapped value line not joined: %q", taijutsu.AbilityPrereq)
	}
	io := byName["Intelligence Operative"]
	if io.ProficienciesGained != "Light and medium armor; One Ninja Tool Kit." {
		t.Errorf("split class name row: %q", io.ProficienciesGained)
	}
	cooking := byName["Cooking Nin"]
	if cooking.JutsuPerLevel != "+1 Every 2 Levels." {
		t.Errorf("hyphenated spelling variant not matched: %+v", cooking)
	}
	science := byName["Science Nin"]
	if strings.Contains(science.ProficienciesGained, "CLASS FEATURES") ||
		strings.Contains(science.ProficienciesGained, "JUTSU MODIFIERS") {
		t.Errorf("caps sub-heading leaked into last row of table: %q", science.ProficienciesGained)
	}
}

// Whole-book regression (verified 2026-07-18 against v3.11): 11 rows, all
// three columns populated, zero anomalies.
func TestParseMulticlassRulesFullCorebook(t *testing.T) {
	lines := loadCoreBookLines(t)
	if lines == nil {
		return
	}
	rules, anomalies := ParseMulticlassRules(lines)
	if len(anomalies) != 0 {
		t.Errorf("anomalies: %+v", anomalies)
	}
	if len(rules) != 11 {
		t.Fatalf("got %d rules, want 11", len(rules))
	}
	for _, r := range rules {
		if r.AbilityPrereq == "" || r.ProficienciesGained == "" || r.JutsuPerLevel == "" {
			t.Errorf("%s: incomplete row: %+v", r.ClassName, r)
		}
		// None of these prose blocks should ever swallow a heading from a
		// neighboring table or section.
		for _, bad := range []string{"CLASS FEATURES", "JUTSU MODIFIERS", "Class Ability Score Minimum",
			"Class Proficiencies Gained", "Class Jutsu Gained Per level"} {
			if strings.Contains(r.AbilityPrereq, bad) || strings.Contains(r.ProficienciesGained, bad) ||
				strings.Contains(r.JutsuPerLevel, bad) {
				t.Errorf("%s: leaked section text %q into a field: %+v", r.ClassName, bad, r)
			}
		}
	}
}
