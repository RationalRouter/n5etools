package main

import (
	"database/sql"
	"log"
	"net/http"
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
// Everything else (the per-subclass "motes/marks/vials/gems/shards" combat
// resources, the 22 bespoke Efficient Molding auto-grants, Refined
// Ninjutsu's own which-benefit-this-casting choice, Ninjutsu Master's
// always-max-damage payload) is documented, not modeled — see
// CLASS_AUDIT.md's Ninjutsu Specialist detail entry for the full breakdown.
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
// same shape knownMartialTechnique/knownIntelligenceOperativePick already
// establish (no auto-grants exist for this catalog yet — see the 22
// bespoke per-subclass Efficient Molding auto-grants deferral above — so
// Granted is always false for now, kept for consistency should that ever
// be built).
type knownNinjutsuMolding struct {
	Slug string
	Name string
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
	}

	data.RefinedCap, err = s.classLevelResourceInt(ninjutsuSpecialistSlug, "Refined Ninjutsu", level)
	if err != nil {
		return nil, err
	}
	if data.RefinedCap > 0 || ninjutsuMasterCap(level) > 0 {
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
