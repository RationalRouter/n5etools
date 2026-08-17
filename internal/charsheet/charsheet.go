// Package charsheet computes a character's derived stats — ability
// modifiers, proficiency bonus, skill/save bonuses, max HP/chakra, AC —
// from the two databases' stored INPUTS. It never writes anything;
// characters.db's own schema comment states the rule this package exists
// to implement: "Store INPUTS and mutable play state; never store derived
// math." Compute is meant to be called fresh on every sheet render (and
// reused directly by click-to-roll), so the displayed number and the
// rolled number can never drift apart.
//
// Every formula below was confirmed by reading the core book directly
// (Naruto 5e - Full Document.pdf, Chapter 6) rather than assumed from
// standard 5e — this ruleset has real, deliberate deviations, most
// notably that EVERY saving throw gets at least half proficiency bonus
// even without proficiency (5e gives zero). See SavingThrowModifier.
package charsheet

import (
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/features"
	"github.com/sergio/n5e/internal/puppetupgrades"
)

// Abilities lists the six abilities in the canonical order used
// throughout this app (matches the DB CHECK constraints and abilityLabels
// in cmd/n5e/classes.go).
var Abilities = []string{"str", "dex", "con", "int", "wis", "cha"}

func abilityIndex(ab string) int {
	for i, a := range Abilities {
		if a == ab {
			return i
		}
	}
	return len(Abilities)
}

// SkillAbility maps each of the game's 21 skills to its governing
// ability. Confirmed directly from the core book, page 51 ("Using Each
// Ability Score") — this mapping exists nowhere in either database
// (rules.db only ever stores skill names as opaque strings in
// class/background/clan proficiency rows), same precedent as the
// abilityLabels map in cmd/n5e/classes.go.
var SkillAbility = map[string]string{
	"Athletics":       "str",
	"Martial Arts":    "str",
	"Acrobatics":      "dex",
	"Sleight of Hand": "dex",
	"Stealth":         "dex",
	"Chakra Control":  "con",
	"Crafting":        "int",
	"History":         "int",
	"Investigation":   "int",
	"Nature":          "int",
	"Ninshou":         "int",
	"Animal Handling": "wis",
	"Illusions":       "wis",
	"Insight":         "wis",
	"Medicine":        "wis",
	"Perception":      "wis",
	"Survival":        "wis",
	"Deception":       "cha",
	"Intimidation":    "cha",
	"Performance":     "cha",
	"Persuasion":      "cha",
}

// abilityAbbrev normalizes a full ability name ("Charisma", as stored in
// class_proficiencies.value for kind='saving_throw') to the three-letter
// form used everywhere else. Case-insensitive; already-abbreviated input
// passes through unchanged.
func abilityAbbrev(name string) string {
	switch strings.ToLower(name) {
	case "strength", "str":
		return "str"
	case "dexterity", "dex":
		return "dex"
	case "constitution", "con":
		return "con"
	case "intelligence", "int":
		return "int"
	case "wisdom", "wis":
		return "wis"
	case "charisma", "cha":
		return "cha"
	default:
		return strings.ToLower(name)
	}
}

// AbilityModifier is the standard floor((score-10)/2). Go's native "/"
// truncates toward zero, not toward negative infinity, so scores below 10
// need an explicit correction on odd differences (e.g. score 7: diff -3,
// -3/2 truncates to -1 in Go, but floor(-3/2) is -2 — the correct 5e-style
// modifier).
func AbilityModifier(score int) int {
	diff := score - 10
	if diff >= 0 || diff%2 == 0 {
		return diff / 2
	}
	return diff/2 - 1
}

// CheckModifier is the shared formula for skill checks and plain ability
// checks: d20 + ability modifier + proficiency bonus, but only if
// proficient. This is standard 5e — SavingThrowModifier below is the one
// that isn't.
func CheckModifier(abilityMod, proficiencyBonus int, proficient bool) int {
	if proficient {
		return abilityMod + proficiencyBonus
	}
	return abilityMod
}

// AttackKinds lists the jutsu attack types that govern their own ability, in
// display order, each with the ability it uses by default: Ninjutsu off
// Intelligence, Genjutsu off Wisdom, Taijutsu off Strength, Bukijutsu off
// Strength (it started life as a copy of Taijutsu's modifier — see the part
// 10 note below — so it keeps that default). A character can override any of
// them (see AttackAbilityField) because class features and feats do change
// them — Genjutsu off Charisma and Taijutsu off Dexterity are two concrete
// examples that occur in play.
//
// Anything whitelisting override fields wants this list, anything displaying
// or rolling attacks wants Sheet.JutsuAttacks.
//
// Hijutsu, the remaining classification in the jutsu table, deliberately has
// no entry: no rule was given for it, and guessing one would put a wrong
// number on the sheet that looks authoritative.
//
// Bukijutsu used to be excluded from this list and instead mirror Taijutsu's
// modifier outright (via the now-removed DerivedAttackKinds) so it could
// never drift out of sync. That design was reversed so Bukijutsu gets a
// modifier dropdown the same as the other three: it now governs its own
// ability like everything else here, decoupled from Taijutsu.
var AttackKinds = []struct{ Kind, Ability string }{
	{"Ninjutsu", "int"},
	{"Genjutsu", "wis"},
	{"Taijutsu", "str"},
	{"Bukijutsu", "str"},
}

// AttackAbilityField is the character_overrides.field name holding a
// player's chosen ability for one attack kind ("Ninjutsu" →
// "ninjutsu_ability"). Shared by Compute and the handler that writes it so
// the two can't drift.
func AttackAbilityField(kind string) string {
	return strings.ToLower(kind) + "_ability"
}

// JutsuAttack is one attack kind's governing ability and to-hit modifier.
type JutsuAttack struct {
	Kind     string // "Ninjutsu", "Genjutsu", "Taijutsu", "Bukijutsu"
	Ability  string // three-letter code actually used, override included
	Modifier int    // ability modifier + full proficiency bonus
}

// ClashCheck is one discipline's Clash resolution — a d20 check made only
// by the character's own side, since this app has no opposing-side/NPC
// system to roll against (most clashes are against an NPC/creature the DM
// controls anyway, so there is nothing to automate on that side even if
// this app tracked NPCs). The governing ability is overridable per
// discipline (see ClashAbilityField), same shape as JutsuAttack's — the
// governing SKILL (and therefore its proficiency) is not: that stays fixed
// by discipline (Ninshou/Illusions/Martial Arts), only which ability feeds
// it can change, matching how a real 5e "use a different ability for this
// check" override normally works.
type ClashCheck struct {
	Discipline string // "Ninjutsu", "Genjutsu", "Taijutsu", "Bukijutsu" — Hijutsu has no stated Clash rule, same reasoning as its absence from AttackKinds
	Ability    string // three-letter code actually used, override included
	Modifier   int
}

// ClashAbilityField is the character_overrides.field name holding a
// player's chosen ability for one discipline's Clash check ("Ninjutsu" →
// "ninjutsu_clash_ability"). Deliberately a separate key from
// AttackAbilityField's — overriding a discipline's Attack ability and its
// Clash ability are independent choices, same as Bukijutsu's Attack
// ability already stands independent of Taijutsu's (AttackKinds' part-10
// note) even though Bukijutsu Clash's default still mirrors Taijutsu's.
func ClashAbilityField(discipline string) string {
	return strings.ToLower(discipline) + "_clash_ability"
}

// clashChecks computes every discipline's Clash modifier from the
// character's already-computed skills and abilities, honouring any
// per-discipline ability override in overrides (see ClashAbilityField).
//
// Default formulas, confirmed to varying degrees (see the implementation
// plan this shipped from for the full research trail):
//   - Ninjutsu: Intelligence (Ninshou) — confirmed directly, SkillDescriptions'
//     own book-transcribed text for Ninshou states it "is also used to
//     resolve a Ninjutsu Clash."
//   - Taijutsu: Strength or Dexterity, whichever's better (Martial Arts) —
//     confirmed by directly reading the core book's Clash rules, and
//     independently corroborated by jutsu/true-rakshasas-palm's own rider
//     text ("Strength (Martial arts) check for the clash"). Martial Arts is
//     normally Strength-only in SkillAbility; Clash's default is a
//     documented exception, hence picking the better of the two abilities
//     below rather than reusing an ordinary skill Modifier.
//   - Genjutsu: Wisdom (Illusions) — no book or ingested-data confirmation
//     exists for this; shipped anyway as the best available default.
//   - Bukijutsu: same default as Taijutsu (Martial Arts), on the reasoning
//     that Bukijutsu ("weapon technique") mirrors Taijutsu (hand-to-hand)
//     closely enough to inherit its default Clash rule. Independently
//     overridable from Taijutsu's, same as its Attack ability already is.
//   - Hijutsu has no entry: no rule is stated for it anywhere, same
//     precedent as its absence from AttackKinds above.
func clashChecks(sheet *Sheet, overrides map[string]string) []ClashCheck {
	_, ninshouProficient := SkillModifier(sheet, "Ninshou")
	_, illusionsProficient := SkillModifier(sheet, "Illusions")
	_, martialArtsProficient := SkillModifier(sheet, "Martial Arts")

	martialArtsDefault := "str"
	if sheet.Abilities["dex"].Modifier > sheet.Abilities["str"].Modifier {
		martialArtsDefault = "dex"
	}

	kinds := []struct {
		Discipline     string
		DefaultAbility string
		Proficient     bool
	}{
		{"Ninjutsu", "int", ninshouProficient},
		{"Genjutsu", "wis", illusionsProficient},
		{"Taijutsu", martialArtsDefault, martialArtsProficient},
		{"Bukijutsu", martialArtsDefault, martialArtsProficient},
	}

	checks := make([]ClashCheck, 0, len(kinds))
	for _, k := range kinds {
		ability := k.DefaultAbility
		if picked := abilityAbbrev(overrides[ClashAbilityField(k.Discipline)]); picked != "" && abilityIndex(picked) < len(Abilities) {
			ability = picked
		}
		checks = append(checks, ClashCheck{
			Discipline: k.Discipline,
			Ability:    ability,
			Modifier:   CheckModifier(sheet.Abilities[ability].Modifier, sheet.ProficiencyBonus, k.Proficient),
		})
	}
	return checks
}

// SkillModifier looks up one named skill's already-computed modifier and
// proficiency from sheet.Skills. Shared by clashChecks above and the sheet
// handler's concentration-check display (which surfaces Chakra Control's
// modifier the same way) — both need "find this one skill's numbers by
// name" rather than a full skill-check formula of their own.
func SkillModifier(sheet *Sheet, name string) (mod int, proficient bool) {
	for _, sk := range sheet.Skills {
		if sk.Name == name {
			return sk.Modifier, sk.Proficient
		}
	}
	return 0, false
}

// FixedGain is one class level's HP or chakra gain when the player doesn't
// roll for it: the die's maximum at the character's very first level, and
// the die's average rounded up (die/2 + 1) at every level after — the same
// "take the fixed value instead of rolling" convention 5e uses and the one
// this app applies automatically on level-up. A die of 0 (no class chosen
// yet) contributes nothing.
func FixedGain(die int, firstLevel bool) int {
	if die <= 0 {
		return 0
	}
	if firstLevel {
		return die
	}
	return die/2 + 1
}

// SavingThrowModifier implements N5E's confirmed deviation from standard
// 5e: every saving throw gets at least half proficiency bonus (rounded
// down), even without proficiency in that save; a proficient save gets
// the full bonus instead. Direct quote, core book Chapter 6, page 50:
// "You add half your proficiency bonus, rounded down, to all saving
// throws you do not have proficiency in. This does not count as having
// proficiency in such a save." Standard 5e gives zero bonus when not
// proficient — do not "simplify" this back to CheckModifier.
//
// proficiencyBonus is always non-negative in practice (it only ever
// starts at +3 and increases), so Go's truncating "/" already matches
// "rounded down" here without needing AbilityModifier's negative-number
// correction.
func SavingThrowModifier(abilityMod, proficiencyBonus int, proficient bool) int {
	if proficient {
		return abilityMod + proficiencyBonus
	}
	return abilityMod + proficiencyBonus/2
}

// Proficiency modes an initiative roll can use. Half is N5E's default (the
// same half-proficiency convention SavingThrowModifier documents); the other
// two exist because class features and feats move it, and a sheet that can
// only express the default forces the player to fudge the number elsewhere.
const (
	ProfNone = "none"
	ProfHalf = "half"
	ProfFull = "full"
)

// ProfModes is the accepted set, in the order the sheet offers them.
var ProfModes = []string{ProfHalf, ProfFull, ProfNone}

// ProficiencyShare is how much of the proficiency bonus a mode contributes.
// Rounding is down; proficiencyBonus is non-negative in practice, so
// truncating "/" already does that (see SavingThrowModifier's note).
// An unrecognised mode is treated as the N5E default rather than as zero, so
// a stale or hand-edited value cannot silently drop a bonus the player has.
func ProficiencyShare(mode string, proficiencyBonus int) int {
	switch mode {
	case ProfNone:
		return 0
	case ProfFull:
		return proficiencyBonus
	default:
		return proficiencyBonus / 2
	}
}

// ComposeModifier is the shape every hand-tunable modifier on this sheet
// takes: an ability's modifier, some share of the proficiency bonus, and a
// flat extra for whatever the app cannot derive. Initiative, weapon attack
// rolls, weapon damage and custom attacks all go through it, so they can never
// disagree about what "half proficiency" or "no ability" means.
//
// Storing these three parts instead of one typed-in total is the point: the
// total is recomputed on every render, so levelling up or raising a score
// moves it by itself, the way the rest of this package works.
func ComposeModifier(abilityMod, proficiencyBonus int, profMode string, flat int) int {
	return abilityMod + ProficiencyShare(profMode, proficiencyBonus) + flat
}

// InitiativeModifier composes an initiative roll: a governing ability
// (Dexterity by default), some share of the proficiency bonus (half by
// default) and a flat bonus for anything the app does not model.
//
// It is NOT a plain Dex check — this app rendered it as one until that was
// corrected — and it is not a proficiency-gated check either: there is no
// such thing as being proficient in initiative, hence a mode rather than a
// bool.
func InitiativeModifier(abilityMod, proficiencyBonus int, profMode string, flat int) int {
	return ComposeModifier(abilityMod, proficiencyBonus, profMode, flat)
}

// ArmorClass computes AC from one equipped armor item's stats:
//
//	AC = 10 + armor bonus + min(sum of the item's ability-derived terms, max mod)
//
// An armor_ability_1/2 value of the literal string "PROF" contributes
// HALF the proficiency bonus, rounded down — confirmed from
// internal/schema/migrations/rules/0006_equipment_sheet.sql's own header
// comment ("+ half prof, per proficiency rules"), the same half-
// proficiency pattern as SavingThrowModifier, not a one-off guess. A nil
// maxMod means uncapped. ability1/ability2 use either the empty string or
// the literal DB value "NONE" for "not present" (armor with only one
// ability slot, or none at all) — both must be treated as zero, not just
// "", since the equipment table actually stores "NONE" (confirmed via
// out/rules.db); falling through to the default branch would otherwise
// compute AbilityModifier(scores["none"]) = AbilityModifier(0) = -5.
func ArmorClass(armorBonus int, ability1, ability2 string, maxMod *float64, scores map[string]int, proficiencyBonus int) int {
	term := func(ab string) int {
		switch {
		case ab == "" || ab == "NONE":
			return 0
		case ab == "PROF":
			return proficiencyBonus / 2
		default:
			return AbilityModifier(scores[abilityAbbrev(ab)])
		}
	}
	sum := term(ability1) + term(ability2)
	if maxMod != nil && float64(sum) > *maxMod {
		sum = int(*maxMod)
	}
	return 10 + armorBonus + sum
}

// AbilityScore is one ability's assigned score and its derived modifier.
type AbilityScore struct {
	Score    int
	Modifier int
}

// SkillEntry is one skill's proficiency and computed check modifier.
type SkillEntry struct {
	Name        string
	Ability     string
	Proficient  bool
	Modifier    int
	MasteryRank int // 0 = no Mastery — see MasteryRankCap/MasteryBonus
}

// MasteryRankCap is the core book's own level-gated ceiling on a single
// skill or toolkit's own Mastery rank (Chapter 6, page 49, "Mastery"):
// "While gaining stacking Ranks of Mastery sounds simple, you are limited
// by your total level. Between levels 1-6, you can only gain 1 Rank of
// Mastery. Between levels 7-11, you can gain up to 2 Ranks of Mastery. When
// you reach levels 12+, you can have up to 3 Ranks of Mastery." This caps
// how HIGH a single skill's rank can go, not how many different skills can
// carry Mastery at all — the book gates that by how many real "sources"
// (specific class/clan features, feats, jutsu, "even environmental
// circumstances") a character has, an open-ended list this app has no way
// to exhaustively enumerate, so — same as Generalized Skill/Puppet
// Tactics — this is a direct player pick per skill, not an attempt to
// auto-derive a source count.
func MasteryRankCap(level int) int {
	switch {
	case level >= 12:
		return 3
	case level >= 7:
		return 2
	default:
		return 1
	}
}

// MaxMasteryRank is the book's own absolute ceiling, independent of level:
// "If you have more than three sources of mastery, the maximum possible
// bonus you can have is +6" — i.e. a fourth source adds nothing.
const MaxMasteryRank = 3

// EffectiveMasteryRank is the rank a stored Mastery entry actually applies
// at right now. A rank is stored once, at the level it was gained, but a
// character's level is an editable field on this sheet (a correction, a
// rebuild, a DM walking a campaign back), and the book's cap is a standing
// limit on what you "can have", not a one-time gate at purchase — so a
// Rank 3 entry on a character now sitting at level 5 applies as Rank 1
// rather than keeping a bonus their level no longer supports. Clamping at
// read time (rather than rewriting the row) keeps the player's own real
// pick intact for when their level comes back up.
func EffectiveMasteryRank(rank, level int) int {
	if rank < 0 {
		return 0
	}
	if cap := MasteryRankCap(level); rank > cap {
		return cap
	}
	if rank > MaxMasteryRank {
		return MaxMasteryRank
	}
	return rank
}

// MasteryBonus is the flat bonus a given Mastery rank adds to a d20 roll:
// "Sources of Mastery: One Source: Rank 1 Mastery (+2), Two Sources: Rank 2
// Mastery (+4), Three Sources: Rank 3 Mastery (+6)." rank 0 (no Mastery)
// correctly falls out of the same formula rather than needing its own case;
// anything above Rank 3 is clamped to +6 per the book's own "more than
// three sources" line, so no caller can produce a bonus the rules don't
// allow even if it hands over an out-of-range rank.
func MasteryBonus(rank int) int {
	if rank < 0 {
		return 0
	}
	if rank > MaxMasteryRank {
		rank = MaxMasteryRank
	}
	return rank * 2
}

// MasteryRankLabel is the Shinobi Rank a given Mastery rank is called by:
// "Ranks of Mastery is categorized utilizing Shinobi Ranks where
// applicable; No Proficiency: Not Proficient. Proficiency: Genin Mastery
// (or Genin Rank). Rank 1 Mastery: Chunin Mastery (or Chunin Rank). Rank 2
// Mastery: Jonin Mastery (or Jonin Rank). Rank 3 Mastery: Sanin or Kage
// Mastery." Rank 0 depends on whether the character is proficient at all,
// which is why proficiency is a parameter rather than derived from rank.
func MasteryRankLabel(rank int, proficient bool) string {
	switch {
	case rank >= MaxMasteryRank:
		return "Sanin or Kage"
	case rank == 2:
		return "Jonin"
	case rank == 1:
		return "Chunin"
	case proficient:
		return "Genin"
	default:
		return "Not Proficient"
	}
}

// SaveEntry is one ability's saving throw proficiency and modifier.
// MasteryRank is feature-granted saving-throw Mastery (features.
// ResolveSaveMasteryRanks), already clamped through EffectiveMasteryRank
// and already included in Modifier — same treatment SkillEntry gives
// stored skill Mastery.
type SaveEntry struct {
	Ability     string
	Proficient  bool
	MasteryRank int
	Modifier    int
}

// Sheet is a character's fully computed derived stats, assembled fresh by
// Compute on every call. Most fields are derived math (nothing here writes
// anything back); a handful (CurrentHP, TempHP, BaseTempHP, Inspiration,
// Ryo, Appearance, Backstory, AlliesOrganizations, AdditionalFeaturesText,
// Treasure, Notes) are stored mutable play state passed through unmodified,
// exactly like Name already was — charstore's sheet.go setters are what
// change those, never this package.
type Sheet struct {
	CharacterID      int64
	Name             string
	ClanSlug         string // "" if no clan chosen
	Level            int    // sum of character_classes.levels; 0 for a draft with no class yet
	ProficiencyBonus int
	Abilities        map[string]AbilityScore
	// AbilityMax is each ability's real maximum (base 20, plus anything
	// from a feature grant whose RaisesMax is true) — what the ASI
	// picker's own cap check compares against instead of an assumed flat
	// 20. Always populated with all 6 abilities, even when no grant raises
	// any of them (every value is then exactly 20).
	AbilityMax    map[string]int
	BaseAbilities map[string]int // the stored base_* scores before clan/background/ASI bonuses — what the sheet's ability editor writes back to
	Skills        []SkillEntry   // grouped by ability then alphabetical, matching the book's own layout (page 51)
	// MasteryRanks is every Mastery pick's EFFECTIVE rank (already clamped
	// to the level cap by EffectiveMasteryRank), keyed by skill or toolkit
	// name. Skills above carry their own copy folded into Modifier; this
	// map is what lets the parts of the sheet built outside this package —
	// the Tool Proficiencies & Custom Skills rows, whose modifier is
	// composed from character_proficiency_mods rather than here — apply the
	// same bonus to a toolkit's own d20 roll.
	MasteryRanks map[string]int
	// PendingFeatureChoices is every choice-gated class/subclass/clan
	// feature grant the character currently qualifies for (its own level
	// threshold reached) but hasn't resolved a pick for yet — the sheet's
	// own picker panel renders one row per entry. See internal/features'
	// own choiceSlotDefs doc comment for what's deliberately excluded from
	// this list entirely (Mastery-only grants, toolkit-only grants,
	// companion-targeted grants).
	PendingFeatureChoices []features.ChoiceSlot
	// PendingASISlots is every class's unresolved Ability Score
	// Improvement pick (levels 4/8/12/16/19) — a separate list from
	// PendingFeatureChoices because a resolved pick writes directly to
	// character_ability_bonuses (source_kind='asi'), not
	// character_feature_choices; sumAbilityBonuses already sums it the
	// moment it's written, no further Compute wiring needed for the bonus
	// itself. See internal/features' own asiFeatureSuffix doc comment for
	// what else the book's ASI text grants that needs no code at all.
	PendingASISlots []features.ASISlot
	Saves           []SaveEntry   // in Abilities order
	JutsuAttacks    []JutsuAttack // one per AttackKinds entry, in that order
	ClashChecks     []ClashCheck  // see clashChecks; Ninjutsu/Genjutsu/Taijutsu/Bukijutsu, no Hijutsu entry
	MaxHP           int
	MaxChakra       int
	MaxHPAuto       int  // the computed maximum before any manual pin, so the sheet can show what it would be
	MaxChakraAuto   int  //   "
	MaxHPPinned     bool // true when the player has pinned Max HP by hand and MaxHP is that number
	MaxChakraPinned bool //   "
	HitDie          int  // sides of the primary class's hit die (0 = no class chosen yet)
	ChakraDie       int  // sides of the primary class's chakra die
	AC              *int // nil when no equipped armor is found and no override is set
	Speed           int  // feet; 30 default, overridden by the character's clan (v_clans.speed_feet) once one is chosen
	// PuppetUpgradeSources names any Puppet Upgrade that raised this
	// character's own Speed or Max HP (see internal/puppetupgrades), so a
	// number that differs from the clan/class default can say why.
	PuppetUpgradeSources []string
	PassivePerception    int // 10 + Perception's Modifier
	Initiative           int // the composed total; see InitiativeModifier
	// The three parts behind Initiative, so the sheet's picker can show what is
	// currently selected rather than guessing from the total.
	InitiativeAbility string // three-letter code, "dex" unless overridden
	InitiativeProf    string // one of ProfModes, "half" unless overridden
	InitiativeBonus   int    // flat extra, 0 unless overridden
	// AC's own Initiative-style adjustment (part 9): stacked ON TOP of the
	// armor-derived (or manually pinned) AC above, not a replacement for it —
	// unlike Initiative, AC already has a real computed base, so these three
	// default to contributing nothing (no ability, no proficiency share, no
	// bonus) rather than to a governing ability the way Initiative's do.
	ACAbility string // "" unless overridden — no ability term by default
	ACProf    string // one of ProfModes, "none" unless overridden
	ACBonus   int    // flat extra, 0 unless overridden

	CurrentHP              int
	TempHP                 int
	BaseTempHP             int
	CurrentChakra          int
	TempChakra             int
	HitDiceSpent           int
	ChakraDiceSpent        int
	Inspiration            bool
	Ryo                    float64
	Appearance             string
	Backstory              string
	AlliesOrganizations    string
	AdditionalFeaturesText string
	Treasure               string
	Notes                  string // the Core tab's scratchpad (migration 0008)
	Portrait               string // data: URL, "" when the player hasn't uploaded one
}

// Compute assembles a character's full derived-stat sheet from rules.db
// (class/background/clan definitions, xp_levels, equipment) and
// characters.db (the character's actual choices). Pure read, no writes.
func Compute(rulesDB, charDB *sql.DB, characterID int64) (*Sheet, error) {
	sheet := &Sheet{CharacterID: characterID}

	var baseStr, baseDex, baseCon, baseInt, baseWis, baseCha int
	var inspiration int
	var clanSlug, appearance, backstory, alliesOrgs, additionalFeatures, treasure, notes, portrait sql.NullString
	err := charDB.QueryRow(`
		SELECT name, clan_slug, base_str, base_dex, base_con, base_int, base_wis, base_cha,
		       current_hp, temp_hp, base_temp_hp, current_chakra, temp_chakra,
		       hit_dice_spent, chakra_dice_spent, inspiration, ryo,
		       appearance, backstory, allies_organizations, additional_features_text, treasure,
		       notes, portrait
		FROM characters WHERE id = ?`, characterID,
	).Scan(&sheet.Name, &clanSlug, &baseStr, &baseDex, &baseCon, &baseInt, &baseWis, &baseCha,
		&sheet.CurrentHP, &sheet.TempHP, &sheet.BaseTempHP, &sheet.CurrentChakra, &sheet.TempChakra,
		&sheet.HitDiceSpent, &sheet.ChakraDiceSpent, &inspiration, &sheet.Ryo,
		&appearance, &backstory, &alliesOrgs, &additionalFeatures, &treasure,
		&notes, &portrait)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("character %d not found", characterID)
	}
	if err != nil {
		return nil, fmt.Errorf("load character: %w", err)
	}
	sheet.ClanSlug = clanSlug.String
	sheet.Inspiration = inspiration != 0
	sheet.Appearance = appearance.String
	sheet.Backstory = backstory.String
	sheet.AlliesOrganizations = alliesOrgs.String
	sheet.AdditionalFeaturesText = additionalFeatures.String
	sheet.Treasure = treasure.String
	sheet.Notes = notes.String
	sheet.Portrait = portrait.String
	base := map[string]int{
		"str": baseStr, "dex": baseDex, "con": baseCon,
		"int": baseInt, "wis": baseWis, "cha": baseCha,
	}

	sheet.BaseAbilities = base

	bonuses, err := sumAbilityBonuses(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load ability bonuses: %w", err)
	}

	scores := make(map[string]int, 6)
	sheet.Abilities = make(map[string]AbilityScore, 6)
	for _, ab := range Abilities {
		score := base[ab] + bonuses[ab]
		scores[ab] = score
		sheet.Abilities[ab] = AbilityScore{Score: score, Modifier: AbilityModifier(score)}
	}

	// The character's classes, in the order they were taken. The first is
	// the primary class: it supplies the hit/chakra die the short-rest
	// roller offers and the maximum-value gain at character level 1.
	classes, err := loadClasses(charDB, rulesDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load classes: %w", err)
	}
	for _, c := range classes {
		sheet.Level += c.Levels
	}
	if len(classes) > 0 {
		sheet.HitDie, sheet.ChakraDie = classes[0].HitDie, classes[0].ChakraDie
	}

	overrides, err := loadOverrides(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load overrides: %w", err)
	}

	sheet.ProficiencyBonus = 2 // a not-yet-leveled draft character has no xp_levels row to read
	if sheet.Level > 0 {
		if err := rulesDB.QueryRow(
			`SELECT proficiency_bonus FROM xp_levels WHERE level = ?`, sheet.Level,
		).Scan(&sheet.ProficiencyBonus); err != nil {
			return nil, fmt.Errorf("load proficiency bonus: %w", err)
		}
	}

	profSkills, profSaves, err := loadProficiencies(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load proficiencies: %w", err)
	}

	// Granted class/subclass/clan features can themselves confer a fixed
	// proficiency, AC-ability-swap, or Speed bonus — resolved here (not
	// just displayed in a side panel the way cmd/n5e's Features & Traits
	// box does) specifically so ClashChecks and PassivePerception below,
	// which both derive FROM Skills, see the boosted numbers rather than
	// stale pre-bonus ones. See internal/features' own doc comment for why
	// this logic lives in a package charsheet can import instead of
	// staying inside cmd/n5e where it used to.
	grantedFeatures, err := features.LoadGrantedFeatures(rulesDB, charDB, characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, fmt.Errorf("load granted features: %w", err)
	}
	armorCategory, err := equippedArmorCategory(rulesDB, charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load equipped armor category: %w", err)
	}
	wornChassisSlug, err := PuppetWornArmorChassisSlug(rulesDB, charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load worn puppet armor chassis: %w", err)
	}
	// Choice-gated grants (a feature that lets the player pick which skill
	// or saving throw it applies to) resolve the exact same way, just gated
	// by a stored pick instead of being unconditional — an unresolved slot
	// contributes nothing yet, same as an unpicked subclass today. Both
	// PendingFeatureChoices (for the sheet's picker panel) and the applied
	// grants themselves are derived from the same ResolveChoiceSlots call
	// so they can never disagree about which slots are currently unlocked.
	classLevels := make(map[string]int, len(classes))
	for _, c := range classes {
		classLevels[c.Slug] = c.Levels
	}
	resolvedChoices, err := features.LoadFeatureChoices(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load feature choices: %w", err)
	}
	choiceSlots := features.ResolveChoiceSlots(grantedFeatures, classLevels, sheet.Level)
	sheet.PendingFeatureChoices = features.PendingChoiceSlots(choiceSlots, resolvedChoices)

	// Ability Score Improvement resolves its ability-score half by writing
	// directly to character_ability_bonuses (source_kind='asi') — already
	// summed into sheet.Abilities above by sumAbilityBonuses, so only the
	// "what's still unresolved" list needs computing here.
	resolvedASIRefs, err := features.LoadResolvedASIRefs(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load asi choices: %w", err)
	}
	sheet.PendingASISlots = features.PendingASISlots(
		features.ResolveASISlots(grantedFeatures, classLevels), resolvedASIRefs)

	for _, g := range append(
		features.ResolveProficiencyGrants(grantedFeatures),
		features.ResolveChoiceGrants(choiceSlots, resolvedChoices)...,
	) {
		switch g.Kind {
		case features.GrantSkill:
			profSkills[g.Value] = true
		case features.GrantSavingThrow:
			profSaves[g.Value] = true
		}
	}

	// Feature-granted ability-score increases — fixed (Master of Focus's
	// +2 Str/+2 Con), choice-gated (Intelligent Design's pick-one +2), and
	// equipment-conditional (Master of the Purple Technique's Juggernaut-
	// Armor-only further +2) alike — land here, AFTER the granted-feature/
	// choice resolution above and BEFORE anything below derives from
	// sheet.Abilities (Skills, Saves, Initiative, AC, HP/Chakra, attack
	// modifiers all read the rebuilt scores). Derived fresh every Compute,
	// never stored: a level edit that drops the granting feature drops its
	// bonus on the next Compute with no cleanup pass, same as every other
	// grant.
	//
	// maxBonuses is accumulated alongside bonuses from every grant whose
	// RaisesMax is true (see AbilityScoreGrant's own doc comment) and
	// exposed as sheet.AbilityMax — the ASI picker's own cap check
	// (ValidASIPicks) reads this instead of an assumed flat 20, so a
	// raised-max score can legally take an ASI past 20.
	maxBonuses := map[string]int{}
	grants := append(
		features.ResolveAbilityScoreGrants(grantedFeatures),
		features.ResolveChoiceAbilityScoreGrants(choiceSlots, resolvedChoices)...,
	)
	grants = append(grants, features.ResolveConditionalAbilityScoreGrants(grantedFeatures, wornChassisSlug != "")...)
	// A Powerful Build chassis's own "+2 Strength, up to 22" is a separate,
	// equipment-tied bonus rather than an AbilityScoreGrant: it's capped at
	// a fixed 22 regardless of RaisesMax, and applied inline AFTER every
	// other grant above so the cap sees their combined total (see
	// puppetupgrades.PowerfulBuildStrengthCap's own doc comment for why this
	// isn't folded into Item 11's persistent max-tracking instead).
	hasPowerfulBuild := puppetupgrades.ChassisHasPowerfulBuild(wornChassisSlug)
	if len(grants) > 0 || hasPowerfulBuild {
		for _, g := range grants {
			bonuses[g.Ability] += g.Amount
			if g.RaisesMax {
				maxBonuses[g.Ability] += g.Amount
			}
		}
		if hasPowerfulBuild {
			if room := puppetupgrades.PowerfulBuildStrengthCap - (base["str"] + bonuses["str"]); room > 0 {
				bonuses["str"] += min(2, room)
			}
		}
		for _, ab := range Abilities {
			score := base[ab] + bonuses[ab]
			scores[ab] = score
			sheet.Abilities[ab] = AbilityScore{Score: score, Modifier: AbilityModifier(score)}
		}
	}
	sheet.AbilityMax = make(map[string]int, len(Abilities))
	for _, ab := range Abilities {
		sheet.AbilityMax[ab] = 20 + maxBonuses[ab]
	}

	storedMastery, err := loadMasteryRanks(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load mastery: %w", err)
	}
	// Clamp every stored rank to what this character's level actually
	// supports right now, once, so skills and toolkits alike read the same
	// already-capped number rather than each re-deriving it.
	masteryRanks := make(map[string]int, len(storedMastery))
	for name, rank := range storedMastery {
		masteryRanks[name] = EffectiveMasteryRank(rank, sheet.Level)
	}
	sheet.MasteryRanks = masteryRanks
	for name, ability := range SkillAbility {
		proficient := profSkills[name]
		rank := masteryRanks[name]
		sheet.Skills = append(sheet.Skills, SkillEntry{
			Name: name, Ability: ability, Proficient: proficient, MasteryRank: rank,
			Modifier: CheckModifier(sheet.Abilities[ability].Modifier, sheet.ProficiencyBonus, proficient) + MasteryBonus(rank),
		})
	}
	sort.Slice(sheet.Skills, func(i, j int) bool {
		if sheet.Skills[i].Ability != sheet.Skills[j].Ability {
			return abilityIndex(sheet.Skills[i].Ability) < abilityIndex(sheet.Skills[j].Ability)
		}
		return sheet.Skills[i].Name < sheet.Skills[j].Name
	})
	for _, sk := range sheet.Skills {
		if sk.Name == "Perception" {
			sheet.PassivePerception = 10 + sk.Modifier
			break
		}
	}
	sheet.ClashChecks = clashChecks(sheet, overrides)

	// Initiative's three parts, each falling back to the N5E default when the
	// override is absent or unrecognised — a stale value must not compute off
	// ability score 0 or quietly drop the proficiency share.
	sheet.InitiativeAbility = "dex"
	if picked := abilityAbbrev(overrides["initiative_ability"]); picked != "" && abilityIndex(picked) < len(Abilities) {
		sheet.InitiativeAbility = picked
	}
	sheet.InitiativeProf = ProfHalf
	if mode := strings.ToLower(strings.TrimSpace(overrides["initiative_prof"])); slices.Contains(ProfModes, mode) {
		sheet.InitiativeProf = mode
	}
	sheet.InitiativeBonus, _ = strconv.Atoi(strings.TrimSpace(overrides["initiative_bonus"]))
	sheet.Initiative = InitiativeModifier(
		sheet.Abilities[sheet.InitiativeAbility].Modifier,
		sheet.ProficiencyBonus, sheet.InitiativeProf, sheet.InitiativeBonus)

	// Feature-granted saving-throw Mastery (Intelligent Design's "Mastery
	// on Strength saves") — clamped through the same level gate stored
	// skill Mastery gets, and folded into the modifier the same way
	// SkillEntry already folds MasteryBonus in.
	saveMasteryRanks := features.ResolveSaveMasteryRanks(grantedFeatures)
	for _, ab := range Abilities {
		proficient := profSaves[ab]
		masteryRank := EffectiveMasteryRank(saveMasteryRanks[ab], sheet.Level)
		sheet.Saves = append(sheet.Saves, SaveEntry{
			Ability: ab, Proficient: proficient, MasteryRank: masteryRank,
			Modifier: SavingThrowModifier(sheet.Abilities[ab].Modifier, sheet.ProficiencyBonus, proficient) + MasteryBonus(masteryRank),
		})
	}

	// Ninjutsu/Genjutsu/Taijutsu attack modifiers: the governing ability's
	// modifier plus the FULL proficiency bonus, always — casting your own
	// jutsu is never a non-proficient roll, so there is no half-proficiency
	// branch here the way SavingThrowModifier has one. An override is only
	// honoured if it names a real ability; a stale or hand-edited value
	// falls back to the default rather than silently computing off score 0.
	for _, k := range AttackKinds {
		ability := k.Ability
		if picked := abilityAbbrev(overrides[AttackAbilityField(k.Kind)]); picked != "" && abilityIndex(picked) < len(Abilities) {
			ability = picked
		}
		sheet.JutsuAttacks = append(sheet.JutsuAttacks, JutsuAttack{
			Kind: k.Kind, Ability: ability,
			Modifier: sheet.Abilities[ability].Modifier + sheet.ProficiencyBonus,
		})
	}

	sheet.Speed = 30 // book default (page 51) when no clan is chosen yet or the clan has no recorded speed
	if sheet.ClanSlug != "" {
		var speed sql.NullInt64
		if err := rulesDB.QueryRow(`SELECT speed_feet FROM v_clans WHERE slug = ?`, sheet.ClanSlug).Scan(&speed); err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("load clan speed: %w", err)
		}
		if speed.Valid {
			sheet.Speed = int(speed.Int64)
		}
	}
	sheet.Speed += features.ResolveSpeedBonus(grantedFeatures, sheet.Level, armorCategory)
	// A handful of Puppet Upgrades modify the Puppet Master's OWN numbers
	// rather than a companion's — Accelerated Movement's "increase all
	// movement speeds you possess by +10 feet", and the White Technique
	// reading of Enhanced Durability. They are resolved from the character's
	// companion upgrade picks (internal/puppetupgrades), which is why they
	// can't come through grantedFeatures like every other bonus here.
	puppetMasterLevel := 0
	for _, c := range classes {
		if c.Slug == puppetupgrades.ClassSlug {
			puppetMasterLevel = c.Levels
		}
	}
	puppetTotals, err := puppetupgrades.LoadCharacterTotals(charDB, characterID, puppetMasterLevel)
	if err != nil {
		return nil, fmt.Errorf("load puppet upgrade bonuses: %w", err)
	}
	sheet.Speed += puppetTotals.Speed
	sheet.PuppetUpgradeSources = puppetTotals.Sources
	if puppetupgrades.ChassisHasMobile(wornChassisSlug) {
		sheet.Speed += puppetupgrades.MobileSpeedBonus
	}
	// A player can pin their own Speed (part 10's "make Speed editable like
	// Ryo" ask) the same way Max HP/Max Chakra can be pinned — an explicit
	// override always wins over the clan default AND any feature bonus
	// above, same as MaxHP/MaxChakra's own auto-then-pin shape.
	if v, ok := overrideInt(overrides, "speed"); ok {
		sheet.Speed = v
	}

	// character_level_gains.hp_gain/chakra_gain store the RAW hit-die/
	// chakra-die roll-or-max value only, with no ability modifier baked
	// in — Max HP/Chakra adds level*currentConModifier separately here,
	// at read time. This is deliberate, not a simplification: the core
	// book (page 53) describes retroactively re-deriving past levels'
	// totals whenever Constitution changes ("as though you had the new
	// modifier from first level"); computing level*currentMod fresh on
	// every read implements that correctly for free, whereas baking the
	// modifier into each stored row would require rewriting every past
	// row on every ability-score change instead.
	//
	// A level with NO stored row takes the fixed (unrolled) gain instead
	// of contributing nothing. That fallback is what makes levelling up
	// actually raise Max HP and Max Chakra: charstore.SetLevel only writes
	// the new class level, and every level it just granted lands here with
	// no row of its own. It also self-heals a character whose level was
	// raised by some other path (an older build's level override, a
	// hand-edited database) rather than leaving them at level 7 with a
	// level-1 hit point total.
	stored, err := loadLevelGains(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("load level gains: %w", err)
	}
	var hpSum, chakraSum int
	for i, c := range classes {
		for lvl := 1; lvl <= c.Levels; lvl++ {
			if g, ok := stored[levelGainKey{c.Slug, lvl}]; ok {
				hpSum += g.hp
				chakraSum += g.chakra
				continue
			}
			first := i == 0 && lvl == 1
			hpSum += FixedGain(c.HitDie, first)
			chakraSum += FixedGain(c.ChakraDie, first)
		}
	}
	sheet.MaxHPAuto = hpSum + sheet.Level*sheet.Abilities["con"].Modifier + puppetTotals.MaxHP
	sheet.MaxChakraAuto = chakraSum + sheet.Level*sheet.Abilities["con"].Modifier
	sheet.MaxHP, sheet.MaxHPPinned = sheet.MaxHPAuto, false
	sheet.MaxChakra, sheet.MaxChakraPinned = sheet.MaxChakraAuto, false
	if v, ok := overrideInt(overrides, "maxhp"); ok {
		sheet.MaxHP, sheet.MaxHPPinned = v, true
	}
	if v, ok := overrideInt(overrides, "maxchakra"); ok {
		sheet.MaxChakra, sheet.MaxChakraPinned = v, true
	}

	// Purple Technique's "worn as armor" mechanic: a Puppet Tool
	// transformed into Juggernaut Armor around the character replaces
	// their equipped gear's AC outright, not stacks with it — while worn,
	// the character isn't using their own armor's formula at all, they're
	// using the puppet's. Checked first so the equipped-gear pipeline (and
	// its own ability-swap check, which is specifically about armor
	// category rules that don't apply here) is skipped entirely when a
	// puppet is actively worn. Reads the companion's own stored AC field —
	// the player-entered value the Puppets tab's ExpectedAC hint helps
	// fill in, not a formula recomputed here — same "the stored field is
	// the real one" boundary Companion.AC already draws.
	puppetArmorAC, err := puppetWornAsArmorAC(charDB, characterID)
	if err != nil {
		return nil, fmt.Errorf("compute puppet armor-form AC: %w", err)
	}
	if puppetArmorAC != nil {
		sheet.AC = puppetArmorAC
	} else {
		ac, err := computeEquippedAC(rulesDB, charDB, characterID, scores, sheet.ProficiencyBonus)
		if err != nil {
			return nil, fmt.Errorf("compute AC: %w", err)
		}
		sheet.AC = ac
		// A granted feature that lets another ability substitute for
		// Dexterity (Hoshigaki's Constitution, Konjiki's Intelligence
		// while in light/medium armor, Medical-Nin's Wisdom) never makes
		// AC worse — recompute it with Dexterity's slot replaced by the
		// granted ability and keep whichever is higher, rather than
		// assuming Dexterity is definitely part of the formula being
		// replaced (unarmored always uses it; some armor's own printed
		// formula might not).
		if swapAbility := features.ResolveACSwapAbility(grantedFeatures, armorCategory); swapAbility != "" && sheet.AC != nil {
			altScores := make(map[string]int, len(scores))
			for k, v := range scores {
				altScores[k] = v
			}
			altScores["dex"] = scores[swapAbility]
			altAC, err := computeEquippedAC(rulesDB, charDB, characterID, altScores, sheet.ProficiencyBonus)
			if err != nil {
				return nil, fmt.Errorf("compute swapped AC: %w", err)
			}
			if altAC != nil && *altAC > *sheet.AC {
				sheet.AC = altAC
			}
		}
	}
	if v, ok := overrideInt(overrides, "ac"); ok {
		sheet.AC = &v
	}

	// AC's Adjust menu (part 9's "same as Initiative" ask): an extra
	// ability + proficiency-share + flat-bonus term stacked on top of
	// whichever AC was just computed above, honoured the same defensive way
	// Initiative's own override reading is — a stale or unrecognised
	// ability/prof mode falls back to "no term" rather than computing off
	// ability score 0 or silently applying a share that isn't there.
	sheet.ACProf = ProfNone
	if mode := strings.ToLower(strings.TrimSpace(overrides["ac_prof"])); slices.Contains(ProfModes, mode) {
		sheet.ACProf = mode
	}
	sheet.ACBonus, _ = strconv.Atoi(strings.TrimSpace(overrides["ac_bonus"]))
	if picked := abilityAbbrev(overrides["ac_ability"]); picked != "" && abilityIndex(picked) < len(Abilities) {
		sheet.ACAbility = picked
	}
	if sheet.AC != nil {
		abilityMod := 0
		if sheet.ACAbility != "" {
			abilityMod = sheet.Abilities[sheet.ACAbility].Modifier
		}
		total := *sheet.AC + ComposeModifier(abilityMod, sheet.ProficiencyBonus, sheet.ACProf, sheet.ACBonus)
		sheet.AC = &total
	}

	return sheet, nil
}

// classRow is one character_classes row joined to its class's dice.
type classRow struct {
	Slug              string
	Levels            int
	HitDie, ChakraDie int
}

// loadClasses reads the character's classes in the order they were taken
// (order_index), each with the hit/chakra die its class rolls. A class slug
// missing from rules.db (a rules update dropped it) keeps zero dice rather
// than failing the whole sheet, same tolerance loadCharacterInventory
// applies to stale item slugs.
func loadClasses(charDB, rulesDB *sql.DB, characterID int64) ([]classRow, error) {
	rows, err := charDB.Query(`
		SELECT class_slug, levels FROM character_classes
		WHERE character_id = ? ORDER BY order_index`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []classRow
	for rows.Next() {
		var c classRow
		if err := rows.Scan(&c.Slug, &c.Levels); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		err := rulesDB.QueryRow(
			`SELECT hit_die, chakra_die FROM classes WHERE slug = ?`, out[i].Slug,
		).Scan(&out[i].HitDie, &out[i].ChakraDie)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
	}
	return out, nil
}

type levelGainKey struct {
	slug  string
	level int
}

// loadLevelGains reads the character's recorded per-level HP/chakra gains,
// keyed by (class slug, level in that class) so Compute can tell an
// explicitly recorded roll apart from a level that has no row at all.
func loadLevelGains(charDB *sql.DB, characterID int64) (map[levelGainKey]struct{ hp, chakra int }, error) {
	rows, err := charDB.Query(`
		SELECT class_slug, class_level, hp_gain, chakra_gain
		FROM character_level_gains WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[levelGainKey]struct{ hp, chakra int }{}
	for rows.Next() {
		var k levelGainKey
		var g struct{ hp, chakra int }
		if err := rows.Scan(&k.slug, &k.level, &g.hp, &g.chakra); err != nil {
			return nil, err
		}
		out[k] = g
	}
	return out, rows.Err()
}

func sumAbilityBonuses(charDB *sql.DB, characterID int64) (map[string]int, error) {
	bonuses := make(map[string]int, 6)
	rows, err := charDB.Query(
		`SELECT ability, amount FROM character_ability_bonuses WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ability string
		var amount int
		if err := rows.Scan(&ability, &amount); err != nil {
			return nil, err
		}
		bonuses[ability] += amount
	}
	return bonuses, rows.Err()
}

func loadProficiencies(charDB *sql.DB, characterID int64) (skills, saves map[string]bool, err error) {
	skills = map[string]bool{}
	saves = map[string]bool{}
	rows, err := charDB.Query(`
		SELECT DISTINCT kind, value FROM character_proficiencies
		WHERE character_id = ? AND kind IN ('skill', 'saving_throw')`, characterID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, nil, err
		}
		if kind == "skill" {
			skills[value] = true
		} else {
			saves[abilityAbbrev(value)] = true
		}
	}
	return skills, saves, rows.Err()
}

// loadMasteryRanks returns every skill/toolkit name -> Mastery rank a
// character has recorded (character_mastery, migration 0026_mastery.sql) —
// a plain map lookup by name, matching loadProficiencies' own shape,
// queried directly here rather than through internal/charstore to avoid
// this package importing that one just for a single SELECT (charsheet
// already reads every other characters.db input this same direct way).
func loadMasteryRanks(charDB *sql.DB, characterID int64) (map[string]int, error) {
	out := map[string]int{}
	rows, err := charDB.Query(
		`SELECT skill_name, rank FROM character_mastery WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var rank int
		if err := rows.Scan(&name, &rank); err != nil {
			return nil, err
		}
		out[name] = rank
	}
	return out, rows.Err()
}

// equippedArmorCategory reports the character's currently equipped armor's
// armor_category ("light", "medium", "heavy"), or "" for no armor equipped
// (or armor with no recorded category) — the same "" the book's own "not
// wearing Heavy Armor" phrasing already covers, since not wearing ANY armor
// is not wearing heavy armor either. Same equipped-item scan
// computeEquippedAC does, kept separate rather than folded into it: this is
// consulted earlier (before Speed) and again for the AC-swap check, and
// computeEquippedAC's own signature (scores in, AC out) has no room for a
// second return value without every existing caller changing.
func equippedArmorCategory(rulesDB, charDB *sql.DB, characterID int64) (string, error) {
	rows, err := charDB.Query(`
		SELECT item_slug FROM character_inventory
		WHERE character_id = ? AND equipped = 1 AND item_slug IS NOT NULL`, characterID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return "", err
		}
		slugs = append(slugs, slug)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	for _, slug := range slugs {
		var category sql.NullString
		err := rulesDB.QueryRow(`
			SELECT armor_category FROM equipment WHERE slug = ? AND kind = 'armor'`, slug,
		).Scan(&category)
		if err == sql.ErrNoRows {
			continue // not armor — keep looking
		}
		if err != nil {
			return "", err
		}
		return category.String, nil
	}
	return "", nil
}

// puppetWornAsArmorAC returns the AC of the character's currently-worn-as-
// armor puppet (character_companions.is_armor_form), or nil if none is set
// or the one that is has no AC entered yet. A character could in theory
// mark more than one puppet this way (the checkbox has no server-side
// exclusivity check of its own) — sort_order picks one deterministically
// rather than erroring, same "don't fail the whole sheet over a player
// bookkeeping inconsistency" leniency the rest of this package uses.
//
// Raw SQL against charDB rather than internal/charstore's Companion type:
// that package already imports this one (for Sheet), so importing it back
// here would be a cycle.
func puppetWornAsArmorAC(charDB *sql.DB, characterID int64) (*int, error) {
	var ac sql.NullInt64
	err := charDB.QueryRow(`
		SELECT ac FROM character_companions
		WHERE character_id = ? AND kind = 'puppet' AND is_armor_form = 1
		ORDER BY sort_order LIMIT 1`, characterID,
	).Scan(&ac)
	if err == sql.ErrNoRows || !ac.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := int(ac.Int64)
	return &v, nil
}

// PuppetWornArmorChassisSlug returns the rules.db slug of the Armor Chassis
// the character's currently-worn-as-armor puppet (character_companions.
// is_armor_form) was built with, or "" if no puppet is marked worn, or the
// worn one hasn't had a chassis picked yet. Unlike puppetWornAsArmorAC, this
// is NOT lenient about a blank AC — "wearing your Juggernaut Armor" (Master
// of the Purple Technique's conditional +2 Int, Item 4's chassis property
// bonuses) is about the transform/is_armor_form state, not whether the
// player has since filled in a derived stat. Exported so cmd/n5e can resolve
// the same worn chassis for its own bulk-bonus computation
// (puppetChassisPropertyBulkBonus) without a second, drifting copy of this
// query.
func PuppetWornArmorChassisSlug(rulesDB, charDB *sql.DB, characterID int64) (string, error) {
	var chassisName string
	err := charDB.QueryRow(`
		SELECT armor_chassis FROM character_companions
		WHERE character_id = ? AND kind = 'puppet' AND is_armor_form = 1
		ORDER BY sort_order LIMIT 1`, characterID,
	).Scan(&chassisName)
	if err == sql.ErrNoRows || chassisName == "" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var slug string
	err = rulesDB.QueryRow(`SELECT slug FROM puppet_armor_chassis WHERE name = ?`, chassisName).Scan(&slug)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return slug, nil
}

func computeEquippedAC(rulesDB, charDB *sql.DB, characterID int64, scores map[string]int, proficiencyBonus int) (*int, error) {
	// Unarmored is a real state with a real AC, not a missing value. This
	// used to return nil whenever no armor was found, and the sheet showed
	// a bare "—" — which is what every character saw, because nothing was
	// ever marked equipped in the first place.
	unarmored := 10 + AbilityModifier(scores["dex"])

	// Every equipped item is checked against the armor table rather than
	// just the first one the database happens to return: an earlier version
	// took a single row with LIMIT 1 and gave up if it wasn't armor, so a
	// character wearing a flak jacket and holding a kunai got an AC that
	// depended on insertion order.
	rows, err := charDB.Query(`
		SELECT item_slug FROM character_inventory
		WHERE character_id = ? AND equipped = 1 AND item_slug IS NOT NULL`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var acBonus sql.NullInt64
	var ability1, ability2 sql.NullString
	var maxMod sql.NullFloat64
	found := false
	for _, slug := range slugs {
		err := rulesDB.QueryRow(`
			SELECT ac_bonus, armor_ability_1, armor_ability_2, armor_max_mod
			FROM equipment WHERE slug = ? AND kind = 'armor'`, slug,
		).Scan(&acBonus, &ability1, &ability2, &maxMod)
		if err == sql.ErrNoRows {
			continue // not armor (a weapon, a tool) — keep looking
		}
		if err != nil {
			return nil, err
		}
		found = true
		break
	}
	if !found {
		return &unarmored, nil
	}

	var maxModPtr *float64
	if maxMod.Valid {
		maxModPtr = &maxMod.Float64
	}
	ac := ArmorClass(int(acBonus.Int64), ability1.String, ability2.String, maxModPtr, scores, proficiencyBonus)
	return &ac, nil
}

// loadOverrides reads the character's whole character_overrides row set in
// one query. The table mirrors the rules-DB override philosophy: it lets a
// player manually pin any computed field the engine gets wrong for their
// specific situation (a magic item's AC bonus this package doesn't model,
// a Max HP total they rolled by hand, a class feature that moves Genjutsu
// onto Charisma).
//
// One map, read once and consulted where each field is actually computed,
// replaces the earlier split between a separate loadLevelOverride called
// early and an applyOverrides called last — an ordering the fields
// themselves kept outgrowing. 'level' is no longer among them: level is a
// real stored class level now (see charstore.SetLevel), and migration
// 0006 folds any leftover row into it.
func loadOverrides(charDB *sql.DB, characterID int64) (map[string]string, error) {
	rows, err := charDB.Query(
		`SELECT field, value FROM character_overrides WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var field, value string
		if err := rows.Scan(&field, &value); err != nil {
			return nil, err
		}
		out[field] = value
	}
	return out, rows.Err()
}

// overrideInt reads one override as a whole number, reporting false when
// there is no such row or its value isn't numeric — a malformed override
// is ignored rather than allowed to zero out the field it pins.
func overrideInt(overrides map[string]string, field string) (int, bool) {
	value, ok := overrides[field]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return n, true
}
