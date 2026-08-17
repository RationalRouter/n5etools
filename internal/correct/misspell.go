package correct

import "github.com/client9/misspell"

// misspellReplacer is package-level and built once — Replacer.Compile()
// walks a fixed rule list at construction, no reason to redo that per call.
var misspellReplacer = misspell.New()

// misspellFix runs the standard misspell dictionary (common English
// mistakes only — "recieve", "teh", "alot" — never jargon or proper nouns,
// so it's safe to auto-apply with no review) against s.
func misspellFix(s string) (string, []textDiff) {
	fixed, diffs := misspellReplacer.Replace(s)
	if len(diffs) == 0 {
		return s, nil
	}
	out := make([]textDiff, len(diffs))
	for i, d := range diffs {
		out[i] = textDiff{tool: "misspell", original: d.Original, corrected: d.Corrected}
	}
	return fixed, out
}
