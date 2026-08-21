// Clan compendium (Tsunade's Studies) parser.
//
// Every clan follows one template, in this order:
//
//	<NAME> CLAN            — flavor fiction (skipped), then general prose
//	<EPITHET>              — e.g. "CREEPY CRAWLY"; overview prose follows
//	<NAME> TRAITS          — labeled trait lines (ability increase, speed,
//	                          skills, languages, plus clan-specific grants)
//	<NAME> FEATURES        — "Name: prose" features, level-gated by ordinals
//	<NAME> CLAN JUTSU      — rank headers + standard jutsu entries
//	                          (same shape as the jutsu compendium; some clans
//	                          have no jutsu section)
//	CLAN FEATS             — feat entries: NAME / Category: / Prerequisite:
//
// Section headings are matched by SUFFIX, not by clan-name prefix, because
// the book is not perfectly consistent ("SYNTHETIC HUMAN CLAN" but
// "SYNTHETIC FEATURES"). The clan list ends at "BLOODLINE LATENTS", which a
// separate parser handles.
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// Clan is one parsed clan section.
type Clan struct {
	Name       string // "Aburame Clan" (title-cased from the heading)
	Epithet    string // "Creepy Crawly"
	Overview   string // prose under the epithet
	SourcePage int

	// From the TRAITS block.
	AbilityRaw       string            // "+2 Intelligence, +1 Wisdom" as printed
	AbilityIncreases []AbilityIncrease // parsed form; empty if unparseable
	SpeedText        string            // "Your base walking speed is 30 feet"
	SpeedFeet        *int              // parsed from SpeedText
	SkillProfs       []string
	ExtraLanguage    string
	Traits           []ClanTrait // clan-specific named grants

	Features []ClanFeature
	Jutsu    []Jutsu // standard jutsu entries from the clan's jutsu section
	Feats    []Feat
}

// AbilityIncrease is one "+2 Intelligence" style grant.
type AbilityIncrease struct {
	Ability string // 'str','dex','con','int','wis','cha'
	Amount  int
}

// ClanTrait is a named always-on grant from the TRAITS block that is not one
// of the standard labels ("Parasitic Technique", "Lightning Literacy", ...).
type ClanTrait struct {
	Name        string
	Description string
}

// ClanFeature is one named feature paragraph. Level is the level the feature
// is FIRST gained (first ordinal in the text); nil means always on.
type ClanFeature struct {
	Name        string
	Level       *int
	Description string
	SourcePage  int
}

// Feat is one feat entry (clan feats here; core-book feats reuse it).
type Feat struct {
	Name          string
	Category      string // "Clan", "Archetype", ...
	Prerequisites string // "Aburame Clan, Level 8+" as printed
	ClassName     string // class feats print "Archetype: Puppet Master"
	Description   string
	SourcePage    int
}

var (
	// Ordinal level gate: "Beginning at 1st level", "at 3rd Level".
	ordinalLevelRe = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)[ \n]+[Ll]evel`)

	// The ability-increase label. The book's template doubles the word
	// ("Recommended Recommended Ability Score Increase:") on most clans.
	abilityLabelRe = regexp.MustCompile(`Recommended\s+(?:Recommended\s+)?Ability\s+Score\s+Increase\s*:`)

	// A "Label: value" line opener in a TRAITS block: 1–5 words starting
	// with a capital, anchored at line start so a value's own colons
	// ("(D-Rank: 1, …)") can never be read as labels. \pL not \w because
	// labels carry accents ("Hyūga Clan Jutsu:") and markers ("[New!]").
	traitLabelRe = regexp.MustCompile(`^(\p{Lu}[\pL\pN’'\-]*(?:\s+[\pL\pN’'\-\[\]!]+){0,4})\s*:`)

	// A feature opens a paragraph: "Bug Host: Beginning at 1st level..."
	featureStartRe = regexp.MustCompile(`^(\p{Lu}[\pL\pN’'\-]*(?:\s+[\pL\pN’'\-()\[\]!]+){0,5})\s*:\s+(\S.*)$`)

	speedFeetRe = regexp.MustCompile(`(\d+)\s*feet`)

	// Stray lines that are only punctuation (layout artifacts).
	punctOnlyRe = regexp.MustCompile(`^[\s.,;:•\-–—]+$`)

	// "+2 Intelligence", "+1 Cha", "-1 Strength"
	abilityAmountRe = regexp.MustCompile(`([+-]\d+)\s+([A-Za-z]+)`)
)

// labelBlacklist rejects prose fragments that look like labels but never
// are: rank tables ("D-Rank: 1"), lone stat abbreviations.
func labelIsBlacklisted(label string) bool {
	switch label {
	case "Rank", "DC", "SHB", "X", "Note":
		return true
	}
	return strings.HasSuffix(label, "-Rank") || strings.HasSuffix(label, " Rank")
}

var abilityNames = map[string]string{
	"strength": "str", "str": "str",
	"dexterity": "dex", "dex": "dex",
	"constitution": "con", "con": "con",
	"intelligence": "int", "int": "int",
	"wisdom": "wis", "wis": "wis",
	"charisma": "cha", "cha": "cha",
}

// ParseClanBook parses the clan compendium's clan sections (everything
// before BLOODLINE LATENTS).
func ParseClanBook(lines []Line) ([]Clan, []Anomaly) {
	var (
		clans     []Clan
		anomalies []Anomaly
	)

	// Drop page-footer numbers and punctuation-only stray lines (they
	// interrupt wrapped lines), and art-credit blocks ("ART CREDIT" heading
	// plus its one credit line).
	var ls []Line
	for idx := 0; idx < len(lines); idx++ {
		ln := lines[idx]
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		if ln.Text == "ART CREDIT" {
			idx++ // skip the credit line under the heading too
			continue
		}
		ls = append(ls, ln)
	}

	const (
		modeNone   = iota
		modeFlavor // after the clan heading, before the epithet
		modeOverview
		modeTraits
		modeFeatures
		modeJutsu
		modeFeats
	)

	var (
		cur         *Clan
		mode        = modeNone
		overview    strings.Builder
		traitLines  []string
		rankSection string
	)

	finishClan := func() {
		if cur == nil {
			return
		}
		cur.Overview = strings.TrimSpace(overview.String())
		overview.Reset()
		parseTraits(cur, traitLines, &anomalies)
		traitLines = nil
		clans = append(clans, *cur)
		cur = nil
	}

	i := 0
	for i < len(ls) {
		text := ls[i].Text

		if text == "BLOODLINE LATENTS" {
			break
		}

		if capsLineRe.MatchString(text) {
			// A caps line that starts a jutsu entry is never a section
			// heading, even when its name ends in a heading-like suffix.
			entryStart := startsEntry(ls, i)
			switch {
			// "NON-CLAN" is the clanless option, laid out exactly like a
			// clan ("NON-CLAN TRAITS", "NON-CLAN FEATURES", "CLAN FEATS").
			case !entryStart && (strings.HasSuffix(text, " CLAN") || text == "NON-CLAN"):
				finishClan()
				cur = &Clan{Name: tidyName(text), SourcePage: ls[i].Page}
				mode = modeFlavor
				rankSection = ""
				i++
				continue
			case cur != nil && !entryStart && strings.HasSuffix(text, " TRAITS"):
				mode = modeTraits
				i++
				continue
			case cur != nil && !entryStart && strings.HasSuffix(text, " FEATURES"):
				mode = modeFeatures
				i++
				continue
			// "SYNTHETIC HUMAN JUTSU" is the one clan-jutsu heading printed
			// without the word CLAN.
			case cur != nil && !entryStart &&
				(strings.HasSuffix(text, " CLAN JUTSU") || text == "SYNTHETIC HUMAN JUTSU"):
				mode = modeJutsu
				i++
				continue
			case cur != nil && text == "CLAN FEATS":
				mode = modeFeats
				i++
				continue
			}

			if m := rankHeaderRe.FindStringSubmatch(text); m != nil {
				rankSection = m[1]
				i++
				continue
			}

			// An unclaimed caps line right after the clan heading is the
			// epithet ("CREEPY CRAWLY") — unless it starts a jutsu/feat
			// entry, which the mode-specific code below claims.
			if cur != nil && mode == modeFlavor && !startsEntry(ls, i) && !startsFeat(ls, i) {
				cur.Epithet = tidyName(text)
				mode = modeOverview
				i++
				continue
			}
		}

		switch mode {
		case modeOverview:
			overview.WriteString(text)
			overview.WriteString(" ")
			i++

		case modeTraits:
			traitLines = append(traitLines, text)
			i++

		case modeFeatures:
			// A caps sub-heading inside a features section is a sidebar
			// ("DATA CHANNELS", "SUPPORTIVE VS. OFFENSIVE MEDICAL JUTSU").
			// Captured as a level-less feature so its text isn't lost.
			if capsLineRe.MatchString(text) {
				sidebar := ClanFeature{Name: tidyName(text), SourcePage: ls[i].Page}
				var b strings.Builder
				i++
				for i < len(ls) && !capsLineRe.MatchString(ls[i].Text) {
					if fm := featureStartRe.FindStringSubmatch(ls[i].Text); fm != nil &&
						!labelIsBlacklisted(fm[1]) {
						break
					}
					b.WriteString(ls[i].Text)
					b.WriteString(" ")
					i++
				}
				sidebar.Description = strings.TrimSpace(b.String())
				// A caps line with no body is a decorative box title (the
				// real feature parsed from its labeled paragraph) — drop it.
				if sidebar.Description != "" {
					cur.Features = append(cur.Features, sidebar)
				}
				continue
			}
			next, feature, ok := parseClanFeature(ls, i)
			if !ok {
				// Prose before the first feature label (rare) — or a
				// wrapped fragment; either way it can't be attached.
				anomalies = append(anomalies, Anomaly{
					Page: ls[i].Page, Subject: cur.Name,
					Problem: "features text outside any feature: " + snippet(text),
				})
				i++
				continue
			}
			cur.Features = append(cur.Features, feature)
			i = next

		case modeJutsu:
			if startsEntry(ls, i) {
				j := i
				nameLines := []string{ls[i].Text}
				for j+1 < len(ls) && capsLineRe.MatchString(ls[j+1].Text) {
					j++
					nameLines = append(nameLines, ls[j].Text)
				}
				entry, next, ans := parseJutsuEntry(ls, i, j+1, strings.Join(nameLines, " "))
				entry.CategoryGroup = "Clan: " + strings.TrimSuffix(cur.Name, " Clan")
				if rankSection != "" && entry.Rank != "" && entry.Rank != rankSection {
					ans = append(ans, Anomaly{
						Page: entry.SourcePage, Subject: entry.Name,
						Problem: "rank " + entry.Rank + " does not match the running " +
							rankSection + "-Rank section header",
					})
				}
				cur.Jutsu = append(cur.Jutsu, entry)
				anomalies = append(anomalies, ans...)
				i = next
				continue
			}
			anomalies = append(anomalies, Anomaly{
				Page: ls[i].Page, Subject: cur.Name,
				Problem: "unexpected text in clan jutsu section: " + snippet(text),
			})
			i++

		case modeFeats:
			if startsFeat(ls, i) {
				feat, next := parseFeat(ls, i)
				cur.Feats = append(cur.Feats, feat)
				i = next
				continue
			}
			// The book occasionally prints a leftover clan jutsu after the
			// CLAN FEATS heading (Vesper's Tool of Night). Keep it a jutsu.
			if startsEntry(ls, i) {
				j := i
				nameLines := []string{ls[i].Text}
				for j+1 < len(ls) && capsLineRe.MatchString(ls[j+1].Text) {
					j++
					nameLines = append(nameLines, ls[j].Text)
				}
				entry, next, ans := parseJutsuEntry(ls, i, j+1, strings.Join(nameLines, " "))
				entry.CategoryGroup = "Clan: " + strings.TrimSuffix(cur.Name, " Clan")
				cur.Jutsu = append(cur.Jutsu, entry)
				anomalies = append(anomalies, ans...)
				i = next
				continue
			}
			anomalies = append(anomalies, Anomaly{
				Page: ls[i].Page, Subject: cur.Name,
				Problem: "unexpected text in clan feats section: " + snippet(text),
			})
			i++

		default: // modeNone, modeFlavor: flavor fiction and front matter
			i++
		}
	}
	finishClan()
	return clans, anomalies
}

// parseClanFeature reads one "Name: prose" feature starting at i. Returns
// ok=false if the line at i doesn't open a feature.
func parseClanFeature(ls []Line, i int) (next int, f ClanFeature, ok bool) {
	m := featureStartRe.FindStringSubmatch(ls[i].Text)
	if m == nil || labelIsBlacklisted(m[1]) {
		return i, f, false
	}
	f.Name = m[1]
	f.SourcePage = ls[i].Page

	var b strings.Builder
	b.WriteString(m[2])
	i++
	for i < len(ls) {
		text := ls[i].Text
		// The feature ends at the next feature label or any section heading.
		if capsLineRe.MatchString(text) {
			break
		}
		if fm := featureStartRe.FindStringSubmatch(text); fm != nil &&
			!labelIsBlacklisted(fm[1]) && !strings.HasPrefix(text, "•") {
			break
		}
		b.WriteString(" ")
		b.WriteString(text)
		i++
	}
	f.Description = strings.TrimSpace(b.String())
	if m := ordinalLevelRe.FindStringSubmatch(f.Description); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			f.Level = &n
		}
	}
	return i, f, true
}

// startsFeat reports whether the caps run at i is a feat name — i.e. it is
// followed by a "Category:" line.
func startsFeat(ls []Line, i int) bool {
	if !capsLineRe.MatchString(ls[i].Text) {
		return false
	}
	j := i + 1
	for j < len(ls) && capsLineRe.MatchString(ls[j].Text) {
		j++
	}
	return j < len(ls) && strings.HasPrefix(ls[j].Text, "Category")
}

// parseFeat reads one feat entry: NAME / Category: / Prerequisite: /
// description prose until the next feat or heading.
func parseFeat(ls []Line, i int) (Feat, int) {
	f := Feat{SourcePage: ls[i].Page}
	nameLines := []string{ls[i].Text}
	i++
	for i < len(ls) && capsLineRe.MatchString(ls[i].Text) {
		nameLines = append(nameLines, ls[i].Text)
		i++
	}
	f.Name = tidyName(strings.Join(nameLines, " "))

	var desc strings.Builder
	for i < len(ls) {
		text := ls[i].Text
		if capsLineRe.MatchString(text) {
			break // next feat name or section heading
		}
		switch {
		case strings.HasPrefix(text, "Category:"):
			f.Category = strings.TrimSpace(strings.TrimPrefix(text, "Category:"))
		case strings.HasPrefix(text, "Prerequisite:"):
			f.Prerequisites = strings.TrimSpace(strings.TrimPrefix(text, "Prerequisite:"))
		case strings.HasPrefix(text, "Prerequisites:"):
			f.Prerequisites = strings.TrimSpace(strings.TrimPrefix(text, "Prerequisites:"))
		case strings.HasPrefix(text, "Archetype:"):
			f.ClassName = strings.TrimSpace(strings.TrimPrefix(text, "Archetype:"))
		case desc.Len() == 0 && f.Prerequisites != "" && startsLower(text):
			// A prerequisite list that wrapped to the next printed line
			// ("…, You cannot have" / "class levels in Science-Nin").
			f.Prerequisites += " " + text
		default:
			desc.WriteString(text)
			desc.WriteString(" ")
		}
		i++
	}
	f.Description = stripArtistCredit(strings.TrimSpace(desc.String()))
	return f, i
}

// parseTraits interprets a clan's TRAITS block. Each trait starts on its own
// line as "Label: value"; values wrap onto following lines. The one label
// that itself wraps is the long ability-increase one ("Recommended
// Recommended Ability Score / Increase: / +2 Intelligence, +1 Wisdom"), so
// lines starting with "Recommended" are merged until the colon appears.
func parseTraits(c *Clan, lines []string, anomalies *[]Anomaly) {
	if len(lines) == 0 {
		*anomalies = append(*anomalies, Anomaly{
			Page: c.SourcePage, Subject: c.Name, Problem: "no TRAITS block found",
		})
		return
	}

	type entry struct{ label, value string }
	var entries []entry

	i := 0
	for i < len(lines) {
		line := lines[i]

		// The ability-increase label, possibly wrapped across lines.
		if strings.HasPrefix(line, "Recommended") {
			joined := line
			for !strings.Contains(joined, ":") && i+1 < len(lines) {
				i++
				joined += " " + lines[i]
			}
			_, value, _ := strings.Cut(joined, ":")
			entries = append(entries, entry{"__ability__", strings.TrimSpace(value)})
			i++
			continue
		}

		if m := traitLabelRe.FindStringSubmatch(line); m != nil && !labelIsBlacklisted(m[1]) {
			label := m[1]
			if strings.Contains(label, "Ability Score Increase") {
				label = "__ability__" // unwrapped variant of the label above
			}
			entries = append(entries, entry{label, strings.TrimSpace(line[len(m[0]):])})
			i++
			continue
		}

		// Continuation of the previous trait's value.
		if len(entries) == 0 {
			*anomalies = append(*anomalies, Anomaly{
				Page: c.SourcePage, Subject: c.Name,
				Problem: "traits text before any label: " + snippet(line),
			})
			i++
			continue
		}
		entries[len(entries)-1].value += " " + line
		i++
	}

	for _, lb := range entries {
		value := strings.TrimSpace(lb.value)

		switch {
		case lb.label == "__ability__":
			c.AbilityRaw = value
			// Choice-based increases ("+2 Wis or Int", "+2 to any Ability
			// Score") must NOT be stored as fixed increases — the raw text
			// stays authoritative and the creation flow presents the choice.
			lower := strings.ToLower(value)
			if strings.Contains(lower, " or ") || strings.Contains(lower, "any ") {
				break
			}
			for _, am := range abilityAmountRe.FindAllStringSubmatch(value, -1) {
				n, _ := strconv.Atoi(am[1])
				ab, ok := abilityNames[strings.ToLower(am[2])]
				if !ok {
					*anomalies = append(*anomalies, Anomaly{
						Page: c.SourcePage, Subject: c.Name,
						Problem: "unrecognized ability in increase: " + am[2],
					})
					continue
				}
				c.AbilityIncreases = append(c.AbilityIncreases, AbilityIncrease{ab, n})
			}
			if len(c.AbilityIncreases) == 0 {
				*anomalies = append(*anomalies, Anomaly{
					Page: c.SourcePage, Subject: c.Name,
					Problem: "could not parse ability increases: " + snippet(value),
				})
			}
		case lb.label == "Speed":
			c.SpeedText = value
			if m := speedFeetRe.FindStringSubmatch(value); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					c.SpeedFeet = &n
				}
			}
			if c.SpeedFeet == nil {
				*anomalies = append(*anomalies, Anomaly{
					Page: c.SourcePage, Subject: c.Name,
					Problem: "could not parse walking speed: " + snippet(value),
				})
			}
		case lb.label == "Skill Proficiencies" || lb.label == "Skill Proficiency":
			for _, s := range strings.Split(value, ",") {
				if s = strings.TrimSpace(s); s != "" {
					c.SkillProfs = append(c.SkillProfs, s)
				}
			}
		case lb.label == "Extra Language" || lb.label == "Extra Languages" || lb.label == "Languages":
			c.ExtraLanguage = value
		default:
			c.Traits = append(c.Traits, ClanTrait{Name: lb.label, Description: value})
		}
	}

	if c.AbilityRaw == "" {
		*anomalies = append(*anomalies, Anomaly{
			Page: c.SourcePage, Subject: c.Name, Problem: "no ability score increase found in TRAITS",
		})
	}
	if c.SpeedText == "" {
		*anomalies = append(*anomalies, Anomaly{
			Page: c.SourcePage, Subject: c.Name, Problem: "no Speed found in TRAITS",
		})
	}
}

// startsLower reports whether the line begins with a lowercase letter —
// used to spot label values that wrapped onto the next printed line.
func startsLower(s string) bool {
	for _, r := range s {
		return r >= 'a' && r <= 'z'
	}
	return false
}
