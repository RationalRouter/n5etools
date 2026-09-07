// Package statlines centralizes parsing of the machine-readable "stat
// line" (Cost/Drain/Keyword/Prerequisite) that several catalogs in
// class_option_entries.description and class_options.description embed
// inline, at the front of otherwise free-form prose (Titan Upgrades,
// Scientific Ninja Tools and its sibling lists like E.I.Ps, Explosive
// Modifications, Inversion Serums, Arsenal Modifications, Shinobi-Ware
// Upgrades, Spyware Programs, Air Treck Enhancements, Technobi
// Mechanizations, S.N.B Upgrades — all split off the same PDF text shape).
//
// This package is shared by cmd/n5e (runtime display/game-logic) and
// cmd/n5e-i18n (the translation export tool) so there is exactly ONE
// implementation of each stat-line's regex, used identically by both:
// cmd/n5e always parses Cost/Drain/Keyword/Prerequisite from the raw
// ENGLISH column text (these are game mechanics and must never be
// re-derived from a translation), while cmd/n5e-i18n uses StripForTranslation
// to strip the same stat line off the ENGLISH text BEFORE it is ever sent to
// DeepL — so a translation never contains the stat line at all, and no
// runtime code ever needs to guess where a translated stat line ends. See
// internal/i18n.TranslatableFields' header doc on the general "machine-
// parsed fields must not be translated" rule this extends to an embedded
// prefix rather than a whole separate column.
package statlines

import (
	"regexp"
	"strconv"
	"strings"
)

// titanUpgradePattern matches a Titan Upgrades entry's own leading "Cost:
// <n> Creation Points Keyword: Mech|Weapon (Drain: <n> CCD Chakra)?" stat
// line. Bijuu Slayer's own bundled class_options row has a PDF-extraction
// double space ("Cost: 32  Creation Points") between the number and
// "Creation" that every other entry doesn't, hence `\s+` rather than a
// literal single space throughout.
var titanUpgradePattern = regexp.MustCompile(
	`^Cost:\s+(\d+)\s+Creation Points?\s+Keyword:\s+(Mech|Weapon)(?:\s+Drain:\s+(\d+)\s+CCD Chakra)?\s*`)

// ParseTitanUpgrade locates the "Cost:" stat line within raw (ENGLISH text
// only) and parses it, returning the parsed fields plus the remaining body
// text with the stat line stripped. rest equals raw unchanged (cost=0,
// keyword="") when raw doesn't contain a recognized stat line at all.
func ParseTitanUpgrade(raw string) (cost, drain int, keyword, rest string) {
	idx := strings.Index(raw, "Cost:")
	if idx < 0 {
		return 0, 0, "", raw
	}
	trimmed := raw[idx:]
	m := titanUpgradePattern.FindStringSubmatch(trimmed)
	if m == nil {
		return 0, 0, "", raw
	}
	cost, _ = strconv.Atoi(m[1])
	keyword = strings.ToLower(m[2])
	if m[3] != "" {
		drain, _ = strconv.Atoi(m[3])
	}
	rest = strings.TrimSpace(trimmed[len(m[0]):])
	return cost, drain, keyword, rest
}

// scienceNinToolPattern matches the "(Prerequisite: <name> )?Cost: <n>
// Creation Points?(?: Drain: <n> CCD Chakra)?" stat line shared by
// Scientific Ninja Tools and every sibling class_option_entries list that
// splits the same PDF-extracted shape (E.I.Ps, Explosive Modifications,
// Inversion Serums, Arsenal Modifications, Shinobi-Ware Upgrades, Spyware
// Programs, Air Treck Enhancements, Technobi Mechanizations, S.N.B
// Upgrades) — none of these carry a "Keyword:" clause, which is what
// distinguishes this shape from Titan Upgrades' own.
var scienceNinToolPattern = regexp.MustCompile(
	`^(?:Prerequisite: (.+?) )?Cost: (\d+)(?:-\d+)? Creation Points?(?: Drain: (\d+) CCD Chakra)?\s*`)

// ParseScienceNinTool splits one entry's raw (ENGLISH text only)
// description into its parsed Cost/Drain/Prerequisite plus the remaining
// prose. cost is 0 and rest is the untouched original string when the
// description doesn't start with a recognized stat line at all.
func ParseScienceNinTool(raw string) (cost, drain int, prereq, rest string) {
	m := scienceNinToolPattern.FindStringSubmatch(raw)
	if m == nil {
		return 0, 0, "", raw
	}
	prereq = strings.TrimSpace(m[1])
	cost, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		drain, _ = strconv.Atoi(m[3])
	}
	rest = strings.TrimSpace(raw[len(m[0]):])
	return cost, drain, prereq, rest
}

// StripForTranslation removes a recognized stat line from raw (ENGLISH)
// text, trying each registered catalog's own parser in turn, so the
// translation export pipeline never sends a machine-parsed stat line to
// DeepL. Returns raw unchanged when no registered pattern matches — the
// overwhelming majority of class_option_entries/class_options rows, which
// carry no stat line at all and are ordinary prose.
func StripForTranslation(raw string) string {
	if _, _, _, rest := ParseTitanUpgrade(raw); rest != raw {
		return rest
	}
	if _, _, _, rest := ParseScienceNinTool(raw); rest != raw {
		return rest
	}
	return raw
}
