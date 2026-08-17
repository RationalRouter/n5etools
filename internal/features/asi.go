package features

import (
	"database/sql"
	"fmt"
	"strings"
)

// asiFeatureSuffix is the feature-slug suffix every class's own Ability
// Score Improvement/Feat shares — confirmed identical wording and slug
// shape ("class/<slug>/feature/ability-score-improvement-feat") across all
// 11 classes in rules.db, so detecting it by suffix (rather than a
// hardcoded per-class list) keeps this in sync automatically if a rules
// update adds or renames a class.
//
// The book's own text grants three things at once: "+1 to an ability
// score, gain 1 rank of Mastery in a skill you are proficient in, & a Feat
// of your choice." Only the ability-score half is modeled here — the
// Mastery clause needs no code (Mastery is already a free-form player pick
// on this sheet, see internal/features' choiceSlotDefs doc comment for why
// duplicating "which feature justifies it" adds nothing), and the Feat
// clause needs no code either (handleSheetFeatAdd is already level-ungated,
// confirmed when this feature was originally audited).
const asiFeatureSuffix = "/feature/ability-score-improvement-feat"

// ASIBreakpoints: the book's own 5 class levels Ability Score Improvement/
// Feat repeats at ("When you reach 4th and again at 8th, 12th, 16th, and
// 19th level..."). Each class tracks these independently off its OWN
// level, same as its own jutsu-known progression would in a multiclass
// build — a level-4 Hunter-Nin and a level-4 Puppet-Master on the same
// character each get their own pick, never sharing one slot.
var ASIBreakpoints = []int{4, 8, 12, 16, 19}

// ASISlot is one independently-resolvable Ability Score Improvement pick.
// Ref uniquely identifies it for character_ability_bonuses.source_ref,
// combining the granting class's own slug with the breakpoint level so two
// classes reaching the same numeric level never collide, and so
// re-resolving the SAME breakpoint replaces the old pick (charstore.
// SetAbilityScoreImprovement deletes-then-inserts by this exact ref) rather
// than stacking a second +1 on top of it.
type ASISlot struct {
	FeatureSlug string
	ClassSlug   string
	Level       int
	Ref         string
}

// ResolveASISlots returns every ASI slot the character currently qualifies
// for (a class with the ASI feature, whose own level has reached the
// breakpoint), regardless of whether it's been picked yet.
func ResolveASISlots(granted []GrantedFeatureRow, classLevels map[string]int) []ASISlot {
	var out []ASISlot
	seen := map[string]bool{}
	for _, g := range granted {
		if !strings.HasSuffix(g.Slug, asiFeatureSuffix) || seen[g.Slug] {
			continue
		}
		seen[g.Slug] = true
		classSlug := classSlugOf(g.Slug)
		level := classLevels[classSlug]
		for _, bp := range ASIBreakpoints {
			if level < bp {
				continue
			}
			out = append(out, ASISlot{
				FeatureSlug: g.Slug, ClassSlug: classSlug, Level: bp,
				Ref: fmt.Sprintf("%s@%d", classSlug, bp),
			})
		}
	}
	return out
}

// PendingASISlots filters slots down to the ones with no resolved pick yet.
func PendingASISlots(slots []ASISlot, resolvedRefs map[string]bool) []ASISlot {
	var out []ASISlot
	for _, s := range slots {
		if !resolvedRefs[s.Ref] {
			out = append(out, s)
		}
	}
	return out
}

// LoadResolvedASIRefs returns every ASI breakpoint ref this character has
// already resolved — either by spending it on an ability increase
// (character_ability_bonuses, source_kind='asi') or by spending it on a Feat
// instead (character_asi_feat_choices, see that table's own migration
// comment for why "the ability path" and "the Feat path" are mutually
// exclusive alternatives for the same ref, never both at once).
func LoadResolvedASIRefs(charDB *sql.DB, characterID int64) (map[string]bool, error) {
	rows, err := charDB.Query(
		`SELECT source_ref FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'asi'
		 UNION
		 SELECT ref FROM character_asi_feat_choices WHERE character_id = ?`,
		characterID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		out[ref] = true
	}
	return out, rows.Err()
}

// AbilityPick is one ability-and-amount pair within a resolved Ability Score
// Improvement pick — see ValidASIPicks for the two shapes the book allows.
type AbilityPick struct {
	Ability string
	Amount  int
}

// ValidASIPicks reports whether picks matches one of the book's own two
// Ability Score Improvement shapes ("+1 to two different ability scores, or
// +2 to one ability score") and respects each ability's own cap. currentScores
// is the character's CURRENT score per 3-letter ability code (post every
// other bonus already applied); maxScores is that same character's real
// per-ability maximum (base 20, plus anything from a RaisesMax grant — see
// internal/charsheet.Sheet.AbilityMax) — so a score sitting one below its
// own max correctly rejects a +2 pick but still allows a +1 one, and a
// score already at its max rejects both, the same shape the old flat-20
// check had, just against the real max instead of an assumed one.
func ValidASIPicks(picks []AbilityPick, currentScores, maxScores map[string]int) bool {
	switch len(picks) {
	case 1:
		if picks[0].Amount != 2 {
			return false
		}
	case 2:
		if picks[0].Amount != 1 || picks[1].Amount != 1 || picks[0].Ability == picks[1].Ability {
			return false
		}
	default:
		return false
	}
	for _, p := range picks {
		current, ok := currentScores[p.Ability]
		if !ok {
			return false
		}
		max, ok := maxScores[p.Ability]
		if !ok {
			max = 20
		}
		if current+p.Amount > max {
			return false
		}
	}
	return true
}
