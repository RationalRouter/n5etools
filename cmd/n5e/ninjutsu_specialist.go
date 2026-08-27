package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Ninjutsu Specialist's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Ninjutsu Specialist entry documents: Chakra Recovery and
// Jutsu Breaker's own use-pools (customResourceGrants, see
// custom_resources.go), Efficient Molding's own cap+catalog pick (level 3,
// class_options list_name "Efficient Molding", cap 2->6 per
// class_level_resources — the 17-row catalog's own subclass_slug mistag is
// already fixed, see internal/parse/subclasses.go and migration
// 0045_efficient_molding_subclass_tag.sql; the SEPARATE "uses per rest"
// spend pool behind whichever moldings a character knows lives in
// customResourceGrants, same "pool here, known-list there" split Hunters
// Exploits/Hunters Patterns already established), and Refined Ninjutsu
// (level 1, cap 2->8) / Ninjutsu Master (level 20, cap 1) — two independent
// picks sourced from the character's own known jutsu (character_jutsu)
// rather than a rules-database catalog, a genuinely new picker shape: no
// existing catalog pick in this codebase sources its Available list from a
// character's own known jutsu rather than a static class_options/
// class_features table.
//
// Ninjutsu Master's own "always deal maximum damage with the chosen Jutsu"
// payload is now a display-only annotation, not a damage-roll override: no
// such override mechanism exists anywhere in this app (jutsu damage dice are
// pinned per row, never computed) — ninjutsuMasterAlwaysMaxDamageSlug below
// resolves the already-tracked pick to a jutsu_slug, and characters.go's
// loadCharacterJutsuSheet/jutsuSheetRow.AlwaysMaxDamage renders it as a
// "Max Damage" badge next to that jutsu wherever jutsu are listed (Known
// Jutsu, Attacks & Jutsu) — the player still applies the rule by hand when
// rolling.
//
// Everything else (the per-subclass "motes/marks/vials/gems/shards" combat
// resources, Refined Ninjutsu's own which-benefit-this-casting choice) is
// documented, not modeled — see CLASS_AUDIT.md's Ninjutsu Specialist detail
// entry for the full breakdown. The 22 bespoke per-subclass Efficient
// Molding auto-grants (ninjutsuMoldingAutoGrants below) ARE modeled: each
// subclass's 6th- and 18th-level feature grants one free, uncapped Efficient
// Molding pick.
const ninjutsuSpecialistSlug = "class/ninjutsu-specialist"

// ninjutsuSpecialistClassLevel returns the character's own Ninjutsu
// Specialist class level, or 0 if they have none — mirrors
// hunterNinClassLevel/taijutsuSpecialistClassLevel.
func (s *server) ninjutsuSpecialistClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, ninjutsuSpecialistSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// ninjutsuMoldingOption is one not-yet-known Efficient Molding, offered in
// the picker.
type ninjutsuMoldingOption struct {
	Slug        string
	Name        string
	Description string
}

// knownNinjutsuMolding is one entry on the Known Efficient Moldings list —
// either a player pick (Slug set, has a remove button, counts against
// MoldingCap) or an auto-granted bespoke molding (Granted true, no Slug, no
// remove button, doesn't count against the cap) — same shape
// knownMartialTechnique (taijutsu.go) already establishes for the identical
// pattern. Description is only populated for a Granted entry (the granting
// feature's own text, which already states the molding's Alternate Cost
// inline) — a player pick's own description is fetched fresh by
// handleNinjutsuMoldingDetail's popup route instead.
type knownNinjutsuMolding struct {
	Slug        string
	Name        string
	Granted     bool
	Description string
}

// ninjutsuMoldingAutoGrants: feature slug -> the bespoke Efficient Molding
// name it grants for free. Every one of the 11 Ninjutsu Focus subclasses has
// a 6th-level and an 18th-level feature each granting one unique molding
// technique — unlike martialTechniqueAutoGrants (taijutsu.go), each of these
// 22 moldings is its OWN distinct subclass_features row (not 2 bundled into
// 1 feature), so this map holds one name per slug rather than a slice. None
// of the 22 has a class_options row of its own (only the 17-row class-wide
// catalog does), so — same as Martial Techniques' auto-grants — this map
// only needs names: the granting feature's own description (already shown
// in the Features & Traits panel, and reused verbatim here) carries the
// molding's full text, Alternate Cost included.
var ninjutsuMoldingAutoGrants = map[string]string{
	"class/ninjutsu-specialist/group/ninjutsu-focus/blaze-walker/feature/crimson-molding":                  "Crimson Molding",
	"class/ninjutsu-specialist/group/ninjutsu-focus/blaze-walker/feature/blaze-walker-technique":           "Blaze Walker Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/hijutsu-elitist/feature/generational-molding":          "Generational Molding",
	"class/ninjutsu-specialist/group/ninjutsu-focus/hijutsu-elitist/feature/elitist-technique":             "Elitist Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/lightning-breaker/feature/gigawatt-molding":            "Gigawatt Molding",
	"class/ninjutsu-specialist/group/ninjutsu-focus/lightning-breaker/feature/lightning-breaker-technique": "Lightning Breaker Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/sanguine-master/feature/macabre-techniques":            "Macabre Techniques",
	"class/ninjutsu-specialist/group/ninjutsu-focus/sanguine-master/feature/sanguine-master-technique":     "Sanguine Master Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/seal-break":                      "Seal Break",
	"class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/scribe-master":                   "Scribe Master",
	"class/ninjutsu-specialist/group/ninjutsu-focus/stone-crusher/feature/tectonic-plate-technique":        "Tectonic Plate Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/stone-crusher/feature/stone-crusher-technique":         "Stone Crusher Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/storm-terror/feature/swirling-technique":               "Swirling Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/storm-terror/feature/storm-terror-technique":           "Storm Terror Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/summoner/feature/combination-technique":                "Combination Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/summoner/feature/synchronization-technique":            "Synchronization Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/chakra-infusion-technique":       "Chakra Infusion Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/the-final-lesson":                "The Final Lesson",
	"class/ninjutsu-specialist/group/ninjutsu-focus/trace-talent/feature/void-break-technique":             "Void Break Technique",
	"class/ninjutsu-specialist/group/ninjutsu-focus/trace-talent/feature/talent-voider":                    "Talent Voider",
	"class/ninjutsu-specialist/group/ninjutsu-focus/tsunami/feature/raining-vortex":                        "Raining Vortex",
	"class/ninjutsu-specialist/group/ninjutsu-focus/tsunami/feature/tsunami-technique":                     "Tsunami Technique",
}

// loadEfficientMoldingCatalog reads Efficient Molding's 17-row class-wide
// catalog (list_name "Efficient Molding", subclass_slug NULL after the
// parser/migration fix above).
func (s *server) loadEfficientMoldingCatalog() ([]ninjutsuMoldingOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND list_name = 'Efficient Molding' ORDER BY name`, ninjutsuSpecialistSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ninjutsuMoldingOption
	for rows.Next() {
		var o ninjutsuMoldingOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// splitNinjutsuMoldingPicks classifies the catalog against a character's
// stored picks — same shape splitHunterPicks/splitIntelligenceOperativePicks
// already establish.
func splitNinjutsuMoldingPicks(catalog []ninjutsuMoldingOption, picked map[string]bool) (known []knownNinjutsuMolding, available []ninjutsuMoldingOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownNinjutsuMolding{Slug: o.Slug, Name: o.Name})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// knownJutsuOption is one of the character's own already-known Ninjutsu,
// offered as a candidate for Refined Ninjutsu or Ninjutsu Master —
// JutsuID is the character_jutsu row id (not a rules slug: a custom jutsu
// has no slug of its own), which is what gets stored in
// character_ninjutsu_jutsu_picks and what ON DELETE CASCADE keys off of.
type knownJutsuOption struct {
	JutsuID        int64
	Name           string
	Rank           string
	Description    string
	HasCombination bool
}

// knownNinjutsuJutsuPick is one entry on a Refined Ninjutsu/Ninjutsu Master
// Known list.
type knownNinjutsuJutsuPick struct {
	JutsuID int64
	Name    string
	Rank    string
}

// loadKnownNinjutsu reads every jutsu of classification "Ninjutsu" the
// character currently knows (character_jutsu, both a published jutsu_slug
// resolved against rules.db's v_jutsu and a player-created custom_jutsu
// resolved entirely within charDB), for Refined Ninjutsu/Ninjutsu Master's
// own pickers to filter and offer. A stale published jutsu_slug (removed in
// a rules update) is silently skipped, same tolerance
// loadCharacterJutsuSheet already extends to the main Known Jutsu list.
func (s *server) loadKnownNinjutsu(characterID int64) ([]knownJutsuOption, error) {
	rows, err := s.charDB.Query(`
		SELECT id, jutsu_slug, custom_jutsu_id FROM character_jutsu WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id        int64
		jutsuSlug sql.NullString
		customID  sql.NullInt64
	}
	var stored []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.jutsuSlug, &r.customID); err != nil {
			rows.Close()
			return nil, err
		}
		stored = append(stored, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []knownJutsuOption
	for _, r := range stored {
		var name, classification, rank, keywords, description string
		if r.jutsuSlug.Valid {
			err := s.rulesDB.QueryRow(
				`SELECT name, classification, rank, keywords, description FROM v_jutsu WHERE slug = ?`, r.jutsuSlug.String,
			).Scan(&name, &classification, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue // stale slug (rules update) — skip rather than break the picker
			}
			if err != nil {
				return nil, err
			}
		} else if r.customID.Valid {
			err := s.charDB.QueryRow(
				`SELECT name, classification, rank, keywords, description FROM custom_jutsu WHERE id = ?`, r.customID.Int64,
			).Scan(&name, &classification, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else {
			continue
		}
		if classification != "Ninjutsu" {
			continue
		}
		out = append(out, knownJutsuOption{
			JutsuID:        r.id,
			Name:           name,
			Rank:           rank,
			Description:    description,
			HasCombination: strings.Contains(keywords, "Combination"),
		})
	}
	return out, nil
}

// ninjutsuMasterCap: Ninjutsu Master unlocks at 20th level with a single
// pick ("select one Ninjutsu of C-Rank or lower"). No class_level_resources
// chart exists for a flat cap-of-1 unlock — hand-curated the same way
// martialDieSize is.
func ninjutsuMasterCap(level int) int {
	if level >= 20 {
		return 1
	}
	return 0
}

// ninjutsuMasterAlwaysMaxDamageSlug resolves Ninjutsu Master's own chosen
// jutsu (character_ninjutsu_jutsu_picks, category "ninjutsu_master") to the
// v_jutsu slug jutsuSheetRow's AlwaysMaxDamage badge annotates (characters.go's
// loadCharacterJutsuSheet) — "" when the character has no pick, or has
// dropped below 20th level since making one (ninjutsuMasterCap gates the
// pick itself the same way KnownMaster is hidden below 20th in
// loadNinjutsuSpecialistTabData). The pick is stored as a character_jutsu
// row id rather than a slug directly (see knownJutsuOption's own doc
// comment), so a custom-jutsu pick (jutsu_slug NULL) resolves to "" too —
// custom jutsu never surface in jutsuSheetRow at all, so there is no row
// left to annotate.
func (s *server) ninjutsuMasterAlwaysMaxDamageSlug(characterID int64) (string, error) {
	level, err := s.ninjutsuSpecialistClassLevel(characterID)
	if err != nil {
		return "", err
	}
	if ninjutsuMasterCap(level) == 0 {
		return "", nil
	}
	picks, err := charstore.ListNinjutsuJutsuPicks(s.charDB, characterID, charstore.NinjutsuPickMaster)
	if err != nil {
		return "", err
	}
	if len(picks) == 0 {
		return "", nil
	}
	var slug sql.NullString
	err = s.charDB.QueryRow(`SELECT jutsu_slug FROM character_jutsu WHERE id = ?`, picks[0]).Scan(&slug)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return slug.String, nil
}

// awakenedScrollFeatureSlug identifies Scribe Master's own subclass feature
// Awakened Scroll — gates the seal-storage picker below on the character
// actually having reached this feature (subclass-gated, unlike Refined
// Ninjutsu/Efficient Molding, which are base-class-wide), the same
// hasGrantedFeature check refinedRefinementBonus uses below.
const awakenedScrollFeatureSlug = "class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/awakened-scroll"

// awakenedScrollCap: "your scroll has two open jutsu seals... You learn to
// create two additional jutsu seals at 6th and 10th levels." No
// class_level_resources chart exists for this bracket — hand-curated the
// same way ninjutsuMasterCap is: 2 at 2nd, 4 at 6th, 6 at 10th. Only the
// "store a Ninjutsu you already know" half of this feature is modeled here;
// storing a Jutsu "from another source, such as a ninjutsu scroll or a
// willing creature" (up to 1 rank above the character's own highest known
// rank) has no mechanism anywhere in this app for temporarily holding a
// jutsu the character doesn't actually know, and stays manual.
func awakenedScrollCap(level int) int {
	switch {
	case level >= 10:
		return 6
	case level >= 6:
		return 4
	case level >= 2:
		return 2
	default:
		return 0
	}
}

// refinedRefinementFeatSlug is feat/class/refined-refinement ("You can
// Refine 1 additional Ninjutsu. You can Refine additional Ninjutsu at 12th
// and 20th Ninjutsu Specialist levels."), prerequisite "At least 4+ levels
// in Ninjutsu Specialist".
const refinedRefinementFeatSlug = "feat/class/refined-refinement"

// refinedRefinementBonus mirrors weaponFocusBonusSlots' shape: a flat +1 for
// taking the feat at all, plus a further +1 at 12th and +1 at 20th Ninjutsu
// Specialist levels, per the feat's own text.
func refinedRefinementBonus(grantedFeatures []grantedFeatureRow, level int) int {
	if !hasGrantedFeature(grantedFeatures, refinedRefinementFeatSlug) {
		return 0
	}
	bonus := 1
	if level >= 12 {
		bonus++
	}
	if level >= 20 {
		bonus++
	}
	return bonus
}

// ninjutsuSpecialistTabData is the sheet_ninjutsu_specialist box's full
// data.
type ninjutsuSpecialistTabData struct {
	MoldingCap       int
	MoldingUsed      int
	KnownMolding     []knownNinjutsuMolding
	AvailableMolding []ninjutsuMoldingOption

	RefinedCap       int
	RefinedUsed      int
	KnownRefined     []knownNinjutsuJutsuPick
	AvailableRefined []knownJutsuOption

	MasterCap       int
	MasterUsed      int
	KnownMaster     []knownNinjutsuJutsuPick
	AvailableMaster []knownJutsuOption

	AwakenedScrollCap       int
	AwakenedScrollUsed      int
	KnownAwakenedScroll     []knownNinjutsuJutsuPick
	AvailableAwakenedScroll []knownJutsuOption
}

// loadNinjutsuSpecialistTabData returns nil for a character with no
// Ninjutsu Specialist levels — the template gates the whole box's existence
// on this being non-nil, same treatment HunterTechniques/IntelligenceOperative
// get. Each of the three sub-sections independently gates itself on its own
// Cap being > 0.
func (s *server) loadNinjutsuSpecialistTabData(characterID int64, sheet *charsheet.Sheet) (*ninjutsuSpecialistTabData, error) {
	level, err := s.ninjutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}

	data := &ninjutsuSpecialistTabData{}

	data.MoldingCap, err = s.classLevelResourceInt(ninjutsuSpecialistSlug, "Efficient Moldings", level)
	if err != nil {
		return nil, err
	}
	if data.MoldingCap > 0 {
		catalog, err := s.loadEfficientMoldingCatalog()
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListNinjutsuMoldingPicks(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.MoldingUsed = len(picks)
		data.KnownMolding, data.AvailableMolding = splitNinjutsuMoldingPicks(catalog, pickedSet)

		// The 22 bespoke per-subclass moldings (ninjutsuMoldingAutoGrants)
		// are auto-known, free molding techniques — they never occupy one of
		// MoldingUsed's cap-gated slots, so they're appended to KnownMolding
		// after the cap accounting above, not folded into it.
		for _, f := range grantedFeatures {
			if name, ok := ninjutsuMoldingAutoGrants[f.Slug]; ok {
				data.KnownMolding = append(data.KnownMolding, knownNinjutsuMolding{Name: name, Granted: true, Description: f.Description})
			}
		}
		sort.Slice(data.KnownMolding, func(i, j int) bool { return data.KnownMolding[i].Name < data.KnownMolding[j].Name })
	}

	data.RefinedCap, err = s.classLevelResourceInt(ninjutsuSpecialistSlug, "Refined Ninjutsu", level)
	if err != nil {
		return nil, err
	}
	data.RefinedCap += refinedRefinementBonus(grantedFeatures, level)
	data.AwakenedScrollCap = 0
	if hasGrantedFeature(grantedFeatures, awakenedScrollFeatureSlug) {
		data.AwakenedScrollCap = awakenedScrollCap(level)
	}
	if data.RefinedCap > 0 || ninjutsuMasterCap(level) > 0 || data.AwakenedScrollCap > 0 {
		known, err := s.loadKnownNinjutsu(characterID)
		if err != nil {
			return nil, err
		}

		if data.RefinedCap > 0 {
			picks, err := charstore.ListNinjutsuJutsuPicks(s.charDB, characterID, charstore.NinjutsuPickRefined)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[int64]bool, len(picks))
			for _, id := range picks {
				pickedSet[id] = true
			}
			data.RefinedUsed = len(picks)
			for _, o := range known {
				if o.HasCombination {
					continue // Refined Ninjutsu excludes the Combination keyword
				}
				if pickedSet[o.JutsuID] {
					data.KnownRefined = append(data.KnownRefined, knownNinjutsuJutsuPick{JutsuID: o.JutsuID, Name: o.Name, Rank: o.Rank})
				} else {
					data.AvailableRefined = append(data.AvailableRefined, o)
				}
			}
		}

		data.MasterCap = ninjutsuMasterCap(level)
		if data.MasterCap > 0 {
			picks, err := charstore.ListNinjutsuJutsuPicks(s.charDB, characterID, charstore.NinjutsuPickMaster)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[int64]bool, len(picks))
			for _, id := range picks {
				pickedSet[id] = true
			}
			data.MasterUsed = len(picks)
			for _, o := range known {
				if jutsuRankOrder[o.Rank] > jutsuRankOrder["C"] {
					continue // "C-Rank or lower" only
				}
				if pickedSet[o.JutsuID] {
					data.KnownMaster = append(data.KnownMaster, knownNinjutsuJutsuPick{JutsuID: o.JutsuID, Name: o.Name, Rank: o.Rank})
				} else {
					data.AvailableMaster = append(data.AvailableMaster, o)
				}
			}
		}

		if data.AwakenedScrollCap > 0 {
			picks, err := charstore.ListNinjutsuJutsuPicks(s.charDB, characterID, charstore.NinjutsuPickAwakenedScroll)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[int64]bool, len(picks))
			for _, id := range picks {
				pickedSet[id] = true
			}
			data.AwakenedScrollUsed = len(picks)
			for _, o := range known {
				if pickedSet[o.JutsuID] {
					data.KnownAwakenedScroll = append(data.KnownAwakenedScroll, knownNinjutsuJutsuPick{JutsuID: o.JutsuID, Name: o.Name, Rank: o.Rank})
				} else {
					data.AvailableAwakenedScroll = append(data.AvailableAwakenedScroll, o)
				}
			}
		}
	}

	return data, nil
}

// handleNinjutsuMoldingAdd learns one Efficient Molding, gated by the
// character's own current cap.
func (s *server) handleNinjutsuMoldingAdd(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing molding", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for ninjutsu molding add:", err)
		return
	}
	data, err := s.loadNinjutsuSpecialistTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load ninjutsu specialist for molding add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no levels in Ninjutsu Specialist", http.StatusBadRequest)
		return
	}
	if data.MoldingUsed >= data.MoldingCap {
		http.Error(w, "no molding slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.AvailableMolding {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid molding to learn", http.StatusBadRequest)
		return
	}
	if err := charstore.AddNinjutsuMoldingPick(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add ninjutsu molding pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_ninjutsu_specialist")
}

// handleNinjutsuMoldingDelete drops one known Efficient Molding — freely,
// at any time, same "trust the player" boundary every other pick removal
// on this sheet draws.
func (s *server) handleNinjutsuMoldingDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing molding", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveNinjutsuMoldingPick(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove ninjutsu molding pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_ninjutsu_specialist")
}

// handleNinjutsuJutsuPickAdd builds one of Refined Ninjutsu/Ninjutsu
// Master's own "pick a known jutsu" routes — shared since both validate/
// store identically, differing only in which of ninjutsuSpecialistTabData's
// own fields govern the cap and the currently-pickable list. Same shape
// handleHunterPickAdd/handleIntelligenceOperativePickAdd already establish,
// keyed by a character_jutsu row id (form field "jutsu_id") instead of a
// rules-database option_slug.
func (s *server) handleNinjutsuJutsuPickAdd(
	category charstore.NinjutsuJutsuPickCategory,
	used func(*ninjutsuSpecialistTabData) int,
	cap func(*ninjutsuSpecialistTabData) int,
	available func(*ninjutsuSpecialistTabData) []knownJutsuOption,
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
		jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
		if err != nil {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for ninjutsu jutsu pick add:", err)
			return
		}
		data, err := s.loadNinjutsuSpecialistTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load ninjutsu specialist for jutsu pick add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Ninjutsu Specialist", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		valid := false
		for _, o := range available(data) {
			if o.JutsuID == jutsuID {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if err := charstore.AddNinjutsuJutsuPick(s.charDB, id, category, jutsuID); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add ninjutsu jutsu pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_ninjutsu_specialist")
	}
}

// addAwakenedScrollPick validates and stores one Awakened Scroll pick —
// extracted from handleNinjutsuJutsuPickAdd's generic three-category factory
// (above) so the Awakened Scroll popup's own route (ninjutsu_specialist_
// awakened_scroll_popup.go) shares the identical validation path as the
// Core-sheet's own AJAX route (handleAwakenedScrollAdd, just below) instead
// of reimplementing it — same "extract, don't duplicate" shape
// addSNBUpgradePick (science_nin_subclasses.go) establishes. Refined
// Ninjutsu/Ninjutsu Master still go through the generic factory unchanged
// — only Awakened Scroll (Scribe Master only) needed its own popup.
func (s *server) addAwakenedScrollPick(id int64, jutsuID int64) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for awakened scroll add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadNinjutsuSpecialistTabData(id, sheet)
	if err != nil {
		log.Println("load ninjutsu specialist for awakened scroll add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.AwakenedScrollCap == 0 {
		return http.StatusBadRequest, "character has no Awakened Scroll seals available"
	}
	if data.AwakenedScrollUsed >= data.AwakenedScrollCap {
		return http.StatusBadRequest, "no seals remaining"
	}
	valid := false
	for _, o := range data.AvailableAwakenedScroll {
		if o.JutsuID == jutsuID {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid pick"
	}
	if err := charstore.AddNinjutsuJutsuPick(s.charDB, id, charstore.NinjutsuPickAwakenedScroll, jutsuID); err != nil {
		log.Println("add awakened scroll pick:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleAwakenedScrollAdd is addAwakenedScrollPick's own Core-sheet AJAX
// wrapper, registered in place of the generic handleNinjutsuJutsuPickAdd
// factory this route used before the Awakened Scroll popup existed.
func (s *server) handleAwakenedScrollAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	if status, msg := s.addAwakenedScrollPick(id, jutsuID); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_ninjutsu_specialist")
}

// handleNinjutsuJutsuPickDelete builds one category's "forget a pick"
// route — freely, at any time.
func (s *server) handleNinjutsuJutsuPickDelete(category charstore.NinjutsuJutsuPickCategory) http.HandlerFunc {
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
		jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
		if err != nil {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveNinjutsuJutsuPick(s.charDB, id, category, jutsuID); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove ninjutsu jutsu pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_ninjutsu_specialist")
	}
}

// handleNinjutsuMoldingDetail serves the click-to-open popup for a Known
// Efficient Molding — same mechanism handleHunterPickDetail/
// handleIntelligenceOperativePickDetail already use, reusing the same
// hunter_pick_detail_card template. Not character-scoped — the catalog is
// static rules content. Refined Ninjutsu/Ninjutsu Master picks don't get an
// equivalent popup: they reference the character's own already-known jutsu,
// whose full text is already reachable from the Known Jutsu/Attacks &
// Jutsu sections elsewhere on the sheet.
func (s *server) handleNinjutsuMoldingDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var name, description string
	err := s.rulesDB.QueryRow(`
		SELECT name, description FROM class_options
		WHERE slug = ? AND class_slug = ? AND list_name = 'Efficient Molding'`,
		slug, ninjutsuSpecialistSlug,
	).Scan(&name, &description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load ninjutsu molding detail:", err)
		return
	}

	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render ninjutsu molding detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render ninjutsu molding detail:", err)
	}
}
