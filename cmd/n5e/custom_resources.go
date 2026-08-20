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
	// int), CON/Intelligence/Charisma/Wisdom modifiers, and Proficiency
	// Bonus (Hunters Exploits' own "uses" pool — see
	// class/hunter-nin/feature/hunters-exploits — is the first entry that
	// needed Proficiency Bonus; Cooking-Nin's Shinobi Snacks needs
	// Intelligence, and its Sugar Rush/Perfected Formula need Charisma;
	// Wolves Legacy's Eyes of a Shinobi is the first entry that needed
	// Wisdom — every grant ignores whichever parameters it doesn't need).
	// When more than one grant shares a Key, the higher computed Max wins
	// (White Chakra Surge stacks onto the base Hatake grant this way).
	Max func(classLevels map[string]int, conMod, intMod, chaMod, profBonus, wisMod int) int
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(map[string]int, int, int, int, int, int) int {
			return 10
		},
		ShortRegen: regenNone,
		LongRegen:  regenNone,
		FullRegen:  regenNone,
	},
	"clan/hoshi/feature/star-chakra": {
		Key:  "star_chakra",
		Name: "Star Chakra",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			if con < 1 {
				con = 1
			}
			return characterLevel(cl) + con
		},
		ShortRegen: regenConMod,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Raises the Calories max further (+character level) -- additive onto
	// the same "calories" key via the higher-Max-wins combination rule,
	// same shape as White Chakra Surge stacking onto Hatake's base grant.
	"feat/akimichi/food-born-hardiness": {
		Key:  "calories",
		Name: "Calories",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			if con < 1 {
				con = 1
			}
			return 2*characterLevel(cl) + con
		},
		ShortRegen: regenConMod,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/uzumaki/feature/chakra-reserves": {
		Key:  "reserve_cells",
		Name: "Reserve Cells",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Augmented Explosives (Level 4+) and Detonator's Gambit (Level 12+, see
	// below) both grant "+2 additional shrapnel die" and neither has an
	// exclusivity clause against the other in its own prerequisites -- the
	// two are independently takeable and not mutually exclusive. This
	// table's higher-Max-wins merge rule (computeCustomResources) picks the
	// single larger Max when multiple grants share a Key rather than
	// summing them, so a level-12+ character who has taken both feats sees
	// only the larger bonus (+2, i.e. prof+2 total) rather than the
	// textually-correct prof+4. Not fixed here -- would require changing
	// computeCustomResources to aggregate by feature-slug presence instead
	// of by max-Key, a change affecting every entry in this file.
	"feat/bakuton/augmented-explosives": {
		Key:  "shrapnel_dice",
		Name: "Shrapnel Dice",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof + 2
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
	"feat/bakuton/detonators-gambit": {
		Key:  "shrapnel_dice",
		Name: "Shrapnel Dice",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof + 2
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Broiling Mind (Level 4+) and Broiling Recharge (Level 8+, see below)
	// both target this same "boil_points" Key and neither has an
	// exclusivity clause against the other in its own prerequisites -- the
	// two are independently takeable and not mutually exclusive. This
	// table's higher-Max-wins merge rule (computeCustomResources) picks the
	// single larger Max when multiple grants share a Key rather than
	// summing them, so a level-8+ character who has taken both feats sees
	// only the larger bonus (+2, i.e. prof+2 total) rather than the
	// textually-correct prof+3. Not fixed here -- would require changing
	// computeCustomResources to aggregate by feature-slug presence instead
	// of by max-Key, a change affecting every entry in this file.
	"feat/futton/broiling-mind": {
		Key:  "boil_points",
		Name: "Boil Points",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof + 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"feat/futton/broiling-recharge": {
		Key:  "boil_points",
		Name: "Boil Points",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof + 1
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Apex Heritage's own Regeneration-use bonus (+1, plus an additional use
	// at 11th and 18th levels) -- this Max recomputes the FULL total (base
	// bracket + feat bonus) rather than just the extra, so it always wins
	// the higher-Max-wins merge above at every level; unlike the Futton
	// Boil Points feats above, no other feat targets "regeneration_uses", so
	// no stacking-conflict caveat applies here. ShortRegen/LongRegen/
	// FullRegen/Restriction are copied verbatim from the base grant so the
	// winning grant's behavior/tooltip text doesn't silently change.
	"feat/hebi/apex-heritage": {
		Key:  "regeneration_uses",
		Name: "Regeneration",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := characterLevel(cl)
			base := 2
			if lvl >= 7 {
				base = 3
			}
			bonus := 1
			if lvl >= 11 {
				bonus++
			}
			if lvl >= 18 {
				bonus++
			}
			return base + bonus
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Blistering Heat's own cap-raise clause only: "You can now hold an
	// amount of Energy Die equal to your proficiency bonus" -- this Max
	// (prof) always exceeds the base grant's (prof+1)/2 for any prof >= 2,
	// so it wins the higher-Max-wins merge and DieSize must be copied
	// forward verbatim or the displayed die size would blank out. The
	// feat's second clause ("You gain 2 Energy Die at the end of any rest")
	// is a flat per-rest gain, not a Max increase -- no restRegen kind
	// expresses that shape yet (see the restRegen enum's own doc), so it
	// isn't modeled here; it needs a new restRegen case and touches
	// applyRestRegen too, a distinct follow-up.
	"feat/keton/blistering-heat": {
		Key:      "energy_die",
		Name:     "Energy Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Superior Speed's own "+2 Speed Die" clause -- this Max always exceeds
	// the base grant's plain "prof", so it wins the higher-Max-wins merge;
	// DieSize is copied forward as the same constant "d8" (not the
	// level-tiered d6/d8/d10 pattern that belongs to the separate
	// "energy_die" resource) or the displayed die size would blank out. The
	// feat's other clauses (+2 Dex saves, +10ft Speed) are modeled elsewhere
	// (internal/features/scores.go, internal/features/grants.go's
	// speedGrants) -- this entry covers only the Speed Die pool.
	"feat/namikaze/superior-speed": {
		Key:      "speed_die",
		Name:     "Speed Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof + 2
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return characterLevel(cl) + 7
		},
		ShortRegen:  regenHalfMax,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Used in place of Chakra to cast Taijutsu; spend at least the jutsu's rank in Inner Chi for +1 damage die or +1 to hit (once per casting); also fuels 72 Earthly Transformations (1/2/4 additional Inner Chi for Simple/Advanced/Mythic Transformations)",
	},
	// Chi Expert: "You gain additional Inner Chi equal to your half
	// character level, rounded up. You gain an additional +1 Inner Chi
	// every 2 levels after you gain this feat." Read as prose restating
	// ceil(current level/2) scaling rather than a separate gain-level-
	// relative bonus, matching this project's established convention for
	// feat-granted pool bonuses (see featMaxPoolBonusSlugs' doc comment,
	// internal/charsheet/charsheet.go: "recompute fresh from the
	// character's current level on every read, never bake in when a bonus
	// was first granted"). This interpretation is not 100% unambiguous --
	// verified algebraically that a literal gain-level-relative reading
	// (ceil(gainLevel/2) at the moment taken, +1 every 2 levels thereafter)
	// does NOT collapse to ceil(currentLevel/2) when the feat is taken at
	// an even level, unlike the HP/Chakra per-level bonuses in
	// featMaxPoolBonusSlugs, which do collapse cleanly regardless of gain
	// level. This Max recomputes the FULL total (base characterLevel+7,
	// plus ceil(characterLevel/2) via the (lvl+1)/2 integer-division
	// idiom), so the higher-Max-wins merge picks it up automatically.
	"feat/shi-hou/chi-expert": {
		Key:  "inner_chi",
		Name: "Inner Chi",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := characterLevel(cl)
			return lvl + 7 + (lvl+1)/2
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Crystal Ascension: "+1 Crystallization Die. You gain an additional die
	// at 12th and 20th levels. Your Crystallization Die become d12's." This
	// Max carries forward the base pool's own +2-at-15th-level term in
	// addition to the feat's own +1 baseline and +1-at-12th/+1-at-20th
	// bonuses, so it composes correctly rather than undercounting by 2 at
	// 15th level and up. MinLevel matches the base grant's own 7th-level
	// gate (the pool doesn't exist below that, even though the feat's own
	// prerequisite is only Level 4+).
	"feat/shoton/crystal-ascension": {
		Key:      "crystallization_die",
		Name:     "Crystallization Die",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := characterLevel(cl)
			n := 4
			if lvl >= 15 {
				n += 2
			}
			n++
			if lvl >= 12 {
				n++
			}
			if lvl >= 20 {
				n++
			}
			return n
		},
		DieSize: func(cl map[string]int) string {
			return "1d12"
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return scoutNinSuperiorityDice("a", cl["class/scout-nin"])
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinSuperiorityDieSize("arbiter-scout", cl["class/scout-nin"])
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Arbiter Scout's Battlefield Judication (3rd level): "Once per turn,
	// when you would take the attack action, in place of one of your
	// attacks, you order one friendly creature... to cast a Jutsu that
	// requires an attack roll, make a weapon attack, take a Skill-Action,
	// Dodge, or disengage... You can command a creature in this way twice
	// per initiative roll." A per-encounter cadence ("per initiative roll")
	// that doesn't map onto any of this app's rest tiers — same regenNone-
	// on-every-tier, player-resets-by-hand pattern already used for
	// feat/hatake/purple-lightning and clan/keton/feature/energy-overflow.
	// Only the twice-per-initiative-roll command use-count is tracked;
	// choosing and resolving the ordered creature's actual action (jutsu
	// cast, weapon attack, Skill-Action, Dodge, or disengage) stays fully
	// manual/narrated. The feature's separate Superiority-Die-spend clause
	// (adding a die's result to any friendly attack roll) is already covered
	// by the shared "superiority_dice" pool above and isn't duplicated here.
	"class/scout-nin/group/scouting-technique/arbiter-scout/feature/battlefield-judication": {
		Key:      "battlefield_judication_command",
		Name:     "Battlefield Judication (Command)",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenNone,
		Restriction: "Order a friendly creature to cast a Jutsu, make a weapon attack, take a Skill-Action, Dodge, or disengage, in place of one of your own attacks; resets each new encounter (no automatic rest-tier regen — reset by hand)",
	},
	"class/scout-nin/group/scouting-technique/assault-scout/feature/superior-assault": {
		Key:      "superiority_dice",
		Name:     "Superiority Dice",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Bonus, per long rest." Phantasmic Power's own die-roll mechanism (what
	// gets rolled in the first place) is now surfaced as a DieSize readout
	// (scoutNinPhantasmicPowerDieSize, scout_nin.go) — rolling it and every
	// downstream effect that consumes it (Phantom's Power's own
	// advantage-attack bonus, Phantasm's Grip, Ethereal Snap) stay fully
	// manual/narrated; only the uses-count and the die-size readout are
	// tracked here.
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/ghastly-leech": {
		Key:      "ghastly_leech",
		Name:     "Ghastly Leech",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		DieSize: func(cl map[string]int) string {
			return scoutNinPhantasmicPowerDieSize(cl["class/scout-nin"])
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Base class Tireless (11th level, post-migration-0051 slug — see that
	// migration's own comment for why the row used to ship as
	// "tireless-11-th-level"): "Once per long rest you can spend a full turn
	// Action recollecting yourself. When you do, you recover all spent
	// Superiority Die that you would normally recover on a short rest."
	// Only the once-per-long-rest use-count is tracked; actually restoring
	// the Superiority Dice pool by that amount stays a manual edit on the
	// existing Superiority Dice tile.
	"class/scout-nin/feature/tireless": {
		Key:      "tireless",
		Name:     "Tireless",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend a full turn Action to recover all spent Superiority Dice, as if taking a short rest",
	},
	// Arbiter Scout's Who Decided That!? (20th level), self-only half:
	// "...when you would fail a saving throw of any kind, you instead pass,
	// suffering no additional effects twice per short rest." The feature's
	// separate per-ally, once-per-long-rest, Superiority-Die-costing clause
	// is a per-target cap this app doesn't track (same boundary War Cry's
	// own self-vs-per-target split already draws against Commanding
	// Presence) and is not part of this entry.
	"class/scout-nin/group/scouting-technique/arbiter-scout/feature/who-decided-that": {
		Key:      "who_decided_that",
		Name:     "Who Decided That!?",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Instead of failing a saving throw, pass with no additional effects (self only — does not cover the separate per-ally, once-per-long-rest, Superiority-Die-costing clause, tracked manually)",
	},
	// Assault Scout's Brutish Durability [Changed] (3rd level): "...once per
	// short rest, whenever you make a saving throw, you may add half a
	// Superiority Die to the result. This does not spend the die. If you
	// used this feature again before completing a short rest, you may spend
	// a Superiority Die to activate this feature an additional time." A
	// second use before the next short rest costs an actual Superiority
	// Die, tracked manually on that pool instead — this entry only covers
	// the one free use. Applying the bonus to an actual roll, and the
	// natural-20 death-save interaction, stay manual.
	"class/scout-nin/group/scouting-technique/assault-scout/feature/brutish-durability-changed": {
		Key:      "brutish_durability",
		Name:     "Brutish Durability",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Add half a Superiority Die to a saving throw without spending it; a death save pushed to 20+ this way counts as a natural 20",
	},
	// Barrier Scout's Rallying Barrier (9th level): "...As a Bonus Action,
	// you can select up to 3 allied creatures within 30 feet of you...
	// Selected allies gain temporary hit points equal to the result of your
	// Superiority Die + your character level... You can use this feature
	// twice per rest." Selecting the allies and applying the rolled THP
	// stays manual — no ally/companion THP mechanism exists in this app.
	"class/scout-nin/group/scouting-technique/barrier-scout/feature/rallying-barrier": {
		Key:      "rallying_barrier",
		Name:     "Rallying Barrier",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: grant up to 3 allies THP equal to your Superiority Die roll + character level, lasting 10 minutes",
	},
	// Barrier Scout's Superior Shielding (20th level): "...As an action, you
	// may select up to 3 creatures you can see within 60 feet of you that
	// you are allied with. Each creature gains a chakra barrier with hit
	// points equal to your Scout-Nin level... You can use this feature
	// twice per long rest." Granting/tracking each ally's own barrier stays
	// manual — no ally-owned-resource mechanism exists, same boundary
	// Absolute Authority's own ally-AC clause already draws
	// (internal/features/grants.go's flatACBonusGrants only models the
	// caster's own half).
	"class/scout-nin/group/scouting-technique/barrier-scout/feature/superior-shielding": {
		Key:      "superior_shielding",
		Name:     "Superior Shielding",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: grant up to 3 allies a chakra barrier with HP equal to your Scout-Nin level",
	},
	// Cloning Scout's Clone Tactics (14th level — distinct from the 3rd-
	// level "Cloning Tactics" feature, which already grants the base
	// Superiority Dice pool above): "...when you would take damage from an
	// attack or fail a saving throw from a jutsu that deals damage, you can
	// spend 1 Superiority Die as a reaction... Alternatively, when one of
	// your clones dies you may spend your reaction... You can only summon
	// clones using either method of this feature twice per rest." Which
	// trigger fired and the actual clone summon/positioning stay manual;
	// overflow uses beyond 2 cost a Superiority/Chakra Die, tracked
	// manually on that pool.
	"class/scout-nin/group/scouting-technique/cloning-scout/feature/clone-tactics": {
		Key:      "clone_tactics_reaction",
		Name:     "Clone Tactics",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction: summon clones via a Ninjutsu with the Clone keyword, or add a clone via Clone Technique, without spending a Superiority Die",
	},
	// Phantom Scout's Ghost Walk (9th level): "...once you use this feature
	// once, you cannot use it again until you complete a Long Rest. If you
	// attempted to use this feature again you must spend a Superiority Die
	// each time." The spectral-form state itself (flying speed, phasing,
	// attack disadvantage against you) has no sheet field and stays fully
	// narrated — only the activation count is tracked.
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/ghost-walk": {
		Key:      "ghost_walk",
		Name:     "Ghost Walk",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spectral form for 10 minutes or until ended as a Bonus Action; overflow uses beyond the free ones cost a Superiority Die",
	},
	// Phantom Scout's Poltergeist (20th level): "You can now use Ghost Walk
	// twice per short rest." Shares Key "ghost_walk" with the entry above so
	// only one tile ever renders per character — the higher-Max grant
	// (this one) also supplies the winning regen kind once Poltergeist is
	// granted, same merge rule White Chakra Surge already establishes for
	// stacking onto the base Hatake grant.
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/poltergeist": {
		Key:      "ghost_walk",
		Name:     "Ghost Walk",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spectral form for 10 minutes or until ended as a Bonus Action; overflow uses beyond the free ones cost a Superiority Die",
	},
	// Phantom Scout's Ethereal Snap (17th level): "...You can only call upon
	// this power twice per long rest. A creature who passes their saving
	// throw does not spend a use of this feature." The damage/condition
	// application and the passed-save refund judgment call stay manual.
	"class/scout-nin/group/scouting-technique/phantom-scout/feature/ethereal-snap": {
		Key:      "ethereal_snap",
		Name:     "Ethereal Snap",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Melee hit while under Ghost Walk: force a Charisma save or deal Chakra+Necrotic damage equal to twice your Phantasmic Power and inflict 3 ranks of Concussed. A passed save does not spend a use (self-tracked)",
	},
	// Base class Jack of All, Master of None (5th level): "...Alternatively,
	// you can spend one Superiority Die from your Scout-Nin Subclass to
	// switch the Generalization you are benefiting from. You can only
	// switch in this way, once per short rest." The picker itself
	// (charstore.AddScoutNinPick/RemoveScoutNinPick) stays freely
	// re-editable at any time regardless of this pool, so this is purely a
	// self-reported informational counter, not an enforced gate — same
	// "trust the player" boundary every other cap+catalog pick in this app
	// already draws.
	"class/scout-nin/feature/jack-of-all-master-of-none": {
		Key:      "jack_of_all_switch",
		Name:     "Jack of All Switch",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend a Superiority Die to switch your active Generalization outside of a rest (tracked separately from the Superiority Dice pool itself, which is spent manually)",
	},
	"class/science-nin/feature/chakra-containment-device": {
		Key:      "ccd",
		Name:     "Chakra Containment Device",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Secondary Storage Device: "You gain an additional 5 CCD chakra per
	// Science-Nin level." Recomputes the full base total (scienceNinCCDMax)
	// plus 5 per Science-Nin class level, so the higher-Max-wins merge
	// picks it up automatically. No MinLevel needed -- the feat's own
	// prerequisite (4+ levels in Science-Nin) already guarantees a nonzero
	// class/science-nin level.
	"feat/class/secondary-storage-device": {
		Key:  "ccd",
		Name: "Chakra Containment Device",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return scienceNinCCDMax(cl["class/science-nin"]) + 5*cl["class/science-nin"]
		},
		ShortRegen:  regenNone,
		LongRegen:   regenHalfMax,
		FullRegen:   regenFull,
		Restriction: "Powers Scientific Ninja Tools and Science-Nin features",
	},
	// Scientist Training: "It can instead hold your level x 5, in CCD
	// Chakra." The feat's own prerequisite chain ("You cannot have class
	// levels in Science-Nin") guarantees cl["class/science-nin"] is always
	// 0, so this must read the character's TOTAL level, not a per-class
	// level. characterLevel(cl) (summing every class the character has)
	// already equals the character's total level (charsheet.Sheet.Level is
	// itself defined as the sum of character_classes.levels, the same rows
	// classLevels is built from), so no extra sentinel entry needs to be
	// threaded into the shared classLevels map to get it -- doing so would
	// corrupt every OTHER grant in this file that also calls
	// characterLevel(cl) to derive total character level, since that
	// helper sums every value present in whatever map it's given.
	"feat/class/scientist-training": {
		Key:  "ccd",
		Name: "Chakra Containment Device",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return characterLevel(cl) * 5
		},
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Scientist Enthusiast: "You gain the Calculated Response feature." The
	// feat's own prerequisite chain (scientist-training -> scientist-expert
	// -> scientist-enthusiast) never grants real Science-Nin class levels,
	// so class/science-nin level is always 0 for a holder -- loadCustomResources
	// has no class-tab gate of its own (only each grant's MinLevel, checked
	// against total character level), so this needs no gate-bypass work.
	// Infused Genius (the feat's other grant) now has its own cap+catalog
	// pick and Infused Tools pool — see "class/science-nin/feature/
	// infused-genius" below — but that entry is gated on the BASE class
	// feature's own slug, which a feat-only holder (no real Science-Nin
	// levels, per the prerequisite chain above) never reaches. Mirroring it
	// onto this feat slug, the same way Calculated Response is mirrored
	// just below, remains out of scope: the base Scientific Ninja Tools
	// catalog Infused Genius restricts itself to (loadScienceNinTabData)
	// already returns nil entirely for a feat-only holder with zero real
	// Science-Nin levels, so mirroring only the resource pool here with no
	// reachable pick behind it would be a dangling, unusable resource.
	"feat/class/scientist-enthusiast": {
		Key:  "calculated_response",
		Name: "Calculated Response",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Pull a scroll holding one basic-quality toolkit with a single charge; lost after use or 10 minutes",
	},
	// "Starting at the 11th Level... Twice per long rest the holder of this
	// weapon or wearer of the armor can use the tool, as if it was wielded
	// by you, at no chakra cost. You can have a number of Infused Tools
	// equal to your Intelligence modifier." Modeled as a single Int-mod-
	// sized pool (2 x Intelligence modifier) approximating "2 uses per
	// tool, per long rest" as one shared total — there is no precedent
	// anywhere else in this codebase for a pool sized per-pick-instance
	// rather than per ability score/level/Proficiency Bonus, so this is a
	// deliberate approximation, not a literal per-tool 2-use counter.
	// WHICH tools are Infused (up to the Intelligence-modifier cap) is a
	// separate cap+catalog pick — see scienceNinInfusedGeniusFeatureSlug,
	// science_nin.go. Only the use-count budget is tracked here; which
	// weapon/armor a tool is attached to, and "the holder/wearer (not just
	// you) can use it," both stay fully manual/narrated — no item-slot
	// binding or other-creature action economy exists anywhere in this app.
	"class/science-nin/feature/infused-genius": {
		Key:      "infused_tools",
		Name:     "Infused Tools",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			if intMod < 0 {
				return 0
			}
			return 2 * intMod
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Use an Infused Tool at no chakra cost — shared budget approximating 2 uses per tool per long rest",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return cl["class/science-nin"] * 5
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenNone,
		Restriction: "Filled only when you choose to redirect a normal C.C.D gain into it instead; withdraw any amount as a bonus action while within 5 feet of your S.N.B",
	},
	// Storm Rider's Gravity Child (9th level): "...Action: spend 15 CCD
	// chakra to gain the benefits of Wind Friction Shatter for 5 rounds
	// without needing concentration..." twice per long rest. The 15 CCD
	// chakra cost is already tracked by the existing "ccd" pool (spent
	// manually, same as every other CCD-costed ability in this class).
	// Wind Friction Shatter's own effects, the darkvision/chakra-sight
	// grant, and the no-concentration clause all stay entirely manual —
	// only the twice-per-long-rest activation gate is tracked. This
	// feature's other clause (advantage on saves against forced movement/
	// being knocked prone) is a flat always-on trait with no field to land
	// in anywhere in this app and is not part of this entry.
	"class/science-nin/group/scientific-inquiry/storm-rider/feature/gravity-child": {
		Key:      "gravity_child",
		Name:     "Gravity Child",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: spend 15 CCD chakra to gain the benefits of Wind Friction Shatter for 5 rounds without needing concentration",
	},
	// Elemental Innovationist's Elemental Innovation (17th level), the
	// Overcharge half: "...once per long rest, you may Overcharge your
	// Exoskeleton by spending 20 chakras from your CCD as a bonus action."
	// The 20 CCD chakra cost is already tracked by the existing "ccd" pool.
	// The Overcharge buff's actual effects (temp HP, resistance, free
	// save-bonus uses, size/speed/jump increases) stay entirely manual.
	// This feature's OTHER clause — designating one known W.o.W as
	// Ascended — is tracked separately, as a cap+catalog pick restricted to
	// the character's own known W.o.W list: see
	// science_nin_subclasses.go's scienceNinElementalInnovationistData.
	// DesignatedWoW. Only WHICH W.o.W is designated Ascended is tracked
	// there; the halved ammo Creation-Point cost and the damage-ignores-
	// resistance/treats-immunity-as-resistance payload stay entirely
	// manual/narrated — no per-item cost-modifier or damage-type-override
	// concept exists anywhere in this app.
	scienceNinElementalInnovationFeatureSlug: {
		Key:      "elemental_innovation_overcharge",
		Name:     "Elemental Innovation: Overcharge",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: spend 20 CCD chakra to Overcharge your Exoskeleton",
	},
	// Mad Scientist's Future of Shinobi: Biology (20th level): "...Once per
	// Long rest you can cast the jutsu using CCD chakra with its cost
	// rounded up to the nearest interval of 10... you must use Mending/
	// Maiming CCD Chakra..." No cap+catalog picker exists anywhere in this
	// app for "pick any jutsu of rank <= X and category Y" (the class's own
	// known-jutsu lists are the only jutsu-selection mechanism built), so
	// which Medical Ninjutsu is designated stays entirely undocumented/
	// manual — only the once-per-long-rest cast gate is tracked. MinLevel
	// gates this independently of the row's own (currently NULL) DB level,
	// so it correctly applies only from 20th level regardless of that data
	// gap.
	"class/science-nin/group/scientific-inquiry/mad-scientist/feature/the-future-of-shinobi-biology": {
		Key:      "future_of_shinobi_biology",
		Name:     "The Future of Shinobi: Biology",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Once per long rest: cast your designated S-Rank-or-lower Medical Ninjutsu using CCD chakra (cost rounded up to the nearest 10, split across Mending/Maiming CCD per its effect)",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Hunters Training's own text: "Use Exploits twice per rest" — a fixed
	// count, not Proficiency-Bonus-derived like the real class feature
	// above. Mutually exclusive with real Hunter-Nin levels (the feat's own
	// prerequisite: "You cannot have class levels in Hunter-Nin"), so this
	// never competes with the class feature grant on a real character; the
	// "higher Max wins" merge rule only matters here for stacking against
	// Hunter Specialist's own bump below.
	huntersTrainingFeatSlug: {
		Key:      "hunter_exploits",
		Name:     "Hunters Exploits",
		MinLevel: 5, // Hunters Training's own prerequisite: Level 5+
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Hunter Specialist's own text: "Increase exploits usable per rest by
	// 1" — on top of Hunters Training's fixed 2 (Specialist requires Expert
	// requires Training as its own feat prerequisite chain, so a character
	// with this feat always has Training's grant too; this entry encodes
	// the cumulative total, 3, so the "higher Max wins" merge above picks
	// it over Training's 2 directly rather than needing real addition).
	hunterSpecialistFeatSlug: {
		Key:      "hunter_exploits",
		Name:     "Hunters Exploits",
		MinLevel: 15, // Hunter Specialist's own prerequisite: Level 15+
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 3
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Wolves Legacy's Wolf's Proficiency: "You can use any of your
	// Prosthetic Attachments a number of times equal to your Proficiency
	// Bonus per rest." The known-list itself (cap 2->4@10th, which of the 7
	// Prosthetic Attachments are known) is a separate cap+catalog picker
	// in cmd/n5e/hunter_nin.go, the same "pool here, known-list there"
	// split Hunters Exploits already establishes. Every attachment's own
	// activated effect (Elemental Vent's cone damage, Sabimaru's poison,
	// etc.) stays entirely manual — only the per-rest use-count is
	// tracked. MinLevel matches Wolf's Proficiency's own 3rd-level grant
	// sentence, even though the Prosthetic Attachments table itself is
	// mistagged onto the 7th-level "Eyes of a Shinobi" row in rules.db
	// (see prostheticAttachmentOptions' own doc comment).
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/wolfs-proficiency": {
		Key:      "prosthetic_attachments",
		Name:     "Prosthetic Attachments",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Necrotic Hand's Necrotic Touch: "You can only use this feature a
	// number of times equal to your Intelligence Modifier per long rest."
	// No stated floor — a 0 or negative Intelligence modifier genuinely
	// grants 0 uses per the book, unlike Calculated Response's own explicit
	// "(minimum of once)" clause. Both branches of the effect (a forced CON
	// save/cellular-degradation debuff, or spending a use to cast Necrosis
	// on an ally as healing) stay entirely manual — only the use-count is
	// tracked.
	"class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/necrotic-touch": {
		Key:      "necrotic_touch",
		Name:     "Necrotic Touch",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return intMod
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Grave Stalker's A Walking Shadow: "you can spend your Reaction to
	// blend into the very ether of shadows avoiding all damage... You can
	// avoid damage in this way up to twice per rest." The damage-negation
	// trigger itself stays manual; only the twice-per-rest charge is
	// tracked.
	"class/hunter-nin/group/hunters-creeds/grave-stalker/feature/a-walking-shadow": {
		Key:      "a_walking_shadow",
		Name:     "A Walking Shadow",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Arsenalist's Tools of the Trade: "Select one of the following when
	// you complete a rest of any type. You can use any of the below a
	// number of times equal to your Proficiency Bonus per rest." Which of
	// the 4 named tools (Caltrops/Ball Bearings/Johyo/Hidden Weapon
	// Launcher) is selected, and each tool's own triggered effect, stay
	// entirely manual/narrated (no picker exists for the active tool) —
	// only the shared use-count is tracked.
	"class/hunter-nin/group/hunters-creeds/arsenalist/feature/tools-of-the-trade": {
		Key:      "tools_of_the_trade",
		Name:     "Tools of the Trade",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Caltrops (bleed+mark), Ball Bearings, Johyo (pull), or Hidden Weapon Launcher — which tool is active is tracked by the player, not this app",
	},
	// Undertaker's Poisonous Embrace: "you create 2 vials of Assassins
	// Blood... increases to 3 vials and 4d6 at 7th level, 4 vials and 5d6
	// at 11th level and 6 vials and 6d6 at 17th level." Vials are created
	// on a short rest and go inert on any rest, matching the pool's
	// all-tiers-regenFull treatment. Applying the poison to a weapon and
	// its on-hit/save effect stay entirely manual — only the vial count
	// (and the damage-die readout) are tracked.
	"class/hunter-nin/group/hunters-creeds/undertaker/feature/poisonous-embrace": {
		Key:      "poisonous_embrace_vials",
		Name:     "Assassin's Blood Vials",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 17:
				return 6
			case lvl >= 11:
				return 4
			case lvl >= 7:
				return 3
			default:
				return 2
			}
		},
		DieSize: func(cl map[string]int) string {
			lvl := characterLevel(cl)
			switch {
			case lvl >= 17:
				return "6d6"
			case lvl >= 11:
				return "5d6"
			case lvl >= 7:
				return "4d6"
			default:
				return "3d6"
			}
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Vice Agent's Greed's Shell: "You can activate this protective armor a
	// number of times equal to your Proficiency Bonus per long rest." The
	// DR-vs-all-sources and the scaling temp-HP grant (10/20/30/40 at
	// 3rd/7th/10th/14th) both still have to be applied by hand — only the
	// use-count is tracked.
	"class/hunter-nin/group/hunters-creeds/vice-agent/feature/greeds-shell": {
		Key:      "greeds_shell",
		Name:     "Greed's Shell",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Vice Agent's Envious Theft: "You can store a number of jutsu this way
	// equal to your Charisma Modifier. A jutsu stored inside of you remains
	// until your next rest, before harmlessly dissipating." Unscoped "until
	// your next rest" — same all-tiers-regenFull treatment as Sealing
	// Arts/Vorpal Casting/A Walking Shadow/Poisonous Embrace below. The
	// Reaction-triggered steal check (an ability check vs 13 + the jutsu's
	// rank, contested against another creature's live cast) and casting the
	// stolen jutsu using the thief's own stats both stay entirely manual/
	// narrated — this app has no mechanism to intercept another creature's
	// cast; only the available-storage-slot COUNT is tracked.
	"class/hunter-nin/group/hunters-creeds/vice-agent/feature/envious-theft": {
		Key:      "envious_theft",
		Name:     "Envious Theft",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return cha
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Void Walker: 4 separate flat per-rest activation gates — the seal-
	// and-suppress, the auto-marking cast, the range-ignoring cast, and the
	// auto-crit teleport-attack all stay entirely manual/narrated; only
	// each feature's own per-rest AVAILABILITY is tracked. "Sealing Arts":
	// "twice per rest" (unscoped).
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/sealing-arts": {
		Key:      "sealing_arts",
		Name:     "Sealing Arts",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Void Walker's "Fuin-Field": "twice per Long Rest" specifically.
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/fuin-field": {
		Key:      "fuin_field",
		Name:     "Fuin-Field",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Void Walker's "Vorpal Casting": "Twice per rest" (unscoped).
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/vorpal-casting": {
		Key:      "vorpal_casting",
		Name:     "Vorpal Casting",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Void Walker's "Spatial Displacement" carries TWO independent caps in
	// its own text: "You can perform this action twice per rest" (the
	// teleport-attack, this primary entry) and, separately, "twice per
	// Long Rest, whenever you would be targeted by an attack" (the
	// reactive teleport, see customResourceSecondaryGrants below for the
	// second pool fed by this same slug).
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/spatial-displacement": {
		Key:      "spatial_displacement_teleport",
		Name:     "Spatial Displacement (Teleport Attack)",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Wolves Legacy's Eyes of a Shinobi: "You can tap into this well of
	// preternatural sensory ability a number of times equal to your Wisdom
	// ability modifier per rest." The first entry in this table that needs
	// Wisdom — see the Max field's own doc comment above for why that
	// parameter exists. The creature-sense/AC-reveal effect itself stays
	// entirely manual (no mechanism exists to surface an enemy's AC or
	// hidden presence on the sheet) — only the use-count is tracked.
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/eyes-of-a-shinobi": {
		Key:      "eyes_of_a_shinobi",
		Name:     "Eyes of a Shinobi",
		MinLevel: 7,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return wisMod
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Wolves Legacy's Kages Die Twice carries two independent once-per-
	// rest-tier revival charges in its own text: "Once per long rest, when
	// your hit points are reduced to 0, you can choose to slowly stand
	// back up..." (this primary entry) and, separately, "if you would die,
	// prior to using this ability, once per full rest, you can choose to
	// rise from the ether..." (see customResourceSecondaryGrants below).
	// The player must still notice reaching 0 HP or dying and manually
	// apply the HP/death-save/resistance effects and manually spend the
	// charge — only the once-per-rest-tier AVAILABILITY of each charge is
	// tracked.
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/kages-die-twice": {
		Key:      "kages_die_twice_stand_up",
		Name:     "Kages Die Twice (Stand Back Up)",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen: regenNone,
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return cl["class/puppet-master"]
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenNone,
		Restriction: "d8s, delegated among your Puppet Tools; they spend these in place of your own chakra dice",
	},
	// Blue Technique Warmaster's Masterful Movement, second half: "...The
	// latter half of this feature can be used twice per long rest." The
	// first half (no-save damage/half-on-fail) and the interpose/redirect
	// mechanic itself stay entirely manual — only the twice-per-long-rest
	// gate on the Reaction+jutsu-cast half is tracked.
	"class/puppet-master/group/puppet-techniques/blue-technique-warmaster/feature/masterful-movement": {
		Key:      "masterful_movement",
		Name:     "Masterful Movement",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction: interpose to become the new target of an attack on an ally within 15ft, optionally casting a jutsu/ability as part of it",
	},
	// Red Technique Performer's Overwhelming Might: "...You can use this
	// feature a number of times equal to your Proficiency Bonus per long
	// rest." The escalating DC-13(+2/puppet, cap +5) Chakra Control checks
	// and the attack/damage/DC bumps per extra puppet stay entirely manual —
	// only the activation count is tracked.
	"class/puppet-master/group/puppet-techniques/red-technique-performer/feature/overwhelming-might": {
		Key:      "overwhelming_might",
		Name:     "Overwhelming Might",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Red Technique Performer's Puppet Theatre, Role-swap half: "...You can
	// use this feature twice per long rest." (The feature's OTHER clause, a
	// Reaction Performance check to copy an observed jutsu, has no use-count
	// language and is not part of this gate.) Which Puppet Role is actually
	// swapped to, the 1-minute duration/cancel-as-free-action, and the
	// separate Reaction-based jutsu-mimicry clause all stay narrated — only
	// the twice-per-long-rest swap count is tracked.
	"class/puppet-master/group/puppet-techniques/red-technique-performer/feature/puppet-theatre": {
		Key:      "puppet_theatre",
		Name:     "Puppet Theatre",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Purple Technique Juggernaut's Armorer's Eye: "...You can do this a
	// number of times equal to your Proficiency Bonus per long rest." The
	// Perception check vs. DC (5 + 2x armor bulk, or 3x jutsu rank), and the
	// tiered success effects (ignore armor bonus; +1 crit range at +5;
	// unreactable at +10) all stay manual/narrated.
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/armorers-eye": {
		Key:      "armorers_eye",
		Name:     "Armorer's Eye",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Purple Technique Juggernaut's Brawny Engineering: a once-per-long-rest
	// availability flag on "when you equip your Juggernaut Armor: gain THP
	// equal to half your Puppet Tool's current HP; for every 1 THP lost this
	// way, your Puppet Tool loses 1 HP (not restorable)." Computing half the
	// puppet's HP, applying the THP, and draining the puppet 1:1 as it's
	// spent all stay entirely manual/player-tracked — only the once-per-
	// long-rest availability is tracked.
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/brawny-engineering": {
		Key:      "brawny_engineering",
		Name:     "Brawny Engineering",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "When you equip your Juggernaut Armor: gain THP equal to half your Puppet Tool's current HP; for every 1 THP lost this way, your Puppet Tool loses 1 HP (not restorable)",
	},
	// Base class Always Prepared (15th level), the quality-tier conversion
	// half: "...You can convert... twice per full rest." Which item is
	// converted, and to what tier, stays entirely manual (no quality-tier
	// field exists on any inventory item in this app) — only the twice-per-
	// full-rest gate is tracked. Distinct from the same feature's own 18th-
	// level temporary Upgrade clause, which has its own separate
	// rest-re-pickable picker (loadAlwaysPreparedUpgradeOptions,
	// puppets.go) — a real, freely re-editable pick, not a spendable-charge
	// pool, so it needs no entry of its own in this map.
	"class/puppet-master/feature/always-prepared": {
		Key:      "always_prepared_conversion",
		Name:     "Always Prepared (Kit/Pill Conversion)",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenNone,
		FullRegen:   regenFull,
		Restriction: "Convert up to 1 charge of a Toolkit/Medical Pill to one quality tier higher (or split into 2 uses of one tier lower)",
	},
	// Base class Resourceful Tactics (2nd level, via Tactics of the Craft;
	// its own class_features row is tagged level 11 because that row's text
	// bundles both the base grant and the 11th-level upgrade together):
	// "...You can also use Explosive Tools as a Bonus Action, a number of
	// times equal to your Intelligence Modifier per long rest. At 11th
	// level, this becomes every short rest." ShortRegen/LongRegen/FullRegen
	// are static per entry, not level-gated (same boundary Yamanaka's
	// Formation Casting already accepts for its own level-15 short-rest
	// upgrade), so the 11th-level switch to short-rest recovery is
	// documented in Restriction only, not modeled mechanically. This slug is
	// one of puppetMasterTacticSlugs (characters.go) — unconditionally
	// excluded from loadGrantedFeatures' output so all 5 Tactics don't get
	// blanket-granted with no player choice — so a bare entry here would
	// never fire for anyone; see loadCustomResources' own
	// puppetResourcefulTacticsPickedRows call, which re-adds this exact slug
	// only for a character who actually picked Resourceful Tactics. Using
	// Explosive Tools and its actual effect stay entirely narrated — only
	// the per-rest use-count is tracked.
	"class/puppet-master/feature/resourceful-tactics": {
		Key:      "resourceful_tactics_explosive_tools",
		Name:     "Explosive Tools (Resourceful Tactics)",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			if intMod < 0 {
				return 0
			}
			return intMod
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: use Explosive Tools. From 11th level, also regains uses on a Short Rest, not just a Long Rest (not modeled below).",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Illusionist Training/Expert/Specialist ("You gain 2 Actualization
	// Die... You gain 1 additional Actualization Die" x2) let a character
	// with no real Genjutsu Specialist levels still have this pool — same
	// "actualization_die" Key as the base class grant above, so the
	// higher-Max-wins merge rule (computeCustomResources) picks whichever
	// is highest. Each entry's own Max is the CUMULATIVE total through
	// that tier (2, then 3, then 4), not just its own increment, since
	// Expert/Specialist both require the previous tier as their own feat
	// prerequisite (see genjutsuArchetypeFeats, genjutsu.go) — a real
	// Genjutsu Specialist's own Proficiency-Bonus-sized pool overtakes
	// these once it exceeds 4 (Proficiency Bonus +4, i.e. 13th level and
	// up), which the higher-Max-wins rule also handles for free. MinLevel
	// mirrors each feat's own stated character-level prerequisite (5th/
	// 10th/15th), not a Genjutsu Specialist class level — these feats
	// require no class levels at all.
	illusionistTrainingFeatSlug: {
		Key:      "actualization_die",
		Name:     "Actualization Die",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	illusionistExpertFeatSlug: {
		Key:      "actualization_die",
		Name:     "Actualization Die",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 3
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	illusionistSpecialistFeatSlug: {
		Key:      "actualization_die",
		Name:     "Actualization Die",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 4
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Force a creature to remain under a Genjutsu you cast despite a
	// remade successful save." The 15th-level upgrade to short-rest
	// recovery ("this also regains on a Short Rest") has no field to land
	// in (ShortRegen/LongRegen/FullRegen are static per entry), same
	// boundary clan/yamanaka/feature/formation-casting-new already draws
	// for an identical single-row "upgrades at a later level" shape, so
	// it's documented in Restriction only. Forcing the save outcome itself
	// stays fully manual/narrated — only the once-per-rest use-count is
	// tracked.
	"class/genjutsu-specialist/feature/the-turn": {
		Key:      "the_turn",
		Name:     "The Turn",
		MinLevel: 11,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Force a creature to remain under a Genjutsu you cast despite a remade successful save; from 15th level this also regains on a Short Rest (not modeled below)",
	},
	// "Once per rest" — unscoped, same all-tiers-regenFull treatment as
	// Chakra Disruption/Before All. Forcing the automatic critical failure
	// stays manual/narrated; only the once-per-rest gate is tracked.
	"class/genjutsu-specialist/feature/the-prestige": {
		Key:      "the_prestige",
		Name:     "The Prestige",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Force an automatic critical failure on a save against a Genjutsu you cast",
	},
	// Real World Conversion's 5 options (Actualized Alteration/Duplicity/
	// Perception/Perfection/Power) are all NULL-level class_features rows,
	// so LoadGrantedFeatures grants every one of them unconditionally to
	// every Genjutsu Specialist 5+ regardless of which the player actually
	// picked via charstore.ListGenjutsuPicks(..., GenjutsuPickConversion) —
	// the same shape puppetMasterTacticSlugs excludes Puppet Master's 5
	// Tactics for. Only Actualized Alteration carries its own extra
	// twice-per-long-rest cap on top of its Actualization Die spend (the
	// other 4 Conversions have no separate cap of their own), so this
	// entry is deliberately keyed to a SYNTHETIC key
	// ("genjutsu-pick/actualized-alteration") that can never collide with
	// the real rules-database slug — loadCustomResources appends a
	// synthetic grantedFeatureRow under this key only when the character
	// has actually picked Actualized Alteration (see there). Spending 1
	// Actualization Die to change a Genjutsu's required save stays
	// entirely manual — only the twice-per-long-rest cap on doing so is
	// tracked.
	"genjutsu-pick/actualized-alteration": {
		Key:      "actualized_alteration",
		Name:     "Actualized Alteration",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend 1 Actualization Die to change a Genjutsu's required save to Int/Wis/Cha",
	},
	// Beguiler's Beguiling Presence: "You can do this twice per rest. You
	// gain an additional use of this feature at 10th level." Actually
	// becoming invisible stays manual/narrated — no invisibility mechanism
	// exists in this app; only the use-count is tracked.
	"class/genjutsu-specialist/group/genjutsu-pledges/beguiler/feature/beguiling-presence": {
		Key:      "beguiling_presence",
		Name:     "Beguiling Presence",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			n := 2
			if cl["class/genjutsu-specialist"] >= 10 {
				n++
			}
			return n
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Become invisible to a chosen creature (or Genjutsu-ability-modifier-many creatures) currently affected by your Visual-keyword Genjutsu, until the end of your next turn",
	},
	// Beguiler's Beyond Sight: "You can use this feature twice per rest."
	// The overflow clause (spend 1 Actualization Die for an additional
	// use past the cap) needs no separate mechanism — it already spends
	// from the existing, separately-tracked Actualization Die pool by
	// hand.
	"class/genjutsu-specialist/group/genjutsu-pledges/beguiler/feature/beyond-sight": {
		Key:      "beyond_sight",
		Name:     "Beyond Sight",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Ignore an effect that would let a creature auto-succeed against your Visual-keyword Genjutsu (true sight, tremorsense, etc.)",
	},
	// Beguiler's Beguiling Influence: "Twice per Long Rest, but no more
	// than once per casting... spend 2 Actualization Die." The "no more
	// than once per casting" sub-restriction and the save-lock effect stay
	// informational/manual; the 2-Actualization-Die spend per use is
	// already covered by the existing, separately-tracked Actualization
	// Die pool — only the twice-per-long-rest activation gate is tracked.
	"class/genjutsu-specialist/group/genjutsu-pledges/beguiler/feature/beguiling-influence": {
		Key:      "beguiling_influence",
		Name:     "Beguiling Influence",
		MinLevel: 18,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend 2 Actualization Die to prevent a creature from remaking saves against a Genjutsu it failed against; no more than once per casting",
	},
	// Corrupt Thoughts' Nightmare Incarnates: "twice per rest... additional
	// use of this feature at 10th and 13th levels... beginning at 13th
	// level you can cast a B-rank or lower genjutsu." Which known Genjutsu
	// is cast, and enforcing the C/B-Rank ceiling, stay manual/
	// informational.
	"class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts/feature/nightmare-incarnates": {
		Key:  "nightmare_incarnates",
		Name: "Nightmare Incarnates",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			gl := cl["class/genjutsu-specialist"]
			n := 2
			if gl >= 10 {
				n++
			}
			if gl >= 13 {
				n++
			}
			return n
		},
		MinLevel:    6,
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast a C-Rank (B-Rank from 13th) or lower damaging Genjutsu with a 1-action casting time as a Bonus Action",
	},
	// Illusionist's Illusionary Fury: "Once you use Illusionary Fury in
	// this way you cannot do so again until you finish a long rest." The
	// keyword swap and duration/condition-rank doubling stay manual; the
	// Actualization Die spend it also requires is already tracked by the
	// existing pool.
	"class/genjutsu-specialist/group/genjutsu-pledges/illusionist/feature/illusionary-fury": {
		Key:      "illusionary_fury",
		Name:     "Illusionary Fury",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend an Actualization Die to swap keywords on, or double the duration/condition-rank of, a qualifying Genjutsu",
	},
	// Illusionist's Illusionary Rage: "Once you use this feature twice, you
	// cannot do so again until you complete a long rest." Spreading the
	// condition, and which Genjutsu was selected, stay manual/narrated (no
	// downstream field to write the selection into either way).
	"class/genjutsu-specialist/group/genjutsu-pledges/illusionist/feature/illusionary-rage": {
		Key:      "illusionary_rage",
		Name:     "Illusionary Rage",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: inflict 1 rank of the selected Genjutsu's Mental Condition on hostile creatures within 5 feet of the original target",
	},
	// Layered Reality's Split Focus: "You may do this twice per short
	// rest." The feature's second clause (flat -1/-2/-3 concentration-cost
	// reduction) stays permanently un-modeled — no concentration-limit/
	// concentration-cost field exists anywhere in this app; only the
	// twice-per-short-rest exemption gate is tracked.
	"class/genjutsu-specialist/group/genjutsu-pledges/layered-reality/feature/split-focus": {
		Key:      "split_focus",
		Name:     "Split Focus",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast a Genjutsu requiring concentration without it counting against your Concentration limit (one at a time)",
	},
	// Layered Reality's Master of Reality: "A bonus Genjutsu can only be
	// cast this way twice per long rest." Casting the bonus Genjutsu and
	// which one is chosen stay manual.
	"class/genjutsu-specialist/group/genjutsu-pledges/layered-reality/feature/master-of-reality": {
		Key:      "master_of_reality",
		Name:     "Master of Reality",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast a second concentration Genjutsu (1-action casting time) as part of the same action as your first",
	},
	// Time Slipper's Misread Clocks: "You can gain this benefit twice per
	// long rest." The Reaction-denial effect and the follow-up 1d10
	// Reaction-miss check against allies' effects stay manual/narrated.
	"class/genjutsu-specialist/group/genjutsu-pledges/time-slipper/feature/misread-clocks": {
		Key:      "misread_clocks",
		Name:     "Misread Clocks",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: deny a Reaction to a creature affected by a save-requiring Genjutsu you cast",
	},
	// Time Slipper's Temporal Mastery: "Twice per Rest, when you would gain
	// the benefit of the A Moment in Time feature, you can spend an
	// additional 1 Actualization Die..." Which additional A Moment in Time
	// benefit is gained stays manual; the underlying A Moment in Time
	// Actualization-Die spend is already covered by the shared
	// Actualization Die pool.
	"class/genjutsu-specialist/group/genjutsu-pledges/time-slipper/feature/temporal-mastery": {
		Key:      "temporal_mastery",
		Name:     "Temporal Mastery",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend an additional Actualization Die when using A Moment in Time to gain an additional effect",
	},
	// Siren's Words of Detriment: "You can only use this feature twice per
	// long rest." The Weakened/Slowed condition application, its
	// repeat-save loop, and the stacking -1-per-fail penalty (capped at
	// -5) all stay entirely manual/narrated — no condition-tracking or
	// escalating-save-penalty mechanism exists in this app; only the
	// twice-per-long-rest activation gate is tracked. Siren's other higher-
	// level features are correctly excluded: Words of Affirmation is
	// per-recipient not per-caster, Words of Repentance is a per-attacker
	// Reaction cooldown, and Sirens Influence only spends the
	// already-tracked Actualization Die with no separate cap.
	"class/genjutsu-specialist/group/genjutsu-pledges/siren/feature/words-of-detriment": {
		Key:      "words_of_detriment",
		Name:     "Words of Detriment",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: swap a Sensory-keyword Genjutsu's keywords for Auditory, inflicting Weakened/Slowed on a failed Wisdom save",
	},
	// Malleable Mirages: ~31 of the catalog's 79 entries carry their own
	// printed rest-scoped use-limit on top of the base known-Mirage pick
	// (MiragesCap/KnownMirages/AvailableMirages, genjutsu.go) — e.g. "you
	// can cast X at no cost... once per rest." Every one of these also
	// prints the identical fallback clause, "if you have already reached
	// this Mirage's use limit, you can choose to cast the jutsu this
	// Mirage provides by spending N Chakra die" — that fallback needs no
	// separate mechanism, since it already spends from the existing,
	// separately-tracked Chakra Die pool by hand. For every entry below:
	// casting the named jutsu/effect itself stays entirely manual/
	// narrated — only the printed per-rest use-count is tracked. Keyed off
	// the synthetic "genjutsu-mirage/<slug>" rows genjutsuMiragePickedRows
	// (genjutsu.go) emits, never the real class_options slug (which never
	// reaches this table at all — see that function's own doc comment).
	//
	// Regen-tier reasoning: entries whose own text names "short rest"
	// specifically regen in full on every tier, exactly like the unscoped
	// group below — a Long Rest, being strictly the more restful option,
	// also recovers whatever a Short Rest would, the same treatment
	// Layered Reality's own "twice per short rest" Split Focus (just
	// above) already gets. Entries naming "long rest" specifically do NOT
	// regen on a mere Short Rest (ShortRegen: regenNone), mirroring
	// "the-turn"'s own ShortRegen:regenNone/LongRegen:regenFull split.
	// Entries with unscoped "per rest"/"short or long rest" text regen in
	// full on every tier, mirroring "the-prestige". Deceitful Duplicate is
	// deliberately absent from this block — its own Genjutsu-Ability-
	// Modifier-sized pool is genjutsuDeceitfulDuplicatePool below instead
	// of a flat entry here, the same reason Visceral Language isn't a flat
	// entry either.

	// Short-rest-scoped text.
	"genjutsu-mirage/accelerate": {
		Key:  "mirage_accelerate",
		Name: "Accelerate",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: up to 3 willing creatures within 15 feet can Bonus-Action Dash for a minute, gaining advantage on their next Dexterity save",
	},
	"genjutsu-mirage/critical-confusion": {
		Key:      "mirage_critical_confusion",
		Name:     "Critical Confusion",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Force a creature that would critically succeed a save against your Genjutsu to reroll it once",
	},
	"genjutsu-mirage/chained-mind": {
		Key:      "mirage_chained_mind",
		Name:     "Chained Mind",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Mind Spike at no cost as if you know it",
	},
	"genjutsu-mirage/dulled-mind": {
		Key:      "mirage_dulled_mind",
		Name:     "Dulled Mind",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Slow at no cost as if you know it",
	},
	"genjutsu-mirage/song-of-the-end": {
		Key:  "mirage_song_of_the_end",
		Name: "Song of the End",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Ringing Bell Distortion at no cost (Ninja Tool and Weapon components only), stacking with an existing casting of it",
	},
	"genjutsu-mirage/thieving-confidence": {
		Key:  "mirage_thieving_confidence",
		Name: "Thieving Confidence",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Ineptitude at no cost as if you know it",
	},
	"genjutsu-mirage/psyche-drinker": {
		Key:  "mirage_psyche_drinker",
		Name: "Psyche Drinker",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 3
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: convert Genjutsu damage you just dealt into temporary hit points until the end of your next turn",
	},

	// Long-rest-scoped text — a mere Short Rest does not recover these.
	"genjutsu-mirage/beneath-the-boot": {
		Key:      "mirage_beneath_the_boot",
		Name:     "Beneath the Boot",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Psionics: Crush! at no cost, targeting up to 3 additional creatures",
	},
	"genjutsu-mirage/chains-of-madness": {
		Key:      "mirage_chains_of_madness",
		Name:     "Chains of Madness",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Tree Binding Death at no cost as if you know it",
	},
	"genjutsu-mirage/dreadful-word": {
		Key:      "mirage_dreadful_word",
		Name:     "Dreadful Word",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Effortless Paralysis at no cost as if you know it",
	},
	"genjutsu-mirage/freedom-of-frenzy": {
		Key:      "mirage_freedom_of_frenzy",
		Name:     "Freedom of Frenzy",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Frenzy: Burst at no cost, targeting a second creature",
	},
	"genjutsu-mirage/magnum-opus": {
		Key:      "mirage_magnum_opus",
		Name:     "Magnum Opus",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Bringer of Darkness or Geas at no cost, once",
	},
	"genjutsu-mirage/superior-thoughts": {
		Key:      "mirage_superior_thoughts",
		Name:     "Superior Thoughts",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Full-Turn Action: cast an S-Rank Genjutsu you qualify for at no cost, gaining 3 ranks of Confused and Concussed",
	},
	"genjutsu-mirage/tentative-escape": {
		Key:  "mirage_tentative_escape",
		Name: "Tentative Escape",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction while under the effects of a Genjutsu: cast Release",
	},

	// Unscoped "per rest" / "short or long rest" text.
	"genjutsu-mirage/ascendant-image": {
		Key:  "mirage_ascendant_image",
		Name: "Ascendant Image",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Haze Clone at no cost as if you know it",
	},
	"genjutsu-mirage/battle-ready-minds": {
		Key:      "mirage_battle_ready_minds",
		Name:     "Battle Ready Minds",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Bless at no cost",
	},
	"genjutsu-mirage/berserk-thoughts": {
		Key:      "mirage_berserk_thoughts",
		Name:     "Berserk Thoughts",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Shadow Monsters at no cost as if you know it; a creature that injures an ally while affected suffers a 1d4 saving-throw penalty",
	},
	"genjutsu-mirage/bewitching-whispers": {
		Key:  "mirage_bewitching_whispers",
		Name: "Bewitching Whispers",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Charming Dissonance at no cost",
	},
	"genjutsu-mirage/black-fangs": {
		Key:  "mirage_black_fangs",
		Name: "Black Fangs",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Shadow Bite at no cost as if you know it, at the highest rank you can cast",
	},
	"genjutsu-mirage/blade-song": {
		Key:      "mirage_blade_song",
		Name:     "Blade Song",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Dancing Blades at no cost as if you know it, gaining 2 additional illusionary weapon copies",
	},
	"genjutsu-mirage/blinding-lights": {
		Key:  "mirage_blinding_lights",
		Name: "Blinding Lights",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Color Spray at no cost as if you know it",
	},
	"genjutsu-mirage/book-of-stolen-secrets": {
		Key:  "mirage_book_of_stolen_secrets",
		Name: "Book of Stolen Secrets",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Compel a truthful written answer from a creature that fails a Wisdom save against your Genjutsu save DC",
	},
	"genjutsu-mirage/evil-thoughts": {
		Key:  "mirage_evil_thoughts",
		Name: "Evil Thoughts",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Mind Crunch at no cost, +1 damage die and a -3 AC penalty on a critical failure",
	},
	"genjutsu-mirage/ghastly-mist": {
		Key:      "mirage_ghastly_mist",
		Name:     "Ghastly Mist",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Fog of War at half cost as if you know it, cube increased to 45 feet",
	},
	"genjutsu-mirage/illusionary-brawler": {
		Key:  "mirage_illusionary_brawler",
		Name: "Illusionary Brawler",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Psionics: Strike! on yourself at no cost, foregoing chakra to maintain concentration and gaining a Melee Genjutsu unarmed-attack option",
	},
	"genjutsu-mirage/misty-thoughts": {
		Key:  "mirage_misty_thoughts",
		Name: "Misty Thoughts",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Cajolery of Glamour at no cost as if you know it",
	},
	"genjutsu-mirage/misty-visions": {
		Key:      "mirage_misty_visions",
		Name:     "Misty Visions",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Innocuous Aspect at half cost as if you know it, breakable only by physical inspection",
	},
	"genjutsu-mirage/no-light-at-the-end": {
		Key:      "mirage_no_light_at_the_end",
		Name:     "No Light At the End",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Word of The Lost at half cost as if you know it, reducing its fracture requirement by 1",
	},
	"genjutsu-mirage/pause": {
		Key:  "mirage_pause",
		Name: "Pause",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: cast Psionics: Pause at no cost",
	},
	"genjutsu-mirage/shaken-up": {
		Key:  "mirage_shaken_up",
		Name: "Shaken Up",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Startle at no cost as if you know it",
	},
	"genjutsu-mirage/shroud-of-shadows": {
		Key:      "mirage_shroud_of_shadows",
		Name:     "Shroud of Shadows",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast Darkness as if it were Genjutsu & Visual instead of Ninjutsu",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Batch Cook: "You gain an additional number of Snacks equal to 1/3rd
	// your proficiency modifier, after a rest." No separate "proficiency
	// modifier" stat exists distinct from Proficiency Bonus (see this
	// file's own header comment on Hunters Exploits), so prof is the
	// correct value. Recomputes the full base formula (prof+intMod, clamped
	// at 0) plus floor(prof/3), so the higher-Max-wins merge picks it up
	// automatically.
	"feat/class/batch-cook": {
		Key:      "shinobi_snacks",
		Name:     "Shinobi Snacks",
		MinLevel: 8,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			n := prof + intMod
			if n < 0 {
				n = 0
			}
			return n + prof/3
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Chef Trainee/Chefs Expert/Chefs Specialist (feat/class/chef-trainee ->
	// chefs-expert -> chefs-specialist): a flat, non-scaling Shinobi Snacks
	// pool for a character with zero real Cooking-Nin levels, keyed to the
	// same "shinobi_snacks" resource the real class feature above uses.
	// Mutually exclusive with real Cooking-Nin levels by the feats' own
	// prerequisites ("You cannot have Levels in the Cooking-Nin Class"), so
	// this never actually stacks with the level-scaled grant above in
	// practice — same shape as Adept Medic's Preserve Life: Mending
	// Presence entry, each Max recomputing the FULL cumulative total (Chef
	// Trainee's base 3, +1 at Chefs Expert, +1 again at Chefs Specialist) so
	// the higher-Max-wins combination rule picks up whichever tier the
	// character actually holds. "Regain these Snacks at the end of a short
	// rest" (Chef Trainee's own text) is a stronger regen than the real
	// class feature's own unscoped rest wording, so this also uses
	// regenFull on every tier, same as the base grant.
	chefTraineeFeatSlug: {
		Key:      "shinobi_snacks",
		Name:     "Shinobi Snacks",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 3
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	chefsExpertFeatSlug: {
		Key:      "shinobi_snacks",
		Name:     "Shinobi Snacks",
		MinLevel: 10,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 4
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	chefsSpecialistFeatSlug: {
		Key:      "shinobi_snacks",
		Name:     "Shinobi Snacks",
		MinLevel: 15,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 5
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// "Beginning at 17th level, you gain Resistance to Fire damage, or
	// Immunity if you already have Resistance. Additionally, when you would
	// inflict a Rank of Burned on yourself, you may increase the Ranks
	// inflicted by +1. Finally, once per Long Rest, you may immediately give
	// yourself, and each creature within 15ft of you, 4 Nova Points." The
	// Fire resistance/immunity clause is already modeled separately in
	// passive_traits.go under this same slug — not duplicated here. This
	// entry tracks only the once-per-Long-Rest grant gate; the 4 Nova Points
	// themselves, and Nova Aura's own in-combat point-accrual/spend economy
	// at the 2/3/4-point buff thresholds (the separate "heat_master_aura"
	// pool above), stay fully manual/narrated for both the caster and any
	// ally who receives points via this burst — no stacking in-combat-
	// currency tracking mechanism exists anywhere in this app for Nova
	// Points.
	"class/cooking-nin/group/cooking-focus/heat-master/feature/heavenly-flame": {
		Key:      "heavenly_flame_nova_burst",
		Name:     "Heavenly Flame (Nova Point Burst)",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Give yourself and each creature within 15ft 4 Nova Points",
	},
	// Fry Cooks' own bonus Aura genuinely reads "which you can activate for
	// free once per long rest" with NO 14th-level clause anywhere in the
	// text — confirmed as real RAW (the surrounding sentence is otherwise
	// clean prose, no extraction-truncation artifact), not a bug to
	// "fix" toward matching the other 8 subclasses' escalating version.
	"class/cooking-nin/group/cooking-focus/fry-cooks/feature/sunny-side-up": {
		Key:      "fry_cooks_aura",
		Name:     "Sunny Side Up",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Activate the Sunny Side Up Aura for free, without spending a use of Auras",
	},
	// "During a Short Rest, you can fry a number of pills... equal to your
	// proficiency bonus." Same shape as the base class's own Shinobi Snacks
	// above — a Short-Rest-gated count, with Long/Full Rest regenerating
	// fully too since both are supersets of a Short Rest. The pill-type
	// choice, the average-of-2-Cooking-Dice bonus-restore math, the 12-hour
	// spoilage/halving timer, the no-refry rule, and the Corroded-rank-cap
	// doubling all stay Group 2/3 — only the fry-count use-gate is tracked.
	"class/cooking-nin/group/cooking-focus/fry-cooks/feature/revv-up-those-friers": {
		Key:      "revv_up_those_friers",
		Name:     "Revv Up Those Friers (Fried Pills)",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "Starting at 13th level... You can use this feature a number of
	// times per Long Rest equal to your charisma modifier (with a minimum
	// bonus of +1)."
	"class/cooking-nin/group/cooking-focus/patissier-chef/feature/sugar-rush": {
		Key:      "sugar_rush",
		Name:     "Sugar Rush",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reroll a save or an attack against you caused by a Nature's Blend jutsu",
	},
	// "You gain 10 Chemic Points, which when spent, reduce the number of
	// remaining Minutes..." A flat pool that fully resets (not accumulates)
	// on a Long Rest, granted once per Long Rest upon entering Philosopher's
	// Form — same "fixed pool, long-rest-only" shape as Day Ahead
	// (class/intelligence-operative/.../feature/day-ahead). Philosopher's
	// Form's own active/duration state, its buff payload (Flight Speed,
	// Disengage, Advantage on 2 chosen saves, Advantage on first attack, 40
	// THP), and the actual application of a spent Chemic Point to a
	// specific jutsu cast all stay fully Group 2/3 — no duration-tracking
	// or active-stance mechanism exists anywhere in this app. Only the flat
	// 10-point count is tracked.
	"class/cooking-nin/group/cooking-focus/gastrochemist/feature/in-touch": {
		Key:      "chemic_points",
		Name:     "Chemic Points (Philosopher's Form)",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 10
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Granted once per Long Rest upon entering Philosopher's Form (Full Turn Action). Spend to extend Philosopher's Form's remaining minutes 1-for-1, or once per Jutsu spend 1 to use a Nature's Blend Enhancement without a Snack.",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// Bad Medicine: "Your Chakra Scalpel class feature gains +2 additional
	// uses, per long rest." Recomputes the full class-table bracket (copied
	// from the base grant's own switch) plus the feat's own +2, so the
	// higher-Max-wins merge picks it up automatically. The feat's other
	// clause (bonus Take Life damage) has no Take Life damage field on the
	// sheet, out of scope here.
	"feat/class/bad-medicine": {
		Key:      "chakra_scalpel_charges",
		Name:     "Chakra Scalpel Charges",
		MinLevel: 8,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := cl["class/medical-nin"]
			base := 3
			switch {
			case lvl >= 19:
				base = 9
			case lvl >= 16:
				base = 8
			case lvl >= 13:
				base = 7
			case lvl >= 10:
				base = 6
			case lvl >= 7:
				base = 5
			case lvl >= 4:
				base = 4
			}
			return base + 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Medical Expert (feat): "You gain the Chakra Scalpel feature as if you
	// were a 3rd level Medical-Nin. You gain 3 charges per long rest." A
	// flat count matching the real class table's own level-3 bracket
	// (v_class_level_resources), not a live read — Max has no database
	// access. Medical Specialist's own separate bump onto this same pool
	// sits just below.
	"feat/class/medical-expert": {
		Key:      "chakra_scalpel_charges",
		Name:     "Chakra Scalpel Charges",
		MinLevel: 10, // feat's own "Level 10+" prerequisite
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 3
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Medical Specialist (feat): "You gain the 7th level benefits of the
	// Chakra Scalpel class feature" — the real class table's own level-7
	// bracket is 5 charges (v_class_level_resources), matching the higher-
	// Max-wins combination rule so this supersedes Medical Expert's own 3
	// once both are held (Specialist requires Expert as its own
	// prerequisite).
	"feat/class/medical-specialist": {
		Key:      "chakra_scalpel_charges",
		Name:     "Chakra Scalpel Charges",
		MinLevel: 15, // feat's own "Level 15+" prerequisite
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 5
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Shaman's Draining Hexes: "when you fall to 0 hit points due to a
	// melee attack, you may immediately spend 10 Chakra... allows you to
	// take make one attack with your Spiritual Weapon... You can use this
	// feature a twice per rest." The 0-HP reaction trigger, the Spiritual
	// Weapon attack roll, and the damage-to-HP conversion all stay manual
	// (the 10-Chakra spend itself already draws from the existing tracked
	// Chakra pool) — only the twice-per-rest gate on the reaction itself
	// is tracked.
	"class/medical-nin/group/tenets-of-medicine/shaman/feature/draining-hexes": {
		Key:      "draining_hexes_uses",
		Name:     "Draining Hexes (Reaction Attack)",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "At 0 HP from a melee attack: spend 10 Chakra to make one Spiritual Weapon attack, healing for damage dealt",
	},
	// Adept Medic's Unmatched Medical Release: "creatures who would recover
	// hit points from a Jutsu you cast with the Medical Keyword or from
	// the Rejuvenating Rest, Channeled Healing or Talented Healer class
	// feature, you automatically end any conditions of B-Rank or lower.
	// You can use this feature twice per short rest." The condition-rank
	// lookup and actual condition-removal stay entirely manual/narrated;
	// only the 2-uses-per-short-rest gate is tracked.
	"class/medical-nin/group/tenets-of-medicine/adept-medic/feature/unmatched-medical-release": {
		Key:      "unmatched_medical_release_uses",
		Name:     "Unmatched Medical Release",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Automatically end a B-Rank-or-lower condition on a creature healed by a Medical-Keyword jutsu, Rejuvenating Rest, Channeled Healing, or Talented Healer",
	},
	// Adept Medic's Healing Ascension: "You can enter the Ascension state,
	// a number of times equal to your Ninjutsu ability modifier per long
	// rest." Ninjutsu's default governing ability is Intelligence for
	// every class (AttackKinds, charsheet.go) — Medical Ninjutsu's own
	// text only grants an OPTIONAL per-jutsu substitution for Medical-
	// keyword jutsu, it doesn't redefine the class's baseline Ninjutsu
	// ability, so intMod is the correct parameter here, not a new Wisdom
	// plumb-through. No stated floor in the book text (unlike Calculated
	// Response's explicit "minimum of once"), so a negative-Int character
	// correctly gets 0 uses. The Ascension state's own 1-round window and
	// damage-triggered HP recovery stay entirely manual/narrated.
	"class/medical-nin/group/tenets-of-medicine/adept-medic/feature/healing-ascension": {
		Key:      "healing_ascension_uses",
		Name:     "Healing Ascension",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			if intMod < 0 {
				return 0
			}
			return intMod
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Enter Ascension on a qualifying heal; the next time you take damage before the end of your next turn, recover HP equal to your level",
	},
	// Black Medicine's Touch of Terror: "when a creature you can see
	// within 30 feet of you is making a saving throw to resist an effect
	// that deals poison damage or inflicts the envenomed condition, you
	// can give them disadvantage on their save. You can do this twice per
	// rest." (The feature's second clause — spending a Chakra Scalpel
	// charge to boost poison potency — already rides the existing
	// chakra_scalpel_charges pool.) Actually imposing disadvantage on the
	// target's save stays fully manual/narrated; only the 2-uses-per-rest
	// gate is tracked.
	"class/medical-nin/group/tenets-of-medicine/black-medicine/feature/touch-of-terror": {
		Key:      "touch_of_terror_uses",
		Name:     "Touch of Terror",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Impose disadvantage on a creature's save vs. poison damage or the envenomed condition",
	},
	// Natural Medicine's Natural Talent: "As an Action, you may assume the
	// form of a D-Rank Summon of your Tribe, replacing your own Strength,
	// Dexterity, Constitution, HP and AC with the summoned creatures... You
	// may transform using this feature twice per long rest." Natures
	// Avatar (17th level, same subclass) explicitly spends a use of this
	// same pool, so it automatically rides this once built — no separate
	// entry needed for it. The actual Str/Dex/Con/HP/AC stat-block swap
	// into the summoned creature's form stays fully manual — this app's
	// companion system keeps every companion stat a plain player-entered
	// field, no stat-block-swap mechanism exists anywhere. Only the
	// twice-per-long-rest use-count is tracked.
	"class/medical-nin/group/tenets-of-medicine/natural-medicine/feature/natural-talent": {
		Key:      "natural_talent_transform_uses",
		Name:     "Natural Talent (Sage-Beast Transform)",
		MinLevel: 5,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Assume a D/C/B/A-Rank summon's Strength/Dexterity/Constitution/HP/AC (rank scales with class level)",
	},
	// Natural Medicine's Guardian Summoner: "Guardian: Twice per Short
	// Rest, this summon can grant all allied creatures within 20 feet of
	// it, a number of temporary hit points equal to its rank." The temp-HP
	// amount (keyed to the summon's own D/C/B/A/S rank, which this app has
	// no field for) and applying it to allies within 20 feet both stay
	// manual — only the twice-per-short-rest trigger-count is tracked.
	"class/medical-nin/group/tenets-of-medicine/natural-medicine/feature/guardian-summoner": {
		Key:      "guardian_summoner_uses",
		Name:     "Guardian Summoner",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Summon grants allies within 20 feet temp HP equal to its rank (D:5, C:10, B:15, A:20, S:25)",
	},
	// Natural Medicine's Protector of Nature: "If you are reduced to 0 Hit
	// points or are Incapacitated against your will, you can, immediately
	// as a Reaction... use your Summoning Technique to summon an S-Rank
	// Creature ignoring normal restrictions... Once you use this feature,
	// you cannot use it again until you complete a long rest." The
	// reaction trigger, the S-Rank summon's stat block, and the
	// conditional Chakra-cost branch all stay manual — only the "have I
	// used my one long-rest charge" flag is tracked.
	"class/medical-nin/group/tenets-of-medicine/natural-medicine/feature/protector-of-nature": {
		Key:      "protector_of_nature_uses",
		Name:     "Protector of Nature",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction at 0 HP/forced Incapacitated: summon an S-Rank creature ignoring normal restrictions (reduces Chakra to 0 if 30 or less, otherwise spends as usual)",
	},
	// Transmuter's Transfigured Technique: "You can remove conditions in
	// this way, a number of times equal to your Proficiency Bonus per long
	// rest."
	"class/medical-nin/group/tenets-of-medicine/transmuter/feature/transfigured-technique": {
		Key:      "transfigured_technique_uses",
		Name:     "Transfigured Technique",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Medical Doctrine's "Not Allowed to Die": "once per rest, you ignore
	// effects of jutsu, features or traits from hostile creatures that would
	// automatically kill you or reduce you to 0 hit points, instead being
	// reduced to 1 hit point." Keyed off medicalDoctrinePickedRows' synthetic
	// row (medical_nin.go), not the raw class_features slug directly — the 4
	// Medical Doctrine options all have a NULL level column and are returned
	// unconditionally by loadGrantedFeatures for every Medical-Nin, so
	// gating through the synthetic "actually picked this doctrine" row is
	// what keeps this pool from appearing for a character who picked a
	// different doctrine (or none yet). Only the once-per-rest use-count is
	// tracked; the death-prevention swap to 1 HP itself stays manual.
	"class/medical-nin/feature/not-allowed-to-die": {
		Key:      "not_allowed_to_die_uses",
		Name:     "Not Allowed to Die",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Ignore a lethal effect while an ally is alive with 1+ HP; drop to 1 HP instead",
	},
	// Medical Doctrine's "Until Their Heart Stops": "Twice per rest, a
	// creature other than yourself, who regains hit points as a result of a
	// jutsu you cast, who are currently under the effects of a hostile
	// inflicted condition, makes their next saving throw or skill check to
	// end said condition at advantage." Same synthetic-row gating as Not
	// Allowed to Die above. Only the twice-per-rest use-count is tracked;
	// the pending-advantage-on-next-save/check itself stays manual.
	"class/medical-nin/feature/until-their-heart-stops": {
		Key:      "until_their_heart_stops_uses",
		Name:     "Until Their Heart Stops",
		MinLevel: 3,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Grant a healed, hostile-condition-afflicted creature advantage on its next save/check to end that condition",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return cl["class/intelligence-operative"]/2 + 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Courageous Orders: "+1 Additional Brave order. +1 additional Brave
	// Order at 11th and 17th Intelligence Operative levels." Recomputes the
	// full base formula plus the feat's own escalating bonus, so the
	// higher-Max-wins merge picks it up automatically.
	"feat/class/courageous-orders": {
		Key:      "brave_orders",
		Name:     "Brave Orders",
		MinLevel: 4,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := cl["class/intelligence-operative"]
			n := lvl/2 + 2 + 1
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
	// Operative Training: "You gain 2 Brave Orders, which you can only spend
	// to activate Plans. You regain spent Brave orders when you complete a
	// rest." The feat's own prerequisite ("You cannot have class levels in
	// Intelligence Operative") means the base grant above would otherwise
	// compute Max = 0/2+2 = 2 for a holder anyway, but it's never applied
	// because it's keyed to the wrong slug -- this entry fixes that.
	// Residual, not fixed here: operative-expert's own "+1 Brave Order" text
	// is meant to stack on top of this, but the higher-Max-wins merge picks
	// the larger of two grants rather than summing them, so operative-
	// expert's +1 won't stack under this fix.
	"feat/class/operative-training": {
		Key:  "brave_orders",
		Name: "Brave Orders",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Mark all allies with Exploit Weakness, granting Tsume; also costs 1 Brave Order",
	},
	// Grave Controller's Undead Fortitude: "you can spend 1 Brave Order to
	// manipulate the bodies of foes near your Bound Corpse... Additionally,
	// when the Bound corpse reaches 0 temporary hit points, you can spend 1
	// Brave order. When you do, it regains Temporary HP... You can use this
	// feature twice per long rest." One shared counter gates two different
	// triggered payloads, same shape as War Cry below. Tracks only the
	// twice-per-long-rest activation-count gate; the Fear-rank save-or-
	// suffer effect on nearby creatures, the Bound Corpse's Temp-HP
	// regeneration amount, and the entire Bound Corpse companion mechanic
	// (summoning, HP, commands, jutsu-casting) remain fully manual/narrated,
	// since no companion stat-block tracking mechanism exists anywhere in
	// this app.
	"class/intelligence-operative/group/master-strategies/grave-controller/feature/undead-fortitude": {
		Key:      "undead_fortitude_activation",
		Name:     "Undead Fortitude",
		MinLevel: 9,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Manipulate foes near your Bound Corpse to inflict 2 ranks of Fear, or regain the Bound Corpse's Temp HP; each use also costs 1 Brave Order",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Also costs 1 Brave Order",
	},
	// Calculated Strategist's Calculated Insight: "As a Bonus Action, Make
	// an Intelligence (Perception) check ... You can use this feature
	// twice per initiative. You gain an additional use at 9th, 13th and
	// 17th levels." The reset boundary is per-initiative/per-encounter —
	// no automated hook for that exists anywhere in this app (confirmed no
	// "per initiative" boundary tracking anywhere in the codebase), so
	// every rest tier is regenNone and the player resets the box by hand
	// at the start of each new fight. Which of the four listed effects
	// (Determine Attack/Predict Movement/Outwit Response/Expose Weakness)
	// is chosen, and its triggered payload, stay fully Group 2/3
	// narrative — only the twice(+)-per-encounter use-count is tracked.
	"class/intelligence-operative/group/master-strategies/calculated-strategist/feature/calculated-insight": {
		Key:  "calculated_insight",
		Name: "Calculated Insight",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			lvl := cl["class/intelligence-operative"]
			n := 2
			if lvl >= 9 {
				n++
			}
			if lvl >= 13 {
				n++
			}
			if lvl >= 17 {
				n++
			}
			return n
		},
		MinLevel:   3,
		ShortRegen: regenNone,
		LongRegen:  regenNone,
		FullRegen:  regenNone,
	},
	// Interrogationist's Morale Burst/Break: "When you enhance a plan ...
	// Select one effect ... You can only use this feature once per
	// initiative." Same manual-reset shape as Calculated Insight just
	// above (no per-initiative/per-encounter hook exists anywhere in this
	// app). "Enhance a plan" already costs one Brave Order via Master
	// Planner's own grant (Key "brave_orders") — this is a separate,
	// additional once-per-initiative limit stacked on top of that
	// already-tracked resource, not a duplicate. Which effect is chosen
	// (Morale Burst's ally-marking vs Morale Break's Fear-rank contest)
	// and its triggered payload stay fully Group 2/3 narrative — only the
	// once-per-encounter use-gate is tracked here.
	"class/intelligence-operative/group/master-strategies/interrogationist/feature/morale-burst-break": {
		Key:  "morale_burst_break",
		Name: "Morale Burst/Break",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		MinLevel:   9,
		ShortRegen: regenNone,
		LongRegen:  regenNone,
		FullRegen:  regenNone,
	},
	// "Once per short rest, you may half the cost of any one Ninjutsu
	// without the Combination keyword that you cast. You gain an additional
	// use of this feature at 6th, 11th & 17th levels." Unscoped "per short
	// rest" resources regenerate on every rest tier in this app — same
	// precedent Chakra Disruption/Chakra Barrier already establish.
	"class/ninjutsu-specialist/feature/chakra-recovery": {
		Key:  "chakra_recovery",
		Name: "Chakra Recovery",
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	// The 11 Ninjutsu Focus subclasses' own 6th-level features each carry an
	// explicit "twice per rest" use-count gating a triggered combat payload
	// — structurally identical to Jutsu Breaker above, not the "generated
	// by casting" shape the 9 named L2 Adept motes/marks/vials/gems/shards
	// resources use (those remain deferred, see CLASS_AUDIT.md). Each
	// subclass's OTHER 6th-level feature (its own named "Efficient Molding"
	// technique, e.g. Crimson Molding, Deadly Tradition's sibling
	// Generational Molding) has an "Alternate Cost of 5" instead — no rest
	// cap of its own, already covered by the class-wide Efficient Molding
	// Uses pool above — and is deliberately excluded here. Where a
	// feature's own text lets a player exceed these 2 uses by spending 1
	// Chakra Die instead (Lightning Tamer, Summoning Adept), that overflow
	// trade stays manual, same "not modeled" precedent Willpower Surge's
	// own Superiority-Die overflow clause already draws above. Only the
	// use-count is tracked in every entry below; each feature's own
	// triggered payload (extra Fire damage, casting-time swap, damage-type
	// change, condition escalation, ally damage reduction, jutsu-consume
	// counter, summon-as-Action, second-jutsu cast) stays manual.
	"class/ninjutsu-specialist/group/ninjutsu-focus/blaze-walker/feature/flash-fire": {
		Key:      "flash_fire_uses",
		Name:     "Flash Fire",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/hijutsu-elitist/feature/deadly-tradition": {
		Key:      "deadly_tradition_uses",
		Name:     "Deadly Tradition",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "If you attempted to Overcharge a jutsu in this way an additional
	// time past these two, you can do so by spending 1 Chakra die" — the
	// Chakra Die overflow trade stays manual.
	"class/ninjutsu-specialist/group/ninjutsu-focus/lightning-breaker/feature/lightning-tamer": {
		Key:      "lightning_tamer_uses",
		Name:     "Lightning Tamer",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/sanguine-master/feature/bloody-evocation": {
		Key:      "bloody_evocation_uses",
		Name:     "Bloody Evocation",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/seal-consumption": {
		Key:      "seal_consumption_uses",
		Name:     "Seal Consumption",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/stone-crusher/feature/mountains-aegis": {
		Key:      "mountains_aegis_uses",
		Name:     "Mountains Aegis",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/storm-terror/feature/storm-herald": {
		Key:      "storm_herald_uses",
		Name:     "Storm Herald",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "If you attempted to summon a creature as an Action after you have
	// expended all uses of this feature, you can do so by spending 1
	// Chakra Die" — the Chakra Die overflow trade stays manual.
	"class/ninjutsu-specialist/group/ninjutsu-focus/summoner/feature/summoning-adept": {
		Key:      "summoning_adept_uses",
		Name:     "Summoning Adept",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "You can only cast Jutsu in this way twice per Long Rest, however you
	// regain one use when you take a Short Rest" — the flat "+1 on a Short
	// Rest, not a full reset" shape doesn't match any of restRegen's 5
	// fixed regen shapes (regenFull is the closest, but would wrongly
	// restore BOTH uses on a Short Rest instead of one), so ShortRegen is
	// left at regenNone here and the player adjusts the box by hand after
	// a Short Rest — same "no automatic gain" purpose regenNone already
	// documents above.
	"class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/twin-cast": {
		Key:      "twin_cast_uses",
		Name:     "Twin Cast",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Also regains 1 use (not modeled) on a Short Rest — adjust by hand",
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/trace-talent/feature/limitless-casting": {
		Key:      "limitless_casting_uses",
		Name:     "Limitless Casting",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/tsunami/feature/frigid-deep": {
		Key:      "frigid_deep_uses",
		Name:     "Frigid Deep",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// The 11 subclasses' own 14th-level features escalate their 6th-level
	// feature's payload — 8 of the 11 carry their own explicit "twice per
	// long rest" use-count on top of that (verified individually against
	// rules.db); the remaining 3 (Sanguine Master's Sanguine Elite,
	// Summoner's Summoning Master, Trace Talent's Kiyo) have no per-rest
	// gate of their own in their printed text and are correctly Group 2/3,
	// not modeled. Only the use-count is tracked in every entry below;
	// each feature's own triggered payload stays manual.
	"class/ninjutsu-specialist/group/ninjutsu-focus/blaze-walker/feature/fire-release-master": {
		Key:      "fire_release_master_uses",
		Name:     "Fire Release Master",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/hijutsu-elitist/feature/hijutsu-master": {
		Key:      "hijutsu_master_uses",
		Name:     "Hijutsu Master",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/lightning-breaker/feature/lightning-release-master": {
		Key:      "lightning_release_master_uses",
		Name:     "Lightning Release Master",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/fuinjutsu-sennin": {
		Key:      "fuinjutsu_sennin_uses",
		Name:     "Fuinjutsu Sennin",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/stone-crusher/feature/earth-release-master": {
		Key:      "earth_release_master_uses",
		Name:     "Earth Release Master",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/storm-terror/feature/wind-release-master": {
		Key:      "wind_release_master_uses",
		Name:     "Wind Release Master",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/soshikage": {
		Key:      "soshikage_uses",
		Name:     "Soshikage",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/ninjutsu-specialist/group/ninjutsu-focus/tsunami/feature/water-release-master": {
		Key:      "water_release_master_uses",
		Name:     "Water Release Master",
		MinLevel: 14,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Summoner's Summoners Will (2nd level): "you can spend 1 of your
	// Chakra Die to recharge one spent Jutsu Slot for your summoned
	// creature. You can spend Chakra Dice in this way, a number of times
	// equal to your Proficiency Bonus, per Long Rest." A Proficiency-Bonus
	// long-rest use-count gating spending FROM the character's own
	// already-tracked Chakra Die pool — only this outer per-long-rest cap
	// is tracked; the Chakra Die spend itself and the summoned creature's
	// Jutsu Slot recovery both stay manual.
	"class/ninjutsu-specialist/group/ninjutsu-focus/summoner/feature/summoners-will": {
		Key:      "summoners_will_uses",
		Name:     "Summoners Will",
		MinLevel: 2,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Spend 1 Chakra Die to recharge a spent Jutsu Slot on your summoned creature",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return prof
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Kill costs 2 charges; Stun and Cripple cost 1 charge each",
	},
	// Battle Dancer Form's Master of Aggression (20th level): "As an
	// Action, by spending 3 Chakra die... You can use this feature once per
	// rest." The 3 Chakra Die cost is a manual spend against the already-
	// tracked Chakra Die pool. Movement, attack resolution, crit
	// determination, Staggered/Incapacitated application, and the ongoing
	// Str-save-to-end-Incapacitated all stay manual — only the once-per-rest
	// activation gate is tracked.
	"class/weapon-specialist/group/weapon-forms/battle-dancer-form/feature/master-of-aggression": {
		Key:      "master_of_aggression_uses",
		Name:     "Master of Aggression",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action, spend 3 Chakra Die: dash to a chosen space and make two advantaged melee Taijutsu attacks against creatures along the path (10dX + Str/Dex mod; crit if you beat AC by 10+, doubling damage and applying Staggered + Incapacitated)",
	},
	// Battle Dancer Form's Whirlwind Sweep (13th level): "...you can use
	// your action to perform a whirlwind sweep attack... You can use this
	// feature twice per rest." The attack roll, crit check, and save
	// resolution stay manual — only the twice-per-rest activation gate is
	// tracked.
	"class/weapon-specialist/group/weapon-forms/battle-dancer-form/feature/whirlwind-sweep": {
		Key:      "whirlwind_sweep_uses",
		Name:     "Whirlwind Sweep",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action: multi-target melee attack vs. the highest-AC target in range, Dex save or be knocked prone and Staggered",
	},
	// Gungnir Piercer Form's Sigurd's Heroism (6th level): "Twice per long
	// rest, when you see a creature within your movement speed range, be
	// hit by an attack, you may, as a Reaction, attempt to block the
	// triggering attack." The move-to-attacker, opposed-Taijutsu-attack,
	// and block resolution all stay manual — only the twice-per-long-rest
	// activation gate is tracked.
	"class/weapon-specialist/group/weapon-forms/gungnir-piercer-form/feature/sigurds-heroism": {
		Key:      "sigurds_heroism_uses",
		Name:     "Sigurd's Heroism",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction: move to a creature within your movement speed that was just hit by an attack you saw, then make an opposed Taijutsu attack against the triggering attack roll to block it",
	},
	// Gungnir Piercer Form's Ragnarök (20th level — the rules-database slug
	// strips the ö, "ragnar-k"): a Bonus Action + 20-chakra-or-3-Chakra-Die
	// activation, once per long rest. Both the chakra/Chakra Die cost and
	// the 1-minute Ninjutsu/Genjutsu lockout, piercing-damage, and
	// Lacerated-doubling payload stay manual/narrated — only the once-per-
	// long-rest activation gate is tracked.
	"class/weapon-specialist/group/weapon-forms/gungnir-piercer-form/feature/ragnar-k": {
		Key:      "ragnarok_uses",
		Name:     "Ragnarök",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action, spend 20 chakra or 3 Chakra Die: 1-minute state — piercing damage, Lacerated doubled, no Ninjutsu/Genjutsu casting",
	},
	// Obsidian Hammer Form's Obsidian Mind (13th level): "...when you would
	// make a saving throw to resist a Genjutsu, you may roll 1 flurry die,
	// adding the result to the roll... You may do this twice per long
	// rest." The passive telepathy-immunity clause in the same feature's
	// opening sentence has no rest-cap and needs no pool — not part of this
	// entry.
	"class/weapon-specialist/group/weapon-forms/obsidian-hammer-form/feature/obsidian-mind": {
		Key:      "obsidian_mind_uses",
		Name:     "Obsidian Mind",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Roll 1 Flurry Die and add it to a saving throw against Genjutsu; immune to that Genjutsu's critical-fail result when used",
	},
	// Obsidian Hammer Form's Obsidian Soul (20th level — the Str/Cha +2
	// ability grant this same slug also carries is already wired via
	// internal/features/scores.go's fixedAbilityScoreGrants): a full turn
	// action, once per long rest, for a 1-minute AoE buff. The buff's
	// actual effects (disadvantage on enemy attacks/concentration checks,
	// bludgeoning/Strength bonus die) stay manual — only the once-per-
	// long-rest activation gate is tracked.
	"class/weapon-specialist/group/weapon-forms/obsidian-hammer-form/feature/obsidian-soul": {
		Key:      "obsidian_soul_uses",
		Name:     "Obsidian Soul",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Full turn action: 1-minute AoE — hostile creatures within 30 feet have disadvantage on attacks vs. others and auto-fail concentration checks (25 chakra to check at disadvantage instead); add a Flurry Die to bludgeoning damage rolls and Strength saves/checks",
	},
	// Primal Weapon Form's Master of the Primal (20th level — the Str-or-Dex
	// choice and fixed +2 Int this same slug also carries are already wired
	// via internal/features/choices.go/scores.go): an action + 3-Chakra-Die
	// activation, once per long rest, for a 1-minute buff. The 3 Chakra Die
	// cost is a manual spend against the already-tracked Chakra Die pool.
	// Resistance, the per-turn ally damage bonus, and the Bonus-Action-
	// Attack trigger all stay manual/narrated.
	"class/weapon-specialist/group/weapon-forms/primal-weapon-form/feature/master-of-the-primal": {
		Key:      "master_of_the_primal_uses",
		Name:     "Master of the Primal",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Action, spend 3 Chakra Die: 1-minute buff — resistance to Slashing/Piercing/Bludgeoning; once per turn add 2 Flurry Die to an ally's attack damage of your chosen nature-release type within 30 feet; casting a jutsu of that Nature Release lets you take the Attack action as a Bonus Action",
	},
	// Ranger Form's Quick Draw (13th level): "On your first turn in
	// combat... Choose up to 6 creatures... Once you've used this feature
	// you must complete a rest before you can use it again." Unscoped "a
	// rest" — regenerates on every tier, same idiom Before All establishes.
	// The ranged attack roll and Flurry Technique application stay manual.
	"class/weapon-specialist/group/weapon-forms/ranger-form/feature/quick-draw": {
		Key:      "quick_draw_uses",
		Name:     "Quick Draw",
		MinLevel: 13,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "First turn in combat only (if not surprised): make a ranged weapon attack against up to 6 creatures that have not yet acted, applying a Flurry Technique to each hit",
	},
	// Ranger Form's Unmatched Efficiency, miss-to-hit half (the Dex/Wis +2
	// ability grant this same slug also carries is already wired via
	// scores.go): "...once per turn if a ranged weapon attack you make,
	// misses a target creature you can treat the miss as a hit... twice per
	// rest." The save-to-success half is a SEPARATE, independently-tracked
	// pool — see the "unmatched_efficiency_save_uses" entry in
	// customResourceSecondaryGrants below (one slug, two pools, same shape
	// Medical Expert's own dual-map split establishes).
	"class/weapon-specialist/group/weapon-forms/ranger-form/feature/unmatched-efficiency": {
		Key:      "unmatched_efficiency_hit_uses",
		Name:     "Unmatched Efficiency (Miss-to-Hit)",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Once per turn, treat a missed ranged weapon attack as a hit",
	},
	// Samurai Form's Circle of Protection (6th level): "If you or a
	// creature you can see within 10 feet of you takes damage, as a
	// Reaction, you ward everyone near you. Roll 1 flurry die... You can
	// use this feature twice per short rest." The AC bonus and DR
	// application stay manual — only the twice-per-short-rest activation
	// gate is tracked.
	"class/weapon-specialist/group/weapon-forms/samurai-form/feature/circle-of-protection": {
		Key:      "circle_of_protection_uses",
		Name:     "Circle of Protection",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Reaction when you or an ally within 10 feet takes damage: roll 1 Flurry Die, grant yourself and nearby allies a bonus to AC equal to half the result against the triggering attack; if it still hits, grant DR equal to the full result until the start of your next turn",
	},
	// Phantom Blade Form's First Step: A Single Drop (6th level), the
	// delay-extension half: "When you would delay damage using any Weapon
	// Specialist Class Feature, you may extend the delayed damage up to the
	// end of your next turn... increased by an amount equal to 4 flurry
	// die. You may use this part of the feature twice per short rest." The
	// feature's separate, unconditional +1 Flurry Die on Phantom Blade
	// Flurry Technique rolls has no cap and needs no pool — not part of
	// this entry.
	"class/weapon-specialist/group/weapon-forms/phantom-blade-form/feature/first-step-a-single-drop": {
		Key:      "first_step_a_single_drop_uses",
		Name:     "First Step: A Single Drop",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Extend a delayed-damage effect from any Weapon Specialist feature to the end of your next turn, adding 4 Flurry Die to the delayed damage",
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Cast an Action-cost Taijutsu as a Bonus Action, or a Bonus-Action-cost Taijutsu as a Reaction with no set trigger",
	},
}

// customResourceSecondaryGrants exists because Medical Expert (feat) grants
// bonuses to TWO independently-tracked pools (Chakra Scalpel Charges AND
// Preserve/Take Life) from one feat slug, and customResourceGrants is a
// map[string]customResourceGrant — one value per key, so a single slug can't
// carry two grants in that map (a second literal with the same key is a
// compile error). computeCustomResources consults this second table after
// the primary one for every feature, merging into the same byKey/order
// result — see the doc there.
var customResourceSecondaryGrants = map[string]customResourceGrant{
	// Medical Expert (feat): "...the Preserve/Take Life class feature as if
	// you were a 5th level Medical Nin. You do not gain any benefits as a
	// result of being higher level. You can use this feature, twice per
	// rest." Matches the real class table's own level-5 bracket (base 2,
	// see "class/medical-nin/feature/preserve-take-life" above) — same
	// ShortRegen/LongRegen shape, "twice per rest" un-scoped the same way.
	"feat/class/medical-expert": {
		Key:      "preserve_take_life",
		Name:     "Preserve/Take Life",
		MinLevel: 10, // feat's own "Level 10+" prerequisite
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Void Walker's Spatial Displacement, second half: "twice per Long
	// Rest, whenever you would be targeted by an attack" — the reactive
	// teleport, independent of the same feature's own teleport-attack cap
	// (the primary "spatial_displacement_teleport" entry in
	// customResourceGrants above). The reactive teleport/miss-redirect
	// effect stays manual; only this half's own twice-per-long-rest gate
	// is tracked.
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/spatial-displacement": {
		Key:      "spatial_displacement_reaction",
		Name:     "Spatial Displacement (Reactive Teleport)",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// Wolves Legacy's Kages Die Twice, second half: "if you would die,
	// prior to using this ability, once per full rest, you can choose to
	// rise from the ether..." — the book says "per full rest" specifically
	// for this half (not long rest), unlike the same feature's own
	// "Stand Back Up" half (the primary "kages_die_twice_stand_up" entry
	// in customResourceGrants above), so ShortRegen and LongRegen both
	// stay regenNone here.
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/kages-die-twice": {
		Key:      "kages_die_twice_rise_from_ether",
		Name:     "Kages Die Twice (Rise From the Ether)",
		MinLevel: 17,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen: regenNone,
		LongRegen:  regenNone,
		FullRegen:  regenFull,
	},
	// Trickster Scout's Tricksters Soul Binding, the fusion-activation half
	// (independent of the "tricksters_words" primary entry above, which
	// tracks only one of the four named benefits gained while fused):
	// "As a Bonus Action, you can fuse with your Void Soul for up to 1
	// Minute... You can use this feature once per short rest. If you
	// attempted to gain this benefit again after using it once, you may
	// spend 1 Superiority Die to do so." A second fusion before the next
	// short rest costs a Superiority Die instead, tracked manually on that
	// pool. The 1-minute transformation and 3 of its 4 sub-benefits
	// (Potential/Strength/Spirit) stay fully manual/narrated — only whether
	// the free (non-Superiority-Die) fusion activation is available this
	// rest is tracked here.
	"class/scout-nin/group/scouting-technique/trickster-scout/feature/tricksters-soul-binding": {
		Key:      "tricksters_fusion",
		Name:     "Tricksters Soul Binding (fusion)",
		MinLevel: 6,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 1
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Bonus Action: fuse with your Void Soul for up to 1 minute, gaining Tricksters Potential/Strength/Spirit/Words. A second fusion before your next short rest costs a Superiority Die instead (tracked manually on that pool)",
	},
	// Ranger Form's Unmatched Efficiency, save-to-success half (independent
	// of the "unmatched_efficiency_hit_uses" primary entry above, which
	// tracks the feature's own separate miss-to-hit half): "...if you fail
	// a Dexterity or Wisdom saving throw, you may treat it as a success...
	// twice per rest." The book's "each of these effects twice per rest" is
	// two independent 2-use pools, not one shared pool of 2 — this is the
	// same "one slug needs two independently-tracked pools" shape Medical
	// Expert's own dual-map split establishes above.
	"class/weapon-specialist/group/weapon-forms/ranger-form/feature/unmatched-efficiency": {
		Key:      "unmatched_efficiency_save_uses",
		Name:     "Unmatched Efficiency (Save-to-Success)",
		MinLevel: 20,
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
			return 2
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
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
		Max: func(cl map[string]int, con, intMod, cha, prof, wisMod int) int {
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
	if key == "ccd_mending" || key == "ccd_maiming" || key == "visceral_language" {
		return true
	}
	for _, g := range customResourceGrants {
		if g.Key == key {
			return true
		}
	}
	for _, g := range customResourceSecondaryGrants {
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
func computeCustomResources(features []grantedFeatureRow, classLevels map[string]int, conMod, intMod, chaMod, profBonus, wisMod int, characterLevel int, stored map[string]int) []CustomResourceEntry {
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
		max := grant.Max(classLevels, conMod, intMod, chaMod, profBonus, wisMod)
		if existing, seen := byKey[grant.Key]; !seen {
			byKey[grant.Key] = resolved{grant: grant, max: max}
			order = append(order, grant.Key)
		} else if max > existing.max {
			// The winning (higher-Max) grant also supplies the regen
			// kind/Restriction that governs this key from here on.
			byKey[grant.Key] = resolved{grant: grant, max: max}
		}
	}

	// customResourceSecondaryGrants: a second lookup for slugs that grant a
	// bonus to a SECOND, differently-keyed pool alongside their primary
	// customResourceGrants entry (Medical Expert grants both Chakra Scalpel
	// Charges above and Preserve/Take Life here) — same MinLevel gate, same
	// higher-Max-wins merge into the shared byKey/order result.
	for _, f := range features {
		grant, ok := customResourceSecondaryGrants[f.Slug]
		if !ok {
			continue
		}
		if grant.MinLevel > 0 && characterLevel < grant.MinLevel {
			continue
		}
		max := grant.Max(classLevels, conMod, intMod, chaMod, profBonus, wisMod)
		if existing, seen := byKey[grant.Key]; !seen {
			byKey[grant.Key] = resolved{grant: grant, max: max}
			order = append(order, grant.Key)
		} else if max > existing.max {
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

// sirenVisceralLanguageFeatureSlug is Siren's own 2nd-level subclass
// feature — see genjutsuVisceralLanguagePool below.
const sirenVisceralLanguageFeatureSlug = "class/genjutsu-specialist/group/genjutsu-pledges/siren/feature/visceral-language"

// genjutsuVisceralLanguagePool appends Visceral Language's own combined
// use-pool ("a combined number of times equal to your Genjutsu Ability
// Modifier per long rest") as a synthetic entry. Not expressible as an
// ordinary customResourceGrants entry: Max's shared signature carries CON/
// Intelligence/Charisma modifiers but not Wisdom, and "Genjutsu Ability
// Modifier" is ambiguous in isolation (Wisdom by default, auto-overridden
// to Charisma by genjutsuExpertiseFeatSlug/mentalBoonsFeatSlug, then
// overridden again by the player's own genjutsu_ability override) —
// re-deriving that priority chain here would risk drifting out of sync
// with charsheet.Compute's own resolution of it, so this reads the
// already-resolved sheet.JutsuAttacks entry instead, the same "trust
// Compute's own answer" approach every other post-process step in this
// file takes. A no-op for every character without Visceral Language
// granted (only ever called when the feature's own slug is present — see
// loadCustomResources). Dealing the psychic damage or granting the
// temp-HP each use stays narrated (the per-level damage/temp-HP die is
// already a computed readout via visceralLanguageDie, genjutsu.go) — only
// the combined use-count pool is tracked here.
func genjutsuVisceralLanguagePool(entries []CustomResourceEntry, sheet *charsheet.Sheet, stored map[string]int) []CustomResourceEntry {
	var genjutsuMod int
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Genjutsu" {
			genjutsuMod = sheet.Abilities[a.Ability].Modifier
			break
		}
	}
	if genjutsuMod < 0 {
		genjutsuMod = 0
	}
	current, ok := stored["visceral_language"]
	if !ok {
		current = genjutsuMod
	}
	if current > genjutsuMod {
		current = genjutsuMod
	}
	if current < 0 {
		current = 0
	}
	return append(entries, CustomResourceEntry{
		Key:         "visceral_language",
		Name:        "Visceral Language",
		Current:     current,
		Max:         genjutsuMod,
		Restriction: "Combined use pool for Visceral Language's damage/temp-HP options",
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
	})
}

// genjutsuDeceitfulDuplicatePool appends the Deceitful Duplicate Malleable
// Mirage's own combined use-pool ("a number of times equal to your Genjutsu
// ability modifier, and these uses recover when you finish a long rest") as
// a synthetic entry — the identical Max-signature limitation
// genjutsuVisceralLanguagePool above already works around (no Wisdom
// parameter, and re-deriving "Genjutsu ability modifier"'s own Expertise/
// Mental-Boons/player-override priority chain here would risk drifting out
// of sync with charsheet.Compute's own resolution of it), so this reads the
// already-resolved sheet.JutsuAttacks entry the same way. A no-op for every
// character without Deceitful Duplicate picked (only ever called when the
// synthetic "genjutsu-mirage/deceitful-duplicate" row is present — see
// loadCustomResources, gated the same way genjutsuVisceralLanguagePool is
// gated off sirenVisceralLanguageFeatureSlug). Becoming invisible (leaving
// a duplicate) behind each use stays narrated — only the combined use-count
// pool is tracked here.
func genjutsuDeceitfulDuplicatePool(entries []CustomResourceEntry, sheet *charsheet.Sheet, stored map[string]int) []CustomResourceEntry {
	var genjutsuMod int
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Genjutsu" {
			genjutsuMod = sheet.Abilities[a.Ability].Modifier
			break
		}
	}
	if genjutsuMod < 0 {
		genjutsuMod = 0
	}
	current, ok := stored["deceitful_duplicate"]
	if !ok {
		current = genjutsuMod
	}
	if current > genjutsuMod {
		current = genjutsuMod
	}
	if current < 0 {
		current = 0
	}
	return append(entries, CustomResourceEntry{
		Key:         "deceitful_duplicate",
		Name:        "Deceitful Duplicate",
		Current:     current,
		Max:         genjutsuMod,
		Restriction: "Bonus Action when casting a Genjutsu: become invisible (leaving a duplicate in your place) until the start of your next turn",
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
	})
}

// puppetResourcefulTacticsGenjutsuBoost raises Explosive Tools' own pool
// ("resourceful_tactics_explosive_tools" above, built from Intelligence
// Modifier alone) when a Green Technique Marionettist's substituted Genjutsu
// ability modifier is higher — the same "You can also substitute your
// Genjutsu ability modifier for your Intelligence, for Puppet Master
// features" clause puppetGeneralizedSkillCap (puppet_skills.go) already
// applies to the Generalized Skill cap. Reads the resolved Genjutsu modifier
// off sheet.JutsuAttacks the same way genjutsuVisceralLanguagePool does,
// rather than re-deriving the ability-override priority chain by hand. A
// no-op for every subclass color besides Green, and for a character with no
// "resourceful_tactics_explosive_tools" entry at all (Resourceful Tactics not
// picked — see puppetResourcefulTacticsPickedRows). Using Explosive Tools and
// its effect stay entirely narrated, same as the base entry already
// documents — only the pool's own Max/Current get the substitution.
func puppetResourcefulTacticsGenjutsuBoost(entries []CustomResourceEntry, sheet *charsheet.Sheet, subclassColor string) []CustomResourceEntry {
	if subclassColor != "Green" || sheet == nil {
		return entries
	}
	var genjutsuMod int
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Genjutsu" {
			genjutsuMod = sheet.Abilities[a.Ability].Modifier
			break
		}
	}
	if genjutsuMod < 0 {
		genjutsuMod = 0
	}
	for i := range entries {
		if entries[i].Key != "resourceful_tactics_explosive_tools" {
			continue
		}
		if genjutsuMod > entries[i].Max {
			delta := genjutsuMod - entries[i].Max
			entries[i].Max = genjutsuMod
			entries[i].Current += delta
			if entries[i].Current > entries[i].Max {
				entries[i].Current = entries[i].Max
			}
		}
		break
	}
	return entries
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
	// Medical Doctrine's 4 options are NULL-level class_features rows,
	// returned unconditionally by loadMergedGrantedFeatures for every
	// Medical-Nin regardless of level or pick (see medicalDoctrinePickedRows'
	// own doc comment, medical_nin.go) — append the character's actually-
	// picked subset before resolving customResourceGrants, so Not Allowed to
	// Die/Until Their Heart Stops' pools only appear for a character who
	// picked that specific doctrine, same "append, don't merge into the
	// stored list" treatment hunterNinPatternPassiveRows already establishes.
	pickedRows, err := s.medicalDoctrinePickedRows(characterID)
	if err != nil {
		return nil, err
	}
	features = append(features, pickedRows...)
	// Same shape, same reason: Real World Conversion's 5 options are all
	// NULL-level class_features rows too, so Actualized Alteration's real
	// slug is already unconditionally present in `features` above for
	// every Genjutsu Specialist 5+ regardless of pick — see
	// genjutsuActualizedAlterationPickedRows (genjutsu.go) and the
	// "genjutsu-pick/actualized-alteration" entry in customResourceGrants.
	conversionRows, err := s.genjutsuActualizedAlterationPickedRows(characterID)
	if err != nil {
		return nil, err
	}
	features = append(features, conversionRows...)
	// Different shape: Malleable Mirages' own class_options rows never
	// reach loadMergedGrantedFeatures' output at all (no NULL-level-row
	// issue to route around here — see genjutsuMiragePickedRows' own doc
	// comment, genjutsu.go), so this injection is what makes the ~31
	// per-Mirage rest-scoped use-limits (and Deceitful Duplicate's own
	// ability-modifier-sized pool below) reachable in the first place.
	mirageRows, err := s.genjutsuMiragePickedRows(characterID)
	if err != nil {
		return nil, err
	}
	features = append(features, mirageRows...)
	// Different shape, same reason: Resourceful Tactics is one of
	// puppetMasterTacticSlugs (characters.go) — unconditionally EXCLUDED
	// from loadGrantedFeatures' output (not unconditionally included, like
	// the two cases above) so Puppet Master's 5 Tactics don't get blanket-
	// granted with no player choice. Re-add its real slug here, but only
	// for a character who actually picked it via the Tactics picker — see
	// puppetResourcefulTacticsPickedRows (puppet_tactics.go) and the
	// "class/puppet-master/feature/resourceful-tactics" entry in
	// customResourceGrants.
	tacticRows, err := s.puppetResourcefulTacticsPickedRows(characterID)
	if err != nil {
		return nil, err
	}
	features = append(features, tacticRows...)
	// Resolved once, up front: Green Technique Marionettist's own Genjutsu-
	// modifier-for-Intelligence substitution reaches Explosive Tools' pool
	// below (puppetResourcefulTacticsGenjutsuBoost) — a safe no-op for every
	// character without a Puppet Master subclass at all.
	subclassSlug, _, err := s.puppetMasterSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	subclassColor := puppetSubclassColorBySlug[subclassSlug]
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
	wisMod := sheet.Abilities["wis"].Modifier
	entries := computeCustomResources(features, classLevels, conMod, intMod, chaMod, sheet.ProficiencyBonus, wisMod, sheet.Level, stored)

	for _, f := range features {
		if f.Slug == madScientistBioticMasteryFeatureSlug {
			entries = madScientistCCDSplit(entries, sheet.CCDMendingPct, stored)
		}
		if f.Slug == sirenVisceralLanguageFeatureSlug {
			entries = genjutsuVisceralLanguagePool(entries, sheet, stored)
		}
		if f.Slug == genjutsuMirageDeceitfulDuplicateSlug {
			entries = genjutsuDeceitfulDuplicatePool(entries, sheet, stored)
		}
	}
	entries = puppetResourcefulTacticsGenjutsuBoost(entries, sheet, subclassColor)
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
