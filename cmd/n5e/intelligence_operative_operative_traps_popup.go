package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// intelligence_operative_operative_traps_popup.go: the "Operative Traps"
// subclass tracker popup — Tactical Strategist's own Operative Traps
// cap+catalog pick, pulled out of sheet_intelligence_operative
// (character_sheet.html) into its own popup. See subclass_tracker_popup.go's
// header doc for the shared pattern this is built from. Plans (Intelligence
// Operative's other catalog pick) stays inline on the Core sheet — it's a
// base-class pick every Master Strategy shares, not subclass-exclusive, so
// it isn't part of this pattern.

// operativeTrapsPopupPath is the Operative Traps popup's own URL.
func operativeTrapsPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/intelligence-operative/operative-traps"
}

// handleOperativeTrapsPopup serves the "Operative Traps" popup. Renders
// with an explanatory hint for a character with no Operative Traps yet
// (below 3rd-level Tactical Strategist, or a different Master Strategy
// entirely) — reachable only by visiting this URL directly after a respec,
// since the sidebar button that links here is itself gated on the same
// OperativeTrapsCap check.
func (s *server) handleOperativeTrapsPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for operative traps popup:", err)
		return
	}
	data, err := s.loadIntelligenceOperativeTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load intelligence operative for operative traps popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Operative Traps", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-intelligence-operative"}
	if data == nil || data.OperativeTrapsCap == 0 {
		popup.EmptyHint = "This character has no Operative Traps yet — Trap Setter grants this at 3rd-level Tactical Strategist."
		s.renderSubclassTrackerPopup(w, "character_intelligence_operative_operative_traps.html", popup, nil)
		return
	}

	popup.Sections = append(popup.Sections, operativeTrapsSection(id, data))
	s.renderSubclassTrackerPopup(w, "character_intelligence_operative_operative_traps.html", popup, nil)
}

func operativeTrapsSection(characterID int64, data *intelligenceOperativeTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Operative Traps (" + strconv.Itoa(data.OperativeTrapsUsed) + "/" + strconv.Itoa(data.OperativeTrapsCap) + " chosen)",
		Hint:         "Uses set tracked in Special Resources on the Core tab (Operative Traps Set).",
		DeleteAction: "/characters/" + idStr + "/intelligence-operative/operative-traps/delete",
	}
	for _, k := range data.KnownOperativeTraps {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/intelligence-operative-picks/operative-trap/" + k.Slug})
	}
	if data.OperativeTrapsUsed < data.OperativeTrapsCap {
		sec.AddAction = "/characters/" + idStr + "/intelligence-operative/operative-traps/add"
		sec.AddLabel = "Learn Trap"
		for _, o := range data.AvailableOperativeTraps {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// handleOperativeTrapPopupAdd is addOperativeTrapPick's own redirect-based
// wrapper for the Operative Traps popup (intelligence_operative.go).
func (s *server) handleOperativeTrapPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addOperativeTrapPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, operativeTrapsPopupPath(id), http.StatusSeeOther)
}

// handleOperativeTrapPopupDelete is a thin redirect-based wrapper around
// RemoveIntelligenceOperativePick — not subclassTrackerPopupDelete's generic
// factory, since that factory is hard-typed to
// charstore.ScienceNinSubclassPickCategory/RemoveScienceNinSubclassPick,
// while Intelligence Operative's own picks live in a separate table under
// charstore.IntelligenceOperativePickCategory/RemoveIntelligenceOperativePick.
// Removing a pick never needs a budget/cap check, so this stays a thin
// wrapper rather than sharing a "core" method with the Core-sheet's own
// handleIntelligenceOperativePickDelete factory.
func (s *server) handleOperativeTrapPopupDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.RemoveIntelligenceOperativePick(s.charDB, id, charstore.IntelligenceOperativePickOperativeTrap, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove operative trap pick (popup):", err)
		return
	}
	http.Redirect(w, r, operativeTrapsPopupPath(id), http.StatusSeeOther)
}
