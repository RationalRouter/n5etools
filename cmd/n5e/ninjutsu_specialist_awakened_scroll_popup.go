package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// ninjutsu_specialist_awakened_scroll_popup.go: the "Awakened Scroll"
// subclass tracker popup — Scribe Master's own Awakened Scroll seal picker,
// pulled out of sheet_ninjutsu_specialist (character_sheet.html) into its
// own popup. See subclass_tracker_popup.go's header doc for the shared
// pattern this is built from. Refined Ninjutsu/Ninjutsu Master (Ninjutsu
// Specialist's other two known-jutsu picks) stay inline on the Core sheet —
// both are base-class picks every Ninjutsu Focus shares, not subclass-
// exclusive, so neither is part of this pattern.
//
// Awakened Scroll sources its Available list from the character's own known
// Ninjutsu (knownJutsuOption) rather than a rules-database catalog, so its
// picker form fields are "jutsu_id", not "option_slug" — this doesn't fit
// subclassTrackerSection's shared markup (hard-coded to "option_slug"), so
// the section is hand-rolled here, same shape character_ninjutsu_specialist's
// own Refined Ninjutsu/Ninjutsu Master inline markup already used.

// awakenedScrollPopupPath is the Awakened Scroll popup's own URL.
func awakenedScrollPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/ninjutsu-specialist/awakened-scroll"
}

// handleAwakenedScrollPopup serves the "Awakened Scroll" popup. Renders
// with an explanatory hint for a character with no Awakened Scroll seals
// yet (below 2nd-level Scribe Master, or a different Ninjutsu Focus
// entirely) — reachable only by visiting this URL directly after a respec,
// since the sidebar button that links here is itself gated on the same
// AwakenedScrollCap check.
func (s *server) handleAwakenedScrollPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for awakened scroll popup:", err)
		return
	}
	data, err := s.loadNinjutsuSpecialistTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load ninjutsu specialist for awakened scroll popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Awakened Scroll", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-ninjutsu-specialist"}
	if data == nil || data.AwakenedScrollCap == 0 {
		popup.EmptyHint = "This character has no Awakened Scroll seals yet — Awakened Scroll grants this at 2nd-level Scribe Master."
		s.renderSubclassTrackerPopup(w, "character_ninjutsu_specialist_awakened_scroll.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_ninjutsu_specialist_awakened_scroll.html", popup, map[string]any{
		"NinjutsuSpecialist": data,
	})
}

// handleAwakenedScrollPopupAdd is addAwakenedScrollPick's own redirect-based
// wrapper for the Awakened Scroll popup (ninjutsu_specialist.go).
func (s *server) handleAwakenedScrollPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	http.Redirect(w, r, awakenedScrollPopupPath(id), http.StatusSeeOther)
}

// handleAwakenedScrollPopupDelete is a thin redirect-based wrapper around
// RemoveNinjutsuJutsuPick — not subclassTrackerPopupDelete's generic
// factory, since that factory is hard-typed to
// charstore.ScienceNinSubclassPickCategory/RemoveScienceNinSubclassPick
// (a slug-keyed table), while Awakened Scroll's own picks are keyed by
// jutsu_id under charstore.NinjutsuJutsuPickCategory/RemoveNinjutsuJutsuPick.
// Removing a pick never needs a budget/cap check, so this stays a thin
// wrapper rather than sharing a "core" method with the Core-sheet's own
// handleNinjutsuJutsuPickDelete factory.
func (s *server) handleAwakenedScrollPopupDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.RemoveNinjutsuJutsuPick(s.charDB, id, charstore.NinjutsuPickAwakenedScroll, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove awakened scroll pick (popup):", err)
		return
	}
	http.Redirect(w, r, awakenedScrollPopupPath(id), http.StatusSeeOther)
}
