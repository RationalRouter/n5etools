// Package textentries finds named sub-entries bundled into one run-on prose
// field — the shape produced when a sourcebook PDF's text layer loses all
// bold/line-break formatting, so a printed list like "Bold Name. Body text.
// Next Name. More body text." (or an ALL-CAPS header variant) collapses into
// one flat sentence. Extracted out of cmd/n5e/format.go, which renders these
// matches to HTML for the sheet's read-only description text; this package
// is the pure, render-agnostic byte-offset detection underneath that
// rendering — reused at ingest time (see internal/store/classoptionentries.go)
// to split a bundled class_options tier field into real per-upgrade rows,
// where there is no HTML to produce at all.
package textentries

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// namedEntryPattern finds the book's unmarked list shape (see e.g. Weapon
// Specialist's Gungnir Piercer Styles): instead of "•" bullets, a run of
// named sub-entries printed as "Bold Name. Body text. Next Name. More body
// text." with no marker at all — the boldness (visible in the book, lost by
// plain-text PDF extraction the same way bullets are) is the only thing
// that told a reader "Bold Name" starts. Detected as a short Title-Case
// phrase (1-4 words, each starting with a plain capital letter) sitting
// right at a sentence boundary and immediately followed by its own ". "
// before the next real sentence starts.
//
// This is a much weaker signal than the literal "•" character, so callers
// only trust it when the SAME field contains two or more such matches — one
// isolated "Bonus Action." mid-paragraph (a real, common false-positive
// shape: capitalized game terms ending a clause) doesn't repeat, but a
// genuine named-entry list always does.
//
// RE2 has no lookahead, so the capital letter that starts the following
// sentence is captured (group 2) rather than merely asserted; callers back
// up one byte into it (see FindEntries' BodyStart comment).
var namedEntryPattern = regexp.MustCompile(`(?:^|[.!?]\)? )(\p{Lu}[\p{L}’'-]*(?: \p{Lu}[\p{L}’'-]*){0,3})\. ([A-Z])`)

// weaponStatBlockSelfReference catches a real false-positive shape for
// namedEntryPattern: a "natural weapon" upgrade (e.g. Puppet Master's
// Chakra Blast, Power Fist) restates its own name right before its own
// D&D-style attack stat block — "...natural weapon, Chakra Blast. This
// weapon counts as... Chakra Blast. Ranged Weapon Attack: Ninjutsu or
// Taijutsu attack bonus to hit...". "Chakra Blast. Ranged" reads exactly
// like namedEntryPattern's own "Name. Capital" shape, splitting the ONE
// real upgrade into two: a truncated original that loses its own stat
// block, and a nameless duplicate holding just the stat block (confirmed
// live: rules.db shipped two separate "Chakra Blast" rows and the sheet
// showed the same tile name twice). Matched against the text starting
// right at the captured next-sentence letter, so it only fires when THIS
// specific match's own body is the stat block opener, not anywhere else in
// the entry.
var weaponStatBlockSelfReference = regexp.MustCompile(`^(?:Ranged|Melee) Weapon Attack:`)

// namedEntryStoplist rules out common sentence-connector words that
// legitimately start a short standalone sentence ("...before deactivating.
// Additionally. You can spend..." — a real run found in the source text) but
// are never a book's name for something. Deliberately small and closed —
// extend it if a future sweep turns up another repeat offender, rather than
// trying to solve this with general-purpose NLP.
var namedEntryStoplist = map[string]bool{
	"additionally": true, "however": true, "themselves": true, "also": true,
	"furthermore": true, "moreover": true, "meanwhile": true, "nevertheless": true,
	"therefore": true, "otherwise": true, "regardless": true, "alternatively": true,
	"similarly": true, "consequently": true, "finally": true, "then": true,
	"thus": true, "instead": true, "afterward": true, "afterwards": true,
	"period": true, // rhetorical emphasis ("...no one works as hard as you. Period."), not a name
}

// capsEntryPattern finds a second, distinct named-entry shape (see e.g.
// Puppet Master's Puppeteer Chassis or the Science-Nin gadget upgrade
// lists): an ALL-CAPS header (1-5 words, e.g. "JET PROPULSION LEGS",
// "SPECIALIZED") immediately followed by a stat-line keyword ending in a
// colon — "ASI:", "Cost:", "Drain:", "Techniques:" all appear in the real
// data, so the keyword itself is matched generically (any capitalized word
// then ":") rather than an enumerated list, so a keyword not yet seen still
// works. Everything from the keyword onward is the entry's body — the
// keyword is real content ("Cost: 8 Creation Points"), not part of the
// name.
//
// The optional leading "(?:\p{Lu}[\p{Lu}\-]+: )?" handles a real sub-shape
// found in Puppet Master's Armory upgrades: a category label glued directly
// to its own colon before the actual name, e.g. "ARMORY: FIRE AND WATER
// BLASTER Techniques: All, Perfect" — without it, "ARMORY:" (no space
// before its colon) doesn't match the space-then-keyword shape at all and
// the whole entry silently fails to split out.
//
// Confirmed directly against a photo of the printed book page that these
// names are NOT actually all-caps in the book — they're bold, ordinary
// mixed case; the PDF's font for this specific header style apparently maps
// every glyph to an uppercase codepoint regardless of visual case (the same
// class of extraction quirk as the missing bold/line-break information
// itself), so the raw database text really is all-caps. See TitleCase,
// applied to these names only (not namedEntryPattern's, which extract with
// their real case intact) when rendering.
//
// The optional "(?:[AI] )?" right before the main caps run (outside the
// captured name — see below) handles a heading that itself starts with the
// single-letter word "A" or "I" (see e.g. Puppet Master's Black Iron
// Upgrades: "...Entrapment Mechanism. A THOUSAND HANDS MANIPULATION FORCE
// Techniques: Black..."). Without it, the pattern silently fails to match at
// that position at all — "A" is only one letter, one short of the
// two-letter minimum every other word in the caps run requires
// (\p{Lu}[\p{Lu}\-’']+), so the anchor right after the previous sentence's
// period never lines up with the real heading, and the entire entry gets
// swallowed into whatever the previous match's body happened to be
// (confirmed: this glued "Triple Iron Maiden"'s body onto "Thousand Hands
// Manipulation Force" in the shipped data). "A"/"I" are the only two
// single-letter words in English that can legitimately start a sentence, so
// this stays a tight, low-false-positive-risk addition rather than a general
// short-word allowance — an ordinary sentence starting "A character..." or
// "A DM..." still won't match, since nothing here relaxes the requirement
// that the very next word also be a genuine 2+-letter all-caps run.
// Deliberately kept OUTSIDE the name-capturing group (matching how the book
// actually prints/is referenced elsewhere) rather than folded into the
// entry's stored name — plausibleEntryName's own per-word length guard
// would otherwise reject the name outright, since "A"/"I" alone are exactly
// the single-letter shape it exists to catch.
//
// Two more anchor gaps found the same way, both in Puppet Master's
// Armorer's Upgrades / Gold Tier: the "[.!?;] " anchor required the
// sentence-ending punctuation to be IMMEDIATELY followed by a space, but
// "...armor remains intact.) INFILTRATOR Techniques: ..." has a closing
// paren sitting between them — the anchor never matched, and "Infiltrator"'s
// whole entry got swallowed into "Full Metal Jacket"'s body instead. The
// optional "\)?" right after the punctuation class fixes it the same way
// the "A"/"I" case was fixed: widen the anchor, not the name. Separately,
// "G30" (a real book heading, a codename rather than a typo) has a digit in
// it — \p{Lu} only covers letters, so the name-run classes below now also
// accept \d, or a name containing any digit fails to match at all and
// swallows into whatever came before it (there: "Fortified Defenses").
//
// A fourth anchor gap, found in Puppeteer Chassis (Black Technique): a
// parenthetical clarifying list closes out its OWN sentence with no period
// before the closing paren at all — "...(Bludgeoning=Bruised /
// Piercing=Weakened / Slashing=Bleeding) QUADRUPEDAL ASI: ..." — so neither
// the bare "[.!?;]" nor its "\)?" suffix (which still requires a preceding
// [.!?;]) matched, and "Quadrupedal"'s whole entry swallowed into
// "Specialized"'s body. Widened to also accept a bare ")" with no
// preceding sentence-ending punctuation. This looks riskier than the
// previous two anchor widenings (a mid-sentence parenthetical is common,
// and would wrongly split there too) but capsEntryPattern's own required
// suffix — an ALL-CAPS multi-word run immediately followed by a
// "Keyword:" — makes a false trigger vanishingly unlikely: no ordinary
// mid-sentence parenthetical is ever followed by that exact shape.
//
// A fifth gap, found across Puppet Master's Interwoven/Upgrades of War/
// Magus Upgrades lists: several names carry their OWN trailing
// parenthetical annotation before the "Keyword:" suffix — "LETHAL STRINGS
// (NEW!) Techniques: White...", "IMPROVED ARCHITECTURE II (BLUE)
// Techniques: Blue...". Without an allowance for it, the caps-run match
// stops right before the "(", the required "Keyword:" never follows
// immediately, and the whole entry silently vanishes into the previous
// one's body — the same failure shape as every gap above, just triggered
// by a trailing annotation instead of a leading anchor. Fixed the same way
// capsProseEntryPattern already allows one trailing parenthetical
// ("(RENAMED)") after its own name run: an optional
// "(\p{Lu}...)" group captured as PART of the name (not stripped), since
// other entries reference these exact names — "(BLUE)" included — as their
// own "Prerequisite:" text (confirmed: "Improved Architecture I (Blue)").
// Widened past capsProseEntryPattern's own character class to also accept
// "!" inside the parenthetical, for "(NEW!)".
var capsEntryPattern = regexp.MustCompile(`(?:^|(?:[.!?;]\)?|\)) )(?:[AI] )?((?:\p{Lu}[\p{Lu}\d\-’']+: )?\p{Lu}[\p{Lu}\d\-’']+(?: \p{Lu}[\p{Lu}\d\-’']+){0,4}(?: \(\p{Lu}[\p{Lu}\d\-’'!]*\))?) (\p{Lu}[\p{L}]*:)`)

// tableCaptionWord catches a real shape found in Puppet Master's Armorer's
// Upgrades: a data table's own caption ("ELEMENTAL REACTOR TABLE") sitting
// immediately before the next real entry's name ("FARADAY FACEPLATE
// Techniques: ..."), with no sentence-ending punctuation between them for
// capsEntryPattern's own anchor to split on — both run together as one
// caps-word run within the pattern's 5-word budget, so the caption got
// captured as part of the next entry's stored name ("Elemental Reactor
// Table Faraday Faceplate"), the same "no separator, so the boundary logic
// swallows two headers as one" root shape as the earlier "A"/"I" and
// trailing-")" anchor gaps this pattern already documents fixes for. A
// table's caption is never itself a real entry name in this corpus (checked:
// no other entry ends in the word "Table"), and its own tabular data isn't
// extractable PDF text at all (same "image-table" shape as Armor Chassis,
// hand-transcribed separately) — so when TABLE appears mid-run, everything
// up through it is dropped and only the real name after it is kept.
var tableCaptionWord = regexp.MustCompile(`\bTABLE\b`)

// capsProseEntryPattern finds a third named-entry shape (see e.g. Puppet
// Master's Purple Technique ~ Juggernaut: Unique Armor Properties, or
// Science-Nin's Air Treck Regalia options): the same all-caps-header
// extraction artifact as capsEntryPattern, but running straight into
// ordinary prose with no "Keyword:" stat line to anchor on at all —
// "ATHLETIC Armor with the Athletic property grants a +1d4 bonus..." An
// optional trailing parenthetical annotation is supported for entries like
// "STURDY (RENAMED)".
//
// Any match whose body word is itself immediately followed by ':' is
// skipped in FindEntries — that's capsEntryPattern's shape claiming the
// exact same starting position (e.g. "SALAMANDER Techniques: Black..."),
// and letting both patterns produce an entry there would double it up,
// since FindEntries' overlap-based dedup assumes distinct starting
// positions rather than two matches at the identical spot.
//
// Carries the same optional leading "(?:[AI] )?" as capsEntryPattern
// (outside the captured name, for the same plausibleEntryName reason), and
// for the same root cause — a heading genuinely starting with "A"/"I"
// otherwise breaks the anchor and swallows the whole entry into the
// previous one's body. Also carries the same trailing-")"-before-the-space
// anchor tolerance and digit-in-name allowance as capsEntryPattern, for the
// identical reasons — see that pattern's own doc for both.
var capsProseEntryPattern = regexp.MustCompile(`(?:^|[.!?;]\)? )(?:[AI] )?(\p{Lu}[\p{Lu}\d\-’']+(?: \p{Lu}[\p{Lu}\d\-’']+){0,4}(?: \(\p{Lu}[\p{Lu}\d\-’']*\))?) (\p{Lu}\p{Ll}[\p{L}’'-]*)`)

// titleCaseSmallWords stays lowercase mid-phrase when converting a caps
// entry's all-caps display artifact back to normal title case (see
// capsEntryPattern) — a short, closed list rather than a general-purpose
// title-casing library, since this only ever runs on short 1-5 word book
// item names.
var titleCaseSmallWords = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true, "but": true,
	"by": true, "for": true, "in": true, "nor": true, "of": true, "on": true,
	"or": true, "so": true, "the": true, "to": true, "up": true, "yet": true,
}

// romanNumeralWord matches a bare Roman numeral token ("II", "III", "IIII")
// — this corpus's own style for a repeated-upgrade suffix ("IMPROVED
// ARCHITECTURE II (BLUE)"), which TitleCase must leave fully capitalized
// rather than folding to "Ii"/"Iii" the way its own generic one-letter-
// capitalized rule would otherwise produce. Deliberately narrow (only the
// letter "I" repeated) rather than a general Roman-numeral matcher — this
// corpus doesn't use V/X/etc. in any name split so far, and a broader
// matcher risks miscasing a genuine word that happens to be built from
// "I"/"V"/"X" letters alone.
var romanNumeralWord = regexp.MustCompile(`^[Ii]+$`)

// TitleCase renders an all-caps name (see capsEntryPattern's doc) in normal
// title case: first LETTER of each word — and each hyphen-joined piece of a
// word, so "SELF-DESTRUCT" becomes "Self-Destruct" not "Self-destruct" —
// capitalized, the rest lowercased, except titleCaseSmallWords' words after
// the first stay lowercase and romanNumeralWord's words stay fully
// capitalized. A word ending in its own colon (the "ARMORY: " label
// prefix) keeps that colon; only its letters change case. The first LETTER
// (not rune) is capitalized so a word with leading punctuation, like
// capsProseEntryPattern's "(RENAMED)" parenthetical, comes out "(Renamed)"
// rather than leaving the '(' untouched and the real first letter lowercase.
func TitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if i > 0 && titleCaseSmallWords[strings.ToLower(w)] {
			words[i] = strings.ToLower(w)
			continue
		}
		if romanNumeralWord.MatchString(w) {
			words[i] = strings.ToUpper(w)
			continue
		}
		pieces := strings.Split(strings.ToLower(w), "-")
		for j, p := range pieces {
			r := []rune(p)
			for k, c := range r {
				if unicode.IsLetter(c) {
					r[k] = unicode.ToUpper(c)
					break
				}
			}
			pieces[j] = string(r)
		}
		words[i] = strings.Join(pieces, "-")
	}
	return strings.Join(words, " ")
}

// plausibleEntryName rejects two known false-positive shapes found by
// sweeping every prose field in the live database before shipping this
// pattern: connector words (see namedEntryStoplist) and abbreviations like
// "A.T." printed in the book as "A. T" — a bare single-letter word is never
// a real book name for something, but "Your A. Ts now deal..." repeats often
// enough in that one subclass to otherwise clear the 2-match threshold.
func plausibleEntryName(name string) bool {
	if namedEntryStoplist[strings.ToLower(name)] {
		return false
	}
	for _, word := range strings.Fields(name) {
		if len([]rune(word)) < 2 {
			return false
		}
	}
	return true
}

// EntryMatch is namedEntryPattern's and capsEntryPattern's matches
// normalized to one shape — byte offsets into the original raw string, not
// rendered output — so callers don't need to know which pattern found which
// entry except for Kind, which says how the name should be displayed/cased.
type EntryMatch struct {
	FullStart, NameStart, NameEnd, BodyStart int
	Kind                                     EntryKind
}

type EntryKind int

const (
	EntryKindTitle EntryKind = iota // namedEntryPattern: real mixed case, rendered as-is
	EntryKindCaps                   // capsEntryPattern/capsProseEntryPattern: all-caps extraction artifact, needs TitleCase
)

// FindEntries runs all three named-entry patterns over raw, keeps only
// plausible names (see plausibleEntryName), and returns them merged in
// document order. Two matches starting at (almost) the same position — not
// observed in practice, since the patterns look for different boundary
// shapes, but cheap to guard — are deduplicated by keeping whichever starts
// first.
func FindEntries(raw string) []EntryMatch {
	var entries []EntryMatch
	for _, loc := range namedEntryPattern.FindAllStringSubmatchIndex(raw, -1) {
		bodyStart := loc[5] - 1 // back up into the captured capital letter (see namedEntryPattern doc)
		if weaponStatBlockSelfReference.MatchString(raw[bodyStart:]) {
			continue // see that pattern's own doc — not a second entry at all
		}
		if plausibleEntryName(raw[loc[2]:loc[3]]) {
			entries = append(entries, EntryMatch{
				FullStart: loc[0], NameStart: loc[2], NameEnd: loc[3],
				BodyStart: bodyStart,
				Kind:      EntryKindTitle,
			})
		}
	}
	for _, loc := range capsEntryPattern.FindAllStringSubmatchIndex(raw, -1) {
		nameStart, nameEnd := loc[2], loc[3]
		if capIdx := tableCaptionWord.FindAllStringIndex(raw[nameStart:nameEnd], -1); len(capIdx) > 0 {
			last := capIdx[len(capIdx)-1]
			if last[1] < nameEnd-nameStart { // TABLE isn't the run's own final word
				nameStart += last[1]
				for nameStart < nameEnd && raw[nameStart] == ' ' {
					nameStart++
				}
			}
		}
		if plausibleEntryName(raw[nameStart:nameEnd]) {
			entries = append(entries, EntryMatch{
				FullStart: loc[0], NameStart: nameStart, NameEnd: nameEnd,
				BodyStart: loc[4], // the keyword ("Cost:", "ASI:", ...) starts the body, unlike namedEntryPattern
				Kind:      EntryKindCaps,
			})
		}
	}
	for _, loc := range capsProseEntryPattern.FindAllStringSubmatchIndex(raw, -1) {
		if loc[5] < len(raw) && raw[loc[5]] == ':' {
			continue // capsEntryPattern's shape claims this position instead, see capsProseEntryPattern's doc
		}
		if plausibleEntryName(raw[loc[2]:loc[3]]) {
			entries = append(entries, EntryMatch{
				FullStart: loc[0], NameStart: loc[2], NameEnd: loc[3],
				BodyStart: loc[4], // the first prose word starts the body, there is no keyword to include
				Kind:      EntryKindCaps,
			})
		}
	}
	sort.Slice(entries, func(i, k int) bool { return entries[i].FullStart < entries[k].FullStart })

	deduped := entries[:0]
	for i, e := range entries {
		if i > 0 && e.FullStart < deduped[len(deduped)-1].BodyStart {
			continue // overlaps the previous match; keep the earlier one
		}
		deduped = append(deduped, e)
	}
	return deduped
}
