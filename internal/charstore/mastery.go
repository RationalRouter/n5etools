package charstore

import "database/sql"

// MasteryEntry is one skill/toolkit's own Mastery rank — see migration
// 0026_mastery.sql for the book rule this backs.
type MasteryEntry struct {
	ID        int64
	SkillName string
	Rank      int
}

// SetMastery records or updates a skill/toolkit's Mastery rank — an
// upsert, since a player changing an existing entry's rank (say, Rank 1 to
// Rank 2 as they level up) is a normal edit, not a new pick, and the
// UNIQUE(character_id, skill_name) constraint would otherwise reject it.
func SetMastery(charDB *sql.DB, characterID int64, skillName string, rank int) error {
	_, err := charDB.Exec(
		`INSERT INTO character_mastery (character_id, skill_name, rank) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, skill_name) DO UPDATE SET rank = excluded.rank`,
		characterID, skillName, rank)
	return err
}

// RemoveMastery drops one skill/toolkit's Mastery entry entirely.
func RemoveMastery(charDB *sql.DB, characterID int64, skillName string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_mastery WHERE character_id = ? AND skill_name = ?`,
		characterID, skillName)
	return err
}

// ListMastery returns every Mastery entry a character has.
func ListMastery(charDB *sql.DB, characterID int64) ([]MasteryEntry, error) {
	rows, err := charDB.Query(
		`SELECT id, skill_name, rank FROM character_mastery WHERE character_id = ? ORDER BY skill_name`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MasteryEntry
	for rows.Next() {
		var e MasteryEntry
		if err := rows.Scan(&e.ID, &e.SkillName, &e.Rank); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
