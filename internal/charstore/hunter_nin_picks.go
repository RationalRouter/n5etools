package charstore

import "database/sql"

// HunterNinPickCategory is one of the three cap-gated catalogs Hunter-Nin's
// base class grants — see cmd/n5e/hunter_nin.go.
type HunterNinPickCategory string

const (
	HunterPickPattern         HunterNinPickCategory = "pattern"
	HunterPickExploit         HunterNinPickCategory = "exploit"
	HunterPickDefensiveTactic HunterNinPickCategory = "defensive_tactic"
)

// AddHunterNinPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddMartialTechnique already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddHunterNinPick(charDB *sql.DB, characterID int64, category HunterNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_hunter_nin_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveHunterNinPick drops one pick — freely, at any time. The book gates
// re-picking Hunters Exploits to a long rest; this app doesn't enforce rest
// timing, same boundary Martial Adept/Mastery/Puppet Tactics already draw.
func RemoveHunterNinPick(charDB *sql.DB, characterID int64, category HunterNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_hunter_nin_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListHunterNinPicks returns the option_slugs a character has picked within
// one category.
func ListHunterNinPicks(charDB *sql.DB, characterID int64, category HunterNinPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_hunter_nin_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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
