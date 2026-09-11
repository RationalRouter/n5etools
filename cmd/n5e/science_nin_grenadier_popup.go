package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/i18n"
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
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for grenadier popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.grenadier"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.Grenadier == nil {
		popup.EmptyHint = tr("subclass_tracker.grenadier.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_grenadier.html", popup, nil)
		return
	}

	gr := data.Grenadier
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, bimSection(id, gr, tr))
	if gr.DesignatedBIM != nil || len(gr.AvailableDesignatedBIM) > 0 {
		popup.Sections = append(popup.Sections, bimSpecialistSection(id, gr, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_grenadier.html", popup, nil)
}

func bimSection(characterID int64, gr *scienceNinGrenadierData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.grenadier.bim.title", gr.BIMUsed, gr.BIMCap),
		Hint:         tr("subclass_tracker.grenadier.bim.hint"),
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
		sec.AddLabel = tr("subclass_tracker.grenadier.bim.add_label")
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

func bimSpecialistSection(characterID int64, gr *scienceNinGrenadierData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.grenadier.bim_specialist.title"),
		Hint:         tr("subclass_tracker.grenadier.bim_specialist.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/grenadier/bim-specialist/delete",
	}
	if gr.DesignatedBIM != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: gr.DesignatedBIM.Slug, Name: gr.DesignatedBIM.Name, DetailHref: "/science-nin-picks/bim/" + gr.DesignatedBIM.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/grenadier/bim-specialist/add"
		sec.AddLabel = tr("subclass_tracker.grenadier.bim_specialist.add_label")
		for _, o := range gr.AvailableDesignatedBIM {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}
