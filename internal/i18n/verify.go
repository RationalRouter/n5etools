package i18n

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// This file provides the pure, language-agnostic checks behind
// `n5e-i18n verify` (see cmd/n5e-i18n/main.go's cmdVerify, which owns the DB
// reads and DeepL back-translation calls and just calls into these).
// "Language-agnostic" is the point: every check here works without reading
// or understanding the target language at all, which is what makes it a
// real signal for a script no one maintaining this project reads fluently (see
// translation_methodology.md's "Automated coverage pipeline" section) --
// unlike a spot-check read, these can't be fooled by fluent-looking nonsense
// or miss a dropped clause just because the surrounding prose still parses.

// PlaceholdersMatch reports whether en and target carry the same
// placeholder tokens, in the same order and count -- a translation that
// dropped, duplicated, or reordered a %s would otherwise render broken or
// crash a Sprintf call, and this is true regardless of what language target
// is written in.
func PlaceholdersMatch(en, target string) (ok bool, enTokens, targetTokens []string) {
	enTokens = PlaceholderTokens(en)
	targetTokens = PlaceholderTokens(target)
	if len(enTokens) != len(targetTokens) {
		return false, enTokens, targetTokens
	}
	for i := range enTokens {
		if enTokens[i] != targetTokens[i] {
			return false, enTokens, targetTokens
		}
	}
	return true, enTokens, targetTokens
}

// identicalValueMinLen is the shortest English string SuspiciouslyIdentical
// bothers flagging -- short strings (a single word, a number, a proper
// noun) legitimately survive translation unchanged far more often than
// prose does, so flagging every one of those would drown out real misses.
const identicalValueMinLen = 15

// SuspiciouslyIdentical reports whether target is byte-identical to en for
// a string long enough that an untouched, un-translated pass-through is the
// far more likely explanation than a genuine coincidence.
func SuspiciouslyIdentical(en, target string) bool {
	return len(en) >= identicalValueMinLen && en == target
}

// LengthRatioOutlier reports whether target's length is implausibly far
// from en's for prose (as opposed to, say, a single glossary-pinned term) --
// catches truncation (a translation cut short by a batch failure recorded
// as a partial success) or garbled/duplicated output. Only meaningful past
// a minimum English length; short strings have too much natural variance
// (a two-word phrase can legitimately double or halve in length across
// languages) to say anything useful.
const lengthRatioMinLen = 20
const lengthRatioLow = 0.3
const lengthRatioHigh = 3.0

func LengthRatioOutlier(en, target string) (ratio float64, outlier bool) {
	if len(en) < lengthRatioMinLen || len(target) == 0 {
		return 0, false
	}
	ratio = float64(len(target)) / float64(len(en))
	return ratio, ratio < lengthRatioLow || ratio > lengthRatioHigh
}

// GlossaryChecker precompiles glossary.md's English->target term pairs into
// matchers once, so checking thousands of translated rows against a few
// hundred glossary entries doesn't recompile a regex per term per row (the
// naive per-call approach this replaced was O(rows * terms) regex
// compilations -- for this project's real volume, tens of millions of
// them). Build one via NewGlossaryChecker and reuse it for every row.
type GlossaryChecker struct {
	entries []glossaryEntry
}

type glossaryEntry struct {
	source        string
	sourceRe      *regexp.Regexp   // \b<source>\b -- checked against English (case-sensitive for single-word terms, see sourceTermRe)
	targetStemRes []*regexp.Regexp // one \b<stem> per word of the target rendering
	singleWord    bool             // subject to the proper-name rule in termPresent
}

// NewGlossaryChecker precompiles terms (English -> approved target-language
// rendering, as returned by ParseGlossaryTables) for repeated MissingTerms
// calls.
func NewGlossaryChecker(terms map[string]string) *GlossaryChecker {
	gc := &GlossaryChecker{}
	for source, want := range terms {
		if source == want {
			continue // identity pairs (proper nouns left untouched) have nothing to verify on the target side beyond "still present", which SuspiciouslyIdentical / a manual read covers better
		}
		entry := glossaryEntry{
			source:     source,
			sourceRe:   sourceTermRe(source),
			singleWord: !strings.ContainsAny(source, " \t"),
		}
		for _, w := range strings.Fields(want) {
			entry.targetStemRes = append(entry.targetStemRes, stemPrefixRe(stem(w)))
		}
		if entry.sourceRe == nil || len(entry.targetStemRes) == 0 {
			continue
		}
		gc.entries = append(gc.entries, entry)
	}
	return gc
}

// stem crudely truncates a word's likely inflectional suffix (plural -s/-es,
// Spanish/Portuguese gender -a/-o, etc.) so a target-language check tolerates
// grammatical agreement DeepL is expected to apply around a pinned glossary
// term -- e.g. glossary "Saving throw" -> "Tirada de Salvación" still
// matches translated prose using the plural "tiradas de salvación". This is
// deliberately crude (no real morphology), which is fine for its one job:
// avoiding exact-phrase false positives, not proving a term IS present.
func stem(word string) string {
	r := []rune(word)
	if len(r) > 5 {
		return string(r[:len(r)-2])
	}
	if len(r) > 3 {
		return string(r[:len(r)-1])
	}
	return word
}

// wordBoundary is a Unicode-aware substitute for regexp's `\b`, which only
// recognizes ASCII word characters ([0-9A-Za-z_]) -- a target-language stem
// or source term starting with an accented letter (e.g. Portuguese "Água")
// never has a real `\b` immediately before it, because RE2 treats "Á" as a
// non-word character, so the space-then-Á transition it sees is
// non-word-to-non-word, not a boundary. Found 2026-09-12 while chasing a
// wave of "missing-glossary-term: Water Release" false positives in
// already-correct Portuguese rows -- confirmed live that `\bÁgu` fails to
// match "e Água" while `\bAgu` matches "e Agua" fine, same text otherwise.
// This matches on any non-letter/non-digit boundary instead of ASCII \w.
const wordBoundary = `(?:^|[^\p{L}\p{N}_])`
const wordBoundaryEnd = `(?:$|[^\p{L}\p{N}_])`

// wholeWordRe matches s as a complete word (both boundaries) -- used for the
// English source side, where a leading-only boundary let "Trip" match inside
// "Triple" and "Light" match inside "Lightning" (Hatake's whole kit is
// "Lightning Release", which is not a hit on the glossary's "Light" entry).
func wholeWordRe(s string) *regexp.Regexp {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return regexp.MustCompile(`(?i)` + wordBoundary + regexp.QuoteMeta(s) + wordBoundaryEnd)
}

// sourceTermRe is wholeWordRe, except that a SINGLE-word glossary term is
// matched case-sensitively.
//
// This is what separates a pinned game term from its ordinary-English
// homograph, and it is the difference between a check with signal and one
// buried in noise. Every one of these terms is a mechanic this game
// capitalizes in its own source text -- "a Light weapon", "the Thrown
// property", "gain Advantage" -- while the same letters in ordinary prose
// are lowercase: "bright light", "set it on fire", "within range". A
// case-insensitive English side cannot tell them apart, so it flagged every
// row whose Spanish correctly rendered "bright light" as "luz brillante"
// for not containing "Ligera" (the feminine adjective meaning lightweight,
// which would be nonsense there). Measured 2026-09-13: this single change
// removed 281 of 368 Spanish findings, all verified false positives of
// exactly that shape.
//
// Multi-word terms stay case-insensitive. "Fire Release", "Class Feature"
// and friends have no ordinary-prose homograph to confuse them with, and
// tolerating sentence-case variation there is worth more than the
// precision would be.
//
// The cost is real but small and in the safe direction: a genuinely drifted
// term sitting at the start of a sentence, where ordinary prose is
// capitalized too, is still flagged (no loss); a pinned term that the
// source text itself spelled lowercase is now missed. Prefer fixing the
// source text's own capitalization over reverting this -- the game's
// mechanics are proper nouns and should read as such.
func sourceTermRe(s string) *regexp.Regexp {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.ContainsAny(s, " \t") {
		return wholeWordRe(s)
	}
	// Group 1 is the term itself. wordBoundary/wordBoundaryEnd are real
	// consuming character classes, not zero-width assertions (RE2 has no
	// Unicode \b -- see wordBoundary's own comment), so a match's own
	// indices include the surrounding punctuation/space. termPresent needs
	// to look at the words either side of the term, so it needs the term's
	// own bounds, not the padded match's.
	return regexp.MustCompile(wordBoundary + `(` + regexp.QuoteMeta(s) + `)` + wordBoundaryEnd)
}

// stemPrefixRe matches s as the start of a word only (leading boundary, no
// trailing one) -- used for target-language stems, which are deliberately
// truncated (see stem) so "tirad" is meant to match inside both "tirada"
// and "tiradas". A trailing boundary here would require the truncated stem
// to BE the whole word, which defeats stemming entirely.
func stemPrefixRe(s string) *regexp.Regexp {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return regexp.MustCompile(`(?i)` + wordBoundary + regexp.QuoteMeta(s))
}

// MissingTerms returns every glossary entry whose English term appears in en
// (as a whole word/phrase) but none of whose approved target-language
// rendering's words (allowing for simple inflection, see stem) appear
// anywhere in target -- flags a translation that drifted from an approved
// proper noun or established rendering even though the glossary was
// supplied as a DeepL constraint (DeepL can still choose not to apply a
// glossary hit inside certain contexts, e.g. mid-compound-word in some
// languages).
//
// Known, accepted noise source: several glossary terms are ordinary English
// words being pinned to a specific in-game sense ("Range" as this game's
// own homebrew mechanic vs. the common word "range"; also "Light",
// "Advantage", "Thrown", "Feats"). This check has no way to tell which
// sense a given hit is -- e.g. "within your weapon's range" is ordinary
// prose, not a hit on the glossary's capitalized "**Range**" -> "Distancia"
// entry, but the check flags it anyway when DeepL (correctly) rendered it
// as "alcance" instead. Verified live against real es data (2026-09-11):
// after fixing the two real bugs this check surfaced (the wordBoundaryRe
// missing its trailing \b, and glossary.md's Bukijutsu row literally
// containing the word "unchanged"), remaining findings are dominated by
// this common-word ambiguity, not further bugs. This is inherent to a
// token-level check against short/common glossary terms, not fixable by
// tuning thresholds -- a length-based cutoff would just blind the check to
// real drift on those same short pinned terms. Left as WARN-level advisory
// noise by design: still catches real term drift, just needs a reviewer's
// glance rather than being treated as a hard failure per row.
func (gc *GlossaryChecker) MissingTerms(en, target string) []string {
	var missing []string
	matched := map[string]bool{}
	for _, e := range gc.entries {
		if !termPresent(e, en) {
			continue
		}
		matched[e.source] = true
		anyStemMissing := false
		for _, re := range e.targetStemRes {
			if !re.MatchString(target) {
				anyStemMissing = true
				break
			}
		}
		if anyStemMissing {
			missing = append(missing, e.source)
		}
	}
	missing = dropSubsumedTerms(missing, matched)
	sort.Strings(missing)
	return missing
}

// termPresent reports whether a glossary term occurs in en in its pinned
// game sense.
//
// For a multi-word term that is just "does the regex match." For a
// single-word one it additionally requires at least one occurrence that is
// NOT part of a multi-word capitalized run, because such a run is a proper
// name rather than the standalone mechanic: "Fire Bomb", "Eye Lights",
// "Mechanical Light Shield", "No Light at the End". Those are item and
// upgrade names whose approved Spanish deliberately does not contain the
// glossary rendering -- "Fire Bomb" is "Bomba incendiaria", which is both
// correct and idiomatic, and demanding it contain "Fuego" would be wrong.
//
// The mechanic's own real usages survive, because they are a capitalized
// term followed by lowercase prose: "a Light weapon", "the Thrown
// property", "gain Advantage on the roll". Measured 2026-09-13: this and
// the case-sensitivity rule together took Spanish from 368 findings to 40,
// with every removed finding spot-checked as a false positive.
func termPresent(e glossaryEntry, en string) bool {
	locs := e.sourceRe.FindAllStringSubmatchIndex(en, -1)
	if len(locs) == 0 {
		return false
	}
	if !e.singleWord {
		return true
	}
	for _, loc := range locs {
		// loc[2]:loc[3] is group 1 -- the term without its consumed
		// boundary characters (see sourceTermRe).
		if !inTitleCaseRun(en, loc[2], loc[3]) && !atSentenceStart(en, loc[2]) {
			return true
		}
	}
	return false
}

// inTitleCaseRun reports whether en[start:end] has a capitalized word
// immediately before or after it, separated only by a space -- the shape of
// a multi-word proper name.
func inTitleCaseRun(en string, start, end int) bool {
	capitalizedAt := func(s string) bool {
		for _, r := range s {
			return unicode.IsUpper(r)
		}
		return false
	}
	// Word immediately after.
	if rest := en[end:]; strings.HasPrefix(rest, " ") {
		if f := strings.Fields(rest); len(f) > 0 && capitalizedAt(f[0]) {
			return true
		}
	}
	// Word immediately before.
	if before := en[:start]; strings.HasSuffix(before, " ") {
		if f := strings.Fields(before); len(f) > 0 && capitalizedAt(f[len(f)-1]) {
			return true
		}
	}
	return false
}

// atSentenceStart reports whether en[start:] begins a sentence or a bullet
// item -- a position where English capitalizes every word, mechanic or not.
//
// This is the same reasoning as inTitleCaseRun: the capital carries no
// information there, so it is not evidence that the pinned mechanic is
// meant. "• Light created by your Enlightening Grasp feature now lasts..."
// is the ordinary noun, correctly rendered "La luz creada por...", and
// demanding the weapon property's "Ligera" would be wrong.
func atSentenceStart(en string, start int) bool {
	before := strings.TrimRight(en[:start], " \t")
	if before == "" {
		return true
	}
	switch before[len(before)-1] {
	case '.', '!', '?', ':', ';', '\n':
		return true
	}
	return strings.HasSuffix(before, "•")
}

// dropSubsumedTerms applies longest-match-wins: when a row's English
// matched both a short term and a longer one that contains it, only the
// longer one is reported.
//
// "Fire" and "Fire Release" are both glossary entries, and every occurrence
// of the latter is also an occurrence of the former -- so a row about Fire
// Release jutsu reported both, doubling the noise while saying nothing the
// specific entry didn't already say. The longer entry is also the correct
// authority: "descarga de Fuego" is what that text should render as, not
// the bare "Fuego" the short entry checks for, so the short entry's
// complaint is actively wrong there.
//
// Only suppresses when the containing term ACTUALLY matched this row's
// English -- a bare "Fire" in a row with no "Fire Release" in it is still
// reported normally.
func dropSubsumedTerms(missing []string, matched map[string]bool) []string {
	if len(missing) == 0 {
		return missing
	}
	kept := missing[:0:0]
	for _, term := range missing {
		subsumed := false
		for other := range matched {
			if other == term || len(other) <= len(term) {
				continue
			}
			if wholeWordRe(term).MatchString(other) {
				subsumed = true
				break
			}
		}
		if !subsumed {
			kept = append(kept, term)
		}
	}
	return kept
}

// TextSimilarity returns a language-agnostic rough similarity score in
// [0,1] between a and b, via word-level Jaccard overlap after lowercasing
// and stripping punctuation. It is deliberately crude -- no stemming, no
// semantics -- but that's sufficient for its one job: scoring a
// back-translation (target text translated back to English via DeepL)
// against the original English to catch gross meaning drift (a dropped
// clause, a swapped subject, a garbled sentence), not to judge translation
// quality. 1.0 means identical word sets; scores in the 0.4-0.7 range are
// normal for a faithful back-translation that just rephrases, since DeepL's
// EN->X->EN round trip is rarely word-for-word even when the meaning holds.
func TextSimilarity(a, b string) float64 {
	wa := wordSet(a)
	wb := wordSet(b)
	if len(wa) == 0 && len(wb) == 0 {
		return 1
	}
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	union := len(wa) + len(wb) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

func wordSet(s string) map[string]bool {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make(map[string]bool, len(fields))
	for _, f := range fields {
		if len(f) >= 2 { // drop single-char tokens (articles/punctuation remnants) -- too noisy to carry signal
			out[f] = true
		}
	}
	return out
}

// BackTranslationDriftThreshold is the TextSimilarity score below which
// RunVerify's back-translation check flags a row for review. Chosen loose
// deliberately (round trips naturally rephrase) -- this is meant to catch
// gross drift (garbled/wrong-meaning output), not to nitpick phrasing.
const BackTranslationDriftThreshold = 0.25

// VerifyFinding is one language-agnostic QA result from `n5e-i18n verify`.
type VerifyFinding struct {
	Table  string
	Slug   string
	Field  string
	Kind   string // "placeholder-mismatch" | "suspiciously-identical" | "length-ratio-outlier" | "missing-glossary-term" | "backtranslation-drift"
	Detail string
}
