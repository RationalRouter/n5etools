package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

// Fixture mirroring the class template: flavor, inspirations, creation
// guidance, quick build, HP/CP dice, proficiencies with a skill choice,
// equipment bullets, wrapped casting lines, level-gated features, a wrapped
// ASI heading, and the subclass section split (which for Medical-Nin reuses
// the exact section heading as a feature name first).
func TestParseClassBookFixture(t *testing.T) {
	lines := mkLines(66,
		"MEDICAL-NIN",
		"A kind shinobi heals allies on the battlefield.",
		"CHARACTER INSPIRATIONS",
		"Built with Tsunade and Sakura in mind.",
		"CREATING A MEDICAL-NIN",
		"Think about how your character approaches medicine.",
		"QUICK BUILD",
		"Put your highest score in Wisdom, then Constitution.",
		"CLASS FEATURES",
		"As a Medical-Nin, you gain the following class features.",
		"HIT POINTS",
		"Hit Dice: 1d8 per Medical-Nin level",
		"Hit Points at 1st Level and beyond: 8 + your constitution modifier",
		"CHAKRA POINTS",
		"Chakra Dice: 1d10 per Medical-Nin level",
		"PROFICIENCIES",
		"Armor: Light armor",
		"Weapons: Simple Weapons",
		"Ninja Tools: Medical Kit, Poison Kit",
		"Saving Throws: Intelligence, Wisdom",
		"Skills: Medicine, Choose two from Chakra Control,",
		"Insight, Perception",
		"EQUIPMENT",
		"You start with the following equipment:",
		"• 1 Simple weapon",
		"• (a) One Kunai stack or (b) One Shuriken",
		"stack",
		"JUTSU CASTING",
		"NINJUTSU",
		"Ninjutsu save DC = 8 + your Proficiency Bonus + your",
		"Intelligence Modifier",
		"Ninjutsu attack modifier = your Proficiency Bonus + your Intelligence Modifier",
		"GENJUTSU",
		"Genjutsu save DC = 8 + your Proficiency Bonus + your Wisdom Modifier",
		"CHAKRA SCALPEL",
		"Starting at 1st Level, you can form a blade of chakra.",
		"TENETS OF MEDICINE",
		"Starting at 3rd level, you choose a Tenet of Medicine. Your",
		"choice grants you features at 3rd, 7th, 11th and 15th levels.",
		"ABILITY SCORE",
		"IMPROVEMENT/FEAT",
		"When you reach certain levels you can increase an ability score.",
		"TENETS OF MEDICINE",
		"The subclass section begins here.",
		"COMBAT MEDIC",
		"A tenet focused on front-line healing.",
	)
	classes, anomalies := ParseClassBook(lines)
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if len(classes) != 1 {
		t.Fatalf("got %d classes, want 1", len(classes))
	}
	c := classes[0]
	if c.Name != "Medical-Nin" || c.SourcePage != 66 {
		t.Errorf("class = %q p%d", c.Name, c.SourcePage)
	}
	if !strings.Contains(c.Description, "heals allies") ||
		!strings.Contains(c.Description, "Tsunade") ||
		!strings.Contains(c.Description, "approaches medicine") {
		t.Errorf("description = %q", c.Description)
	}
	if !strings.Contains(c.QuickBuild, "highest score in Wisdom") {
		t.Errorf("quick build = %q", c.QuickBuild)
	}
	if c.HitDie != 8 || c.ChakraDie != 10 {
		t.Errorf("dice = d%d/d%d, want d8/d10", c.HitDie, c.ChakraDie)
	}

	wantProfs := []ClassProficiency{
		{Kind: "armor", Value: "Light armor"},
		{Kind: "weapon", Value: "Simple Weapons"},
		{Kind: "tool", Value: "Medical Kit"},
		{Kind: "tool", Value: "Poison Kit"},
		{Kind: "saving_throw", Value: "Intelligence"},
		{Kind: "saving_throw", Value: "Wisdom"},
		{Kind: "skill", Value: "Medicine"},
		{Kind: "skill_choice", Value: "Chakra Control", ChooseN: 2},
		{Kind: "skill_choice", Value: "Insight", ChooseN: 2},
		{Kind: "skill_choice", Value: "Perception", ChooseN: 2},
	}
	if len(c.Proficiencies) != len(wantProfs) {
		t.Fatalf("got %d proficiencies, want %d: %+v", len(c.Proficiencies), len(wantProfs), c.Proficiencies)
	}
	for i, want := range wantProfs {
		if c.Proficiencies[i] != want {
			t.Errorf("prof %d = %+v, want %+v", i, c.Proficiencies[i], want)
		}
	}

	if len(c.Equipment) != 2 || c.Equipment[1] != "(a) One Kunai stack or (b) One Shuriken stack" {
		t.Errorf("equipment = %+v", c.Equipment)
	}

	// The wrapped ninjutsu DC line must still yield int; genjutsu wis.
	if len(c.Casting) != 2 ||
		c.Casting[0] != (ClassCasting{Discipline: "ninjutsu", Ability: "int"}) ||
		c.Casting[1] != (ClassCasting{Discipline: "genjutsu", Ability: "wis"}) {
		t.Errorf("casting = %+v", c.Casting)
	}

	if len(c.Features) != 3 {
		t.Fatalf("got %d features, want 3: %+v", len(c.Features), c.Features)
	}
	if c.Features[0].Name != "Chakra Scalpel" || c.Features[0].Level == nil || *c.Features[0].Level != 1 {
		t.Errorf("feature 0 = %+v", c.Features[0])
	}
	// First TENETS OF MEDICINE occurrence is the choice-granting feature.
	if c.Features[1].Name != "Tenets of Medicine" || c.Features[1].Level == nil || *c.Features[1].Level != 3 {
		t.Errorf("feature 1 = %+v", c.Features[1])
	}
	// Wrapped ASI heading joins into one feature.
	if c.Features[2].Name != "Ability Score Improvement/Feat" {
		t.Errorf("feature 2 = %+v", c.Features[2])
	}

	// Second occurrence starts the subclass section.
	if c.GroupHeading != "TENETS OF MEDICINE" {
		t.Errorf("group heading = %q", c.GroupHeading)
	}
	if len(c.SubclassLines) != 4 || c.SubclassLines[0].Text != "TENETS OF MEDICINE" {
		t.Errorf("subclass lines = %+v", c.SubclassLines)
	}
}

// Puppet Master's "Puppet Tool" feature has a sidebar stat card interleaved
// into the flat PDF text stream mid-paragraph — this fixture reproduces the
// exact confirmed real text (queried directly from the shipped rules.db)
// and checks flushFeature splits it into Class.PuppetToolStatBlock instead
// of leaving it glued onto the feature's prose.
func TestParseClassBookSplitsPuppetToolUnitCard(t *testing.T) {
	lines := mkLines(142,
		"PUPPET MASTER",
		"CHARACTER INSPIRATIONS",
		"A tinkerer who fights through their creations.",
		"QUICK BUILD",
		"Put your highest score in Dexterity or Intelligence.",
		"CLASS FEATURES",
		"As a Puppet Master, you gain the following class features.",
		"HIT POINTS",
		"Hit Dice: 1d8 per Puppet Master level",
		"CHAKRA POINTS",
		"Chakra Dice: 1d10 per Puppet Master level",
		"PROFICIENCIES",
		"Armor: Light armor",
		"EQUIPMENT",
		"• 1 Simple weapon",
		"JUTSU CASTING",
		"NINJUTSU",
		"Ninjutsu save DC = 8 + your Proficiency Bonus + your Intelligence Modifier",
		"PUPPET TOOL",
		"Starting at 6th level when you do not command a Puppet you are connected to on your turn, that puppet takes the dodge action. You also do not need to use your Reaction to command a Puppet to take its Reaction.",
		"Medium Construct, Proficiency = Puppet Master’s Proficiency Hit Points 4 + [(5+Constitution Modifier) x Puppet Master level] Speed 30 ft. 15 (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3) Senses Passive Perception 7 Bound. A Puppet is bound to its user via chakra threads and can be no more than 500ft. from you. If the strings are not bound to the Puppet, it cannot move or take actions of any kind. If a creature other than its creator uses chakra threads to manipulate the puppet, it cannot use any Upgrades. Hollow Shell. Puppet Tools cannot be affected by Genjutsu. Mechanical Limits. Puppets cannot cast jutsu (or have such jutsu cast through them), or use effects, that increase its ranks of exhaustion or make Clones.",
	)
	classes, _ := ParseClassBook(lines)
	if len(classes) != 1 {
		t.Fatalf("got %d classes, want 1", len(classes))
	}
	c := classes[0]
	if len(c.Features) != 1 || c.Features[0].Name != "Puppet Tool" {
		t.Fatalf("features = %+v", c.Features)
	}
	desc := c.Features[0].Description
	if !strings.HasSuffix(desc, "take its Reaction.") {
		t.Errorf("description did not end cleanly, got suffix %q", desc[max(0, len(desc)-60):])
	}
	if strings.Contains(desc, "Medium Construct") {
		t.Errorf("description still contains the unit card: %q", desc)
	}

	sb := c.PuppetToolStatBlock
	if sb == nil {
		t.Fatalf("PuppetToolStatBlock not populated")
	}
	want := PuppetToolStatBlock{
		CreatureType:        "Medium Construct",
		ProficiencyRuleText: "Puppet Master’s Proficiency",
		HPBase:              4,
		HPConBonusAdd:       5,
		Speed:               30,
		Str:                 15, Dex: 13, Con: 13, Int: 5, Wis: 5, Cha: 5,
		PassivePerception: 7,
	}
	got := *sb
	got.TraitsText = "" // checked separately below
	if got != want {
		t.Errorf("stat block = %+v, want %+v", got, want)
	}
	if !strings.HasPrefix(sb.TraitsText, "Bound. A Puppet is bound") {
		t.Errorf("traits text prefix = %q", sb.TraitsText[:min(60, len(sb.TraitsText))])
	}
	if !strings.HasSuffix(sb.TraitsText, "make Clones.") {
		t.Errorf("traits text suffix = %q", sb.TraitsText)
	}
}

// Real shape from Weapon Specialist's Weapon Flurry (2nd level): its
// "following Flurry Techniques" are printed as separate headings with no
// gating level of their own. Enhanced Deflection never states a level at
// all, so knownClassFeatureLevelOverrides must fill in where ordinalLevelRe
// finds nothing. Chakra Strike DOES contain an ordinal ("...at 11th
// level"), but it's an internal damage-escalation clause, not the
// feature's own gate — the override must win over that match too, unlike
// subclasses.go's fallback-only knownFeatureLevelOverrides.
func TestParseClassBookKnownFeatureLevelOverride(t *testing.T) {
	lines := mkLines(200,
		"WEAPON SPECIALIST",
		"CHARACTER INSPIRATIONS",
		"A warrior who trains with a signature weapon.",
		"QUICK BUILD",
		"Put your highest score in Strength or Dexterity.",
		"CLASS FEATURES",
		"As a Weapon Specialist, you gain the following class features.",
		"HIT POINTS",
		"Hit Dice: 1d10 per Weapon Specialist level",
		"CHAKRA POINTS",
		"Chakra Dice: 1d8 per Weapon Specialist level",
		"PROFICIENCIES",
		"Armor: Light armor",
		"EQUIPMENT",
		"• 1 Martial weapon",
		"JUTSU CASTING",
		"NINJUTSU",
		"Ninjutsu save DC = 8 + your Proficiency Bonus + your Intelligence Modifier",
		"WEAPON FLURRY",
		"Also, at 2nd Level, you are an unrelenting flurry of weapon attacks. Once per turn, you can use one of the following Flurry Techniques.",
		"ENHANCED DEFLECTION",
		"As a Reaction to taking damage, you gain DR vs the triggering creature, equal to the maximum possible result of your Flurry Die.",
		"CHAKRA STRIKE",
		"On your turn, you deal additional damage equal to +2 Flurry Die. This increases to +3 Flurry Die at 11th level.",
	)
	classes, _ := ParseClassBook(lines)
	if len(classes) != 1 {
		t.Fatalf("got %d classes, want 1", len(classes))
	}
	c := classes[0]
	if len(c.Features) != 3 {
		t.Fatalf("got %d features, want 3: %+v", len(c.Features), c.Features)
	}
	if c.Features[0].Name != "Weapon Flurry" || c.Features[0].Level == nil || *c.Features[0].Level != 2 {
		t.Errorf("Weapon Flurry level = %+v, want 2", c.Features[0].Level)
	}
	if c.Features[1].Name != "Enhanced Deflection" || c.Features[1].Level == nil || *c.Features[1].Level != 2 {
		t.Errorf("Enhanced Deflection level = %+v, want 2 via override (ordinalLevelRe finds nothing)", c.Features[1].Level)
	}
	if c.Features[2].Name != "Chakra Strike" || c.Features[2].Level == nil || *c.Features[2].Level != 2 {
		t.Errorf("Chakra Strike level = %+v, want 2 via override (must beat the wrong 11th-level regex match)", c.Features[2].Level)
	}
}

// Whole-book regression (verified 2026-07-17 against v3.12). Skips when the
// sourcebook is absent.
func TestParseClassBookFullCompendium(t *testing.T) {
	path := filepath.Join("/home/sergio/Documents/N5E", "Orochimarus_Observation_Compendium.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook not available: %v", err)
	}
	doc, err := extract.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []Line
	for n := 6; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, Line{Page: n, Text: ln})
		}
	}
	classes, anomalies := ParseClassBook(lines)
	for _, a := range anomalies {
		t.Errorf("anomaly p%d %s: %s", a.Page, a.Subject, a.Problem)
	}
	if len(classes) != 11 {
		t.Fatalf("parsed %d classes, want 11", len(classes))
	}

	// Stage 2: subclasses, anchored on the bookmark outline.
	names := make([]string, len(classes))
	for i, c := range classes {
		names[i] = c.Name
	}
	sections := ClassSections(doc.Outline(), names)
	var archAnomalies []Anomaly
	for i := range classes {
		archAnomalies = append(archAnomalies, ParseSubclasses(&classes[i], sections[classes[i].Name])...)
	}
	// Exactly one known defect: a body sentence Word bookmarked by accident
	// in Cooking-Nin.
	if len(archAnomalies) != 1 || !strings.Contains(archAnomalies[0].Problem, "YOU MAY USE CHARISMA") {
		t.Errorf("subclass anomalies = %+v, want only Cooking-Nin's junk bookmark", archAnomalies)
	}
	totArch, totArchFeat, totLists, totOpts := 0, 0, 0, 0
	for _, c := range classes {
		if c.Group == nil {
			t.Errorf("%s: no subclass group", c.Name)
			continue
		}
		totArch += len(c.Group.Subclasses)
		for _, a := range c.Group.Subclasses {
			totArchFeat += len(a.Features)
		}
		totLists += len(c.OptionLists)
		for _, l := range c.OptionLists {
			totOpts += len(l.Options)
		}
	}
	if totArch != 92 {
		t.Errorf("total subclasses = %d, want 92", totArch)
	}
	if totArchFeat != 652 {
		t.Errorf("total subclass features = %d, want 652", totArchFeat)
	}
	if totLists != 37 {
		t.Errorf("total option lists = %d, want 37", totLists)
	}
	if totOpts != 394 {
		t.Errorf("total options = %d, want 394", totOpts)
	}

	// The closing class-feat sections (verified 2026-07-18 against v3.12).
	feats, featAnomalies := ParseClassFeats(lines)
	if len(feats) != 78 {
		t.Errorf("class feats = %d, want 78", len(feats))
	}
	withClass := 0
	for _, f := range feats {
		if f.ClassName != "" {
			withClass++
		}
		if f.Category == "" || f.Description == "" {
			t.Errorf("feat %q: empty category or description", f.Name)
		}
	}
	if withClass != 52 {
		t.Errorf("feats with an Archetype: class label = %d, want 52", withClass)
	}
	// One known quirk: a chart printed mid-section is appended to the
	// nearest feat and flagged.
	if len(featAnomalies) != 1 || !strings.Contains(featAnomalies[0].Problem, "sidebar/table") {
		t.Errorf("feat anomalies = %+v", featAnomalies)
	}
	for _, c := range classes {
		if c.HitDie == 0 || c.ChakraDie == 0 {
			t.Errorf("%s: missing dice (d%d/d%d)", c.Name, c.HitDie, c.ChakraDie)
		}
		if len(c.Casting) != 3 {
			t.Errorf("%s: %d casting disciplines, want 3", c.Name, len(c.Casting))
		}
		if len(c.Features) == 0 || len(c.Proficiencies) == 0 || len(c.Equipment) == 0 {
			t.Errorf("%s: features/profs/equipment empty", c.Name)
		}
		if c.GroupHeading == "" || len(c.SubclassLines) == 0 {
			t.Errorf("%s: subclass section not split", c.Name)
		}
	}
	// Spot totals pinned to v3.12 print.
	totFeatures := 0
	for _, c := range classes {
		totFeatures += len(c.Features)
	}
	if totFeatures != 168 {
		t.Errorf("total class features = %d, want 168", totFeatures)
	}
}
