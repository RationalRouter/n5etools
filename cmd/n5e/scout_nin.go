package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Superiority Dice is base-class-wide (all 9 subclasses grant their own
// copy, each keyed to their own 3rd-level introducing feature — see
// customResourceGrants in custom_resources.go for the pool itself and the
// Cloning Scout granting-slug exception). This file hand-transcribes the
// count and die-size progressions each subclass's own "Superior <Name>
// Table" states, since class_level_resources has zero rows for
// class/scout-nin — every table was independently re-read from the real
// book text (five of the nine have their own numeric table glued onto a
// later, unrelated feature by the same PDF-extraction column-wrap quirk
// documented elsewhere in this package: Assault's onto Relentless Assault
// [20th], Barrier's onto Rallying Barrier [9th], Cloning's onto Superior
// Cloning [glued cleanly onto Cloning Tactics itself], Tactical's onto
// Unmatched Tactics [20th], Trickster's onto Void Soul Awakening [3rd] —
// none of that affects this hand-transcription, since it reads the
// resolved table values directly rather than relying on which feature row
// the table text happens to sit under).
//
// Every subclass's own count table reduces to one of four shapes once the
// per-level values are laid out side by side:
//   - "a" (Arbiter, Cloning, Trickster — base d8/d8/d6): 3 dice at 3rd,
//     +1 every 3 levels, capping at 8 by 18th.
//   - "b" (Barrier, Elemental, Phantom — base d8/d6/d6): 3 dice at 3rd,
//     +1 every 5 levels, capping at 6 by 18th.
//   - "c" (Assault, Pathfinder — base d4/d6): 3 dice at 3rd, +1 every 4
//     levels, capping at 7 by 19th.
//   - "tactical" (base d10, its own unique table): 3 dice at 3rd, +1 every
//     3 levels through 12th, then +1 every 2 levels, capping at 10 by 20th.
func scoutNinSuperiorityDice(shape string, level int) int {
	switch shape {
	case "a":
		switch {
		case level >= 18:
			return 8
		case level >= 15:
			return 7
		case level >= 12:
			return 6
		case level >= 9:
			return 5
		case level >= 6:
			return 4
		case level >= 3:
			return 3
		}
	case "b":
		switch {
		case level >= 18:
			return 6
		case level >= 13:
			return 5
		case level >= 8:
			return 4
		case level >= 3:
			return 3
		}
	case "c":
		switch {
		case level >= 19:
			return 7
		case level >= 15:
			return 6
		case level >= 11:
			return 5
		case level >= 7:
			return 4
		case level >= 3:
			return 3
		}
	case "tactical":
		switch {
		case level >= 20:
			return 10
		case level >= 18:
			return 9
		case level >= 16:
			return 8
		case level >= 14:
			return 7
		case level >= 12:
			return 6
		case level >= 9:
			return 5
		case level >= 6:
			return 4
		case level >= 3:
			return 3
		}
	}
	return 0
}

// scoutNinSuperiorityDieSize is each subclass's own base Superiority Die
// size (Assault Scout's own "Superior Assault" text: d4; Elemental/
// Pathfinder/Phantom/Trickster: d6; Arbiter/Barrier/Cloning: d8; Tactical:
// d10 — every value independently re-read against the book, not inferred).
// Assault Scout's own Untapped Potential (6th level) grows this die as the
// character levels: "Your Superiority Die grows 1 step, into a D6. It
// repeats this process at 9th (d8), 14th (d10) and 17th levels (d12)." No
// other subclass's die size changes with level. Untapped Potential's own
// second clause (regaining 1 Superiority Die on a critical hit or a
// natural 20 saving throw) is a combat-trigger effect this app has no
// mechanism to automate and is not modeled here — only the die-size
// readout is.
func scoutNinSuperiorityDieSize(subclassSlug string, level int) string {
	switch subclassSlug {
	case "assault-scout":
		switch {
		case level >= 17:
			return "d12"
		case level >= 14:
			return "d10"
		case level >= 9:
			return "d8"
		case level >= 6:
			return "d6"
		default:
			return "d4"
		}
	case "arbiter-scout", "barrier-scout", "cloning-scout":
		return "d8"
	case "tactical-scout":
		return "d10"
	default: // elemental-scout, pathfinder-scout, phantom-scout, trickster-scout
		return "d6"
	}
}

// Group 1 base-class picks closing further Scout-Nin gaps beyond
// Superiority Dice itself: Shinobi Adept and Jack of All, Master of None
// (both cap+catalog picks over a hand-curated, live-read class_features
// list — neither has a class_options catalog of its own, same "book states
// the options directly as their own feature rows" shape
// medical_nin.go's medicalDoctrineSlugs already established), Fighting
// Stance (base class, level 1 — generalizes taijutsu.go's
// buildFightingStanceView/storeFightingStanceChoice to a Taijutsu-OR-Weapon
// stance choice instead of Taijutsu-only), and Absolute Authority (Arbiter
// Scout's flat AC bonus — see internal/features/grants.go's
// flatACBonusGrants).
//
// Deft Explorer's own Mobile sub-tier (6th level, automatically granted —
// not a player pick) is rendered as a computed informational readout
// (deftExplorerMobileFeatureSlug below) rather than new persistent
// swim/climb/wall/water-walk speed fields: every one of those speeds is
// simply set equal to the character's own Speed, so a stored field would
// only ever duplicate a value already computed elsewhere on the sheet, the
// same "no bonus-composition mechanism to wire a new value into" boundary
// unarmedDamageDieSize (taijutsu.go) already draws for a similarly
// display-only value. Tireless (11th level) is a Group 2 activated ability
// (spend a full turn Action, once per long rest, to recover all spent
// Superiority Dice) — no action-economy/rest-count tracking exists
// anywhere in this app to automate that, so it's left as the blanket-
// granted class_features text in Features & Traits, not modeled further
// here.
//
// Combat/Control/Mobility/Skill/Support (Jack of All's 5 Generalizations)
// are, like Medical Doctrine's 4 options, blanket-granted class_features
// rows once the character reaches 5th level — this picker tracks which
// ones the player has actually chosen (for display, and because the cap
// itself is meaningful), but does not suppress the other, unpicked
// options' own text from Features & Traits, matching the precedent
// medicalDoctrineSlugs/loadMedicalDoctrineCatalog already established
// rather than inventing a new "hide the unpicked options" mechanism here.
// Combat's attack/damage bonus, Control's save-DC bonus, and Skill's flat
// check bonus are correspondingly NOT wired into the attack table/save-DC
// computation — same informational-readout-only boundary as Mobile above;
// each Generalization's own current-level bonus is already legible in its
// class_features description, shown via this picker's own tooltip. Mobility
// is the one exception: a real speedGrants entry already existed
// pre-dating this stage (internal/features/grants.go), gated the same
// blanket way (not on this pick), and only needed its level tiers
// corrected to 5/11 to match the class_features level-tagging fix.
const (
	scoutNinSlug                      = "class/scout-nin"
	scoutNinFightingStanceFeatureSlug = "class/scout-nin/feature/fighting-stance"
	jackOfAllFeatureSlug              = "class/scout-nin/feature/jack-of-all-master-of-none"
	deftExplorerMobileFeatureSlug     = "class/scout-nin/feature/mobile"

	// expertOfAllFeatureSlug/unmatchedTacticsFeatureSlug: Tactical Scout's
	// own two Jack of All extensions — "Expert of All, Jack of None" (9th
	// level) lets the character gain the benefit of 2 Generalizations at
	// once; "Unmatched Tactics" (20th level) adds one more on top of that
	// (its own text: "you may gain the benefit of an additional
	// Generalization" — a +1, not the unlimited count its OWN separate
	// "any number of maneuvers... per attack" clause grants for Maneuvers,
	// a different mechanic entirely). Read directly off the character's
	// merged granted features (hasGrantedFeature), the same reliable
	// per-subclass signal ironcladFeaturePrefix/unnaturalTalentFeatureSlug
	// already use elsewhere, rather than a separate subclass-slug check.
	expertOfAllFeatureSlug      = "class/scout-nin/group/scouting-technique/tactical-scout/feature/expert-of-all-jack-of-none"
	unmatchedTacticsFeatureSlug = "class/scout-nin/group/scouting-technique/tactical-scout/feature/unmatched-tactics"

	// scoutNinElementalKnowledgeFeatureSlug/scoutNinElementalResistanceFeatureSlug:
	// Elemental Scout's own 3rd-level Nature Release pick and the 6th-level
	// resistance it feeds — see elemental_affinity.go's
	// elementalScoutAffinitySlots for the pick itself and
	// scoutNinElementalResistanceEntry below for the resistance grant.
	scoutNinElementalKnowledgeFeatureSlug  = "class/scout-nin/group/scouting-technique/elemental-scout/feature/elemental-knowledge"
	scoutNinElementalResistanceFeatureSlug = "class/scout-nin/group/scouting-technique/elemental-scout/feature/elemental-resistance"
)

// The 9 Scouting Technique subclass slugs, named once here since Maneuvers
// Known's cap (below) needs to switch on them and no such constant list
// existed anywhere in this file yet — every other per-subclass lookup in
// this package so far only needed a granting FEATURE slug, not the bare
// subclass slug itself.
const (
	arbiterScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/arbiter-scout"
	assaultScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/assault-scout"
	barrierScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/barrier-scout"
	cloningScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/cloning-scout"
	elementalScoutSubclassSlug  = "class/scout-nin/group/scouting-technique/elemental-scout"
	pathfinderScoutSubclassSlug = "class/scout-nin/group/scouting-technique/pathfinder-scout"
	phantomScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/phantom-scout"
	tacticalScoutSubclassSlug   = "class/scout-nin/group/scouting-technique/tactical-scout"
	tricksterScoutSubclassSlug  = "class/scout-nin/group/scouting-technique/trickster-scout"
)

// scoutNinManeuversKnownCap: each subclass's own "Maneuvers Known" column,
// read directly off the same 9 "Superior <Name> Table"s
// scoutNinSuperiorityDice already hand-transcribes the "Superiority Dice"
// column from (5 of the 9 tables were found glued onto a later, unrelated
// feature by a PDF-extraction column-wrap quirk — see scoutNinSuperiorityDice's
// own doc comment for exactly which; every one of the 9 was independently
// re-read here too, not inferred from the Dice column's own shape).
//
// A pattern falls out once all 9 tables are laid side by side: for every
// subclass whose Dice column follows shape "a" (+1 every 3 levels, cap 8 by
// 18th) or "b" (+1 every 5 levels, cap 6 by 18th), the Maneuvers Known
// column follows the OTHER of that pair — Arbiter/Cloning/Trickster's own
// Dice use shape "a" but their Maneuvers Known column is shape "b"
// (verified against Arbiter's, Cloning's, and Trickster's own printed
// tables); Barrier/Elemental/Phantom's Dice use shape "b" but their
// Maneuvers Known column is shape "a" (verified against all 3). Assault and
// Pathfinder's Dice already use shape "c", and their own Maneuvers Known
// columns are IDENTICAL to their Dice columns, also shape "c" (verified
// against both). Tactical's Maneuvers Known column is likewise identical to
// its own unique "tactical" Dice column (verified against its own table).
// This isn't assumed from the pattern — every one of the 9 values above was
// independently read off its own subclass's printed table text before this
// function was written, and the pattern is simply what the values happen to
// reduce to.
func scoutNinManeuversKnownCap(subclassSlug string, level int) int {
	switch subclassSlug {
	case arbiterScoutSubclassSlug, cloningScoutSubclassSlug, tricksterScoutSubclassSlug:
		return scoutNinSuperiorityDice("b", level)
	case barrierScoutSubclassSlug, elementalScoutSubclassSlug, phantomScoutSubclassSlug:
		return scoutNinSuperiorityDice("a", level)
	case assaultScoutSubclassSlug, pathfinderScoutSubclassSlug:
		return scoutNinSuperiorityDice("c", level)
	case tacticalScoutSubclassSlug:
		return scoutNinSuperiorityDice("tactical", level)
	default:
		return 0
	}
}

// shinobiAdeptSlugs: Shinobi Adept's 10 named options exist as separate
// class_features rows (sort_order 7-13 and 28-30 — scattered around Deft
// Explorer's own sub-tiers, Shinobi's Training, and Signature Jutsu's own
// rows, not grouped together) rather than a class_options catalog list —
// read live here rather than hand-transcribed, same "read the chart, don't
// retype it" discipline loadMedicalDoctrineCatalog already established.
var shinobiAdeptSlugs = []string{
	"class/scout-nin/feature/shinobis-tactics",
	"class/scout-nin/feature/shinobis-general-literacy",
	"class/scout-nin/feature/shinobis-tool-competency",
	"class/scout-nin/feature/shinobis-precision",
	"class/scout-nin/feature/shinobis-edge",
	"class/scout-nin/feature/shinobis-drive",
	"class/scout-nin/feature/shinobis-focus",
	"class/scout-nin/feature/hidden-technique",
	"class/scout-nin/feature/aggressive-technique",
	"class/scout-nin/feature/tactical-technique",
}

// jackOfAllSlugs: Jack of All, Master of None's 5 named Generalizations —
// same "live class_features rows, no class_options catalog" shape as
// Shinobi Adept above. "mobility" (this list) and "mobile"
// (deftExplorerMobileFeatureSlug above) are two entirely independent
// features that happen to share a near-identical name and speed-related
// text: one is Deft Explorer's automatic 6th-level grant, the other is one
// of these 5 player-picked Generalizations, gained starting 5th level —
// never conflate the two slugs.
var jackOfAllSlugs = []string{
	"class/scout-nin/feature/combat",
	"class/scout-nin/feature/control",
	"class/scout-nin/feature/mobility",
	"class/scout-nin/feature/skill",
	"class/scout-nin/feature/support",
}

// scoutNinClassLevel returns the character's own Scout-Nin class level, or
// 0 if they have none — mirrors taijutsuSpecialistClassLevel.
func (s *server) scoutNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, scoutNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// scoutNinSubclassSlug resolves the character's own chosen Scouting
// Technique subclass, if any — mirrors hunterNinSubclassSlug exactly.
func (s *server) scoutNinSubclassSlug(characterID int64) (slug, name string, err error) {
	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return "", "", err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var sc string
		if err := subRows.Scan(&sc); err != nil {
			subRows.Close()
			return "", "", err
		}
		subclassSlugs = append(subclassSlugs, sc)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return "", "", err
	}

	for _, sc := range subclassSlugs {
		var n, classSlug string
		if err := s.rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, sc,
		).Scan(&n, &classSlug); err != nil {
			continue // a stale/removed subclass slug just isn't a match
		}
		if classSlug == scoutNinSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// shinobiAdeptCap: 2 at 2nd level, 4 at 13th ("select two additional
// features").
func shinobiAdeptCap(level int) int {
	switch {
	case level >= 13:
		return 4
	case level >= 2:
		return 2
	default:
		return 0
	}
}

// jackOfAllCap: 1 by default from 5th level, extended for Tactical Scout
// only — 2 once Expert of All, Jack of None (9th) is granted, 3 once
// Unmatched Tactics (20th) is granted on top of that.
func jackOfAllCap(level int, grantedFeatures []grantedFeatureRow) int {
	if level < 5 {
		return 0
	}
	cap := 1
	if hasGrantedFeature(grantedFeatures, expertOfAllFeatureSlug) {
		cap = 2
	}
	if hasGrantedFeature(grantedFeatures, unmatchedTacticsFeatureSlug) {
		cap = 3
	}
	return cap
}

// scoutNinPickOption is one entry in either of Scout-Nin's two hand-curated
// (no class_options row) catalog picks.
type scoutNinPickOption struct {
	Slug        string
	Name        string
	Description string
}

// knownScoutNinPick is one chosen entry on a Known list — no Granted
// variant exists (nothing auto-grants a Shinobi Adept option or a Jack of
// All Generalization for free outside the pick), unlike Hunter-Nin's
// knownHunterPick. Carries its own Description so the Known list can show
// a plain rollover tooltip, same treatment knownMedicalDoctrine gets.
type knownScoutNinPick struct {
	Slug        string
	Name        string
	Description string
}

// loadScoutNinFeatureCatalog reads one hand-curated slug list's rows live
// from class_features, in sort_order — mirrors
// loadMedicalDoctrineCatalog's identical shape, generalized to take any
// slug list since Scout-Nin has two of these (Shinobi Adept, Jack of All)
// rather than Medical-Nin's one.
func (s *server) loadScoutNinFeatureCatalog(slugs []string) ([]scoutNinPickOption, error) {
	placeholders := make([]string, len(slugs))
	args := make([]any, 0, len(slugs)+1)
	args = append(args, scoutNinSlug)
	for i, slug := range slugs {
		placeholders[i] = "?"
		args = append(args, slug)
	}
	rows, err := s.rulesDB.Query(fmt.Sprintf(`
		SELECT slug, name, description FROM class_features
		WHERE class_slug = ? AND slug IN (%s)
		ORDER BY sort_order`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scoutNinPickOption
	for rows.Next() {
		var o scoutNinPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// loadScoutNinManeuverCatalog reads one subclass's own Maneuvers catalog —
// unlike Shinobi Adept/Jack of All (hand-curated slug lists, since the book
// states those options directly in their own class_features rows), the 92
// Maneuvers rows genuinely exist as class_options rows, scoped by their own
// subclass_slug column, so this reads that table directly instead —
// mirrors loadHunterOptionCatalog, generalized to filter by subclass_slug
// rather than list_name since every subclass shares class_slug
// "class/scout-nin" but each only ever sees its own Maneuvers. Also
// returns the catalog's own list_name (e.g. "Arbiter Maneuvers") for the
// sheet's own section heading, read from the data rather than a hand-typed
// per-subclass label map.
func (s *server) loadScoutNinManeuverCatalog(subclassSlug string) ([]scoutNinPickOption, string, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description, list_name FROM class_options
		WHERE class_slug = ? AND subclass_slug = ? ORDER BY name`, scoutNinSlug, subclassSlug)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []scoutNinPickOption
	var listName string
	for rows.Next() {
		var o scoutNinPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description, &listName); err != nil {
			return nil, "", err
		}
		out = append(out, o)
	}
	return out, listName, rows.Err()
}

// splitScoutNinPicks classifies a catalog against a character's stored
// picks — mirrors splitMedicalDoctrinePicks/splitHunterPicks.
func splitScoutNinPicks(catalog []scoutNinPickOption, picked map[string]bool) (known []knownScoutNinPick, available []scoutNinPickOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownScoutNinPick{Slug: o.Slug, Name: o.Name, Description: o.Description})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// scoutNinTabData is the sheet_scout_nin box's full data — nil for a
// character with no Scout-Nin levels, same "no empty state" treatment
// every other class box establishes.
type scoutNinTabData struct {
	ShinobiAdeptCap       int
	ShinobiAdeptUsed      int
	KnownShinobiAdept     []knownScoutNinPick
	AvailableShinobiAdept []scoutNinPickOption

	JackOfAllCap       int
	JackOfAllUsed      int
	KnownJackOfAll     []knownScoutNinPick
	AvailableJackOfAll []scoutNinPickOption

	// ManeuversListName is the character's own subclass Maneuvers list's
	// book name (e.g. "Arbiter Maneuvers"), read live off class_options —
	// empty (and Cap 0) for a character with no chosen subclass yet.
	ManeuversListName  string
	ManeuversCap       int
	ManeuversUsed      int
	KnownManeuvers     []knownScoutNinPick
	AvailableManeuvers []scoutNinPickOption

	// MobileActive is Deft Explorer's own automatic 6th-level grant — see
	// this file's own doc comment for why this is a computed readout
	// (swim/climb/wall/water-walk speed all equal Speed) rather than new
	// persistent fields.
	MobileActive bool
	Speed        int

	// Stance is Scout-Nin's own base-class Fighting Stance pick (1st
	// level) — every Scout-Nin has one, unlike Combat Medic's subclass-
	// gated Martial Competency pick.
	Stance *fightingStanceView
}

// loadScoutNinTabData returns nil for a character with no Scout-Nin
// levels — the template gates the whole box's existence on this being
// non-nil, same treatment HunterTechniques/MedicalNin already establish.
func (s *server) loadScoutNinTabData(characterID int64, sheet *charsheet.Sheet) (*scoutNinTabData, error) {
	scoutNinLevel, err := s.scoutNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if scoutNinLevel == 0 {
		return nil, nil
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}

	data := &scoutNinTabData{
		MobileActive: hasGrantedFeature(grantedFeatures, deftExplorerMobileFeatureSlug),
		Speed:        sheet.Speed,
	}

	data.ShinobiAdeptCap = shinobiAdeptCap(scoutNinLevel)
	if data.ShinobiAdeptCap > 0 {
		catalog, err := s.loadScoutNinFeatureCatalog(shinobiAdeptSlugs)
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickShinobiAdept)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.ShinobiAdeptUsed = len(picks)
		data.KnownShinobiAdept, data.AvailableShinobiAdept = splitScoutNinPicks(catalog, pickedSet)
	}

	data.JackOfAllCap = jackOfAllCap(scoutNinLevel, grantedFeatures)
	if data.JackOfAllCap > 0 {
		catalog, err := s.loadScoutNinFeatureCatalog(jackOfAllSlugs)
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickJackOfAll)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.JackOfAllUsed = len(picks)
		data.KnownJackOfAll, data.AvailableJackOfAll = splitScoutNinPicks(catalog, pickedSet)
	}

	subclassSlug, _, err := s.scoutNinSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	if subclassSlug != "" {
		data.ManeuversCap = scoutNinManeuversKnownCap(subclassSlug, scoutNinLevel)
		if data.ManeuversCap > 0 {
			catalog, listName, err := s.loadScoutNinManeuverCatalog(subclassSlug)
			if err != nil {
				return nil, err
			}
			data.ManeuversListName = listName
			picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickManeuvers)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.KnownManeuvers, data.AvailableManeuvers = splitScoutNinPicks(catalog, pickedSet)
			// Used counts only picks that are actually IN the current
			// subclass's own catalog, not every stored row in this
			// category — unlike Shinobi Adept/Jack of All (base-class-wide,
			// their catalog never shrinks), Maneuvers picks are
			// subclass-scoped, so a character who picked Maneuvers under
			// one subclass and later switched to another would otherwise
			// have orphaned, invisible picks silently eating their new
			// subclass's cap. They're left in storage (never deleted here)
			// rather than lost, so switching back restores them.
			data.ManeuversUsed = len(data.KnownManeuvers)
		}
	}

	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	stanceOptions, err := s.loadStanceOptionsByType("taijutsu", "bukijutsu")
	if err != nil {
		return nil, err
	}
	data.Stance = buildFightingStanceView(choices, stanceOptions, scoutNinFightingStanceFeatureSlug)

	return data, nil
}

// handleScoutNinPickAdd builds one category's "learn a pick" route — shared
// since Shinobi Adept and Jack of All validate/store identically, differing
// only in which of scoutNinTabData's own fields govern the cap and the
// currently-pickable list. Mirrors handleHunterPickAdd exactly.
func (s *server) handleScoutNinPickAdd(
	category charstore.ScoutNinPickCategory,
	used func(*scoutNinTabData) int,
	cap func(*scoutNinTabData) int,
	available func(*scoutNinTabData) []scoutNinPickOption,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for scout-nin pick add:", err)
			return
		}
		data, err := s.loadScoutNinTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load scout-nin for add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Scout-Nin", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		valid := false
		for _, o := range available(data) {
			if o.Slug == slug {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if err := charstore.AddScoutNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add scout-nin pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_scout_nin")
	}
}

// handleScoutNinPickDelete builds one category's "forget a pick" route —
// freely, at any time, same "trust the player" boundary every other cap+
// catalog pick in this codebase already draws.
func (s *server) handleScoutNinPickDelete(category charstore.ScoutNinPickCategory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveScoutNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove scout-nin pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_scout_nin")
	}
}

// handleScoutNinFightingStance records Scout-Nin's own base-class Fighting
// Stance pick — generalizes taijutsu.go's storeFightingStanceChoice to a
// Taijutsu-OR-Weapon stance choice (its own text: "between Taijutsu Stance
// and Weapon Stance") instead of the Taijutsu-only catalog Unarmed
// Technique/Martial Competency offer.
func (s *server) handleScoutNinFightingStance(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("stance_slug"))
	scoutNinLevel, err := s.scoutNinClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin level for fighting stance:", err)
		return
	}
	if scoutNinLevel == 0 {
		http.Error(w, "character has no levels in Scout-Nin", http.StatusBadRequest)
		return
	}
	options, err := s.loadStanceOptionsByType("taijutsu", "bukijutsu")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin stance options:", err)
		return
	}
	if err := s.storeFightingStanceChoice(id, scoutNinFightingStanceFeatureSlug, options, slug); err != nil {
		if err == errInvalidStance {
			http.Error(w, "not a valid stance", http.StatusBadRequest)
		} else {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set scout-nin fighting stance:", err)
		}
		return
	}
	s.respondSheet(w, r, id, "sheet_scout_nin")
}

// scoutNinElementalResistanceEntry resolves Elemental Resistance's own
// "you gain resistance to the damage of your chosen nature release" grant
// (Elemental Scout, 6th level) — not modeled via the static
// passiveTraitGrants table (passive_traits.go) since its Target is the
// player's own Elemental Knowledge pick, not a fixed value that table's
// shape assumes every entry has. Returns nil if the character hasn't
// reached Elemental Resistance yet or hasn't picked a Nature Release.
//
// The same feature's second clause ("add half your Intelligence Modifier"
// to an ally's saving throw against a Nature Release jutsu) is a
// per-roll/reactive bonus, not a standing trait — same boundary this
// codebase already draws elsewhere for combat-trigger effects (e.g.
// Untapped Potential's own "regain a die on a crit" clause) — and isn't
// modeled here.
func scoutNinElementalResistanceEntry(grantedFeatures []grantedFeatureRow, element string) *PassiveTraitEntry {
	if element == "" {
		return nil
	}
	if !hasGrantedFeature(grantedFeatures, scoutNinElementalResistanceFeatureSlug) {
		return nil
	}
	return &PassiveTraitEntry{Target: element, Sources: []string{"Elemental Resistance"}}
}
