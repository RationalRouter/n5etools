package features

import (
	"database/sql"
	"strings"
)

// ChoiceKind identifies what a resolved pick turns into once applied.
type ChoiceKind string

const (
	ChoiceSkillProficiency       ChoiceKind = "skill_proficiency"
	ChoiceSavingThrowProficiency ChoiceKind = "saving_throw_proficiency"
	// ChoiceAbilityScoreIncrease: the feature permanently raises one
	// ability score by Amount, and the player picks which from Options —
	// "increase either your Strength or Dexterity scores by +2 (Pick one;
	// This cannot be changed)". Resolved picks become AbilityScoreGrants
	// (ResolveChoiceAbilityScoreGrants), the same shape scores.go's fixed
	// table produces.
	ChoiceAbilityScoreIncrease ChoiceKind = "ability_score_increase"
	// ChoiceUpgradeEntry: the feature grants ONE of several named Upgrade
	// catalog entries for free, outside the character's normal Upgrade-slot
	// cap — Puppet Master's Purple Technique Proficiency ("Choose between
	// Chakra Blast, Integrated Weapon, or Power Fist... You gain this
	// Upgrade") and Chakra String Augments ("Select one of Better Bonds,
	// Chakra Pathway, Defensive Manipulation, or Thread Weapon... You gain
	// this upgrade for free"), each with a small, fixed Options list (an
	// Upgrade entry's own class_option_entries.slug) known ahead of time —
	// this package has no need to reach the real catalog for these, unlike
	// ChoiceSkillProficiency's "any skill." cmd/n5e resolves a picked value
	// into the same granted-display treatment puppetUpgradeAutoGrants'
	// fixed single-entry grants already get.
	ChoiceUpgradeEntry ChoiceKind = "upgrade_entry"
	// ChoiceArmorProperty: the feature grants one or more Armor Properties
	// (Chapter 5: Equipment) applied to a worn suit of armor — Puppet
	// Master's Nearly Perfected Architecture ("select two Armor Properties
	// from Chapter 5: Equipment. Your Juggernaut Armor gains these
	// properties"). Options is nil (like ChoiceSkillProficiency): the real
	// catalog is rules.db's armor_properties table, which this package
	// can't reach — cmd/n5e resolves/validates against it directly.
	ChoiceArmorProperty ChoiceKind = "armor_property"
	// ChoiceSymphonyBranch: Puppet Master's Symphony of Puppetry ("Select
	// one of the following options: Expansion... Enhancement...") — a
	// top-level either/or pick between two entirely different mechanical
	// payoffs, not a "which one of these similar things" pick. Options is
	// the fixed 2-value branch-name list ("expansion"/"enhancement");
	// cmd/n5e resolves each branch's own further effects once picked.
	ChoiceSymphonyBranch ChoiceKind = "symphony_branch"
	// ChoiceCompanionAbilityScoreIncrease: the feature grants a +2 to a
	// player-chosen ability score, applied to the character's COMPANIONS
	// rather than the character's own score — Puppet Master's Elevated
	// Design ("All Puppet Tools you possess gain a +2 to two different
	// ability scores. The maximums for these chosen scores are increased
	// to 22."). Deliberately a distinct kind from ChoiceAbilityScoreIncrease
	// rather than reusing it: that kind's own resolution
	// (ResolveChoiceAbilityScoreGrants) raises the CHARACTER's score, and
	// its UI is a full-page-reload confirm form (an ability change ripples
	// into Skills/Saves/AC/HP/Chakra) — neither is correct here, since this
	// pick never touches the character's own ability scores at all. cmd/n5e
	// resolves it into a display-only bonus shown next to (not baked into)
	// each Puppet Tool's own player-editable ability score fields — see
	// puppetElevatedDesignAbilityBonus.
	ChoiceCompanionAbilityScoreIncrease ChoiceKind = "companion_ability_score_increase"
)

// ChoiceKey identifies one independent pick a granted feature offers,
// matching character_feature_choices' own composite key (minus character_id,
// which the caller already scopes queries by).
type ChoiceKey struct {
	FeatureSlug string
	ChoiceIndex int
}

// ChoiceSlot is one independently-resolvable player pick — Nara's
// Masterwork Skill offers two SEPARATE 2-skill choice slots (one unlocked at
// 7th level, one at 18th), not one 4-skill slot, since the book gates them
// at different levels. Options is nil for "any skill" (the real catalog is
// internal/charsheet.SkillAbility, which this package can't import — see
// this file's own header — so the caller supplies/validates it) or a fixed
// list of 3-letter ability codes for a saving-throw pick.
type ChoiceSlot struct {
	FeatureSlug string
	ChoiceIndex int
	Kind        ChoiceKind
	Label       string
	Options     []string
	// Amount is how much a ChoiceAbilityScoreIncrease pick raises the
	// chosen score by; zero for every other kind.
	Amount int
	// RaisesMax: ChoiceAbilityScoreIncrease only — see AbilityScoreGrant's
	// own field of the same name in scores.go.
	RaisesMax bool
}

// choiceSlotDef is the level-gated definition backing a ChoiceSlot.
// MinLevel is checked against the OWNING class's own level for a
// class/subclass feature (parsed from FeatureSlug's own "class/<slug>/..."
// prefix — subclass features live under the same prefix, so no
// subclass-to-class lookup is needed here) or the character's TOTAL level
// for a clan feature, matching the same class-level-vs-character-level split
// LoadGrantedFeatures/ResolveSpeedBonus already draw.
//
// Every entry here was read against the real book text, same bar
// fixedProficiencyGrants sets: several proficiency-granting features found
// in the same audit pass are deliberately NOT here —
//   - Any feature that only grants a TOOL/TOOLKIT/ARMOR/WEAPON proficiency
//     (Genwa's Hacker/Security kits, Ironclad's Heavy Armor, Purple
//     Technique's Heavy Armor + Disguise kit, and others) — same "nothing
//     computes a tool/weapon check yet" boundary fixedProficiencyGrants'
//     own doc comment already draws; a toolkit-choice picker is a distinct,
//     still-deferred piece of work, not folded in here.
//   - Any feature whose payoff is Mastery rather than proficiency (Hunter-Nin
//     Expertise, Puppet Master's Tool Expertise, every "or gain Mastery if
//     already proficient" escalation on the slots below) — Mastery is
//     already a free-form player pick on this sheet (any skill or toolkit,
//     gated only by the level cap, see character_mastery/SetMastery), by
//     deliberate design (0026_mastery.sql: not an attempt to derive "how
//     many real sources" a player has). A feature that grants Mastery adds
//     no capability the player doesn't already have through that picker, so
//     modeling it here would be pure duplication.
//   - Puppet Master's Generalized Skill: its plain "you gain proficiency in
//     these chosen skills" sentence only unambiguously applies to the
//     "if you do not have a Puppet Tool" branch; the far more common
//     has-a-Puppet-Tool case reads as a companion-side skill-sharing
//     mechanic, not a character proficiency grant. Same "unreadable/
//     conditional clauses are dropped, never guessed" precedent as
//     Scout-Nin Mobility's contradictory second tier (grants.go).
//   - Every companion/summon-targeted grant (Inuzuka's Nin-Dog, Scout-Nin's
//     clones, Shadowhand's sigils) — those are the COMPANION's own
//     proficiencies, not the character's; out of this package's scope the
//     same way equipment-granted bonuses are.
type choiceSlotDef struct {
	FeatureSlug string
	ChoiceIndex int
	Kind        ChoiceKind
	MinLevel    int
	Label       string
	Options     []string
	Amount      int  // ChoiceAbilityScoreIncrease only; see ChoiceSlot.Amount
	RaisesMax   bool // ChoiceAbilityScoreIncrease only; see ChoiceSlot.RaisesMax
}

var choiceSlotDefs = []choiceSlotDef{
	// Nara "Masterwork Skill": "At 7th Level You may select two skills, you
	// gain proficiency in these skills. You select two more skills at level
	// 18th level."
	{FeatureSlug: "clan/nara/feature/masterwork-skill", ChoiceIndex: 0, Kind: ChoiceSkillProficiency, MinLevel: 7, Label: "Masterwork Skill — 7th level, first pick"},
	{FeatureSlug: "clan/nara/feature/masterwork-skill", ChoiceIndex: 1, Kind: ChoiceSkillProficiency, MinLevel: 7, Label: "Masterwork Skill — 7th level, second pick"},
	{FeatureSlug: "clan/nara/feature/masterwork-skill", ChoiceIndex: 2, Kind: ChoiceSkillProficiency, MinLevel: 18, Label: "Masterwork Skill — 18th level, first pick"},
	{FeatureSlug: "clan/nara/feature/masterwork-skill", ChoiceIndex: 3, Kind: ChoiceSkillProficiency, MinLevel: 18, Label: "Masterwork Skill — 18th level, second pick"},

	// Non-Clan "Self-Taught Skills": one pick each at 1st/7th/15th level.
	{FeatureSlug: "clan/non-clan/feature/self-taught-skills", ChoiceIndex: 0, Kind: ChoiceSkillProficiency, MinLevel: 1, Label: "Self-Taught Skills — 1st level"},
	{FeatureSlug: "clan/non-clan/feature/self-taught-skills", ChoiceIndex: 1, Kind: ChoiceSkillProficiency, MinLevel: 7, Label: "Self-Taught Skills — 7th level"},
	{FeatureSlug: "clan/non-clan/feature/self-taught-skills", ChoiceIndex: 2, Kind: ChoiceSkillProficiency, MinLevel: 15, Label: "Self-Taught Skills — 15th level"},

	// Scout-Nin "Canny": "Choose any one skill. You gain proficiency in this
	// skill."
	{FeatureSlug: "class/scout-nin/feature/canny-1-st-level", ChoiceIndex: 0, Kind: ChoiceSkillProficiency, MinLevel: 1, Label: "Canny"},

	// Interrogationist "Perfect Mind": "You gain proficiency in Wisdom or
	// Charisma Saving throws (Pick one)."
	{FeatureSlug: "class/intelligence-operative/group/master-strategies/interrogationist/feature/perfect-mind", ChoiceIndex: 0, Kind: ChoiceSavingThrowProficiency, MinLevel: 17, Label: "Perfect Mind", Options: []string{"wis", "cha"}},

	// The three pick-one ability-score increases found by the 2026-08-16
	// full-corpus sweep (see scores.go's own doc comment for the sweep and
	// for the three FIXED increases that need no player pick). Each
	// MinLevel restates the granting feature's own level — redundant with
	// LoadGrantedFeatures' level filter, but self-documenting and cheap.
	//
	// Juggernaut "Intelligent Design": "increase either your Strength or
	// Dexterity scores (and their maximum) by +2 (Pick one; This cannot be
	// changed)."
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design", ChoiceIndex: 0, Kind: ChoiceAbilityScoreIncrease, MinLevel: 10, Label: "Intelligent Design — permanently increase Strength or Dexterity by +2", Options: []string{"str", "dex"}, Amount: 2, RaisesMax: true},

	// Battle Dancer Form "Master of Aggression": "Your Strength or
	// Dexterity scores increase by 2. Your maximum for those scores
	// increases by 2."
	{FeatureSlug: "class/weapon-specialist/group/weapon-forms/battle-dancer-form/feature/master-of-aggression", ChoiceIndex: 0, Kind: ChoiceAbilityScoreIncrease, MinLevel: 20, Label: "Master of Aggression — permanently increase Strength or Dexterity by +2", Options: []string{"str", "dex"}, Amount: 2, RaisesMax: true},

	// Primal Weapon Form "Master of the Primal": "Your Strength or
	// Dexterity (your choice) and Intelligence scores increase by 2. Your
	// maximum for those scores also increases by 2." The fixed Intelligence
	// half is in scores.go; only the choice half is here.
	{FeatureSlug: "class/weapon-specialist/group/weapon-forms/primal-weapon-form/feature/master-of-the-primal", ChoiceIndex: 0, Kind: ChoiceAbilityScoreIncrease, MinLevel: 20, Label: "Master of the Primal — permanently increase Strength or Dexterity by +2", Options: []string{"str", "dex"}, Amount: 2, RaisesMax: true},

	// Puppet Master's 5 named free-Upgrade grants (see CLASS_AUDIT.md's
	// Puppet Master entry). Purple Technique Proficiency and Chakra String
	// Augments are each a ChoiceUpgradeEntry pick among a small, fixed,
	// already-known list; the class table's own 2nd-level subclass choice
	// is what MinLevel restates here, same "redundant with
	// LoadGrantedFeatures' own level filter, but self-documenting" note as
	// the ability-score entries above.
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/purple-technique-proficiency", ChoiceIndex: 0, Kind: ChoiceUpgradeEntry, MinLevel: 2, Label: "Purple Technique Proficiency — choose a free Upgrade", Options: []string{
		"class/puppet-master/option/armorers-upgrades/wood-tier/entry/chakra-blast",
		"class/puppet-master/option/armorers-upgrades/wood-tier/entry/integrated-weapon",
		"class/puppet-master/option/armorers-upgrades/wood-tier/entry/power-fist",
	}},
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/chakra-string-augments", ChoiceIndex: 0, Kind: ChoiceUpgradeEntry, MinLevel: 2, Label: "Chakra String Augments — choose a free Upgrade", Options: []string{
		"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/better-bonds",
		"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/chakra-pathway",
		"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/defensive-manipulation",
		"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/thread-weapon",
	}},

	// Nearly Perfected Architecture: "select two Armor Properties from
	// Chapter 5: Equipment. Your Juggernaut Armor gains these properties."
	// The free-Upgrade half of this same feature (2 Silver tier or lower)
	// is a bonus tier-cap slot, not a choice slot — see
	// cmd/n5e/puppet_upgrade_bonus_tiers.go.
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/nearly-perfected-architecture", ChoiceIndex: 0, Kind: ChoiceArmorProperty, MinLevel: 17, Label: "Nearly Perfected Architecture — first Armor Property"},
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/nearly-perfected-architecture", ChoiceIndex: 1, Kind: ChoiceArmorProperty, MinLevel: 17, Label: "Nearly Perfected Architecture — second Armor Property"},

	// Symphony of Puppetry: "Select one of the following options:
	// Expansion... Enhancement..." — see cmd/n5e/puppets.go's own
	// resolution of each branch's further effects.
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/red-technique-performer/feature/symphony-of-puppetry", ChoiceIndex: 0, Kind: ChoiceSymphonyBranch, MinLevel: 17, Label: "Symphony of Puppetry", Options: []string{"expansion", "enhancement"}},

	// Elevated Design: "All Puppet Tools you possess gain a +2 to two
	// different ability scores." Two independent slots, same "pick N
	// separately" shape Nearly Perfected Architecture's own two Armor
	// Property slots use just above.
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design", ChoiceIndex: 0, Kind: ChoiceCompanionAbilityScoreIncrease, MinLevel: 17, Label: "Elevated Design — first ability (+2, max 22, applies to all Puppet Tools)", Options: []string{"str", "dex", "con", "int", "wis", "cha"}, Amount: 2},
	{FeatureSlug: "class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design", ChoiceIndex: 1, Kind: ChoiceCompanionAbilityScoreIncrease, MinLevel: 17, Label: "Elevated Design — second ability (+2, max 22, applies to all Puppet Tools)", Options: []string{"str", "dex", "con", "int", "wis", "cha"}, Amount: 2},
}

// classSlugOf returns the owning class's own slug ("class/<slug>") for a
// class or subclass feature slug, or "" for a clan feature — both class and
// subclass feature slugs share the same "class/<slug>/..." prefix, so no
// subclass-to-parent-class lookup is needed here.
func classSlugOf(featureSlug string) string {
	if !strings.HasPrefix(featureSlug, "class/") {
		return ""
	}
	parts := strings.SplitN(featureSlug, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// ResolveChoiceSlots returns every choice slot the character currently
// qualifies for — the granting feature is actually present in granted, and
// the slot's own MinLevel has been reached — regardless of whether it's
// already been picked. classLevels keys off each character_classes.class_slug
// (the same slug shape LoadGrantedFeatures' own classLevels map uses
// internally); characterLevel is the character's summed total level.
func ResolveChoiceSlots(granted []GrantedFeatureRow, classLevels map[string]int, characterLevel int) []ChoiceSlot {
	grantedSlugs := make(map[string]bool, len(granted))
	for _, g := range granted {
		grantedSlugs[g.Slug] = true
	}
	var out []ChoiceSlot
	for _, def := range choiceSlotDefs {
		if !grantedSlugs[def.FeatureSlug] {
			continue
		}
		level := characterLevel
		if cs := classSlugOf(def.FeatureSlug); cs != "" {
			level = classLevels[cs]
		}
		if level < def.MinLevel {
			continue
		}
		out = append(out, ChoiceSlot{
			FeatureSlug: def.FeatureSlug, ChoiceIndex: def.ChoiceIndex,
			Kind: def.Kind, Label: def.Label, Options: def.Options, Amount: def.Amount,
			RaisesMax: def.RaisesMax,
		})
	}
	return out
}

// PendingChoiceSlots filters slots down to the ones with no resolved pick
// yet — what a picker UI needs to show.
func PendingChoiceSlots(slots []ChoiceSlot, resolved map[ChoiceKey]string) []ChoiceSlot {
	var out []ChoiceSlot
	for _, s := range slots {
		if v, ok := resolved[ChoiceKey{s.FeatureSlug, s.ChoiceIndex}]; !ok || v == "" {
			out = append(out, s)
		}
	}
	return out
}

// ResolveChoiceGrants turns every RESOLVED slot's stored pick into a
// ProficiencyGrant, the same shape ResolveProficiencyGrants returns for
// fixed grants — internal/charsheet.Compute folds both into profSkills/
// profSaves the same way.
func ResolveChoiceGrants(slots []ChoiceSlot, resolved map[ChoiceKey]string) []ProficiencyGrant {
	var out []ProficiencyGrant
	for _, s := range slots {
		value, ok := resolved[ChoiceKey{s.FeatureSlug, s.ChoiceIndex}]
		if !ok || value == "" {
			continue
		}
		switch s.Kind {
		case ChoiceSkillProficiency:
			out = append(out, ProficiencyGrant{GrantSkill, value})
		case ChoiceSavingThrowProficiency:
			out = append(out, ProficiencyGrant{GrantSavingThrow, value})
		}
	}
	return out
}

// ResolveChoiceAbilityScoreGrants turns every RESOLVED ability-score choice
// slot's stored pick into an AbilityScoreGrant, the same shape
// ResolveAbilityScoreGrants returns for fixed increases — Compute folds
// both into the ability scores the same way. A stored value that isn't in
// the slot's own Options (a rules rename, a hand-edited row) contributes
// nothing rather than raising an arbitrary score.
func ResolveChoiceAbilityScoreGrants(slots []ChoiceSlot, resolved map[ChoiceKey]string) []AbilityScoreGrant {
	var out []AbilityScoreGrant
	for _, s := range slots {
		if s.Kind != ChoiceAbilityScoreIncrease {
			continue
		}
		value, ok := resolved[ChoiceKey{s.FeatureSlug, s.ChoiceIndex}]
		if !ok || value == "" {
			continue
		}
		for _, opt := range s.Options {
			if opt == value {
				out = append(out, AbilityScoreGrant{Ability: value, Amount: s.Amount, RaisesMax: s.RaisesMax})
				break
			}
		}
	}
	return out
}

// LoadFeatureChoices returns every choice this character has already
// resolved, keyed the same way character_feature_choices' own composite key
// is shaped.
func LoadFeatureChoices(charDB *sql.DB, characterID int64) (map[ChoiceKey]string, error) {
	rows, err := charDB.Query(
		`SELECT feature_slug, choice_index, value FROM character_feature_choices WHERE character_id = ?`,
		characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ChoiceKey]string{}
	for rows.Next() {
		var k ChoiceKey
		var v string
		if err := rows.Scan(&k.FeatureSlug, &k.ChoiceIndex, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
