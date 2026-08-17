package correct

import "regexp"

// possessiveDropRe targets a specific, mechanical error pattern found
// throughout this corpus (full-database audit, 2026-08-05, 210+ confirmed
// instances): the PDF extraction repeatedly drops the possessive apostrophe
// on one of a small set of singular game-mechanical nouns specifically when
// one of a small set of possessed nouns follows immediately, mid-sentence —
// "a creatures cells" instead of "a creature's cells", "your weapons range"
// instead of "your weapon's range". A genuine plural use of these head
// nouns is always followed by a verb ("creatures gain...") or nothing (list
// end), never directly by another noun, so requiring one of the possessed
// nouns immediately after is a safe, low-false-positive signal without full
// sentence parsing — same reasoning as apostrophePluralRe's own scoping.
// Both lists are closed and hand-confirmed against the full corpus; extend
// either one confirmed instance at a time, not by loosening the pattern.
var possessiveDropRe = regexp.MustCompile(`(?i)\b(creature|target|weapon|caster|user|clone)s (arm|arms|body|cells|chakra|consciousness|core|current space|damage|damage reduction|hit points|maximum hit points|mind|next turn|range|space|throat|turn|turns)\b`)

// adversaryPossessiveDropRe is the same drop, isolated to adversary_role_
// traits: "Adversary" pluralizes irregularly to "Adversaries", so the
// regular-plural pattern above never matches it. Scoped to "this/these
// Adversaries <noun>" specifically — confirmed against every instance in
// the table (19) that the word immediately following is always one of this
// closed noun list, never a verb, which is what makes the drop safe to
// correct without also having to decide whether "this"/"these" themselves
// need to change (left as printed, matching the audit's own recommendation).
var adversaryPossessiveDropRe = regexp.MustCompile(`\b(this|these) Adversaries (allies|hit|movement|next|rank|reach|turn)\b`)

// possessiveFix inserts the missing apostrophe, preserving whatever case
// the head noun happened to be found in (almost always lowercase, but the
// capture — not a hardcoded literal — is what gets reused).
func possessiveFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := possessiveDropRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := possessiveDropRe.FindStringSubmatch(m)
		head, tail := sub[1], sub[2]
		corrected := head + "’s " + tail
		diffs = append(diffs, textDiff{tool: "possessive", original: m, corrected: corrected})
		return corrected
	})
	fixed = adversaryPossessiveDropRe.ReplaceAllStringFunc(fixed, func(m string) string {
		sub := adversaryPossessiveDropRe.FindStringSubmatch(m)
		det, tail := sub[1], sub[2]
		corrected := det + " Adversary’s " + tail
		diffs = append(diffs, textDiff{tool: "possessive", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
