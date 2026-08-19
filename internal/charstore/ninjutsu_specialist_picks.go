package charstore

import "database/sql"

// AddNinjutsuMoldingPick records one Efficient Molding catalog pick (see
// cmd/n5e/ninjutsu_specialist.go). A duplicate add is a silent no-op — same
// shape AddMedicalDoctrinePick/AddMartialTechnique already use, since the
// picker's own cap check is what actually decides whether a NEW pick can be
// added, not this.
func AddNinjutsuMoldingPick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_ninjutsu_molding_picks (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveNinjutsuMoldingPick drops one pick — freely, at any time. RAW gates
// re-picking to a class level-up ("When you would level up in this class,
// you may swap your current Moldings"); this app doesn't enforce that
// timing anywhere else either (Hunters Exploits, Martial Techniques, Plans
// all allow free re-picks), same "trust the player" boundary.
func RemoveNinjutsuMoldingPick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_ninjutsu_molding_picks WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListNinjutsuMoldingPicks returns the option_slugs a character has chosen.
func ListNinjutsuMoldingPicks(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_ninjutsu_molding_picks WHERE character_id = ?`,
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

// NinjutsuJutsuPickCategory is one of Ninjutsu Specialist's two picks
// sourced from the character's own known jutsu (character_jutsu) rather
// than a rules-database catalog — see cmd/n5e/ninjutsu_specialist.go.
type NinjutsuJutsuPickCategory string

const (
	NinjutsuPickRefined NinjutsuJutsuPickCategory = "refined_ninjutsu"
	NinjutsuPickMaster  NinjutsuJutsuPickCategory = "ninjutsu_master"
)

// AddNinjutsuJutsuPick records one category's pick of a specific known-jutsu
// row. A duplicate add is a silent no-op, same boundary every other pick
// table in this file draws.
func AddNinjutsuJutsuPick(charDB *sql.DB, characterID int64, category NinjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`INSERT INTO character_ninjutsu_jutsu_picks (character_id, category, jutsu_id) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, jutsu_id) DO NOTHING`,
		characterID, category, jutsuID)
	return err
}

// RemoveNinjutsuJutsuPick drops one pick — freely, at any time. Refined
// Ninjutsu's own text allows changing on a long rest; this app doesn't
// enforce that timing, same "trust the player" boundary as above.
func RemoveNinjutsuJutsuPick(charDB *sql.DB, characterID int64, category NinjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_ninjutsu_jutsu_picks WHERE character_id = ? AND category = ? AND jutsu_id = ?`,
		characterID, category, jutsuID)
	return err
}

// ListNinjutsuJutsuPicks returns the character_jutsu row ids a character has
// picked within one category. A row id disappears from this list on its own
// once the underlying character_jutsu row is deleted (ON DELETE CASCADE),
// so forgetting a jutsu automatically clears any Refined Ninjutsu/Ninjutsu
// Master pick that pointed at it.
func ListNinjutsuJutsuPicks(charDB *sql.DB, characterID int64, category NinjutsuJutsuPickCategory) ([]int64, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id FROM character_ninjutsu_jutsu_picks WHERE character_id = ? AND category = ?`,
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
