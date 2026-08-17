package charstore

import "database/sql"

// StartConcentration records the character's one active concentration slot,
// replacing whatever was there before. Only one slot exists in the base
// rules (see migration 0016_concentration.sql), so the upsert against
// character_id's primary key is what makes "casting a second concentration
// jutsu silently drops the first" fall out for free, rather than needing an
// explicit delete-then-insert.
func StartConcentration(charDB *sql.DB, characterID int64, jutsuSlug, castAtRank string) error {
	_, err := charDB.Exec(`
		INSERT INTO character_concentration (character_id, jutsu_slug, cast_at_rank)
		VALUES (?, ?, ?)
		ON CONFLICT(character_id) DO UPDATE SET
			jutsu_slug = excluded.jutsu_slug,
			cast_at_rank = excluded.cast_at_rank,
			started_at = datetime('now')`,
		characterID, jutsuSlug, castAtRank,
	)
	return err
}

// BreakConcentration clears the character's active concentration slot, if
// any. A no-op (not an error) when nothing was active.
func BreakConcentration(charDB *sql.DB, characterID int64) error {
	_, err := charDB.Exec(`DELETE FROM character_concentration WHERE character_id = ?`, characterID)
	return err
}

// GetConcentration returns the character's active concentration, if any.
// ok is false (with jutsuSlug/castAtRank empty) when nothing is active.
func GetConcentration(charDB *sql.DB, characterID int64) (jutsuSlug, castAtRank string, ok bool, err error) {
	err = charDB.QueryRow(
		`SELECT jutsu_slug, cast_at_rank FROM character_concentration WHERE character_id = ?`, characterID,
	).Scan(&jutsuSlug, &castAtRank)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return jutsuSlug, castAtRank, true, nil
}
