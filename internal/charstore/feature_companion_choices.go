package charstore

import "database/sql"

// SetFeatureCompanionChoice records (or changes) a player's pick for one
// companion-scoped choice slot — see 0033_feature_companion_choices.sql for
// what feature_slug/companion_id/choice_index mean together. An upsert, the
// same shape SetFeatureChoice already uses for its character-scoped sibling.
func SetFeatureCompanionChoice(charDB *sql.DB, characterID int64, featureSlug string, companionID int64, choiceIndex int, value string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_feature_companion_choices (character_id, feature_slug, companion_id, choice_index, value) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (character_id, feature_slug, companion_id, choice_index) DO UPDATE SET value = excluded.value`,
		characterID, featureSlug, companionID, choiceIndex, value)
	return err
}

// GetFeatureCompanionChoice returns a companion-scoped pick's stored value,
// or "" if it hasn't been made yet.
func GetFeatureCompanionChoice(charDB *sql.DB, characterID int64, featureSlug string, companionID int64, choiceIndex int) (string, error) {
	var value string
	err := charDB.QueryRow(
		`SELECT value FROM character_feature_companion_choices
		 WHERE character_id = ? AND feature_slug = ? AND companion_id = ? AND choice_index = ?`,
		characterID, featureSlug, companionID, choiceIndex,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ListFeatureCompanionChoices returns every companion's own resolved pick
// for one (feature_slug, choice_index) slot at once, keyed by companion_id —
// for a caller resolving a whole roster's worth of picks together instead
// of one GetFeatureCompanionChoice call per companion.
func ListFeatureCompanionChoices(charDB *sql.DB, characterID int64, featureSlug string, choiceIndex int) (map[int64]string, error) {
	rows, err := charDB.Query(
		`SELECT companion_id, value FROM character_feature_companion_choices
		 WHERE character_id = ? AND feature_slug = ? AND choice_index = ?`,
		characterID, featureSlug, choiceIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var companionID int64
		var value string
		if err := rows.Scan(&companionID, &value); err != nil {
			return nil, err
		}
		out[companionID] = value
	}
	return out, rows.Err()
}
