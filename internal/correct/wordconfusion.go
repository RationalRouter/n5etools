package correct

import "regexp"

// allayRe matches the standalone word "allay" (to soothe/lessen) — a
// real-word homophone confusion for "ally" (a companion) that misspell
// can't catch since both are valid dictionary words. Confirmed by a
// full-corpus scan (2026-08-05): every one of the 34 occurrences of this
// word anywhere in the database is this mistake, in game-mechanical
// context ("your Bonded allay", "an allay is reduced to 0") where only
// "ally" makes sense — none is a legitimate use of the verb "allay".
var allayRe = regexp.MustCompile(`\b([Aa])llay\b`)

// wordConfusionFix applies this project's closed set of confirmed
// real-word confusions. Extend one confirmed, full-corpus-checked instance
// at a time — this is not a general homophone corrector.
func wordConfusionFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := allayRe.ReplaceAllStringFunc(s, func(m string) string {
		corrected := m[:1] + "lly" // preserves whichever case the first letter matched in
		diffs = append(diffs, textDiff{tool: "wordconfusion", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
