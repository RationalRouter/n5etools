package charstore

import "database/sql"

// CompanionUpgradeChoice is one sub-pick under a repeatable upgrade (e.g.
// one of the up-to-3 Poison Tag qualities Poison Mist Hell lets a player
// install) — see migration 0021_companion_upgrade_choices.sql.
type CompanionUpgradeChoice struct {
	ID                 int64
	CompanionUpgradeID int64
	ChoiceSlug         string
}

// AddCompanionUpgradeChoice records one sub-pick, scoped through
// character_companion_upgrades → character_companions by characterID AND
// companionID so a forged companionUpgradeID belonging to another
// companion or character can't be touched.
func AddCompanionUpgradeChoice(charDB *sql.DB, characterID, companionID, companionUpgradeID int64, choiceSlug string) (int64, error) {
	res, err := charDB.Exec(`
		INSERT INTO character_companion_upgrade_choices (companion_upgrade_id, choice_slug)
		SELECT u.id, ? FROM character_companion_upgrades u
		JOIN character_companions c ON c.id = u.companion_id
		WHERE u.id = ? AND c.id = ? AND c.character_id = ?`,
		choiceSlug, companionUpgradeID, companionID, characterID)
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

// DeleteCompanionUpgradeChoice removes one sub-pick, ownership-scoped the
// same way.
func DeleteCompanionUpgradeChoice(charDB *sql.DB, characterID, companionID, choiceID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_upgrade_choices
		WHERE id = ? AND companion_upgrade_id IN (
			SELECT u.id FROM character_companion_upgrades u
			JOIN character_companions c ON c.id = u.companion_id
			WHERE c.id = ? AND c.character_id = ?
		)`,
		choiceID, companionID, characterID)
	return err
}

// ListCompanionUpgradeChoices returns every sub-pick recorded under any of
// this companion's upgrade picks, ownership-scoped the same way — callers
// group by CompanionUpgradeID themselves (see cmd/n5e/puppets.go), same
// shape ListCompanionUpgrades already leaves to its own callers.
func ListCompanionUpgradeChoices(charDB *sql.DB, characterID, companionID int64) ([]CompanionUpgradeChoice, error) {
	rows, err := charDB.Query(`
		SELECT ch.id, ch.companion_upgrade_id, ch.choice_slug
		FROM character_companion_upgrade_choices ch
		JOIN character_companion_upgrades u ON u.id = ch.companion_upgrade_id
		JOIN character_companions c ON c.id = u.companion_id
		WHERE c.id = ? AND c.character_id = ?
		ORDER BY ch.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionUpgradeChoice
	for rows.Next() {
		var c CompanionUpgradeChoice
		if err := rows.Scan(&c.ID, &c.CompanionUpgradeID, &c.ChoiceSlug); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
