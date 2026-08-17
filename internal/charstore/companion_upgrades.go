package charstore

import "database/sql"

// CompanionUpgrade is one Puppet Upgrade a companion has taken — see
// migration 0019_puppet_upgrades.sql.
type CompanionUpgrade struct {
	ID               int64
	CompanionID      int64
	ClassOptionSlug  string
	UpgradeEntrySlug string
}

// AddCompanionUpgrade records one upgrade pick, scoped through
// character_companions by characterID so a forged companionID from another
// character can't be touched — same ownership-scoping shape
// GetCompanion/DeleteCompanion already use.
func AddCompanionUpgrade(charDB *sql.DB, characterID, companionID int64, classOptionSlug, upgradeEntrySlug string) (int64, error) {
	res, err := charDB.Exec(`
		INSERT INTO character_companion_upgrades (companion_id, class_option_slug, upgrade_entry_slug)
		SELECT id, ?, ? FROM character_companions WHERE id = ? AND character_id = ?`,
		classOptionSlug, upgradeEntrySlug, companionID, characterID)
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

// DeleteCompanionUpgrade removes one pick, ownership-scoped the same way.
func DeleteCompanionUpgrade(charDB *sql.DB, characterID, companionID, upgradeID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_upgrades
		WHERE id = ? AND companion_id IN (
			SELECT id FROM character_companions WHERE id = ? AND character_id = ?
		)`,
		upgradeID, companionID, characterID)
	return err
}

// ListCompanionUpgrades returns one companion's picks in the order taken,
// ownership-scoped the same way.
func ListCompanionUpgrades(charDB *sql.DB, characterID, companionID int64) ([]CompanionUpgrade, error) {
	rows, err := charDB.Query(`
		SELECT u.id, u.companion_id, u.class_option_slug, u.upgrade_entry_slug
		FROM character_companion_upgrades u
		JOIN character_companions c ON c.id = u.companion_id
		WHERE u.companion_id = ? AND c.character_id = ?
		ORDER BY u.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionUpgrade
	for rows.Next() {
		var u CompanionUpgrade
		if err := rows.Scan(&u.ID, &u.CompanionID, &u.ClassOptionSlug, &u.UpgradeEntrySlug); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
