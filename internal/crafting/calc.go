package crafting

import (
	"math"
	"strings"
)

// SealPick is one seal chosen for a recipe and the options picked for it
// (option IDs, or a jutsu/summon slug — see ChoiceRule).
type SealPick struct {
	Slug    string   `json:"slug"`
	Choices []string `json:"choices,omitempty"`
}

// BaseSource is how the crafter obtains the base item at the chosen quality.
type BaseSource string

const (
	BaseCraft BaseSource = "craft" // forge it (half its value in materials)
	BaseBuy   BaseSource = "buy"   // pay its full value
	BaseOwn   BaseSource = "own"   // already owned: no cost, no downtime
)

// Crafting holds who does the work and with what. It only affects cost,
// downtime and the requirement warnings, never the finished item.
type Crafting struct {
	BaseSource    BaseSource `json:"base_source"`
	HireSealsmith bool       `json:"hire_sealsmith"`
	Participants  int        `json:"participants"`
	// Masters is how many participants have Mastery in the kit.
	Masters      int    `json:"masters"`
	KitTier      string `json:"kit_tier"`
	Proficient   bool   `json:"proficient"`
	HasForge     bool   `json:"has_forge"`
	HasSealForge bool   `json:"has_seal_forge"`
	// CrafterLevel is the lowest participant's level; every participant must
	// meet the seals' minimum level.
	CrafterLevel int `json:"crafter_level"`
}

// DefaultCrafting is a lone, proficient crafter with a standard kit and
// access to both forges.
func DefaultCrafting() Crafting {
	return Crafting{
		BaseSource: BaseCraft, Participants: 1, KitTier: "standard",
		Proficient: true, HasForge: true, HasSealForge: true, CrafterLevel: 1,
	}
}

// Recipe is a planned item: what it is, plus how it gets made.
type Recipe struct {
	Name     string
	BaseSlug string
	Quality  string
	Seals    []SealPick
	Crafting Crafting
}

// Message is a player-facing note as an i18n key and its format arguments.
type Message struct {
	Key  string
	Args []any
}

// ChosenOption is one resolved choice on an applied seal. Exactly one of
// LabelKey (a damage/creature type, translated by the caller) or Name (a
// jutsu or summon tribe, already localized) is set. Href links a jutsu to
// its detail page.
type ChosenOption struct {
	Value    string
	LabelKey string
	Name     string
	Href     string
}

// AppliedSeal is a seal on the finished item.
type AppliedSeal struct {
	Seal   Seal
	Chosen []ChosenOption
}

// Result is everything Calculate derives from a Recipe.
type Result struct {
	Base    Base
	HasBase bool
	Quality Quality
	Seals   []AppliedSeal

	SlotsUsed      int
	SlotsAvailable int
	// Rank is the item's rank, the highest of its seals' ranks; "" with no
	// seals. "If even one seal infused on your weapon is of a higher rank,
	// the weapon is counted as the highest rank imbued on it" (p.72).
	Rank     string
	MinLevel int
	CraftDC  int

	// Base item.
	BaseValue    float64 // catalogue price × quality multiplier
	BaseCost     float64 // what obtaining it costs, per BaseSource
	BaseDowntime int     // weeks to forge it, BaseCraft only

	// Seals.
	SealMaterials       float64 // sum of the seals' listed Ryo Cost
	SealCost            float64 // materials, or the Sealsmith's +50% price
	SealPrepDowntime    int     // the 1 Downtime spent finding materials
	SealWeeks           int     // the highest seal rank's Downtime Cost
	SealWeeksPerCrafter int     // SealWeeks split across participants
	FailureDowntime     int

	MarketValue float64 // BaseValue + SealMaterials
	TotalCost   float64 // BaseCost + SealCost

	Errors   []Message // the recipe is invalid and can't be saved
	Warnings []Message // valid, but a crafting requirement or table rule needs attention
}

// Valid reports whether the recipe can be saved.
func (r Result) Valid() bool { return len(r.Errors) == 0 }

// CraftsSeals reports whether the crafter imbues the seals personally (so
// the DC, downtime and requirements apply).
func (r Result) CraftsSeals(c Crafting) bool { return len(r.Seals) > 0 && !c.HireSealsmith }

func (r *Result) errorf(key string, args ...any) {
	r.Errors = append(r.Errors, Message{Key: key, Args: args})
}

func (r *Result) warnf(key string, args ...any) {
	r.Warnings = append(r.Warnings, Message{Key: key, Args: args})
}

// Normalize clamps crafter inputs into range and fills defaults, so a
// hand-edited or partial form never divides by zero.
func (c *Crafting) Normalize() {
	switch c.BaseSource {
	case BaseCraft, BaseBuy, BaseOwn:
	default:
		c.BaseSource = BaseCraft
	}
	c.Participants = clamp(c.Participants, 1, 20)
	c.Masters = clamp(c.Masters, 0, c.Participants)
	c.KitTier = KitTierByID(c.KitTier).ID
	c.CrafterLevel = clamp(c.CrafterLevel, 1, 20)
}

func clamp(v, lo, hi int) int {
	return int(math.Max(float64(lo), math.Min(float64(hi), float64(v))))
}

// Calculate prices and validates a recipe against the catalog.
func Calculate(c *Catalog, recipe Recipe) Result {
	recipe.Crafting.Normalize()
	cr := recipe.Crafting
	var out Result

	if strings.TrimSpace(recipe.Name) == "" {
		out.errorf("crafting.err.name_required")
	}
	base, ok := c.Base(recipe.BaseSlug)
	if !ok {
		out.errorf("crafting.err.no_base")
	}
	out.Base, out.HasBase = base, ok
	quality, ok := QualityByID(recipe.Quality)
	if !ok {
		out.errorf("crafting.err.bad_quality")
		quality = Qualities[0]
	}
	out.Quality = quality
	out.MinLevel = 1

	out.SlotsAvailable = quality.Slots
	if base.Kind == "armor" && base.HighQuality {
		out.SlotsAvailable++
	}

	families := map[string]bool{}
	hasDiceBonusSeal, hasDieDamageSeal := false, false
	for _, pick := range recipe.Seals {
		seal, ok := c.Seal(pick.Slug)
		if !ok {
			out.errorf("crafting.err.unknown_seal")
			continue
		}
		if out.HasBase && seal.AppliesTo != base.Kind {
			out.errorf("crafting.err.wrong_kind", seal.Name)
			continue
		}
		if families[seal.Family] {
			out.errorf("crafting.err.same_family", seal.Name)
			continue
		}
		families[seal.Family] = true

		applied := AppliedSeal{Seal: seal}
		if seal.Choice != nil {
			chosen, valid := resolveChoices(c, *seal.Choice, pick.Choices)
			if !valid {
				out.errorf("crafting.err.choice", seal.Name)
			}
			applied.Chosen = chosen
			if seal.Choice.Kind == ChoiceJutsu {
				out.warnf("crafting.warn.storing_check_gm", seal.Name)
			}
		}
		out.Seals = append(out.Seals, applied)

		rule := seal.RankRule()
		out.SlotsUsed += rule.Slots
		out.SealMaterials += seal.CostRyo
		if out.Rank == "" || rankIndex(seal.Rank) > rankIndex(out.Rank) {
			out.Rank = seal.Rank
		}
		if seal.AppliesTo == "weapon" {
			if (seal.Family == "elemental-seal" || seal.Family == "medical-seal") &&
				(seal.Rank == "B" || seal.Rank == "S") {
				hasDiceBonusSeal = true
			}
			if dieDamageFamilies[seal.Family] {
				hasDieDamageSeal = true
			}
		}
	}
	if out.SlotsUsed > out.SlotsAvailable {
		out.errorf("crafting.err.too_many_slots", out.SlotsUsed, out.SlotsAvailable)
	}
	if hasDiceBonusSeal && hasDieDamageSeal {
		out.warnf("crafting.warn.dice_bonus_nonstacking")
	}

	if out.Rank != "" {
		rule, _ := RankRuleFor(out.Rank)
		out.MinLevel = rule.MinLevel
		out.CraftDC = rule.CraftDC
		out.FailureDowntime = rule.FailureDowntime
		out.SealWeeks = rule.DowntimeWeeks
	}

	priceBase(&out, cr)
	priceSeals(&out, cr)
	out.MarketValue = out.BaseValue + out.SealMaterials
	out.TotalCost = out.BaseCost + out.SealCost
	checkRequirements(&out, cr)
	return out
}

func priceBase(out *Result, cr Crafting) {
	if !out.HasBase {
		return
	}
	out.BaseValue = out.Base.CostRyo * out.Quality.Multiplier(out.Base.Kind)
	switch cr.BaseSource {
	case BaseOwn:
	case BaseBuy:
		out.BaseCost = out.BaseValue
	case BaseCraft:
		// "you must expend ryo worth half the total value" (p.71).
		out.BaseCost = out.BaseValue / 2
		progress := progressPerCrafter*(cr.Participants-cr.Masters) +
			progressPerMaster*cr.Masters + KitTierByID(cr.KitTier).ProgressBonus
		if out.BaseValue > 0 {
			out.BaseDowntime = int(math.Ceil(out.BaseValue / float64(progress)))
		}
	}
}

func priceSeals(out *Result, cr Crafting) {
	if len(out.Seals) == 0 {
		return
	}
	if cr.HireSealsmith {
		// "Increase the cost of your chosen seal(s) by 50% and immediately
		// gain the seal once paid" (p.72).
		out.SealCost = out.SealMaterials * hireSealsmithMultiplier
		return
	}
	out.SealCost = out.SealMaterials
	out.SealPrepDowntime = sealPrepDowntime
	// "Each creature splits the Ryo cost and Downtime required as equally as
	// possible amongst each other" (p.72); never below 1 week.
	out.SealWeeksPerCrafter = int(math.Max(1, math.Ceil(float64(out.SealWeeks)/float64(cr.Participants))))
}

// checkRequirements adds a warning for every crafting precondition the
// crafter inputs don't meet. These never invalidate the recipe: the planned
// item is the same, only who can make it changes.
func checkRequirements(out *Result, cr Crafting) {
	craftsBase := out.HasBase && cr.BaseSource == BaseCraft
	craftsSeals := out.CraftsSeals(cr)

	if out.HasBase && cr.BaseSource == BaseBuy && out.Quality.ID != "standard" {
		out.warnf("crafting.warn.rare_quality")
	}
	if !craftsBase && !craftsSeals {
		return
	}
	if !cr.Proficient {
		out.warnf("crafting.warn.no_proficiency", kitKey(out.Base.Kind))
	}
	if craftsBase {
		if !cr.HasForge {
			out.warnf("crafting.warn.no_forge")
		}
		switch out.Quality.Requirement {
		case RequireKitMastery:
			if cr.Masters < 1 {
				out.warnf("crafting.warn.needs_mastery", kitKey(out.Base.Kind))
			}
		case RequireThreeMasters:
			if cr.Masters < 3 {
				out.warnf("crafting.warn.needs_three_masters", kitKey(out.Base.Kind))
			}
		}
	}
	if craftsSeals {
		if !cr.HasSealForge {
			out.warnf("crafting.warn.no_seal_forge")
		}
		if cr.CrafterLevel < out.MinLevel {
			out.warnf("crafting.warn.crafter_level", out.MinLevel)
		}
		if out.Base.Kind == "armor" && cr.KitTier == "standard" &&
			rankIndex(out.Rank) > rankIndex(standardArmorsmithMaxRank) {
			out.warnf("crafting.warn.armor_kit_rank")
		}
	}
}

// kitKey is the i18n key naming the toolkit a base kind is crafted with.
func kitKey(kind string) string {
	if kind == "armor" {
		return "crafting.kit_name.armorsmith"
	}
	return "crafting.kit_name.weaponsmith"
}

// resolveChoices validates picks against rule and resolves them for
// display. The returned options keep whatever did resolve even when the
// whole pick is invalid, so the form can show them back.
func resolveChoices(c *Catalog, rule ChoiceRule, picks []string) ([]ChosenOption, bool) {
	valid := len(picks) == rule.Count
	seen := map[string]bool{}
	var out []ChosenOption
	for _, v := range picks {
		if seen[v] {
			valid = false
			continue
		}
		seen[v] = true
		switch rule.Kind {
		case ChoiceDamageType, ChoiceCreatureTypes:
			if !contains(rule.Options, v) {
				valid = false
				continue
			}
			prefix := "crafting.damage."
			if rule.Kind == ChoiceCreatureTypes {
				prefix = "crafting.creature."
			}
			out = append(out, ChosenOption{Value: v, LabelKey: prefix + v})
		case ChoiceSummonTribe:
			s, ok := c.SummonOption(v)
			if !ok || !contains(rule.SummonTypes, s.SummonType) {
				valid = false
				continue
			}
			out = append(out, ChosenOption{Value: v, Name: s.Name})
		case ChoiceJutsu:
			j, ok := c.JutsuOption(v)
			if !ok || rankIndex(j.Rank) > rankIndex(rule.MaxJutsuRank) {
				valid = false
				continue
			}
			out = append(out, ChosenOption{Value: v, Name: j.Name, Href: "/jutsu/" + v})
		}
	}
	return out, valid
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
