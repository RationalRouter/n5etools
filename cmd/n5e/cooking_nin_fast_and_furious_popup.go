package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/i18n"
)

// cooking_nin_fast_and_furious_popup.go: the "Fast and Furious" subclass
// tracker popup — Entremetier Chef's own 2nd-level Int-or-Cha-for-Initiative
// pick, pulled out of sheet_cooking_nin (character_sheet.html) into its own
// popup. See subclass_tracker_popup.go's header doc for the shared pattern
// this is built from.
//
// A single, freely re-editable <select>, not a catalog Known/Available
// list, so this is hand-rolled rather than routed through the shared
// subclass_tracker_section partial — same shape Combat Medic's own Fighting
// Stance <select> already establishes.

// fastAndFuriousPopupPath is the Fast and Furious popup's own URL.
func fastAndFuriousPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/fast-and-furious"
}

// handleFastAndFuriousPopup serves the "Fast and Furious" popup. Renders
// with an explanatory hint for a character without the feature (below
// 2nd-level Entremetier Chef, or a different Cooking Focus entirely) —
// reachable only by visiting this URL directly after a respec, since the
// sidebar button that links here is itself gated on the same
// .CookingNin.FastAndFurious check.
func (s *server) handleFastAndFuriousPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for fast and furious popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadCookingNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for fast and furious popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.fast_and_furious"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-cooking-nin"}
	if data == nil || data.FastAndFurious == nil {
		popup.EmptyHint = tr("subclass_tracker.cooking_fast_and_furious.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_fast_and_furious.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_fast_and_furious.html", popup, map[string]any{
		"FastAndFurious": data.FastAndFurious,
	})
}

// handleFastAndFuriousPopupSet is setFastAndFurious's own redirect-based
// wrapper for the Fast and Furious popup (cooking_nin.go).
func (s *server) handleFastAndFuriousPopupSet(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setFastAndFurious(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, fastAndFuriousPopupPath(id), http.StatusSeeOther)
}
