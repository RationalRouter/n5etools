// Package parse turns extracted sourcebook text into structured records.
//
// Each sourcebook gets its own file and its own parser, named after the
// book's structure. Parsers never touch the database — they produce plain
// structs plus a list of Anomalies for the validation report. If a parser is
// unsure about something it records an anomaly instead of guessing silently.
package parse

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Line is one extracted text line tagged with the physical PDF page it came
// from, so every parsed record can carry provenance.
type Line struct {
	Page int
	Text string
}

// Jutsu is one parsed jutsu entry, fields matching the book's rigid block:
//
//	CHAKRA BLOW
//	Classification: Ninjutsu
//	Rank: E-Rank
//	Casting Time: 1 Action
//	Range: Weapons Range
//	Duration: Instant
//	Components: HS, CM, W(any)
//	Cost: 2 Chakra
//	Keywords: Ninjutsu
//	Description: ...
//	At Higher Ranks: ...       (E-ranks say "At Higher Levels")
type Jutsu struct {
	Name           string
	Classification string
	Rank           string // normalized single letter: E,D,C,B,A,S
	RankRaw        string // as printed: "E-Rank"
	CastingTime    string
	Range          string
	Duration       string
	Components     string
	CostChakra     *int   // numeric chakra cost when the line starts with a number
	CostText       string // full cost text as printed
	Keywords       string
	Description    string
	AtHigherRanks  string
	CategoryGroup  string // book section, e.g. "Ninjutsu / Fire Release", "Summoning: Toad"
	SourcePage     int    // physical PDF page the entry starts on
}

// Anomaly is something a human should look at, destined for the validation
// report.
type Anomaly struct {
	Page    int
	Subject string // usually the entry name
	Problem string
}

// ---------------------------------------------------------------------------
// Book structure knowledge (explicit and auditable on purpose)
// ---------------------------------------------------------------------------

// Top-level sections of Jiraiya's Jutsu Compendium, as printed.
var jutsuTopSections = map[string]string{
	"NINJUTSU":        "Ninjutsu",
	"SUMMONING JUTSU": "Summoning",
	"GENJUTSU":        "Genjutsu",
	"TAIJUTSU":        "Taijutsu",
	"BUKIJUTSU":       "Bukijutsu",
}

// Ninjutsu subsections (nature releases).
var jutsuSubSections = map[string]string{
	"NON-ELEMENTAL":     "Non-Elemental",
	"EARTH RELEASE":     "Earth Release",
	"WIND RELEASE":      "Wind Release",
	"FIRE RELEASE":      "Fire Release",
	"WATER RELEASE":     "Water Release",
	"LIGHTNING RELEASE": "Lightning Release",
}

// Summoning animal contracts (from the book's own table of contents).
var summonAnimals = map[string]string{
	"BEAR": "Bear", "BOAR": "Boar", "DEER": "Deer", "DOG/WOLF": "Dog/Wolf",
	"FOX": "Fox", "HARE/ RABBIT": "Hare/Rabbit", "HARE/RABBIT": "Hare/Rabbit",
	"HAWK/ PREDATOR BIRDS": "Hawk/Predator Birds", "HAWK/PREDATOR BIRDS": "Hawk/Predator Birds",
	"INSECT SWARM": "Insect Swarm", "LIZARD": "Lizard",
	"MONKEY/ PRIMATE": "Monkey/Primate", "MONKEY/PRIMATE": "Monkey/Primate",
	"OX/RAM": "Ox/Ram", "OX/ RAM": "Ox/Ram", "RAT": "Rat",
	"SHARK/PREDATOR FISH": "Shark/Predator Fish", "SHARK/ PREDATOR FISH": "Shark/Predator Fish",
	"SLUG": "Slug", "SNAKE": "Snake", "SPIDER": "Spider",
	"TIGER/LION": "Tiger/Lion", "TIGER/ LION": "Tiger/Lion",
	"TOAD": "Toad", "TURTLE": "Turtle", "WEASEL": "Weasel",
}

var (
	rankHeaderRe = regexp.MustCompile(`^([EDCBAS])-RANK:?\s*$`)
	// A field line: "Casting Time: 1 Action". Field names are fixed.
	fieldRe = regexp.MustCompile(`^(Classification|Rank|Casting Time|Range|Duration|Components|Cost|Keywords|Description|At Higher Ranks|At Higher Levels)\s*:\s*(.*)$`)
	// The book has a handful of typos where a field label lost its colon
	// ("Rank C-Rank", "Cost 12 Chakra", "Description You take..."). This
	// lenient form recovers them — but it is only consulted when the strict
	// form fails AND that field has not been seen yet in the entry, so prose
	// lines that merely begin with a field word can't hijack a parsed value.
	fieldNoColonRe = regexp.MustCompile(`^(Classification|Rank|Casting Time|Range|Duration|Components|Cost|Keywords|Description|At Higher Ranks|At Higher Levels)\s+(\S.*)$`)
	// Entry names / headings are set in caps (small-caps in print). Allow
	// digits and common punctuation: "SEALING ART: STRING LIGHT FORMATION",
	// "8-INNER GATES: SEIMON", "AMMO HEART [NAME/ CHANGED]". \p{Lu} instead
	// of A-Z because clan names carry accents: "HYŪGA", "SHÍ HÓU". At least
	// one uppercase letter is required so a bare wrapped number ("20.") is
	// never mistaken for a heading.
	capsLineRe = regexp.MustCompile(`^[0-9 ,:;''’\-–—&/().!?\[\]]*\p{Lu}[\p{Lu}0-9 ,:;''’\-–—&/().!?\[\]]*$`)
	// Page footers arrive as bare numbers.
	pageNumberRe = regexp.MustCompile(`^\d+$`)
	leadingIntRe = regexp.MustCompile(`^(\d+)`)
	rankValueRe  = regexp.MustCompile(`^([EDCBAS])\b`)
)

// jutsuFieldOrder is the fixed order fields are printed in every entry. A
// field label can only START a field if it comes strictly later than the
// field currently being collected — anything else is wrapped prose that
// merely begins with a field word (e.g. a description line-break falling
// right before "Rank: 3, A-Rank: 4" in a DC table).
var jutsuFieldOrder = map[string]int{
	"Classification": 1, "Rank": 2, "Casting Time": 3, "Range": 4,
	"Duration": 5, "Components": 6, "Cost": 7, "Keywords": 8,
	"Description": 9, "At Higher Ranks": 10, "At Higher Levels": 10,
}

// ParseJutsuBook parses the whole jutsu compendium line stream. It tracks the
// running section context (top section → subsection → rank header) and cuts
// entries wherever a caps line is immediately followed by a
// "Classification:" line — the one pattern that reliably starts every jutsu
// entry in this book.
func ParseJutsuBook(lines []Line) ([]Jutsu, []Anomaly) {
	var (
		out       []Jutsu
		anomalies []Anomaly

		topSection  string // "Ninjutsu", "Summoning", ...
		subSection  string // "Fire Release", "Toad", ...
		rankSection string // current rank header letter, for cross-checking
	)

	// Drop page-footer numbers up front; they interrupt wrapped field values.
	var ls []Line
	for _, ln := range lines {
		if pageNumberRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}

	group := func() string {
		switch {
		case topSection == "Summoning" && subSection != "":
			return "Summoning: " + subSection
		case topSection != "" && subSection != "":
			return topSection + " / " + subSection
		case topSection != "":
			return topSection
		default:
			return ""
		}
	}

	i := 0
	for i < len(ls) {
		text := ls[i].Text

		// Section bookkeeping. Order matters: rank headers and known section
		// headings are also caps lines, so they are claimed first.
		if m := rankHeaderRe.FindStringSubmatch(text); m != nil {
			rankSection = m[1]
			i++
			continue
		}
		if name, ok := jutsuTopSections[text]; ok {
			topSection = name
			subSection = ""
			i++
			continue
		}
		if name, ok := jutsuSubSections[text]; ok && topSection == "Ninjutsu" {
			subSection = name
			i++
			continue
		}
		if name, ok := summonAnimals[text]; ok && topSection == "Summoning" {
			subSection = name
			i++
			continue
		}

		// Entry start: one or more caps lines directly followed by
		// "Classification:". Multi-line caps runs are wrapped long names.
		if capsLineRe.MatchString(text) {
			nameLines := []string{text}
			j := i + 1
			for j < len(ls) && capsLineRe.MatchString(ls[j].Text) &&
				!rankHeaderRe.MatchString(ls[j].Text) {
				nameLines = append(nameLines, ls[j].Text)
				j++
			}
			if j < len(ls) && strings.HasPrefix(ls[j].Text, "Classification") {
				entry, next, ans := parseJutsuEntry(ls, i, j, strings.Join(nameLines, " "))
				entry.CategoryGroup = group()
				if rankSection != "" && entry.Rank != "" && entry.Rank != rankSection {
					ans = append(ans, Anomaly{
						Page: entry.SourcePage, Subject: entry.Name,
						Problem: "rank " + entry.Rank + " does not match the running " +
							rankSection + "-Rank section header",
					})
				}
				out = append(out, entry)
				anomalies = append(anomalies, ans...)
				i = next
				continue
			}
		}

		// Anything else (section intro prose, summoning flavor, creature stat
		// blocks) is skipped; the validation report totals what was parsed.
		i++
	}
	return out, anomalies
}

// parseJutsuEntry consumes one entry starting at nameIdx (name lines) with
// fields starting at fieldIdx. Returns the entry, the index after it, and any
// anomalies found.
func parseJutsuEntry(ls []Line, nameIdx, fieldIdx int, name string) (Jutsu, int, []Anomaly) {
	entry := Jutsu{
		Name:       tidyName(name),
		SourcePage: ls[nameIdx].Page,
	}
	var anomalies []Anomaly

	// Fields arrive as "Key: value" lines; wrapped values continue on
	// following non-field lines. Description and At Higher Ranks absorb
	// everything until the next entry/heading.
	currentField := ""
	var value strings.Builder

	setField := func() {
		if currentField == "" {
			return
		}
		v := strings.TrimSpace(value.String())
		switch currentField {
		case "Classification":
			entry.Classification = v
		case "Rank":
			entry.RankRaw = v
			if m := rankValueRe.FindStringSubmatch(v); m != nil {
				entry.Rank = m[1]
			}
		case "Casting Time":
			entry.CastingTime = fixDigitWordSquish(v)
		case "Range":
			entry.Range = v
		case "Duration":
			entry.Duration = v
		case "Components":
			entry.Components = v
		case "Cost":
			entry.CostText = fixDigitWordSquish(v)
			if m := leadingIntRe.FindStringSubmatch(v); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					entry.CostChakra = &n
				}
			}
		case "Keywords":
			entry.Keywords = v
		case "Description":
			entry.Description = stripArtistCredit(v)
		case "At Higher Ranks", "At Higher Levels":
			entry.AtHigherRanks = stripArtistCredit(v)
		}
		value.Reset()
	}

	// startsLaterField enforces the fixed print order (jutsuFieldOrder): a
	// label only opens a new field if it comes strictly after the one being
	// collected. Same-or-earlier labels are wrapped prose or a book typo
	// (e.g. Sunbeam labels its description "Keywords:" a second time) — they
	// are absorbed into the current field and flagged for review, so no text
	// is ever dropped.
	startsLaterField := func(name string) bool {
		return currentField == "" || jutsuFieldOrder[name] > jutsuFieldOrder[currentField]
	}

	i := fieldIdx
	for i < len(ls) {
		text := ls[i].Text

		// Next entry or section heading ends this one — but only once we are
		// in the free-text tail (Description onward). Field values never
		// contain caps-only lines; descriptions can be followed by them.
		if capsLineRe.MatchString(text) {
			// Peek: is this the start of the next entry (caps run then
			// Classification), a rank header, or a known section heading?
			if rankHeaderRe.MatchString(text) || isKnownHeading(text) || startsEntry(ls, i) {
				break
			}
			// Otherwise it is caps-looking content inside a description
			// (e.g. "DC 15" alone on a line); fall through and absorb it.
		}

		if m := fieldRe.FindStringSubmatch(text); m != nil {
			if !startsLaterField(m[1]) {
				// Out-of-order label: wrapped prose or a mislabeled field.
				// Absorb into the current field. One case is provably benign
				// and not worth review: "Rank:" right after a line ending in
				// "-" is a rank table wrapped mid-word ("…C-Rank: 2, B-\n
				// Rank: 3…"). Everything else gets flagged.
				benignWrap := m[1] == "Rank" &&
					strings.HasSuffix(strings.TrimSpace(value.String()), "-")
				if !benignWrap {
					anomalies = append(anomalies, Anomaly{
						Page: ls[i].Page, Subject: entry.Name,
						Problem: "label " + m[1] + ": inside " + currentField +
							" kept as text (wrapped line or mislabeled field — check)",
					})
				}
			} else {
				setField()
				currentField = m[1]
				rest := m[2]
				// The value may itself glue the next label on (art-wrapped
				// layouts): "Classification: Hijutsu Rank: D-Rank".
				for jutsuFieldOrder[currentField] < jutsuFieldOrder["Description"] {
					lbl, before, after, found := splitAtLaterLabel(rest, currentField)
					if !found {
						break
					}
					value.WriteString(before)
					setField()
					currentField = lbl
					rest = after
				}
				value.WriteString(rest)
				i++
				continue
			}
		}

		// Art-wrapped layouts can break a multi-word label ACROSS lines
		// ("Casting" / "Time: 1 Reaction"). If this short line plus the next
		// forms a field line, consume both.
		if len(strings.Fields(text)) <= 2 && i+1 < len(ls) {
			joined := text + " " + ls[i+1].Text
			if m := fieldRe.FindStringSubmatch(joined); m != nil && startsLaterField(m[1]) {
				setField()
				currentField = m[1]
				value.WriteString(m[2])
				i += 2
				continue
			}
		}

		// Book-typo recovery: a field label printed without its colon
		// ("Rank C-Rank", "Cost 12 Chakra"). Only taken for a field later in
		// print order than the current one, so ordinary prose starting with
		// a field word can't hijack a parsed value.
		if m := fieldNoColonRe.FindStringSubmatch(text); m != nil && startsLaterField(m[1]) {
			anomalies = append(anomalies, Anomaly{
				Page: ls[i].Page, Subject: entry.Name,
				Problem: "field " + m[1] + " printed without a colon (book typo; value recovered)",
			})
			setField()
			currentField = m[1]
			value.WriteString(m[2])
			i++
			continue
		}

		if currentField == "" {
			// Text before any field — should not happen inside an entry.
			anomalies = append(anomalies, Anomaly{
				Page: ls[i].Page, Subject: entry.Name,
				Problem: "unexpected text before first field: " + snippet(text),
			})
			i++
			continue
		}

		// Entries laid out around artwork have very short lines that glue a
		// value and the NEXT label together ("Hijutsu Rank:", "D-Rank
		// Casting Time: 1"). While still in the short-field phase (before
		// Description), split continuation lines at any embedded label that
		// comes later in print order. Description prose is never touched.
		if jutsuFieldOrder[currentField] < jutsuFieldOrder["Description"] {
			rest := text
			for {
				lbl, before, after, found := splitAtLaterLabel(rest, currentField)
				if !found {
					break
				}
				if before != "" {
					value.WriteString(" ")
					value.WriteString(before)
				}
				setField()
				currentField = lbl
				rest = after
			}
			value.WriteString(" ")
			value.WriteString(rest)
			i++
			continue
		}

		// Wrapped continuation of the current field.
		value.WriteString(" ")
		value.WriteString(text)
		i++
	}
	setField()

	// Required-field audit: every printed entry has these.
	required := map[string]string{
		"Classification": entry.Classification,
		"Rank":           entry.RankRaw,
		"Casting Time":   entry.CastingTime,
		"Range":          entry.Range,
		"Duration":       entry.Duration,
		"Components":     entry.Components,
		"Cost":           entry.CostText,
		"Keywords":       entry.Keywords,
		"Description":    entry.Description,
	}
	for field, v := range required {
		if v == "" {
			anomalies = append(anomalies, Anomaly{
				Page: entry.SourcePage, Subject: entry.Name,
				Problem: "missing field " + field,
			})
		}
	}
	return entry, i, anomalies
}

// midLabelRe finds a field label embedded mid-line, for the glued-label
// recovery in parseJutsuEntry. \b keeps "D-Rank Casting Time:" from
// matching at "Rank" (no colon there) while still matching "Casting Time:".
var midLabelRe = regexp.MustCompile(`\b(Classification|Rank|Casting Time|Range|Duration|Components|Cost|Keywords|Description|At Higher Ranks|At Higher Levels)\s*:\s*`)

// digitWordSquishRe catches a real PDF-extraction artifact found in Casting
// Time/Cost values specifically: a lost space between a leading digit and
// the unit word that follows it ("1Action", "6Chakra" — confirmed the only
// two rows in the whole shipped corpus with this shape). Casting
// Time/Cost are short, semi-structured fields (deliberately excluded from
// internal/correct's prose pipeline, see its registry.go doc, since a
// grammar tool is more likely to mangle jargon there than fix a real
// mistake) — this is narrower and safe: a digit is never legitimately
// followed directly by a capitalized word with no separator in this book.
var digitWordSquishRe = regexp.MustCompile(`(\d)([A-Z][a-z])`)

func fixDigitWordSquish(s string) string {
	return digitWordSquishRe.ReplaceAllString(s, "$1 $2")
}

// splitAtLaterLabel finds the first label in s that comes strictly later in
// print order than current, and splits s around it.
func splitAtLaterLabel(s, current string) (label, before, after string, found bool) {
	for _, m := range midLabelRe.FindAllStringSubmatchIndex(s, -1) {
		lbl := s[m[2]:m[3]]
		if jutsuFieldOrder[lbl] > jutsuFieldOrder[current] {
			return lbl, strings.TrimSpace(s[:m[0]]), strings.TrimSpace(s[m[1]:]), true
		}
	}
	return "", "", "", false
}

// startsEntry reports whether the caps run beginning at i is followed by a
// Classification line (i.e. a new jutsu entry starts here).
func startsEntry(ls []Line, i int) bool {
	j := i
	for j < len(ls) && capsLineRe.MatchString(ls[j].Text) &&
		!rankHeaderRe.MatchString(ls[j].Text) {
		j++
	}
	return j > i && j < len(ls) && strings.HasPrefix(ls[j].Text, "Classification")
}

func isKnownHeading(text string) bool {
	if _, ok := jutsuTopSections[text]; ok {
		return true
	}
	if _, ok := jutsuSubSections[text]; ok {
		return true
	}
	if _, ok := summonAnimals[text]; ok {
		return true
	}
	// Clan-book section headings — jutsu entries also appear inside clan
	// sections, and an entry's free-text tail must end at these too.
	// "SYNTHETIC HUMAN JUTSU" is that clan's irregular jutsu heading.
	if text == "CLAN FEATS" || text == "BLOODLINE LATENTS" ||
		text == "SYNTHETIC HUMAN JUTSU" || text == "ART CREDIT" {
		return true
	}
	for _, suffix := range []string{" CLAN", " TRAITS", " FEATURES", " CLAN JUTSU"} {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

// tidyName converts an ALL-CAPS printed name to title case for storage:
// "SEALING ART: STRING LIGHT FORMATION" → "Sealing Art: String Light
// Formation". Small connector words stay lowercase except at the start.
func tidyName(name string) string {
	small := map[string]bool{"OF": true, "THE": true, "A": true, "AN": true,
		"AND": true, "OR": true, "TO": true, "IN": true, "ON": true}
	words := strings.Fields(name)
	for i, w := range words {
		if i > 0 && small[w] && !strings.HasSuffix(words[i-1], ":") {
			words[i] = strings.ToLower(w)
			continue
		}
		words[i] = titleWord(w)
	}
	return strings.Join(words, " ")
}

// titleWord lowercases a caps word except its first letter, preserving
// punctuation-separated parts: "STRING" → "String", "8-INNER" → "8-Inner",
// "DOG/WOLF" → "Dog/Wolf". Unicode-aware for accented clan names:
// "FŪSHIN" → "Fūshin", "HÓU" → "Hóu".
func titleWord(w string) string {
	runes := []rune(w)
	upperNext := true
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if !upperNext {
				runes[i] = unicode.ToLower(r)
			}
			upperNext = false
			continue
		}
		if r == '-' || r == '/' || r == '(' || r == ':' || r == '.' {
			upperNext = true
		}
	}
	return string(runes)
}

func snippet(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
