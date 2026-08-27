package main

import "sort"

// traitCategory distinguishes a damage-type grant from a condition grant —
// the same target name can otherwise mean two different things ("Poison"
// damage vs. the "Poisoned" condition are separate things in this ruleset).
type traitCategory string

const (
	traitDamage    traitCategory = "damage"
	traitCondition traitCategory = "condition"
)

// traitLevel orders Resistance below Immunity so combining grants from
// multiple sources (or an escalating grant, see passiveTraitGrant.Escalate)
// never lets a later, weaker grant downgrade a target the character is
// already immune to.
type traitLevel int

const (
	levelResistance traitLevel = 1
	levelImmunity   traitLevel = 2
)

// passiveTraitGrant is one hand-verified, always-on resistance or immunity a
// class feature, subclass feature, clan feature, or feat grants the
// character outright — keyed by the rules-database slug of the feature that
// grants it (passiveTraitGrants below).
//
// This is a curated table, not a text parser. The sourcebook's prose is
// saturated with resistance/immunity language that is NOT a permanent
// character trait: temporary transformations ("for the next minute"),
// reactive/triggered effects ("as a Reaction, you gain..."), benefits
// granted to OTHER creatures (allies, summoned puppets, a Scientific Ninja
// Beast, consumers of a cooked snack), and features that ignore an enemy's
// resistance rather than grant the caster one. Every entry below was read in
// full against that bar — a blind regex over the ~170 candidate rows in
// class/subclass/clan/feat text would produce far more false positives than
// a hand-reviewed table costs in coverage.
type passiveTraitGrant struct {
	Category traitCategory
	Target   string
	Level    traitLevel
	// MinLevel is an additional character-level gate beyond the feature's
	// own row-level, for a feature whose text grants something stronger
	// partway through its own progression — e.g. Ashen Resilience is gained
	// at 1st level but its Fire resistance clause explicitly starts at 7th
	// ("Starting at 7th level, your resilience to fire improves..."). Zero
	// means the feature's own row-level (already enforced by
	// loadGrantedFeatures before this table is ever consulted) is the only
	// gate.
	MinLevel int
	// Escalate marks a "Resistance, or Immunity if you already have
	// Resistance" grant (Heavenly Flame: "you gain Resistance to Fire
	// damage, or Immunity if you already have Resistance"). Level is
	// ignored when this is set — computePassiveTraits resolves every
	// Escalate grant after all non-escalating ones, so "already have" means
	// "from some other source."
	Escalate bool
}

// passiveTraitGrants is the curated table itself, reviewed 2026-07-31 across
// every class_features/subclass_features/clan_features/feats row mentioning
// resistance, immunity, or darkvision in the shipped rulebooks.
var passiveTraitGrants = map[string][]passiveTraitGrant{
	// Class feature.
	"class/taijutsu-specialist/feature/perfect-mind": {
		{Category: traitCondition, Target: "Fear", Level: levelImmunity},
	},

	// Clan features.
	"clan/hebi/feature/poisonous-diet": {
		{Category: traitDamage, Target: "Poison", Level: levelImmunity, MinLevel: 18},
		{Category: traitCondition, Target: "Envenomed", Level: levelImmunity, MinLevel: 18},
	},
	"clan/hoshi/feature/celestial-body": {
		{Category: traitDamage, Target: "Chakra", Level: levelResistance},
	},
	"clan/iburi/feature/ashen-resilience": {
		{Category: traitDamage, Target: "Fire", Level: levelResistance, MinLevel: 7},
		{Category: traitCondition, Target: "Burned", Level: levelImmunity, MinLevel: 11},
	},
	"clan/jiton/feature/swirling-currents": {
		{Category: traitDamage, Target: "Bludgeoning", Level: levelResistance, MinLevel: 18},
		{Category: traitDamage, Target: "Piercing", Level: levelResistance, MinLevel: 18},
		{Category: traitDamage, Target: "Slashing", Level: levelResistance, MinLevel: 18},
	},
	"clan/keton/feature/stargazer": {
		{Category: traitCondition, Target: "Blinded", Level: levelImmunity, MinLevel: 18},
		{Category: traitCondition, Target: "Dazzled", Level: levelImmunity, MinLevel: 18},
	},
	"clan/synthetic-human/feature/immune-system": {
		{Category: traitDamage, Target: "Poison", Level: levelResistance},
		{Category: traitDamage, Target: "Poison", Level: levelImmunity, MinLevel: 18},
	},
	"clan/tsuchigumo/feature/tsuchigumo-senses": {
		{Category: traitCondition, Target: "Surprised", Level: levelImmunity},
	},
	"clan/vesper/feature/immortal": {
		{Category: traitCondition, Target: "Charmed", Level: levelImmunity},
	},
	"clan/yuki/feature/chilled-body": {
		{Category: traitDamage, Target: "Cold", Level: levelResistance},
		{Category: traitCondition, Target: "Chilled", Level: levelImmunity, MinLevel: 11},
		{Category: traitDamage, Target: "Cold", Level: levelImmunity, MinLevel: 18},
	},

	// Feats.
	"feat/akimichi/iron-gut": {
		{Category: traitCondition, Target: "Envenomed", Level: levelResistance},
		{Category: traitCondition, Target: "Weakened", Level: levelResistance},
	},
	"feat/futton/broiling-rage": {
		{Category: traitDamage, Target: "Acid", Level: levelResistance},
	},
	"feat/namikaze/swift-shield": {
		{Category: traitDamage, Target: "Wind", Level: levelResistance},
	},
	"feat/uzumaki/chakra-tenacity": {
		{Category: traitDamage, Target: "Chakra", Level: levelResistance},
	},
	"feat/class/the-certainty-of-steel": {
		{Category: traitDamage, Target: "Poison", Level: levelImmunity},
	},
	"feat/dungeon-delver": {
		{Category: traitDamage, Target: "Trap", Level: levelResistance},
	},
	"feat/endurance-realized": {
		{Category: traitDamage, Target: "Chakra", Level: levelResistance},
	},
	"feat/ranton/storming-rain": {
		{Category: traitDamage, Target: "Lightning", Level: levelResistance},
	},
	// The dojutsu's own activation-gated bullet immediately before this one
	// ("While you are gaining the benefit of your Kurugan, you cannot be
	// surprised") stays unmodeled as Group 2 — it's explicitly tied to the
	// base feature's 10-minute Bonus Action activation (clan/kuru/feature/
	// kurugan). This bullet carries no such qualifier and says "constantly",
	// reading as a permanent upgrade to the dojutsu's baseline rather than
	// another activation-gated effect, the same always-on shape as
	// clan/keton/feature/stargazer's Dazzled immunity above.
	"feat/kuru/adept-kurugan": {
		{Category: traitCondition, Target: "Dazzled", Level: levelImmunity},
		{Category: traitCondition, Target: "Demoralized", Level: levelImmunity},
	},

	// Subclass features.
	"class/intelligence-operative/group/master-strategies/precognitive/feature/precognition": {
		{Category: traitCondition, Target: "Surprised", Level: levelImmunity},
	},
	// Sibling grant to Precognition above: "you and allied creatures within
	// 30 feet of you cannot be surprised..." Only the self-immunity half is
	// modeled, same "allies too" simplification Precognition's own entry
	// already draws — no aura mechanism exists to extend a condition to
	// nearby allies.
	"class/intelligence-operative/group/master-strategies/calculated-strategist/feature/nothing-is-a-surprise": {
		{Category: traitCondition, Target: "Surprised", Level: levelImmunity},
	},
	"class/medical-nin/group/tenets-of-medicine/black-medicine/feature/toxic-tongue": {
		{Category: traitDamage, Target: "Poison", Level: levelResistance},
		{Category: traitCondition, Target: "Poisoned", Level: levelImmunity},
	},
	"class/taijutsu-specialist/group/taijutsu-style/ironclad/feature/iron-will": {
		{Category: traitCondition, Target: "Berserk", Level: levelImmunity},
		{Category: traitCondition, Target: "Dazed", Level: levelImmunity},
	},
	"class/taijutsu-specialist/group/taijutsu-style/ruin/feature/battle-ready-catalyst": {
		{Category: traitDamage, Target: "Chakra", Level: levelResistance},
	},
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design": {
		{Category: traitDamage, Target: "Poison", Level: levelImmunity},
		{Category: traitCondition, Target: "Poisoned", Level: levelImmunity},
	},
	// The advantage-on-envenomed-saves half of this feature's text ("...you
	// gain resistance to poison damage and have advantage on saving throws
	// that would inflict the envenomed condition" — rules.db subclass_features,
	// confirmed verbatim 2026-08-26) is wired below in advantageGrants, keyed
	// to this same slug.
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/noxious-handiwork": {
		{Category: traitDamage, Target: "Poison", Level: levelResistance},
	},
	"class/cooking-nin/group/cooking-focus/fry-cooks/feature/always-ready": {
		{Category: traitCondition, Target: "Surprised", Level: levelImmunity},
	},
	"class/cooking-nin/group/cooking-focus/heat-master/feature/heavenly-flame": {
		{Category: traitDamage, Target: "Fire", Escalate: true},
	},
	"class/science-nin/group/scientific-inquiry/storm-rider/feature/trick-paths": {
		{Category: traitCondition, Target: "Surprised", Level: levelImmunity},
	},
	"class/science-nin/group/scientific-inquiry/storm-rider/feature/the-future-of-shinobi-sky-keeper": {
		{Category: traitDamage, Target: "Falling", Level: levelImmunity},
	},
	// Full-Metal Shinobi lets the player choose Bludgeoning, Piercing, or
	// Slashing at 6th level, another at 9th, and the last one at 14th — a
	// real, staged player choice tracked via full_metal_shinobi.go's own
	// picks table rather than a flat always-on grant. fullMetalShinobiPassiveRows
	// appends one synthetic grantedFeatureRow per damage type the character
	// currently holds, each keyed by fullMetalShinobiPassiveTraitSlug below
	// (never a real rules-database slug), so the real feature slug itself
	// carries no entry here.
	fullMetalShinobiPassiveTraitSlug("Bludgeoning"): {
		{Category: traitDamage, Target: "Bludgeoning", Level: levelResistance},
	},
	fullMetalShinobiPassiveTraitSlug("Piercing"): {
		{Category: traitDamage, Target: "Piercing", Level: levelResistance},
	},
	fullMetalShinobiPassiveTraitSlug("Slashing"): {
		{Category: traitDamage, Target: "Slashing", Level: levelResistance},
	},

	// Hunters Patterns. These slugs are class_options catalog rows, not
	// class/subclass/clan features, so they never appear in
	// loadGrantedFeatures's own output — they only reach computePassiveTraits
	// when hunter_nin.go appends synthetic grantedFeatureRow entries for the
	// character's picks alongside the normal granted-features list.
	"class/hunter-nin/option/hunters-patterns/horror-films": {
		{Category: traitCondition, Target: "Fear", Level: levelImmunity},
	},
	"class/hunter-nin/option/hunters-patterns/illicit-literature": {
		{Category: traitCondition, Target: "Charmed", Level: levelImmunity},
	},
}

// advantageDirection distinguishes an Advantage grant from a Disadvantage
// grant on the same roll type — Untouchable's Disadvantage on reaction
// attacks made against the character and Battle Readiness's Advantage on
// Initiative checks are both handled by the identical registration/render
// path below, just filed into different PassiveTraitSummary slices.
type advantageDirection string

const (
	directionAdvantage    advantageDirection = "advantage"
	directionDisadvantage advantageDirection = "disadvantage"
)

// advantageGrant is one hand-verified, always-on Advantage or Disadvantage
// a class feature, subclass feature, clan feature, or feat grants the
// character on a named roll type — the Advantage/Disadvantage counterpart
// to passiveTraitGrant. This app has no generic advantage/disadvantage-flag
// tracking mechanism prior to this table (see e.g. this file's own
// black-technique-puppeteer/feature/noxious-handiwork comment and
// titan.go's ResistanceAdvantageText, both landmarks of the gap this closes)
// — every entry below is the FULL integration surface a class file needs:
// register one row here, keyed by the feature's own rules-database slug
// (the same grantedFeatureRow.Slug loadGrantedFeatures already resolves by
// character level, exactly like passiveTraitGrants), and
// computePassiveTraits resolves it into PassiveTraitSummary.Advantages /
// .Disadvantages automatically. No other file needs to change, and no
// second call site needs wiring — both sheet-render call sites already
// funnel their granted-features list through computePassiveTraits for the
// resistance/immunity/sense table above, and PassiveTraitSummary is what
// the template already renders.
//
// RollType is free-form display text naming exactly which roll this
// applies to, in the form the sheet should show it ("Initiative checks",
// "saving throws against being Envenomed or Poisoned", "reaction attacks
// made against you", ...). There is no fixed enum of roll types: the
// features this table exists for don't share one vocabulary — some name a
// whole roll (Initiative), some a family of conditions, some a specific
// kind of incoming attack.
//
// This app has no dice-roll-resolution code that could mechanically apply
// Advantage/Disadvantage to a roll — dice-roller.js's rollMode is a manual
// Normal/Advantage/Disadvantage toggle the player sets before clicking
// Roll (the same one Elemental Advantage combines with), not something any
// server-side roll result flows through. A grant registered here renders
// as a visible, permanent reminder for the player to set that toggle
// themselves for the named roll type — it does not and cannot flip a die
// roll on its own, matching how every other rollable stat on this sheet
// (skills, saves, attacks) already works.
type advantageGrant struct {
	RollType  string
	Direction advantageDirection
	// MinLevel mirrors passiveTraitGrant.MinLevel — an extra character-level
	// gate beyond the feature's own row-level, for a feature whose text
	// grants the Advantage/Disadvantage partway through its own
	// progression. Zero means the feature's own row-level (already enforced
	// by loadGrantedFeatures before this table is ever consulted) is the
	// only gate.
	MinLevel int
}

// advantageGrants is the curated table itself. See advantageGrant's own doc
// comment for exactly how a new class file registers into this mechanism.
var advantageGrants = map[string][]advantageGrant{
	// Weapon Specialist, base class, 7th level (rules.db class_features,
	// slug below, confirmed verbatim 2026-08-26): "Starting at 7th level,
	// you have fully learned how to instantly switch from a neutral stance
	// to that of a combat one. You have advantage on Initiative Checks."
	"class/weapon-specialist/feature/battle-readiness": {
		{RollType: "Initiative checks", Direction: directionAdvantage},
	},
	// Noxious Handiwork, Puppet Master/Black Technique, 14th level (rules.db
	// subclass_features, confirmed verbatim 2026-08-26): "...you gain
	// resistance to poison damage and have advantage on saving throws that
	// would inflict the envenomed condition." The flat Poison resistance
	// half is the passiveTraitGrants entry for this same slug, above.
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/noxious-handiwork": {
		{RollType: "saving throws against becoming Envenomed", Direction: directionAdvantage},
	},
	// Disturbance, Taijutsu Specialist, 6th level (rules.db
	// subclass_features, slug below, confirmed verbatim 2026-08-26):
	// "creatures who would spend their Reaction to make an attack of any
	// type targeting you, is made at disadvantage." Untouchable's other
	// clause ("creatures cannot take attacks of opportunities against you,
	// by any means") stays unmodeled — no mechanism on this sheet
	// represents immunity to a specific triggered attack type, only
	// advantage/disadvantage on a named roll and flat resistance/immunity
	// to a damage type or condition.
	"class/taijutsu-specialist/group/taijutsu-style/disturbance/feature/untouchable": {
		{RollType: "reaction attacks made against you", Direction: directionDisadvantage},
	},
	// Shinobi's Karma: Body, Hunter-Nin/Wolves Legacy, 3rd level (rules.db
	// subclass_features, confirmed verbatim 2026-08-26): "Increase the
	// number of failed death saves you need by 2, before you die, and you
	// gain advantage on death saves." The death-save-count increase itself
	// has no death-save-tracking mechanism to hook into — see this same
	// slug's entry in passiveNoteGrants below. The exclusive Deflection
	// Exploit grant this feature also confers is wired via
	// hunterNinShinobisKarmaBodyExploitGrant (hunter_nin.go); its
	// disadvantage-on-grapple/trip/push-attempts-against-you clause stays
	// unmodeled — that's Disadvantage imposed on the OTHER creature's
	// check/save, not a roll the character themselves makes, so it doesn't
	// fit this table's "advantage/disadvantage on a roll YOU make" shape.
	// The exclusive Deflection Exploit grant this feature also confers is
	// wired via hunter_nin.go's own huntersExploitAutoGrants table.
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-body": {
		{RollType: "Death saving throws", Direction: directionAdvantage},
	},
	// Shinobi's Karma: Will, Hunter-Nin/Wolves Legacy, 14th level (rules.db
	// subclass_features, confirmed verbatim 2026-08-26): "Additionally,
	// saving throws you make against a Genjutsu that would Restrain,
	// incapacitate, slow or stun you are made at advantage." The
	// unconditional Charisma save proficiency this same feature also grants
	// is already wired via internal/features/grants.go's
	// fixedProficiencyGrants.
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-will": {
		{RollType: "saving throws against a Genjutsu that would Restrain, Incapacitate, Slow, or Stun you", Direction: directionAdvantage},
	},
}

// AdvantageEntry is one resolved Advantage or Disadvantage grant, with the
// feature name(s) that grant it for the sheet's tooltip — the
// Advantage/Disadvantage counterpart to PassiveTraitEntry.
type AdvantageEntry struct {
	RollType string
	Sources  []string
}

// senseGrant is a curated always-on darkvision/tremorsense/blindsight grant,
// the sense-adjacent counterpart to passiveTraitGrant.
type senseGrant struct {
	Sense    string
	Feet     int
	MinLevel int
	// Stacks adds Feet to an existing grant of the same Sense from another
	// source instead of taking the larger of the two — Enhanced Vision:
	// "grants you 60 feet of Darkvision... If you already have Darkvision,
	// it is increased by 60 feet instead."
	Stacks bool
}

var senseGrants = map[string][]senseGrant{
	"clan/hebi/feature/serpent-mimicry": {
		{Sense: "Darkvision", Feet: 60},
		{Sense: "Tremorsense", Feet: 30},
	},
	// Increases (not stacks onto) Serpent Mimicry's own 30ft Tremorsense
	// grant — both resolve through the same non-Stacks "higher Feet wins"
	// path, matching the feat's own "Your tremor sense range is increased
	// to 45 feet." Requires Hebi Clan, Level 4+, so Serpent Mimicry's own
	// grant is always present alongside this one.
	"feat/hebi/apex-heritage": {
		{Sense: "Tremorsense", Feet: 45},
	},
	"clan/vesper/feature/supreme-nightvision": {
		{Sense: "Darkvision", Feet: 60},
	},
	"feat/aburame/symbiotic-insects": {
		{Sense: "Darkvision", Feet: 60},
	},
	// Only the 60ft-stacking-Darkvision half of this feature's text is
	// modeled here. The rest of the same sentence ("...and doubles your
	// normal sight range... You can accurately make out the details of
	// things within 1 mile of you.") is wired below in passiveNoteGrants,
	// keyed to this same slug — no sight-range field exists anywhere in
	// this app to literally double.
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/enhanced-vision": {
		{Sense: "Darkvision", Feet: 60, Stacks: true},
	},
	"class/hunter-nin/group/hunters-creeds/grave-stalker/feature/shadow-stalker": {
		{Sense: "Blindsight", Feet: 20},
	},
	// Replaces (not stacks onto) Shadow Stalker's own 20ft grant — both
	// grants resolve through the same non-Stacks "higher Feet wins" path,
	// matching the book's own "Your blindsight increases to 40 feet."
	"class/hunter-nin/group/hunters-creeds/grave-stalker/feature/one-with-the-darkness": {
		{Sense: "Blindsight", Feet: 40},
	},
	"class/hunter-nin/group/hunters-creeds/blade-warden/feature/superior-offense": {
		{Sense: "Blindsight", Feet: 30},
	},
	"clan/konjiki/feature/blood-of-the-earth": {
		{Sense: "Tremorsense", Feet: 60, MinLevel: 11},
	},
	"feat/kuru/adept-kurugan": {
		{Sense: "Blindsight", Feet: 30},
	},
	// Synthetic key genjutsuMirageDemonSightPassiveRow (genjutsu.go) emits
	// for a picked Demon Sight Malleable Mirage — that Mirage's own real
	// class_options slug never reaches loadMergedGrantedFeatures' output at
	// all, the same reason genjutsuMirageRestLimitedSlugs' entries need
	// their own synthetic injection into computeCustomResources' input.
	"genjutsu-mirage/demon-sight": {
		{Sense: "Darkvision", Feet: 120},
	},
}

// PassiveTraitEntry is one resolved resistance/immunity/vulnerability, with
// the feature name(s) that grant it for the sheet's tooltip.
type PassiveTraitEntry struct {
	Target  string
	Sources []string
}

// SenseEntry is one resolved special sense (Darkvision, Tremorsense, ...).
type SenseEntry struct {
	Sense   string
	Feet    int
	Sources []string
}

// passiveNoteGrant is one hand-verified, always-on rules effect that is
// neither a resistance/immunity/vulnerability, a special sense, nor an
// Advantage/Disadvantage grant — a free-form reminder for a clause this app
// has no numeric field to compute against (a range-doubling effect with no
// range field, an extra-perception-detail clause with no perception-detail
// field, ...), the passiveTraitGrant/advantageGrant sibling for prose that
// fits neither of those shapes. Keyed by the granting feature's own
// rules-database slug, exactly like passiveTraitGrants/advantageGrants.
type passiveNoteGrant struct {
	Text string
	// MinLevel mirrors passiveTraitGrant.MinLevel — an extra character-level
	// gate beyond the feature's own row-level. Zero means the feature's own
	// row-level (already enforced by loadGrantedFeatures before this table
	// is ever consulted) is the only gate.
	MinLevel int
}

// passiveNoteGrants is the curated table itself. See passiveNoteGrant's own
// doc comment for what belongs here versus in passiveTraitGrants/
// advantageGrants/senseGrants.
var passiveNoteGrants = map[string][]passiveNoteGrant{
	// Enhanced Vision, Puppet Master/Purple Technique, 6th level (rules.db
	// subclass_features, confirmed verbatim 2026-08-26): "...doubles your
	// normal sight range... You can accurately make out the details of
	// things within 1 mile of you." The same sentence's 60ft-stacking-
	// Darkvision half is senseGrants' entry for this same slug, above.
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/enhanced-vision": {
		{Text: "Sight range doubled; 1-mile detail"},
	},
	// Master of the White Technique, Puppet Master/White Technique, 20th
	// level (rules.db subclass_features, confirmed verbatim 2026-08-26):
	// "...double the specified ranges of any White Technique Feature, as
	// well as your Chakra Threads." No range field exists anywhere in this
	// app for Chakra Threads (Chakra Hands' 30ft range, already doubled once
	// by White Technique Proficiency and again by Doubled Thread) or any
	// other White Technique feature to literally double.
	"class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/master-of-the-white-technique": {
		{Text: "Chakra Threads & Feature ranges doubled"},
	},
	// Shinobi's Karma: Body, Hunter-Nin/Wolves Legacy, 3rd level (rules.db
	// subclass_features, confirmed verbatim 2026-08-26): "Increase the
	// number of failed death saves you need by 2, before you die..." — the
	// base rule's 3 failed death saves before dying becomes 5. No death-
	// save-count field or UI exists anywhere in this app (confirmed via a
	// full grep of cmd/n5e and internal/ for death-save handling — the only
	// hits are unrelated custom-resource pools that interact with a death
	// save, e.g. Channeled Healing's "remove one failed death save" spend,
	// never a save/fail counter itself), so this renders as a plain
	// reminder rather than a tracked value. See this same slug's entry in
	// advantageGrants above for the same feature's "advantage on death
	// saves" half.
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-body": {
		{Text: "You need 5 failed death saving throws (instead of 3) before you die"},
	},
}

// PassiveNoteEntry is one resolved passiveNoteGrant, with the feature
// name(s) that grant it for the sheet's tooltip — the free-form-text
// counterpart to PassiveTraitEntry/AdvantageEntry.
type PassiveNoteEntry struct {
	Text    string
	Sources []string
}

// PassiveTraitSummary is the sheet's full resistances/immunities/senses
// panel, computed fresh on every render from the character's granted
// features — never stored, consistent with this project's "derive, don't
// store" pattern (see loadGrantedJutsuLabels for the same approach applied
// to free jutsu grants).
type PassiveTraitSummary struct {
	Resistances []PassiveTraitEntry
	Immunities  []PassiveTraitEntry
	Senses      []SenseEntry
	// Advantages/Disadvantages resolve advantageGrants the same way
	// Resistances/Immunities resolve passiveTraitGrants — grouped by
	// RollType, sorted, each entry's Sources naming every granting feature.
	Advantages    []AdvantageEntry
	Disadvantages []AdvantageEntry
	// Notes resolves passiveNoteGrants the same way, grouped by Text.
	Notes []PassiveNoteEntry
}

// computePassiveTraits resolves passiveTraitGrants and senseGrants against a
// character's granted features (class, subclass, clan, and taken feats —
// whatever loadGrantedFeatures/mergeFeatFeatures produced) and their total
// character level.
//
// Escalating grants (Escalate: true) are resolved in a second pass after
// every non-escalating grant, so "you already have Resistance" always means
// "from some other source" and never the escalating grant seeing itself.
func computePassiveTraits(features []grantedFeatureRow, characterLevel int) PassiveTraitSummary {
	type key struct {
		Category traitCategory
		Target   string
	}
	levels := map[key]traitLevel{}
	sources := map[key][]string{}

	var escalating []struct {
		key     key
		feature grantedFeatureRow
	}

	for _, f := range features {
		for _, g := range passiveTraitGrants[f.Slug] {
			if g.MinLevel > 0 && characterLevel < g.MinLevel {
				continue
			}
			k := key{g.Category, g.Target}
			if g.Escalate {
				escalating = append(escalating, struct {
					key     key
					feature grantedFeatureRow
				}{k, f})
				continue
			}
			if g.Level > levels[k] {
				levels[k] = g.Level
			}
			sources[k] = append(sources[k], f.Name)
		}
	}
	for _, e := range escalating {
		lvl := levelResistance
		if levels[e.key] >= levelResistance {
			lvl = levelImmunity
		}
		if lvl > levels[e.key] {
			levels[e.key] = lvl
		}
		sources[e.key] = append(sources[e.key], e.feature.Name)
	}

	senseFeet := map[string]int{}
	senseSources := map[string][]string{}
	var stackingSenses []struct {
		sense   string
		feet    int
		feature grantedFeatureRow
	}
	for _, f := range features {
		for _, g := range senseGrants[f.Slug] {
			if g.MinLevel > 0 && characterLevel < g.MinLevel {
				continue
			}
			if g.Stacks {
				stackingSenses = append(stackingSenses, struct {
					sense   string
					feet    int
					feature grantedFeatureRow
				}{g.Sense, g.Feet, f})
				continue
			}
			if g.Feet > senseFeet[g.Sense] {
				senseFeet[g.Sense] = g.Feet
			}
			senseSources[g.Sense] = append(senseSources[g.Sense], f.Name)
		}
	}
	for _, s := range stackingSenses {
		senseFeet[s.sense] += s.feet
		senseSources[s.sense] = append(senseSources[s.sense], s.feature.Name)
	}

	// Advantage/Disadvantage grants: same "group by key, dedupe/collect
	// sources, sort for stable render order" shape as the resistance/
	// immunity loop above, just keyed by RollType instead of
	// (Category, Target) and split into two output slices instead of
	// leveled into one.
	advSources := map[string][]string{}
	disadvSources := map[string][]string{}
	for _, f := range features {
		for _, g := range advantageGrants[f.Slug] {
			if g.MinLevel > 0 && characterLevel < g.MinLevel {
				continue
			}
			switch g.Direction {
			case directionAdvantage:
				advSources[g.RollType] = append(advSources[g.RollType], f.Name)
			case directionDisadvantage:
				disadvSources[g.RollType] = append(disadvSources[g.RollType], f.Name)
			}
		}
	}

	// Notes: same "group by key, dedupe/collect sources" shape as the
	// Advantage/Disadvantage loop above, keyed by Text instead of RollType,
	// into a single output slice instead of two.
	noteSources := map[string][]string{}
	for _, f := range features {
		for _, g := range passiveNoteGrants[f.Slug] {
			if g.MinLevel > 0 && characterLevel < g.MinLevel {
				continue
			}
			noteSources[g.Text] = append(noteSources[g.Text], f.Name)
		}
	}

	var out PassiveTraitSummary
	for rollType, srcs := range advSources {
		out.Advantages = append(out.Advantages, AdvantageEntry{RollType: rollType, Sources: srcs})
	}
	for rollType, srcs := range disadvSources {
		out.Disadvantages = append(out.Disadvantages, AdvantageEntry{RollType: rollType, Sources: srcs})
	}
	for text, srcs := range noteSources {
		out.Notes = append(out.Notes, PassiveNoteEntry{Text: text, Sources: srcs})
	}
	sort.Slice(out.Advantages, func(i, k int) bool { return out.Advantages[i].RollType < out.Advantages[k].RollType })
	sort.Slice(out.Disadvantages, func(i, k int) bool { return out.Disadvantages[i].RollType < out.Disadvantages[k].RollType })
	sort.Slice(out.Notes, func(i, k int) bool { return out.Notes[i].Text < out.Notes[k].Text })

	for k, lvl := range levels {
		entry := PassiveTraitEntry{Target: k.Target, Sources: sources[k]}
		switch lvl {
		case levelImmunity:
			out.Immunities = append(out.Immunities, entry)
		case levelResistance:
			out.Resistances = append(out.Resistances, entry)
		}
	}
	for sense, feet := range senseFeet {
		out.Senses = append(out.Senses, SenseEntry{Sense: sense, Feet: feet, Sources: senseSources[sense]})
	}

	sort.Slice(out.Resistances, func(i, k int) bool { return out.Resistances[i].Target < out.Resistances[k].Target })
	sort.Slice(out.Immunities, func(i, k int) bool { return out.Immunities[i].Target < out.Immunities[k].Target })
	sort.Slice(out.Senses, func(i, k int) bool { return out.Senses[i].Sense < out.Senses[k].Sense })
	return out
}

// mergePassiveResistance folds one dynamically-resolved resistance entry
// (e.g. Elemental Resistance's own grant, keyed to the player's own
// Elemental Knowledge pick rather than a fixed value — see scout_nin.go's
// scoutNinElementalResistanceEntry) into an already-computed
// PassiveTraitSummary. Not representable in the static passiveTraitGrants
// table computePassiveTraits itself resolves, since that table's Target is
// always a fixed string, not a per-character pick. A no-op if entry is nil
// (nothing to add). Merges into an existing same-Target entry's Sources
// rather than creating a duplicate row if some other grant already covers
// the same element.
func mergePassiveResistance(summary PassiveTraitSummary, entry *PassiveTraitEntry) PassiveTraitSummary {
	if entry == nil {
		return summary
	}
	for i, r := range summary.Resistances {
		if r.Target == entry.Target {
			summary.Resistances[i].Sources = append(summary.Resistances[i].Sources, entry.Sources...)
			return summary
		}
	}
	summary.Resistances = append(summary.Resistances, *entry)
	sort.Slice(summary.Resistances, func(i, k int) bool { return summary.Resistances[i].Target < summary.Resistances[k].Target })
	return summary
}

// mergePassiveAdvantage folds one dynamically-resolved Advantage entry (e.g.
// Food For the Soul's grant, keyed to the player's own re-pickable ability
// score rather than a fixed value — see cooking_nin.go's
// cookingNinFoodForTheSoulAdvantageEntry) into an already-computed
// PassiveTraitSummary. The Advantage/Disadvantage counterpart to
// mergePassiveResistance, for the identical reason: advantageGrants' Target
// (RollType) is always a fixed string, so a grant whose roll type varies
// per-character can't live in that static table. A no-op if entry is nil.
// Merges into an existing same-RollType entry's Sources rather than
// creating a duplicate row if some other grant already names the same roll.
func mergePassiveAdvantage(summary PassiveTraitSummary, entry *AdvantageEntry) PassiveTraitSummary {
	if entry == nil {
		return summary
	}
	for i, a := range summary.Advantages {
		if a.RollType == entry.RollType {
			summary.Advantages[i].Sources = append(summary.Advantages[i].Sources, entry.Sources...)
			return summary
		}
	}
	summary.Advantages = append(summary.Advantages, *entry)
	sort.Slice(summary.Advantages, func(i, k int) bool { return summary.Advantages[i].RollType < summary.Advantages[k].RollType })
	return summary
}
