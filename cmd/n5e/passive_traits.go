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
	// The advantage-on-envenomed-saves half of this feature's text has no
	// field anywhere in this app to land in (no advantage/disadvantage-flag
	// tracking exists) — only the flat Poison resistance is modeled here.
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
	// Slashing at 6th level, another at 9th, and the last one at 14th —
	// three player choices whose intermediate state (which one or two of
	// the three are covered before 14th level) this table doesn't track.
	// The end state at 14th level is unambiguous regardless of pick order,
	// so only that is modeled here.
	"class/science-nin/group/scientific-inquiry/shinobi-ware/feature/full-metal-shinobi": {
		{Category: traitDamage, Target: "Bludgeoning", Level: levelResistance, MinLevel: 14},
		{Category: traitDamage, Target: "Piercing", Level: levelResistance, MinLevel: 14},
		{Category: traitDamage, Target: "Slashing", Level: levelResistance, MinLevel: 14},
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

// PassiveTraitSummary is the sheet's full resistances/immunities/senses
// panel, computed fresh on every render from the character's granted
// features — never stored, consistent with this project's "derive, don't
// store" pattern (see loadGrantedJutsuLabels for the same approach applied
// to free jutsu grants).
type PassiveTraitSummary struct {
	Resistances []PassiveTraitEntry
	Immunities  []PassiveTraitEntry
	Senses      []SenseEntry
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

	var out PassiveTraitSummary
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
