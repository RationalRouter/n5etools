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
// real signal for a script neither Sergio nor Claude reads fluently (see
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
	sourceRe      *regexp.Regexp   // \b<source>\b, case-insensitive -- checked against English
	targetStemRes []*regexp.Regexp // one \b<stem> per word of the target rendering
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
			source:   source,
			sourceRe: wholeWordRe(source),
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

// wholeWordRe matches s as a complete word (both boundaries) -- used for the
// English source side, where a leading-only \b let "Trip" match inside
// "Triple" and "Light" match inside "Lightning" (Hatake's whole kit is
// "Lightning Release", which is not a hit on the glossary's "Light" entry).
func wholeWordRe(s string) *regexp.Regexp {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(s) + `\b`)
}

// stemPrefixRe matches s as the start of a word only (leading \b, no
// trailing one) -- used for target-language stems, which are deliberately
// truncated (see stem) so "tirad" is meant to match inside both "tirada"
// and "tiradas". A trailing \b here would require the truncated stem to
// BE the whole word, which defeats stemming entirely.
func stemPrefixRe(s string) *regexp.Regexp {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(s))
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
// noise by design: still catches real term drift, just needs a human (or
// Claude) glance rather than being treated as a hard failure per row.
func (gc *GlossaryChecker) MissingTerms(en, target string) []string {
	var missing []string
	for _, e := range gc.entries {
		if !e.sourceRe.MatchString(en) {
			continue
		}
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
	sort.Strings(missing)
	return missing
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
