package main

import (
	"regexp"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// featSkillProficiencyRe matches the common "You gain Proficiency in/with
// [the] X [skill]." clause, terminated by either a period or a comma (the
// second form covers feats phrased "You gain proficiency in X, or +1 ranks
// of Mastery if already proficient." — validated against the live rules.db
// corpus, see FEAT_AUDIT.md). "with" alongside "in" covers feats phrased
// "You gain proficiency with performance" (feat/master-of-disguise) — the
// same clause shape, just the other preposition the book uses
// interchangeably for this grant. Because the capture group only allows
// letters and spaces, any clause that mixes in a weapon/tool/kit name, a
// parenthetical choice, or a list (all of which introduce characters
// outside that set before the next period/comma) fails to match entirely
// rather than capturing a truncated fragment — that's deliberate: this
// mechanism only ever applies a clean, unconditional, single-named-skill
// grant, never a partial read of a compound clause. A captured fragment
// that isn't a real skill name (a tool/kit name, or a compound "X or Y"
// choice fragment that still happened to end in a period/comma) is caught
// downstream by skillNameLookup, not by the regex itself.
var featSkillProficiencyRe = regexp.MustCompile(
	`(?i)you gain proficiency (?:in|with) (?:the )?([A-Za-z ]+?)(?: skill)?[.,]`,
)

// featSkillProficiencyOverrides lists feats whose skill-proficiency clause
// doesn't fit featSkillProficiencyRe's shape but still grants exactly one
// fixed, unconditional skill — same override-list precedent as
// featAbilityIncreaseOverrides (feat_ability_bonus.go). feat/crafter's own
// clause is "You gain proficiency in Crafting and either Weaponsmiths or
// Armorsmith Kit (Pick one)." — Crafting itself is an unconditional grant,
// but the regex's non-greedy capture can't separate it from the kit choice
// that follows in the same sentence (it would either swallow "Crafting and
// either Weaponsmiths" as one fragment, or fail to match at all). The
// Weaponsmith-vs-Armorsmith half of that clause is a kit-vs-kit choice —
// no skill on either side — so it stays unparsed, out of scope for both
// this mechanism and feat_skill_or_tool_choice.go's picker (see that
// file's featSkillOrToolChoiceExcluded).
//
// feat/konjiki/steel-weapon-manifestation ("You gain proficiency in Martial
// Weapons and the Martial Arts skill.") and feat/konjiki/steel-smith ("You
// gain proficiency in Crafting and Weaponsmith kits, gaining +1 ranks of
// Mastery if already proficient.") hit the same shape of problem: each
// clause names an unconditional skill alongside a weapon/tool grant in the
// same sentence, with no comma before the skill's own terminating period —
// featSkillProficiencyRe's non-greedy capture runs past the skill name to
// the next period/comma, swallowing the weapon/tool text too, and the
// resulting blob fails skillNameLookup. The Martial Weapons and Weaponsmith
// kit halves of these clauses are handled separately by
// feat_weapon_proficiency.go / the tool-proficiency path.
var featSkillProficiencyOverrides = map[string]string{
	"feat/crafter": "Crafting",
	"feat/konjiki/steel-weapon-manifestation": "Martial Arts",
	"feat/konjiki/steel-smith":                "Crafting",
}

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
// saving-throw proficiencies, and "choose N skills" clauses all fail
// validation.
//
// Scans every "proficiency in/with X" clause in the description, not just
// the first, and returns the first one that names a real skill —
// necessary because several feats (feat/master-of-disguise: "You gain
// proficiency with the disguise kit. ... You gain proficiency with
// performance.") state an unrelated tool/kit grant before the skill grant
// in the same description. Stopping at the leftmost regex match alone
// would see only the disguise kit, fail skillNameLookup, and report the
// whole feat as unparsed — silently dropping the real Performance grant
// that follows it. Verified against the live rules.db corpus: no feat
// currently carries two DIFFERENT valid skill clauses this way, so
// returning the first valid match is unambiguous, not just first-found.
func parseFeatSkillProficiency(slug, description string) (string, bool) {
	if skill, found := featSkillProficiencyOverrides[slug]; found {
		return skill, true
	}
	for _, m := range featSkillProficiencyRe.FindAllStringSubmatch(description, -1) {
		if canonical, found := skillNameLookup[strings.ToLower(strings.TrimSpace(m[1]))]; found {
			return canonical, true
		}
	}
	return "", false
}

// featSkillProficiencyMasteryRe recognizes the recurring "already
// proficient, you instead gain Mastery" clause that follows many feats'
// fixed skill/tool-proficiency grant, in the three phrasings the live
// rules.db corpus actually uses:
//
//   - "already proficient, you instead gain Mastery" — the majority
//     phrasing (~28 instances, see FEAT_AUDIT.md's catalogued list),
//     modulo the capitalization of "Proficient" that feat/chemist alone
//     uses.
//   - "already have Proficiency, you instead gain one rank of Mastery" —
//     feat/class/witches-training's own wording for the same clause shape.
//     The six feat/elite-* feats also contain "already have proficiency in"
//     but as a trailing relative clause on a separate "You gain Mastery in
//     one X skill..." sentence, never followed by "you instead gain", so
//     they don't match this alternative and aren't meant to: those are
//     player-choice Mastery grants tied to a skill the player already
//     picked, not a conditional upgrade of THIS feat's own proficiency
//     grant.
//   - "gain [+N ranks of] Mastery ... if already proficient" — the reverse
//     word order used by feat/vesper/echo-location, feat/yamada/
//     asayama-pills, and feat/konjiki/steel-smith.
//
// This gates whether applyFeatProficiencyGrant, finding the character
// already proficient, should raise that skill/tool's Mastery rank instead
// of writing a duplicate, no-op proficiency row — plenty of feats
// (feat/class/hunters-training and its siblings) grant a fixed proficiency
// with no such clause, where a duplicate row really is just an inert no-op,
// same as before this mechanism existed.
var featSkillProficiencyMasteryRe = regexp.MustCompile(
	`(?i)(already (?:proficient|have proficiency),? you instead gain (?:one rank of )?mastery)|(mastery[^.]*if already proficient)`,
)

// applyFeatProficiencyGrant applies one feat's fixed skill or tool/kit
// proficiency grant for a character: an ordinary charstore.
// ApplyFeatSkillProficiency/ApplyFeatToolProficiency call when the
// character isn't already proficient in name, or — for the feats whose
// text carries the clause featSkillProficiencyMasteryRe matches — a
// one-rank increase of that skill/tool's Mastery entry instead (capped at
// charsheet.MaxMasteryRank; a further "source" beyond that grants nothing
// more, per the book's own "more than three sources" rule).
//
// character_mastery carries no source tracking of its own (migration
// 0026_mastery.sql: a direct player pick, not a derived source count — see
// that migration's own doc comment), so this Mastery increase is, unlike
// the ordinary proficiency grant, not reversed if the feat is later
// dropped from the sheet. That already matches how this app treats every
// OTHER real Mastery source (class/clan features, jutsu) it has no way to
// enumerate: none of them are auto-tracked or auto-reversed either, and a
// player who removes a feat that had granted a Mastery rank can freely
// adjust the Mastery panel by hand, the same as for any of those.
func (s *server) applyFeatProficiencyGrant(characterID int64, featSlug, kind, value, description string) error {
	already, err := charstore.HasProficiency(s.charDB, characterID, kind, value)
	if err != nil {
		return err
	}
	if already && featSkillProficiencyMasteryRe.MatchString(description) {
		rank, err := charstore.MasteryRankFor(s.charDB, characterID, value)
		if err != nil {
			return err
		}
		if rank >= charsheet.MaxMasteryRank {
			return nil
		}
		return charstore.SetMastery(s.charDB, characterID, value, rank+1)
	}
	if kind == "tool" {
		return charstore.ApplyFeatToolProficiency(s.charDB, characterID, featSlug, value)
	}
	return charstore.ApplyFeatSkillProficiency(s.charDB, characterID, featSlug, value)
}
