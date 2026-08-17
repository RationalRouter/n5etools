package charstore

import "database/sql"

// AddGeneralizedSkill records one skill chosen for Puppet Master's
// "Generalized Skill" feature (see cmd/n5e/puppet_skills.go). A duplicate
// add is a silent no-op — the UNIQUE constraint on (character_id,
// skill_name) makes this safe, and the picker's own cap check (2 + Int
// modifier) is what actually decides whether a NEW skill can be added, not
// this.
func AddGeneralizedSkill(charDB *sql.DB, characterID int64, skillName string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_generalized_skills (character_id, skill_name) VALUES (?, ?)
		 ON CONFLICT (character_id, skill_name) DO NOTHING`,
		characterID, skillName)
	return err
}

// RemoveGeneralizedSkill drops one chosen skill — freely, at any time (the
// book says "you may swap the skills... on a long rest," but this app
// doesn't enforce rest-gated pacing on swaps anywhere else either, see
// RemovePuppetTactic's own doc for the same tradeoff).
func RemoveGeneralizedSkill(charDB *sql.DB, characterID int64, skillName string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_generalized_skills WHERE character_id = ? AND skill_name = ?`,
		characterID, skillName)
	return err
}

// ListGeneralizedSkills returns every skill a character has chosen for
// Generalized Skill.
func ListGeneralizedSkills(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT skill_name FROM character_generalized_skills WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
