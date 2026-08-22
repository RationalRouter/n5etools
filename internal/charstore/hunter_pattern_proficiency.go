package charstore

import "database/sql"

// HunterPatternProficiency is one skill or tool proficiency a Hunters
// Pattern grants once its own player choice is resolved — Kleptomaniac
// grants exactly one (Sleight of Hand or Security Kits, the player's pick),
// Habitual Researcher grants exactly two (any two distinct skills).
type HunterPatternProficiency struct {
	Kind  string // "skill" or "tool"
	Value string
}

// ApplyHunterPatternProficiencies records a Hunters Pattern's resolved
// proficiency grant(s) as character_proficiencies rows, source_kind=
// 'hunter_pattern', source_ref=patternSlug. internal/charsheet's own skill
// loader and cmd/n5e's Tool Proficiencies panel both already read
// character_proficiencies with no source_kind filter (see
// ApplyFeatSkillProficiency's own doc comment, feat_effects.go), so these
// rows take effect with no further engine changes on either side.
// Delete-then-insert by source, same shape every Apply* function in
// feat_effects.go uses: re-resolving a pattern's choice (unpicking then
// re-picking Kleptomaniac with the other option) replaces cleanly instead of
// stacking a stale row alongside the new one.
func ApplyHunterPatternProficiencies(charDB *sql.DB, characterID int64, patternSlug string, profs []HunterPatternProficiency) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'hunter_pattern' AND source_ref = ?`,
		characterID, patternSlug,
	); err != nil {
		return err
	}
	for _, p := range profs {
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, ?, ?, 'hunter_pattern', ?)`,
			characterID, p.Kind, p.Value, patternSlug,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveHunterPatternProficiencies drops every proficiency row tied to one
// Hunters Pattern pick, if any — called when the pattern itself is unpicked
// (cmd/n5e's handleHunterPickDelete), so removing Kleptomaniac or Habitual
// Researcher retracts whatever proficiency it granted instead of leaving an
// orphaned row behind with no pick to justify it.
func RemoveHunterPatternProficiencies(charDB *sql.DB, characterID int64, patternSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'hunter_pattern' AND source_ref = ?`,
		characterID, patternSlug,
	)
	return err
}
