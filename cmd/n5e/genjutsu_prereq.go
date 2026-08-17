package main

import (
	"regexp"
	"strconv"
	"strings"
)

// Malleable Mirages' own "Prerequisite" column (class_options.prerequisites,
// list_name "Malleable Mirages", 49 of 79 rows) stores three independent
// clause shapes, sometimes combined with a comma and in EITHER order —
// "Temporal Stopwatch, 10th level" (name-first) vs "13th level, Reality
// marble" (level-first, Magnum Opus) — unlike Puppet Master's own upgrade
// corpus (puppet_upgrade_prereq.go), which is always name-first. This parser
// therefore splits on top-level commas and tests each clause independently
// against all three matchers, order-independent, rather than porting that
// file's left-to-right greedy scanner.
//
// The three clause shapes:
//   - A level threshold ("9th Level", "9th level.", "13th level.") — always
//     Genjutsu Specialist class level, never character level (the feature's
//     own text: "A level prerequisite in a Malleable Mirage refers to
//     Genjutsu Specialist Level, not character level").
//   - One of the 6 Genjutsu Inception names, sometimes with a trailing
//     PDF-extraction artifact ("Reality Marble Genjutsu pledge",
//     "Phantasmal Force Genjutsu pledge") that an exact match alone would
//     miss — handled by stripping a small set of known filler suffixes
//     before retrying, never a general substring search (a substring search
//     would wrongly match Inception name "Illusionary Weapon" inside the
//     unrelated Mirage name "Supreme Illusionary Weapon").
//   - Another Mirage's own name, matched once Mirages are real tracked
//     picks.
//
// 3 rows (Maddening Pain, Relentless Pain, Vampiric Pain) reference a named
// Genjutsu jutsu ("Pain, Doubled Pain or Unlimited Pain") that exists in
// neither catalog — same accepted-gap shape puppet_upgrade_prereq.go already
// documents for "Improved Architecture I": a clause matching none of the 3
// shapes above is simply dropped (never treated as unmet), so these 3 rows
// resolve to zero enforced clauses and are shown with their raw prerequisite
// text, never blocked.
var reGenjutsuLevelPrereq = regexp.MustCompile(`(?i)(\d+)(?:st|nd|rd|th)\s*level`)

var genjutsuPrereqFillerSuffixes = []string{
	" genjutsu pledge",
	" pledge",
}

// genjutsuPrereqClause is one parsed comma-split clause. Kind is "", "level",
// "inception", or "mirage" — "" means the clause didn't resolve to any of
// the 3 known shapes and is dropped by the caller.
type genjutsuPrereqClause struct {
	Kind  string
	Level int
	Name  string // lowercased; only set for "inception"/"mirage"
}

// parseGenjutsuPrereqClause classifies one clause against the full known
// name sets (every real Inception/Mirage name, not just what the character
// currently knows) — that distinction matters: a clause naming a REAL
// Inception the character doesn't have must block (return unmet below), while
// a clause naming nothing recognizable must not.
func parseGenjutsuPrereqClause(clause string, allInceptionNames, allMirageNames map[string]bool) genjutsuPrereqClause {
	clause = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(clause), "."))
	if clause == "" {
		return genjutsuPrereqClause{}
	}
	if m := reGenjutsuLevelPrereq.FindStringSubmatch(clause); m != nil {
		need, err := strconv.Atoi(m[1])
		if err == nil {
			return genjutsuPrereqClause{Kind: "level", Level: need}
		}
	}
	low := strings.ToLower(clause)
	if allInceptionNames[low] {
		return genjutsuPrereqClause{Kind: "inception", Name: low}
	}
	if allMirageNames[low] {
		return genjutsuPrereqClause{Kind: "mirage", Name: low}
	}
	for _, suf := range genjutsuPrereqFillerSuffixes {
		if strings.HasSuffix(low, suf) {
			if stripped := strings.TrimSpace(strings.TrimSuffix(low, suf)); allInceptionNames[stripped] {
				return genjutsuPrereqClause{Kind: "inception", Name: stripped}
			}
		}
	}
	return genjutsuPrereqClause{}
}

// genjutsuMiragePrereqMet reports whether a character meets one Mirage's
// full prerequisite text (blank text always passes). allInceptionNames/
// allMirageNames are the full catalogs (lowercased names); known* are the
// character's own picks (lowercased names, not slugs — the prerequisite
// column stores names).
func genjutsuMiragePrereqMet(prereqText string, genjutsuLevel int, allInceptionNames, allMirageNames, knownInceptionNames, knownMirageNames map[string]bool) bool {
	prereqText = strings.TrimSpace(prereqText)
	if prereqText == "" {
		return true
	}
	for _, raw := range strings.Split(prereqText, ",") {
		clause := parseGenjutsuPrereqClause(raw, allInceptionNames, allMirageNames)
		switch clause.Kind {
		case "level":
			if genjutsuLevel < clause.Level {
				return false
			}
		case "inception":
			if !knownInceptionNames[clause.Name] {
				return false
			}
		case "mirage":
			if !knownMirageNames[clause.Name] {
				return false
			}
		default:
			// Unresolvable clause: never blocks (see doc above).
		}
	}
	return true
}
