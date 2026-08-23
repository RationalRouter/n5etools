package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// taijutsu_stancer_popup.go: the "Stancer" subclass tracker popup —
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
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for stancer popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Stancer", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || (data.StancerMixedMartialArts == nil && data.StancerStanceBlending == nil) {
		popup.EmptyHint = "This character has no Stancer stance picks available yet — Mixed Martial Arts grants at 3rd-level Stancer, Stance Blending at 9th."
		s.renderSubclassTrackerPopup(w, "character_taijutsu_stancer.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_taijutsu_stancer.html", popup, map[string]any{
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
