// Curated, hand-verified tables of FIXED (no player choice involved) numeric
// grants a class/subclass/clan feature confers — proficiency, an
// AC-ability-swap, or a flat Speed bonus. Same convention cmd/n5e's
// passiveTraitGrants/senseGrants already established: every entry here was
// read in full against the real book text, not regex-matched, because the
// same "gain proficiency"/"your speed increases" phrasing recurs constantly
// in text that is NOT a fixed grant — a player choice ("choose any one
// skill"), a companion/summon-targeted benefit, or a proficiency tied to an
// unspecified pick ("gain proficiency in an additional 2 Toolkits"). Those
// are deliberately excluded here; they need a picker UI, not a curated
// table (see the project plan's Phase 2).
//
// Feats are out of scope — see this package's own doc comment.
package features

import "strings"

// ProficiencyGrantKind mirrors character_proficiencies.kind (minus
// 'language', which no feature in the audited text ever grants).
type ProficiencyGrantKind string

const (
	GrantSkill       ProficiencyGrantKind = "skill"
	GrantSavingThrow ProficiencyGrantKind = "saving_throw"
)

// ProficiencyGrant is one fixed skill or saving-throw proficiency a feature
// grants outright. Value is a real SkillAbility key for GrantSkill, or a
// 3-letter ability code for GrantSavingThrow — internal/charsheet.Compute
// merges these directly into the same maps loadProficiencies produces, so
// the spelling has to match exactly.
type ProficiencyGrant struct {
	Kind  ProficiencyGrantKind
	Value string
}

// fixedProficiencyGrants covers every class/subclass/clan feature found in
// a full read of rules.db's "gain proficiency" text that grants a SKILL or
// SAVING THROW with no player choice involved. Features in this same text
// that grant a fixed TOOL/WEAPON/ARMOR proficiency (e.g. Hoshigaki's own
// "Brute Strength" also granting weapon-property benefits, Medical-Nin's
// Combat Bracers) are display-only today — nothing in this app computes a
// tool/weapon check, so they're left for the Tool Proficiencies panel to
// pick up later rather than modeled here; only what changes an actual
// computed number is in this table for now.
var fixedProficiencyGrants = map[string][]ProficiencyGrant{
	"class/hunter-nin/group/hunters-creeds/blade-warden/feature/wardens-proficiency": {
		{GrantSkill, "Athletics"}, {GrantSkill, "Intimidation"},
	},
	"class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/medical-proficiency": {
		{GrantSkill, "Medicine"},
	},
	"class/hunter-nin/group/hunters-creeds/grave-stalker/feature/stalkers-proficiency": {
		{GrantSkill, "Sleight of Hand"}, {GrantSkill, "Illusions"},
	},
	"class/hunter-nin/group/hunters-creeds/arsenalist/feature/arsenals-proficiency": {
		{GrantSkill, "Crafting"}, // the kit choice alongside this is player-picked, not modeled here
	},
	"class/hunter-nin/group/hunters-creeds/undertaker/feature/toxic-proficiency": {
		{GrantSkill, "Deception"}, // Disguise/Poison Kit grants alongside this are tool-only, not modeled here
	},
	"class/hunter-nin/group/hunters-creeds/vice-agent/feature/sins-proficiency": {
		{GrantSkill, "Persuasion"}, {GrantSkill, "Insight"},
	},
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/stalker-proficiency": {
		{GrantSkill, "Ninshou"}, {GrantSkill, "Survival"},
	},
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/wolfs-proficiency": {
		{GrantSkill, "Martial Arts"}, {GrantSkill, "Insight"},
	},
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-will": {
		{GrantSavingThrow, "cha"},
	},
	// The intelligence-operative Strategy features below all read "gain
	// proficiency in X, if you weren't previously — if you already are,
	// instead gain proficiency in another [ability]-based skill." The
	// escalation half is a player choice (which OTHER skill) and isn't
	// modeled — only the base, unconditional grant is.
	"class/intelligence-operative/group/master-strategies/azure-analyst/feature/azure-research": {
		{GrantSkill, "Insight"},
	},
	"class/intelligence-operative/group/master-strategies/calculated-strategist/feature/conflict-book": {
		{GrantSkill, "Perception"},
	},
	"class/intelligence-operative/group/master-strategies/grave-controller/feature/twisted-anatomy": {
		{GrantSkill, "Medicine"},
	},
	"class/intelligence-operative/group/master-strategies/interrogationist/feature/info-wars": {
		{GrantSkill, "Intimidation"},
	},
	"class/scout-nin/group/scouting-technique/arbiter-scout/feature/master-of-arbitration": {
		{GrantSkill, "Deception"}, {GrantSkill, "Intimidation"}, {GrantSkill, "Persuasion"},
	},
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/phantasms-knowledge": {
		{GrantSkill, "Illusions"}, {GrantSkill, "Insight"}, {GrantSkill, "Stealth"},
	},
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/black-technique-proficiency": {
		{GrantSkill, "Chakra Control"}, {GrantSkill, "Stealth"},
	},
	"class/puppet-master/group/puppet-techniques/blue-technique-warmaster/feature/blue-technique-proficiency": {
		{GrantSkill, "Martial Arts"},
	},
	"class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/green-technique-proficiency": {
		{GrantSkill, "Chakra Control"}, // the Ninshou-or-Illusions half of this grant is a player choice
	},
	"class/puppet-master/group/puppet-techniques/red-technique-performer/feature/red-technique-proficiency": {
		{GrantSkill, "Performance"}, {GrantSkill, "Insight"},
	},
	"class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/white-technique-proficiency": {
		{GrantSkill, "Martial Arts"}, {GrantSkill, "Medicine"},
	},
	// "While wearing your Juggernaut Armor, you are proficient in all
	// Strength skills" — Athletics and Martial Arts are the only two
	// Strength-keyed skills (internal/charsheet.SkillAbility). The
	// while-wearing-armor condition is treated as always-on for a
	// Juggernaut, the same reading passiveTraitGrants already applies to
	// this feature's own poison-immunity clause. The feature's other
	// clauses (Mastery on Strength saves, a permanent +2 to Strength or
	// Dexterity) have no mechanism here to land in — see CLASS_AUDIT.md's
	// Puppet Master entry.
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design": {
		{GrantSkill, "Athletics"}, {GrantSkill, "Martial Arts"},
	},
	"clan/hoshigaki/feature/brute-strength": {
		{GrantSkill, "Intimidation"},
	},
	"clan/uzumaki/feature/inhuman-lifeforce": {
		// The book escalates this to a flat +2 bonus instead of proficiency
		// if the character is somehow already proficient in Con saves from
		// another source — a rare interaction, not modeled; the base grant
		// (proficiency itself) always applies and is the common case.
		{GrantSavingThrow, "con"},
	},
	// "At 15th level, your intense training grants you proficiency in
	// Constitution saving throws." This feature's second clause (add
	// Dexterity modifier to non-proficient saves) is modeled separately, in
	// nonProficientSaveBonusGrants below.
	"class/taijutsu-specialist/feature/perfect-body": {
		{GrantSavingThrow, "con"},
	},
}

// ResolveProficiencyGrants returns every fixed skill/saving-throw
// proficiency the character's granted features confer, keyed by feature
// slug against fixedProficiencyGrants.
func ResolveProficiencyGrants(granted []GrantedFeatureRow) []ProficiencyGrant {
	var out []ProficiencyGrant
	for _, f := range granted {
		out = append(out, fixedProficiencyGrants[f.Slug]...)
	}
	return out
}

// nonProficientSaveBonusGrants covers a feature whose text adds a fixed
// ability modifier to every saving throw the character is NOT already
// proficient in — distinct from fixedProficiencyGrants' "gain proficiency"
// shape above, which changes which saves get the proficiency bonus rather
// than adding a flat modifier to the ones that don't. Value is a 3-letter
// ability code.
var nonProficientSaveBonusGrants = map[string]string{
	// "Additionally, whenever you make a saving throw, with an ability you
	// are not proficient in, add your Dexterity Modifier to the saving
	// throw." The Constitution-save-proficiency half of this same feature is
	// modeled above, in fixedProficiencyGrants.
	"class/taijutsu-specialist/feature/perfect-body": "dex",
}

// ResolveNonProficientSaveBonusAbility returns the 3-letter ability code to
// add to every saving throw the character isn't proficient in, or "" if no
// granted feature applies. If more than one grant applies (a homebrew
// multi-source combination the book never anticipated), the first found
// wins, the same precedent ResolveACSwapAbility already sets.
func ResolveNonProficientSaveBonusAbility(granted []GrantedFeatureRow) string {
	for _, f := range granted {
		if ability, ok := nonProficientSaveBonusGrants[f.Slug]; ok {
			return ability
		}
	}
	return ""
}

// acSwapGrant is one feature that lets its ability substitute for Dexterity
// in the AC formula. armorCategory gates Konjiki's grant (light/medium
// armor only); the empty string means "always applies once granted."
type acSwapGrant struct {
	Ability       string // 3-letter code
	ArmorCategory string // "", "light", or "medium" — "" means any/unarmored too
}

var acSwapGrants = map[string]acSwapGrant{
	"clan/hoshigaki/feature/shark-skinned-predator":                                       {Ability: "con"},
	"class/medical-nin/group/tenets-of-medicine/combat-medic/feature/competent-combatant": {Ability: "wis"},
	"clan/konjiki/feature/blood-of-the-earth":                                             {Ability: "int", ArmorCategory: "light_or_medium"},
}

// ResolveACSwapAbility returns the 3-letter ability code that may substitute
// for Dexterity in the AC formula, or "" if no granted feature applies given
// the character's current equippedArmorCategory ("", "light", "medium", or
// "heavy" — "" covers both no armor and armor with no recorded category).
// Callers take the BETTER of the normal and swapped AC, never worse — see
// internal/charsheet.Compute, which recomputes AC with the substitution
// applied and keeps the higher result. If more than one grant applies (a
// homebrew multi-clan/class combination the book never anticipated), the
// first found wins — real cross-source stacking here has no book rule to
// follow.
func ResolveACSwapAbility(granted []GrantedFeatureRow, equippedArmorCategory string) string {
	for _, f := range granted {
		g, ok := acSwapGrants[f.Slug]
		if !ok {
			continue
		}
		if g.ArmorCategory == "light_or_medium" && equippedArmorCategory != "light" && equippedArmorCategory != "medium" {
			continue
		}
		return g.Ability
	}
	return ""
}

// flatACBonusGrant is one feature that adds a flat, always-on bonus to AC —
// either derived from an ability modifier (Ability set, Divisor applied) or
// a plain integer (Flat set, Ability left ""). Distinct from acSwapGrants'
// "substitute an ability for Dexterity in the formula" shape above, and from
// speedGrant's flat integer (that one is Speed, not AC). Divisor lets a
// "half of X modifier, rounded down" grant (Arbiter Scout's Absolute
// Authority) share this table with a future full-modifier grant (Divisor 1)
// instead of needing its own separate mechanism. ArmorCategory reuses
// acSwapGrant's "light_or_medium" gate value for a grant conditioned on the
// character's currently equipped armor.
type flatACBonusGrant struct {
	Ability       string // 3-letter code; "" means Flat applies instead
	Divisor       int    // 1 for the full modifier, 2 for "half, rounded down"
	Flat          int    // used when Ability == ""
	ArmorCategory string // "" (always applies), or "light_or_medium"
}

var flatACBonusGrants = map[string]flatACBonusGrant{
	// "You and allied creatures within 30 feet of you, can add Half of your
	// Charisma Modifier (Rounded Down) to their AC." Only the character's
	// own half is reachable here — no companion/ally AC mechanism exists in
	// this app to extend the 30-foot-radius half to.
	"class/scout-nin/group/scouting-technique/arbiter-scout/feature/absolute-authority": {Ability: "cha", Divisor: 2},
	// "+1 Bonus to AC, if integrated into Light or Medium Armor." The other
	// two bullets of this same feature (Chakra damage resistance; adding the
	// unarmed combat die to Chakra Control checks) are modeled elsewhere
	// (passiveTraitGrants) or left manual (a die-roll bonus has no flat-
	// modifier home here) — see this feature's audit notes.
	"class/taijutsu-specialist/group/taijutsu-style/ruin/feature/battle-ready-catalyst": {Flat: 1, ArmorCategory: "light_or_medium"},
}

// ResolveFlatACBonus sums every granted feature's flat AC bonus from
// abilityMods (the character's own already-computed ability MODIFIERS, not
// raw scores — this package never re-derives a modifier from a score itself,
// same boundary ResolveSpeedBonus/ResolveACSwapAbility already draw) and
// equippedArmorCategory ("", "light", "medium", or "heavy" — same values
// ResolveACSwapAbility takes), gating any ArmorCategory-restricted grant.
// floorDivRoundDown, not Go's truncating "/", matches "rounded down" for a
// negative modifier too (e.g. a -3 Charisma modifier's half is -2, not -1).
func ResolveFlatACBonus(granted []GrantedFeatureRow, abilityMods map[string]int, equippedArmorCategory string) int {
	total := 0
	for _, f := range granted {
		g, ok := flatACBonusGrants[f.Slug]
		if !ok {
			continue
		}
		if g.ArmorCategory == "light_or_medium" && equippedArmorCategory != "light" && equippedArmorCategory != "medium" {
			continue
		}
		if g.Ability == "" {
			total += g.Flat
			continue
		}
		total += floorDivRoundDown(abilityMods[g.Ability], g.Divisor)
	}
	return total
}

// floorDivRoundDown is integer division that rounds toward negative
// infinity (floor), unlike Go's "/" operator, which truncates toward zero.
func floorDivRoundDown(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// passivePerceptionBonusGrants covers a feature whose text adds a fixed
// bonus to passive Wisdom (Perception) specifically — Perceptive ("+5
// bonuses to your passive Wisdom (Perception) and passive Intelligence
// (Investigation) scores") and Elite Awareness ("+5 bonuses to your passive
// perception and Insight"). Only the Perception half of each feat's clause
// is modeled: sheet.PassivePerception (internal/charsheet) is the only
// passive-score field that exists — no PassiveInsight/PassiveInvestigation
// field exists anywhere in the sheet to feed the other half.
var passivePerceptionBonusGrants = map[string]int{
	"feat/perceptive":      5,
	"feat/elite-awareness": 5,
}

// ResolvePassivePerceptionBonus sums every granted feature's flat bonus to
// passive Wisdom (Perception) from passivePerceptionBonusGrants.
func ResolvePassivePerceptionBonus(granted []GrantedFeatureRow) int {
	total := 0
	for _, f := range granted {
		total += passivePerceptionBonusGrants[f.Slug]
	}
	return total
}

// speedGrant is one level threshold of a feature's Speed bonus. Amount is
// the TOTAL bonus effective from MinLevel on, not a delta — the source text
// itself states each tier as a running total (e.g. Taijutsu-Specialist's
// Enhanced Movement: "+10 feet... increases to 15 feet at 6th level..."),
// and Namikaze's incremental "+5, then an additional +5" phrasing is
// pre-summed into the same shape here so both features resolve the same
// way: pick the highest MinLevel tier the character has reached.
type speedGrant struct {
	FeatureSlug           string
	MinLevel              int
	Amount                int
	RequiresNotHeavyArmor bool
}

// speedGrants is intentionally a slice, not a map keyed by slug: several
// entries share one slug, one per level tier.
var speedGrants = []speedGrant{
	// Scout-Nin's Mobility (one of Jack of All, Master of None's 5
	// Generalizations, granted starting 5th level): "+10 bonus to your
	// speed... increases to +15... at 11th level." The MinLevel 11 tier
	// used to be the only one modeled, back when Mobility's own
	// class_features row was still mistagged level=11 (a separate data bug,
	// fixed by migration 0051_scout_nin_null_level_features.sql) — that made
	// this entry's threshold coincidentally match the row's own
	// (wrong) blanket-grant level instead of representing the real
	// "+15 at 11th" tier. Now that the row is correctly tagged level=5, both
	// tiers are modeled: +10 from 5th, +15 from 11th. Like every other
	// Jack of All Generalization, this bonus applies once the class_features
	// row itself is granted (blanket, not gated on the player having
	// specifically picked Mobility over Combat/Control/Skill/Support in the
	// cap+catalog pick — see cmd/n5e/scout_nin.go's own doc comment for why
	// that boundary matches this codebase's existing precedent).
	{FeatureSlug: "class/scout-nin/feature/mobility", MinLevel: 5, Amount: 10},
	{FeatureSlug: "class/scout-nin/feature/mobility", MinLevel: 11, Amount: 15},

	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 2, Amount: 10, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 6, Amount: 15, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 9, Amount: 20, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 13, Amount: 25, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 17, Amount: 30, RequiresNotHeavyArmor: true},

	{FeatureSlug: "clan/namikaze/feature/supernatural-speed", MinLevel: 1, Amount: 5},
	{FeatureSlug: "clan/namikaze/feature/supernatural-speed", MinLevel: 11, Amount: 10},
	{FeatureSlug: "clan/namikaze/feature/supernatural-speed", MinLevel: 18, Amount: 15},

	// "Starting at 6th Level... you gain a +10-speed increase." No armor
	// restriction in the book text, unlike Enhanced Movement above — a
	// single flat tier, matching Namikaze's Supernatural Speed's shape. This
	// same feature's other clause (add Strength or Constitution Modifier to
	// initiative) is left unmodeled; no per-class initiative-bonus
	// automation exists anywhere in this app.
	{FeatureSlug: "class/weapon-specialist/group/weapon-forms/battle-dancer-form/feature/relentless", MinLevel: 6, Amount: 10},

	// Future of Shinobi: Sky Keeper (20th level, Storm Rider): "your
	// movement speed is increased by +60 feet." The same capstone's flying-
	// speed/hover and difficult-terrain clauses have no field anywhere in
	// this app to land in (no flying-speed or hover field exists) and stay
	// Group 3, same restraint already documented on the Regalia cap-2 half
	// of this capstone (cmd/n5e/science_nin_subclasses.go's
	// scienceNinStormRiderData).
	{FeatureSlug: "class/science-nin/group/scientific-inquiry/storm-rider/feature/the-future-of-shinobi-sky-keeper", MinLevel: 20, Amount: 60},

	// Feats — mergeFeatFeatures (cmd/n5e/characters.go) folds every
	// character_feats row into the same granted-features list this function
	// reads, keyed by the feat's own slug, so these resolve with no other
	// code change. Each is a flat, un-tiered bonus available from 1st level,
	// with no armor restriction in the feat's own text.
	{FeatureSlug: "feat/maneuverable", MinLevel: 1, Amount: 10},
	{FeatureSlug: "feat/mobile", MinLevel: 1, Amount: 10},
	// Slipstream: "Your speed increases by 10 feet. You increase your
	// movement speed by an additional 5 feet for each jutsu with the Wind
	// Release keyword you are concentrating on." Only the flat +10 base is
	// modeled here; the per-Wind-Release-jutsu-concentrated-on addition is
	// conditional and left as reference text — this app has no
	// concentration-tracking mechanism to key it off of.
	{FeatureSlug: "feat/fushin/slipstream", MinLevel: 1, Amount: 10},
	// Superior Speed: "Your speed Increased by 10 feet." The feat's other
	// clause ("+2 Speed Die") is a separate resource pool, not a Speed bonus
	// — see cmd/n5e/custom_resources.go's "feat/namikaze/superior-speed"
	// entry, keyed to the same "speed_die" Key as the base clan grant.
	{FeatureSlug: "feat/namikaze/superior-speed", MinLevel: 1, Amount: 10},
	{FeatureSlug: "feat/class/martial-arts-expert", MinLevel: 1, Amount: 5},
	{FeatureSlug: "feat/class/martial-arts-training", MinLevel: 1, Amount: 5},
	// Elite Agility: "Increase your initiative by +5 and your movement speed
	// by +10." Only the Speed half is modeled here — the Initiative half is
	// modeled in initiativeBonusGrants below instead.
	{FeatureSlug: "feat/elite-agility", MinLevel: 1, Amount: 10},
}

// ironcladFeaturePrefix identifies a character who took the Ironclad
// Taijutsu Style subclass — subclass features carry the full group-scoped
// slug, so any granted feature under this prefix (gained once the subclass
// is chosen at level 3) is a reliable signal, the same way acSwapGrants/
// fixedProficiencyGrants above key purely off granted feature slugs rather
// than a separate subclass parameter. Ironclad's own feature text overrides
// Enhanced Movement's heavy-armor restriction outright: "While wearing
// heavy armor, you still gain the benefits of Enhanced Movement."
const ironcladFeaturePrefix = "class/taijutsu-specialist/group/taijutsu-style/ironclad/"

// enhancedMovementFeatureSlug is the one speedGrants entry Ironclad's
// exception applies to — Namikaze's Supernatural Speed has no such armor
// gate to begin with, so there is nothing else for the exception to touch.
const enhancedMovementFeatureSlug = "class/taijutsu-specialist/feature/enhanced-movement"

// ResolveSpeedBonus sums every granted feature's Speed bonus at
// characterLevel, picking each feature's own highest-reached tier (not
// summing tiers within one feature) but summing ACROSS different features,
// since two different sources (e.g. two different clans/classes in an
// unusual multiclass) stacking is the normal case elsewhere in this engine.
// equippedArmorCategory gates Taijutsu-Specialist's grant the same way
// ResolveACSwapAbility's does Konjiki's; "" (no armor, or armor with no
// recorded category) counts as "not wearing Heavy Armor." An Ironclad
// character is exempt from that gate for Enhanced Movement specifically —
// see ironcladFeaturePrefix.
func ResolveSpeedBonus(granted []GrantedFeatureRow, characterLevel int, equippedArmorCategory string) int {
	isIronclad := false
	for _, f := range granted {
		if strings.HasPrefix(f.Slug, ironcladFeaturePrefix) {
			isIronclad = true
			break
		}
	}

	bestBySlug := map[string]int{}
	for _, f := range granted {
		for _, g := range speedGrants {
			if g.FeatureSlug != f.Slug || g.MinLevel > characterLevel {
				continue
			}
			exempt := isIronclad && g.FeatureSlug == enhancedMovementFeatureSlug
			if g.RequiresNotHeavyArmor && equippedArmorCategory == "heavy" && !exempt {
				continue
			}
			if g.Amount > bestBySlug[f.Slug] {
				bestBySlug[f.Slug] = g.Amount
			}
		}
	}
	total := 0
	for _, amount := range bestBySlug {
		total += amount
	}
	return total
}

// initiativeBonusGrant is one level threshold of a feature's flat Initiative
// bonus, same "Amount is the running total for that tier, not a delta" shape
// speedGrant uses — pick each feature's own highest-reached tier, then sum
// across distinct features.
type initiativeBonusGrant struct {
	FeatureSlug string
	MinLevel    int
	Amount      int
}

// initiativeBonusGrants is intentionally a slice, not a map keyed by slug:
// First in Line has two level tiers sharing one slug.
var initiativeBonusGrants = []initiativeBonusGrant{
	// First in Line (Cooking-Nin, Entremetier Chef): "Whenever you would roll
	// initiative add a +5 to your result. Starting at 11th level, this
	// becomes a +10." Only the flat bonus is modeled here — the same
	// feature's "Advantage on attacks against creatures that have not acted
	// this combat" clause stays Group 2/3 unmodeled; no "has this creature
	// acted yet" combat-state tracking exists anywhere in this app to key it
	// off of.
	{FeatureSlug: "class/cooking-nin/group/cooking-focus/entremetier-chef/feature/first-in-line", MinLevel: 5, Amount: 5},
	{FeatureSlug: "class/cooking-nin/group/cooking-focus/entremetier-chef/feature/first-in-line", MinLevel: 11, Amount: 10},
	// Elite Agility: "Increase your initiative by +5..." The Speed half of
	// this same feat is modeled in speedGrants above.
	{FeatureSlug: "feat/elite-agility", MinLevel: 1, Amount: 5},
}

// ResolveInitiativeBonus sums every granted feature's flat Initiative bonus
// at characterLevel, picking each feature's own highest-reached tier (not
// summing tiers within one feature) but summing across different features —
// same resolution shape as ResolveSpeedBonus, minus that function's
// armor-category gate, which no Initiative grant here needs.
func ResolveInitiativeBonus(granted []GrantedFeatureRow, characterLevel int) int {
	bestBySlug := map[string]int{}
	for _, f := range granted {
		for _, g := range initiativeBonusGrants {
			if g.FeatureSlug != f.Slug || g.MinLevel > characterLevel {
				continue
			}
			if g.Amount > bestBySlug[f.Slug] {
				bestBySlug[f.Slug] = g.Amount
			}
		}
	}
	total := 0
	for _, amount := range bestBySlug {
		total += amount
	}
	return total
}
