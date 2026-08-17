// Weapon "properties" free-text normalizer.
//
// equipment.properties is a single comma-separated column copied verbatim
// from the Mastersheet's WeapArmor tab (see internal/sheet/weaparmor.go),
// e.g. "Thrown (30/60), Multiattack, Ammunition" or "Reach 1, Grapple,
// Finesse, Tactical, Two-Handed, Winding". This file splits that text into
// individual (property, per-weapon detail) pairs so the Items page can
// render each one as a rollover tooltip against the weapon_properties
// glossary (see migration 0012_weapon_properties.sql).
//
// The live data has real inconsistencies, confirmed against out/rules.db
// rather than assumed: a stray OCR period standing in for a comma
// ("Thrown(60/120). Hidden, Returning"), inconsistent casing
// ("ammunition" vs "Ammunition"), a no-hyphen spelling ("Two Handed" vs
// "Two-Handed"), and one weapon (the Net) spelled "Grappling" where every
// other weapon uses "Grapple" — canonicalProperty maps all of these
// explicitly rather than guessing algorithmically. Deadly and Lethal are
// deliberately NOT merged despite looking similar: the Hidden Blade's
// properties list ("Deadly, Lethal 5, Hidden, Finesse") has both in the
// same weapon, proving they're distinct mechanics, not a spelling
// inconsistency.
package parse

import (
	"regexp"
	"strings"
)

// canonicalProperty maps every raw token spelling observed in the live data
// to its weapon_properties slug (see migration 0012_weapon_properties.sql).
// Keys are lowercase with whitespace collapsed, matched after
// normalizeToken strips any parenthetical/trailing-number detail.
var canonicalProperty = map[string]string{
	"ammunition":  "property/ammunition",
	"blocking":    "property/blocking",
	"critical":    "property/critical",
	"deadly":      "property/deadly",
	"disarm":      "property/disarm",
	"evocation":   "property/evocation",
	"finesse":     "property/finesse",
	"flexible":    "property/flexible",
	"grapple":     "property/grapple",
	"grappling":   "property/grapple", // one weapon (Net) spells it this way; same mechanic
	"heavy":       "property/heavy",
	"hidden":      "property/hidden",
	"lethal":      "property/lethal",
	"light":       "property/light",
	"loading":     "property/loading",
	"multiattack": "property/multiattack",
	"range":       "property/range",
	"reach":       "property/reach",
	"returning":   "property/returning",
	"special":     "property/special",
	"tactical":    "property/tactical",
	"thrown":      "property/thrown",
	"trip":        "property/trip",
	"two-handed":  "property/two-handed",
	"two handed":  "property/two-handed", // one weapon prints no hyphen
	"unarmed":     "property/unarmed",
	"versatile":   "property/versatile",
	"volatile":    "property/volatile",
	"winding":     "property/winding",
}

// EquipmentProperty is one normalized (property, detail) pair parsed from an
// equipment row's raw properties text.
type EquipmentProperty struct {
	PropertySlug string // "" if the raw name didn't match canonicalProperty — see Anomaly
	RawName      string // the original token, for the Anomaly/notes trail
	Detail       string // "30/60", "2", ... — "" if the property carries no per-weapon number
}

// ocrPeriodCommaRe fixes a stray OCR period standing in for a comma inside a
// properties list: a period followed by whitespace and a capital letter,
// mid-string (not the trailing "." at the very end, which doesn't occur in
// this column but guarded against anyway by requiring trailing content).
var ocrPeriodCommaRe = regexp.MustCompile(`\.\s+([A-Z])`)

// parenDetailRe matches "Name (detail)" or "Name(detail)" — a property name
// followed by a parenthesized detail (a range pair, a die, a damage type).
var parenDetailRe = regexp.MustCompile(`^([A-Za-z][A-Za-z \-]*?)\s*\(([^)]*)\)$`)

// trailingNumberRe matches "Name N" — a property name followed by a bare
// rank number ("Reach 1", "Deadly 2", "Volatile 3").
var trailingNumberRe = regexp.MustCompile(`^([A-Za-z][A-Za-z\-]*)\s+(\d+)$`)

// ParseEquipmentProperties splits one equipment row's raw properties text
// into normalized (property, detail) pairs. Unrecognized tokens are
// returned with an empty PropertySlug and reported as an Anomaly — flagged
// for human review via the standard needs_review mechanism, never silently
// dropped.
func ParseEquipmentProperties(raw string, subject string) ([]EquipmentProperty, []Anomaly) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = ocrPeriodCommaRe.ReplaceAllString(raw, ", $1")

	var props []EquipmentProperty
	var anomalies []Anomaly
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}

		name, detail := tok, ""
		if m := parenDetailRe.FindStringSubmatch(tok); m != nil {
			name, detail = m[1], m[2]
		} else if m := trailingNumberRe.FindStringSubmatch(tok); m != nil {
			name, detail = m[1], m[2]
		}
		name = strings.TrimSpace(name)

		key := strings.ToLower(strings.Join(strings.Fields(name), " "))
		slug, ok := canonicalProperty[key]
		if !ok {
			anomalies = append(anomalies, Anomaly{Subject: subject,
				Problem: "unrecognized weapon property: " + tok})
		}
		props = append(props, EquipmentProperty{PropertySlug: slug, RawName: name, Detail: detail})
	}
	return props, anomalies
}
