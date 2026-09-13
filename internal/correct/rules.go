package correct

import (
	"regexp"
	"strings"

	"github.com/client9/misspell"
)

// The local correction rules, one function each, all sharing the signature
//
//	func(string) (string, []textDiff)
//
// which is what lets Sweep drive them from the localPasses table in
// correct.go instead of repeating a near-identical block per rule. Adding a
// rule means writing its function here and adding one line to that table.
//
// Every rule below is deliberately narrow and was derived from a full-corpus
// audit rather than from general English grammar. That is the standing
// discipline for this package: the corpus mixes real prose with terms of art
// that look like mistakes, so a rule that is "obviously right" in the
// abstract will mangle game text. Where a rule carries an allowlist or a
// closed set, extend it one confirmed instance at a time; do not loosen the
// pattern. See internal/i18n/source_text_grammar_audit.md for the audits.

// --- misspell: standard dictionary ---------------------------------------

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

// --- apostrophe: false plural-possessive in a list ------------------------

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

// --- hyphenspace: compound split by a stray space -------------------------

// hyphenSpaceRe's continuation list is a closed, hand-confirmed allowlist
// (full corpus scan, 2026-08-05: 289 confirmed instances of this exact
// shape) of words that legitimately continue a hyphenated compound the PDF
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
// confirmed instance at a time — do not loosen this to a general rule.
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

// --- possessive: dropped apostrophe --------------------------------------

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

// possessiveDetRe is the second branch of the same dropped-apostrophe bug,
// found by a follow-up full-corpus sweep (2026-09-13, 45 confirmed instances).
// Where possessiveDropRe identifies the drop by the *possessed* noun that
// follows, this one identifies it by the singular determiner that precedes:
// "a creatures speed", "an affected targets AC", "one clones turn". A
// singular determiner immediately before a plural head noun is already
// ungrammatical, so the head is necessarily a dropped possessive — which
// makes the possessed noun free to be anything, and lifts the need for a
// closed list on that side.
//
// One optional adjective is allowed between determiner and head ("a hostile
// creatures neck"). The word after the head is captured rather than
// lookahead-asserted (RE2 has no lookahead) and checked against
// possessiveNotPossessed below: "a creatures is" and "a creature's attack"
// are both real corpus shapes, and only the second is a possessive.
var possessiveDetRe = regexp.MustCompile(`(?i)\b(a|an|one)(\s+(?:[A-Za-z-]+\s+)?)(creature|target|weapon|caster|user|clone)s(’?'?)(\s+)([A-Za-z]+)`)

// possessiveNotPossessed are the words that, following the head noun, mean
// the sentence is not a possessive at all — pronouns, prepositions,
// conjunctions and the handful of verbs the corpus actually uses in this
// position. Anything not listed here is a noun being possessed.
var possessiveNotPossessed = map[string]bool{
	"you": true, "them": true, "it": true, "me": true, "us": true, "him": true,
	"her": true, "they": true, "a": true, "an": true, "the": true, "to": true,
	"with": true, "and": true, "or": true, "of": true, "in": true, "on": true,
	"at": true, "are": true, "is": true, "was": true, "were": true, "be": true,
	"been": true, "have": true, "has": true, "had": true, "can": true,
	"could": true, "will": true, "would": true, "may": true, "might": true,
	"must": true, "that": true, "this": true, "these": true, "those": true,
	"who": true, "which": true, "within": true, "from": true, "by": true,
	"for": true, "if": true, "when": true, "as": true, "into": true,
	"but": true, "so": true, "then": true, "than": true, "there": true,
	"no": true, "not": true, "do": true, "does": true, "did": true,
	"make": true, "makes": true, "gain": true, "gains": true, "take": true,
	"takes": true, "move": true, "moves": true, "attack": true, "attacks": true,
}

// possessiveNounRe is the same shape as possessiveDropRe — head noun plus a
// closed list of possessed nouns — extended with the 149 further possessed
// nouns the 2026-09-13 sweep confirmed across the corpus. It is kept separate
// from possessiveDropRe rather than merged into it so the original 2026-08-05
// list stays auditable against its own findings.
var possessiveNounRe = regexp.MustCompile(`(?i)\b(creature|target|weapon|caster|user|clone)s (armor class|adversary role|attack bonuses|attack bonus|attack roll|attack total|attack result|attack range|attack scores|temporary hit points|shocked condition|burned condition|chilled condition|critical threat range|chosen ability score|chosen striking preference|walking speed|maximum health|saving throws|saving throw|ninjutsu|ninshou|genjutsu|jutsu|level|ac|dr|choice|speed|rank|wisdom|strength|constitution|dexterity|intelligence|charisma|movements|movement|physical|passive|classification|joints|shadow|status|clan|mental|unarmed|weight|statistics|corpse|original|perception|current|highest|path|antics|essence|senses|thoughts|spirit|neck|brain|presence|physiology|armor|synaptic|effective|combat|reaction|ability|allies|ammunition|reach)\b`)

// possessivePluralContextRe guards possessiveNounRe against the cases where
// the head really is plural and so needs a trailing "s’" rather than "’s" —
// "all targets AC", "both casters unarmed damage", "a penalty to creatures
// saving throws". Those are genuinely plural possessives and a handful of
// them are ditransitive rather than possessive at all ("grant the affected
// creatures temporary hit points"), so they are excluded here and handled
// individually in curated.go instead of being guessed at by pattern.
var possessivePluralContextRe = regexp.MustCompile(`(?i)(?:\b(?:all|both|multiple|several|other|affected)\s+(?:[A-Za-z-]+\s+)?|\b(?:to|of|by|with|for|from|in|on|at|than|vs)\s+)$`)

// possessiveExtFix applies the two 2026-09-13 branches. Determiner-anchored
// matches run first; the apostrophe they insert is itself what keeps the
// noun-anchored pass from matching the same text twice.
func possessiveExtFix(s string) (string, []textDiff) {
	var diffs []textDiff

	fixed := possessiveDetRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := possessiveDetRe.FindStringSubmatch(m)
		det, adj, head, apos, gap, next := sub[1], sub[2], sub[3], sub[4], sub[5], sub[6]
		lower := strings.ToLower(next)
		// An -ly adverb after the head means the head itself is the
		// mis-pluralized word ("a Creatures currently under the effects"),
		// not a possessive with the apostrophe dropped. Those are corrected
		// individually in curated.go.
		if apos != "" || possessiveNotPossessed[lower] || (len(lower) > 4 && strings.HasSuffix(lower, "ly")) {
			return m
		}
		corrected := det + adj + head + "’s" + gap + next
		diffs = append(diffs, textDiff{tool: "possessive", original: m, corrected: corrected})
		return corrected
	})

	var b strings.Builder
	last := 0
	for _, loc := range possessiveNounRe.FindAllStringSubmatchIndex(fixed, -1) {
		if possessivePluralContextRe.MatchString(fixed[last:loc[0]]) {
			continue
		}
		head := fixed[loc[2]:loc[3]]
		tail := fixed[loc[4]:loc[5]]
		corrected := head + "’s " + tail
		diffs = append(diffs, textDiff{tool: "possessive", original: fixed[loc[0]:loc[1]], corrected: corrected})
		b.WriteString(fixed[last:loc[0]])
		b.WriteString(corrected)
		last = loc[1]
	}
	b.WriteString(fixed[last:])
	return b.String(), diffs
}

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

// --- abbrevsplit: a dotted abbreviation broken by the PDF extraction ------

// The sourcebook's dotted abbreviations (A.T for Air Trecks, B.I.M for Bomb
// Integrated Machine, E.I.P, S.E.N.T, A.N.T) are repeatedly extracted with a
// space eaten into them just before the final letter — "your A. Ts", "2 B.I.
// Ms into 1 B.I.M" — 96 instances corpus-wide (2026-09-13). The canonical
// rendering is a period between every letter, no trailing period, and a bare
// "s" for the plural, which is what the un-split occurrences already use
// (B.I.M 146, S.N.B 124, E.I.P 38, B.I.Ms 4, A.N.Ts 2).
//
// The stems and their tail letters are enumerated rather than matched
// generically, because the generic shape ("any dotted run, a space, a single
// capital") also matches an abbreviation that legitimately ends a sentence
// before a one-letter word — "...the B.I.M. A creature must..." — and would
// silently weld the two together.
var abbrevSplitRe = regexp.MustCompile(`\b(A|A\.N|B\.I|C\.C|E\.I|S\.E\.N|S\.N)\.[ ]+([A-Z])(s?)\b`)

// abbrevSplitTails is the letter each stem's full form actually ends in. A
// stem/tail pair outside this map is a coincidence, not a split abbreviation.
var abbrevSplitTails = map[string]string{
	"A": "T", "A.N": "T", "B.I": "M", "C.C": "D", "E.I": "P", "S.E.N": "T", "S.N": "B",
}

func abbrevSplitFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := abbrevSplitRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := abbrevSplitRe.FindStringSubmatch(m)
		stem, tail, plural := sub[1], sub[2], sub[3]
		if abbrevSplitTails[stem] != tail {
			return m
		}
		corrected := stem + "." + tail + plural
		diffs = append(diffs, textDiff{tool: "abbrevsplit", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}

// --- pluralverb: a plural subject given a singular verb -------------------

// The "Failure: Affected creatures gains 1 rank..." shape, repeated across
// the saving-throw outcome lines of several jutsu (8 instances, 2026-09-13).
// Scoped to the handful of plural head nouns the outcome lines actually use
// and to "gains" specifically: those lines are formulaic enough that the word
// before "gains" is always the subject, with no intervening clause to make
// the agreement ambiguous.
var pluralVerbRe = regexp.MustCompile(`\b([Cc]reatures|[Tt]argets|[Aa]llies|[Ee]nemies|[Cc]lones) gains\b`)

func pluralVerbFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := pluralVerbRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := pluralVerbRe.FindStringSubmatch(m)
		corrected := sub[1] + " gain"
		diffs = append(diffs, textDiff{tool: "pluralverb", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}

// --- wordconfusion: closed set of real-word confusions --------------------

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

// --- bonusplural / penaltyplural: number agreement ------------------------

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

// --- succeedon: "succeed a saving throw" -> "succeed on a saving throw" ---

// succeedOnRe matches the book's house phrasing for passing a roll.
// "Succeed" is intransitive in this sense — you succeed *on* a check, not
// *a* check — and the corpus is overwhelmingly consistent in getting it
// wrong: full-corpus audit 2026-09-13 found 257 occurrences of the
// prepositionless form against 52 correct "succeeds on a", across every
// table with prose. A systemic authoring habit, not 257 unrelated slips.
//
// Scoped by requiring an actual roll noun (saving throw / save / check)
// after the determiner, which is what keeps it off the genuinely correct
// intransitive uses the same audit turned up — "succeed by 5 or more",
// "succeeds and is reset", "choose to succeed instead", "automatically
// succeeds if it is immune". Up to four words are allowed between the
// determiner and the roll noun so compound abilities still match
// ("succeed a Wisdom or Intelligence saving throw").
var succeedOnRe = regexp.MustCompile(`\b([Ss]ucceed|[Ss]ucceeds|[Ss]ucceeded|[Ss]ucceeding) ((?:a|an|the|its|their|your|his|her|this|these|3|three) (?:\w+ ){0,4}?(?:[Ss]aving [Tt]hrows?|[Ss]aves?|[Cc]hecks?))\b`)

// succeedOnFix inserts the missing preposition, leaving the verb's tense
// and the determiner exactly as found.
func succeedOnFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := succeedOnRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := succeedOnRe.FindStringSubmatch(m)
		corrected := sub[1] + " on " + sub[2]
		diffs = append(diffs, textDiff{tool: "succeedon", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}

// --- straycaps: game-term homograph used as an ordinary word --------------

// Stray capitalization: a game term that is also an ordinary English word,
// left capitalized by the PDF's authoring in a position where it is plainly
// the ordinary word, not the term of art. "You form bullets ... and Fire
// them off" is the verb "fire," not the Fire element; "When you Hit a
// creature" is the verb "hit," not Hit Points.
//
// This matters beyond English typography. internal/i18n's glossary check
// pins element names case-sensitively (see sourceTermRe), so a capitalized
// verb reads to the checker as an unrendered mechanic and produces a
// finding against a translation that was in fact correct — the Portuguese
// for these rows says "dispara," which is right. Correcting the source is
// the fix; suppressing the finding would have hidden real drift too.
//
// The rules below are deliberately narrow. Each requires BOTH a preceding
// word that can license a verb AND a following complement that only a verb
// takes, because the overwhelming majority of capitalized "Fire" in this
// corpus is the legitimate element ("resistance to Fire damage", "Wind or
// Fire Release", "Fire Bombs") and must not be touched. Full-corpus audit
// 2026-09-13: 11 verb-position occurrences of Fire/Fires against 400+
// legitimate element uses, and 2 of Hit against 600+ legitimate "Hit
// Points" uses.

// fireVerbRe matches capitalized Fire/Fires in a transitive or particle
// verb slot — "and Fire them off", "you can Fire up to three beams", "to
// Fire this wave again". The lookahead complement set is what separates it
// from the element: "to Fire damage" and "or Fire Release" continue with a
// noun the set excludes.
var fireVerbRe = regexp.MustCompile(`\b(to|and|then|can|may|must|will|would|could|you|they|it|the)(\s+)(Fire|Fires)\b(?:\s+(?:them|it|this|these|those|off|out|outward|outwards|up|again|away|back|down|in|into|at|toward|towards|as|a|an)\b)`)

// fireVerbEndRe covers the clause-final case a complement set cannot reach:
// "up to the number of beams you Fire." A pronoun subject immediately
// before sentence-ending punctuation leaves no reading where Fire is a noun.
var fireVerbEndRe = regexp.MustCompile(`\b(you|they|it)(\s+)(Fire|Fires)(?:[.,;])`)

// hitVerbRe matches capitalized Hit as a verb. "Hit Points"/"Hit Die"/"Hit
// Dice" are the real term and are excluded by the complement set, which
// admits only articles and the "with" of "a creature you Hit with".
var hitVerbRe = regexp.MustCompile(`\b(you|they|it|creature|target)(\s+)(Hit|Hits)\b(?:\s+(?:a|an|the|with|it|them|this)\b)`)

var strayCapsRules = []*regexp.Regexp{fireVerbRe, fireVerbEndRe, hitVerbRe}

// strayCapsFix lowercases a game-term homograph used as an ordinary verb.
// Local, instant, safe to always run. Only the term itself is rewritten;
// the surrounding words the pattern matched are put back untouched, so a
// rule can be anchored on context without rewriting it.
func strayCapsFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := s
	for _, re := range strayCapsRules {
		fixed = re.ReplaceAllStringFunc(fixed, func(m string) string {
			sub := re.FindStringSubmatch(m)
			lead, gap, term := sub[1], sub[2], sub[3]
			lowered := string(term[0]|0x20) + term[1:]
			corrected := lead + gap + lowered + m[len(lead)+len(gap)+len(term):]
			diffs = append(diffs, textDiff{tool: "straycaps", original: m, corrected: corrected})
			return corrected
		})
	}
	return fixed, diffs
}

// --- commacaps: ordinary English word capitalized after a comma -----------

// The sourcebook routinely starts a mid-sentence clause with a capital after
// a comma — "Beginning at 6th level, Your body has been strengthened",
// "Select a Construct or Structure within range, Make a Ranged Taijutsu
// Attack", "On a failed save, until the end of their next turn, If the
// affected creature attempts any action". The comma is right; the capital is
// not. Full-corpus audit 2026-09-13 found 153 occurrences across every table
// with prose.
//
// The rule is an explicit vocabulary rather than a general "lowercase
// anything after a comma", because most capitals in that position are
// legitimate: ", B-Rank", ", Ninjutsu", ", AC", ", Grappled", ", Genjutsu"
// are the book's real terms of art and lowercasing them would be a new bug.
// Every word below was checked against the full corpus for a term-of-art
// reading and has none.
//
// Two words that look like they belong here and deliberately do not:
//
//   - "Within", which the book uses to open each entry of a parallel list
//     ("Within the past day: 12, Within the past week: 16"). Lowercasing the
//     entries after the first would make the list internally inconsistent.
//   - "For", whose only two occurrences follow a stray ".," sequence that
//     needs its own correction, not a case change.
var commaCapsRe = regexp.MustCompile(`(,\s+)(You|Your|Yours|Select|Choose|When|As|If|On|Once|This|These|Those|They|Their|Them|The|All|Make|May|Might|Must|Can|Could|Should|Would|Half|Failed|Gain|Gains|Gaining|Increase|Increases|Reduce|Reduces|While|After|Before|Each|Also|However|Additionally|Creature|Creatures|It|Its|Instead|Otherwise|Then|There)\b`)

func commaCapsFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := commaCapsRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := commaCapsRe.FindStringSubmatch(m)
		gap, word := sub[1], sub[2]
		corrected := gap + string(word[0]|0x20) + word[1:]
		diffs = append(diffs, textDiff{tool: "commacaps", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}

// --- midcaps: ordinary English word capitalized mid-clause ----------------

// The same authoring habit as commacaps, without a comma to anchor on: an
// ordinary word left capitalized in the middle of a clause. "On a Failed
// save", "they take Half.", "advantage on checks to Stabilize a creature",
// "this seal Gains +2 Damage reduction", "you can Increase your walking
// speed". Full-corpus audit 2026-09-13.
//
// Anchored on a preceding lowercase word, which is what establishes the
// position as mid-clause rather than sentence-initial. A bare "o" does not
// count: the corpus renders one flavour of bullet that way, so the word
// after it genuinely starts a list item and its capital is correct.
//
// Narrower vocabulary than commacaps, because without a comma the risk of
// catching a term of art is higher. "Reduce" and "Interrupt" were audited
// and left out on purpose — the book also uses each as a real noun mechanic
// ("spend them on the Amplify or Reduce features", "this jutsu counts as an
// Interrupt"), so their genuine verb uses are curated entries instead.
//
// The trailing group exists only so "Half Moon", a named benefit, can be
// told apart from the ordinary "half" this rule is after; it is put back
// untouched either way.
var midCapsRe = regexp.MustCompile(`\b([a-z][a-z'’-]*)(\s+)(Failed|Half|Make|Makes|May|Stabilize|Suffer|Suffers|Select|Choose|Gain|Gains|Increase|Increases)\b(\s+[A-Za-z]+)?`)

func midCapsFix(s string) (string, []textDiff) {
	var diffs []textDiff
	fixed := midCapsRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := midCapsRe.FindStringSubmatch(m)
		lead, gap, word, tail := sub[1], sub[2], sub[3], sub[4]
		if lead == "o" || (word == "Half" && strings.TrimSpace(tail) == "Moon") {
			return m
		}
		corrected := lead + gap + string(word[0]|0x20) + word[1:] + tail
		diffs = append(diffs, textDiff{tool: "midcaps", original: m, corrected: corrected})
		return corrected
	})
	return fixed, diffs
}
