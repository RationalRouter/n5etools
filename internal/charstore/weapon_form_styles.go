package charstore

import "database/sql"

// AddWeaponFormStyle records a Weapon Specialist learning one Style from
// their chosen Weapon Form's own list (see cmd/n5e/weapon_specialist.go). A
// duplicate add is a silent no-op — the UNIQUE constraint on (character_id,
// option_slug) makes this safe, and the picker's own cap check is what
// actually decides whether a NEW Style can be added, not this.
func AddWeaponFormStyle(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_weapon_form_styles (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveWeaponFormStyle drops one known Style — freely, at any time. This
// app doesn't enforce rest timing, same boundary Martial Techniques already
// draws.
func RemoveWeaponFormStyle(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_weapon_form_styles WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListWeaponFormStyles returns the option_slugs of every Style a character
// has learned.
func ListWeaponFormStyles(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_weapon_form_styles WHERE character_id = ?`, characterID)
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
