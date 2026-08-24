package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// taijutsu_ruin_popup.go: the "Ruin" subclass tracker popup — the
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
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for ruin popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Ruin", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-martial-techniques"}
	if data == nil || data.Ruin == nil {
		popup.EmptyHint = "This character isn't a Ruin Taijutsu Specialist."
		s.renderSubclassTrackerPopup(w, "character_taijutsu_ruin.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_taijutsu_ruin.html", popup, map[string]any{
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
