package charstore

import (
	"database/sql"
	"fmt"
)

// Companion is one Puppet/Summon/custom companion sheet — see migration
// 0017_companions.sql for why every field here is a plain player-entered
// value rather than anything derived.
type Companion struct {
	ID              int64
	Kind            string // "puppet", "summon", "custom", "nin-dog", "titan"
	Name            string
	SummonTribeSlug string

	AC        sql.NullInt64
	HPCurrent sql.NullInt64
	HPMax     sql.NullInt64
	Speed     sql.NullInt64
	// FlySpeed is a separate movement speed, not a replacement for Speed —
	// see migration 0027_companion_fly_speed.sql. NULL for any companion
	// that has never had one.
	FlySpeed sql.NullInt64
	Str      sql.NullInt64
	Dex      sql.NullInt64
	Con      sql.NullInt64
	Int      sql.NullInt64
	Wis      sql.NullInt64
	Cha      sql.NullInt64

	Attacks string
	Traits  string
	Notes   string

	// ArmorChassis/IsArmorForm are only meaningful (and player-editable)
	// for kind = "puppet" — Puppet Master's Purple Technique lets a Puppet
	// Tool transform into Juggernaut Armor built around a chosen Armor
	// Chassis. Free text, not a structured pick: the book's actual chassis
	// stat blocks live in prose that was never ingested as rules-database
	// rows, same "customization over automation" boundary Attacks/Traits/
	// Notes already use.
	ArmorChassis string
	IsArmorForm  bool

	// Size is a companion's size category ("Small", "Medium", "Large", ...),
	// free text like ArmorChassis rather than a closed enum — a player-typed
	// override is just as valid as a computed default from a Puppeteer
	// Chassis/Puppet Framework/Puppet Role/Puppet Weapon Type pick (see
	// migration 0032). Blank means never set.
	Size string

	// NinDogBreed is only meaningful (and player-editable) for kind =
	// "nin-dog" — the Inuzuka clan's Beast Master feature's one-time Young
	// Inuit/Young Kugsha/Young Tamaskan pick (see migration 0065). Locked
	// once set, the same "permanent crafting-style choice" treatment
	// ArmorChassis already gets, since the book's own text never mentions
	// re-selecting a breed.
	NinDogBreed string
	// JutsuSlotsCurrent/JutsuSlotsMax: a Nin-Dog's own spendable Jutsu Slots
	// resource, delta-editable exactly like AC/HPMax (see migration 0065's
	// own doc for why this is a separate pair of columns from HP).
	JutsuSlotsCurrent sql.NullInt64
	JutsuSlotsMax     sql.NullInt64

	// TitanSpecialization is only meaningful (and player-editable) for
	// kind = "titan" — Ordnance Training's one-time Legion/Monarch/Ronin
	// pick (see migration 0066). Locked once set, same pattern as
	// NinDogBreed above.
	TitanSpecialization string
	// BarrierCurrent/BarrierMax: a Titan's own Battery Powered Barrier hit
	// points, delta-editable exactly like AC/HPMax (see migration 0066's
	// own doc for why this is a separate pair of columns from HP).
	BarrierCurrent sql.NullInt64
	BarrierMax     sql.NullInt64

	// MatryoshkaGroupID/MatryoshkaJutsuSlots: Matryoshka Framework's own
	// multi-body split (see migration 0034_matryoshka_bodies.sql).
	// MatryoshkaGroupID is NULL for an ordinary, unsplit companion; a
	// non-NULL value names the id of whichever body is this group's own
	// primary (the row a re-merge collapses back onto).
	MatryoshkaGroupID    sql.NullInt64
	MatryoshkaJutsuSlots int64

	SortOrder int
}

// companionFields is a Companion's editable columns, in the exact order
// AddCompanion/GetCompanion/ListCompanions/companionFieldSetters below scan
// and bind them — kept in one place so a new column only needs to be added
// once, not kept in sync across four separate SQL statements by hand.
const companionSelectColumns = `id, kind, name, summon_tribe_slug,
	ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
	attacks, traits, notes, armor_chassis, is_armor_form, size,
	matryoshka_group_id, matryoshka_jutsu_slots, sort_order,
	nin_dog_breed, jutsu_slots_current, jutsu_slots_max,
	titan_specialization, barrier_current, barrier_max`

func scanCompanion(row interface{ Scan(...any) error }) (Companion, error) {
	var c Companion
	var isArmorForm int
	err := row.Scan(
		&c.ID, &c.Kind, &c.Name, &c.SummonTribeSlug,
		&c.AC, &c.HPCurrent, &c.HPMax, &c.Speed, &c.FlySpeed, &c.Str, &c.Dex, &c.Con, &c.Int, &c.Wis, &c.Cha,
		&c.Attacks, &c.Traits, &c.Notes, &c.ArmorChassis, &isArmorForm, &c.Size,
		&c.MatryoshkaGroupID, &c.MatryoshkaJutsuSlots, &c.SortOrder,
		&c.NinDogBreed, &c.JutsuSlotsCurrent, &c.JutsuSlotsMax,
		&c.TitanSpecialization, &c.BarrierCurrent, &c.BarrierMax,
	)
	c.IsArmorForm = isArmorForm != 0
	return c, err
}

// AddCompanion creates a new companion sheet for a character, sorted after
// every existing one, with every stat field blank until the player fills it
// in on the companion's own popup.
func AddCompanion(charDB *sql.DB, characterID int64, kind, name string) (int64, error) {
	var nextOrder int
	if err := charDB.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM character_companions WHERE character_id = ?`,
		characterID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := charDB.Exec(
		`INSERT INTO character_companions (character_id, kind, name, sort_order) VALUES (?, ?, ?, ?)`,
		characterID, kind, name, nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert companion: %w", err)
	}
	return res.LastInsertId()
}

// DeleteCompanion removes one companion, scoped to characterID so a
// stale/forged companion id from another character's popup can't touch this
// one.
func DeleteCompanion(charDB *sql.DB, characterID, companionID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_companions WHERE id = ? AND character_id = ?`,
		companionID, characterID,
	)
	return err
}

// ListCompanions returns a character's companions in display order, for the
// main sheet's Companions box.
func ListCompanions(charDB *sql.DB, characterID int64) ([]Companion, error) {
	rows, err := charDB.Query(
		`SELECT `+companionSelectColumns+` FROM character_companions WHERE character_id = ? ORDER BY sort_order`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Companion
	for rows.Next() {
		c, err := scanCompanion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCompanion loads one companion's full record for its own popup page,
// scoped to characterID for the same reason DeleteCompanion is.
func GetCompanion(charDB *sql.DB, characterID, companionID int64) (Companion, error) {
	row := charDB.QueryRow(
		`SELECT `+companionSelectColumns+` FROM character_companions WHERE id = ? AND character_id = ?`,
		companionID, characterID,
	)
	return scanCompanion(row)
}

// SetCompanionFields replaces every editable field on one companion's popup
// EXCEPT hp_current, AC, and hp_max in a single UPDATE, the same "one form,
// whole-field autosave on blur" shape as SetBio — the popup's Speed row,
// ability scores, and the three free-text blocks all belong to one logical
// form, so a blur on any field resends all of them. hp_current/AC/hp_max
// are deliberately excluded and live behind their own AddCompanionHP/
// SetCompanionHP/AddCompanionIntField/SetCompanionIntField instead: each has
// its own dedicated <form>/endpoint (see handleCompanionHP/
// handleCompanionIntField's doc comments for why), and this function
// unconditionally overwrites every column it's given — if any of the three
// were included here, this form's own "blur on any field resends every
// field" behavior would resubmit an EMPTY value (those fields live outside
// this form now) on every unrelated field's blur and silently wipe them
// back to blank. Confirmed live: AC/hp_max were both still being read and
// unconditionally overwritten here even after their own delta-editable
// fields were split out of this form, silently nulling them on the very
// next Armor Chassis/Speed/ability-score edit. Numeric fields are
// sql.NullInt64 because every one of them is optional (a fresh companion
// starts with every stat blank) — parsing and range-checking the raw form
// values happens in the HTTP handler, the same division of labor
// SetBaseAbility's caller already uses.
// Armor Chassis (Purple Technique's Armor Chassis feature, class/puppet-
// master/group/puppet-techniques/purple-technique-juggernaut/feature/armor-
// chassis) is a one-time crafting choice made "when you craft your
// Juggernaut Armor" at 2nd level — the book's own text never mentions
// re-selecting or re-crafting it with a different chassis afterward, unlike
// features elsewhere in this class that explicitly say a pick is
// changeable ("you may swap... on a long rest") or explicitly say it's
// permanent. Silence read together with "no further mention of swapping"
// is treated here as permanent by design, since several Purple Upgrades
// gate on a specific chassis (puppet_upgrade_prereq.go) and an editable
// chassis would let a player toggle it to bypass those prerequisites and
// toggle back. Enforced here, not just in the UI: once armor_chassis is
// non-empty, this UPDATE can never change it again, regardless of what a
// stale or hand-crafted form POST sends.
func SetCompanionFields(charDB *sql.DB, characterID, companionID int64, name, summonTribeSlug string,
	speed, flySpeed, str, dex, con, intScore, wis, cha sql.NullInt64,
	attacks, traits, notes, armorChassis string, isArmorForm bool, size string, ninDogBreed string,
	titanSpecialization string,
) error {
	armorFormValue := 0
	if isArmorForm {
		armorFormValue = 1
	}
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			name = ?, summon_tribe_slug = ?,
			speed = ?, fly_speed = ?,
			str_score = ?, dex_score = ?, con_score = ?, int_score = ?, wis_score = ?, cha_score = ?,
			attacks = ?, traits = ?, notes = ?,
			armor_chassis = CASE WHEN armor_chassis = '' THEN ? ELSE armor_chassis END,
			is_armor_form = ?, size = ?,
			nin_dog_breed = CASE WHEN nin_dog_breed = '' THEN ? ELSE nin_dog_breed END,
			titan_specialization = CASE WHEN titan_specialization = '' THEN ? ELSE titan_specialization END,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		name, summonTribeSlug,
		speed, flySpeed,
		str, dex, con, intScore, wis, cha,
		attacks, traits, notes, armorChassis, armorFormValue, size, ninDogBreed,
		titanSpecialization,
		companionID, characterID,
	)
	return err
}

// SetCompanionStatDefaults prefills AC/HP-current/HP-max/Speed/six ability
// scores on a puppet companion from the Puppet Tool's baseline unit card
// (see cmd/n5e/companions.go's puppetToolMaxHP/puppetToolDefaultAC) — used
// both right after AddCompanion (every field genuinely NULL) and as a
// lazy per-render backfill for any older companion still missing SOME of
// these fields (see cmd/n5e/puppets.go's loadPuppetsTabData). Every column
// is written via COALESCE(column, ?), so a field the player has already
// set — through play, or through an earlier partial version of this
// prefill — is never touched; only a still-NULL field gets the computed
// default. This is what makes the backfill call safe to run unconditionally
// on every render instead of needing an exact "every field is still unset"
// gate, which left a companion with SOME fields already set (AC/HP but not
// Speed/ability scores, say) permanently unbackfilled.
//
// Deliberately NOT routed through SetCompanionFields (the whole-field
// autosave path): that function is called on every blur of any OTHER
// field, and if it also carried hp_current would silently null it back out
// (see SetCompanionFields' own doc). This function writes only the columns
// listed here and nothing else, so it can't reintroduce that class of bug
// even though it runs at a different time than a normal field save.
// flySpeed is deliberately a NullInt64 rather than a plain int: a puppet
// with no flying-speed source has no flying speed at all, which is not the
// same as a flying speed of 0. Passing NULL leaves the column untouched
// (COALESCE(fly_speed, NULL) is a no-op), so the field stays blank on the
// card instead of reading "0 ft" for every ordinary walking puppet.
//
// size follows the same "never overwrite a real value" rule as every other
// param here, but size is TEXT NOT NULL DEFAULT ” (see migration 0032),
// not a nullable column — COALESCE only short-circuits on NULL, not on an
// empty string, so an already-blank-but-set ” would never match a NULL
// check anyway. A blank incoming size (no Puppeteer Chassis/Puppet
// Framework/Puppet Role/Puppet Weapon Type pick resolved yet) is passed
// through as ”, which the CASE below reads as "nothing to backfill" the
// same way ArmorChassis's own CASE in SetCompanionFields does.
func SetCompanionStatDefaults(charDB *sql.DB, characterID, companionID int64,
	ac, hpCurrent, hpMax, speed int64, flySpeed sql.NullInt64,
	str, dex, con, intScore, wis, cha int64, size string,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = COALESCE(ac, ?), hp_current = COALESCE(hp_current, ?), hp_max = COALESCE(hp_max, ?),
			speed = COALESCE(speed, ?), fly_speed = COALESCE(fly_speed, ?),
			str_score = COALESCE(str_score, ?), dex_score = COALESCE(dex_score, ?),
			con_score = COALESCE(con_score, ?), int_score = COALESCE(int_score, ?),
			wis_score = COALESCE(wis_score, ?), cha_score = COALESCE(cha_score, ?),
			size = CASE WHEN size = '' THEN ? ELSE size END,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpCurrent, hpMax, speed, flySpeed, str, dex, con, intScore, wis, cha, size,
		companionID, characterID,
	)
	return err
}

// SetNinDogStatDefaults prefills AC/HP-current/HP-max/Speed/Jutsu-Slots-Max/
// six ability scores on a freshly created nin-dog companion from Beast
// Master's own computed baseline (see cmd/n5e/nindog.go's
// prefillNinDogStatDefaults) — the Nin-Dog equivalent of
// SetCompanionStatDefaults just above, kept as its own function rather than
// folded into that one so puppet's flySpeed/size params (meaningless for a
// Nin-Dog) don't have to grow two more kind-specific nullable params
// (jutsuSlotsMax, and a future Titan barrierMax) neither kind would ever
// both use at once. Every column is written via COALESCE(column, ?), the
// same "never overwrite an already-set value" contract
// SetCompanionStatDefaults documents — a field the player has already set
// (through play, or through an earlier partial version of this prefill) is
// never touched, which is what makes this safe to call again later (e.g. a
// future per-render backfill for older companions) without risk of
// clobbering a manual edit.
func SetNinDogStatDefaults(charDB *sql.DB, characterID, companionID int64,
	ac, hpCurrent, hpMax, speed, jutsuSlotsMax int64,
	str, dex, con, intScore, wis, cha int64,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = COALESCE(ac, ?), hp_current = COALESCE(hp_current, ?), hp_max = COALESCE(hp_max, ?),
			speed = COALESCE(speed, ?), jutsu_slots_max = COALESCE(jutsu_slots_max, ?),
			str_score = COALESCE(str_score, ?), dex_score = COALESCE(dex_score, ?),
			con_score = COALESCE(con_score, ?), int_score = COALESCE(int_score, ?),
			wis_score = COALESCE(wis_score, ?), cha_score = COALESCE(cha_score, ?),
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpCurrent, hpMax, speed, jutsuSlotsMax, str, dex, con, intScore, wis, cha,
		companionID, characterID,
	)
	return err
}

// SetTitanStatDefaults prefills HP-current/HP-max/Speed/Barrier-current/
// Barrier-max/six ability scores on a freshly created titan companion from
// Ordnance Training's own computed baseline (see cmd/n5e/titan.go's
// prefillTitanStatDefaults) — the Titan equivalent of SetNinDogStatDefaults
// just above. No ac param, unlike that function: no AC formula exists
// anywhere in the Titan's own source text (see titan.go's own header doc on
// titan_unit_card.raw_text never stating one), so a Titan's AC stays a
// plain player-entered field like every other companion kind's default,
// never prefilled — this mirrors loadTitanReference's own ExpectedAC never
// being set. Every column is written via COALESCE(column, ?), the same
// "never overwrite an already-set value" contract SetCompanionStatDefaults/
// SetNinDogStatDefaults document — a field the player has already set is
// never touched, which is what makes this safe to call again later (e.g. a
// future per-render backfill for older companions) without risk of
// clobbering a manual edit.
func SetTitanStatDefaults(charDB *sql.DB, characterID, companionID int64,
	hpCurrent, hpMax, speed, barrierCurrent, barrierMax int64,
	str, dex, con, intScore, wis, cha int64,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			hp_current = COALESCE(hp_current, ?), hp_max = COALESCE(hp_max, ?),
			speed = COALESCE(speed, ?),
			barrier_current = COALESCE(barrier_current, ?), barrier_max = COALESCE(barrier_max, ?),
			str_score = COALESCE(str_score, ?), dex_score = COALESCE(dex_score, ?),
			con_score = COALESCE(con_score, ?), int_score = COALESCE(int_score, ?),
			wis_score = COALESCE(wis_score, ?), cha_score = COALESCE(cha_score, ?),
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		hpCurrent, hpMax, speed, barrierCurrent, barrierMax, str, dex, con, intScore, wis, cha,
		companionID, characterID,
	)
	return err
}

// AddCompanionHP applies a signed delta to one companion's hp_current,
// floored at 0 — the companion-popup equivalent of the main sheet's SetHP,
// minus the temp-HP cascade (companions have no temp HP field to absorb
// damage first). A NULL hp_current (never touched yet) is treated as 0, so
// the very first delta on a fresh companion still lands correctly instead
// of erroring or silently doing nothing.
func AddCompanionHP(charDB *sql.DB, characterID, companionID int64, delta int64) (int64, error) {
	tx, err := charDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var current sql.NullInt64
	if err := tx.QueryRow(
		`SELECT hp_current FROM character_companions WHERE id = ? AND character_id = ?`, companionID, characterID,
	).Scan(&current); err != nil {
		return 0, fmt.Errorf("load hp_current: %w", err)
	}
	newValue := current.Int64 + delta
	if newValue < 0 {
		newValue = 0
	}
	if _, err := tx.Exec(
		`UPDATE character_companions SET hp_current = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		newValue, companionID, characterID,
	); err != nil {
		return 0, fmt.Errorf("update hp_current: %w", err)
	}
	return newValue, tx.Commit()
}

// SetCompanionHP sets one companion's hp_current outright — the "bare
// number" half of the HP box's dual-purpose input, and also how the box
// gets cleared back to blank (value.Valid == false).
func SetCompanionHP(charDB *sql.DB, characterID, companionID int64, value sql.NullInt64) error {
	_, err := charDB.Exec(
		`UPDATE character_companions SET hp_current = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		value, companionID, characterID,
	)
	return err
}

// companionIntFields whitelists which character_companions columns
// AddCompanionIntField/SetCompanionIntField may touch — field is a small,
// fixed set of literal strings chosen by each route registration
// (cmd/n5e/companions.go), never user input, but a whitelist keeps that
// true by construction instead of by convention (same reasoning as
// cmd/n5e/characters.go's own sheetOverrideFields).
var companionIntFields = map[string]bool{
	"ac": true, "hp_max": true, "matryoshka_jutsu_slots": true,
	"jutsu_slots_current": true, "jutsu_slots_max": true,
	"barrier_current": true, "barrier_max": true,
}

// AddCompanionIntField applies a signed delta to one of a companion's own
// int fields (AC, HP-max — see companionIntFields), floored at 0, same
// shape as AddCompanionHP just above but generalized: this is what lets a
// player type "+1" into the AC box after picking up an AC-boosting upgrade
// instead of doing the arithmetic themselves, the "populate then edit"
// replacement for the removed "Use computed" button (see
// puppetToolMaxHP/puppetToolDefaultAC).
func AddCompanionIntField(charDB *sql.DB, characterID, companionID int64, field string, delta int64) (int64, error) {
	if !companionIntFields[field] {
		return 0, fmt.Errorf("companion int field not writable: %s", field)
	}
	tx, err := charDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var current sql.NullInt64
	if err := tx.QueryRow(
		`SELECT `+field+` FROM character_companions WHERE id = ? AND character_id = ?`, companionID, characterID,
	).Scan(&current); err != nil {
		return 0, fmt.Errorf("load %s: %w", field, err)
	}
	newValue := current.Int64 + delta
	if newValue < 0 {
		newValue = 0
	}
	if _, err := tx.Exec(
		`UPDATE character_companions SET `+field+` = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		newValue, companionID, characterID,
	); err != nil {
		return 0, fmt.Errorf("update %s: %w", field, err)
	}
	return newValue, tx.Commit()
}

// SetCompanionIntField sets one of a companion's own int fields outright
// (or clears it, value.Valid == false) — the "bare number"/blank half of
// the same delta-editable box AddCompanionIntField's "+3"/"-2" half serves.
func SetCompanionIntField(charDB *sql.DB, characterID, companionID int64, field string, value sql.NullInt64) error {
	if !companionIntFields[field] {
		return fmt.Errorf("companion int field not writable: %s", field)
	}
	_, err := charDB.Exec(
		`UPDATE character_companions SET `+field+` = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		value, companionID, characterID,
	)
	return err
}

// SplitCompanionIntoBodies implements Matryoshka Framework's own "split
// into 1 to 3 bodies (on a rest), dividing max HP among them" action:
// companionID becomes the new group's own primary (matryoshka_group_id set
// to its own id), count-1 additional companion rows are cloned from it
// (same kind/stats/attacks/traits/notes, name suffixed "(Body N)"), and
// hp_max is divided across all of them by floor division, with the
// remainder landing on the primary — an invented tie-break the rules text
// doesn't specify. count must be 2 or 3; splitting "into 1 body" is a
// no-op the caller should simply not offer.
func SplitCompanionIntoBodies(charDB *sql.DB, characterID, companionID int64, count int) error {
	if count < 2 || count > 3 {
		return fmt.Errorf("split count must be 2 or 3, got %d", count)
	}
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	primary, err := scanCompanion(tx.QueryRow(
		`SELECT `+companionSelectColumns+` FROM character_companions WHERE id = ? AND character_id = ?`,
		companionID, characterID,
	))
	if err != nil {
		return fmt.Errorf("load companion: %w", err)
	}
	if primary.MatryoshkaGroupID.Valid {
		return fmt.Errorf("companion %d is already split", companionID)
	}

	hasMax := primary.HPMax.Valid
	base := primary.HPMax.Int64 / int64(count)
	remainder := primary.HPMax.Int64 % int64(count)

	var nextOrder int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM character_companions WHERE character_id = ?`,
		characterID,
	).Scan(&nextOrder); err != nil {
		return fmt.Errorf("compute sort_order: %w", err)
	}

	primaryHPMax := sql.NullInt64{Int64: base + remainder, Valid: hasMax}
	if _, err := tx.Exec(
		`UPDATE character_companions SET hp_max = ?, matryoshka_group_id = ?, updated_at = datetime('now')
		 WHERE id = ? AND character_id = ?`,
		primaryHPMax, companionID, companionID, characterID,
	); err != nil {
		return fmt.Errorf("update primary body: %w", err)
	}

	armorFormValue := 0
	if primary.IsArmorForm {
		armorFormValue = 1
	}
	for i := 2; i <= count; i++ {
		bodyHPMax := sql.NullInt64{Int64: base, Valid: hasMax}
		if _, err := tx.Exec(`
			INSERT INTO character_companions (
				character_id, kind, name, summon_tribe_slug,
				ac, hp_current, hp_max, speed, fly_speed,
				str_score, dex_score, con_score, int_score, wis_score, cha_score,
				attacks, traits, notes, armor_chassis, is_armor_form, size,
				matryoshka_group_id, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			characterID, primary.Kind, fmt.Sprintf("%s (Body %d)", primary.Name, i), primary.SummonTribeSlug,
			primary.AC, sql.NullInt64{}, bodyHPMax, primary.Speed, primary.FlySpeed,
			primary.Str, primary.Dex, primary.Con, primary.Int, primary.Wis, primary.Cha,
			primary.Attacks, primary.Traits, primary.Notes, primary.ArmorChassis, armorFormValue, primary.Size,
			companionID, nextOrder+i-2,
		); err != nil {
			return fmt.Errorf("insert body %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// MergeCompanionBodies implements Matryoshka Framework's own "re-merge on a
// rest" action: every sibling body sharing groupCompanionID's own
// matryoshka_group_id is deleted, and the primary's hp_max is restored to
// the sum of every body's own current hp_max — the lossless inverse of
// SplitCompanionIntoBodies' own floor-division split, since no play action
// ever reduces hp_max, only hp_current.
func MergeCompanionBodies(charDB *sql.DB, characterID, groupCompanionID int64) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		`SELECT hp_max FROM character_companions WHERE character_id = ? AND matryoshka_group_id = ?`,
		characterID, groupCompanionID,
	)
	if err != nil {
		return err
	}
	var total int64
	var hasMax bool
	for rows.Next() {
		var hp sql.NullInt64
		if err := rows.Scan(&hp); err != nil {
			rows.Close()
			return err
		}
		if hp.Valid {
			total += hp.Int64
			hasMax = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if _, err := tx.Exec(
		`DELETE FROM character_companions WHERE character_id = ? AND matryoshka_group_id = ? AND id != ?`,
		characterID, groupCompanionID, groupCompanionID,
	); err != nil {
		return fmt.Errorf("delete sibling bodies: %w", err)
	}

	mergedHPMax := sql.NullInt64{Int64: total, Valid: hasMax}
	if _, err := tx.Exec(
		`UPDATE character_companions SET hp_max = ?, matryoshka_group_id = NULL, updated_at = datetime('now')
		 WHERE id = ? AND character_id = ?`,
		mergedHPMax, groupCompanionID, characterID,
	); err != nil {
		return fmt.Errorf("merge primary body: %w", err)
	}
	return tx.Commit()
}
