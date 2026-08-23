package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Cooking-Nin's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Cooking-Nin entry documents that don't already fit the
// generic customResourceGrants pool (see custom_resources.go for Shinobi
// Snacks, Wandering Aroma, War and Food, the 9 subclasses' own bonus-Aura
// pools, and the other Group 1 use-count pools): the Cooking Die's own
// size readout (1d4->1d12, referenced by nearly every base and subclass
// feature but never surfaced per-character before), Food For the Soul's
// freely re-editable ability-score pick, and the Gastrochemist subclass's
// own Nature's Blend Enhancement pick — a cap-gated (jutsu, Enhancement
// type) choice sourced from the character's own known jutsu, filtered by
// their Nature's Blend release element(s) (elemental_affinity.go), same
// "known-jutsu-as-catalog" shape ninjutsu_specialist.go's Refined Ninjutsu
// pioneered, with one extra dimension (the Enhancement type) alongside it.
//
// Everything else this pass deferred — Revv Up Those Friers' fried-pill
// size readout, the always-on debuff/buff payloads behind the various
// use-count gates above (including the Enhancements' own chakra-spend-to-
// apply mechanic once picked), per-Snack-ingredient effect automation, and
// any mechanic needing a subsystem this app has no shape for yet — is
// documented, not modeled; see CLASS_AUDIT.md's Cooking-Nin detail entry.
const cookingNinSlug = "class/cooking-nin"

// foodForTheSoulFeatureSlug is the base class feature ("Whenever you would
// complete a short or Long Rest, select one ability score of your
// choice...") whose pick this box records.
const foodForTheSoulFeatureSlug = "class/cooking-nin/feature/food-for-the-soul"

// Three archetype-training feats (Chef Trainee -> Chefs Expert -> Chefs
// Specialist) grant a taste of these mechanics to a character with zero
// real Cooking-Nin class levels — each feat's own prerequisites text ("You
// cannot have Levels in the Cooking-Nin Class") makes real levels and this
// feat progression mutually exclusive, same shape as Hunter-Nin's own
// archetype-training feats (see hunter_nin.go). loadCookingNinTabData's own
// level gate widens to also open for these feats; the matching Shinobi
// Snacks pool bump lives in custom_resources.go, keyed by the same feat
// slugs. Chef Trainee's own "Cooking Tool Infusion feature as though you
// were a Level 1 Cooking Nin" clause and its Cooking Tools proficiency
// clause are not modeled — no Cooking Tool Infusion mechanism exists
// anywhere in this app yet, and tool proficiencies are an established
// out-of-scope boundary.
const (
	chefTraineeFeatSlug     = "feat/class/chef-trainee"
	chefsExpertFeatSlug     = "feat/class/chefs-expert"
	chefsSpecialistFeatSlug = "feat/class/chefs-specialist"
)

// cookingArchetypeFeats reports which of Chef Trainee/Chefs Expert/Chefs
// Specialist a character holds. Specialist requires Expert as its own
// prerequisite, which requires Trainee — a character with a deeper tier is
// assumed to also carry every shallower tier's own benefits, same
// "trust the player" boundary huntersArchetypeGrants already draws for its
// own tiered archetype chain.
type cookingArchetypeFeats struct {
	Trainee, Expert, Specialist bool
}

// loadCookingArchetypeFeats scans a character's merged granted-features
// list (feats included, via mergeFeatFeatures) for the 3 archetype-training
// slugs. Named "granted", not "features", so it doesn't shadow this file's
// own internal/features package import.
func loadCookingArchetypeFeats(granted []grantedFeatureRow) cookingArchetypeFeats {
	var a cookingArchetypeFeats
	for _, f := range granted {
		switch f.Slug {
		case chefTraineeFeatSlug:
			a.Trainee = true
		case chefsExpertFeatSlug:
			a.Expert = true
		case chefsSpecialistFeatSlug:
			a.Specialist = true
		}
	}
	return a
}

// any reports whether the character holds any of the 3 archetype-training
// feats — the caller's own gating signal for widening the sheet_cooking_nin
// box's existence beyond real Cooking-Nin levels.
func (a cookingArchetypeFeats) any() bool {
	return a.Trainee || a.Expert || a.Specialist
}

// dieSize returns the flat Cooking Die size the highest archetype tier held
// grants in place of the real class's level-scaled chart (cookingDieSize)
// — "a Cooking Dice Equal to 1d4" (Chef Trainee), "Your Cooking Dice
// becomes 1d6" (Chefs Expert), "...becomes 1d8" (Chefs Specialist). ""
// if the character holds none of the three.
func (a cookingArchetypeFeats) dieSize() string {
	switch {
	case a.Specialist:
		return "1d8"
	case a.Expert:
		return "1d6"
	case a.Trainee:
		return "1d4"
	default:
		return ""
	}
}

// foodForTheSoulOptions: freely re-editable any time (RAW re-picks on every
// short or Long Rest; this app doesn't enforce rest timing, same "trust the
// player" boundary Hand Wraps of Passion/Mastery already draw). The applied
// effect itself (Advantage on allies' first check/save with the chosen
// score) stays a manual/Group 2-3 boundary — no advantage-flag tracking
// exists anywhere in this app — so this only records WHICH score is
// currently selected.
var foodForTheSoulOptions = []featureChoiceOption{
	{Value: "str", Label: "Strength", Description: "Allied creatures gain Advantage on their first Strength Ability Check and Strength saving throw before your next rest."},
	{Value: "dex", Label: "Dexterity", Description: "Allied creatures gain Advantage on their first Dexterity Ability Check and Dexterity saving throw before your next rest."},
	{Value: "con", Label: "Constitution", Description: "Allied creatures gain Advantage on their first Constitution Ability Check and Constitution saving throw before your next rest."},
	{Value: "int", Label: "Intelligence", Description: "Allied creatures gain Advantage on their first Intelligence Ability Check and Intelligence saving throw before your next rest."},
	{Value: "wis", Label: "Wisdom", Description: "Allied creatures gain Advantage on their first Wisdom Ability Check and Wisdom saving throw before your next rest."},
	{Value: "cha", Label: "Charisma", Description: "Allied creatures gain Advantage on their first Charisma Ability Check and Charisma saving throw before your next rest."},
}

// fastAndFuriousFeatureSlug is Entremetier Chef's 2nd-level feature slug —
// "you can use your intelligence or charisma instead of your dexterity for
// Initiative rolls." This is a separate, same-named constant from
// internal/charsheet's own copy — that package can't import cmd/n5e (cmd/n5e
// imports charsheet, not the reverse), so the literal is duplicated rather
// than shared, same precedent as gastrochemistNaturesBlendFeatureSlug's own
// cross-package siblings.
const fastAndFuriousFeatureSlug = "class/cooking-nin/group/cooking-focus/entremetier-chef/feature/fast-and-furious"

// fastAndFuriousOptions: freely re-editable any time, same "trust the
// player" boundary foodForTheSoulOptions draws above — the book gives no
// rest-timing restriction on this pick at all, unlike Food For the Soul's
// own per-rest re-pick. Only WHICH ability is picked is tracked; charsheet.go
// reads it back as InitiativeAbility's own default (still overridable by the
// sheet's existing Adjust-Initiative ability dropdown).
var fastAndFuriousOptions = []featureChoiceOption{
	{Value: "int", Label: "Intelligence", Description: "Use Intelligence instead of Dexterity for Initiative rolls."},
	{Value: "cha", Label: "Charisma", Description: "Use Charisma instead of Dexterity for Initiative rolls."},
}

// gastrochemistNaturesBlendFeatureSlug is Nature's Blend's own 2nd-level
// feature slug — the same slug gastrochemistAffinitySlots
// (elemental_affinity.go) keys its own "natures-blend" elemental-release
// pick to. Reused here to gate the Enhancement picker below on the
// character actually holding the Gastrochemist subclass, not merely
// Cooking-Nin levels of any subclass.
const gastrochemistNaturesBlendFeatureSlug = "class/cooking-nin/group/cooking-focus/gastrochemist/feature/natures-blend"

// gastrochemistEnhancementCap: Nature's Blend's own pick count — "choose a
// Jutsu of your Nature's Blend release, and give it one of the following
// Enhancements" at 2nd level, "You may choose an additional Jutsu to gain
// an Enhancement at 3rd, 5th, 9th, 13th, and 17th Level." No
// class_level_resources chart exists for this (Cooking-Nin's only chart row
// is "Cooking Die") — hand-curated the same way ninjutsuMasterCap is.
func gastrochemistEnhancementCap(level int) int {
	switch {
	case level >= 17:
		return 6
	case level >= 13:
		return 5
	case level >= 9:
		return 4
	case level >= 5:
		return 3
	case level >= 3:
		return 2
	case level >= 2:
		return 1
	default:
		return 0
	}
}

// gastrochemistEnhancementOptions: the 4 Enhancement types Nature's Blend
// lets a chosen Jutsu be given. Only WHICH type was chosen for a given Jutsu
// is tracked by this pick — the applied effect (spend Chakra equal to half
// the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to
// boost the Jutsu's DC/damage/healing-THP-DR/range at cast time, scaled by
// Cooking Dice and the Jutsu's Rank) stays fully Group 2/3, same boundary
// Food For the Soul's own applied Advantage effect draws above — no
// chakra-spend-at-cast-time mechanism exists anywhere in this app.
var gastrochemistEnhancementOptions = []featureChoiceOption{
	{Value: "texture", Label: "Enhance Texture", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase the Jutsu's DC by half your Cooking Dice result — capped at 1 Cooking Die for D/C-Rank Jutsu, 2 for B/A-Rank, 3 for S-Rank."},
	{Value: "kick", Label: "Enhance Kick", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase the Jutsu's damage dice by a number of Cooking Dice set by its Rank (1 for D/C, 2 for B/A, 3 for S) — or deal that much damage to a creature that fails its save if the Jutsu has no damage dice — once per casting."},
	{Value: "temperature", Label: "Enhance Temperature", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase Healing, THP, or DR the Jutsu provides by twice your Cooking Dice result times its Rank tier, or by half a Cooking Die if the Jutsu's duration is longer than instant."},
	{Value: "aroma", Label: "Enhance Aroma", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase the Jutsu's Range, including any Area of Effect it creates, by 5 times half your Cooking Dice result."},
}

// gastrochemistEnhancementLabel resolves a stored enhancement_type value
// back to its display label, for the Known list.
func gastrochemistEnhancementLabel(value string) string {
	for _, o := range gastrochemistEnhancementOptions {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

// patissierChefByTheBookFeatureSlug is Patissier Chef's "Gotta Do the
// Cooking By the Book" (5th level): "You can learn any Jutsu with the
// Medical release that recovers Hit Points, or gives temp Hit Points, and
// may use Charisma as your Jutsu Modifier for them. When you would heal a
// creature using Jutsu you may, as a part of the same Action, feed them a
// Snack" — the jutsu-access half is a curated eligibility grant
// (patissierChefByTheBookHealingJutsuSlugs, wired into jutsuEligible) and
// the Charisma half is a per-row AttackAbility/DamageAbility override
// (loadCharacterJutsuSheet); the Snack-feeding clause stays narrated, same
// as every other "as a part of the same Action" flavor clause in this
// class (see this file's own package doc comment).
const patissierChefByTheBookFeatureSlug = "class/cooking-nin/group/cooking-focus/patissier-chef/feature/gotta-do-the-cooking-by-the-book"

// patissierChefByTheBookHealingJutsuSlugs curates which of the 91
// "Medical Release: X" jutsu (jutsu/medical-release-*, dist/rules.db)
// actually "recover Hit Points, or give temp Hit Points" per By the
// Book's own text above. No column distinguishes this — the Medical
// keyword covers both the release's supportive/healing half and its acid/
// poison/necrotic offensive half equally — so this list was built by
// hand-reading all 91 jutsu's full description text, not a live query.
// Confirmed 2026-08-22: a crude "%Hit Point%" substring search over the
// same 91 rows produces both false positives (Blood Rot Curse, Corrosive
// Plume, Dead Heartbeat Technique, Grim Weapon, Impending End, and Plague
// all mention "Hit Points" while being purely harmful, with no recovery
// for the caster or an ally) and false negatives (Heal uses "restore"
// rather than "regain"/"recover"), so it cannot be computed live.
var patissierChefByTheBookHealingJutsuSlugs = map[string]bool{
	"jutsu/medical-release-acid-armor":          true, // self Temporary Hit Points
	"jutsu/medical-release-adept-healing":       true,
	"jutsu/medical-release-aid":                 true,
	"jutsu/medical-release-aura-of-life":        true,
	"jutsu/medical-release-goodberry":           true,
	"jutsu/medical-release-grim-siphon":         true, // an ally "regains Hit Points equal to the damage dealt"
	"jutsu/medical-release-heal":                true,
	"jutsu/medical-release-healing-elixir":      true,
	"jutsu/medical-release-healing-wave":        true,
	"jutsu/medical-release-medical-aura":        true,
	"jutsu/medical-release-reconstructive-hand": true,
	"jutsu/medical-release-regenerate":          true,
	"jutsu/medical-release-revival":             true, // returns the target to 1 Hit Point
	"jutsu/medical-release-strength-of-1000":    true, // "regenerate Hit Points ... at the start of each of your turns"
	"jutsu/medical-release-transfer-of-life":    true,
	"jutsu/medical-release-vampiric-touch":      true, // caster "regain Hit Points equal to half" the damage dealt
	"jutsu/medical-release-virtue":              true,
	"jutsu/medical-release-wither-and-bloom":    true,
}

// cookingToolInfusionFeatureSlug is the base class feature (1st level)
// granting the Cooking Tool itself: an implement, a damage type, and a
// weapon property at 1st level, one more property at 6th, and one more at
// 11th (class_features.description's own prose — see this file's own
// package doc comment and CLASS_AUDIT.md for the full verbatim text). The
// 9 subclasses' own "Bonus Tool Infusion" 2nd/3rd-level features each grant
// a second, wholly bespoke weapon with subclass-specific (non-generic)
// properties — out of scope here; only the base weapon this feature grants
// is modeled.
const cookingToolInfusionFeatureSlug = "class/cooking-nin/feature/cooking-tool-infusion"

// The 5 independent picks Cooking Tool Infusion offers are stored as
// separate ChoiceIndex slots under cookingToolInfusionFeatureSlug in the
// shared character_feature_choices table (features.LoadFeatureChoices/
// charstore.SetFeatureChoice) — the same "one feature slug, several
// independent slots" shape Food For the Soul/Fast and Furious already use
// above, just with more slots. No dedicated table needed.
//
// Unlike Food For the Soul/Fast and Furious (both explicitly re-pickable at
// will by their own rule text), each of these 5 slots is locked once it
// holds a non-empty value: the feature's own text ("You gain a Cooking
// Tool... you can choose a damage type... and one of the following
// properties...", repeated once per level tier) describes each pick as a
// one-time selection made at a specific level, with no mention anywhere of
// re-selecting or swapping it afterward. Silence read as permanent is the
// same reasoning this app already applies to Titan Specialization
// (charstore.SetCompanionFields' titan_specialization CASE guard) and
// Purple Technique's Armor Chassis (armor_chassis CASE guard, same
// function) — both equally silent on re-editability in their own printed
// text. character_feature_choices is a shared table whose own
// SetFeatureChoice/ClearFeatureChoice stay a plain upsert/delete (they also
// serve OTHER features that use the same table and genuinely are freely
// re-editable), so the lock is enforced by each of the 5 handlers below
// itself, checking the slot's current value before writing rather than in
// the shared storage functions.
const (
	cookingToolChoiceImplement = iota
	cookingToolChoiceDamageType
	cookingToolChoicePropertyL1
	cookingToolChoicePropertyL6
	cookingToolChoicePropertyL11
)

// cookingToolChoiceKey builds the features.ChoiceKey for one of the 5 slots
// above, so each handler's own lock check reads the same key
// loadCookingToolInfusionView's get closure and charstore.SetFeatureChoice/
// ClearFeatureChoice already address by.
func cookingToolChoiceKey(idx int) features.ChoiceKey {
	return features.ChoiceKey{FeatureSlug: cookingToolInfusionFeatureSlug, ChoiceIndex: idx}
}

// cookingToolDamageTypeOptions: "At 1st level you can choose a damage type
// between Bludgeoning, Piercing or Slashing."
var cookingToolDamageTypeOptions = []featureChoiceOption{
	{Value: "Bludgeoning", Label: "Bludgeoning", Description: "Your Cooking Tool deals Bludgeoning damage."},
	{Value: "Piercing", Label: "Piercing", Description: "Your Cooking Tool deals Piercing damage."},
	{Value: "Slashing", Label: "Slashing", Description: "Your Cooking Tool deals Slashing damage."},
}

// cookingToolPropertyL1Options: 1st level's own property table — "one
// following properties: Blocking, Hidden, Thrown (45/90), Unarmed,
// Deadly." Descriptions are transcribed from weapon_properties (rules.db)
// verbatim, since these are real, shared weapon properties, not bespoke
// Cooking-Nin text. Thrown's own printed range is fixed at 45/90 by this
// feature's own text, overriding the property's normal per-weapon range.
var cookingToolPropertyL1Options = []featureChoiceOption{
	{Value: "blocking", Label: "Blocking", Description: "While equipped, increase your AC by +1. This bonus can only be applied once. If applied to a weapon with the Unarmed property, you do not gain this benefit if using another weapon."},
	{Value: "hidden", Label: "Hidden", Description: "You have advantage on Dexterity (Sleight of Hand) checks made to conceal this weapon."},
	{Value: "thrown", Label: "Thrown (45/90)", Description: "You can throw this weapon to make a ranged attack (range 45/90) using your Strength instead of Dexterity for attack and damage rolls."},
	{Value: "unarmed", Label: "Unarmed", Description: "Equipped to both hands and cannot be disarmed. Uses your Unarmed Damage as its base damage (otherwise its listed die). Usable to cast both Taijutsu and Bukijutsu."},
	{Value: "deadly", Label: "Deadly", Description: "Adds +1 additional damage die on a critical hit, once per turn."},
}

// cookingToolPropertyL6Options: 6th level's own property table — "Light
// (you also gain an additional Cooking Tool, an exact copy of this one,
// usable only while wielding this Cooking Tool), Lethal 2, Returning (this
// can only be taken if your Cooking Tool has the Thrown Property),
// Critical, Trip, Disarm, Grapple." The last bullet is printed as one
// comma-joined property, read here as one combined pick granting all
// three, not three separate options — same reading multi-property
// equipment.properties rows elsewhere in this ruleset already use.
// Returning's own gate (requires "thrown" from the 1st-level pick) is
// enforced by handleCookingToolPropertyL6, not filtered out of this fixed
// list — filterCookingToolPropertyL6Options (below) does the actual
// per-character filtering.
var cookingToolPropertyL6Options = []featureChoiceOption{
	{Value: "light", Label: "Light", Description: "You also gain a second, exact-copy Cooking Tool, usable only while wielding this one. Once per turn, while wielding both, an attack with one lets you make a second attack with the other as part of the same action."},
	{Value: "lethal-2", Label: "Lethal 2", Description: "Increases this weapon's damage die by +2 against a surprised creature, or one that can't see its attacker, once per turn."},
	{Value: "returning", Label: "Returning", Description: "After throwing it to make a ranged attack, you can call it back to you if it was thrown less than 30 feet from you — one hand must be free to catch it. Requires the Thrown property."},
	{Value: "critical", Label: "Critical", Description: "+1 bonus to Critical Threat range when attacking with this weapon."},
	{Value: "trip-disarm-grapple", Label: "Trip, Disarm, Grapple", Description: "In place of one weapon attack, attempt to Trip/Shove, Disarm, or Grapple a target creature with this weapon."},
}

// cookingToolPropertyL11Options: 11th level's own property table — "Reach
// 3, Multiattack, Tactical, Critical 2 (this can only be taken if your
// Cooking Tool has the Critical Property)." Critical 2's own gate (requires
// "critical" from the 6th-level pick) is enforced by
// handleCookingToolPropertyL11, filtered per-character by
// filterCookingToolPropertyL11Options (below).
var cookingToolPropertyL11Options = []featureChoiceOption{
	{Value: "reach-3", Label: "Reach 3", Description: "Adds 15 feet to your reach when you attack with this weapon."},
	{Value: "multiattack", Label: "Multiattack", Description: "Can also be used to attack as a bonus action. That bonus-action attack adds no ability modifier or other bonus to its damage roll."},
	{Value: "tactical", Label: "Tactical", Description: "Once per turn, deals +1 additional damage to a target for each rank of any Physical condition the target has."},
	{Value: "critical-2", Label: "Critical 2", Description: "A second rank of Critical: a further +1 bonus to Critical Threat range (total +2) when attacking with this weapon. Requires the Critical property."},
}

// cookingToolOptionLabel resolves a stored option value back to its
// display label within one of the 4 hand-curated catalogs above — mirrors
// gastrochemistEnhancementLabel, generalized to take the catalog as a
// parameter since there are 4 of these instead of 1.
func cookingToolOptionLabel(options []featureChoiceOption, value string) string {
	for _, o := range options {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

// filterCookingToolPropertyL6Options drops Returning from the offered list
// unless the character's own 1st-level property pick is Thrown — except a
// character who already holds Returning keeps seeing it (so the select
// still shows their current pick even if the 1st-level pick was changed
// out from under it afterward; the "trust the player" boundary this app's
// picks already draw elsewhere, not a hard lock).
func filterCookingToolPropertyL6Options(propertyL1, current string) []featureChoiceOption {
	if propertyL1 == "thrown" {
		return cookingToolPropertyL6Options
	}
	out := make([]featureChoiceOption, 0, len(cookingToolPropertyL6Options))
	for _, o := range cookingToolPropertyL6Options {
		if o.Value == "returning" && current != "returning" {
			continue
		}
		out = append(out, o)
	}
	return out
}

// filterCookingToolPropertyL11Options drops Critical 2 unless the
// character's own 6th-level property pick is Critical — same "keep
// showing an already-made pick even if its own prerequisite pick changed
// later" treatment as filterCookingToolPropertyL6Options.
func filterCookingToolPropertyL11Options(propertyL6, current string) []featureChoiceOption {
	if propertyL6 == "critical" {
		return cookingToolPropertyL11Options
	}
	out := make([]featureChoiceOption, 0, len(cookingToolPropertyL11Options))
	for _, o := range cookingToolPropertyL11Options {
		if o.Value == "critical-2" && current != "critical-2" {
			continue
		}
		out = append(out, o)
	}
	return out
}

// cookingToolInfusionLevel resolves the effective level Cooking Tool
// Infusion's own picks gate against: the character's real Cooking-Nin
// level, or (for a character with none — the archetype-training feats'
// own prerequisite makes the two mutually exclusive) the level Chef
// Trainee/Chefs Expert/Chefs Specialist's own text grants this feature
// "as though" — 1st/6th/11th respectively, matching wardenWeaponCap's own
// "level > 0 wins outright, otherwise fall back to the archetype tier"
// shape.
func cookingToolInfusionLevel(level int, arch cookingArchetypeFeats) int {
	if level > 0 {
		return level
	}
	switch {
	case arch.Specialist:
		return 11
	case arch.Expert:
		return 6
	case arch.Trainee:
		return 1
	default:
		return 0
	}
}

// loadCookingToolImplementCatalog reads every Cooking Tool implement off
// the equipment table (slug prefix weapon/cooking-tool-, added by
// 0059_cooking_tool_infusion_implements.sql) — a real, DB-backed catalog
// rather than free text, same "read the catalog, don't hand-transcribe it"
// shape loadWardenWeaponCatalog already established, just filtered by slug
// prefix instead of weapon_category (Cooking Tool implements have no
// weapon_category of their own — they aren't a purchasable starting-gear
// weapon type).
func (s *server) loadCookingToolImplementCatalog() ([]featureChoiceOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(description, '')
		FROM equipment WHERE slug LIKE 'weapon/cooking-tool-%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []featureChoiceOption
	for rows.Next() {
		var o featureChoiceOption
		if err := rows.Scan(&o.Value, &o.Label, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// cookingToolInfusionView is the Cooking Tool Infusion box's own sub-panel
// — 5 independent picks (implement, damage type, and one property each at
// 1st/6th/11th level), each locked once chosen (see the doc comment above
// cookingToolChoiceImplement's own const block) and rendered as its own
// select-plus-Set-button row (same shape Food For the Soul/Fast and
// Furious above already use, since every one of these picks is a single
// current value, not a multi-entry known/available catalog the way Martial
// Techniques needs). Nil unless the character has reached the feature's
// own 1st-level gate (real or archetype-feat-equivalent).
type cookingToolInfusionView struct {
	Implement        string // equipment slug, "" unpicked
	ImplementName    string
	ImplementOptions []featureChoiceOption

	DamageType        string
	DamageTypeOptions []featureChoiceOption

	PropertyL1        string
	PropertyL1Label   string
	PropertyL1Options []featureChoiceOption

	PropertyL6Available bool // true once effective level >= 6
	PropertyL6          string
	PropertyL6Label     string
	PropertyL6Options   []featureChoiceOption

	PropertyL11Available bool // true once effective level >= 11
	PropertyL11          string
	PropertyL11Label     string
	PropertyL11Options   []featureChoiceOption
}

// loadCookingToolInfusionView loads every current pick plus the option
// lists the sheet's picker forms need — toolLevel is the value
// cookingToolInfusionLevel already resolved (0 means "don't call this at
// all", enforced by the caller).
func (s *server) loadCookingToolInfusionView(characterID int64, toolLevel int) (*cookingToolInfusionView, error) {
	catalog, err := s.loadCookingToolImplementCatalog()
	if err != nil {
		return nil, err
	}
	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	get := func(idx int) string {
		return choices[features.ChoiceKey{FeatureSlug: cookingToolInfusionFeatureSlug, ChoiceIndex: idx}]
	}

	v := &cookingToolInfusionView{
		Implement:         get(cookingToolChoiceImplement),
		ImplementOptions:  catalog,
		DamageType:        get(cookingToolChoiceDamageType),
		DamageTypeOptions: cookingToolDamageTypeOptions,
		PropertyL1:        get(cookingToolChoicePropertyL1),
		PropertyL1Options: cookingToolPropertyL1Options,
		PropertyL6:        get(cookingToolChoicePropertyL6),
		PropertyL11:       get(cookingToolChoicePropertyL11),
	}
	v.PropertyL1Label = cookingToolOptionLabel(cookingToolPropertyL1Options, v.PropertyL1)
	for _, o := range catalog {
		if o.Value == v.Implement {
			v.ImplementName = o.Label
			break
		}
	}
	if toolLevel >= 6 {
		v.PropertyL6Available = true
		v.PropertyL6Options = filterCookingToolPropertyL6Options(v.PropertyL1, v.PropertyL6)
		v.PropertyL6Label = cookingToolOptionLabel(cookingToolPropertyL6Options, v.PropertyL6)
	}
	if toolLevel >= 11 {
		v.PropertyL11Available = true
		v.PropertyL11Options = filterCookingToolPropertyL11Options(v.PropertyL6, v.PropertyL11)
		v.PropertyL11Label = cookingToolOptionLabel(cookingToolPropertyL11Options, v.PropertyL11)
	}
	return v, nil
}

// syncCookingToolInfusionInventory keeps the character's inventory in step
// with the implement pick: any previously auto-added Cooking Tool
// implement (slug prefix weapon/cooking-tool-) is removed first — there is
// only ever one, since the implement pick's own cap is 1 — before the
// newly picked one (if any) is added back, equipped by default so it
// immediately shows up in the Attacks table the same way any other
// equipped weapon does. newSlug == "" just clears the old one, for
// "un-pick my implement."
func (s *server) syncCookingToolInfusionInventory(characterID int64, newSlug string) error {
	rows, err := s.charDB.Query(
		`SELECT id FROM character_inventory WHERE character_id = ? AND item_slug LIKE 'weapon/cooking-tool-%'`,
		characterID)
	if err != nil {
		return err
	}
	var staleIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		staleIDs = append(staleIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range staleIDs {
		if err := charstore.DeleteInventoryItem(s.charDB, characterID, id); err != nil {
			return err
		}
	}
	if newSlug == "" {
		return nil
	}
	return charstore.AddInventoryItemWithEquipped(s.charDB, characterID, newSlug, 1, true)
}

// cookingToolInfusionAttackOverrides resolves the flat, always-on bonuses
// buildAttacks (characters.go) applies to whichever equipped item matches
// the character's own picked Cooking Tool implement: the level-scaling
// Cooking Die as its damage dice (in place of the catalog row's own inert
// 1d4), the player's chosen damage type, and a Critical-threat-range widen
// for the Critical/Critical 2 property picks (the one property whose
// numeric effect this app already has an existing per-attack mechanism
// for — weaponSpecialistCritRangeThreshold's own CritRangeThreshold field).
// implementSlug == "" means "no pick yet, or no Cooking Tool Infusion at
// all" — buildAttacks treats that as "never matches any equipped item",
// same "empty means untouched" shape hunterNinWardenWeaponBonuses already
// uses. Every other property (Blocking's AC, Deadly/Lethal's extra damage
// die, Light's second weapon, Multiattack's bonus-action attack, Reach,
// Tactical, Trip/Disarm/Grapple, Returning, Hidden) stays narrated, not
// modeled — the same boundary this app already draws around Hunter-Nin's
// own Warden Weapon Property pick.
func (s *server) cookingToolInfusionAttackOverrides(characterID int64, sheet *charsheet.Sheet) (implementSlug, dieSize, damageType string, critRangeBonus int, err error) {
	level, err := s.cookingNinClassLevel(characterID)
	if err != nil {
		return "", "", "", 0, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return "", "", "", 0, err
	}
	arch := loadCookingArchetypeFeats(grantedFeatures)
	toolLevel := cookingToolInfusionLevel(level, arch)
	if toolLevel == 0 {
		return "", "", "", 0, nil
	}
	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return "", "", "", 0, err
	}
	get := func(idx int) string {
		return choices[features.ChoiceKey{FeatureSlug: cookingToolInfusionFeatureSlug, ChoiceIndex: idx}]
	}
	implementSlug = get(cookingToolChoiceImplement)
	if implementSlug == "" {
		return "", "", "", 0, nil
	}
	if level > 0 {
		dieSize, err = s.cookingDieSize(level)
		if err != nil {
			return "", "", "", 0, err
		}
	} else {
		dieSize = arch.dieSize()
	}
	damageType = get(cookingToolChoiceDamageType)
	if get(cookingToolChoicePropertyL6) == "critical" {
		critRangeBonus++
	}
	if get(cookingToolChoicePropertyL11) == "critical-2" {
		critRangeBonus++
	}
	return implementSlug, dieSize, damageType, critRangeBonus, nil
}

// bonusToolInfusionPipeFeatureSlug is Herbalist's own 3rd-level "Bonus Tool
// Infusion: Pipe" feature — "you gain a second Cooking Tool Infusion
// weapon, following the same structure as your original one, but with some
// custom unique features... this Weapon is some kind of smoking
// implement." Same 5-independent-picks shape as the base
// cookingToolInfusionFeatureSlug above (implement, damage type, one
// property each at 3rd/6th/11th level instead of 1st/6th/11th), a wholly
// separate equipped weapon from the base Cooking Tool — a character can
// hold both simultaneously, each with its own implement, damage type, and
// property picks.
const bonusToolInfusionPipeFeatureSlug = "class/cooking-nin/group/cooking-focus/herbalist/feature/bonus-tool-infusion-pipe"

// The 5 independent picks Bonus Tool Infusion: Pipe offers are stored the
// same way the base feature's own 5 picks are (see the doc comment above
// cookingToolChoiceImplement's own const block) — separate ChoiceIndex
// slots under bonusToolInfusionPipeFeatureSlug in character_feature_choices,
// each locked once it holds a non-empty value, for the identical "the
// feature's own text describes a one-time pick with no mention of
// re-selecting" reasoning.
const (
	cookingToolPipeChoiceImplement = iota
	cookingToolPipeChoiceDamageType
	cookingToolPipeChoicePropertyL3
	cookingToolPipeChoicePropertyL6
	cookingToolPipeChoicePropertyL11
)

// cookingToolPipeChoiceKey builds the features.ChoiceKey for one of the 5
// Pipe slots above — mirrors cookingToolChoiceKey.
func cookingToolPipeChoiceKey(idx int) features.ChoiceKey {
	return features.ChoiceKey{FeatureSlug: bonusToolInfusionPipeFeatureSlug, ChoiceIndex: idx}
}

// cookingToolPipePropertyL3Options: the Pipe's own 3rd-level property
// table — "you can choose a damage type... and one following properties...
// Deep Breath, Didn't Know You Was Chill Like That, Herb In the Pipe."
// Transcribed verbatim from class_features.description. Every one of these
// 8 named Pipe properties (this tier and the 2 below) stays narrated text
// only — see this file's own package doc comment for the established
// boundary this already draws around the base Cooking Tool Infusion's own
// non-Blocking/Deadly/Critical properties; only WHICH property was picked
// is tracked and shown, none of the range-change/poison-kit-charge/
// jutsu-cost-halving effects described below are modeled as new resource
// pools or jutsu-cost overrides.
var cookingToolPipePropertyL3Options = []featureChoiceOption{
	{Value: "deep-breath", Label: "Deep Breath", Description: "As a bonus action when you would cast a Genjutsu with the inhaled keyword and a range other than Self, you can take a deep breath from your tool before blowing it out around you, the Genjutsu's Range becomes a 15ft Radius Sphere centered on yourself."},
	{Value: "didnt-know-you-was-chill-like-that", Label: "Didn't Know You Was Chill Like That", Description: "This tool counts as a Poison Kit, with 3 Charges, which it regains at the end of a Long Rest, for the purpose of casting Genjutsu with the inhale keyword."},
	{Value: "herb-in-the-pipe", Label: "Herb In the Pipe", Description: "When you would take a Long or Short Rest, you may choose two Jutsu with the Inhaled Keyword of a rank that you can cast. While wielding this Weapon, you may cast those Jutsu, once each, reducing their Chakra Costs by half, unless they have Special Cost."},
}

// cookingToolPipePropertyL6Options: "Starting at 6th level, your Cooking
// Tool gains an additional weapon property of your choice; Inhaled Herb,
// Deeper Breath, Harsh Inhale." Transcribed verbatim.
var cookingToolPipePropertyL6Options = []featureChoiceOption{
	{Value: "inhaled-herb", Label: "Inhaled Herb", Description: "When a creature makes a saving throw against a Genjutsu you cast with the inhaled keyword, you may spend 1 charge from a poison kit, giving the creature disadvantage on the next attack, skill check, or saving throw, other than the one that triggered this effect, until the end of their next turn."},
	{Value: "deeper-breath", Label: "Deeper Breath", Description: "Whenever you use your Deep Breath feature, increase the area of effect to 30ft, and you may change the saving throw to constitution."},
	{Value: "harsh-inhale", Label: "Harsh Inhale", Description: "While wielding this Weapon, increase the damage you deal to creatures with Genjutsu, with the Inhaled Keyword, by 1 Cooking Dice."},
}

// cookingToolPipePropertyL11Options: "Beginning at 11th level, your
// Cooking Tool gains one additional weapon property of your choice;
// Constant Smoke, Quick Inhale." Transcribed verbatim.
var cookingToolPipePropertyL11Options = []featureChoiceOption{
	{Value: "constant-smoke", Label: "Constant Smoke", Description: "There is a constant smoke stream of smoke emanating from your pipe, if you stay in a room for longer than 5 Minutes while holding this Weapon the room becomes filled with smoke, the room is under the effects of \"Water release: Hidden Mist\" as if you had cast it."},
	{Value: "quick-inhale", Label: "Quick Inhale", Description: "When you would cast an Inhaled Genjutsu as an Action, you can instead cast it as a Bonus Action, a number of times equal to half your charisma modifier per long rest."},
}

// loadCookingToolPipeImplementCatalog reads every Pipe implement off the
// equipment table (slug prefix weapon/cooking-pipe-, added by
// 0064_cooking_tool_infusion_pipe_implements.sql) — mirrors
// loadCookingToolImplementCatalog exactly, filtered by the Pipe's own
// disjoint slug prefix (see that migration's own doc comment for why the
// two prefixes must never nest one inside the other: both
// loadCookingToolImplementCatalog and syncCookingToolInfusionInventory
// match by LIKE 'weapon/cooking-tool-%', which would wrongly catch a Pipe
// slug nested under that same prefix).
func (s *server) loadCookingToolPipeImplementCatalog() ([]featureChoiceOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(description, '')
		FROM equipment WHERE slug LIKE 'weapon/cooking-pipe-%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []featureChoiceOption
	for rows.Next() {
		var o featureChoiceOption
		if err := rows.Scan(&o.Value, &o.Label, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// cookingToolInfusionPipeView is the Bonus Tool Infusion: Pipe box's own
// sub-panel — same 5-independent-picks shape as cookingToolInfusionView
// (implement, damage type, and one property each at 3rd/6th/11th level
// instead of 1st/6th/11th), each locked once chosen. Nil unless the
// character currently holds bonusToolInfusionPipeFeatureSlug.
type cookingToolInfusionPipeView struct {
	Implement        string // equipment slug, "" unpicked
	ImplementName    string
	ImplementOptions []featureChoiceOption

	DamageType        string
	DamageTypeOptions []featureChoiceOption

	PropertyL3        string
	PropertyL3Label   string
	PropertyL3Options []featureChoiceOption

	PropertyL6Available bool // true once the character's own Cooking-Nin class level >= 6
	PropertyL6          string
	PropertyL6Label     string
	PropertyL6Options   []featureChoiceOption

	PropertyL11Available bool // true once the character's own Cooking-Nin class level >= 11
	PropertyL11          string
	PropertyL11Label     string
	PropertyL11Options   []featureChoiceOption
}

// loadCookingToolInfusionPipeView loads every current pick plus the option
// lists the sheet's picker forms need — classLevel is the character's own
// real Cooking-Nin class level, used only to gate the 6th/11th property
// tiers (the caller already confirmed the feature itself is granted before
// reaching here, via cookingToolInfusionPipeEffectiveLevel).
func (s *server) loadCookingToolInfusionPipeView(characterID int64, classLevel int) (*cookingToolInfusionPipeView, error) {
	catalog, err := s.loadCookingToolPipeImplementCatalog()
	if err != nil {
		return nil, err
	}
	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	get := func(idx int) string {
		return choices[features.ChoiceKey{FeatureSlug: bonusToolInfusionPipeFeatureSlug, ChoiceIndex: idx}]
	}

	v := &cookingToolInfusionPipeView{
		Implement:         get(cookingToolPipeChoiceImplement),
		ImplementOptions:  catalog,
		DamageType:        get(cookingToolPipeChoiceDamageType),
		DamageTypeOptions: cookingToolDamageTypeOptions,
		PropertyL3:        get(cookingToolPipeChoicePropertyL3),
		PropertyL3Options: cookingToolPipePropertyL3Options,
		PropertyL6:        get(cookingToolPipeChoicePropertyL6),
		PropertyL11:       get(cookingToolPipeChoicePropertyL11),
	}
	v.PropertyL3Label = cookingToolOptionLabel(cookingToolPipePropertyL3Options, v.PropertyL3)
	for _, o := range catalog {
		if o.Value == v.Implement {
			v.ImplementName = o.Label
			break
		}
	}
	if classLevel >= 6 {
		v.PropertyL6Available = true
		v.PropertyL6Options = cookingToolPipePropertyL6Options
		v.PropertyL6Label = cookingToolOptionLabel(cookingToolPipePropertyL6Options, v.PropertyL6)
	}
	if classLevel >= 11 {
		v.PropertyL11Available = true
		v.PropertyL11Options = cookingToolPipePropertyL11Options
		v.PropertyL11Label = cookingToolOptionLabel(cookingToolPipePropertyL11Options, v.PropertyL11)
	}
	return v, nil
}

// syncCookingToolInfusionPipeInventory mirrors
// syncCookingToolInfusionInventory exactly, keyed off the Pipe's own
// disjoint slug prefix (weapon/cooking-pipe-, not weapon/cooking-tool-) so
// this feature's own inventory swap never touches — or is touched by — the
// base Cooking Tool Infusion's own implement pick: the two are genuinely
// two separate equipped weapons, per the feature's own "a second Cooking
// Tool Infusion weapon" text.
func (s *server) syncCookingToolInfusionPipeInventory(characterID int64, newSlug string) error {
	rows, err := s.charDB.Query(
		`SELECT id FROM character_inventory WHERE character_id = ? AND item_slug LIKE 'weapon/cooking-pipe-%'`,
		characterID)
	if err != nil {
		return err
	}
	var staleIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		staleIDs = append(staleIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range staleIDs {
		if err := charstore.DeleteInventoryItem(s.charDB, characterID, id); err != nil {
			return err
		}
	}
	if newSlug == "" {
		return nil
	}
	return charstore.AddInventoryItemWithEquipped(s.charDB, characterID, newSlug, 1, true)
}

// cookingToolInfusionPipeAttackOverrides resolves the flat, always-on
// bonuses buildAttacks applies to whichever equipped item matches the
// character's own picked Pipe implement — mirrors
// cookingToolInfusionAttackOverrides (Intelligence in place of Strength,
// the level-scaling Cooking Die as damage dice, the player's chosen damage
// type), minus a crit-range-bonus return value: Pipe's own 3 property
// tiers include nothing resembling Critical/Critical 2
// (cookingToolPipePropertyL6Options/L11Options), so there is no numeric
// crit-range effect to surface here. implementSlug == "" means "no pick
// yet, or the character doesn't hold this feature at all" — buildAttacks
// treats that as "never matches any equipped item", same "empty means
// untouched" shape cookingToolInfusionAttackOverrides already uses.
func (s *server) cookingToolInfusionPipeAttackOverrides(characterID int64, sheet *charsheet.Sheet) (implementSlug, dieSize, damageType string, err error) {
	level, err := s.cookingToolInfusionPipeEffectiveLevel(characterID, sheet)
	if err != nil {
		return "", "", "", err
	}
	if level == 0 {
		return "", "", "", nil
	}
	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return "", "", "", err
	}
	get := func(idx int) string {
		return choices[features.ChoiceKey{FeatureSlug: bonusToolInfusionPipeFeatureSlug, ChoiceIndex: idx}]
	}
	implementSlug = get(cookingToolPipeChoiceImplement)
	if implementSlug == "" {
		return "", "", "", nil
	}
	dieSize, err = s.cookingDieSize(level)
	if err != nil {
		return "", "", "", err
	}
	damageType = get(cookingToolPipeChoiceDamageType)
	return implementSlug, dieSize, damageType, nil
}

// expertCombatantFeatureSlug is Battle Cook's 2nd-level feature — "choose
// one martial or simple weapon, your Cooking Tools are considered to be
// this weapon for the purpose of Bukijutsu, including fulfilling keyword
// requirements." Confirmed against jutsu_eligibility.go: no mechanism
// anywhere in this app gates Bukijutsu jutsu eligibility by a specific
// equipped weapon GROUP (Polearm, Flail, etc. — the only gates
// jutsuEligible applies are elemental affinity and the Medical rank cap),
// even though several real jutsu name such a requirement in free text. So
// this pick's own downstream effect (bypassing a weapon-group keyword
// requirement) stays narrated, the same documented boundary this file's
// own package doc comment already draws around Cooking Tool Infusion's
// non-Blocking/Deadly/Critical properties — only WHICH weapon was chosen
// is tracked and shown here.
const expertCombatantFeatureSlug = "class/cooking-nin/group/cooking-focus/battle-cook/feature/expert-combatant"

// loadExpertCombatantWeaponCatalog reads every simple and martial weapon
// off the equipment table — "choose one martial or simple weapon" is a real
// filter over real equipment rows, same "read the catalog, don't
// hand-transcribe it" shape loadWardenWeaponCatalog (hunter_nin.go) and
// loadCookingToolImplementCatalog above already establish. Exotic weapons
// are excluded — the feature's own text names only martial and simple.
func (s *server) loadExpertCombatantWeaponCatalog() ([]featureChoiceOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(damage_dice, ''), COALESCE(damage_type, ''), COALESCE(properties, '')
		FROM equipment
		WHERE kind = 'weapon' AND weapon_category IN ('simple', 'martial')
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []featureChoiceOption
	for rows.Next() {
		var o featureChoiceOption
		var damageDice, damageType, properties string
		if err := rows.Scan(&o.Value, &o.Label, &damageDice, &damageType, &properties); err != nil {
			return nil, err
		}
		o.Description = strings.TrimSpace(damageDice + " " + damageType + ". " + properties)
		out = append(out, o)
	}
	return out, rows.Err()
}

// expertCombatantView holds Battle Cook's own Expert Combatant weapon pick
// — nil unless the character holds the feature. Locked once chosen, same
// "silence on re-editability read as permanent" boundary Cooking Tool
// Infusion's own 5 picks already draw (see the doc comment above
// cookingToolChoiceImplement's own const block) — the feature's own text
// describes a single choice made at 2nd level, with no mention of
// re-selecting it later.
type expertCombatantView struct {
	Weapon        string // equipment slug, "" unpicked
	WeaponName    string
	WeaponOptions []featureChoiceOption
}

// loadExpertCombatantView loads the current pick plus the catalog the
// sheet's picker form needs.
func (s *server) loadExpertCombatantView(characterID int64) (*expertCombatantView, error) {
	catalog, err := s.loadExpertCombatantWeaponCatalog()
	if err != nil {
		return nil, err
	}
	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	v := &expertCombatantView{
		Weapon:        choices[features.ChoiceKey{FeatureSlug: expertCombatantFeatureSlug, ChoiceIndex: 0}],
		WeaponOptions: catalog,
	}
	for _, o := range catalog {
		if o.Value == v.Weapon {
			v.WeaponName = o.Label
			break
		}
	}
	return v, nil
}

// knownBlendEnhancementPick is one entry on the Nature's Blend Enhancement
// Known list — a known jutsu (see knownJutsuOption, ninjutsu_specialist.go)
// paired with which of the 4 Enhancement types it was given.
type knownBlendEnhancementPick struct {
	JutsuID          int64
	Name             string
	Rank             string
	EnhancementType  string
	EnhancementLabel string
}

// cookingNinClassLevel returns the character's own Cooking-Nin class
// level, or 0 if they have none — mirrors hunterNinClassLevel.
func (s *server) cookingNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, cookingNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// cookingDieSize reads the Cooking Die's own live chart
// (v_class_level_resources, resource_name "Cooking Die") rather than
// hand-transcribing it — same "read the chart, don't retype it" precedent
// lethalAttackDice/flurryDieSize already established. The Cooking Die is
// the Cooking Tool weapon's own damage die and the base die nearly every
// Snack ingredient and Aura effect scales off of.
func (s *server) cookingDieSize(level int) (string, error) {
	var value sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = 'Cooking Die'`,
		cookingNinSlug, level,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// loadKnownBlendJutsu reads every jutsu the character currently knows whose
// keywords name at least one of the given elements (the character's own
// Nature's Blend release element(s), from the "natures-blend"/
// "many-colored-blend" affinity picks — elemental_affinity.go), for the
// Gastrochemist's own Enhancement picker to filter and offer. Same "resolve
// a published jutsu_slug or a player-created custom_jutsu, tolerate a stale
// slug" shape as loadKnownNinjutsu (ninjutsu_specialist.go), just filtered
// by element via jutsuElements instead of by classification == "Ninjutsu".
func (s *server) loadKnownBlendJutsu(characterID int64, blendElements map[string]bool) ([]knownJutsuOption, error) {
	if len(blendElements) == 0 {
		return nil, nil
	}
	rows, err := s.charDB.Query(`
		SELECT id, jutsu_slug, custom_jutsu_id FROM character_jutsu WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id        int64
		jutsuSlug sql.NullString
		customID  sql.NullInt64
	}
	var stored []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.jutsuSlug, &r.customID); err != nil {
			rows.Close()
			return nil, err
		}
		stored = append(stored, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []knownJutsuOption
	for _, r := range stored {
		var name, rank, keywords, description string
		if r.jutsuSlug.Valid {
			err := s.rulesDB.QueryRow(
				`SELECT name, rank, keywords, description FROM v_jutsu WHERE slug = ?`, r.jutsuSlug.String,
			).Scan(&name, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue // stale slug (rules update) — skip rather than break the picker
			}
			if err != nil {
				return nil, err
			}
		} else if r.customID.Valid {
			err := s.charDB.QueryRow(
				`SELECT name, rank, keywords, description FROM custom_jutsu WHERE id = ?`, r.customID.Int64,
			).Scan(&name, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else {
			continue
		}
		matched := false
		for _, el := range jutsuElements(keywords) {
			if blendElements[el] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		out = append(out, knownJutsuOption{JutsuID: r.id, Name: name, Rank: rank, Description: description})
	}
	return out, nil
}

// foodForTheSoulView holds Food For the Soul's current pick — nil unless
// the character has reached 2nd level in Cooking-Nin.
type foodForTheSoulView struct {
	Current string // ability key ("str".."cha"), "" if unpicked
	Options []featureChoiceOption
}

// fastAndFuriousView holds Fast and Furious's current pick — nil unless the
// character holds Entremetier Chef's own 2nd-level feature.
type fastAndFuriousView struct {
	Current string // "int" or "cha", "" if unpicked
	Options []featureChoiceOption
}

// cookingNinTabData is the sheet_cooking_nin box's full data — nil for a
// character with no Cooking-Nin levels, same "no empty state" treatment
// HunterTechniques/MartialTechniques already establish.
type cookingNinTabData struct {
	CookingDieSize string
	FoodForTheSoul *foodForTheSoulView
	FastAndFurious *fastAndFuriousView

	// CookingToolInfusion is the base class's own 1st-level weapon
	// pick (implement, damage type, and 3 level-gated properties) — nil
	// until the character (or an archetype-training feat) reaches the
	// feature's own 1st-level gate.
	CookingToolInfusion *cookingToolInfusionView

	// BonusToolInfusionPipe is Herbalist's own 3rd-level second weapon pick
	// (implement, damage type, and 3 level-gated properties, same shape as
	// CookingToolInfusion but its own wholly separate equipped item) — nil
	// until the character holds bonusToolInfusionPipeFeatureSlug.
	BonusToolInfusionPipe *cookingToolInfusionPipeView

	// ExpertCombatant is Battle Cook's own 2nd-level weapon-classification
	// pick — nil until the character holds the feature.
	ExpertCombatant *expertCombatantView

	// Gastrochemist's Nature's Blend Enhancement pick (2nd level, cap
	// 1->6 across 2nd/3rd/5th/9th/13th/17th level) — only rendered when
	// BlendEnhancementCap > 0, i.e. the character actually holds the
	// Gastrochemist subclass.
	BlendEnhancementCap     int
	BlendEnhancementUsed    int
	KnownBlendEnhancements  []knownBlendEnhancementPick
	AvailableBlendJutsu     []knownJutsuOption
	BlendEnhancementOptions []featureChoiceOption
}

// loadCookingNinTabData returns nil for a character with no Cooking-Nin
// levels and none of the 3 archetype-training feats — the template gates
// the whole box's existence on this being non-nil, same treatment
// HunterTechniques gets for its own archetype feats.
func (s *server) loadCookingNinTabData(characterID int64, sheet *charsheet.Sheet) (*cookingNinTabData, error) {
	level, err := s.cookingNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	arch := loadCookingArchetypeFeats(grantedFeatures)
	if level == 0 && !arch.any() {
		return nil, nil
	}
	var dieSize string
	if level > 0 {
		dieSize, err = s.cookingDieSize(level)
		if err != nil {
			return nil, err
		}
	} else {
		// Real levels and the archetype feats are mutually exclusive by the
		// feats' own prerequisites ("You cannot have Levels in the
		// Cooking-Nin Class"), so this only ever fills in an empty
		// real-level readout, never overrides a real chart value.
		dieSize = arch.dieSize()
	}
	data := &cookingNinTabData{CookingDieSize: dieSize}

	if toolLevel := cookingToolInfusionLevel(level, arch); toolLevel > 0 {
		data.CookingToolInfusion, err = s.loadCookingToolInfusionView(characterID, toolLevel)
		if err != nil {
			return nil, err
		}
	}

	if hasGrantedFeature(grantedFeatures, bonusToolInfusionPipeFeatureSlug) {
		data.BonusToolInfusionPipe, err = s.loadCookingToolInfusionPipeView(characterID, level)
		if err != nil {
			return nil, err
		}
	}

	if level >= 2 {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		data.FoodForTheSoul = &foodForTheSoulView{
			Current: choices[features.ChoiceKey{FeatureSlug: foodForTheSoulFeatureSlug, ChoiceIndex: 0}],
			Options: foodForTheSoulOptions,
		}
	}

	if hasGrantedFeature(grantedFeatures, expertCombatantFeatureSlug) {
		data.ExpertCombatant, err = s.loadExpertCombatantView(characterID)
		if err != nil {
			return nil, err
		}
	}

	if hasGrantedFeature(grantedFeatures, fastAndFuriousFeatureSlug) {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		data.FastAndFurious = &fastAndFuriousView{
			Current: choices[features.ChoiceKey{FeatureSlug: fastAndFuriousFeatureSlug, ChoiceIndex: 0}],
			Options: fastAndFuriousOptions,
		}
	}

	if hasGrantedFeature(grantedFeatures, gastrochemistNaturesBlendFeatureSlug) {
		data.BlendEnhancementCap = gastrochemistEnhancementCap(level)
		if data.BlendEnhancementCap > 0 {
			affinityPicks, err := charstore.ListElementalAffinities(s.charDB, characterID)
			if err != nil {
				return nil, err
			}
			blendElements := map[string]bool{}
			if el := affinityPicks["natures-blend"]; el != "" {
				blendElements[el] = true
			}
			if el := affinityPicks["many-colored-blend"]; el != "" {
				blendElements[el] = true
			}
			known, err := s.loadKnownBlendJutsu(characterID, blendElements)
			if err != nil {
				return nil, err
			}
			enhancementPicks, err := charstore.ListCookingNinBlendEnhancementPicks(s.charDB, characterID)
			if err != nil {
				return nil, err
			}
			pickedByJutsu := make(map[int64]string, len(enhancementPicks))
			for _, p := range enhancementPicks {
				pickedByJutsu[p.JutsuID] = p.EnhancementType
			}
			data.BlendEnhancementUsed = len(enhancementPicks)
			data.BlendEnhancementOptions = gastrochemistEnhancementOptions
			for _, o := range known {
				if enhType, ok := pickedByJutsu[o.JutsuID]; ok {
					data.KnownBlendEnhancements = append(data.KnownBlendEnhancements, knownBlendEnhancementPick{
						JutsuID:          o.JutsuID,
						Name:             o.Name,
						Rank:             o.Rank,
						EnhancementType:  enhType,
						EnhancementLabel: gastrochemistEnhancementLabel(enhType),
					})
				} else {
					data.AvailableBlendJutsu = append(data.AvailableBlendJutsu, o)
				}
			}
		}
	}

	return data, nil
}

// handleFoodForTheSoul records Food For the Soul's re-selectable
// ability-score pick.
func (s *server) handleFoodForTheSoul(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	valid := false
	for _, o := range foodForTheSoulOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	level, err := s.cookingNinClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin level for food for the soul:", err)
		return
	}
	if level < 2 {
		http.Error(w, "character has not reached 2nd level in Cooking-Nin", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, foodForTheSoulFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set food for the soul:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// setFastAndFurious validates and stores Fast and Furious's re-selectable
// Initiative ability pick (Intelligence or Charisma in place of Dexterity)
// — only the pick is tracked; internal/charsheet.Compute reads it back as
// InitiativeAbility's own default (still overridable by the sheet's
// existing Adjust-Initiative dropdown, same override-beats-default shape
// every other Initiative source on this sheet already follows). Extracted
// from handleFastAndFurious so the Fast and Furious popup (cooking_nin_
// fast_and_furious_popup.go) shares the identical validation path as the
// Core-sheet's own AJAX route instead of reimplementing it — same
// "extract, don't duplicate" shape addAwakenedScrollPick (ninjutsu_
// specialist.go) establishes.
func (s *server) setFastAndFurious(id int64, value string) (int, string) {
	valid := false
	for _, o := range fastAndFuriousOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid pick"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for fast and furious:", err)
		return http.StatusInternalServerError, "database error"
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		log.Println("load granted features for fast and furious:", err)
		return http.StatusInternalServerError, "database error"
	}
	if !hasGrantedFeature(grantedFeatures, fastAndFuriousFeatureSlug) {
		return http.StatusBadRequest, "character does not have Fast and Furious"
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, fastAndFuriousFeatureSlug, 0, value); err != nil {
		log.Println("set fast and furious:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleFastAndFurious is setFastAndFurious's own Core-sheet AJAX wrapper.
func (s *server) handleFastAndFurious(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setFastAndFurious(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// cookingToolInfusionEffectiveLevel resolves the level Cooking Tool
// Infusion's own picks gate against for one character — real Cooking-Nin
// level, or the matching archetype-training feat's own "as though" tier
// (cookingToolInfusionLevel). 0 means the character holds neither.
func (s *server) cookingToolInfusionEffectiveLevel(characterID int64, sheet *charsheet.Sheet) (int, error) {
	level, err := s.cookingNinClassLevel(characterID)
	if err != nil {
		return 0, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return 0, err
	}
	return cookingToolInfusionLevel(level, loadCookingArchetypeFeats(grantedFeatures)), nil
}

// cookingToolInfusionPipeEffectiveLevel mirrors
// cookingToolInfusionEffectiveLevel's own "resolve once, reuse in every
// handler" shape, but for Bonus Tool Infusion: Pipe (Herbalist, 3rd level)
// instead of the base class feature. Unlike the base feature, Pipe has no
// archetype-feat "as though" fallback to thread through — Chef Trainee/
// Chefs Expert/Chefs Specialist's own text only grants the BASE Cooking
// Tool Infusion feature "as though" a given level, never this subclass
// feature — so the gate here is simply "does the character currently hold
// bonusToolInfusionPipeFeatureSlug" (hasGrantedFeature, itself already
// gated by subclass_features' own level column and the character's own
// Herbalist subclass pick via loadMergedGrantedFeatures). 0 means
// ungranted; a non-zero return is the character's real Cooking-Nin class
// level, used by callers to gate the 6th/11th property tiers the same way
// the base feature's own toolLevel does.
func (s *server) cookingToolInfusionPipeEffectiveLevel(characterID int64, sheet *charsheet.Sheet) (int, error) {
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return 0, err
	}
	if !hasGrantedFeature(grantedFeatures, bonusToolInfusionPipeFeatureSlug) {
		return 0, nil
	}
	level, err := s.cookingNinClassLevel(characterID)
	if err != nil {
		return 0, err
	}
	return level, nil
}

// handleCookingToolImplement records which catalog implement is infused as
// the character's Cooking Tool. Locked once chosen — see the doc comment
// above cookingToolChoiceImplement's own const block for why the feature's
// silence on re-editability is read as permanent here, the same as Titan
// Specialization/Armor Chassis. Setting it re-syncs the character's own
// inventory (syncCookingToolInfusionInventory) so the pick is a real,
// equipped item, not just an abstract tracked choice.
func (s *server) handleCookingToolImplement(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for cooking tool implement:", err)
		return
	}
	toolLevel, err := s.cookingToolInfusionEffectiveLevel(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking tool infusion level:", err)
		return
	}
	if toolLevel == 0 {
		http.Error(w, "character does not have Cooking Tool Infusion", http.StatusBadRequest)
		return
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feature choices for cooking tool implement:", err)
		return
	}
	if choices[cookingToolChoiceKey(cookingToolChoiceImplement)] != "" {
		http.Error(w, "Cooking Tool implement is already chosen and cannot be changed", http.StatusBadRequest)
		return
	}
	if value != "" {
		catalog, err := s.loadCookingToolImplementCatalog()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load cooking tool implement catalog:", err)
			return
		}
		valid := false
		for _, o := range catalog {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid Cooking Tool implement", http.StatusBadRequest)
			return
		}
	}
	if value == "" {
		if err := charstore.ClearFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoiceImplement); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("clear cooking tool implement:", err)
			return
		}
	} else if err := charstore.SetFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoiceImplement, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set cooking tool implement:", err)
		return
	}
	if err := s.syncCookingToolInfusionInventory(id, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("sync cooking tool infusion inventory:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingToolDamageType records the 1st-level damage-type pick
// (Bludgeoning/Piercing/Slashing) — locked once chosen (see the doc comment
// above cookingToolChoiceImplement's own const block), no inventory sync
// needed (the damage TYPE isn't a separate item, just an attribute
// buildAttacks reads back via cookingToolInfusionAttackOverrides).
func (s *server) handleCookingToolDamageType(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for cooking tool damage type:", err)
		return
	}
	toolLevel, err := s.cookingToolInfusionEffectiveLevel(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking tool infusion level:", err)
		return
	}
	if toolLevel == 0 {
		http.Error(w, "character does not have Cooking Tool Infusion", http.StatusBadRequest)
		return
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feature choices for cooking tool damage type:", err)
		return
	}
	if choices[cookingToolChoiceKey(cookingToolChoiceDamageType)] != "" {
		http.Error(w, "Cooking Tool damage type is already chosen and cannot be changed", http.StatusBadRequest)
		return
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolDamageTypeOptions {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid damage type", http.StatusBadRequest)
			return
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoiceDamageType)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoiceDamageType, value)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set cooking tool damage type:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingToolPropertyL1 records the 1st-level weapon-property pick
// (Blocking/Hidden/Thrown/Unarmed/Deadly) — locked once chosen (see the doc
// comment above cookingToolChoiceImplement's own const block).
func (s *server) handleCookingToolPropertyL1(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for cooking tool property (1st):", err)
		return
	}
	toolLevel, err := s.cookingToolInfusionEffectiveLevel(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking tool infusion level:", err)
		return
	}
	if toolLevel == 0 {
		http.Error(w, "character does not have Cooking Tool Infusion", http.StatusBadRequest)
		return
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feature choices for cooking tool property (1st):", err)
		return
	}
	if choices[cookingToolChoiceKey(cookingToolChoicePropertyL1)] != "" {
		http.Error(w, "Cooking Tool 1st-level property is already chosen and cannot be changed", http.StatusBadRequest)
		return
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolPropertyL1Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid 1st-level property", http.StatusBadRequest)
			return
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL1)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL1, value)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set cooking tool property (1st):", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingToolPropertyL6 records the 6th-level weapon-property pick
// (Light/Lethal 2/Returning/Critical/Trip,Disarm,Grapple) — gated on the
// character having reached 6th level's own effective tier, Returning
// specifically gated on the 1st-level pick already being Thrown ("this can
// only be taken if your Cooking Tool has the Thrown Property"), and locked
// once chosen (see the doc comment above cookingToolChoiceImplement's own
// const block).
func (s *server) handleCookingToolPropertyL6(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for cooking tool property (6th):", err)
		return
	}
	toolLevel, err := s.cookingToolInfusionEffectiveLevel(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking tool infusion level:", err)
		return
	}
	if toolLevel < 6 {
		http.Error(w, "character has not reached 6th level in Cooking Tool Infusion", http.StatusBadRequest)
		return
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feature choices for cooking tool property (6th):", err)
		return
	}
	if choices[cookingToolChoiceKey(cookingToolChoicePropertyL6)] != "" {
		http.Error(w, "Cooking Tool 6th-level property is already chosen and cannot be changed", http.StatusBadRequest)
		return
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolPropertyL6Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid 6th-level property", http.StatusBadRequest)
			return
		}
		if value == "returning" && choices[cookingToolChoiceKey(cookingToolChoicePropertyL1)] != "thrown" {
			http.Error(w, "Returning requires the Thrown property", http.StatusBadRequest)
			return
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL6)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL6, value)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set cooking tool property (6th):", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingToolPropertyL11 records the 11th-level weapon-property pick
// (Reach 3/Multiattack/Tactical/Critical 2) — gated on the character
// having reached 11th level's own effective tier, Critical 2 specifically
// gated on the 6th-level pick already being Critical ("this can only be
// taken if your Cooking Tool has the Critical Property"), and locked once
// chosen (see the doc comment above cookingToolChoiceImplement's own const
// block).
func (s *server) handleCookingToolPropertyL11(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for cooking tool property (11th):", err)
		return
	}
	toolLevel, err := s.cookingToolInfusionEffectiveLevel(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking tool infusion level:", err)
		return
	}
	if toolLevel < 11 {
		http.Error(w, "character has not reached 11th level in Cooking Tool Infusion", http.StatusBadRequest)
		return
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feature choices for cooking tool property (11th):", err)
		return
	}
	if choices[cookingToolChoiceKey(cookingToolChoicePropertyL11)] != "" {
		http.Error(w, "Cooking Tool 11th-level property is already chosen and cannot be changed", http.StatusBadRequest)
		return
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolPropertyL11Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid 11th-level property", http.StatusBadRequest)
			return
		}
		if value == "critical-2" && choices[cookingToolChoiceKey(cookingToolChoicePropertyL6)] != "critical" {
			http.Error(w, "Critical 2 requires the Critical property", http.StatusBadRequest)
			return
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL11)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, cookingToolInfusionFeatureSlug, cookingToolChoicePropertyL11, value)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set cooking tool property (11th):", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingToolPipeImplement records which catalog implement is
// infused as the character's Pipe — mirrors handleCookingToolImplement
// exactly, gated on cookingToolInfusionPipeEffectiveLevel instead of
// cookingToolInfusionEffectiveLevel and syncing the Pipe's own disjoint
// inventory slug prefix (syncCookingToolInfusionPipeInventory) instead of
// the base tool's. Extracted from handleCookingToolPipeImplement so the
// Pipe popup (cooking_nin_pipe_popup.go) shares the identical validation
// path as the Core-sheet's own AJAX route instead of reimplementing it —
// same "extract, don't duplicate" shape addAwakenedScrollPick (ninjutsu_
// specialist.go) establishes. Equipping/unequipping the implement here
// still re-syncs the character's real inventory (syncCookingToolInfusion
// PipeInventory) exactly as the Core-sheet route does — the popup's own
// window has no live DOM link back to the Core sheet's Inventory/Attacks
// boxes to refresh in place (see subclass_tracker_popup.go's own header
// doc on this pattern's plain POST-and-redirect convention), so a stale
// Inventory box on the Core sheet's own window only catches up once that
// window is reloaded, the same accepted gap Ninjaneer's own Enhanced
// Weapon/Legendary Weapon popup picks already draw.
func (s *server) setCookingToolPipeImplement(id int64, value string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for cooking tool pipe implement:", err)
		return http.StatusInternalServerError, "database error"
	}
	toolLevel, err := s.cookingToolInfusionPipeEffectiveLevel(id, sheet)
	if err != nil {
		log.Println("load cooking tool infusion pipe level:", err)
		return http.StatusInternalServerError, "database error"
	}
	if toolLevel == 0 {
		return http.StatusBadRequest, "character does not have Bonus Tool Infusion: Pipe"
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		log.Println("load feature choices for cooking tool pipe implement:", err)
		return http.StatusInternalServerError, "database error"
	}
	if choices[cookingToolPipeChoiceKey(cookingToolPipeChoiceImplement)] != "" {
		return http.StatusBadRequest, "Pipe implement is already chosen and cannot be changed"
	}
	if value != "" {
		catalog, err := s.loadCookingToolPipeImplementCatalog()
		if err != nil {
			log.Println("load cooking tool pipe implement catalog:", err)
			return http.StatusInternalServerError, "database error"
		}
		valid := false
		for _, o := range catalog {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return http.StatusBadRequest, "not a valid Pipe implement"
		}
	}
	if value == "" {
		if err := charstore.ClearFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoiceImplement); err != nil {
			log.Println("clear cooking tool pipe implement:", err)
			return http.StatusInternalServerError, "database error"
		}
	} else if err := charstore.SetFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoiceImplement, value); err != nil {
		log.Println("set cooking tool pipe implement:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := s.syncCookingToolInfusionPipeInventory(id, value); err != nil {
		log.Println("sync cooking tool infusion pipe inventory:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleCookingToolPipeImplement is setCookingToolPipeImplement's own
// Core-sheet AJAX wrapper.
func (s *server) handleCookingToolPipeImplement(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setCookingToolPipeImplement(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// setCookingToolPipeDamageType validates and stores the 3rd-level Pipe
// damage-type pick (Bludgeoning/Piercing/Slashing) — locked once chosen,
// no inventory sync needed (the damage TYPE isn't a separate item, just an
// attribute buildAttacks reads back via
// cookingToolInfusionPipeAttackOverrides). Extracted from
// handleCookingToolPipeDamageType so the Pipe popup (cooking_nin_pipe_
// popup.go) shares the identical validation path as the Core-sheet's own
// AJAX route instead of reimplementing it.
func (s *server) setCookingToolPipeDamageType(id int64, value string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for cooking tool pipe damage type:", err)
		return http.StatusInternalServerError, "database error"
	}
	toolLevel, err := s.cookingToolInfusionPipeEffectiveLevel(id, sheet)
	if err != nil {
		log.Println("load cooking tool infusion pipe level:", err)
		return http.StatusInternalServerError, "database error"
	}
	if toolLevel == 0 {
		return http.StatusBadRequest, "character does not have Bonus Tool Infusion: Pipe"
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		log.Println("load feature choices for cooking tool pipe damage type:", err)
		return http.StatusInternalServerError, "database error"
	}
	if choices[cookingToolPipeChoiceKey(cookingToolPipeChoiceDamageType)] != "" {
		return http.StatusBadRequest, "Pipe damage type is already chosen and cannot be changed"
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolDamageTypeOptions {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return http.StatusBadRequest, "not a valid damage type"
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoiceDamageType)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoiceDamageType, value)
	}
	if err != nil {
		log.Println("set cooking tool pipe damage type:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleCookingToolPipeDamageType is setCookingToolPipeDamageType's own
// Core-sheet AJAX wrapper.
func (s *server) handleCookingToolPipeDamageType(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setCookingToolPipeDamageType(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// setCookingToolPipePropertyL3 validates and stores the Pipe's own
// 3rd-level weapon-property pick (Deep Breath/Didn't Know You Was Chill
// Like That/Herb In the Pipe) — locked once chosen. Extracted from
// handleCookingToolPipePropertyL3 so the Pipe popup (cooking_nin_pipe_
// popup.go) shares the identical validation path as the Core-sheet's own
// AJAX route instead of reimplementing it.
func (s *server) setCookingToolPipePropertyL3(id int64, value string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for cooking tool pipe property (3rd):", err)
		return http.StatusInternalServerError, "database error"
	}
	toolLevel, err := s.cookingToolInfusionPipeEffectiveLevel(id, sheet)
	if err != nil {
		log.Println("load cooking tool infusion pipe level:", err)
		return http.StatusInternalServerError, "database error"
	}
	if toolLevel == 0 {
		return http.StatusBadRequest, "character does not have Bonus Tool Infusion: Pipe"
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		log.Println("load feature choices for cooking tool pipe property (3rd):", err)
		return http.StatusInternalServerError, "database error"
	}
	if choices[cookingToolPipeChoiceKey(cookingToolPipeChoicePropertyL3)] != "" {
		return http.StatusBadRequest, "Pipe 3rd-level property is already chosen and cannot be changed"
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolPipePropertyL3Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return http.StatusBadRequest, "not a valid 3rd-level property"
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoicePropertyL3)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoicePropertyL3, value)
	}
	if err != nil {
		log.Println("set cooking tool pipe property (3rd):", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleCookingToolPipePropertyL3 is setCookingToolPipePropertyL3's own
// Core-sheet AJAX wrapper.
func (s *server) handleCookingToolPipePropertyL3(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setCookingToolPipePropertyL3(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// setCookingToolPipePropertyL6 validates and stores the Pipe's own
// 6th-level weapon-property pick (Inhaled Herb/Deeper Breath/Harsh Inhale)
// — gated on the character having reached 6th level, and locked once
// chosen. Extracted from handleCookingToolPipePropertyL6 so the Pipe popup
// (cooking_nin_pipe_popup.go) shares the identical validation path as the
// Core-sheet's own AJAX route instead of reimplementing it.
func (s *server) setCookingToolPipePropertyL6(id int64, value string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for cooking tool pipe property (6th):", err)
		return http.StatusInternalServerError, "database error"
	}
	toolLevel, err := s.cookingToolInfusionPipeEffectiveLevel(id, sheet)
	if err != nil {
		log.Println("load cooking tool infusion pipe level:", err)
		return http.StatusInternalServerError, "database error"
	}
	if toolLevel < 6 {
		return http.StatusBadRequest, "character has not reached 6th level in Bonus Tool Infusion: Pipe"
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		log.Println("load feature choices for cooking tool pipe property (6th):", err)
		return http.StatusInternalServerError, "database error"
	}
	if choices[cookingToolPipeChoiceKey(cookingToolPipeChoicePropertyL6)] != "" {
		return http.StatusBadRequest, "Pipe 6th-level property is already chosen and cannot be changed"
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolPipePropertyL6Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return http.StatusBadRequest, "not a valid 6th-level property"
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoicePropertyL6)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoicePropertyL6, value)
	}
	if err != nil {
		log.Println("set cooking tool pipe property (6th):", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleCookingToolPipePropertyL6 is setCookingToolPipePropertyL6's own
// Core-sheet AJAX wrapper.
func (s *server) handleCookingToolPipePropertyL6(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setCookingToolPipePropertyL6(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// setCookingToolPipePropertyL11 validates and stores the Pipe's own
// 11th-level weapon-property pick (Constant Smoke/Quick Inhale) — gated on
// the character having reached 11th level, and locked once chosen.
// Extracted from handleCookingToolPipePropertyL11 so the Pipe popup
// (cooking_nin_pipe_popup.go) shares the identical validation path as the
// Core-sheet's own AJAX route instead of reimplementing it.
func (s *server) setCookingToolPipePropertyL11(id int64, value string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for cooking tool pipe property (11th):", err)
		return http.StatusInternalServerError, "database error"
	}
	toolLevel, err := s.cookingToolInfusionPipeEffectiveLevel(id, sheet)
	if err != nil {
		log.Println("load cooking tool infusion pipe level:", err)
		return http.StatusInternalServerError, "database error"
	}
	if toolLevel < 11 {
		return http.StatusBadRequest, "character has not reached 11th level in Bonus Tool Infusion: Pipe"
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		log.Println("load feature choices for cooking tool pipe property (11th):", err)
		return http.StatusInternalServerError, "database error"
	}
	if choices[cookingToolPipeChoiceKey(cookingToolPipeChoicePropertyL11)] != "" {
		return http.StatusBadRequest, "Pipe 11th-level property is already chosen and cannot be changed"
	}
	if value != "" {
		valid := false
		for _, o := range cookingToolPipePropertyL11Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return http.StatusBadRequest, "not a valid 11th-level property"
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoicePropertyL11)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, bonusToolInfusionPipeFeatureSlug, cookingToolPipeChoicePropertyL11, value)
	}
	if err != nil {
		log.Println("set cooking tool pipe property (11th):", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleCookingToolPipePropertyL11 is setCookingToolPipePropertyL11's own
// Core-sheet AJAX wrapper.
func (s *server) handleCookingToolPipePropertyL11(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setCookingToolPipePropertyL11(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// setBattleCookExpertCombatantWeapon validates and stores Battle Cook's own
// Expert Combatant weapon-type pick — distinct from Combat Medic's own
// unrelated "Expert Combatant" feature (Medical-Nin, medical_nin.go's
// handleExpertCombatantAdd/handleExpertCombatantDelete); the two classes'
// features simply share a printed name, nothing else. Locked once chosen
// (see the doc comment above expertCombatantFeatureSlug). Extracted from
// handleBattleCookExpertCombatantWeapon so the Expert Combatant popup
// (cooking_nin_expert_combatant_popup.go) shares the identical validation
// path as the Core-sheet's own AJAX route instead of reimplementing it —
// same "extract, don't duplicate" shape setFastAndFurious above establishes.
func (s *server) setBattleCookExpertCombatantWeapon(id int64, value string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for expert combatant weapon:", err)
		return http.StatusInternalServerError, "database error"
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		log.Println("load granted features for expert combatant weapon:", err)
		return http.StatusInternalServerError, "database error"
	}
	if !hasGrantedFeature(grantedFeatures, expertCombatantFeatureSlug) {
		return http.StatusBadRequest, "character does not have Expert Combatant"
	}
	choices, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		log.Println("load feature choices for expert combatant weapon:", err)
		return http.StatusInternalServerError, "database error"
	}
	if choices[features.ChoiceKey{FeatureSlug: expertCombatantFeatureSlug, ChoiceIndex: 0}] != "" {
		return http.StatusBadRequest, "Expert Combatant weapon is already chosen and cannot be changed"
	}
	if value != "" {
		catalog, err := s.loadExpertCombatantWeaponCatalog()
		if err != nil {
			log.Println("load expert combatant weapon catalog:", err)
			return http.StatusInternalServerError, "database error"
		}
		valid := false
		for _, o := range catalog {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			return http.StatusBadRequest, "not a valid weapon"
		}
	}
	if value == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, expertCombatantFeatureSlug, 0)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, expertCombatantFeatureSlug, 0, value)
	}
	if err != nil {
		log.Println("set expert combatant weapon:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleBattleCookExpertCombatantWeapon is
// setBattleCookExpertCombatantWeapon's own Core-sheet AJAX wrapper.
func (s *server) handleBattleCookExpertCombatantWeapon(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setBattleCookExpertCombatantWeapon(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// addCookingNinBlendEnhancementPick validates and stores one Nature's Blend
// Enhancement pick — a known jutsu of the character's own Nature's Blend
// release element(s) paired with one of the 4 Enhancement types. Only the
// PICK is tracked; the Enhancement's actual chakra-plus-Snack-spend-to-apply
// mechanic stays fully Group 2/3 — see gastrochemistEnhancementOptions' own
// comment. Extracted from handleCookingNinBlendEnhancementAdd so the Blend
// Enhancements popup (cooking_nin_blend_enhancements_popup.go) shares the
// identical validation path as the Core-sheet's own AJAX route instead of
// reimplementing it — same "extract, don't duplicate" shape
// addAwakenedScrollPick (ninjutsu_specialist.go) establishes.
func (s *server) addCookingNinBlendEnhancementPick(id int64, jutsuID int64, enhancementType string) (int, string) {
	valid := false
	for _, o := range gastrochemistEnhancementOptions {
		if o.Value == enhancementType {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid Enhancement type"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for blend enhancement add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		log.Println("load cooking-nin for blend enhancement add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.BlendEnhancementUsed >= data.BlendEnhancementCap {
		return http.StatusBadRequest, "no Enhancement slots remaining"
	}
	jutsuValid := false
	for _, o := range data.AvailableBlendJutsu {
		if o.JutsuID == jutsuID {
			jutsuValid = true
			break
		}
	}
	if !jutsuValid {
		return http.StatusBadRequest, "not a valid jutsu to enhance"
	}
	if err := charstore.AddCookingNinBlendEnhancementPick(s.charDB, id, jutsuID, enhancementType); err != nil {
		log.Println("add cooking-nin blend enhancement pick:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleCookingNinBlendEnhancementAdd is addCookingNinBlendEnhancementPick's
// own Core-sheet AJAX wrapper (form fields "jutsu_id", "enhancement_type").
func (s *server) handleCookingNinBlendEnhancementAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing jutsu", http.StatusBadRequest)
		return
	}
	enhancementType := strings.TrimSpace(r.FormValue("enhancement_type"))
	if status, msg := s.addCookingNinBlendEnhancementPick(id, jutsuID, enhancementType); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingNinBlendEnhancementDelete drops one Enhancement pick —
// freely, at any time, same "trust the player" boundary every other pick
// removal on this sheet draws.
func (s *server) handleCookingNinBlendEnhancementDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing jutsu", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveCookingNinBlendEnhancementPick(s.charDB, id, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove cooking-nin blend enhancement pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}
