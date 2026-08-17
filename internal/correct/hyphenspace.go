package correct

import "regexp"

// hyphenSpaceContinuations is a closed, hand-confirmed allowlist (full
// corpus scan, 2026-08-05: 289 confirmed instances of this exact shape)
// of words that legitimately continue a hyphenated compound the PDF
// extraction split with a stray space — "D-Rank" -> "D- Rank", "30-foot" ->
// "30- foot", "hand-to-hand" -> "hand-to- hand", "Will-O-Wisp" -> "Will-
// O- Wisp". This is deliberately an ALLOWLIST rather than a blacklist of
// stopwords: a bare hyphen is also used in this corpus as dash-style
// punctuation ("seclusion-either in a community or alone- for a formative
// part of your life", "adjacent to it-friend or foe- the affected
// creature"), and a naked number before a hyphen can also be a dropped
// range ending immediately followed by an unrelated new sentence/heading
// ("critical hit on a roll of 19- Tiger Grapple (Recharge 5-6)." — "20."
// was lost, "Tiger Grapple" is a new stat-block entry, not a compound).
// Both shapes look identical to the real bug under a loose stopword-based
// filter, so only join when the word after the space is confirmed, by
// direct inspection, to be a genuine compound continuation. Extend one
// confirmed instance at a time, same discipline as apostrophePluralRe's
// own scoping — do not loosen this to a general rule.
var hyphenSpaceRe = regexp.MustCompile(`(?i)\b([A-Za-z0-9]+)- (action|allied|based|body|case|chain|chakra|chilled|circuit|clan|compressed|cone|covered|damaging|dice|dog|elemental|enhanced|excluded|expansion|eyed|feet|flowing|foot|footlong|hand|handed|harm|heated|hostile|jutsu|like|loathing|minute|movement|nin|o|on|orchestrated|radius|range|rank|ranking|requisite|rest|sharp|shattering|sheathe|shocked|side|sized|so|suit|targeting|taught|teleportation|to|turn|up|wide|wisps)\b`)

// hyphenSpaceFix closes the stray space, preserving the case both sides
// were actually found in.
func hyphenSpaceFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := hyphenSpaceRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := hyphenSpaceRe.FindStringSubmatch(m)
		corrected := sub[1] + "-" + sub[2]
		diffs = append(diffs, textDiff{tool: "hyphenspace", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
