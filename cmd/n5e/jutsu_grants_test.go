package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
)

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

// TestParseJutsuGrants_AddPhrasing covers Scout-Nin's Tajū Kage Bunshin
// (class/scout-nin/group/scouting-technique/cloning-scout/feature/
// taju-kage-bunshin), the one feature in the book phrased "you add the X
// Ninjutsu to your known jutsu list" rather than "you learn"/"you gain" —
// rank-less like TestParseJutsuGrants_NoRankPhrasing above, so it's
// jutsuGrantNoRankPattern's verb alternation being exercised here, not a
// new pattern.
func TestParseJutsuGrants_AddPhrasing(t *testing.T) {
	description := "Starting at 3rd Level, you add the Shadow Clone Technique Ninjutsu " +
		"to your known jutsu list and can learn any Ninjutsu with the Clone keyword, " +
		"excluding jutsu with the Hijutsu keyword."

	grants := parseJutsuGrants(description, 3)
	if len(grants) != 1 {
		t.Fatalf("parseJutsuGrants(taju-kage-bunshin) = %d grants, want 1: %+v", len(grants), grants)
	}
	if grants[0].Name != "Shadow Clone Technique" {
		t.Errorf("Name = %q, want %q", grants[0].Name, "Shadow Clone Technique")
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

// TestParseJutsuGrants_NecroticHandAliasResolves covers Necrotic Hand's
// Medical Proficiency (class/hunter-nin/group/hunters-creeds/necrotic-hand/
// feature/medical-proficiency, 3rd level), whose free-jutsu grant names the
// shorthand "Necrosis" — the jutsu's own printed name in the rules database
// is "Medical Release: Necrosis" (jutsu/medical-release-necrosis), so the
// parsed grant name only resolves via jutsuGrantNameAliases, not a direct
// jutsuNameIndex lookup. Sentence pulled verbatim from subclass_features in
// rules.db.
func TestParseJutsuGrants_NecroticHandAliasResolves(t *testing.T) {
	description := "At 3rd level, you gain proficiency in Medicine and Medicine Kits. " +
		"You can make any checks using the aforementioned skills with Intelligence or " +
		"Wisdom. Also, you have learned to take the teachings of the medical core of " +
		"your village or from your direct master and blend it into your assassination " +
		"techniques. You learn the Necrosis Ninjutsu. When you do, it loses the " +
		"Medical Keyword."

	grants := parseJutsuGrants(description, 3)
	if len(grants) != 1 {
		t.Fatalf("parseJutsuGrants(necrotic-hand medical-proficiency) = %d grants, want 1: %+v", len(grants), grants)
	}
	if grants[0].Name != "Necrosis" {
		t.Errorf("Name = %q, want %q", grants[0].Name, "Necrosis")
	}

	normalized := normalizeJutsuGrantName(grants[0].Name)
	slug, ok := jutsuGrantNameAliases[normalized]
	if !ok {
		t.Fatalf("jutsuGrantNameAliases[%q] missing, want jutsu/medical-release-necrosis", normalized)
	}
	if slug != "jutsu/medical-release-necrosis" {
		t.Errorf("jutsuGrantNameAliases[%q] = %q, want %q", normalized, slug, "jutsu/medical-release-necrosis")
	}
}

// TestWhiteTechniqueChakraStringBonusJutsuSlots covers White Technique
// Weaver's Chakra String Augments: +2 bonus known-jutsu slots at Puppet
// Master 2nd level, escalating by +1 at 6th, 10th, and 14th, for a cap of
// 5 — gated on the character's own Puppet Master class level, not total
// character level.
func TestWhiteTechniqueChakraStringBonusJutsuSlots(t *testing.T) {
	granted := []grantedFeatureRow{{Slug: whiteTechniqueChakraStringAugmentsFeatureSlug}}

	cases := []struct {
		puppetMasterLevel int
		want              int
	}{
		{2, 2},
		{5, 2},
		{6, 3},
		{9, 3},
		{10, 4},
		{13, 4},
		{14, 5},
		{20, 5},
	}
	for _, tc := range cases {
		if got := whiteTechniqueChakraStringBonusJutsuSlots(granted, tc.puppetMasterLevel); got != tc.want {
			t.Errorf("whiteTechniqueChakraStringBonusJutsuSlots(level %d) = %d, want %d", tc.puppetMasterLevel, got, tc.want)
		}
	}

	if got := whiteTechniqueChakraStringBonusJutsuSlots(nil, 14); got != 0 {
		t.Errorf("whiteTechniqueChakraStringBonusJutsuSlots(no feature) = %d, want 0", got)
	}
}

// TestNinjutsuFocusReleaseBonusJutsuSlots covers the five Ninjutsu Focus
// "[Element] Release" subclass features' own conditional +2 known-jutsu
// bonus — e.g. Blaze Walker's Fire Release: "If you can already do this you
// learn 2 additional Fire Release Ninjutsu that you qualify for." Only the
// bonus SLOT COUNT is tracked here; which 2 jutsu fill it is still the
// player's own pick via the known-jutsu picker. Mirrors
// TestWaterAndOilBonusJutsuSlots' own "no access from elsewhere" vs. "real
// clan affinity" cases, since both go through natureReleaseBonusJutsuSlots.
func TestNinjutsuFocusReleaseBonusJutsuSlots(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/ninjutsu-specialist', 'Ninjutsu Specialist', 8, 8)`)

	features := []grantedFeatureRow{{
		Slug: "class/ninjutsu-specialist/group/ninjutsu-focus/blaze-walker/feature/fire-release",
		Name: "Fire Release",
	}}

	// No Fire access from elsewhere: the feature's own flat affinity grant
	// (elemental_affinity.go's subclassFlatAffinity) is excluded from this
	// check by natureReleaseBonusJutsuSlots' own delete(grantedSlugs, ...)
	// step, so gaining the keyword fresh from this same feature earns
	// nothing extra.
	bonus, err := s.ninjutsuFocusReleaseBonusJutsuSlots(1, features, "class/ninjutsu-specialist", "")
	if err != nil {
		t.Fatalf("ninjutsuFocusReleaseBonusJutsuSlots (no other Fire access): %v", err)
	}
	if bonus != 0 {
		t.Errorf("bonus = %d, want 0 (no Fire access from elsewhere)", bonus)
	}

	// A real Fire affinity from elsewhere (an Uchiha clan trait) triggers
	// the flat +2.
	bonus, err = s.ninjutsuFocusReleaseBonusJutsuSlots(1, features, "class/ninjutsu-specialist", "clan/uchiha")
	if err != nil {
		t.Fatalf("ninjutsuFocusReleaseBonusJutsuSlots (Uchiha Fire affinity): %v", err)
	}
	if bonus != 2 {
		t.Errorf("bonus = %d, want 2 (flat +2 known jutsu via a real clan Fire affinity)", bonus)
	}

	noFeature, err := s.ninjutsuFocusReleaseBonusJutsuSlots(1, nil, "class/ninjutsu-specialist", "clan/uchiha")
	if err != nil {
		t.Fatalf("ninjutsuFocusReleaseBonusJutsuSlots (no feature): %v", err)
	}
	if noFeature != 0 {
		t.Errorf("bonus = %d, want 0 (character doesn't have any Release feature)", noFeature)
	}
}

// TestNinjutsuSpecializationBonusJutsuSlots covers the four Ninjutsu Focus
// "[Discipline] Specialization" subclass features' own unconditional flat
// bonus known-jutsu count — Hijutsu and Hemomantic Specialization grant +1
// each, Fuinjutsu and Void Specialization grant +2 each. Unlike the five
// Release features, none of these four are gated on already having access
// from elsewhere, so this is a plain flat-map lookup with no rulesDB
// interaction needed.
func TestNinjutsuSpecializationBonusJutsuSlots(t *testing.T) {
	if got := ninjutsuSpecializationBonusJutsuSlots(nil); got != 0 {
		t.Errorf("ninjutsuSpecializationBonusJutsuSlots(no features) = %d, want 0", got)
	}

	fuinjutsu := []grantedFeatureRow{{
		Slug: "class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/fuinjutsu-specialization",
		Name: "Fuinjutsu Specialization",
	}}
	if got := ninjutsuSpecializationBonusJutsuSlots(fuinjutsu); got != 2 {
		t.Errorf("ninjutsuSpecializationBonusJutsuSlots(Fuinjutsu Specialization) = %d, want 2", got)
	}

	hijutsu := []grantedFeatureRow{{
		Slug: "class/ninjutsu-specialist/group/ninjutsu-focus/hijutsu-elitist/feature/hijutsu-specialization",
		Name: "Hijutsu Specialization",
	}}
	if got := ninjutsuSpecializationBonusJutsuSlots(hijutsu); got != 1 {
		t.Errorf("ninjutsuSpecializationBonusJutsuSlots(Hijutsu Specialization) = %d, want 1", got)
	}
}

// TestAdaptiveMovementGrantedJutsu covers Mech Crafter's Adaptive Movement
// (3rd level): "You learn the Body Flicker and Chakra Leaping Ninjutsu."
// jutsuGrantNoRankPattern captures the whole "Body Flicker and Chakra
// Leaping" span as one bogus name that resolves against neither jutsu, so
// both are hand-added by loadGrantedJutsuLabels's own special case rather
// than parsed out of the sentence. Confirms both jutsu land with the
// Subclass Feature badge once the character reaches 3rd level, and neither
// lands before then.
func TestAdaptiveMovementGrantedJutsu(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 8)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/mech-crafter', 'class/science-nin/group/scientific-inquiry', 'Mech Crafter')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/science-nin/group/scientific-inquiry/mech-crafter/feature/adaptive-movement',
		 'class/science-nin/group/scientific-inquiry/mech-crafter', 'Adaptive Movement', 3,
		 'Also at 3rd level, to complement your fighting style you are always moving to either keep your distance or put on the pressure. You learn the Body Flicker and Chakra Leaping Ninjutsu. This does not count against your known jutsu. You can cast these jutsu using chakra from your CCD. When you cast one of these jutsu, you automatically gain the benefits of other jutsu you did not cast (You are considered as casting both). While under either jutsu''s effect, you ignore difficult terrain, can walk on walls and water without reduction to your speed and can use Intelligence in place of Strength for calculating your jump distance.', 1)`)
	for _, j := range [][2]string{
		{"jutsu/body-flicker", "Body Flicker"},
		{"jutsu/chakra-leaping", "Chakra Leaping"},
	} {
		mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
			VALUES (?, ?, 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Ninjutsu', 'test jutsu')`,
			j[0], j[1])
	}

	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kabuto', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	characterID, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', 2, 0)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/mech-crafter', 3)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	// Below level 3 (character_classes.levels, what subclass features are
	// actually gated on — sheet.Level alone doesn't restrict this): feature
	// not yet reached, neither jutsu granted.
	labels, err := s.loadGrantedJutsuLabels(characterID, &charsheet.Sheet{Level: 2})
	if err != nil {
		t.Fatalf("loadGrantedJutsuLabels at level 2: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("labels at level 2 = %v, want none (Adaptive Movement not yet reached)", labels)
	}

	if _, err := s.charDB.Exec(
		`UPDATE character_classes SET levels = 3 WHERE character_id = ? AND class_slug = 'class/science-nin'`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	// At level 3: both jutsu land with the Subclass Feature badge.
	labels, err = s.loadGrantedJutsuLabels(characterID, &charsheet.Sheet{Level: 3})
	if err != nil {
		t.Fatalf("loadGrantedJutsuLabels at level 3: %v", err)
	}
	want := map[string]string{
		"jutsu/body-flicker":   "Subclass Feature",
		"jutsu/chakra-leaping": "Subclass Feature",
	}
	if len(labels) != len(want) {
		t.Fatalf("labels at level 3 = %v, want %v", labels, want)
	}
	for slug, label := range want {
		if labels[slug] != label {
			t.Errorf("labels[%q] at level 3 = %q, want %q", slug, labels[slug], label)
		}
	}
}
