package main

import (
	"regexp"
	"strings"
)

// featWeaponProficiencyRe matches the "You gain proficiency in [the] X
// Weapon[s]." clause naming a single weapon category, the weapon-
// proficiency counterpart of featSkillProficiencyRe
// (feat_skill_proficiency.go). Same lazy-capture, fail-closed shape: the
// capture group only allows letters and spaces, so a clause that lists
// individual weapon names (comma-separated, e.g. "Kunai, Shuriken, and
// Senbon"), offers a choice between named equipment sets ("one set of
// equipment... known as X Weapons"), or asks for "N weapons of your
// choice" either fails to match at all (the comma breaks the capture) or
// captures a fragment that fails weaponCategoryLookup validation below —
// never a silently-truncated partial grant. Validated against the full
// live rules.db corpus (371 feats, see FEAT_AUDIT.md's weapon-set
// follow-up candidate): exactly one feat (konjiki/steel-weapon-
// manifestation) produces a validated match; five "one set of equipment"
// feats hit the regex but correctly fail category validation.
var featWeaponProficiencyRe = regexp.MustCompile(
	`(?i)you gain proficiency in (?:the )?([A-Za-z ]+?) weapons?\b`,
)

// weaponCategoryLookup maps a weapon category's lowercase name to its
// canonical display string — matching the exact spelling
// internal/charstore/multiclass.go's multiclassGrants table already writes
// into character_proficiencies (kind='weapon') for class-granted weapon
// proficiency (e.g. "All Simple and Martial Weapons" for
// class/weapon-specialist), so a feat-granted and a class-granted weapon
// category proficiency end up identically spelled in the same table.
var weaponCategoryLookup = map[string]string{
	"simple":                 "Simple Weapons",
	"martial":                "Martial Weapons",
	"exotic":                 "Exotic Weapons",
	"simple and martial":     "Simple and Martial Weapons",
	"all simple and martial": "All Simple and Martial Weapons",
}

// parseFeatWeaponProficiency extracts a feat's fixed weapon-category
// proficiency grant, if it has one. Only a clause naming exactly one
// recognized category (per weaponCategoryLookup) is recognized — a fixed
// list of individually-named weapons, a choice between named equipment
// sets, and "N weapons of your choice" clauses are all deliberately out of
// scope for this mechanism (they name a player choice or individual
// catalog items, not one of the handful of fixed weapon categories) and
// are left unparsed, falling through to FEAT_AUDIT.md's Group 2/3
// documentation instead of being silently mis-applied.
func parseFeatWeaponProficiency(description string) (string, bool) {
	m := featWeaponProficiencyRe.FindStringSubmatch(description)
	if m == nil {
		return "", false
	}
	canonical, found := weaponCategoryLookup[strings.ToLower(strings.TrimSpace(m[1]))]
	if !found {
		return "", false
	}
	return canonical, true
}
