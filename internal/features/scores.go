// Curated, hand-verified tables of always-on ability-score increases and
// saving-throw Mastery that class/subclass features confer — the same
// convention fixedProficiencyGrants (grants.go) established: every entry
// here was read in full against the real book text, because "increase your
// X score" phrasing also appears constantly in text that is NOT a fixed
// self-grant (an enemy debuff like Necrotic Hand's Necrotic Touch, a
// creation-time clan increase, an equipment property like Powerful Build).
// Found by a full-corpus sweep of class_features + subclass_features
// (2026-08-16): six features grant a permanent ability-score increase, one
// grants always-on saving-throw Mastery. The three whose increase is a
// player CHOICE ("Strength or Dexterity") live in choices.go's
// choiceSlotDefs instead — only the fixed, no-choice halves are here.
//
// Level gating comes free: these resolve against LoadGrantedFeatures'
// output, which already filters each feature row by the owning class's own
// level, so a level-19 Samurai Form character simply has no
// master-of-focus row to match yet.
package features

// AbilityScoreGrant is one fixed ability-score increase a feature confers
// outright. Ability is a 3-letter code; internal/charsheet.Compute adds
// Amount on top of base + stored character_ability_bonuses rows, derived
// fresh every Compute rather than stored — the same always-recomputed
// treatment proficiency/speed grants already get, so a level edit that
// drops the feature drops its bonus with no cleanup pass.
//
// RaisesMax: whether this grant also raises the ability's max (the book's
// "(and its maximum)"/"Your maximum for those scores also increases"
// phrasing) rather than just its current value — re-verified against every
// entry's full rules text on 2026-08-16 and found true on all of them, but
// kept as a per-entry field rather than a package-wide assumption since a
// future grant could plausibly be flat-only.
type AbilityScoreGrant struct {
	Ability   string
	Amount    int
	RaisesMax bool
}

var fixedAbilityScoreGrants = map[string][]AbilityScoreGrant{
	// "Your Strength or Dexterity (your choice) and Intelligence scores
	// increase by 2. Your maximum for those scores also increases by 2." —
	// the Str-or-Dex half is a choice slot (choices.go); only the
	// unconditional Intelligence half lives here.
	"class/weapon-specialist/group/weapon-forms/primal-weapon-form/feature/master-of-the-primal": {
		{Ability: "int", Amount: 2, RaisesMax: true},
	},
	// "Your Dexterity and Wisdom scores increase by 2. Your maximum for
	// those scores increases by 2."
	"class/weapon-specialist/group/weapon-forms/ranger-form/feature/unmatched-efficiency": {
		{Ability: "dex", Amount: 2, RaisesMax: true},
		{Ability: "wis", Amount: 2, RaisesMax: true},
	},
	// "Your Strength and Constitution scores increase by 2. Your maximum
	// for these scores increases by 2."
	"class/weapon-specialist/group/weapon-forms/samurai-form/feature/master-of-focus": {
		{Ability: "str", Amount: 2, RaisesMax: true},
		{Ability: "con", Amount: 2, RaisesMax: true},
	},
	// "Your Strength and Charisma scores both increase by +2, to a maximum
	// of 22." A differently-worded printing of the same "+2 to score and
	// max" pattern every other L20 Weapon Form capstone uses — 20 (the
	// standard max) + 2 = 22, so RaisesMax:true produces the stated cap
	// exactly, not a special-cased absolute value.
	"class/weapon-specialist/group/weapon-forms/obsidian-hammer-form/feature/obsidian-soul": {
		{Ability: "str", Amount: 2, RaisesMax: true},
		{Ability: "cha", Amount: 2, RaisesMax: true},
	},
	// "Your Intelligence score (and its maximum) is increased by +2." The
	// feature's FURTHER conditional +2 while wearing Juggernaut Armor is a
	// separate, equipment-gated grant — see
	// ConditionalAbilityScoreGrant/conditionalAbilityScoreGrants below,
	// which deliberately does NOT raise the max (see that grant's own
	// comment for why).
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/master-of-the-purple-technique": {
		{Ability: "int", Amount: 2, RaisesMax: true},
	},
}

// ConditionalAbilityScoreGrant is an ability-score increase that only
// applies while some runtime equipped-state condition holds — the caller
// (internal/charsheet.Compute) resolves the condition and passes it in,
// this package only gates on it. Same "caller resolves runtime state, this
// package only reacts to it" shape as ResolveACSwapAbility/
// ResolveSpeedBonus (grants.go).
type ConditionalAbilityScoreGrant struct {
	FeatureSlug string
	Ability     string
	Amount      int
}

var conditionalAbilityScoreGrants = []ConditionalAbilityScoreGrant{
	// "While wearing your Juggernaut Armor, it is increased by +2 once
	// more." (the base, unconditional +2 above is separate and always
	// raises the max; this conditional second +2 deliberately does not —
	// see ResolveConditionalAbilityScoreGrants' own comment).
	{
		FeatureSlug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/master-of-the-purple-technique",
		Ability:     "int", Amount: 2,
	},
}

// ResolveConditionalAbilityScoreGrants returns the ability-score grants
// whose condition (currently: wearing a Juggernaut Armor chassis as armor)
// is met right now. Not RaisesMax: "wearing armor" is reversible, and
// letting a reversible state raise the ASI-eligible max could strand a
// previously-legal ASI pick above the character's new effective max the
// moment the armor comes off — a state ValidASIPicks has no clean way to
// represent. The base +2 (fixedAbilityScoreGrants, above) already raises
// the max permanently; only this conditional second +2 is current-score-only.
func ResolveConditionalAbilityScoreGrants(granted []GrantedFeatureRow, wearingJuggernautArmor bool) []AbilityScoreGrant {
	if !wearingJuggernautArmor {
		return nil
	}
	grantedSlugs := make(map[string]bool, len(granted))
	for _, g := range granted {
		grantedSlugs[g.Slug] = true
	}
	var out []AbilityScoreGrant
	for _, g := range conditionalAbilityScoreGrants {
		if grantedSlugs[g.FeatureSlug] {
			out = append(out, AbilityScoreGrant{Ability: g.Ability, Amount: g.Amount})
		}
	}
	return out
}

// ResolveAbilityScoreGrants returns every fixed ability-score increase the
// character's granted features confer, keyed by feature slug against
// fixedAbilityScoreGrants — the ability-score counterpart to
// ResolveProficiencyGrants.
func ResolveAbilityScoreGrants(granted []GrantedFeatureRow) []AbilityScoreGrant {
	var out []AbilityScoreGrant
	for _, f := range granted {
		out = append(out, fixedAbilityScoreGrants[f.Slug]...)
	}
	return out
}

// saveMasteryGrants: always-on saving-throw Mastery a feature confers, by
// feature slug. Each entry is one SOURCE of Mastery on that ability's
// saves — Rank 1 (+2) per the book's own "One Source: Rank 1 Mastery (+2)"
// scale (Chapter 6, page 49; see MasteryBonus/0026_mastery.sql). The only
// always-on grant of this shape in the whole corpus (2026-08-16 sweep) is
// Intelligent Design; Science-Nin Exoskeleton's spend-chakra-per-roll save
// Mastery is activated, not always-on, and stays out per the same Group-2
// boundary every conditional effect gets.
var saveMasteryGrants = map[string][]string{
	// "While wearing your Juggernaut Armor, you... have Mastery on Strength
	// saves" — the armor condition is treated as always-on for a
	// Juggernaut, the same reading this feature's poison-immunity clause
	// already gets in passiveTraitGrants.
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design": {"str"},
}

// ResolveSaveMasteryRanks returns the character's feature-granted Mastery
// rank per saving-throw ability (3-letter code), one rank per granting
// source, stacking the way the book's own sources-count scale does. The
// caller clamps through EffectiveMasteryRank the same way stored skill
// Mastery already is.
func ResolveSaveMasteryRanks(granted []GrantedFeatureRow) map[string]int {
	out := map[string]int{}
	for _, f := range granted {
		for _, ability := range saveMasteryGrants[f.Slug] {
			out[ability]++
		}
	}
	return out
}
