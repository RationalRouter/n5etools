package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Eight of Science-Nin's ten subclasses' own cap+catalog picks — Elemental
// Innovationist (E.I.Ps, W.O.W, Perma Perk), Grenadier (B.I.M, the B.I.M
// Specialist designated-type overlay), Mad Scientist (Inversion Serums, the
// Sheep and the Shepherd designated-Serum overlay), Ninjaneer (Arsenal
// Modifications, Perfected Weapons), Shinobi-Ware (Shinobi-Ware Upgrades,
// the Glorious Evolution slot-free overlay, the In His Image permanent-pick
// overlay), Spyware (Spyware Programs, the Netrunner slot-free Quick Hack
// overlay), Storm Rider (A.T. Enhancements, the King's Road Regalia
// overlay), Technobi (Technobi Mechanizations), and S.N.B Specialist (S.N.B
// Upgrades, the Future of Shinobi slot-free-permanent overlay — the
// Scientific Ninja Beast companion itself remains unbuilt). Only Mech
// Crafter (Titan, a second companion system) is deliberately untouched here
// — see CLASS_AUDIT.md's Science-Nin detail entry.
//
// Five of these subclasses' own 20th-level "Future of Shinobi" capstones add
// a tracked slot/budget/permanent-pick bonus on top of an existing catalog
// above — see each one's own const doc and gating block in
// loadScienceNinSubclassData for exactly what's tracked vs. left manual.
//
// Every catalog here follows hunter_nin.go's simpler cap+count picker shape
// (a flat known-count against a level/ability-derived cap), NOT
// science_nin.go's own Creation-Points-BUDGET shape the base Scientific
// Ninja Tools catalog uses — confirmed against the book text for every one
// of them: each subclass's own granting feature states a slot/held COUNT
// ("Your bandolier holds a number of B.I.Ms equal to your proficiency
// bonus", "You can hold a number of Serums equal to your Intelligence
// modifier", "Your body also comes with a number of upgrade slots equal to
// your Proficiency Bonus"), not a budget spent from Creation Points the way
// Scientific Ninja Tools' own 33 tools are. Every entry's own "Cost: N
// Creation Points" stat line (present on most tiers, absent on W.O.W and
// on Regalia) is shown informationally only, via the same
// splitLeadingStatLine (format.go) badge every other rendered class_options
// entry already gets — not deducted from any tracked budget. This is a
// deliberate scope reduction: the book's own text for several of these
// catalogs (Arsenal Modifications, B.I.M, Shinobi-Ware Upgrades, ...) does
// describe an additional Creation-Points cost layered on top of the slot
// cap ("During a Long Rest you can spend creation points to install an
// upgrade"), but nothing in this app tracks per-weapon/per-chassis
// installation against a second budget, and several entries' own Cost is a
// RANGE ("2-6 Creation Points", the upgrade amount folded into the same
// field) with no separate "upgrade level" concept anywhere in this schema
// either — modeling the count-cap half (which fully answers "how many of
// these do you know") without inventing an unbuilt spend-tracking mechanism
// for the other half matches this file's own restraint elsewhere
// (Prerequisite is informational-only whenever it can't resolve to a real
// sibling entry).
//
// S.N.B Upgrades (S.N.B Specialist), Spyware Programs, and E.I.Ps (both
// Elemental Innovationist) are this file's three exceptions to
// "informational only": their own Cost IS deducted from
// data.CreationPointsUsed (see each one's own block in
// loadScienceNinSubclassData, below), because all three share the SAME
// single Creation Points budget the base Scientific Ninja Tools catalog
// spends from, rather than a second, separate per-installation budget — the
// identical shared-pool treatment Titan Upgrades already gets for Mech
// Crafter (titanUpgradesCreationPointsSpend, titan.go) and Ever Evolving
// Seal already gets for Shinobi-Ware (this file, below). Every entry's own
// Cost in these three catalogs is a flat single number (never a RANGE the
// way some of the other catalogs' entries are, confirmed against every
// entry in all three), which is what makes tracking it exact rather than
// another instance of the "no separate upgrade level concept" problem the
// rest of this paragraph describes. Cruel Angel's Thesis (Spyware) states
// its own Creation-Points spend explicitly in the book text; E.I.P's own
// granting feature doesn't name Creation Points directly, but every E.I.P
// entry carries the identical "Cost: N Creation Points" stat line the other
// two exceptions do, and this app's existing parser/display pipeline
// already treats that line the same way across all three catalogs.
//
// Storage is internal/charstore/science_nin_subclass_picks.go's single
// generic character_science_nin_subclass_picks table (migrations
// 0044_science_nin_subclass_picks.sql,
// 0046_science_nin_subclass_picks_more_categories.sql,
// 0055_science_nin_more_pick_categories.sql, and
// 0058_science_nin_ascended_wow.sql), with 20 category discriminators this
// file manages directly (eip/wow/ascended_wow/perma_perk/bim/
// bim_specialist/inversion_serum/sheep_and_shepherd_serum/arsenal_mod/
// perfected_weapon/shinobi_ware_upgrade/evolved_upgrade/shinjutsu_upgrade/
// spyware_program/quick_hack/air_treck_enhancement/regalia/
// technobi_mechanization/snb_upgrade/snb_upgrade_permanent) plus a 21st
// (mixed_studies_inquiry, base-class Mixed Studies — see
// cmd/n5e/science_nin.go) this file doesn't touch, and a pool column scoped
// to Inversion Serums alone.
//
// Storm Rider's own Air Trecks weapon is granted through a different
// mechanism entirely (ensureAirTrecksGranted, cmd/n5e/science_nin.go) — a
// real character_inventory row rather than a category pick, since it's a
// craftable, equippable, bulk-carrying WEAPON (with its own attack row in
// the normal Attacks & Jutsu table), not a pick from a static catalog.
const (
	scienceNinEIPFeatureSlug             = "class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/elemental-infused-perks-e-i-ps"
	scienceNinWOWFeatureSlug             = "class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/weapons-of-wonder-w-o-w"
	scienceNinPermaPerkFeatureSlug       = "class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/perma-perk"
	scienceNinBIMFeatureSlug             = "class/science-nin/group/scientific-inquiry/grenadier/feature/b-i-m"
	scienceNinInversionSerumsFeatureSlug = "class/science-nin/group/scientific-inquiry/mad-scientist/feature/inversion-serums"
	scienceNinArsenalFeatureSlug         = "class/science-nin/group/scientific-inquiry/ninjaneer/feature/enhanced-arsenal"
	scienceNinPerfectedWeaponFeatureSlug = "class/science-nin/group/scientific-inquiry/ninjaneer/feature/a-weapon-to-surpass"

	// The final four subclasses' own granting features.
	scienceNinEdgeRunnerFeatureSlug        = "class/science-nin/group/scientific-inquiry/shinobi-ware/feature/edge-runner"
	scienceNinGloriousEvolutionFeatureSlug = "class/science-nin/group/scientific-inquiry/shinobi-ware/feature/glorious-evolution"
	scienceNinCruelAngelsThesisFeatureSlug = "class/science-nin/group/scientific-inquiry/spyware/feature/cruel-angels-thesis"
	scienceNinNetrunnerFeatureSlug         = "class/science-nin/group/scientific-inquiry/spyware/feature/netrunner"
	// scienceNinFamiliarFacesFeatureSlug is Spyware's 9th-level Familiar
	// Faces — see jutsu_grants.go's loadGrantedJutsuLabels for the two named
	// jutsu (Transform, Advanced Transformation) it grants "as if it were on
	// your jutsu list." The Disguise Kit/Hacker's Kit charge costs gating
	// each cast, and Advanced Transformation's own 5-CCD-chakra payment,
	// stay entirely manual/narrated — no kit-charge tracking mechanism
	// exists anywhere in this app, the same deliberate carve-out Cruel
	// Angel's Thesis's own "expend 7+ Hacker's Kit charges" Program-cast
	// cost already gets elsewhere in this class.
	scienceNinFamiliarFacesFeatureSlug = "class/science-nin/group/scientific-inquiry/spyware/feature/familiar-faces"
	scienceNinAirTrecksFeatureSlug     = "class/science-nin/group/scientific-inquiry/storm-rider/feature/air-trecks"
	scienceNinKingsRoadFeatureSlug     = "class/science-nin/group/scientific-inquiry/storm-rider/feature/kings-road"
	scienceNinBestLaidTrapFeatureSlug  = "class/science-nin/group/scientific-inquiry/technobi/feature/the-best-laid-trap"
	scienceNinSNBUpgradesFeatureSlug   = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/s-n-b-upgrades"

	// Five of the ten 20th-level "Future of Shinobi" subclass capstones —
	// see each one's own gating block in loadScienceNinSubclassData below
	// for what's tracked vs. left manual. (The other five — Elemental
	// Innovationist's Aether, Grenadier's Destruction, Mech-Crafter's Mecha,
	// S.N.B Specialist's own S.N.B, and Storm Rider's Sky Keeper — either
	// have no countable component here, or (Sky Keeper) resolve through
	// internal/features/grants.go's speedGrants and passive_traits.go
	// instead of a cap+catalog pick.) Every one of these five previously
	// shipped with a NULL level in rules.db (ordinalLevelRe couldn't match
	// "At Level 20, ..." — no ordinal suffix); see migration
	// 0055_science_nin_future_of_shinobi_null_level.sql and
	// internal/parse/subclasses.go's knownFeatureLevelOverrides for the fix.
	scienceNinFutureOfShinobiShinobiWareFeatureSlug = "class/science-nin/group/scientific-inquiry/shinobi-ware/feature/the-future-of-shinobi-shinobi-ware"
	scienceNinFutureOfShinobiWeaponsFeatureSlug     = "class/science-nin/group/scientific-inquiry/ninjaneer/feature/the-future-of-shinobi-weapons"
	scienceNinFutureOfShinobiSNBFeatureSlug         = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/the-future-of-shinobi-s-n-b"
	// scienceNinSkyKeeperFeatureSlug's own tracked half (a flat +60ft Speed
	// bonus) lives in internal/features/grants.go's speedGrants, and its
	// falling-damage immunity in passive_traits.go's passiveTraitGrants —
	// this file only reads it to widen King's Road's Regalia cap to 2.
	scienceNinSkyKeeperFeatureSlug = "class/science-nin/group/scientific-inquiry/storm-rider/feature/the-future-of-shinobi-sky-keeper"

	// Four more 9th/17th-level subclass features, each granting a single
	// designated pick drawn from a subset of a character's own already-known
	// picks in a sibling catalog above.
	scienceNinBIMSpecialistFeatureSlug    = "class/science-nin/group/scientific-inquiry/grenadier/feature/b-i-m-specialist"
	scienceNinSheepAndShepherdFeatureSlug = "class/science-nin/group/scientific-inquiry/mad-scientist/feature/the-sheep-and-the-shepherd"
	scienceNinInHisImageFeatureSlug       = "class/science-nin/group/scientific-inquiry/shinobi-ware/feature/in-his-image"

	// scienceNinElementalInnovationFeatureSlug is Elemental Innovationist's
	// 17th-level Elemental Innovation, granting two independent halves: an
	// Overcharge spend pool (custom_resources.go's
	// "elemental_innovation_overcharge" key, keyed off this same const) and
	// an Ascended W.o.W designation (DesignatedWoW below).
	scienceNinElementalInnovationFeatureSlug = "class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/elemental-innovation"

	// scienceNinElementalInnovationistSubclassSlug identifies Elemental
	// Innovationist itself, for scienceNinHasPermaPerk's own direct
	// class/subclass-level gate check below.
	scienceNinElementalInnovationistSubclassSlug = "class/science-nin/group/scientific-inquiry/elemental-innovationist"

	// scienceNinPermaPerkGrantingLevel is Perma Perk's own granting level —
	// confirmed against dist/rules.db's subclass_features row for
	// scienceNinPermaPerkFeatureSlug.
	scienceNinPermaPerkGrantingLevel = 14

	// scienceNinRazorEIPSlug is Razor E.I.P's own class_option_entries slug
	// (Greater tier): "Increase your critical threat range with your
	// Unarmed Strike and Weapon Attacks by +1." Like the four Perma-Perk-
	// eligible E.I.Ps applied in internal/charsheet/charsheet.go, this
	// clause is only live once Razor is the character's designated Perma
	// Perk (14th level: "gaining its benefits... without needing to spend
	// CCD chakra to activate") — a merely-known Razor E.I.P grants nothing,
	// same as every other E.I.P (see this file's own header doc). Applied
	// in weaponSpecialistCritRangeThreshold (characters.go) since attack
	// rows are built there rather than in internal/charsheet.
	scienceNinRazorEIPSlug = "class/science-nin/option/e-i-ps/greater/entry/razor-e-i-p"
)

// scienceNinHasPermaPerk reports whether characterID currently has Perma
// Perk active (14+ levels of Science-Nin with Elemental Innovationist
// chosen) with wantSlug (a class_option_entries E.I.P slug) as their
// designated Perma Perk. Checks the level/subclass gate directly against
// character_classes/character_subclasses rather than trusting the stored
// pick alone — mirrors exactly what features.LoadGrantedFeatures itself
// checks for a subclass feature (class level >= feature level AND the
// subclass slug present in character_subclasses), so a delevel below 14th
// or a subclass swap away from Elemental Innovationist (charstore.SetLevel/
// SetSubclass, both of which leave any existing pick row in place rather
// than clearing it) correctly stops matching, the same "recheck the
// granting condition at the consumption site" discipline
// ninjaneer.go's recomputeNinjaneerWeaponOverrides already applies to its
// own weapon designations.
func (s *server) scienceNinHasPermaPerk(characterID int64, wantSlug string) (bool, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, scienceNinSlug,
	).Scan(&level)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if level < scienceNinPermaPerkGrantingLevel {
		return false, nil
	}
	var hasSubclass bool
	if err := s.charDB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM character_subclasses WHERE character_id = ? AND subclass_slug = ?)`,
		characterID, scienceNinElementalInnovationistSubclassSlug,
	).Scan(&hasSubclass); err != nil {
		return false, err
	}
	if !hasSubclass {
		return false, nil
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickPermaPerk)
	if err != nil {
		return false, err
	}
	matched := false
	for _, p := range picks {
		if p.OptionSlug == wantSlug {
			matched = true
			break
		}
	}
	if !matched {
		return false, nil
	}
	// Cross-checked against Known E.I.Ps rather than trusted outright — see
	// resolveScienceNinPermaPerk's own doc (internal/charsheet/charsheet.go)
	// for why a stored Perma Perk pick can outlive the E.I.P it designates.
	knownEIPs, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickEIP)
	if err != nil {
		return false, err
	}
	for _, k := range knownEIPs {
		if k.OptionSlug == wantSlug {
			return true, nil
		}
	}
	return false, nil
}

// eipCap: Exoskeleton begins with 2 E.I.P slots, +1 at 6th, +1 at 9th.
func eipCap(level int) int {
	switch {
	case level >= 9:
		return 4
	case level >= 6:
		return 3
	default:
		return 2
	}
}

// wowCap: 1 Weapon of Wonder, 2 starting at 17th.
func wowCap(level int) int {
	if level >= 17 {
		return 2
	}
	return 1
}

// bimCap/arsenalModCap: both "a number equal to your Proficiency Bonus" —
// Grenadier's bandolier and Ninjaneer's Enhanced Arsenal Upgrade Slots.
func bimCap(proficiencyBonus int) int { return proficiencyBonus }

func arsenalModCap(proficiencyBonus int) int { return proficiencyBonus }

// inversionSerumCap/perfectedWeaponCap: both "a number equal to your
// Intelligence modifier" — clamped at 0 rather than letting a negative Int
// modifier produce a negative cap (an available-list existing with a
// negative remaining budget reads as broken, not as "you truly can't have
// any").
func inversionSerumCap(intMod int) int {
	if intMod < 0 {
		return 0
	}
	return intMod
}

func perfectedWeaponCap(intMod int) int {
	if intMod < 0 {
		return 0
	}
	return intMod
}

// nonNegativeIntMod clamps a negative Intelligence modifier to 0 before it's
// added as a bonus on top of an existing cap — same "a negative addend reads
// as broken, not as fewer slots" reasoning inversionSerumCap/
// perfectedWeaponCap already apply to their own base caps. Shared by every
// Future of Shinobi capstone below that grants "additional slots equal to
// your Intelligence Modifier" (Shinobi-Ware, Ninjaneer).
func nonNegativeIntMod(intMod int) int {
	if intMod < 0 {
		return 0
	}
	return intMod
}

// shinobiWareUpgradeCap/spywareProgramCap/airTreckEnhancementCap/
// technobiMechanizationCap/snbUpgradeCap: every one of the final four
// subclasses' own base catalog caps is "a number equal to your Proficiency
// Bonus" — Edge Runner's upgrade slots, Cruel Angel's Thesis' held
// Programs, Air Trecks' own enhancement slots, The Best Laid Trap's held
// Mechanizations, and S.N.B Upgrades' installed upgrade slots, all
// independently re-confirmed against the book text.
func shinobiWareUpgradeCap(proficiencyBonus int) int    { return proficiencyBonus }
func spywareProgramCap(proficiencyBonus int) int        { return proficiencyBonus }
func airTreckEnhancementCap(proficiencyBonus int) int   { return proficiencyBonus }
func technobiMechanizationCap(proficiencyBonus int) int { return proficiencyBonus }
func snbUpgradeCap(proficiencyBonus int) int            { return proficiencyBonus }

// gloriousEvolutionAllowedTiers: Glorious Evolution (6th level) evolves a
// Refined Shinobi-Ware Upgrade for free; at 9th level a Greater Upgrade is
// also allowed, at 14th a Superior Upgrade too — each level threshold ADDS
// a tier rather than replacing the one before it, confirmed against the
// feature's own "At 9th Level you can INSTEAD choose a Greater Upgrade...
// At Level 14 you can choose a Superior Upgrade" (both earlier tiers stay
// legal picks, "instead" reads as "in addition" given the cap is a held
// count of 2, not a single always-current slot).
func gloriousEvolutionAllowedTiers(level int) map[string]bool {
	allowed := map[string]bool{"refined": true}
	if level >= 9 {
		allowed["greater"] = true
	}
	if level >= 14 {
		allowed["superior"] = true
	}
	return allowed
}

// scienceNinSubclassOption is one entry in any of this file's seven
// catalogs. Tier is "" for W.O.W (already one row per named weapon, no
// tier bundling to split) and for the Perma Perk/Perfected Weapon
// restricted sub-catalogs (drawn from another category's own rows, tier
// carried on the underlying entry instead). Epithet is a display-only
// "Cost: ..." badge (see splitLeadingStatLine, format.go) — informational,
// never gates picking.
type scienceNinSubclassOption struct {
	Slug        string
	Name        string
	Tier        string
	Epithet     string
	Description string
	// FixedPool is Inversion Serums only ("mending"/"maiming" when the
	// entry's own Drain line names one CCD half specifically, "" for a
	// dual-effect serum the player may pay from either half).
	FixedPool string
}

// knownScienceNinPick is one entry on a Known list for any of this file's
// seven catalogs. Pool is Inversion Serums only. Quantity is left at its
// zero value everywhere except B.I.M (splitScienceNinPicks never sets it;
// the template only reads it in the B.I.M section) — see
// splitScienceNinBIMPicks for the one catalog where a single Known entry
// can represent more than one held copy. Cost is S.N.B Upgrades, Spyware
// Programs, and E.I.Ps only, set in a separate pass after
// splitScienceNinPicks runs (that function has no Cost concept — every
// other catalog's own Cost badge stays purely informational, see this
// file's own header doc) since those three are the catalogs here that
// actually spend from the shared Creation Points budget
// (loadScienceNinSubclassData's own S.N.B Upgrades/Spyware/E.I.P blocks).
type knownScienceNinPick struct {
	Slug     string
	Name     string
	Tier     string
	Pool     string
	Quantity int
	Cost     int
}

// scienceNinRegaliaOptions: King's Road's own 8 Regalia types, hand-
// curated from the "Air Treck Enhancements" class_options row's own
// bundled Regalia entry. Confirmed live against a freshly re-ingested
// rules.db that internal/store/classoptionentries.go's own bundling logic
// DOES split this tier's 8 ALL-CAPS-headed Regalia names into real
// class_option_entries rows after all (a "Cost:" stat line was never a
// requirement for a split, just usually present alongside one) — that
// real, splittable data is filtered out of loadScienceNinSubclassData's
// own Air Treck Enhancements catalog (see the "Regalia" tier exclusion
// there) rather than switched to as this table's source, since this
// hand-curated copy already carries its own deliberately-preserved source
// quirk (see below) and a second, differently-keyed data source for the
// same 8 options would only add confusion. Text transcribed verbatim from
// rules.db, including the source's own "Flame Regalia" naming slip inside
// Lightning Regalia's own description (left uncorrected, same
// "informational only, never silently fixed" restraint this file's own
// header doc already applies to Prerequisite text).
var scienceNinRegaliaOptions = []scienceNinSubclassOption{
	{Slug: "science-nin/regalia/gem", Name: "Gem Regalia", Description: "While equipped with the Gem Regalia you can cast ninjutsu with the Earth Release keyword without needing the HS component. Your A. Ts now deal earth damage and all earth damage you deal ignores resistance. Additionally, while equipped with this Regalia you reduce all damage you take by an amount equal to your Constitution modifier."},
	{Slug: "science-nin/regalia/fang", Name: "Fang Regalia", Description: "While equipped with the Fang Regalia you can cast bukijutsu with the slashing component without needing the mobility component. Your A. Ts have their damage die increased by 1 step and all slashing damage you deal ignores resistance. Additionally, while equipped with this Regalia once per turn when you deal slashing damage you can inflict a rank of bleed."},
	{Slug: "science-nin/regalia/flame", Name: "Flame Regalia", Description: "While equipped with the Flame Regalia you can cast ninjutsu with the Fire Release keyword without needing the HS component. Your A. Ts now deal fire damage and all fire damage you deal ignores resistance. Additionally, while equipped with this Regalia once per turn when you deal fire damage you can inflict a rank of burned."},
	{Slug: "science-nin/regalia/lightning", Name: "Lightning Regalia", Description: "While equipped with the Lightning Regalia you can cast ninjutsu with the Lightning Release keyword without needing the HS component. Your A. Ts now deal lightning damage and all lightning damage you deal ignores resistance. Additionally, while equipped with this Regalia once per turn when you deal lightning damage you can inflict a rank of shocked."},
	{Slug: "science-nin/regalia/roar", Name: "Roar Regalia", Description: "While equipped with the Roar Regalia you can cast genjutsu with the Auditory keyword without needing the HS component. Your A. Ts now deal force damage and all force damage you deal ignores resistance. Additionally, while equipped with this Regalia once per turn when you deal force damage you can inflict a rank of Bruised."},
	{Slug: "science-nin/regalia/thorn", Name: "Thorn Regalia", Description: "While equipped with the Thorn Regalia you can cast bukijutsu with the piercing component without needing the mobility component. Your A. Ts now deal piercing damage and all piercing damage you deal ignores resistance. Additionally, while equipped with this Regalia whenever you are hit with a melee attack a creature takes piercing damage equal to your Constitution modifier. A creature can only take damage from this Regalia twice per round."},
	{Slug: "science-nin/regalia/water", Name: "Water Regalia", Description: "While equipped with the Water Regalia you can cast ninjutsu with the Water Release keyword without needing the HS component. Your A. Ts now deal cold damage and all cold damage you deal ignores resistance. Additionally, while equipped with this Regalia you can spend 5 CCD chakra to gain the benefits of having a source of water when you cast a jutsu with the Water Release Keyword."},
	{Slug: "science-nin/regalia/wind", Name: "Wind Regalia", Description: "While equipped with the Wind Regalia you can cast ninjutsu with the Wind Release keyword without needing the HS component. Your A. Ts now deal wind damage and all wind damage you deal ignores resistance. Additionally, while equipped with this Regalia your AC is increased by +1."},
}

// inversionSerumPoolPattern finds an Inversion Serum entry's own Drain
// line to determine whether it's paid from a specific CCD half or either —
// see scienceNinSubclassOption.FixedPool's own doc.
var inversionSerumPoolPattern = regexp.MustCompile(`Drain:\s*\d+\s*(Mending|Maiming)?\s*CCD Chakra`)

func inversionSerumFixedPool(description string) string {
	m := inversionSerumPoolPattern.FindStringSubmatch(description)
	if m == nil || m[1] == "" {
		return ""
	}
	return strings.ToLower(m[1])
}

// loadScienceNinEntryCatalog reads a bundled-and-split catalog (E.I.Ps,
// Explosive Modifications, Inversion Serums, Arsenal Modifications — each
// tier's own class_options row split into class_option_entries rows the
// same way Scientific Ninja Tools' own 6 tiers already are, see
// internal/store/classoptionentries.go). serumPool controls whether
// FixedPool is resolved (Inversion Serums only; every other catalog leaves
// it "").
//
// The main query is an INNER JOIN against class_option_entries, so a tier
// with ZERO split rows drops out of the result entirely rather than
// appearing with an empty item list — internal/store/classoptionentries.go
// only splits a tier's own description when it detects 2+ named
// sub-entries (see textentries.FindEntries), so every catalog's own
// Mastercraft tier (bundling exactly one named item: OVERCHARGE, STUN
// B.I.M, Amplification Matrix, C-DRIVE, WONDER E.I.P, GIANT SIZE, STUNNING
// BLAST — confirmed via sqlite3 against dist/rules.db) falls below that
// threshold and silently vanishes from every affected catalog. Appending
// loadScienceNinChildlessTierOptions' own synthesized rows repairs every
// affected catalog through this one shared call site, the same "fix a
// whole class of bug in one pass" shape loadTitanUpgradeCatalog (titan.go)
// already applies to Titan Upgrades' own Mastercraft/Bijuu Slayer row —
// this is the general form of that same special case.
func (s *server) loadScienceNinEntryCatalog(listName string, resolveSerumPool bool) ([]scienceNinSubclassOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT e.slug, e.name, e.description, o.name
		FROM class_option_entries e
		JOIN class_options o ON o.slug = e.class_option_slug
		WHERE o.class_slug = ? AND o.list_name = ?
		ORDER BY o.sort_order, e.sort_order`, scienceNinSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scienceNinSubclassOption
	for rows.Next() {
		var o scienceNinSubclassOption
		var description string
		if err := rows.Scan(&o.Slug, &o.Name, &description, &o.Tier); err != nil {
			return nil, err
		}
		o.Epithet, _ = splitLeadingStatLine(description)
		o.Description = description
		if resolveSerumPool {
			o.FixedPool = inversionSerumFixedPool(description)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	childless, err := s.loadScienceNinChildlessTierOptions(listName)
	if err != nil {
		return nil, err
	}
	for _, o := range childless {
		if resolveSerumPool {
			o.FixedPool = inversionSerumFixedPool(o.Description)
		}
		out = append(out, o)
	}
	return out, nil
}

// loadScienceNinChildlessTierOptions synthesizes one scienceNinSubclassOption
// per class_options row under listName that has zero class_option_entries
// children — see loadScienceNinEntryCatalog's own doc for why such a tier
// exists and why it would otherwise vanish from the catalog entirely.
// Slug is the class_options row's own slug (there is no
// class_option_entries row to point at); Name/Description are split off
// the tier's raw class_options.description via splitScienceNinSoloTierItem.
func (s *server) loadScienceNinChildlessTierOptions(listName string) ([]scienceNinSubclassOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT o.slug, o.name, o.description
		FROM class_options o
		WHERE o.class_slug = ? AND o.list_name = ?
		  AND NOT EXISTS (SELECT 1 FROM class_option_entries e WHERE e.class_option_slug = o.slug)
		ORDER BY o.sort_order`, scienceNinSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scienceNinSubclassOption
	for rows.Next() {
		var slug, tier, description string
		if err := rows.Scan(&slug, &tier, &description); err != nil {
			return nil, err
		}
		name, body := splitScienceNinSoloTierItem(tier, description)
		o := scienceNinSubclassOption{Slug: slug, Name: name, Tier: tier}
		o.Epithet, _ = splitLeadingStatLine(body)
		o.Description = body
		out = append(out, o)
	}
	return out, rows.Err()
}

// splitScienceNinSoloTierItem peels a lone bundled item's own ALL-CAPS name
// off the front of its tier's raw class_options.description — e.g.
// Spyware Programs' own Mastercraft tier, "OVERCHARGE Cost: 32 Creation
// Points Drain: 30 CCD Chakra ..." Reuses findEntries (format.go's own
// wrapper around textentries.FindEntries, the SAME detection
// internal/store/classoptionentries.go runs at ingest time to split a
// MULTI-item tier), just without that caller's own 2-match threshold — one
// match is exactly what a single bundled item produces. Falls back to the
// tier's own name/description untouched (never observed live against
// dist/rules.db, but keeps a future malformed row from silently rendering
// with a garbage Name instead of failing visibly as "just the tier name").
func splitScienceNinSoloTierItem(tierName, description string) (name, body string) {
	entries := findEntries(description)
	if len(entries) == 0 {
		return tierName, description
	}
	e := entries[0]
	name = description[e.NameStart:e.NameEnd]
	if e.Kind == entryKindCaps {
		name = titleCase(name)
	}
	return name, strings.TrimSpace(description[e.BodyStart:])
}

// loadScienceNinFlatCatalog reads a catalog that is already one row per
// named choice with nothing to split (W.O.W's own 9 Weapons of Wonder).
func (s *server) loadScienceNinFlatCatalog(listName string) ([]scienceNinSubclassOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND list_name = ? ORDER BY sort_order, name`, scienceNinSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scienceNinSubclassOption
	for rows.Next() {
		var o scienceNinSubclassOption
		var description string
		if err := rows.Scan(&o.Slug, &o.Name, &description); err != nil {
			return nil, err
		}
		o.Epithet, _ = splitLeadingStatLine(description)
		o.Description = description
		out = append(out, o)
	}
	return out, rows.Err()
}

// splitScienceNinPicks classifies a catalog against a character's own
// stored picks for one category — shared by every category except
// Inversion Serums, which additionally carries a per-pick Pool (see
// splitInversionSerumPicks below).
func splitScienceNinPicks(catalog []scienceNinSubclassOption, picked map[string]bool) (known []knownScienceNinPick, available []scienceNinSubclassOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownScienceNinPick{Slug: o.Slug, Name: o.Name, Tier: o.Tier})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// splitScienceNinBIMPicks is splitScienceNinPicks' own B.I.M-specific
// cousin: "you can pick any modification with 'B.I.M' in the name more
// than once, other than the Barrier B.I.M." Known carries each held type's
// own Quantity (the row's stored held count, always >= 1 for anything
// present at all); unlike splitScienceNinPicks, Known and Available are NOT
// mutually exclusive here — a known, non-Barrier type stays in Available so
// it can be picked again, and only drops out once BIMCap is reached (the
// generic Add handler's own used>=cap gate, not this split). Barrier B.I.M
// is the one exception, matched by name the same way Perma Perk excludes
// Wonder E.I.P above: once known, it drops out of Available for good, the
// same "known picks leave Available" treatment every other category's
// picks get unconditionally.
func splitScienceNinBIMPicks(catalog []scienceNinSubclassOption, picks []charstore.ScienceNinSubclassPick) (known []knownScienceNinPick, available []scienceNinSubclassOption) {
	qtyBySlug := make(map[string]int, len(picks))
	for _, p := range picks {
		qtyBySlug[p.OptionSlug] = p.Quantity
	}
	for _, o := range catalog {
		qty, isKnown := qtyBySlug[o.Slug]
		if isKnown {
			known = append(known, knownScienceNinPick{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Quantity: qty})
			if strings.EqualFold(o.Name, "Barrier B.I.M") {
				continue // the one explicitly-named exception — no second Barrier B.I.M
			}
		}
		available = append(available, o)
	}
	return known, available
}

// scienceNinDescBySlug indexes a catalog by slug so a Designated/Permanent
// picker (built from an already-Known knownScienceNinPick list, which
// carries no Description of its own) can look up the same catalog text its
// sibling non-designated picker already renders as its tooltip.
func scienceNinDescBySlug(catalog []scienceNinSubclassOption) map[string]string {
	m := make(map[string]string, len(catalog))
	for _, o := range catalog {
		m[o.Slug] = o.Description
	}
	return m
}

// scienceNinElementalInnovationistData backs E.I.Ps, W.O.W, and Perma Perk
// — all three gated on the character actually having the matching granting
// feature (checked in loadScienceNinSubclassData), so a nil field here
// means "not this subclass, or not this level yet" and the template
// removes that whole sub-section from the DOM rather than showing it
// empty/disabled.
type scienceNinElementalInnovationistData struct {
	EIPCap        int
	EIPUsed       int
	KnownEIPs     []knownScienceNinPick
	AvailableEIPs []scienceNinSubclassOption

	WOWCap       int
	WOWUsed      int
	KnownWOW     []knownScienceNinPick
	AvailableWOW []scienceNinSubclassOption

	// DesignatedWoW is non-nil only once the character has the 17th-level
	// Elemental Innovation feature (the same feature backing the
	// Overcharge spend pool, custom_resources.go's
	// "elemental_innovation_overcharge" key). AvailableDesignatedWoW is
	// restricted to the character's own KnownWOW (any known W.o.W type
	// qualifies, no exclusion clause) — same "restricted to already-known"
	// shape B.I.M Specialist (scienceNinGrenadierData.DesignatedBIM) uses.
	// Only WHICH W.o.W is designated Ascended is tracked here; the halved
	// ammo Creation-Point cost and the damage-ignores-resistance/treats-
	// immunity-as-resistance payload stay entirely manual/narrated — no
	// per-item cost-modifier or damage-type-override concept exists
	// anywhere in this app.
	DesignatedWoW          *knownScienceNinPick
	AvailableDesignatedWoW []scienceNinSubclassOption

	// PermaPerk is non-nil only once the character has the 14th-level Perma
	// Perk feature. AvailablePermaPerk is drawn from the character's own
	// KnownEIPs, excluding Wonder E.I.P (the book's own restriction) and
	// whichever E.I.P is already the Perma Perk — a single-slot pick, same
	// "cap 1, freely re-picked" shape Cooking-Nin's Food For the Soul uses,
	// modeled here as a cap+catalog category instead purely so it reuses
	// this file's one generic Add/Delete handler pair rather than a bespoke
	// single-select route.
	PermaPerk          *knownScienceNinPick
	AvailablePermaPerk []scienceNinSubclassOption
}

// scienceNinGrenadierData backs B.I.M (Explosive Modifications) and the
// B.I.M Specialist overlay. BIMUsed is the TOTAL held count (summed
// Quantity across every KnownBIM entry, not len(KnownBIM)) — the book's own
// "you can pick any modification with 'B.I.M' in the name more than once,
// other than the Barrier B.I.M" means the bandolier's real capacity is
// against total copies held, not distinct types known. See
// splitScienceNinBIMPicks for how KnownBIM/AvailableBIM are built.
type scienceNinGrenadierData struct {
	BIMCap       int
	BIMUsed      int
	KnownBIM     []knownScienceNinPick
	AvailableBIM []scienceNinSubclassOption

	// DesignatedBIM is non-nil only once the character has the 9th-level
	// B.I.M Specialist feature. AvailableDesignatedBIM is restricted to the
	// character's own KnownBIM (any known B.I.M type qualifies, no exclusion
	// clause) — same "restricted to already-known" shape Perma Perk/Quick
	// Hack establish. Only WHICH B.I.M type is designated is tracked here —
	// the half-cost upgrade discount and bonus-action on-the-fly creation
	// (paying twice the max cost from CCD) stay Group 2/3, manual/narrated;
	// no creation-point-cost-modifier or action-economy tracking exists for
	// either.
	DesignatedBIM          *knownScienceNinPick
	AvailableDesignatedBIM []scienceNinSubclassOption
}

// scienceNinMadScientistData backs Inversion Serums and the Sheep and the
// Shepherd overlay. Known/Available pairs with Pool are used instead of
// splitScienceNinPicks' plain shape — see splitInversionSerumPicks.
type scienceNinMadScientistData struct {
	SerumCap        int
	SerumUsed       int
	KnownSerums     []knownScienceNinPick
	AvailableSerums []scienceNinSubclassOption

	// DesignatedSerum is non-nil only once the character has the 17th-level
	// The Sheep and the Shepherd feature. AvailableDesignatedSerum is
	// restricted to the character's own KnownSerums, further filtered to
	// dual-effect Serums only (FixedPool == "" on the underlying catalog
	// entry — a Serum whose own Drain line names one CCD half specifically
	// doesn't qualify), cross-referenced by slug against the catalog the
	// same way Perma Perk cross-references ei.KnownEIPs against its own
	// catalog. Only WHICH known dual-effect Serum is designated is tracked
	// — auto-applying its Mend/Maim effect whenever a Medical Ninjutsu heals
	// or damages stays Group 2/3, manual/narrated; no jutsu-cast side-effect
	// automation exists anywhere in this app.
	DesignatedSerum          *knownScienceNinPick
	AvailableDesignatedSerum []scienceNinSubclassOption
}

// scienceNinNinjaneerData backs Arsenal Modifications and the independent
// Perfected Weapons cap.
type scienceNinNinjaneerData struct {
	ArsenalModCap        int
	ArsenalModUsed       int
	KnownArsenalMods     []knownScienceNinPick
	AvailableArsenalMods []scienceNinSubclassOption

	// PerfectedWeaponCap/Used/Known/Available restrict Arsenal
	// Modifications' own Minor tier only ("gains 1 Minor Modification of
	// your choice at no creation point cost") — a second, independent cap
	// (Intelligence modifier) drawing from the same underlying rows as
	// ArsenalMod above but tracked as its own category so a Minor
	// Modification bought with Creation Points and one granted for free by
	// A Weapon to Surpass never collide over the same option_slug (the
	// storage table's own UNIQUE (character_id, category, option_slug) is
	// keyed by category first for exactly this reason).
	PerfectedWeaponCap        int
	PerfectedWeaponUsed       int
	KnownPerfectedWeapons     []knownScienceNinPick
	AvailablePerfectedWeapons []scienceNinSubclassOption

	// EnhancedWeapon/AvailableEnhancedWeapons back Enhanced Arsenal's own
	// single-slot "which owned weapon is my Enhanced Weapon" designation —
	// see cmd/n5e/ninjaneer.go. Always populated once Enhanced Arsenal is
	// granted (unlike ArsenalModCap above, this isn't itself capped by
	// anything — designating IS the cap, one weapon at a time).
	EnhancedWeapon           *ninjaneerWeaponOption
	AvailableEnhancedWeapons []ninjaneerWeaponOption

	// WarriorOfScience is non-nil only once the character has the 9th-level
	// Warrior of Science feature — LegendaryWeapon/AvailableLegendaryWeapons
	// back the same single-slot designation shape as EnhancedWeapon above,
	// and LegendaryWeaponActive backs the feature's own CCD-funded
	// activation toggle (character.ninjaneer_legendary_weapon_active,
	// handleSheetNinjaneerLegendaryWeaponToggle) — the +2 attack roll/
	// doubled property/THP bypass/force-conversion effects that toggle
	// gates stay manual/narrated, only the spend-gate and rules text are
	// surfaced.
	WarriorOfScience *scienceNinWarriorOfScienceData

	// PerfectedWeaponMark/AvailablePerfectedWeaponMarks back A Weapon to
	// Surpass's own single-slot "which owned weapon is MY Perfected
	// Weapon" designation, for The Future of Shinobi: Weapons' own flat
	// +Intelligence-Modifier damage bonus — a different tracked quantity
	// from PerfectedWeaponCap/KnownPerfectedWeapons above (the count of
	// free Minor Arsenal Modifications the same feature also grants,
	// capped at Intelligence Modifier since the book allows multiple
	// Perfected Weapons at once). See cmd/n5e/ninjaneer.go.
	PerfectedWeaponMark           *ninjaneerWeaponOption
	AvailablePerfectedWeaponMarks []ninjaneerWeaponOption
}

// scienceNinWarriorOfScienceData backs Warrior of Science's own Legendary
// Weapon designation and its activation toggle.
type scienceNinWarriorOfScienceData struct {
	LegendaryWeapon           *ninjaneerWeaponOption
	AvailableLegendaryWeapons []ninjaneerWeaponOption
	Active                    bool
}

// scienceNinShinobiWareData backs Shinobi-Ware Upgrades, the Glorious
// Evolution overlay, and the In His Image overlay.
type scienceNinShinobiWareData struct {
	// UpgradeCap is Edge Runner's own Proficiency-Bonus slot count, plus
	// +Intelligence modifier once the 20th-level Future of Shinobi:
	// Shinobi-Ware capstone is granted ("You gain extra upgrade slots equal
	// to your Intelligence Modifier"). KnownUpgrades/AvailableUpgrades are
	// drawn from the Shinobi-Ware Upgrades catalog with its Shinjutsu tier
	// excluded — Edge Runner's own text grants "any upgrade, besides a
	// Shinjutsu"; that tier is offered only through In His Image below.
	UpgradeCap        int
	UpgradeUsed       int
	KnownUpgrades     []knownScienceNinPick
	AvailableUpgrades []scienceNinSubclassOption

	// Evolved is non-nil only once the character has the 6th-level
	// Glorious Evolution feature. AvailableEvolved is drawn from the SAME
	// Shinobi-Ware Upgrades catalog as above (not restricted to already-
	// known upgrades — Glorious Evolution evolves a fresh copy, independent
	// of a character's own owned Upgrade slots), filtered to whichever
	// tiers the character's own level has unlocked (see
	// gloriousEvolutionAllowedTiers). Cap is always 2 once unlocked.
	EvolvedCap       int
	EvolvedUsed      int
	KnownEvolved     []knownScienceNinPick
	AvailableEvolved []scienceNinSubclassOption

	// ShinjutsuUpgrade is non-nil only once the character has the 9th-level
	// In His Image feature AND has already made this permanent pick.
	// AvailableShinjutsuUpgrade is the Shinobi-Ware Upgrades catalog's own
	// Shinjutsu tier only. Unlike every other pick in this file, this one is
	// permanent ("You gain 1 Shinjutsu Upgrade the first time you activate
	// this ability. You can not change this later") — no delete route is
	// wired for it (see server.go). Only WHICH Shinjutsu Upgrade was chosen
	// is tracked — activating/maintaining the seal (20 CCD to start, 5
	// CCD/turn upkeep, 1-minute duration), the +2 AC/+1 crit-range clauses
	// while sealed, and the "only while sealed" restriction stay Group 2/3,
	// manual/narrated.
	ShinjutsuUpgrade          *knownScienceNinPick
	AvailableShinjutsuUpgrade []scienceNinSubclassOption

	// FullMetalShinobiResistances is every damage type the character
	// currently holds via Full-Metal Shinobi's own staged progression (two
	// player picks at 6th/9th level, the third automatic at 14th) — see
	// fullMetalShinobiResistances' own doc (full_metal_shinobi.go).
	FullMetalShinobiResistances []string
	// FullMetalShinobiSlots is every currently-open pick the player can
	// still make or change (6th/9th level), each excluding whatever's
	// already been picked in the OTHER slot.
	FullMetalShinobiSlots []fullMetalShinobiSlot

	// EverEvolvingSeal/AvailableEverEvolvingSeal back the 14th-level Ever
	// Evolving feature (ever_evolving.go): a single Armor Seal (cap 1,
	// "apply IT to your Full Metal Shinobi armor") instantly applied for a
	// Creation Points cost equal to its own rank, spent from the SAME
	// shared budget every other Science-Nin pick draws from — unlike every
	// other field in this struct, gated by data.CreationPointsUsed/Cap
	// rather than a flat slot count. Both non-nil/non-empty only once the
	// character has the Ever Evolving feature; AvailableEverEvolvingSeal
	// draws from the full D-through-S armor seal catalog with no tier cap
	// (contrast guardSealTierCap, martial_defense.go — Ever Evolving's own
	// text states no level-gated ceiling).
	EverEvolvingSeal          *everEvolvingSealOption
	AvailableEverEvolvingSeal []everEvolvingSealOption
}

// scienceNinSpywareData backs Spyware Programs and the Netrunner Quick Hack
// overlay.
type scienceNinSpywareData struct {
	ProgramCap        int
	ProgramUsed       int
	KnownPrograms     []knownScienceNinPick
	AvailablePrograms []scienceNinSubclassOption

	// QuickHack is non-nil only once the character has the 14th-level
	// Netrunner feature. AvailableQuickHack is restricted to the
	// character's own KnownPrograms (Netrunner selects an already-known
	// Program to make always-prepared, no exclusion clause the way Perma
	// Perk excludes Wonder E.I.P) — same "restricted to already-known"
	// shape Perma Perk already establishes.
	QuickHack          *knownScienceNinPick
	AvailableQuickHack []scienceNinSubclassOption
}

// scienceNinStormRiderData backs A.T. Enhancements and the King's Road
// Regalia overlay. The Air Trecks weapon itself isn't tracked here — it's
// granted straight into the character's own inventory (see
// ensureAirTrecksGranted, cmd/n5e/science_nin.go) and shows up in the main
// Attacks & Jutsu table like any other equipped weapon, not as a pick in
// this box.
type scienceNinStormRiderData struct {
	EnhancementCap        int
	EnhancementUsed       int
	KnownEnhancements     []knownScienceNinPick
	AvailableEnhancements []scienceNinSubclassOption

	// RegaliaCap/RegaliaUsed/Regalia/AvailableRegalia are non-zero/non-nil
	// only once the character has the 14th-level King's Road feature.
	// RegaliaCap is 1, or 2 once the 20th-level Future of Shinobi: Sky
	// Keeper capstone is also granted ("Choose another Regalia... can be
	// used in conjunction with the other"). AvailableRegalia is the "Air
	// Treck Enhancements" list's own Regalia row (8 named entries, unlike
	// every other tier not gated by a Cost stat line — see this file's own
	// header doc), never restricted to already-known Enhancements since
	// Regalia is an entirely independent 8-option catalog of its own. Sky
	// Keeper's own flat +60ft Speed bonus is a speedGrants entry
	// (internal/features/grants.go), not tracked here; its flying-
	// speed/hover and difficult-terrain clauses have no field anywhere in
	// this app to land in (no flying-speed or hover field exists) and stay
	// Group 3, same restraint already applied to the Air Trecks weapon-stat
	// bump clauses of this same capstone.
	RegaliaCap       int
	RegaliaUsed      int
	Regalia          []knownScienceNinPick
	AvailableRegalia []scienceNinSubclassOption
}

// scienceNinTechnobiData backs Technobi Mechanizations. The Shinobi
// Gauntlet (Kote)'s own scroll-storage/casting system (6th level) is a
// separate, larger mechanic deliberately left unbuilt — see
// CLASS_AUDIT.md's Science-Nin detail entry.
type scienceNinTechnobiData struct {
	MechanizationCap        int
	MechanizationUsed       int
	KnownMechanizations     []knownScienceNinPick
	AvailableMechanizations []scienceNinSubclassOption

	// SENTOptions/SENTCurrent back S.E.N.Ts' own freely re-pickable "which
	// ammo weapon is currently my S.E.N.T" choice — see sents.go. Nil
	// SENTOptions means the character hasn't reached S.E.N.Ts yet (3rd
	// level Technobi, same level as The Best Laid Trap).
	SENTOptions []featureChoiceOption
	SENTCurrent string
}

// scienceNinSNBSpecialistData backs S.N.B Upgrades — the only piece of
// S.N.B Specialist built so far. The Scientific Ninja Beast companion
// itself (stat block, Combat Programming, Improved Servos, Artificial
// Sentience, Regenerative Armor) and its Secondary C.C.D pool's own
// deposit/withdraw mechanic (the pool's SIZE is tracked as an ordinary
// customResourceGrants entry, cmd/n5e/custom_resources.go, but nothing
// models an actual companion to be "within 5 feet of") are deliberately
// left unbuilt — see CLASS_AUDIT.md's Science-Nin detail entry.
type scienceNinSNBSpecialistData struct {
	// UpgradeCap/UpgradeUsed are the flat Upgrade Slot count (Proficiency
	// Bonus). Each KnownUpgrades entry's own Cost also draws from the
	// SEPARATE, shared Creation Points budget (scienceNinToolsTabData's own
	// CreationPointsCap/Used) — a known upgrade can be blocked by either
	// running out first, same as Titan Slots/Creation Points for Mech
	// Crafter.
	UpgradeCap        int
	UpgradeUsed       int
	KnownUpgrades     []knownScienceNinPick
	AvailableUpgrades []scienceNinSubclassOption

	// PermanentCap/PermanentUsed/KnownPermanent/AvailablePermanent back the
	// 20th-level Future of Shinobi: S.N.B capstone: "choose 3 S.N.B.
	// Upgrades, your Scientific Ninja Beast permanently gains these
	// upgrades, and they no longer take an upgrade slot or cost creation
	// points." PermanentCap is always 3 once granted. AvailablePermanent is
	// restricted to the character's own KnownUpgrades — stored under its
	// own category (snb_upgrade_permanent), so these picks never count
	// against UpgradeUsed/UpgradeCap above. Prerequisite-checking ("you
	// must still meet the prerequisites") stays unenforced/manual, matching
	// this file's existing Prerequisite-is-informational-only restraint.
	PermanentCap       int
	PermanentUsed      int
	KnownPermanent     []knownScienceNinPick
	AvailablePermanent []scienceNinSubclassOption
}

// loadScienceNinSubclassData populates whichever of
// scienceNinToolsTabData's four subclass pointers apply, based on which
// granting features the character actually has. Called from
// loadScienceNinTabData once the base Scientific Ninja Tools budget is
// already resolved.
func (s *server) loadScienceNinSubclassData(characterID int64, sheet *charsheet.Sheet, level int, granted []grantedFeatureRow, data *scienceNinToolsTabData) error {
	has := make(map[string]bool, len(granted))
	for _, f := range granted {
		has[f.Slug] = true
	}
	proficiencyBonus := sheet.ProficiencyBonus
	intMod := sheet.Abilities["int"].Modifier

	if has[scienceNinEIPFeatureSlug] || has[scienceNinWOWFeatureSlug] || has[scienceNinPermaPerkFeatureSlug] {
		ei := &scienceNinElementalInnovationistData{}

		if has[scienceNinEIPFeatureSlug] {
			ei.EIPCap = eipCap(level)
			catalog, err := s.loadScienceNinEntryCatalog("E.I.Ps", false)
			if err != nil {
				return err
			}
			// E.I.Ps draw from the SAME shared Creation Points budget the base
			// Scientific Ninja Tools catalog and S.N.B Upgrades both spend
			// from — every E.I.P entry's own Cost is a flat single number
			// (confirmed against every entry in this catalog, never a RANGE
			// the way this file's own "informational only" catalogs are), the
			// same criterion that makes S.N.B Upgrades this file's other
			// exception (see this file's header doc). Perma Perk (below)
			// selects an already-known, already-paid-for E.I.P rather than
			// acquiring a new one, so it needs no exclusion here the way
			// Spyware's Quick Hack or S.N.B's permanent picks do.
			costBySlug := make(map[string]int, len(catalog))
			for _, o := range catalog {
				cost, _, _, _ := parseScienceNinToolStatLine(o.Description)
				costBySlug[o.Slug] = cost
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickEIP)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			ei.EIPUsed = len(picks)
			ei.KnownEIPs, ei.AvailableEIPs = splitScienceNinPicks(catalog, pickedSet)
			for i := range ei.KnownEIPs {
				ei.KnownEIPs[i].Cost = costBySlug[ei.KnownEIPs[i].Slug]
				data.CreationPointsUsed += ei.KnownEIPs[i].Cost
			}
		}

		if has[scienceNinWOWFeatureSlug] {
			ei.WOWCap = wowCap(level)
			catalog, err := s.loadScienceNinFlatCatalog("W.O.W (Weapons of Wonder)")
			if err != nil {
				return err
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickWOW)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			ei.WOWUsed = len(picks)
			ei.KnownWOW, ei.AvailableWOW = splitScienceNinPicks(catalog, pickedSet)

			if has[scienceNinElementalInnovationFeatureSlug] {
				designatedPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickAscendedWoW)
				if err != nil {
					return err
				}
				designatedSet := make(map[string]bool, len(designatedPicks))
				for _, p := range designatedPicks {
					designatedSet[p.OptionSlug] = true
				}
				descBySlug := scienceNinDescBySlug(catalog)
				for _, k := range ei.KnownWOW {
					if designatedSet[k.Slug] {
						pick := k
						ei.DesignatedWoW = &pick
						continue
					}
					ei.AvailableDesignatedWoW = append(ei.AvailableDesignatedWoW, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier, Description: descBySlug[k.Slug]})
				}
			}
		}

		if has[scienceNinPermaPerkFeatureSlug] {
			permaPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickPermaPerk)
			if err != nil {
				return err
			}
			permaSet := make(map[string]bool, len(permaPicks))
			for _, p := range permaPicks {
				permaSet[p.OptionSlug] = true
			}
			eipCatalog, err := s.loadScienceNinEntryCatalog("E.I.Ps", false)
			if err != nil {
				return err
			}
			descBySlug := scienceNinDescBySlug(eipCatalog)
			for _, k := range ei.KnownEIPs {
				if permaSet[k.Slug] {
					pick := k
					ei.PermaPerk = &pick
					continue
				}
				if strings.EqualFold(k.Name, "Wonder E.I.P") {
					continue // Perma Perk explicitly cannot be Wonder E.I.P
				}
				ei.AvailablePermaPerk = append(ei.AvailablePermaPerk, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier, Description: descBySlug[k.Slug]})
			}
		}

		data.ElementalInnovationist = ei
	}

	if has[scienceNinBIMFeatureSlug] {
		gr := &scienceNinGrenadierData{BIMCap: bimCap(proficiencyBonus)}
		catalog, err := s.loadScienceNinEntryCatalog("Explosive Modifications", false)
		if err != nil {
			return err
		}
		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickBIM)
		if err != nil {
			return err
		}
		for _, p := range picks {
			gr.BIMUsed += p.Quantity
		}
		gr.KnownBIM, gr.AvailableBIM = splitScienceNinBIMPicks(catalog, picks)

		if has[scienceNinBIMSpecialistFeatureSlug] {
			designatedPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickBIMSpecialist)
			if err != nil {
				return err
			}
			designatedSet := make(map[string]bool, len(designatedPicks))
			for _, p := range designatedPicks {
				designatedSet[p.OptionSlug] = true
			}
			descBySlug := scienceNinDescBySlug(catalog)
			for _, k := range gr.KnownBIM {
				if designatedSet[k.Slug] {
					pick := k
					gr.DesignatedBIM = &pick
					continue
				}
				gr.AvailableDesignatedBIM = append(gr.AvailableDesignatedBIM, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier, Description: descBySlug[k.Slug]})
			}
		}

		data.Grenadier = gr
	}

	if has[scienceNinInversionSerumsFeatureSlug] {
		ms := &scienceNinMadScientistData{SerumCap: inversionSerumCap(intMod)}
		catalog, err := s.loadScienceNinEntryCatalog("Inversion Serums", true)
		if err != nil {
			return err
		}
		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickInversionSerum)
		if err != nil {
			return err
		}
		pickedPool := make(map[string]string, len(picks))
		for _, p := range picks {
			pickedPool[p.OptionSlug] = p.Pool
		}
		ms.SerumUsed = len(picks)
		fixedPoolBySlug := make(map[string]string, len(catalog))
		for _, o := range catalog {
			fixedPoolBySlug[o.Slug] = o.FixedPool
			if pool, ok := pickedPool[o.Slug]; ok {
				ms.KnownSerums = append(ms.KnownSerums, knownScienceNinPick{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Pool: pool})
				continue
			}
			ms.AvailableSerums = append(ms.AvailableSerums, o)
		}

		if has[scienceNinSheepAndShepherdFeatureSlug] {
			designatedPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickSheepAndShepherdSerum)
			if err != nil {
				return err
			}
			designatedSet := make(map[string]bool, len(designatedPicks))
			for _, p := range designatedPicks {
				designatedSet[p.OptionSlug] = true
			}
			descBySlug := scienceNinDescBySlug(catalog)
			for _, k := range ms.KnownSerums {
				if fixedPoolBySlug[k.Slug] != "" {
					continue // The Sheep and the Shepherd requires a dual-effect Serum (FixedPool == "")
				}
				if designatedSet[k.Slug] {
					pick := k
					ms.DesignatedSerum = &pick
					continue
				}
				ms.AvailableDesignatedSerum = append(ms.AvailableDesignatedSerum, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier, Description: descBySlug[k.Slug]})
			}
		}

		data.MadScientist = ms
	}

	if has[scienceNinArsenalFeatureSlug] || has[scienceNinPerfectedWeaponFeatureSlug] {
		nj := &scienceNinNinjaneerData{}
		catalog, err := s.loadScienceNinEntryCatalog("Arsenal Modifications", false)
		if err != nil {
			return err
		}

		// candidateWeapons feeds every one of this subclass's own weapon
		// designations below (Enhanced/Legendary/Perfected) — loaded once
		// per request rather than per tier, since all three draw from the
		// same "character's own weapon-kind inventory rows" set.
		candidateWeapons, err := s.loadNinjaneerCandidateWeapons(characterID)
		if err != nil {
			return err
		}

		if has[scienceNinArsenalFeatureSlug] {
			nj.ArsenalModCap = arsenalModCap(proficiencyBonus)
			if has[scienceNinFutureOfShinobiWeaponsFeatureSlug] {
				// "You gain additional upgrade slots for your Enhanced
				// Arsenal equal to your Intelligence modifier." The same
				// capstone's own flat proficiency-bonus/2x-proficiency/
				// Int-mod damage-roll riders on Enhanced/Legendary/
				// Perfected Weapons are applied automatically via
				// recomputeNinjaneerWeaponOverrides (ninjaneer.go), not
				// here.
				nj.ArsenalModCap += nonNegativeIntMod(intMod)
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickArsenalMod)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			nj.ArsenalModUsed = len(picks)
			nj.KnownArsenalMods, nj.AvailableArsenalMods = splitScienceNinPicks(catalog, pickedSet)

			nj.EnhancedWeapon, nj.AvailableEnhancedWeapons, err = s.ninjaneerDesignatedWeapon(characterID, charstore.ScienceNinPickEnhancedWeapon, candidateWeapons)
			if err != nil {
				return err
			}
		}

		if has[scienceNinWarriorOfScienceFeatureSlug] {
			active, err := charstore.NinjaneerLegendaryWeaponActive(s.charDB, characterID)
			if err != nil {
				return err
			}
			ws := &scienceNinWarriorOfScienceData{Active: active}
			ws.LegendaryWeapon, ws.AvailableLegendaryWeapons, err = s.ninjaneerDesignatedWeapon(characterID, charstore.ScienceNinPickLegendaryWeapon, candidateWeapons)
			if err != nil {
				return err
			}
			nj.WarriorOfScience = ws
		}

		if has[scienceNinPerfectedWeaponFeatureSlug] {
			nj.PerfectedWeaponCap = perfectedWeaponCap(intMod)
			var minorOnly []scienceNinSubclassOption
			for _, o := range catalog {
				if strings.EqualFold(o.Tier, "Minor") {
					minorOnly = append(minorOnly, o)
				}
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickPerfectedWeapon)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			nj.PerfectedWeaponUsed = len(picks)
			nj.KnownPerfectedWeapons, nj.AvailablePerfectedWeapons = splitScienceNinPicks(minorOnly, pickedSet)

			nj.PerfectedWeaponMark, nj.AvailablePerfectedWeaponMarks, err = s.ninjaneerDesignatedWeapon(characterID, charstore.ScienceNinPickPerfectedWeaponMark, candidateWeapons)
			if err != nil {
				return err
			}
		}

		data.Ninjaneer = nj
	}

	if has[scienceNinEdgeRunnerFeatureSlug] || has[scienceNinGloriousEvolutionFeatureSlug] || has[scienceNinEverEvolvingFeatureSlug] {
		sw := &scienceNinShinobiWareData{}
		catalog, err := s.loadScienceNinEntryCatalog("Shinobi-Ware Upgrades", false)
		if err != nil {
			return err
		}

		if has[scienceNinEdgeRunnerFeatureSlug] {
			sw.UpgradeCap = shinobiWareUpgradeCap(proficiencyBonus)
			if has[scienceNinFutureOfShinobiShinobiWareFeatureSlug] {
				// "You gain extra upgrade slots equal to your Intelligence
				// Modifier." The same capstone's own doubling of the base
				// Creation Points budget is applied in loadScienceNinTabData
				// (science_nin.go), not here.
				sw.UpgradeCap += nonNegativeIntMod(intMod)
			}
			// Edge Runner's own text grants "any upgrade, besides a
			// Shinjutsu" — exclude the Shinjutsu tier here so it's only ever
			// offered through In His Image below, mirroring Glorious
			// Evolution's own allowedTiers filter a few lines down.
			var edgeRunnerCatalog []scienceNinSubclassOption
			for _, o := range catalog {
				if !strings.EqualFold(o.Tier, "Shinjutsu") {
					edgeRunnerCatalog = append(edgeRunnerCatalog, o)
				}
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickShinobiWareUpgrade)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sw.UpgradeUsed = len(picks)
			sw.KnownUpgrades, sw.AvailableUpgrades = splitScienceNinPicks(edgeRunnerCatalog, pickedSet)

			if has[scienceNinInHisImageFeatureSlug] {
				var shinjutsuOnly []scienceNinSubclassOption
				for _, o := range catalog {
					if strings.EqualFold(o.Tier, "Shinjutsu") {
						shinjutsuOnly = append(shinjutsuOnly, o)
					}
				}
				shinjutsuPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickShinjutsuUpgrade)
				if err != nil {
					return err
				}
				pickedShinjutsu := make(map[string]bool, len(shinjutsuPicks))
				for _, p := range shinjutsuPicks {
					pickedShinjutsu[p.OptionSlug] = true
				}
				for _, o := range shinjutsuOnly {
					if pickedShinjutsu[o.Slug] {
						pick := knownScienceNinPick{Slug: o.Slug, Name: o.Name, Tier: o.Tier}
						sw.ShinjutsuUpgrade = &pick
						continue
					}
					sw.AvailableShinjutsuUpgrade = append(sw.AvailableShinjutsuUpgrade, o)
				}
			}
		}

		if has[scienceNinGloriousEvolutionFeatureSlug] {
			sw.EvolvedCap = 2
			allowedTiers := gloriousEvolutionAllowedTiers(level)
			var eligible []scienceNinSubclassOption
			for _, o := range catalog {
				if allowedTiers[strings.ToLower(o.Tier)] {
					eligible = append(eligible, o)
				}
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickEvolvedUpgrade)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sw.EvolvedUsed = len(picks)
			sw.KnownEvolved, sw.AvailableEvolved = splitScienceNinPicks(eligible, pickedSet)
		}

		if has[scienceNinFullMetalShinobiFeatureSlug] {
			resistancePicks, err := charstore.ListFullMetalShinobiResistances(s.charDB, characterID)
			if err != nil {
				return err
			}
			sw.FullMetalShinobiResistances = fullMetalShinobiResistances(level, resistancePicks)
			sw.FullMetalShinobiSlots = fullMetalShinobiSlots(level, resistancePicks)
		}

		if has[scienceNinEverEvolvingFeatureSlug] {
			sealCatalog, err := s.loadEverEvolvingSealCatalog()
			if err != nil {
				return err
			}
			sealPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickEverEvolvingSeal)
			if err != nil {
				return err
			}
			pickedSeal := make(map[string]bool, len(sealPicks))
			for _, p := range sealPicks {
				pickedSeal[p.OptionSlug] = true
			}
			for i, o := range sealCatalog {
				if pickedSeal[o.Slug] {
					sw.EverEvolvingSeal = &sealCatalog[i]
					// Spent from the SAME shared Creation Points budget the
					// base Scientific Ninja Tools loop (loadScienceNinTabData)
					// adds its own picks' cost into — order between the two
					// loops doesn't matter, since both write into the same
					// running total.
					data.CreationPointsUsed += o.Cost
					continue
				}
				sw.AvailableEverEvolvingSeal = append(sw.AvailableEverEvolvingSeal, o)
			}
		}

		data.ShinobiWare = sw
	}

	if has[scienceNinCruelAngelsThesisFeatureSlug] || has[scienceNinNetrunnerFeatureSlug] {
		sp := &scienceNinSpywareData{}
		quickHackSet := map[string]bool{}

		if has[scienceNinCruelAngelsThesisFeatureSlug] {
			sp.ProgramCap = spywareProgramCap(proficiencyBonus)
			catalog, err := s.loadScienceNinEntryCatalog("Spyware Programs", false)
			if err != nil {
				return err
			}
			// Spyware Programs draw from the SAME shared Creation Points
			// budget S.N.B Upgrades and E.I.Ps both spend from — Cruel
			// Angel's Thesis states this explicitly ("Over the course of a
			// long rest, you can spend Creation Points to make Programs"),
			// and every entry's own Cost is a flat single number (never a
			// RANGE), the same criterion that makes S.N.B Upgrades this
			// file's other exception (see this file's header doc).
			costBySlug := make(map[string]int, len(catalog))
			for _, o := range catalog {
				cost, _, _, _ := parseScienceNinToolStatLine(o.Description)
				costBySlug[o.Slug] = cost
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickSpywareProgram)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sp.KnownPrograms, sp.AvailablePrograms = splitScienceNinPicks(catalog, pickedSet)
			for i := range sp.KnownPrograms {
				sp.KnownPrograms[i].Cost = costBySlug[sp.KnownPrograms[i].Slug]
			}
		}

		if has[scienceNinNetrunnerFeatureSlug] {
			quickHackPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickQuickHack)
			if err != nil {
				return err
			}
			for _, p := range quickHackPicks {
				quickHackSet[p.OptionSlug] = true
			}
			programCatalog, err := s.loadScienceNinEntryCatalog("Spyware Programs", false)
			if err != nil {
				return err
			}
			descBySlug := scienceNinDescBySlug(programCatalog)
			for _, k := range sp.KnownPrograms {
				if quickHackSet[k.Slug] {
					pick := k
					sp.QuickHack = &pick
					continue
				}
				sp.AvailableQuickHack = append(sp.AvailableQuickHack, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier, Description: descBySlug[k.Slug]})
			}
		}

		// Program slot usage and Creation Points spend both exclude whichever
		// Known Program has been designated the Netrunner's Quick Hack —
		// Netrunner's own text: "You always have this program prepared, and
		// it does not count against your slot limit or Creation points."
		// Computed after both blocks above so quickHackSet is complete
		// regardless of which feature(s) the character actually has.
		for _, k := range sp.KnownPrograms {
			if quickHackSet[k.Slug] {
				continue
			}
			sp.ProgramUsed++
			data.CreationPointsUsed += k.Cost
		}

		data.Spyware = sp
	}

	if has[scienceNinAirTrecksFeatureSlug] || has[scienceNinKingsRoadFeatureSlug] {
		sr := &scienceNinStormRiderData{}

		if has[scienceNinAirTrecksFeatureSlug] {
			if err := s.ensureAirTrecksGranted(characterID); err != nil {
				return err
			}
			sr.EnhancementCap = airTreckEnhancementCap(proficiencyBonus)
			// "Air Treck Enhancements" also bundles a Regalia tier (King's
			// Road below) in the same class_options list — confirmed live
			// against a freshly re-ingested rules.db that this tier's own
			// row DOES get split into class_option_entries after all (its 8
			// named Regalia types are each their own ALL-CAPS header, which
			// is enough for textentries.FindEntries' 2+-match threshold on
			// its own; a "Cost:" stat line was never required for a split
			// to happen, only assumed to correlate with one). Left
			// unfiltered, those 8 rows show up as extra slot-costed A.T.
			// Enhancement picks alongside the correct, separate King's Road
			// section below — filtered out here by tier so this catalog
			// only ever offers Minor through Mastercraft.
			catalog, err := s.loadScienceNinEntryCatalog("Air Treck Enhancements", false)
			if err != nil {
				return err
			}
			var enhancementsOnly []scienceNinSubclassOption
			for _, o := range catalog {
				if !strings.EqualFold(o.Tier, "Regalia") {
					enhancementsOnly = append(enhancementsOnly, o)
				}
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickAirTreckEnhancement)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sr.EnhancementUsed = len(picks)
			sr.KnownEnhancements, sr.AvailableEnhancements = splitScienceNinPicks(enhancementsOnly, pickedSet)
		}

		if has[scienceNinKingsRoadFeatureSlug] {
			sr.RegaliaCap = 1
			if has[scienceNinSkyKeeperFeatureSlug] {
				// "Choose another Regalia... can be used in conjunction
				// with the other." Sky Keeper's own flat +60ft Speed bonus
				// is a speedGrants entry (internal/features/grants.go), not
				// tracked here.
				sr.RegaliaCap = 2
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickRegalia)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sr.Regalia, sr.AvailableRegalia = splitScienceNinPicks(scienceNinRegaliaOptions, pickedSet)
			sr.RegaliaUsed = len(sr.Regalia)
		}

		data.StormRider = sr
	}

	if has[scienceNinBestLaidTrapFeatureSlug] {
		tb := &scienceNinTechnobiData{MechanizationCap: technobiMechanizationCap(proficiencyBonus)}
		catalog, err := s.loadScienceNinEntryCatalog("Technobi Mechanizations", false)
		if err != nil {
			return err
		}
		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickTechnobiMechanization)
		if err != nil {
			return err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, p := range picks {
			pickedSet[p.OptionSlug] = true
		}
		tb.MechanizationUsed = len(picks)
		tb.KnownMechanizations, tb.AvailableMechanizations = splitScienceNinPicks(catalog, pickedSet)

		if has[scienceNinSENTsFeatureSlug] {
			sentCatalog, err := s.loadSENTWeaponCatalog()
			if err != nil {
				return err
			}
			choices, err := features.LoadFeatureChoices(s.charDB, characterID)
			if err != nil {
				return err
			}
			tb.SENTOptions = sentCatalog
			tb.SENTCurrent = choices[sentChoiceKey]
		}

		data.Technobi = tb
	}

	if has[scienceNinSNBUpgradesFeatureSlug] {
		snb := &scienceNinSNBSpecialistData{UpgradeCap: snbUpgradeCap(proficiencyBonus)}
		catalog, err := s.loadScienceNinEntryCatalog("S.N.B Upgrades", false)
		if err != nil {
			return err
		}
		// Parsed with the identical stat-line pattern the base Scientific
		// Ninja Tools catalog uses (parseScienceNinToolStatLine,
		// science_nin.go) — both catalogs' own entries share the exact same
		// "(Prerequisite: ...)? Cost: N Creation Points (Drain: N CCD
		// Chakra)?" text shape, confirmed against every entry in this
		// catalog. Built once here and reused below both for each Known
		// pick's own display Cost and for the running Creation Points total.
		costBySlug := make(map[string]int, len(catalog))
		for _, o := range catalog {
			cost, _, _, _ := parseScienceNinToolStatLine(o.Description)
			costBySlug[o.Slug] = cost
		}

		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickSNBUpgrade)
		if err != nil {
			return err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, p := range picks {
			pickedSet[p.OptionSlug] = true
		}
		snb.UpgradeUsed = len(picks)
		snb.KnownUpgrades, snb.AvailableUpgrades = splitScienceNinPicks(catalog, pickedSet)
		for i := range snb.KnownUpgrades {
			snb.KnownUpgrades[i].Cost = costBySlug[snb.KnownUpgrades[i].Slug]
		}

		permanentSlugs := map[string]bool{}
		if has[scienceNinFutureOfShinobiSNBFeatureSlug] {
			snb.PermanentCap = 3
			permPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickSNBUpgradePermanent)
			if err != nil {
				return err
			}
			for _, p := range permPicks {
				permanentSlugs[p.OptionSlug] = true
			}
			descBySlug := scienceNinDescBySlug(catalog)
			for _, k := range snb.KnownUpgrades {
				if permanentSlugs[k.Slug] {
					pick := k
					snb.KnownPermanent = append(snb.KnownPermanent, pick)
					continue
				}
				snb.AvailablePermanent = append(snb.AvailablePermanent, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier, Description: descBySlug[k.Slug]})
			}
			snb.PermanentUsed = len(snb.KnownPermanent)
		}

		// S.N.B Upgrades draw from the SAME shared Creation Points budget
		// the base Scientific Ninja Tools loop (loadScienceNinTabData) and,
		// for Mech Crafter, Titan Upgrades both spend from — see this
		// file's own header doc on why S.N.B Upgrades is the one exception
		// among this file's catalogs to "Cost is informational only".
		// Future of Shinobi: S.N.B's own permanent picks are excluded
		// ("they no longer take an upgrade slot or cost creation points").
		for _, k := range snb.KnownUpgrades {
			if permanentSlugs[k.Slug] {
				continue
			}
			data.CreationPointsUsed += k.Cost
		}

		data.SNBSpecialist = snb
	}

	return nil
}

// addSNBUpgradePick validates and installs one S.N.B Upgrade, gated by BOTH
// the flat Upgrade Slot cap (UpgradeCap/Used) and the shared Creation Points
// budget — unlike every other catalog in this file (see this file's own
// header doc on why those stay slot-only), S.N.B Upgrades draw from the
// SAME Creation Points pool the base Scientific Ninja Tools catalog and,
// for Mech Crafter, Titan Upgrades both spend from, the same shape
// handleTitanUpgradeAdd (titan.go) already established for its own sibling
// pool. Returns (http.StatusOK, "") on success, or the status/message an
// http.Error should report on failure. Shared by the Core-sheet's own AJAX
// route (handleScienceNinSNBUpgradeAdd, just below) and the S.N.B Upgrades
// popup's plain POST route (handleSNBUpgradesAdd, snb_upgrades_popup.go),
// which differ only in how they turn this outcome into an HTTP response —
// extracted so the two entry points can never drift apart on which
// upgrades a budget/slot check actually allows.
func (s *server) addSNBUpgradePick(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing upgrade"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for snb upgrade add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		log.Println("load science-nin for snb upgrade add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.SNBSpecialist == nil {
		return http.StatusBadRequest, "character has no S.N.B Upgrades yet"
	}
	snb := data.SNBSpecialist
	if snb.UpgradeUsed >= snb.UpgradeCap {
		return http.StatusBadRequest, "no S.N.B Upgrade slots remaining"
	}
	var picked *scienceNinSubclassOption
	for i, o := range snb.AvailableUpgrades {
		if o.Slug == slug {
			picked = &snb.AvailableUpgrades[i]
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid upgrade"
	}
	cost, _, _, _ := parseScienceNinToolStatLine(picked.Description)
	if data.CreationPointsUsed+cost > data.CreationPointsCap {
		return http.StatusBadRequest, "not enough Creation Points remaining"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickSNBUpgrade, slug, ""); err != nil {
		log.Println("add snb upgrade:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleScienceNinSNBUpgradeAdd is the Core-sheet's own AJAX route for
// addSNBUpgradePick (above) — not built on the generic
// handleScienceNinSubclassPickAdd factory below, since that factory's
// signature only ever checks a flat slot count, and threading a second,
// catalog-specific Creation-Points check through it would complicate the
// eighteen other call sites that must stay slot-only.
// handleScienceNinSubclassPickDelete (below) is still used for S.N.B
// Upgrade removal — freeing a slot never needs a budget check.
func (s *server) handleScienceNinSNBUpgradeAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addSNBUpgradePick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// addSpywareProgramPick validates and installs one Spyware Program, gated by
// BOTH the flat Program hold cap (ProgramCap/Used) and the shared Creation
// Points budget — same dual-check shape addSNBUpgradePick establishes above,
// for the identical reason (Cruel Angel's Thesis explicitly spends Creation
// Points to make Programs; see this file's header doc). Shared by the
// Core-sheet's own AJAX route (handleScienceNinSpywareProgramAdd, just
// below) and the Spyware popup's own redirect route
// (handleSpywareProgramPopupAdd), matching addSNBUpgradePick/
// addEIPPick's own split.
func (s *server) addSpywareProgramPick(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing program"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for spyware program add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		log.Println("load science-nin for spyware program add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.Spyware == nil {
		return http.StatusBadRequest, "character has no Spyware Programs yet"
	}
	sp := data.Spyware
	if sp.ProgramUsed >= sp.ProgramCap {
		return http.StatusBadRequest, "no Program slots remaining"
	}
	var picked *scienceNinSubclassOption
	for i, o := range sp.AvailablePrograms {
		if o.Slug == slug {
			picked = &sp.AvailablePrograms[i]
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid program"
	}
	cost, _, _, _ := parseScienceNinToolStatLine(picked.Description)
	if data.CreationPointsUsed+cost > data.CreationPointsCap {
		return http.StatusBadRequest, "not enough Creation Points remaining"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickSpywareProgram, slug, ""); err != nil {
		log.Println("add spyware program:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleScienceNinSpywareProgramAdd is addSpywareProgramPick's own
// Core-sheet AJAX route — not built on the generic
// handleScienceNinSubclassPickAdd factory below, for the same reason
// handleScienceNinSNBUpgradeAdd isn't. handleScienceNinSubclassPickDelete
// (below) is still used for Spyware Program removal — freeing a slot never
// needs a budget check, and data.CreationPointsUsed is always recomputed
// from scratch from whichever picks remain, so no separate refund step is
// needed either.
func (s *server) handleScienceNinSpywareProgramAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addSpywareProgramPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleSpywareProgramPopupAdd is addSpywareProgramPick's own redirect-based
// wrapper for the Spyware popup, matching handleEverEvolvingSealPopupAdd's
// own shape (ever_evolving.go).
func (s *server) handleSpywareProgramPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addSpywareProgramPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, spywarePopupPath(id), http.StatusSeeOther)
}

// addEIPPick validates and installs one E.I.P, gated by BOTH the flat
// Exoskeleton slot cap (EIPCap/Used) and the shared Creation Points budget —
// same dual-check shape addSNBUpgradePick/addSpywareProgramPick establish
// above, for the identical reason (see this file's header doc on why E.I.Ps
// are this file's third exception to "informational only"). Shared by the
// Core-sheet's own AJAX route (handleScienceNinEIPAdd, just below) and the
// Elemental Innovationist popup's own redirect route (handleEIPPopupAdd).
func (s *server) addEIPPick(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing perk"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for eip add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		log.Println("load science-nin for eip add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.ElementalInnovationist == nil {
		return http.StatusBadRequest, "character has no E.I.Ps yet"
	}
	ei := data.ElementalInnovationist
	if ei.EIPUsed >= ei.EIPCap {
		return http.StatusBadRequest, "no Exoskeleton slots remaining"
	}
	var picked *scienceNinSubclassOption
	for i, o := range ei.AvailableEIPs {
		if o.Slug == slug {
			picked = &ei.AvailableEIPs[i]
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid perk"
	}
	cost, _, _, _ := parseScienceNinToolStatLine(picked.Description)
	if data.CreationPointsUsed+cost > data.CreationPointsCap {
		return http.StatusBadRequest, "not enough Creation Points remaining"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickEIP, slug, ""); err != nil {
		log.Println("add eip:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleScienceNinEIPAdd is addEIPPick's own Core-sheet AJAX route — not
// built on the generic handleScienceNinSubclassPickAdd factory below, for
// the same reason handleScienceNinSNBUpgradeAdd isn't.
// handleScienceNinSubclassPickDelete (below) is still used for E.I.P
// removal — freeing a slot never needs a budget check.
func (s *server) handleScienceNinEIPAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addEIPPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleEIPPopupAdd is addEIPPick's own redirect-based wrapper for the
// Elemental Innovationist popup, matching handleEverEvolvingSealPopupAdd's
// own shape (ever_evolving.go).
func (s *server) handleEIPPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addEIPPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, elementalInnovationistPopupPath(id), http.StatusSeeOther)
}

// removeEIPPick clears one known E.I.P — freely, at any time, same "trust
// the player" boundary every other pick removal in this codebase draws. A
// forgotten E.I.P can no longer be the character's designated Perma Perk
// either, so a matching Perma Perk pick is cleared alongside it rather than
// left dangling — same shape removeWoWPick (science_nin_elemental_
// innovationist_popup.go) already draws for a forgotten W.o.W's Ascended
// designation. Without this, the stale Perma Perk row survives (its own
// slug no longer among Known EIPs), and while the popup's own display
// already hides it (see loadScienceNinSubclassData's KnownEIPs-gated
// PermaPerk lookup), that same gap lets scienceNinSubclassPickAddCore's cap
// check see PermaPerk as empty and accept a second Perma Perk pick — leaving
// two rows in the perma_perk category and making which one's stat effects
// apply (resolveScienceNinPermaPerk, scienceNinHasPermaPerk) depend on
// insertion order rather than the player's actual, current designation.
func (s *server) removeEIPPick(id int64, slug string) error {
	if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickEIP, slug); err != nil {
		return err
	}
	permaPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickPermaPerk)
	if err != nil {
		return err
	}
	for _, p := range permaPicks {
		if p.OptionSlug == slug {
			if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickPermaPerk, slug); err != nil {
				return err
			}
		}
	}
	return nil
}

// handleScienceNinEIPDelete is removeEIPPick's own Core-sheet AJAX wrapper,
// matching handleScienceNinWOWDelete's shape (science_nin_elemental_
// innovationist_popup.go) — not the generic handleScienceNinSubclassPickDelete
// below, since forgetting an E.I.P also needs to clear a dangling Perma Perk
// designation, unlike every other flat category that factory still covers.
func (s *server) handleScienceNinEIPDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := s.removeEIPPick(id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove eip pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleEIPPopupDelete is removeEIPPick's own redirect-based wrapper for the
// Elemental Innovationist popup, matching handleWoWPopupDelete's shape.
func (s *server) handleEIPPopupDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := s.removeEIPPick(id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove eip pick:", err)
		return
	}
	http.Redirect(w, r, elementalInnovationistPopupPath(id), http.StatusSeeOther)
}

// handleScienceNinSubclassPickAdd builds one category's "learn a pick"
// route — shared by all eighteen of this file's remaining catalogs (S.N.B
// Upgrades' own ADD route now uses the dedicated handleScienceNinSNBUpgradeAdd
// above; its DELETE route still uses this file's own
// handleScienceNinSubclassPickDelete below), differing only in which of
// scienceNinToolsTabData's own fields govern the cap/current-count and
// currently-pickable list, same factory shape
// hunter_nin.go's handleHunterPickAdd already establishes. requiresPool is
// Inversion Serums only: the submitted option_slug is expected in
// "<slug>|<pool>" form (see sheet_science_nin's own Available Serums
// markup, which renders a separate radio row per pool for a dual-effect
// serum), and the pool half is validated against the picked option's own
// FixedPool before being stored.
func (s *server) handleScienceNinSubclassPickAdd(
	category charstore.ScienceNinSubclassPickCategory,
	used func(*scienceNinToolsTabData) int,
	cap func(*scienceNinToolsTabData) int,
	available func(*scienceNinToolsTabData) []scienceNinSubclassOption,
	requiresPool bool,
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
		if status, msg := s.scienceNinSubclassPickAddCore(id, category, used, cap, available, requiresPool, slug); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		s.respondSheet(w, r, id, "sheet_science_nin")
	}
}

// scienceNinTrackerPopupAdd is scienceNinSubclassPickAddCore's own
// popup-flavored wrapper — same validation, redirecting back to a subclass
// tracker popup's own URL instead of swapping the Core sheet's
// sheet_science_nin fragment. Shared by every flat-slot-only Science-Nin
// subclass tracker popup this pattern's other files build (see
// subclass_tracker_popup.go's header doc for the pattern), the popup
// equivalent of handleScienceNinSubclassPickAdd immediately above.
func (s *server) scienceNinTrackerPopupAdd(
	category charstore.ScienceNinSubclassPickCategory,
	used func(*scienceNinToolsTabData) int,
	cap func(*scienceNinToolsTabData) int,
	available func(*scienceNinToolsTabData) []scienceNinSubclassOption,
	requiresPool bool,
	popupPath func(int64) string,
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
		if status, msg := s.scienceNinSubclassPickAddCore(id, category, used, cap, available, requiresPool, slug); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		http.Redirect(w, r, popupPath(id), http.StatusSeeOther)
	}
}

// scienceNinSubclassPickAddCore validates and stores one category's pick —
// the shared validate+mutate path handleScienceNinSubclassPickAdd's
// Core-sheet AJAX route and scienceNinTrackerPopupAdd's popup route both
// call, so the two response shapes (fragment swap vs. redirect) can never
// drift apart on what they actually validate.
func (s *server) scienceNinSubclassPickAddCore(
	id int64,
	category charstore.ScienceNinSubclassPickCategory,
	used func(*scienceNinToolsTabData) int,
	cap func(*scienceNinToolsTabData) int,
	available func(*scienceNinToolsTabData) []scienceNinSubclassOption,
	requiresPool bool,
	rawSlug string,
) (int, string) {
	slug := rawSlug
	if slug == "" {
		return http.StatusBadRequest, "missing pick"
	}
	pool := ""
	if requiresPool {
		parts := strings.SplitN(slug, "|", 2)
		if len(parts) != 2 {
			return http.StatusBadRequest, "missing pool"
		}
		slug, pool = parts[0], parts[1]
		if pool != "mending" && pool != "maiming" {
			return http.StatusBadRequest, "invalid pool"
		}
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for science-nin subclass pick add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		log.Println("load science-nin for subclass pick add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil {
		return http.StatusBadRequest, "character has no levels in Science-Nin"
	}
	if used(data) >= cap(data) {
		return http.StatusBadRequest, "no slots remaining"
	}
	var picked *scienceNinSubclassOption
	for _, o := range available(data) {
		if o.Slug == slug {
			picked = &o
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid pick"
	}
	if requiresPool && picked.FixedPool != "" && picked.FixedPool != pool {
		return http.StatusBadRequest, "this serum can only be paid from its own CCD half"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, category, slug, pool); err != nil {
		log.Println("add science-nin subclass pick:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleScienceNinSubclassPickDelete builds one category's "forget a pick"
// route — freely, at any time, same "trust the player" boundary every
// other pick removal in this codebase draws.
func (s *server) handleScienceNinSubclassPickDelete(category charstore.ScienceNinSubclassPickCategory) http.HandlerFunc {
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
			log.Println("remove science-nin subclass pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_science_nin")
	}
}

// handleScienceNinSubclassPickDetail serves the click-to-open popup for a
// Known pick in any of this file's nineteen catalogs — same
// hunter_pick_detail_card mechanism handleHunterPickDetail/
// handleScienceNinToolDetail already use. Not character-scoped — every one
// of these catalogs is static rules content.
func (s *server) handleScienceNinSubclassPickDetail(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	slug := r.PathValue("slug")

	// Regalia is hand-curated (scienceNinRegaliaOptions), not backed by any
	// class_options/class_option_entries row — see that catalog's own doc
	// for why. Looked up in memory instead of the two DB shapes every
	// other category below uses.
	if category == "regalia" {
		for _, o := range scienceNinRegaliaOptions {
			if o.Slug == slug {
				s.renderScienceNinSubclassPickDetail(w, o.Name, o.Description)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	// Mixed Studies is also hand-curated (mixedStudiesInquiryOptions,
	// science_nin.go) — slug here is a subclasses table slug, not a
	// class_options/class_option_entries one, so it needs its own lookup
	// too. Description is built live from both of the picked Inquiry's own
	// 3rd Level features (mixedStudiesInquiryDescription), the same text
	// the picker's own tooltip already shows.
	if category == "mixed-studies" {
		for _, o := range mixedStudiesInquiryOptions {
			if o.SubclassSlug == slug {
				s.renderScienceNinSubclassPickDetail(w, o.Name, s.mixedStudiesInquiryDescription(o))
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	var listName string
	var flat bool
	switch category {
	case "eip":
		listName = "E.I.Ps"
	case "wow":
		listName = "W.O.W (Weapons of Wonder)"
		flat = true
	case "bim":
		listName = "Explosive Modifications"
	case "inversion-serum":
		listName = "Inversion Serums"
	case "arsenal-mod", "perfected-weapon":
		listName = "Arsenal Modifications"
	case "shinobi-ware-upgrade", "evolved-upgrade":
		listName = "Shinobi-Ware Upgrades"
	case "spyware-program", "quick-hack":
		listName = "Spyware Programs"
	case "air-treck-enhancement":
		listName = "Air Treck Enhancements"
	case "technobi-mechanization":
		listName = "Technobi Mechanizations"
	case "snb-upgrade":
		listName = "S.N.B Upgrades"
	default:
		http.NotFound(w, r)
		return
	}

	var name, description string
	var err error
	if flat {
		err = s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = ?`,
			slug, scienceNinSlug, listName,
		).Scan(&name, &description)
	} else {
		err = s.rulesDB.QueryRow(`
			SELECT e.name, e.description FROM class_option_entries e
			JOIN class_options o ON o.slug = e.class_option_slug
			WHERE e.slug = ? AND o.class_slug = ? AND o.list_name = ?`,
			slug, scienceNinSlug, listName,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			// slug may point at a childless tier's own class_options row
			// (e.g. Mastercraft's single bundled item) rather than a
			// class_option_entries row — see
			// loadScienceNinChildlessTierOptions' own doc for why that
			// shape exists and carries the class_options slug directly as
			// Slug instead of a split entry's.
			var tier string
			tierErr := s.rulesDB.QueryRow(`
				SELECT o.name, o.description FROM class_options o
				WHERE o.slug = ? AND o.class_slug = ? AND o.list_name = ?
				  AND NOT EXISTS (SELECT 1 FROM class_option_entries e WHERE e.class_option_slug = o.slug)`,
				slug, scienceNinSlug, listName,
			).Scan(&tier, &description)
			if tierErr == nil {
				name, description = splitScienceNinSoloTierItem(tier, description)
				err = nil
			}
		}
	}
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin subclass pick detail:", err)
		return
	}
	if category == "wow" {
		s.renderScienceNinSubclassPickDetailWoW(w, name, description)
		return
	}
	s.renderScienceNinSubclassPickDetail(w, name, description)
}

// renderScienceNinSubclassPickDetail renders the shared popup body once a
// caller has resolved a pick's own Name/Description, from either a DB
// lookup or the in-memory Regalia catalog.
func (s *server) renderScienceNinSubclassPickDetail(w http.ResponseWriter, name, description string) {
	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render science-nin subclass pick detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render science-nin subclass pick detail:", err)
	}
}

// renderScienceNinSubclassPickDetailWoW is renderScienceNinSubclassPickDetail's
// own W.O.W-specific twin — executes wow_pick_detail_card (formatWoWDescription)
// instead of hunter_pick_detail_card (formatDescription), so a Known W.o.W's
// detail popup gets the base/Ascension split. Both the plain "wow" category
// and "ascended-wow" (an Ascended W.o.W's own DetailHref, set in
// ascendedWowSection) route here — an Ascended designation is a pointer at
// the same underlying class_options row, not a separate catalog.
func (s *server) renderScienceNinSubclassPickDetailWoW(w http.ResponseWriter, name, description string) {
	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render science-nin subclass pick detail (wow): no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "wow_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render science-nin subclass pick detail (wow):", err)
	}
}
