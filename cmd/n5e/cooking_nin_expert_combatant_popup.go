package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// cooking_nin_expert_combatant_popup.go: the "Expert Combatant (Battle
// Cook)" subclass tracker popup — Battle Cook's own 2nd-level
// weapon-classification pick, pulled out of sheet_cooking_nin
// (character_sheet.html) into its own popup. See subclass_tracker_popup.go's
// header doc for the shared pattern this is built from. Distinct from
// Combat Medic's own unrelated "Expert Combatant" feature (Medical-Nin,
// medical_nin_combat_medic_popup.go's handleExpertCombatantPopupAdd/Delete)
// — the two classes' features simply share a printed name, nothing else —
// so every identifier here says "BattleCook" explicitly rather than reusing
// the bare "ExpertCombatant" names Combat Medic's own popup already claims.
//
// A single lock-once-chosen <select>, not a catalog Known/Available list,
// so this is hand-rolled rather than routed through the shared
// subclass_tracker_section partial — same shape Combat Medic's own Fighting
// Stance <select> already establishes.

// battleCookExpertCombatantPopupPath is the Expert Combatant (Battle Cook)
// popup's own URL.
func battleCookExpertCombatantPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/expert-combatant"
}

// handleBattleCookExpertCombatantPopup serves the "Expert Combatant (Battle
// Cook)" popup. Renders with an explanatory hint for a character with no
// Expert Combatant yet (below 2nd-level Battle Cook, or a different Cooking
// Focus entirely) — reachable only by visiting this URL directly after a
// respec, since the sidebar button that links here is itself gated on the
// same .CookingNin.ExpertCombatant check.
func (s *server) handleBattleCookExpertCombatantPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for expert combatant (battle cook) popup:", err)
		return
	}
	data, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for expert combatant (battle cook) popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Expert Combatant (Battle Cook)", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-cooking-nin"}
	if data == nil || data.ExpertCombatant == nil {
		popup.EmptyHint = "This character has no Expert Combatant yet — Battle Cook grants this at 2nd level."
		s.renderSubclassTrackerPopup(w, "character_cooking_nin_expert_combatant.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_cooking_nin_expert_combatant.html", popup, map[string]any{
		"ExpertCombatant": data.ExpertCombatant,
	})
}

// handleBattleCookExpertCombatantPopupSet is
// setBattleCookExpertCombatantWeapon's own redirect-based wrapper for the
// Expert Combatant (Battle Cook) popup (cooking_nin.go).
func (s *server) handleBattleCookExpertCombatantPopupSet(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setBattleCookExpertCombatantWeapon(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, battleCookExpertCombatantPopupPath(id), http.StatusSeeOther)
}
