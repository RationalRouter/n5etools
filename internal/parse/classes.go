// Class compendium parser (Orochimaru's Observation Compendium).
//
// Every class follows the same Word template:
//
//	<CLASS NAME>            caps heading
//	  flavor prose
//	CHARACTER INSPIRATIONS   design-intent prose
//	CREATING A <NAME>        creation guidance prose
//	QUICK BUILD              suggested build
//	CLASS FEATURES
//	HIT POINTS               "Hit Dice: 1d6 per <class> level"
//	CHAKRA POINTS            "Chakra Dice: 1d12 per <class> level"
//	PROFICIENCIES            Armor:/Weapons:/Ninja Tools:/Saving Throws:/Skills:
//	EQUIPMENT                bullet list
//	JUTSU CASTING            NINJUTSU/GENJUTSU/TAIJUTSU blocks (casting ability)
//	<FEATURE NAME>           level-gated prose features, repeated
//	<GROUP HEADING>          subclass section ("GENJUTSU PLEDGES", ...)
//
// The per-level progression table is an IMAGE in the PDF and is handled by
// the OCR tier, not this parser.
//
// A caps heading is a class start iff the next caps heading after it is
// CHARACTER INSPIRATIONS — that pairing appears nowhere else in the book.
//
// Each class names its subclass system differently (Genjutsu Pledges,
// Hunters Creeds, Cooking Focus, ...). The section heading text is fixed per
// class, but four classes reuse the exact same words for the class FEATURE
// that grants the choice (e.g. "TENETS OF MEDICINE" appears once as a
// feature, then again as the section) — so the section is the LAST exact
// occurrence of the heading within the class's page range.
//
// Stage 1 (this file): everything up to the subclass section. The subclass
// section's lines are carried on Class.SubclassLines for the subclass
// parser to consume.
package parse

import (
	"regexp"
	"strings"
)

// ClassProficiency is one row of a class's Proficiencies block.
// Kind matches class_proficiencies.kind in the schema; ChooseN is set only
// for kind "skill_choice" ("Choose three from ...").
type ClassProficiency struct {
	Kind    string
	Value   string
	ChooseN int
}

// ClassCasting says which ability powers one casting discipline.
type ClassCasting struct {
	Discipline string // "ninjutsu", "genjutsu", "taijutsu", "bukijutsu"
	Ability    string // "str", "dex", "con", "int", "wis", "cha"
}

// ClassFeature is one level-gated feature parsed from prose. Nested option
// headings inside a feature (e.g. Medical-Nin doctrine choices) become
// level-less features in reading order, like clan sidebar features.
type ClassFeature struct {
	Name        string
	Level       *int
	Description string
	SourcePage  int
}

// PuppetToolStatBlock is Puppet Master's "Puppet Tool" feature's baseline
// stat card — printed as a sidebar box in the book, interleaved into the
// PDF's flat text stream mid-paragraph (see puppetToolUnitCardRe). HPBase/
// HPConBonusAdd describe the formula "HPBase + (HPConBonusAdd + Puppet
// Master's own Con modifier) × Puppet Master's class level" — the Puppet
// MASTER's Con modifier and level, not the puppet's own ability scores.
// No AC is printed anywhere in this block.
type PuppetToolStatBlock struct {
	CreatureType        string
	ProficiencyRuleText string
	HPBase              int
	HPConBonusAdd       int
	Speed               int
	Str, Dex, Con       int
	Int, Wis, Cha       int
	PassivePerception   int
	TraitsText          string // raw "Bound. ... Hollow Shell. ... Mechanical Limits. ..." block
}

// Class is one parsed class, up to its subclass section.
type Class struct {
	Name        string
	Description string // flavor + inspirations + creation guidance
	QuickBuild  string
	SourcePage  int

	// PuppetToolStatBlock is set only for Puppet Master, once flushFeature
	// splits it out of the Puppet Tool feature's description (see
	// puppetToolUnitCardRe).
	PuppetToolStatBlock *PuppetToolStatBlock

	// TitanBaseText is set only for Science-Nin, once ParseSubclasses splits
	// it out of the Titan option list's "Ronin Specialization" entry (see
	// titanUnitCardMarker in subclasses.go) — the Titan's own shared base
	// unit card (creature type, HP/AC formula, ability scores, senses, and
	// its Battery Powered Barrier/Extra Attack/Gradual Expansion/Ninja Tool
	// Integration/Steady Improvement/Titan Specialization traits plus its
	// Bash natural weapon), printed once in the book but glued by the flat
	// PDF text extractor onto whichever specialization option it happened to
	// be crossing, same root cause as PuppetToolStatBlock. Kept as raw text,
	// not parsed into fields — nothing renders it yet (Science-Nin has no
	// companion tab), so structuring it now would be speculative; this only
	// exists to stop it corrupting Ronin Specialization's own description.
	TitanBaseText string

	HitDie           int    // 6 for "Hit Dice: 1d6"
	ChakraDie        int    // 12 for "Chakra Dice: 1d12"
	HitPointsText    string // raw block, kept verbatim for the sheet
	ChakraPointsText string

	Proficiencies []ClassProficiency
	Equipment     []string // one entry per bullet, verbatim
	Casting       []ClassCasting
	Features      []ClassFeature

	// GroupHeading is the class's subclass section heading as printed
	// ("GENJUTSU PLEDGES"); SubclassLines is everything from that heading
	// to the next class, for the stage-2 subclass parser.
	GroupHeading  string
	SubclassLines []Line

	// Filled by ParseSubclasses (stage 2).
	Group       *SubclassGroup
	OptionLists []OptionList
}

// classGroupHeadings maps each class name (as printed) to its subclass
// section heading. Fixed per book version; a new class or renamed system
// shows up as a loud "subclass section not found" anomaly.
var classGroupHeadings = map[string]string{
	"GENJUTSU SPECIALIST":    "GENJUTSU PLEDGES",
	"HUNTER-NIN":             "HUNTERS CREEDS",
	"INTELLIGENCE OPERATIVE": "MASTER STRATEGIES",
	"MEDICAL-NIN":            "TENETS OF MEDICINE",
	"NINJUTSU SPECIALIST":    "NINJUTSU FOCUS",
	"SCOUT-NIN":              "SCOUTING TECHNIQUE",
	"TAIJUTSU SPECIALIST":    "TAIJUTSU STYLE",
	"WEAPON SPECIALIST":      "WEAPON FORMS",
	"PUPPET MASTER":          "PUPPET TECHNIQUES",
	"COOKING-NIN":            "COOKING FOCUS",
	"SCIENCE-NIN":            "SCIENTIFIC INQUIRY",
}

var (
	hitDieRe    = regexp.MustCompile(`Hit Dice:\s*1d(\d+)`)
	chakraDieRe = regexp.MustCompile(`Chakra Dice:\s*1d(\d+)`)
	// "save DC = 8 + your Proficiency Bonus + your Wisdom Modifier"
	castAbilityRe = regexp.MustCompile(`save DC = .*your (Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma)`)
	// "Choose three from Chakra Control, Deception, ..."
	skillChooseRe = regexp.MustCompile(`(?i)choose (one|two|three|four|five) from (.+)$`)
)

// puppetToolUnitCardRe matches Puppet Master's "Puppet Tool" sidebar stat
// card, glued verbatim onto the end of the feature's real prose by the flat
// PDF text extractor (see PuppetToolStatBlock's doc comment). Confirmed
// against the real book text, e.g.:
//
//	Medium Construct, Proficiency = Puppet Master’s Proficiency Hit Points
//	4 + [(5+Constitution Modifier) x Puppet Master level] Speed 30 ft.
//	15 (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3) Senses Passive Perception 7
//	Bound. A Puppet is bound to its user ... Mechanical Limits. Puppets
//	cannot cast jutsu ...
//
// Fixed literal shape, not a generic stat-block detector — this is the only
// sidebar stat card in either sourcebook (see V2_ROADMAP.md's "no monster
// stat-block ingestion" note), so a small targeted regex is the right size
// of fix, same as the earlier Science-Nin/Mastercraft boundary fix.
var puppetToolUnitCardRe = regexp.MustCompile(
	`Medium Construct, Proficiency = (.+?) Hit Points (\d+) \+ \[\((\d+)\+Constitution Modifier\) x Puppet Master level\] Speed (\d+) ft\. ` +
		`(\d+) \([+-]\d+\) (\d+) \([+-]\d+\) (\d+) \([+-]\d+\) (\d+) \([+-]\d+\) (\d+) \([+-]\d+\) (\d+) \([+-]\d+\) ` +
		`Senses Passive Perception (\d+) (.+)$`)

// artistCreditRe strips a stray "Artist Credit: <name> on <site>" image
// caption the flat PDF text extractor sometimes glues onto the end of a
// feature's real prose — confirmed on Weapon Specialist's Superior Weapon
// Flurry ("... Artist Credit: KSatoshiK on DeviantArt"). The caption always
// belongs to a nearby illustration, never the feature's own rules text, and
// always trails whatever block it lands in, so a trailing-anchor strip is
// enough — no per-instance customization needed, unlike puppetToolUnitCardRe
// above.
var artistCreditRe = regexp.MustCompile(`\s*Artist Credit:.*$`)

func stripArtistCredit(s string) string {
	return strings.TrimSpace(artistCreditRe.ReplaceAllString(s, ""))
}

// levelSuffixRe matches a gating level accidentally glued onto the end of a
// feature's own printed NAME instead of surfacing as separate level data —
// a PDF-extraction artifact confirmed on Scout-Nin's Deft Explorer sub-tiers,
// whose raw names read "CANNY (1 ST LEVEL)", "MOBILE (6TH LEVEL)", "TIRELESS
// (11 TH LEVEL)" (an inconsistent stray space sometimes splits the digit from
// its ordinal suffix, the same "extra space" artifact class documented
// elsewhere in this package, e.g. quickBuildSquishes). tidyName's own casing
// pass then produces "Canny (1 St Level)" / "Mobile (6Th Level)" / "Tireless
// (11 Th Level)" — level and level_override both NULL, since nothing ever
// reads a level out of a feature's NAME. Matched case-insensitively against
// the already-tidied name so it catches the suffix regardless of exactly
// where tidyName's casing pass lands.
//
// stripLevelSuffix (below) must run in flushFeature, on the fully assembled
// Name, not at the moment a heading first creates curFeature: confirmed
// against the real book text that Canny's and Tireless's own headings are
// split across two PDF lines by a column wrap ("CANNY (1" / "ST LEVEL)"),
// so the parenthetical isn't complete until the wrapped-heading-join step
// a few lines below appends the second line. Mobile's own heading happens
// to land on one line, so it worked even with an earlier version of this
// fix that only checked at creation time — that gave false confidence the
// fix was complete; only running it against the REAL sourcebook PDF (not
// just a same-shaped hand-written fixture) surfaced the two-line case.
var levelSuffixRe = regexp.MustCompile(`(?i)\s*\(\s*(\d{1,2})\s*(?:st|nd|rd|th)\s*Level\s*\)\s*$`)

// stripLevelSuffix removes a trailing "(<n>th Level)" parenthetical from a
// feature's fully-assembled name and returns the gating level it encoded.
// Returns the name unchanged and ok=false when no such suffix is present, or
// the parsed number falls outside a plausible character level (1-20).
func stripLevelSuffix(name string) (clean string, level int, ok bool) {
	m := levelSuffixRe.FindStringSubmatchIndex(name)
	if m == nil {
		return name, 0, false
	}
	lvl := atoiSafe(name[m[2]:m[3]])
	if lvl < 1 || lvl > 20 {
		return name, 0, false
	}
	return strings.TrimSpace(name[:m[0]]), lvl, true
}

// knownClassFeatureLevelOverrides hand-confirms a small, closed list of
// class (not subclass) features whose printed text never states the
// feature's own gating level — the class_features counterpart of
// subclasses.go's knownFeatureLevelOverrides, but this one WINS OVER a
// regex match rather than only filling in when the regex finds none (see
// flushFeature). Weapon Specialist's Weapon Flurry (2nd level) introduces
// "the following Flurry Techniques" as a named list: Enhanced Deflection,
// Chained Reaction [New], Chakra Strike, Perceptive Augmentation, and
// Focused Efficiency — confirmed by their shared sort_order block sitting
// directly between Weapon Flurry (sort_order 2) and Weapon Stance
// (sort_order 8), and by Superior Weapon Flurry's own text naming two of
// them by name ("Your Enhanced Deflection flurry technique...", "Your
// Perceptive Augmentation..."). Four of the five never state a level
// anywhere in their printed text, so ordinalLevelRe found nothing and they
// parsed as always-on (level NULL) instead of gated to 2nd level like their
// parent. Chakra Strike doesn't state its own gating level either — its
// only ordinal is an internal damage-escalation clause ("...+2 Flurry Die.
// This increases to +3 Flurry Die at 11th level."), which ordinalLevelRe
// matched instead, parsing it as an 11th-level feature. A fallback-only
// override (like subclasses.go's) can't fix that case, since the regex did
// find a match — just the wrong one — so this map takes precedence over
// any regex match for these five names.
var knownClassFeatureLevelOverrides = map[string]map[string]int{
	"Weapon Specialist": {
		"Enhanced Deflection":     2,
		"Chained Reaction [New]":  2,
		"Chakra Strike":           2,
		"Perceptive Augmentation": 2,
		"Focused Efficiency":      2,
	},
	// Rejuvenating Rest's own text opens "Also, at Level 1 you use your
	// medical skills to revitalize wounded allies during a short rest...
	// This amount of extra healing increases to 2d6 at 7th level, 3d6 at
	// 11th, and 4d6 at 17th Level." The feature itself is 1st level; its
	// only ordinals are the escalation tiers, and ordinalLevelRe matched
	// the first one it found ("7th level") instead, same failure shape as
	// Weapon Specialist's Chakra Strike above.
	"Medical-Nin": {
		"Rejuvenating Rest": 1,
	},
	// Shinobi Adept (2nd level) presents its 10-option catalog as separate
	// named class_features with no level text anywhere in their own printed
	// prose — the catalog's only level is on the introducing feature itself
	// ("You can choose between two of the following features..."). Nothing
	// for ordinalLevelRe to find, so all 10 parsed as always-on (level NULL)
	// and were blanket-granted from 1st level regardless of picker state.
	// Tag each option to Shinobi Adept's own 2nd level; a later cap+catalog
	// picker (2 known at 2nd -> 4 at 13th) narrows this further on top.
	// Signature Jutsu (7th level) has the identical shape for its own
	// 3-option effect catalog. Combat/Control/Mobility/Skill/Support are
	// Jack of All, Master of None's 5 generalizations (5th level unlock) —
	// each one's own prose states only a LATER numeric tier bump ("...
	// increases to +2 at 11th level"), which ordinalLevelRe matches instead
	// of the feature's real 5th-level unlock, the same failure shape as
	// Weapon Specialist's Chakra Strike above; the override wins over that
	// wrong match.
	// Map keys use the curly apostrophe (’) the book's own text stream
	// prints, not a straight one — tidyName never normalizes it (see
	// subclasses.go's normTitle, which folds curly to straight only for its
	// own bookmark-matching use, not for stored feature names), so a
	// straight-apostrophe key here would silently never match.
	"Scout-Nin": {
		"Shinobi’s Tactics":          2,
		"Shinobi’s General Literacy": 2,
		"Shinobi’s Tool Competency":  2,
		"Shinobi’s Precision":        2,
		"Shinobi’s Edge":             2,
		"Shinobi’s Drive":            2,
		"Shinobi’s Focus":            2,
		"Hidden Technique":           2,
		"Aggressive Technique":       2,
		"Tactical Technique":         2,
		"Signature Power":            7,
		"Signature Ramping":          7,
		"Signature Control":          7,
		"Combat":                     5,
		"Control":                    5,
		"Mobility":                   5,
		"Skill":                      5,
		"Support":                    5,
	},
}

// quickBuildSquishes hand-confirms a PDF text-extraction artifact specific
// to QUICK BUILD's clan-choice sentence: "Non-Clan" gets split into
// "Non- Clan" by the same stray-space-after-hyphen bug internal/correct's
// general hyphenspace heuristic fixes elsewhere, but QuickBuild is
// deliberately excluded from that package's Sweep (see internal/correct/
// registry.go's own doc comment — it's short, semi-structured build
// shorthand, not free prose a grammar tool should touch), so the fix lives
// here instead, narrowly scoped to the one confirmed phrase, same
// discipline as subclasses.go's own knownExtractionSquishes.
var quickBuildSquishes = strings.NewReplacer(
	"Non- Clan, Clans", "Non-Clan, Clans",
)

func fixQuickBuildSquish(s string) string {
	return quickBuildSquishes.Replace(s)
}

var abilityShort = map[string]string{
	"Strength": "str", "Dexterity": "dex", "Constitution": "con",
	"Intelligence": "int", "Wisdom": "wis", "Charisma": "cha",
}

var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
}

var profKinds = map[string]string{
	"Armor":         "armor",
	"Weapons":       "weapon",
	"Ninja Tools":   "tool",
	"Saving Throws": "saving_throw",
	"Skills":        "skill",
}

// ParseClassBook parses the whole class compendium line stream (everything
// before LEGACY CONTENT). Stage 1: per-class core blocks; subclass sections
// are carried raw on each Class.
func ParseClassBook(lines []Line) ([]Class, []Anomaly) {
	var anomalies []Anomaly

	// Standard line filter, and stop at the legacy section or the closing
	// Archetype/Caster/Martial Class Feats chapter. That chapter follows the
	// last class with no class-name heading of its own, so leaving it in
	// glues its ~1700 lines onto whichever class happens to be last (see
	// classfeats.go's classFeatSections, the same boundary reused here).
	var ls []Line
	for _, ln := range lines {
		if ln.Text == "LEGACY CONTENT" || classFeatSections[ln.Text] {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}

	// A class starts where a caps heading's NEXT caps heading is
	// CHARACTER INSPIRATIONS.
	var starts []int
	lastHeading := -1
	for i, ln := range ls {
		if !capsLineRe.MatchString(ln.Text) {
			continue
		}
		if ln.Text == "CHARACTER INSPIRATIONS" && lastHeading >= 0 {
			starts = append(starts, lastHeading)
		}
		lastHeading = i
	}

	var classes []Class
	for k, start := range starts {
		end := len(ls)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		c, anoms := parseClass(ls[start:end])
		classes = append(classes, c)
		anomalies = append(anomalies, anoms...)
	}
	return classes, anomalies
}

// parseClass parses one class's line range (class heading up to the next
// class or end of content).
func parseClass(ls []Line) (Class, []Anomaly) {
	var anomalies []Anomaly
	c := Class{Name: tidyName(ls[0].Text), SourcePage: ls[0].Page}
	flag := func(page int, problem string) {
		anomalies = append(anomalies, Anomaly{Page: page, Subject: c.Name, Problem: problem})
	}

	// Split off the subclass section: last exact occurrence of the class's
	// group heading (see package comment for why LAST).
	groupHeading, known := classGroupHeadings[ls[0].Text]
	body := ls
	if known {
		sectionIdx := -1
		for i := 1; i < len(ls); i++ {
			if ls[i].Text == groupHeading {
				sectionIdx = i
			}
		}
		if sectionIdx < 0 {
			flag(ls[0].Page, "subclass section not found: "+groupHeading)
		} else {
			c.GroupHeading = groupHeading
			c.SubclassLines = ls[sectionIdx:]
			body = ls[:sectionIdx]
		}
	} else {
		flag(ls[0].Page, "class not in classGroupHeadings map — subclasses not split")
	}

	// Mode machine over the body. Each case name mirrors the book heading.
	const (
		modeFlavor      = iota
		modeDescription // CHARACTER INSPIRATIONS + CREATING A <NAME>
		modeQuickBuild
		modeHitPoints
		modeChakraPoints
		modeProficiencies
		modeEquipment
		modeCasting
		modeFeatures
	)
	mode := modeFlavor
	var desc, quick, hpText, cpText []string
	var profLabel, profValue string
	var profs []ClassProficiency
	castDiscipline := ""
	var castText []string // the DC lines wrap mid-sentence; match on the join
	var curFeature *ClassFeature

	flushCasting := func() {
		if castDiscipline == "" {
			return
		}
		if m := castAbilityRe.FindStringSubmatch(strings.Join(castText, " ")); m != nil {
			c.Casting = append(c.Casting, ClassCasting{
				Discipline: castDiscipline, Ability: abilityShort[m[1]],
			})
		}
		castDiscipline, castText = "", nil
	}

	flushProf := func() {
		if profLabel != "" {
			profs = append(profs, splitProficiency(profLabel, profValue)...)
		}
		profLabel, profValue = "", ""
	}
	flushFeature := func() {
		if curFeature == nil {
			return
		}
		curFeature.Description = stripArtistCredit(fixKnownExtractionSquish(strings.TrimSpace(curFeature.Description)))
		if curFeature.Description == "" {
			// Decorative box title with no body — drop it.
			curFeature = nil
			return
		}
		if c.Name == "Puppet Master" && curFeature.Name == "Puppet Tool" {
			if loc := puppetToolUnitCardRe.FindStringSubmatchIndex(curFeature.Description); loc != nil {
				m := make([]string, len(loc)/2)
				for i := range m {
					if loc[2*i] >= 0 {
						m[i] = curFeature.Description[loc[2*i]:loc[2*i+1]]
					}
				}
				c.PuppetToolStatBlock = &PuppetToolStatBlock{
					CreatureType:        "Medium Construct",
					ProficiencyRuleText: m[1],
					HPBase:              atoiSafe(m[2]),
					HPConBonusAdd:       atoiSafe(m[3]),
					Speed:               atoiSafe(m[4]),
					Str:                 atoiSafe(m[5]),
					Dex:                 atoiSafe(m[6]),
					Con:                 atoiSafe(m[7]),
					Int:                 atoiSafe(m[8]),
					Wis:                 atoiSafe(m[9]),
					Cha:                 atoiSafe(m[10]),
					PassivePerception:   atoiSafe(m[11]),
					TraitsText:          strings.TrimSpace(m[12]),
				}
				curFeature.Description = strings.TrimSpace(curFeature.Description[:loc[0]])
			}
		}
		if clean, lvl, ok := stripLevelSuffix(curFeature.Name); ok {
			curFeature.Name = clean
			if curFeature.Level == nil {
				lvl := lvl
				curFeature.Level = &lvl
			}
		}
		if overrides, ok := knownClassFeatureLevelOverrides[c.Name]; ok {
			if lvl, ok := overrides[curFeature.Name]; ok {
				curFeature.Level = &lvl
			}
		}
		if curFeature.Level == nil {
			if m := ordinalLevelRe.FindStringSubmatch(curFeature.Description); m != nil {
				lvl := 0
				for _, ch := range m[1] {
					lvl = lvl*10 + int(ch-'0')
				}
				if lvl >= 1 && lvl <= 20 {
					curFeature.Level = &lvl
				}
			}
		}
		c.Features = append(c.Features, *curFeature)
		curFeature = nil
	}

	for i := 1; i < len(body); i++ {
		text := body[i].Text
		if capsLineRe.MatchString(text) {
			switch {
			case text == "CHARACTER INSPIRATIONS", strings.HasPrefix(text, "CREATING A"):
				mode = modeDescription
				continue
			case text == "QUICK BUILD":
				mode = modeQuickBuild
				continue
			case text == "CLASS FEATURES":
				continue // intro sentence only; HIT POINTS follows
			case text == "HIT POINTS":
				mode = modeHitPoints
				continue
			case text == "CHAKRA POINTS":
				mode = modeChakraPoints
				continue
			case text == "PROFICIENCIES":
				mode = modeProficiencies
				continue
			case text == "EQUIPMENT":
				flushProf()
				mode = modeEquipment
				continue
			case text == "JUTSU CASTING":
				mode = modeCasting
				continue
			}
			if mode == modeCasting {
				if d := disciplineHeading(text); d != "" {
					flushCasting()
					castDiscipline = d
					continue
				}
				flushCasting()
				mode = modeFeatures // first non-discipline heading = features
			}
			if mode == modeFeatures {
				// A heading directly followed by another caps line is a
				// wrapped heading ("ABILITY SCORE" / "IMPROVEMENT/FEAT") —
				// join them rather than creating an empty feature.
				if curFeature != nil && curFeature.Description == "" {
					curFeature.Name += " " + tidyName(text)
					continue
				}
				flushFeature()
				curFeature = &ClassFeature{Name: tidyName(text), SourcePage: body[i].Page}
				continue
			}
			// Caps headings inside flavor/description (sub-headings like
			// "ON THE COVER") are kept as text — harmless in prose.
		}

		switch mode {
		case modeFlavor, modeDescription:
			desc = append(desc, text)
		case modeQuickBuild:
			quick = append(quick, text)
		case modeHitPoints:
			hpText = append(hpText, text)
		case modeChakraPoints:
			cpText = append(cpText, text)
		case modeProficiencies:
			if label, value, ok := splitProfLabel(text); ok {
				flushProf()
				profLabel, profValue = label, value
			} else if profLabel != "" {
				profValue += " " + text
			}
		case modeEquipment:
			if strings.HasPrefix(text, "•") {
				c.Equipment = append(c.Equipment, strings.TrimSpace(strings.TrimPrefix(text, "•")))
			} else if n := len(c.Equipment); n > 0 {
				c.Equipment[n-1] += " " + text
			}
			// Non-bullet text before the first bullet is the standard
			// "You start with the following equipment" intro — dropped.
		case modeCasting:
			castText = append(castText, text)
		case modeFeatures:
			if curFeature != nil {
				curFeature.Description += " " + text
			}
		}
	}
	flushProf()
	flushCasting()
	flushFeature()

	c.Description = strings.Join(desc, " ")
	c.QuickBuild = fixQuickBuildSquish(strings.Join(quick, " "))
	c.HitPointsText = strings.Join(hpText, " ")
	c.ChakraPointsText = strings.Join(cpText, " ")
	c.Proficiencies = profs

	if m := hitDieRe.FindStringSubmatch(c.HitPointsText); m != nil {
		c.HitDie = atoiSafe(m[1])
	} else {
		flag(c.SourcePage, "hit die not found in HIT POINTS block")
	}
	if m := chakraDieRe.FindStringSubmatch(c.ChakraPointsText); m != nil {
		c.ChakraDie = atoiSafe(m[1])
	} else {
		flag(c.SourcePage, "chakra die not found in CHAKRA POINTS block")
	}
	if len(c.Casting) == 0 {
		flag(c.SourcePage, "no casting abilities parsed from JUTSU CASTING")
	}
	if len(c.Features) == 0 {
		flag(c.SourcePage, "no class features parsed")
	}
	return c, anomalies
}

// disciplineHeading maps a JUTSU CASTING sub-heading to its discipline name,
// or "" if the heading is not a discipline.
func disciplineHeading(text string) string {
	switch text {
	case "NINJUTSU":
		return "ninjutsu"
	case "GENJUTSU":
		return "genjutsu"
	case "TAIJUTSU":
		return "taijutsu"
	case "BUKIJUTSU":
		return "bukijutsu"
	}
	return ""
}

// splitProfLabel recognizes "Armor: Light armor" proficiency lines. The
// label match is case-insensitive: Cooking-Nin's book text prints "weapons:"
// and "saving throws:" in lowercase (unlike every other class), and a
// case-sensitive match silently swallowed those lines as continuation text
// of the preceding field instead of recognizing them as new ones.
func splitProfLabel(text string) (label, value string, ok bool) {
	for l := range profKinds {
		if len(text) > len(l) && text[len(l)] == ':' && strings.EqualFold(text[:len(l)], l) {
			return l, strings.TrimSpace(text[len(l)+1:]), true
		}
	}
	return "", "", false
}

// splitProficiency turns one labeled block into schema rows. Skills split
// into fixed proficiencies and a "Choose N from ..." option list; everything
// else is a comma-separated list of values.
func splitProficiency(label, raw string) []ClassProficiency {
	kind := profKinds[label]
	raw = strings.TrimSpace(raw)
	var rows []ClassProficiency

	if kind == "skill" {
		if m := skillChooseRe.FindStringSubmatch(raw); m != nil {
			fixed := strings.TrimSuffix(strings.TrimSpace(raw[:len(raw)-len(m[0])]), ",")
			for _, s := range splitCommaList(fixed) {
				rows = append(rows, ClassProficiency{Kind: "skill", Value: s})
			}
			n := numberWords[strings.ToLower(m[1])]
			for _, s := range splitCommaList(m[2]) {
				rows = append(rows, ClassProficiency{Kind: "skill_choice", Value: s, ChooseN: n})
			}
			return rows
		}
	}
	for _, s := range splitCommaList(raw) {
		rows = append(rows, ClassProficiency{Kind: kind, Value: s})
	}
	return rows
}

// splitCommaList splits "A, B, and C." into trimmed entries, dropping
// noise like "None".
func splitCommaList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSuffix(strings.TrimSpace(part), ".")
		part = strings.TrimSpace(strings.TrimPrefix(part, "and "))
		if part == "" || strings.EqualFold(part, "none") {
			continue
		}
		out = append(out, part)
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, ch := range s {
		n = n*10 + int(ch-'0')
	}
	return n
}
