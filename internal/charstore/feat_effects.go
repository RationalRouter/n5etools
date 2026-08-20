package charstore

import "database/sql"

// ApplyFeatAbilityBonus records a feat's ability-score-increase effect as a
// character_ability_bonuses row, source_kind='feat', source_ref=featSlug —
// the same source_kind-agnostic table clan/background/ASI bonuses already
// write to (see SetClan/SetAbilityScoreImprovement), so charsheet.Compute's
// sumAbilityBonuses picks it up with no engine changes. Delete-then-insert
// by (character_id, source_kind, source_ref), same shape
// SetAbilityScoreImprovement uses: character_ability_bonuses has no unique
// index to upsert against, and a feat only ever grants one ability's worth
// of increase, so replacing beats stacking.
func ApplyFeatAbilityBonus(charDB *sql.DB, characterID int64, featSlug, ability string, amount int) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ?`,
		characterID, featSlug,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO character_ability_bonuses (character_id, source_kind, source_ref, ability, amount)
		 VALUES (?, 'feat', ?, ?, ?)`,
		characterID, featSlug, ability, amount,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveFeatAbilityBonus drops a feat's ability-score bonus row, if any —
// a safe no-op when the feat never had one (a single-fixed-ability feat
// that was never taken, or a multi-option feat whose pending choice was
// never resolved).
func RemoveFeatAbilityBonus(charDB *sql.DB, characterID int64, featSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ?`,
		characterID, featSlug,
	)
	return err
}

// ApplyFeatSkillProficiency records a feat's fixed skill-proficiency grant
// as a character_proficiencies row, source_kind='feat', source_ref=featSlug
// — loadProficiencies (internal/charsheet) already sums kind='skill' rows
// with no source_kind filter, same reasoning as ApplyFeatAbilityBonus above.
func ApplyFeatSkillProficiency(charDB *sql.DB, characterID int64, featSlug, skill string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'skill'`,
		characterID, featSlug,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
		 VALUES (?, 'skill', ?, 'feat', ?)`,
		characterID, skill, featSlug,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveFeatSkillProficiency drops a feat's skill-proficiency row, if any.
func RemoveFeatSkillProficiency(charDB *sql.DB, characterID int64, featSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'skill'`,
		characterID, featSlug,
	)
	return err
}

// ApplyFeatToolProficiency records a feat's fixed tool/kit-proficiency grant
// as a character_proficiencies row, source_kind='feat', source_ref=featSlug,
// kind='tool' — same shape and same downstream reasoning as
// ApplyFeatSkillProficiency, for the "Kit or Skill (Pick one)" feats whose
// player picked the kit side (cmd/n5e's feat_skill_or_tool_choice.go).
func ApplyFeatToolProficiency(charDB *sql.DB, characterID int64, featSlug, tool string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'tool'`,
		characterID, featSlug,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
		 VALUES (?, 'tool', ?, 'feat', ?)`,
		characterID, tool, featSlug,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveFeatToolProficiency drops a feat's tool-proficiency row, if any.
func RemoveFeatToolProficiency(charDB *sql.DB, characterID int64, featSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'tool'`,
		characterID, featSlug,
	)
	return err
}

// HasProficiency reports whether a character already has a skill or tool
// proficiency row for the given name, from any source — checked before
// inserting a feat's own proficiency grant, so a feat whose text upgrades to
// Mastery when the target is already proficient can detect that case
// instead of writing a duplicate, no-op proficiency row (see
// cmd/n5e/feat_skill_proficiency.go's applyFeatProficiencyGrant).
func HasProficiency(charDB *sql.DB, characterID int64, kind, value string) (bool, error) {
	var n int
	err := charDB.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = ? AND kind = ? AND value = ?`,
		characterID, kind, value,
	).Scan(&n)
	return n > 0, err
}

// ApplyFeatWeaponProficiency records a feat's fixed weapon-category
// proficiency grant as a character_proficiencies row, source_kind='feat',
// source_ref=featSlug, kind='weapon' — the same kind internal/charstore/
// multiclass.go's multiclassGrants table already writes for class-granted
// weapon proficiency (e.g. "All Simple and Martial Weapons" for
// class/weapon-specialist), so a feat-granted and a class-granted weapon
// proficiency land in the same place with the same shape. Same
// delete-then-insert-by-source reasoning as ApplyFeatSkillProficiency above.
func ApplyFeatWeaponProficiency(charDB *sql.DB, characterID int64, featSlug, category string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'weapon'`,
		characterID, featSlug,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
		 VALUES (?, 'weapon', ?, 'feat', ?)`,
		characterID, category, featSlug,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveFeatWeaponProficiency drops a feat's weapon-category proficiency
// row, if any.
func RemoveFeatWeaponProficiency(charDB *sql.DB, characterID int64, featSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'weapon'`,
		characterID, featSlug,
	)
	return err
}
