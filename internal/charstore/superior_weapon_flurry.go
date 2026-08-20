package charstore

import "database/sql"

// AddSuperiorWeaponFlurryBenefit records a Weapon Specialist selecting one
// of Superior Weapon Flurry's own 4 named benefits (see cmd/n5e/
// weapon_specialist.go — the catalog is a small hardcoded Go slice, not a
// rules.db table; no class_options rows exist for these 4 benefits). A
// duplicate add is a silent no-op — the UNIQUE constraint on (character_id,
// option_slug) makes this safe, and the picker's own cap check is what
// actually decides whether a NEW benefit can be added, not this.
func AddSuperiorWeaponFlurryBenefit(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_superior_weapon_flurry (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveSuperiorWeaponFlurryBenefit drops one selected benefit — freely, at
// any time. This app doesn't enforce rest/level-up timing, same boundary
// Martial Techniques and Weapon Form Styles already draw.
func RemoveSuperiorWeaponFlurryBenefit(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_superior_weapon_flurry WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListSuperiorWeaponFlurryBenefits returns the option_slugs of every
// Superior Weapon Flurry benefit a character has selected.
func ListSuperiorWeaponFlurryBenefits(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_superior_weapon_flurry WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}
