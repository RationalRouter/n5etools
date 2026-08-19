package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Eight of Science-Nin's ten subclasses' own cap+catalog picks — Elemental
// Innovationist (E.I.Ps, W.O.W, Perma Perk), Grenadier (B.I.M), Mad
// Scientist (Inversion Serums), Ninjaneer (Arsenal Modifications,
// Perfected Weapons), Shinobi-Ware (Shinobi-Ware Upgrades, the Glorious
// Evolution slot-free overlay), Spyware (Spyware Programs, the Netrunner
// slot-free Quick Hack overlay), Storm Rider (A.T. Enhancements, the
// King's Road Regalia overlay), Technobi (Technobi Mechanizations), and
// S.N.B Specialist (S.N.B Upgrades only — the Scientific Ninja Beast
// companion itself remains unbuilt). Only Mech Crafter (Titan, a second
// companion system) is deliberately untouched here — see CLASS_AUDIT.md's
// Science-Nin detail entry.
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
// Storage is internal/charstore/science_nin_subclass_picks.go's single
// generic character_science_nin_subclass_picks table (migrations
// 0044_science_nin_subclass_picks.sql and
// 0046_science_nin_subclass_picks_more_categories.sql), with 15 category
// discriminators (eip/wow/perma_perk/bim/inversion_serum/arsenal_mod/
// perfected_weapon/shinobi_ware_upgrade/evolved_upgrade/spyware_program/
// quick_hack/air_treck_enhancement/regalia/technobi_mechanization/
// snb_upgrade) and a pool column scoped to Inversion Serums alone.
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
	scienceNinAirTrecksFeatureSlug         = "class/science-nin/group/scientific-inquiry/storm-rider/feature/air-trecks"
	scienceNinKingsRoadFeatureSlug         = "class/science-nin/group/scientific-inquiry/storm-rider/feature/kings-road"
	scienceNinBestLaidTrapFeatureSlug      = "class/science-nin/group/scientific-inquiry/technobi/feature/the-best-laid-trap"
	scienceNinSNBUpgradesFeatureSlug       = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/s-n-b-upgrades"
)

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
// seven catalogs. Pool is Inversion Serums only.
type knownScienceNinPick struct {
	Slug string
	Name string
	Tier string
	Pool string
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
	return out, rows.Err()
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

// scienceNinGrenadierData backs B.I.M (Explosive Modifications).
type scienceNinGrenadierData struct {
	BIMCap       int
	BIMUsed      int
	KnownBIM     []knownScienceNinPick
	AvailableBIM []scienceNinSubclassOption
}

// scienceNinMadScientistData backs Inversion Serums. Known/Available pairs
// with Pool are used instead of splitScienceNinPicks' plain shape — see
// splitInversionSerumPicks.
type scienceNinMadScientistData struct {
	SerumCap        int
	SerumUsed       int
	KnownSerums     []knownScienceNinPick
	AvailableSerums []scienceNinSubclassOption
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
}

// scienceNinShinobiWareData backs Shinobi-Ware Upgrades and the Glorious
// Evolution overlay.
type scienceNinShinobiWareData struct {
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

	// Regalia is non-nil only once the character has the 14th-level King's
	// Road feature. AvailableRegalia is the "Air Treck Enhancements" list's
	// own Regalia row (8 named entries, unlike every other tier not gated
	// by a Cost stat line — see this file's own header doc), never
	// restricted to already-known Enhancements since Regalia is an
	// entirely independent 8-option catalog of its own.
	Regalia          *knownScienceNinPick
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
	UpgradeCap        int
	UpgradeUsed       int
	KnownUpgrades     []knownScienceNinPick
	AvailableUpgrades []scienceNinSubclassOption
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
			for _, k := range ei.KnownEIPs {
				if permaSet[k.Slug] {
					pick := k
					ei.PermaPerk = &pick
					continue
				}
				if strings.EqualFold(k.Name, "Wonder E.I.P") {
					continue // Perma Perk explicitly cannot be Wonder E.I.P
				}
				ei.AvailablePermaPerk = append(ei.AvailablePermaPerk, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier})
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
		pickedSet := make(map[string]bool, len(picks))
		for _, p := range picks {
			pickedSet[p.OptionSlug] = true
		}
		gr.BIMUsed = len(picks)
		gr.KnownBIM, gr.AvailableBIM = splitScienceNinPicks(catalog, pickedSet)
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
		for _, o := range catalog {
			if pool, ok := pickedPool[o.Slug]; ok {
				ms.KnownSerums = append(ms.KnownSerums, knownScienceNinPick{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Pool: pool})
				continue
			}
			ms.AvailableSerums = append(ms.AvailableSerums, o)
		}
		data.MadScientist = ms
	}

	if has[scienceNinArsenalFeatureSlug] || has[scienceNinPerfectedWeaponFeatureSlug] {
		nj := &scienceNinNinjaneerData{}
		catalog, err := s.loadScienceNinEntryCatalog("Arsenal Modifications", false)
		if err != nil {
			return err
		}

		if has[scienceNinArsenalFeatureSlug] {
			nj.ArsenalModCap = arsenalModCap(proficiencyBonus)
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
		}

		data.Ninjaneer = nj
	}

	if has[scienceNinEdgeRunnerFeatureSlug] || has[scienceNinGloriousEvolutionFeatureSlug] {
		sw := &scienceNinShinobiWareData{}
		catalog, err := s.loadScienceNinEntryCatalog("Shinobi-Ware Upgrades", false)
		if err != nil {
			return err
		}

		if has[scienceNinEdgeRunnerFeatureSlug] {
			sw.UpgradeCap = shinobiWareUpgradeCap(proficiencyBonus)
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickShinobiWareUpgrade)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sw.UpgradeUsed = len(picks)
			sw.KnownUpgrades, sw.AvailableUpgrades = splitScienceNinPicks(catalog, pickedSet)
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

		data.ShinobiWare = sw
	}

	if has[scienceNinCruelAngelsThesisFeatureSlug] || has[scienceNinNetrunnerFeatureSlug] {
		sp := &scienceNinSpywareData{}

		if has[scienceNinCruelAngelsThesisFeatureSlug] {
			sp.ProgramCap = spywareProgramCap(proficiencyBonus)
			catalog, err := s.loadScienceNinEntryCatalog("Spyware Programs", false)
			if err != nil {
				return err
			}
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickSpywareProgram)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			sp.ProgramUsed = len(picks)
			sp.KnownPrograms, sp.AvailablePrograms = splitScienceNinPicks(catalog, pickedSet)
		}

		if has[scienceNinNetrunnerFeatureSlug] {
			quickHackPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickQuickHack)
			if err != nil {
				return err
			}
			quickHackSet := make(map[string]bool, len(quickHackPicks))
			for _, p := range quickHackPicks {
				quickHackSet[p.OptionSlug] = true
			}
			for _, k := range sp.KnownPrograms {
				if quickHackSet[k.Slug] {
					pick := k
					sp.QuickHack = &pick
					continue
				}
				sp.AvailableQuickHack = append(sp.AvailableQuickHack, scienceNinSubclassOption{Slug: k.Slug, Name: k.Name, Tier: k.Tier})
			}
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
			picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickRegalia)
			if err != nil {
				return err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, p := range picks {
				pickedSet[p.OptionSlug] = true
			}
			var known []knownScienceNinPick
			known, sr.AvailableRegalia = splitScienceNinPicks(scienceNinRegaliaOptions, pickedSet)
			if len(known) > 0 {
				sr.Regalia = &known[0]
			}
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
		data.Technobi = tb
	}

	if has[scienceNinSNBUpgradesFeatureSlug] {
		snb := &scienceNinSNBSpecialistData{UpgradeCap: snbUpgradeCap(proficiencyBonus)}
		catalog, err := s.loadScienceNinEntryCatalog("S.N.B Upgrades", false)
		if err != nil {
			return err
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
		data.SNBSpecialist = snb
	}

	return nil
}

// handleScienceNinSubclassPickAdd builds one category's "learn a pick"
// route — shared by all fifteen of this file's catalogs, differing only in
// which of scienceNinToolsTabData's own fields govern the cap/current-count
// and currently-pickable list, same factory shape
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
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		pool := ""
		if requiresPool {
			parts := strings.SplitN(slug, "|", 2)
			if len(parts) != 2 {
				http.Error(w, "missing pool", http.StatusBadRequest)
				return
			}
			slug, pool = parts[0], parts[1]
			if pool != "mending" && pool != "maiming" {
				http.Error(w, "invalid pool", http.StatusBadRequest)
				return
			}
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for science-nin subclass pick add:", err)
			return
		}
		data, err := s.loadScienceNinTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load science-nin for subclass pick add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Science-Nin", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		var picked *scienceNinSubclassOption
		for _, o := range available(data) {
			if o.Slug == slug {
				picked = &o
				break
			}
		}
		if picked == nil {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if requiresPool && picked.FixedPool != "" && picked.FixedPool != pool {
			http.Error(w, "this serum can only be paid from its own CCD half", http.StatusBadRequest)
			return
		}
		if err := charstore.AddScienceNinSubclassPick(s.charDB, id, category, slug, pool); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add science-nin subclass pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_science_nin")
	}
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
// Known pick in any of this file's fifteen catalogs — same
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
