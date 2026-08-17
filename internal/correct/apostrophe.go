package correct

import (
	"regexp"
	"strings"
)

// apostrophePluralRe matches a word ending in a possessive-looking 's
// (either apostrophe style the PDFs use) that is directly abutted by
// closing punctuation with nothing after it — "Katana's," or "Odachi's."
var apostrophePluralRe = regexp.MustCompile(`([\p{L}]+)['’]s([,.;:])`)

// contractionBases excludes the short list of function words where a
// genuine contraction ("it's," "that's,") could plausibly land in exactly
// this position — the one real false-positive risk for this heuristic.
// Item/weapon/proper nouns don't coincide with these.
var contractionBases = map[string]bool{
	"it": true, "that": true, "there": true, "here": true,
	"what": true, "who": true, "where": true, "when": true,
	"why": true, "how": true, "let": true, "he": true, "she": true,
	"one": true, "everyone": true, "someone": true, "anyone": true,
	"nothing": true, "something": true, "anything": true,
	"everybody": true, "somebody": true, "anybody": true, "nobody": true,
	"today": true, "tomorrow": true, "yesterday": true,
}

// apostropheFix targets a specific, mechanical error pattern found in this
// corpus: a plural noun in a list mistakenly given a possessive apostrophe
// by the PDF's typesetting ("proficient with Katana's, Broadswords and
// Odachi's."). A genuine possessive is always followed by the noun it
// possesses ("the Hokage's decision") — it is never immediately abutted by
// closing punctuation with nothing after it, so that shape alone is a safe,
// low-false-positive signal without needing full sentence parsing. This
// intentionally only strips the apostrophe; it does not attempt to fix
// capitalization ("Katana's" -> "Katanas", not "katanas") since telling
// deliberately-capitalized proper nouns apart from mis-capitalized common
// nouns needs a dictionary of game terms this package doesn't have.
func apostropheFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := apostrophePluralRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := apostrophePluralRe.FindStringSubmatch(m)
		base, punct := sub[1], sub[2]
		if contractionBases[strings.ToLower(base)] {
			return m
		}
		corrected := base + "s" + punct
		diffs = append(diffs, textDiff{tool: "apostrophe", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
