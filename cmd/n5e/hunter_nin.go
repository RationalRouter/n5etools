package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Hunter-Nin's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Hunter-Nin entry documents: Lethal Attack (class_features
// level 1, a Sneak-Attack-shaped bonus-damage die scaling 1d8->10d8, already
// used by the class-browsing reference table via v_class_level_resources
// but never surfaced per-character before) rendered as a computed
// informational line, and four independent cap-gated catalog picks —
// Lethal Precision (level 1, a hand-curated 2-option Taijutsu/Bukijutsu
// table with no class_options rows of its own, cap 1, no re-cap — picking
// one swaps that discipline's JutsuAttacks entry to Dexterity, see
// charsheet.go), Hunters Patterns (level 2, class_options list_name
// "Hunters Patterns", cap 1->2@9th->3@15th), Hunters Exploits (level 3,
// "Hunters Exploits", cap 2->3@10th->4@17th, plus a Proficiency-Bonus-per-
// Short-Rest spend pool handled by customResourceGrants — see
// custom_resources.go), and Defensive Tactics (level 6, a hand-curated
// 4-option catalog with no class_options rows of its own — the book embeds
// its options directly in the granting feature's own text — cap
// 1->2@11th->3@17th).
//
// Every one of the 8 Hunter's Creeds subclasses grants exclusive, free
// access to exactly one Hunters Exploit via its own 3rd-level feature ("you
// gain exclusive access to the X Hunter Exploit. This does not count
// against your Exploit Limit.") — huntersExploitAutoGrants/
// huntersExploitSubclassLocks below mirror Taijutsu's
// martialTechniqueAutoGrants for the "known for free, outside the cap"
// half, plus a lock so the other 7 subclasses can never manually pick that
// exploit from the shared catalog.
//
// Everything else (per-subclass numeric pools like Undertaker's Poisonous
// Embrace or Vice Agent's Greed's Shell, the 7 subclasses' own
// jutsu-plus-technique-rider grants, all Group 2/3 conditional/triggered
// features) is documented, not modeled — see CLASS_AUDIT.md's Hunter-Nin
// detail entry for the full per-subclass breakdown.
//
// Four archetype-training feats (feat/class/hunters-training ->
// hunters-expert -> {hunter-specialist, hunters-enthusiast}) grant a taste
// of these mechanics to a character with zero real Hunter-Nin levels — each
// feat's own prerequisites text ("You cannot have class levels in
// Hunter-Nin") makes real levels and this feat progression mutually
// exclusive. loadHunterTechniquesTabData's own class-level gate is widened
// to also open for these feats (see huntersArchetypeGrants), granting
// extra Hunters Exploit slots and a flat, non-scaling Lethal Attack die
// count; the matching per-rest Exploit-use pool bump lives in
// custom_resources.go, keyed by the same feat slugs. Each feat's own text
// says "Learn one Hunters Exploit that you qualify for" — checked directly
// against rules.db: no Hunters Exploit row carries a prerequisites value or
// any level/subclass gate in its description (unlike Malleable Mirages),
// so "qualify for" imposes no real filtering beyond the existing subclass-
// exclusive locks (huntersExploitSubclassLocks) already applied to every
// Hunter-Nin's own Available list; the extra slot simply opens the same
// catalog a real level-3+ Hunter-Nin sees. Cunning Action
// (hunters-expert) and Primary Target (hunter-specialist) need no plumbing
// of their own — mergeFeatFeatures already surfaces the feat's own
// description text in the Core tab's granted-features list regardless of
// this tab's gate, and neither feature has a numeric effect to compute
// (both Group 2). Hunters Enthusiast's "Select one Hunter-Nin Class,
// Hunters Creed (Subclass)" clause is NOT modeled: setCharacterSubclass
// hard-requires a character_classes row for the subclass's own parent
// class before it will store a pick, so a feat-only character has no way
// to choose a Creed at all today — a structural gap in the subclass-picker
// itself, not something this file can route around.
const hunterNinSlug = "class/hunter-nin"

// Hunter-Nin's four archetype-training feat slugs, referenced by both this
// file (Exploit slot count, flat Lethal Attack dice) and
// custom_resources.go (the matching Exploit-use-per-rest pool bump) — kept
// as named constants so both stay in sync with a single source of truth.
const (
	huntersTrainingFeatSlug   = "feat/class/hunters-training"
	huntersExpertFeatSlug     = "feat/class/hunters-expert"
	hunterSpecialistFeatSlug  = "feat/class/hunter-specialist"
	huntersEnthusiastFeatSlug = "feat/class/hunters-enthusiast"
)

// huntersArchetypeLethalAttackDice: the flat, non-scaling Lethal Attack die
// count each archetype tier grants in place of the real class's
// level-scaled chart (lethalAttackDice) — "does not scale with level"
// (Hunters Training's own text), "increases to 4d6" (Hunters Expert),
// "increases to 6d6" (Hunter Specialist). Each tier requires the one before
// it as its own feat prerequisite (Expert needs Training; Specialist and
// Enthusiast both separately need Expert), so a character holding a later
// tier always holds every earlier one too — the highest tier owned is
// always the character's whole Lethal Attack, never a sum of tiers.
var huntersArchetypeLethalAttackDice = map[string]string{
	huntersTrainingFeatSlug:  "2d6",
	huntersExpertFeatSlug:    "4d6",
	hunterSpecialistFeatSlug: "6d6",
}

// huntersArchetypeGrants reads the character's own granted-features list
// (feats included, via mergeFeatFeatures) for the 4 archetype-training
// feats and returns how many extra Hunters Exploit slots they grant
// (Training/Expert/Specialist each say "Learn one Hunters Exploit" in
// their own right — three separate slots, not one upgraded slot —
// Enthusiast grants none) and the flat Lethal Attack dice the highest tier
// owned grants ("" if none owned). Both zero/empty is also the caller's own
// gating signal for "no archetype feat renders anything" — Enthusiast
// alone (never possible under the real prerequisite chain, which requires
// Expert first) would otherwise open an empty box, since it grants neither
// an Exploit slot nor a Lethal Attack die on its own.
func huntersArchetypeGrants(features []grantedFeatureRow) (exploitSlots int, lethalDice string) {
	has := make(map[string]bool, 4)
	for _, f := range features {
		has[f.Slug] = true
	}
	if has[huntersTrainingFeatSlug] {
		exploitSlots++
	}
	if has[huntersExpertFeatSlug] {
		exploitSlots++
	}
	if has[hunterSpecialistFeatSlug] {
		exploitSlots++
	}
	switch {
	case has[hunterSpecialistFeatSlug]:
		lethalDice = huntersArchetypeLethalAttackDice[hunterSpecialistFeatSlug]
	case has[huntersExpertFeatSlug]:
		lethalDice = huntersArchetypeLethalAttackDice[huntersExpertFeatSlug]
	case has[huntersTrainingFeatSlug]:
		lethalDice = huntersArchetypeLethalAttackDice[huntersTrainingFeatSlug]
	}
	return exploitSlots, lethalDice
}

// huntersExploitsResourceKey must match custom_resources.go's own Key for
// the "class/hunter-nin/feature/hunters-exploits" grant — the spend pool
// behind whichever exploits a character knows.
const huntersExploitsResourceKey = "hunter_exploits"

const (
	bladeWardenSubclassSlug  = "class/hunter-nin/group/hunters-creeds/blade-warden"
	necroticHandSubclassSlug = "class/hunter-nin/group/hunters-creeds/necrotic-hand"
	graveStalkerSubclassSlug = "class/hunter-nin/group/hunters-creeds/grave-stalker"
	arsenalistSubclassSlug   = "class/hunter-nin/group/hunters-creeds/arsenalist"
	undertakerSubclassSlug   = "class/hunter-nin/group/hunters-creeds/undertaker"
	viceAgentSubclassSlug    = "class/hunter-nin/group/hunters-creeds/vice-agent"
	voidWalkerSubclassSlug   = "class/hunter-nin/group/hunters-creeds/void-walker"
	wolvesLegacySubclassSlug = "class/hunter-nin/group/hunters-creeds/wolves-legacy"
)

// Blade Warden's own always-on flat bonuses layered onto whichever equipped
// weapon matches the character's Warden Weapon pick (Warden's Proficiency,
// 3rd level) — consulted by buildAttacks (characters.go) via
// hunterNinWardenWeaponBonuses below, the same "granted feature slug ->
// flat bonus" shape bukijutsuFocusBonus/weaponFocusBonusSet already use.
const (
	bladeWardenAggressiveAttackSlug = "class/hunter-nin/group/hunters-creeds/blade-warden/feature/aggressive-attack"
	bladeWardenBladesAggressionSlug = "class/hunter-nin/group/hunters-creeds/blade-warden/feature/blades-aggression"
)

// huntersExploitAutoGrants: the granting feature slug -> the one Hunters
// Exploit option_slug it grants for free, outside the known-exploit cap.
var huntersExploitAutoGrants = map[string]string{
	"class/hunter-nin/group/hunters-creeds/blade-warden/feature/blades-prey":          "class/hunter-nin/option/hunters-exploits/wardens-assault",
	"class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/medicinal-blade":     "class/hunter-nin/option/hunters-exploits/festering-siphonage",
	"class/hunter-nin/group/hunters-creeds/grave-stalker/feature/shadow-stalker":      "class/hunter-nin/option/hunters-exploits/shadow-step",
	"class/hunter-nin/group/hunters-creeds/arsenalist/feature/tools-of-the-trade":     "class/hunter-nin/option/hunters-exploits/sharp-rain",
	"class/hunter-nin/group/hunters-creeds/undertaker/feature/poisonous-embrace":      "class/hunter-nin/option/hunters-exploits/incurable-affliction",
	"class/hunter-nin/group/hunters-creeds/vice-agent/feature/greeds-shell":           "class/hunter-nin/option/hunters-exploits/foxs-sin",
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/blink":                 "class/hunter-nin/option/hunters-exploits/void-assault",
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-body": "class/hunter-nin/option/hunters-exploits/deflection",
}

// huntersExploitSubclassLocks: the same 8 exclusive exploits, keyed the
// other direction — an option_slug here can only ever be manually picked
// (i.e. appear in a character's own Available list) by a character whose
// own subclass matches. A character of the matching subclass never needs
// to pick it manually — huntersExploitAutoGrants already grants it for
// free — so in practice this only ever filters the option OUT of every
// OTHER subclass's picker.
var huntersExploitSubclassLocks = map[string]string{
	"class/hunter-nin/option/hunters-exploits/wardens-assault":      bladeWardenSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/festering-siphonage":  necroticHandSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/shadow-step":          graveStalkerSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/sharp-rain":           arsenalistSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/incurable-affliction": undertakerSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/foxs-sin":             viceAgentSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/void-assault":         voidWalkerSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/deflection":           wolvesLegacySubclassSlug,
}

// lethalPrecisionOptions: Lethal Precision's own 2-option Taijutsu/Bukijutsu
// table (class_features level 1, "Select one between Taijutsu & Bukijutsu.
// You can cast the chosen Jutsu type using Dexterity in place of Strength
// for all calculations. You cannot switch this choice later.") — hand-
// curated straight from the feature's own text, not class_options rows,
// same reasoning as defensiveTacticsOptions below. Slugs match
// AttackKinds' own Kind strings lowercased ("taijutsu"/"bukijutsu") so
// charsheet.go's JutsuAttacks loop can match the pick directly. Only the
// PICK is tracked here (cap 1, no re-cap — see lethalPrecisionCap); the
// Dexterity-for-Strength ability swap itself is applied automatically once
// picked, the same way the Genjutsu Charisma-swap feats already are.
var lethalPrecisionOptions = []hunterPickOption{
	{Slug: "taijutsu", Name: "Taijutsu", Description: "Cast Taijutsu using Dexterity in place of Strength for all calculations."},
	{Slug: "bukijutsu", Name: "Bukijutsu", Description: "Cast Bukijutsu using Dexterity in place of Strength for all calculations."},
}

// defensiveTacticsOptions: Defensive Tactics' own 4-option catalog, hand-
// curated straight from class_features (the book embeds these options
// directly in the granting feature's text, not as class_options rows) —
// same reasoning as Taijutsu's handWrapsOfPassionOptions.
var defensiveTacticsOptions = []hunterPickOption{
	{Slug: "escaping-danger", Name: "Escaping Danger", Description: "Attacks of opportunity and attacks made as a result of a creature's Reaction, against you are made at disadvantage."},
	{Slug: "unbroken-will", Name: "Unbroken Will", Description: "You have advantage on saving throws to resist any Mental or Sensory Condition."},
	{Slug: "hunters-revenge", Name: "Hunter's Revenge", Description: "When you are hit by a creature's attack, the next time you deal damage to that creature you can Lethal Attack ignoring its normal trigger requirements."},
	{Slug: "evasion", Name: "Evasion", Description: "When you are subjected to an effect that allows you to make a Dexterity saving throw to take only half damage, you instead take no damage if you succeed on a saving throw, and only half damage if you fail."},
}

// wardenWeaponPropertyOptions: Blade Warden's own 3-option Warden Weapon
// Property table (cap 1->2@10th), granted alongside the Warden Weapon TYPE
// pick by the same Warden's Proficiency (3rd level) sentence. Transcribed
// from the 17th-level "Superior Offense" subclass_features row's own tail
// text, NOT Warden's Proficiency's own row — rules.db mistags this table
// there, the same systematic mistagging every one of Hunter-Nin's 6
// technique/property tables has (see the sibling option-catalog comments
// below). Entirely informational: which propert(ies) a Warden Weapon has
// is tracked here; Flurry Attack's bonus-action extra attack, Mortal's
// ignore-temp-HP, and Fatal's no-reaction-on-first-hit all stay narrated.
var wardenWeaponPropertyOptions = []hunterPickOption{
	{Slug: "flurry-attack", Name: "Flurry Attack", Description: "A weapon with the Flurry Attack property can be used to make two weapon attacks as a Bonus Action. Bonus Action attacks made in this way do not add your ability modifier to damage."},
	{Slug: "mortal", Name: "Mortal", Description: "A Weapon with the Mortal property ignores Temporary hit points, instead dealing damage directly to a creature's hit points once per turn."},
	{Slug: "fatal", Name: "Fatal", Description: "A Weapon with the Fatal property cannot have its damage reacted to when used as a part of your first weapon attack or Bukijutsu attack, per turn."},
}

// medicalAssassinationTechniques: Necrotic Hand's own 9-option Medical
// Assassination Technique table (cap 1->2@10th), augmenting the Necrosis
// jutsu when cast. Transcribed from the 7th-level "Necrotic Touch"
// subclass_features row's own tail text, NOT Medical Proficiency's own
// row — same mistagging shape as every sibling subclass. Entirely
// informational: each technique's on-hit rider (Sepsis granting
// Laceration, Destructive's +1d12, etc.) stays narrated when Necrosis is
// cast — only which technique is known is tracked.
var medicalAssassinationTechniques = []hunterPickOption{
	{Slug: "cell-failure", Name: "Cell Failure", Description: "A creature who fails their constitution saving throw also gains one rank of the Weakened condition."},
	{Slug: "overdose", Name: "Overdose", Description: "A creature who fails their constitution saving throw instead becomes Berserk, in place of Envenomed."},
	{Slug: "sedative", Name: "Sedative", Description: "A creature who fails their constitution saving throw instead gains one rank of the slowed condition in place of Envenomed."},
	{Slug: "sepsis", Name: "Sepsis", Description: "A creature who fails their constitution saving throw also gains one rank of Laceration."},
	{Slug: "viral", Name: "Viral", Description: "A creature who fails their constitution saving throw also gains one rank of Concussed."},
	{Slug: "destructive", Name: "Destructive", Description: "This jutsu's base Damage is increased by +1d12."},
	{Slug: "surgical", Name: "Surgical", Description: "This jutsu adds your Casting ability modifier to the damage rolls."},
	{Slug: "narcotic", Name: "Narcotic", Description: "This jutsu reduces its upcast cost by 1."},
	{Slug: "salutary", Name: "Salutary", Description: "This jutsu reduces its base cost by 2."},
}

// shadowAssassinationTechniques: Grave Stalker's own 8-option Shadow
// Assassination Technique table (cap 1->2@10th), augmenting the Weapons of
// Darkness jutsu when cast. Transcribed from the 7th-level "Master
// Ambusher" subclass_features row's own tail text, NOT Stalkers
// Proficiency's own row — same mistagging shape as every sibling
// subclass. Entirely informational: each technique's rider (Shadow
// Sword's Lethal+Deadly properties, Dark Lethality's crit-range bump,
// etc.) stays narrated when the Shadow Weapon is used — only which
// technique is known is tracked.
var shadowAssassinationTechniques = []hunterPickOption{
	{Slug: "shadow-shuriken", Name: "Shadow Shuriken", Description: "This jutsu can take on the shape of a shuriken of your description at will. Ranged attacks with this shadow weapon have a range of 60 feet, and can be made as a Bonus Action, once per turn."},
	{Slug: "shadow-tanto", Name: "Shadow Tanto", Description: "This jutsu can take on the shape of a Tanto of your description at will. Melee attacks made with this Shadow weapon adds your ability modifier to the damage rolled."},
	{Slug: "shadow-battle-wire", Name: "Shadow Battle Wire", Description: "This jutsu can take on the shape of Battle Wires of your description at will. You can attempt a grapple check from up to 30 feet away, using Dexterity or Wisdom (illusion) in place of Athletics contested by the target's Wisdom (Illusions)."},
	{Slug: "shadow-sword", Name: "Shadow Sword", Description: "This jutsu can take on the shape of a sword of your description at will. This weapon gains the Lethal and Deadly weapon properties."},
	{Slug: "shadow-iron-claw", Name: "Shadow Iron Claw", Description: "This jutsu can take on the shape of Iron Claws of your description at will. This weapon has the Critical and Tactical weapon properties."},
	{Slug: "dark-blood", Name: "Dark Blood", Description: "If you would deal 15 damage with this weapon in a single attack, the target gains +1 rank of Demoralized."},
	{Slug: "dark-lethality", Name: "Dark Lethality", Description: "Increase this jutsu's critical threat range by +1."},
	{Slug: "dark-efficiency", Name: "Dark Efficiency", Description: "This jutsu's base cost is reduced by 2."},
}

// toxicAssassinationTechniques: Undertaker's own 8-option Toxic
// Assassination Technique table (cap 1->2@10th), augmenting whichever
// jutsu or weapon the Toxic Chakra is infused into. Transcribed from the
// 7th-level "False Faces" subclass_features row's own tail text, NOT
// Toxic Proficiency's own row — same mistagging shape as every sibling
// subclass. Entirely informational: each toxin's on-hit rider condition
// stays narrated when the Toxic Chakra is activated — only which
// technique is known is tracked.
var toxicAssassinationTechniques = []hunterPickOption{
	{Slug: "aconite", Name: "Aconite", Description: "On a hit or failed saving throw, once per turn, the target loses the ability to communicate effectively. They also find it difficult to breathe, suffering a -2 to Constitution ability checks and saving throws until the end of its next turn."},
	{Slug: "akee", Name: "Akee", Description: "On a hit or failed saving throw, once per turn, the target gains the weakened condition. If the target takes a reaction before the end of its next turn, at the conclusion of its reaction it gains the restrained condition until the end of its next turn."},
	{Slug: "belladonna", Name: "Belladonna", Description: "On a hit or failed saving throw, once per turn, the target loses the ability to discern allies from hostiles. If a jutsu or Area of Effect would allow them to choose targets, they cannot exclude their allies at will until the end of their next turn."},
	{Slug: "daphne", Name: "Daphne", Description: "On a hit or failed saving throw, once per turn, the target gains the bruised condition."},
	{Slug: "fire-salamander", Name: "Fire Salamander", Description: "On a hit or failed saving throw, once per turn, the target gains one rank of the corroded condition."},
	{Slug: "foxglove", Name: "Foxglove", Description: "On a hit or failed saving throw, once per turn, the target gains one rank of the bleeding condition."},
	{Slug: "frost-frog", Name: "Frost Frog", Description: "On a hit or failed saving throw, once per turn, the target gains the chilled condition. If the target takes a Bonus Action on its next turn, it gains the slowed condition until the end of its next turn."},
	{Slug: "hemlock", Name: "Hemlock", Description: "On a hit or failed saving throw, once per turn, the target gains the envenomed condition."},
}

// viceAssassinationTechniques: Vice Agent's own 6-option Vice Assassination
// Technique table (cap 1->2@10th), augmenting the Shadow Bite jutsu when
// cast. Transcribed from the 7th-level "Arrogance's Influence"
// subclass_features row's own tail text, NOT Sin's Proficiency's own
// row — same mistagging shape as every sibling subclass. Entirely
// informational: each vice's forced-save rider effect stays narrated when
// Shadow Bite is cast — only which technique is known is tracked.
var viceAssassinationTechniques = []hunterPickOption{
	{Slug: "anger", Name: "Anger", Description: "On a hit, this jutsu forces a Constitution saving throw, injecting feelings of malice and aggression into the target. On a failed save, until the end of the target's next turn, if they make an attack or cast a jutsu that targets a creature, they must target the creature closest to them."},
	{Slug: "arrogance", Name: "Arrogance", Description: "On a hit, this jutsu forces a Constitution saving throw, injecting feelings of pride into the target. On a failed save, until the end of the target's next turn, they cannot take a reaction that would reduce damage or grant themselves temporary hit points."},
	{Slug: "envy", Name: "Envy", Description: "On a hit, this jutsu forces a Constitution saving throw, injecting feelings of jealousy into its target. On a failed save, until the end of the target's next turn, when they would see an allied creature attempt to aid a creature other than themselves, they must attempt to interrupt the aiding effect."},
	{Slug: "gluttony", Name: "Gluttony", Description: "On a hit, this jutsu forces a Constitution saving throw, injecting feelings of excess into its target. On a failed save, until the end of the target's next turn, when they would cast a jutsu that would aid themselves, they must recast the same jutsu targeting themselves using their remaining Action and/or Bonus Actions, ignoring the jutsu's normal casting time."},
	{Slug: "greed", Name: "Greed", Description: "On a hit, this jutsu forces a Constitution saving throw, injecting feelings of greed into its target. On a failed save, until the end of the target's turn, when they would cast a jutsu that would allow them to aid their allies, they instead can only aid themselves."},
	{Slug: "sloth", Name: "Sloth", Description: "On a hit, this jutsu forces a Constitution saving throw, injecting feelings of inactivity into its target. On a failed save, until the end of the target's next turn, they gain 1 rank of Slow."},
}

// voidAssassinationTechniques: Void Walker's own 5-option Void
// Assassination Technique table (cap 1->2@10th), augmenting the Chakra
// Mark jutsu when cast. Transcribed from the 7th-level "Vorpal Strike"
// subclass_features row's own tail text, NOT Stalker Proficiency's own
// row — same mistagging shape as every sibling subclass. Entirely
// informational: each technique's penalty/teleport rider stays narrated
// when triggered — only which technique is known is tracked.
var voidAssassinationTechniques = []hunterPickOption{
	{Slug: "aether", Name: "Aether", Description: "A creature marked with this jutsu who would make a saving throw against a jutsu with the Fuinjutsu keyword suffers a -1 penalty, once per turn. This penalty increases to -2 at 10th and -3 at 17th."},
	{Slug: "arcane", Name: "Arcane", Description: "You can mark a construct, structure or surface of your choice (at no action cost). When you do, you can teleport to a space within 5 feet of the marked construct, structure or surface as a Reaction to taking damage. The seal then breaks, needing to be reapplied. You can only have 1 seal active in this way at a time."},
	{Slug: "enigma", Name: "Enigma", Description: "A creature marked with this jutsu who would take damage from a Ninjutsu or Genjutsu suffers a -1 penalty to their first attack roll before the end of their next turn. This penalty increases to -3 at 10th and -5 at 17th levels."},
	{Slug: "disruptive", Name: "Disruptive", Description: "A creature marked with this jutsu who would take damage from a Taijutsu or Bukijutsu increases the cost of their next jutsu before the beginning of your next turn by +3. This cost increases to +6 at 10th and +9 at 17th levels."},
	{Slug: "rune", Name: "Rune", Description: "You can mark a weapon or weapon stack with the thrown property (at no action cost). When you deal weapon damage with a ranged weapon attack using the marked weapon, you can spend 10 feet of movement to teleport to a space within 5 feet of the damaged creature."},
}

// prostheticAttachmentOptions: Wolves Legacy's own 7-option Prosthetic
// Attachments table (cap 2->4@10th, plus a shared Proficiency-Bonus-per-
// rest use pool — see customResourceGrants' "prosthetic_attachments" key
// in custom_resources.go). Transcribed from the 7th-level "Eyes of a
// Shinobi" subclass_features row's own tail text, NOT Wolf's Proficiency's
// own row — a mistagging quirk like the other 5 subclasses, but landing on
// a DIFFERENT subclass feature at a different level (7th vs. the pick's
// own granting level of 3rd); the pick/pool's own MinLevel still matches
// Wolf's Proficiency's own 3rd-level grant sentence, not this row's 7th.
// Entirely informational: each attachment's activated effect (Elemental
// Vent's cone damage, Sabimaru's poison, Mist Raven's teleport, etc.)
// stays narrated — only which 2-4 attachments are known, and how many
// uses remain this rest, are tracked.
var prostheticAttachmentOptions = []hunterPickOption{
	{Slug: "divine-abduction", Name: "Divine Abduction", Description: "Your arm is outfitted with a fan you can quickly pull out to gain a boost to stealth. As an action you cloak yourself in divine winds, giving yourself advantage on your next stealth check."},
	{Slug: "elemental-vent", Name: "Elemental Vent", Description: "In your arm you have equipped a small cannon that unleashes a blast of fire on your foes. As an action you can force every creature in a 15-foot cone to make a Dexterity saving throw, taking 4d6 fire damage on a failed save and half as much on a success. If you have access to another nature release, you can instead use that element's corresponding damage type (Water = Cold). This damage increases by +2d6 at 10th and 17th levels."},
	{Slug: "loaded-shuriken", Name: "Loaded Shuriken", Description: "Your arm is fitted with exceedingly sharp shuriken, launched from a wheel bound with extreme tension. As a Bonus Action you may select a creature you can see and make a ranged weapon attack as if using shuriken, with double its normal range. On a hit, you deal 4d4 slashing damage and inflict 1 rank of bleed. The damage die and ranks of bleed increase by +2 at 10th and 17th levels."},
	{Slug: "loaded-umbrella", Name: "Loaded Umbrella", Description: "Your arm is outfitted with an indestructible, iron-ribbed umbrella made to absorb damage and disperse it harmlessly around you. As a reaction when you would take damage, you open this umbrella, intercepting the triggering damage and the next instance of damage by 3d4. The damage you reduce is increased by +3d4 at 10th and 17th levels."},
	{Slug: "mist-raven", Name: "Mist Raven", Description: "Your arm is fitted with a special feather that gives you the ability to teleport in a cloud of mist to avoid taking consistent damage. As a reaction when you would take damage, you teleport up to 30 feet away. The distance you teleport is increased by +15 feet at 10th and 17th levels."},
	{Slug: "sabimaru", Name: "Sabimaru", Description: "Your arm is equipped with a secret poison, with which you can coat your weapon to quickly poison your foes when you make a melee attack with a weapon or make a melee Taijutsu attack as part of casting a Bukijutsu. As part of the same action used to attack, you can spend a use of your Prosthetic Attachments. On a hit you deal an additional 3d6 poison damage. The target must make a Constitution saving throw. On a failed save, they gain the envenomed condition for 1 minute. If you score a critical hit with this attack, the target instead gains 3 ranks of the Envenomed condition."},
	{Slug: "shinobi-firecracker", Name: "Shinobi Firecracker", Description: "Your arm is equipped with a firecracker that is made for disorienting your foes. As a Bonus Action you can force every creature in a 10-foot cube originating from you to make a Wisdom saving throw. On a failed save, they become dazed and blinded until the beginning of their next turn."},
}

// wolfTechniqueOptions: Wolves Legacy's own 10th-level Wolf Techniques
// catalog ("Select 1 of the following jutsu. The selected jutsu are now
// counted as your Wolf Techniques. The jutsu selected are added to your
// Jutsu known and also gain their listed effects" — Wolf Techniques'
// own subclass_features row). The book lists 8 named jutsu; only 6 resolve
// to real jutsu rows in rules.db (Weapon Deflect, Weapon Break, Earth
// Breaker, Triple Windmill Blades, Ichimonji, Judgement Cut). Kunai Assault
// and Reapers Swing have no matching jutsu row under any name or spelling
// variant — a data-ingestion gap out of scope here, so both are omitted
// from the catalog entirely rather than offered as a pick with nothing to
// grant. Each Description is the feature's own listed rider verbatim (see
// wolfTechniqueJutsuGrants for the "also learn the jutsu" side effect this
// pick shares with arsenalItems' Manipulated Tools entries) — the rider
// itself stays narrated, same as every other Hunter-Nin technique table.
// Deliberately NOT modeled: the alternate casting cost the feature also
// grants ("cast a jutsu counted as your Wolf Technique by spending two uses
// of your Prosthetic Attachment instead of spending chakra... always cast
// as if it were B-Rank... ignores its component requirement") — no analog
// exists anywhere in the jutsu-casting-cost pipeline for an alternate
// resource substituting for Chakra, so a player using this still spends
// Prosthetic Attachment uses (tracked in Special Resources) and narrates
// the cast manually.
var wolfTechniqueOptions = []hunterPickOption{
	{Slug: "jutsu/weapon-deflect", Name: "Weapon Deflect", Description: "Learns the Weapon Deflect jutsu, counted as your Wolf Technique. Bonus damage increases to a d10."},
	{Slug: "jutsu/weapon-break", Name: "Weapon Break", Description: "Learns the Weapon Break jutsu, counted as your Wolf Technique. Your weapon is treated as a B-Rank or a +2, whichever is higher in the given scenario."},
	{Slug: "jutsu/earth-breaker", Name: "Earth Breaker", Description: "Learns the Earth Breaker jutsu, counted as your Wolf Technique. Increase the saving throw damage die to a d10, and instead make two melee taijutsu attacks against a prone target."},
	{Slug: "jutsu/triple-windmill-blades", Name: "Triple Windmill Blades", Description: "Learns the Triple Windmill Blades jutsu, counted as your Wolf Technique. The jutsu's range is increased to 90 feet."},
	{Slug: "jutsu/ichimonji", Name: "Ichimonji", Description: "Learns the Ichimonji jutsu, counted as your Wolf Technique. Make one additional attack."},
	{Slug: "jutsu/judgement-cut", Name: "Judgement Cut", Description: "Learns the Judgement Cut jutsu, counted as your Wolf Technique. Reduce the additional cost to target the same space to 4 chakras."},
}

// wolfTechniqueJutsuGrants: wolfTechniqueOptions' own Slug -> the jutsu slug
// charstore.AddJutsu should learn when that pick is added — "The jutsu
// selected are added to your Jutsu known" (Wolf Techniques' own text). Every
// wolfTechniqueOptions entry IS a jutsu slug already (unlike arsenalItems,
// which mixes 3 jutsu among 11 non-jutsu items), so this is an identity map
// kept as its own named lookup for the same "explicit map, not a prefix
// check" reasoning arsenalJutsuGrants documents.
var wolfTechniqueJutsuGrants = map[string]string{
	"jutsu/weapon-deflect":         "jutsu/weapon-deflect",
	"jutsu/weapon-break":           "jutsu/weapon-break",
	"jutsu/earth-breaker":          "jutsu/earth-breaker",
	"jutsu/triple-windmill-blades": "jutsu/triple-windmill-blades",
	"jutsu/ichimonji":              "jutsu/ichimonji",
	"jutsu/judgement-cut":          "jutsu/judgement-cut",
}

// arsenalItems: Arsenalist's own 14-option Arsenal catalog (cap 4->6@10th),
// transcribed directly from Arsenal's Proficiency's own row (unlike its 6
// sibling subclasses' tables, this one is NOT mistagged elsewhere). Three
// entries are real jutsu (Manipulated Tools: Blade Kick/Rain/Wall) — see
// arsenalJutsuGrants, which learns the matching jutsu automatically when
// one of these three is picked, the same generic charstore.AddJutsu call
// the Jutsu tab's own "Learn" button uses. Everything else about the
// Arsenal stays Group 2/3 informational text (Arsenal Jutsu's -1/-2/-3
// cost reduction, Arsenal Weapons' +2/+4/+6 damage and die-step, the
// Lethal-Attack trigger override) — only which 4-6 items are in the
// Arsenal, and the 3 jutsu grants, are tracked.
var arsenalItems = []hunterPickOption{
	{Slug: "jutsu/manipulated-tools-blade-kick", Name: "Manipulated Tools: Blade Kick", Description: "Learns the Manipulated Tools: Blade Kick jutsu, added to your Arsenal."},
	{Slug: "jutsu/manipulated-tools-blade-rain", Name: "Manipulated Tools: Blade Rain", Description: "Learns the Manipulated Tools: Blade Rain jutsu, added to your Arsenal."},
	{Slug: "jutsu/manipulated-tools-blade-wall", Name: "Manipulated Tools: Blade Wall", Description: "Learns the Manipulated Tools: Blade Wall jutsu, added to your Arsenal."},
	{Slug: "paper-bombs", Name: "Paper Bombs", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "explosive-tag-balls", Name: "Explosive Tag Balls", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "fire-bombs", Name: "Fire Bombs", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "flash-tags", Name: "Flash Tags", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "ice-bombs", Name: "Ice Bombs", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "poison-tags", Name: "Poison Tags", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "shock-bombs", Name: "Shock Bombs", Description: "Added to your Arsenal. Arsenal Tools gain an escalating cost reduction, die-size increase, range, or DC bonus — see Arsenal's Proficiency's own text."},
	{Slug: "thrown-weapons", Name: "Weapons with the Thrown Keyword", Description: "Added to your Arsenal. Arsenal Weapons gain a die-size increase, the Hidden Weapon property, and a +2/+4/+6 damage bonus — see Arsenal's Proficiency's own text."},
	{Slug: "multiattack-weapons", Name: "Weapons with the Multiattack Keyword", Description: "Added to your Arsenal. Arsenal Weapons gain a die-size increase, the Hidden Weapon property, and a +2/+4/+6 damage bonus — see Arsenal's Proficiency's own text."},
	{Slug: "light-weapons", Name: "Weapons with the Light Keyword", Description: "Added to your Arsenal. Arsenal Weapons gain a die-size increase, the Hidden Weapon property, and a +2/+4/+6 damage bonus — see Arsenal's Proficiency's own text."},
	{Slug: "finesse-weapons", Name: "Weapons with the Finesse Keyword", Description: "Added to your Arsenal. Arsenal Weapons gain a die-size increase, the Hidden Weapon property, and a +2/+4/+6 damage bonus — see Arsenal's Proficiency's own text."},
}

// arsenalJutsuGrants: the 3 arsenalItems slugs that are real jutsu rows,
// mapped to the jutsu slug charstore.AddJutsu should learn when that item
// is picked — "If you select a jutsu, you learn it" (Arsenal's
// Proficiency's own text). Keyed by the pick's own Slug rather than a
// prefix check, the same explicit-map shape huntersExploitAutoGrants
// already uses.
var arsenalJutsuGrants = map[string]string{
	"jutsu/manipulated-tools-blade-kick": "jutsu/manipulated-tools-blade-kick",
	"jutsu/manipulated-tools-blade-rain": "jutsu/manipulated-tools-blade-rain",
	"jutsu/manipulated-tools-blade-wall": "jutsu/manipulated-tools-blade-wall",
}

// hunterNinClassLevel returns the character's own Hunter-Nin class level, or
// 0 if they have none — mirrors taijutsuSpecialistClassLevel.
func (s *server) hunterNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, hunterNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// hunterNinPatternPassiveRows returns synthetic grantedFeatureRow entries for
// the character's picked Hunters Patterns that carry an unconditional passive
// trait (Horror Films' Fear immunity, Illicit Literature's Charmed immunity —
// see passiveTraitGrants). Hunters Patterns are class_options catalog rows,
// not class/subclass/clan features, so they never appear in
// loadGrantedFeatures' output on their own; callers that want
// computePassiveTraits to see them must append this slice at the call site,
// the same "append, don't merge into the stored list" treatment
// mergeFeatFeatures gives feats. Level/SourceLabel/Description are left zero
// — computePassiveTraits only reads Slug (against passiveTraitGrants) and
// Name (for the Sources tooltip).
func (s *server) hunterNinPatternPassiveRows(characterID int64) ([]grantedFeatureRow, error) {
	picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickPattern)
	if err != nil {
		return nil, err
	}
	names := map[string]string{
		"class/hunter-nin/option/hunters-patterns/horror-films":       "Horror Films",
		"class/hunter-nin/option/hunters-patterns/illicit-literature": "Illicit Literature",
	}
	var rows []grantedFeatureRow
	for _, slug := range picks {
		if name, ok := names[slug]; ok {
			rows = append(rows, grantedFeatureRow{Slug: slug, Name: name, SourceLabel: "Hunters Pattern"})
		}
	}
	return rows, nil
}

// hunterNinSubclassSlug resolves the character's own chosen Hunter's Creed,
// if any — mirrors taijutsuSpecialistSubclassSlug/puppetMasterSubclassSlug
// exactly.
func (s *server) hunterNinSubclassSlug(characterID int64) (slug, name string, err error) {
	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return "", "", err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var sc string
		if err := subRows.Scan(&sc); err != nil {
			subRows.Close()
			return "", "", err
		}
		subclassSlugs = append(subclassSlugs, sc)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return "", "", err
	}

	for _, sc := range subclassSlugs {
		var n, classSlug string
		if err := s.rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, sc,
		).Scan(&n, &classSlug); err != nil {
			continue // a stale/removed subclass slug just isn't a match
		}
		if classSlug == hunterNinSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// hunterNinWardenWeaponBonuses resolves Blade Warden's own Warden Weapon
// pick (Warden's Proficiency, 3rd level) plus whether Aggressive Attack
// (7th) and Blade's Aggression (10th) are granted — both flat, always-on
// bonuses that apply only to the equipped weapon matching the pick.
// wardenWeaponSlug is "" for a character with no pick yet (or no Hunter-Nin
// levels at all), which buildAttacks treats as "never matches any equipped
// item" — same "empty means untouched" shape weaponFocusBonusSet already
// uses. The pick/pool split mirrors Hunters Exploits: which weapon TYPE is
// chosen is tracked here; Aggressive Attack's dual-ability damage and
// Blade's Aggression's die-step are both applied automatically once known,
// nothing about either is spent or requires the player to remember to
// trigger it.
func (s *server) hunterNinWardenWeaponBonuses(characterID int64, sheet *charsheet.Sheet) (wardenWeaponSlug string, aggressiveAttack, bladesAggression bool, err error) {
	picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickWardenWeapon)
	if err != nil || len(picks) == 0 {
		return "", false, false, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return "", false, false, err
	}
	aggressiveAttack = hasGrantedFeature(grantedFeatures, bladeWardenAggressiveAttackSlug)
	bladesAggression = hasGrantedFeature(grantedFeatures, bladeWardenBladesAggressionSlug)
	return picks[0], aggressiveAttack, bladesAggression, nil
}

// arsenalWeaponKeywordSlugs maps the 4 weapon-keyword arsenalItems picks to
// the lowercase property-string keyword buildAttacks matches against an
// equipped weapon's own properties (the same substring-match mechanism
// buildAttacks already uses for its own ability-selection logic).
var arsenalWeaponKeywordSlugs = map[string]string{
	"thrown-weapons":      "thrown",
	"multiattack-weapons": "multiattack",
	"light-weapons":       "light",
	"finesse-weapons":     "finesse",
}

// hunterNinArsenalWeaponBonus resolves Arsenalist's own Arsenal Weapons
// clause (Arsenal's Proficiency, 3rd level): "Increase the [Weapon's
// Damage] die by 1 Step, and they gain the Hidden Weapon property. The
// Weapon also gains a +2 bonuses to damage from weapon attacks. This bonus
// increases to +4 at 10th and +6 at 17th level." keywords holds the
// lowercase property keywords ("thrown"/"multiattack"/"light"/"finesse")
// for whichever of the 4 weapon-keyword arsenalItems entries the character
// has picked; it is nil for a character with none picked (or below 3rd
// Hunter-Nin level), which buildAttacks treats as "never matches any
// equipped item" — same "empty means untouched" shape
// hunterNinWardenWeaponBonuses already uses. bonus is the level-gated flat
// damage bonus (+2/+4/+6), 0 when keywords is nil. The Hidden Weapon
// property tag and the Lethal-Attack trigger override (crit-on-a-failed-
// save even without an attack roll) have no sheet field to reach and stay
// entirely manual — only the keyword match, die-step, and flat damage
// bonus are tracked here.
func (s *server) hunterNinArsenalWeaponBonus(characterID int64) (keywords map[string]bool, bonus int, err error) {
	picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickArsenalItem)
	if err != nil {
		return nil, 0, err
	}
	for _, slug := range picks {
		if kw, ok := arsenalWeaponKeywordSlugs[slug]; ok {
			if keywords == nil {
				keywords = map[string]bool{}
			}
			keywords[kw] = true
		}
	}
	if len(keywords) == 0 {
		return nil, 0, nil
	}
	level, err := s.hunterNinClassLevel(characterID)
	if err != nil {
		return nil, 0, err
	}
	if level < 3 {
		return nil, 0, nil
	}
	switch {
	case level >= 17:
		bonus = 6
	case level >= 10:
		bonus = 4
	default:
		bonus = 2
	}
	return keywords, bonus, nil
}

// lethalAttackDice reads Lethal Attack's own live chart (v_class_level_resources,
// resource_name "Lethal Attack") rather than hand-transcribing it — same
// "read the chart, don't retype it" precedent martialDiceMax established.
func (s *server) lethalAttackDice(level int) (string, error) {
	var value sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = 'Lethal Attack'`,
		hunterNinSlug, level,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// huntersPatternsCap: Hunters Patterns' own chart (1 base, 2 at 9th, 3 at
// 15th) — no v_class_level_resources row exists for this one (confirmed:
// only "Lethal Attack" is charted for this class), so it's hand-curated
// the same way martialDieSize is.
func huntersPatternsCap(level int) int {
	switch {
	case level >= 15:
		return 3
	case level >= 9:
		return 2
	case level >= 2:
		return 1
	default:
		return 0
	}
}

// huntersExploitsCap: Hunters Exploits' own known-count chart (2 at 3rd, 3
// at 10th, 4 at 17th) — the SEPARATE "how many times per short rest can you
// use one" number lives in customResourceGrants (custom_resources.go),
// keyed off Proficiency Bonus instead of a level chart.
func huntersExploitsCap(level int) int {
	switch {
	case level >= 17:
		return 4
	case level >= 10:
		return 3
	case level >= 3:
		return 2
	default:
		return 0
	}
}

// huntersExploitingExploitsFeatSlug identifies Exploiting Exploits, a feat
// requiring Hunter-Nin level 4+ that stacks an extra Hunters Exploit slot on
// top of real Hunter-Nin levels.
const huntersExploitingExploitsFeatSlug = "feat/class/exploiting-exploits"

// huntersExploitingExploitsBonus: Exploiting Exploits' own text — "You gain
// 1 Additional Hunters Exploit. You gain an additional Hunters Exploit at
// 7th and 15th Hunter-Nin levels" — 1 base, +1 at 7th, +1 at 15th (up to 3
// total). The feat's own prerequisite ("Hunter-Nin, Level 4+") means this
// only ever applies alongside real class levels — same "no effective-level
// substitution needed" situation mirageExhibitionMirageBonus (genjutsu.go)
// already handles for Genjutsu.
func huntersExploitingExploitsBonus(hunterLevel int) int {
	n := 1
	if hunterLevel >= 7 {
		n++
	}
	if hunterLevel >= 15 {
		n++
	}
	return n
}

// lethalPrecisionCap: Lethal Precision's own Taijutsu/Bukijutsu pick — 1 at
// 1st, no re-cap ("You cannot switch this choice later" has no "select a
// second" clause at any later level, unlike Warden Weapon Property right
// alongside wardenWeaponCap's own no-re-cap pick).
func lethalPrecisionCap(level int) int {
	if level >= 1 {
		return 1
	}
	return 0
}

// defensiveTacticsCap: 1 at 6th, 2 at 11th, 3 at 17th.
func defensiveTacticsCap(level int) int {
	switch {
	case level >= 17:
		return 3
	case level >= 11:
		return 2
	case level >= 6:
		return 1
	default:
		return 0
	}
}

// wardenWeaponCap: Blade Warden's own Warden Weapon TYPE pick — 1 at 3rd,
// no re-cap ("This weapon is hereby treated as your Warden Weapon" has no
// "select a second" clause, unlike the Property pick right after it in the
// same sentence).
func wardenWeaponCap(level int) int {
	if level >= 3 {
		return 1
	}
	return 0
}

// subclassTechniqueCap: the shared 1->2@10th chart 5 of Hunter-Nin's 8
// Creeds' own 3rd-level technique/property picks all use verbatim —
// Blade Warden's Warden Weapon Property, Necrotic Hand's Medical, Grave
// Stalker's Shadow, Undertaker's Toxic, Vice Agent's Vice, and Void
// Walker's Void Assassination Techniques all read "Select one of the
// following... You can select a second ... when you reach 10th level,"
// identical shape, just a different catalog.
func subclassTechniqueCap(level int) int {
	switch {
	case level >= 10:
		return 2
	case level >= 3:
		return 1
	default:
		return 0
	}
}

// arsenalItemCap: Arsenalist's own Arsenal chart — 4 at 3rd, 6 at 10th
// ("Select four of the following. You can select two more at 10th level").
func arsenalItemCap(level int) int {
	switch {
	case level >= 10:
		return 6
	case level >= 3:
		return 4
	default:
		return 0
	}
}

// prostheticAttachmentCap: Wolves Legacy's own Prosthetic Attachments
// chart — 2 at 3rd, 4 at 10th ("Select two of the following... You can
// select two more at 10th level").
func prostheticAttachmentCap(level int) int {
	switch {
	case level >= 10:
		return 4
	case level >= 3:
		return 2
	default:
		return 0
	}
}

// wolfTechniqueCap: Wolves Legacy's own Wolf Techniques chart — 1 at 10th,
// no re-cap ("Select 1 of the following jutsu" has no "select a second"
// clause anywhere in the feature's own text, unlike the 6 sibling
// technique/property tables above which all step to 2 at 10th).
func wolfTechniqueCap(level int) int {
	if level >= 10 {
		return 1
	}
	return 0
}

// stepWeaponDie steps a "[N]dX" damage dice string's die size up one notch
// on the standard d4->d6->d8->d10->d12 progression — Blade's Aggression's
// own text: "your Wardens Weapon's damage die is increased by 1 step."
// Every simple, non-Two-Handed/non-Heavy weapon a Warden Weapon can
// actually be uses d4, d6, or d8 (confirmed against the equipment table),
// so d10 and d12 are included only so a future data change can't silently
// under-step; a die size with no defined next step (d12, or anything that
// fails to parse as damageDicePattern expects) is returned unchanged
// rather than guessed at.
func stepWeaponDie(dice string) string {
	m := damageDicePattern.FindStringSubmatch(strings.TrimSpace(dice))
	if m == nil {
		return dice
	}
	sides, err := strconv.Atoi(m[2])
	if err != nil {
		return dice
	}
	steps := map[int]int{4: 6, 6: 8, 8: 10, 10: 12}
	next, ok := steps[sides]
	if !ok {
		return dice
	}
	return m[1] + "d" + strconv.Itoa(next)
}

// hunterPickOption is one entry in any of Hunter-Nin's catalog picks.
type hunterPickOption struct {
	Slug        string
	Name        string
	Description string
}

// lookupHunterPickOption finds one entry by Slug in a hand-curated
// []hunterPickOption catalog — shared by every handleHunterPickDetail case
// backed by one of the hand-curated lists in this file (as opposed to
// "warden-weapon", which is sourced live from the equipment table).
func lookupHunterPickOption(options []hunterPickOption, slug string) (name, description string, ok bool) {
	for _, o := range options {
		if o.Slug == slug {
			return o.Name, o.Description, true
		}
	}
	return "", "", false
}

// knownHunterPick is one entry on a Known list — either a player pick
// (has a remove button, counts against its own cap) or Granted true (one
// of the 8 subclass-exclusive Hunters Exploits, no remove button, doesn't
// count against the cap) — same shape Taijutsu's knownMartialTechnique
// already established.
type knownHunterPick struct {
	Slug    string
	Name    string
	Granted bool
}

// loadHunterOptionCatalog reads one of Hunter-Nin's two DB-backed catalogs
// (list_name "Hunters Patterns" or "Hunters Exploits") in full.
func (s *server) loadHunterOptionCatalog(listName string) ([]hunterPickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND list_name = ? ORDER BY name`, hunterNinSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hunterPickOption
	for rows.Next() {
		var o hunterPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// loadWardenWeaponCatalog reads Blade Warden's own Warden Weapon TYPE
// candidates directly off the equipment table — unlike the hand-curated
// option lists above, "Select one simple weapon type that you have
// proficiency in, that does not have the Two-Handed or Heavy properties"
// (Warden's Proficiency's own text) is a real filter over real equipment
// rows, the same "read the catalog, don't hand-transcribe it" shape
// loadWeaponFocusCatalog (weapon_specialist.go) already established.
// Description is composed from the weapon's own damage dice/type/
// properties rather than left blank, so the picker still gets a real
// rollover tooltip.
func (s *server) loadWardenWeaponCatalog() ([]hunterPickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(damage_dice, ''), COALESCE(damage_type, ''), COALESCE(properties, '')
		FROM equipment
		WHERE kind = 'weapon' AND weapon_category = 'simple'
		  AND properties NOT LIKE '%Two-Handed%' AND properties NOT LIKE '%Heavy%'
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hunterPickOption
	for rows.Next() {
		var slug, name, damageDice, damageType, properties string
		if err := rows.Scan(&slug, &name, &damageDice, &damageType, &properties); err != nil {
			return nil, err
		}
		out = append(out, hunterPickOption{
			Slug: slug, Name: name,
			Description: strings.TrimSpace(damageDice + " " + damageType + ". " + properties),
		})
	}
	return out, rows.Err()
}

// splitHunterPicks classifies a catalog against a character's stored picks
// for one category — shared by Patterns and Defensive Tactics; Exploits
// needs its own version below since it also merges subclass auto-grants
// and filters out the other 7 subclasses' locked exploits.
func splitHunterPicks(catalog []hunterPickOption, picked map[string]bool) (known []knownHunterPick, available []hunterPickOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownHunterPick{Slug: o.Slug, Name: o.Name})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// hunterTechniquesTabData is the sheet_hunter_techniques box's full data.
type hunterTechniquesTabData struct {
	LethalAttackDice string

	LethalPrecisionCap       int
	LethalPrecisionUsed      int
	KnownLethalPrecision     []knownHunterPick
	AvailableLethalPrecision []hunterPickOption

	PatternsCap       int
	PatternsUsed      int
	KnownPatterns     []knownHunterPick
	AvailablePatterns []hunterPickOption

	ExploitsCap       int
	ExploitsUsed      int
	KnownExploits     []knownHunterPick
	AvailableExploits []hunterPickOption

	DefensiveTacticsCap       int
	DefensiveTacticsUsed      int
	KnownDefensiveTactics     []knownHunterPick
	AvailableDefensiveTactics []hunterPickOption

	// The remaining 8 fields below are each gated on the character's own
	// chosen Hunter's Creed — every Cap stays 0 (hiding the sub-section,
	// same "don't leave other creeds looking at an empty box" treatment
	// Patterns/Exploits/Defensive Tactics already use) unless the matching
	// subclassSlug check in loadHunterTechniquesTabData passes.
	WardenWeaponCap       int // Blade Warden only
	WardenWeaponUsed      int
	KnownWardenWeapon     []knownHunterPick
	AvailableWardenWeapon []hunterPickOption

	WardenWeaponPropertyCap       int // Blade Warden only
	WardenWeaponPropertyUsed      int
	KnownWardenWeaponProperty     []knownHunterPick
	AvailableWardenWeaponProperty []hunterPickOption

	MedicalTechniqueCap       int // Necrotic Hand only
	MedicalTechniqueUsed      int
	KnownMedicalTechnique     []knownHunterPick
	AvailableMedicalTechnique []hunterPickOption

	ShadowTechniqueCap       int // Grave Stalker only
	ShadowTechniqueUsed      int
	KnownShadowTechnique     []knownHunterPick
	AvailableShadowTechnique []hunterPickOption

	ArsenalItemCap       int // Arsenalist only
	ArsenalItemUsed      int
	KnownArsenalItem     []knownHunterPick
	AvailableArsenalItem []hunterPickOption

	ToxicTechniqueCap       int // Undertaker only
	ToxicTechniqueUsed      int
	KnownToxicTechnique     []knownHunterPick
	AvailableToxicTechnique []hunterPickOption

	ViceTechniqueCap       int // Vice Agent only
	ViceTechniqueUsed      int
	KnownViceTechnique     []knownHunterPick
	AvailableViceTechnique []hunterPickOption

	VoidTechniqueCap       int // Void Walker only
	VoidTechniqueUsed      int
	KnownVoidTechnique     []knownHunterPick
	AvailableVoidTechnique []hunterPickOption

	// ProstheticAttachmentCap/Used/Known/Available: Wolves Legacy only.
	// The per-rest use pool behind these attachments is a separate
	// Special Resources tile (customResourceGrants, key
	// "prosthetic_attachments"), not shown here — same "pool here, known-
	// list there" split Hunters Exploits already uses.
	ProstheticAttachmentCap       int
	ProstheticAttachmentUsed      int
	KnownProstheticAttachment     []knownHunterPick
	AvailableProstheticAttachment []hunterPickOption

	// WolfTechniqueCap/Used/Known/Available: Wolves Legacy only, 10th level.
	// Picking one both records the pick and learns the matching jutsu (see
	// wolfTechniqueJutsuGrants). The alternate Prosthetic-Attachment-funded
	// casting cost this feature also grants is not modeled — see
	// wolfTechniqueOptions' own doc comment.
	WolfTechniqueCap       int
	WolfTechniqueUsed      int
	KnownWolfTechnique     []knownHunterPick
	AvailableWolfTechnique []hunterPickOption
}

// loadHunterTechniquesTabData returns nil for a character with no
// Hunter-Nin levels and none of the 4 archetype-training feats — the
// template gates the whole box's existence on this being non-nil, same
// treatment MartialTechniques gets. Each of the three sub-sections
// independently gates itself on its own Cap being > 0 (a level 1-2
// Hunter-Nin has Lethal Attack but neither Patterns nor Exploits yet; an
// archetype-feat-only character has Lethal Attack and Exploits but never
// Patterns or Defensive Tactics, which none of the 4 feats touch).
func (s *server) loadHunterTechniquesTabData(characterID int64, sheet *charsheet.Sheet) (*hunterTechniquesTabData, error) {
	hunterLevel, err := s.hunterNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	archetypeExploitSlots, archetypeLethalDice := huntersArchetypeGrants(grantedFeatures)
	if hunterLevel == 0 && archetypeExploitSlots == 0 && archetypeLethalDice == "" {
		return nil, nil
	}

	subclassSlug, _, err := s.hunterNinSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	lethalDice, err := s.lethalAttackDice(hunterLevel)
	if err != nil {
		return nil, err
	}
	if hunterLevel == 0 && archetypeLethalDice != "" {
		// Real levels and the archetype feats are mutually exclusive by
		// the feats' own prerequisites ("You cannot have class levels in
		// Hunter-Nin"), so this only ever overrides an empty real-level
		// readout, never a real chart value.
		lethalDice = archetypeLethalDice
	}

	data := &hunterTechniquesTabData{LethalAttackDice: lethalDice}

	data.LethalPrecisionCap = lethalPrecisionCap(hunterLevel)
	if data.LethalPrecisionCap > 0 {
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickLethalPrecision)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.LethalPrecisionUsed = len(picks)
		data.KnownLethalPrecision, data.AvailableLethalPrecision = splitHunterPicks(lethalPrecisionOptions, pickedSet)
	}

	data.PatternsCap = huntersPatternsCap(hunterLevel)
	if data.PatternsCap > 0 {
		catalog, err := s.loadHunterOptionCatalog("Hunters Patterns")
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickPattern)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.PatternsUsed = len(picks)
		data.KnownPatterns, data.AvailablePatterns = splitHunterPicks(catalog, pickedSet)
	}

	exploitingExploitsBonus := 0
	if hasGrantedFeature(grantedFeatures, huntersExploitingExploitsFeatSlug) {
		exploitingExploitsBonus = huntersExploitingExploitsBonus(hunterLevel)
	}
	data.ExploitsCap = huntersExploitsCap(hunterLevel) + archetypeExploitSlots + exploitingExploitsBonus
	if data.ExploitsCap > 0 {
		catalog, err := s.loadHunterOptionCatalog("Hunters Exploits")
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickExploit)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.ExploitsUsed = len(picks)

		grantedExploitSlugs := map[string]bool{}
		for _, f := range grantedFeatures {
			if slug, ok := huntersExploitAutoGrants[f.Slug]; ok {
				grantedExploitSlugs[slug] = true
			}
		}

		for _, o := range catalog {
			switch {
			case grantedExploitSlugs[o.Slug]:
				data.KnownExploits = append(data.KnownExploits, knownHunterPick{Slug: o.Slug, Name: o.Name, Granted: true})
			case pickedSet[o.Slug]:
				data.KnownExploits = append(data.KnownExploits, knownHunterPick{Slug: o.Slug, Name: o.Name})
			default:
				if lock, locked := huntersExploitSubclassLocks[o.Slug]; locked && lock != subclassSlug {
					continue // exclusive to a different Hunter's Creed
				}
				data.AvailableExploits = append(data.AvailableExploits, o)
			}
		}
	}

	data.DefensiveTacticsCap = defensiveTacticsCap(hunterLevel)
	if data.DefensiveTacticsCap > 0 {
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickDefensiveTactic)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.DefensiveTacticsUsed = len(picks)
		data.KnownDefensiveTactics, data.AvailableDefensiveTactics = splitHunterPicks(defensiveTacticsOptions, pickedSet)
	}

	// The 8 Hunter's Creeds' own subclass-exclusive picks — each block only
	// ever runs for the one matching subclass, same "gate the whole
	// sub-section on subclassSlug" shape Intelligence Operative's Tactical
	// Strategist-only Operative Traps already establishes.
	switch subclassSlug {
	case bladeWardenSubclassSlug:
		data.WardenWeaponCap = wardenWeaponCap(hunterLevel)
		if data.WardenWeaponCap > 0 {
			catalog, err := s.loadWardenWeaponCatalog()
			if err != nil {
				return nil, err
			}
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickWardenWeapon)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.WardenWeaponUsed = len(picks)
			data.KnownWardenWeapon, data.AvailableWardenWeapon = splitHunterPicks(catalog, pickedSet)
		}
		data.WardenWeaponPropertyCap = subclassTechniqueCap(hunterLevel)
		if data.WardenWeaponPropertyCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickWardenWeaponProperty)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.WardenWeaponPropertyUsed = len(picks)
			data.KnownWardenWeaponProperty, data.AvailableWardenWeaponProperty = splitHunterPicks(wardenWeaponPropertyOptions, pickedSet)
		}
	case necroticHandSubclassSlug:
		data.MedicalTechniqueCap = subclassTechniqueCap(hunterLevel)
		if data.MedicalTechniqueCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickMedicalTechnique)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.MedicalTechniqueUsed = len(picks)
			data.KnownMedicalTechnique, data.AvailableMedicalTechnique = splitHunterPicks(medicalAssassinationTechniques, pickedSet)
		}
	case graveStalkerSubclassSlug:
		data.ShadowTechniqueCap = subclassTechniqueCap(hunterLevel)
		if data.ShadowTechniqueCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickShadowTechnique)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.ShadowTechniqueUsed = len(picks)
			data.KnownShadowTechnique, data.AvailableShadowTechnique = splitHunterPicks(shadowAssassinationTechniques, pickedSet)
		}
	case arsenalistSubclassSlug:
		data.ArsenalItemCap = arsenalItemCap(hunterLevel)
		if data.ArsenalItemCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickArsenalItem)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.ArsenalItemUsed = len(picks)
			data.KnownArsenalItem, data.AvailableArsenalItem = splitHunterPicks(arsenalItems, pickedSet)
		}
	case undertakerSubclassSlug:
		data.ToxicTechniqueCap = subclassTechniqueCap(hunterLevel)
		if data.ToxicTechniqueCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickToxicTechnique)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.ToxicTechniqueUsed = len(picks)
			data.KnownToxicTechnique, data.AvailableToxicTechnique = splitHunterPicks(toxicAssassinationTechniques, pickedSet)
		}
	case viceAgentSubclassSlug:
		data.ViceTechniqueCap = subclassTechniqueCap(hunterLevel)
		if data.ViceTechniqueCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickViceTechnique)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.ViceTechniqueUsed = len(picks)
			data.KnownViceTechnique, data.AvailableViceTechnique = splitHunterPicks(viceAssassinationTechniques, pickedSet)
		}
	case voidWalkerSubclassSlug:
		data.VoidTechniqueCap = subclassTechniqueCap(hunterLevel)
		if data.VoidTechniqueCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickVoidTechnique)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.VoidTechniqueUsed = len(picks)
			data.KnownVoidTechnique, data.AvailableVoidTechnique = splitHunterPicks(voidAssassinationTechniques, pickedSet)
		}
	case wolvesLegacySubclassSlug:
		data.ProstheticAttachmentCap = prostheticAttachmentCap(hunterLevel)
		if data.ProstheticAttachmentCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickProstheticAttachment)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.ProstheticAttachmentUsed = len(picks)
			data.KnownProstheticAttachment, data.AvailableProstheticAttachment = splitHunterPicks(prostheticAttachmentOptions, pickedSet)
		}
		data.WolfTechniqueCap = wolfTechniqueCap(hunterLevel)
		if data.WolfTechniqueCap > 0 {
			picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickWolfTechnique)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.WolfTechniqueUsed = len(picks)
			data.KnownWolfTechnique, data.AvailableWolfTechnique = splitHunterPicks(wolfTechniqueOptions, pickedSet)
		}
	}

	return data, nil
}

// handleHunterPickAdd builds one category's "learn a pick" route — shared
// since all three catalogs validate/store identically, differing only in
// which of hunterTechniquesTabData's own fields govern the cap and the
// currently-pickable list.
func (s *server) handleHunterPickAdd(
	category charstore.HunterNinPickCategory,
	used func(*hunterTechniquesTabData) int,
	cap func(*hunterTechniquesTabData) int,
	available func(*hunterTechniquesTabData) []hunterPickOption,
) http.HandlerFunc {
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
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for hunter pick add:", err)
			return
		}
		data, err := s.loadHunterTechniquesTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter techniques for add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Hunter-Nin", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		valid := false
		for _, o := range available(data) {
			if o.Slug == slug {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if err := charstore.AddHunterNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add hunter pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_hunter_techniques")
	}
}

// handleHunterArsenalItemAdd is handleHunterPickAdd's own validation/cap
// logic for the one category (Arsenal Items) that needs a side effect on
// top of the plain pick: "If you select a jutsu, you learn it" (Arsenal's
// Proficiency's own text) — 3 of the 14 Arsenal items are real jutsu (see
// arsenalJutsuGrants), so a successful add also calls the same generic
// charstore.AddJutsu the Jutsu tab's own "Learn" button uses. A thin
// wrapper duplicating handleHunterPickAdd's body rather than threading an
// optional callback through that shared factory for one caller.
func (s *server) handleHunterArsenalItemAdd(w http.ResponseWriter, r *http.Request) {
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
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for arsenal item add:", err)
		return
	}
	data, err := s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load hunter techniques for arsenal item add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no levels in Hunter-Nin", http.StatusBadRequest)
		return
	}
	if data.ArsenalItemUsed >= data.ArsenalItemCap {
		http.Error(w, "no slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.AvailableArsenalItem {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickArsenalItem, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add arsenal item pick:", err)
		return
	}
	if jutsuSlug, ok := arsenalJutsuGrants[slug]; ok {
		if err := charstore.AddJutsu(s.charDB, id, jutsuSlug, sheet.Level); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("learn arsenal item jutsu:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_hunter_techniques")
}

// handleHunterWolfTechniqueAdd mirrors handleHunterArsenalItemAdd's own
// shape: "The jutsu selected are added to your Jutsu known" (Wolf
// Techniques' own text) means every successful pick also learns the
// matching jutsu via the same generic charstore.AddJutsu the Jutsu tab's
// own "Learn" button uses. Unlike Arsenal Items, every wolfTechniqueOptions
// entry grants a jutsu (see wolfTechniqueJutsuGrants), so this always fires
// on a valid pick, not conditionally.
func (s *server) handleHunterWolfTechniqueAdd(w http.ResponseWriter, r *http.Request) {
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
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for wolf technique add:", err)
		return
	}
	data, err := s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load hunter techniques for wolf technique add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no levels in Hunter-Nin", http.StatusBadRequest)
		return
	}
	if data.WolfTechniqueUsed >= data.WolfTechniqueCap {
		http.Error(w, "no slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.AvailableWolfTechnique {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	if err := charstore.AddHunterNinPick(s.charDB, id, charstore.HunterPickWolfTechnique, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add wolf technique pick:", err)
		return
	}
	if jutsuSlug, ok := wolfTechniqueJutsuGrants[slug]; ok {
		if err := charstore.AddJutsu(s.charDB, id, jutsuSlug, sheet.Level); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("learn wolf technique jutsu:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_hunter_techniques")
}

// handleHunterPickDelete builds one category's "forget a pick" route —
// freely, at any time, same "trust the player" boundary Martial Technique
// forgetting already draws.
func (s *server) handleHunterPickDelete(category charstore.HunterNinPickCategory) http.HandlerFunc {
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
		if err := charstore.RemoveHunterNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove hunter pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_hunter_techniques")
	}
}

// handleHunterPickDetail serves the click-to-open popup for a Known
// Hunters Pattern, Hunters Exploit, or Defensive Tactic (see
// sheet_hunter_techniques' KnownPatterns/KnownExploits/
// KnownDefensiveTactics — both player-picked and the 8 subclass-exclusive
// Granted exploits carry a real Slug, so all three link here) — same
// "sheet-popup-trigger fetches its own href + ?fragment=1 and drops the
// result into the shared dialog" mechanism sheet-popup.js already uses
// for feats/items/jutsu, just without those pages' full standalone-page
// half: nothing else on the site ever links to a single Hunters Pattern/
// Exploit/Defensive Tactic, so this only ever needs to answer the
// fragment request, not a real full-page view. Not character-scoped —
// all three catalogs are static rules content, same as /feats/{slug...}.
func (s *server) handleHunterPickDetail(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	slug := r.PathValue("slug")

	var name, description string
	switch category {
	case "pattern":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = 'Hunters Patterns'`,
			slug, hunterNinSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter pattern detail:", err)
			return
		}
	case "exploit":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = 'Hunters Exploits'`,
			slug, hunterNinSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter exploit detail:", err)
			return
		}
	case "defensive-tactic":
		var found bool
		name, description, found = lookupHunterPickOption(defensiveTacticsOptions, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "lethal-precision":
		var found bool
		name, description, found = lookupHunterPickOption(lethalPrecisionOptions, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "warden-weapon":
		// The one hand-curated-list case above doesn't cover — Warden
		// Weapon TYPE is sourced live from the equipment table, not a
		// fixed catalog (see loadWardenWeaponCatalog).
		var damageDice, damageType, properties string
		err := s.rulesDB.QueryRow(`
			SELECT name, COALESCE(damage_dice, ''), COALESCE(damage_type, ''), COALESCE(properties, '')
			FROM equipment WHERE slug = ? AND kind = 'weapon'`,
			slug,
		).Scan(&name, &damageDice, &damageType, &properties)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load warden weapon detail:", err)
			return
		}
		description = strings.TrimSpace(damageDice + " " + damageType + ". " + properties)
	case "warden-weapon-property":
		var found bool
		name, description, found = lookupHunterPickOption(wardenWeaponPropertyOptions, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "medical-technique":
		var found bool
		name, description, found = lookupHunterPickOption(medicalAssassinationTechniques, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "shadow-technique":
		var found bool
		name, description, found = lookupHunterPickOption(shadowAssassinationTechniques, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "arsenal-item":
		var found bool
		name, description, found = lookupHunterPickOption(arsenalItems, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "toxic-technique":
		var found bool
		name, description, found = lookupHunterPickOption(toxicAssassinationTechniques, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "vice-technique":
		var found bool
		name, description, found = lookupHunterPickOption(viceAssassinationTechniques, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "void-technique":
		var found bool
		name, description, found = lookupHunterPickOption(voidAssassinationTechniques, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "prosthetic-attachment":
		var found bool
		name, description, found = lookupHunterPickOption(prostheticAttachmentOptions, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	case "wolf-technique":
		var found bool
		name, description, found = lookupHunterPickOption(wolfTechniqueOptions, slug)
		if !found {
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}

	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render hunter pick detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render hunter pick detail:", err)
	}
}
