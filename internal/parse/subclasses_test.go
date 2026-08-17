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
