// Package crafting plans chakra-enhanced weapons and armor (core book,
// Chapter 7 downtime crafting, printed pp. 71–73): a base weapon or armor
// from the equipment catalogue, a quality tier, and enhancement seals filling
// that quality's seal slots. Calculate turns a Recipe into slot use, item
// rank, level requirement, crafting DC, downtime and Ryo cost.
//
// The book's tables live here as Go data rather than in rules.db: they are
// short, fixed, and every number is cited to its printed page.
package crafting

import "strings"

// Quality is one of the five item qualities (p.71–72). ID doubles as the
// i18n key suffix (crafting.quality.<id>).
type Quality struct {
	ID               string
	Slots            int
	WeaponMultiplier float64
	ArmorMultiplier  float64
	Requirement      QualityRequirement
}

// QualityRequirement is the extra crafter requirement a quality imposes.
type QualityRequirement int

const (
	RequireNothing QualityRequirement = iota
	// Supreme (p.71): "Crafting an item of this quality requires Mastery
	// with the required Tool Kit."
	RequireKitMastery
	// Mastercraft (p.72): "requires 3 creatures with Mastery with the
	// Crafting Skill and the required Tool Kit."
	RequireThreeMasters
)

// Qualities in ascending order. Standard is the base quality every catalogue
// item is listed at; "All base mundane weapons and armor have the potential
// to have up to 3 Seals imbued upon them" (p.72).
var Qualities = []Quality{
	{ID: "standard", Slots: 3, WeaponMultiplier: 1, ArmorMultiplier: 1},
	{ID: "greater", Slots: 5, WeaponMultiplier: 5, ArmorMultiplier: 1.5},
	{ID: "superior", Slots: 7, WeaponMultiplier: 7, ArmorMultiplier: 1.7},
	{ID: "supreme", Slots: 9, WeaponMultiplier: 10, ArmorMultiplier: 2, Requirement: RequireKitMastery},
	{ID: "mastercraft", Slots: 11, WeaponMultiplier: 15, ArmorMultiplier: 2.5, Requirement: RequireThreeMasters},
}

// QualityByID looks up a quality by its ID.
func QualityByID(id string) (Quality, bool) {
	for _, q := range Qualities {
		if q.ID == id {
			return q, true
		}
	}
	return Quality{}, false
}

// Multiplier is the quality's base-price multiplier for kind ("weapon" or
// "armor").
func (q Quality) Multiplier(kind string) float64 {
	if kind == "armor" {
		return q.ArmorMultiplier
	}
	return q.WeaponMultiplier
}

// RankRule is one row of the four per-rank seal tables (p.72): Minimum Level
// Requirement, Minimum Crafting DC, Slot Allotment and Downtime Cost, plus the
// failure rule's "D-Rank: 1, C-Rank: 2, B-Rank: 3…" downtime.
type RankRule struct {
	Rank            string
	Slots           int
	MinLevel        int
	CraftDC         int
	DowntimeWeeks   int
	FailureDowntime int
}

var rankRules = []RankRule{
	{Rank: "D", Slots: 1, MinLevel: 1, CraftDC: 14, DowntimeWeeks: 2, FailureDowntime: 1},
	{Rank: "C", Slots: 2, MinLevel: 5, CraftDC: 18, DowntimeWeeks: 4, FailureDowntime: 2},
	{Rank: "B", Slots: 3, MinLevel: 9, CraftDC: 22, DowntimeWeeks: 8, FailureDowntime: 3},
	{Rank: "A", Slots: 4, MinLevel: 13, CraftDC: 26, DowntimeWeeks: 12, FailureDowntime: 4},
	{Rank: "S", Slots: 5, MinLevel: 17, CraftDC: 30, DowntimeWeeks: 20, FailureDowntime: 5},
}

// RankRuleFor returns the seal-rank row for rank ("D"…"S").
func RankRuleFor(rank string) (RankRule, bool) {
	for _, r := range rankRules {
		if r.Rank == rank {
			return r, true
		}
	}
	return RankRule{}, false
}

// rankIndex orders jutsu/seal ranks E < D < C < B < A < S; -1 for anything
// else.
func rankIndex(rank string) int { return strings.Index("EDCBAS", rank) }

// KitTier is one of the four Armorsmith/Weaponsmith Kit variants (p.71).
// ID doubles as the i18n key suffix (crafting.kit.<id>).
type KitTier struct {
	ID string
	// ProgressBonus is the "increase the per week Market value contribution
	// on items you are crafting with this kit" amount.
	ProgressBonus int
	// CheckBonus is the "+1d4 bonus to all checks made to create" the
	// Greater and better kits grant; shown, not rolled.
	CheckBonus bool
}

var KitTiers = []KitTier{
	{ID: "standard"},
	{ID: "greater", CheckBonus: true},
	{ID: "superior", ProgressBonus: 100, CheckBonus: true},
	{ID: "supreme", ProgressBonus: 150, CheckBonus: true},
}

// KitTierByID looks up a kit tier, defaulting to the standard kit.
func KitTierByID(id string) KitTier {
	for _, k := range KitTiers {
		if k.ID == id {
			return k
		}
	}
	return KitTiers[0]
}

// Base (non-enhanced) crafting progress per downtime week (p.71): "you can
// craft one or more items with a total value not exceeding 100 Ryo", raised
// to 150 by "at least 1 rank of Mastery in your chosen tool"; each helper
// contributes the same.
const (
	progressPerCrafter = 100
	progressPerMaster  = 150
)

// Seal-crafting premiums (p.72): a Sealsmith imbues a seal for 50% more and
// the seal is gained immediately; finding materials costs 1 Downtime before
// the seal work begins.
const (
	hireSealsmithMultiplier = 1.5
	sealPrepDowntime        = 1
)

// standardArmorsmithMaxRank is the plain Armorsmith Kit's cap (p.40: "grants
// a creature the ability to create up to C-Rank Armor Seals. Higher quality
// Armorsmith Kits, will allow a creature to create higher ranked armor
// seals"). The Weaponsmith Kit has no such cap ("Chakra-Enhanced Weapons of
// any type").
const standardArmorsmithMaxRank = "C"
