package charstore

import "database/sql"

// IntelligenceOperativePickCategory is one of the two cap-gated catalogs
// Intelligence Operative grants — see cmd/n5e/intelligence_operative.go.
type IntelligenceOperativePickCategory string

const (
	IntelligenceOperativePickPlan          IntelligenceOperativePickCategory = "plan"
	IntelligenceOperativePickOperativeTrap IntelligenceOperativePickCategory = "operative_trap"
)

// AddIntelligenceOperativePick records one catalog pick within its own
// category. A duplicate add is a silent no-op — same shape
// AddHunterNinPick/AddGenjutsuPick already use, since the picker's own cap
// check is what actually decides whether a NEW pick can be added, not this.
func AddIntelligenceOperativePick(charDB *sql.DB, characterID int64, category IntelligenceOperativePickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_intelligence_operative_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveIntelligenceOperativePick drops one pick — freely, at any time.
// Plans' own text allows switching on a long rest ("When you would gain the
// benefits of a long rest you may switch the plans you know with another
// plan"); this app doesn't enforce that timing anywhere, same boundary
// Hunter-Nin/Genjutsu Specialist's own picks already draw.
func RemoveIntelligenceOperativePick(charDB *sql.DB, characterID int64, category IntelligenceOperativePickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_intelligence_operative_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListIntelligenceOperativePicks returns the option_slugs a character has
// picked within one category.
func ListIntelligenceOperativePicks(charDB *sql.DB, characterID int64, category IntelligenceOperativePickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_intelligence_operative_picks WHERE character_id = ? AND category = ?`,
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
