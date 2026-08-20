package charstore

import "database/sql"

// CookingNinBlendEnhancementPick is one Gastrochemist Nature's Blend
// Enhancement pick — a known jutsu (character_jutsu.id) paired with which
// of the 4 Enhancement types was chosen for it (see cmd/n5e/cooking_nin.go).
type CookingNinBlendEnhancementPick struct {
	JutsuID         int64
	EnhancementType string
}

// AddCookingNinBlendEnhancementPick records one (jutsu, Enhancement type)
// pick. A duplicate add (the jutsu already has an Enhancement) is a silent
// no-op — same shape AddNinjutsuJutsuPick already uses, since the picker's
// own cap check is what actually decides whether a NEW pick can be added,
// not this.
func AddCookingNinBlendEnhancementPick(charDB *sql.DB, characterID int64, jutsuID int64, enhancementType string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_cooking_nin_blend_enhancement_picks (character_id, jutsu_id, enhancement_type) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, jutsu_id) DO NOTHING`,
		characterID, jutsuID, enhancementType)
	return err
}

// RemoveCookingNinBlendEnhancementPick drops one pick — freely, at any
// time, same "trust the player" boundary every other pick removal on this
// sheet draws.
func RemoveCookingNinBlendEnhancementPick(charDB *sql.DB, characterID int64, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_cooking_nin_blend_enhancement_picks WHERE character_id = ? AND jutsu_id = ?`,
		characterID, jutsuID)
	return err
}

// ListCookingNinBlendEnhancementPicks returns every (jutsu, Enhancement
// type) pick a character has chosen. A row disappears from this list on its
// own once the underlying character_jutsu row is deleted (ON DELETE
// CASCADE), so forgetting a jutsu automatically clears any Enhancement pick
// that pointed at it.
func ListCookingNinBlendEnhancementPicks(charDB *sql.DB, characterID int64) ([]CookingNinBlendEnhancementPick, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id, enhancement_type FROM character_cooking_nin_blend_enhancement_picks WHERE character_id = ?`,
		characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CookingNinBlendEnhancementPick
	for rows.Next() {
		var p CookingNinBlendEnhancementPick
		if err := rows.Scan(&p.JutsuID, &p.EnhancementType); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
