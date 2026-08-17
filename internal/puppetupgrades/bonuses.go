// Package puppetupgrades holds the Puppet Master class's own upgrade
// bonuses: the flat, always-on "+X to a stat this app actually tracks"
// benefits a Puppet Upgrade grants, resolved into real numbers rather than
// left as prose for the player to apply by hand.
//
// It is its own package because these bonuses land on two different sheets.
// Most modify a companion's stat block, which cmd/n5e computes; two modify
// the PUPPET MASTER's own Speed and Max HP, which internal/charsheet
// computes. Neither of those packages can import the other, so the shared
// rules data lives here.
package puppetupgrades

// This file is the outcome of a full sweep of every Puppet Master upgrade's
// own description across all six technique lists (177 class_option_entries
// rows). 60 of those rows contain a literal "+N"; classifying each one
// against what this app stores produced exactly three groups, and only the
// first can be computed at all:
//
//  1. ALWAYS-ON, TRACKED FIELD — modeled here. An unconditional modifier to
//     the companion's AC/Max HP/Speed/flying speed, or to the character's
//     own Speed/Max HP. These are the entries in StatBonuses
//     and CharacterBonuses below.
//
//  2. CONDITIONAL OR ACTIVATED — deliberately not computed, and this is a
//     correctness decision, not a backlog. Folding a bonus that applies only
//     "for the next minute", "while a creature is in your Puppet's chakra
//     sight", "when this property's effects trigger", or "once per turn"
//     into a static stat block would put a number on the sheet that is
//     wrong most of the time, which is worse than showing the player the
//     text and letting them apply it when it actually fires. This is the
//     large majority: Armory: Overdrive, Blade Dance, Marionette
//     Manipulation, Mech Pilot, Juggernaut Slayer, G30, Kinetic Overflow,
//     Chakra Sensors, Stonecold Stronghold, Transforming Apparatus (a
//     toggled size change, +1 AC one way and -5 speed the other), Momentum,
//     Kill Command, Halo Dance, King Puppet, Undying Form, Evasive Dance,
//     Gilded Weapon, Monzaemon's Legacy, Iron Maiden, Chakra Disruption
//     Blade, Mechanical Light Shield Seal, Successive Casting, Emboldened
//     Elements, Collaborative Genjutsu, Critical Ninjutsu, Symphony of
//     Destruction, Combined Efforts, Clashing Adept, Chakra Sealing Trap,
//     Chakra Regulators, Countermeasures, Charismatic Presence, Entrapment
//     Mechanism, Armory: Beheader's Blade, Armory: Mirage Disc, Armory:
//     Explosive Launcher, Opening Missiles, Chakra Blast, Exploding Puppet
//     Mechanism, Environmental Disruption, Tag-Team, Dead Zone, Flourishing
//     Chakra Reserves, Overflowing Chakra, Peerless Casting, Ghillie
//     Coating, Unlimited Blade Works, Ingrained Strength, Deep-Learning
//     Analysis, Infiltrator, Environmental Adaptation, Accumulated Strength,
//     Expanded Puppetry.
//
//  3. ALWAYS-ON BUT NO FIELD TO PUT IT IN — the bonus is real and permanent,
//     but targets something this app has no numeric slot for. Recorded here
//     so the gap is a known, named thing rather than an unexamined absence:
//     Expanded Frame ("gains 5 damage reduction", plus a size-category
//     increase and a weapon damage-die step — no DR, size, or per-weapon
//     die-step tracking exists for companions), Bulky Build's own second
//     clause ("reduces all damage it receives by -2", same missing DR
//     field — its +1 AC half IS modeled below), and Eye Lights ("raises its
//     Passive Perception by +2" — companions have no computed passive
//     scores; their sheet is a stat block, not a full skill engine).
//
// Group 1 is what this file computes. Groups 2 and 3 already display their
// full rules text on the pick itself, which is the app's standing treatment
// for any effect it does not automate.

import (
	"strconv"
	"strings"
)

// Entry slugs for the upgrades whose bonuses are modeled. Matching on the
// ENTRY slug (not class_option_slug) is what makes these survive tier
// escalation: taking an upgrade from a higher tier's slot changes the
// pick's class_option_slug but never its entry.
const (
	EnhancedDurabilityEntrySlug  = "class/puppet-master/option/puppet-master-upgrades/bronze-tier/entry/enhanced-durability"
	BulkyBuildEntrySlug          = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/bulky-build"
	QuickfootedEntrySlug         = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/quickfooted"
	HoveringMechanismEntrySlug   = "class/puppet-master/option/puppet-master-upgrades/silver-tier/entry/hovering-mechanism"
	AcceleratedMovementEntrySlug = "class/puppet-master/option/armorers-upgrades/wood-tier/entry/accelerated-movement"
)

// BonusContext is everything the bonus formulas below need
// about the character and the one companion being computed. Assembled once
// per companion by loadPuppetsTabData.
type BonusContext struct {
	// SubclassColor is the Puppet Technique ("Black", "Blue", "Green",
	// "Purple", "Red", "White") — most of these upgrades read differently
	// per technique, which is the whole reason they need real formulas
	// rather than a constant.
	SubclassColor string
	MasterLevel   int
	// ArmorChassis is the companion's own chosen chassis name, blank for
	// anything that isn't Juggernaut Armor. Only Hovering Mechanism reads
	// it, and only for Purple.
	ArmorChassis string
	// PicksOnThisCompanion counts how many times the entry was taken on the
	// companion being computed; PicksOnAnyCompanion counts it across the
	// character's whole roster. Both are needed because the two shapes
	// genuinely occur: Bulky Build stacks per companion (up to 3), while
	// Red Technique's Enhanced Durability is one take that raises the HP of
	// every Puppet Tool at once.
	PicksOnThisCompanion int
	PicksOnAnyCompanion  int
}

// StatBonus is one upgrade's contribution to a companion's numbers.
type StatBonus struct {
	AC    int
	MaxHP int
	Speed int
	// FlySpeed is a total, not a delta, whenever GrantsFlight is set: the
	// upgrade grants a flying speed a puppet did not otherwise have, so
	// there is no base to add to. Both fields exist because "increase an
	// existing flying speed by +20" and "gain 30 feet of flying speed" are
	// different statements and the same upgrade makes both.
	FlySpeed     int
	GrantsFlight bool
}

func (b StatBonus) IsZero() bool {
	return b.AC == 0 && b.MaxHP == 0 && b.Speed == 0 && b.FlySpeed == 0 && !b.GrantsFlight
}

// StatBonusParts renders one bonus as the short phrases shown on the
// card ("+1 AC", "+15 ft. Speed"). A granted flying speed reads as a total
// rather than a signed delta because that is what it is — the puppet had
// none before.
func StatBonusParts(b StatBonus) []string {
	signed := func(n int) string {
		if n >= 0 {
			return "+" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	var parts []string
	if b.AC != 0 {
		parts = append(parts, signed(b.AC)+" AC")
	}
	if b.MaxHP != 0 {
		parts = append(parts, signed(b.MaxHP)+" Max HP")
	}
	if b.Speed != 0 {
		parts = append(parts, signed(b.Speed)+" ft. Speed")
	}
	if b.GrantsFlight {
		parts = append(parts, strconv.Itoa(b.FlySpeed)+" ft. flying speed")
	}
	return parts
}

// StatBonusEntry is one modeled upgrade: the entry it applies to,
// the name shown next to its computed numbers, and the formula.
type StatBonusEntry struct {
	EntrySlug string
	Name      string
	Bonus     func(BonusContext) StatBonus
}

// StatBonuses is group 1 above, companion side. Order is the
// order the hints render in.
var StatBonuses = []StatBonusEntry{
	{
		EntrySlug: EnhancedDurabilityEntrySlug,
		Name:      "Enhanced Durability",
		Bonus:     enhancedDurabilityBonus,
	},
	{
		EntrySlug: BulkyBuildEntrySlug,
		Name:      "Bulky Build",
		Bonus:     bulkyBuildBonus,
	},
	{
		EntrySlug: QuickfootedEntrySlug,
		Name:      "Quickfooted",
		Bonus:     quickfootedBonus,
	},
	{
		EntrySlug: HoveringMechanismEntrySlug,
		Name:      "Hovering Mechanism",
		Bonus:     hoveringMechanismBonus,
	},
}

// enhancedDurabilityBonus: "This Upgrade changes depending on your chosen
// technique;
//   - Black/Blue/Green: Increase the hit points of your Puppet Tool by +10.
//     For each level after 2nd level, increase your Puppet's hit points by
//     +2. You can take this upgrade multiple times, once for each puppet
//     tool you have. [per-companion: only the puppet holding the pick]
//   - Purple: Your armor gains a +1 bonus to its AC calculation, and
//     increases the Reinforced property by 3 [the Reinforced term is
//     applied to the chassis's own property list, see
//     ChassisReinforcedBonus]
//   - Red: Increase the hit points of all of your Puppet Tools by +5. For
//     each level after 2nd level, increase their hit points by +1. [one
//     take, every companion benefits]
//   - White: Increase your hit points by +1 for each level you have in this
//     class. [the CHARACTER's own max HP — see
//     CharacterBonuses, not here]"
func enhancedDurabilityBonus(ctx BonusContext) StatBonus {
	levelsAfter2nd := ctx.MasterLevel - 2
	if levelsAfter2nd < 0 {
		levelsAfter2nd = 0
	}
	switch ctx.SubclassColor {
	case "Black", "Blue", "Green":
		if ctx.PicksOnThisCompanion > 0 {
			return StatBonus{MaxHP: 10 + 2*levelsAfter2nd}
		}
	case "Purple":
		if ctx.PicksOnThisCompanion > 0 {
			return StatBonus{AC: 1}
		}
	case "Red":
		if ctx.PicksOnAnyCompanion > 0 {
			return StatBonus{MaxHP: 5 + 1*levelsAfter2nd}
		}
	}
	return StatBonus{}
}

// bulkyBuildBonus: "Your Puppet Tool gains a +1 to its AC, and reduces all
// damage it receives by -2. You can take this upgrade up to 3 times, per
// Puppet Tool." Both halves are unconditional; only the AC half has a field
// to land in (see group 3 in this file's header for the DR half). The cap
// is applied here as well as at pick time so a pre-existing over-cap set of
// rows — from any earlier version of the picker — can't inflate AC.
func bulkyBuildBonus(ctx BonusContext) StatBonus {
	takes := ctx.PicksOnThisCompanion
	if takes > 3 {
		takes = 3
	}
	return StatBonus{AC: takes}
}

// quickfootedBonus: "Your Puppet increases its movement speed by 15 feet...
// You can take this upgrade multiple times, once for each Puppet." Once per
// Puppet Tool means the per-companion count is 1 at most, so this is a flat
// +15 for the companion holding it; the count is still read rather than
// assumed so a duplicate row can't silently double it.
func quickfootedBonus(ctx BonusContext) StatBonus {
	if ctx.PicksOnThisCompanion > 0 {
		return StatBonus{Speed: 15}
	}
	return StatBonus{}
}

// hoveringMechanismBonus: "Your Puppet Tool gains 30 feet of flying speed.
// If your Puppet already has a flying speed, increase its flying speed by
// +20 feet... For those who practice the Red Technique, two Puppet Tools
// gain the benefits of this upgrade when you acquire it. For those who
// practice the Purple technique, if your Juggernaut Armor is Weaved Mail or
// a Wooden Suit, increase the flying speed by +10 feet. If it is a Steel
// Fortress, decrease the flying speed by -10ft."
//
// The "already has a flying speed" branch is the granted-speed case only
// (an upgrade taken twice on one puppet); a flying speed the player typed
// in by hand is their own number and is never overwritten, the same rule
// every other computed default on this card follows.
//
// Red's "two Puppet Tools gain the benefits" is why the character-wide
// count matters: the pick is stored against one companion but reaches a
// second one. Which second one the book leaves to the player, so this
// grants it to every Puppet Tool and says so in the hint rather than
// silently choosing one.
func hoveringMechanismBonus(ctx BonusContext) StatBonus {
	held := ctx.PicksOnThisCompanion > 0
	if ctx.SubclassColor == "Red" && ctx.PicksOnAnyCompanion > 0 {
		held = true
	}
	if !held {
		return StatBonus{}
	}
	speed := 30
	if ctx.PicksOnThisCompanion > 1 {
		speed += 20 * (ctx.PicksOnThisCompanion - 1)
	}
	if ctx.SubclassColor == "Purple" {
		switch ctx.ArmorChassis {
		case "Weaved Mail", "Wooden Suit":
			speed += 10
		case "Steel Fortress":
			speed -= 10
		}
	}
	return StatBonus{FlySpeed: speed, GrantsFlight: true}
}

// CharacterBonus is group 1's character side: an upgrade whose
// own text modifies the PUPPET MASTER's numbers, not a companion's. Both
// members are Purple/White technique upgrades, where the "puppet" is worn
// armor or gloves rather than a separate creature, so the bonus genuinely
// lands on the character's own sheet.
type CharacterBonus struct {
	EntrySlug string
	Name      string
	// Speed/MaxHP are computed from the character's own Puppet Master level
	// and technique; colors lists which techniques the bonus reads for.
	Colors []string
	Speed  func(masterLevel int) int
	MaxHP  func(masterLevel int) int
}

var CharacterBonuses = []CharacterBonus{
	{
		// "Increase all movement speeds you possess by +10 feet."
		// (Armorer's Wood Tier.) No Colors gate: the sentence is about
		// whoever took the upgrade, with no per-technique split of its own,
		// and which techniques may take it at all is already decided by the
		// eligibility filter that let the pick be made in the first place.
		EntrySlug: AcceleratedMovementEntrySlug,
		Name:      "Accelerated Movement",
		Speed:     func(int) int { return 10 },
	},
	{
		// Enhanced Durability, White Technique: "Increase your hit points
		// by +1 for each level you have in this class. If your Weaver
		// Gloves are ever removed, you lose this increase to your maximum
		// hit points, though this cannot cause you to go below 1 hp." The
		// gloves-removed clause is a play-state condition this app has no
		// flag for; the bonus is computed as held, which matches the
		// overwhelmingly common case of a Puppet Master wearing their own
		// gloves.
		EntrySlug: EnhancedDurabilityEntrySlug,
		Name:      "Enhanced Durability",
		Colors:    []string{"White"},
		MaxHP:     func(masterLevel int) int { return masterLevel },
	},
}

// AppliesTo reports whether this bonus reads for a given Puppet Technique.
// An empty Colors list means the upgrade's own text draws no per-technique
// distinction, so it applies to whoever holds it.
func (b CharacterBonus) AppliesTo(color string) bool {
	if len(b.Colors) == 0 {
		return true
	}
	for _, c := range b.Colors {
		if strings.EqualFold(c, color) {
			return true
		}
	}
	return false
}

// ChassisReinforcedBonus is Enhanced Durability's remaining Purple
// term: "increases the Reinforced property by 3". Reinforced is an armor
// property whose numeric value is the DR it grants against Bludgeoning,
// Piercing and Slashing damage, written into the chassis's own property
// list as "Reinforced (4)" (Iron Shell) or "Reinforced (7)" (Steel
// Fortress). Weaved Mail and Wooden Suit have no Reinforced property at
// all, so there is nothing for the +3 to increase — the upgrade adds a
// value to a property, it does not grant the property.
func ChassisReinforcedBonus(subclassColor string, hasEnhancedDurability bool) int {
	if subclassColor == "Purple" && hasEnhancedDurability {
		return 3
	}
	return 0
}
