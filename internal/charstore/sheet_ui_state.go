package charstore

import "database/sql"

// GetSheetUIState reads every saved UI-state blob for a character —
// sheet-layout.js's grid layouts ("grid:core"/"grid:bio") and
// sheet-subgrid.js's tile order/orientation ("subgrid:<name>") alike — in
// one query, keyed by state_key. Absent keys are simply absent from the
// returned map; callers treat that identically to "no saved state" the way
// a missing localStorage key used to.
func GetSheetUIState(charDB *sql.DB, characterID int64) (map[string]string, error) {
	rows, err := charDB.Query(
		`SELECT state_key, state_json FROM character_sheet_ui_state WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	state := map[string]string{}
	for rows.Next() {
		var key, json string
		if err := rows.Scan(&key, &json); err != nil {
			return nil, err
		}
		state[key] = json
	}
	return state, rows.Err()
}

// SetSheetUIState upserts one keyed blob. stateJSON is stored verbatim —
// this package doesn't parse it any more than sheet-layout.js's own
// serialize/deserialize round-trip needed a shared Go type; validating it's
// at least well-formed JSON is the HTTP handler's job (see
// handleSheetUIState), same boundary every other free-text field in this
// codebase draws.
func SetSheetUIState(charDB *sql.DB, characterID int64, stateKey, stateJSON string) error {
	_, err := charDB.Exec(`
		INSERT INTO character_sheet_ui_state (character_id, state_key, state_json, updated_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT (character_id, state_key) DO UPDATE
		SET state_json = excluded.state_json, updated_at = excluded.updated_at`,
		characterID, stateKey, stateJSON)
	return err
}

// DeleteSheetUIState clears one saved blob — Reset Layout's server-side
// half, so a reset genuinely returns to the template's computed defaults
// rather than reloading straight back into what was just cleared.
func DeleteSheetUIState(charDB *sql.DB, characterID int64, stateKey string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_sheet_ui_state WHERE character_id = ? AND state_key = ?`,
		characterID, stateKey)
	return err
}
