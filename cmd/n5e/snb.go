package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file is the S.N.B (Scientific Ninja Beast) stat block — Science-Nin's
// S.N.B Specialist subclass's own companion (class/science-nin/group/
// scientific-inquiry/s-n-b-specialist, 3rd level). An S.N.B is stored as an
// ordinary kind="snb" charstore.Companion (companions.go), the same generic
// row every other companion kind uses; this file supplies the computed
// baseline stats, the base traits, the level-gated subclass feature list,
// and Combat Programming's own one-time Striker/Caster/Defensive/Lurker
// pick — mirroring how nindog.go turns a kind="nin-dog" row into a full
// stat block and titan.go turns a kind="titan" row into one. Secondary
// C.C.D's own custom-resource pool (custom_resources.go) is NOT touched
// here.
//
// S.N.B Upgrades' own cap+catalog picker (science_nin_subclasses.go's
// scienceNinSNBSpecialistData) IS read here — loadSNBReference folds every
// known upgrade's own flat, always-on numeric effect (Armored Exterior's AC
// floor, Improved Accuracy's attack bonus, Sturdy's HP bonus, Piston Legs'
// speed override, Enhanced Size's size override) into the same Expected*
// fields ninDogReference/titanReference already fold their own bonuses
// into — the identical "known upgrade, matched by name, folded into a
// computed baseline" treatment titan.go gives Bijuu Slayer's own size
// override (loadTitanReference). Every OTHER S.N.B Upgrade is a
// resource-gated ability (spend a CCD Drain to activate, or a reactive
// death-trigger) rather than a flat stat modifier — those stay
// informational-only, shown via KnownUpgrades' own full description text,
// the same restraint titanWeaponUpgradeAttacks documents for every Titan
// Upgrade that isn't a standing at-will weapon.
//
// Source: subclass_features (rules.db), subclass_slug LIKE
// '%s-n-b-specialist%'. The base stat block (Medium Construct, AC/HP/Speed/
// ability scores/Immunities/Resistances/Senses/Bite) is NOT where a stat
// block would normally live (the granting "Scientific Ninja Beast" feature
// at 3rd level just says "This companion has the following statistics:"
// and then jumps straight into unrelated command-economy text) — it
// actually landed bundled onto the END of "Combat Programming"'s own
// description instead, immediately after that feature's own four-option
// bullet list and before its trailing Artificial Intelligence/Bio-Organic
// Identifier/Internal Radio Link trait text. This reads as the same class
// of PDF-table-to-text extraction failure titan.go's own header doc
// documents for titan_unit_card (a table's row/column structure collapsing
// into one run-on string, landing wherever the extractor's cursor happened
// to be) — confirmed by the fact that a "Medium Construct / Armor Class ...
// / Hit Points ... / Speed ... / STR DEX CON ..." block has no business
// appearing mid-sentence inside a feature about choosing a combat role.
// snbBaseTraits/snbCombatProgrammingOptions/the Immunities/Resistances/
// Senses constants below are hand-transcribed from that same run-on
// description text rather than read live, the same "hand-curated, not
// live-queried" treatment titanBaseTraits/ninDogBreeds already draw for
// source text that isn't stored as individually addressable rows.
//
// Two genuine transcription judgment calls, both called out where they
// matter below rather than silently resolved:
//   - snbMaxHP's own formula reading (no parentheses survive the
//     extraction, so operator precedence has to be assumed).
//   - the Immunities/Resistances/Senses line's own doubled "Damage
//     Resistances" header, read as a duplicate-header artifact and
//     corrected to "Condition Immunities" in front of the four conditions
//     it actually precedes (see the const block's own doc).
//
// A third real fact, NOT a conflict: the granting "Scientific Ninja Beast"
// feature also says "Your S.N.B has a number of Hit Die (d8) equal to your
// Science-Nin level" — a SEPARATE spendable Hit Dice pool, not an
// alternate Max HP formula. Regenerative Armor's own healing text confirms
// this ("your S.N.B can spend any number of Hit Die to heal itself, for
// each Hit Die spent this way roll the die and add your S.N.B's
// Constitution modifier") — the same "Hit Dice as a spendable healing
// resource, separate from a flat Max HP formula" shape a player
// character's own Hit Dice already have on this sheet. Regenerative
// Armor's own healing math stays manual/narrated (informational Feature
// text only, per this file's scope), so nothing here tracks that pool as a
// number — HitDiceMax on snbReference is shown for reference only.
const (
	snbComboAttackFeatureSlug         = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/combo-attack"
	snbCombatProgrammingFeatureSlug   = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/combat-programming"
	snbImprovedServosFeatureSlug      = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/improved-servos"
	snbArtificialSentienceFeatureSlug = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/artificial-sentience"
	snbRegenerativeArmorFeatureSlug   = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/regenerative-armor"
)

// snbBaseAbilityScores: the base stat block's own six numbers, "12 (+1) 14
// (+2) 12 (+1) 8 (-1) 5 (-3) 5 (-3)" in standard STR/DEX/CON/INT/WIS/CHA
// column order, immediately following "Speed 30 ft." in the same run-on
// extraction this file's header doc describes. Before the granting
// feature's own free ASI allocation ("you increase one of its ability
// scores by 2, and two other scores by 1") — a free player choice this app
// never allocates automatically, the same "computed hint, player edits
// from there" boundary ninDogBaseAbilityScores/titanBaseAbilityScores
// already draw.
var snbBaseAbilityScores = map[string]int{
	"str": 12, "dex": 14, "con": 12, "int": 8, "wis": 5, "cha": 5,
}

const snbBaseSpeed = 30 // ft. — flat, no stated progression (unlike Nin-Dog's own rank-gated speed)

// snbDamageImmunities/snbDamageResistances/snbConditionImmunities/
// snbSenses: the base stat block's own defenses line, hand-transcribed
// with one correction. The raw extracted text reads "Damage Immunity
// Psychic, Poison Damage Resistances Acid Damage Resistances Charmed,
// Exhaustion, Frightened, Poisoned Senses Dark Vision...", repeating
// "Damage Resistances" twice in a row. Charmed/Exhaustion/Frightened/
// Poisoned are CONDITIONS, never damage types, anywhere else in this
// system — the second "Damage Resistances" is read as a duplicated
// table-header artifact from the same extraction that dropped this whole
// stat block into Combat Programming's own description in the first
// place, corrected here to "Condition Immunities", the header that
// actually belongs in front of a condition list in every other creature
// stat block in rules.db.
const (
	snbDamageImmunities    = "Psychic, Poison"
	snbDamageResistances   = "Acid"
	snbConditionImmunities = "Charmed, Exhaustion, Frightened, Poisoned"
	snbSenses              = "Darkvision (30 feet)"
)

// snbBiteDescription: "Melee Weapon Attack: Your Intelligence modifier +
// Your Prof Bonus to hit, reach 5 ft., one target. Hit: 1d6 + Your
// Intelligence modifier piercing damage. The damage die for this attack
// increases to a d8 at Level 9 and then a d10 at Level 14." — shown
// verbatim as reference text; the rollable Attacks row itself (
// snbBiteAttack) computes the CURRENT die size fresh every render instead
// of re-parsing this string.
const snbBiteDescription = "Melee Weapon Attack: Your Intelligence modifier + Your Prof Bonus to hit, reach 5 ft., one target. Hit: 1d6 + Your Intelligence modifier piercing damage. The damage die for this attack increases to a d8 at Level 9 and then a d10 at Level 14."

// snbBaseTraits: the base stat block's own named traits, hand-transcribed
// verbatim from the same run-on description text this file's header doc
// describes (they trail immediately after the stat line, the same
// position a normal 5e stat block's own named-traits section would sit
// in). Always shown, ungated beyond the whole panel requiring a kind="snb"
// companion — these describe every S.N.B, not something leveled into
// (Bio-Organic Identifier's own 15th-level DNA-database upgrade is kept
// inline as prose within its own entry instead of split into a separate
// Feature, the same "embedded level check inside the trait text itself"
// treatment titanBaseTraits gives Steady Improvement's own bonus-ASI math).
var snbBaseTraits = []companionFeatureRef{
	{Name: "Artificial Intelligence", Description: "The Scientific Ninja Beast is programmed to respond only to its creator; however, it can still perform basic functions when not commanded. If the Scientific Ninja Beast has not received any commands on a turn, it will take the Dodge action. If its master is incapacitated in any way, it will choose an ally of theirs who can now use their bonus action to direct the Scientific Ninja Beast."},
	{Name: "Bio-Organic Identifier", Description: "Your S.N.B can spend 1 minute analyzing a sample of a substance that is organic in nature; it can identify what the substance is, its regional origin, and how old it is. Beginning at 15th level, you have connected your S.N.B to a DNA database. At DM discretion, when your S.N.B successfully identifies a substance as blood, it may be able to determine who the blood belongs to, if your S.N.B has successfully hit the creature with a melee Bite attack and has dealt damage to the target's Hit Points."},
	{Name: "Internal Radio Link", Description: "You can issue orders to your S.N.B as long as you are within 1 mile of it."},
}

// snbFeatureDefs: S.N.B Specialist's own subclass features that are plain
// passive reference text once granted (as opposed to S.N.B Upgrades, its
// own cap+catalog picker already built elsewhere, and Secondary C.C.D, its
// own tracked resource pool already built elsewhere — neither duplicated
// here). Combo Attack is included even though it grants no new picker:
// same "show the granted rule text, gated by level" treatment
// titanFeatureDefs already gives Titan's own equivalent passive features.
var snbFeatureDefs = []struct {
	Slug  string
	Name  string
	Level int
}{
	{snbComboAttackFeatureSlug, "Combo Attack", 6},
	{snbImprovedServosFeatureSlug, "Improved Servos", 9},
	{snbArtificialSentienceFeatureSlug, "Artificial Sentience", 14},
	{snbRegenerativeArmorFeatureSlug, "Regenerative Armor", 17},
	{scienceNinFutureOfShinobiSNBFeatureSlug, "The Future of Shinobi: S.N.B", 20},
}

// snbCombatProgrammingOptions: Combat Programming's own 4-way choice,
// hand-transcribed from that feature's own bundled bullet list (see this
// file's header doc) — there is no class_options catalog row for these,
// unlike every other Science-Nin subclass's own cap+catalog picks
// (science_nin_subclasses.go), since the whole feature is one bundled
// prose block rather than a real ingested list_name.
var snbCombatProgrammingOptions = []companionFeatureRef{
	{Name: "Striker", Description: "Your S.N.B gains the Multiattack trait, allowing it to make two attacks using its natural weapons."},
	{Name: "Caster", Description: "Your S.N.B gains the ability to learn jutsu and a number of Jutsu Slots equal to 1/4th your Science-Nin level (rounded down). You can spend 3 Weeks of downtime programming a jutsu into your S.N.B, and it can learn a number of jutsu equal to half your proficiency bonus. The highest ranked jutsu your S.N.B can learn is always one rank lower than your current highest rank jutsu known, as indicated by your class table."},
	{Name: "Defensive", Description: "Your S.N.B gains a bonus to its AC equal to 1/3rd your Proficiency Bonus (rounded down)."},
	{Name: "Lurker", Description: "Your S.N.B gains the Lethal Attack trait. Once per turn, your S.N.B can deal extra damage to one creature it hits with an attack if another enemy of the target is within 5 feet of it. This extra damage is Xd8 (X = half your Science-Nin level)."},
}

// snbEffectiveAbilityModifier resolves one ability's modifier for a fixed
// baseline stat (Passive Perception's own Wisdom term) — the companion's
// own stored score if the player has already set one, else the base stat
// block's own baseline (snbBaseAbilityScores), the identical fallback
// shape ninDogEffectiveAbilityModifier already gives a brand-new Nin-Dog
// whose ability score fields are still blank.
func snbEffectiveAbilityModifier(companion charstore.Companion, key string) int {
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
	return charsheet.AbilityModifier(snbBaseAbilityScores[key])
}

// snbAC: "Armor Class 10 + Your Proficiency Bonus (Natural Armor)", plus
// Combat Programming's own Defensive option ("a bonus to its AC equal to
// 1/3rd your Proficiency bonus, rounded down") layered on top once chosen
// — the same "breed/specialization bonus folded straight into the
// computed baseline" treatment ninDogBaseSpeedForRank's own Young Inuit
// +10ft already gets, rather than a separate annotation-only note.
//
// Armored Exterior ("Your S.N.B's AC calculation now begins at 12, this
// increases to 13 at 9th level and 14 at 17th level") replaces the base
// "10" term the same formula still adds Proficiency Bonus on top of — read
// as a base-value increase, not a full formula replacement, since every
// other tier of this same upgrade line states its own progression purely as
// "the AC calculation begins at N" against the identical "+ Proficiency
// Bonus" frame the base stat block already establishes.
func snbAC(proficiencyBonus, level int, combatProgrammingPick string, hasArmoredExterior bool) int {
	base := 10
	if hasArmoredExterior {
		switch {
		case level >= 17:
			base = 14
		case level >= 9:
			base = 13
		default:
			base = 12
		}
	}
	ac := base + proficiencyBonus
	if combatProgrammingPick == "Defensive" {
		ac += proficiencyBonus / 3
	}
	return ac
}

// snbImprovedAccuracyBonus: "Your S.N.B gains a +1 to attack rolls. This
// bonus increases to a +2 at 9th level and a +3 at 17th level." Zero when
// Improved Accuracy isn't known — callers gate this themselves.
func snbImprovedAccuracyBonus(level int) int {
	switch {
	case level >= 17:
		return 3
	case level >= 9:
		return 2
	default:
		return 1
	}
}

// snbMaxHP: Combat Programming's own bundled stat block states "Hit Points
// Your Intelligence modifier + your Science-Nin level times 5" — no
// parentheses survive the source PDF's table-to-text extraction (see this
// file's header doc), so this reads the formula by plain left-to-right
// operator precedence (IntMod + (Level x 5)) rather than assuming an
// unstated "(IntMod + Level) x 5" grouping.
//
// "Your Intelligence modifier" is read as the PLAYER's own Intelligence
// modifier, not the S.N.B's own fixed Int score (8, modifier -1) shown in
// the same stat block. This sourcebook consistently names the CREATURE's
// own stat explicitly whenever a formula means the creature's own score —
// ninDogAC's own "Nin-Dogs Defensive Ability score", titanMaxHP's "Titan's
// Constitution Modifer", and this very subclass's own Regenerative Armor
// text a few sentences later ("add your S.N.B's Constitution modifier") —
// so a bare, unqualified "Your Intelligence modifier" reads as the
// player's own, the same distinction titanAC's own doc confirms (against
// the source PDF page image directly) for an identically bare
// "Intelligence Modifier" phrase in the same sourcebook. A flat,
// level-invariant -1 the companion's own fixed Int score would otherwise
// contribute also reads as pointless to name as a formula term at all —
// the same "the modifier should scale with the crafter's own progression"
// reasoning titanAC's doc gives for reaching the identical conclusion.
// Sturdy adds a second, identical "5 times your Science-Nin level" term on
// top of the base formula's own "level times 5" ("hit points increase by an
// amount equal to 5 times your Science-Nin level, each Science-Nin level
// thereafter your S.N.B gains an additional 5 hit points" — the same linear
// "5 x level" shape as the base formula's own level term, added rather than
// replacing it).
func snbMaxHP(level, playerIntMod int, hasSturdy bool) int {
	hp := playerIntMod + level*5
	if hasSturdy {
		hp += level * 5
	}
	if hp < 1 {
		hp = 1
	}
	return hp
}

// snbBiteDieForLevel: "Hit: 1d6 + Your Intelligence modifier piercing
// damage. The damage die for this attack increases to a d8 at Level 9 and
// then a d10 at Level 14."
func snbBiteDieForLevel(level int) int {
	switch {
	case level >= 14:
		return 10
	case level >= 9:
		return 8
	default:
		return 6
	}
}

// snbBiteAttack computes Bite's own rollable attack row fresh on every
// render — never stored as a charstore.CompanionAttack row, the same
// "computed, not stored" treatment ninDogBiteAttack/titanBashAttack give
// their own companion's built-in attack. "Your Intelligence modifier" is
// read as the PLAYER's own, same reasoning as snbMaxHP's own doc.
// accuracyBonus is Improved Accuracy's own to-hit bonus (0 when not known;
// see snbImprovedAccuracyBonus).
func snbBiteAttack(level, playerIntMod, ownerProfBonus, accuracyBonus int) companionAttackRow {
	return companionAttackRow{
		CompanionAttack: charstore.CompanionAttack{
			Name:        "Bite",
			Description: snbBiteDescription,
			DamageCount: 1,
			DamageSides: snbBiteDieForLevel(level),
			DamageType:  "piercing",
		},
		AttackTotal: playerIntMod + ownerProfBonus + accuracyBonus,
		DamageTotal: playerIntMod,
	}
}

// snbReference is the read-only/interactive panel shown on a kind="snb"
// companion's own popup and its card on the Companions tab — the S.N.B
// equivalent of ninDogReference/titanReference, built from S.N.B
// Specialist's own subclass features rather than a generic tribe or
// unit-card table, since neither exists for this companion.
type snbReference struct {
	Level               int
	ProficiencyBonus    int
	Size                string
	Senses              string
	Immunities          string
	Resistances         string
	ConditionImmunities string
	PassivePerception   int
	// HitDiceMax: "a number of Hit Die (d8) equal to your Science-Nin
	// level" — Regenerative Armor's own spend pool, shown for reference
	// only (see this file's header doc for why it isn't tracked as a
	// number here).
	HitDiceMax      int
	BiteDescription string

	Traits   []companionFeatureRef // Artificial Intelligence, Bio-Organic Identifier, Internal Radio Link — always shown, ungated
	Features []companionFeatureRef // Combo Attack/Improved Servos/Artificial Sentience/Regenerative Armor/Future of Shinobi: S.N.B, Locked by level

	CombatProgrammingUnlocked bool
	CombatProgrammingOptions  []companionFeatureRef // Striker/Caster/Defensive/Lurker
	CombatProgrammingPick     string

	// KnownUpgrades: every known S.N.B Upgrade with its own full
	// description text, shown on the companion sheet itself — most of these
	// are resource-gated abilities (spend a CCD Drain to activate) rather
	// than a flat stat modifier this app tracks as a number, so this is the
	// only place on the sheet a player can see what they actually do (the
	// same "always-visible full text" treatment titan.go's own Known Titan
	// Upgrades block gives its own non-hardcoded upgrades). The handful
	// folded into Expected* below (Armored Exterior/Improved Accuracy/
	// Sturdy/Piston Legs/Enhanced Size) are still listed here too.
	KnownUpgrades []scienceNinSubclassOption

	// AccuracyBonus is Improved Accuracy's own to-hit bonus (0 when not
	// known) — companions.go's own snbBiteAttack call sites read this
	// directly, so the bonus can never drift out of sync with whichever
	// upgrades this struct itself resolved as known.
	AccuracyBonus int

	// Expected*: the same "computed hint, never silently overwritten, Sync
	// button available" treatment ninDogReference's own Expected* fields
	// already give a Nin-Dog. ExpectedBiteDie is informational only (no
	// stored field to sync it into — the rollable Bite row above already
	// computes the current die size live every render). AC/MaxHP/Speed/Size
	// already fold in whichever of Armored Exterior/Sturdy/Piston Legs/
	// Enhanced Size are known — see loadSNBReference.
	ExpectedAC      int
	ExpectedMaxHP   int
	ExpectedSpeed   int
	ExpectedSize    string
	ExpectedBiteDie int
	ExpectedStr     int
	ExpectedDex     int
	ExpectedCon     int
	ExpectedInt     int
	ExpectedWis     int
	ExpectedCha     int
}

// snbReferenceOrZero returns *ref, or a zero-value snbReference if ref is
// nil — same reason ninDogReferenceOrZero/titanReferenceOrZero exist:
// companion_stat_fields' own dict call needs to field-access
// .SNBReference's Expected* values regardless of the companion's actual
// kind, and html/template rejects an {{if}} inside a pipeline argument.
func snbReferenceOrZero(ref *snbReference) snbReference {
	if ref == nil {
		return snbReference{}
	}
	return *ref
}

// loadSNBReference builds the panel for one kind="snb" companion — always
// non-nil (unlike loadTitanReference, which returns nil pending Ordnance
// Training): like a Nin-Dog, an S.N.B companion is a free pick from the
// Add Companion dropdown, not gated behind actually having levels in the
// granting class, so this computes purely from the character's own total
// level/ability scores/proficiency bonus, the same "any companion of this
// kind gets its reference panel" shape loadNinDogReference already uses.
func (s *server) loadSNBReference(characterID int64, companion charstore.Companion, sheet *charsheet.Sheet) (*snbReference, error) {
	level := sheet.Level
	profBonus := sheet.ProficiencyBonus
	playerIntMod := sheet.Abilities["int"].Modifier

	pick, err := charstore.GetFeatureCompanionChoice(s.charDB, characterID, snbCombatProgrammingFeatureSlug, companion.ID, 0)
	if err != nil {
		return nil, err
	}

	var features []companionFeatureRef
	for _, fd := range snbFeatureDefs {
		var desc string
		if err := s.rulesDB.QueryRow(`SELECT description FROM v_subclass_features WHERE slug = ?`, fd.Slug).Scan(&desc); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		features = append(features, companionFeatureRef{Name: fd.Name, Level: fd.Level, Description: desc, Locked: level < fd.Level})
	}

	// Known S.N.B Upgrades — see this file's own header doc and
	// snbReference.KnownUpgrades' own doc for why every known upgrade is
	// listed here (full text) while only a few are also folded into the
	// Expected* numeric fields below.
	upgradeCatalog, err := s.loadScienceNinEntryCatalog("S.N.B Upgrades", false)
	if err != nil {
		return nil, err
	}
	upgradeDescBySlug := scienceNinDescBySlug(upgradeCatalog)
	upgradePicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickSNBUpgrade)
	if err != nil {
		return nil, err
	}
	knownUpgradeNames := make(map[string]bool, len(upgradePicks))
	var knownUpgrades []scienceNinSubclassOption
	for _, p := range upgradePicks {
		for _, o := range upgradeCatalog {
			if o.Slug != p.OptionSlug {
				continue
			}
			knownUpgradeNames[o.Name] = true
			knownUpgrades = append(knownUpgrades, scienceNinSubclassOption{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Description: upgradeDescBySlug[o.Slug]})
			break
		}
	}

	hasArmoredExterior := knownUpgradeNames["Armored Exterior"]
	hasSturdy := knownUpgradeNames["Sturdy"]
	accuracyBonus := 0
	if knownUpgradeNames["Improved Accuracy"] {
		accuracyBonus = snbImprovedAccuracyBonus(level)
	}
	speed := snbBaseSpeed
	if knownUpgradeNames["Piston Legs"] {
		// "Your S.N.B's movement speed becomes 60 feet" — a flat override,
		// not additive, and always higher than the base 30 ft. regardless.
		speed = 60
	}
	size := "Medium"
	if knownUpgradeNames["Enhanced Size"] {
		size = "Large"
	}

	// overrides: this S.N.B's own manual pins (companionOverrideFields),
	// consulted below ahead of SetSNBStatDefaultsLive writing whichever
	// value (pinned or freshly computed) actually lands in the row — see
	// migration 0079_companion_overrides.sql's own doc.
	overrides, err := charstore.GetCompanionOverrides(s.charDB, companion.ID)
	if err != nil {
		return nil, err
	}
	finalAbilityScore := func(key string) int {
		if v, ok := companionOverrideInt(overrides, key+"_score"); ok {
			return int(v)
		}
		return snbBaseAbilityScores[key]
	}
	if v, ok := overrides["size"]; ok {
		size = v
	}
	ac := snbAC(profBonus, level, pick, hasArmoredExterior)
	if v, ok := companionOverrideInt(overrides, "ac"); ok {
		ac = int(v)
	}
	hpMax := snbMaxHP(level, playerIntMod, hasSturdy)
	if v, ok := companionOverrideInt(overrides, "hp_max"); ok {
		hpMax = int(v)
	}
	if v, ok := companionOverrideInt(overrides, "speed"); ok {
		speed = int(v)
	}

	if err := charstore.SetSNBStatDefaultsLive(s.charDB, characterID, companion.ID,
		int64(ac), int64(hpMax), int64(speed),
		int64(finalAbilityScore("str")), int64(finalAbilityScore("dex")), int64(finalAbilityScore("con")),
		int64(finalAbilityScore("int")), int64(finalAbilityScore("wis")), int64(finalAbilityScore("cha")), size,
	); err != nil {
		return nil, err
	}

	return &snbReference{
		Level:               level,
		ProficiencyBonus:    profBonus,
		Size:                size,
		Senses:              snbSenses,
		Immunities:          snbDamageImmunities,
		Resistances:         snbDamageResistances,
		ConditionImmunities: snbConditionImmunities,
		PassivePerception:   10 + snbEffectiveAbilityModifier(companion, "wis"),
		HitDiceMax:          level,
		BiteDescription:     snbBiteDescription,

		Traits:   snbBaseTraits,
		Features: features,

		CombatProgrammingUnlocked: level >= 6,
		CombatProgrammingOptions:  snbCombatProgrammingOptions,
		CombatProgrammingPick:     pick,

		KnownUpgrades: knownUpgrades,
		AccuracyBonus: accuracyBonus,

		ExpectedAC:      ac,
		ExpectedMaxHP:   hpMax,
		ExpectedSpeed:   speed,
		ExpectedSize:    size,
		ExpectedBiteDie: snbBiteDieForLevel(level),
		ExpectedStr:     finalAbilityScore("str"),
		ExpectedDex:     finalAbilityScore("dex"),
		ExpectedCon:     finalAbilityScore("con"),
		ExpectedInt:     finalAbilityScore("int"),
		ExpectedWis:     finalAbilityScore("wis"),
		ExpectedCha:     finalAbilityScore("cha"),
	}, nil
}

// prefillSNBStatDefaults populates a freshly-created S.N.B's AC/HP-max/
// Speed/six ability scores from the base stat block's own computed
// baseline — called exactly once, right after creation, mirroring
// prefillNinDogStatDefaults/prefillTitanStatDefaults so a brand-new S.N.B
// reaches its first render already showing correct starting numbers
// instead of a blank card the player has to click every Sync button on
// just to see them. Reuses loadSNBReference for the actual computation, so
// this can never drift from what a later manual Sync click would set. Uses
// charstore.SetSNBStatDefaults (COALESCE-based), never SetCompanionFields,
// so it can't clobber a field the player somehow already touched between
// creation and this call.
func (s *server) prefillSNBStatDefaults(characterID, companionID int64) error {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return err
	}
	companion, err := charstore.GetCompanion(s.charDB, characterID, companionID)
	if err != nil {
		return err
	}
	ref, err := s.loadSNBReference(characterID, companion, sheet)
	if err != nil || ref == nil {
		return err
	}
	return charstore.SetSNBStatDefaults(s.charDB, characterID, companionID,
		int64(ref.ExpectedAC), int64(ref.ExpectedMaxHP), int64(ref.ExpectedMaxHP), int64(ref.ExpectedSpeed),
		int64(ref.ExpectedStr), int64(ref.ExpectedDex), int64(ref.ExpectedCon),
		int64(ref.ExpectedInt), int64(ref.ExpectedWis), int64(ref.ExpectedCha),
	)
}

// handleSNBCombatProgrammingPick records Combat Programming's own one-time
// Striker/Caster/Defensive/Lurker pick — a permanent choice once made (the
// feature's own text offers no re-selection mechanism, and the template
// stops showing the picker form once a pick exists), the same one-shot-pick
// shape handleNinDogHijutsuTraitPick already uses for Beast Master's own
// bonus Hijutsu pick, just validated against a hand-transcribed 4-option
// catalog (snbCombatProgrammingOptions) instead of a rules.db jutsu query.
func (s *server) handleSNBCombatProgrammingPick(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for snb combat programming pick:", err)
		return
	}
	if companion.Kind != "snb" {
		http.Error(w, "not an s.n.b", http.StatusBadRequest)
		return
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for snb combat programming pick:", err)
		return
	}
	if sheet.Level < 6 {
		http.Error(w, "that pick is not open", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	optionName := strings.TrimSpace(r.FormValue("option_name"))
	valid := false
	for _, o := range snbCombatProgrammingOptions {
		if o.Name == optionName {
			valid = true
		}
	}
	if !valid {
		http.Error(w, "not a valid Combat Programming option", http.StatusBadRequest)
		return
	}

	if err := charstore.SetFeatureCompanionChoice(s.charDB, id, snbCombatProgrammingFeatureSlug, cid, 0, optionName); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set snb combat programming pick:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}
