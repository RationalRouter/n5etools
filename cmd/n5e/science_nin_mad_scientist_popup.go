package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/i18n"
)

// science_nin_mad_scientist_popup.go: the "Mad Scientist" subclass tracker
// popup — Inversion Serums and The Sheep and the Shepherd, pulled out of
// sheet_science_nin (character_sheet.html) into their own popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from.

func madScientistPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/mad-scientist"
}

func (s *server) handleMadScientistPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for mad scientist popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for mad scientist popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.mad_scientist"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.MadScientist == nil {
		popup.EmptyHint = tr("subclass_tracker.mad_scientist.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_mad_scientist.html", popup, nil)
		return
	}

	ms := data.MadScientist
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, inversionSerumSection(id, ms, tr))
	if ms.DesignatedSerum != nil || len(ms.AvailableDesignatedSerum) > 0 {
		popup.Sections = append(popup.Sections, sheepAndShepherdSection(id, ms, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_mad_scientist.html", popup, nil)
}

func inversionSerumSection(characterID int64, ms *scienceNinMadScientistData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.mad_scientist.inversion_serum.title", ms.SerumUsed, ms.SerumCap),
		Hint:         tr("subclass_tracker.mad_scientist.inversion_serum.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/mad-scientist/inversion-serum/delete",
	}
	for _, k := range ms.KnownSerums {
		pool := "Maiming"
		if k.Pool == "mending" {
			pool = "Mending"
		}
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier + " · " + pool, DetailHref: "/science-nin-picks/inversion-serum/" + k.Slug})
	}
	if ms.SerumUsed < ms.SerumCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/mad-scientist/inversion-serum/add"
		sec.AddLabel = tr("subclass_tracker.mad_scientist.inversion_serum.add_label")
		for _, o := range ms.AvailableSerums {
			epithet := o.Epithet
			if o.FixedPool != "" {
				pool := "Maiming"
				if o.FixedPool == "mending" {
					pool = "Mending"
				}
				e := o.Tier + " · " + pool
				if epithet != "" {
					e += " · " + epithet
				}
				sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug + "|" + o.FixedPool, Name: o.Name, Epithet: e, Description: o.Description})
				continue
			}
			mendE := o.Tier + " · Mending"
			maimE := o.Tier + " · Maiming"
			if epithet != "" {
				mendE += " · " + epithet
				maimE += " · " + epithet
			}
			sec.Available = append(sec.Available,
				subclassTrackerOption{Slug: o.Slug + "|mending", Name: o.Name, Epithet: mendE, Description: o.Description},
				subclassTrackerOption{Slug: o.Slug + "|maiming", Name: o.Name, Epithet: maimE, Description: o.Description})
		}
	}
	return sec
}

func sheepAndShepherdSection(characterID int64, ms *scienceNinMadScientistData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.mad_scientist.sheep_and_shepherd.title"),
		Hint:         tr("subclass_tracker.mad_scientist.sheep_and_shepherd.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/mad-scientist/sheep-and-shepherd/delete",
	}
	if ms.DesignatedSerum != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: ms.DesignatedSerum.Slug, Name: ms.DesignatedSerum.Name, DetailHref: "/science-nin-picks/inversion-serum/" + ms.DesignatedSerum.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/mad-scientist/sheep-and-shepherd/add"
		sec.AddLabel = tr("subclass_tracker.mad_scientist.sheep_and_shepherd.add_label")
		for _, o := range ms.AvailableDesignatedSerum {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}
