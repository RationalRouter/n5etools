package charstore

import "database/sql"

// SetFullMetalShinobiResistance records (or changes) which damage type a
// character has chosen for one Full-Metal Shinobi resistance slot (see
// cmd/n5e/science_nin_subclasses.go for what the slot keys mean) — an
// upsert, since changing a pick is just replacing the old damage type with a
// new one, not adding a second row.
func SetFullMetalShinobiResistance(charDB *sql.DB, characterID int64, slotKey, damageType string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_full_metal_shinobi_resistances (character_id, slot_key, damage_type) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, slot_key) DO UPDATE SET damage_type = excluded.damage_type`,
		characterID, slotKey, damageType)
	return err
}

// ListFullMetalShinobiResistances returns every slot this character has
// picked a damage type for, keyed by slot_key.
func ListFullMetalShinobiResistances(charDB *sql.DB, characterID int64) (map[string]string, error) {
	rows, err := charDB.Query(
		`SELECT slot_key, damage_type FROM character_full_metal_shinobi_resistances WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slot, damageType string
		if err := rows.Scan(&slot, &damageType); err != nil {
			return nil, err
		}
		out[slot] = damageType
	}
	return out, rows.Err()
}
