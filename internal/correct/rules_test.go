package correct

import (
	"testing"
)

// --- from misspell ---
func TestMisspellFix(t *testing.T) {
	fixed, diffs := misspellFix("You will recieve the item.")
	if fixed != "You will receive the item." {
		t.Errorf("fixed = %q, want %q", fixed, "You will receive the item.")
	}
	if len(diffs) != 1 || diffs[0].tool != "misspell" || diffs[0].original != "recieve" || diffs[0].corrected != "receive" {
		t.Errorf("diffs = %+v", diffs)
	}
}

func TestMisspellFix_LeavesJargonAlone(t *testing.T) {
	s := "The Aburame clan channels chakra through hijutsu and ninjutsu."
	fixed, diffs := misspellFix(s)
	if fixed != s {
		t.Errorf("fixed = %q, want unchanged %q", fixed, s)
	}
	if len(diffs) != 0 {
		t.Errorf("diffs = %+v, want none", diffs)
	}
}

// --- from apostrophe ---
func TestApostropheFix_UserExample(t *testing.T) {
	in := "You are proficient with Katana's, Broadswords and Odachi's."
	want := "You are proficient with Katanas, Broadswords and Odachis."
	got, diffs := apostropheFix(in)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(diffs) != 2 {
		t.Fatalf("diffs = %+v, want 2", diffs)
	}
	if diffs[0].original != "Katana's," || diffs[0].corrected != "Katanas," {
		t.Errorf("diffs[0] = %+v", diffs[0])
	}
	if diffs[1].original != "Odachi's." || diffs[1].corrected != "Odachis." {
		t.Errorf("diffs[1] = %+v", diffs[1])
	}
}

func TestApostropheFix_CurlyApostrophe(t *testing.T) {
	got, diffs := apostropheFix("proficient with Odachi’s.")
	if got != "proficient with Odachis." {
		t.Errorf("got %q", got)
	}
	if len(diffs) != 1 {
		t.Fatalf("diffs = %+v, want 1", diffs)
	}
}

func TestApostropheFix_LeavesGenuinePossessivesAlone(t *testing.T) {
	s := "The Hokage's decision was final."
	got, diffs := apostropheFix(s)
	if got != s {
		t.Errorf("got %q, want unchanged %q", got, s)
	}
	if len(diffs) != 0 {
		t.Errorf("diffs = %+v, want none", diffs)
	}
}

func TestApostropheFix_LeavesContractionsAlone(t *testing.T) {
	for _, s := range []string{
		"It's, admittedly, a stretch.",
		"That's, in short, the plan.",
	} {
		got, diffs := apostropheFix(s)
		if got != s {
			t.Errorf("got %q, want unchanged %q", got, s)
		}
		if len(diffs) != 0 {
			t.Errorf("s=%q diffs = %+v, want none", s, diffs)
		}
	}
}

// --- from hyphenspace ---
func TestHyphenSpaceFix_ClosesConfirmedSplits(t *testing.T) {
	cases := map[string]string{
		"D- Rank":              "D-Rank",
		"a 30- foot cube":      "a 30-foot cube",
		"hand-to- hand combat": "hand-to-hand combat",
		"a Nin- Dog":           "a Nin-Dog",
		"Will- O-Wisp":         "Will-O-Wisp",
		"Will-O- Wisps":        "Will-O-Wisps",
		"ever- so-slightly":    "ever-so-slightly",
		"head- to-head match":  "head-to-head match",
	}
	for in, want := range cases {
		got, diffs := hyphenSpaceFix(in)
		if got != want {
			t.Errorf("hyphenSpaceFix(%q) = %q, want %q", in, got, want)
		}
		if len(diffs) == 0 {
			t.Errorf("hyphenSpaceFix(%q) produced no diffs", in)
		}
	}
}

func TestHyphenSpaceFix_LeavesDashPunctuationAlone(t *testing.T) {
	// Both of these are real corpus sentences using a bare hyphen as
	// dash-style punctuation, not a split compound word — closing them
	// would create a nonsense fused word ("alone-for", "foe-the").
	cases := []string{
		"alone- for a formative part of your life",
		"adjacent to it-friend or foe- the affected creature",
		"critical hit on a roll of 19- Tiger Grapple",
	}
	for _, in := range cases {
		got, diffs := hyphenSpaceFix(in)
		if got != in || len(diffs) != 0 {
			t.Errorf("hyphenSpaceFix(%q) = %q, diffs=%d; want unchanged", in, got, len(diffs))
		}
	}
}

// --- from possessive ---
func TestPossessiveFix_DropsFixed(t *testing.T) {
	cases := map[string]string{
		"a creatures cells":            "a creature’s cells",
		"your weapons range":           "your weapon’s range",
		"the targets Hit Points":       "the target’s Hit Points",
		"the casters next turn":        "the caster’s next turn",
		"the clones body":              "the clone’s body",
		"a creatures Damage Reduction": "a creature’s Damage Reduction",
		"this Adversaries hit points":  "this Adversary’s hit points",
		"these Adversaries next turn":  "these Adversary’s next turn",
		"these Adversaries allies":     "these Adversary’s allies",
	}
	for in, want := range cases {
		got, diffs := possessiveFix(in)
		if got != want {
			t.Errorf("possessiveFix(%q) = %q, want %q", in, got, want)
		}
		if len(diffs) != 1 {
			t.Errorf("possessiveFix(%q) diffs = %d, want 1", in, len(diffs))
		}
	}
}

func TestPossessiveFix_LeavesGenuinePluralsAlone(t *testing.T) {
	cases := []string{
		"the creatures gain advantage",
		"all creatures within 10 feet",
		"your weapons are ready",
		"these Adversaries deal damage",
	}
	for _, in := range cases {
		got, diffs := possessiveFix(in)
		if got != in || len(diffs) != 0 {
			t.Errorf("possessiveFix(%q) = %q, diffs=%d; want unchanged", in, got, len(diffs))
		}
	}
}

// --- from wordconfusion ---
func TestWordConfusionFix_AllayToAlly(t *testing.T) {
	got, diffs := wordConfusionFix("your Bonded allay gain advantage")
	if want := "your Bonded ally gain advantage"; got != want {
		t.Errorf("wordConfusionFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}

// --- from bonusplural ---
func TestBonusPluralFix_SingularArticlePluralNoun(t *testing.T) {
	got, diffs := bonusPluralFix("You gain a +2 bonuses to your AC.")
	if want := "You gain a +2 bonus to your AC."; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}

func TestBonusPluralFix_NoSign(t *testing.T) {
	got, _ := bonusPluralFix("the weapon has a 10 bonuses to trip and grapple checks")
	if want := "the weapon has a 10 bonus to trip and grapple checks"; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
}

func TestBonusPluralFix_MultipleOccurrences(t *testing.T) {
	got, diffs := bonusPluralFix("gains a +2 bonuses to AC, deals +1 extra and a +3 bonuses to the damage roll.")
	if want := "gains a +2 bonus to AC, deals +1 extra and a +3 bonus to the damage roll."; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
	if len(diffs) != 2 {
		t.Errorf("diffs = %d, want 2", len(diffs))
	}
}

func TestBonusPluralFix_NoMatch(t *testing.T) {
	got, diffs := bonusPluralFix("You gain a +2 bonus to your AC.")
	if want := "You gain a +2 bonus to your AC."; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
	if diffs != nil {
		t.Errorf("diffs = %v, want nil", diffs)
	}
}

// --- from penaltyplural ---
func TestPenaltyPluralFix_SingularArticlePluralNoun(t *testing.T) {
	got, diffs := penaltyPluralFix("they have a -3 penalties on all Constitution Saving Throws.")
	if want := "they have a -3 penalty on all Constitution Saving Throws."; got != want {
		t.Errorf("penaltyPluralFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}

func TestPenaltyPluralFix_NoMatch(t *testing.T) {
	got, diffs := penaltyPluralFix("they have a -3 penalty on all Constitution Saving Throws.")
	if want := "they have a -3 penalty on all Constitution Saving Throws."; got != want {
		t.Errorf("penaltyPluralFix() = %q, want %q", got, want)
	}
	if diffs != nil {
		t.Errorf("diffs = %v, want nil", diffs)
	}
}

// --- from straycaps ---
// TestStrayCapsFix_VerbPositions covers the real corpus occurrences this
// pass was written against (full-corpus audit 2026-09-13).
func TestStrayCapsFix_VerbPositions(t *testing.T) {
	cases := []struct{ in, want string }{
		{"You form bullets on the edge of each of your fingers and Fire them off with enough spread to imitate a shotgun.",
			"You form bullets on the edge of each of your fingers and fire them off with enough spread to imitate a shotgun."},
		{"once per turn as an action or bonus action, you may Fire a jolt of Lightning",
			"once per turn as an action or bonus action, you may fire a jolt of Lightning"},
		{"you can Fire off a defensive jolt of Lightning towards a single creature",
			"you can fire off a defensive jolt of Lightning towards a single creature"},
		{"a small sphere in front of you, which then Fires outward into a 60-foot line",
			"a small sphere in front of you, which then fires outward into a 60-foot line"},
		{"you can use an action on each subsequent turn to Fire this wave again",
			"you can use an action on each subsequent turn to fire this wave again"},
		{"you can Fire up to three beams of Lightning",
			"you can fire up to three beams of Lightning"},
		{"You conjure and Fire a 60-foot cone in front of you.",
			"You conjure and fire a 60-foot cone in front of you."},
		{"prepare to Fire in all directions by striking the ground",
			"prepare to fire in all directions by striking the ground"},
		{"a single globe that you then Fire as a beam of white flame",
			"a single globe that you then fire as a beam of white flame"},
		{"up to the number of beams you Fire. On a hit, you deal 3d6 Lightning damage.",
			"up to the number of beams you fire. On a hit, you deal 3d6 Lightning damage."},
		{"• Burn: When you Hit a creature, who has Ranks of Burned, with this Weapon",
			"• Burn: When you hit a creature, who has Ranks of Burned, with this Weapon"},
		{"you may force a creature you Hit with this Weapon to Succeed a Constitution saving throw",
			"you may force a creature you hit with this Weapon to Succeed a Constitution saving throw"},
	}
	for _, c := range cases {
		got, diffs := strayCapsFix(c.in)
		if got != c.want {
			t.Errorf("strayCapsFix(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if len(diffs) == 0 {
			t.Errorf("strayCapsFix(%q) recorded no diff", c.in)
		}
	}
}

// TestStrayCapsFix_LeavesRealTermsAlone is the guard that matters: the
// element and the Hit Points resource are capitalized on purpose in
// hundreds of rows, and a pass that lowercases those is far worse than the
// handful of typos it fixes.
func TestStrayCapsFix_LeavesRealTermsAlone(t *testing.T) {
	unchanged := []string{
		"all Keton Clan Hijutsu you cast ignore resistance to Fire damage",
		"jutsu you cast with the Wind Release or Fire Release keywords",
		"Craft Paper Bombs, Explosive Tag balls and Fire Bombs",
		"you can change the damage type to Fire.",
		"your Titan is resistant to Fire and Acid damage",
		"You begin with either Water or Fire Release Affinity. (Pick one)",
		"an AC equal to your Ninjutsu Save DC, and Hit Points equal to twice your Ninjutsu Save DC",
		"Any damage you deal to a target, grants you Hit Points equal to the damage dealt.",
		"increase the cost of this jutsu by 3 and Hit points by 2d10 + 15",
		"Fire Release: Fire Damage (Immune)",
	}
	for _, in := range unchanged {
		got, diffs := strayCapsFix(in)
		if got != in {
			t.Errorf("strayCapsFix(%q) = %q, want unchanged (legitimate game term)", in, got)
		}
		if len(diffs) != 0 {
			t.Errorf("strayCapsFix(%q) recorded %+v, want none", in, diffs)
		}
	}
}

// --- from succeedon ---

func TestSucceedOnFix_InsertsPreposition(t *testing.T) {
	cases := []struct{ in, want string }{
		{"the creature must succeed a Dexterity saving throw vs. your Ninjutsu Save DC",
			"the creature must succeed on a Dexterity saving throw vs. your Ninjutsu Save DC"},
		{"A creature needs to succeed an Investigation or Perception check against you",
			"A creature needs to succeed on an Investigation or Perception check against you"},
		{"or a creature Succeeds their Constitution saving throw",
			"or a creature Succeeds on their Constitution saving throw"},
		{"cannot gain the benefit of critically succeeding the save",
			"cannot gain the benefit of critically succeeding on the save"},
		{"If you succeed this check, while within this jutsu's area",
			"If you succeed on this check, while within this jutsu's area"},
		{"If the target creature succeeds 3 saves they reject the earth chakra",
			"If the target creature succeeds on 3 saves they reject the earth chakra"},
		{"must succeed a Wisdom or Intelligence saving throw",
			"must succeed on a Wisdom or Intelligence saving throw"},
	}
	for _, c := range cases {
		got, diffs := succeedOnFix(c.in)
		if got != c.want {
			t.Errorf("succeedOnFix(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if len(diffs) == 0 {
			t.Errorf("succeedOnFix(%q) recorded no diff", c.in)
		}
	}
}

// TestSucceedOnFix_LeavesCorrectAndIntransitiveAlone guards the two ways
// this rule could misfire: text that already reads correctly, and the
// genuinely intransitive uses of "succeed" the same audit turned up.
func TestSucceedOnFix_LeavesCorrectAndIntransitiveAlone(t *testing.T) {
	unchanged := []string{
		"the target succeeds on a saving throw against any Jiton clan Hijutsu",
		"If you succeed by 5 or more, increase this attack's critical threshold",
		"the total penalty is lost if the creature succeeds and is reset to -1",
		"to take half damage, you can choose to succeed instead, taking no damage",
		"A creature automatically succeeds if it is immune to the fear condition",
		"would allow a creature to automatically succeed against the genjutsu",
		"You know whether each creature succeeds or fails on its Saving Throw.",
		"spend a charge of your Hackers Kit and succeed hacking into a system",
		"A creature that succeeds suffers no ill effects",
	}
	for _, in := range unchanged {
		got, diffs := succeedOnFix(in)
		if got != in {
			t.Errorf("succeedOnFix(%q) = %q, want unchanged", in, got)
		}
		if len(diffs) != 0 {
			t.Errorf("succeedOnFix(%q) recorded %+v, want none", in, diffs)
		}
	}
}

// --- from commacaps ---

func TestCommaCapsFix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Beginning at 6th level, Your body has been strengthened to obsidian.",
			"Beginning at 6th level, your body has been strengthened to obsidian."},
		{"Select a Construct or Structure within range, Make a Ranged Taijutsu Attack",
			"Select a Construct or Structure within range, make a Ranged Taijutsu Attack"},
		{"On a failed save, until the end of their next turn, If the affected creature attempts any action",
			"On a failed save, until the end of their next turn, if the affected creature attempts any action"},
		{"All previous effects, The target loses 2 AC for 1 minute",
			"All previous effects, the target loses 2 AC for 1 minute"},
		{"destroying the surface it is inscribed on, Creatures in radius must make a save",
			"destroying the surface it is inscribed on, creatures in radius must make a save"},
	}
	for _, c := range cases {
		got, diffs := commaCapsFix(c.in)
		if got != c.want {
			t.Errorf("commaCapsFix(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if len(diffs) == 0 {
			t.Errorf("commaCapsFix(%q) recorded no diff", c.in)
		}
	}
}

// TestCommaCapsFix_LeavesTermsOfArtAlone is the rule's real risk: most
// capitals after a comma in this corpus are the book's own vocabulary.
func TestCommaCapsFix_LeavesTermsOfArtAlone(t *testing.T) {
	unchanged := []string{
		"If this jutsu is cast at B-Rank, Ninjutsu attacks against you have disadvantage",
		"the creature is knocked Prone, Grappled, and Restrained",
		"you gain resistance to Fire damage, Lightning damage, and Earth damage",
		"reduce the cost by 3, AC begins its calculation with 14",
		"recorded. (Within the past day: 12, Within the past week: 16)",
		"a spiked hair protruding from it., For the duration, your speed is reduced",
		"you deal Genjutsu damage, Taijutsu damage, or Bukijutsu damage",
	}
	for _, in := range unchanged {
		got, diffs := commaCapsFix(in)
		if got != in {
			t.Errorf("commaCapsFix(%q) = %q, want unchanged", in, got)
		}
		if len(diffs) != 0 {
			t.Errorf("commaCapsFix(%q) recorded %+v, want none", in, diffs)
		}
	}
}

// --- from midcaps ---

func TestMidCapsFix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"must make a Saving Throw. On a Failed save taking 12d8+12 Fire Damage",
			"must make a Saving Throw. On a failed save taking 12d8+12 Fire Damage"},
		{"On a successful save they take Half.", "On a successful save they take half."},
		{"You have advantage on checks to Stabilize a creature",
			"You have advantage on checks to stabilize a creature"},
		{"Armor imbued with this seal Gains +2 Damage reduction",
			"Armor imbued with this seal gains +2 Damage reduction"},
		{"As a Bonus action you can Increase your walking speed by 10 feet.",
			"As a Bonus action you can increase your walking speed by 10 feet."},
		{"the creature this Aura is centered on May, as a reaction, end this Aura",
			"the creature this Aura is centered on may, as a reaction, end this Aura"},
		{"can add Half of your Charisma Modifier", "can add half of your Charisma Modifier"},
	}
	for _, c := range cases {
		got, diffs := midCapsFix(c.in)
		if got != c.want {
			t.Errorf("midCapsFix(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if len(diffs) == 0 {
			t.Errorf("midCapsFix(%q) recorded no diff", c.in)
		}
	}
}

func TestMidCapsFix_LeavesNamedTermsAndBulletsAlone(t *testing.T) {
	unchanged := []string{
		"you may gain the Full Moon benefits on a Half Moon",
		"o Increase its attack and damage rolls by the number of Snacks you spend",
		"o Gains +1 extra D-Rank Dog/Wolf special feature",
		"spend them on the Amplify or Reduce features on the following keywords",
		"This jutsu counts as an Interrupt.",
	}
	for _, in := range unchanged {
		got, diffs := midCapsFix(in)
		if got != in {
			t.Errorf("midCapsFix(%q) = %q, want unchanged", in, got)
		}
		if len(diffs) != 0 {
			t.Errorf("midCapsFix(%q) recorded %+v, want none", in, diffs)
		}
	}
}

func TestPossessiveExtFix_InsertsApostrophe(t *testing.T) {
	cases := map[string]string{
		"a creatures neck":                      "a creature’s neck",
		"an affected targets AC is reduced":     "an affected target’s AC is reduced",
		"one clones antics":                     "one clone’s antics",
		"reduce the targets Armor Class":        "reduce the target’s Armor Class",
		"the creatures perception of reality":   "the creature’s perception of reality",
		"add the weapons critical threat range": "add the weapon’s critical threat range",
		"equal to the Creatures Level":          "equal to the Creature’s Level",
	}
	for in, want := range cases {
		got, diffs := possessiveExtFix(in)
		if got != want {
			t.Errorf("possessiveExtFix(%q) = %q, want %q", in, got, want)
		}
		if len(diffs) == 0 {
			t.Errorf("possessiveExtFix(%q) recorded no diff", in)
		}
	}
}

func TestPossessiveExtFix_LeavesRealPluralsAlone(t *testing.T) {
	cases := []string{
		"If multiple shadow clones attack the same creature",
		"the component weapons chosen",
		"Creatures Burned by this jutsu",
		"grant the affected creatures temporary hit points",
		"comparing the single attack roll with all targets AC",
		"equal to both casters unarmed damage",
		"a -1 penalty to creatures saving throws",
		"contested by all hostile creatures passive perception",
		"When a Creatures currently under the effects of a jutsu",
		"a creature’s senses",
		"Each time a creature targets you with an attack",
	}
	for _, in := range cases {
		got, diffs := possessiveExtFix(in)
		if got != in {
			t.Errorf("possessiveExtFix(%q) = %q, want unchanged", in, got)
		}
		if len(diffs) != 0 {
			t.Errorf("possessiveExtFix(%q) recorded %d diffs, want 0", in, len(diffs))
		}
	}
}

func TestAbbrevSplitFix(t *testing.T) {
	cases := map[string]string{
		"you outfit your A. Ts with a device":   "you outfit your A.Ts with a device",
		"Your A. T gains the Blocking property": "Your A.T gains the Blocking property",
		"combine 2 B.I. Ms into 1 B.I.M":        "combine 2 B.I.Ms into 1 B.I.M",
		"exchange one E.I. Ps on a short rest":  "exchange one E.I.Ps on a short rest",
		"called A.N. Ts. As an action":          "called A.N.Ts. As an action",
		"rolls using S.E.N. Ts.":                "rolls using S.E.N.Ts.",
	}
	for in, want := range cases {
		got, diffs := abbrevSplitFix(in)
		if got != want {
			t.Errorf("abbrevSplitFix(%q) = %q, want %q", in, got, want)
		}
		if len(diffs) == 0 {
			t.Errorf("abbrevSplitFix(%q) recorded no diff", in)
		}
	}
}

func TestAbbrevSplitFix_LeavesSentenceBoundariesAlone(t *testing.T) {
	cases := []string{
		"against this B.I.M. On a miss it sticks to the ground",
		"This stack becomes your S.E.N.T. It immediately becomes a d10 stack",
		"Air Trecks or A.T. These A.Ts have a design of your choice",
		"you own. B.I. As of that type",
	}
	for _, in := range cases {
		if got, diffs := abbrevSplitFix(in); got != in || len(diffs) != 0 {
			t.Errorf("abbrevSplitFix(%q) = %q with %d diffs, want unchanged", in, got, len(diffs))
		}
	}
}

func TestPluralVerbFix(t *testing.T) {
	got, diffs := pluralVerbFix("Failure: Affected creatures gains one rank of fear")
	if want := "Failure: Affected creatures gain one rank of fear"; got != want {
		t.Errorf("pluralVerbFix = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("pluralVerbFix recorded %d diffs, want 1", len(diffs))
	}
	for _, in := range []string{
		"If either of the tethered creatures is reduced to 0 Hit Points",
		"your ability to reinforce your allies has heightened even more",
		"a creature gains 1 rank of the Burned Condition",
	} {
		if got, diffs := pluralVerbFix(in); got != in || len(diffs) != 0 {
			t.Errorf("pluralVerbFix(%q) = %q with %d diffs, want unchanged", in, got, len(diffs))
		}
	}
}
