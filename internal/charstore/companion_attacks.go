package charstore

import (
	"database/sql"
	"fmt"
	"strconv"
)

// CompanionAttack is one structured, rollable attack belonging to a
// companion — see migration 0020_companion_attacks.sql. Same shape as
// CustomAttack, minus Kind (a companion has one unified attack list).
type CompanionAttack struct {
	ID          int64
	CompanionID int64
	Name        string
	Description string

	AttackAbility string
	AttackProf    string
	AttackBonus   int

	DamageCount   int
	DamageSides   int
	DamageAbility string
	DamageBonus   int
	DamageType    string

	SortOrder int

	// NoAttackRoll marks a stored row as damage/save-only, no attack roll of
	// its own — see migration 0083_companion_attack_no_roll.sql. False for
	// every row a player adds by hand through the ordinary "Add an attack"
	// form, which has no control for this; only ensureWhelpCompanion
	// (cmd/n5e/wow_whelp.go) sets it true today, for Aether Breath/Aether
	// Meteor.
	NoAttackRoll bool
}

// DamageDice renders the stored parts back as dice notation ("2d6") for
// display — empty when the row has no damage roll, mirroring
// CustomAttack.DamageDice.
func (a CompanionAttack) DamageDice() string {
	if a.DamageCount <= 0 || a.DamageSides <= 0 {
		return ""
	}
	return strconv.Itoa(a.DamageCount) + "d" + strconv.Itoa(a.DamageSides)
}

// AddCompanionAttack records one attack, ownership-scoped through
// character_companions the same way AddCompanionUpgrade is.
func AddCompanionAttack(charDB *sql.DB, characterID, companionID int64, a CompanionAttack) (int64, error) {
	var nextOrder int
	if err := charDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM character_companion_attacks WHERE companion_id = ?`,
		companionID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	noAttackRoll := 0
	if a.NoAttackRoll {
		noAttackRoll = 1
	}
	res, err := charDB.Exec(`
		INSERT INTO character_companion_attacks
			(companion_id, name, description, attack_ability, attack_prof, attack_bonus,
			 damage_count, damage_sides, damage_ability, damage_bonus, damage_type, sort_order, no_attack_roll)
		SELECT id, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		FROM character_companions WHERE id = ? AND character_id = ?`,
		a.Name, a.Description, a.AttackAbility, a.AttackProf, a.AttackBonus,
		nullableCount(a.DamageCount), nullableCount(a.DamageSides),
		a.DamageAbility, a.DamageBonus, a.DamageType, nextOrder, noAttackRoll,
		companionID, characterID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert companion attack: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, sql.ErrNoRows
	}
	return res.LastInsertId()
}

// DeleteCompanionAttack removes one attack, ownership-scoped the same way.
func DeleteCompanionAttack(charDB *sql.DB, characterID, companionID, attackID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_attacks
		WHERE id = ? AND companion_id IN (
			SELECT id FROM character_companions WHERE id = ? AND character_id = ?
		)`,
		attackID, companionID, characterID)
	return err
}

// ListCompanionAttacks returns one companion's attacks in display order,
// ownership-scoped the same way.
func ListCompanionAttacks(charDB *sql.DB, characterID, companionID int64) ([]CompanionAttack, error) {
	rows, err := charDB.Query(`
		SELECT a.id, a.companion_id, a.name, a.description, a.attack_ability, a.attack_prof, a.attack_bonus,
		       a.damage_count, a.damage_sides, a.damage_ability, a.damage_bonus, a.damage_type, a.sort_order,
		       a.no_attack_roll
		FROM character_companion_attacks a
		JOIN character_companions c ON c.id = a.companion_id
		WHERE a.companion_id = ? AND c.character_id = ?
		ORDER BY a.sort_order, a.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionAttack
	for rows.Next() {
		var a CompanionAttack
		var count, sides sql.NullInt64
		var noAttackRoll int
		if err := rows.Scan(&a.ID, &a.CompanionID, &a.Name, &a.Description, &a.AttackAbility, &a.AttackProf, &a.AttackBonus,
			&count, &sides, &a.DamageAbility, &a.DamageBonus, &a.DamageType, &a.SortOrder,
			&noAttackRoll); err != nil {
			return nil, err
		}
		a.DamageCount = int(count.Int64)
		a.DamageSides = int(sides.Int64)
		a.NoAttackRoll = noAttackRoll != 0
		out = append(out, a)
	}
	return out, rows.Err()
}
