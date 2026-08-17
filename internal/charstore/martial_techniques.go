package charstore

import "database/sql"

// AddMartialTechnique records a Taijutsu Specialist learning one of the 20
// class-wide Martial Techniques (see cmd/n5e/taijutsu.go). A duplicate add
// is a silent no-op — the UNIQUE constraint on (character_id, option_slug)
// makes this safe, and the picker's own cap check is what actually decides
// whether a NEW technique can be added, not this.
func AddMartialTechnique(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_martial_techniques (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveMartialTechnique drops one known technique — freely, at any time.
// Martial Adept's own text gates re-picking to a Full Rest; this app
// doesn't enforce rest timing, same boundary Mastery/Puppet Tactics already
// draw.
func RemoveMartialTechnique(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_martial_techniques WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListMartialTechniques returns the option_slugs of every Martial Technique
// a character has learned.
func ListMartialTechniques(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_martial_techniques WHERE character_id = ?`, characterID)
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
