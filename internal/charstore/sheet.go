package charstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SetHP applies a signed HP delta to a character's current_hp/temp_hp,
// implementing the "temp HP absorbs damage first" rule: a negative delta is
// subtracted from temp_hp first (floored at 0), and only the remainder (if
// any) comes off current_hp (also floored at 0); a positive delta (healing)
// applies straight to current_hp. Deliberately doesn't cap current_hp
// against Sheet.MaxHP — that's derived math internal/charsheet computes,
// this function only ever applies the input, same as every other Set*
// here.
func SetHP(charDB *sql.DB, characterID int64, delta int) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentHP, tempHP int
	if err := tx.QueryRow(
		`SELECT current_hp, temp_hp FROM characters WHERE id = ?`, characterID,
	).Scan(&currentHP, &tempHP); err != nil {
		return fmt.Errorf("load hp: %w", err)
	}

	if delta < 0 {
		damage := -delta
		absorbed := damage
		if absorbed > tempHP {
			absorbed = tempHP
		}
		tempHP -= absorbed
		damage -= absorbed
		currentHP -= damage
		if currentHP < 0 {
			currentHP = 0
		}
	} else {
		currentHP += delta
	}

	if _, err := tx.Exec(
		`UPDATE characters SET current_hp = ?, temp_hp = ?, updated_at = datetime('now') WHERE id = ?`,
		currentHP, tempHP, characterID,
	); err != nil {
		return fmt.Errorf("update hp: %w", err)
	}
	return tx.Commit()
}

// SetBaseTempHP sets both base_temp_hp (the THP fraction's denominator) and
// temp_hp (its numerator) to value — granting new temp HP replaces the old
// pool rather than stacking with it, same rule 5e uses.
func SetBaseTempHP(charDB *sql.DB, characterID int64, value int) error {
	_, err := charDB.Exec(
		`UPDATE characters SET base_temp_hp = ?, temp_hp = ?, updated_at = datetime('now') WHERE id = ?`,
		value, value, characterID,
	)
	return err
}

// SetChakra applies a signed delta to current_chakra, floored at 0.
//
// Deliberately simpler than SetHP: chakra has no temp-pool equivalent to
// absorb the spend first, so there is nothing to cascade through. Like
// SetHP it does NOT cap against Sheet.MaxChakra — that maximum is derived
// math internal/charsheet computes from class dice and level (and can be
// pinned by hand), and clamping here would silently discard a gain the
// player is entitled to on a sheet whose maximum this package cannot see.
func SetChakra(charDB *sql.DB, characterID int64, delta int) error {
	_, err := charDB.Exec(
		`UPDATE characters SET current_chakra = MAX(0, current_chakra + ?), updated_at = datetime('now') WHERE id = ?`,
		delta, characterID,
	)
	return err
}

// SetInspiration sets the character's inspiration toggle.
func SetInspiration(charDB *sql.DB, characterID int64, on bool) error {
	val := 0
	if on {
		val = 1
	}
	_, err := charDB.Exec(
		`UPDATE characters SET inspiration = ?, updated_at = datetime('now') WHERE id = ?`,
		val, characterID,
	)
	return err
}

// SetBio replaces the Bio tab's five free-text fields.
func SetBio(charDB *sql.DB, characterID int64, appearance, backstory, alliesOrgs, additionalFeatures, treasure string) error {
	_, err := charDB.Exec(`
		UPDATE characters SET
			appearance = ?, backstory = ?, allies_organizations = ?,
			additional_features_text = ?, treasure = ?, updated_at = datetime('now')
		WHERE id = ?`,
		appearance, backstory, alliesOrgs, additionalFeatures, treasure, characterID,
	)
	return err
}

// SetNotes replaces the Core tab's free-text Notes box (migration 0008).
// Separate from SetBio rather than a sixth argument to it: the two are saved
// by different forms on different tabs, and folding them together would mean
// every Bio autosave rewrote the notes and every notes autosave rewrote the
// backstory.
func SetNotes(charDB *sql.DB, characterID int64, notes string) error {
	_, err := charDB.Exec(
		`UPDATE characters SET notes = ?, updated_at = datetime('now') WHERE id = ?`,
		notes, characterID,
	)
	return err
}

// CustomFeature is one player-added row alongside the Core tab's auto-seeded
// class/clan features.
type CustomFeature struct {
	ID          int64
	Name        string
	SourceLabel string
	Description string
	SortOrder   int
}

// AddCustomFeature appends one custom feature, sorted after every existing
// one for this character.
func AddCustomFeature(charDB *sql.DB, characterID int64, name, sourceLabel, description string) (int64, error) {
	var nextOrder int
	if err := charDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM character_custom_features WHERE character_id = ?`,
		characterID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := charDB.Exec(
		`INSERT INTO character_custom_features (character_id, name, source_label, description, sort_order)
		 VALUES (?, ?, ?, ?, ?)`,
		characterID, name, sourceLabel, description, nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert custom feature: %w", err)
	}
	return res.LastInsertId()
}

// DeleteCustomFeature removes one custom feature, scoped to characterID so
// a stale/forged featureID can't touch another character's row.
func DeleteCustomFeature(charDB *sql.DB, characterID, featureID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_custom_features WHERE id = ? AND character_id = ?`,
		featureID, characterID,
	)
	return err
}

// ListCustomFeatures returns a character's custom features in display order.
func ListCustomFeatures(charDB *sql.DB, characterID int64) ([]CustomFeature, error) {
	rows, err := charDB.Query(
		`SELECT id, name, source_label, description, sort_order
		 FROM character_custom_features WHERE character_id = ? ORDER BY sort_order`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CustomFeature
	for rows.Next() {
		var f CustomFeature
		if err := rows.Scan(&f.ID, &f.Name, &f.SourceLabel, &f.Description, &f.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CustomAttack is one player-defined row in the sheet's Attacks & Jutsu
// block — see migration 0007_custom_attacks.sql for why these
// exist at all.
//
// DamageCount == 0 means the row has no damage roll, only a to-hit.
//
// AttackBonus and DamageBonus are the FLAT parts of their modifiers, not the
// whole thing — see migration 0009. The totals are composed at render time
// from AttackAbility/AttackProf/AttackBonus and DamageAbility/DamageBonus, so
// a level-up or an ability-score change moves them on its own instead of
// leaving a stale number the player typed once.
type CustomAttack struct {
	ID   int64
	Kind string // "weapon" or "jutsu" — which table the row renders in
	Name string

	AttackAbility string // three-letter code, "" for no ability term
	AttackProf    string // charsheet.ProfNone / ProfHalf / ProfFull
	AttackBonus   int    // flat extra on top of ability + proficiency

	DamageCount   int
	DamageSides   int
	DamageAbility string // three-letter code, "" for no ability term
	DamageBonus   int    // flat extra on top of the ability
	DamageType    string

	Notes     string
	SortOrder int
}

// DamageDice renders the stored parts back as dice notation ("2d6") for
// display. Empty when the row has no damage roll.
func (a CustomAttack) DamageDice() string {
	if a.DamageCount <= 0 || a.DamageSides <= 0 {
		return ""
	}
	return strconv.Itoa(a.DamageCount) + "d" + strconv.Itoa(a.DamageSides)
}

// AddCustomAttack appends one custom attack, sorted after every existing one
// of the same kind for this character.
func AddCustomAttack(charDB *sql.DB, characterID int64, a CustomAttack) (int64, error) {
	var nextOrder int
	if err := charDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM character_custom_attacks WHERE character_id = ? AND kind = ?`,
		characterID, a.Kind,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := charDB.Exec(`
		INSERT INTO character_custom_attacks
			(character_id, kind, name, attack_ability, attack_prof, attack_bonus,
			 damage_count, damage_sides, damage_ability, damage_bonus, damage_type, notes, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		characterID, a.Kind, a.Name, a.AttackAbility, a.AttackProf, a.AttackBonus,
		nullableCount(a.DamageCount), nullableCount(a.DamageSides),
		a.DamageAbility, a.DamageBonus, a.DamageType, a.Notes, nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert custom attack: %w", err)
	}
	return res.LastInsertId()
}

// nullableCount stores a missing damage die as NULL rather than 0, so the
// column reads as "no damage roll" rather than as "zero dice" — the
// distinction the migration's comment describes.
func nullableCount(n int) any {
	if n <= 0 {
		return nil
	}
	return n
}

// UpdateCustomAttack rewrites one custom attack in place, scoped to
// characterID for the same reason DeleteCustomAttack is.
//
// kind is rewritten along with the numbers, so changing a row from a weapon
// to a jutsu moves it between the two tables — that is the point of the
// weapon/jutsu choice being editable rather than fixed at creation. sort_order
// is deliberately left alone: an edit is not a re-add, and a row jumping to
// the bottom of its table on every tweak would be its own annoyance.
func UpdateCustomAttack(charDB *sql.DB, characterID int64, a CustomAttack) error {
	_, err := charDB.Exec(`
		UPDATE character_custom_attacks
		SET kind = ?, name = ?, attack_ability = ?, attack_prof = ?, attack_bonus = ?,
		    damage_count = ?, damage_sides = ?, damage_ability = ?,
		    damage_bonus = ?, damage_type = ?, notes = ?
		WHERE id = ? AND character_id = ?`,
		a.Kind, a.Name, a.AttackAbility, a.AttackProf, a.AttackBonus,
		nullableCount(a.DamageCount), nullableCount(a.DamageSides),
		a.DamageAbility, a.DamageBonus, a.DamageType, a.Notes,
		a.ID, characterID,
	)
	return err
}

// DeleteCustomAttack removes one custom attack, scoped to characterID so a
// stale or forged id cannot touch another character's row.
func DeleteCustomAttack(charDB *sql.DB, characterID, attackID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_custom_attacks WHERE id = ? AND character_id = ?`,
		attackID, characterID,
	)
	return err
}

// ListCustomAttacks returns a character's custom attacks of one kind in
// display order.
func ListCustomAttacks(charDB *sql.DB, characterID int64, kind string) ([]CustomAttack, error) {
	rows, err := charDB.Query(`
		SELECT id, kind, name, attack_ability, attack_prof, attack_bonus,
		       damage_count, damage_sides, damage_ability, damage_bonus,
		       COALESCE(damage_type, ''), COALESCE(notes, ''), sort_order
		FROM character_custom_attacks WHERE character_id = ? AND kind = ? ORDER BY sort_order, id`,
		characterID, kind,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CustomAttack
	for rows.Next() {
		var a CustomAttack
		var count, sides sql.NullInt64
		if err := rows.Scan(&a.ID, &a.Kind, &a.Name, &a.AttackAbility, &a.AttackProf, &a.AttackBonus,
			&count, &sides, &a.DamageAbility, &a.DamageBonus,
			&a.DamageType, &a.Notes, &a.SortOrder); err != nil {
			return nil, err
		}
		a.DamageCount, a.DamageSides = int(count.Int64), int(sides.Int64)
		out = append(out, a)
	}
	return out, rows.Err()
}

// WeaponAttackOptions overrides how one equipped weapon's attack row is
// computed. Every field is optional and independently so: an empty ability
// means "keep whatever the weapon's printed properties imply" (buildAttacks
// reads finesse/thrown/ammunition to pick Strength or Dexterity), and an empty
// damage ability means "the same one the attack uses". See migration 0009.
type WeaponAttackOptions struct {
	InventoryID   int64
	AttackAbility string
	AttackProf    string // charsheet.ProfNone / ProfHalf / ProfFull; "" reads as full
	AttackBonus   int
	DamageAbility string
	DamageBonus   int
}

// ListWeaponAttackOptions returns every weapon override this character has, by
// inventory row id. Weapons with no row simply aren't in the map — the caller
// keeps its derived defaults for those.
func ListWeaponAttackOptions(charDB *sql.DB, characterID int64) (map[int64]WeaponAttackOptions, error) {
	rows, err := charDB.Query(`
		SELECT inventory_id, attack_ability, attack_prof, attack_bonus, damage_ability, damage_bonus
		FROM character_weapon_attack_options WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]WeaponAttackOptions{}
	for rows.Next() {
		var o WeaponAttackOptions
		if err := rows.Scan(&o.InventoryID, &o.AttackAbility, &o.AttackProf,
			&o.AttackBonus, &o.DamageAbility, &o.DamageBonus); err != nil {
			return nil, err
		}
		out[o.InventoryID] = o
	}
	return out, rows.Err()
}

// SetWeaponAttackOptions writes one weapon's overrides, replacing whatever was
// there. An all-default row is deleted rather than stored, so "put it back the
// way it was" leaves no trace and the weapon goes back to being fully derived.
func SetWeaponAttackOptions(charDB *sql.DB, characterID int64, o WeaponAttackOptions) error {
	if o.AttackAbility == "" && (o.AttackProf == "" || o.AttackProf == "full") &&
		o.AttackBonus == 0 && o.DamageAbility == "" && o.DamageBonus == 0 {
		_, err := charDB.Exec(
			`DELETE FROM character_weapon_attack_options WHERE character_id = ? AND inventory_id = ?`,
			characterID, o.InventoryID)
		return err
	}
	if o.AttackProf == "" {
		o.AttackProf = "full"
	}
	_, err := charDB.Exec(`
		INSERT INTO character_weapon_attack_options
			(character_id, inventory_id, attack_ability, attack_prof, attack_bonus, damage_ability, damage_bonus)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (character_id, inventory_id) DO UPDATE SET
			attack_ability = excluded.attack_ability,
			attack_prof    = excluded.attack_prof,
			attack_bonus   = excluded.attack_bonus,
			damage_ability = excluded.damage_ability,
			damage_bonus   = excluded.damage_bonus`,
		characterID, o.InventoryID, o.AttackAbility, o.AttackProf,
		o.AttackBonus, o.DamageAbility, o.DamageBonus,
	)
	return err
}

// JutsuOptions overrides how one known jutsu's attack and damage are rolled.
// Every field is optional: an empty attack ability keeps whichever attack kind
// the jutsu's own description names, and DamageCount == 0 means the row has no
// damage roll — which is the default, since the book prints jutsu damage in
// prose rather than as dice. See migration 0010.
type JutsuOptions struct {
	Slug          string
	AttackAbility string
	AttackProf    string // charsheet.ProfNone / ProfHalf / ProfFull; "" reads as full
	AttackBonus   int
	DamageCount   int
	DamageSides   int
	DamageAbility string
	DamageBonus   int
	DamageType    string
}

// IsDefault reports whether these options say nothing at all, in which case
// storing them would only be a row that has to be read back and ignored.
func (o JutsuOptions) IsDefault() bool {
	return o.AttackAbility == "" && (o.AttackProf == "" || o.AttackProf == "full") &&
		o.AttackBonus == 0 && o.DamageCount <= 0 && o.DamageSides <= 0 &&
		o.DamageAbility == "" && o.DamageBonus == 0 && o.DamageType == ""
}

// ListJutsuOptions returns every jutsu override this character has, by slug.
// Jutsu with no row simply aren't in the map — the caller keeps its derived
// defaults for those.
func ListJutsuOptions(charDB *sql.DB, characterID int64) (map[string]JutsuOptions, error) {
	rows, err := charDB.Query(`
		SELECT jutsu_slug, attack_ability, attack_prof, attack_bonus,
		       damage_count, damage_sides, damage_ability, damage_bonus, COALESCE(damage_type, '')
		FROM character_jutsu_options WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]JutsuOptions{}
	for rows.Next() {
		var o JutsuOptions
		var count, sides sql.NullInt64
		if err := rows.Scan(&o.Slug, &o.AttackAbility, &o.AttackProf, &o.AttackBonus,
			&count, &sides, &o.DamageAbility, &o.DamageBonus, &o.DamageType); err != nil {
			return nil, err
		}
		o.DamageCount, o.DamageSides = int(count.Int64), int(sides.Int64)
		out[o.Slug] = o
	}
	return out, rows.Err()
}

// SetJutsuOptions writes one jutsu's overrides, replacing whatever was there.
// An all-default set is deleted rather than stored, so putting a jutsu back the
// way it was leaves no trace and it goes back to being fully derived.
func SetJutsuOptions(charDB *sql.DB, characterID int64, o JutsuOptions) error {
	if o.IsDefault() {
		return DeleteJutsuOptions(charDB, characterID, o.Slug)
	}
	if o.AttackProf == "" {
		o.AttackProf = "full"
	}
	_, err := charDB.Exec(`
		INSERT INTO character_jutsu_options
			(character_id, jutsu_slug, attack_ability, attack_prof, attack_bonus,
			 damage_count, damage_sides, damage_ability, damage_bonus, damage_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (character_id, jutsu_slug) DO UPDATE SET
			attack_ability = excluded.attack_ability,
			attack_prof    = excluded.attack_prof,
			attack_bonus   = excluded.attack_bonus,
			damage_count   = excluded.damage_count,
			damage_sides   = excluded.damage_sides,
			damage_ability = excluded.damage_ability,
			damage_bonus   = excluded.damage_bonus,
			damage_type    = excluded.damage_type`,
		characterID, o.Slug, o.AttackAbility, o.AttackProf, o.AttackBonus,
		nullableCount(o.DamageCount), nullableCount(o.DamageSides),
		o.DamageAbility, o.DamageBonus, o.DamageType,
	)
	return err
}

// DeleteJutsuOptions drops one jutsu's overrides. Called when the jutsu itself
// is forgotten: character_jutsu_options is keyed by slug, not by a
// character_jutsu row id, so there is nothing for SQLite to cascade from.
func DeleteJutsuOptions(charDB *sql.DB, characterID int64, slug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_jutsu_options WHERE character_id = ? AND jutsu_slug = ?`,
		characterID, slug)
	return err
}

// AddFeat records that a character has taken a feat.
//
// character_feats has existed since 0001_init.sql — schema-ready for a flow
// that was never built — so nothing new is created here; the sheet's Feats
// tab is simply the first thing to write to it. source='other' because a
// feat dragged onto the sheet by hand is exactly that: not an ASI pick the
// app tracked, not a background or class grant it derived.
//
// Idempotent: taking the same feat twice is not a thing, and a repeated drop
// onto the pane should be a no-op rather than an error.
func AddFeat(charDB *sql.DB, characterID int64, featSlug string, level int) error {
	_, err := charDB.Exec(
		`INSERT OR IGNORE INTO character_feats (character_id, feat_slug, chosen_at_level, source)
		 VALUES (?, ?, ?, 'other')`,
		characterID, featSlug, level,
	)
	return err
}

// DeleteFeat removes one taken feat, scoped to the character.
func DeleteFeat(charDB *sql.DB, characterID int64, featSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_feats WHERE character_id = ? AND feat_slug = ?`,
		characterID, featSlug,
	)
	return err
}

// AddJutsu records one learned jutsu on the sheet, outside the creation
// step's set-the-whole-list SetJutsu.
//
// source is 'learned', matching what SetJutsu writes, so a jutsu added from
// the sheet and one picked during creation are the same kind of row —
// otherwise revisiting the creation step (which reads back source='learned')
// would silently drop everything added later. Idempotent on the slug.
func AddJutsu(charDB *sql.DB, characterID int64, jutsuSlug string, level int) error {
	var exists int
	if err := charDB.QueryRow(
		`SELECT COUNT(*) FROM character_jutsu WHERE character_id = ? AND jutsu_slug = ?`,
		characterID, jutsuSlug,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check existing jutsu: %w", err)
	}
	if exists > 0 {
		return nil
	}
	_, err := charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug, learned_at_level, source) VALUES (?, ?, ?, 'learned')`,
		characterID, jutsuSlug, level,
	)
	return err
}

// SetProficiencyToggle turns one character_proficiencies grant on or off by
// hand from the sheet. Turning it on inserts a single row tagged
// source_kind='other', source_ref='manual'. Turning it off deletes EVERY
// row matching (character_id, kind, value) regardless of source — this
// table has no "auto-granted vs manually toggled" distinction once written,
// so toggling off a class-granted skill removes its provenance row too,
// same as any other row here. Matches the literal ask (let the player
// toggle proficiencies on/off from the sheet); revisit if finer-grained
// tracking (e.g. re-deriving a class grant after an off-toggle) is wanted
// later.
func SetProficiencyToggle(charDB *sql.DB, characterID int64, kind, value string, on bool) error {
	if !on {
		_, err := charDB.Exec(
			`DELETE FROM character_proficiencies WHERE character_id = ? AND kind = ? AND value = ?`,
			characterID, kind, value,
		)
		return err
	}
	_, err := charDB.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
		 VALUES (?, ?, ?, 'other', 'manual')`,
		characterID, kind, value,
	)
	return err
}

// AddCustomProficiency adds one player-typed tool/skill/language proficiency
// from the sheet's "+" panels — same insert SetProficiencyToggle's "on"
// branch does, exposed under its own name since the panels add new values
// rather than toggling an existing one.
func AddCustomProficiency(charDB *sql.DB, characterID int64, kind, value string) error {
	return SetProficiencyToggle(charDB, characterID, kind, value, true)
}

// SetProficiencyMod writes or clears one item's roll tweak in the Tool
// Proficiencies & Custom Skills box (ability + proficiency share + flat
// bonus, same shape as Initiative — see charsheet.ComposeModifier). Keyed by
// the item's displayed name rather than a character_proficiencies row id;
// see 0011_proficiency_mods.sql for why.
//
// All-default (no ability, no proficiency, zero bonus) deletes the row
// instead of storing a no-op, the same "clear rather than store a blank"
// rule SetOverride follows.
func SetProficiencyMod(charDB *sql.DB, characterID int64, kind, value, ability, profMode string, bonus int) error {
	if ability == "" && profMode == "none" && bonus == 0 {
		_, err := charDB.Exec(
			`DELETE FROM character_proficiency_mods WHERE character_id = ? AND kind = ? AND value = ?`,
			characterID, kind, value,
		)
		return err
	}
	_, err := charDB.Exec(`
		INSERT INTO character_proficiency_mods (character_id, kind, value, ability, prof_mode, bonus)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (character_id, kind, value) DO UPDATE SET
			ability = excluded.ability, prof_mode = excluded.prof_mode, bonus = excluded.bonus`,
		characterID, kind, value, ability, profMode, bonus,
	)
	return err
}

// CustomItem is one entry in the shared, local item library (custom_items —
// not scoped to a single character, see 0013_custom_items.sql). It plays the
// same role for a homebrew item that a rules.db equipment row plays for a
// published one: everything downstream (inventory rows, detail pages,
// weapon-attack gating) reads it by slug, generically, with no "is this
// custom?" branch of its own.
//
// Kind is the free-text "Type" column shown in the inventory table — never
// checked against a fixed vocabulary, since a player can type anything.
// RollableKind is the separate, enum-like flag ("", "weapon", "toolkit",
// "other") that actually gates the rollable wiring — kept apart from Kind so
// a Type of "weapon" can never accidentally make an item rollable, and a
// rollable item can carry any Type at all.
type CustomItem struct {
	ID           int64
	Slug         string
	Name         string
	Kind         string
	RollableKind string
	DamageDice   string // weapon rollables only
	DamageType   string
	Properties   string // same free-text properties rules.db weapons carry —
	// drives buildAttacks' finesse/thrown/ammunition ability pick
	Bulk        sql.NullFloat64
	Description string
}

// customItemSlug derives a unique, custom/-namespaced slug from an item's
// name and its own row id — the same scheme 0013_custom_items.sql used to
// backfill existing inline custom rows. Two items can share a name (two
// characters, two "Grappling Hook"s) without colliding, since the id alone
// already guarantees uniqueness.
func customItemSlug(name string, id int64) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	clean := strings.NewReplacer(" ", "-", "'", "", `"`, "", "/", "-").Replace(lower)
	return fmt.Sprintf("custom/%s-%d", clean, id)
}

// AddCustomItem inserts a new library entry and stamps its slug from the row
// id SQLite only assigns once the row exists — insert, then a follow-up
// update rather than one statement.
func AddCustomItem(charDB *sql.DB, item CustomItem) (CustomItem, error) {
	res, err := charDB.Exec(`
		INSERT INTO custom_items (slug, name, kind, rollable_kind, damage_dice, damage_type, properties, bulk, description)
		VALUES ('', ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Name, item.Kind, nullIfEmpty(item.RollableKind), nullIfEmpty(item.DamageDice),
		nullIfEmpty(item.DamageType), nullIfEmpty(item.Properties), item.Bulk, item.Description,
	)
	if err != nil {
		return CustomItem{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CustomItem{}, err
	}
	item.ID = id
	item.Slug = customItemSlug(item.Name, id)
	if _, err := charDB.Exec(`UPDATE custom_items SET slug = ? WHERE id = ?`, item.Slug, id); err != nil {
		return CustomItem{}, err
	}
	return item, nil
}

// UpdateCustomItem overwrites every editable field of an existing library
// entry, keyed by id. Slug is deliberately never touched here — every
// character_inventory row pointing at this item, across every character,
// points at it by slug, so renaming the item must not move its address.
func UpdateCustomItem(charDB *sql.DB, id int64, item CustomItem) error {
	_, err := charDB.Exec(`
		UPDATE custom_items
		SET name = ?, kind = ?, rollable_kind = ?, damage_dice = ?, damage_type = ?, properties = ?, bulk = ?, description = ?
		WHERE id = ?`,
		item.Name, item.Kind, nullIfEmpty(item.RollableKind), nullIfEmpty(item.DamageDice),
		nullIfEmpty(item.DamageType), nullIfEmpty(item.Properties), item.Bulk, item.Description, id,
	)
	return err
}

// GetCustomItemByID loads one library entry by its own id — used only to
// recover the slug after an edit, since UpdateCustomItem never changes it.
func GetCustomItemByID(charDB *sql.DB, id int64) (CustomItem, error) {
	var item CustomItem
	var rollableKind, damageDice, damageType, properties, description sql.NullString
	err := charDB.QueryRow(`
		SELECT id, slug, name, kind, rollable_kind, damage_dice, damage_type, properties, bulk, description
		FROM custom_items WHERE id = ?`, id,
	).Scan(&item.ID, &item.Slug, &item.Name, &item.Kind, &rollableKind, &damageDice, &damageType, &properties, &item.Bulk, &description)
	if err != nil {
		return CustomItem{}, err
	}
	item.RollableKind, item.DamageDice, item.DamageType, item.Properties, item.Description =
		rollableKind.String, damageDice.String, damageType.String, properties.String, description.String
	return item, nil
}

// GetCustomItemBySlug loads one library entry for a detail page or an
// edit form. Returns sql.ErrNoRows if nothing matches, same as a rules.db
// lookup miss — callers 404 on that exactly like loadItem's callers do.
func GetCustomItemBySlug(charDB *sql.DB, slug string) (CustomItem, error) {
	var item CustomItem
	var rollableKind, damageDice, damageType, properties, description sql.NullString
	err := charDB.QueryRow(`
		SELECT id, slug, name, kind, rollable_kind, damage_dice, damage_type, properties, bulk, description
		FROM custom_items WHERE slug = ?`, slug,
	).Scan(&item.ID, &item.Slug, &item.Name, &item.Kind, &rollableKind, &damageDice, &damageType, &properties, &item.Bulk, &description)
	if err != nil {
		return CustomItem{}, err
	}
	item.RollableKind, item.DamageDice, item.DamageType, item.Properties, item.Description =
		rollableKind.String, damageDice.String, damageType.String, properties.String, description.String
	return item, nil
}

// nullIfEmpty turns a blank string into a SQL NULL rather than storing an
// empty string — keeps "no type given" reading the same way an omitted
// custom_bulk does, instead of two different spellings of "nothing here".
func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// AppendChatLog inserts one chat/roll line and trims the log back to the
// most recent 300 rows for this character, in the same transaction — the
// log is durable but bounded, not an ever-growing table.
func AppendChatLog(charDB *sql.DB, characterID int64, kind, text, crit string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO character_chat_log (character_id, kind, text, crit) VALUES (?, ?, ?, ?)`,
		characterID, kind, text, crit,
	); err != nil {
		return fmt.Errorf("insert chat log row: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM character_chat_log
		WHERE character_id = ? AND id NOT IN (
			SELECT id FROM character_chat_log WHERE character_id = ? ORDER BY id DESC LIMIT 300
		)`,
		characterID, characterID,
	); err != nil {
		return fmt.Errorf("trim chat log: %w", err)
	}
	return tx.Commit()
}

// ClearChatLog deletes every chat/roll line for one character — the whole
// log, not a trim to some smaller count, since this is the player's own
// deliberate "start fresh" action rather than the routine bounding
// AppendChatLog already does on every insert.
func ClearChatLog(charDB *sql.DB, characterID int64) error {
	_, err := charDB.Exec(`DELETE FROM character_chat_log WHERE character_id = ?`, characterID)
	return err
}

// ErrNoClass is returned by SetLevel when the character has no class yet.
// Level is a class level here, so there is nothing to raise until a class
// has been picked in the creation flow.
var ErrNoClass = errors.New("character has no class to level up")

// SetLevel sets the character's real level by raising (or lowering) the
// PRIMARY class's character_classes.levels — a thin convenience wrapper
// around SetClassLevel (see multiclass.go) for the single-class creation
// flow and the sheet's original Level control. A second or later class is
// leveled directly via SetClassLevel with its own class_slug instead.
//
// This replaces the old SetLevelOverride, which wrote a display-only
// 'level' row into character_overrides and deliberately granted nothing.
// That design was reversed: leveling up needed to automatically calculate
// HP and progress Chakra rather than leaving them static, so levelling up
// now moves the whole sheet.
func SetLevel(charDB *sql.DB, characterID int64, level int) error {
	var primary string
	err := charDB.QueryRow(`
		SELECT class_slug FROM character_classes
		WHERE character_id = ? ORDER BY order_index LIMIT 1`, characterID,
	).Scan(&primary)
	if err == sql.ErrNoRows {
		return ErrNoClass
	}
	if err != nil {
		return fmt.Errorf("load primary class: %w", err)
	}
	return SetClassLevel(charDB, characterID, primary, level)
}

// SetSubclass replaces whichever subclass the character has chosen for one
// class with newSubclassSlug, or clears the pick entirely if
// newSubclassSlug is "". siblingSlugs is every subclass belonging to that
// same class's one subclass group (resolved by the caller against rules.db,
// since this package never touches that database) — deleting by the whole
// sibling set, not just the character's previously stored slug, means a
// stale/renamed pick from a rules update still gets cleared out rather than
// left behind as an orphan row alongside the new one.
func SetSubclass(charDB *sql.DB, characterID int64, siblingSlugs []string, newSubclassSlug string, chosenAtLevel int) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(siblingSlugs) > 0 {
		placeholders := make([]string, len(siblingSlugs))
		args := make([]any, 0, len(siblingSlugs)+1)
		args = append(args, characterID)
		for i, slug := range siblingSlugs {
			placeholders[i] = "?"
			args = append(args, slug)
		}
		query := `DELETE FROM character_subclasses WHERE character_id = ? AND subclass_slug IN (` +
			strings.Join(placeholders, ",") + `)`
		if _, err := tx.Exec(query, args...); err != nil {
			return fmt.Errorf("clear existing subclass: %w", err)
		}
	}
	if newSubclassSlug != "" {
		if _, err := tx.Exec(
			`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, ?, ?)`,
			characterID, newSubclassSlug, chosenAtLevel,
		); err != nil {
			return fmt.Errorf("insert subclass: %w", err)
		}
	}
	return tx.Commit()
}

// SetOverride writes or clears one character_overrides row — the manual
// escape hatch behind the sheet's "pin this number by hand" controls (Max
// HP and Max Chakra when the player rolled their own hit dice, and which
// ability each jutsu attack type rolls off when a class feature moves it).
// An empty value clears the row, restoring the computed number.
//
// field is whitelisted by the caller, not here: this is a plain setter like
// the rest of the file, and the handler is where a form value stops being
// arbitrary input.
func SetOverride(charDB *sql.DB, characterID int64, field, value, note string) error {
	if value == "" {
		_, err := charDB.Exec(
			`DELETE FROM character_overrides WHERE character_id = ? AND field = ?`, characterID, field)
		return err
	}
	_, err := charDB.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note) VALUES (?, ?, ?, ?)
		ON CONFLICT (character_id, field) DO UPDATE SET value = excluded.value, note = excluded.note`,
		characterID, field, value, note,
	)
	return err
}

// AddRyo applies a signed delta to the character's ryo total — the
// currency row's own plain "+200"/"-50" entry, same "apply the input, no
// clamping" rule every other setter here follows.
func AddRyo(charDB *sql.DB, characterID int64, delta float64) error {
	_, err := charDB.Exec(
		`UPDATE characters SET ryo = ryo + ?, updated_at = datetime('now') WHERE id = ?`,
		delta, characterID,
	)
	return err
}

// SetStartingRyo applies a background's printed starting money, idempotently.
//
// The equipment step can be revisited and resubmitted any number of times
// (and the background behind it changed), so this cannot simply add: doing
// that would hand out another 100 Ryo every time the player pressed the
// button. Instead the last granted amount is remembered in a
// character_overrides row and only the DIFFERENCE is applied — resubmitting
// the same background is a no-op, and switching to one that grants a
// different amount corrects the purse by exactly the change.
//
// character_overrides is reused rather than a new column because it already
// exists for precisely this kind of per-character scalar, and a schema
// migration for one bookkeeping number would be the more invasive option.
// The field name is namespaced ('creation_ryo') and is not in the sheet's
// sheetOverrideFields whitelist, so no sheet control can collide with it.
func SetStartingRyo(charDB *sql.DB, characterID int64, amount float64) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var previous float64
	var stored sql.NullString
	if err := tx.QueryRow(
		`SELECT value FROM character_overrides WHERE character_id = ? AND field = 'creation_ryo'`, characterID,
	).Scan(&stored); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load previous starting ryo: %w", err)
	}
	if stored.Valid {
		// A malformed stored value means "grant the whole amount again",
		// which is the same behaviour as never having granted it — better
		// than refusing to save the equipment step over a bad row.
		previous, _ = strconv.ParseFloat(stored.String, 64)
	}

	if delta := amount - previous; delta != 0 {
		if _, err := tx.Exec(
			`UPDATE characters SET ryo = ryo + ?, updated_at = datetime('now') WHERE id = ?`,
			delta, characterID,
		); err != nil {
			return fmt.Errorf("apply starting ryo: %w", err)
		}
	}

	if amount == 0 {
		if _, err := tx.Exec(
			`DELETE FROM character_overrides WHERE character_id = ? AND field = 'creation_ryo'`, characterID,
		); err != nil {
			return fmt.Errorf("clear starting ryo marker: %w", err)
		}
		return tx.Commit()
	}
	if _, err := tx.Exec(`
		INSERT INTO character_overrides (character_id, field, value, note)
		VALUES (?, 'creation_ryo', ?, 'starting money from your background')
		ON CONFLICT (character_id, field) DO UPDATE SET value = excluded.value`,
		characterID, strconv.FormatFloat(amount, 'f', -1, 64),
	); err != nil {
		return fmt.Errorf("record starting ryo: %w", err)
	}
	return tx.Commit()
}

// SetRyo overwrites the character's ryo total outright. The currency box
// accepts both forms: a signed entry ("+200", "-50") adjusts the running
// total through AddRyo, while a bare number ("2000") means "my purse now
// holds exactly this much" and comes here instead.
func SetRyo(charDB *sql.DB, characterID int64, value float64) error {
	_, err := charDB.Exec(
		`UPDATE characters SET ryo = ?, updated_at = datetime('now') WHERE id = ?`,
		value, characterID,
	)
	return err
}

// AddInventoryItem adds one rules item to the character's inventory, or
// bumps the quantity if that item is already there — a player adding three
// more kunai expects one row reading 5, not a second row reading 3.
// Free-text rows written by character creation (custom_name, no slug) are
// never merged into, since they have no slug to match on.
func AddInventoryItem(charDB *sql.DB, characterID int64, itemSlug string, quantity int) error {
	if quantity < 1 {
		quantity = 1
	}
	res, err := charDB.Exec(`
		UPDATE character_inventory SET quantity = quantity + ?
		WHERE character_id = ? AND item_slug = ?`, quantity, characterID, itemSlug)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return nil
	}
	_, err = charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (?, ?, ?, 0)`, characterID, itemSlug, quantity)
	return err
}

// AddInventoryItemWithEquipped is AddInventoryItem plus an initial equipped
// state, for a caller that already knows the row can't exist yet — a
// freshly minted custom_items slug (see AddCustomItem) is unique by
// construction, so there is nothing to merge into and no merge-or-insert
// dance is needed.
func AddInventoryItemWithEquipped(charDB *sql.DB, characterID int64, itemSlug string, quantity int, equipped bool) error {
	if quantity < 1 {
		quantity = 1
	}
	_, err := charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (?, ?, ?, ?)`, characterID, itemSlug, quantity, equipped)
	return err
}

// UpdateInventoryItem sets one row's quantity and equipped flag. The row id
// is checked against the character so one character's request can never
// touch another's inventory.
func UpdateInventoryItem(charDB *sql.DB, characterID, rowID int64, quantity int, equipped bool) error {
	if quantity < 1 {
		quantity = 1
	}
	_, err := charDB.Exec(`
		UPDATE character_inventory SET quantity = ?, equipped = ?
		WHERE id = ? AND character_id = ?`, quantity, equipped, rowID, characterID)
	return err
}

// DeleteInventoryItem removes one inventory row.
func DeleteInventoryItem(charDB *sql.DB, characterID, rowID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_inventory WHERE id = ? AND character_id = ?`, rowID, characterID)
	return err
}

// UnpackedItem is one thing a container held: a real item when Slug is set,
// otherwise a free-text line (a pack that grants "1 Toolkit (pick one)" hands
// out a note, not an item — the player still has to choose).
type UnpackedItem struct {
	Slug     string
	Text     string
	Quantity int
}

// UnpackInventoryItem replaces one container row with its contents, in a
// single transaction so a failure halfway cannot leave the character holding
// both the pack and half of what was inside it.
//
// The pack row is removed rather than kept alongside its contents: Bulk is
// this ruleset's encumbrance currency, and a pack whose contents are also in
// the bag would be counted twice. Unpacking is a one-way door for that reason,
// which is what the sheet's confirmation prompt says.
//
// Items merge into an existing stack of the same slug (the same rule
// AddInventoryItem follows) so unpacking a second pack of glow rods deepens
// the stack instead of starting a second row.
func UnpackInventoryItem(charDB *sql.DB, characterID, rowID int64, contents []UnpackedItem) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`DELETE FROM character_inventory WHERE id = ? AND character_id = ?`, rowID, characterID)
	if err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	// A row that wasn't there (already unpacked in another tab, or a forged
	// id) must not still spill its contents into the bag.
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return sql.ErrNoRows
	}

	for _, item := range contents {
		qty := item.Quantity
		if qty < 1 {
			qty = 1
		}
		if item.Slug == "" {
			// A pack's contents line with no matching catalogue slug (prose
			// the ingest couldn't resolve to a real item) still needs a real
			// item_slug now — every character_inventory row does, since a
			// custom row's slug is what lookupCarriedItem and DetailHref key
			// off. Same insert-then-stamp-slug two-step AddCustomItem uses,
			// inlined here so it shares this call's transaction.
			res, err := tx.Exec(`INSERT INTO custom_items (slug, name) VALUES ('', ?)`, item.Text)
			if err != nil {
				return fmt.Errorf("create custom item for unpacked text line: %w", err)
			}
			ciID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("create custom item for unpacked text line: %w", err)
			}
			slug := customItemSlug(item.Text, ciID)
			if _, err := tx.Exec(`UPDATE custom_items SET slug = ? WHERE id = ?`, slug, ciID); err != nil {
				return fmt.Errorf("stamp custom item slug: %w", err)
			}
			if _, err := tx.Exec(
				`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
				 VALUES (?, ?, ?, 0)`, characterID, slug, qty,
			); err != nil {
				return fmt.Errorf("insert unpacked text line: %w", err)
			}
			continue
		}
		merged, err := tx.Exec(`
			UPDATE character_inventory SET quantity = quantity + ?
			WHERE character_id = ? AND item_slug = ?`, qty, characterID, item.Slug)
		if err != nil {
			return fmt.Errorf("merge unpacked item: %w", err)
		}
		if n, err := merged.RowsAffected(); err == nil && n > 0 {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
			VALUES (?, ?, ?, 0)`, characterID, item.Slug, qty,
		); err != nil {
			return fmt.Errorf("insert unpacked item: %w", err)
		}
	}
	return tx.Commit()
}

// SetBaseAbility overwrites one base ability score. The sheet's ability
// editor writes here rather than through SetAbilities because it changes
// one score at a time and has no business restating the other five.
//
// The column name is chosen by an explicit switch, not built from the
// caller's string — ability is ultimately form input, and a column name
// cannot be a bound parameter, so this is the one place where the
// difference between "validated against a fixed set" and "interpolated"
// would be a SQL injection.
func SetBaseAbility(charDB *sql.DB, characterID int64, ability string, value int) error {
	var query string
	switch ability {
	case "str":
		query = `UPDATE characters SET base_str = ?, updated_at = datetime('now') WHERE id = ?`
	case "dex":
		query = `UPDATE characters SET base_dex = ?, updated_at = datetime('now') WHERE id = ?`
	case "con":
		query = `UPDATE characters SET base_con = ?, updated_at = datetime('now') WHERE id = ?`
	case "int":
		query = `UPDATE characters SET base_int = ?, updated_at = datetime('now') WHERE id = ?`
	case "wis":
		query = `UPDATE characters SET base_wis = ?, updated_at = datetime('now') WHERE id = ?`
	case "cha":
		query = `UPDATE characters SET base_cha = ?, updated_at = datetime('now') WHERE id = ?`
	default:
		return fmt.Errorf("unknown ability %q", ability)
	}
	_, err := charDB.Exec(query, value, characterID)
	return err
}

// SetPortrait stores the character's portrait as a data: URL, or clears it
// when dataURL is empty. Encoding and size limits are the upload handler's
// job (see handleSheetPortrait) — this stays a plain setter like the rest
// of this file.
func SetPortrait(charDB *sql.DB, characterID int64, dataURL string) error {
	var value any
	if dataURL != "" {
		value = dataURL
	}
	_, err := charDB.Exec(
		`UPDATE characters SET portrait = ?, updated_at = datetime('now') WHERE id = ?`,
		value, characterID,
	)
	return err
}

// SetRestGains applies a short or long rest's outcome as plain deltas: hp/
// chakra add to current_hp/current_chakra (healing only in practice, but
// not clamped here — same "apply the input" rule every other setter
// follows), and hitDiceSpent/chakraDiceSpent add to the stored spent
// counters (positive when a short rest spends dice, negative when a long
// rest recovers them back).
func SetRestGains(charDB *sql.DB, characterID int64, hp, chakra, hitDiceSpent, chakraDiceSpent int) error {
	_, err := charDB.Exec(`
		UPDATE characters SET
			current_hp = current_hp + ?,
			current_chakra = current_chakra + ?,
			hit_dice_spent = hit_dice_spent + ?,
			chakra_dice_spent = chakra_dice_spent + ?,
			updated_at = datetime('now')
		WHERE id = ?`,
		hp, chakra, hitDiceSpent, chakraDiceSpent, characterID,
	)
	return err
}

// characterChildTables returns every table with a character_id column, in
// an order safe for deletion (character_jutsu before custom_jutsu, which it
// references; nothing else here has an inter-table ordering constraint).
// Discovered from the live schema rather than hard-coded, so a table added
// by a future migration is covered the day it exists — a hard-coded list
// drifts silently, and an orphaned row is invisible until a new character
// reuses the freed id and inherits it.
func characterChildTables(charDB *sql.DB) ([]string, error) {
	rows, err := charDB.Query(`
		SELECT m.name FROM sqlite_master m
		WHERE m.type = 'table'
		  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'character_id')
		ORDER BY CASE m.name WHEN 'character_jutsu' THEN 0 ELSE 1 END, m.name`)
	if err != nil {
		return nil, fmt.Errorf("enumerate character tables: %w", err)
	}
	defer rows.Close()
	var children []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan character table name: %w", err)
		}
		children = append(children, name)
	}
	return children, rows.Err()
}

// deleteCompanionChildRows removes every row keyed by companion_id — not
// character_id — for every companion belonging to characterID:
// character_companion_upgrades, its own child character_companion_upgrade_choices,
// and character_companion_attacks. characterChildTables' character_id-column
// scan can't see these (they hang off character_companions, not characters,
// directly), so without this pass they'd only ever be cleaned up by ON
// DELETE CASCADE — which, same as the rest of this file, cannot be relied
// on since PRAGMA foreign_keys is per-connection in SQLite.
func deleteCompanionChildRows(tx *sql.Tx, characterID int64) error {
	rows, err := tx.Query(`SELECT id FROM character_companions WHERE character_id = ?`, characterID)
	if err != nil {
		return fmt.Errorf("list companions: %w", err)
	}
	var companionIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan companion id: %w", err)
		}
		companionIDs = append(companionIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("companion rows: %w", err)
	}
	for _, companionID := range companionIDs {
		if _, err := tx.Exec(`
			DELETE FROM character_companion_upgrade_choices
			WHERE companion_upgrade_id IN (
				SELECT id FROM character_companion_upgrades WHERE companion_id = ?
			)`, companionID); err != nil {
			return fmt.Errorf("delete companion upgrade choices: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM character_companion_upgrades WHERE companion_id = ?`, companionID); err != nil {
			return fmt.Errorf("delete companion upgrades: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM character_companion_attacks WHERE companion_id = ?`, companionID); err != nil {
			return fmt.Errorf("delete companion attacks: %w", err)
		}
	}
	return nil
}

// deleteCharacterChildRows removes every row belonging to characterID from
// every table that hangs off it — companion-owned tables first (see
// deleteCompanionChildRows), then everything with its own character_id
// column — without relying on ON DELETE CASCADE. Leaves the characters row
// itself untouched; callers decide whether to delete or reset it.
func deleteCharacterChildRows(tx *sql.Tx, charDB *sql.DB, characterID int64) error {
	if err := deleteCompanionChildRows(tx, characterID); err != nil {
		return err
	}
	children, err := characterChildTables(charDB)
	if err != nil {
		return err
	}
	for _, table := range children {
		// Table names come from sqlite_master, not from user input, so the
		// concatenation is safe; the id stays a bound parameter.
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE character_id = ?`, characterID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	return nil
}

// DeleteCharacter removes a character and everything hanging off it.
func DeleteCharacter(charDB *sql.DB, characterID int64) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := deleteCharacterChildRows(tx, charDB, characterID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM characters WHERE id = ?`, characterID); err != nil {
		return fmt.Errorf("delete character: %w", err)
	}
	return tx.Commit()
}

// ResetCharacterCreation retcons a character back to the start of creation:
// level, clan, class, subclass, jutsu, feats, proficiencies, inventory,
// companions, and everything else derived from those choices is wiped, the
// same way DeleteCharacter wipes it — but the characters row itself is
// reset in place rather than deleted, so the id (and any link to it)
// survives, and identity/cosmetic fields the retcon has no opinion about
// (name, portrait, notes, and the freeform narrative fields — appearance,
// backstory, allies_organizations, additional_features_text, treasure) are
// kept rather than blanked. creation_status goes back to 'draft', so the
// creation hub's own per-step "done" checks (each of which looks for rows
// this just deleted) correctly show every step as not yet done.
func ResetCharacterCreation(charDB *sql.DB, characterID int64) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := deleteCharacterChildRows(tx, charDB, characterID); err != nil {
		return err
	}
	res, err := tx.Exec(`
		UPDATE characters SET
			clan_slug = NULL,
			background_slug = NULL,
			base_str = 10, base_dex = 10, base_con = 10,
			base_int = 10, base_wis = 10, base_cha = 10,
			xp = 0,
			current_hp = 0, temp_hp = 0, base_temp_hp = 0,
			current_chakra = 0, temp_chakra = 0,
			hit_dice_spent = 0, chakra_dice_spent = 0,
			ryo = 0,
			inspiration = 0,
			creation_status = 'draft',
			updated_at = datetime('now')
		WHERE id = ?`, characterID)
	if err != nil {
		return fmt.Errorf("reset character row: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("reset character row: %w", err)
	} else if n == 0 {
		return fmt.Errorf("character %d not found", characterID)
	}
	return tx.Commit()
}
