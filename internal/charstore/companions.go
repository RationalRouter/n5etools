package charstore

import (
	"database/sql"
	"fmt"
)

// Companion is one Puppet/Summon/custom companion sheet — see migration
// 0017_companions.sql for the original "every field is plain player-entered"
// design and cmd/n5e/companions.go's own header for how that's since changed:
// kind="titan"/"nin-dog"/"snb" (and Puppet Tools) now get these fields
// auto-computed from a documented formula at creation and on the events that
// would change them, with a player free to overwrite the computed value
// afterward. kind="summon"/"custom" still have no formula behind them at all
// and remain fully manual.
type Companion struct {
	ID              int64
	Kind            string // "puppet", "summon", "custom", "nin-dog", "titan", "snb"
	Name            string
	SummonTribeSlug string

	AC        sql.NullInt64
	HPCurrent sql.NullInt64
	HPMax     sql.NullInt64
	// TempHP: absorbed by AddCompanionHP before hp_current on damage, same
	// "extra pool depleted first" mechanic as the main sheet's own Temp HP
	// (charstore.SetHP) — see migration 0080_companion_temp_hp.sql.
	TempHP sql.NullInt64
	Speed  sql.NullInt64
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
	// IsDemonFoe is only meaningful (and player-editable) for kind =
	// "titan" — Bijuu Slayer's own "+2 damage dice vs Demon type foes"
	// clause, toggled by the player per encounter rather than computed
	// (this app has no target/foe-type state to derive it from) — see
	// migration 0082's own doc.
	IsDemonFoe bool

	// MatryoshkaGroupID/MatryoshkaJutsuSlots: Matryoshka Framework's own
	// multi-body split (see migration 0034_matryoshka_bodies.sql).
	// MatryoshkaGroupID is NULL for an ordinary, unsplit companion; a
	// non-NULL value names the id of whichever body is this group's own
	// primary (the row a re-merge collapses back onto).
	MatryoshkaGroupID    sql.NullInt64
	MatryoshkaJutsuSlots int64

	// SaveProficiencies: which of the six saving throws this companion is
	// proficient in, comma-separated ability codes (e.g. "str,dex,con") —
	// see migration 0077's own doc for why this is free text, why it
	// defaults to '' even for a kind whose rules text names a fixed or
	// starting set, and why it's never read for kind="puppet".
	SaveProficiencies string

	// Resistances/Immunities/ConditionImmunities: only meaningful (and
	// player-editable) for kind = "custom" — see migration 0081's own doc.
	// Every other kind has this exact same information computed instead
	// (titanReference/snbReference in cmd/n5e/titan.go/snb.go,
	// ninDogReference in cmd/n5e/nindog.go, summonTribeReference in
	// cmd/n5e/companions.go, puppetToolStatBlock's DamageResistance/
	// DamageImmunity/ConditionImmunity in cmd/n5e/puppets.go), so these
	// three columns are simply never read for any kind but "custom" — a
	// "custom" companion has no upgrade catalog or tribe table behind it to
	// compute them from at all, the same reasoning Attacks/Traits/Notes
	// above are already plain player-entered text for every kind.
	Resistances         string
	Immunities          string
	ConditionImmunities string

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
	titan_specialization, barrier_current, barrier_max, save_proficiencies, temp_hp,
	resistances, immunities, condition_immunities, is_demon_foe`

func scanCompanion(row interface{ Scan(...any) error }) (Companion, error) {
	var c Companion
	var isArmorForm, isDemonFoe int
	err := row.Scan(
		&c.ID, &c.Kind, &c.Name, &c.SummonTribeSlug,
		&c.AC, &c.HPCurrent, &c.HPMax, &c.Speed, &c.FlySpeed, &c.Str, &c.Dex, &c.Con, &c.Int, &c.Wis, &c.Cha,
		&c.Attacks, &c.Traits, &c.Notes, &c.ArmorChassis, &isArmorForm, &c.Size,
		&c.MatryoshkaGroupID, &c.MatryoshkaJutsuSlots, &c.SortOrder,
		&c.NinDogBreed, &c.JutsuSlotsCurrent, &c.JutsuSlotsMax,
		&c.TitanSpecialization, &c.BarrierCurrent, &c.BarrierMax, &c.SaveProficiencies, &c.TempHP,
		&c.Resistances, &c.Immunities, &c.ConditionImmunities, &isDemonFoe,
	)
	c.IsArmorForm = isArmorForm != 0
	c.IsDemonFoe = isDemonFoe != 0
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
// EXCEPT hp_current, AC, hp_max, Speed, Fly Speed, the six ability scores,
// Jutsu Slots Max, Barrier Max, and Size, in a single UPDATE, the same
// "one form, whole-field autosave on blur" shape as SetBio — the three
// free-text blocks, the name, and the one-time crafting/breed/
// specialization picks all belong to one logical form, so a blur on any
// field resends all of them. Every field listed above is deliberately
// excluded and lives behind its own dedicated <form>/endpoint instead
// (AddCompanionHP/SetCompanionHP, AddCompanionIntField/
// SetCompanionIntField, SetCompanionSize/SetCompanionOverride — see each
// one's own doc comment for why): this function unconditionally overwrites
// every column it's given, so if any of them were included here, this
// form's own "blur on any field resends every field" behavior would
// resubmit an EMPTY value (those fields live outside this form now) on
// every unrelated field's blur and silently wipe them back to blank.
// Confirmed live, twice: first for AC/hp_max, then again for Speed/Fly
// Speed/the six ability scores/Size once those also grew their own
// delta-editable/pin-capable fields — both times the column was still
// being read and unconditionally overwritten here even after its own
// field was split out of this form, silently nulling it on the very next
// Armor Chassis/Notes/whatever-else-remains-here edit.
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
// resistances/immunities/conditionImmunities are only ever meaningful for
// kind = "custom" (see Companion's own field doc and migration 0081), but
// are written unconditionally here for every kind, same as attacks/traits/
// notes just above them — cmd/n5e/companions.go's handleCompanionSave only
// ever sends real text for these three on a "custom" companion in the first
// place (the template's own {{if eq .Companion.Kind "custom"}} guard is what
// keeps the input fields from existing at all for any other kind), and an
// UPDATE with three empty strings is a harmless no-op for a kind that never
// populates them.
func SetCompanionFields(charDB *sql.DB, characterID, companionID int64, name, summonTribeSlug string,
	attacks, traits, notes, armorChassis string, isArmorForm bool, ninDogBreed string,
	titanSpecialization string,
	resistances, immunities, conditionImmunities string,
	isDemonFoe bool,
) error {
	armorFormValue := 0
	if isArmorForm {
		armorFormValue = 1
	}
	demonFoeValue := 0
	if isDemonFoe {
		demonFoeValue = 1
	}
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			name = ?, summon_tribe_slug = ?,
			attacks = ?, traits = ?, notes = ?,
			armor_chassis = CASE WHEN armor_chassis = '' THEN ? ELSE armor_chassis END,
			is_armor_form = ?,
			nin_dog_breed = CASE WHEN nin_dog_breed = '' THEN ? ELSE nin_dog_breed END,
			titan_specialization = CASE WHEN titan_specialization = '' THEN ? ELSE titan_specialization END,
			resistances = ?, immunities = ?, condition_immunities = ?,
			is_demon_foe = ?,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		name, summonTribeSlug,
		attacks, traits, notes, armorChassis, armorFormValue, ninDogBreed,
		titanSpecialization,
		resistances, immunities, conditionImmunities,
		demonFoeValue,
		companionID, characterID,
	)
	return err
}

// SetCompanionStatDefaults writes AC/HP-max/Speed/six ability scores (plus
// flying speed and size) on a puppet companion from the Puppet Tool's
// baseline unit card (see cmd/n5e/companions.go's puppetToolMaxHP/
// puppetToolDefaultAC) — called on every render of the Puppets tab and the
// companion popup alike (see cmd/n5e/puppets.go's loadPuppetsTabData),
// unconditionally overwriting these columns with the freshly computed
// total. A Puppet Tool's stats are fully auto-calculated with no manual
// override (companion_fields.html no longer renders these as editable
// inputs at all), so there is no player-set value left to protect —
// unlike the COALESCE-based prefill this function used to be, back when a
// "Sync" button let the player choose whether to accept a newer computed
// total. hp_current is deliberately excluded (not a formula target — see
// handleCompanionHP's own doc for why it stays a separately tracked,
// player-adjusted pool).
//
// Deliberately NOT routed through SetCompanionFields (the whole-field
// autosave path): that function is called on every blur of any OTHER
// field, and if it also carried hp_current would silently null it back out
// (see SetCompanionFields' own doc). This function writes only the columns
// listed here and nothing else, so it can't reintroduce that class of bug
// even though it runs at a different time than a normal field save.
// flySpeed is deliberately a NullInt64 rather than a plain int: a puppet
// with no flying-speed source has no flying speed at all, which is not the
// same as a flying speed of 0 — passing NULL clears the column instead of
// leaving a stale flying speed behind once its granting source is removed.
func SetCompanionStatDefaults(charDB *sql.DB, characterID, companionID int64,
	ac, hpMax, speed int64, flySpeed sql.NullInt64,
	str, dex, con, intScore, wis, cha int64, size string,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = ?, hp_max = ?,
			speed = ?, fly_speed = ?,
			str_score = ?, dex_score = ?,
			con_score = ?, int_score = ?,
			wis_score = ?, cha_score = ?,
			size = ?,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpMax, speed, flySpeed, str, dex, con, intScore, wis, cha, size,
		companionID, characterID,
	)
	return err
}

// SetNinDogStatDefaults prefills AC/HP-current/HP-max/Speed/Jutsu-Slots-
// current+max/six ability scores on a freshly created nin-dog companion
// from Beast Master's own computed baseline (see cmd/n5e/nindog.go's
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
//
// jutsuSlotsCurrent was missing entirely until this fix — the UPDATE wrote
// jutsu_slots_max but never jutsu_slots_current, so a brand-new Nin-Dog got
// its correct max (per ninDogJutsuSlotsMaxForRank) but a NULL/zero current
// pool, the same "max right, current zero" shape the player-character
// creation flow's own zero-vitals bug had (handleCreateFinish, fixed
// separately). Mirrors the hpCurrent/hpMax pattern immediately above:
// callers pass the same computed max for both params on creation.
func SetNinDogStatDefaults(charDB *sql.DB, characterID, companionID int64,
	ac, hpCurrent, hpMax, speed, jutsuSlotsCurrent, jutsuSlotsMax int64,
	str, dex, con, intScore, wis, cha int64,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = COALESCE(ac, ?), hp_current = COALESCE(hp_current, ?), hp_max = COALESCE(hp_max, ?),
			speed = COALESCE(speed, ?),
			jutsu_slots_current = COALESCE(jutsu_slots_current, ?), jutsu_slots_max = COALESCE(jutsu_slots_max, ?),
			str_score = COALESCE(str_score, ?), dex_score = COALESCE(dex_score, ?),
			con_score = COALESCE(con_score, ?), int_score = COALESCE(int_score, ?),
			wis_score = COALESCE(wis_score, ?), cha_score = COALESCE(cha_score, ?),
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpCurrent, hpMax, speed, jutsuSlotsCurrent, jutsuSlotsMax, str, dex, con, intScore, wis, cha,
		companionID, characterID,
	)
	return err
}

// SetTitanStatDefaults prefills AC/HP-current/HP-max/Speed/Barrier-current/
// Barrier-max/six ability scores on a freshly created titan companion from
// Ordnance Training's own computed baseline (see cmd/n5e/titan.go's
// prefillTitanStatDefaults) — the Titan equivalent of SetNinDogStatDefaults
// just above. Every column is written via COALESCE(column, ?), the same
// "never overwrite an already-set value" contract SetCompanionStatDefaults/
// SetNinDogStatDefaults document — a field the player has already set is
// never touched, which is what makes this safe to call again later (e.g. a
// future per-render backfill for older companions) without risk of
// clobbering a manual edit.
func SetTitanStatDefaults(charDB *sql.DB, characterID, companionID int64,
	ac, hpCurrent, hpMax, speed, barrierCurrent, barrierMax int64,
	str, dex, con, intScore, wis, cha int64,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = COALESCE(ac, ?),
			hp_current = COALESCE(hp_current, ?), hp_max = COALESCE(hp_max, ?),
			speed = COALESCE(speed, ?),
			barrier_current = COALESCE(barrier_current, ?), barrier_max = COALESCE(barrier_max, ?),
			str_score = COALESCE(str_score, ?), dex_score = COALESCE(dex_score, ?),
			con_score = COALESCE(con_score, ?), int_score = COALESCE(int_score, ?),
			wis_score = COALESCE(wis_score, ?), cha_score = COALESCE(cha_score, ?),
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpCurrent, hpMax, speed, barrierCurrent, barrierMax, str, dex, con, intScore, wis, cha,
		companionID, characterID,
	)
	return err
}

// SetSNBStatDefaults prefills AC/HP-current/HP-max/Speed/six ability scores
// on a freshly created snb companion from S.N.B Specialist's own computed
// baseline (see cmd/n5e/snb.go's prefillSNBStatDefaults) — the S.N.B
// equivalent of SetNinDogStatDefaults/SetTitanStatDefaults just above. No
// jutsuSlots/barrier params: an S.N.B has neither resource by default (a
// Combat Programming: Caster pick grants Jutsu Slots, but that count is
// shown as reference text only, not tracked as a stored pool — see
// snb.go's own header doc). Every column is written via COALESCE(column,
// ?), the same "never overwrite an already-set value" contract
// SetCompanionStatDefaults/SetNinDogStatDefaults/SetTitanStatDefaults
// document — a field the player has already set is never touched, which is
// what makes this safe to call again later without risk of clobbering a
// manual edit.
func SetSNBStatDefaults(charDB *sql.DB, characterID, companionID int64,
	ac, hpCurrent, hpMax, speed int64,
	str, dex, con, intScore, wis, cha int64,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = COALESCE(ac, ?),
			hp_current = COALESCE(hp_current, ?), hp_max = COALESCE(hp_max, ?),
			speed = COALESCE(speed, ?),
			str_score = COALESCE(str_score, ?), dex_score = COALESCE(dex_score, ?),
			con_score = COALESCE(con_score, ?), int_score = COALESCE(int_score, ?),
			wis_score = COALESCE(wis_score, ?), cha_score = COALESCE(cha_score, ?),
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpCurrent, hpMax, speed, str, dex, con, intScore, wis, cha,
		companionID, characterID,
	)
	return err
}

// SetNinDogStatDefaultsLive unconditionally overwrites AC/HP-max/Speed/
// Jutsu-Slots-Max/six ability scores/Size on every render — the Nin-Dog
// equivalent of SetCompanionStatDefaults (puppet), called from
// cmd/n5e/nindog.go's loadNinDogReference now that a Nin-Dog's stats need to
// keep pace with the owning character's level instead of staying frozen at
// whatever SetNinDogStatDefaults' one-time creation prefill wrote. Unlike
// that COALESCE-based prefill, every field here is a fresh formula result
// the caller has already resolved against character_companion_overrides
// (migration 0079) — a pinned field's OVERRIDDEN value is what gets passed
// in here, not the raw auto value, so this function itself doesn't need to
// know about pins at all. hp_current/jutsu_slots_current are deliberately
// excluded, the same "not a formula target" exclusion
// SetCompanionStatDefaults' own doc explains for hp_current. Size is
// included (unlike those two) because it's a formula target too — without
// this, the raw column would stay stuck at "" forever for any Nin-Dog whose
// Size was never manually pinned, since nothing else keeps it in sync.
func SetNinDogStatDefaultsLive(charDB *sql.DB, characterID, companionID int64,
	ac, hpMax, speed, jutsuSlotsMax int64,
	str, dex, con, intScore, wis, cha int64, size string,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = ?, hp_max = ?,
			speed = ?,
			jutsu_slots_max = ?,
			str_score = ?, dex_score = ?,
			con_score = ?, int_score = ?,
			wis_score = ?, cha_score = ?,
			size = ?,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpMax, speed, jutsuSlotsMax, str, dex, con, intScore, wis, cha, size,
		companionID, characterID,
	)
	return err
}

// SetTitanStatDefaultsLive unconditionally overwrites AC/HP-max/Speed/
// Barrier-max/six ability scores/Size on every render — the Titan equivalent
// of SetNinDogStatDefaultsLive just above, called from cmd/n5e/titan.go's
// loadTitanReference. Same override-already-resolved-by-the-caller contract
// as SetNinDogStatDefaultsLive, including Size for the same "keep the raw
// column in sync with its own formula" reason given there. hp_current/
// barrier_current are deliberately excluded, the same resource-pool
// exclusion SetCompanionStatDefaults' own doc explains for hp_current.
func SetTitanStatDefaultsLive(charDB *sql.DB, characterID, companionID int64,
	ac, hpMax, speed, barrierMax int64,
	str, dex, con, intScore, wis, cha int64, size string,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = ?, hp_max = ?,
			speed = ?,
			barrier_max = ?,
			str_score = ?, dex_score = ?,
			con_score = ?, int_score = ?,
			wis_score = ?, cha_score = ?,
			size = ?,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpMax, speed, barrierMax, str, dex, con, intScore, wis, cha, size,
		companionID, characterID,
	)
	return err
}

// SetSNBStatDefaultsLive unconditionally overwrites AC/HP-max/Speed/six
// ability scores/Size on every render — the S.N.B equivalent of
// SetNinDogStatDefaultsLive/SetTitanStatDefaultsLive just above, called from
// cmd/n5e/snb.go's loadSNBReference. Same override-already-resolved-by-the-
// caller contract as those two functions, including Size for the same
// "keep the raw column in sync with its own formula" reason given there. No
// jutsuSlots/barrier params, the same absent-resource treatment
// SetSNBStatDefaults' own doc explains. HP-current is deliberately excluded,
// the same resource-pool exclusion SetCompanionStatDefaults' own doc
// explains for hp_current.
func SetSNBStatDefaultsLive(charDB *sql.DB, characterID, companionID int64,
	ac, hpMax, speed int64,
	str, dex, con, intScore, wis, cha int64, size string,
) error {
	_, err := charDB.Exec(`
		UPDATE character_companions SET
			ac = ?, hp_max = ?,
			speed = ?,
			str_score = ?, dex_score = ?,
			con_score = ?, int_score = ?,
			wis_score = ?, cha_score = ?,
			size = ?,
			updated_at = datetime('now')
		WHERE id = ? AND character_id = ?`,
		ac, hpMax, speed, str, dex, con, intScore, wis, cha, size,
		companionID, characterID,
	)
	return err
}

// SetCompanionSaveProficiencies overwrites one companion's whole saving-
// throw proficiency list outright (proficiencies is the already-joined
// comma-separated string — see cmd/n5e/companion_saves.go's own join/parse
// helpers) — unlike ArmorChassis/NinDogBreed/TitanSpecialization above, this
// is NOT a locked-once-chosen field, so (unlike those) it carries no CASE
// WHEN guard: a save proficiency toggle can freely add or remove an ability
// at any time, the same "ordinary editable field" treatment Speed or an
// ability score already gets.
func SetCompanionSaveProficiencies(charDB *sql.DB, characterID, companionID int64, proficiencies string) error {
	_, err := charDB.Exec(
		`UPDATE character_companions SET save_proficiencies = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		proficiencies, companionID, characterID,
	)
	return err
}

// AddCompanionHP applies a signed delta to one companion's hp_current,
// floored at 0 — the companion-popup equivalent of the main sheet's SetHP,
// including the same temp-HP cascade: damage is absorbed by temp_hp first
// (see migration 0080_companion_temp_hp.sql), and only the remainder comes
// off hp_current. A NULL hp_current/temp_hp (never touched yet) is treated
// as 0 either way — sql.NullInt64's zero value already does this, so a
// companion that has never had temp HP simply never absorbs anything and
// behaves exactly as it did before temp_hp existed.
func AddCompanionHP(charDB *sql.DB, characterID, companionID int64, delta int64) (int64, error) {
	tx, err := charDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var current, tempHP sql.NullInt64
	if err := tx.QueryRow(
		`SELECT hp_current, temp_hp FROM character_companions WHERE id = ? AND character_id = ?`, companionID, characterID,
	).Scan(&current, &tempHP); err != nil {
		return 0, fmt.Errorf("load hp_current: %w", err)
	}
	newHP := current.Int64
	if delta < 0 {
		damage := -delta
		absorbed := damage
		if absorbed > tempHP.Int64 {
			absorbed = tempHP.Int64
		}
		if absorbed > 0 {
			tempHP.Int64 -= absorbed
			damage -= absorbed
		}
		newHP -= damage
		if newHP < 0 {
			newHP = 0
		}
	} else {
		newHP += delta
	}
	if _, err := tx.Exec(
		`UPDATE character_companions SET hp_current = ?, temp_hp = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		newHP, tempHP, companionID, characterID,
	); err != nil {
		return 0, fmt.Errorf("update hp_current: %w", err)
	}
	return newHP, tx.Commit()
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
	"speed": true, "fly_speed": true,
	"str_score": true, "dex_score": true, "con_score": true,
	"int_score": true, "wis_score": true, "cha_score": true,
	"temp_hp": true,
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

// SetCompanionSize sets a companion's size category outright (or clears it
// back to "" — the size column is NOT NULL with an empty-string default,
// see migration 0032_companion_size.sql, so there is no separate NULL state to preserve
// the way the numeric stat fields have). Called by handleCompanionSize for
// every kind alike — a plain player-typed value for Summon/Custom (no Size
// formula of their own), or the raw column a formula kind's own
// loadXReference recomputes on every render (SetCompanionOverride is what
// makes that recompute respect a manual pin; this call just keeps the raw
// column in sync so it reads back correctly before the next render runs).
// See companionOverrideFields' own doc for why Size needs a dedicated
// setter instead of joining companionIntFields, which is int-only.
func SetCompanionSize(charDB *sql.DB, characterID, companionID int64, value string) error {
	_, err := charDB.Exec(
		`UPDATE character_companions SET size = ?, updated_at = datetime('now') WHERE id = ? AND character_id = ?`,
		value, companionID, characterID,
	)
	return err
}

// SetCompanionOverride pins one companion's own auto-computed field
// (companionOverrideFields in cmd/n5e/companions.go — AC, Max HP, Speed,
// Fly Speed, the six ability scores, Jutsu Slots Max, Barrier Max, Size) to
// value, or un-pins it if value is blank — the same blank-deletes/
// non-blank-upserts contract charstore.SetOverride already gives the main
// sheet's own character_overrides table (see migration
// 0079_companion_overrides.sql for why companions get their own separate
// table rather than reusing that one). The field itself is never validated
// here — every caller already goes through a fixed set of route
// registrations naming a literal field string, the same "whitelisted by
// construction at the call site" trust SetCompanionIntField's own
// companionIntFields gives its callers.
func SetCompanionOverride(charDB *sql.DB, companionID int64, field, value string) error {
	if value == "" {
		_, err := charDB.Exec(
			`DELETE FROM character_companion_overrides WHERE companion_id = ? AND field = ?`,
			companionID, field,
		)
		return err
	}
	_, err := charDB.Exec(`
		INSERT INTO character_companion_overrides (companion_id, field, value) VALUES (?, ?, ?)
		ON CONFLICT (companion_id, field) DO UPDATE SET value = excluded.value`,
		companionID, field, value,
	)
	return err
}

// GetCompanionOverrides returns every field a player has manually pinned on
// one companion, field -> stored value — read once per companion at the top
// of each kind's own per-render loader (loadPuppetsTabData/
// loadNinDogReference/loadTitanReference/loadSNBReference) and consulted
// before falling back to that field's own computed default. An empty map
// (never a nil-vs-empty distinction callers need to care about) for a
// companion with nothing pinned.
func GetCompanionOverrides(charDB *sql.DB, companionID int64) (map[string]string, error) {
	rows, err := charDB.Query(
		`SELECT field, value FROM character_companion_overrides WHERE companion_id = ?`, companionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var field, value string
		if err := rows.Scan(&field, &value); err != nil {
			return nil, err
		}
		out[field] = value
	}
	return out, rows.Err()
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
