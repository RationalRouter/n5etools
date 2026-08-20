package charstore

import "database/sql"

// HunterNinPickCategory is one of the four cap-gated catalogs Hunter-Nin's
// base class grants — see cmd/n5e/hunter_nin.go.
type HunterNinPickCategory string

const (
	// HunterPickLethalPrecision: Lethal Precision's own 1st-level Taijutsu/
	// Bukijutsu pick ("Select one between Taijutsu & Bukijutsu. You can
	// cast the chosen Jutsu type using Dexterity in place of Strength for
	// all calculations. You cannot switch this choice later.") — two fixed
	// option slugs ('taijutsu'/'bukijutsu'), not class_options rows, same
	// hand-curated shape as HunterPickDefensiveTactic. cap 1, no re-cap:
	// the book's "cannot switch this choice later" is enforced by
	// handleHunterPickAdd rejecting any add once the category is already
	// at its cap, same as HunterPickWardenWeapon's own no-re-cap pick.
	HunterPickLethalPrecision HunterNinPickCategory = "lethal_precision" // base class, cap 1, no re-cap, 1st level
	HunterPickPattern         HunterNinPickCategory = "pattern"
	HunterPickExploit         HunterNinPickCategory = "exploit"
	HunterPickDefensiveTactic HunterNinPickCategory = "defensive_tactic"

	// The remaining 6 subclasses' own 3rd-level (cap 1, ->2@10th unless
	// noted) technique/property/item picks — see cmd/n5e/hunter_nin.go's
	// own option-catalog doc comments for where each one's real text
	// actually lives in rules.db (every one of them is mistagged onto an
	// unrelated 7th/17th-level feature's own description row, not the
	// granting feature's own row).
	HunterPickWardenWeapon         HunterNinPickCategory = "warden_weapon"          // Blade Warden, cap 1, no re-cap
	HunterPickWardenWeaponProperty HunterNinPickCategory = "warden_weapon_property" // Blade Warden, cap 1->2@10th
	HunterPickMedicalTechnique     HunterNinPickCategory = "medical_technique"      // Necrotic Hand, cap 1->2@10th
	HunterPickShadowTechnique      HunterNinPickCategory = "shadow_technique"       // Grave Stalker, cap 1->2@10th
	HunterPickArsenalItem          HunterNinPickCategory = "arsenal_item"           // Arsenalist, cap 4->6@10th
	HunterPickToxicTechnique       HunterNinPickCategory = "toxic_technique"        // Undertaker, cap 1->2@10th
	HunterPickViceTechnique        HunterNinPickCategory = "vice_technique"         // Vice Agent, cap 1->2@10th
	HunterPickVoidTechnique        HunterNinPickCategory = "void_technique"         // Void Walker, cap 1->2@10th
	HunterPickProstheticAttachment HunterNinPickCategory = "prosthetic_attachment"  // Wolves Legacy, cap 2->4@10th

	// HunterPickWolfTechnique: Wolves Legacy's own 10th-level Wolf
	// Techniques pick — a 6-option named-jutsu table (cap 1, no re-cap),
	// unlike the 6 tables above which all live at 3rd level. See
	// cmd/n5e/hunter_nin.go's wolfTechniqueOptions doc comment.
	HunterPickWolfTechnique HunterNinPickCategory = "wolf_technique" // Wolves Legacy, cap 1 @10th
)

// AddHunterNinPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddMartialTechnique already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddHunterNinPick(charDB *sql.DB, characterID int64, category HunterNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_hunter_nin_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveHunterNinPick drops one pick — freely, at any time. The book gates
// re-picking Hunters Exploits to a long rest; this app doesn't enforce rest
// timing, same boundary Martial Adept/Mastery/Puppet Tactics already draw.
func RemoveHunterNinPick(charDB *sql.DB, characterID int64, category HunterNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_hunter_nin_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListHunterNinPicks returns the option_slugs a character has picked within
// one category.
func ListHunterNinPicks(charDB *sql.DB, characterID int64, category HunterNinPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_hunter_nin_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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
