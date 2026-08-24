package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
)

// science_nin_spyware_popup.go: the "Spyware" subclass tracker popup —
// Spyware Programs and the Netrunner Quick Hack overlay, pulled out of
// sheet_science_nin (character_sheet.html) into their own popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from.

func spywarePopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/spyware"
}

func (s *server) handleSpywarePopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for spyware popup:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for spyware popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Spyware", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.Spyware == nil {
		popup.EmptyHint = "This character has no Spyware Programs yet — Cruel Angel's Thesis grants this at 3rd-level Spyware."
		s.renderSubclassTrackerPopup(w, "character_science_nin_spyware.html", popup, nil)
		return
	}

	sp := data.Spyware
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, spywareProgramSection(id, sp))
	if sp.QuickHack != nil || len(sp.AvailableQuickHack) > 0 {
		popup.Sections = append(popup.Sections, quickHackSection(id, sp))
	}
	s.renderSubclassTrackerPopup(w, "character_science_nin_spyware.html", popup, nil)
}

func spywareProgramSection(characterID int64, sp *scienceNinSpywareData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Spyware Programs (" + strconv.Itoa(sp.ProgramUsed) + "/" + strconv.Itoa(sp.ProgramCap) + " held)",
		DeleteAction: "/characters/" + idStr + "/science-nin/spyware/program/delete",
	}
	for _, k := range sp.KnownPrograms {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/spyware-program/" + k.Slug})
	}
	if sp.ProgramUsed < sp.ProgramCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/spyware/program/add"
		sec.AddLabel = "Install Program"
		for _, o := range sp.AvailablePrograms {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

// quickHackSection has no slot count in its title — Netrunner grants
// exactly one Quick Hack designation, the same single-slot shape
// bimSpecialistSection/ascendedWowSection/permaPerkSection already use.
func quickHackSection(characterID int64, sp *scienceNinSpywareData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Quick Hack",
		Hint:         "One known Program made always-prepared — no cast needed to ready it.",
		DeleteAction: "/characters/" + idStr + "/science-nin/spyware/quick-hack/delete",
	}
	if sp.QuickHack != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: sp.QuickHack.Slug, Name: sp.QuickHack.Name, DetailHref: "/science-nin-picks/spyware-program/" + sp.QuickHack.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/spyware/quick-hack/add"
		sec.AddLabel = "Set Quick Hack"
		for _, o := range sp.AvailableQuickHack {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}
