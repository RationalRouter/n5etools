package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
)

// science_nin_storm_rider_popup.go: the "Storm Rider" subclass tracker
// popup — A.T. Enhancements and the King's Road Regalia overlay, pulled
// out of sheet_science_nin (character_sheet.html) into their own popup.
// See subclass_tracker_popup.go's header doc for the shared pattern this
// is built from. The Air Trecks weapon itself isn't tracked here — it's
// granted straight into inventory (ensureAirTrecksGranted, science_nin.go)
// and shows up in the main Attacks & Jutsu table like any other equipped
// weapon, unchanged by this conversion.

func stormRiderPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/storm-rider"
}

func (s *server) handleStormRiderPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for storm rider popup:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for storm rider popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Storm Rider", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.StormRider == nil {
		popup.EmptyHint = "This character has no A.T. Enhancements yet — Air Trecks grants this at 3rd-level Storm Rider."
		s.renderSubclassTrackerPopup(w, "character_science_nin_storm_rider.html", popup, nil)
		return
	}

	sr := data.StormRider
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, atEnhancementSection(id, sr))
	if sr.RegaliaCap > 0 {
		popup.Sections = append(popup.Sections, regaliaSection(id, sr))
	}
	s.renderSubclassTrackerPopup(w, "character_science_nin_storm_rider.html", popup, nil)
}

func atEnhancementSection(characterID int64, sr *scienceNinStormRiderData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "A.T. Enhancements (" + strconv.Itoa(sr.EnhancementUsed) + "/" + strconv.Itoa(sr.EnhancementCap) + " slots)",
		Hint:         "Cost badges are informational — Creation Points spent installing an enhancement aren't tracked separately from enhancement slots.",
		DeleteAction: "/characters/" + idStr + "/science-nin/storm-rider/enhancement/delete",
	}
	for _, k := range sr.KnownEnhancements {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/air-treck-enhancement/" + k.Slug})
	}
	if sr.EnhancementUsed < sr.EnhancementCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/storm-rider/enhancement/add"
		sec.AddLabel = "Install Enhancement"
		for _, o := range sr.AvailableEnhancements {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

func regaliaSection(characterID int64, sr *scienceNinStormRiderData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	hint := "One permanent Regalia, worn alongside your A. Ts."
	if sr.RegaliaCap > 1 {
		hint = "Two Regalia can be worn at once, granted by Future of Shinobi: Sky Keeper."
	}
	sec := subclassTrackerSection{
		Title:        "Regalia (" + strconv.Itoa(sr.RegaliaUsed) + "/" + strconv.Itoa(sr.RegaliaCap) + " worn)",
		Hint:         hint,
		DeleteAction: "/characters/" + idStr + "/science-nin/storm-rider/regalia/delete",
	}
	for _, k := range sr.Regalia {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/science-nin-picks/regalia/" + k.Slug})
	}
	if sr.RegaliaUsed < sr.RegaliaCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/storm-rider/regalia/add"
		sec.AddLabel = "Choose Regalia"
		for _, o := range sr.AvailableRegalia {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}
