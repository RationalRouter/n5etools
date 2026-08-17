// Core book's Multiclassing rules (filed under the book's own "Legacy
// Rules" chapter — the author has since replaced multiclassing with the
// Archetype system, but the toolkit supports it anyway; see the maintainer
// discussion in the project history for why).
//
// Three small per-class reference tables, always in the same class order,
// each row "<Class Name> <value…>" with the value sometimes wrapping onto
// following lines (and, once, the class name itself wrapping — "Intelligence
// Operative" prints as "Intelligence" / "Operative <value…>"):
//
//	Class Ability Score Minimum      → MulticlassRule.AbilityPrereq
//	Class Proficiencies Gained       → MulticlassRule.ProficienciesGained
//	Class Jutsu Gained Per level     → MulticlassRule.JutsuPerLevel
//
// Values are kept as printed text (compound conditions like "Strength or
// Dexterity 14 & Constitution 14" aren't worth a bespoke grammar for a
// three-column reference table); a future rules-engine pass can parse them
// further if the multiclassing character-creation flow needs to evaluate
// them programmatically.
//
// The book also references a "MULTICLASSING JUTSU KNOWN TABLE" (total jutsu
// known by character level for a multiclassed character) — that table is an
// image with no extractable text and isn't in the Mastersheet either; it's a
// known gap, not a parser bug.
package parse

import "strings"

// MulticlassRule is one class's row across all three tables.
type MulticlassRule struct {
	ClassName           string // as printed in this table: "Cooking Nin", not "Cooking-Nin"
	AbilityPrereq       string
	ProficienciesGained string
	JutsuPerLevel       string
	SourcePage          int
}

// multiclassClassNames is the fixed, book-printed order all three tables
// share. Spelling here doesn't always match the class compendium's own
// hyphenation ("Cooking Nin" vs "Cooking-Nin") — store.Slugify normalizes
// both to the same slug.
var multiclassClassNames = []string{
	"Genjutsu Specialist", "Hunter-Nin", "Intelligence Operative", "Medical-Nin",
	"Ninjutsu Specialist", "Scout-Nin", "Taijutsu Specialist", "Weapon Specialist",
	"Puppet Master", "Cooking Nin", "Science Nin",
}

// multiclassSpellings lists every literal spelling a canonical class name
// might print as — the book hyphenates "Cooking-Nin"/"Science-Nin" in the
// jutsu-ratio table but not in the other two.
var multiclassSpellings = map[string][]string{
	"Cooking Nin": {"Cooking Nin", "Cooking-Nin"},
	"Science Nin": {"Science Nin", "Science-Nin"},
}

// multiclassTableHeaders must also stop a row's continuation text — the
// last row of each table runs straight into the next table's header line.
var multiclassTableHeaders = map[string]bool{
	"Class Ability Score Minimum":  true,
	"Class Proficiencies Gained":   true,
	"Class Jutsu Gained Per level": true,
}

// matchMulticlassName tests whether a class name starts at ls[i] — either
// on one line, or (only "Intelligence Operative", in either table) split as
// "Intelligence" / "Operative <value…>" across two. Used both to open a row
// and to decide when the previous row's continuation text must stop, so the
// split case can never be mistaken for ordinary prose in either direction.
func matchMulticlassName(ls []Line, i int) (name, valueTail string, consumed int, ok bool) {
	if i >= len(ls) {
		return "", "", 0, false
	}
	if ls[i].Text == "Intelligence" && i+1 < len(ls) && strings.HasPrefix(ls[i+1].Text, "Operative") {
		return "Intelligence Operative", strings.TrimSpace(strings.TrimPrefix(ls[i+1].Text, "Operative")), 2, true
	}
	for _, n := range multiclassClassNames {
		spellings := multiclassSpellings[n]
		if spellings == nil {
			spellings = []string{n}
		}
		for _, spelling := range spellings {
			if strings.HasPrefix(ls[i].Text, spelling) {
				return n, strings.TrimSpace(strings.TrimPrefix(ls[i].Text, spelling)), 1, true
			}
		}
	}
	return "", "", 0, false
}

// parseMulticlassRows reads one table starting at ls[i] (just past its
// header line) and returns the row text keyed by class name, plus the index
// just past the table.
func parseMulticlassRows(ls []Line, i int) (map[string]string, int, []Anomaly) {
	var anomalies []Anomaly
	rows := map[string]string{}
	for _, name := range multiclassClassNames {
		matched, rest, consumed, ok := matchMulticlassName(ls, i)
		if !ok || matched != name {
			anomalies = append(anomalies, Anomaly{
				Page: lineOr(ls, i).Page, Subject: name,
				Problem: "expected row not found in multiclassing table",
			})
			continue
		}
		i += consumed
		for i < len(ls) {
			_, _, _, nameStop := matchMulticlassName(ls, i)
			// Sub-headings between tables ("CLASS FEATURES", "JUTSU
			// MODIFIERS", ...) print in caps, unlike the Title Case class
			// names, so capsLineRe safely stops a row without an explicit
			// list of every such heading.
			if nameStop || multiclassTableHeaders[ls[i].Text] || capsLineRe.MatchString(ls[i].Text) {
				break
			}
			rest += " " + ls[i].Text
			i++
		}
		rows[name] = strings.TrimSpace(rest)
	}
	return rows, i, anomalies
}

func lineOr(ls []Line, i int) Line {
	if i < len(ls) {
		return ls[i]
	}
	return Line{}
}

// ParseMulticlassRules scans the core-book line stream for the Multiclassing
// section's three tables.
func ParseMulticlassRules(lines []Line) ([]MulticlassRule, []Anomaly) {
	var anomalies []Anomaly

	var ls []Line
	inSection := false
	prevPage := 0
	for _, ln := range lines {
		if ln.Text == "MULTICLASSING" {
			inSection = true
		}
		if !inSection {
			continue
		}
		// A genuine page-footer number is always the first line extracted
		// from its physical page; the ability-prereq table wraps a bare
		// stat value ("...& Constitution\n14") onto its own line mid-page,
		// which pageNumberRe would otherwise wrongly discard as a footer.
		isPageBreak := ln.Page != prevPage
		prevPage = ln.Page
		if (isPageBreak && pageNumberRe.MatchString(ln.Text)) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		return nil, []Anomaly{{Subject: "Multiclassing", Problem: "section not found"}}
	}

	abilityIdx, profIdx, jutsuIdx := -1, -1, -1
	for i, ln := range ls {
		switch ln.Text {
		case "Class Ability Score Minimum":
			abilityIdx = i + 1
		case "Class Proficiencies Gained":
			profIdx = i + 1
		case "Class Jutsu Gained Per level":
			jutsuIdx = i + 1
		}
	}
	if abilityIdx < 0 || profIdx < 0 || jutsuIdx < 0 {
		return nil, []Anomaly{{Subject: "Multiclassing",
			Problem: "one or more table headers not found"}}
	}

	abilities, _, aAnoms := parseMulticlassRows(ls, abilityIdx)
	profs, _, pAnoms := parseMulticlassRows(ls, profIdx)
	jutsu, _, jAnoms := parseMulticlassRows(ls, jutsuIdx)
	anomalies = append(anomalies, aAnoms...)
	anomalies = append(anomalies, pAnoms...)
	anomalies = append(anomalies, jAnoms...)

	rules := make([]MulticlassRule, 0, len(multiclassClassNames))
	for _, name := range multiclassClassNames {
		rules = append(rules, MulticlassRule{
			ClassName: name, AbilityPrereq: abilities[name],
			ProficienciesGained: profs[name], JutsuPerLevel: jutsu[name],
			SourcePage: ls[abilityIdx].Page,
		})
	}
	return rules, anomalies
}
