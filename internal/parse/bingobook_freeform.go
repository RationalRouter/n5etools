// Bingo Book Pack 1, Adversary Combat chapter: the slice of tables and
// glossaries whose printed layout is regular enough to parse safely.
//
// Several other tables in this same chapter — Freeform DR, Freeform Damage
// Penalty, Freeform Jutsu Shapes, Effect & Conditions — were investigated
// and deliberately left OUT of this parser: their column layout wraps
// irregularly across lines depending on cell content length (e.g. a short
// row prints "Standard +2 +4 +6 +8 +10" on one line, while an adjacent row
// with an em-dash placeholder wraps its row label onto its own line first).
// A parser built to tolerate that wrapping would be guessing at row
// boundaries rather than reading a fixed layout, which is exactly the
// "wrong column layout is expensive to unwind" risk this project's own
// investigate-before-committing discipline exists to catch. Those tables
// are small (4-17 rows each) and are hand-transcribed into a seed migration
// instead, the same precedent 0012_weapon_properties.sql already set for
// this book's non-enumerable content.
//
// "Elite Abilities" (the book's own EXAMPLES under FREEFORM ABILITIES —
// Dirty Fighting, Shuriken Trick, Amaterasu, etc.) and "Elite Actions" are
// NOT parsed as a catalog at all, auto or otherwise: the book explicitly
// frames the former as GM-authoring inspiration, not a pool to randomly
// assign to generated adversaries, and the latter is a fixed 4-option rules
// mechanic (Attack/Cast/Command/Trait), not enumerable data.
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// FreeformStrikeRow is one level's row of the Freeform Strike Table —
// damage dice for a Freeform Strike, read from the Low/Medium/High column
// matching the adversary's class (Minion/Standard/Elite-Solo).
type FreeformStrikeRow struct {
	LevelMin int
	Low      string
	Medium   string
	High     string // empty at levels 1-2, where the book prints an em-dash
}

var freeformStrikeRowRe = regexp.MustCompile(`^(\d+) (\S+) (\S+) (\S+|—)$`)

// ParseFreeformStrikeTable scans the FREEFORM STRIKE TABLE (Adversary Combat
// chapter): a clean one-line-per-level table, "Level Low Medium High".
func ParseFreeformStrikeTable(lines []Line) ([]FreeformStrikeRow, []Anomaly) {
	i := indexOfText(lines, "FREEFORM STRIKE TABLE")
	if i < 0 {
		return nil, []Anomaly{{Subject: "Freeform Strike Table", Problem: "heading not found"}}
	}
	var rows []FreeformStrikeRow
	var anomalies []Anomaly
	// lines[i+1] is the "Level Low Medium High" column header.
	for j := i + 2; j < len(lines); j++ {
		text := lines[j].Text
		m := freeformStrikeRowRe.FindStringSubmatch(text)
		if m == nil {
			break
		}
		lvl, _ := strconv.Atoi(m[1])
		high := m[4]
		if high == "—" {
			high = ""
		}
		rows = append(rows, FreeformStrikeRow{LevelMin: lvl, Low: m[2], Medium: m[3], High: high})
	}
	if len(rows) == 0 {
		anomalies = append(anomalies, Anomaly{Page: lines[i].Page, Subject: "Freeform Strike Table",
			Problem: "no rows matched after heading"})
	}
	return rows, anomalies
}

// FreeformJutsuDamageRow is one level's row of one of the two Freeform Jutsu
// Damage tables (Minion/Standard, or Elite/Solo). Tier is
// "minion_standard" or "elite_solo".
type FreeformJutsuDamageRow struct {
	Tier     string
	LevelMin int
	VeryLow  string
	Low      string
	Medium   string
	High     string
	VeryHigh string
}

// freeformJutsuDieBreakRe fixes a PDF kerning artifact where a die
// expression's flat modifier prints with a stray space before it (e.g.
// "6d10 +11" for what should read "6d10+11") — confirmed against the real
// PDF, a one-off rendering glitch rather than a real alternate format.
var freeformJutsuDieBreakRe = regexp.MustCompile(`(\d+d\d+) +([+-]\d+)`)
var freeformJutsuRowStartRe = regexp.MustCompile(`^\d+ `)

// ParseFreeformJutsuDamageTable scans both FREEFORM JUTSU DAMAGE TABLE
// blocks (Adversary Combat chapter). Each row is "Lvl cell cell cell cell
// cell", where a real cell is "dice (avg)" (the parenthesized average is
// dropped — not needed for table lookups) and a blank cell at low levels
// prints as a lone em-dash.
func ParseFreeformJutsuDamageTable(lines []Line) ([]FreeformJutsuDamageRow, []Anomaly) {
	var rows []FreeformJutsuDamageRow
	var anomalies []Anomaly
	parse := func(tier, heading string) {
		i := indexOfText(lines, heading)
		if i < 0 {
			anomalies = append(anomalies, Anomaly{Subject: "Freeform Jutsu Damage Table", Problem: heading + " heading not found"})
			return
		}
		// lines[i+1..i+10] are the wrapped 5-column header ("Lvl", "Very
		// Low", "D-Rank", "Low", "C-Rank", ...); the first real data row is
		// the first line matching "<level> ".
		j := i + 1
		for j < len(lines) && !freeformJutsuRowStartRe.MatchString(lines[j].Text) {
			j++
		}
		for ; j < len(lines); j++ {
			text := lines[j].Text
			if !freeformJutsuRowStartRe.MatchString(text) {
				break
			}
			fields := strings.Fields(freeformJutsuDieBreakRe.ReplaceAllString(text, "$1$2"))
			lvl, _ := strconv.Atoi(fields[0])
			cells, ok := extractParenCells(fields[1:], 5)
			if !ok {
				anomalies = append(anomalies, Anomaly{Page: lines[j].Page,
					Subject: heading, Problem: "row did not resolve to 5 cells: " + text})
				continue
			}
			rows = append(rows, FreeformJutsuDamageRow{
				Tier: tier, LevelMin: lvl,
				VeryLow: cells[0], Low: cells[1], Medium: cells[2], High: cells[3], VeryHigh: cells[4],
			})
		}
	}
	parse("minion_standard", "FREEFORM JUTSU DAMAGE TABLE – MINION/STANDARDS")
	parse("elite_solo", "FREEFORM JUTSU DAMAGE TABLE – ELITES/SOLO’S")
	return rows, anomalies
}

// extractParenCells walks tokens pairing a dice expression with its
// following "(avg)" token (dropped) into one cell, or taking a lone em-dash
// as a standalone blank cell, until want cells are collected.
func extractParenCells(tokens []string, want int) ([]string, bool) {
	var cells []string
	for i := 0; i < len(tokens) && len(cells) < want; {
		tok := tokens[i]
		if tok == "—" {
			cells = append(cells, "")
			i++
			continue
		}
		if i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "(") && strings.HasSuffix(tokens[i+1], ")") {
			cells = append(cells, tok)
			i += 2
			continue
		}
		return nil, false
	}
	return cells, len(cells) == want
}

// ClassificationModifier is the flat stat bumps an Iconic or Epic Optional
// Classification adds. Kind is "iconic" or "epic".
type ClassificationModifier struct {
	Kind             string
	HPChakraModifier int
	ACBonus          int
	AttackSaveBonus  int
	AbilityPoints    int
	InitiativeBonus  int
}

var (
	classModHitChakraRe   = regexp.MustCompile(`^Hit/Chakra Point Modifier: ([+-]?\d+)$`)
	classModACSaveRe      = regexp.MustCompile(`^AC Bonus ([+-]?\d+) Attack Bonus/Save DC ([+-]?\d+)$`)
	classModAbilityInitRe = regexp.MustCompile(`^Ability Points\s+([+-]?\d+) Initiative ([+-]?\d+)$`)
)

// ParseClassificationModifiers scans the ICONIC MODIFIERS/EPIC MODIFIERS
// blocks (Optional Classifications section), each a fixed 3-line pattern.
func ParseClassificationModifiers(lines []Line) ([]ClassificationModifier, []Anomaly) {
	var mods []ClassificationModifier
	var anomalies []Anomaly
	parse := func(kind, heading string) {
		i := indexOfText(lines, heading)
		if i < 0 || i+3 >= len(lines) {
			anomalies = append(anomalies, Anomaly{Subject: heading, Problem: "heading not found or block truncated"})
			return
		}
		hcM := classModHitChakraRe.FindStringSubmatch(lines[i+1].Text)
		acM := classModACSaveRe.FindStringSubmatch(lines[i+2].Text)
		abM := classModAbilityInitRe.FindStringSubmatch(lines[i+3].Text)
		if hcM == nil || acM == nil || abM == nil {
			anomalies = append(anomalies, Anomaly{Page: lines[i].Page, Subject: heading, Problem: "modifier lines not recognized"})
			return
		}
		hc, _ := strconv.Atoi(hcM[1])
		ac, _ := strconv.Atoi(acM[1])
		as, _ := strconv.Atoi(acM[2])
		ap, _ := strconv.Atoi(abM[1])
		init, _ := strconv.Atoi(abM[2])
		mods = append(mods, ClassificationModifier{
			Kind: kind, HPChakraModifier: hc, ACBonus: ac, AttackSaveBonus: as,
			AbilityPoints: ap, InitiativeBonus: init,
		})
	}
	parse("iconic", "ICONIC MODIFIERS")
	parse("epic", "EPIC MODIFIERS")
	return mods, anomalies
}

// AdversaryClarification is one term from the book's own CLARIFICATIONS
// glossary, defining how certain Role Trait wording should be read.
type AdversaryClarification struct {
	Term        string
	Description string
	SourcePage  int
}

// adversaryClarificationTerms is the book's fixed, closed set of glossary
// terms — same "closed vocabulary" convention as adversaryRoleNames, since
// each term always opens with "Term. " at the start of its own paragraph
// and nothing else in this glossary block takes that shape.
var adversaryClarificationTerms = []string{
	"Auras", "Ambush", "Barrier", "Intercept", "Mark", "Strike", "Saving Throws",
}

// ParseAdversaryClarifications scans the CLARIFICATIONS block between the
// ROLE TRAITS heading and the first Role heading.
func ParseAdversaryClarifications(lines []Line) ([]AdversaryClarification, []Anomaly) {
	i := indexOfText(lines, "CLARIFICATIONS")
	if i < 0 {
		return nil, []Anomaly{{Subject: "Adversary Clarifications", Problem: "CLARIFICATIONS heading not found"}}
	}
	var end int
	for end = i + 1; end < len(lines); end++ {
		if adversaryRoleNames[lines[end].Text] {
			break
		}
	}
	ls := lines[i+1 : end]

	var terms []AdversaryClarification
	var anomalies []Anomaly
	var cur *AdversaryClarification
	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if cur.Description == "" {
			anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Term, Problem: "clarification with empty description"})
		} else {
			terms = append(terms, *cur)
		}
		cur = nil
	}
	for _, ln := range ls {
		matched := ""
		rest := ""
		for _, term := range adversaryClarificationTerms {
			if strings.HasPrefix(ln.Text, term+". ") {
				matched = term
				rest = strings.TrimPrefix(ln.Text, term+". ")
				break
			}
		}
		if matched != "" {
			flush()
			cur = &AdversaryClarification{Term: matched, Description: rest, SourcePage: ln.Page}
			continue
		}
		if cur == nil {
			continue
		}
		cur.Description += " " + ln.Text
	}
	flush()
	return terms, anomalies
}

// TransformationBuff is one of the fixed enhancement options a GM picks 2-3
// of when building a Solo Adversary's Transformation.
type TransformationBuff struct {
	Name        string
	Description string
	SortOrder   int
	SourcePage  int
}

// ParseTransformationBuffs scans the TRANSFORMATION BUFFS bullet list
// (Transformations section under Adversary Combat): each bullet opens with
// "• Name. description...", running until the next "• " bullet or the
// TRANSFORMATION EXCLUSIVE ABILITY heading that ends the list. A nested
// "o ..." sub-bullet inside one entry (Expanded Size) is not a new buff —
// it has no "• " prefix, so it stays part of that buff's description.
func ParseTransformationBuffs(lines []Line) ([]TransformationBuff, []Anomaly) {
	i := indexOfText(lines, "TRANSFORMATION BUFFS")
	if i < 0 {
		return nil, []Anomaly{{Subject: "Transformation Buffs", Problem: "heading not found"}}
	}
	end := indexOfText(lines[i:], "TRANSFORMATION EXCLUSIVE ABILITY")
	if end < 0 {
		return nil, []Anomaly{{Subject: "Transformation Buffs", Problem: "TRANSFORMATION EXCLUSIVE ABILITY heading not found"}}
	}
	ls := lines[i+1 : i+end]

	var buffs []TransformationBuff
	var anomalies []Anomaly
	var cur *TransformationBuff
	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if cur.Description == "" {
			anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Name, Problem: "transformation buff with empty description"})
		} else {
			buffs = append(buffs, *cur)
		}
		cur = nil
	}
	order := 0
	for _, ln := range ls {
		if strings.HasPrefix(ln.Text, "• ") {
			flush()
			rest := strings.TrimPrefix(ln.Text, "• ")
			name, desc, _ := strings.Cut(rest, ". ")
			order++
			cur = &TransformationBuff{Name: name, Description: desc, SortOrder: order, SourcePage: ln.Page}
			continue
		}
		if cur == nil {
			continue
		}
		cur.Description += " " + ln.Text
	}
	flush()
	return buffs, anomalies
}

// indexOfText returns the index of the first line whose text equals want,
// or -1 if none does.
func indexOfText(lines []Line, want string) int {
	for i, ln := range lines {
		if ln.Text == want {
			return i
		}
	}
	return -1
}
