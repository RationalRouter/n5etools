package store

import (
	"regexp"
	"strconv"
	"strings"
)

// equipmentChoiceOption is one real alternative extracted from a printed
// class-equipment bullet (e.g. "(a) Padded Cloth or (b) Combat Jacket"
// becomes two of these) — see splitEquipmentChoice.
type equipmentChoiceOption struct {
	Description string // as printed, minus the "(a)"/"(b)" label
	ItemSlug    string // "" when the text doesn't name one specific, resolvable item
	Quantity    int
}

var letterPrefixPattern = regexp.MustCompile(`(?i)^\([a-z]\)\s*`)
var orSplitPattern = regexp.MustCompile(`(?i)\s+or\s+`)
var leadingQuantityPattern = regexp.MustCompile(`(?i)^(a|an|one|two|three|four|five|\d+)\s+`)
var quantityWords = map[string]int{"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5}

// equipmentNameLookup maps a cleaned, singular item name (lowercase) to its
// real equipment.slug. Hand-built against the actual printed class
// equipment bullets across all 11 classes (checked directly against a live
// `SELECT slug, name FROM equipment` query, not a fuzzy matcher) — the
// vocabulary here is small and closed, so an explicit, reviewable table is
// more trustworthy than pattern-matching. Deliberately does NOT include
// category/choice phrases ("Simple Weapon", "Martial Weapon", "Toolkit",
// "Kit of your choice", "Ninjutsu Scroll", "stack of bolts") — those
// describe a class of item or a free player pick, not one specific
// equipment row, so resolving them would be inventing a link the data
// doesn't actually support. Multi-item bundles ("Padded Cloth, Poison Kit,
// and 1 smoke bombs") also correctly fail to match here and stay
// unresolved, since a single item_slug column can't represent three items
// — that's intentional, not a gap to chase.
var equipmentNameLookup = map[string]string{
	"padded cloth":   "armor/padded-cloth",
	"combat jacket":  "armor/combat-jacket",
	"combat armor":   "armor/combat-armor",
	"combat bracer":  "weapon/combat-bracers",
	"iron claw":      "weapon/iron-claw",
	"knuckle blade":  "weapon/knuckle-blades",
	"kunai stack":    "weapon/kunai",
	"kunai":          "weapon/kunai",
	"shuriken stack": "weapon/shuriken",
	"shuriken":       "weapon/shuriken",
	"senbon stack":   "weapon/senbon",
	"senbon":         "weapon/senbon",
	"fuma shuriken":  "weapon/fuma-shuriken",
	"fuma-shuriken":  "weapon/fuma-shuriken",
	"paper bomb":     "tool/paper-bombs",
	"flash tag":      "tool/flash-tag",
	"smoke bomb":     "tool/smoke-bomb",
	"hand crossbow":  "weapon/crossbow-hand",
	"crafter's pack": "gear/crafters-pack",
	"crafters pack":  "gear/crafters-pack",
}

// splitEquipmentChoice splits one printed equipment-choice bullet into its
// real alternatives (split on a top-level " or "; real item names in this
// data never contain the word "or", confirmed by inspection of every
// class_equipment_options row, so a plain split is safe) and attempts to
// resolve each to a real equipment.slug.
func splitEquipmentChoice(raw string) []equipmentChoiceOption {
	parts := orSplitPattern.Split(raw, -1)
	options := make([]equipmentChoiceOption, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = letterPrefixPattern.ReplaceAllString(part, "")
		part = strings.TrimSuffix(strings.TrimSpace(part), ".")
		if part == "" {
			continue
		}
		options = append(options, equipmentChoiceOption{
			Description: part,
			ItemSlug:    resolveEquipmentSlug(part),
			Quantity:    parseLeadingQuantity(part),
		})
	}
	return options
}

// resolveEquipmentSlug normalizes text down to a bare item name (strip
// leading quantity, lowercase, drop a trailing "s" if the singular form is
// what's in the lookup table) and checks equipmentNameLookup. Returns ""
// for anything that doesn't name one specific known item — category
// phrases, free-choice text, and multi-item bundles all correctly miss.
func resolveEquipmentSlug(text string) string {
	cleaned := strings.ToLower(strings.TrimSpace(text))
	cleaned = leadingQuantityPattern.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)
	if slug, ok := equipmentNameLookup[cleaned]; ok {
		return slug
	}
	if singular, ok := strings.CutSuffix(cleaned, "s"); ok {
		if slug, ok := equipmentNameLookup[singular]; ok {
			return slug
		}
	}
	return ""
}

// parseLeadingQuantity reads a leading count word/digit ("2 Simple
// Weapons", "One Kunai Stack") off already-lowercased-or-not text; defaults
// to 1 when nothing is found (the overwhelmingly common case, and correct
// for a bare item name with no explicit count).
func parseLeadingQuantity(text string) int {
	m := leadingQuantityPattern.FindStringSubmatch(text)
	if m == nil {
		return 1
	}
	word := strings.ToLower(m[1])
	if n, ok := quantityWords[word]; ok {
		return n
	}
	if n, err := strconv.Atoi(word); err == nil {
		return n
	}
	return 1
}
