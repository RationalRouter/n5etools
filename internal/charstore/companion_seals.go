package charstore

import "database/sql"

// CompanionSeal is one Armor or Weapon seal equipped on a companion via
// Chakra Enhanced Retrofit (Class: Puppet Master/3rd Level) — see migration
// 0025_companion_seals.sql. SealSlug is a rules.db equipment slug
// (kind='enhancement_seal'), same cross-DB slug-reference tolerance the
// rest of this app's companion tables already use.
type CompanionSeal struct {
	ID       int64
	SealSlug string
}

// AddCompanionSeal records one equipped seal, ownership-scoped by
// characterID AND companionID so a forged companionID belonging to another
// character can't be touched.
func AddCompanionSeal(charDB *sql.DB, characterID, companionID int64, sealSlug string) (int64, error) {
	res, err := charDB.Exec(`
		INSERT INTO character_companion_seals (companion_id, seal_slug)
		SELECT c.id, ? FROM character_companions c
		WHERE c.id = ? AND c.character_id = ?`,
		sealSlug, companionID, characterID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, sql.ErrNoRows
	}
	return res.LastInsertId()
}

// DeleteCompanionSeal removes one equipped seal, ownership-scoped the same
// way.
func DeleteCompanionSeal(charDB *sql.DB, characterID, companionID, sealID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_seals
		WHERE id = ? AND companion_id IN (
			SELECT c.id FROM character_companions c
			WHERE c.id = ? AND c.character_id = ?
		)`,
		sealID, companionID, characterID)
	return err
}

// ListCompanionSeals returns every seal equipped on this companion,
// ownership-scoped the same way.
func ListCompanionSeals(charDB *sql.DB, characterID, companionID int64) ([]CompanionSeal, error) {
	rows, err := charDB.Query(`
		SELECT s.id, s.seal_slug
		FROM character_companion_seals s
		JOIN character_companions c ON c.id = s.companion_id
		WHERE c.id = ? AND c.character_id = ?
		ORDER BY s.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionSeal
	for rows.Next() {
		var s CompanionSeal
		if err := rows.Scan(&s.ID, &s.SealSlug); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
