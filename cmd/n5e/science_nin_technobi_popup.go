package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// science_nin_technobi_popup.go: the "Technobi" subclass tracker popup —
// Technobi Mechanizations and the S.E.N.Ts weapon pick, pulled out of
// sheet_science_nin (character_sheet.html) into their own popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from. S.E.N.T is a single freely re-editable <select>, not a
// catalog picker — it isn't part of subclassTrackerSection, so it's
// hand-rolled in character_science_nin_technobi.html rather than routed
// through the shared subclass_tracker_section partial (same shape as
// character_titan_slots.html's own Specialist Crafting <select>). The
// Core sheet's own inline form used data-also-refresh="sheet-weapon-
// attacks" to update the Attacks & Jutsu table live in place — this
// popup's plain POST-and-redirect form has no cross-window equivalent
// (see subclass_tracker_popup.go's header doc on why), so a S.E.N.T
// change made here only reaches the Core sheet's own Attacks & Jutsu
// table the next time that window is reloaded.

func technobiPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/technobi"
}

func (s *server) handleTechnobiPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for technobi popup:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for technobi popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Technobi", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.Technobi == nil {
		popup.EmptyHint = "This character has no Technobi Mechanizations yet — The Best Laid Trap grants this at 3rd-level Technobi."
		s.renderSubclassTrackerPopup(w, "character_science_nin_technobi.html", popup, nil)
		return
	}

	tb := data.Technobi
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, technobiMechanizationSection(id, tb))
	s.renderSubclassTrackerPopup(w, "character_science_nin_technobi.html", popup, map[string]any{
		"Technobi": tb,
	})
}

func technobiMechanizationSection(characterID int64, tb *scienceNinTechnobiData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Technobi Mechanizations (" + strconv.Itoa(tb.MechanizationUsed) + "/" + strconv.Itoa(tb.MechanizationCap) + " held)",
		DeleteAction: "/characters/" + idStr + "/science-nin/technobi/mechanization/delete",
	}
	for _, k := range tb.KnownMechanizations {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/technobi-mechanization/" + k.Slug})
	}
	if tb.MechanizationUsed < tb.MechanizationCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/technobi/mechanization/add"
		sec.AddLabel = "Install Mechanization"
		for _, o := range tb.AvailableMechanizations {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

// handleSENTPopupPick is handleSENTPick's own redirect-based equivalent for
// the Technobi popup — reuses setSENTPick's own validation (sents.go).
func (s *server) handleSENTPopupPick(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setSENTPick(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, technobiPopupPath(id), http.StatusSeeOther)
}
