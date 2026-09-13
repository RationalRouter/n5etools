package i18n

import (
	"regexp"
	"strings"
)

// properNounPattern matches runs of 1-4 capitalized words -- the candidate
// shape for an in-game proper noun or coined term (e.g. "Chakra Control",
// "Fire Release", "Byakugan"). It's a coarse heuristic, not a real NLP
// pass: it will also match ordinary sentence-initial words and other
// incidental capitalization, which FindUnglossaried's caller filters with a
// known-terms set built from the database itself.
var properNounPattern = regexp.MustCompile(`\b[A-Z][a-zA-Z']*(?:\s+[A-Z][a-zA-Z']*){0,3}\b`)

// FindUnglossaried scans English text for candidate proper nouns/game terms
// that appear in neither glossary (glossary.md's English column, as parsed
// by ParseGlossaryTables) nor known (e.g. every class/clan/jutsu/item name
// already on record in rules.db) -- see cmd/n5e-i18n's translate-book
// command. Terms are returned in first-seen order, deduplicated. This is a
// first-pass approximation (see the plan doc/translation_methodology.md):
// it will miss a brand-new term that happens to share every word with an
// already-known one, and will over-flag ordinary capitalized text (a
// sentence's first word, an ability score) that the known-terms set doesn't
// happen to cover -- both are acceptable starting-point tradeoffs, meant to
// be revisited if they prove too noisy or too lax in practice.
func FindUnglossaried(text string, glossary map[string]string, known map[string]struct{}) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, loc := range properNounPattern.FindAllStringIndex(text, -1) {
		m := text[loc[0]:loc[1]]
		// A single capitalized word right at the start of the text is
		// almost always just ordinary sentence-initial capitalization, not
		// a proper noun -- skip it unless it's part of a longer phrase
		// (multi-word matches starting at 0, e.g. "Fire Release deals...",
		// are still worth flagging).
		if loc[0] == 0 && !strings.Contains(m, " ") {
			continue
		}
		if _, ok := glossary[m]; ok {
			continue
		}
		if _, ok := known[m]; ok {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}
