package correct

import "regexp"

// bonusPluralRe matches "a +N bonuses" (or "a N bonuses" without the sign) —
// a singular article paired with the plural noun "bonuses," where the game
// text always means a single bonus. Confirmed by a full-corpus audit
// (2026-09-04, see internal/i18n/source_text_grammar_audit.md): 147
// occurrences across 11 tables, all in the identical "gain a +N bonuses to
// ..." shape, none a legitimate plural use — this is a systemic authoring
// habit, not 147 unrelated typos.
var bonusPluralRe = regexp.MustCompile(`\ba (\+?\d+) bonuses\b`)

// bonusPluralFix corrects the "a +N bonuses" -> "a +N bonus" number
// agreement error. Local, instant, safe to always run.
func bonusPluralFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := bonusPluralRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := bonusPluralRe.FindStringSubmatch(m)
		corrected := "a " + sub[1] + " bonus"
		diffs = append(diffs, textDiff{tool: "bonusplural", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
