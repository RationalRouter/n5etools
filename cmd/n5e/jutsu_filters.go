package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// castingActionBucket collapses the book's free-text casting_time field
// (174 distinct raw strings — "1 Action", "1Action" with no space, "1
// Reaction, which you take when you would take damage.", "Full Turn
// Action", "1 Minute", ...) into the 4 buckets a player actually filters
// by. Checked with Contains rather than a word-boundary regex specifically
// because "1Action" (no space, 2 real rows) would fail a \baction\b match —
// there's no word-boundary between the digit and the letter — but Contains
// still finds it. "Full Turn Action" is deliberately bucketed as Action
// (it still costs your action-type resource, just all of it) rather than
// invented as a 5th bucket the caller never asked for. Anything that names
// none of the three real action types (rare non-turn casting times like "1
// Minute"/"10 Minutes"/"1 Hour", plus the book's own "Special") falls into
// Special, the natural catch-all.
func castingActionBucket(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "bonus action"):
		return "Bonus Action"
	case strings.Contains(lower, "reaction"):
		return "Reaction"
	case strings.Contains(lower, "action"):
		return "Action"
	default:
		return "Special"
	}
}

// castingActionOrder fixes the display order of the 4 casting-action
// buckets; anything unrecognized (there shouldn't be any — see
// castingActionBucket) sorts last rather than panicking a caller that
// forgets to extend this map.
var castingActionOrder = map[string]int{"Action": 0, "Bonus Action": 1, "Reaction": 2, "Special": 3}

// durationLengthPattern matches a plain "<N> <unit>[s]" duration phrase
// after any "Concentration, up to" prefix has been stripped — e.g. "1
// minute", "10 Minutes", "up to 1 Hour" (the leading "up to" is consumed
// but not captured).
var durationLengthPattern = regexp.MustCompile(`(?:up to )?(\d+)\s*(round|minute|hour|day|week|month|year)s?\b`)

// isConcentrationDuration reports whether a jutsu's raw duration text opens
// with "Concentration" (any case/whitespace) — the one place in the free-text
// duration field that carries real mechanical meaning (see the character
// sheet's concentration tracking), as opposed to the length bucketing
// durationBucket does below, which treats concentration as pure flavor text.
func isConcentrationDuration(duration string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(duration)), "concentration")
}

// durationBucket collapses the book's free-text duration field (90 distinct
// raw strings once you count every capitalization/punctuation variant of
// "Concentration, up to 1 minute") into one canonical label per underlying
// length — "Concentration, Up to 1 Minute." and "concentration, up to 1
// minute" both become "1 Minute". Concentration itself is intentionally not
// its own filter axis: the ask was to bucket by duration length ("Instant,
// 1 minute, 10 minutes, etc"), and a jutsu's raw duration text (shown on
// its detail page) still says "Concentration, ..." in full regardless of
// which bucket it's filed under.
func durationBucket(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".")
	lower := strings.ToLower(s)

	if isConcentrationDuration(lower) {
		rest := strings.TrimLeft(lower[len("concentration"):], ", -")
		lower = strings.TrimSpace(rest)
	}

	switch {
	case strings.HasPrefix(lower, "instant"): // "instant", "instantaneous"
		return "Instant"
	case strings.HasPrefix(lower, "permanent"):
		return "Permanent"
	case strings.Contains(lower, "until dispelled"):
		return "Until Dispelled"
	case strings.Contains(lower, "until short rest"):
		return "Until Short Rest"
	case strings.Contains(lower, "until long rest"):
		return "Until Long Rest"
	}

	if m := durationLengthPattern.FindStringSubmatch(lower); m != nil {
		n, err := strconv.Atoi(m[1])
		if err == nil {
			unit := m[2]
			unit = strings.ToUpper(unit[:1]) + unit[1:]
			if n != 1 {
				unit += "s"
			}
			return fmt.Sprintf("%d %s", n, unit)
		}
	}

	return "Special" // "Special" itself, plus anything else this doesn't recognize
}

// durationOrder fixes the display order of the recognized duration
// buckets, shortest to longest, with the open-ended/non-numeric buckets
// last. Any bucket not listed here (there shouldn't be any beyond real book
// duration lengths — see durationBucket) sorts after everything else.
var durationOrder = map[string]int{
	"Instant": 0, "1 Round": 1, "1 Minute": 2, "10 Minutes": 3, "1 Hour": 4,
	"8 Hours": 5, "24 Hours": 6, "1 Day": 7, "10 Days": 8, "1 Year": 9,
	"Until Short Rest": 10, "Until Long Rest": 11, "Until Dispelled": 12,
	"Permanent": 13, "Special": 14,
}

// mileRangePattern and feetRangePattern pull the PRIMARY distance out of a
// range string — the number that appears before any parenthetical
// area-of-effect size, e.g. the 120 in "120 Feet (30-foot-cube)", not the
// 30. Both patterns are applied with FindStringSubmatch, which always
// returns the first match, so this falls out for free rather than needing
// special handling.
var (
	mileRangePattern = regexp.MustCompile(`(\d+)\s*[-\s]?mile`)
	feetRangePattern = regexp.MustCompile(`(\d+)\s*[-\s]?(?:feet|foot|ft)\b`)
)

const feetPerMile = 5280

// jutsuRange is one jutsu's parsed range: either a real distance in feet
// (Numeric true, e.g. Self/Touch both parse to 0 feet, "1 Mile" to 5280),
// or one of the book's few non-distance ranges (Weapon Range, Movement
// Speed, Special) that a feet-based slider can't meaningfully place on its
// own scale.
type jutsuRange struct {
	Feet    int
	Numeric bool
	Special string // set only when !Numeric — the filter-panel bucket label
}

// parseJutsuRange classifies the book's free-text range field (over 250
// distinct raw strings once every capitalization/spacing/typo variant is
// counted — "60 Feet" vs "60 feet" vs "60ft" vs "60-feet", "Self (30-foot
// cone)" vs "self (30 Foot Cone)", ...). "Self"/"Touch" are checked first
// via HasPrefix specifically so a trailing area-of-effect distance in
// parens (e.g. "Self (120-foot line)") is never mistaken for the jutsu's
// own range — the character's range there really is 0 feet, the 120 feet
// describes the area the effect fills, a different quantity entirely.
// "Weapon"/"Movement"/"Special" are the book's few genuinely non-numeric
// ranges (a weapon's range varies by weapon, "Movement Speed" varies by
// character) — Special is also the fallback for anything unrecognized
// (rare parse-order glitches like a "1 Action" value that leaked in from
// the wrong column), same "flag, don't invent" spirit as the rest of this
// codebase's real book-data gaps.
func parseJutsuRange(raw string) jutsuRange {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(lower, "self"):
		return jutsuRange{Feet: 0, Numeric: true}
	case strings.HasPrefix(lower, "touch"):
		return jutsuRange{Feet: 0, Numeric: true}
	case strings.Contains(lower, "weapon"):
		return jutsuRange{Special: "Weapon Range"}
	case strings.Contains(lower, "movement"):
		return jutsuRange{Special: "Movement Speed"}
	case strings.HasPrefix(lower, "special"):
		return jutsuRange{Special: "Special"}
	}

	if m := mileRangePattern.FindStringSubmatch(lower); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return jutsuRange{Feet: n * feetPerMile, Numeric: true}
		}
	}
	if m := feetRangePattern.FindStringSubmatch(lower); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return jutsuRange{Feet: n, Numeric: true}
		}
	}
	return jutsuRange{Special: "Special"}
}

// componentCodes and componentNames are the 6 real component-code
// abbreviations printed throughout the jutsu compendium and defined in the
// core book's jutsu-creation chapter (Hand Seals/Chakra Molding/Chakra
// Seals/Mobility/Weapon/Ninja Tools, core book p.84-85, 90-91) — a fixed,
// closed set, so an unrecognized leading token (there shouldn't be any) is
// silently dropped rather than invented as a new filter category.
var componentNames = map[string]string{
	"HS": "Hand Seals",
	"CM": "Chakra Molding",
	"CS": "Chakra Seals",
	"M":  "Mobility",
	"W":  "Weapon",
	"NT": "Ninja Tools",
}

var componentCodePattern = regexp.MustCompile(`^([A-Za-z]{1,2})\b`)

// componentCodes extracts which of the 6 known component codes appear in a
// jutsu's raw components field, e.g. "HS, CM, W(any)" -> ["HS","CM","W"],
// "NT (Poison Kit, 1 Charges)" -> ["NT"]. Splits on commas that sit outside
// any parenthesis, not every comma in the string — a naive strings.Split
// would wrongly cut "NT (Poison Kit, 1 Charges)" into two tokens on the
// comma inside the parenthetical detail.
func componentCodes(raw string) []string {
	var codes []string
	depth := 0
	start := 0
	raw = strings.TrimSuffix(strings.TrimSpace(raw), ".")
	emit := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		m := componentCodePattern.FindStringSubmatch(token)
		if m == nil {
			return
		}
		code := strings.ToUpper(m[1])
		if _, ok := componentNames[code]; ok {
			codes = append(codes, code)
		}
	}
	for i, r := range raw {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				emit(raw[start:i])
				start = i + 1
			}
		}
	}
	emit(raw[start:])
	return codes
}
