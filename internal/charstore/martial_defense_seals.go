package charstore

import "database/sql"

// AddMartialDefenseSeal records a Taijutsu Specialist infusing one guard
// slot with an armor seal (equipment.slug, seal_applies_to = 'armor') under
// Martial Defense's 5th-level guard-slot feature (see
// cmd/n5e/martial_defense.go). A duplicate add is a silent no-op — the
// UNIQUE constraint on (character_id, seal_slug) makes this safe, and the
// picker's own slot-cap check is what actually decides whether a NEW seal
// can be added, not this.
func AddMartialDefenseSeal(charDB *sql.DB, characterID int64, sealSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_martial_defense_seals (character_id, seal_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, seal_slug) DO NOTHING`,
		characterID, sealSlug)
	return err
}

// RemoveMartialDefenseSeal drops one infused guard-slot seal — freely, at
// any time. The book gates re-infusing to "over the course of a full
// rest"; this app doesn't enforce that, same boundary Weapon Focus/
// Mastery/Puppet Tactics/Martial Techniques already draw.
func RemoveMartialDefenseSeal(charDB *sql.DB, characterID int64, sealSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_martial_defense_seals WHERE character_id = ? AND seal_slug = ?`,
		characterID, sealSlug)
	return err
}

// ListMartialDefenseSeals returns the equipment slugs of every armor seal a
// character has infused into a Martial Defense guard slot.
func ListMartialDefenseSeals(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT seal_slug FROM character_martial_defense_seals WHERE character_id = ?`, characterID)
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
