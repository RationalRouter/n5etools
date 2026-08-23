package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// science_nin_elemental_innovationist_popup.go: the "Elemental
// Innovationist" subclass tracker popup — Exoskeleton, E.I.Ps, W.O.W,
// Ascended W.o.W, and Perma Perk, pulled out of sheet_science_nin
// (character_sheet.html) into their own popup. See subclass_tracker_popup.
// go's header doc for the shared pattern this is built from.

func elementalInnovationistPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/elemental-innovationist"
}

func (s *server) handleElementalInnovationistPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for elemental innovationist popup:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for elemental innovationist popup:", err)
		return
	}

	popup := subclassTrackerPopupData{
		Title:         "Elemental Innovationist",
		CharacterID:   id,
		CharacterName: sheet.Name,
	}
	if data == nil || data.ElementalInnovationist == nil {
		popup.EmptyHint = "This character has no Elemental Innovationist picks yet — E.I.Ps grants this at 3rd-level Elemental Innovationist."
		s.renderSubclassTrackerPopup(w, "character_science_nin_elemental_innovationist.html", popup, nil)
		return
	}

	ei := data.ElementalInnovationist
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, eipSection(id, ei))
	if ei.WOWCap > 0 {
		popup.Sections = append(popup.Sections, wowSection(id, ei))
		if ei.DesignatedWoW != nil || len(ei.AvailableDesignatedWoW) > 0 {
			popup.Sections = append(popup.Sections, ascendedWowSection(id, ei))
		}
	}
	if ei.PermaPerk != nil || len(ei.AvailablePermaPerk) > 0 {
		popup.Sections = append(popup.Sections, permaPerkSection(id, ei))
	}
	s.renderSubclassTrackerPopup(w, "character_science_nin_elemental_innovationist.html", popup, map[string]any{
		"Exoskeleton": data.Exoskeleton,
	})
}

func eipSection(characterID int64, ei *scienceNinElementalInnovationistData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "E.I.Ps (" + strconv.Itoa(ei.EIPUsed) + "/" + strconv.Itoa(ei.EIPCap) + " known)",
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/eip/delete",
	}
	for _, k := range ei.KnownEIPs {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/eip/" + k.Slug})
	}
	if ei.EIPUsed < ei.EIPCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/eip/add"
		sec.AddLabel = "Fit Perk"
		for _, o := range ei.AvailableEIPs {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

func wowSection(characterID int64, ei *scienceNinElementalInnovationistData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "W.O.W (" + strconv.Itoa(ei.WOWUsed) + "/" + strconv.Itoa(ei.WOWCap) + " known)",
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/wow/delete",
	}
	for _, k := range ei.KnownWOW {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/science-nin-picks/wow/" + k.Slug})
	}
	if ei.WOWUsed < ei.WOWCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/wow/add"
		sec.AddLabel = "Forge Weapon"
		for _, o := range ei.AvailableWOW {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func ascendedWowSection(characterID int64, ei *scienceNinElementalInnovationistData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Elemental Innovation: Ascended W.o.W",
		Hint:         "One known W.o.W designated Ascended — half ammo Creation Point cost, and its damage ignores resistance and treats immunity as resistance. Only the designation itself is tracked; the cost discount and damage-type override stay on paper.",
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/ascended-wow/delete",
	}
	if ei.DesignatedWoW != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: ei.DesignatedWoW.Slug, Name: ei.DesignatedWoW.Name, DetailHref: "/science-nin-picks/wow/" + ei.DesignatedWoW.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/ascended-wow/add"
		sec.AddLabel = "Set Ascended W.o.W"
		for _, o := range ei.AvailableDesignatedWoW {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func permaPerkSection(characterID int64, ei *scienceNinElementalInnovationistData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Perma Perk",
		Hint:         "One known E.I.P (never Wonder E.I.P) made permanent — usable without CCD chakra or an Exoskeleton.",
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/perma-perk/delete",
	}
	if ei.PermaPerk != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: ei.PermaPerk.Slug, Name: ei.PermaPerk.Name, DetailHref: "/science-nin-picks/eip/" + ei.PermaPerk.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/perma-perk/add"
		sec.AddLabel = "Set Perma Perk"
		for _, o := range ei.AvailablePermaPerk {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}

// handleExoskeletonPopupToggle is handleSheetExoskeletonToggle's own
// redirect-based equivalent for the Elemental Innovationist popup — a bare
// boolean set with no real validation to extract into a shared core.
func (s *server) handleExoskeletonPopupToggle(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetExoskeletonDonned(s.charDB, id, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set exoskeleton donned (popup):", err)
		return
	}
	http.Redirect(w, r, elementalInnovationistPopupPath(id), http.StatusSeeOther)
}
