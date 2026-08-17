package charstore

import "database/sql"

// AddPuppetTactic records a character taking one of Puppet Master's 5 named
// Tactics (see cmd/n5e/puppet_tactics.go). A duplicate add (the same
// tactic_slug already recorded) is a silent no-op rather than an error —
// the UNIQUE constraint on (character_id, tactic_slug) makes this safe, and
// the picker's own cap check is what actually decides whether a NEW tactic
// can be added, not this.
func AddPuppetTactic(charDB *sql.DB, characterID int64, tacticSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_puppet_tactics (character_id, tactic_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, tactic_slug) DO NOTHING`,
		characterID, tacticSlug)
	return err
}

// RemovePuppetTactic drops one chosen tactic — freely, at any time, matching
// how this app already treats Puppet Upgrade swapping (see puppets.go's own
// doc on why the real-game "only on a rest"/"only on level up" pacing rules
// aren't enforced here).
func RemovePuppetTactic(charDB *sql.DB, characterID int64, tacticSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_puppet_tactics WHERE character_id = ? AND tactic_slug = ?`,
		characterID, tacticSlug)
	return err
}

// ListPuppetTactics returns the slugs of every tactic a character has
// chosen.
func ListPuppetTactics(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT tactic_slug FROM character_puppet_tactics WHERE character_id = ?`, characterID)
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
