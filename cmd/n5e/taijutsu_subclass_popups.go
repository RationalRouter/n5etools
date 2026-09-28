package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/charsheet"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// taijutsu_subclass_popups.go: every Taijutsu Specialist subclass tracker
// popup. Each section below is one subclass's popup, all built from the
// shared pattern documented in subclass_tracker_popup.go. Tested by
// taijutsu_subclass_popups_test.go.

// The "Ruin" subclass tracker popup — the
// Anti-Chakra Wavelength 2-of-10 keyword pick, pulled out of sheet_
// martial_techniques (character_sheet.html) into its own popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from. Two independent keyword <select>s, not a catalog picker —
// it isn't part of subclassTrackerSection, so it's hand-rolled here rather
// than routed through the shared subclass_tracker_section partial (same
// shape as character_science_nin_technobi.html's own S.E.N.T <select>).

func ruinPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/taijutsu-specialist/ruin"
}

func (s *server) handleRuinPopup(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		var exists int
		if s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&exists) == nil && exists == 0 {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for ruin popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadMartialTechniquesTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for ruin popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.ruin"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-martial-techniques"}
	if data == nil || data.Ruin == nil {
		popup.EmptyHint = tr("subclass_tracker.ruin.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_taijutsu_ruin.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_taijutsu_ruin.html", popup, map[string]any{
		"Ruin": data.Ruin,
	})
}

// handleAntiChakraWavelengthPopup is setAntiChakraWavelength's own
// redirect-based equivalent for the Ruin popup.
func (s *server) handleAntiChakraWavelengthPopup(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	first := strings.TrimSpace(r.FormValue("keyword1"))
	second := strings.TrimSpace(r.FormValue("keyword2"))
	if status, msg := s.setAntiChakraWavelength(id, first, second); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, ruinPopupPath(id), http.StatusSeeOther)
}

// The "Stancer" subclass tracker popup —
// Stancer's own Mixed Martial Arts (3rd level) and Stance Blending (9th
// level) known-stance picks, pulled out of sheet_martial_techniques
// (character_sheet.html) into their own popup. See subclass_tracker_
// popup.go's header doc for the shared pattern this is built from. Both
// picks are single freely re-editable Taijutsu Stance <select>s, not
// catalog pickers — neither is part of subclassTrackerSection, so both are
// hand-rolled here rather than routed through the shared subclass_tracker_
// section partial (same shape as character_science_nin_technobi.html's own
// S.E.N.T <select>).

func stancerPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/taijutsu-specialist/stancer"
}

func (s *server) handleStancerPopup(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		var exists int
		if s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&exists) == nil && exists == 0 {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for stancer popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadMartialTechniquesTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for stancer popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.stancer"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-martial-techniques"}
	if data == nil || (data.StancerMixedMartialArts == nil && data.StancerStanceBlending == nil) {
		popup.EmptyHint = tr("subclass_tracker.stancer.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_taijutsu_stancer.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_taijutsu_stancer.html", popup, map[string]any{
		"MixedMartialArts": data.StancerMixedMartialArts,
		"StanceBlending":   data.StancerStanceBlending,
	})
}

// handleStancerMixedMartialArtsPopup is setStancerMixedMartialArtsStance's
// own redirect-based equivalent for the Stancer popup.
func (s *server) handleStancerMixedMartialArtsPopup(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setStancerMixedMartialArtsStance(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, stancerPopupPath(id), http.StatusSeeOther)
}

// handleStancerStanceBlendingPopup is setStancerStanceBlendingStance's own
// redirect-based equivalent for the Stancer popup.
func (s *server) handleStancerStanceBlendingPopup(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setStancerStanceBlendingStance(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, stancerPopupPath(id), http.StatusSeeOther)
}

// The "Passionate Flame" subclass
// tracker popup — Fists of Iron's bonus-dice readout and the re-selectable
// Hand Wraps of Passion pick, pulled out of sheet_martial_techniques
// (character_sheet.html) into their own popup. See subclass_tracker_
// popup.go's header doc for the shared pattern this is built from. Hand
// Wraps of Passion is a single freely re-editable <select>, not a catalog
// picker — it isn't part of subclassTrackerSection, so it's hand-rolled
// here rather than routed through the shared subclass_tracker_section
// partial (same shape as character_science_nin_technobi.html's own S.E.N.T
// <select>).

func passionateFlamePopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/taijutsu-specialist/passionate-flame"
}

func (s *server) handlePassionateFlamePopup(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		var exists int
		if s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&exists) == nil && exists == 0 {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for passionate flame popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadMartialTechniquesTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for passionate flame popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.passionate_flame"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-martial-techniques"}
	if data == nil || data.PassionateFlame == nil {
		popup.EmptyHint = tr("subclass_tracker.passionate_flame.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_taijutsu_passionate_flame.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_taijutsu_passionate_flame.html", popup, map[string]any{
		"PassionateFlame": data.PassionateFlame,
	})
}

// handleHandWrapsOfPassionPopup is setHandWrapsOfPassion's own
// redirect-based equivalent for the Passionate Flame popup.
func (s *server) handleHandWrapsOfPassionPopup(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setHandWrapsOfPassion(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, passionateFlamePopupPath(id), http.StatusSeeOther)
}
