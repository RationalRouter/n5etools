package charstore

import "database/sql"

// SetElementalAffinity records (or changes) which element a character has
// chosen for one affinity "slot" (see cmd/n5e/elemental_affinity.go for what
// the slot keys mean) — an upsert, since changing a pick is just replacing
// the old element with a new one, not adding a second row.
func SetElementalAffinity(charDB *sql.DB, characterID int64, slotKey, element string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_elemental_affinities (character_id, slot_key, element) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, slot_key) DO UPDATE SET element = excluded.element`,
		characterID, slotKey, element)
	return err
}

// ClearElementalAffinity drops one slot's pick entirely (e.g. the slot no
// longer applies — a clan swap during play, however rare).
func ClearElementalAffinity(charDB *sql.DB, characterID int64, slotKey string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_elemental_affinities WHERE character_id = ? AND slot_key = ?`,
		characterID, slotKey)
	return err
}

// ListElementalAffinities returns every slot this character has picked an
// element for, keyed by slot_key.
func ListElementalAffinities(charDB *sql.DB, characterID int64) (map[string]string, error) {
	rows, err := charDB.Query(
		`SELECT slot_key, element FROM character_elemental_affinities WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slot, element string
		if err := rows.Scan(&slot, &element); err != nil {
			return nil, err
		}
		out[slot] = element
	}
	return out, rows.Err()
}
