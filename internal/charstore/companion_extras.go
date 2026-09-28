package charstore

import (
	"database/sql"
	"fmt"
	"strconv"
)

// companion_extras.go: the companion-owned side tables that hang off
// companions.go's core rows — upgrades, seals, upgrade choices, structured
// attacks, and the feature-driven companion choices that grant them.

// CompanionUpgrade is one Puppet Upgrade a companion has taken — see
// migration 0019_puppet_upgrades.sql.
type CompanionUpgrade struct {
	ID               int64
	CompanionID      int64
	ClassOptionSlug  string
	UpgradeEntrySlug string
}

// AddCompanionUpgrade records one upgrade pick, scoped through
// character_companions by characterID so a forged companionID from another
// character can't be touched — same ownership-scoping shape
// GetCompanion/DeleteCompanion already use.
func AddCompanionUpgrade(charDB *sql.DB, characterID, companionID int64, classOptionSlug, upgradeEntrySlug string) (int64, error) {
	res, err := charDB.Exec(`
		INSERT INTO character_companion_upgrades (companion_id, class_option_slug, upgrade_entry_slug)
		SELECT id, ?, ? FROM character_companions WHERE id = ? AND character_id = ?`,
		classOptionSlug, upgradeEntrySlug, companionID, characterID)
	if err != nil {
		return 0, err
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

// DeleteCompanionUpgrade removes one pick, ownership-scoped the same way.
func DeleteCompanionUpgrade(charDB *sql.DB, characterID, companionID, upgradeID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_upgrades
		WHERE id = ? AND companion_id IN (
			SELECT id FROM character_companions WHERE id = ? AND character_id = ?
		)`,
		upgradeID, companionID, characterID)
	return err
}

// ListCompanionUpgrades returns one companion's picks in the order taken,
// ownership-scoped the same way.
func ListCompanionUpgrades(charDB *sql.DB, characterID, companionID int64) ([]CompanionUpgrade, error) {
	rows, err := charDB.Query(`
		SELECT u.id, u.companion_id, u.class_option_slug, u.upgrade_entry_slug
		FROM character_companion_upgrades u
		JOIN character_companions c ON c.id = u.companion_id
		WHERE u.companion_id = ? AND c.character_id = ?
		ORDER BY u.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionUpgrade
	for rows.Next() {
		var u CompanionUpgrade
		if err := rows.Scan(&u.ID, &u.CompanionID, &u.ClassOptionSlug, &u.UpgradeEntrySlug); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CompanionSeal is one Armor or Weapon seal equipped on a companion via
// Chakra Enhanced Retrofit (Class: Puppet Master/3rd Level) — see migration
// 0025_companion_seals.sql. SealSlug is a rules.db equipment slug
// (kind='enhancement_seal'), same cross-DB slug-reference tolerance the
// rest of this app's companion tables already use.
type CompanionSeal struct {
	ID       int64
	SealSlug string
}

// AddCompanionSeal records one equipped seal, ownership-scoped by
// characterID AND companionID so a forged companionID belonging to another
// character can't be touched.
func AddCompanionSeal(charDB *sql.DB, characterID, companionID int64, sealSlug string) (int64, error) {
	res, err := charDB.Exec(`
		INSERT INTO character_companion_seals (companion_id, seal_slug)
		SELECT c.id, ? FROM character_companions c
		WHERE c.id = ? AND c.character_id = ?`,
		sealSlug, companionID, characterID)
	if err != nil {
		return 0, err
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

// DeleteCompanionSeal removes one equipped seal, ownership-scoped the same
// way.
func DeleteCompanionSeal(charDB *sql.DB, characterID, companionID, sealID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_seals
		WHERE id = ? AND companion_id IN (
			SELECT c.id FROM character_companions c
			WHERE c.id = ? AND c.character_id = ?
		)`,
		sealID, companionID, characterID)
	return err
}

// ListCompanionSeals returns every seal equipped on this companion,
// ownership-scoped the same way.
func ListCompanionSeals(charDB *sql.DB, characterID, companionID int64) ([]CompanionSeal, error) {
	rows, err := charDB.Query(`
		SELECT s.id, s.seal_slug
		FROM character_companion_seals s
		JOIN character_companions c ON c.id = s.companion_id
		WHERE c.id = ? AND c.character_id = ?
		ORDER BY s.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionSeal
	for rows.Next() {
		var s CompanionSeal
		if err := rows.Scan(&s.ID, &s.SealSlug); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CompanionUpgradeChoice is one sub-pick under a repeatable upgrade (e.g.
// one of the up-to-3 Poison Tag qualities Poison Mist Hell lets a player
// install) — see migration 0021_companion_upgrade_choices.sql.
type CompanionUpgradeChoice struct {
	ID                 int64
	CompanionUpgradeID int64
	ChoiceSlug         string
}

// AddCompanionUpgradeChoice records one sub-pick, scoped through
// character_companion_upgrades → character_companions by characterID AND
// companionID so a forged companionUpgradeID belonging to another
// companion or character can't be touched.
func AddCompanionUpgradeChoice(charDB *sql.DB, characterID, companionID, companionUpgradeID int64, choiceSlug string) (int64, error) {
	res, err := charDB.Exec(`
		INSERT INTO character_companion_upgrade_choices (companion_upgrade_id, choice_slug)
		SELECT u.id, ? FROM character_companion_upgrades u
		JOIN character_companions c ON c.id = u.companion_id
		WHERE u.id = ? AND c.id = ? AND c.character_id = ?`,
		choiceSlug, companionUpgradeID, companionID, characterID)
	if err != nil {
		return 0, err
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

// DeleteCompanionUpgradeChoice removes one sub-pick, ownership-scoped the
// same way.
func DeleteCompanionUpgradeChoice(charDB *sql.DB, characterID, companionID, choiceID int64) error {
	_, err := charDB.Exec(`
		DELETE FROM character_companion_upgrade_choices
		WHERE id = ? AND companion_upgrade_id IN (
			SELECT u.id FROM character_companion_upgrades u
			JOIN character_companions c ON c.id = u.companion_id
			WHERE c.id = ? AND c.character_id = ?
		)`,
		choiceID, companionID, characterID)
	return err
}

// ListCompanionUpgradeChoices returns every sub-pick recorded under any of
// this companion's upgrade picks, ownership-scoped the same way — callers
// group by CompanionUpgradeID themselves (see cmd/n5e/puppets.go), same
// shape ListCompanionUpgrades already leaves to its own callers.
func ListCompanionUpgradeChoices(charDB *sql.DB, characterID, companionID int64) ([]CompanionUpgradeChoice, error) {
	rows, err := charDB.Query(`
		SELECT ch.id, ch.companion_upgrade_id, ch.choice_slug
		FROM character_companion_upgrade_choices ch
		JOIN character_companion_upgrades u ON u.id = ch.companion_upgrade_id
		JOIN character_companions c ON c.id = u.companion_id
		WHERE c.id = ? AND c.character_id = ?
		ORDER BY ch.id`,
		companionID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompanionUpgradeChoice
	for rows.Next() {
		var c CompanionUpgradeChoice
		if err := rows.Scan(&c.ID, &c.CompanionUpgradeID, &c.ChoiceSlug); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

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

// SetFeatureCompanionChoice records (or changes) a player's pick for one
// companion-scoped choice slot — see 0033_feature_companion_choices.sql for
// what feature_slug/companion_id/choice_index mean together. An upsert, the
// same shape SetFeatureChoice already uses for its character-scoped sibling.
func SetFeatureCompanionChoice(charDB *sql.DB, characterID int64, featureSlug string, companionID int64, choiceIndex int, value string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_feature_companion_choices (character_id, feature_slug, companion_id, choice_index, value) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (character_id, feature_slug, companion_id, choice_index) DO UPDATE SET value = excluded.value`,
		characterID, featureSlug, companionID, choiceIndex, value)
	return err
}

// GetFeatureCompanionChoice returns a companion-scoped pick's stored value,
// or "" if it hasn't been made yet.
func GetFeatureCompanionChoice(charDB *sql.DB, characterID int64, featureSlug string, companionID int64, choiceIndex int) (string, error) {
	var value string
	err := charDB.QueryRow(
		`SELECT value FROM character_feature_companion_choices
		 WHERE character_id = ? AND feature_slug = ? AND companion_id = ? AND choice_index = ?`,
		characterID, featureSlug, companionID, choiceIndex,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ListFeatureCompanionChoices returns every companion's own resolved pick
// for one (feature_slug, choice_index) slot at once, keyed by companion_id —
// for a caller resolving a whole roster's worth of picks together instead
// of one GetFeatureCompanionChoice call per companion.
func ListFeatureCompanionChoices(charDB *sql.DB, characterID int64, featureSlug string, choiceIndex int) (map[int64]string, error) {
	rows, err := charDB.Query(
		`SELECT companion_id, value FROM character_feature_companion_choices
		 WHERE character_id = ? AND feature_slug = ? AND choice_index = ?`,
		characterID, featureSlug, choiceIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var companionID int64
		var value string
		if err := rows.Scan(&companionID, &value); err != nil {
			return nil, err
		}
		out[companionID] = value
	}
	return out, rows.Err()
}
