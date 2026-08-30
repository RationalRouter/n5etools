// Class compendium parser, stage 2: subclass sections.
//
// Anchor: the PDF bookmark outline (extract.Document.Outline). Word built it
// from the book's heading styles, so it is the author's own structure:
//
//	Table of Contents
//	  Genjutsu Specialist        ← class (stage 1 parses this)
//	  Genjutsu Pledges           ← THE subclass group (matches GroupHeading)
//	    Beguiler                 ← subclass
//	      Inspired Appearance    ← subclass feature
//	    ...
//	  Genjutsu Inception         ← class-wide option list
//	  Malleable Mirages          ← class-wide option list
//	  Hunter-Nin                 ← next class
//
// Inside a group, an subclass's pick-list rides as its next sibling
// ("Arbiter Scout" then "Arbiter Maneuvers"). Bookmark titles don't say
// which depth-2 nodes are subclasses and which are option lists, so nodes
// are classified by CONTENT: subclass features are level-gated prose
// ("When you choose this path at 2nd level…"), options mostly aren't.
//
// The body text is segmented by matching caps headings against the
// bookmark titles in pre-order. Headings with no bookmark (chart captions,
// decorative boxes) stay as content of the current node; bookmarks that
// never match the text are anomalies.
package parse

import (
	"regexp"
	"sort"
	"strings"

	"github.com/sergio/n5e/internal/extract"
)


// ClassOption is one purchasable/selectable entry of an option list
// (a maneuver, mirage, upgrade, …).
type ClassOption struct {
	Name          string
	Prerequisites string // "Prerequisite: …" prefix line, when present
	Description   string
	SourcePage    int

	// StatBlock is set whenever buildOptionOn's call to SplitStatBlock finds
	// a companion/summon stat card glued into this option's raw text — see
	// statblock.go. Found is false for the overwhelming majority of
	// options, which never had anything split out.
	StatBlock StatBlockMatch
}

// OptionList is a named list of options, either class-wide ("Malleable
// Mirages") or belonging to one subclass ("Arbiter Maneuvers").
type OptionList struct {
	Name         string
	SubclassName string // "" for class-wide lists
	Description  string // intro prose between the heading and first option
	Options      []ClassOption
	SourcePage   int
}

// Subclass is one subclass choice within a class's group.
type Subclass struct {
	Name        string
	Description string // flavor prose before the first feature
	Features    []ClassFeature
	SourcePage  int
}

// SubclassGroup is a class's whole subclass system.
type SubclassGroup struct {
	DisplayName     string
	SelectionLevels []int  // levels granting subclass features, from the intro
	Description     string // the intro prose itself
	Subclasses      []Subclass
	SourcePage      int
}

// knownExtractionSquishes hand-confirms a small, closed list of PDF
// text-extraction artifacts — most commonly a lost space at a text-run
// boundary (a real extraction-layer information loss, not something a
// general regex can safely recover — a blind "insert a space at every
// lowercase-to-uppercase transition" rule would also mangle real compound
// proper nouns like "TenTen" or "DeviantArt" credit lines, both confirmed
// present elsewhere in this same corpus), but also a lost WORD where one
// confirmed (Cooking-Nin's Shinobi Snacks drops "modifier" outright, not
// just a space around it). Each entry here was found by sweeping every
// prose column in rules.db for this shape and hand-verifying against the
// source PDF page or a sibling feature's own matching text, not guessed —
// extend one confirmed instance at a time, the same discipline
// capsEntryPattern's own doc follows.
var knownExtractionSquishes = strings.NewReplacer(
	"Internal Radio LinkYou", "Internal Radio Link You", // Science-Nin's Combat Programming (S&B Specialist)
	"1oth level", "10th level", // Hunter-Nin's Blade Warden Blade's Aggression: a lowercase "o" swapped in for the digit "0" breaks ordinalLevelRe's \d match entirely
	"Natural Weapon of to summon you chose.", "Natural Weapon of the Sage Creature you chose to summon.", // Puppet Master's Green Technique Bestial Framework: garbled, ungrammatical printed sentence
	"equal to your proficiency bonus plus your Intelligence.", "equal to your proficiency bonus plus your Intelligence modifier.", // Cooking-Nin's Shinobi Snacks: the word "modifier" is dropped after "Intelligence" — confirmed by this same feature's own War and Food (7th level), which spells out "equal to your intelligence modifier" for the identical recreate-Snacks effect
	"IRON CLAD SHIELD Armor Name AC Bulk Properties Iron Clad Shield +1 1 Bulk Blocking, Light martial die", "martial die", // Taijutsu Specialist's Ironclad Technique (L20): the L3 Ironclad feature's own truncated "Iron Clad Shield" stat block table resurfaced glued mid-sentence into this unrelated feature's text

	// Medical-Nin's Natural Medicine/Shaman/Transmuter subclasses each print
	// a 4-row "5th/9th/13th/17th" jutsu chart at the end of their own
	// 17th-level feature; the leading digit off each of the first three
	// rows' ordinal is dropped, reading as a bare "th" (Natural Medicine
	// loses its own "17th" row's digit too — its chart has no intact row to
	// anchor a shorter, generalized pattern against, so all 4 of its rows
	// are listed explicitly below rather than 3). Same lost-character-at-a-
	// text-run-boundary shape as "1oth level" above; each pair confirmed
	// against dist/rules.db, not guessed — see
	// 0058_medical_nin_chart_fixes.sql for the matching one-time repair of
	// an already-shipped install.
	"Feature th Chakra Transfer", "Feature 5th Chakra Transfer", // Natural Medicine's Natures Avatar
	"target gains. th Gift of the Apex", "target gains. 9th Gift of the Apex",
	"first selection. th Bestial Art Predator", "first selection. 13th Bestial Art Predator",
	"to yourself. th Supreme Water Lion", "to yourself. 17th Supreme Water Lion",
	"Feature th Vampiric Touch", "Feature 5th Vampiric Touch", // Shaman's Master of Hexes
	"of you. th Phantasmal killer", "of you. 9th Phantasmal killer",
	"affected creature. th Aura of Power", "affected creature. 13th Aura of Power",
	"Feature th Restorative", "Feature 5th Restorative", // Transmuter's Transmogrified Biology
	"Class level. th Curse of Prey", "Class level. 9th Curse of Prey",
	"Penalty to -6. th Reconstructive Hand", "Penalty to -6. 13th Reconstructive Hand",
)

func fixKnownExtractionSquish(s string) string {
	return knownExtractionSquishes.Replace(s)
}

// knownFeatureLevelOverrides hand-confirms a small, closed list of subclass
// features whose printed text never states its own gating level anywhere in
// its description (unlike every sibling feature of the same shape), so
// ordinalLevelRe has nothing to match against. Keyed by subclass name, then
// feature name. Consulted by buildFeature only as a fallback when the regex
// finds no match — a real ordinal in the text always wins. Each entry here
// was confirmed by reading every sibling feature's own opening sentence, not
// guessed.
var knownFeatureLevelOverrides = map[string]map[string]int{
	// All 7 other Weapon Forms' own "Techniques[changed]" feature opens with
	// "Starting at 3rd level, ..."; Gungnir Piercer Form's skips straight to
	// its named abilities and never states a level at all.
	"Gungnir Piercer Form": {"Gungnir Piercer Techniques[changed]": 3},
	// Science-Nin's "Future of Shinobi" capstones open with "At Level 20,
	// ..." (a bare number after the word "Level", no ordinal suffix), which
	// ordinalLevelRe cannot match -- unlike the other 5 subclasses' own
	// capstones of the same shape, which open "At/Finally at 20th level"
	// and match the regex directly.
	"Mad Scientist": {"The Future of Shinobi: Biology": 20},
	"Ninjaneer":     {"The Future of Shinobi: Weapons": 20},
	"Shinobi-Ware":  {"The Future of Shinobi: Shinobi-Ware": 20},
	"Spyware":       {"The Future of Shinobi: Programs": 20},
	"Technobi":      {"The Future of Shinobi: Scrolls": 20},
	// Herbalist's own identity feature ("You may use Charisma in place of
	// Wisdom for calculating your Genjutsu attack bonus and DC") never
	// states a level in its text at all, unlike every other Cooking-Nin
	// Focus's own sort_order-0 identity feature (Expert Combatant, Fast
	// and Furious, Water and Oil Do Mix, ..., all explicitly 2nd level —
	// confirmed against dist/rules.db, every sibling matches 2).
	"Herbalist": {"Gaseous Haze": 2},
}

// mistaggedSubclassTable describes one Hunter-Nin subclass option table (a
// technique/property/attachment list, printed with its own ALL-CAPS
// heading) that the flat PDF text extractor glued onto the wrong sibling
// feature instead of the one that actually grants the choice. Every Hunters
// Creed subclass's 3rd-level "Proficiency" feature says "Select one of the
// following ... Technique/Property/Attachment", but in 7 of the 8 subclasses
// the table itself resurfaces appended to a LATER, unrelated feature's raw
// text block instead — confirmed against dist/rules.db for every entry
// below (only Arsenalist, whose option list is a plain bullet list rather
// than an ALL-CAPS-headed table, escaped the bug).
type mistaggedSubclassTable struct {
	subclass string // Subclass.Name
	marker   string // the table's own printed ALL-CAPS heading, verbatim
	from     string // ClassFeature.Name currently holding the table
	to       string // ClassFeature.Name that grants the choice and should hold it instead

	// resumeAfter, when set, is the literal tail of FROM's own sentence
	// that the extractor spliced the table into the MIDDLE of, rather than
	// appending it cleanly onto the end of FROM's description like every
	// other entry here — the table run ends where this text begins, and
	// this text is stitched back onto FROM, not moved to TO along with the
	// table.
	resumeAfter string
}

var mistaggedSubclassTables = []mistaggedSubclassTable{
	{subclass: "Blade Warden", marker: "WARDEN WEAPON PROPERTY TABLE", from: "Superior Offense", to: "Warden’s Proficiency"},
	{subclass: "Necrotic Hand", marker: "MEDICAL ASSASSINATION TECHNIQUE TABLE", from: "Necrotic Touch", to: "Medical Proficiency"},
	{subclass: "Grave Stalker", marker: "SHADOW ASSASSINATION TECHNIQUE TABLE", from: "Master Ambusher", to: "Stalkers Proficiency",
		resumeAfter: "can attempt to interject socially in this way, once every 10 minutes."},
	{subclass: "Undertaker", marker: "TOXIC ASSASSINATION TECHNIQUE TABLE", from: "False Faces", to: "Toxic Proficiency"},
	{subclass: "Vice Agent", marker: "VICE ASSASSINATION TECHNIQUE TABLE", from: "Arrogance’s Influence", to: "Sin’s Proficiency"},
	{subclass: "Void Walker", marker: "VOID ASSASSINATION TECHNIQUE TABLE", from: "Vorpal Strike", to: "Stalker Proficiency"},
	{subclass: "Wolves Legacy", marker: "PROSTHETIC ATTACHMENTS TABLE", from: "Eyes of a Shinobi", to: "Wolf’s Proficiency"},
}

// redistributeMistaggedTables applies every mistaggedSubclassTables entry
// for the given subclass, moving each table's text from the feature it was
// wrongly glued to over to the feature that actually grants the choice.
// Mutates features in place through pointers into its backing array; a
// no-op for any subclass with no matching entries, and for any entry whose
// marker text isn't found (e.g. re-running against an already-fixed row).
func redistributeMistaggedTables(subclassName string, features []ClassFeature) {
	for _, t := range mistaggedSubclassTables {
		if t.subclass != subclassName {
			continue
		}
		var from, to *ClassFeature
		for i := range features {
			switch features[i].Name {
			case t.from:
				from = &features[i]
			case t.to:
				to = &features[i]
			}
		}
		if from == nil || to == nil {
			continue
		}
		idx := strings.Index(from.Description, t.marker)
		if idx < 0 {
			continue
		}
		before := strings.TrimSpace(from.Description[:idx])
		table := from.Description[idx:]
		after := ""
		if t.resumeAfter != "" {
			if ai := strings.Index(table, t.resumeAfter); ai >= 0 {
				after = strings.TrimSpace(table[ai:])
				table = table[:ai]
			}
		}
		table = strings.TrimSpace(table)
		if after == "" {
			from.Description = before
		} else {
			from.Description = before + " " + after
		}
		to.Description = strings.TrimSpace(to.Description) + " " + table
	}
}

// medicalNinChartMarker, medicalNinChartFromSubclass/Feature, and
// medicalNinChartToSubclass/Feature describe the one cross-SUBCLASS instance
// of the same table-misattribution bug mistaggedSubclassTables fixes within
// a single subclass's own feature list: Medical-Nin's Combat Medic Chart
// (the 4-row jutsu chart at the end of Combat Medic's own 17th-level
// feature) is glued by the flat PDF text extractor onto the END of a
// DIFFERENT subclass's 17th-level feature — Black Medicine's own Venomous
// Sting — instead of Combat Medic's own Yin Seal: Release. Confirmed
// against dist/rules.db: Venomous Sting's description runs straight from
// Black Medicine's own Black Medicine Chart into "COMBAT MEDIC CHART Level
// Jutsu Learned Jutsu Feature 5th Pressure Point Barrage ..." with no
// separator, while Yin Seal: Release's own description holds only its own
// feature text and no chart at all. redistributeMistaggedTables can't fix
// this: it only ever moves text between two features of the SAME subclass's
// own feature slice, passed in one subclass at a time as each is parsed —
// this move needs both subclasses already built, so it runs as a separate
// post-pass over the whole group instead (redistributeMedicalNinCharts).
const (
	medicalNinChartMarker       = "COMBAT MEDIC CHART"
	medicalNinChartFromSubclass = "Black Medicine"
	medicalNinChartFromFeature  = "Venomous Sting"
	medicalNinChartToSubclass   = "Combat Medic"
	medicalNinChartToFeature    = "Yin Seal: Release"
)

// redistributeMedicalNinCharts moves the Combat Medic Chart (see above)
// after every subclass in g has been parsed. A no-op if either named
// subclass/feature isn't found, or if the marker text isn't present (e.g.
// re-running against an already-fixed source).
func redistributeMedicalNinCharts(g *SubclassGroup) {
	var from, to *ClassFeature
	for i := range g.Subclasses {
		switch g.Subclasses[i].Name {
		case medicalNinChartFromSubclass:
			for j := range g.Subclasses[i].Features {
				if g.Subclasses[i].Features[j].Name == medicalNinChartFromFeature {
					from = &g.Subclasses[i].Features[j]
				}
			}
		case medicalNinChartToSubclass:
			for j := range g.Subclasses[i].Features {
				if g.Subclasses[i].Features[j].Name == medicalNinChartToFeature {
					to = &g.Subclasses[i].Features[j]
				}
			}
		}
	}
	if from == nil || to == nil {
		return
	}
	idx := strings.Index(from.Description, medicalNinChartMarker)
	if idx < 0 {
		return
	}
	table := strings.TrimSpace(from.Description[idx:])
	from.Description = strings.TrimSpace(from.Description[:idx])
	to.Description = strings.TrimSpace(to.Description) + " " + table
}

// ordinalRe matches a bare ordinal ("2nd", "18th") — the group intros list
// selection levels as "at 2nd, 6th, 10th, 14th and 18th levels", where only
// the last ordinal touches the word "levels".
var ordinalRe = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)\b`)

// selectionLevels collects the distinct levels named early in a group intro
// (the intro is 2-4 sentences; scanning further would pick up feature text).
func selectionLevels(intro string) []int {
	if len(intro) > 500 {
		intro = intro[:500]
	}
	seen := map[int]bool{}
	var levels []int
	for _, m := range ordinalRe.FindAllStringSubmatch(intro, -1) {
		lvl := atoiSafe(m[1])
		if lvl >= 1 && lvl <= 20 && !seen[lvl] {
			seen[lvl] = true
			levels = append(levels, lvl)
		}
	}
	sort.Ints(levels)
	return levels
}

// singularNorm folds a title to a crude singular form so the group section
// ("HUNTERS CREEDS") can be matched with its choice-granting class feature
// ("Hunter Creed"): each word drops a plural -s / -ies ending.
func singularNorm(s string) string {
	words := strings.Fields(normTitle(s))
	for i, w := range words {
		switch {
		case len(w) > 4 && strings.HasSuffix(w, "IES"):
			words[i] = strings.TrimSuffix(w, "IES") + "Y"
		case len(w) > 3 && strings.HasSuffix(w, "S") &&
			!strings.HasSuffix(w, "SS") && !strings.HasSuffix(w, "US") && !strings.HasSuffix(w, "IS"):
			words[i] = strings.TrimSuffix(w, "S")
		}
	}
	return strings.Join(words, " ")
}

// subclassHeadingRe is deliberately looser than capsLineRe: subclass and
// feature headings carry punctuation ("BLACK TECHNIQUE ~ PUPPETEER",
// "JACK OF ALL, MASTER OF NONE", "WHO DECIDED THAT!?", "SHAMAN’S HEX").
// Loose is safe here — a line only becomes a heading when its normalized
// text equals the next bookmark title.
var subclassHeadingRe = regexp.MustCompile(`^[\p{Lu}0-9 ~/&.,:;()'’‘!?\[\]\-–—…]+$`)

func subclassHeadingLine(s string) bool {
	if !subclassHeadingRe.MatchString(s) {
		return false
	}
	return strings.ContainsFunc(s, func(r rune) bool { return r >= 'A' && r <= 'Z' })
}

// normTitle folds a bookmark title or heading line for matching: uppercase,
// collapsed spaces, straight apostrophes.
func normTitle(s string) string {
	s = strings.ToUpper(strings.Join(strings.Fields(s), " "))
	return strings.NewReplacer("’", "'", "‘", "'").Replace(s)
}

// ClassSections returns, per class name, the outline nodes between that
// class's bookmark and the next class's — its subclass group and any
// class-wide option lists, in book order.
func ClassSections(outline []extract.OutlineNode, classNames []string) map[string][]extract.OutlineNode {
	// The per-class nodes live under the "Table of Contents" top node.
	var toc []extract.OutlineNode
	for _, n := range outline {
		if normTitle(n.Title) == "TABLE OF CONTENTS" {
			toc = n.Children
		}
	}
	isClass := map[string]string{}
	for _, name := range classNames {
		isClass[normTitle(name)] = name
	}
	sections := map[string][]extract.OutlineNode{}
	current := ""
	for _, n := range toc {
		if name, ok := isClass[normTitle(n.Title)]; ok {
			current = name
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], n)
		}
	}
	return sections
}

// ParseSubclasses consumes the class's SubclassLines using its outline
// sections, filling c.Group and c.OptionLists.
func ParseSubclasses(c *Class, sections []extract.OutlineNode) []Anomaly {
	var anomalies []Anomaly
	flag := func(page int, problem string) {
		anomalies = append(anomalies, Anomaly{Page: page, Subject: c.Name, Problem: problem})
	}
	if len(c.SubclassLines) == 0 || len(sections) == 0 {
		flag(c.SourcePage, "no subclass sections to parse")
		return anomalies
	}

	// Flatten the sections to a pre-order match sequence.
	type expected struct {
		title string
		depth int // 0 = section, 1 = subclass/list, 2 = feature/option
	}
	var seq []expected
	var add func(n extract.OutlineNode, depth int)
	add = func(n extract.OutlineNode, depth int) {
		seq = append(seq, expected{normTitle(n.Title), depth})
		for _, ch := range n.Children {
			add(ch, depth+1)
		}
	}
	for _, s := range sections {
		add(s, 0)
	}

	// Segment the body: each matched heading opens a node; everything up to
	// the next matched heading is its content.
	type segment struct {
		title   string
		depth   int
		page    int
		content []string
	}
	// Some bookmarks are junk (Word occasionally bookmarks a body sentence)
	// and will never match a heading — a heading may therefore match a few
	// entries ahead, skipping (and flagging) the junk in between.
	const matchWindow = 5
	var segs []segment
	next := 0
	arch := c.SubclassLines
	for i := 0; i < len(arch); i++ {
		ln := arch[i]
		if next < len(seq) && subclassHeadingLine(ln.Text) {
			// Long headings wrap across up to three caps lines — try the
			// line alone, then progressively joined with its successors.
			joined := normTitle(ln.Text)
			name := tidyName(ln.Text)
			matched := false
		join:
			for j := i; j < len(arch) && j <= i+2; j++ {
				if j > i {
					if !subclassHeadingLine(arch[j].Text) {
						break
					}
					// A trailing hyphen means the heading wrapped mid-word
					// ("...SHINOBI-" / "WARE") — join without a space.
					if strings.HasSuffix(joined, "-") {
						joined += normTitle(arch[j].Text)
						name += tidyName(arch[j].Text)
					} else {
						joined += " " + normTitle(arch[j].Text)
						name += " " + tidyName(arch[j].Text)
					}
				}
				for k := next; k < len(seq) && k < next+matchWindow; k++ {
					if joined != seq[k].title {
						continue
					}
					for ; next < k; next++ {
						flag(ln.Page, "bookmark never matched in text: "+seq[next].title)
					}
					segs = append(segs, segment{
						title: name, depth: seq[k].depth, page: ln.Page,
					})
					next = k + 1
					i = j
					matched = true
					break join
				}
			}
			if matched {
				continue
			}
		}
		if len(segs) > 0 {
			segs[len(segs)-1].content = append(segs[len(segs)-1].content, ln.Text)
		}
	}
	for ; next < len(seq); next++ {
		flag(c.SourcePage, "bookmark never matched in text: "+seq[next].title)
	}

	// Rebuild the tree from the flat segments.
	type node struct {
		segment
		children []*node
	}
	var roots []*node
	var stack []*node
	for i := range segs {
		n := &node{segment: segs[i]}
		for len(stack) > 0 && stack[len(stack)-1].depth >= n.depth {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			p := stack[len(stack)-1]
			p.children = append(p.children, n)
		}
		stack = append(stack, n)
	}

	// A depth-1 node is an subclass if most of its children read as
	// level-gated features; otherwise it is an option list.
	buildFeature := func(n *node, subclassName string) ClassFeature {
		f := ClassFeature{Name: n.title, SourcePage: n.page,
			Description: stripArtistCredit(fixKnownExtractionSquish(strings.TrimSpace(strings.Join(n.content, " "))))}
		// Same bug class as buildOptionOn above (a companion/summon stat card
		// glued into a subclass feature's prose, e.g. the S.N.B Specialist's
		// own base creature glued onto "Combat Programming") — split it out
		// here too. subclass_features has no stat_block_* columns (migration
		// 0067 only covers class_features/class_options, the two tables with
		// proven instances at the time), so f.StatBlock is discarded at the
		// store layer for this table; only the cleaned Description persists.
		if sbm := SplitStatBlock(f.Description); sbm.Found {
			f.StatBlock = sbm
			f.Description = sbm.Prose
		}
		if m := ordinalLevelRe.FindStringSubmatch(f.Description); m != nil {
			lvl := atoiSafe(m[1])
			if lvl >= 1 && lvl <= 20 {
				f.Level = &lvl
			}
		} else if overrides, ok := knownFeatureLevelOverrides[subclassName]; ok {
			if lvl, ok := overrides[f.Name]; ok {
				f.Level = &lvl
			}
		}
		return f
	}
	buildOption := func(n *node) ClassOption {
		o := ClassOption{Name: n.title, SourcePage: n.page}
		desc := fixKnownExtractionSquish(strings.TrimSpace(strings.Join(n.content, " ")))
		if rest, ok := strings.CutPrefix(desc, "Prerequisite:"); ok {
			// The prerequisite runs to the end of its printed line — the
			// first content line — not the whole joined text.
			if len(n.content) > 0 {
				first := strings.TrimSpace(strings.TrimPrefix(n.content[0], "Prerequisite:"))
				o.Prerequisites = first
				desc = strings.TrimSpace(strings.TrimPrefix(rest, first))
			}
		}
		o.Description = desc
		return o
	}
	// buildOptionOn calls buildOption then runs the generic stat-block
	// splitter (statblock.go) on the result — catches a companion/summon
	// stat card glued into an option's description by the flat PDF
	// extractor, same bug class as Puppet Tool (classes.go's flushFeature).
	// For Science-Nin specifically, a match also populates the legacy
	// Class.TitanBaseText field (confirmed: only "Ronin Specialization"
	// carries the Titan base unit card, in reading order — see that field's
	// own doc comment). c is captured from ParseSubclasses' own parameter.
	buildOptionOn := func(n *node) ClassOption {
		o := buildOption(n)
		if sbm := SplitStatBlock(o.Description); sbm.Found {
			o.StatBlock = sbm
			o.Description = sbm.Prose
			if c.Name == "Science-Nin" {
				c.TitanBaseText = sbm.RawStatBlock
			}
		}
		return o
	}
	isSubclass := func(n *node) bool {
		if len(n.children) == 0 {
			return false
		}
		gated := 0
		for _, ch := range n.children {
			// "Prerequisite: 9th Level" lines are option metadata, not
			// level-gated feature prose — exclude them from the test.
			var body []string
			for _, l := range ch.content {
				if !strings.HasPrefix(l, "Prerequisite:") {
					body = append(body, l)
				}
			}
			if ordinalLevelRe.MatchString(strings.Join(body, " ")) {
				gated++
			}
		}
		return gated*2 >= len(n.children)
	}

	groupNorm := normTitle(c.GroupHeading)
	lastSubclass := ""
	for _, root := range roots {
		intro := strings.TrimSpace(strings.Join(root.content, " "))
		if normTitle(root.title) == groupNorm && c.Group == nil {
			g := &SubclassGroup{
				DisplayName: root.title, Description: intro, SourcePage: root.page,
			}
			g.SelectionLevels = selectionLevels(intro)
			for _, ch := range root.children {
				// Science-Nin's "S.N.B Upgrades" bookmark node is a
				// Minor/Refined/Greater/Superior/Supreme/Mastercraft upgrade
				// catalog for the Scientific Ninja Beast companion, not a
				// real 11th subclass of the Scientific Inquiry group — but
				// isSubclass's own heuristic (>=50% of children reading as
				// level-gated prose) misclassifies it as one, since several
				// bundled upgrade entries mention a level number in their
				// own scaling text ("this bonus increases to +2 at 9th
				// level") despite the upgrade itself having no level gate.
				// Every other subclass's own analogous upgrade catalog (e.g.
				// Shinobi-Ware's "Shinobi-Ware Upgrades") correctly lands as
				// class_options scoped to its subclass instead of a second
				// subclass row — forced into the option-list branch below so
				// it's attributed to S.N.B Specialist, the subclass printed
				// immediately before it in the bookmark outline, the same
				// "option list belongs to the subclass printed right before
				// it" rule every other subclass's catalog already uses.
				misclassifiedAsSubclass := c.Name == "Science-Nin" && ch.title == "S.N.B Upgrades"
				if isSubclass(ch) && !misclassifiedAsSubclass {
					a := Subclass{Name: ch.title, SourcePage: ch.page,
						Description: strings.TrimSpace(strings.Join(ch.content, " "))}
					for _, f := range ch.children {
						a.Features = append(a.Features, buildFeature(f, ch.title))
					}
					if c.Name == "Hunter-Nin" {
						redistributeMistaggedTables(ch.title, a.Features)
					}
					g.Subclasses = append(g.Subclasses, a)
					lastSubclass = ch.title
					continue
				}
				// An option list inside the group belongs to the subclass
				// printed right before it ("Arbiter Scout" → "Arbiter
				// Maneuvers").
				subclassName := lastSubclass
				if c.Name == "Taijutsu Specialist" && ch.title == "Martial Techniques" {
					// Martial Techniques is granted by Martial Adept, a base
					// class feature with no subclass gate — it's bookmarked
					// right after Passionate Flame's own section, which is a
					// PDF-layout artifact, not a rules fact. Confirmed
					// against the shipped rules.db: all 20 rows had
					// subclass_slug = .../passionate-flame before this fix.
					subclassName = ""
				}
				if c.Name == "Ninjutsu Specialist" && ch.title == "Efficient Molding" {
					// Efficient Molding is granted by Efficient Molding, a
					// base class feature (3rd level) with no subclass gate
					// -- same PDF-outline-bookmarking artifact as Martial
					// Techniques above, just bookmarked right after
					// Tsunami's own section instead. Confirmed against the
					// shipped rules.db: all 17 rows had subclass_slug =
					// .../tsunami before this fix.
					subclassName = ""
				}
				if c.Name == "Intelligence Operative" && ch.title == "Plans" {
					// Plans is granted by Master Planner, a base class feature
					// (2nd level) with no subclass gate -- same PDF-outline-
					// bookmarking artifact as Martial Techniques/Efficient
					// Molding above, bookmarked right after Tactical
					// Strategist's own section instead. Confirmed against the
					// shipped rules.db: all 21 rows had subclass_slug =
					// .../tactical-strategist before this fix, despite every
					// Plan's own text being subclass-agnostic ("Base Plan: ...").
					subclassName = ""
				}
				list := OptionList{Name: ch.title, SubclassName: subclassName,
					Description: strings.TrimSpace(strings.Join(ch.content, " ")),
					SourcePage:  ch.page}
				for _, o := range ch.children {
					list.Options = append(list.Options, buildOptionOn(o))
				}
				c.OptionLists = append(c.OptionLists, list)
			}
			if c.Name == "Medical-Nin" {
				redistributeMedicalNinCharts(g)
			}
			c.Group = g
			continue
		}
		// Any other section is a class-wide option list.
		list := OptionList{Name: root.title, Description: intro, SourcePage: root.page}
		for _, o := range root.children {
			opt := buildOptionOn(o)
			// Some lists nest one level deeper (sub-groups); flatten their
			// children in as options too.
			list.Options = append(list.Options, opt)
			for _, oo := range o.children {
				list.Options = append(list.Options, buildOptionOn(oo))
			}
		}
		c.OptionLists = append(c.OptionLists, list)
	}
	if c.Group == nil {
		flag(c.SourcePage, "subclass group section not matched in outline: "+c.GroupHeading)
	} else if len(c.Group.Subclasses) == 0 {
		flag(c.SourcePage, "no subclasses classified in "+c.Group.DisplayName)
	}
	// Most classes state the selection levels in the choice-granting class
	// feature ("Hunter Creed: ... grants you features at 3rd, 7th, ...")
	// rather than the section intro — fall back to it.
	if c.Group != nil && len(c.Group.SelectionLevels) == 0 {
		want := singularNorm(c.GroupHeading)
		for _, f := range c.Features {
			if singularNorm(f.Name) == want {
				c.Group.SelectionLevels = selectionLevels(f.Description)
				break
			}
		}
	}
	return anomalies
}
