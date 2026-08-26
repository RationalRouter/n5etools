package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file is the Nin-Dog stat block — the Inuzuka clan's Beast Master
// clan feature (clan/inuzuka/feature/beast-master, book/clan-compendium
// p.86-87). A Nin-Dog is stored as an ordinary kind="nin-dog"
// charstore.Companion (companions.go), the same generic row every other
// companion kind uses; this file supplies the computed baseline stats, the
// breed pick, the rank-gated special-feature picks, and the read-only
// reference panel that turn that generic row into a real stat block —
// mirroring how puppets.go turns a kind="puppet" row into one, but with
// Nin-Dog-specific rules throughout rather than reusing any Puppet-only
// mechanic.
//
// One real numeric conflict in the source text is called out where it
// matters below (ninDogToughness's own doc) rather than silently
// resolved — see that comment for the actual verbatim text and which
// reading this file uses. A second one — Beast Master's own text says a
// Nin-Dog gains "+8 ability score increases" at levels 4/8/12/16/20, while
// the Dog/Wolf progression table it cites as its source prints "+6" at
// every rank-up instead — is NOT resolved by auto-computing anything: like
// a player character's own ASI picks, which ability(ies) absorb an
// increase is a free player choice this app never allocates automatically,
// so ExpectedStr..ExpectedCha below are always just the flat D-Rank
// baseline (ninDogBaseAbilityScores), and the level-4/8/12/16/20 increases
// (6 or 8 points, whichever the table is played with) are applied by the
// player directly editing the companion's own ability-score fields, the
// same as every other companion's ability scores already work.

const ninDogBeastMasterFeatureSlug = "clan/inuzuka/feature/beast-master"
const ninDogHijutsuTraitFeatureSlug = "clan/inuzuka/trait/inuzuka-hijutsu"
const ninDogDogWolfTribeSlug = "summon/dog-wolf"

// ninDogBaseAbilityScores: the Dog/Wolf tribe's own D-Rank base array,
// "14 14 14 10 12 10" (summon_tribe_progression, rank D, stats_text) — six
// numbers in a fixed order that is not itself labeled anywhere in the
// database. Read as STR/DEX/CON/INT/WIS/CHA: Perception is a listed skill
// (favors WIS > INT/CHA), the tribe's own defensive_ability is Dexterity,
// and its one Attack (Bite) is Strength-based, all consistent with three
// tied 14s up front (Str/Dex/Con) and Wis outranking Int/Cha (12 vs 10/10).
var ninDogBaseAbilityScores = map[string]int{
	"str": 14, "dex": 14, "con": 14, "int": 10, "wis": 12, "cha": 10,
}

// ninDogEffectiveAbilityModifier resolves one ability's modifier for
// ExpectedAC/ExpectedMaxHP's own baseline suggestion: the companion's own
// stored score if the player has already set one, else the D-Rank baseline
// (ninDogBaseAbilityScores) — never companionAbilityModifier's plain
// fallback of a bare, unset SQL NULL reading as score 0 (modifier -5).
// A brand-new Nin-Dog (unlike a brand-new puppet, which prefillPuppetStat
// Defaults populates immediately on creation) starts with every ability
// score field blank, so without this fallback the very first "Sync AC to
// N"/"Sync Max HP to N" hint offered to a fresh companion computes N from
// as if the Nin-Dog had all-0 ability scores instead of its real 14/14/14/
// 10/12/10 baseline — confirmed live: a level-8 Nin-Dog with no ability
// scores entered yet showed "Sync AC to 11" (12 + 4 + a -5 Dex modifier)
// instead of the correct 18 (12 + 4 + a +2 Dex modifier from the 14
// baseline).
func ninDogEffectiveAbilityModifier(companion charstore.Companion, key string) int {
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
	return charsheet.AbilityModifier(ninDogBaseAbilityScores[key])
}

// ninDogToughness: Beast Master's own explicit override ("Your Nin-Dogs
// Toughness is 10") of the generic Dog/Wolf tribe's printed Toughness of 8
// (summon_tribes.toughness) — used here, not the tribe's row, since Beast
// Master is the Inuzuka-specific text describing a Nin-Dog specifically,
// the same "the more specific override wins" reading this file applies
// throughout wherever the two disagree.
const ninDogToughness = 10

// ninDogRankForLevel: Beast Master's own explicit level breakpoints —
// "When the Nin-Dog would reach levels 8, 12, 16 and 20 it would become
// the associated rank" — distinct from the generic band formula
// (highestRankForLevel in companions.go, 5/9/13/17) an ordinary
// kind="summon" companion uses. A Nin-Dog "Shares your level" outright, so
// its rank is gated directly off the character's own total level with
// these four specific breakpoints, not the summon-cast-rank formula.
func ninDogRankForLevel(level int) string {
	switch {
	case level >= 20:
		return "S"
	case level >= 16:
		return "A"
	case level >= 12:
		return "B"
	case level >= 8:
		return "C"
	default:
		return "D"
	}
}

// ninDogSizeForLevel: Beast Master's own explicit size progression —
// "Your Nin-Dogs size begins as small, grows to medium when it reaches
// 8th level and grows to Large at 12th" — with no stated growth past 12th
// level, so it stays Large through 16th/20th by omission. This is more
// specific than the generic Dog/Wolf tribe's own size_text ranges
// (summon_tribe_progression.size_text: "S-M"/"S-L"/"S-H"/"S-G" at
// C/B/A/S — the range of sizes a normal, non-Inuzuka summoned Dog/Wolf
// could choose), so this function's fixed progression is used instead,
// same "specific override wins" reasoning as ninDogToughness above.
func ninDogSizeForLevel(level int) string {
	switch {
	case level >= 12:
		return "Large"
	case level >= 8:
		return "Medium"
	default:
		return "Small"
	}
}

// ninDogJutsuSlotsMaxForRank: summon_tribe_progression's own "Jutsu Slots"
// column at each rank (5/7/10/13/16 at D/C/B/A/S) — the one column from
// that table still meaningful for a Nin-Dog, since Beast Master overrides
// the adjacent "Jutsu Known" column entirely ("Cannot learn jutsu like a
// Dog/Wolf summon, instead knowing all Inuzuka Clan Hijutsu you know").
func ninDogJutsuSlotsMaxForRank(rank string) int {
	switch rank {
	case "S":
		return 16
	case "A":
		return 13
	case "B":
		return 10
	case "C":
		return 7
	default:
		return 5
	}
}

// ninDogJutsuSlotShortRestRegen: "Your Nin-Dog regains 1 Jutsu Slot on a
// Short Rest... Starting at 11th level, your Nin-Dog regains 2 Jutsu Slots
// on a rest." Informational only — this app's short-rest action has no
// hook into a specific companion's own resource fields (unlike the
// character-wide customResourceGrants table), so recovering Jutsu Slots on
// a rest stays a manual edit of the delta-editable field below, the same
// "pool tracked, regen applied by hand" boundary several entries in
// custom_resources.go already draw for a flat-per-rest gain shape no
// existing restRegen kind expresses.
func ninDogJutsuSlotShortRestRegen(level int) int {
	if level >= 11 {
		return 2
	}
	return 1
}

// ninDogMaxHP: (Toughness + CON modifier) x level — see migration
// 0008_summon_tribes.sql's own comment on summon_tribes.toughness for this
// formula shape, with Toughness fixed at ninDogToughness (10) per Beast
// Master's own override rather than read from the tribe row.
func ninDogMaxHP(level, conMod int) int {
	hp := (ninDogToughness + conMod) * level
	if hp < 1 {
		hp = 1
	}
	return hp
}

// ninDogAC: "12+Half Nin-Dogs Level + Nin-Dogs Defensive Ability score" —
// read as ability MODIFIER, not the raw score the text literally says: the
// tribe's own defensive_ability is Dexterity, and every other formula in
// this book (e.g. jutsu_save_dc_text: "8 + Strength modifier + ...")
// spells out "modifier" explicitly when that's meant, so a bare "score"
// here reads as loose phrasing rather than a deliberate departure — using
// the raw score (14+) would produce an absurd AC in the high 20s at 1st
// level.
func ninDogAC(level, dexMod int) int {
	return 12 + level/2 + dexMod
}

// ninDogBiteAttack computes Bite's own rollable attack row fresh on every
// render — never stored as a charstore.CompanionAttack row, the same
// "computed, not stored" treatment titanBashAttack (titan.go) gives Titan's
// own built-in Bash attack, discovered to have the identical "flavor text
// only, never auto-added as a rollable row" gap while fixing that one.
// summon_tribe_attacks' own Bite description states the full formula for
// the attack roll ("Str + Prof to hit") but names no base weapon die at
// all for the damage — confirmed directly against the table, not merely
// unparsed — so DamageCount/DamageSides are deliberately left at 0 here
// rather than inventing a die size: the row still shows a flat Strength-
// modifier damage bonus (matching "+Str Piercing Damage" verbatim), just
// with no dice notation/roll button, since none is stated in the source.
// Young Kugsha's own breed enhancement ("Multiattack: up to two Bite
// attacks; adds Dexterity to damage once per turn") isn't modeled here —
// out of scope for this fix, same as this file's header doc already flags
// Nin-Dog's own per-rank ability-score-increase progression as a related,
// not-yet-addressed gap.
func ninDogBiteAttack(companion charstore.Companion, ownerProfBonus int) companionAttackRow {
	strMod := ninDogEffectiveAbilityModifier(companion, "str")
	return companionAttackRow{
		CompanionAttack: charstore.CompanionAttack{
			Name:        "Bite",
			Description: "Melee Weapon Attack: 5 ft., one target. On a roll of 16 or higher, the target is knocked Prone or dragged 5 feet in any direction the summon wants. No base weapon die is stated in the source — this is a flat Strength-modifier damage bonus only.",
			DamageType:  "piercing",
		},
		AttackTotal: strMod + ownerProfBonus,
		DamageTotal: strMod,
	}
}

// ninDogBaseSpeedForRank: summon_tribe_progression's own Speed column
// (30/45/50/60/75ft at D/C/B/A/S) — nothing in Beast Master's own text
// overrides this the way it overrides Toughness/size/AC, so the generic
// tribe progression is used as-is. Young Inuit's own "+10 increase to
// their speed" is added on top by the caller, not baked in here, since it
// only applies to that one breed.
func ninDogBaseSpeedForRank(rank string) int {
	switch rank {
	case "S":
		return 75
	case "A":
		return 60
	case "B":
		return 50
	case "C":
		return 45
	default:
		return 30
	}
}

// ninDogBreed is one of the three Young [Inuit/Kugsha/Tamaskan] breeds
// Beast Master lets a Nin-Dog take as its Role — hand-transcribed from the
// clan feature's own text (clan_features, sort_order 1) since no
// dedicated Nin-Dog monster block or breed table exists anywhere in
// rules.db; the generic summon_tribe_roles rows for tribe_slug=
// "summon/dog-wolf" (Lurker/Striker) are explicitly NOT used here — Beast
// Master says a Nin-Dog "cannot have the listed Roles in the Dog/Wolf
// summon page. They Instead treat their breed as their Role."
type ninDogBreed struct {
	Slug   string
	Name   string
	Blurb  string
	Traits []string
}

var ninDogBreeds = []ninDogBreed{
	{
		Slug:  "young-inuit",
		Name:  "Young Inuit",
		Blurb: "A breed known for its hunting skill.",
		Traits: []string{
			"Proficiency in Investigation and Stealth.",
			"+1 rank of Mastery in Perception.",
			"+10 increase to speed and ignores difficult terrain.",
			"Can Dash or Disengage at no action cost, once per round as part of one of its commands.",
			"Beginning at 11th level, gains the benefits of your Wild Sense feature.",
		},
	},
	{
		Slug:  "young-kugsha",
		Name:  "Young Kugsha",
		Blurb: "A breed known for its combat skill.",
		Traits: []string{
			"Proficiency in Acrobatics and Martial Arts.",
			"+1 rank of Mastery in Athletics.",
			"Multiattack: up to two Bite attacks; adds Dexterity to damage once per turn.",
			"+1 extra D-Rank Dog/Wolf special feature.",
			"Beginning at 11th level, +1 additional Dog/Wolf special feature of either D or C-Rank.",
		},
	},
	{
		Slug:  "young-tamaskan",
		Name:  "Young Tamaskan",
		Blurb: "A breed known for its ability to mold Chakra.",
		Traits: []string{
			"Proficiency in Chakra control and Ninshou.",
			"+1 rank of Mastery in Insight.",
			"Learns 1 D-Rank jutsu following the Dog/Wolf Jutsu specialty limits.",
			"The first jutsu it casts per short rest does not spend a Jutsu Slot.",
			"Beginning at 11th level, learns 1 C-Rank jutsu following the Dog/Wolf Jutsu specialty limits.",
		},
	},
}

func ninDogBreedBySlug(slug string) *ninDogBreed {
	for i := range ninDogBreeds {
		if ninDogBreeds[i].Slug == slug {
			return &ninDogBreeds[i]
		}
	}
	return nil
}

// ninDogFeaturePickSlot is one open (or not-yet-unlocked) special-feature
// pick slot on a Nin-Dog's own popup — see ninDogFeaturePickSlots.
type ninDogFeaturePickSlot struct {
	ChoiceIndex int
	Label       string
	// Ranks: which summon_tribe_features.rank values this slot may pick
	// from — normally a single rank, except Young Kugsha's 11th-level
	// bonus pick ("either D or C-Rank").
	Ranks    []string
	Unlocked bool
	Picked   string // feature name already chosen for this slot, "" if none yet
	// Options: the subset of the full Special Feature catalog eligible for
	// this slot's own Ranks — filtered in Go (ninDogFilterFeaturesByRank)
	// rather than in the template, since html/template has no built-in
	// "is this rank in that slice" test.
	Options []companionFeatureRef
}

// sortFeaturesByRank sorts features into rank order (E through S) in place.
// summon_tribe_features.sort_order reflects source-document parse order,
// not rank — it happens to coincide with rank for the one tribe currently
// in the DB, but isn't guaranteed to, so loadNinDogReference re-sorts
// explicitly here rather than trusting the query's own ORDER BY. Stable so
// same-rank entries keep sort_order as their tiebreaker, mirroring
// loadSummonTribeReference's own Progression sort (companions.go).
func sortFeaturesByRank(features []companionFeatureRef) {
	sort.SliceStable(features, func(i, k int) bool {
		return summonRankOrder[features[i].Rank] < summonRankOrder[features[k].Rank]
	})
}

// ninDogFilterFeaturesByRank returns the subset of features whose Rank is
// in ranks — used to populate each ninDogFeaturePickSlot's own Options.
func ninDogFilterFeaturesByRank(features []companionFeatureRef, ranks []string) []companionFeatureRef {
	var out []companionFeatureRef
	for _, f := range features {
		for _, r := range ranks {
			if f.Rank == r {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// ninDogFeaturePickSlots enumerates every special-feature pick slot a
// Nin-Dog has, in order: the four base rank-up picks Beast Master's own
// text grants ("gain 1 new Special feature of the appropriate rank" at
// levels 8/12/16/20), plus Young Kugsha's own two bonus picks. picks is
// keyed by ChoiceIndex, from loadNinDogFeaturePicks.
func ninDogFeaturePickSlots(breedSlug string, level int, picks map[int]string) []ninDogFeaturePickSlot {
	slots := []ninDogFeaturePickSlot{
		{ChoiceIndex: 0, Label: "C-Rank Special Feature (8th level)", Ranks: []string{"C"}, Unlocked: level >= 8},
		{ChoiceIndex: 1, Label: "B-Rank Special Feature (12th level)", Ranks: []string{"B"}, Unlocked: level >= 12},
		{ChoiceIndex: 2, Label: "A-Rank Special Feature (16th level)", Ranks: []string{"A"}, Unlocked: level >= 16},
		{ChoiceIndex: 3, Label: "S-Rank Special Feature (20th level)", Ranks: []string{"S"}, Unlocked: level >= 20},
	}
	if breedSlug == "young-kugsha" {
		slots = append(slots,
			ninDogFeaturePickSlot{ChoiceIndex: 4, Label: "Young Kugsha Bonus D-Rank Special Feature", Ranks: []string{"D"}, Unlocked: true},
			ninDogFeaturePickSlot{ChoiceIndex: 5, Label: "Young Kugsha Bonus D- or C-Rank Special Feature (11th level)", Ranks: []string{"D", "C"}, Unlocked: level >= 11},
		)
	}
	for i := range slots {
		slots[i].Picked = picks[slots[i].ChoiceIndex]
	}
	return slots
}

// loadNinDogFeaturePicks returns every special-feature pick already made
// for one Nin-Dog companion, keyed by choice_index — see
// character_feature_companion_choices (migration 0033), reused here
// exactly as Puppet Master's own Symphony Enhancement ability pick already
// reuses it (that table's own doc comment), just with this file's own
// feature_slug instead.
func (s *server) loadNinDogFeaturePicks(characterID, companionID int64) (map[int]string, error) {
	rows, err := s.charDB.Query(
		`SELECT choice_index, value FROM character_feature_companion_choices
		 WHERE character_id = ? AND feature_slug = ? AND companion_id = ?`,
		characterID, ninDogBeastMasterFeatureSlug, companionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var idx int
		var value string
		if err := rows.Scan(&idx, &value); err != nil {
			return nil, err
		}
		out[idx] = value
	}
	return out, rows.Err()
}

// loadNinDogSpecialFeatures returns the Dog/Wolf tribe's own rank-gated
// Special Features (summon_tribe_features, tribe_slug="summon/dog-wolf") —
// the same catalog summonTribeReference already reads for an ordinary
// kind="summon" companion, reused here as the picker's own option list.
func (s *server) loadNinDogSpecialFeatures() ([]companionFeatureRef, error) {
	rows, err := s.rulesDB.Query(
		`SELECT name, rank, description FROM summon_tribe_features WHERE tribe_slug = ? ORDER BY sort_order`,
		ninDogDogWolfTribeSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []companionFeatureRef
	for rows.Next() {
		var f companionFeatureRef
		if err := rows.Scan(&f.Name, &f.Rank, &f.Description); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ninDogKnownHijutsu returns the character's own known Inuzuka Clan
// Hijutsu — what the Nin-Dog itself knows per Beast Master ("Cannot learn
// jutsu like a Dog/Wolf summon, instead knowing all Inuzuka Clan Hijutsu
// you know") and the separate Inuzuka Hijutsu clan trait's own bonus
// D-Rank pick (below). character_jutsu (charDB) and jutsu (rulesDB) live
// in two separate SQLite files, so this is two queries rather than one
// cross-database join — the same shape every other cross-DB lookup in this
// app already uses.
func (s *server) ninDogKnownHijutsu(characterID int64) ([]summonNamedText, error) {
	slugRows, err := s.charDB.Query(`SELECT jutsu_slug FROM character_jutsu WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for slugRows.Next() {
		var slug string
		if err := slugRows.Scan(&slug); err != nil {
			slugRows.Close()
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	slugRows.Close()
	if err := slugRows.Err(); err != nil {
		return nil, err
	}
	if len(slugs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(slugs))
	args := make([]any, 0, len(slugs)+1)
	args = append(args, "clan/inuzuka")
	for i, slug := range slugs {
		placeholders[i] = "?"
		args = append(args, slug)
	}
	query := `SELECT name, description FROM jutsu WHERE clan_slug = ? AND classification = 'Hijutsu' AND slug IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY name`
	rows, err := s.rulesDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []summonNamedText
	for rows.Next() {
		var t summonNamedText
		if err := rows.Scan(&t.Name, &t.Description); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ninDogHijutsuTraitOptions returns the D-Rank Inuzuka Clan Hijutsu list —
// the option pool for the Inuzuka Hijutsu clan trait's own bonus pick
// ("Your Nin-Dog knows 1 Inuzuka Clan D-Rank Jutsu, that you do not need
// to add to your known jutsu list"), independent of what the character has
// personally learned.
func (s *server) ninDogHijutsuTraitOptions() ([]summonNamedText, error) {
	rows, err := s.rulesDB.Query(
		`SELECT name, description FROM jutsu WHERE clan_slug = 'clan/inuzuka' AND classification = 'Hijutsu' AND rank = 'D' ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []summonNamedText
	for rows.Next() {
		var t summonNamedText
		if err := rows.Scan(&t.Name, &t.Description); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ninDogReference is the read-only/interactive panel shown on a
// kind="nin-dog" companion's own popup and its card on the Companions tab
// — the Nin-Dog equivalent of summonTribeReference, but built from Beast
// Master's own Inuzuka-specific rules rather than the generic Dog/Wolf
// tribe row.
type ninDogReference struct {
	Rank     string
	Size     string
	Level    int
	Breeds   []ninDogBreed
	Breed    *ninDogBreed
	Features []companionFeatureRef // full catalog, Locked set by rank
	Slots    []ninDogFeaturePickSlot

	JutsuSlotsMax       int
	JutsuSlotShortRegen int
	KnownHijutsu        []summonNamedText
	HijutsuTraitOptions []summonNamedText
	HijutsuTraitPick    string
	JutsuSpecialtyText  string
	BiteDescription     string

	// Resistances/Immunities/ConditionImmunities: comma-joined free text,
	// matching snbReference/titanReference/summonTribeReference's own
	// identically-named/-shaped fields exactly — see
	// ninDogResistancesImmunities for how these are computed and why every
	// branch there currently resolves to "". Kept as real, always-run
	// infrastructure (not simply omitted) so the "nindog_reference"
	// template's own conditional dt/dd rows already exist and are ready the
	// moment a future Dog/Wolf special feature or breed trait actually
	// grants one — the exact same reasoning titanReference's own doc gives
	// for its own two upgrade-gated fields.
	Resistances         string
	Immunities          string
	ConditionImmunities string

	// Expected*: the same "computed hint, never silently overwritten, Sync
	// button available" treatment puppetCompanionView's own Expected*
	// fields already give a puppet — see companion_stat_fields.html's own
	// Sync-button block, which reads these same field names generically
	// regardless of companion kind.
	ExpectedAC    int
	ExpectedMaxHP int
	ExpectedSpeed int
	ExpectedSize  string
	ExpectedStr   int
	ExpectedDex   int
	ExpectedCon   int
	ExpectedInt   int
	ExpectedWis   int
	ExpectedCha   int
}

// ninDogReferenceOrZero returns *ref, or a zero-value ninDogReference if ref
// is nil. companion_sheet.html's own companion_stat_fields dict call needs
// to field-access NinDogReference's Expected* values regardless of the
// companion's kind (the dict is built once, before the kind branch), and
// html/template's parser rejects an {{if}} inside a pipeline argument
// outright (it is a block keyword, not an expression) — so the nil guard
// has to live here in Go, reached via a plain function call the parser
// accepts, rather than inline in the template.
func ninDogReferenceOrZero(ref *ninDogReference) ninDogReference {
	if ref == nil {
		return ninDogReference{}
	}
	return *ref
}

func (s *server) loadNinDogReference(characterID int64, companion charstore.Companion, characterLevel int) (*ninDogReference, error) {
	rank := ninDogRankForLevel(characterLevel)
	currentOrder := summonRankOrder[rank]

	features, err := s.loadNinDogSpecialFeatures()
	if err != nil {
		return nil, err
	}
	sortFeaturesByRank(features)
	for i := range features {
		features[i].Locked = summonRankOrder[features[i].Rank] > currentOrder
	}

	picks, err := s.loadNinDogFeaturePicks(characterID, companion.ID)
	if err != nil {
		return nil, err
	}
	slots := ninDogFeaturePickSlots(companion.NinDogBreed, characterLevel, picks)
	for i := range slots {
		slots[i].Options = ninDogFilterFeaturesByRank(features, slots[i].Ranks)
	}

	knownHijutsu, err := s.ninDogKnownHijutsu(characterID)
	if err != nil {
		return nil, err
	}
	hijutsuOptions, err := s.ninDogHijutsuTraitOptions()
	if err != nil {
		return nil, err
	}
	hijutsuPick, err := charstore.GetFeatureCompanionChoice(s.charDB, characterID, ninDogHijutsuTraitFeatureSlug, companion.ID, 0)
	if err != nil {
		return nil, err
	}

	var jutsuSpecialtyText, biteDescription string
	if err := s.rulesDB.QueryRow(
		`SELECT COALESCE(jutsu_specialty_text, '') FROM summon_tribes WHERE slug = ?`, ninDogDogWolfTribeSlug,
	).Scan(&jutsuSpecialtyText); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err := s.rulesDB.QueryRow(
		`SELECT description FROM summon_tribe_attacks WHERE tribe_slug = ? AND name = 'Bite'`, ninDogDogWolfTribeSlug,
	).Scan(&biteDescription); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// overrides: this Nin-Dog's own manual pins (companionOverrideFields),
	// consulted by every formula below ahead of SetNinDogStatDefaultsLive
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
		return ninDogBaseAbilityScores[key]
	}
	dexMod := charsheet.AbilityModifier(finalAbilityScore("dex"))
	conMod := charsheet.AbilityModifier(finalAbilityScore("con"))

	speed := ninDogBaseSpeedForRank(rank)
	if companion.NinDogBreed == "young-inuit" {
		speed += 10
	}
	if v, ok := companionOverrideInt(overrides, "speed"); ok {
		speed = int(v)
	}

	ac := ninDogAC(characterLevel, dexMod)
	if v, ok := companionOverrideInt(overrides, "ac"); ok {
		ac = int(v)
	}
	hpMax := ninDogMaxHP(characterLevel, conMod)
	if v, ok := companionOverrideInt(overrides, "hp_max"); ok {
		hpMax = int(v)
	}
	jutsuSlotsMax := ninDogJutsuSlotsMaxForRank(rank)
	if v, ok := companionOverrideInt(overrides, "jutsu_slots_max"); ok {
		jutsuSlotsMax = int(v)
	}
	size := ninDogSizeForLevel(characterLevel)
	if v, ok := overrides["size"]; ok {
		size = v
	}

	if err := charstore.SetNinDogStatDefaultsLive(s.charDB, characterID, companion.ID,
		int64(ac), int64(hpMax), int64(speed), int64(jutsuSlotsMax),
		int64(finalAbilityScore("str")), int64(finalAbilityScore("dex")), int64(finalAbilityScore("con")),
		int64(finalAbilityScore("int")), int64(finalAbilityScore("wis")), int64(finalAbilityScore("cha")), size,
	); err != nil {
		return nil, err
	}

	ref := &ninDogReference{
		Rank:                rank,
		Size:                size,
		Level:               characterLevel,
		Breeds:              ninDogBreeds,
		Breed:               ninDogBreedBySlug(companion.NinDogBreed),
		Features:            features,
		Slots:               slots,
		JutsuSlotsMax:       jutsuSlotsMax,
		JutsuSlotShortRegen: ninDogJutsuSlotShortRestRegen(characterLevel),
		KnownHijutsu:        knownHijutsu,
		HijutsuTraitOptions: hijutsuOptions,
		HijutsuTraitPick:    hijutsuPick,
		JutsuSpecialtyText:  jutsuSpecialtyText,
		BiteDescription:     biteDescription,
		ExpectedAC:          ac,
		ExpectedMaxHP:       hpMax,
		ExpectedSpeed:       speed,
		ExpectedSize:        size,
		ExpectedStr:         finalAbilityScore("str"),
		ExpectedDex:         finalAbilityScore("dex"),
		ExpectedCon:         finalAbilityScore("con"),
		ExpectedInt:         finalAbilityScore("int"),
		ExpectedWis:         finalAbilityScore("wis"),
		ExpectedCha:         finalAbilityScore("cha"),
	}
	ref.Resistances, ref.Immunities, ref.ConditionImmunities = ninDogResistancesImmunities(ref)
	return ref, nil
}

// ninDogResistanceCatalog: Dog/Wolf summon_tribe_features row name -> what
// it grants the Nin-Dog itself, the Nin-Dog-scoped twin of
// summonTribeResistanceCatalog (companions.go) — reusing that same
// summonTribeResistanceGrant shape rather than a parallel type, since the
// underlying data (a summon_tribe_features row's own name -> granted
// resistance/immunity/condition-immunity) is identical in shape here, just
// scoped to one tribe (summon/dog-wolf) instead of the whole catalog.
//
// Empty today: the investigation summonTribeResistanceCatalog's own header
// doc already ran across the FULL summon catalog (all ranks, every tribe,
// including summon/dog-wolf) found no qualifying row for Dog/Wolf — its
// only "resist"/"immun" hit, A-Rank's "Iron Fangs" ("This summon cannot
// have its damage resisted"), is the same offensive shape excluded there
// (a TARGET being unable to resist the Nin-Dog's own damage, not a trait of
// the Nin-Dog itself) — and re-confirmed directly against ninDogBreeds'
// own three hand-curated Traits lists (Young Inuit/Young Kugsha/Young
// Tamaskan), none of which mention a resistance, immunity, or condition
// immunity either. Kept as a real lookup table rather than a bare
// early-return specifically so a future Dog/Wolf feature or breed trait
// that does grant one is a one-line addition here instead of new plumbing.
var ninDogResistanceCatalog = map[string]summonTribeResistanceGrant{}

// ninDogResistancesImmunities resolves the Damage Resistances/Damage
// Immunities/Condition Immunities a Nin-Dog's own base stat block never
// states, checking ref's own currently-unlocked Dog/Wolf special features
// (ref.Features, already rank-gated by loadNinDogReference) against
// ninDogResistanceCatalog — the Nin-Dog equivalent of
// summonTribeResistancesImmunities, gated the exact same way (a Locked
// feature never contributes, so a resistance from a not-yet-reached rank
// doesn't show early). Breed traits (ninDogBreeds) are plain prose strings
// rather than a table row with a stable name to key a catalog by, so they
// aren't checked here at all; ninDogResistanceCatalog's own header doc
// records that none of the three currently grant anything either way.
// Every return value is "" today (see that table's own doc for why), which
// the "nindog_reference" template's own {{if}} guards read as "no row at
// all" rather than an empty one, the same convention titan_reference/
// summon_tribe_reference already use.
func ninDogResistancesImmunities(ref *ninDogReference) (resistances, immunities, conditionImmunities string) {
	var resistanceParts, immunityParts, conditionParts []string
	for _, f := range ref.Features {
		if f.Locked {
			continue
		}
		g, ok := ninDogResistanceCatalog[f.Name]
		if !ok {
			continue
		}
		if g.Resistance != "" {
			resistanceParts = append(resistanceParts, g.Resistance)
		}
		if g.Immunity != "" {
			immunityParts = append(immunityParts, g.Immunity)
		}
		if g.ConditionImmunity != "" {
			conditionParts = append(conditionParts, g.ConditionImmunity)
		}
	}
	return strings.Join(resistanceParts, ", "), strings.Join(immunityParts, ", "), strings.Join(conditionParts, ", ")
}

// prefillNinDogStatDefaults populates a freshly-created Nin-Dog's AC/HP-max/
// Speed/Jutsu-Slots-Max/six ability scores from Beast Master's own computed
// baseline — called exactly once, right after creation, mirroring
// prefillPuppetStatDefaults (companions.go) so a brand-new Nin-Dog reaches
// its first render already showing correct starting numbers instead of a
// blank card the player has to click every Sync button on just to see them.
// Reuses loadNinDogReference for the actual computation (ninDogAC/
// ninDogMaxHP/ninDogBaseSpeedForRank/ninDogJutsuSlotsMaxForRank/
// ninDogBaseAbilityScores, transitively) — the exact same values the Sync
// buttons already offer, so this can never drift from what a later manual
// Sync click would set. Uses charstore.SetNinDogStatDefaults (COALESCE-
// based), never SetCompanionFields, so it can't clobber a field the player
// somehow already touched between creation and this call.
func (s *server) prefillNinDogStatDefaults(characterID, companionID int64) error {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return err
	}
	companion, err := charstore.GetCompanion(s.charDB, characterID, companionID)
	if err != nil {
		return err
	}
	ref, err := s.loadNinDogReference(characterID, companion, sheet.Level)
	if err != nil || ref == nil {
		return err
	}
	return charstore.SetNinDogStatDefaults(s.charDB, characterID, companionID,
		int64(ref.ExpectedAC), int64(ref.ExpectedMaxHP), int64(ref.ExpectedMaxHP), int64(ref.ExpectedSpeed),
		int64(ref.JutsuSlotsMax), int64(ref.JutsuSlotsMax),
		int64(ref.ExpectedStr), int64(ref.ExpectedDex), int64(ref.ExpectedCon),
		int64(ref.ExpectedInt), int64(ref.ExpectedWis), int64(ref.ExpectedCha),
	)
}

// handleNinDogFeaturePick records (or changes) one special-feature pick —
// scoped to kind="nin-dog" the same way every other kind-specific action on
// a companion is, and validated server-side against the character's
// current pick slots so a stale/hand-crafted form POST can't pick a
// not-yet-unlocked rank or a feature from the wrong rank pool.
func (s *server) handleNinDogFeaturePick(w http.ResponseWriter, r *http.Request) {
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
		log.Println("load companion for nin-dog feature pick:", err)
		return
	}
	if companion.Kind != "nin-dog" {
		http.Error(w, "not a nin-dog", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	choiceIndex, err := strconv.Atoi(strings.TrimSpace(r.FormValue("choice_index")))
	if err != nil {
		http.Error(w, "bad choice_index", http.StatusBadRequest)
		return
	}
	featureName := strings.TrimSpace(r.FormValue("feature_name"))
	if featureName == "" {
		http.Error(w, "feature_name is required", http.StatusBadRequest)
		return
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for nin-dog feature pick:", err)
		return
	}
	slots := ninDogFeaturePickSlots(companion.NinDogBreed, sheet.Level, map[int]string{})
	var slot *ninDogFeaturePickSlot
	for i := range slots {
		if slots[i].ChoiceIndex == choiceIndex {
			slot = &slots[i]
			break
		}
	}
	if slot == nil || !slot.Unlocked {
		http.Error(w, "that pick is not open", http.StatusBadRequest)
		return
	}

	features, err := s.loadNinDogSpecialFeatures()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load nin-dog special features:", err)
		return
	}
	valid := false
	for _, f := range features {
		if f.Name == featureName {
			for _, allowedRank := range slot.Ranks {
				if f.Rank == allowedRank {
					valid = true
				}
			}
		}
	}
	if !valid {
		http.Error(w, "feature not eligible for that rank", http.StatusBadRequest)
		return
	}

	if err := charstore.SetFeatureCompanionChoice(s.charDB, id, ninDogBeastMasterFeatureSlug, cid, choiceIndex, featureName); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set nin-dog feature pick:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// handleNinDogHijutsuTraitPick records the Inuzuka Hijutsu clan trait's own
// one-time bonus D-Rank jutsu pick ("Your Nin-Dog knows 1 Inuzuka Clan
// D-Rank Jutsu, that you do not need to add to your known jutsu list") —
// its own feature_slug/handler, separate from the special-feature picks
// above, since it draws from a different catalog (D-Rank Inuzuka Hijutsu,
// not summon_tribe_features).
func (s *server) handleNinDogHijutsuTraitPick(w http.ResponseWriter, r *http.Request) {
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
		log.Println("load companion for nin-dog hijutsu trait pick:", err)
		return
	}
	if companion.Kind != "nin-dog" {
		http.Error(w, "not a nin-dog", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuName := strings.TrimSpace(r.FormValue("jutsu_name"))
	if jutsuName == "" {
		http.Error(w, "jutsu_name is required", http.StatusBadRequest)
		return
	}
	options, err := s.ninDogHijutsuTraitOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load nin-dog hijutsu trait options:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Name == jutsuName {
			valid = true
		}
	}
	if !valid {
		http.Error(w, "not a D-Rank Inuzuka Hijutsu", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureCompanionChoice(s.charDB, id, ninDogHijutsuTraitFeatureSlug, cid, 0, jutsuName); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set nin-dog hijutsu trait pick:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}
