package charstore

import "database/sql"

// GenjutsuPickCategory is one of the four cap-gated catalogs Genjutsu
// Specialist's base class grants — see cmd/n5e/genjutsu.go.
type GenjutsuPickCategory string

const (
	GenjutsuPickMirage          GenjutsuPickCategory = "mirage"
	GenjutsuPickInception       GenjutsuPickCategory = "inception"
	GenjutsuPickConversion      GenjutsuPickCategory = "conversion"
	GenjutsuPickIllusionMastery GenjutsuPickCategory = "illusion_mastery"
)

// AddGenjutsuPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddHunterNinPick already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddGenjutsuPick(charDB *sql.DB, characterID int64, category GenjutsuPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_genjutsu_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveGenjutsuPick drops one pick — freely, at any time. Genjutsu
// Inception's own text implies a permanent choice and Malleable Mirages
// gates re-picking to a class level-up, but this app doesn't enforce
// re-pick timing anywhere, same boundary Hunter-Nin's own picks draw.
func RemoveGenjutsuPick(charDB *sql.DB, characterID int64, category GenjutsuPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_genjutsu_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListGenjutsuPicks returns the option_slugs a character has picked within
// one category.
func ListGenjutsuPicks(charDB *sql.DB, characterID int64, category GenjutsuPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_genjutsu_picks WHERE character_id = ? AND category = ?`,
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
