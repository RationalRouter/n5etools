package charstore

import "database/sql"

// AddMedicalDoctrinePick records one of Medical Doctrine's 4 catalog picks
// (see cmd/n5e/medical_nin.go). A duplicate add is a silent no-op — same
// shape AddHunterNinPick/AddGenjutsuPick already use, since the picker's
// own cap check is what actually decides whether a NEW pick can be added,
// not this.
func AddMedicalDoctrinePick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_medical_doctrine_picks (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveMedicalDoctrinePick drops one pick — freely, at any time. RAW says
// "You cannot change this choice once made," but this app doesn't enforce
// pick-timing anywhere else either (Hunters Exploits, Malleable Mirages,
// Martial Techniques all allow free re-picks), same "trust the player"
// boundary.
func RemoveMedicalDoctrinePick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_medical_doctrine_picks WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListMedicalDoctrinePicks returns the option_slugs a character has chosen.
func ListMedicalDoctrinePicks(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_medical_doctrine_picks WHERE character_id = ?`,
		characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

// AddMedicalNinExpertCombatantPick records one of Combat Medic's Expert
// Combatant picks — a specific known-jutsu row (see
// cmd/n5e/medical_nin.go), not a rules-database slug. A duplicate add is a
// silent no-op, same boundary AddMedicalDoctrinePick draws.
func AddMedicalNinExpertCombatantPick(charDB *sql.DB, characterID int64, jutsuID int64) error {
	_, err := charDB.Exec(
		`INSERT INTO character_medical_nin_expert_combatant_picks (character_id, jutsu_id) VALUES (?, ?)
		 ON CONFLICT (character_id, jutsu_id) DO NOTHING`,
		characterID, jutsuID)
	return err
}

// RemoveMedicalNinExpertCombatantPick drops one pick — freely, at any time.
// RAW allows switching which jutsu this feature affects on a Long Rest; this
// app doesn't enforce that timing anywhere else either, same "trust the
// player" boundary as RemoveMedicalDoctrinePick.
func RemoveMedicalNinExpertCombatantPick(charDB *sql.DB, characterID int64, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_medical_nin_expert_combatant_picks WHERE character_id = ? AND jutsu_id = ?`,
		characterID, jutsuID)
	return err
}

// ListMedicalNinExpertCombatantPicks returns the character_jutsu row ids a
// character has picked for Expert Combatant. A row id disappears from this
// list on its own once the underlying character_jutsu row is deleted (ON
// DELETE CASCADE), so forgetting a jutsu automatically clears any pick that
// pointed at it.
func ListMedicalNinExpertCombatantPicks(charDB *sql.DB, characterID int64) ([]int64, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id FROM character_medical_nin_expert_combatant_picks WHERE character_id = ?`,
		characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
