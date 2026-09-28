package charstore

import (
	"database/sql"
)

// weapon_features.go: the small per-feature weapon/martial tables —
// Superior Weapon Flurry, Weapon Focus, Martial Defense seals, and Hunter
// Pattern proficiency. Each is a handful of rows keyed by character.

// AddSuperiorWeaponFlurryBenefit records a Weapon Specialist selecting one
// of Superior Weapon Flurry's own 4 named benefits (see cmd/n5e/
// weapon_specialist.go — the catalog is a small hardcoded Go slice, not a
// rules.db table; no class_options rows exist for these 4 benefits). A
// duplicate add is a silent no-op — the UNIQUE constraint on (character_id,
// option_slug) makes this safe, and the picker's own cap check is what
// actually decides whether a NEW benefit can be added, not this.
func AddSuperiorWeaponFlurryBenefit(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_superior_weapon_flurry (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveSuperiorWeaponFlurryBenefit drops one selected benefit — freely, at
// any time. This app doesn't enforce rest/level-up timing, same boundary
// Martial Techniques and Weapon Form Styles already draw.
func RemoveSuperiorWeaponFlurryBenefit(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_superior_weapon_flurry WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListSuperiorWeaponFlurryBenefits returns the option_slugs of every
// Superior Weapon Flurry benefit a character has selected.
func ListSuperiorWeaponFlurryBenefits(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_superior_weapon_flurry WHERE character_id = ?`, characterID)
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

// AddWeaponFocus records a Weapon Specialist choosing one weapon type
// (equipment.slug) as a Weapon Focus (see cmd/n5e/weapon_specialist.go). A
// duplicate add is a silent no-op — the UNIQUE constraint on (character_id,
// weapon_slug) makes this safe, and the picker's own slot-cap check is what
// actually decides whether a NEW focus can be added, not this.
func AddWeaponFocus(charDB *sql.DB, characterID int64, weaponSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_weapon_focus (character_id, weapon_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, weapon_slug) DO NOTHING`,
		characterID, weaponSlug)
	return err
}

// RemoveWeaponFocus drops one chosen Weapon Focus type — freely, at any
// time. The book's own text gates re-picking to "you cannot go back and
// change your selection"; this app doesn't enforce that, same boundary
// Mastery/Puppet Tactics/Martial Techniques already draw.
func RemoveWeaponFocus(charDB *sql.DB, characterID int64, weaponSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_weapon_focus WHERE character_id = ? AND weapon_slug = ?`,
		characterID, weaponSlug)
	return err
}

// ListWeaponFocus returns the equipment slugs of every weapon type a
// character has chosen as a Weapon Focus.
func ListWeaponFocus(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT weapon_slug FROM character_weapon_focus WHERE character_id = ?`, characterID)
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

// WeaponFocusSlotCap is Weapon Focus's own schedule: 1 type at 1st level, a
// 2nd at 9th, a 3rd at 15th. Exported (rather than kept local to
// cmd/n5e/weapon_specialist.go) so internal/charsheet can derive the same
// bonus for Bukijutsu jutsu-casting without duplicating the schedule.
func WeaponFocusSlotCap(level int) int {
	switch {
	case level >= 15:
		return 3
	case level >= 9:
		return 2
	case level >= 1:
		return 1
	default:
		return 0
	}
}

// WeaponFocusBonus is the shared Attack & Damage bonus ALL chosen Weapon
// Focus types get at once — the book increases one shared bonus as more
// types are added (+1 with 1 type, +2 once a 2nd is added at 9th, +3 once a
// 3rd is added at 15th), not a per-type stacking bonus.
func WeaponFocusBonus(level int) int {
	return WeaponFocusSlotCap(level)
}

// AddMartialDefenseSeal records a Taijutsu Specialist infusing one guard
// slot with an armor seal (equipment.slug, seal_applies_to = 'armor') under
// Martial Defense's 5th-level guard-slot feature (see
// cmd/n5e/martial_defense.go). A duplicate add is a silent no-op — the
// UNIQUE constraint on (character_id, seal_slug) makes this safe, and the
// picker's own slot-cap check is what actually decides whether a NEW seal
// can be added, not this.
func AddMartialDefenseSeal(charDB *sql.DB, characterID int64, sealSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_martial_defense_seals (character_id, seal_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, seal_slug) DO NOTHING`,
		characterID, sealSlug)
	return err
}

// RemoveMartialDefenseSeal drops one infused guard-slot seal — freely, at
// any time. The book gates re-infusing to "over the course of a full
// rest"; this app doesn't enforce that, same boundary Weapon Focus/
// Mastery/Puppet Tactics/Martial Techniques already draw.
func RemoveMartialDefenseSeal(charDB *sql.DB, characterID int64, sealSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_martial_defense_seals WHERE character_id = ? AND seal_slug = ?`,
		characterID, sealSlug)
	return err
}

// ListMartialDefenseSeals returns the equipment slugs of every armor seal a
// character has infused into a Martial Defense guard slot.
func ListMartialDefenseSeals(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT seal_slug FROM character_martial_defense_seals WHERE character_id = ?`, characterID)
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
