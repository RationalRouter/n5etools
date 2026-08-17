// Class feats — the three sections closing the class compendium
// (ARCHETYPE / CASTER / MARTIAL CLASS FEATS, before LEGACY CONTENT).
//
// Entries are ordinary feat blocks (same shape as clan feats), grouped
// under per-class sub-headings ("SCIENCE-NIN") that carry no data of their
// own — each feat's "Archetype: <class>" label already names its class.
package parse

import "strings"

// classFeatSections are the headings that open the class-feat run.
var classFeatSections = map[string]bool{
	"CLASS FEATS":           true,
	"ARCHETYPE CLASS FEATS": true,
	"CASTER CLASS FEATS":    true,
	"MARTIAL CLASS FEATS":   true,
}

// isClassSubheading reports whether a caps line is one of the 11 class
// names (letters compared only — the sub-headings drop punctuation:
// "COOKING NIN" for Cooking-Nin).
func isClassSubheading(text string) bool {
	letters := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToUpper(s) {
			if r >= 'A' && r <= 'Z' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	t := letters(text)
	if t == "" {
		return false
	}
	for name := range classGroupHeadings {
		if letters(name) == t {
			return true
		}
	}
	// The sections also group feats for subclasses with no class in this
	// book ("WITCH ARCHETYPE", "KNIGHTS ARCHETYPE [NEW]").
	return strings.HasSuffix(strings.TrimSuffix(t, "NEW"), "ARCHETYPE")
}

// ParseClassFeats scans the class-book line stream for the class-feat
// sections and returns their feat entries.
func ParseClassFeats(lines []Line) ([]Feat, []Anomaly) {
	var (
		feats     []Feat
		anomalies []Anomaly
	)

	// Locate the run, applying the standard line filter.
	var ls []Line
	inSection := false
	for _, ln := range lines {
		if !inSection {
			inSection = classFeatSections[ln.Text]
			continue
		}
		if ln.Text == "LEGACY CONTENT" {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		anomalies = append(anomalies, Anomaly{Subject: "Class Feats",
			Problem: "sections not found"})
		return nil, anomalies
	}

	fs, anoms := parseFeatRun(ls, func(text string) bool {
		// Section and per-class sub-headings ("MARTIAL CLASS FEATS",
		// "SCIENCE-NIN", "COOKING NIN") carry no data of their own — and
		// must be skipped BEFORE feat detection, or they glue into the
		// following feat's multi-line name.
		return classFeatSections[text] || isClassSubheading(text)
	}, "Class Feats")
	feats = append(feats, fs...)
	anomalies = append(anomalies, anoms...)

	// Every subclass feat should name its class.
	for _, f := range feats {
		if strings.Contains(f.Category, "Archetype") && f.ClassName == "" {
			anomalies = append(anomalies, Anomaly{Page: f.SourcePage, Subject: f.Name,
				Problem: "subclass feat without an Archetype: class label"})
		}
	}
	return feats, anomalies
}

// parseFeatRun walks a stretch of feat blocks. skipHeading marks the
// navigational headings between feats; anything else that is not a feat
// (sidebar tables and their captions) is appended to the feat printed above
// it and flagged once per feat for human review.
func parseFeatRun(ls []Line, skipHeading func(string) bool, subject string) ([]Feat, []Anomaly) {
	var (
		feats     []Feat
		anomalies []Anomaly
	)
	flaggedAppend := false
	for i := 0; i < len(ls); i++ {
		text := ls[i].Text
		if skipHeading(text) {
			continue
		}
		if startsFeat(ls, i) {
			f, next := parseFeat(ls, i)
			feats = append(feats, f)
			flaggedAppend = false
			i = next - 1
			continue
		}
		if len(feats) > 0 {
			last := &feats[len(feats)-1]
			last.Description += " " + text
			if !flaggedAppend {
				flaggedAppend = true
				anomalies = append(anomalies, Anomaly{Page: ls[i].Page, Subject: last.Name,
					Problem: "sidebar/table text appended to description: " + snippet(text)})
			}
			continue
		}
		anomalies = append(anomalies, Anomaly{Page: ls[i].Page, Subject: subject,
			Problem: "text outside any feat: " + snippet(text)})
	}
	return feats, anomalies
}
