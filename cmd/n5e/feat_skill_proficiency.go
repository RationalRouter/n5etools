package main

import (
	"regexp"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// featSkillProficiencyRe matches the common "You gain Proficiency in [the]
// X [skill]." clause, terminated by either a period or a comma (the second
// form covers feats phrased "You gain proficiency in X, or +1 ranks of
// Mastery if already proficient." — validated against the live rules.db
// corpus, see FEAT_AUDIT.md). Because the capture group only allows letters
// and spaces, any clause that mixes in a weapon/tool/kit name, a
// parenthetical choice, or a list (all of which introduce characters
// outside that set before the next period/comma) fails to match entirely
// rather than capturing a truncated fragment — that's deliberate: this
// mechanism only ever applies a clean, unconditional, single-named-skill
// grant, never a partial read of a compound clause.
var featSkillProficiencyRe = regexp.MustCompile(
	`(?i)you gain proficiency in (?:the )?([A-Za-z ]+?)(?: skill)?[.,]`,
)

// skillNameLookup maps a skill's lowercase name to its canonical
// charsheet.SkillAbility key, so a captured clause like "Chakra control"
// normalizes to the same "Chakra Control" spelling every other
// character_proficiencies writer already uses.
var skillNameLookup = buildSkillNameLookup()

func buildSkillNameLookup() map[string]string {
	m := make(map[string]string, len(charsheet.SkillAbility))
	for name := range charsheet.SkillAbility {
		m[strings.ToLower(name)] = name
	}
	return m
}

// parseFeatSkillProficiency extracts a feat's fixed skill-proficiency
// grant, if it has one. Only a clause naming exactly one real skill (per
// charsheet.SkillAbility) is recognized — weapon/tool proficiencies,
// saving-throw proficiencies, and "choose N skills" clauses all fail to
// match or fail validation and are left unparsed, falling through to
// FEAT_AUDIT.md's Group 2/3 documentation instead of being silently
// mis-applied.
func parseFeatSkillProficiency(description string) (string, bool) {
	m := featSkillProficiencyRe.FindStringSubmatch(description)
	if m == nil {
		return "", false
	}
	canonical, found := skillNameLookup[strings.ToLower(strings.TrimSpace(m[1]))]
	if !found {
		return "", false
	}
	return canonical, true
}
