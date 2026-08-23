package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// cooking_nin_blend_enhancements_popup.go: the "Nature's Blend Enhancements"
// subclass tracker popup — Gastrochemist's own Enhancement picker, pulled
// out of sheet_cooking_nin (character_sheet.html) into its own popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from.
//
// Sources its Available list from the character's own known jutsu of their
// Nature's Blend release element(s) (knownJutsuOption) rather than a
// rules-database catalog, and pairs each pick with a second field
// (enhancement_type) — this doesn't fit subclassTrackerSection's shared
// markup (hard-coded to a single "option_slug"), so the section is
// hand-rolled here, same shape character_genjutsu_twisted_casting.html's
// own hand-rolled section uses for its own single extra field.

// blendEnhancementsPopupPath is the Nature's Blend Enhancements popup's own
// URL.
func blendEnhancementsPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/blend-enhancements"
}

// handleBlendEnhancementsPopup serves the "Nature's Blend Enhancements"
// popup. Renders with an explanatory hint for a character with no
// Enhancement slots yet (not a Gastrochemist, or below the level granting
// the first slot) — reachable only by visiting this URL directly after a
// respec, since the sidebar button that links here is itself gated on the
// same BlendEnhancementCap check.
func (s *server) handleBlendEnhancementsPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for blend enhancements popup:", err)
		return
	}
	data, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for blend enhancements popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Nature's Blend Enhancements", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.BlendEnhancementCap == 0 {
		popup.EmptyHint = "This character has no Nature's Blend Enhancements yet — Gastrochemist grants this at 2nd level."
		s.renderSubclassTrackerPopup(w, "character_cooking_nin_blend_enhancements.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_cooking_nin_blend_enhancements.html", popup, map[string]any{
		"CookingNin": data,
	})
}

// handleBlendEnhancementsPopupAdd is addCookingNinBlendEnhancementPick's
// own redirect-based wrapper for the Blend Enhancements popup
// (cooking_nin.go).
func (s *server) handleBlendEnhancementsPopupAdd(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing jutsu", http.StatusBadRequest)
		return
	}
	enhancementType := strings.TrimSpace(r.FormValue("enhancement_type"))
	if status, msg := s.addCookingNinBlendEnhancementPick(id, jutsuID, enhancementType); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, blendEnhancementsPopupPath(id), http.StatusSeeOther)
}

// handleBlendEnhancementsPopupDelete is a thin redirect-based wrapper
// around RemoveCookingNinBlendEnhancementPick, matching
// handleCookingNinBlendEnhancementDelete's own Core-sheet route — removing
// a pick never needs a budget/cap check, so this stays a thin wrapper
// rather than sharing a "core" method with that route.
func (s *server) handleBlendEnhancementsPopupDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing jutsu", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveCookingNinBlendEnhancementPick(s.charDB, id, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove cooking-nin blend enhancement pick (popup):", err)
		return
	}
	http.Redirect(w, r, blendEnhancementsPopupPath(id), http.StatusSeeOther)
}
