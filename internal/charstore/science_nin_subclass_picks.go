package charstore

import "database/sql"

// ScienceNinSubclassPickCategory is one of eight subclasses' own
// cap+catalog picks handled by cmd/n5e/science_nin_subclasses.go — a
// second, more heterogeneous cousin of character_science_nin_tools (the
// base-class Scientific Ninja Tools catalog, which needs no category
// discriminator since it's the only Creation-Points-budget pick in this
// schema).
type ScienceNinSubclassPickCategory string

const (
	ScienceNinPickEIP             ScienceNinSubclassPickCategory = "eip"
	ScienceNinPickWOW             ScienceNinSubclassPickCategory = "wow"
	ScienceNinPickPermaPerk       ScienceNinSubclassPickCategory = "perma_perk"
	ScienceNinPickBIM             ScienceNinSubclassPickCategory = "bim"
	ScienceNinPickInversionSerum  ScienceNinSubclassPickCategory = "inversion_serum"
	ScienceNinPickArsenalMod      ScienceNinSubclassPickCategory = "arsenal_mod"
	ScienceNinPickPerfectedWeapon ScienceNinSubclassPickCategory = "perfected_weapon"

	// The final four subclasses' own categories — see
	// 0046_science_nin_subclass_picks_more_categories.sql for what each one
	// backs.
	ScienceNinPickShinobiWareUpgrade    ScienceNinSubclassPickCategory = "shinobi_ware_upgrade"
	ScienceNinPickEvolvedUpgrade        ScienceNinSubclassPickCategory = "evolved_upgrade"
	ScienceNinPickSpywareProgram        ScienceNinSubclassPickCategory = "spyware_program"
	ScienceNinPickQuickHack             ScienceNinSubclassPickCategory = "quick_hack"
	ScienceNinPickAirTreckEnhancement   ScienceNinSubclassPickCategory = "air_treck_enhancement"
	ScienceNinPickRegalia               ScienceNinSubclassPickCategory = "regalia"
	ScienceNinPickTechnobiMechanization ScienceNinSubclassPickCategory = "technobi_mechanization"
	ScienceNinPickSNBUpgrade            ScienceNinSubclassPickCategory = "snb_upgrade"

	// ScienceNinPickMixedStudiesInquiry is base-class Mixed Studies (18th
	// level) — a single-slot pick, added by
	// 0051_science_nin_mixed_studies_pick.sql. option_slug holds the chosen
	// OTHER Scientific Inquiry's own subclass slug rather than a
	// class_options/class_option_entries reference like every category
	// above — see cmd/n5e/science_nin.go's handleMixedStudiesPickAdd.
	ScienceNinPickMixedStudiesInquiry ScienceNinSubclassPickCategory = "mixed_studies_inquiry"

	// ScienceNinPickInfusedTool is Infused Genius (11th level, base class) —
	// added by 0053_science_nin_infused_genius.sql. Unlike every category
	// above, its option_slug is shared with the base Scientific Ninja Tools
	// catalog (character_science_nin_tools) rather than pointing at a
	// catalog of its own — see cmd/n5e/science_nin.go's InfusedGenius field.
	ScienceNinPickInfusedTool ScienceNinSubclassPickCategory = "infused_tool"

	// The four categories below — added by
	// 0055_science_nin_more_pick_categories.sql — each restrict a cap-gated
	// pick to a subset of a character's own already-known picks in a
	// sibling category above (bim/inversion_serum/shinobi_ware_upgrade/
	// snb_upgrade), the same "restricted to already-known subset via
	// cross-reference" shape perma_perk/quick_hack/infused_tool already
	// use. See each field's own doc in
	// cmd/n5e/science_nin_subclasses.go's scienceNinGrenadierData/
	// scienceNinMadScientistData/scienceNinShinobiWareData/
	// scienceNinSNBSpecialistData.
	ScienceNinPickBIMSpecialist         ScienceNinSubclassPickCategory = "bim_specialist"
	ScienceNinPickSheepAndShepherdSerum ScienceNinSubclassPickCategory = "sheep_and_shepherd_serum"
	// ScienceNinPickShinjutsuUpgrade is PERMANENT once made ("You can not
	// change this later") — unlike every other category in this table, no
	// delete route is wired for it (see server.go).
	ScienceNinPickShinjutsuUpgrade    ScienceNinSubclassPickCategory = "shinjutsu_upgrade"
	ScienceNinPickSNBUpgradePermanent ScienceNinSubclassPickCategory = "snb_upgrade_permanent"

	// ScienceNinPickAscendedWoW is Elemental Innovationist's own Elemental
	// Innovation (17th level) — added by
	// 0058_science_nin_ascended_wow.sql. Same "restricted to already-known
	// subset via cross-reference" shape as ScienceNinPickBIMSpecialist
	// above (any known 'wow' pick qualifies, no exclusion clause): one
	// designated Ascended W.o.W, cap 1. See
	// cmd/n5e/science_nin_subclasses.go's
	// scienceNinElementalInnovationistData.DesignatedWoW.
	ScienceNinPickAscendedWoW ScienceNinSubclassPickCategory = "ascended_wow"

	// The three Mech Crafter (Titan) categories below — added by
	// 0067_titan_subclass_picks.sql. Unlike every category above, all
	// three are character-scoped rather than tied to a specific companion
	// row: Ordnance Training's own text ("You can only have 1 Titan
	// created at a time") means a character never has more than one Titan
	// to distinguish between, so no companion_id column exists on this
	// table for these categories either. See cmd/n5e/titan.go.
	//
	// ScienceNinPickTitanUpgrade is Ordnance Training's own Titan Slots
	// (cap = Proficiency Bonus) — a Mech- or Weapon-keyword Titan Upgrade
	// installed on a long rest. option_slug is the picked Titan Upgrades
	// class_option_entries.slug (or the Mastercraft tier's own
	// class_options slug for Bijuu Slayer, which has no entries row, never
	// split since it's a single named item bundled straight into its own
	// tier). Spends from the SAME Creation Points budget as the base
	// Scientific Ninja Tools catalog — see titanEffectiveUpgradeCost.
	ScienceNinPickTitanUpgrade ScienceNinSubclassPickCategory = "titan_upgrade"

	// ScienceNinPickTitanExosuitUpgrade is Endless Work's (6th level) own
	// separate 1 (2 at 14th level) Mech-keyword-only slot, restricted to
	// upgrades of Cost 8 or lower ("Greater or lower") — see
	// cmd/n5e/titan.go's own header doc on the Refined/Greater tier merge
	// this restriction is read against. Tracked as its own category (not
	// ScienceNinPickTitanUpgrade above) so the same upgrade can occupy an
	// ordinary Titan Slot and the Exo-Suit slot independently, without the
	// two colliding over the same option_slug — same "separate category
	// per independent slot pool" precedent
	// ScienceNinPickPerfectedWeapon already sets against ScienceNinPickArsenalMod.
	ScienceNinPickTitanExosuitUpgrade ScienceNinSubclassPickCategory = "titan_exosuit_upgrade"

	// ScienceNinPickTitanSpecialistCraftingKeyword is Specialist Crafting's
	// (14th level) own single-slot "Mech or Weapon" keyword designation —
	// option_slug is the literal string "mech" or "weapon", cap 1, freely
	// re-picked (the current pick is dropped, then the other is added), the
	// same "trust the player" boundary ScienceNinPickMixedStudiesInquiry's
	// own single-slot pick already draws.
	ScienceNinPickTitanSpecialistCraftingKeyword ScienceNinSubclassPickCategory = "titan_specialist_crafting_keyword"
)

// ScienceNinSubclassPick is one stored pick, along with its own pool
// ("mending"/"maiming"/"") when the category records one — every category
// except inversion_serum leaves this "".
type ScienceNinSubclassPick struct {
	OptionSlug string
	Pool       string
}

// AddScienceNinSubclassPick records one catalog pick within its own
// category, optionally noting which CCD pool paid for it (Inversion Serums
// only; every other category passes pool=""). A duplicate add UPDATES the
// stored pool rather than silently no-opping — unlike every other pick
// table in this package, Inversion Serums' own pool choice is meant to be
// changeable after the fact (the book allows "created during a Long Rest",
// this app doesn't enforce that timing, same "trust the player" boundary
// every other pick already draws), so re-submitting the same serum with a
// different pool is how a player switches it, not a no-op.
func AddScienceNinSubclassPick(charDB *sql.DB, characterID int64, category ScienceNinSubclassPickCategory, optionSlug, pool string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_science_nin_subclass_picks (character_id, category, option_slug, pool) VALUES (?, ?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO UPDATE SET pool = excluded.pool`,
		characterID, category, optionSlug, pool)
	return err
}

// RemoveScienceNinSubclassPick drops one pick — freely, at any time, same
// "trust the player" boundary every other pick removal in this package
// draws.
func RemoveScienceNinSubclassPick(charDB *sql.DB, characterID int64, category ScienceNinSubclassPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_science_nin_subclass_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListScienceNinSubclassPicks returns every pick a character has stored
// within one category, each carrying its own pool (empty for every
// category but inversion_serum).
func ListScienceNinSubclassPicks(charDB *sql.DB, characterID int64, category ScienceNinSubclassPickCategory) ([]ScienceNinSubclassPick, error) {
	rows, err := charDB.Query(
		`SELECT option_slug, pool FROM character_science_nin_subclass_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScienceNinSubclassPick
	for rows.Next() {
		var p ScienceNinSubclassPick
		if err := rows.Scan(&p.OptionSlug, &p.Pool); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
