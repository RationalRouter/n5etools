package charstore

import "database/sql"

// SetCustomResourceValue sets a custom resource straight to a computed
// absolute value — a rest's regen result, or an edit box's delta already
// applied against the resource's own Max — rather than applying a raw SQL
// delta. This is the ONLY writer for character_custom_resources, precisely
// because a resource with no stored row yet is meant to start at its own
// Max (see computeCustomResources), not 0 — a delta applied via SQL
// (`current = current + ?`) against a row that doesn't exist yet would
// have no baseline to add against, silently landing wrong (confirmed live:
// a fresh CCD's first -20 spend produced 0, not Max-20, because the INSERT
// path had no way to know Max). Computing the new absolute value in Go
// (where Max is known) and only ever SETting it here avoids that class of
// bug entirely.
func SetCustomResourceValue(charDB *sql.DB, characterID int64, key string, value int) error {
	_, err := charDB.Exec(`
		INSERT INTO character_custom_resources (character_id, resource_key, current)
		VALUES (?, ?, ?)
		ON CONFLICT(character_id, resource_key) DO UPDATE SET current = excluded.current`,
		characterID, key, value,
	)
	return err
}

// GetCustomResources returns every custom resource value stored for a
// character, keyed by resource_key. A resource with no row yet (never
// spent, never rested) is simply absent — computeCustomResources treats
// that as "start at Max" for display, and every write path
// (handleSheetCustomResource, applyCustomResourceRest) computes its new
// absolute value off that same default before calling
// SetCustomResourceValue, so the first-ever write for a resource always
// persists correctly relative to Max rather than an assumed 0.
func GetCustomResources(charDB *sql.DB, characterID int64) (map[string]int, error) {
	rows, err := charDB.Query(
		`SELECT resource_key, current FROM character_custom_resources WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var key string
		var current int
		if err := rows.Scan(&key, &current); err != nil {
			return nil, err
		}
		out[key] = current
	}
	return out, rows.Err()
}
