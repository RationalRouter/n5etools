package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// cooking_nin_pipe_popup.go: the "Bonus Tool Infusion: Pipe" subclass
// tracker popup — Herbalist's own second Cooking Tool Infusion weapon
// (implement, damage type, and one weapon property each at 3rd/6th/11th
// level), pulled out of sheet_cooking_nin (character_sheet.html) into its
// own popup. See subclass_tracker_popup.go's header doc for the shared
// pattern this is built from.
//
// Every pick here is a single-value, lock-once-chosen <select> — the same
// shape Cooking Tool Infusion's own (base-class, still inline) picks use —
// not a catalog Known/Available list, so this is hand-rolled rather than
// routed through the shared subclass_tracker_section partial, same
// reasoning character_titan_slots.html's own Specialist Crafting <select>
// and character_medical_nin_combat_medic.html's own Fighting Stance
// <select> already establish. pipePopupSetHandler is a small local factory
// (not subclassTrackerPopupDelete's generic, cross-file one) since all 5
// picks share the identical (id, value) -> (status, msg) shape — unlike the
// Add side elsewhere in this pattern, there is no per-catalog budget/cap
// check to differ here.

// pipePopupPath is the Pipe popup's own URL.
func pipePopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/pipe"
}

// handlePipePopup serves the "Bonus Tool Infusion: Pipe" popup. Renders
// with an explanatory hint for a character with no Pipe yet (below
// 3rd-level Herbalist, or a different Cooking Focus entirely) — reachable
// only by visiting this URL directly after a respec, since the sidebar
// button that links here is itself gated on the same
// .CookingNin.BonusToolInfusionPipe check.
func (s *server) handlePipePopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for pipe popup:", err)
		return
	}
	data, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for pipe popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Bonus Tool Infusion: Pipe", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.BonusToolInfusionPipe == nil {
		popup.EmptyHint = "This character has no Bonus Tool Infusion: Pipe yet — Herbalist grants this at 3rd level."
		s.renderSubclassTrackerPopup(w, "character_cooking_nin_pipe.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_cooking_nin_pipe.html", popup, map[string]any{
		"Pipe": data.BonusToolInfusionPipe,
	})
}

// pipePopupSetHandler builds one of the Pipe popup's 5 redirect-based
// "Set" routes, each a thin wrapper around one of cooking_nin.go's own
// setCookingToolPipeImplement/DamageType/PropertyL3/L6/L11 — the identical
// validation path the Core-sheet's own AJAX routes already use.
func (s *server) pipePopupSetHandler(setFn func(id int64, value string) (int, string)) http.HandlerFunc {
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
		value := strings.TrimSpace(r.FormValue("value"))
		if status, msg := setFn(id, value); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		http.Redirect(w, r, pipePopupPath(id), http.StatusSeeOther)
	}
}
