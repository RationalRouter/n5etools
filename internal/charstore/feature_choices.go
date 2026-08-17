package charstore

import "database/sql"

// SetFeatureChoice records (or changes) a player's pick for one independent
// choice slot a granted feature offers — an upsert, since changing a pick is
// just replacing the old value, not adding a second row. See
// internal/schema's 0028_feature_choices.sql for what feature_slug and
// choiceIndex mean together.
func SetFeatureChoice(charDB *sql.DB, characterID int64, featureSlug string, choiceIndex int, value string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_feature_choices (character_id, feature_slug, choice_index, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT (character_id, feature_slug, choice_index) DO UPDATE SET value = excluded.value`,
		characterID, featureSlug, choiceIndex, value)
	return err
}

// ClearFeatureChoice drops one slot's pick entirely (e.g. a level-down or
// clan/class swap during play makes the slot no longer apply).
func ClearFeatureChoice(charDB *sql.DB, characterID int64, featureSlug string, choiceIndex int) error {
	_, err := charDB.Exec(
		`DELETE FROM character_feature_choices WHERE character_id = ? AND feature_slug = ? AND choice_index = ?`,
		characterID, featureSlug, choiceIndex)
	return err
}

// AbilityPick mirrors internal/features.AbilityPick — charstore can't import
// internal/features itself (features already documents charsheet/charstore
// as its own callers, the other direction), so the shape is duplicated here
// rather than introducing an import cycle for one small struct. Callers pass
// internal/features.AbilityPick values across as this type field-for-field.
type AbilityPick struct {
	Ability string
	Amount  int
}

// SetAbilityScoreImprovement records (or changes) one class's Ability Score
// Improvement pick at one level breakpoint — ref is
// internal/features.ASISlot's own Ref (class slug + breakpoint), which
// already keeps two different classes reaching the same numeric level from
// colliding. character_ability_bonuses has no unique index to upsert
// against (every other writer into it does a bulk source_kind replace, not
// a per-row one — see SetClan), so this does its own delete-then-insert by
// ref inside a transaction: re-resolving the SAME breakpoint replaces the
// old pick instead of stacking on top of it. picks is 1 or 2 entries — see
// internal/features.ValidASIPicks for the two shapes the book allows
// ("+1 to two different scores" or "+2 to one score"); this function trusts
// the caller already validated picks and only enforces the ref-uniqueness
// half of the invariant itself.
//
// Also clears any character_asi_feat_choices row for this ref (and the Feat
// it points at, from character_feats) first — a character re-resolving this
// breakpoint as an Ability Score Improvement after previously taking the
// Feat alternative should end up with ONLY the ability increase, not both.
func SetAbilityScoreImprovement(charDB *sql.DB, characterID int64, ref string, picks []AbilityPick) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := clearASIFeatChoice(tx, characterID, ref); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'asi' AND source_ref = ?`,
		characterID, ref,
	); err != nil {
		return err
	}
	for _, p := range picks {
		if _, err := tx.Exec(
			`INSERT INTO character_ability_bonuses (character_id, source_kind, source_ref, ability, amount)
			 VALUES (?, 'asi', ?, ?, ?)`,
			characterID, ref, p.Ability, p.Amount,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetAbilityScoreImprovementFeat records (or changes) one ASI breakpoint's
// pick as the Feat alternative rather than an ability increase — ref is the
// same internal/features.ASISlot.Ref SetAbilityScoreImprovement uses.
// Clears any existing ability-increase rows for this ref (switching from
// that branch to this one), clears any PRIOR feat choice for this same ref
// (and its own character_feats row, so switching from one feat to another
// doesn't leave the old one behind), then records the new linkage and grants
// the feat. source='asi' on the character_feats row (unlike the Feats tab's
// own AddFeat, which always writes 'other') so this pick is distinguishable
// as ASI-granted if ever audited directly against that table.
func SetAbilityScoreImprovementFeat(charDB *sql.DB, characterID int64, ref, featSlug string, level int) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'asi' AND source_ref = ?`,
		characterID, ref,
	); err != nil {
		return err
	}
	if err := clearASIFeatChoice(tx, characterID, ref); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO character_asi_feat_choices (character_id, ref, feat_slug) VALUES (?, ?, ?)`,
		characterID, ref, featSlug,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO character_feats (character_id, feat_slug, chosen_at_level, source)
		 VALUES (?, ?, ?, 'asi')`,
		characterID, featSlug, level,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearASIFeatChoiceByFeatSlug drops any character_asi_feat_choices row
// pointing at featSlug. Called by the ordinary Feats-tab delete handler
// (DeleteFeat is a separate call, made alongside this one) so removing an
// ASI-granted feat through that tab — rather than by switching the pending
// choice itself — still reopens its breakpoint as pending; without this, the
// linkage row would dangle, pointing at a feat_slug no longer in
// character_feats, and LoadResolvedASIRefs would keep reporting the
// breakpoint as resolved with no feat anywhere to show for it.
func ClearASIFeatChoiceByFeatSlug(charDB *sql.DB, characterID int64, featSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_asi_feat_choices WHERE character_id = ? AND feat_slug = ?`,
		characterID, featSlug)
	return err
}

// clearASIFeatChoice drops ref's character_asi_feat_choices row, if any, and
// the character_feats row it pointed at — shared by both Set* functions
// above so a switch in either direction (ability<->feat, or feat A -> feat
// B) never leaves a stale feat grant or a dangling linkage row behind.
func clearASIFeatChoice(tx *sql.Tx, characterID int64, ref string) error {
	var oldFeatSlug string
	err := tx.QueryRow(
		`SELECT feat_slug FROM character_asi_feat_choices WHERE character_id = ? AND ref = ?`,
		characterID, ref,
	).Scan(&oldFeatSlug)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM character_asi_feat_choices WHERE character_id = ? AND ref = ?`,
		characterID, ref,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM character_feats WHERE character_id = ? AND feat_slug = ?`,
		characterID, oldFeatSlug,
	); err != nil {
		return err
	}
	// The old feat may have applied its own ability bonus (see
	// ApplyFeatAbilityBonus in feat_effects.go) — without this, switching
	// away from it here (ability<->feat, or feat A -> feat B) would leave
	// that bonus behind with no feat anywhere to show for it, the same
	// dangling-effect shape ClearASIFeatChoiceByFeatSlug's own doc comment
	// describes for the Feats-tab delete path.
	if _, err := tx.Exec(
		`DELETE FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ?`,
		characterID, oldFeatSlug,
	); err != nil {
		return err
	}
	// Same dangling-effect reasoning for a skill proficiency the old feat may
	// have granted (see ApplyFeatSkillProficiency in feat_effects.go).
	_, err = tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = ? AND kind = 'skill'`,
		characterID, oldFeatSlug,
	)
	return err
}
