package main

import (
	"strings"
	"testing"
)

func TestFormatDescriptionNoBullets(t *testing.T) {
	got := formatDescription("Plain text with no bullets, & an ampersand.")
	want := `<p>Plain text with no bullets, &amp; an ampersand.</p>`
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatDescriptionEmpty(t *testing.T) {
	if got := formatDescription(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFormatDescriptionSimpleBullets(t *testing.T) {
	raw := "Your insects treat you and your body as a hive. You gain the following benefits; " +
		"• Increase your Intelligence or Wisdom score by 1. " +
		"• When you would make a check, you may instead make a different check."
	got := string(formatDescription(raw))

	if !strings.Contains(got, "<p>Your insects treat you and your body as a hive. You gain the following benefits;</p>") {
		t.Errorf("missing intro paragraph, got: %s", got)
	}
	if !strings.Contains(got, `<ul class="prose-list"><li>Increase your Intelligence or Wisdom score by 1.</li>`) {
		t.Errorf("missing first bullet, got: %s", got)
	}
	if !strings.Contains(got, "<li>When you would make a check, you may instead make a different check.</li>") {
		t.Errorf("missing second bullet, got: %s", got)
	}
	if strings.Contains(got, "•") {
		t.Errorf("raw bullet character leaked into output: %s", got)
	}
}

func TestFormatDescriptionNestedSubBullets(t *testing.T) {
	// Real shape from Hunter-Nin's Primary Target feature.
	raw := "While marked, you gain the following benefits; " +
		"• You have Advantage on Wisdom checks made to track your Primary Target. " +
		"• You become aware of one of the following; " +
		"o One damage immunity they have. " +
		"o One damage resistance they have. " +
		"o Their current Damage reduction value."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<ul class="prose-list prose-sublist">`) {
		t.Fatalf("expected a nested sub-list, got: %s", got)
	}
	for _, want := range []string{
		"<li>One damage immunity they have.</li>",
		"<li>One damage resistance they have.</li>",
		"<li>Their current Damage reduction value.</li>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing sub-bullet %q, got: %s", want, got)
		}
	}
	if strings.Contains(got, " o ") {
		t.Errorf("raw sub-bullet marker leaked into output: %s", got)
	}
}

func TestFormatDescriptionNamedEntries(t *testing.T) {
	// Real shape from Weapon Specialist's Gungnir Piercer Styles.
	raw := "Also, at 3rd level, you get to choose a Style that supports your combat ability. " +
		"Mímir’s Wisdom. As a Bonus Action, select one creature whom you can see, within 60 feet of you. " +
		"You cannot use this effect more than once per short rest. " +
		"Hœnir’s Silence. As a Bonus Action, you coat a weapon you are holding that deals piercing damage. " +
		"Freya’s Golden Tears. Twice per turn, when you would deal piercing damage with a bukijutsu, the target gains 1 rank of bleeding."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<p>Also, at 3rd level, you get to choose a Style that supports your combat ability.</p>`) {
		t.Errorf("missing intro paragraph, got: %s", got)
	}
	if !strings.Contains(got, `<div class="named-entries">`) {
		t.Fatalf("expected a named-entries block, got: %s", got)
	}
	for _, want := range []string{
		"<p><strong>Mímir’s Wisdom.</strong> As a Bonus Action, select one creature whom you can see, within 60 feet of you. You cannot use this effect more than once per short rest.</p>",
		"<p><strong>Hœnir’s Silence.</strong> As a Bonus Action, you coat a weapon you are holding that deals piercing damage.</p>",
		"<p><strong>Freya’s Golden Tears.</strong> Twice per turn, when you would deal piercing damage with a bukijutsu, the target gains 1 rank of bleeding.</p>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing named entry %q, got: %s", want, got)
		}
	}
}

func TestFormatDescriptionDoesNotFalsePositiveOnSingleCapitalizedPhrase(t *testing.T) {
	// A single "Bonus Action."-shaped sentence boundary must NOT trigger
	// named-entries mode — the pattern needs 2+ occurrences to be trusted.
	raw := "As part of this feature, you may take the Attack Action. Bonus Action. " +
		"You still cannot cast a jutsu that requires a Reaction on the same turn."
	got := string(formatDescription(raw))

	if strings.Contains(got, `<div class="named-entries">`) {
		t.Errorf("false-positive named-entries block from a single capitalized phrase, got: %s", got)
	}
}

func TestFormatDescriptionNamedEntriesNoIntro(t *testing.T) {
	raw := "Fenrir’s Claw. When you would use the Chakra Strike Flurry Technique and deal piercing damage, you can pierce through the target’s defenses. " +
		"Heimdallr Vision. As a Bonus Action, you enhance your senses to gain hyper accuracy until the end of the current turn."
	got := string(formatDescription(raw))

	if strings.Contains(got, "<p></p>") {
		t.Errorf("empty intro paragraph rendered when there was no intro text, got: %s", got)
	}
	if !strings.HasPrefix(got, `<div class="named-entries"><p><strong>Fenrir’s Claw.</strong>`) {
		t.Errorf("expected the entries block to start immediately with no stray intro paragraph, got: %s", got)
	}
	if !strings.Contains(got, "<p><strong>Heimdallr Vision.</strong>") {
		t.Errorf("missing Heimdallr Vision entry, got: %s", got)
	}
}

func TestFormatDescriptionNamedEntriesNoIntroAtCapsStart(t *testing.T) {
	// Real shape from Puppet Master's Wood Tier upgrades: the field starts
	// directly with an ALL-CAPS entry, no lead-in sentence at all. Regression
	// test for a real bug: slicing the intro at fullStart+1 assumed a
	// boundary punctuation character always precedes the first match, but a
	// "^"-anchored match at position 0 has none, and the +1 wrongly grabbed
	// the entry name's own first letter as a spurious one-character intro
	// paragraph ("<p>C</p>" was observed in production for this exact text).
	raw := "CHAKRA DISRUPTION BLADE Techniques: Black, Perfect You fit your Puppet Tool with chakra disrupting sand. " +
		"COVERING FIRE Techniques: Blue Your Puppet is able to fight in such a way that it gives allies an opportunity to escape."
	got := string(formatDescription(raw))

	if !strings.HasPrefix(got, `<div class="named-entries"><p><strong class="caps-entry-name">Chakra Disruption Blade.</strong>`) {
		t.Errorf("expected the entries block to start immediately with no stray intro paragraph, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Covering Fire.</strong>`) {
		t.Errorf("missing Covering Fire entry, got: %s", got)
	}
}

func TestFormatDescriptionDoesNotFalsePositiveOnConnectorWords(t *testing.T) {
	// Real shape from a Science-Nin gadget option (class_options rowid 326):
	// "Additionally." and "Themselves." are ordinary sentence starts, not
	// named entries, even though they clear the 2-match repeat threshold.
	raw := "The rocket boots last for 1 minute before deactivating. Themselves. " +
		"You are proficient with this kit. Additionally. You can spend the CCD Drain of this upgrade."
	got := string(formatDescription(raw))

	if strings.Contains(got, `<div class="named-entries">`) {
		t.Errorf("false-positive named-entries block from connector words, got: %s", got)
	}
}

func TestFormatDescriptionDoesNotFalsePositiveOnAbbreviation(t *testing.T) {
	// Real shape from a Weapon Specialist subclass feature (rowid 639): the
	// book abbreviates "Air Trecks" as "A.T.", extracted with a stray space
	// as "A. T" — repeats often enough to otherwise clear the threshold.
	raw := "You have created a highly efficient pair of skates called Air Trecks or A.T. " +
		"These A. Ts have a design of your choice, are Greater quality, and take up 1 bulk. " +
		"Starting at the 9th level your A. Ts cannot be broken and are Superior quality."
	got := string(formatDescription(raw))

	if strings.Contains(got, `<div class="named-entries">`) {
		t.Errorf("false-positive named-entries block from the \"A.T.\" abbreviation, got: %s", got)
	}
}

func TestFormatDescriptionNamedEntriesWithNestedBullets(t *testing.T) {
	// Real (trimmed) shape from Puppet Master's Puppeteer Chassis: ALL-CAPS
	// headers followed by "ASI:", each entry's own body containing its own
	// "•" bullets.
	raw := "Below are the available Puppet Chassis. When you select a chassis, you add the prefix of the chassis to your puppet's name. " +
		"SPECIALIZED ASI: +2 to any ability score of your choice Your Puppet Tool defies all expectations, its design fueled by your own rampant creativity; " +
		"• You can select 2 free Wood tier Upgrades to start with that do not count against your Upgrade total. " +
		"• Your Puppet is able to be any size you like picking from Small, Medium, or Large. " +
		"QUADRUPEDAL ASI: Strength & Constitution scores become 16. Your Puppet Tool takes on a quadrupedal design. " +
		"• Your Puppet gains the Bulky Build Upgrade, which does not count against your Upgrade total."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<div class="named-entries">`) {
		t.Fatalf("expected a named-entries block, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Specialized.</strong> ASI: +2 to any ability score of your choice Your Puppet Tool defies all expectations, its design fueled by your own rampant creativity;</p>`) {
		t.Errorf("missing Specialized entry's lead paragraph, got: %s", got)
	}
	if !strings.Contains(got, `<ul class="prose-list"><li>You can select 2 free Wood tier Upgrades to start with that do not count against your Upgrade total.</li>`) {
		t.Errorf("missing Specialized entry's nested bullet list, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Quadrupedal.</strong> ASI: Strength &amp; Constitution scores become 16. Your Puppet Tool takes on a quadrupedal design.</p>`) {
		t.Errorf("missing Quadrupedal entry, got: %s", got)
	}
	if !strings.Contains(got, "<li>Your Puppet gains the Bulky Build Upgrade, which does not count against your Upgrade total.</li>") {
		t.Errorf("missing QUADRUPEDAL entry's own bullet, got: %s", got)
	}
	if strings.Contains(got, "•") {
		t.Errorf("raw bullet character leaked into output: %s", got)
	}
}

func TestFormatDescriptionDoesNotPromoteDeeplyNestedNameToTopLevel(t *testing.T) {
	// Real shape from a Witch feat: the top-level structure is a "•"
	// bullet list; "Witch Walk." only appears deep inside one bullet's
	// nested "o" sub-item. It must NOT be promoted to a top-level named
	// entry just because a second capitalized-phrase-period pattern
	// ("Witches Aura.") also happens to appear somewhere later in the text.
	raw := "You have found new ways to utilize free flowing chakra particles; " +
		"• You gain Proficiency in Nature. " +
		"• You gain an ability from one of the following schools; " +
		"o School of Shadows. You learn a new form of special movement called Witch Walk. Witch Walk. You can teleport between areas currently affected by jutsu. " +
		"o School of Enhancements. You learn to manifest an esoteric aura called Witches Aura. Witches Aura. All creatures in a 10ft radius of you are under the effects of this aura."
	got := string(formatDescription(raw))

	if strings.Contains(got, `<div class="named-entries">`) {
		t.Errorf("Witch Walk/Witches Aura were wrongly promoted to top-level named entries, got: %s", got)
	}
	if !strings.Contains(got, `<ul class="prose-list">`) {
		t.Errorf("expected the genuine top-level bullet list to still render, got: %s", got)
	}
}

func TestFormatDescriptionCapsEntryStandalone(t *testing.T) {
	// Cost/Drain is the stat-line shape splitLeadingStatLine pulls onto its
	// own line (see TestFormatDescriptionCapsEntrySplitsLeadingStatLine for
	// that specifically) — asserted here as part of the wider entry-caps
	// scenario this test otherwise covers.
	raw := "Your class grants two upgrades. JET PROPULSION LEGS Cost: 8 Creation Points Drain: 10 CCD Chakra You add chakra powered jet thrusters to the soles of your feet. " +
		"INTEGRATED WEAPON Cost: 8 Creation Points Drain: 10 CCD Chakra You can integrate a one-handed weapon into your arm."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<p><strong class="caps-entry-name">Jet Propulsion Legs.</strong> <span class="entry-stat-line">Cost: 8 Creation Points · Drain: 10 CCD Chakra</span></p><p>You add chakra powered jet thrusters to the soles of your feet.</p>`) {
		t.Errorf("missing Jet Propulsion Legs entry, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Integrated Weapon.</strong> <span class="entry-stat-line">Cost: 8 Creation Points · Drain: 10 CCD Chakra</span></p><p>You can integrate a one-handed weapon into your arm.</p>`) {
		t.Errorf("missing Integrated Weapon entry, got: %s", got)
	}
}

// TestFormatDescriptionCapsEntrySplitsLeadingStatLine pins the real
// Science-Nin Mastercraft shape that motivated splitLeadingStatLine: two
// stat-line fields back to back, then prose, with zero punctuation
// separating any of it in the raw text.
func TestFormatDescriptionCapsEntrySplitsLeadingStatLine(t *testing.T) {
	raw := "SUPER ENHANCED DEFENSE PROTOCOL Cost: 32 Creation Points Drain: 30 CCD Chakra You gain a barrier of HP equal to your Intelligence modifier. " +
		"SUPER ENHANCED OFFENSE PROTOCOL Cost: 32 Creation Points Drain: 30 CCD Chakra As a bonus action you may enhance all weapons within 30 ft."
	got := string(formatDescription(raw))

	want1 := `<p><strong class="caps-entry-name">Super Enhanced Defense Protocol.</strong> <span class="entry-stat-line">Cost: 32 Creation Points · Drain: 30 CCD Chakra</span></p><p>You gain a barrier of HP equal to your Intelligence modifier.</p>`
	if !strings.Contains(got, want1) {
		t.Errorf("stat line not split onto its own line, got: %s", got)
	}
	want2 := `<p><strong class="caps-entry-name">Super Enhanced Offense Protocol.</strong> <span class="entry-stat-line">Cost: 32 Creation Points · Drain: 30 CCD Chakra</span></p><p>As a bonus action you may enhance all weapons within 30 ft.</p>`
	if !strings.Contains(got, want2) {
		t.Errorf("second entry's stat line not split, got: %s", got)
	}
}

// TestFormatDescriptionCapsEntryStatLineOnlyCost covers just one field
// (Drain absent) — splitLeadingStatLine must not require both.
func TestFormatDescriptionCapsEntryStatLineOnlyCost(t *testing.T) {
	raw := "GADGET ONE Cost: 5 Chakra You do a thing. GADGET TWO Cost: 6 Chakra You do another thing."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<p><strong class="caps-entry-name">Gadget One.</strong> <span class="entry-stat-line">Cost: 5 Chakra</span></p><p>You do a thing.</p>`) {
		t.Errorf("single-field stat line not split, got: %s", got)
	}
}

func TestSplitLeadingStatLine(t *testing.T) {
	statLine, rest := splitLeadingStatLine("Cost: 32 Creation Points Drain: 30 CCD Chakra You gain a barrier.")
	if statLine != "Cost: 32 Creation Points · Drain: 30 CCD Chakra" {
		t.Errorf("statLine = %q", statLine)
	}
	if rest != "You gain a barrier." {
		t.Errorf("rest = %q", rest)
	}

	// No recognized field at all — must return the body untouched, not a
	// zero-length "matched everything" result.
	statLine, rest = splitLeadingStatLine("You gain a barrier with no stat line at all.")
	if statLine != "" {
		t.Errorf("statLine = %q, want empty", statLine)
	}
	if rest != "You gain a barrier with no stat line at all." {
		t.Errorf("rest = %q", rest)
	}

	// A keyword this function doesn't handle (Prerequisite) must not be
	// swallowed into the stat line, and must not be treated as the field
	// separator either — the whole body is prose as far as this function
	// is concerned.
	statLine, rest = splitLeadingStatLine("Prerequisite: Ranged Weapon You must be wielding one.")
	if statLine != "" {
		t.Errorf("statLine = %q, want empty (Prerequisite: not handled)", statLine)
	}
	if rest != "Prerequisite: Ranged Weapon You must be wielding one." {
		t.Errorf("rest = %q", rest)
	}
}

func TestFormatDescriptionCapsEntryWithColonPrefix(t *testing.T) {
	// Real shape from Puppet Master's Bronze Tier: a category label glued
	// directly to its own colon ("ARMORY:", no space before the colon)
	// sits in front of the actual item name. Without capsEntryPattern's
	// optional prefix group, this entire entry (and everything after it,
	// since it never gets recognized as an entry boundary at all) renders
	// as one undifferentiated paragraph instead of splitting out.
	raw := "An armament first created by the renowned puppet master, Shugi Gizo. " +
		"ARMORY: FIRE AND WATER BLASTER Techniques: All, Perfect You install a small blaster inside of your Puppet. " +
		"ARMORY: NEEDLE WAVE Techniques: All, Perfect You fit your Puppet Tool with a senbon launcher."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<p><strong class="caps-entry-name">Armory: Fire and Water Blaster.</strong> Techniques: All, Perfect You install a small blaster inside of your Puppet.</p>`) {
		t.Errorf("missing Armory: Fire and Water Blaster entry, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Armory: Needle Wave.</strong> Techniques: All, Perfect You fit your Puppet Tool with a senbon launcher.</p>`) {
		t.Errorf("missing Armory: Needle Wave entry, got: %s", got)
	}
}

func TestFormatDescriptionCapsEntryWithApostrophe(t *testing.T) {
	// Real shape from Puppet Master's Iron Tier: a possessive inside the
	// ALL-CAPS name ("BEHEADER'S BLADE") — without apostrophe support in
	// capsEntryPattern's word class, this whole entry silently fails to
	// split out at all.
	raw := "The object becomes too heavy to bear and falls into an unusable glob of debris. " +
		"ARMORY: BEHEADER’S BLADE Techniques: All A macabre upgrade first invented by the Puppet Corps. " +
		"SALAMANDER Techniques: Black Your Puppet gains an amount of burrowing speed equal to half your movement speed."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<p><strong class="caps-entry-name">Armory: Beheader’s Blade.</strong> Techniques: All A macabre upgrade first invented by the Puppet Corps.</p>`) {
		t.Errorf("missing Armory: Beheader's Blade entry, got: %s", got)
	}
}

func TestFormatDescriptionCapsEntryProseNoKeyword(t *testing.T) {
	// Real shape from Puppet Master's Purple Technique ~ Juggernaut: Unique
	// Armor Properties — an ALL-CAPS header running straight into ordinary
	// prose with no "Keyword:" stat line to anchor on (unlike
	// TestFormatDescriptionCapsEntryStandalone's "Cost: ..." shape), plus a
	// trailing parenthetical annotation on one entry.
	raw := "ATHLETIC Armor with the Athletic property grants a +1d4 bonus to Acrobatics. " +
		"MOBILE Armor with the Mobile property grants a +5 bonus to movement speed. " +
		"STURDY (RENAMED) Armor with the Sturdy property enhances Reactions."
	got := string(formatDescription(raw))

	if !strings.Contains(got, `<p><strong class="caps-entry-name">Athletic.</strong> Armor with the Athletic property grants a +1d4 (2.5) bonus to Acrobatics.</p>`) {
		t.Errorf("missing Athletic entry, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Mobile.</strong> Armor with the Mobile property grants a +5 bonus to movement speed.</p>`) {
		t.Errorf("missing Mobile entry, got: %s", got)
	}
	if !strings.Contains(got, `<p><strong class="caps-entry-name">Sturdy (Renamed).</strong> Armor with the Sturdy property enhances Reactions.</p>`) {
		t.Errorf("missing Sturdy (Renamed) entry, got: %s", got)
	}
}

func TestFormatDescriptionCapsEntryProseDoesNotDoubleUpWithColonShape(t *testing.T) {
	// SALAMANDER's body ("Techniques: Black ...") must still be claimed
	// exactly once, by capsEntryPattern — capsProseEntryPattern must defer
	// to it rather than also matching the same starting position.
	raw := "An intro sentence. " +
		"SALAMANDER Techniques: Black Your Puppet gains an amount of burrowing speed. " +
		"PARRYING ATTACK Techniques: All You can parry an incoming attack."
	got := string(formatDescription(raw))

	if n := strings.Count(got, "Salamander"); n != 1 {
		t.Errorf("expected exactly one Salamander entry, got %d in: %s", n, got)
	}
}

func TestTitleCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CHAKRA DISRUPTION BLADE", "Chakra Disruption Blade"},
		{"FIRE AND WATER BLASTER", "Fire and Water Blaster"},
		{"ARMORY: NEEDLE WAVE", "Armory: Needle Wave"},
		{"SELF-DESTRUCT SEQUENCE", "Self-Destruct Sequence"},
		{"WINGED", "Winged"},
	}
	for _, c := range cases {
		if got := titleCase(c.in); got != c.want {
			t.Errorf("titleCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDiceAvg(t *testing.T) {
	cases := []struct{ in, want string }{
		{"3d6", "3d6 (10.5)"},
		{"d8", "d8 (4.5)"},
		{"2d6", "2d6 (7)"},
		{"1d4", "1d4 (2.5)"},
		{"d20", "d20 (10.5)"},
		{"No dice here", "No dice here"},
		{"", ""},
		{"2 (1 use)", "2 (1 use)"}, // plain numbers must not be mistaken for dice
		{"Deal 1d8 fire damage, then 2d4 more.", "Deal 1d8 (4.5) fire damage, then 2d4 (5) more."},
	}
	for _, c := range cases {
		if got := diceAvg(c.in); got != c.want {
			t.Errorf("diceAvg(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDescriptionAnnotatesDice(t *testing.T) {
	got := string(formatDescription("Roll 3d6 to determine the damage."))
	want := `<p>Roll 3d6 (10.5) to determine the damage.</p>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatDescriptionDoesNotFalsePositiveOnTheWordOr(t *testing.T) {
	raw := "Choose the Fire Release. Or, if you prefer, choose the Water Release instead. " +
		"• A real bullet follows this sentence to confirm bullets still work."
	got := string(formatDescription(raw))

	if strings.Contains(got, `<ul class="prose-list prose-sublist">`) {
		t.Errorf("false-positive nested sub-list from the word \"Or\", got: %s", got)
	}
	if !strings.Contains(got, "Or, if you prefer, choose the Water Release instead.") {
		t.Errorf("intro sentence containing \"Or\" was mangled, got: %s", got)
	}
}

// wowDraconicGauntletDescription is the real Draconic Gauntlet raw text
// (class_options.description, list_name "W.O.W (Weapons of Wonder)") —
// no "•" bullets and no named-entry shape in its base half, so
// formatDescription alone falls all the way through to its single-<p>
// fallback for the whole ~2500-character field. This is the exact text
// behind the user-reported "one big jumble" screenshot.
const wowDraconicGauntletDescription = `You forge a mighty gauntlet using your element, granting your fists the strength of dragons. This weapon has the Deadly, Reach, Trip, and Unarmed properties. While equipped, increase your [Unarmed Damage] by +1d6 force damage, and you may calculate your Unarmed attack and damage rolls with Intelligence instead of Strength. When you cast a Taijutsu, you may use this Weapon of Wonder as a component. ASCENSION: DRACONIC GAUNTLET If the Draconic Gauntlet is Ascended, you use your element to create and give life to a small Whelp, a tiny youngling dragon. Aether Fireball (Cost: 5 Chakra). Ranged Weapon Attack: Range 90 feet, one target. Hit: 4d10+4 fire damage.`

func TestFormatWoWDescriptionSplitsBaseFromAscension(t *testing.T) {
	got := string(formatWoWDescription(wowDraconicGauntletDescription))

	if !strings.Contains(got, "granting your fists the strength of dragons") {
		t.Errorf("missing base description text, got: %s", got)
	}
	if !strings.Contains(got, `<h4 class="wow-ascension-heading">Ascension: Draconic Gauntlet</h4>`) {
		t.Errorf("missing Ascension heading, got: %s", got)
	}
	if !strings.Contains(got, "If the Draconic Gauntlet is Ascended, you use your element") {
		t.Errorf("missing Ascension body text, got: %s", got)
	}
	if strings.Contains(got, "ASCENSION: DRACONIC GAUNTLET") {
		t.Errorf("raw ASCENSION marker leaked into output, got: %s", got)
	}

	baseEnd := strings.Index(got, "wow-ascension-heading")
	ascensionStart := strings.Index(got, "If the Draconic Gauntlet is Ascended")
	if baseEnd == -1 || ascensionStart == -1 || ascensionStart < baseEnd {
		t.Errorf("Ascension heading/body did not come after the base description, got: %s", got)
	}
}

func TestFormatWoWDescriptionFallsBackWithoutMarker(t *testing.T) {
	raw := "A plain weapon with no Ascension text at all."
	got := formatWoWDescription(raw)
	want := formatDescription(raw)
	if got != want {
		t.Errorf("got %q, want fallback to formatDescription: %q", got, want)
	}
}

// wowBlunderbussAscensionExcerpt is a trimmed excerpt of the real Blunderbuss
// description's own Ascension half — it lists three "•"-marked custom
// properties, confirming formatWoWDescription's per-half formatDescription
// call still detects and renders bullets nested inside the Ascension text.
const wowBlunderbussAscensionExcerpt = `Clip Size: 8 [16] You make a large double-barreled exotic rifle with a rustic appearance using your element. ASCENSION: BLUNDERBUSS If the Blunderbuss is Ascended, it gains one of the following custom properties, granting it new effects. • Sweeping: The normal and maximum ranged of the Blunderbuss doubles. • Vitriolic: The Blunderbuss’ damage becomes Acid. • Withering: The Blunderbuss’ damage becomes Fire and is increased by +5.`

func TestFormatWoWDescriptionPreservesNestedBullets(t *testing.T) {
	got := string(formatWoWDescription(wowBlunderbussAscensionExcerpt))

	if !strings.Contains(got, `<h4 class="wow-ascension-heading">Ascension: Blunderbuss</h4>`) {
		t.Errorf("missing Ascension heading, got: %s", got)
	}
	if !strings.Contains(got, `<ul class="prose-list">`) {
		t.Errorf("missing bullet list in Ascension half, got: %s", got)
	}
	for _, want := range []string{"Sweeping:", "Vitriolic:", "Withering:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing bullet %q, got: %s", want, got)
		}
	}
}
