package charstore

import "database/sql"

// GenjutsuJutsuPickCategory is one of Beguiler's Twisted Casting / Corrupt
// Thoughts' Psyche Breaker — both "select two Genjutsu that you know" picks
// sourced from the character's own known jutsu (character_jutsu) rather
// than a rules-database catalog, the same shape
// NinjutsuJutsuPickCategory/AddNinjutsuJutsuPick already establish for
// Ninjutsu Specialist's Refined Ninjutsu/Ninjutsu Master — see
// cmd/n5e/genjutsu.go's loadKnownGenjutsu. A distinct type from
// GenjutsuPickCategory (genjutsu_picks.go), which keys the class's other
// four picks by rules-database option_slug instead.
type GenjutsuJutsuPickCategory string

const (
	GenjutsuPickTwistedCasting GenjutsuJutsuPickCategory = "twisted_casting"
	GenjutsuPickPsycheBreaker  GenjutsuJutsuPickCategory = "psyche_breaker"
)

// AddGenjutsuJutsuPick records one category's pick of a specific known-jutsu
// row. A duplicate add is a silent no-op — same shape AddNinjutsuJutsuPick
// already uses, since the picker's own cap check is what actually decides
// whether a NEW pick can be added, not this.
func AddGenjutsuJutsuPick(charDB *sql.DB, characterID int64, category GenjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`INSERT INTO character_genjutsu_jutsu_picks (character_id, category, jutsu_id) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, jutsu_id) DO NOTHING`,
		characterID, category, jutsuID)
	return err
}

// RemoveGenjutsuJutsuPick drops one pick — freely, at any time. Twisted
// Casting's own text allows changing on a Long Rest; this app doesn't
// enforce that timing, same "trust the player" boundary
// RemoveNinjutsuJutsuPick already draws.
func RemoveGenjutsuJutsuPick(charDB *sql.DB, characterID int64, category GenjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_genjutsu_jutsu_picks WHERE character_id = ? AND category = ? AND jutsu_id = ?`,
		characterID, category, jutsuID)
	return err
}

// ListGenjutsuJutsuPicks returns the character_jutsu row ids a character
// has picked within one category. A row id disappears from this list on its
// own once the underlying character_jutsu row is deleted (ON DELETE
// CASCADE), so forgetting a jutsu automatically clears any Twisted Casting/
// Psyche Breaker pick that pointed at it.
func ListGenjutsuJutsuPicks(charDB *sql.DB, characterID int64, category GenjutsuJutsuPickCategory) ([]int64, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id FROM character_genjutsu_jutsu_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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
