package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// customResourceGrants is the curated table backing the sheet's "Special
// Resources" box — the second spendable point pool several classes/clans
// layer on top of the shared HP/Chakra pair (Science-Nin's Chakra
// Containment Device, Hatake's White Chakra, Hoshi's Star Chakra, and a
// handful more found while sweeping the book for this feature: Akimichi's
// Calories, Uzumaki's Reserve Cells, Scout-Nin's Chakra Barrier, and
// Hatake's feat-derived Purple Lightning, plus Hunter-Nin's Hunters
// Exploits "uses" pool, which introduced Proficiency Bonus to Max's
// signature, Cooking-Nin's Shinobi Snacks/Sugar Rush/Perfected Formula,
// which introduced Intelligence and Charisma modifiers, and Medical-Nin,
// the most resource-dense class found so far — Chakra Scalpel Charges and
// Preserve/Take Life base-class-wide, plus five more subclass-specific
// pools. Scout-Nin adds its own base-class-wide Superiority Dice, plus four
// more subclass-specific pools: Barrier Scout's Chakra Sphere/Repelling
// Burst uses, Phantom Scout's Ghastly Leech uses, and Trickster Scout's
// Willpower Surge and Tricksters Words uses).
//
// Same "curated table keyed by rules-database feature slug" shape
// passiveTraitGrants (passive_traits.go) already established, for the same
// reason: a blind text scan over "pool"/"chakra"/"per rest" language would
// false-positive constantly (Martial Dice, Bloodline Points, and Chakra
// Barrier's own per-turn regen all use similar words but don't fit this
// tracker's rest-tier shape at all) — every entry below was read against
// the real book text instead.
//
// Deliberately NOT modeled here (see plan doc / memory for the full case
// for each): bloodline-latent variants of these same resources (no
// character-side "I took this latent" selection exists anywhere in the app
// yet), Martial Dice (resets every turn, not on a rest), Bloodline Points
// (a one-time allocation, not a spendable pool), CCD's Mending/Maiming
// split (one narrow subclass option), and Chakra Barrier's per-turn regen
// (a manual click on the resource's own edit box, same boundary
// Concentration Check's DC entry already uses).
type restRegen int

const (
	regenNone      restRegen = iota // no automatic gain; player adjusts the resource box by hand
	regenConMod                     // gains CON modifier (min 1), capped at max — Star Chakra, Calories
	regenHalfSpent                  // regains half of whatever was spent (floor), capped at max — White Chakra
	regenHalfMax                    // resets to half of Max outright — CCD's "charges to half-full"
	regenFull                       // resets straight to max
)

// customResourceGrant is one hand-verified custom resource a class
// feature, subclass feature, clan feature, or feat grants — keyed by the
// rules-database slug of the feature that grants it, exactly like
// passiveTraitGrant.
type customResourceGrant struct {
	// Key groups multiple granting slugs into one resource (e.g. the base
	// Hatake clan feature and the White Chakra Surge feat both feed
	// "white_chakra") and is the column stored in
	// character_custom_resources.resource_key.
	Key  string
	Name string
	// MinLevel is an independent character-level gate beyond the
	// feature's own row-level, same purpose as passiveTraitGrant.MinLevel.
	MinLevel int
	// Max computes the resource's maximum from the character's per-class
	// level map (keyed by class slug, e.g. "class/science-nin" — CCD is
	// the one entry that needs one specific class's level rather than the
	// total, which is why this takes the whole map instead of a single
	// int), CON/Intelligence/Charisma modifiers, and Proficiency Bonus
	// (Hunters Exploits' own "uses" pool — see
	// class/hunter-nin/feature/hunters-exploits — is the first entry that
	// needed Proficiency Bonus; Cooking-Nin's Shinobi Snacks needs
	// Intelligence, and its Sugar Rush/Perfected Formula need Charisma —
	// every grant ignores whichever parameters it doesn't need). When more
	// than one grant shares a Key, the higher computed Max wins (White
	// Chakra Surge stacks onto the base Hatake grant this way).
	Max func(classLevels map[string]int, conMod, intMod, chaMod, profBonus int) int
	// ShortRegen/LongRegen/FullRegen: how much the resource recovers on
	// each rest tier. FullRegen is always >= LongRegen (Full Rest is a
	// superset of Long Rest, confirmed against the book).
	ShortRegen  restRegen
	LongRegen   restRegen
	FullRegen   restRegen
	Restriction string // informational only, shown on the sheet
	// DieSize is an optional readout of what die the pool's own count
	// spends (e.g. Scout-Nin's Superiority Dice are a per-subclass die
	// size, escalating with level for Assault Scout only — see
	// scoutNinSuperiorityDieSize in scout_nin.go). nil for every resource
	// that's a plain count with no associated die, which is most of them.
	DieSize func(classLevels map[string]int) string
}

func characterLevel(classLevels map[string]int) int {
	total := 0
	for _, v := range classLevels {
		total += v
	}
	return total
}

var customResourceGrants = map[string]customResourceGrant{
	"clan/hatake/feature/white-chakra": {
		Key:  "white_chakra",
		Name: "White Chakra",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 5 + characterLevel(cl)
		},
		ShortRegen:  regenHalfSpent,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Lightning Release jutsu only",
	},
	// Raises the White Chakra max further (+character level, +1/level
	// after) — additive onto the same "white_chakra" key via the
	// higher-Max-wins combination rule.
	"feat/hatake/white-chakra-surge": {
		Key:  "white_chakra",
		Name: "White Chakra",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 5 + 2*characterLevel(cl)
		},
		ShortRegen:  regenHalfSpent,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Lightning Release jutsu only",
	},
	// A derived pool, converted from White Chakra by hand at the end of
	// any rest (2 White Chakra -> 1 Purple Lightning, up to 10/rest) — no
	// automatic regen of its own; the player moves points into it via the
	// resource's own edit box.
	"feat/hatake/purple-lightning": {
		Key:  "purple_lightning",
		Name: "Purple Lightning",
		Max: func(map[string]int, int, int, int, int) int {
			return 10
		},
		ShortRegen: regenNone,
		LongRegen:  regenNone,
		FullRegen:  regenNone,
	},
	"clan/hoshi/feature/star-chakra": {
		Key:  "star_chakra",
		Name: "Star Chakra",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if con < 1 {
				con = 1
			}
			return con + characterLevel(cl)
		},
		ShortRegen: regenConMod,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Also at 3rd level, as a bonus action you can activate this unique
	// chakra mode twice per long rest... You can activate this mode an
	// additional time per long rest at 11th and 18th levels." Same
	// "pool here, effect there" precedent as Jutsu Breaker
	// (class/ninjutsu-specialist/feature/jutsu-breaker): the mode's own
	// stacked effects while active (temp HP, Chakra Control bonus die,
	// +10ft object-interaction reach, ignore HS component on Hoshi
	// Hijutsu, AC+1, reaction damage reduction) aren't automated — no
	// stance/temporary-mode system exists anywhere on the sheet; this
	// pool only tracks the activation count. Unlike Jutsu Breaker, the
	// book text ties recovery specifically to a long rest, not any rest,
	// so ShortRegen is regenNone here.
	"clan/hoshi/feature/kujaku-mode": {
		Key:      "kujaku_mode_uses",
		Name:     "Kujaku Mode",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			n := 2
			if lvl >= 11 {
				n++
			}
			if lvl >= 18 {
				n++
			}
			return n
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/akimichi/feature/calories": {
		Key:  "calories",
		Name: "Calories",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if con < 1 {
				con = 1
			}
			return characterLevel(cl) + con
		},
		ShortRegen: regenConMod,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/uzumaki/feature/chakra-reserves": {
		Key:  "reserve_cells",
		Name: "Reserve Cells",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return characterLevel(cl)
		},
		// The book only ever says "per long rest" — no short rest gain.
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/bakuton/feature/concussive-blasts": {
		Key:  "shrapnel_dice",
		Name: "Shrapnel Dice",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			if characterLevel(cl) >= 11 {
				return "d8"
			}
			return "d6"
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bakuton clan Hijutsu damage rolls only",
	},
	// "For the duration of your Ketsuryūgan, you can take the following
	// actions; You can use these clan features a number of times equal to
	// your proficiency bonus per long rest." Tracks only this shared
	// use-count pool for the three sub-actions below -- activating the
	// Ketsuryūgan itself (bonus action, 5 chakra, 10 minutes) is a
	// separate cost not tracked here.
	"clan/chinoike/feature/ketsuryugan": {
		Key:      "ketsuryugan_uses",
		Name:     "Ketsuryūgan",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: supercharge blood circulation (+1 AC/damage and +15 speed to allies within 15 feet); Bonus Action: convert a Water Release jutsu's excess chakra into temporary hit points and necrotic drain; Reaction: grant an ally your Wisdom or Intelligence modifier against a failed Mental save — each costs one use while your Ketsuryūgan is active",
	},
	// "Beginning at 3rd level, twice per short rest, when you would miss a
	// ranged weapon, or taijutsu attack that uses a weapon with the Thrown
	// Property, you can remake the attack roll, taking the second result."
	// A flat 2-charge pool reset on every rest tier, same "per Short Rest"
	// shape as The Right Tool (ShortRegen/LongRegen/FullRegen all
	// regenFull, since a Long or Full Rest is a superset of a Short Rest).
	"clan/fuma/feature/razor-sharp-senses": {
		Key:      "razor_sharp_senses",
		Name:     "Razor Sharp Senses",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reroll a missed ranged weapon or Thrown-property taijutsu attack, taking the second result",
	},
	"clan/futton/feature/boiling-chakra": {
		Key:  "boil_points",
		Name: "Boil Points",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Starting at 11th level, when you affect a creature with a Genwa
	// Hijutsu, you can perform a Data Leak on the creature... You can perform
	// a Data Leak twice per long rest. At 18th level, this feature resets on
	// a short rest." The 18th-level upgrade to short-rest recovery can't be
	// expressed by this table's single ShortRegen/LongRegen/FullRegen-per-Key
	// shape (Max stays 2 at both tiers, so a second same-Key grant could never
	// win the "higher Max wins" merge rule) — this entry models the 11th-level
	// baseline (long-rest-only recovery) and notes the 18th-level change in
	// Restriction only, same as Calculated Response's 15th-level upgrade note.
	"clan/genwa/feature/data-release": {
		Key:      "data_leak",
		Name:     "Data Leak",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "When you affect a creature with a Genwa Hijutsu, mine 2 of 6 pieces of info about it; from 18th level, resets on a short rest instead of only a long rest",
	},
	// "Once per rest, when you cast a Taijutsu, or take the Dash,
	// Disengage, or Dodge actions, you can cast a Supportive Medical Jutsu
	// with the casting time of one Action, as a bonus action. You gain
	// additional uses of this feature at 7th and 15th levels." The book
	// never names a rest tier ("per rest" alone) — same unscoped phrasing
	// Genjutsu Specialist's own Chakra Disruption already resolves by
	// regenerating fully on every tier, applied identically here rather
	// than guessing at one tier.
	"clan/hanami/feature/combat-medicine": {
		Key:      "combat_medicine",
		Name:     "Combat Medicine",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 15:
				return 3
			case lvl >= 7:
				return 2
			default:
				return 1
			}
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus action, after casting a Taijutsu or taking the Dash, Disengage, or Dodge action; casts a Supportive Medical Jutsu with a casting time of one Action",
	},
	// "Twice per long rest, when you would use a jutsu with the Medical
	// keyword that is Supportive, you may remove all ranks of any one
	// condition affecting the creature as if you used the Restorative
	// Ninjutsu, cast at your highest known jutsu rank. If you cast a jutsu
	// with the Medical keyword that removes conditions, you may spend a use
	// of this feature to leave a remnant of your chakra within the creature,
	// making them immune to gaining one condition removed again, from a
	// hostile creature, for the next minute. You gain an additional use of
	// this feature at 11th and 18th levels." (2 -> 3@11th -> 4@18th.)
	"clan/hanami/feature/empowered-healing": {
		Key:      "empowered_healing",
		Name:     "Empowered Healing",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 18:
				return 4
			case lvl >= 11:
				return 3
			default:
				return 2
			}
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Remove all ranks of one condition (as Restorative Ninjutsu) when casting a Supportive Medical jutsu, or spend a use to grant 1-minute immunity to a condition just removed from a hostile creature",
	},
	// Hanami Boons: "Beginning at 7th level, when you cast a jutsu with the
	// Medical keyword, you can inflict one of the following Hanami Boons... You
	// can provide one Hanami Boon per round, to all affected creatures. You may
	// use this feature a number of times equal to half your Proficiency Bonus,
	// rounded up, per long rest." Ceiling division, unlike this file's usual
	// floored "half Proficiency Bonus" convention (see Preserve/Take Life
	// above) — same (x+1)/2 ceiling-halving shape as
	// internal/puppetupgrades/foundation.go's abilityCeilThird, which does the
	// analogous ceiling-third. The book only ever says "per long rest" — no
	// short rest gain.
	"clan/hanami/feature/hanami-boons": {
		Key:      "hanami_boons",
		Name:     "Hanami Boons",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return (prof + 1) / 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Hebi's Regeneration: "Beginning at 1st level you can as a bonus action
	// begin to focus on your accelerated regeneration, regaining 1d8+Half your
	// level in hit points... You can activate this feature twice per long
	// rest, regaining one use every short rest... At 7th level... You also
	// gain an additional use of your Regeneration per long rest" (Max becomes
	// 3). The healing roll itself (1d8+half level, scaling to 2d8 at 7th and
	// 3d8 at 15th), the free-action Chakra cost to maintain it on later turns,
	// and the 15th-level condition-removal option aren't automated — this pool
	// only tracks the activation use-count, same "pool here, effect there"
	// split Jutsu Breaker already established. No existing restRegen kind
	// expresses a flat "+1 per short rest, capped at Max" recovery exactly;
	// regenHalfSpent is the closest fit (exact when an even number of uses
	// were spent, undercounts by one when an odd number were spent — see
	// White Chakra's own "regain half of your spent" text for what this kind
	// is actually tuned for).
	"clan/hebi/feature/regeneration": {
		Key:  "regeneration_uses",
		Name: "Regeneration",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if characterLevel(cl) >= 7 {
				return 3
			}
			return 2
		},
		ShortRegen:  regenHalfSpent,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus action to activate; 4 Chakra as a free action each turn to maintain (no HP regen at 0 HP before 7th level)",
	},
	// "You can coat a weapon in poison this way twice per rest." Unscoped
	// "per rest" regenerates on every rest tier. The Vipers Venom coating
	// effect itself (damage type swap to Poison, bonus +1d4/+1d6/+1d8 damage
	// twice per turn, the 7th-level halved-healing clause) isn't automated —
	// this pool only tracks the use-count, same "pool here, effect there"
	// split Jutsu Breaker already established.
	"clan/hebi/feature/poison-potency": {
		Key:      "poison_potency_uses",
		Name:     "Poison Potency",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning at 3rd level, you store extra Water in your body, giving you
	// an amount of uses equals to your proficiency bonus per long rest. You
	// can use these reservoirs to empower a ninjutsu with the Water Release
	// Keyword, either increases its damage by 1 damage die or save DC by +1.
	// At 11th level, increase the damage die or save DC by 2 and at 18th
	// level, increase the damage die or save DC by 3." Only the uses-count
	// pool is tracked here; the escalating empower amount (+1/+2/+3 damage die
	// or save DC at 3rd/11th/18th) has no numeric-effect resolution mechanism
	// in this app, the same boundary Ghastly Leech/Willpower Surge already
	// draw around their own scaling empower effects. No MinLevel needed — the
	// clan_features row's own level (3) already gates this via
	// loadGrantedFeatures, same precedent class/hunter-nin/feature/hunters-
	// exploits (also Max = prof) already establishes.
	"clan/hozuki/feature/water-reservoirs": {
		Key:  "water_reservoirs",
		Name: "Water Reservoirs",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		// The book only ever says "per long rest" — no short rest gain.
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/iburi/feature/will-o-wisps": {
		Key:  "will_o_wisps",
		Name: "Will-O-Wisps",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if characterLevel(cl) >= 15 {
				return prof
			}
			return (prof + 1) / 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Fire Release jutsu attack rolls only — spreads half the target's damage (Dex save negates) to creatures within 5 ft (10 ft at 11th level) of the target",
	},
	// Beginning at 15th level, twice per long rest, when you deal Fire
	// damage, you automatically blind all affected creatures until the end
	// of each of their turns. The feature's own row-level is 7th (its
	// earlier "-2 penalty to a saving throw against the jutsu or Will-O-
	// Wisps" clause at that level is a conditional in-combat choice, not a
	// numeric pool, and is deliberately not modeled here). This later clause
	// needs its own MinLevel gate, the same shape Ashen Resilience already
	// establishes for this same clan (see passive_traits.go's MinLevel: 7 /
	// MinLevel: 11 on that single level-1 row). The pool exists whether or
	// not the app also models the automatic-blind trigger itself, same
	// boundary this table's other automatic-effect pools already draw
	// (Ghastly Leech, Tricksters Words, above).
	"clan/iburi/feature/smoke-release": {
		Key:      "iburi_smoke_release_blind",
		Name:     "Smoke Release (Blind)",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Automatically blinds creatures you deal Fire damage to, until the end of each of their turns",
	},
	// "Starting at 1st level, you gain an amount of Raw Chakra Die, equal to
	// your proficiency bonus. Raw Chakra die are d4's... Beginning at 7th and
	// 11th levels the size of your Raw Chakra dice increase to a d6 and d8,
	// respectively. You regain half your spent Raw Chakra die on a short
	// rest, and all on a long rest." The four spend effects (Amplify, Absorb,
	// Reduce, Heal) are conditional, activated combat text with no sheet
	// field of their own and stay manual — only the die pool/size is
	// tracked, the same boundary every other DieSize entry in this table
	// draws (e.g. Hatake's White Chakra below).
	"clan/jugo/feature/raw-chakra": {
		Key:      "raw_chakra",
		Name:     "Raw Chakra",
		MinLevel: 1,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			switch {
			case characterLevel(cl) >= 11:
				return "d8"
			case characterLevel(cl) >= 7:
				return "d6"
			default:
				return "d4"
			}
		},
		ShortRegen: regenHalfSpent,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Starting at 3rd level... You may enter this form as an Action, twice
	// per long rest, for up to 1 minute. You can transform an additional
	// time per long rest at 11th and 18th levels." Only the transformation-
	// use count is tracked here — the while-transformed bonuses themselves
	// (Strength check/save die, temp HP formula, unarmed damage die step-up,
	// speed increase tiers, 11th-level initiative-roll transform, 18th-level
	// movement/condition immunity) are not computed, same boundary
	// Unmatched Botanist/Eye of the Storm already draw for their own
	// bundled effects.
	"clan/jugo/feature/raw-chakra-form-changing": {
		Key:      "raw_chakra_form",
		Name:     "Raw Chakra Form",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			n := 2
			if characterLevel(cl) >= 11 {
				n++
			}
			if characterLevel(cl) >= 18 {
				n++
			}
			return n
		},
		// The book only ever says "per long rest" — no short rest gain,
		// same "Unscoped 'per Rest'" reasoning as Reserve Cells.
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Enter Raw Chakra Form as an action for up to 1 minute",
	},
	// "Beginning at 11th level, you have learned to tap into the lust for
	// battle, blacking out your thoughts, entering a state of known to the
	// Kaguya as your Lust for Battle. You remain in this state for 1 minute,
	// until you are incapacitated, knocked unconscious, are calmed down in
	// some other way or spend a bonus action to end this status. Once you use
	// this feature, you cannot do so again until you finish a rest." Embedded
	// as the "Battle Hunger" sub-feature inside the Shikotsumyaku Stance clan
	// feature row (which itself gates at 1st level) — hence the MinLevel
	// override, same purpose as Chakra Barrier's. Unscoped "a rest" treated as
	// regenFull on every tier, same precedent Unmatched Botanist's own
	// unscoped "per Rest" phrasing already establishes. The book's own
	// description of what the Lust for Battle status mechanically does is
	// missing/garbled in the source text itself ("entering a state of known
	// to..."), so only the trigger/duration/once-per-rest gate is tracked;
	// the effect stays informational, same boundary Eye of the Storm/
	// Unmatched Botanist already use.
	"clan/kaguya/feature/shikotsumyaku-stance": {
		Key:      "battle_hunger",
		Name:     "Battle Hunger",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Enter your Lust for Battle for 1 minute, until incapacitated, unconscious, calmed down, or ended early as a bonus action",
	},
	// "Additionally, twice per long rest, when a creature fails a saving
	// throw against a Genjutsu you cast, you can give them 1 rank of
	// Disorientation." Only the twice-per-long-rest use count is tracked —
	// the Disorientation condition itself (ranks, penalties, DC 15/20 removal
	// save, increasing to DC 20 at 15th level) has no target/NPC-state field
	// anywhere in the app to hold it.
	"clan/kashu/feature/disorienting-chords": {
		Key:      "disorienting_chords",
		Name:     "Disorienting Chords",
		MinLevel: 1,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Give a creature that fails a save against your Genjutsu 1 rank of Disorientation",
	},
	// "Potent Genjutsu: Beginning at 18th level, as long as you are equipped
	// with your Auditory Ninja Tool, when you would inflict damage with a
	// genjutsu with the Auditory keyword you can instead as a bonus action,
	// deal maximum damage. This can be done twice per long rest." Embedded as
	// the closing clause of the 11th-level Reckless Genjutsu row's own
	// description — MinLevel gates the later clause the same way
	// clan/hebi/feature/poisonous-diet, clan/yuki/feature/chilled-body, and
	// clan/namikaze/feature/supernatural-speed already do for their own
	// higher-level clauses embedded in a lower-level row. Only "per Long Rest"
	// is stated — no short-rest regen.
	"clan/kashu/feature/reckless-genjutsu": {
		Key:      "potent_genjutsu",
		Name:     "Potent Genjutsu",
		MinLevel: 18,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "As a bonus action, deal maximum damage with an Auditory genjutsu instead of rolling, while wielding your Auditory Ninja Tool",
	},
	"clan/keton/feature/enlightening-grasp": {
		Key:  "enlightening_grasp",
		Name: "Enlightening Grasp",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend 5 chakra (no action) for +half proficiency bonus additional uses",
	},
	// "Each time you would take damage that is not self-inflicted or cast a
	// jutsu with the Fire or Lightning Release keywords, you can gain an Energy
	// Die at the start of your next turn, which are d6s. Your Energy Die
	// becomes d8s at 11th level and d10s at 18th level. You can hold a maximum
	// amount of Energy Die equal to half your proficiency bonus, rounded up.
	// Whenever you cast a jutsu with the Fire or Lightning Release keywords,
	// you can spend any number of Energy Die, adding it to the damage roll."
	// Gained by trigger, not by rest — same regenNone, manual-edit-box pattern
	// already used for feat/hatake/purple-lightning.
	"clan/keton/feature/energy-overflow": {
		Key:      "energy_die",
		Name:     "Energy Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return (prof + 1) / 2
		},
		DieSize: func(cl map[string]int) string {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 18:
				return "d10"
			case lvl >= 11:
				return "d8"
			default:
				return "d6"
			}
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenNone,
		Restriction: "Gained when you take non-self-inflicted damage or cast a Fire/Lightning Release jutsu; spend on Fire/Lightning jutsu damage rolls",
	},
	"clan/kurama/feature/onijutsu": {
		Key:      "onijutsu_coils",
		Name:     "Onijutsu Coils",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return characterLevel(cl)
		},
		// The book only ever says "per Long Rest" — no short rest gain.
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend 1 Coil or 5 Chakra to use a known Onijutsu when casting a Genjutsu (up to two Onijutsu per casting)",
	},
	// "You can use the following Kuru Clan Features a number of times equal to
	// your proficiency bonus, per long rest." Fuels the three effects gated
	// behind an active Kurugan (Action: rally allies' AC/next save, or from 7th
	// level their next attack roll; Bonus Action: add your Wisdom modifier to
	// your next attack roll; Reaction: advantage on saving throws, or from 7th
	// level +5 AC, or from 18th level negate a lethal hit). Book only says "per
	// long rest" -- no short-rest regen, same "per long rest" precedent
	// clan/uzumaki/feature/chakra-reserves already establishes.
	"clan/kuru/feature/kurugan": {
		Key:      "kurugan_uses",
		Name:     "Kurugan Uses",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action (rally allies' AC/save, or attack from 7th), Bonus Action (add Wisdom mod to next attack), or Reaction (advantage on saves, or +5 AC from 7th) — requires an active Kurugan",
	},
	// Speed Die — the spendable d8 pool Supernatural Speed grants
	// beginning at 3rd level, layered on top of the same feature's flat
	// Speed increases (clan/namikaze/feature/supernatural-speed already
	// feeds internal/features/grants.go's speedGrants for those — this is
	// an independent curated table keyed by the same rules-database slug,
	// since one feature can have more than one distinct numeric effect).
	// "You have a number of speed die equal to your proficiency bonus...
	// You regain your speed die on a long rest" is the same count+D8+
	// long-rest-only-regen shape as Scout-Nin's Superiority Dice, so
	// MinLevel gates this to 3rd even though the flat Speed bonus itself
	// starts at 1st. Only the pool is tracked here, not each individual
	// effect it fuels (Speed Dodge, Speed Amplification, Quickened
	// Assault, Blink of an Eye, and — from 11th level — Swift Focus, Like
	// the Wind, Flash of Defense), the same "pool only, not each
	// maneuver" boundary Superiority Dice's own Maneuvers already draw.
	"clan/namikaze/feature/supernatural-speed": {
		Key:      "speed_die",
		Name:     "Speed Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			return "d8"
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend up to 3/turn on Speed Dodge (+2 AC per die as a reaction), Speed Amplification (+5ft speed x roll), Quickened Assault (extra 1d8 weapon attacks), or Blink of an Eye (teleport within your speed, +1d8 to a melee attack against an adjacent target); from 11th level, also Swift Focus, Like the Wind, and Flash of Defense",
	},
	// "Beginning at 15th level, when you would be forced to make a
	// Dexterity saving throw to take half damage, you can choose to succeed
	// instead, taking no damage or any adverse effects. You can only use
	// this effect of Evasive Nature twice per long rest." The +1/+2
	// Dexterity saving-throw bonus (1st/11th level, while wearing Light or
	// Medium armor) and the concentration-triggered disadvantage clause
	// earlier in the same feature text aren't modeled — no existing
	// mechanism in this app tracks a flat numeric bonus to a specific
	// saving throw. Only this fixed-count "succeed instead of failing"
	// pool is tracked here; the save-override itself isn't automated, same
	// as Jutsu Breaker's clash payload above.
	"clan/namikaze/feature/evasive-nature": {
		Key:      "evasive_nature_uses",
		Name:     "Evasive Nature Uses",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Succeed instead of failing a Dexterity saving throw to take half damage, taking no damage or adverse effects",
	},
	// "Beginning at 1st level, when you would roll initiative, you can spend
	// your Reaction coordinating your allies... once you use this feature
	// twice, you cannot do so again until you complete a Long rest. This
	// increases to three times at 11th level and four times at 18th level."
	"clan/nara/feature/coordinate": {
		Key:  "coordinate_uses",
		Name: "Coordinate Uses",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			n := 2
			if lvl >= 11 {
				n++
			}
			if lvl >= 18 {
				n++
			}
			return n
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning, at 3rd level, you can provide a Tactical Die (D4), to any
	// ally that fails an attack roll, saving throw or skill/ability check...
	// You can use this clan feature a number of times equal to your
	// proficiency bonus per long rest. At 11th Level, the die becomes a d6
	// and at 18th level a d8." The book only ever says "per long rest" — no
	// short rest gain, same idiom as clan/uzumaki/feature/chakra-reserves.
	"clan/nara/feature/master-tactician": {
		Key:  "tactical_die",
		Name: "Tactical Die",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			lvl := characterLevel(cl)
			if lvl >= 18 {
				return "d8"
			}
			if lvl >= 11 {
				return "d6"
			}
			return "d4"
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "At 7th level When you would make a Dexterity, Wisdom, or Charisma
	// saving throw to resist a hostile creatures feature, trait or jutsu, you
	// may add your Intelligence Modifier to the result. You may use this
	// feature a twice per long rest. You gain an additional use of this
	// feature at 15th level." The Intelligence-modifier-to-save payload itself
	// isn't automated — this pool only tracks the use-count, same "pool here,
	// effect manual" boundary Jutsu Breaker/Calculated Response already use.
	"clan/nara/feature/genius-potential": {
		Key:  "genius_potential",
		Name: "Genius Potential",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			n := 2
			if lvl >= 15 {
				n++
			}
			return n
		},
		// The book only ever says "per long rest" — no short rest gain, same
		// reasoning Reserve Cells' own comment documents.
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Add your Intelligence modifier to a Dexterity, Wisdom, or Charisma saving throw against a hostile creature's feature, trait, or jutsu",
	},
	// "You gain a number of Focus points equal to your proficiency bonus per
	// long rest" — the book only ever says "per long rest", no short rest
	// gain, same shape as Uzumaki's Reserve Cells above.
	"clan/sarutobi/feature/inheritors-of-the-will-of-fire": {
		Key:  "focus_points",
		Name: "Focus Points",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "1 FP: +2 on an Attack Roll, Saving Throw, or Skill Check; 4 FP: gain a Reaction usable for an Action-only ability or jutsu, once per round; 2 FP: automatically succeed a Death Saving Throw",
	},
	"clan/senju/feature/mitotic-regeneration": {
		Key:  "senju_cells",
		Name: "Senju Cells",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return characterLevel(cl)
		},
		// The book only ever says "per long rest" — no short rest gain.
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning at 3rd level... You gain [Scorch Die], which are D4's,
	// equal to your proficiency bonus... You regain spent scorch die when
	// you would take a rest of any type." Scorch die are spent to add to
	// Fire damage (ignoring resistances) or to reduce a creature's save vs
	// an effect from a Fire/Wind Release jutsu by half the dice rolled —
	// that spend choice is Group 2 (conditional/activated); only the pool
	// itself (count, die size, regen) is tracked here, same boundary White
	// Chakra's own Restriction field already draws. Distinct from the
	// separate "Latent Scorching Heat I/II/III" bloodline-latent variant of
	// this same resource (bloodline_latents table, not clan_features) —
	// deliberately not modeled, see this file's header comment on
	// bloodline-latent exclusions.
	"clan/shakuton/feature/scorching-heat": {
		Key:      "scorch_die",
		Name:     "Scorch Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			return "d4"
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Fire damage, or saves vs. Fire/Wind Release jutsu",
	},
	"clan/shi-hou/feature/monkeys-paw": {
		Key:  "monkeys_paw",
		Name: "Monkey's Paw",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 11:
				return "1d8"
			case lvl >= 7:
				return "1d6"
			default:
				return "1d4"
			}
		},
		// The book only says "per long rest" — no short rest gain.
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning at 3rd level, you gain 10 Inner Chi. You gain 1 additional
	// Inner Chi at each level." -- 10 at 3rd level, +1 per level thereafter,
	// i.e. characterLevel+7. MinLevel already gates Max to 3rd level and up
	// (computeCustomResources skips the grant entirely below MinLevel), so no
	// internal level check is needed here, matching every other MinLevel-gated
	// entry in this table (e.g. white-chakra's bare "5 + characterLevel(cl)").
	// "When you would gain the benefit of a short rest, you regain Inner Chi
	// equal to half your maximum" is an ADDITIVE gain (current + max/2, capped
	// at max) -- regenHalfMax is the closest existing kind, but it's a hard
	// floor-to-half (raises current up to max/2, no-ops if already above it)
	// rather than an additive gain, so it under-regens whenever the pool is
	// already above half full (e.g. current=8/max=10 stays at 8 instead of
	// capping at 10). Closest available approximation until a dedicated
	// additive-half-max restRegen kind exists; revisit if one gets added.
	"clan/shi-hou/feature/inner-chi": {
		Key:      "inner_chi",
		Name:     "Inner Chi",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return characterLevel(cl) + 7
		},
		ShortRegen:  regenHalfMax,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Used in place of Chakra to cast Taijutsu; spend at least the jutsu's rank in Inner Chi for +1 damage die or +1 to hit (once per casting); also fuels 72 Earthly Transformations (1/2/4 additional Inner Chi for Simple/Advanced/Mythic Transformations)",
	},
	// "Starting at 7th level ... represented as a pool of d10's called
	// Crystallization Die. You begin with 4 of these Crystallization Die.
	// You gain more as you increase in level, gaining 2 more when you
	// reach 15th level. You regain all expended uses of this die when you
	// complete a short or long rest." The four spend criteria (Earth
	// Ninjutsu AC bonus, added damage, added Quake Shard HP, added Damage
	// Reduction on an intercepting jutsu) aren't automated — same "pool
	// tracked here, spend effects manual" split every other multi-use
	// resource in this table already follows.
	"clan/shoton/feature/crystalized-focus": {
		Key:      "crystallization_die",
		Name:     "Crystallization Die",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			n := 4
			if characterLevel(cl) >= 15 {
				n += 2
			}
			return n
		},
		DieSize: func(cl map[string]int) string {
			return "1d10"
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Earth Release Ninjutsu only",
	},
	// "You may enter this form as an Action, twice per long rest, for up to
	// 1 minute. At 11th level you may now transform as a bonus action. You
	// can transform an additional time per long rest at 18th levels."
	// Standalone activation-gate pool (2/long rest, +1 at 18th), same shape
	// as War Cry/In Perfect Sync. The transformation's own temporary
	// Chakra/HP, tremorsense, weapon-die, and speed effects remain an
	// untracked Group 2 buff (no field for them anywhere on the sheet).
	"clan/synthetic-human/feature/corrupted-chakra-mode": {
		Key:  "corrupted_chakra_mode",
		Name: "Corrupted Chakra Mode",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			n := 2
			if characterLevel(cl) >= 18 {
				n++
			}
			return n
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action (Bonus Action at 11th level) to enter, lasts up to 1 minute",
	},
	// "Once per rest, as a Bonus Action, you open your third eye, increasing
	// your perceptive abilities by a wide margin for the next 10 minutes."
	// Unscoped "per rest" — regenerates on every tier, same precedent Chakra
	// Barrier's own "whenever you complete a rest of any type" phrasing
	// already established (see Perfected Formula/Before All above for the
	// identical treatment). Only the once-per-rest activation gate is tracked
	// here; the three while-active benefits (+1 Mastery to Search checks, the
	// Spider Search action, and the 3rd/11th/18th-level scaling longbow/
	// shortbow range increase) have no mechanism modeled elsewhere in this app
	// and stay untracked, same "pool tracked here, effect applied by hand"
	// split Checkmate/Tsume/Before All above already follow.
	"clan/tsuchigumo/feature/third-eye": {
		Key:  "third_eye",
		Name: "Third Eye",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action to open for 10 minutes; grants +1 Mastery to Search checks, the Spider Search action, and (from 3rd level) increased longbow/shortbow range while active — only the activation gate is tracked",
	},
	// "Activating your Sharingan costs a bonus action and remains active
	// for 10 minutes. You may only use abilities gained from this clan
	// feature while it is active. You can use these abilities three times
	// per Short Rest. You gain an additional use per Short Rest at 7th and
	// 11th levels. You may spend 10 Chakra while your Sharingan is active
	// to reset the number of uses you have as if you took a short rest."
	// Tracks only the activation-use count. The Tomoe-tier ability picks
	// (Eye of Insight, Sharingan Adaptation, Sharingan Dodge, Copy Wheel,
	// Genjutsu Counter, Hypnotic Eye, Aggressive Foresight, Aggressive
	// Jutsu Augmentation, etc.) have no field anywhere in the app. The
	// 10-Chakra manual reset is not modeled as an automatic regen tier —
	// it reuses the character's existing Chakra pool by hand, the same
	// boundary other manual cross-pool trades in this file already draw
	// (see Purple Lightning above).
	"clan/uchiha/feature/sharingan": {
		Key:  "sharingan",
		Name: "Sharingan",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			n := 3
			if lvl >= 7 {
				n++
			}
			if lvl >= 11 {
				n++
			}
			return n
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action to activate, lasts 10 minutes",
	},
	// "One per rest, you can cast a jutsu you know with the Fuinjutsu
	// keyword that has a casting time of 1 action, as a bonus action. This
	// increases to twice per rest at 11th level and three times per rest at
	// 18th level." A bare escalating usage counter, no die, no other computed
	// tie-in — same shape Performance Scroll and Checkmate already use for
	// "activate this feature N times per rest" gates. The book's "per rest"
	// here carries no long/short qualifier at all, so it follows Preserve/Take
	// Life's and Chakra Barrier's precedent for genuinely unscoped "per
	// rest"/"between rests" phrasing: regenFull on every tier — not the
	// long-rest-only reading Reserve Cells gets, since Reserve Cells' own text
	// explicitly says "per long rest" rather than leaving the tier unscoped.
	"clan/uzumaki/feature/fuinjutsu-master": {
		Key:  "fuinjutsu_bonus_casts",
		Name: "Fuinjutsu Bonus Casts",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			level := characterLevel(cl)
			switch {
			case level >= 18:
				return 3
			case level >= 11:
				return 2
			default:
				return 1
			}
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast a jutsu you know with the Fuinjutsu keyword and a casting time of 1 action as a Bonus Action instead",
	},
	// "Twice per long rest when a creature would fail a saving throw against a
	// Genjutsu you cast, you can automatically Charm the creature." Tracks only
	// this activation gate; the automatic Charm application itself has no
	// generic "apply a condition on a failed save" mechanism anywhere in this
	// app, same untracked-effect boundary Checkmate/Tsume draw around their own
	// triggered effects.
	"clan/vesper/feature/enthralling-strength": {
		Key:      "enthralling_strength_charms",
		Name:     "Enthralling Strength",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Automatically Charm a creature that fails a saving throw against your Genjutsu",
	},
	"clan/yamada/feature/balance-of-emotion": {
		Key:      "balance_die",
		Name:     "Balance Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			level := characterLevel(cl)
			return (level + 1) / 2
		},
		DieSize: func(cl map[string]int) string {
			return "d6"
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Katana/Odachi weapon or Bukijutsu attacks/saves — Balanced Edge (damage), Balanced Focus (reroll a natural 1), Balanced Precision (attack roll), or Balanced Insight (half to Save DC); once per turn (twice at 15th level)",
	},
	// "Beginning at 11th level as a bonus action, you can target one
	// creature you can see within 60 feet and make a Wisdom or Charisma
	// (Insight) check vs a DC (8 + their level) to learn either the
	// targets Wisdom or Charisma score. You can do this a number of times
	// equal to your proficiency bonus per long rest." The feature's own
	// row is tagged level 7 (its 7th-level saving-throw bonus/Insight
	// advantage passives, not modeled here) — same "DB row level marks
	// the block's first appearance, not every tier inside it" shape as
	// clan/iburi/feature/ashen-resilience (passive_traits.go).
	"clan/yamanaka/feature/mental-clarity": {
		Key:      "mental_clarity_insight",
		Name:     "Mental Clarity (Insight)",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus action: target one creature you can see within 60 feet and make a Wisdom or Charisma (Insight) check vs DC 8 + their level to learn either their Wisdom or Charisma score",
	},
	// "Twice per long rest, as a bonus action you may select 2 allies
	// (self-excluded)... At 15th level, you regain uses of this feature on
	// a short rest, and may initiate this technique when you score a hit
	// or when a target fails a saving throw against any type of jutsu you
	// cast or any type of attack you make." The level-15 upgrade has no
	// field to land in (ShortRegen/LongRegen/FullRegen are static per
	// entry, not level-gated — same boundary Calculated Response's own
	// level-15 clause already accepts), so it's documented in Restriction
	// only, not modeled mechanically.
	"clan/yamanaka/feature/formation-casting-new": {
		Key:      "formation_casting",
		Name:     "Formation Casting",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus action: select 2 allies (self-excluded) to work in tandem until the start of your next turn; when you hit with or force a failed save against a Genjutsu/Yamanaka Hijutsu, they gain advantage on their next Ninjutsu/Taijutsu attack against the target, or impose a -3 penalty on the target's next save against their own Ninjutsu/Taijutsu/Bukijutsu. From 15th level, regains uses on a Short Rest instead of a Long Rest (not modeled below) and triggers off any jutsu or attack, not just Genjutsu/Hijutsu.",
	},
	"clan/yoton/feature/lava-release": {
		Key:  "lava_release_enhancements",
		Name: "Lava Release Enhancements",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Once per casting, adds up to two qualities (Molten Rock, Acidic Mud, Corrosive Quicklime, Vulcanized Rubber, Malleable Onyx) to a Yoton Hijutsu or Ninjutsu with the Earth or Fire Release keyword",
	},
	"clan/yuki/feature/frigid-cold": {
		Key:  "frigid_cold_uses",
		Name: "Frigid Cold",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 15:
				return 4
			case lvl >= 7:
				return 3
			default:
				return 2
			}
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus action: freeze the ground under you into difficult terrain within a 15/30/45-ft radius; from 3rd level, spend a use when you deal cold damage to freeze a 15-ft sphere around the target (Constitution save vs your Ninjutsu save DC or gain 1 rank of Chilled)",
	},
	"class/scout-nin/group/scouting-technique/barrier-scout/feature/chakra-barrier": {
		Key:      "chakra_barrier",
		Name:     "Chakra Barrier",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2 * characterLevel(cl)
		},
		// "Whenever you complete a rest of any type" — recreated fresh on
		// every tier, including Short Rest.
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Superiority Dice — the base-class-wide spendable pool fueling every
	// Scout-Nin subclass's own Maneuvers (the Maneuvers Known cap+catalog
	// pick itself is a separate, later build — see scout_nin.go). One
	// entry per subclass, all sharing Key "superiority_dice" so only one
	// tile ever renders per character (a character has exactly one Scout-
	// Nin subclass). Each subclass's own 3rd-level "Superior <Name>"
	// feature states the identical shape ("You have three Superiority
	// Dice... A Superiority Die is expended when you use it. You regain
	// all of your expended Superiority Dice after 10 minutes.") — treated
	// as regenFull on every rest tier, same precedent Chakra Barrier's own
	// unscoped "whenever you complete a rest of any type" phrasing already
	// establishes for a recovery faster than this app's own Short Rest
	// tier. Cloning Scout is the one exception to the granting slug
	// pattern: its Superiority Dice text lives in "Cloning Tactics", NOT
	// "Superior Clones" (a different, later 9th-level feature about clone
	// stat-block upgrades) — confirmed by direct query of both features'
	// full description text, not assumed from the naming pattern the other
	// 8 subclasses follow. Dice-count progressions and base die sizes are
	// hand-transcribed from each subclass's own "Superior X Table" (see
	// scoutNinSuperiorityDice/scoutNinSuperiorityDieSize, scout_nin.go) —
	// class_level_resources has zero rows for class/scout-nin.
	"class/scout-nin/group/scouting-technique/arbiter-scout/feature/superior-arbitration": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("a", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("arbiter-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/assault-scout/feature/superior-assault": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("c", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("assault-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/barrier-scout/feature/superior-defense": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("b", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("barrier-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Cloning Scout's own Superiority Dice pool is granted by "Cloning
	// Tactics" (3rd level), not "Superior Clones" (9th level, an unrelated
	// clone-upgrade feature) — see this block's own header comment.
	"class/scout-nin/group/scouting-technique/cloning-scout/feature/cloning-tactics": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("a", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("cloning-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/elemental-scout/feature/superior-elements": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("b", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("elemental-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/pathfinder-scout/feature/superior-movement": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("c", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("pathfinder-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/superior-phantasm": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("b", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("phantom-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/tactical-scout/feature/superior-tactics": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("tactical", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("tactical-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/trickster-scout/feature/superior-trickster": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scoutNinSuperiorityDice("a", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("trickster-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Chakra Sphere / Repelling Burst — Barrier Scout's own combined "uses"
	// pool (3rd level, Projected Barrier): "You can use any combination of
	// these abilities a number of times equal to your Proficiency Bonus
	// per Long Rest." Beyond that, "it costs 1 Superiority Die or Chakra
	// Die per use" — a manual trade against those other pools the player
	// tracks by hand, same "the app has no cross-pool spend mechanism"
	// boundary Clone Technique/Tricksters Soul Binding's identical
	// "spend a Superiority Die instead" clauses also draw below.
	"class/scout-nin/group/scouting-technique/barrier-scout/feature/projected-barrier": {
		Key:      "projected_barrier",
		Name:     "Chakra Sphere / Repelling Burst",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Chakra Sphere (reaction, reduce allies' damage) or Repelling Burst (action, AoE Force damage + push/prone) — extra uses beyond this cost 1 Superiority Die or Chakra Die each",
	},
	// Ghastly Leech — Phantom Scout's own 14th-level "convert a rolled
	// Phantasmic Power die into HP or Chakra" uses pool: "You can gain the
	// benefit of this feature a number of times equal to your Proficiency
	// Bonus, per long rest." Phantasmic Power's own die-roll mechanism
	// (what gets rolled in the first place) has no size/readout modeled
	// anywhere in this app yet — only this uses-count is tracked here.
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/ghastly-leech": {
		Key:      "ghastly_leech",
		Name:     "Ghastly Leech",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Convert one rolled Phantasmic Power die into Hit Points or Chakra Points equal to the roll",
	},
	// Willpower Surge — Trickster Scout's own 14th-level "call your Void
	// Soul forth mid-cast" uses pool: "you may use this feature a number
	// of times equal to your Charisma Ability Modifier per long rest." No
	// stated minimum (unlike Sugar Rush's own explicit "minimum bonus of
	// +1" above), so a non-positive Charisma modifier floors at 0 uses —
	// same "no minimum stated" precedent Perfected Formula already
	// establishes for this exact wording shape. "If you attempt to use
	// this feature when you have no more uses left, you may spend a
	// Superiority Die to use it" is the same manual cross-pool trade
	// Projected Barrier's own overflow clause draws above, not modeled.
	"class/scout-nin/group/scouting-technique/trickster-scout/feature/willpower-surge": {
		Key:      "willpower_surge",
		Name:     "Willpower Surge",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cha < 0 {
				return 0
			}
			return cha
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast a jutsu through your Void Soul, ignoring its Hand Sign component and adding 3x your Charisma modifier in damage once per casting",
	},
	// Tricksters Words — one of the four named benefits Tricksters Soul
	// Binding (6th level) grants while fused with the Void Soul: "You can
	// use Tricksters Words in this way a number of times equal to your
	// Charisma Modifier per rest." Unscoped "per rest" regenerates on
	// every tier, same precedent Chakra Barrier's own unscoped phrasing
	// already establishes. The feature's other three named benefits
	// (Tricksters Potential's own Superiority Die regen trigger,
	// Tricksters Strength's ability-modifier swap, Tricksters Spirit's
	// advantage-on-saves) and the fusion itself ("once per short rest,"
	// its own separate resource) are not modeled — only this uses-count is
	// tracked here, unconditionally, the same "the pool exists whether or
	// not the app also models its own gating trigger" boundary Ghastly
	// Leech's own Phantasmic Power dependency above draws.
	"class/scout-nin/group/scouting-technique/trickster-scout/feature/tricksters-soul-binding": {
		Key:      "tricksters_words",
		Name:     "Tricksters Words",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cha < 0 {
				return 0
			}
			return cha
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Influence a creature's next skill check, saving throw, or attack roll by 1d6 (aid an ally, hinder a foe)",
	},
	"class/science-nin/feature/chakra-containment-device": {
		Key:      "ccd",
		Name:     "Chakra Containment Device",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return scienceNinCCDMax(cl["class/science-nin"])
		},
		// No automatic Short Rest gain in the book text at all — sealing
		// chakra into the CCD on a short rest is a player choice tied to
		// a Chakra Dice roll (same "player rolls, types the result in"
		// shape the main HP/Chakra short rest already uses), covered by
		// the resource's own manual edit box rather than an automatic
		// regen here.
		ShortRegen:  regenNone,
		LongRegen:   regenHalfMax,
		FullRegen:   regenFull,
		Restriction: "Powers Scientific Ninja Tools and Science-Nin features",
	},
	// "Starting at 10th level ... you can use your reaction to add your
	// Intelligence modifier to the roll [a Skill check or saving throw
	// within 30 feet]. At Level 15, you also use this feature to subtract
	// your Intelligence Modifier from an enemy creature's Skill check or
	// saving throw. You can use this feature a number of times equal to
	// your Intelligence modifier (minimum of once). You regain all
	// expended uses when you finish a long rest."
	"class/science-nin/feature/calculated-response": {
		Key:      "calculated_response",
		Name:     "Calculated Response",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if intMod < 1 {
				return 1
			}
			return intMod
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction: add (or, from 15th level, subtract) your Intelligence modifier to a Skill check or saving throw within 30 feet",
	},
	// "Once per short rest, you can reveal that you predicated the current
	// situation. You can pull a scroll from your inventory that has one
	// basic quality toolkit with a single charge. This can only be used by
	// you and loses its charge after being used or in 10 minutes,
	// whichever comes first." A flat single-charge pool reset on every
	// rest tier, same "per Short Rest" shape Hunters Exploits already uses
	// below (ShortRegen/LongRegen/FullRegen all regenFull, since a Long or
	// Full Rest is a superset of a Short Rest).
	"class/science-nin/feature/the-right-tool": {
		Key:      "the_right_tool",
		Name:     "The Right Tool",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Pull a scroll holding one basic-quality toolkit with a single charge; lost after use or 10 minutes",
	},
	// "Beginning at 9th level, you have outfitted your Scientific Ninja
	// Beast with its own Chakra Containment Device, when you would regain
	// chakra into your C.C.D you may choose to instead store it into your
	// Scientific Ninja Beast... Your S.N.B's Chakra Containment Device can
	// hold an amount of chakra equal to your Science-Nin level x 5." A
	// second CCD-shaped pool, tracked here as its own size/readout with
	// fully manual regen — it is filled only when the player chooses to
	// redirect what would otherwise be a normal CCD gain (never an
	// automatic rest formula of its own, unlike the base CCD's own
	// LongRegen), and drained only by a bonus-action withdrawal while
	// within 5 feet of the S.N.B. The S.N.B companion itself (the creature
	// this pool lives inside, and the "within 5 feet" gate on withdrawing
	// from it) has no stat-block/presence tracking anywhere in this app —
	// see CLASS_AUDIT.md's Science-Nin detail entry.
	"class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/secondary-c-c-d": {
		Key:      "snb_secondary_ccd",
		Name:     "S.N.B Secondary C.C.D",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return cl["class/science-nin"] * 5
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenNone,
		Restriction: "Filled only when you choose to redirect a normal C.C.D gain into it instead; withdraw any amount as a bonus action while within 5 feet of your S.N.B",
	},
	// "You can use these features a number of times equal to your
	// Proficiency Bonus per Short Rest" — the spend pool behind whichever
	// Hunters Exploits the character knows (the known-list itself, cap
	// 2->3@10th->4@17th, is a separate cap+catalog picker in
	// cmd/n5e/hunter_nin.go, the same "pool here, known-list there" split
	// Martial Dice/Known Martial Techniques already established).
	"class/hunter-nin/feature/hunters-exploits": {
		Key:  "hunter_exploits",
		Name: "Hunters Exploits",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "This Scroll can be used once per long rest." — a single-charge
	// resource, not a pool the Puppet Swarm's own AC/HP/Commands (all
	// computed live in puppetSwarmStats, not tracked here) has any part in.
	"class/puppet-master/group/puppet-techniques/red-technique-performer/feature/performance-of-10-puppets": {
		Key:      "performance_scroll",
		Name:     "Performance Scroll",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Summons the Puppet Swarm",
	},
	// "Also, your Puppets gain a number of chakra dice (d8s) equal to your
	// level in this class. Delegate these dice among your Puppet Tools...
	// Your Puppet Tools recover these dice the same way as you recover
	// your dice." That last sentence points at the character's own Hit/
	// Chakra Dice, which this app already tracks as a fully player-managed
	// delta (charstore.SetRestGains — the player types in how many they
	// spend/regain, no automatic per-rest formula) rather than an
	// automatic regen tier, so this pool gets the same regenNone/manual-
	// edit-box treatment as Purple Lightning above, not regenFull.
	"class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/chakra-conduits": {
		Key:      "puppet_chakra_dice",
		Name:     "Puppet Chakra Dice",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return cl["class/puppet-master"]
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenNone,
		Restriction: "d8s, delegated among your Puppet Tools; they spend these in place of your own chakra dice",
	},
	// "Once per rest, when a creature is affected by a Genjutsu that you
	// cast, you can disrupt their chakra..." Gains an additional use of
	// either of the feature's two effects at 7th and 14th level (1 -> 2 ->
	// 3 total). The book never names a rest tier ("per rest" alone) — same
	// unscoped phrasing Chakra Barrier's own "whenever you complete a rest
	// of any type" already resolves by regenerating fully on every tier,
	// applied identically here rather than guessing at one tier.
	"class/genjutsu-specialist/feature/chakra-disruption": {
		Key:  "chakra_disruption",
		Name: "Chakra Disruption",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			n := 1
			gl := cl["class/genjutsu-specialist"]
			if gl >= 7 {
				n++
			}
			if gl >= 14 {
				n++
			}
			return n
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Disrupt a Genjutsu-affected creature's chakra, or (5th+) disable their chakra-molding until the end of their next turn",
	},
	// "You have a number of Actualization Die equal to your Proficiency
	// Bonus. You recover spent die on a short or long rest." Die SIZE
	// (d4->d6@9th->d8@17th) is a separate, level-only lookup
	// (actualizationDieSize, genjutsu.go) — this pool only tracks the
	// COUNT, same split Hunter-Nin's Lethal Attack die-size readout draws
	// against its own unrelated resource pools.
	"class/genjutsu-specialist/feature/actualization": {
		Key:  "actualization_die",
		Name: "Actualization Die",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "At 1st level, when you would take a short or Long Rest, you can
	// create a number of Shinobi Snacks equal to your proficiency bonus
	// plus your Intelligence [modifier]." The printed text drops the word
	// "modifier" — confirmed a PDF-extraction dropped word, not a
	// deliberate use-full-score rule, since this same feature's own War
	// and Food (7th level) explicitly says "equal to your intelligence
	// modifier" for the identical recreate-Snacks effect. Clamped to 0:
	// the book states no floor, and a very low Intelligence score could
	// otherwise drive this negative.
	"class/cooking-nin/feature/shinobi-snacks": {
		Key:      "shinobi_snacks",
		Name:     "Shinobi Snacks",
		MinLevel: 1,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			n := prof + intMod
			if n < 0 {
				n = 0
			}
			return n
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You may activate an Aura in this way number of times per long rest
	// equal to your proficiency bonus." The base Aura-activation pool
	// every subclass's own 9th-level bonus Aura (below) layers alongside,
	// not on top of — each subclass grant uses its own distinct Key, since
	// the book's "without spending a use of your Auras" phrasing makes the
	// 9th-level Aura explicitly a separate pool from this one.
	"class/cooking-nin/feature/wandering-aroma": {
		Key:      "wandering_aroma",
		Name:     "Wandering Aroma",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction: activate an Aura, centered on a creature eating one of your Snacks, for the Snack's duration",
	},
	// "As an action, you may spend a use of your Cooking Kit... You can
	// use this feature once per long rest. Beginning at 15th level, you
	// can use this feature twice times per long rest." "Cooking Kit" here
	// is flavor text for the class's own Cooking Kit tool proficiency, not
	// a separate resource — the real gate is this once/twice-per-long-rest
	// use count (see CLASS_AUDIT.md's Cooking-Nin detail entry for the
	// full reasoning).
	"class/cooking-nin/feature/war-and-food": {
		Key:      "war_and_food",
		Name:     "War and Food",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cl["class/cooking-nin"] >= 15 {
				return 2
			}
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Gain the benefit of Shinobi Snacks mid-combat, as an action",
	},
	// Every Cooking Focus subclass grants a second, subclass-named Aura at
	// 9th level with word-for-word identical phrasing: "You may activate
	// this Aura once, then twice at 14th level, per Long Rest without
	// spending a use of your Auras." — a pool distinct from the base
	// Wandering Aroma pool above (own Key per subclass, since only one of
	// these nine a character can ever have applies to them).
	"class/cooking-nin/group/cooking-focus/battle-cook/feature/fighting-aura":                 cookingFocusBonusAuraGrant("battle_cook_aura", "Fighting Aura"),
	"class/cooking-nin/group/cooking-focus/entremetier-chef/feature/speedy-aura":              cookingFocusBonusAuraGrant("entremetier_chef_aura", "Speedy Aura"),
	"class/cooking-nin/group/cooking-focus/patissier-chef/feature/sugary-aura":                cookingFocusBonusAuraGrant("patissier_chef_aura", "Sugary Aura"),
	"class/cooking-nin/group/cooking-focus/herbalist/feature/inebriated-aura":                 cookingFocusBonusAuraGrant("herbalist_aura", "Inebriated Aura"),
	"class/cooking-nin/group/cooking-focus/gastrochemist/feature/aura-of-equivalent-exchange": cookingFocusBonusAuraGrant("gastrochemist_aura", "Aura of Equivalent Exchange"),
	"class/cooking-nin/group/cooking-focus/show-cook/feature/a-satisfying-display":            cookingFocusBonusAuraGrant("show_cook_aura", "A Satisfying Display"),
	"class/cooking-nin/group/cooking-focus/sour-taste/feature/poisoned-snacks-2":              cookingFocusBonusAuraGrant("sour_taste_aura", "Poisoned Snacks (Aura)"),
	"class/cooking-nin/group/cooking-focus/heat-master/feature/nova-aura":                     cookingFocusBonusAuraGrant("heat_master_aura", "Nova Aura"),
	// Fry Cooks' own bonus Aura genuinely reads "which you can activate for
	// free once per long rest" with NO 14th-level clause anywhere in the
	// text — confirmed as real RAW (the surrounding sentence is otherwise
	// clean prose, no extraction-truncation artifact), not a bug to
	// "fix" toward matching the other 8 subclasses' escalating version.
	"class/cooking-nin/group/cooking-focus/fry-cooks/feature/sunny-side-up": {
		Key:      "fry_cooks_aura",
		Name:     "Sunny Side Up",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Activate the Sunny Side Up Aura for free, without spending a use of Auras",
	},
	// "Starting at 13th level... You can use this feature a number of
	// times per Long Rest equal to your charisma modifier (with a minimum
	// bonus of +1)."
	"class/cooking-nin/group/cooking-focus/patissier-chef/feature/sugar-rush": {
		Key:      "sugar_rush",
		Name:     "Sugar Rush",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cha < 1 {
				return 1
			}
			return cha
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning at 13th level... You may use this Feature twice per Long
	// Rest."
	"class/cooking-nin/group/cooking-focus/herbalist/feature/vibe-killer": {
		Key:      "vibe_killer",
		Name:     "Vibe Killer",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning at 13th level... You can do this once per Long Rest. You
	// gain an additional use of this feature at 17th level."
	"class/cooking-nin/group/cooking-focus/show-cook/feature/grand-finale": {
		Key:      "grand_finale",
		Name:     "Grand Finale",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cl["class/cooking-nin"] >= 17 {
				return 2
			}
			return 1
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Starting at 17th level... A number of times per rest equal to your
	// Charisma Modifier, you may... grant one creature... the effects of
	// your chosen Sour Snack." Unscoped "per rest" — regenerates on every
	// tier, same precedent Chakra Barrier's own unscoped phrasing already
	// established.
	"class/cooking-nin/group/cooking-focus/sour-taste/feature/perfected-formula": {
		Key:      "perfected_formula",
		Name:     "Perfected Formula",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cha < 0 {
				return 0
			}
			return cha
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Beginning at 17th level, Once per rest, you can choose to add a +20
	// to your initiative score." Unscoped "per rest" — regenerates on
	// every tier, same treatment as Perfected Formula above. Only the
	// use-count gate is tracked here; applying the +20 itself stays a
	// manual player action (no initiative-modifier-override mechanism
	// exists in this app).
	"class/cooking-nin/group/cooking-focus/entremetier-chef/feature/before-all": {
		Key:      "before_all",
		Name:     "Before All",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Add +20 to your initiative score before or after rolling",
	},
	// "Beginning at 17th level... Once per Rest, when a creature would
	// make a saving throw or Ability Check to resist a Genjutsu you cast
	// with the Inhaled Keyword, you may spend a Snack, and a Charge of a
	// Poison Kit, reducing their result..." Unscoped "per Rest" — same
	// all-tiers-regenFull treatment as Perfected Formula/Before All. Only
	// the once-per-rest gate is tracked; the spend-a-Snack-and-a-Poison-
	// Kit-charge debuff itself stays a manual effect.
	"class/cooking-nin/group/cooking-focus/herbalist/feature/unmatched-botanist": {
		Key:      "unmatched_botanist",
		Name:     "Unmatched Botanist",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reduce a save/check to resist your Inhaled Genjutsu, spending a Snack and a Poison Kit charge",
	},
	// "Beginning at 2nd Level... may, once per long rest, reroll any
	// saving throw you make, or attack targeting you, caused by jutsu of
	// your Nature's Blend release."
	"class/cooking-nin/group/cooking-focus/gastrochemist/feature/eye-of-the-storm": {
		Key:      "eye_of_the_storm",
		Name:     "Eye of the Storm",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reroll a save or an attack against you caused by a Nature's Blend jutsu",
	},
	// "Finally, twice per Long Rest, when a creature succeeds a saving
	// throw against a Jutsu with the medical Keyword that deals Poison
	// damage, or against a Poison placed/created by you, you may cause
	// them to take damage from the envenomed condition as though they had
	// started their turn." This is the 13th-level Sour Taste feature
	// (slug poisoned-snacks-3), distinct from the L2 ingredient list
	// (poisoned-snacks) and the L9 bonus-Aura feature (poisoned-snacks-2,
	// its own "sour_taste_aura" Key above) — a separate Key avoids
	// colliding with either.
	"class/cooking-nin/group/cooking-focus/sour-taste/feature/poisoned-snacks-3": {
		Key:      "poisoned_snacks_13",
		Name:     "Poisoned Snacks (Envenomed Trigger)",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You can only activate this feature a 3 times per Long Rest, you gain
	// additional uses as you gain levels in this class as seen in the
	// Chakra Scalpel charges column of the Medical-Nin class table." The
	// bracket below is hand-transcribed from v_class_level_resources'
	// "Chakra Scalpel Charges" column (3@3rd, 4@4th-6th, 5@7th-9th,
	// 6@10th-12th, 7@13th-15th, 8@16th-18th, 9@19th-20th) since Max has no
	// database access — the feature's own melee-attack damage die
	// (1d4->5d4, "Chakra Scalpel damage" column) is a separate size-only
	// readout with live DB access, not a pool — see medical_nin.go's
	// chakraScalpelDamageDie. Many subclass features "spend a use of
	// Chakra Scalpel" against this same pool. Only "per Long Rest" is
	// stated — no short-rest regen.
	"class/medical-nin/feature/chakra-scalpel": {
		Key:      "chakra_scalpel_charges",
		Name:     "Chakra Scalpel Charges",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/medical-nin"]
			switch {
			case lvl >= 19:
				return 9
			case lvl >= 16:
				return 8
			case lvl >= 13:
				return 7
			case lvl >= 10:
				return 6
			case lvl >= 7:
				return 5
			case lvl >= 4:
				return 4
			default:
				return 3
			}
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You learn both Preserve Life and Take Life... you can use Preserve
	// Life or Take Life twice between rests. You gain an additional use at
	// 9th, 13th and 17th levels." A single shared use-count pool drained by
	// either half of the feature. Book says "between rests" unscoped — same
	// precedent Chakra Barrier's own unscoped phrasing already resolves as
	// regenFull on every tier.
	"class/medical-nin/feature/preserve-take-life": {
		Key:      "preserve_take_life",
		Name:     "Preserve/Take Life",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/medical-nin"]
			switch {
			case lvl >= 17:
				return 5
			case lvl >= 13:
				return 4
			case lvl >= 9:
				return 3
			default:
				return 2
			}
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Adept Medic's "Preserve Life: Mending Presence": "Beginning at 6th
	// level, you can use the Preserve Life Feature a number of additional
	// times equal to half of your Proficiency Bonus, per long rest." An
	// additive escalation onto the base Preserve/Take Life pool above,
	// expressed the same way White Chakra Surge stacks onto Hatake's base
	// grant: this Max recomputes the FULL total (base bracket + half
	// Proficiency Bonus, floored, matching the project's existing "half
	// Proficiency Bonus" convention — see jutsu_grants.go) rather than just
	// the extra, so the higher-Max-wins combination rule picks this one up
	// automatically once the character has it.
	"class/medical-nin/group/tenets-of-medicine/adept-medic/feature/preserve-life-mending-presence": {
		Key:      "preserve_take_life",
		Name:     "Preserve/Take Life",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/medical-nin"]
			base := 2
			switch {
			case lvl >= 17:
				base = 5
			case lvl >= 13:
				base = 4
			case lvl >= 9:
				base = 3
			}
			return base + prof/2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Channeled Healing's own second clause, distinct from the same
	// feature's flat bonus-healing readout (medical_nin.go's
	// channeledHealingBonus, a size-only line, not a pool): "jutsu with the
	// Medical Keyword you cast that restores hit points or ends conditions,
	// remove one failed death saving throw from an affected creature, if
	// any. You can remove failed death saving throws in this way a number
	// of times equal to your Proficiency Bonus per long rest."
	"class/medical-nin/feature/channeled-healing": {
		Key:      "channeled_healing_death_save_removal",
		Name:     "Channeled Healing (Remove Failed Death Save)",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Combat Medic's Yin Seal: Charge: "You have a number of Yin Motes,
	// equal to your Proficiency Bonus per rest." Spent by this feature
	// itself, Passive Regeneration, and Yin Seal: Release.
	"class/medical-nin/group/tenets-of-medicine/combat-medic/feature/yin-seal-charge": {
		Key:      "yin_motes",
		Name:     "Yin Motes",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Combat Medic's Competent Combatant, second half — distinct from the
	// same feature's Wisdom-for-Dexterity AC substitution (already
	// implemented in internal/features/grants.go's acSwapGrants): "you may
	// spend 5 Chakra. When you do, you instead, double the relevant
	// modifier when calculating damage dealt. You may do this twice per
	// rest. Increase the number of uses by one at 5th, 9th, 13th and 17th
	// levels." (2@2nd -> 3@5th -> 4@9th -> 5@13th -> 6@17th.)
	"class/medical-nin/group/tenets-of-medicine/combat-medic/feature/competent-combatant": {
		Key:      "competent_combatant_uses",
		Name:     "Competent Combatant (Double/Triple Modifier)",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/medical-nin"]
			switch {
			case lvl >= 17:
				return 6
			case lvl >= 13:
				return 5
			case lvl >= 9:
				return 4
			case lvl >= 5:
				return 3
			default:
				return 2
			}
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Combat Medic's Passive Regeneration: a SECOND, independent use-count
	// pool (Proficiency-Bonus-scaled, per long rest) gating the ability
	// itself, on top of the 1 Yin Mote it also spends per use: "You can
	// only use this feature a number of times equal to your Proficiency
	// Bonus per long rest."
	"class/medical-nin/group/tenets-of-medicine/combat-medic/feature/passive-regeneration": {
		Key:      "passive_regeneration_uses",
		Name:     "Passive Regeneration",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Natural Medicine's Natural Healing: "You have stored a pool of this
	// healing energy represented by a number of d6's equal to your
	// Medical-Nin level... You regain all expended dice when you finish a
	// long rest."
	"class/medical-nin/group/tenets-of-medicine/natural-medicine/feature/natural-healing": {
		Key:      "natural_healing_dice",
		Name:     "Natural Healing Dice",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return cl["class/medical-nin"]
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Shaman's Shaman's Hex: "You can mark a creature using this feature a
	// number of times equal to your Proficiency Bonus per rest."
	"class/medical-nin/group/tenets-of-medicine/shaman/feature/shamans-hex": {
		Key:      "shamans_hex_marks",
		Name:     "Shaman's Hex Marks",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Transmuter's Transfigured Technique: "You can remove conditions in
	// this way, a number of times equal to your Proficiency Bonus per long
	// rest."
	"class/medical-nin/group/tenets-of-medicine/transmuter/feature/transfigured-technique": {
		Key:      "transfigured_technique_uses",
		Name:     "Transfigured Technique",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You can spend up to two brave orders per turn. You gain more brave
	// orders at later levels, as shown in the Brave Order column of the
	// Intelligence Operative class table. A Brave Order is expended when you
	// use it. You regain all of your Brave Orders when you finish a short or
	// long rest." class_level_resources' own "Brave Orders" column confirmed
	// via direct SQL query to equal floor(level/2)+2 at every row from 2nd to
	// 20th level (3@2 -> 12@20) -- the class's core resource, spent by nearly
	// every subclass feature below.
	"class/intelligence-operative/feature/master-planner": {
		Key:      "brave_orders",
		Name:     "Brave Orders",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return cl["class/intelligence-operative"]/2 + 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Trap Setter, second half -- distinct from the same feature's own
	// known-traps cap+catalog pick (cmd/n5e/intelligence_operative.go):
	// "You can set a number of Operative Traps equal to your Proficiency
	// Bonus before you run out of resources and need to take a rest to
	// prepare more." Book only says "a rest" -- same unscoped-tier precedent
	// Chakra Barrier's own "whenever you complete a rest of any type"
	// already resolves by regenerating on every tier.
	"class/intelligence-operative/group/master-strategies/tactical-strategist/feature/trap-setter": {
		Key:      "operative_traps_set",
		Name:     "Operative Traps Set",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You have a number of Sensory Tags equal to your Proficiency Bonus.
	// You regain spent sensory seals at the end of a long rest." Situational
	// Awareness (13th level, same subclass) escalates recovery to short-or-
	// long rest -- the existing "two grants share a Key, higher Max wins"
	// combination rule can't express a regen-cadence escalation cleanly
	// (both grants would compute the identical Max), so this ships Short/
	// Long regenFull unconditionally from 3rd level, the same minor RAW-
	// generosity-below-the-stated-level Chakra Barrier's own unscoped-rest-
	// tier phrasing already accepts.
	"class/intelligence-operative/group/master-strategies/sensory/feature/sensory-seals": {
		Key:      "sensory_seals",
		Name:     "Sensory Seals",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "As a Bonus Action, all allies gain the Checkmate condition... Once
	// you use this feature, you cannot do so again until you complete a
	// long rest." Tracks only this once-per-long-rest activation gate -- the
	// feature's own Proficiency-Bonus-charge, per-creature condition economy
	// ("A charge is expended when an affected creature fails a saving
	// throw, or you end your turn") has no tracking mechanism anywhere in
	// this app and stays untracked, same boundary Performance Scroll draws
	// around the Puppet Swarm's own stat block.
	"class/intelligence-operative/feature/checkmate": {
		Key:      "checkmate_activation",
		Name:     "Checkmate",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Grant all allies the Checkmate condition as a Bonus Action; tracks only the once-per-long-rest activation, not the Proficiency-Bonus in-combat charge economy",
	},
	// "You can spend a Brave Order to activate this feature twice per long
	// rest." Tracks only this activation gate; the per-creature "twice-
	// then-locked-out" Tsume action economy has no tracking mechanism here
	// and stays untracked.
	"class/intelligence-operative/feature/tsume": {
		Key:      "tsume_activation",
		Name:     "Tsume",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Mark all allies with Exploit Weakness, granting Tsume; also costs 1 Brave Order",
	},
	// "Once you use this feature, to replace a hostile creature's d20 roll
	// twice, you cannot do so again until you complete a long rest." Only
	// the 15th-level hostile-targeting sub-use carries this cap -- the
	// ally-targeting use (available from 5th) has no stated per-rest limit
	// and stays an untracked Group 2 buff.
	"class/intelligence-operative/feature/tactical-scheme": {
		Key:      "tactical_scheme_hostile",
		Name:     "Tactical Scheme (Hostile Targeting)",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Replace a hostile creature marked by Exploit Weakness's d20 roll; the ally-targeting use of this feature has no per-rest cap and isn't tracked here",
	},
	// "You may use your ability to perceive possible futures... You can use
	// this feature once per long rest." Also spends 1 Brave Order per use,
	// but the long-rest cap is a separately-trackable limit.
	"class/intelligence-operative/group/master-strategies/precognitive/feature/momentary-pause": {
		Key:      "momentary_pause",
		Name:     "Momentary Pause",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Take another turn at the end of your turn; also costs 1 Brave Order",
	},
	// "you can use either effect of this feature a number of times per
	// long rest, equal to your Proficiency Bonus."
	"class/intelligence-operative/group/master-strategies/precognitive/feature/converging-timelines": {
		Key:      "converging_timelines",
		Name:     "Converging Timelines",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "roll three d20s and record the numbers rolled... Each precognitive
	// roll can be used only once. When you finish a long rest, you lose any
	// unused precognitive rolls." A fixed pool that fully RESETS (not
	// accumulates) on a long rest -- functionally identical to regenFull
	// against a constant Max of 3.
	"class/intelligence-operative/group/master-strategies/precognitive/feature/day-ahead": {
		Key:      "day_ahead_rolls",
		Name:     "Day Ahead (Precognitive Rolls)",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 3
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You may expend 1 Brave Order to enter into a trance of sorts...
	// You may only do this once per Long Rest."
	"class/intelligence-operative/group/master-strategies/precognitive/feature/omniscient-clairvoyance": {
		Key:      "omniscient_clairvoyance",
		Name:     "Omniscient Clairvoyance",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Advantage on all rolls, disadvantage on attacks against you, for 1 minute; also costs 1 Brave Order",
	},
	// "Once you use this feature you cannot do so again until you complete
	// a rest and the trait vanishes from your Azure Scroll." Book says only
	// "a rest" -- same unscoped-tier precedent as Operative Traps Set above.
	"class/intelligence-operative/group/master-strategies/azure-analyst/feature/sapphire-insights": {
		Key:      "sapphire_insights",
		Name:     "Sapphire Insights",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Share one Azure Scroll trait with all allies within 30 feet; also costs 1 Brave Order",
	},
	// "Regardless of the choice, you can only use this feature twice per
	// long rest."
	"class/intelligence-operative/group/master-strategies/mastermind-strategist/feature/war-cry": {
		Key:      "war_cry",
		Name:     "War Cry",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You can use this feature twice per long rest."
	"class/intelligence-operative/group/master-strategies/tactical-strategist/feature/in-perfect-sync": {
		Key:      "in_perfect_sync",
		Name:     "In Perfect Sync",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Also costs 1 Brave Order",
	},
	// "Once per short rest, you may half the cost of any one Ninjutsu
	// without the Combination keyword that you cast. You gain an additional
	// use of this feature at 6th, 11th & 17th levels." Unscoped "per short
	// rest" resources regenerate on every rest tier in this app — same
	// precedent Chakra Disruption/Chakra Barrier already establish.
	"class/ninjutsu-specialist/feature/chakra-recovery": {
		Key:  "chakra_recovery",
		Name: "Chakra Recovery",
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/ninjutsu-specialist"]
			n := 1
			if lvl >= 6 {
				n++
			}
			if lvl >= 11 {
				n++
			}
			if lvl >= 17 {
				n++
			}
			return n
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Half the chakra cost of one Ninjutsu without the Combination keyword",
	},
	// "You may use this feature twice per rest. You gain an additional use
	// of this feature at 11th and 17th level." The clash-execution payload
	// itself isn't automated — this pool only tracks the use-count.
	"class/ninjutsu-specialist/feature/jutsu-breaker": {
		Key:      "jutsu_breaker",
		Name:     "Jutsu Breaker",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/ninjutsu-specialist"]
			n := 2
			if lvl >= 11 {
				n++
			}
			if lvl >= 17 {
				n++
			}
			return n
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You can use any mix of efficient moldings you have 2 times per rest.
	// You gain an additional use of your moldings at 9th, and 15th levels."
	// The separate KNOWN-Efficient-Moldings cap+catalog pick (which
	// moldings a character knows at all) lives in ninjutsu_specialist.go,
	// not here — same "pool here, known-list there" split Hunters
	// Exploits/Hunters Patterns already established.
	"class/ninjutsu-specialist/feature/efficient-molding": {
		Key:      "efficient_molding_uses",
		Name:     "Efficient Molding Uses",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			lvl := cl["class/ninjutsu-specialist"]
			n := 2
			if lvl >= 9 {
				n++
			}
			if lvl >= 15 {
				n++
			}
			return n
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Also, at 7th level, your wind moves at such speeds that it tears
	// through most defenses. Your wind damage ignores resistance and
	// disperses vapors, gases, and fogs that can be dispersed by strong
	// winds. At 11th level, your wind's speed is unmatched, once per rest,
	// when a creature would cast a Reaction against a wind release jutsu you
	// cast, you can force them to make a Dexterity saving throw against the
	// jutsu save DC, being unable to take their reaction on a failed save.
	// At 15th level, after you have used this feature, you may spend 5
	// chakras to use it again. A jutsu can only force this save once per
	// casting." The 7th-level resistance/dispersal clause is a passive
	// effect with nothing to spend and isn't tracked here; only the
	// 11th-level once-per-rest Reaction-denial use is a spendable pool,
	// gated to that level independently of the row's own 7th-level grant —
	// same MinLevel-vs-row-level split Chakra Barrier/Ghastly Leech already
	// establish. Unscoped "per rest" regenerates on every tier, same
	// precedent Chakra Barrier's own phrasing already establishes. The
	// 15th-level "spend 5 chakra for an additional use" is a manual
	// cross-pool trade against the character's own Chakra pool, not
	// modeled — same boundary Projected Barrier's/Willpower Surge's own
	// overflow clauses draw above.
	"clan/fushin/feature/forceful-gale": {
		Key:      "forceful_gale",
		Name:     "Forceful Gale",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Force a Dexterity save against your jutsu's save DC to deny a creature's Reaction against a Wind Release jutsu you cast (once per casting); at 15th level, spend 5 chakra for an additional use",
	},
	// "This feature has a number of charges equal to your Proficiency Bonus
	// per long rest, with each option spending 1 charge, unless otherwise
	// specified." Kill costs 2 charges; Stun and Cripple cost 1 each — not
	// modeled per-option here (this table tracks the pool's own size/
	// regen, same "pool here, per-option cost is informational" split
	// Master Slayer's own sibling capstones use), just surfaced via
	// Restriction so the player knows Kill is pricier when spending by
	// hand.
	"class/weapon-specialist/group/weapon-forms/slayer-form/feature/master-slayer": {
		Key:      "master_slayer_charges",
		Name:     "Master Slayer",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Kill costs 2 charges; Stun and Cripple cost 1 charge each",
	},
	// Disturbance's Debilitating Barrage: "...you can spend 2 martial die to
	// force the creature to make a Constitution saving throw... You can only
	// use this feature twice per rest." The saving throw and vulnerability
	// payload itself isn't automated — this pool only tracks the use-count.
	// Book text doesn't scope "per rest" to a specific tier, so all three
	// regen tiers are full, matching this file's own convention for
	// unscoped "per rest" wording elsewhere (see Before All/Perfected
	// Formula above).
	"class/taijutsu-specialist/group/taijutsu-style/disturbance/feature/debilitating-barrage": {
		Key:      "debilitating_barrage_uses",
		Name:     "Debilitating Barrage",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Hit a creature with 2+ attacks in one action, spend 2 Martial Dice, force a Con save vs your Taijutsu Save DC or the creature becomes vulnerable to a damage type of your choice",
	},
	// Ironclad's Iron Fury: "Twice per rest, as an action, you may spend 4
	// martial die to enter a combat trance for 1 minute..." The trance's own
	// mechanical payload (damage resistance, extra unarmed attack, +1 AC)
	// isn't automated — no state exists to auto-apply it — this pool only
	// tracks the twice-per-rest activation cap.
	"class/taijutsu-specialist/group/taijutsu-style/ironclad/feature/iron-fury": {
		Key:      "iron_fury_uses",
		Name:     "Iron Fury",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action, spend 4 Martial Dice, enter a 1-minute combat trance (resistance to all damage except Psychic, extra unarmed attack on Iron Heart trigger, +1 AC while holding your Shield)",
	},
	// Nin-Tai's Elemental Recharge: "...you can spend a Reaction to absorb
	// some of the residual chakra and power. You regain Hit points and
	// Martial Dice based on the rank of the jutsu that triggered this
	// effect. You can gain the benefit of this feature a number of times
	// equal to your Proficiency Bonus per long rest." Explicitly scoped to
	// long rest, unlike the other three Taijutsu Style capstones above — no
	// Short Rest regen.
	"class/taijutsu-specialist/group/taijutsu-style/nin-tai/feature/elemental-recharge": {
		Key:      "elemental_recharge_uses",
		Name:     "Elemental Recharge",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction when you take damage of your associated nature-release type: regain HP/Martial Dice by triggering jutsu's rank (D:10HP/1MD, C:15HP/2MD, B:20HP/3MD, A:30HP/4MD, S:40HP/5MD)",
	},
	// Passionate Flame's Sheer Will: "you can cast Taijutsu with a casting
	// time of an Action as a Bonus Action. Alternatively, you can cast
	// Taijutsu with a casting time of a bonus Action as a Reaction, with no
	// set trigger. You can perform either of these effects twice per rest."
	"class/taijutsu-specialist/group/taijutsu-style/passionate-flame/feature/sheer-will": {
		Key:      "sheer_will_uses",
		Name:     "Sheer Will",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast an Action-cost Taijutsu as a Bonus Action, or a Bonus-Action-cost Taijutsu as a Reaction with no set trigger",
	},
}

// cookingFocusBonusAuraGrant builds one of the 8 identically-worded Cooking
// Focus subclasses' 9th-level bonus-Aura pool ("You may activate this Aura
// once, then twice at 14th level, per Long Rest without spending a use of
// your Auras.") — everything but the Key/Name differs, so this factors out
// the shared shape rather than repeating it 8 times. Fry Cooks' Sunny Side
// Up is NOT built with this helper — its own text has no 14th-level clause
// (see its own entry above).
func cookingFocusBonusAuraGrant(key, name string) customResourceGrant {
	return customResourceGrant{
		Key:      key,
		Name:     name,
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof int) int {
			if cl["class/cooking-nin"] >= 14 {
				return 2
			}
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Activate this bonus Aura for free, without spending a use of Auras",
	}
}

// validCustomResourceKey reports whether key matches some grant's Key, or
// one of the two synthetic keys madScientistCCDSplit produces (never a real
// customResourceGrants entry — see its own doc on why the split can't be
// expressed as one) — used by handleSheetCustomResource to reject a crafted
// request naming an arbitrary resource_key before it ever reaches
// charstore.
func validCustomResourceKey(key string) bool {
	if key == "ccd_mending" || key == "ccd_maiming" {
		return true
	}
	for _, g := range customResourceGrants {
		if g.Key == key {
			return true
		}
	}
	return false
}

// CustomResourceEntry is one resolved custom resource, ready for the
// sheet's Special Resources box.
type CustomResourceEntry struct {
	Key         string
	Name        string
	Current     int
	Max         int
	Restriction string
	// DieSize mirrors customResourceGrant.DieSize's own already-resolved
	// value ("" when the winning grant has none) — see that field's doc.
	DieSize string
	// ShortRegen/LongRegen/FullRegen let handleSheetRest apply this
	// entry's own regen rule for whichever tier of rest was just taken,
	// without a second lookup back into customResourceGrants.
	ShortRegen restRegen
	LongRegen  restRegen
	FullRegen  restRegen
}

// computeCustomResources resolves customResourceGrants against a
// character's granted features (the same loadMergedGrantedFeatures result
// the Core tab and passive traits already use — feats included, since the
// White Chakra Surge and Purple Lightning feats must reach this table
// too), the character's per-class level map, and CON/Intelligence/Charisma
// modifiers.
//
// stored holds whatever's actually saved in character_custom_resources,
// keyed by resource Key (charstore.GetCustomResources) — a resource with
// no stored row yet starts at its own Max, the same implicit-seed
// convention Sheet.MaxChakraAuto already establishes for current_chakra.
//
// When more than one grant shares a Key (White Chakra Surge stacking onto
// the base Hatake grant), the higher computed Max wins, same "take the
// stronger" shape computePassiveTraits already uses for escalating grants.
func computeCustomResources(features []grantedFeatureRow, classLevels map[string]int, conMod, intMod, chaMod, profBonus int, characterLevel int, stored map[string]int) []CustomResourceEntry {
	type resolved struct {
		grant customResourceGrant
		max   int
	}
	byKey := map[string]resolved{}
	var order []string

	for _, f := range features {
		grant, ok := customResourceGrants[f.Slug]
		if !ok {
			continue
		}
		if grant.MinLevel > 0 && characterLevel < grant.MinLevel {
			continue
		}
		max := grant.Max(classLevels, conMod, intMod, chaMod, profBonus)
		if existing, seen := byKey[grant.Key]; !seen {
			byKey[grant.Key] = resolved{grant: grant, max: max}
			order = append(order, grant.Key)
		} else if max > existing.max {
			// The winning (higher-Max) grant also supplies the regen
			// kind/Restriction that governs this key from here on.
			byKey[grant.Key] = resolved{grant: grant, max: max}
		}
	}

	out := make([]CustomResourceEntry, 0, len(order))
	for _, key := range order {
		r := byKey[key]
		current, ok := stored[key]
		if !ok {
			current = r.max
		}
		if current > r.max {
			current = r.max
		}
		var dieSize string
		if r.grant.DieSize != nil {
			dieSize = r.grant.DieSize(classLevels)
		}
		out = append(out, CustomResourceEntry{
			Key:         key,
			Name:        r.grant.Name,
			Current:     current,
			Max:         r.max,
			Restriction: r.grant.Restriction,
			DieSize:     dieSize,
			ShortRegen:  r.grant.ShortRegen,
			LongRegen:   r.grant.LongRegen,
			FullRegen:   r.grant.FullRegen,
		})
	}
	return out
}

// applyRestRegen computes a custom resource's new current value for one
// rest tier. Returns the unchanged current value for regenNone.
func applyRestRegen(kind restRegen, current, max, conMod int) int {
	clamp := func(v int) int {
		if v > max {
			return max
		}
		if v < 0 {
			return 0
		}
		return v
	}
	switch kind {
	case regenConMod:
		gain := conMod
		if gain < 0 {
			gain = 0
		}
		return clamp(current + gain)
	case regenHalfSpent:
		spent := max - current
		return clamp(current + spent/2)
	case regenHalfMax:
		v := max / 2
		if v < current {
			return current
		}
		return v
	case regenFull:
		return max
	default:
		return current
	}
}

// madScientistBioticMasteryFeatureSlug is Mad Scientist's own 3rd-level
// "you split your CCD into two" feature — see madScientistCCDSplit below.
const madScientistBioticMasteryFeatureSlug = "class/science-nin/group/scientific-inquiry/mad-scientist/feature/biotic-mastery"

// madScientistCCDSplit replaces entries' own single "ccd" resource (if
// present) with two independently-spent halves, "ccd_mending" and
// "ccd_maiming" — Biotic Mastery's own text: "you split your CCD into two,
// each leading into an outlet on the palms of your hand... your CCD is
// split into two pools... You can change the ratio of the two Devices
// during a long rest in intervals of 5." mendingPct (0-100, a multiple of
// 5 — see charstore.SetCCDMendingPct) is Mending's own share of the base
// CCD total; Maiming gets the remainder, so the two always sum back to
// exactly what a non-Mad-Scientist Science-Nin's single "ccd" pool would
// have been. Not expressed as two ordinary customResourceGrants entries:
// Max there is a pure function of class levels/modifiers/Proficiency
// Bonus, with nowhere to thread a player-chosen split ratio through, so
// this runs as a post-process step on computeCustomResources' own output
// instead — a no-op for every character without Biotic Mastery granted.
func madScientistCCDSplit(entries []CustomResourceEntry, mendingPct int, stored map[string]int) []CustomResourceEntry {
	out := make([]CustomResourceEntry, 0, len(entries)+1)
	var base *CustomResourceEntry
	for i := range entries {
		if entries[i].Key == "ccd" {
			base = &entries[i]
			continue
		}
		out = append(out, entries[i])
	}
	if base == nil {
		return entries // character has no Chakra Containment Device at all yet
	}

	mendingMax := base.Max * mendingPct / 100
	maimingMax := base.Max - mendingMax

	clamp := func(current, max int) int {
		if current > max {
			return max
		}
		if current < 0 {
			return 0
		}
		return current
	}
	mendingCurrent, ok := stored["ccd_mending"]
	if !ok {
		mendingCurrent = mendingMax
	}
	maimingCurrent, ok := stored["ccd_maiming"]
	if !ok {
		maimingCurrent = maimingMax
	}

	out = append(out,
		CustomResourceEntry{
			Key: "ccd_mending", Name: "Mending CCD",
			Current: clamp(mendingCurrent, mendingMax), Max: mendingMax,
			Restriction: "Powers Mad Scientist's beneficial/healing tools and Inversion Serums",
			ShortRegen:  base.ShortRegen, LongRegen: base.LongRegen, FullRegen: base.FullRegen,
		},
		CustomResourceEntry{
			Key: "ccd_maiming", Name: "Maiming CCD",
			Current: clamp(maimingCurrent, maimingMax), Max: maimingMax,
			Restriction: "Powers Mad Scientist's harmful tools and Inversion Serums",
			ShortRegen:  base.ShortRegen, LongRegen: base.LongRegen, FullRegen: base.FullRegen,
		},
	)
	return out
}

// handleSetCCDMendingPct updates Biotic Mastery's own Mending/Maiming split
// ratio (form field "pct") — surfaced as a "Set %" box beside the Mending
// CCD tile in sheet_vitals, rendered only when the character actually has a
// "ccd_mending" custom resource entry (i.e. Biotic Mastery granted).
// Clamping/rounding to a multiple of 5 is charstore.SetCCDMendingPct's own
// job, not duplicated here. The book's own text gates changing this to
// during a Long Rest; like every other pick/toggle in this app, that's left
// to the player's own honesty rather than enforced server-side.
func (s *server) handleSetCCDMendingPct(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	pct, err := strconv.Atoi(strings.TrimSpace(r.FormValue("pct")))
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	if err := charstore.SetCCDMendingPct(s.charDB, id, pct); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set ccd mending pct:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// loadCustomResources gathers everything computeCustomResources needs for
// one character: its granted features (class/subclass/clan/feats, same
// merged list the Core tab and passive traits use), per-class level map,
// and CON/Intelligence/Charisma modifiers, then resolves the curated grant
// table against them.
func (s *server) loadCustomResources(characterID int64, sheet *charsheet.Sheet) ([]CustomResourceEntry, error) {
	features, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	classes, err := s.loadCharacterClassLevels(characterID)
	if err != nil {
		return nil, err
	}
	classLevels := make(map[string]int, len(classes))
	for _, c := range classes {
		classLevels[c.Slug] = c.Levels
	}
	stored, err := charstore.GetCustomResources(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	conMod := sheet.Abilities["con"].Modifier
	intMod := sheet.Abilities["int"].Modifier
	chaMod := sheet.Abilities["cha"].Modifier
	entries := computeCustomResources(features, classLevels, conMod, intMod, chaMod, sheet.ProficiencyBonus, sheet.Level, stored)

	for _, f := range features {
		if f.Slug == madScientistBioticMasteryFeatureSlug {
			entries = madScientistCCDSplit(entries, sheet.CCDMendingPct, stored)
			break
		}
	}
	return entries, nil
}

// applyCustomResourceRest applies one rest tier's regen to every custom
// resource a character has, via SetCustomResourceValue. tier selects which
// of ShortRegen/LongRegen/FullRegen governs each entry.
func (s *server) applyCustomResourceRest(characterID int64, sheet *charsheet.Sheet, tier string) error {
	entries, err := s.loadCustomResources(characterID, sheet)
	if err != nil {
		return err
	}
	conMod := sheet.Abilities["con"].Modifier
	for _, e := range entries {
		var kind restRegen
		switch tier {
		case "full":
			kind = e.FullRegen
		case "long":
			kind = e.LongRegen
		default:
			kind = e.ShortRegen
		}
		if kind == regenNone {
			continue
		}
		newValue := applyRestRegen(kind, e.Current, e.Max, conMod)
		if newValue == e.Current {
			continue
		}
		if err := charstore.SetCustomResourceValue(s.charDB, characterID, e.Key, newValue); err != nil {
			return err
		}
	}
	return nil
}
