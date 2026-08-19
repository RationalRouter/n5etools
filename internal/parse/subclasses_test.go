package parse

import (
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

// Fixture exercising the outline-anchored segmentation: group intro with
// selection levels, an subclass with level-gated features, its option list
// (content-classified), a junk sentence bookmark that must be skipped, a
// mid-word hyphen heading wrap, and a class-wide option list.
func TestParseSubclassesFixture(t *testing.T) {
	c := &Class{
		Name: "Scout-Nin", SourcePage: 90, GroupHeading: "SCOUTING TECHNIQUE",
		SubclassLines: mkLines(91,
			"SCOUTING TECHNIQUE",
			"Starting at 3rd level, you choose a Scouting Technique.",
			"Your choice grants you features at 3rd, 6th, 9th and 14th levels.",
			"ARBITER SCOUT",
			"The arbiter directs the battlefield.",
			"SUPERIOR ARBITRATION",
			"When you choose this technique at 3rd level, you gain Superiority Dice.",
			"BATTLEFIELD JUDICATION",
			"Starting at 6th level, allies within 30 feet gain a bonus.",
			"WHO DECIDED THAT!?",
			"Starting at 9th level, you can veto an enemy attack once per rest.",
			"ARBITER MANEUVERS",
			"ASSISTED ACCURACY",
			"You may spend a Superiority Die targeting one allied creature.",
			"ASSISTED SHINOBI-",
			"WARE",
			"Prerequisite: 9th Level",
			"You may spend a Superiority Die to empower a gadget.",
			"SCOUT SIGNALS",
			"A class-wide list of signals.",
			"SMOKE SIGNAL",
			"You can send a message via smoke.",
		),
	}
	sections := []extract.OutlineNode{
		{Title: "Scouting Technique", Children: []extract.OutlineNode{
			{Title: "Arbiter Scout", Children: []extract.OutlineNode{
				{Title: "Superior Arbitration"},
				{Title: "This sentence got bookmarked by accident"},
				{Title: "Battlefield Judication"},
				{Title: "Who Decided That!?"},
			}},
			{Title: "Arbiter Maneuvers", Children: []extract.OutlineNode{
				{Title: "Assisted Accuracy"},
				{Title: "Assisted Shinobi-Ware"},
			}},
		}},
		{Title: "Scout Signals", Children: []extract.OutlineNode{
			{Title: "Smoke Signal"},
		}},
	}
	anomalies := ParseSubclasses(c, sections)
	if len(anomalies) != 1 || !strings.Contains(anomalies[0].Problem, "BOOKMARKED BY ACCIDENT") {
		t.Errorf("anomalies = %+v, want only the junk bookmark", anomalies)
	}
	g := c.Group
	if g == nil {
		t.Fatal("group not parsed")
	}
	if got, want := len(g.SelectionLevels), 4; got != want {
		t.Errorf("selection levels = %v", g.SelectionLevels)
	}
	if len(g.Subclasses) != 1 {
		t.Fatalf("subclasses = %+v", g.Subclasses)
	}
	a := g.Subclasses[0]
	if a.Name != "Arbiter Scout" || !strings.Contains(a.Description, "directs the battlefield") {
		t.Errorf("subclass = %+v", a)
	}
	if len(a.Features) != 3 {
		t.Fatalf("features = %+v", a.Features)
	}
	if a.Features[0].Level == nil || *a.Features[0].Level != 3 ||
		a.Features[2].Name != "Who Decided That!?" || *a.Features[2].Level != 9 {
		t.Errorf("feature levels: %+v", a.Features)
	}

	if len(c.OptionLists) != 2 {
		t.Fatalf("option lists = %+v", c.OptionLists)
	}
	man := c.OptionLists[0]
	if man.Name != "Arbiter Maneuvers" || man.SubclassName != "Arbiter Scout" {
		t.Errorf("maneuvers list = %+v", man)
	}
	if len(man.Options) != 2 || man.Options[0].Name != "Assisted Accuracy" {
		t.Fatalf("maneuver options = %+v", man.Options)
	}
	// Mid-word hyphen wrap joins without a space; prerequisite extracted.
	if man.Options[1].Name != "Assisted Shinobi-Ware" ||
		man.Options[1].Prerequisites != "9th Level" ||
		strings.Contains(man.Options[1].Description, "Prerequisite") {
		t.Errorf("wrapped option = %+v", man.Options[1])
	}
	// Class-wide list has no subclass owner.
	if c.OptionLists[1].Name != "Scout Signals" || c.OptionLists[1].SubclassName != "" ||
		len(c.OptionLists[1].Options) != 1 {
		t.Errorf("class-wide list = %+v", c.OptionLists[1])
	}
}

// Real shape from Science-Nin's Titan option list (queried directly from
// the shipped rules.db) — the Titan's shared base unit card was glued onto
// the end of "Ronin Specialization"'s own bullet list, with no separator,
// by the flat PDF text extractor. ParseSubclasses must split it off onto
// Class.TitanBaseText and leave Ronin Specialization's own description
// clean, without touching its untouched siblings.
func TestParseSubclassesSplitsTitanUnitCard(t *testing.T) {
	c := &Class{
		Name: "Science-Nin", SourcePage: 230, GroupHeading: "SCIENTIFIC INQUIRY",
		SubclassLines: mkLines(230,
			"TITAN",
			"LEGION SPECIALIZATION",
			"Legion Titans act as pillars on the battlefield.",
			"MONARCH SPECIALIZATION",
			"Monarch Titans are built to go the distance.",
			"RONIN SPECIALIZATION",
			"The Ronin Titans focus on mobility. • The Ronin Titan has a movement speed of 35 feet. X Construct, Proficiency bonus = your proficiency bonus, unaligned Hit Points 20+[2*Titan’s Constitution Modifer x Science Nin level] Speed 30 ft. 15 (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3) Senses Darkvision (30 feet). Bash. Melee Weapon Attack: reach 10 ft., one target.",
		),
	}
	sections := []extract.OutlineNode{
		{Title: "Titan", Children: []extract.OutlineNode{
			{Title: "Legion Specialization"},
			{Title: "Monarch Specialization"},
			{Title: "Ronin Specialization"},
		}},
	}
	ParseSubclasses(c, sections)

	if len(c.OptionLists) != 1 || len(c.OptionLists[0].Options) != 3 {
		t.Fatalf("option lists = %+v", c.OptionLists)
	}
	opts := c.OptionLists[0].Options
	if opts[0].Description != "Legion Titans act as pillars on the battlefield." {
		t.Errorf("legion (untouched) = %+v", opts[0])
	}
	if opts[1].Description != "Monarch Titans are built to go the distance." {
		t.Errorf("monarch (untouched) = %+v", opts[1])
	}
	if !strings.HasSuffix(opts[2].Description, "movement speed of 35 feet.") {
		t.Errorf("ronin description did not end cleanly: %q", opts[2].Description)
	}
	if strings.Contains(opts[2].Description, "Construct") {
		t.Errorf("ronin description still contains the unit card: %q", opts[2].Description)
	}
	if !strings.HasPrefix(c.TitanBaseText, "X Construct, Proficiency bonus") ||
		!strings.HasSuffix(c.TitanBaseText, "one target.") {
		t.Errorf("TitanBaseText = %q", c.TitanBaseText)
	}
}

// Martial Techniques is granted by Taijutsu Specialist's base Martial Adept
// feature (no subclass gate), but in the real book it's bookmarked right
// after Passionate Flame's own section — the general lastSubclass heuristic
// would wrongly scope all 20 options to Passionate Flame, confirmed against
// the shipped rules.db before this fix. ParseSubclasses must special-case
// this one option list to a class-wide (SubclassName == "") list.
func TestParseSubclassesMartialTechniquesClassWide(t *testing.T) {
	c := &Class{
		Name: "Taijutsu Specialist", SourcePage: 300, GroupHeading: "TAIJUTSU STYLE",
		SubclassLines: mkLines(300,
			"TAIJUTSU STYLE",
			"Starting at 3rd level, you choose a Taijutsu Style.",
			"PASSIONATE FLAME",
			"Your fists burn with passion.",
			"FISTS OF IRON",
			"When you choose this style at 3rd level, you gain bonus damage.",
			"MARTIAL TECHNIQUES",
			"You learn the following Martial Techniques.",
			"BRUTAL TAIJUTSU",
			"When you would cast a Taijutsu, you can spend any number of martial die.",
		),
	}
	sections := []extract.OutlineNode{
		{Title: "Taijutsu Style", Children: []extract.OutlineNode{
			{Title: "Passionate Flame", Children: []extract.OutlineNode{
				{Title: "Fists of Iron"},
			}},
			{Title: "Martial Techniques", Children: []extract.OutlineNode{
				{Title: "Brutal Taijutsu"},
			}},
		}},
	}
	ParseSubclasses(c, sections)

	if len(c.Group.Subclasses) != 1 || c.Group.Subclasses[0].Name != "Passionate Flame" {
		t.Fatalf("subclasses = %+v", c.Group.Subclasses)
	}
	if len(c.OptionLists) != 1 {
		t.Fatalf("option lists = %+v", c.OptionLists)
	}
	mt := c.OptionLists[0]
	if mt.Name != "Martial Techniques" || mt.SubclassName != "" {
		t.Errorf("Martial Techniques list = %+v, want SubclassName \"\"", mt)
	}
	if len(mt.Options) != 1 || mt.Options[0].Name != "Brutal Taijutsu" {
		t.Errorf("Martial Techniques options = %+v", mt.Options)
	}
}

// Real shape from Science-Nin's "S.N.B Upgrades" bookmark node: its own
// upgrade entries have no level gate of their own, but their bundled prose
// mentions a level number in scaling text ("this bonus increases to +2 at
// 9th level") — enough to trip isSubclass's >=50%-level-gated heuristic and
// misclassify the whole node as an 11th subclass instead of S.N.B
// Specialist's own upgrade catalog. ParseSubclasses must route it into
// c.OptionLists, scoped to S.N.B Specialist (the subclass printed right
// before it), not into c.Group.Subclasses.
func TestParseSubclassesSNBUpgradesNotAPhantomSubclass(t *testing.T) {
	c := &Class{
		Name: "Science-Nin", SourcePage: 200, GroupHeading: "SCIENTIFIC INQUIRY",
		SubclassLines: mkLines(200,
			"SCIENTIFIC INQUIRY",
			"Starting at 3rd level, you choose a Scientific Inquiry.",
			"S.N.B SPECIALIST",
			"You build a Scientific Ninja Beast.",
			"SCIENTIFIC NINJA BEAST",
			"When you choose this inquiry at 3rd level, you gain a companion.",
			"S.N.B UPGRADES",
			"MINOR",
			"ARMORED EXTERIOR Cost: 2 Creation Points. Your S.N.B's AC increases. This bonus increases at 9th level.",
			"REFINED",
			"ENHANCED SIZE Cost: 4 Creation Points. Your S.N.B's size increases. This upgrade improves further at 9th level.",
		),
	}
	sections := []extract.OutlineNode{
		{Title: "Scientific Inquiry", Children: []extract.OutlineNode{
			{Title: "S.N.B Specialist", Children: []extract.OutlineNode{
				{Title: "Scientific Ninja Beast"},
			}},
			{Title: "S.N.B Upgrades", Children: []extract.OutlineNode{
				{Title: "Minor"},
				{Title: "Refined"},
			}},
		}},
	}
	ParseSubclasses(c, sections)

	if len(c.Group.Subclasses) != 1 || c.Group.Subclasses[0].Name != "S.N.B Specialist" {
		t.Fatalf("subclasses = %+v, want only S.N.B Specialist", c.Group.Subclasses)
	}
	if len(c.OptionLists) != 1 {
		t.Fatalf("option lists = %+v", c.OptionLists)
	}
	up := c.OptionLists[0]
	if up.Name != "S.N.B Upgrades" || up.SubclassName != "S.N.B Specialist" {
		t.Errorf("S.N.B Upgrades list = %+v, want SubclassName \"S.N.B Specialist\"", up)
	}
	if len(up.Options) != 2 || up.Options[0].Name != "Minor" || up.Options[1].Name != "Refined" {
		t.Errorf("S.N.B Upgrades options = %+v", up.Options)
	}
}

// Real shape from Weapon Specialist's Gungnir Piercer Form: unlike its 7
// sibling Weapon Forms, "Techniques[changed]" never states its own gating
// level anywhere in its printed text — it launches straight into named
// abilities instead of opening with "Starting at 3rd level, ...". buildFeature
// must fall back to knownFeatureLevelOverrides, keyed by the subclass name,
// only when ordinalLevelRe finds nothing to match.
func TestParseSubclassesKnownFeatureLevelOverride(t *testing.T) {
	c := &Class{
		Name: "Weapon Specialist", SourcePage: 400, GroupHeading: "WEAPON FORMS",
		SubclassLines: mkLines(400,
			"WEAPON FORMS",
			"Starting at 3rd level, you choose a Weapon Form.",
			"Your choice grants you features at 3rd and 7th levels.",
			"GUNGNIR PIERCER FORM",
			"You wield a piercing polearm.",
			"GUNGNIR PIERCER TECHNIQUES[CHANGED]",
			"Fenrir's Claw. When you deal piercing damage, you can pierce through defenses.",
			"GUNGNIR PIERCER STYLES",
			"Starting at 3rd level, you learn a Gungnir Piercer Style.",
		),
	}
	sections := []extract.OutlineNode{
		{Title: "Weapon Forms", Children: []extract.OutlineNode{
			{Title: "Gungnir Piercer Form", Children: []extract.OutlineNode{
				{Title: "Gungnir Piercer Techniques[changed]"},
				{Title: "Gungnir Piercer Styles"},
			}},
		}},
	}
	ParseSubclasses(c, sections)

	if len(c.Group.Subclasses) != 1 {
		t.Fatalf("subclasses = %+v", c.Group.Subclasses)
	}
	feats := c.Group.Subclasses[0].Features
	if len(feats) != 2 {
		t.Fatalf("features = %+v", feats)
	}
	if feats[0].Name != "Gungnir Piercer Techniques[changed]" ||
		feats[0].Level == nil || *feats[0].Level != 3 {
		t.Errorf("Gungnir Piercer Techniques[changed] level = %+v, want 3 via override", feats[0].Level)
	}
}

// Real shape from Hunter-Nin's Blade Warden Blade's Aggression: the printed
// text reads "Beginning at 1oth level, ..." — a lowercase "o" swapped in for
// the digit "0" by PDF extraction, which breaks ordinalLevelRe's \d match
// entirely. knownExtractionSquishes must correct it before the level regex
// runs, so the feature still resolves to a real level (10) instead of NULL.
func TestParseSubclassesLevelTypoSquish(t *testing.T) {
	c := &Class{
		Name: "Hunter-Nin", SourcePage: 150, GroupHeading: "HUNTERS CREEDS",
		SubclassLines: mkLines(150,
			"HUNTERS CREEDS",
			"Starting at 3rd level, you choose a Hunters Creed.",
			"Your choice grants you features at 3rd, 7th and 10th levels.",
			"BLADE WARDEN",
			"You wield a Warden Weapon.",
			"WARDEN'S PROFICIENCY",
			"When you choose this creed at 3rd level, you gain proficiency with your Warden Weapon.",
			"BLADE'S AGGRESSION",
			"Beginning at 1oth level, your Wardens Weapon's damage die is increased by 1 step.",
		),
	}
	sections := []extract.OutlineNode{
		{Title: "Hunters Creeds", Children: []extract.OutlineNode{
			{Title: "Blade Warden", Children: []extract.OutlineNode{
				{Title: "Warden's Proficiency"},
				{Title: "Blade's Aggression"},
			}},
		}},
	}
	ParseSubclasses(c, sections)

	if len(c.Group.Subclasses) != 1 {
		t.Fatalf("subclasses = %+v", c.Group.Subclasses)
	}
	feats := c.Group.Subclasses[0].Features
	if len(feats) != 2 {
		t.Fatalf("features = %+v", feats)
	}
	aggression := feats[1]
	if aggression.Name != "Blade's Aggression" ||
		aggression.Level == nil || *aggression.Level != 10 {
		t.Errorf("Blade's Aggression level = %+v, want 10 after squish fix", aggression.Level)
	}
	if strings.Contains(aggression.Description, "1oth") {
		t.Errorf("Blade's Aggression description still contains the typo: %q", aggression.Description)
	}
}
