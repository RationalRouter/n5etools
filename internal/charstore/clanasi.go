package charstore

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// This file turns a clan's ability-score increase into something a player
// can actually be asked about.
//
// 29 of the 45 clans grant fixed increases and were parsed into
// clan_ability_increases at ingest time ("+2 Intelligence, +1 Wisdom").
// The other 16 offer a choice — "+2 Str or Dex, +1 Con", "+2 Wis or Cha, +1
// Int" — and the ingest had nowhere to put a choice, so it wrote no rows for
// them at all. SetClan read only that table, so those 16 clans silently
// granted a character NOTHING: the bonus never appeared on the sheet and the
// player was never asked which option they wanted: clan bonuses to stats
// were not added/computed during character creation for these 16 clans.
//
// The choice text survives verbatim in clans.ability_increase_text, so this
// parses that column into pick-slots and the clan step renders a dropdown
// per slot. Same shape as ToolkitChoiceCount above: the book's prose is the
// only place the rule exists, so the prose is what gets read.

// AbilitySlot is one ability-score increase a clan grants. Options holds
// every ability the slot may be spent on, as 3-letter codes; a slot with
// exactly one option is a fixed grant and needs no dropdown.
type AbilitySlot struct {
	Amount  int
	Options []string
}

// Fixed reports whether the slot has no choice to make.
func (s AbilitySlot) Fixed() bool { return len(s.Options) == 1 }

// AbilityVariant is one complete way to take a clan's increases. Almost
// every clan has exactly one; clan/non-clan is the sole clan whose book text
// offers a genuine alternative spread ("+2/+1, or +1 to any 3"), and it gets
// two.
type AbilityVariant struct {
	Label string
	Slots []AbilitySlot
}

// abilityCodes maps every spelling the book uses for an ability onto the
// 3-letter code characters.db stores. Both the abbreviations ("+2 Wis") and
// the full names ("+2 Intelligence") appear in ability_increase_text, and
// they appear in the same column for different clans.
var abilityCodes = map[string]string{
	"str": "str", "strength": "str",
	"dex": "dex", "dexterity": "dex",
	"con": "con", "constitution": "con",
	"int": "int", "intelligence": "int",
	"wis": "wis", "wisdom": "wis",
	"cha": "cha", "charisma": "cha",
}

// allAbilities is the option set for an "any Ability Score" slot, in the
// canonical sheet order rather than map order.
var allAbilities = []string{"str", "dex", "con", "int", "wis", "cha"}

// asiSegmentPattern matches one "+N <abilities>" clause of the text.
var asiSegmentPattern = regexp.MustCompile(`^\+(\d+)\s+(.*)$`)

// asiAnyNPattern matches the parenthetical alternative spread that only
// clan/non-clan has: "(or +1 to any 3 Ability Scores.)".
var asiAnyNPattern = regexp.MustCompile(`(?i)or\s+\+(\d+)\s+to\s+any\s+(\d+)\s+ability`)

// ClanAbilityVariants reports how a clan's ability-score increases may be
// taken.
//
// clan_ability_increases wins when it has rows: those were parsed at ingest
// from unambiguous text and are already the data every other part of the app
// trusts. Only when a clan has no rows there — which is exactly the 16
// choice-granting clans — is the prose parsed. A clan with neither returns
// no variants, and the caller grants nothing rather than guessing.
func ClanAbilityVariants(rulesDB *sql.DB, clanSlug string) ([]AbilityVariant, error) {
	rows, err := rulesDB.Query(
		`SELECT ability, amount FROM clan_ability_increases WHERE clan_slug = ? ORDER BY amount DESC, ability`, clanSlug)
	if err != nil {
		return nil, fmt.Errorf("query clan ability increases: %w", err)
	}
	var fixed []AbilitySlot
	for rows.Next() {
		var ability string
		var amount int
		if err := rows.Scan(&ability, &amount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan clan ability increase: %w", err)
		}
		fixed = append(fixed, AbilitySlot{Amount: amount, Options: []string{ability}})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clan ability increase rows: %w", err)
	}
	if len(fixed) > 0 {
		return []AbilityVariant{{Slots: fixed}}, nil
	}

	var text sql.NullString
	if err := rulesDB.QueryRow(`SELECT ability_increase_text FROM clans WHERE slug = ?`, clanSlug).Scan(&text); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query clan ability increase text: %w", err)
	}
	return ParseAbilityIncreaseText(text.String), nil
}

// ParseAbilityIncreaseText turns a clans.ability_increase_text value into
// pick-slot variants. Exported so it can be tested against the real column's
// 16 distinct values without a database.
//
// Anything it cannot make sense of yields no variants rather than a partial
// guess — a clan granting nothing is a visible, reportable bug, whereas a
// clan granting the wrong ability silently corrupts a character sheet.
func ParseAbilityIncreaseText(text string) []AbilityVariant {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Parentheses carry two different things in this column: a genuine
	// alternative spread (clan/non-clan) and a restriction restating the
	// all-picks-distinct rule SetClan already enforces for every clan
	// (uchiha's "You cannot choose Int for both ASI's", yuki's equivalent).
	// The alternative is extracted; the restriction needs nothing, so
	// stripping the parenthetical is safe in both cases.
	var alternative *AbilityVariant
	if m := asiAnyNPattern.FindStringSubmatch(text); m != nil {
		amount, _ := strconv.Atoi(m[1])
		count, _ := strconv.Atoi(m[2])
		if amount > 0 && count > 0 && count <= len(allAbilities) {
			slots := make([]AbilitySlot, count)
			for i := range slots {
				slots[i] = AbilitySlot{Amount: amount, Options: allAbilities}
			}
			alternative = &AbilityVariant{
				Label: fmt.Sprintf("+%d to any %d abilities", amount, count),
				Slots: slots,
			}
		}
	}
	if i := strings.IndexByte(text, '('); i >= 0 {
		text = text[:i]
	}

	var slots []AbilitySlot
	for _, segment := range strings.Split(text, ",") {
		segment = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(segment), "."))
		if segment == "" {
			continue
		}
		m := asiSegmentPattern.FindStringSubmatch(segment)
		if m == nil {
			return nil
		}
		amount, err := strconv.Atoi(m[1])
		if err != nil || amount <= 0 {
			return nil
		}
		options := parseAbilityOptions(m[2])
		if len(options) == 0 {
			return nil
		}
		slots = append(slots, AbilitySlot{Amount: amount, Options: options})
	}
	if len(slots) == 0 {
		return nil
	}

	primary := AbilityVariant{Slots: slots}
	if alternative == nil {
		return []AbilityVariant{primary}
	}
	primary.Label = describeSlots(slots)
	return []AbilityVariant{primary, *alternative}
}

// parseAbilityOptions reads the ability half of one "+N ..." clause: either a
// list of specific abilities joined by "or", or a phrase meaning any ability
// at all.
func parseAbilityOptions(s string) []string {
	lower := strings.ToLower(s)
	// "to any Ability Score" / "to any other Ability Score not selected
	// previously" — the "other"/"not selected previously" part is the
	// all-picks-distinct rule, which SetClan enforces for every clan.
	if strings.Contains(lower, "any") {
		return allAbilities
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(lower, " or ") {
		// Trailing words like "Ability Score" would break a whole-string
		// lookup, so match on the first word that names an ability.
		var code string
		for _, field := range strings.FieldsFunc(part, func(r rune) bool { return r < 'a' || r > 'z' }) {
			if c, ok := abilityCodes[field]; ok {
				code = c
				break
			}
		}
		if code == "" || seen[code] {
			return nil
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

// describeSlots renders a variant's slots as a short label ("+2 Str or Dex,
// +1 Con") for the radio button that picks between variants. Only used when
// a clan has more than one variant, so most clans never need it.
func describeSlots(slots []AbilitySlot) string {
	parts := make([]string, 0, len(slots))
	for _, slot := range slots {
		names := make([]string, 0, len(slot.Options))
		for _, code := range slot.Options {
			names = append(names, strings.ToUpper(code))
		}
		if len(names) == len(allAbilities) {
			parts = append(parts, fmt.Sprintf("+%d to any", slot.Amount))
			continue
		}
		parts = append(parts, fmt.Sprintf("+%d %s", slot.Amount, strings.Join(names, " or ")))
	}
	return strings.Join(parts, ", ")
}

// ResolveAbilityPicks validates a player's per-slot picks against a variant
// and returns the ability/amount pairs to store.
//
// Two rules, both applied to every clan rather than only the ones whose book
// text spells them out: a pick must be one of its own slot's options, and no
// two slots may raise the same ability. The second is what uchiha's "You
// cannot choose Int for both ASI's", yuki's equivalent note and non-clan's
// "any other Ability Score not selected previously" are all saying, and it
// holds for the 29 fixed clans too — every one of them names two different
// abilities.
func ResolveAbilityPicks(variant AbilityVariant, picks []string) ([]AbilityGrant, error) {
	if len(picks) != len(variant.Slots) {
		return nil, fmt.Errorf("%w: expected %d ability picks, got %d", ErrInvalidPick, len(variant.Slots), len(picks))
	}
	seen := map[string]bool{}
	out := make([]AbilityGrant, 0, len(variant.Slots))
	for i, slot := range variant.Slots {
		pick := strings.ToLower(strings.TrimSpace(picks[i]))
		if pick == "" {
			return nil, fmt.Errorf("%w: no ability chosen for the +%d increase", ErrInvalidPick, slot.Amount)
		}
		allowed := false
		for _, option := range slot.Options {
			if option == pick {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("%w: %q is not an option for the +%d increase", ErrInvalidPick, picks[i], slot.Amount)
		}
		if seen[pick] {
			return nil, fmt.Errorf("%w: %s is chosen twice — each increase has to go to a different ability", ErrInvalidPick, strings.ToUpper(pick))
		}
		seen[pick] = true
		out = append(out, AbilityGrant{Ability: pick, Amount: slot.Amount})
	}
	return out, nil
}

// DefaultPicks returns a legal starting answer for a variant: each slot
// takes its first option that no earlier slot has already claimed.
//
// This exists because "leave the dropdowns at their first option" is NOT
// automatically legal. clan/non-clan's slots each offer all six abilities,
// so every dropdown would default to Strength and the very first submit
// would be rejected for picking the same ability twice — a form that is
// invalid until touched. Pre-selecting distinct defaults makes the common
// case (accept what's offered, press the button) work.
func DefaultPicks(variant AbilityVariant) []string {
	picks := make([]string, len(variant.Slots))
	taken := map[string]bool{}
	for i, slot := range variant.Slots {
		for _, option := range slot.Options {
			if !taken[option] {
				picks[i] = option
				taken[option] = true
				break
			}
		}
		// A slot with every option already claimed can only happen for a
		// clan whose slots genuinely cannot all be satisfied, which no
		// current clan does; leaving the pick empty surfaces that as a
		// rejected submit rather than silently writing a duplicate.
	}
	return picks
}

// ErrInvalidPick marks a rejection caused by what the player chose rather
// than by a database or rules failure, so a handler can re-render its form
// with the message instead of returning a 500.
var ErrInvalidPick = errors.New("invalid choice")

// AbilityGrant is one resolved increase, ready to be written to
// character_ability_bonuses.
type AbilityGrant struct {
	Ability string
	Amount  int
}

// resolveClanAbilityGrants is SetClan's half of the picking: it looks up the
// clan's variants, selects the one the player took, fills in the slots that
// have nothing to choose, and validates the rest.
//
// A clan with no recorded increases at all grants nothing and is not an
// error — clan/non-clan aside, that would mean a rules gap, and refusing to
// save the clan choice over it would leave the player unable to proceed
// past a step they cannot fix.
func resolveClanAbilityGrants(rulesDB *sql.DB, clanSlug string, variantIndex int, picks []string) ([]AbilityGrant, error) {
	variants, err := ClanAbilityVariants(rulesDB, clanSlug)
	if err != nil {
		return nil, err
	}
	if len(variants) == 0 {
		return nil, nil
	}
	if variantIndex < 0 || variantIndex >= len(variants) {
		variantIndex = 0
	}
	variant := variants[variantIndex]

	// Fill in from the slots themselves wherever the caller supplied
	// nothing: a fixed slot has exactly one legal answer, so requiring the
	// caller to echo it back would only create a way to get it wrong.
	resolved := make([]string, len(variant.Slots))
	for i, slot := range variant.Slots {
		if i < len(picks) && strings.TrimSpace(picks[i]) != "" {
			resolved[i] = picks[i]
			continue
		}
		if slot.Fixed() {
			resolved[i] = slot.Options[0]
		}
	}
	return ResolveAbilityPicks(variant, resolved)
}
