package charstore

import "database/sql"

// ScoutNinPickCategory is one of Scout-Nin's cap-gated catalog picks — see
// cmd/n5e/scout_nin.go. Mirrors HunterNinPickCategory's shape exactly.
type ScoutNinPickCategory string

const (
	ScoutNinPickShinobiAdept ScoutNinPickCategory = "shinobi_adept"
	ScoutNinPickJackOfAll    ScoutNinPickCategory = "jack_of_all"
	// ScoutNinPickManeuvers is subclass-scoped (each subclass's own
	// Maneuvers catalog uses different option_slugs), unlike the other two
	// categories — cmd/n5e/scout_nin.go's own loadScoutNinTabData handles
	// the "picks from a different, no-longer-current subclass stay stored
	// but don't count against the cap" case, not this package.
	ScoutNinPickManeuvers ScoutNinPickCategory = "maneuvers"
	// ScoutNinPickSignatureTechnique is base-class-wide (Signature
	// Technique, 10th/18th level) over its own 3-option catalog (Hidden/
	// Aggressive/Tactical Technique).
	ScoutNinPickSignatureTechnique ScoutNinPickCategory = "signature_technique"
	// ScoutNinPickMobileSavant is Pathfinder Scout's own 3rd/6th/9th/14th
	// level pick over its 4-jutsu catalog — option_slugs are jutsu slugs,
	// not class_features slugs, unlike every other category here.
	ScoutNinPickMobileSavant ScoutNinPickCategory = "mobile_savant"
	// ScoutNinPickTacticalSuperiority is Tactical Scout's own 6th/14th/20th
	// level cross-subclass Maneuver pick — option_slugs are drawn from
	// several OTHER subclasses' own Maneuvers catalogs, not the
	// character's own chosen subclass.
	ScoutNinPickTacticalSuperiority ScoutNinPickCategory = "tactical_superiority"
	// ScoutNinPickSignatureManeuver is Tactical Scout's own 3rd/9th level
	// pick of an already-known Tactical-keyword Maneuver — option_slugs
	// always overlap with a subset of that character's own ScoutNinPickManeuvers
	// picks.
	ScoutNinPickSignatureManeuver ScoutNinPickCategory = "signature_maneuver"
	// ScoutNinPickSupremeClones is Cloning Scout's own flat 20th-level
	// pick of an already-known Maneuver — like ScoutNinPickSignatureManeuver,
	// option_slugs always overlap with a subset of that character's own
	// ScoutNinPickManeuvers picks, but with no keyword restriction and no
	// escalating cap (always exactly 1 slot).
	ScoutNinPickSupremeClones ScoutNinPickCategory = "supreme_clones"
	// ScoutNinPickVoidSoulJutsu is Trickster Scout's own 3rd-level Void Soul
	// Awakening companion-scoped known-jutsu pick (cmd/n5e/void_soul.go) —
	// option_slugs are jutsu slugs, like ScoutNinPickMobileSavant, but these
	// are jutsu the Void Soul itself knows and casts, not the player, so
	// they are deliberately never written into character_jutsu (see
	// void_soul.go's own header doc).
	ScoutNinPickVoidSoulJutsu ScoutNinPickCategory = "void_soul_jutsu"
)

// AddScoutNinPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddHunterNinPick already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddScoutNinPick(charDB *sql.DB, characterID int64, category ScoutNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_scout_nin_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveScoutNinPick drops one pick — freely, at any time, same "trust the
// player" boundary Hunters Exploits/Martial Techniques already draw.
func RemoveScoutNinPick(charDB *sql.DB, characterID int64, category ScoutNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_scout_nin_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListScoutNinPicks returns the option_slugs a character has picked within
// one category.
func ListScoutNinPicks(charDB *sql.DB, characterID int64, category ScoutNinPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_scout_nin_picks WHERE character_id = ? AND category = ?`,
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
