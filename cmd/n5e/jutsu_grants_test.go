package main

import "testing"

func TestParseJutsuGrants_RankFirst(t *testing.T) {
	cases := []struct {
		name        string
		description string
		fallbackLvl int
		wantName    string
		wantLevel   int
	}{
		{
			name: "Enhanced Vision",
			description: "Starting at 6th level, you have fit your armor with a special " +
				"chakra visor that grants you 60 feet of Darkvision and doubles your " +
				"normal sight range. If you already have Darkvision, it is increased by " +
				"60 feet instead. You can accurately make out the details of things " +
				"within 1 mile of you. Also at 6th level, you learn the E-Rank Ninjutsu " +
				"Enhanced Defense. When you take the attack action, you can cast " +
				"Enhanced Defense at the conclusion of your attack action.",
			fallbackLvl: 6,
			wantName:    "Enhanced Defense",
			wantLevel:   6,
		},
		{
			name: "Battle Pressure",
			description: "Starting at 6th level, upon rolling initiative, during the " +
				"first round of combat, you and your Puppet gain a +15 bonuses to " +
				"speed, a +1 to AC, and advantage on the next Physical saving " +
				"throw/skill check you make this round. Also at 6th level, you learn " +
				"the E-Rank Ninjutsu Chakra Blow. When you take the Attack action, as " +
				"long as you hit with at least 1 attack with the Attack action, you can " +
				"cast Chakra Blow as part of the same action, affecting your final " +
				"attack that hits.",
			fallbackLvl: 6,
			wantName:    "Chakra Blow",
			wantLevel:   6,
		},
		{
			name: "Overture",
			description: "Also at 6th level, your control over your puppets is so " +
				"precise that even enemies get lost in the beauty of it. Twice per " +
				"turn, when both of your puppets damage to the same creature, you add " +
				"half your Puppet Master level to the damage. Also at 6th level, you " +
				"learn the E-Rank Ninjutsu Sealing Art: String Light Formation. When " +
				"you take the attack action, you can cast Sealing Art: String Light " +
				"Formation as part of the same action.",
			fallbackLvl: 6,
			wantName:    "Sealing Art: String Light Formation",
			wantLevel:   6,
		},
		{
			name: "Combat Alertness",
			description: "Also at 6th level, you are always on edge with your chakra " +
				"threads at the ready. You have advantage on initiative checks, and " +
				"you can connect your Chakra Threads to an allied Creature as part of " +
				"rolling initiative, you can also make one creature you are connected " +
				"to move up to their full movement speed. Also at 6th level, you learn " +
				"the E-Rank Ninjutsu Medical Release: Virtue. When you take the attack " +
				"action or command an ally to take the attack action, you can cast " +
				"Medical Release: Virtue as part of the same action, affecting all " +
				"creatures you are connected to.",
			fallbackLvl: 6,
			wantName:    "Medical Release: Virtue",
			wantLevel:   6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grants := parseJutsuGrants(tc.description, tc.fallbackLvl)
			if len(grants) != 1 {
				t.Fatalf("parseJutsuGrants(%q) = %d grants, want 1: %+v", tc.name, len(grants), grants)
			}
			if grants[0].Name != tc.wantName {
				t.Errorf("Name = %q, want %q", grants[0].Name, tc.wantName)
			}
			if grants[0].Level != tc.wantLevel {
				t.Errorf("Level = %d, want %d", grants[0].Level, tc.wantLevel)
			}
		})
	}
}

// TestParseJutsuGrants_NameFirstStillWorks is the negative case: a
// name-first grant (Hoshigaki's Commander of the Deep) must still resolve
// via the pre-existing jutsuGrantWithLevelPattern, unaffected by the new
// rank-first pattern.
func TestParseJutsuGrants_NameFirstStillWorks(t *testing.T) {
	description := "Beginning at 1st level, aquatic creatures have an affinity with " +
		"people of your clan. You can communicate simple ideas with beasts that can " +
		"breathe Water. They can understand the meaning of your words, though you " +
		"have no special ability to control them directly. Starting at 7th level you " +
		"learn the Summoning Technique D-Rank ninjutsu with a contract with the Shark " +
		"tribe. You can cast this jutsu ignoring Ability Score Requirements."

	grants := parseJutsuGrants(description, 1)
	if len(grants) != 1 {
		t.Fatalf("parseJutsuGrants(commander-of-the-deep) = %d grants, want 1: %+v", len(grants), grants)
	}
	if grants[0].Name != "Summoning Technique" {
		t.Errorf("Name = %q, want %q", grants[0].Name, "Summoning Technique")
	}
	if grants[0].Level != 7 {
		t.Errorf("Level = %d, want 7 (own restated level, not the fallback of 1)", grants[0].Level)
	}
}

// TestParseJutsuGrants_GainPhrasingStillWorks covers the Genjutsu Pledge
// subclasses' 2nd-level free-jutsu features, which are printed with "you
// gain the E-Rank Genjutsu X" rather than "you learn" — 6 of the 7 pledges
// use this phrasing (only Corrupt Thoughts has no such grant at all).
// Sentences pulled verbatim from subclass_features in rules.db.
func TestParseJutsuGrants_GainPhrasingStillWorks(t *testing.T) {
	cases := []struct {
		name        string
		description string
		wantName    string
	}{
		{
			name: "Beguiler - Inspired Appearance",
			description: "When you choose this path at 2nd level, you gain the E-Rank " +
				"Genjutsu Transform. If you already know this Genjutsu, you gain another " +
				"E-Rank Genjutsu you qualify for. You can cast Transform at 0 Cost, as a " +
				"Bonus Action.",
			wantName: "Transform",
		},
		{
			name: "Illusionist - Shaping Your World",
			description: "When you choose this path at 2nd Level, you gain the E-Rank " +
				"Genjutsu, Minor Illusion. If you already know this genjutsu, you learn a " +
				"different E-Rank genjutsu of your choice. The genjutsu you learn this way " +
				"does not count against your number of jutsu known.",
			wantName: "Minor Illusion",
		},
		{
			name: "Siren - Alluring Words",
			description: "When you choose this path at 2nd level, you gain the E-Rank " +
				"Genjutsu Affection, if you already know this Genjutsu, you gain another " +
				"E-Rank Genjutsu you qualify for. The Genjutsu you learn this way does not " +
				"count against your Jutsu known.",
			wantName: "Affection",
		},
		{
			name: "Time Slipper - Temporal Shift",
			description: "When you choose this path at 2nd level, you gain the E-Rank " +
				"Genjutsu Feather Burst, if you already know this Genjutsu, you gain " +
				"another E-Rank Genjutsu you qualify for.",
			wantName: "Feather Burst",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grants := parseJutsuGrants(tc.description, 2)
			if len(grants) != 1 {
				t.Fatalf("parseJutsuGrants(%q) = %d grants, want 1: %+v", tc.name, len(grants), grants)
			}
			if grants[0].Name != tc.wantName {
				t.Errorf("Name = %q, want %q", grants[0].Name, tc.wantName)
			}
			if grants[0].Level != 2 {
				t.Errorf("Level = %d, want 2 (fallback level)", grants[0].Level)
			}
		})
	}
}

// TestParseJutsuGrants_NoRankPhrasing covers Systematic Breakdown
// (Intelligence Operative, Interrogationist), whose free-jutsu grant states
// neither a rank token nor a restated level — "you learn the Bane
// Genjutsu, if you do not know it already." — a shape none of the other
// three patterns can match.
func TestParseJutsuGrants_NoRankPhrasing(t *testing.T) {
	description := "Also, at 3rd level, Genjutsu you cast targeting a creature marked " +
		"by your Exploit Weakness, can use your Charisma Modifier instead of Wisdom. " +
		"Finally, you learn the Bane Genjutsu, if you do not know it already."

	grants := parseJutsuGrants(description, 3)
	if len(grants) != 1 {
		t.Fatalf("parseJutsuGrants(systematic-breakdown) = %d grants, want 1: %+v", len(grants), grants)
	}
	if grants[0].Name != "Bane" {
		t.Errorf("Name = %q, want %q", grants[0].Name, "Bane")
	}
	if grants[0].Level != 3 {
		t.Errorf("Level = %d, want 3 (fallback level)", grants[0].Level)
	}
}

// TestParseJutsuGrants_NoRankPatternDoesNotDoubleMatchRankedGrants confirms
// the new rank-agnostic pattern doesn't add a second, wrongly-named grant
// for a sentence one of the other three patterns already resolves
// correctly — the case that motivates rankTokenSuffixPattern.
func TestParseJutsuGrants_NoRankPatternDoesNotDoubleMatchRankedGrants(t *testing.T) {
	description := "Beginning at 1st level, you craft a Puppet Tool to carry out your " +
		"orders and protect you. You learn the Mending E-Rank Ninjutsu, which does " +
		"not count against your known."

	grants := parseJutsuGrants(description, 1)
	if len(grants) != 1 {
		t.Fatalf("parseJutsuGrants(puppet-tool) = %d grants, want 1: %+v", len(grants), grants)
	}
	if grants[0].Name != "Mending" {
		t.Errorf("Name = %q, want %q (not a garbled \"Mending E-Rank\")", grants[0].Name, "Mending")
	}
}

// TestParseJutsuGrants_GenericRankNoSchoolIgnored confirms a "you learn
// E-Rank jutsu" grant with no specific jutsu name and no school keyword
// (Science-Nin's Chakra Cell Enhancement) does not get mistakenly matched
// by the new rank-first pattern, which requires one of the four school
// keywords immediately after the rank.
func TestParseJutsuGrants_GenericRankNoSchoolIgnored(t *testing.T) {
	description := "Also at Level 1, you have undergone the first step all Science " +
		"Nin take when they begin their studies; Chakra Cell Enhancement, a genetic " +
		"modification to improve and better control your chakra flow. You learn " +
		"E-Rank jutsu equal to your Intelligence Modifer. You can change these jutsu " +
		"over the course of a rest"

	grants := parseJutsuGrants(description, 1)
	if len(grants) != 0 {
		t.Fatalf("parseJutsuGrants(chakra-cell-enhancement) = %d grants, want 0: %+v", len(grants), grants)
	}
}
