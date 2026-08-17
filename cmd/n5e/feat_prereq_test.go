package main

import "testing"

// The prerequisite strings below are copied verbatim out of rules.db
// (SELECT DISTINCT prerequisites FROM feats), truncations and all — the
// parser's whole job is coping with the book's prose as it actually is.
func TestParseFeatPrereq(t *testing.T) {
	skills := map[string]bool{"chakra control": true, "medicine": true, "ninshou": true}
	feats := map[string]bool{"medical expert": true, "chef": true, "weapon arts training": true}
	classes := map[string]bool{"cooking-nin": true, "medical-nin": true, "taijutsu specialist": true}
	clans := map[string]bool{"hyūga": true, "aburame": true, "akimichi": true}

	base := featCharacter{
		Level:       10,
		Clan:        "hyūga",
		ClassLevels: map[string]int{"cooking-nin": 6},
		Abilities:   map[string]int{"str": 16, "dex": 12, "con": 10, "int": 14, "wis": 8, "cha": 20},
		Skills:      map[string]bool{"chakra control": true},
		AllSkills:   skills,
		Feats:       map[string]bool{"medical expert": true},
	}

	cases := []struct {
		prereq string
		want   bool
		why    string
	}{
		{"Level 8+", true, "level 10 clears an 8+ gate"},
		{"Level 12+", false, "level 10 does not reach 12"},
		{"Chef Trainee, Level 10.", true, "trailing period on the level, unknown feat name ignored"},
		{"Hyūga Clan, Level 4+", true, "right clan, level met"},
		{"Aburame Clan, Level 4+", false, "wrong clan"},
		{"At least 4+ levels in Cooking-Nin.", true, "6 levels in the class"},
		{"At least 8+ levels in Cooking-Nin.", false, "only 6 levels in the class"},
		{"Level 5+, Dexterity 15+, You cannot have class levels in Hunter-Nin", false, "DEX 12 fails"},
		{"Level 5+, Strength or Dexterity 15+, You cannot have class levels in Weapon Specialist", true, "STR 16 satisfies the any-of"},
		{"Intelligence 15 +, Level 5+, You cannot have Levels in the Cooking-Nin Class.", false, "has levels in the forbidden class"},
		{"Charisma 20, Level 10+", true, "no plus sign on the score"},
		{"Level 5+, Intelligence 14+ or Wisdom 14+", true, "INT 14 satisfies the any-of"},
		{"Proficiency in Chakra Control, Level 8+", true, "proficient in the named skill"},
		{"Proficiency in Medicine & Chakra Control.", true, "any-of over &, Chakra Control matches"},
		{"Proficiency in either Ninshou or Martial", false, "Ninshou is a real skill and isn't proficient"},
		{"Medical Expert, Level 15+", false, "feat held but level 15 not reached"},
		{"Medical Expert", true, "feat already taken"},
		{"Weapon Arts Training, Level 10+", false, "prerequisite feat not taken"},
		// The parser must not invent requirements out of prose it cannot
		// read. Both of these are truncated in the source PDF.
		{"Level 5+, Any two of the following three ability scores must be 14+ (Strength, Dexterity,", true, "unreadable clause ignored, Level 5+ still enforced"},
		{"Hoshigaki Clan, Wielder of a Legend,", false, "clan clause still applies to a truncated line"},
		{"8-Inner Gates: Seimon, Taijutsu Ability Score of 18+, Level 10+.", true, "only the level clause is readable"},
		{"", true, "no prerequisites at all"},

		// Bare class/clan name clauses — no "at least N+ levels in" or
		// trailing "Clan" wrapper, just the name on its own (real examples:
		// "Bad Medicine" is "Medical-Nin, Level 8+"; "Food Born Hardiness"
		// is just "Akimichi"). These must gate on actually belonging to
		// that class/clan, not silently reduce to only the Level clause.
		{"Medical-Nin, Level 8+", false, "bare class name: character has 0 levels in Medical-Nin"},
		{"Cooking-Nin, Level 8+", true, "bare class name: character has 6 levels in Cooking-Nin, any amount satisfies the bare form"},
		{"Akimichi", false, "bare clan name: character is Hyūga, not Akimichi"},
		{"Hyūga", true, "bare clan name, no Level clause at all, matching clan"},
		{"Taijutsu Specialist, Level 8+", false, "bare class name for a class the character has 0 levels in, even though Level 8+ alone would pass"},
	}

	for _, tc := range cases {
		got := base.Meets(parseFeatPrereq(tc.prereq, skills, feats, classes, clans))
		if got != tc.want {
			t.Errorf("Meets(%q) = %v, want %v (%s)", tc.prereq, got, tc.want, tc.why)
		}
	}
}

func TestParseFeatPrereqClauseSplitting(t *testing.T) {
	// The parenthesised list must stay in one clause — splitting inside it
	// would turn one ignorable clause into three bogus ones.
	got := splitPrereqClauses("Level 5+, must be 14+ (Strength, Dexterity, Constitution)")
	if len(got) != 2 {
		t.Fatalf("splitPrereqClauses = %q, want 2 clauses", got)
	}
	if got[0] != "Level 5+" {
		t.Errorf("first clause = %q", got[0])
	}
}
