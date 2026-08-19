package charstore

import "database/sql"

// AddScienceNinTool records one Scientific Ninja Tools catalog pick (see
// cmd/n5e/science_nin.go). A duplicate add is a silent no-op — same shape
// AddNinjutsuMoldingPick/AddMedicalDoctrinePick already use, since the
// picker's own Creation Points budget check is what actually decides
// whether a NEW pick can be added, not this.
func AddScienceNinTool(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_science_nin_tools (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveScienceNinTool drops one known tool — freely, at any time. The book
// states no rest/level-up gate on this pick at all (unlike Efficient
// Molding or Hunters Exploits), so this is the least restrictive of every
// pick-removal function in this package, not just following their same
// "trust the player" boundary.
func RemoveScienceNinTool(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_science_nin_tools WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListScienceNinTools returns the class_option_entries slugs a character has
// picked.
func ListScienceNinTools(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_science_nin_tools WHERE character_id = ?`,
		characterID)
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
