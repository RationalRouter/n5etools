package correct

import "regexp"

// penaltyPluralRe matches "a -N penalties" — the mirror-image of
// bonusPluralRe's "a +N bonuses": a singular article paired with the plural
// noun "penalties," where a single penalty is being applied. Confirmed by a
// full-corpus audit (2026-09-05, see internal/i18n/source_text_grammar_audit.md):
// 63 occurrences across 10 tables, same systemic authoring habit as the
// bonuses case.
var penaltyPluralRe = regexp.MustCompile(`\ba (-?\d+) penalties\b`)

// penaltyPluralFix corrects the "a -N penalties" -> "a -N penalty" number
// agreement error. Local, instant, safe to always run.
func penaltyPluralFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := penaltyPluralRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := penaltyPluralRe.FindStringSubmatch(m)
		corrected := "a " + sub[1] + " penalty"
		diffs = append(diffs, textDiff{tool: "penaltyplural", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
