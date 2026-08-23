package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// taijutsu_passionate_flame_popup.go: the "Passionate Flame" subclass
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
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for passionate flame popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Passionate Flame", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.PassionateFlame == nil {
		popup.EmptyHint = "This character isn't a Passionate Flame Taijutsu Specialist."
		s.renderSubclassTrackerPopup(w, "character_taijutsu_passionate_flame.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_taijutsu_passionate_flame.html", popup, map[string]any{
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
