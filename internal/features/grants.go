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
	// Scout-Nin's Mobility also states a further increase "at 11th level" —
	// the same level the feature itself is already gained at, which reads
	// as a book error with no legible intended threshold. Only the base
	// +10 (unambiguous) is modeled; see the project plan for this class of
	// gap.
	{FeatureSlug: "class/scout-nin/feature/mobility", MinLevel: 11, Amount: 10},

	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 2, Amount: 10, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 6, Amount: 15, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 9, Amount: 20, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 13, Amount: 25, RequiresNotHeavyArmor: true},
	{FeatureSlug: "class/taijutsu-specialist/feature/enhanced-movement", MinLevel: 17, Amount: 30, RequiresNotHeavyArmor: true},

	{FeatureSlug: "clan/namikaze/feature/supernatural-speed", MinLevel: 1, Amount: 5},
	{FeatureSlug: "clan/namikaze/feature/supernatural-speed", MinLevel: 11, Amount: 10},
	{FeatureSlug: "clan/namikaze/feature/supernatural-speed", MinLevel: 18, Amount: 15},
}

// ResolveSpeedBonus sums every granted feature's Speed bonus at
// characterLevel, picking each feature's own highest-reached tier (not
// summing tiers within one feature) but summing ACROSS different features,
// since two different sources (e.g. two different clans/classes in an
// unusual multiclass) stacking is the normal case elsewhere in this engine.
// equippedArmorCategory gates Taijutsu-Specialist's grant the same way
// ResolveACSwapAbility's does Konjiki's; "" (no armor, or armor with no
// recorded category) counts as "not wearing Heavy Armor."
func ResolveSpeedBonus(granted []GrantedFeatureRow, characterLevel int, equippedArmorCategory string) int {
	bestBySlug := map[string]int{}
	for _, f := range granted {
		for _, g := range speedGrants {
			if g.FeatureSlug != f.Slug || g.MinLevel > characterLevel {
				continue
			}
			if g.RequiresNotHeavyArmor && equippedArmorCategory == "heavy" {
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
