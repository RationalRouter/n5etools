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
