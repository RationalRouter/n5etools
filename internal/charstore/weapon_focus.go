package charstore

import "database/sql"

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
