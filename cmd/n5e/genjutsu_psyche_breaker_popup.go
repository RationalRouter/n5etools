package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// genjutsu_psyche_breaker_popup.go: the "Psyche Breaker" subclass tracker
// popup — Corrupt Thoughts' own 10th-level "select two Genjutsu you know
// that deal damage" pick, pulled out of sheet_genjutsu (character_sheet.
// html) into its own popup. See subclass_tracker_popup.go's header doc for
// the shared pattern this is built from; see genjutsu_twisted_casting_popup.
// go's own header doc for why this is a separate popup rather than bundled
// with Twisted Casting.
//
// Psyche Breaker sources its Available list from the character's own known
// Genjutsu (knownJutsuOption) rather than a rules-database catalog — same
// "jutsu_id" hand-rolled shape as Twisted Casting. The "that deal damage"
// qualifier is not enforced here either (see psycheBreakerCap's own doc
// comment in genjutsu.go).

// psycheBreakerPopupPath is the Psyche Breaker popup's own URL.
func psycheBreakerPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/genjutsu-specialist/psyche-breaker"
}

// handlePsycheBreakerPopup serves the "Psyche Breaker" popup. Renders with
// an explanatory hint for a character with no Psyche Breaker picks
// available yet (below 10th-level Corrupt Thoughts, or a different Genjutsu
// Pledge entirely) — reachable only by visiting this URL directly after a
// respec, since the sidebar button that links here is itself gated on the
// same PsycheBreakerCap check.
func (s *server) handlePsycheBreakerPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for psyche breaker popup:", err)
		return
	}
	data, err := s.loadGenjutsuTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load genjutsu for psyche breaker popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Psyche Breaker", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.PsycheBreakerCap == 0 {
		popup.EmptyHint = "This character isn't a 10th-level (or higher) Corrupt Thoughts Genjutsu Specialist — Psyche Breaker is Corrupt Thoughts' own 10th-level feature."
		s.renderSubclassTrackerPopup(w, "character_genjutsu_psyche_breaker.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_genjutsu_psyche_breaker.html", popup, map[string]any{
		"Genjutsu": data,
	})
}

// handlePsycheBreakerPopupAdd is addGenjutsuJutsuPick's own redirect-based
// wrapper for the Psyche Breaker popup.
func (s *server) handlePsycheBreakerPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	status, msg := s.addGenjutsuJutsuPick(id, charstore.GenjutsuPickPsycheBreaker, jutsuID,
		func(d *genjutsuTabData) int { return d.PsycheBreakerUsed },
		func(d *genjutsuTabData) int { return d.PsycheBreakerCap },
		func(d *genjutsuTabData) []knownJutsuOption { return d.AvailablePsycheBreaker })
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, psycheBreakerPopupPath(id), http.StatusSeeOther)
}

// handlePsycheBreakerPopupDelete is a thin redirect-based wrapper around
// RemoveGenjutsuJutsuPick, matching handleGenjutsuJutsuPickDelete's own
// Core-sheet route — removing a pick never needs a budget/cap check, so
// this stays a thin wrapper rather than sharing a "core" method with that
// route.
func (s *server) handlePsycheBreakerPopupDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.RemoveGenjutsuJutsuPick(s.charDB, id, charstore.GenjutsuPickPsycheBreaker, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove psyche breaker pick (popup):", err)
		return
	}
	http.Redirect(w, r, psycheBreakerPopupPath(id), http.StatusSeeOther)
}
