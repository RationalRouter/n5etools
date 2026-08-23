package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// medical_nin_combat_medic_popup.go: the "Combat Medic" subclass tracker
// popup — Combat Medic's own Martial Competency Fighting Stance pick (2nd
// level) and Expert Combatant pick (9th/13th/17th level), pulled out of
// sheet_medical_nin (character_sheet.html) into one popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from; one popup covering two independently level-gated sub-panels
// under a single subclass-chosen gate is the same shape Stancer
// (taijutsu_stancer_popup.go) already establishes for Mixed Martial
// Arts/Stance Blending.
//
// Fighting Stance is a single freely re-editable Taijutsu Stance <select>,
// not a catalog picker — same shape character_taijutsu_stancer.html's own
// selects use, hand-rolled rather than routed through subclass_tracker_
// section. Expert Combatant sources its Available list from the character's
// own known Taijutsu/Bukijutsu rather than a rules-database catalog, so its
// picker form fields are "jutsu_id", also hand-rolled — same reasoning
// character_ninjutsu_specialist_awakened_scroll.html's own section uses.

// combatMedicPopupPath is the Combat Medic popup's own URL.
func combatMedicPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/medical-nin/combat-medic"
}

// handleCombatMedicPopup serves the "Combat Medic" popup. Renders with an
// explanatory hint for a character whose Tenet of Medicine isn't Combat
// Medic — reachable only by visiting this URL directly after a respec,
// since the sidebar button that links here is itself gated on the same
// .MedicalNin.CombatMedic check.
func (s *server) handleCombatMedicPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for combat medic popup:", err)
		return
	}
	data, err := s.loadMedicalNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load medical-nin for combat medic popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Combat Medic", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || !data.CombatMedic {
		popup.EmptyHint = "This character isn't a Combat Medic — Martial Competency and Expert Combatant are Combat Medic's own Tenet of Medicine features."
		s.renderSubclassTrackerPopup(w, "character_medical_nin_combat_medic.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, "character_medical_nin_combat_medic.html", popup, map[string]any{
		"MedicalNin": data,
	})
}

// handleMedicalNinFightingStancePopup is setMedicalNinFightingStance's own
// redirect-based wrapper for the Combat Medic popup (medical_nin.go).
func (s *server) handleMedicalNinFightingStancePopup(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setMedicalNinFightingStance(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, combatMedicPopupPath(id), http.StatusSeeOther)
}

// handleExpertCombatantPopupAdd is addExpertCombatantPick's own
// redirect-based wrapper for the Combat Medic popup (medical_nin.go).
func (s *server) handleExpertCombatantPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addExpertCombatantPick(id, jutsuID); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, combatMedicPopupPath(id), http.StatusSeeOther)
}

// handleExpertCombatantPopupDelete is a thin redirect-based wrapper around
// RemoveMedicalNinExpertCombatantPick — removing a pick never needs a
// budget/cap check, so this stays a thin wrapper rather than sharing a
// "core" method with the Core-sheet's own handleExpertCombatantDelete.
func (s *server) handleExpertCombatantPopupDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.RemoveMedicalNinExpertCombatantPick(s.charDB, id, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove expert combatant pick (popup):", err)
		return
	}
	http.Redirect(w, r, combatMedicPopupPath(id), http.StatusSeeOther)
}
