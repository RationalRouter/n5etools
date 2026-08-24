package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file is the Titan stat block — Science-Nin's Mech Crafter subclass's
// Ordnance Training feature (class/science-nin/group/scientific-inquiry/
// mech-crafter/feature/ordnance-training, 3rd level). A Titan is stored as
// an ordinary kind="titan" charstore.Companion (companions.go), the same
// generic row every other companion kind uses; this file supplies the
// computed baseline stats, the Titan Specialization pick, the Titan
// Upgrades cap+catalog picker (Ordnance Training's own Titan Slots, cap =
// Proficiency Bonus), Endless Work's separate Exo-Suit slot, and Specialist
// Crafting's per-keyword cost discount — mirroring how puppets.go turns a
// kind="puppet" row into a full stat block and nindog.go turns a
// kind="nin-dog" row into one, but with Titan-specific rules throughout.
//
// Source: titan_unit_card (rules.db), class_slug='class/science-nin',
// detection_status='needs_review' — a SINGLE unstructured raw_text blob
// confirmed (hex dump) to contain zero newlines, extracted from a PDF table
// as one run-on string. Unlike puppet_tool_stat_block (Puppet Master's own
// columnized stat-block table, with dedicated ac_base/saving_throws_text/
// damage_resistance_text/... columns), titan_unit_card has no such columns
// — only raw_text — so the values below (titanBaseAbilityScores,
// titanBaseTraits, titanBashAttackText) are hand-transcribed verbatim from
// that blob rather than read live, the same "hand-curated, not live-
// queried" treatment scienceNinRegaliaOptions (science_nin_subclasses.go)
// and ninDogBreeds (nindog.go) already draw for source text that isn't
// stored as individually addressable rows.
//
// AC ("12 + Intelligence Modifier + half your Proficiency Bonus", Natural
// Armor) and Hit Dice are genuinely MISSING from titan_unit_card.raw_text —
// a real PDF-extraction gap between "unaligned" and "Hit Points" in the
// blob, not a true absence from the rulebook. Verified directly against the
// source PDF page image (Orochimaru's Observation Compendium,
// titan_unit_card.source_page 236) rather than re-guessed from the broken
// extraction — see titanAC. Hit Dice still has no computed field here (a
// Titan's Hit Points already come from titanMaxHP's own flat formula, with
// no separate Hit Dice roll anywhere else in this app's math), and the
// saving-throw-proficiency bonus beyond "makes Strength, Dexterity, and
// Constitution saving throws in your place, using its own statistics"
// remains unconfirmed (no stated proficiency add-on anywhere in the raw
// text) — those two stay out of scope.
//
// Titan Upgrades (class_options list_name='Titan Upgrades', 5 tiers: Minor/
// Refined/Superior/Supreme/Mastercraft) spend from the SAME Creation Points
// budget as the base Scientific Ninja Tools catalog (science_nin.go) — the
// class's own single character-wide pool, confirmed by matching the
// identical "Cost: N Creation Points" stat-line shape every entry across
// both catalogs uses. Each entry ALSO carries a "Keyword: Mech|Weapon"
// clause science_nin.go's own scienceNinToolStatLinePattern never needs to
// parse (no other Science-Nin catalog gates on a keyword), so this file
// parses its own stat line (titanUpgradeStatLinePattern) rather than
// reusing that one.
//
// The "Refined" tier bucket in class_option_entries actually merges TWO
// source-book tiers: true Refined (4 Creation Points: Leadwall, Thermite
// Launcher, Xo-16 Gatling, Battle Tower, Scroll Array) and true Greater (8
// Creation Points: Greater Missile Racks, Critical Ejection, Siege Engine,
// Sturdy Frame — "Greater Missile Racks" own name literally contains
// "Greater"). Confirmed against the base Scientific Ninja Tools catalog,
// which DOES have all 6 canonical tier names (Minor/Refined/Greater/
// Superior/Supreme/Mastercraft) — Titan Upgrades' own catalog is missing a
// "Greater" tier row entirely, its cost-8 items folded into "Refined"
// instead, apparently at ingest time. This matters for Endless Work's own
// 6th-level Exo-Suit ("equipped with a Greater or lower Mech keyword
// upgrade") — read here as Cost <= 8 (Minor's 2 CP plus both true-Refined's
// 4 CP and true-Greater's 8 CP), not by tier name, since the tier name
// alone can't distinguish true-Refined from true-Greater inside the merged
// bucket.
//
// Mastercraft's own single item (Bijuu Slayer, 32 Creation Points) was
// never split into a class_option_entries row — its whole "BIJUU SLAYER
// Cost: 32 Creation Points ..." text lives directly in the Mastercraft
// tier's own class_options.description, bundled as a single named item
// rather than a multi-item tier worth splitting. loadTitanUpgradeCatalog
// reads it as a special case.

const (
	titanEndlessWorkFeatureSlug          = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/endless-work"
	titanSpatialWarpingFeatureSlug       = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/spatial-warping"
	titanSpecialistCraftingFeatureSlug   = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/specialist-crafting"
	titanTitanicArsenalFeatureSlug       = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/titanic-arsenal"
	titanFutureOfShinobiMechaFeatureSlug = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/the-future-of-shinobi-mecha"

	titanUpgradesListName        = "Titan Upgrades"
	titanSpecializationsListName = "Titan"
)

// titanBaseAbilityScores: titan_unit_card raw_text's own six numbers, "15
// (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3)" immediately following "Speed
// 30 ft." with zero column labels in the source — read in the standard
// N5e/5e stat-block column order (STR/DEX/CON/INT/WIS/CHA). Before Steady
// Improvement's own ASI points (distributed at crafting, redistributed on
// a long rest) and before any Titan Specialization ability bonus — both
// are free player choices this app never allocates automatically, the
// same "computed hint, player edits from there" boundary
// ninDogBaseAbilityScores (nindog.go) already draws.
var titanBaseAbilityScores = map[string]int{
	"str": 15, "dex": 13, "con": 13, "int": 5, "wis": 5, "cha": 5,
}

const titanBaseSpeed = 30 // ft. — titan_unit_card's own flat "Speed 30 ft.", before any Titan Specialization change

// titanSenses: titan_unit_card's own "Senses Darkvision(30 feet), Passive Perception(...)" line —
// only the Darkvision half, since Passive Perception is already its own computed titanReference field.
const titanSenses = "Darkvision (30 ft.)"

// titanBaseTraits: the base stat block's own named traits, hand-transcribed
// verbatim from titan_unit_card.raw_text (see this file's header doc for
// why this can't be read live). Always shown, ungated beyond the whole box
// requiring Ordnance Training — these describe every Titan, not something
// leveled into.
var titanBaseTraits = []companionFeatureRef{
	{Name: "Battery Powered Barrier", Description: "All Titans are fitted with a barrier that protects the titan from damage. When a Titan is damaged, it subtracts hit points from its barrier first. The Battery Powered Barrier has a maximum number of hit points equal to twice your Science-Nin level, and on your turn, you can spend increments of 5 chakra from your CCD to replenish 10 of the barrier's hit points."},
	{Name: "Extra Attack", Description: "Your Titan can attack twice with the attack action."},
	{Name: "Gradual Expansion", Description: "Your Titan starts off as Large, becoming Huge at 14th level."},
	{Name: "Ninja Tool Integration", Description: "The Titan's attacks are chakra enhanced."},
	{Name: "Steady Improvement", Description: "The Titan gains an additional number of ASI points equal to 1 + your proficiency bonus. You distribute these points when you craft your Titan, and you can redistribute them during a long rest."},
	{Name: "Titan Specialization", Description: "When you craft your Titan, you choose a Titan Specialization for it to following, picking between a Legion Titan, a Monarch Titan, or a Ronin Titan, which grant your Titan additional abilities."},
}

// titanBashAttackText: titan_unit_card's own built-in attack, verbatim.
// "Any other upgrades you have with the Weapon Keyword" (the blob's own
// trailing, mid-sentence cutoff) is garbage PDF-extraction fragment, not a
// real rule — omitted here rather than reproduced as if it were complete.
const titanBashAttackText = "Melee Weapon Attack: reach 10 ft., one target. Hit: 1d6 + Str + Dex in bludgeoning damage. This weapon can be used for the unarmed damage of Taijutsu."

// titanMaxHP: "20+[2*Titan's Constitution Modifer x Science Nin level]" —
// titan_unit_card's own verbatim HP formula.
func titanMaxHP(scienceNinLevel, titanConMod int) int {
	hp := 20 + 2*titanConMod*scienceNinLevel
	if hp < 1 {
		hp = 1
	}
	return hp
}

// titanAC: "12 + Intelligence Modifier + half your Proficiency Bonus"
// (Natural Armor) — see this file's header doc for why this had to be
// verified against the source PDF page image rather than read from
// titan_unit_card.raw_text, which drops the line entirely. Unlike
// titanMaxHP's own formula text ("Titan's Constitution Modifer", explicit),
// this line never says "Titan's" — it's the PLAYER's own Intelligence
// modifier (a Mech Crafter builds a tougher Titan as they invest in their
// own Intelligence), so callers must pass sheet.Abilities["int"].Modifier,
// not titanEffectiveAbilityModifier(companion, "int"). Half proficiency is
// floored, the same convention internal/charsheet.ArmorClass's own "PROF"
// armor-ability term already uses for a player character's armor.
func titanAC(playerIntMod, profBonus int) int {
	return 12 + playerIntMod + profBonus/2
}

// titanBarrierMax: Battery Powered Barrier's own "maximum number of hit
// points equal to twice your Science-Nin level" — a separate HP pool
// layered on top of the Titan's own HP (see migration
// 0066_titan_fields.sql), not affected by Titan Specialization (Monarch's
// own text changes the barrier's REFILL rate per 5 chakra spent, 15
// instead of 10, not its maximum).
func titanBarrierMax(scienceNinLevel int) int {
	return scienceNinLevel * 2
}

// titanSizeForLevel: Gradual Expansion's own "starts off as Large, becoming
// Huge at 14th level" — Bijuu Slayer's own further override to Gargantuan
// is applied by the caller (loadTitanReference), since it depends on an
// owned upgrade pick, not level alone.
func titanSizeForLevel(scienceNinLevel int) string {
	if scienceNinLevel >= 14 {
		return "Huge"
	}
	return "Large"
}

// titanSpeedForSpecialization: Legion/Monarch both restate "movement speed
// of 30 feet" (no change from the base block); Ronin's own text is "35
// feet". specializationSlug is companion.TitanSpecialization — "" (not yet
// chosen) reads as the base 30 ft, same as Legion/Monarch.
func titanSpeedForSpecialization(specializationSlug string) int {
	if strings.Contains(specializationSlug, "ronin") {
		return 35
	}
	return titanBaseSpeed
}

// titanExoSuitCap: Endless Work (6th level) grants 1 Exo-Suit upgrade slot,
// "You can add second upgrade to your Exo-Suit at level 14."
func titanExoSuitCap(level int) int {
	if level >= 14 {
		return 2
	}
	return 1
}

// titanEffectiveAbilityModifier mirrors ninDogEffectiveAbilityModifier
// (nindog.go) exactly: the companion's own stored score if the player has
// set one, else the base stat block's own baseline
// (titanBaseAbilityScores) — never a bare unset SQL NULL reading as score 0
// (modifier -5), which would make a fresh Titan's very first "Sync Max HP"
// hint compute off a nonsensical -5 Constitution modifier instead of the
// real +1 baseline.
func titanEffectiveAbilityModifier(companion charstore.Companion, key string) int {
	var score sql.NullInt64
	switch key {
	case "str":
		score = companion.Str
	case "dex":
		score = companion.Dex
	case "con":
		score = companion.Con
	case "int":
		score = companion.Int
	case "wis":
		score = companion.Wis
	case "cha":
		score = companion.Cha
	}
	if score.Valid {
		return charsheet.AbilityModifier(int(score.Int64))
	}
	return charsheet.AbilityModifier(titanBaseAbilityScores[key])
}

// titanLegionAbilityBonusFeatureSlug is the synthetic (no class_features/
// clan_features row backs it — feature_slug carries no FK constraint, the
// same allowance 0033_feature_companion_choices.sql's own doc comment
// documents) key Legion Specialization's own two-ability-score +2 choice is
// stored under via character_feature_companion_choices, the direct
// precedent Puppet Master's own Symphony of Puppetry Enhancement ability
// pick already established (puppet_companion_bonuses.go) — just keyed off a
// class_options pick (Titan Specialization) rather than a class feature.
// choice_index 0 and 1 hold the two independently-chosen abilities — Legion
// Specialization's own text ("increases its two ability scores by +2. The
// maximums also increase by +2") names no fixed pair, so both are a free
// player pick, same "computed hint, player chooses freely" boundary
// Symphony of Puppetry Enhancement already draws for its own ability pick.
const titanLegionAbilityBonusFeatureSlug = "companion/titan/legion-specialization-ability-bonus"

// titanSpecializationAbilityBonuses resolves the ability-score bonus a
// chosen Titan Specialization grants beyond the base stat block —
// Monarch's flat +4 Constitution, Ronin's flat +4 Dexterity, or Legion's
// own two player-chosen abilities (legionAbility1/2, "" until picked) at
// +2 each. Additive per key rather than an overwrite, so picking the SAME
// ability for both Legion slots correctly stacks to +4 instead of one
// bonus silently replacing the other.
func titanSpecializationAbilityBonuses(specializationSlug, legionAbility1, legionAbility2 string) map[string]int {
	bonuses := map[string]int{}
	switch {
	case strings.Contains(specializationSlug, "monarch"):
		bonuses["con"] += 4
	case strings.Contains(specializationSlug, "ronin"):
		bonuses["dex"] += 4
	case strings.Contains(specializationSlug, "legion"):
		if legionAbility1 != "" {
			bonuses[legionAbility1] += 2
		}
		if legionAbility2 != "" {
			bonuses[legionAbility2] += 2
		}
	}
	return bonuses
}

// titanBashAttack computes Bash's own rollable attack row fresh on every
// render — never stored as a charstore.CompanionAttack row, the same
// "computed, not stored" treatment puppetIntegratedWeaponAttack
// (puppets.go) already gives a granted (not player-added) attack. Both the
// attack roll and the damage draw on the companion's OWN Strength and
// Dexterity modifiers (never the owning character's) per titan_unit_card's
// own "Hit: 1d6 + Str + Dex" text. No separate "+X to hit" formula is
// stated anywhere in the source (confirmed directly against
// titan_unit_card.raw_text) — the attack roll here sums the SAME two
// ability modifiers the damage formula names, plus the owning character's
// proficiency bonus, mirroring both Ninja Tool Integration's own "the
// Titan's attacks are chakra enhanced" (treated as proficient) and Beast
// Master's explicit "Str + Prof to hit" formula for a Nin-Dog's own Bite
// (see ninDogBiteAttack, nindog.go) — the closest confirmed precedent for
// how this app already resolves an unstated companion to-hit bonus. Ronin
// Specialization's own "+1 critical threat range on melee weapon and
// Taijutsu attacks" widens the crit range to 19 here, the same mechanism
// Puppet Roles' Lurker Role Effect already uses (see
// companionAttackRow.CritRangeThreshold's own doc, puppets.go).
// titanHasKnownUpgrade reports whether ref (nil-safe) lists a Known Titan
// Upgrade by exact catalog name (case-insensitive) — shared check for
// entries whose effect hooks into an existing computed field elsewhere on
// the sheet, rather than surfacing only as reference text (loadTitanReference
// already duplicates this same loop inline for Bijuu Slayer's own Gargantuan
// size override; Hulking Strength's Bash die bump below and Sturdy Frame's
// saving-throw bonus, applyTitanSturdyFrameSaves, both reuse it here instead
// of a third copy).
//
// Checks BOTH ref.KnownUpgrades (regular Titan Slots) and ref.ExoSuit.Known
// (Endless Work's separate slot) — Endless Work only accepts a Mech-keyword
// entry costing 8 Creation Points or less, which Sturdy Frame (Refined, 8
// CP) exactly qualifies for, so a Titan can genuinely carry it in EITHER
// slot. Bijuu Slayer (Mastercraft, 32 CP) and Hulking Strength (Supreme, 24
// CP) both cost far more than Endless Work's own cap and so can never
// actually appear in ExoSuit.Known — checking it for them too is still
// correct, just never matches.
func titanHasKnownUpgrade(ref *titanReference, name string) bool {
	if ref == nil {
		return false
	}
	for _, k := range ref.KnownUpgrades {
		if strings.EqualFold(k.Name, name) {
			return true
		}
	}
	if ref.ExoSuit != nil {
		for _, k := range ref.ExoSuit.Known {
			if strings.EqualFold(k.Name, name) {
				return true
			}
		}
	}
	return false
}

// titanBashDamageSteps is Bash's own damage die, stepped up one size by
// Hulking Strength (Supreme, Mech): "increases the damage of its Bash attack
// by 1 damage die" — the same one-step-up shape hunter_nin.go's own
// stepWeaponDie applies to a dice STRING; Bash keeps its die as separate
// DamageCount/DamageSides ints instead, so this is a small parallel table
// rather than a round trip through that string-based helper.
var titanBashDamageSteps = map[int]int{4: 6, 6: 8, 8: 10, 10: 12, 12: 20}

func titanBashAttack(companion charstore.Companion, ref *titanReference, ownerProfBonus int) companionAttackRow {
	strMod := titanEffectiveAbilityModifier(companion, "str")
	dexMod := titanEffectiveAbilityModifier(companion, "dex")
	critRange := 20
	if strings.Contains(companion.TitanSpecialization, "ronin") {
		critRange = 19
	}
	damageSides := 6
	if titanHasKnownUpgrade(ref, "Hulking Strength") {
		if stepped, ok := titanBashDamageSteps[damageSides]; ok {
			damageSides = stepped
		}
	}
	return companionAttackRow{
		CompanionAttack: charstore.CompanionAttack{
			Name:        "Bash",
			Description: "Melee Weapon Attack: reach 10 ft., one target. This weapon can be used for the unarmed damage of Taijutsu.",
			DamageCount: 1, DamageSides: damageSides, DamageType: "bludgeoning",
		},
		AttackTotal:        strMod + dexMod + ownerProfBonus,
		DamageTotal:        strMod + dexMod,
		CritRangeThreshold: critRange,
	}
}

// titanWeaponAttackClausePattern matches a Weapon-keyword Titan Upgrade's own
// baseline "[a] [Melee|Ranged] [Blade|Ammunition|...] Weapon with a
// [Reach|Range] of X[,] and deals NdM [+ Str|Dex] in TYPE damage" clause —
// the stated at-will attack profile Ion Sword/Predator Cannon/Quad Rocket/
// Leadwall's own two modes each give, before any separate CCD-gated bonus
// effect. Confirmed directly against every one of this catalog's 19 raw
// description strings (rules.db, list_name='Titan Upgrades'): the other 5
// Weapon-keyword entries (Thermite Launcher, Xo-16 Gatling, Greater Missile
// Racks, Critical Ejection, Enhanced Attack Protocol) never contain a
// "Weapon with a Reach/Range of" clause at all — their entire offense is
// gated behind spending the CCD Drain just to act, a reactive death-trigger,
// or an attack-economy bonus — so this pattern naturally excludes them
// without a separate name-based denylist, and naturally extends to any
// future entry sharing the same baseline-weapon shape instead of growing a
// hardcoded per-name switch.
var titanWeaponAttackClausePattern = regexp.MustCompile(
	`(Melee|Ranged)\s+\w+\s+Weapon\s+with\s+a\s+(?:Reach|Range)\s+of\s+(.+?)\s*,?\s+and\s+deals:?\s+(\d+d\d+)\s*\+\s*(Str|Dex)\s+in\s+(\w+)\s+damage`)

// titanWeaponAttacksFromDescription extracts every baseline weapon clause
// (titanWeaponAttackClausePattern) from a Known Titan Upgrade's own raw
// description text and turns each into a standing rollable attack row — one
// row per clause, so Leadwall's own two built-in modes ("both a Melee
// ...and a Ranged...") each still produce their own row exactly as the
// former hand-coded switch did, with a "(Melee)"/"(Ranged)" name suffix only
// added when an entry has more than one clause. nil for any description with
// no matching clause (every Mech-keyword entry, and the 5 Weapon-keyword
// entries with no standing weapon of their own — see
// titanWeaponAttackClausePattern's own doc).
func titanWeaponAttacksFromDescription(name, description string, companion charstore.Companion, ownerProfBonus int) []companionAttackRow {
	matches := titanWeaponAttackClausePattern.FindAllStringSubmatch(description, -1)
	if matches == nil {
		return nil
	}
	const grantedHint = "Granted by a Titan Upgrade — remove the upgrade to remove this attack"
	var out []companionAttackRow
	for _, m := range matches {
		isMelee := strings.EqualFold(m[1], "Melee")
		reachOrRange := strings.TrimSpace(strings.Trim(m[2], "()"))
		count, sides := 1, 6
		if dice := damageDicePattern.FindStringSubmatch(m[3]); dice != nil {
			count, _ = strconv.Atoi(dice[1])
			sides, _ = strconv.Atoi(dice[2])
		}
		abilityMod := titanEffectiveAbilityModifier(companion, strings.ToLower(m[4]))
		damageType := strings.ToLower(m[5])

		rowName := name
		var desc string
		if isMelee {
			desc = fmt.Sprintf("Melee Weapon Attack: reach %s ft., one target.", strings.TrimSuffix(reachOrRange, "ft"))
			if len(matches) > 1 {
				rowName = name + " (Melee)"
			}
		} else {
			desc = fmt.Sprintf("Ranged Weapon Attack: range %s ft., one target.", reachOrRange)
			if len(matches) > 1 {
				rowName = name + " (Ranged)"
			}
		}

		out = append(out, companionAttackRow{
			CompanionAttack: charstore.CompanionAttack{
				Name:        rowName,
				Description: desc,
				DamageCount: count, DamageSides: sides, DamageType: damageType,
			},
			AttackTotal: abilityMod + ownerProfBonus,
			DamageTotal: abilityMod,
			GrantedHint: grantedHint,
		})
	}
	return out
}

// titanKnownWeaponUpgradeAttacks flattens every Known Titan Upgrade with the
// Weapon keyword into its own standing attack row(s) — nil-safe against a
// character with no Ordnance Training (loadTitanReference's own nil case).
func titanKnownWeaponUpgradeAttacks(ref *titanReference, companion charstore.Companion, ownerProfBonus int) []companionAttackRow {
	if ref == nil {
		return nil
	}
	var out []companionAttackRow
	for _, k := range ref.KnownUpgrades {
		if k.Keyword != "weapon" {
			continue
		}
		out = append(out, titanWeaponAttacksFromDescription(k.Name, k.Description, companion, ownerProfBonus)...)
	}
	return out
}

// titanSturdyFrameSaveAbilities is Sturdy Frame's own "Physical saving
// throws" — this app's existing STR/DEX/CON split for "Physical" vs.
// "Mental" (INT/WIS/CHA), the same grouping titan_reference's own base
// traits text already uses for Titan's fixed-proficiency saves.
var titanSturdyFrameSaveAbilities = map[string]bool{"str": true, "dex": true, "con": true}

// applyTitanSturdyFrameSaves adds Sturdy Frame's (Refined, Mech) own "you can
// add half your Intelligence modifier to Physical saving throws that you are
// not proficient in" bonus to a Titan's already-computed Saving Throws box,
// no-op if the Titan doesn't have the upgrade. Uses the PLAYER's own
// Intelligence modifier, not the Titan's, the same precedent titanAC and
// titanPassivePerception already set for every other Titan formula that
// names "Intelligence Modifier" without saying "Titan's" (loadTitanReference
// captures this once per render as playerIntMod; callers pass it through
// rather than each recomputing it). Only reaches non-proficient STR/DEX/CON
// rows — a row already proficient, or any of INT/WIS/CHA, is left untouched.
func applyTitanSturdyFrameSaves(saves companionSavesView, ref *titanReference, playerIntMod int) companionSavesView {
	if !titanHasKnownUpgrade(ref, "Sturdy Frame") {
		return saves
	}
	bonus := playerIntMod / 2
	if playerIntMod%2 != 0 && playerIntMod < 0 {
		bonus-- // floor, not truncate — half of -3 is -2, not -1
	}
	for i, row := range saves.Rows {
		if !row.Proficient && titanSturdyFrameSaveAbilities[row.Ability] {
			saves.Rows[i].Modifier += bonus
		}
	}
	return saves
}

// titanSpecializationOption is one of the 3 Titan Specializations
// (class_options list_name='Titan') — each a single self-contained option
// with no class_option_entries sub-rows, read live from rules.db (unlike
// titanBaseTraits above) since these ARE stored as individually
// addressable rows.
type titanSpecializationOption struct {
	Slug        string
	Name        string
	Description string
}

func (s *server) loadTitanSpecializationOptions() ([]titanSpecializationOption, error) {
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, description FROM class_options WHERE class_slug = ? AND list_name = ? ORDER BY sort_order`,
		scienceNinSlug, titanSpecializationsListName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []titanSpecializationOption
	for rows.Next() {
		var o titanSpecializationOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// titanUpgradeStatLinePattern peels a Titan Upgrades entry's own leading
// "Cost: N Creation Points Keyword: Mech|Weapon (Drain: N CCD Chakra)?"
// stat line off the front of its description — a different shape from
// science_nin.go's scienceNinToolStatLinePattern (no catalog there needs to
// parse a Keyword clause). Confirmed against every one of the 19 real Titan
// Upgrades entries plus the Mastercraft tier's own bundled Bijuu Slayer
// text before writing this pattern — Sturdy Frame is the one entry with no
// Drain line at all (a passive upgrade with no CCD activation cost), which
// is why that whole group is optional.
var titanUpgradeStatLinePattern = regexp.MustCompile(
	`^Cost: (\d+) Creation Points? Keyword: (Mech|Weapon)(?: Drain: (\d+) CCD Chakra)?\s*`)

// parseTitanUpgradeStatLine locates the "Cost:" stat line within raw (which
// for the Mastercraft tier's own bundled text is preceded by "BIJUU SLAYER
// ", the item's own name folded into the description since it was never
// split into its own class_option_entries row — for every other entry
// nothing precedes "Cost:" at all) and parses it, returning the remaining
// body text with the stat line stripped.
func parseTitanUpgradeStatLine(raw string) (cost, drain int, keyword, rest string) {
	idx := strings.Index(raw, "Cost:")
	if idx < 0 {
		return 0, 0, "", raw
	}
	trimmed := raw[idx:]
	m := titanUpgradeStatLinePattern.FindStringSubmatch(trimmed)
	if m == nil {
		return 0, 0, "", raw
	}
	cost, _ = strconv.Atoi(m[1])
	keyword = strings.ToLower(m[2])
	if m[3] != "" {
		drain, _ = strconv.Atoi(m[3])
	}
	rest = strings.TrimSpace(trimmed[len(m[0]):])
	return cost, drain, keyword, rest
}

// titanUpgradeOption is one individually named Titan Upgrades entry.
type titanUpgradeOption struct {
	Slug        string
	Name        string
	Tier        string // "Minor" | "Refined" | "Superior" | "Supreme" | "Mastercraft" — see this file's header doc on the Refined/Greater merge
	Keyword     string // "mech" | "weapon"
	Cost        int    // Creation Points, before Specialist Crafting's own discount
	Drain       int    // CCD Chakra to activate; 0 = no drain stated (Sturdy Frame only)
	Description string
}

// loadTitanUpgradeCatalog reads every Titan Upgrades entry (Minor through
// Supreme, split into class_option_entries the same way Scientific Ninja
// Tools' own tiers are) plus the Mastercraft tier's own single bundled item
// (Bijuu Slayer, read directly off the tier's class_options row — see this
// file's header doc for why it was never split).
func (s *server) loadTitanUpgradeCatalog() ([]titanUpgradeOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT e.slug, e.name, e.description, o.name
		FROM class_option_entries e
		JOIN class_options o ON o.slug = e.class_option_slug
		WHERE o.class_slug = ? AND o.list_name = ?
		ORDER BY o.sort_order, e.sort_order`, scienceNinSlug, titanUpgradesListName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []titanUpgradeOption
	for rows.Next() {
		var o titanUpgradeOption
		var raw string
		if err := rows.Scan(&o.Slug, &o.Name, &raw, &o.Tier); err != nil {
			return nil, err
		}
		o.Cost, o.Drain, o.Keyword, o.Description = parseTitanUpgradeStatLine(raw)
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var mastercraftSlug, mastercraftDesc string
	err = s.rulesDB.QueryRow(`
		SELECT slug, description FROM class_options
		WHERE class_slug = ? AND list_name = ? AND name = 'Mastercraft'`,
		scienceNinSlug, titanUpgradesListName,
	).Scan(&mastercraftSlug, &mastercraftDesc)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		o := titanUpgradeOption{Slug: mastercraftSlug, Name: "Bijuu Slayer", Tier: "Mastercraft"}
		o.Cost, o.Drain, o.Keyword, o.Description = parseTitanUpgradeStatLine(mastercraftDesc)
		out = append(out, o)
	}
	return out, nil
}

// titanEffectiveUpgradeCost applies Specialist Crafting's (14th level) own
// "Upgrades of that keyword cost 2 less creation points (min. 1)" discount
// — catalog-wide, applied identically whether the upgrade is going into an
// ordinary Titan Slot or the Exo-Suit slot. specialistKeyword is ""
// (nothing designated, or the character doesn't have Specialist Crafting
// yet) or "mech"/"weapon".
func titanEffectiveUpgradeCost(o titanUpgradeOption, specialistKeyword string) int {
	if specialistKeyword != "" && o.Keyword == specialistKeyword {
		cost := o.Cost - 2
		if cost < 1 {
			cost = 1
		}
		return cost
	}
	return o.Cost
}

// knownTitanUpgrade is one entry on a Known Titan Slot / Exo-Suit list —
// Cost is the EFFECTIVE cost actually charged (after Specialist Crafting's
// discount, if any), not the catalog's own raw Cost.
type knownTitanUpgrade struct {
	Slug, Name, Tier, Keyword string
	Cost, Drain               int
	// Description: the catalog entry's own rules text (stat line already
	// stripped by parseTitanUpgradeStatLine) — shown on the companion sheet
	// itself (companion_fields.html) so a Mech-keyword upgrade's effect is
	// readable right there rather than only inside the separate Titan Slots
	// popup. Weapon-keyword upgrades with a standing attack profile also
	// show up as their own rollable row (titanWeaponUpgradeAttacks) — this
	// text is what covers everything that ISN'T a fixed die+ability+type
	// attack, since most of it (reactions, once-per-rest triggers, passive
	// stat changes) has no other place on the sheet to live yet.
	Description string
}

// titanExoSuitData backs Endless Work's own separate Exo-Suit slot — non-
// nil only once the character has that 6th-level feature.
type titanExoSuitData struct {
	Cap       int
	Used      int
	Known     []knownTitanUpgrade
	Available []titanUpgradeOption
}

// titanUpgradesData is the character-scoped half of the Titan companion
// system — the Creation Points budget, Titan Slots, and Exo-Suit slot, all
// independent of which specific companion row (there is ever at most one
// live Titan per Ordnance Training's own "You can only have 1 Titan created
// at a time") the picks belong to. Loaded on its own by the add/delete
// route handlers (which don't need a companion row at all), and embedded
// into the fuller titanReference for display.
type titanUpgradesData struct {
	CreationPointsCap  int
	CreationPointsUsed int

	SpecialistCraftingAvailable bool
	SpecialistCraftingKeyword   string // "" (not yet chosen), "mech", or "weapon"

	TitanSlotsCap     int // Ordnance Training: "a number of Titan Slots equal to your Proficiency Bonus"
	TitanSlotsUsed    int
	KnownUpgrades     []knownTitanUpgrade
	AvailableUpgrades []titanUpgradeOption

	// ExoSuit is non-nil only once Endless Work (6th level) is granted.
	ExoSuit *titanExoSuitData
}

// titanUpgradesCreationPointsSpend sums the Creation Points committed to
// Titan Slots and, once unlocked, the Exo-Suit slot — the Titan half of Mech
// Crafter's shared Creation Points budget. Called from
// loadScienceNinTabData (science_nin.go) so the base Scientific Ninja Tools
// panel's own CreationPointsUsed total — and therefore
// handleScienceNinToolAdd's own spend-budget check — sees Titan spend too,
// closing the asymmetry where only loadTitanUpgradesData (below) ever
// combined the two: a Mech Crafter could previously spend the full cap on
// SNTs, then independently spend the full cap again on Titan Upgrades,
// because the SNT side's own CreationPointsUsed never counted Titan spend.
//
// Deliberately independent of loadTitanUpgradesData itself (which needs the
// full per-entry Known/Available lists for display; this only needs the
// sum) rather than sharing code with it, to avoid a call cycle:
// loadTitanUpgradesData already calls loadScienceNinTabData for its own
// baseline, so loadScienceNinTabData calling loadTitanUpgradesData back
// would recurse forever. Both apply the identical titanEffectiveUpgradeCost
// formula against the identical picks/catalog, so the two totals cannot
// drift apart in practice even though the code itself isn't shared — keep
// both in sync if either is ever restructured.
func (s *server) titanUpgradesCreationPointsSpend(characterID int64, granted []grantedFeatureRow) (int, error) {
	if !hasFeature(granted, scienceNinOrdnanceTrainingFeatureSlug) {
		return 0, nil
	}

	specialistKeyword := ""
	if hasFeature(granted, titanSpecialistCraftingFeatureSlug) {
		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTitanSpecialistCraftingKeyword)
		if err != nil {
			return 0, err
		}
		if len(picks) > 0 {
			specialistKeyword = picks[0].OptionSlug
		}
	}

	catalog, err := s.loadTitanUpgradeCatalog()
	if err != nil {
		return 0, err
	}
	catalogBySlug := make(map[string]titanUpgradeOption, len(catalog))
	for _, o := range catalog {
		catalogBySlug[o.Slug] = o
	}

	total := 0
	slotPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTitanUpgrade)
	if err != nil {
		return 0, err
	}
	for _, p := range slotPicks {
		if o, ok := catalogBySlug[p.OptionSlug]; ok {
			total += titanEffectiveUpgradeCost(o, specialistKeyword)
		}
	}

	if hasFeature(granted, titanEndlessWorkFeatureSlug) {
		exoPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTitanExosuitUpgrade)
		if err != nil {
			return 0, err
		}
		for _, p := range exoPicks {
			if o, ok := catalogBySlug[p.OptionSlug]; ok {
				total += titanEffectiveUpgradeCost(o, specialistKeyword)
			}
		}
	}

	return total, nil
}

// loadTitanUpgradesData returns nil for a character with no Ordnance
// Training granted — the template gates the whole Titan reference panel's
// existence on this being non-nil (via loadTitanReference), same treatment
// loadScienceNinTabData's own nil return gets.
func (s *server) loadTitanUpgradesData(characterID int64, sheet *charsheet.Sheet) (*titanUpgradesData, error) {
	granted, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	has := make(map[string]bool, len(granted))
	for _, f := range granted {
		has[f.Slug] = true
	}
	if !has[scienceNinOrdnanceTrainingFeatureSlug] {
		return nil, nil
	}

	scienceNinData, err := s.loadScienceNinTabData(characterID, sheet)
	if err != nil {
		return nil, err
	}
	data := &titanUpgradesData{TitanSlotsCap: sheet.ProficiencyBonus}
	if scienceNinData != nil {
		data.CreationPointsCap = scienceNinData.CreationPointsCap
		// Already the FULL combined total, not just SNT spend —
		// loadScienceNinTabData folds this same Titan Upgrades cost (via
		// titanUpgradesCreationPointsSpend, above) into its own
		// CreationPointsUsed before returning. The loops below build
		// KnownUpgrades/ExoSuit.Known for DISPLAY only and must NOT add
		// their own cost into data.CreationPointsUsed again — doing so
		// would double-count every Titan pick against the budget.
		data.CreationPointsUsed = scienceNinData.CreationPointsUsed
	}

	if has[titanSpecialistCraftingFeatureSlug] {
		data.SpecialistCraftingAvailable = true
		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTitanSpecialistCraftingKeyword)
		if err != nil {
			return nil, err
		}
		if len(picks) > 0 {
			data.SpecialistCraftingKeyword = picks[0].OptionSlug
		}
	}

	catalog, err := s.loadTitanUpgradeCatalog()
	if err != nil {
		return nil, err
	}
	catalogBySlug := make(map[string]titanUpgradeOption, len(catalog))
	for _, o := range catalog {
		catalogBySlug[o.Slug] = o
	}

	slotPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTitanUpgrade)
	if err != nil {
		return nil, err
	}
	pickedSlot := make(map[string]bool, len(slotPicks))
	for _, p := range slotPicks {
		pickedSlot[p.OptionSlug] = true
		o, ok := catalogBySlug[p.OptionSlug]
		if !ok {
			continue // stale pick pointing at a since-renamed/removed catalog entry
		}
		cost := titanEffectiveUpgradeCost(o, data.SpecialistCraftingKeyword)
		data.KnownUpgrades = append(data.KnownUpgrades, knownTitanUpgrade{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Keyword: o.Keyword, Cost: cost, Drain: o.Drain, Description: o.Description})
		// NOT added into data.CreationPointsUsed here — see this cost's
		// twin computation in titanUpgradesCreationPointsSpend, already
		// folded into scienceNinData.CreationPointsUsed above.
	}
	data.TitanSlotsUsed = len(slotPicks)
	for _, o := range catalog {
		if !pickedSlot[o.Slug] {
			data.AvailableUpgrades = append(data.AvailableUpgrades, o)
		}
	}

	if has[titanEndlessWorkFeatureSlug] {
		exo := &titanExoSuitData{Cap: titanExoSuitCap(sheet.Level)}
		exoPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTitanExosuitUpgrade)
		if err != nil {
			return nil, err
		}
		pickedExo := make(map[string]bool, len(exoPicks))
		for _, p := range exoPicks {
			pickedExo[p.OptionSlug] = true
			o, ok := catalogBySlug[p.OptionSlug]
			if !ok {
				continue
			}
			cost := titanEffectiveUpgradeCost(o, data.SpecialistCraftingKeyword)
			exo.Known = append(exo.Known, knownTitanUpgrade{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Keyword: o.Keyword, Cost: cost, Drain: o.Drain, Description: o.Description})
			// Also already folded in — see the identical note above.
		}
		exo.Used = len(exoPicks)
		for _, o := range catalog {
			// Endless Work: "Greater or lower Mech keyword upgrade" — see
			// this file's header doc on why Cost <= 8 is the correct read
			// of "Greater or lower" against the merged Refined/Greater
			// bucket, rather than gating by tier name.
			if o.Keyword != "mech" || o.Cost > 8 {
				continue
			}
			if !pickedExo[o.Slug] {
				exo.Available = append(exo.Available, o)
			}
		}
		data.ExoSuit = exo
	}

	return data, nil
}

// titanReference is the read-only/interactive panel shown on a kind="titan"
// companion's own popup and its card on the Companions tab — the Titan
// equivalent of ninDogReference/summonTribeReference, built from Ordnance
// Training's own crafting rules and the base stat block rather than a
// generic rank-gated tribe catalog.
type titanReference struct {
	*titanUpgradesData

	Level                      int
	Size                       string // Large/Huge/Gargantuan — Gradual Expansion + Bijuu Slayer
	ProficiencyBonus           int
	Senses                     string // titan_unit_card's own "Darkvision(30 feet)" — printed alongside Passive Perception in the same Senses line but previously dropped when that line was transcribed
	PassivePerception          int    // "Yours + Intelligence Modifer" — the caster's own Passive Perception plus the Titan's own Intelligence modifier
	SteadyImprovementASIPoints int

	BaseTraits     []companionFeatureRef
	BashAttackText string

	Specializations []titanSpecializationOption
	Specialization  *titanSpecializationOption // nil until chosen (locked once set — see charstore.SetCompanionFields)
	// IsLegionSpecialization: true once Legion Specialization is chosen —
	// gates the LegionAbility1/2 picker below, since Legion is the only one
	// of the three specializations with a free player ability-score choice
	// rather than a fixed bonus (see titanSpecializationAbilityBonuses).
	IsLegionSpecialization bool
	// LegionAbility1/LegionAbility2: Legion Specialization's own two
	// player-chosen abilities (+2 each), "" until picked — see
	// titanLegionAbilityBonusFeatureSlug's own doc for how these are
	// stored/resolved.
	LegionAbility1 string
	LegionAbility2 string

	Features []companionFeatureRef // Adaptive Movement/Endless Work/Spatial Warping/Specialist Crafting/Titanic Arsenal/The Future of Shinobi: Mecha, Locked by level

	// Expected*: the same "computed hint, never silently overwritten, Sync
	// button available" treatment ninDogReference's own Expected* fields
	// already give a Nin-Dog — companion_stat_fields.html's Sync-button
	// block reads these same field names generically regardless of
	// companion kind. ExpectedStr..ExpectedCha already include whichever
	// Titan Specialization ability bonus applies (titanSpecializationAbilityBonuses).
	ExpectedAC         int
	ExpectedMaxHP      int
	ExpectedSpeed      int
	ExpectedSize       string
	ExpectedBarrierMax int
	ExpectedStr        int
	ExpectedDex        int
	ExpectedCon        int
	ExpectedInt        int
	ExpectedWis        int
	ExpectedCha        int
}

// titanReferenceOrZero returns *ref, or a zero-value titanReference if ref
// is nil — mirrors ninDogReferenceOrZero (nindog.go) exactly, same reason:
// companion_stat_fields' dict call needs to field-access TitanReference's
// Expected* values regardless of the companion's kind, built once before
// the kind branch, and html/template rejects an {{if}} inside a pipeline
// argument. Safe even with the embedded *titanUpgradesData left nil in the
// zero value: every Expected* field read through this path is a plain
// field on titanReference itself, never a field promoted through the
// embedded pointer.
func titanReferenceOrZero(ref *titanReference) titanReference {
	if ref == nil {
		return titanReference{}
	}
	return *ref
}

// titanFeatureDef is one Mech Crafter subclass feature shown in the
// reference panel's own Features list, beyond Ordnance Training (which the
// whole panel is already gated on) — description read live from
// v_subclass_features rather than hand-transcribed, same restraint
// mergeMixedStudiesFeatures (science_nin.go) already applies.
var titanFeatureDefs = []struct {
	Slug  string
	Name  string
	Level int
}{
	{scienceNinAdaptiveMovementFeatureSlug, "Adaptive Movement", 3},
	{titanEndlessWorkFeatureSlug, "Endless Work", 6},
	{titanSpatialWarpingFeatureSlug, "Spatial Warping", 9},
	{titanSpecialistCraftingFeatureSlug, "Specialist Crafting", 14},
	{titanTitanicArsenalFeatureSlug, "Titanic Arsenal", 17},
	{titanFutureOfShinobiMechaFeatureSlug, "The Future of Shinobi: Mecha", 20},
}

// loadTitanReference returns nil for a character with no Ordnance Training
// granted (same gate loadTitanUpgradesData already applies) — the template
// shows a short explanatory line instead of the whole panel in that case
// (see the "titan_reference" template's own top guard), the same shape a
// kind="nin-dog" companion falls back to before a breed is chosen.
func (s *server) loadTitanReference(characterID int64, sheet *charsheet.Sheet, companion charstore.Companion) (*titanReference, error) {
	upgrades, err := s.loadTitanUpgradesData(characterID, sheet)
	if err != nil {
		return nil, err
	}
	if upgrades == nil {
		return nil, nil
	}

	scienceNinLevel, err := s.scienceNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}

	specOptions, err := s.loadTitanSpecializationOptions()
	if err != nil {
		return nil, err
	}
	var chosenSpec *titanSpecializationOption
	for i := range specOptions {
		if specOptions[i].Slug == companion.TitanSpecialization {
			chosenSpec = &specOptions[i]
			break
		}
	}

	var features []companionFeatureRef
	for _, fd := range titanFeatureDefs {
		var desc string
		if err := s.rulesDB.QueryRow(`SELECT description FROM v_subclass_features WHERE slug = ?`, fd.Slug).Scan(&desc); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		features = append(features, companionFeatureRef{Name: fd.Name, Level: fd.Level, Description: desc, Locked: sheet.Level < fd.Level})
	}

	legionAbility1, err := charstore.GetFeatureCompanionChoice(s.charDB, characterID, titanLegionAbilityBonusFeatureSlug, companion.ID, 0)
	if err != nil {
		return nil, err
	}
	legionAbility2, err := charstore.GetFeatureCompanionChoice(s.charDB, characterID, titanLegionAbilityBonusFeatureSlug, companion.ID, 1)
	if err != nil {
		return nil, err
	}
	abilityBonuses := titanSpecializationAbilityBonuses(companion.TitanSpecialization, legionAbility1, legionAbility2)

	// overrides: this Titan's own manual pins (companionOverrideFields),
	// consulted by every formula below ahead of SetTitanStatDefaultsLive
	// writing whichever value (pinned or freshly computed) actually lands
	// in the row — see migration 0079_companion_overrides.sql's own doc.
	overrides, err := charstore.GetCompanionOverrides(s.charDB, companion.ID)
	if err != nil {
		return nil, err
	}
	finalAbilityScore := func(key string) int {
		if v, ok := companionOverrideInt(overrides, key+"_score"); ok {
			return int(v)
		}
		return titanBaseAbilityScores[key] + abilityBonuses[key]
	}

	conMod := charsheet.AbilityModifier(finalAbilityScore("con"))
	// Passive Perception's own line ("Yours + Intelligence Modifer") is kept
	// reading the TITAN's own Intelligence modifier, per this struct field's
	// existing doc comment — distinct from AC below.
	intMod := charsheet.AbilityModifier(finalAbilityScore("int"))
	// AC's "Intelligence Modifier" is the PLAYER's own, not the Titan's own
	// stat block — unlike titanMaxHP, whose source text explicitly says
	// "Titan's Constitution Modifer". titanAC's own source line never says
	// "Titan's", and a Titan's base Int score (5, modifier -3) would make
	// AC go DOWN as a player invests more into their own Intelligence,
	// which contradicts a Mech Crafter building a smarter machine.
	playerIntMod := sheet.Abilities["int"].Modifier

	size := titanSizeForLevel(scienceNinLevel)
	for _, k := range upgrades.KnownUpgrades {
		if strings.EqualFold(k.Name, "Bijuu Slayer") {
			size = "Gargantuan" // Mastercraft: "Your Titan's size becomes Gargantuan"
		}
	}
	if v, ok := overrides["size"]; ok {
		size = v
	}

	ac := titanAC(playerIntMod, sheet.ProficiencyBonus)
	if v, ok := companionOverrideInt(overrides, "ac"); ok {
		ac = int(v)
	}
	hpMax := titanMaxHP(scienceNinLevel, conMod)
	if v, ok := companionOverrideInt(overrides, "hp_max"); ok {
		hpMax = int(v)
	}
	speed := titanSpeedForSpecialization(companion.TitanSpecialization)
	if v, ok := companionOverrideInt(overrides, "speed"); ok {
		speed = int(v)
	}
	barrierMax := titanBarrierMax(scienceNinLevel)
	if v, ok := companionOverrideInt(overrides, "barrier_max"); ok {
		barrierMax = int(v)
	}

	if err := charstore.SetTitanStatDefaultsLive(s.charDB, characterID, companion.ID,
		int64(ac), int64(hpMax), int64(speed), int64(barrierMax),
		int64(finalAbilityScore("str")), int64(finalAbilityScore("dex")), int64(finalAbilityScore("con")),
		int64(finalAbilityScore("int")), int64(finalAbilityScore("wis")), int64(finalAbilityScore("cha")), size,
	); err != nil {
		return nil, err
	}

	return &titanReference{
		titanUpgradesData: upgrades,

		Level:                      sheet.Level,
		Size:                       size,
		ProficiencyBonus:           sheet.ProficiencyBonus,
		Senses:                     titanSenses,
		PassivePerception:          sheet.PassivePerception + intMod,
		SteadyImprovementASIPoints: 1 + sheet.ProficiencyBonus,

		BaseTraits:     titanBaseTraits,
		BashAttackText: titanBashAttackText,

		Specializations:        specOptions,
		Specialization:         chosenSpec,
		IsLegionSpecialization: strings.Contains(companion.TitanSpecialization, "legion"),
		LegionAbility1:         legionAbility1,
		LegionAbility2:         legionAbility2,

		Features: features,

		ExpectedAC:         ac,
		ExpectedMaxHP:      hpMax,
		ExpectedSpeed:      speed,
		ExpectedSize:       size,
		ExpectedBarrierMax: barrierMax,
		ExpectedStr:        finalAbilityScore("str"),
		ExpectedDex:        finalAbilityScore("dex"),
		ExpectedCon:        finalAbilityScore("con"),
		ExpectedInt:        finalAbilityScore("int"),
		ExpectedWis:        finalAbilityScore("wis"),
		ExpectedCha:        finalAbilityScore("cha"),
	}, nil
}

// prefillTitanStatDefaults populates a freshly-created Titan's AC/HP-max/
// Speed/Barrier-max/six ability scores from Ordnance Training's own
// computed baseline — called exactly once, right after creation, mirroring
// prefillNinDogStatDefaults (nindog.go) so a brand-new Titan reaches its
// first render already showing correct starting numbers instead of a blank
// card the player has to click every Sync button on just to see them.
// Reuses loadTitanReference for the actual computation (titanAC/titanMaxHP/
// titanSpeedForSpecialization/titanBarrierMax/titanBaseAbilityScores,
// transitively) — the exact same values the Sync buttons already offer, so
// this can never drift from what a later manual Sync click would set. If
// the character has no Ordnance Training yet (loadTitanReference returns
// nil), this is a no-op — the companion just starts blank like any other
// companion would, same as prefillNinDogStatDefaults' own treatment of a
// load failure. Uses charstore.SetTitanStatDefaults (COALESCE-based), never
// SetCompanionFields, so it can't clobber a field the player somehow
// already touched between creation and this call.
func (s *server) prefillTitanStatDefaults(characterID, companionID int64) error {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return err
	}
	companion, err := charstore.GetCompanion(s.charDB, characterID, companionID)
	if err != nil {
		return err
	}
	ref, err := s.loadTitanReference(characterID, sheet, companion)
	if err != nil || ref == nil {
		return err
	}
	return charstore.SetTitanStatDefaults(s.charDB, characterID, companionID,
		int64(ref.ExpectedAC),
		int64(ref.ExpectedMaxHP), int64(ref.ExpectedMaxHP), int64(ref.ExpectedSpeed),
		int64(ref.ExpectedBarrierMax), int64(ref.ExpectedBarrierMax),
		int64(ref.ExpectedStr), int64(ref.ExpectedDex), int64(ref.ExpectedCon),
		int64(ref.ExpectedInt), int64(ref.ExpectedWis), int64(ref.ExpectedCha),
	)
}

// handleSheetTitanLegionAbilityBonus records (or changes) one of Legion
// Specialization's own two ability-score +2 picks — slot "0" or "1"
// (choice_index), freely re-pickable at any time since Legion's own text
// states no restriction on the choice the way a class feature's permanent
// pick would, the same "trust the player, no lock" boundary most companion
// field edits already draw. Re-derives eligibility from the companion's own
// stored TitanSpecialization rather than trusting the posted companion id
// blindly, the same rule handleSheetPuppetSymphonyEnhancementAbility
// (puppet_companion_bonuses.go) already follows for its own ability pick.
func (s *server) handleSheetTitanLegionAbilityBonus(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slot := r.FormValue("slot")
	if slot != "0" && slot != "1" {
		http.Error(w, "slot must be 0 or 1", http.StatusBadRequest)
		return
	}
	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "not a valid ability pick", http.StatusBadRequest)
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for titan legion ability pick:", err)
		return
	}
	if !strings.Contains(companion.TitanSpecialization, "legion") {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}
	choiceIndex, _ := strconv.Atoi(slot)
	if err := charstore.SetFeatureCompanionChoice(s.charDB, id, titanLegionAbilityBonusFeatureSlug, cid, choiceIndex, ability); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set titan legion ability pick:", err)
		return
	}
	s.respondCompanionAction(w, r, id, cid)
}

// addTitanSlotPick validates and installs one Titan Upgrade into an
// ordinary Titan Slot (Ordnance Training, cap = Proficiency Bonus), gated
// by the character's own remaining Creation Points budget — the same
// Creation-Points-BUDGET shape handleScienceNinToolAdd (science_nin.go)
// already uses, not a flat slot-count gate, since Titan Upgrades spend
// from the identical shared pool. Returns (http.StatusOK, "") on success,
// or the status/message an http.Error should report on failure. Shared by
// the Companions-tab's own AJAX route (handleTitanUpgradeAdd, just below)
// and the Titan Slots popup's plain POST route (handleTitanSlotsAdd,
// titan_slots_popup.go) — see addSNBUpgradePick's own doc
// (science_nin_subclasses.go) for why this split exists.
func (s *server) addTitanSlotPick(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing upgrade"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for titan upgrade add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadTitanUpgradesData(id, sheet)
	if err != nil {
		log.Println("load titan upgrades for add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil {
		return http.StatusBadRequest, "character has no Ordnance Training yet"
	}
	if data.TitanSlotsUsed >= data.TitanSlotsCap {
		return http.StatusBadRequest, "no Titan Slots remaining"
	}
	var picked *titanUpgradeOption
	for i, o := range data.AvailableUpgrades {
		if o.Slug == slug {
			picked = &data.AvailableUpgrades[i]
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid upgrade"
	}
	cost := titanEffectiveUpgradeCost(*picked, data.SpecialistCraftingKeyword)
	if data.CreationPointsUsed+cost > data.CreationPointsCap {
		return http.StatusBadRequest, "not enough Creation Points remaining"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickTitanUpgrade, slug, ""); err != nil {
		log.Println("add titan upgrade:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleTitanUpgradeAdd is the Companions-tab's own AJAX route for
// addTitanSlotPick (above).
func (s *server) handleTitanUpgradeAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addTitanSlotPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// addTitanExoSuitPick validates and installs one Titan Upgrade into
// Endless Work's own separate Exo-Suit slot — Mech-keyword-only, Cost 8 or
// lower (see this file's header doc), still gated by the SAME shared
// Creation Points budget as an ordinary Titan Slot pick. Same
// success/failure return shape as addTitanSlotPick, shared the same way
// between handleTitanExoSuitUpgradeAdd (below) and the Titan Slots popup's
// handleTitanSlotsExoSuitAdd (titan_slots_popup.go).
func (s *server) addTitanExoSuitPick(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing upgrade"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for titan exo-suit upgrade add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadTitanUpgradesData(id, sheet)
	if err != nil {
		log.Println("load titan upgrades for exo-suit add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.ExoSuit == nil {
		return http.StatusBadRequest, "character has no Exo-Suit yet"
	}
	if data.ExoSuit.Used >= data.ExoSuit.Cap {
		return http.StatusBadRequest, "no Exo-Suit slots remaining"
	}
	var picked *titanUpgradeOption
	for i, o := range data.ExoSuit.Available {
		if o.Slug == slug {
			picked = &data.ExoSuit.Available[i]
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid upgrade"
	}
	cost := titanEffectiveUpgradeCost(*picked, data.SpecialistCraftingKeyword)
	if data.CreationPointsUsed+cost > data.CreationPointsCap {
		return http.StatusBadRequest, "not enough Creation Points remaining"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickTitanExosuitUpgrade, slug, ""); err != nil {
		log.Println("add titan exo-suit upgrade:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleTitanExoSuitUpgradeAdd is the Companions-tab's own AJAX route for
// addTitanExoSuitPick (above).
func (s *server) handleTitanExoSuitUpgradeAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addTitanExoSuitPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// handleTitanPickDelete builds a "forget a Titan pick" route for either
// category (Titan Slot or Exo-Suit) — freely, at any time, same "trust the
// player" boundary every other pick removal in this codebase draws. Unlike
// handleScienceNinSubclassPickDelete (science_nin_subclasses.go), this
// responds via companionRespondFragment rather than a hardcoded
// "sheet_science_nin", since the picker UI these routes serve lives on the
// Companions tab / companion popup, not the Science-Nin box.
func (s *server) handleTitanPickDelete(category charstore.ScienceNinSubclassPickCategory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove titan pick:", err)
			return
		}
		s.respondSheet(w, r, id, companionRespondFragment(r))
	}
}

// setTitanSpecialistCraftingKeyword records (or changes) Specialist
// Crafting's own single-slot "Mech or Weapon" designation — a single-slot,
// freely re-picked choice, same "drop whatever's already set, then add the
// new one" shape Mixed Studies' own single-slot Inquiry pick uses
// (mergeMixedStudiesFeatures's own caller, handleScienceNinSubclassPickAdd,
// achieves the same "cap 1" effect by gating on Used>=Cap instead — this
// route can't reuse that factory since it isn't keyed off
// scienceNinToolsTabData, so the drop-then-add is done explicitly here).
// Same success/failure return shape as addTitanSlotPick, shared the same
// way between handleTitanSpecialistCraftingKeywordSet (below) and the
// Titan Slots popup's handleTitanSlotsSpecialistCraftingSet
// (titan_slots_popup.go).
func (s *server) setTitanSpecialistCraftingKeyword(id int64, keyword string) (int, string) {
	if keyword != "mech" && keyword != "weapon" {
		return http.StatusBadRequest, "keyword must be mech or weapon"
	}
	existing, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickTitanSpecialistCraftingKeyword)
	if err != nil {
		log.Println("load titan specialist crafting keyword:", err)
		return http.StatusInternalServerError, "database error"
	}
	for _, p := range existing {
		if p.OptionSlug == keyword {
			continue
		}
		if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickTitanSpecialistCraftingKeyword, p.OptionSlug); err != nil {
			log.Println("clear titan specialist crafting keyword:", err)
			return http.StatusInternalServerError, "database error"
		}
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickTitanSpecialistCraftingKeyword, keyword, ""); err != nil {
		log.Println("set titan specialist crafting keyword:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleTitanSpecialistCraftingKeywordSet is the Companions-tab's own AJAX
// route for setTitanSpecialistCraftingKeyword (above).
func (s *server) handleTitanSpecialistCraftingKeywordSet(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	if status, msg := s.setTitanSpecialistCraftingKeyword(id, keyword); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// handleTitanUpgradeDetail serves the click-to-open popup for a Known Titan
// Upgrade pick — same hunter_pick_detail_card mechanism
// handleScienceNinToolDetail already uses. Not character-scoped — the
// catalog is static rules content.
func (s *server) handleTitanUpgradeDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	catalog, err := s.loadTitanUpgradeCatalog()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load titan upgrade catalog for detail:", err)
		return
	}
	for _, o := range catalog {
		if o.Slug != slug {
			continue
		}
		tmpl, ok := pageTemplates["character_sheet.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render titan upgrade detail: no template registered")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": o.Name, "Description": o.Description}); err != nil {
			log.Println("render titan upgrade detail:", err)
		}
		return
	}
	http.NotFound(w, r)
}
