package main

import (
	"regexp"
	"strconv"
	"strings"
)

// Breaking a background's printed starting-equipment sentence into real
// items.
//
// backgrounds.equipment_text is one sentence listing everything a character
// begins with — "A Map of the Village you grew up in, A Token of intimate
// value to you, Set of Basic Clothing, Wallet containing 100 Ryo". The
// equipment step used to store that whole sentence as ONE free-text
// character_inventory row, which rendered on the sheet as a single
// unclickable line stretching across the Item column with no type, no bulk
// and nothing to equip.
//
// parseStartingEquipment splits it into one line per thing, resolves the
// ones that name a real equipment row, and pulls out the starting Ryo so the
// sheet's purse begins with the right amount instead of zero.
//
// Lines that name something the rules have no row for — a love letter, a
// signet ring, a manifesto you are currently writing — stay as free text.
// That is correct, not a gap: they are real possessions with real story
// weight and no stats, and inventing equipment rows for them would put
// fictional items in the rules database.

// startingEquipmentLine is one parsed possession.
type startingEquipmentLine struct {
	Text     string // as printed, so an unresolved line still reads naturally
	Slug     string // "" when the text names nothing in the rules
	Quantity int

	// IsToolkitChoice marks this line as an unresolved toolkit PICK rather
	// than a plain unresolved line — the book grants a choice, not one
	// specific item ("1 Toolkit (pick one)", "3 Toolkits (pick three)", "1
	// Hackers Kit or Security Kit (pick one)"). Quantity is how many
	// toolkits the pick grants; ToolkitChoiceOptions is "" for "any toolkit
	// in the catalog" or a "|"-joined list of specific names for a scoped
	// choice ("Hackers Kit|Security Kit"). Set only when Slug == "" — see
	// detectPackToolkitChoice and UnpackedItem/ResolvePackToolkitChoice for
	// how the placeholder this becomes gets resolved into a real item and a
	// tool proficiency at unpack time.
	IsToolkitChoice      bool
	ToolkitChoiceOptions string
}

// startingEquipment is a whole parsed equipment_text.
type startingEquipment struct {
	Lines []startingEquipmentLine
	Ryo   float64
}

// containingRyoPattern matches a purse: "Wallet containing 100 Ryo", "a
// pouch containing 100 Ryo".
//
// The "containing" is load-bearing. background/traveler's line also says "a
// small piece of jewelry worth 10 Ryo in the style of your homeland's
// craftsmanship" — that 10 Ryo is an item's value, not money in hand, and a
// bare "\d+ Ryo" match would have quietly added it to the character's purse.
var containingRyoPattern = regexp.MustCompile(`(?i)^(.*?)\bcontain(?:ing|s)\s+([\d,]+)\s*ryo\b.*$`)

// bareRyoPattern matches a segment that is nothing but an amount of money —
// background/hermit ends "...a poison kit, and 100 Ryo" with no container.
var bareRyoPattern = regexp.MustCompile(`(?i)^([\d,]+)\s*ryo$`)

// leadingCountPattern strips the article or count a printed line opens with.
var leadingCountPattern = regexp.MustCompile(`(?i)^(a|an|one|two|three|four|five|six|the|\d+)\s+`)

// countWords are the spelled-out counts that appear in this data.
var countWords = map[string]int{"a": 1, "an": 1, "one": 1, "the": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6}

// startingEquipmentAliases maps the book's prose names for a possession onto
// the equipment row that actually is that thing.
//
// Hand-built against the ten backgrounds' real equipment_text values, and
// deliberately small: every entry is a genuine synonym ("commoners' clothes"
// IS Ordinary Clothing), never an approximation. Anything not obviously the
// same object — a winter blanket is not Winter Clothing, a gaming set is not
// any row that exists — is left out so it stays honest free text.
// Both apostrophe-bearing and bare spellings are listed: normalizeItemName
// only straightens the curly apostrophe, it does not remove it, and the
// backgrounds genuinely print both "commoners’ clothes" and "common
// clothes".
var startingEquipmentAliases = map[string]string{
	"common clothes":      "gear/ordinary-clothing",
	"commoners' clothes":  "gear/ordinary-clothing",
	"commoner's clothes":  "gear/ordinary-clothing",
	"commoners clothes":   "gear/ordinary-clothing",
	"commoner clothes":    "gear/ordinary-clothing",
	"basic clothing":      "gear/ordinary-clothing",
	"basic clothes":       "gear/ordinary-clothing",
	"ordinary clothes":    "gear/ordinary-clothing",
	"traveler's clothes":  "gear/ordinary-clothing",
	"travelers' clothes":  "gear/ordinary-clothing",
	"travelers clothes":   "gear/ordinary-clothing",
	"traveller's clothes": "gear/ordinary-clothing",
	"fine clothes":        "gear/fine-clothing",
	"blank jutsu scroll":  "scroll/blank-scroll",
	"blank jutsu scrolls": "scroll/blank-scroll",
	"jutsu scroll":        "scroll/blank-scroll",
	"blank weapon scroll": "scroll/blank-scroll",

	// The class equipment lines below go through the same resolver (see
	// parseCompoundEquipment). Each of these is the book naming a row that
	// exists under a different printed spelling, never an approximation:
	//   - The only cooking implement in the rules is the Cooking Kit.
	//   - The equipment table inverts the crossbow names ("Crossbow, Hand")
	//     while the class line writes them the way people say them.
	//   - "Ninjutsu Scroll (D-Rank)" is exactly the D-Rank Jutsu Scroll row.
	"cooking tools":            "toolkit/cooking-kit",
	"cooking tool":             "toolkit/cooking-kit",
	"hand crossbow":            "weapon/crossbow-hand",
	"heavy crossbow":           "weapon/crossbow-heavy",
	"ninjutsu scroll (e-rank)": "scroll/e-rank-jutsu-scroll",
	"ninjutsu scroll (d-rank)": "scroll/d-rank-jutsu-scroll",
	"ninjutsu scroll (c-rank)": "scroll/c-rank-jutsu-scroll",
	"ninjutsu scroll (b-rank)": "scroll/b-rank-jutsu-scroll",
	"ninjutsu scroll (a-rank)": "scroll/a-rank-jutsu-scroll",
	"ninjutsu scroll (s-rank)": "scroll/s-rank-jutsu-scroll",
}

// packContentsPrefix is how every pack in the book opens its contents list.
const packContentsPrefix = "Contents:"

// packContentOverrides handle the four segments in the five packs' contents
// that the generic resolver cannot read, each for a specific reason:
//
//   - "7 days of field rations" names a row that is one day's rations, so the
//     count belongs to the item rather than to the phrase. Quantity 0 means
//     "keep the count parsed off the segment" — seven of them.
//   - "50 feet of rope sealed in a scroll" names Rope (50 ft): the 50 is part
//     of the item's own name, so taking it as a count would put fifty ropes in
//     the inventory.
//   - "blank Keycard" and "blank Data Scroll" are the book naming the
//     Keycards and Data Scroll rows with the word "blank" in front.
//
// Everything else in the five lists either resolves on its own or genuinely
// names no equipment row ("an empty book", "writing utensils", "1 Toolkit
// (pick one)") and stays as free text.
var packContentOverrides = map[string]struct {
	Slug     string
	Quantity int
}{
	"days of field rations":           {"gear/field-rations-1-day", 0},
	"feet of rope sealed in a scroll": {"gear/rope-50ft", 1},
	"blank keycard":                   {"tool/keycards", 1},
	"blank data scroll":               {"scroll/data-scroll", 1},
}

// packToolkitChoicePattern matches a pack-contents segment offering N picks
// from the whole toolkit catalog: "1 Toolkit (pick one)", "3 Toolkits (pick
// three)". The number word inside "(pick ...)" always repeats the segment's
// own leading count in the book's data, so only the leading count (already
// parsed by parseStartingCount) is used for how many picks to grant.
var packToolkitChoicePattern = regexp.MustCompile(`(?i)^\d+\s+toolkits?\s*\(pick\s+\w+\)\.?$`)

// packToolkitOrChoicePattern matches a pack-contents segment offering a pick
// between two specifically named items — "1 Hackers Kit or Security Kit
// (pick one)". Captures both names as printed; detectPackToolkitChoice
// still has to confirm both are real toolkits before trusting this as a
// toolkit choice at all; a structural match on unrelated "X or Y (pick
// one)" prose (none exists in the five packs today, but nothing stops a
// future one) must stay honest, unresolved free text instead.
var packToolkitOrChoicePattern = regexp.MustCompile(`(?i)^\d+\s+(.+?)\s+or\s+(.+?)\s*\(pick\s+\w+\)\.?$`)

// detectPackToolkitChoice recognizes a pack-contents segment that grants a
// toolkit PICK rather than one specific item, and reports how many picks
// and from which options (empty means "any toolkit in the live catalog").
// ok is false for anything else, including a segment that structurally
// matches packToolkitOrChoicePattern but whose two named options aren't
// both real toolkits.
func detectPackToolkitChoice(segment string, toolkitByName map[string]toolkitOption) (count int, options []string, ok bool) {
	trimmed := strings.TrimSpace(segment)
	if packToolkitChoicePattern.MatchString(trimmed) {
		return parseStartingCount(trimmed), nil, true
	}
	if m := packToolkitOrChoicePattern.FindStringSubmatch(trimmed); m != nil {
		a, aok := toolkitByName[normalizeItemName(m[1])]
		b, bok := toolkitByName[normalizeItemName(m[2])]
		if aok && bok {
			return parseStartingCount(trimmed), []string{a.Name, b.Name}, true
		}
	}
	return 0, nil, false
}

// parsePackContents reads an item's description and returns what it contains,
// or nil for an item that contains nothing.
//
// For items that contain other items, such as traveler's packs, the
// description is read to determine what the pack contains. The pack itself
// is added to the inventory alongside an unpack button that adds its
// constituent items to the inventory.
//
// Driven by the description text rather than by a list of pack slugs, so a
// rules update that adds a sixth pack needs nothing here — any item whose
// description opens "Contents:" unpacks.
//
// A segment that names a toolkit PICK rather than one item ("1 Toolkit
// (pick one)", "3 Toolkits (pick three)", "1 Hackers Kit or Security Kit
// (pick one)") comes back flagged IsToolkitChoice instead of resolved
// outright — the pack still can't choose FOR the player, but unpacking it
// now turns the pick into a real dropdown on the sheet (see
// buildPendingPackToolkitChoiceRows/ResolvePackToolkitChoice) rather than
// leaving an inert sentence sitting in the inventory with nothing to do
// with it.
func (s *server) parsePackContents(description string) ([]startingEquipmentLine, error) {
	text := strings.TrimSpace(description)
	if !strings.HasPrefix(text, packContentsPrefix) {
		return nil, nil
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, packContentsPrefix))
	if text == "" {
		return nil, nil
	}
	index, err := s.equipmentNameIndex()
	if err != nil {
		return nil, err
	}
	toolkits, err := s.loadToolkitOptions()
	if err != nil {
		return nil, err
	}
	toolkitByName := make(map[string]toolkitOption, len(toolkits))
	for _, t := range toolkits {
		toolkitByName[normalizeItemName(t.Name)] = t
	}

	var out []startingEquipmentLine
	for _, segment := range splitEquipmentSentence(text) {
		line := startingEquipmentLine{
			Text:     segment,
			Slug:     resolveStartingItem(segment, index),
			Quantity: parseStartingCount(segment),
		}
		if override, ok := packContentOverrides[normalizeItemName(segment)]; ok {
			line.Slug = override.Slug
			if override.Quantity > 0 {
				line.Quantity = override.Quantity
			}
		}
		if line.Slug == "" {
			if count, options, ok := detectPackToolkitChoice(segment, toolkitByName); ok {
				line.IsToolkitChoice = true
				line.Quantity = count
				line.ToolkitChoiceOptions = strings.Join(options, "|")
			}
		}
		out = append(out, line)
	}
	return out, nil
}

// parseCompoundEquipment resolves a class equipment option that names several
// possessions in one line — "Cooking Tools, Flash Tag, Paper Bomb", "Padded
// Cloth, Poison Kit, and 1 smoke bombs", "Crafter's pack, and 1 paper bomb".
//
// Rather than adding that prose to inventory directly, the program parses it,
// looks the named items up in the database, and adds those resolved items to
// the inventory on character creation.
//
// This is parseStartingEquipment with the money handling left out — a class
// equipment line never contains a purse, and the Ryo half of that function
// exists specifically for the backgrounds' "wallet containing 100 Ryo". Every
// other rule is shared, deliberately: the two kinds of line are written in the
// same prose by the same book, so a name that resolves in one must resolve in
// the other.
//
// A part that names nothing in the rules stays as its own free-text line
// rather than dragging the whole option back to being one unresolved
// sentence. "Padded Cloth, Poison Kit, and 1 smoke bombs" with an unknown
// third part should still put the armor and the kit in the inventory as real
// items.
func (s *server) parseCompoundEquipment(text string) ([]startingEquipmentLine, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	index, err := s.equipmentNameIndex()
	if err != nil {
		return nil, err
	}
	var out []startingEquipmentLine
	for _, segment := range splitEquipmentSentence(text) {
		out = append(out, startingEquipmentLine{
			Text:     segment,
			Slug:     resolveStartingItem(segment, index),
			Quantity: parseStartingCount(segment),
		})
	}
	return out, nil
}

// parseStartingEquipment splits equipment_text into its parts and resolves
// what it can against the live equipment table.
func (s *server) parseStartingEquipment(text string) (startingEquipment, error) {
	var out startingEquipment
	if strings.TrimSpace(text) == "" {
		return out, nil
	}
	index, err := s.equipmentNameIndex()
	if err != nil {
		return out, err
	}

	for _, segment := range splitEquipmentSentence(text) {
		if m := containingRyoPattern.FindStringSubmatch(segment); m != nil {
			out.Ryo += parseRyoAmount(m[2])
			// The container itself is a real possession — a Wallet is an
			// equipment row with a cost. Add it when it resolves; a
			// generic "pouch" doesn't, and adds nothing.
			if container := strings.TrimSpace(m[1]); container != "" {
				if slug := resolveStartingItem(container, index); slug != "" {
					out.Lines = append(out.Lines, startingEquipmentLine{
						Text: container, Slug: slug, Quantity: 1,
					})
				}
			}
			continue
		}
		if m := bareRyoPattern.FindStringSubmatch(segment); m != nil {
			out.Ryo += parseRyoAmount(m[1])
			continue
		}
		out.Lines = append(out.Lines, startingEquipmentLine{
			Text:     segment,
			Slug:     resolveStartingItem(segment, index),
			Quantity: parseStartingCount(segment),
		})
	}
	return out, nil
}

// sentenceSplitPattern breaks the printed sentence at commas and at the
// final " and ". Item names in this data never contain either, checked
// against all ten backgrounds' text.
var sentenceSplitPattern = regexp.MustCompile(`(?i),|\s+and\s+`)

func splitEquipmentSentence(text string) []string {
	var out []string
	for _, part := range sentenceSplitPattern.Split(text, -1) {
		part = strings.TrimSpace(part)
		part = strings.TrimSuffix(part, ".")
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseRyoAmount(s string) float64 {
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return n
}

func parseStartingCount(segment string) int {
	m := leadingCountPattern.FindStringSubmatch(segment)
	if m == nil {
		return 1
	}
	word := strings.ToLower(m[1])
	if n, ok := countWords[word]; ok {
		return n
	}
	if n, err := strconv.Atoi(word); err == nil && n > 0 {
		return n
	}
	return 1
}

// equipmentNameIndex maps every item's normalized name to its slug.
//
// Built from the live table rather than hardcoded (unlike internal/store's
// ingest-time equipmentNameLookup, which resolves a different and much
// narrower vocabulary) so a rules update that adds an item makes it
// resolvable here the same day, with no code change.
//
// Greater/Superior/Supreme rows are excluded: a background never grants
// upgraded gear, and leaving them in would let a stray text match hand out
// a 1200-Ryo kit at character creation — the same problem
// isStartingTierGear exists to prevent on the toolkit dropdowns.
func (s *server) equipmentNameIndex() (map[string]string, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name FROM equipment`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := map[string]string{}
	for rows.Next() {
		var slug, name string
		if err := rows.Scan(&slug, &name); err != nil {
			return nil, err
		}
		if !isStartingTierGear(name) {
			continue
		}
		index[normalizeItemName(name)] = slug
	}
	return index, rows.Err()
}

// normalizeItemName reduces a printed name or a prose fragment to a
// comparable key: lowercase, straight apostrophes, no leading article or
// count, no "set of" preamble, single-spaced.
func normalizeItemName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "’", "'")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSuffix(s, ".")
	for {
		trimmed := leadingCountPattern.ReplaceAllString(s, "")
		trimmed = strings.TrimPrefix(trimmed, "set of ")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == s {
			break
		}
		s = trimmed
	}
	return s
}

// resolveStartingItem finds the equipment row a printed fragment names, or
// "" when it names nothing in the rules.
//
// Matching is exact against the normalized name, plus a singular/plural
// retry and the alias table — never a substring or fuzzy match. "A Book full
// of teachings from your master" must not resolve to some row that happens
// to contain the word "book": a wrong item on a character sheet is worse
// than an honest free-text line.
func resolveStartingItem(text string, index map[string]string) string {
	key := normalizeItemName(text)
	if key == "" {
		return ""
	}
	if slug, ok := startingEquipmentAliases[key]; ok {
		return slug
	}
	if slug, ok := index[key]; ok {
		return slug
	}
	// The book writes "1 Blank Jutsu Scrolls" and "a poison kit" against
	// rows named "Blank Weapon/Item/Jutsu Scroll" and "Poison Kit", so both
	// directions of the plural are worth one retry each.
	if singular, ok := strings.CutSuffix(key, "s"); ok {
		if slug, ok := startingEquipmentAliases[singular]; ok {
			return slug
		}
		if slug, ok := index[singular]; ok {
			return slug
		}
	}
	if slug, ok := index[key+"s"]; ok {
		return slug
	}
	return ""
}
