package puppetupgrades

import "strconv"

// This file is the Puppet Master class's other big always-on-effect table,
// alongside bonuses.go — but for the 4 mandatory, one-time 2nd-level
// "build your puppet" picks (Puppeteer Chassis/Black, Puppet Frameworks/
// Green, Puppet Roles/Red, Puppet Weapon Types/Blue) rather than the
// Upgrade catalog. Each of these 17 rows sets a companion's ability
// scores, size, and (usually) a natural weapon, and Roles additionally
// grant a bonus skill proficiency and an always-on Proficiency-Bonus-scaled
// "Role Effect."
//
// Same "hand-curated, not a text parser" precedent as bonuses.go: the ASI
// prose alone takes 4 different shapes across these 17 entries ("Strength
// & Constitution scores become 16", "+2 to Constitution score, +1 to
// another ability score", "+2 to Strength or Dexterity score", "+2 to any
// ability score of your choice") with no single reliable grammar. Every
// entry's full raw text was read directly from dist/rules.db
// (class_options, list_name in ('Puppeteer Chassis','Puppet Frameworks',
// 'Puppet Roles','Puppet Weapon Types')) before writing this table.
//
// Same Group 1/2/3 split as bonuses.go's own header comment: ability
// scores, size, natural weapons, bonus skills, and the Role Effects that
// reach a real field (AC, attack rolls, crit range, a skill's own ability)
// are computed here. Everything else each entry's text also grants —
// resistances, weapon/armor proficiencies, grapple/lift bonuses, a
// remote-view action, long-rest weapon-mode properties, Controller's/
// Supporter's own spelled-out Proficiency Bonus formulas — has no tracked
// field or combat-resolution engine to compute into, so it's carried as
// ReferenceTraits: real, curated, always-visible text, not silently
// dropped, following the exact same treatment puppetToolStatBlock's own
// BaseTraits block already gives the class-wide baseline's resistances/
// immunities/proficiencies.
//
// Deliberately NOT modeled at all (own subsystem-scale gaps, flagged in
// CLASS_AUDIT.md, not silently folded in here):
//   - Bestial Framework's Sage Creature pick and its 3-level D-Rank trait
//     grants ARE modeled (SageCreatureFramework, cmd/n5e/
//     puppet_bestial_framework.go), reusing the existing Summoning
//     Technique tribe/feature catalog.
//   - Matryoshka Framework's own multi-body split/merge (SplitBodyFramework,
//     charstore.SplitCompanionIntoBodies/MergeCompanionBodies) IS modeled
//     as a real companion-multiplicity action — HP division across bodies
//     and a plain editable "known jutsu slots" counter. Radio Link
//     networking, the bonus-action delegate-to-body ability, and "command
//     only one body per turn" remain reference-only text (this app has no
//     turn-by-turn action economy to enforce any of the three against).
//   - Lurker's "add Proficiency Bonus to damage on a crit" is modeled via
//     RoleEffectSpec.CritDamageBonus, correlated client-side against the
//     Attack roll immediately preceding a Damage roll on the same row (see
//     cmd/n5e/static/js/dice-roller.js's onResult and character-sheet.js).
//   - Shade/Spellblade Framework's granted D/C/B-Rank jutsu picks are
//     modeled as a real 3-slot sub-choice pick, not left as reference text
//     (see the entries themselves, below, and
//     cmd/n5e/puppet_foundation.go's shadeFrameworkJutsuOptions/
//     spellbladeFrameworkJutsuOptions).

// AbilityChoiceGroup is one "your choice of ability" component of an
// entry's ASI text, resolved through a real per-companion sub-choice pick
// (cmd/n5e/puppets.go's existing puppetUpgradeSubChoiceSpecs mechanism),
// never guessed. An entry with more than one group (Controller, Bestial
// Framework, Sentinel) is matched POSITIONALLY against the companion's own
// ordered SubChoicePicks (oldest pick first, same order
// puppetUpgradeSubChoiceSpec's existing consumers already return them in) —
// simpler than a named-group refactor of that shared mechanism, and safe
// because every entry needing more than one group offers the SAME choice
// list for each (e.g. Controller's own "+2 to one, +1 to another" both draw
// from all 6 abilities), so which physical pick maps to which group only
// changes which Delta/Floor/Set applies, never which options are offered.
// Exactly one of Delta/Set is meaningful per group — Delta adds to the
// chosen ability (after Floor, if set); Set overrides it outright
// (Supporter's "Intelligence or Wisdom start at 10" — a floor on whichever
// one is chosen, not an addition).
type AbilityChoiceGroup struct {
	Choices []string // eligible ability keys ("str","dex","con","int","wis","cha")
	Delta   int
	Floor   int // floor the baseline to at least this value before adding Delta; 0 = no floor
	Set     int // if nonzero, sets the chosen ability to this value outright instead of adding Delta
}

// NaturalWeaponSpec is the one natural weapon a Chassis/Weapon Type entry's
// own text grants — Frameworks other than Bestial (whose weapon depends on
// an unmodeled Sage Creature choice) and every Role have none (nil).
type NaturalWeaponSpec struct {
	Name              string
	Reach             string // display only, e.g. "5ft.", "90/180ft."
	Property          string // display only, e.g. "Ammunition", "Unarmed"
	DamageCount       int
	DamageSides       int
	DamageAbility     string // "str" or "dex" — which of the companion's OWN scores the text reads
	AddProficiency    bool   // most read "+ ability mod + your Proficiency Bonus"; a few omit the Proficiency Bonus term
	DamageType        string // blank when DamageTypeChoices is set instead (Specialized's Slam)
	DamageTypeChoices []string
	CritText          string // the "on a roll of 17-20..." clause, reference-only
}

// RoleEffectSpec is a Puppet Role's own "Role Effect" — 6 mechanically
// distinct, always-on, Proficiency-Bonus-scaled formulas, each resolved as
// far as a real hook in this app allows.
type RoleEffectSpec struct {
	// ACBonus/AttackRollBonus reach a real tracked field/formula — folded
	// into the same computed total every other AC/attack-roll bonus in this
	// app uses.
	ACBonus         func(profBonus int) int // Defender
	AttackRollBonus func(profBonus int) int // Striker
	// SkillAbilityOverride swaps which ability a named skill reads for THIS
	// companion — Surveyor's "may use Wisdom in place of Intelligence for
	// Investigation."
	SkillAbilityOverride map[string]string
	// CritRangeBonus widens the companion's attack-roll crit threshold —
	// Lurker's own +1 (20 becomes 19-20).
	CritRangeBonus int
	// CritDamageBonus reaches a real client-side hook (see SM-D,
	// dice-roller.js's onResult) — Lurker's own "on a critical hit, add
	// your Proficiency Bonus to the damage dealt," folded into the Damage
	// roll's modifier when the immediately-preceding Attack roll on the
	// same row crit. nil for every Role but Lurker; TextBonus (below) is
	// kept as the reference-text fallback for anywhere this doesn't reach.
	CritDamageBonus func(profBonus int) int
	// TextBonus is a real, fully-computed number with nowhere live to put
	// it (Controller's Save DC bonus, Supporter's healing bonus, Lurker's
	// crit damage bonus) — the sentence is pre-computed here, not left as a
	// fraction of Proficiency Bonus for the player to work out by hand.
	TextBonus func(profBonus int) string
}

// FoundationEntry is one of the 17 Chassis/Framework/Role/Weapon Type
// options.
type FoundationEntry struct {
	EntrySlug string // == class_options.slug (these are untiered picks — see puppets.go's own doc)
	Name      string
	Catalog   string // "Puppeteer Chassis" | "Puppet Framework" | "Puppet Role" | "Puppet Weapon Type"

	AbilitySets         map[string]int // "con": 16 — "becomes/starts at X" (floor, applied before AbilityDeltas)
	AbilityDeltas       map[string]int // "str": 2  — flat +N to a NAMED ability
	AbilityChoiceGroups []AbilityChoiceGroup

	Size        string   // fixed size; blank if only SizeChoices/no size text at all
	SizeChoices []string // player-chosen size (Specialized, Sentinel) — resolved via sub-choice
	// SizeAtLevel: level-gated size escalation (Bestial Framework Large at
	// 10th, Ogre Huge at 14th) — keys are the Puppet Master level it takes
	// effect, read against the character's OWN class level.
	SizeAtLevel map[int]string

	SpeedDelta          int  // flat, additive — folds into the same total.Speed every other bonus uses
	SpeedSet            int  // absolute override (0 = not set) — Quadrupedal's "Movement Speed becomes 35 feet"
	FlySpeedEqualsSpeed bool // grants a flying speed equal to the FINAL computed speed (Winged, Drone)

	// FlatACBonus/FlatAttackRollBonus are a fixed (non-Proficiency-scaled)
	// bonus straight from an entry's own text (Spellblade's own +1 AC,
	// Sentinel's own +1 to attack rolls with wielded weapons) — folded the
	// same way RoleEffectSpec's scaled versions are.
	FlatACBonus         int
	FlatAttackRollBonus int

	NaturalWeapon *NaturalWeaponSpec

	// BonusSkills is a flat skill-proficiency grant — Roles' own "Bonus
	// Skill Proficiency" line, but also used by Drone Weapon (its own 4
	// skills). Folded into puppet_skills.go's puppetFlatSkillGrants map by
	// entry slug.
	BonusSkills []string
	RoleEffect  *RoleEffectSpec

	// FixedGrantEntrySlug: the ONE Upgrade entry this pick grants for free,
	// outside the Upgrade cap — same display-only "merge into the tier
	// list, no stored row" treatment puppetUpgradeAutoGrants already gives
	// its own 2 entries, just keyed by this catalog pick instead of a
	// subclass feature+level.
	FixedGrantEntrySlug string
	// BonusTierSlots: a genuine capacity increase rather than a fixed
	// entry — Specialized's own "2 free Wood tier Upgrades of your
	// choice."
	BonusTierSlots map[string]int

	// ReferenceTraits: every other clause this entry's own text grants,
	// with no computed hook to land in — see this file's header comment.
	ReferenceTraits []string

	// SageCreatureFramework marks Bestial Framework specifically — its own
	// sub-choices aren't just AbilityChoiceGroups (the generic auto-builder
	// puppetFoundationSubChoiceSpec covers) but ALSO a Sage Creature tribe
	// pick and 3 level-gated D-Rank trait picks from that tribe's own
	// catalog (cmd/n5e/puppet_foundation.go's bestialFrameworkSubChoice
	// mechanism, which reuses the existing Summoning Technique
	// summon_tribes catalog rather than building a new one). This flag only
	// affects puppetFoundationMixedChoiceKinds' own kind count (so the
	// shared ability-pick reader knows to expect a "ability:"-prefixed
	// slug, same as any other entry offering more than one kind of
	// sub-choice) — everything else about the tribe/trait picks is wired
	// directly in cmd/n5e, not through this package.
	SageCreatureFramework bool

	// SplitBodyFramework marks Matryoshka Framework specifically — its own
	// puppet can split into 1-3 independent companion rows sharing a
	// matryoshka_group_id (charstore.SplitCompanionIntoBodies/
	// MergeCompanionBodies) via a Split/Merge action on the Puppets tab,
	// not a sub-choice pick at all. This flag only marks the entry for that
	// UI action; the split/merge mechanism itself lives entirely in
	// cmd/n5e and internal/charstore.
	SplitBodyFramework bool
}

// Fixed-grant entry slugs, matching cmd/n5e/puppets.go's own unexported
// constants of the same value (that package can't be imported from here,
// and this package can't be imported there without an import cycle through
// cmd/n5e's already-heavy dependency on internal/puppetupgrades — so the
// slug strings are duplicated, not shared, same as any other cross-package
// rules-data constant in this app).
const (
	bulkyBuildGrantSlug          = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/bulky-build"
	quickfootedGrantSlug         = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/quickfooted"
	ghillieCoatingGrantSlug      = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/ghillie-coating"
	warfareAugmentationGrantSlug = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/warfare-augmentation"
)

func abilityCeilThird(profBonus int) int {
	// "equal to 1/3rd your Proficiency Bonus, rounded up" — Controller/
	// Defender/Striker's shared Role Effect formula.
	return (profBonus + 2) / 3
}

// allAbilities is every eligible ability key, for the wide-open "your
// choice of ability score" groups (Specialized, Bestial Framework,
// Controller).
var allAbilities = []string{"str", "dex", "con", "int", "wis", "cha"}

// FoundationEntries is every Puppeteer Chassis/Puppet Framework/Puppet
// Role/Puppet Weapon Type option, keyed by its own class_options.slug.
var FoundationEntries = buildFoundationEntries()

func buildFoundationEntries() map[string]FoundationEntry {
	entries := []FoundationEntry{
		// --- Puppeteer Chassis (Black Technique) ---
		{
			EntrySlug: "class/puppet-master/option/puppeteer-chassis/specialized",
			Name:      "Specialized",
			Catalog:   "Puppeteer Chassis",
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: allAbilities, Delta: 2},
			},
			SizeChoices: []string{"Small", "Medium", "Large"},
			NaturalWeapon: &NaturalWeaponSpec{
				Name: "Slam", Reach: "5ft.", DamageCount: 1, DamageSides: 6,
				DamageAbility: "str", AddProficiency: true,
				DamageTypeChoices: []string{"bludgeoning", "piercing", "slashing"},
				CritText: "On a roll of 17-20, inflicts 1 rank of a condition matching its chosen damage type " +
					"(Bludgeoning → Bruised; Piercing → Weakened until the end of your next turn; Slashing → Bleeding).",
			},
			BonusTierSlots: map[string]int{"Wood Tier": 2},
		},
		{
			EntrySlug:           "class/puppet-master/option/puppeteer-chassis/quadrupedal",
			Name:                "Quadrupedal",
			Catalog:             "Puppeteer Chassis",
			AbilitySets:         map[string]int{"str": 16, "con": 16},
			Size:                "Large",
			SpeedSet:            35,
			FixedGrantEntrySlug: bulkyBuildGrantSlug,
			NaturalWeapon: &NaturalWeaponSpec{
				Name: "Bite", Reach: "5ft.", DamageCount: 1, DamageSides: 10,
				DamageAbility: "str", AddProficiency: true, DamageType: "piercing",
				CritText: "On a roll of 17-20, inflicts 1 rank of Weakened until the end of your next turn.",
			},
			ReferenceTraits: []string{
				"Reduces all damage received that is not fire by 2 (no damage-reduction field exists to compute this automatically).",
			},
		},
		{
			EntrySlug:           "class/puppet-master/option/puppeteer-chassis/warforged",
			Name:                "Warforged",
			Catalog:             "Puppeteer Chassis",
			AbilitySets:         map[string]int{"str": 16, "dex": 16},
			FixedGrantEntrySlug: warfareAugmentationGrantSlug,
			NaturalWeapon: &NaturalWeaponSpec{
				Name: "Strike", Reach: "5ft.", DamageCount: 1, DamageSides: 6,
				DamageAbility: "str", AddProficiency: true, DamageType: "bludgeoning",
				CritText: "On a roll of 17-20, inflicts 1 rank of Bruised.",
			},
			ReferenceTraits: []string{
				"Gains proficiency in all Simple and Martial Weapons, and in light and medium armor.",
				"While wearing armor, adds its Dexterity Modifier to its Armor Class, up to the armor's own maximum (seals on the Puppet and its Armorset don't stack).",
			},
		},
		{
			EntrySlug:           "class/puppet-master/option/puppeteer-chassis/winged",
			Name:                "Winged",
			Catalog:             "Puppeteer Chassis",
			AbilitySets:         map[string]int{"dex": 18, "con": 14},
			Size:                "Small",
			FlySpeedEqualsSpeed: true,
			FixedGrantEntrySlug: quickfootedGrantSlug,
			NaturalWeapon: &NaturalWeaponSpec{
				Name: "Talons", Reach: "5ft.", DamageCount: 1, DamageSides: 8,
				DamageAbility: "dex", AddProficiency: true, DamageType: "slashing",
				CritText: "On a roll of 17-20, inflicts 1 rank of Bleeding.",
			},
		},

		// --- Puppet Frameworks (Green Technique) ---
		{
			EntrySlug: "class/puppet-master/option/puppet-frameworks/bestial-framework",
			Name:      "Bestial Framework",
			Catalog:   "Puppet Framework",
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: allAbilities, Delta: 2},
				{Choices: allAbilities, Delta: 1},
			},
			Size:                  "Medium",
			SizeAtLevel:           map[int]string{10: "Large"},
			SageCreatureFramework: true,
			ReferenceTraits: []string{
				"This Puppet's Sage Creature rank equals your highest jutsu known rank — not modeled (this app has no combat-resolution engine for a Sage Creature's own rank-gated stat progression); the chosen tribe's own D-Rank traits and natural weapon(s) below apply at the D-Rank entry regardless.",
				"Traits requiring jutsu slots cost 5 chakra per slot instead; traits granting chakra or temporary chakra cannot be selected.",
			},
		},
		{
			EntrySlug:     "class/puppet-master/option/puppet-frameworks/matryoshka-framework",
			Name:          "Matryoshka Framework",
			Catalog:       "Puppet Framework",
			AbilityDeltas: map[string]int{"con": 2},
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: []string{"str", "dex", "int", "wis", "cha"}, Delta: 1},
			},
			Size:               "Small",
			SplitBodyFramework: true,
			ReferenceTraits: []string{
				"Splitting shares most upgrades across every body but not weapon/jutsu-granting ones, and each body is its own distinct creature (command only one per turn) — not modeled beyond the split/merge action and HP division itself, this app has no turn-by-turn action economy for \"command only one per turn.\"",
				"Can be up to 1000 feet away at 2nd level, 1 mile at 10th, 10 miles at 17th; carries a Radio Link of the same range, auto-connected to allies' own networks.",
				"A body can attach to an adjacent ally, who may then spend a Bonus Action to have it concentrate on a jutsu in their place, boost their passive perception/insight by half your Proficiency Bonus, or take an action on their behalf.",
			},
		},
		// Shade/Spellblade Framework's own "1 D-Rank jutsu of your choice at
		// 2nd level, 1 C-Rank at 10th, 1 B-Rank at 17th" clause is modeled
		// as a real 3-slot sub-choice pick (cmd/n5e/puppet_foundation.go's
		// shadeFrameworkJutsuOptions/spellbladeFrameworkJutsuOptions), not
		// left as reference text here — each slot is gated to its own rank
		// and Puppet Master level. The picked jutsu is display-only (this
		// app has no companion casting economy to plug it into, the same
		// gap noted on every other jutsu-granting entry in this file).
		{
			EntrySlug:           "class/puppet-master/option/puppet-frameworks/shade-framework",
			Name:                "Shade Framework",
			Catalog:             "Puppet Framework",
			AbilitySets:         map[string]int{"cha": 16},
			AbilityDeltas:       map[string]int{"dex": 1},
			FixedGrantEntrySlug: ghillieCoatingGrantSlug,
			ReferenceTraits: []string{
				"Can use Charisma as its Genjutsu ability modifier.",
				"Genjutsu it casts gain a +1 bonus to attack rolls and Save DC, and deal an additional damage die once per turn.",
			},
		},
		{
			EntrySlug:           "class/puppet-master/option/puppet-frameworks/spellblade-framework",
			Name:                "Spellblade Framework",
			Catalog:             "Puppet Framework",
			AbilitySets:         map[string]int{"int": 16},
			AbilityDeltas:       map[string]int{"str": 1},
			FlatACBonus:         1,
			FixedGrantEntrySlug: warfareAugmentationGrantSlug,
			ReferenceTraits: []string{
				"Once per turn, when casting a Ninjutsu, can make a melee or ranged weapon attack as part of the same action (ranged attacks within 5ft. are not at disadvantage).",
			},
		},

		// --- Puppet Roles (Red Technique) ---
		{
			EntrySlug: "class/puppet-master/option/puppet-roles/controller",
			Name:      "Controller",
			Catalog:   "Puppet Role",
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: allAbilities, Delta: 2, Floor: 10},
				{Choices: allAbilities, Delta: 1},
			},
			BonusSkills: []string{"Chakra Control"},
			RoleEffect: &RoleEffectSpec{
				TextBonus: func(profBonus int) string {
					return "Save DC bonus for upgrades requiring a saving throw: +" + strconv.Itoa(abilityCeilThird(profBonus)) +
						" (1/3 Proficiency Bonus, rounded up)."
				},
			},
			ReferenceTraits: []string{
				"Once per rest, can attempt to deflect a Clash-keyword jutsu targeting you or an ally within 15ft., initiating a Clash; adds your Proficiency to that Clash check, and may reroll a failed Clash check once per combat if it already would for another reason.",
			},
		},
		{
			EntrySlug:     "class/puppet-master/option/puppet-roles/defender",
			Name:          "Defender",
			Catalog:       "Puppet Role",
			AbilityDeltas: map[string]int{"con": 2, "str": 1},
			BonusSkills:   []string{"Athletics"},
			RoleEffect:    &RoleEffectSpec{ACBonus: abilityCeilThird},
			ReferenceTraits: []string{
				"+3 bonus to Constitution skill checks and saving throws (no companion skill-check-bonus field exists to compute this automatically).",
			},
		},
		{
			EntrySlug:     "class/puppet-master/option/puppet-roles/lurker",
			Name:          "Lurker",
			Catalog:       "Puppet Role",
			AbilityDeltas: map[string]int{"dex": 2},
			AbilitySets:   map[string]int{"wis": 11},
			BonusSkills:   []string{"Stealth"},
			RoleEffect: &RoleEffectSpec{
				CritRangeBonus:  1,
				CritDamageBonus: func(profBonus int) int { return profBonus },
				TextBonus: func(profBonus int) string {
					return "On a critical hit, add your Proficiency Bonus (+" + strconv.Itoa(profBonus) + ") to the damage dealt."
				},
			},
		},
		{
			EntrySlug:     "class/puppet-master/option/puppet-roles/striker",
			Name:          "Striker",
			Catalog:       "Puppet Role",
			AbilityDeltas: map[string]int{"con": 1},
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: []string{"str", "dex"}, Delta: 2},
			},
			BonusSkills: []string{"Acrobatics", "Martial Arts"},
			RoleEffect:  &RoleEffectSpec{AttackRollBonus: abilityCeilThird},
		},
		{
			EntrySlug:     "class/puppet-master/option/puppet-roles/supporter",
			Name:          "Supporter",
			Catalog:       "Puppet Role",
			AbilityDeltas: map[string]int{"con": 2},
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: []string{"int", "wis"}, Set: 10},
			},
			BonusSkills: []string{"Medicine"},
			RoleEffect: &RoleEffectSpec{
				TextBonus: func(profBonus int) string {
					return "Increases all healing or temporary hit points its upgrades/actions/jutsu grant by your Proficiency Bonus (+" + strconv.Itoa(profBonus) + ")."
				},
			},
		},
		{
			EntrySlug:   "class/puppet-master/option/puppet-roles/surveyor",
			Name:        "Surveyor",
			Catalog:     "Puppet Role",
			AbilitySets: map[string]int{"wis": 16},
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: []string{"str", "dex", "con", "int", "cha"}, Delta: 1},
			},
			BonusSkills: []string{"Perception", "Insight", "Investigation"},
			RoleEffect: &RoleEffectSpec{
				SkillAbilityOverride: map[string]string{"Investigation": "wis"},
			},
			ReferenceTraits: []string{
				"Has 60 feet of Darkvision (no senses field exists to compute this automatically).",
				"May use a Skill-Based action for a skill this role grants proficiency in as a Bonus Action, even if it would normally require an Action.",
			},
		},

		// --- Puppet Weapon Types (Blue Technique) ---
		{
			EntrySlug:           "class/puppet-master/option/puppet-weapon-types/drone-weapon",
			Name:                "Drone Weapon",
			Catalog:             "Puppet Weapon Type",
			AbilitySets:         map[string]int{"wis": 14},
			AbilityDeltas:       map[string]int{"dex": 1},
			SpeedDelta:          30,
			FlySpeedEqualsSpeed: true,
			BonusSkills:         []string{"Investigation", "Perception", "Stealth", "Survival"},
			NaturalWeapon: &NaturalWeaponSpec{
				Name: "Arrow Missile", Reach: "90/180ft.", Property: "Ammunition",
				DamageCount: 1, DamageSides: 8, DamageAbility: "str", AddProficiency: false,
				DamageType: "piercing",
			},
			ReferenceTraits: []string{
				"Can be up to 1000 feet away from you.",
				"As an action or Bonus Action, you can look through the Drone's eyes — blind to your own surroundings, gaining 10× your level in Darkvision through it; stop at any time with no action needed.",
				"Treats a roll of 7 or lower on the d20 for Investigation/Perception/Stealth/Survival as an 8 (the flat proficiency above is modeled; this floor isn't).",
				"During a long rest, may add the Overseer property to its Weapon Mode (doesn't count against max properties): melee gains an extra Reach rank, or ranged attacks within max range ignore disadvantage from range; a Perception check vs. DC 5 + target level can ignore non-dice penalties until end of turn.",
			},
		},
		{
			EntrySlug:     "class/puppet-master/option/puppet-weapon-types/ogre-weapon",
			Name:          "Ogre Weapon",
			Catalog:       "Puppet Weapon Type",
			AbilityDeltas: map[string]int{"str": 2, "con": 1},
			Size:          "Large",
			SizeAtLevel:   map[int]string{14: "Huge"},
			NaturalWeapon: &NaturalWeaponSpec{
				Name: "Smash", Reach: "5ft.", Property: "Unarmed",
				DamageCount: 3, DamageSides: 4, DamageAbility: "str", AddProficiency: false,
				DamageType: "bludgeoning",
			},
			ReferenceTraits: []string{
				"Applies your Proficiency Bonus when attempting to Grapple a creature, and doesn't reduce movement speed when carrying a creature smaller than itself.",
				"Deals double damage to structures, constructs, and objects; its maximum lift is multiplied by your Proficiency Bonus.",
				"During a long rest, may add the Titan property to its Weapon Mode (doesn't count against max properties): damage die increases by 1 step (+1 damage if already past 1d12), and damage rolls ignore up to 3 DR.",
			},
		},
		{
			EntrySlug:   "class/puppet-master/option/puppet-weapon-types/sentinel-weapon",
			Name:        "Sentinel Weapon",
			Catalog:     "Puppet Weapon Type",
			SizeChoices: []string{"Small", "Medium"},
			SpeedDelta:  10,
			AbilityChoiceGroups: []AbilityChoiceGroup{
				{Choices: []string{"str", "dex"}, Delta: 2},
				{Choices: []string{"str", "dex"}, Delta: 1},
			},
			FlatAttackRollBonus: 1,
			ReferenceTraits: []string{
				"Increases the damage die of weapons it wields by 1 step.",
				"Always under the effects of the Disengage action.",
				"Gains an additional Reaction per round, usable only for attacks of opportunity; creatures provoke from it when entering within 5ft. of it or an ally within 10ft. of it.",
				"Can use Dexterity in place of Strength for weapons and Bukijutsu.",
				"During a long rest, may add the Okizeme property to its Weapon Mode (doesn't count against max properties): the weapon gains an extra property, and Bukijutsu/Taijutsu using it can be cast with Dexterity instead of Strength.",
			},
		},
	}

	out := make(map[string]FoundationEntry, len(entries))
	for _, e := range entries {
		out[e.EntrySlug] = e
	}
	return out
}
