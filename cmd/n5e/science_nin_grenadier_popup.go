package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
)

// science_nin_grenadier_popup.go: the "Grenadier" subclass tracker popup —
// B.I.M and B.I.M Specialist, pulled out of sheet_science_nin
// (character_sheet.html) into their own popup. See subclass_tracker_popup.
// go's header doc for the shared pattern this is built from.

func grenadierPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/grenadier"
}

func (s *server) handleGrenadierPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for grenadier popup:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for grenadier popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Grenadier", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.Grenadier == nil {
		popup.EmptyHint = "This character has no B.I.M yet — Explosive Modifications grants this at 3rd-level Grenadier."
		s.renderSubclassTrackerPopup(w, "character_science_nin_grenadier.html", popup, nil)
		return
	}

	gr := data.Grenadier
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, bimSection(id, gr))
	if gr.DesignatedBIM != nil || len(gr.AvailableDesignatedBIM) > 0 {
		popup.Sections = append(popup.Sections, bimSpecialistSection(id, gr))
	}
	s.renderSubclassTrackerPopup(w, "character_science_nin_grenadier.html", popup, nil)
}

func bimSection(characterID int64, gr *scienceNinGrenadierData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "B.I.M (" + strconv.Itoa(gr.BIMUsed) + "/" + strconv.Itoa(gr.BIMCap) + " held)",
		Hint:         "Cost badges are informational — Creation Points spent installing/upgrading B.I.Ms aren't tracked separately from the bandolier's own held count.",
		DeleteAction: "/characters/" + idStr + "/science-nin/grenadier/bim/delete",
	}
	for _, k := range gr.KnownBIM {
		epithet := k.Tier
		if k.Quantity > 1 {
			epithet += " · ×" + strconv.Itoa(k.Quantity)
		}
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: epithet, DetailHref: "/science-nin-picks/bim/" + k.Slug})
	}
	if gr.BIMUsed < gr.BIMCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/grenadier/bim/add"
		sec.AddLabel = "Load Bandolier"
		for _, o := range gr.AvailableBIM {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

func bimSpecialistSection(characterID int64, gr *scienceNinGrenadierData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "B.I.M Specialist",
		Hint:         "One known B.I.M type designated — half Creation Point cost to upgrade, and creatable on the fly as a bonus action. Only the designation itself is tracked; the cost discount and bonus-action creation stay on paper.",
		DeleteAction: "/characters/" + idStr + "/science-nin/grenadier/bim-specialist/delete",
	}
	if gr.DesignatedBIM != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: gr.DesignatedBIM.Slug, Name: gr.DesignatedBIM.Name, DetailHref: "/science-nin-picks/bim/" + gr.DesignatedBIM.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/grenadier/bim-specialist/add"
		sec.AddLabel = "Set B.I.M Specialist"
		for _, o := range gr.AvailableDesignatedBIM {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}
