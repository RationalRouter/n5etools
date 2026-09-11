package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/i18n"
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
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for storm rider popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.storm_rider"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.StormRider == nil {
		popup.EmptyHint = tr("subclass_tracker.storm_rider.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_storm_rider.html", popup, nil)
		return
	}

	sr := data.StormRider
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, atEnhancementSection(id, sr, tr))
	if sr.RegaliaCap > 0 {
		popup.Sections = append(popup.Sections, regaliaSection(id, sr, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_storm_rider.html", popup, nil)
}

func atEnhancementSection(characterID int64, sr *scienceNinStormRiderData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.storm_rider.enhancement.title", sr.EnhancementUsed, sr.EnhancementCap),
		Hint:         tr("subclass_tracker.storm_rider.enhancement.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/storm-rider/enhancement/delete",
	}
	for _, k := range sr.KnownEnhancements {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/air-treck-enhancement/" + k.Slug})
	}
	if sr.EnhancementUsed < sr.EnhancementCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/storm-rider/enhancement/add"
		sec.AddLabel = tr("subclass_tracker.storm_rider.enhancement.add_label")
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

func regaliaSection(characterID int64, sr *scienceNinStormRiderData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	hint := tr("subclass_tracker.storm_rider.regalia.hint_single")
	if sr.RegaliaCap > 1 {
		hint = tr("subclass_tracker.storm_rider.regalia.hint_multiple")
	}
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.storm_rider.regalia.title", sr.RegaliaUsed, sr.RegaliaCap),
		Hint:         hint,
		DeleteAction: "/characters/" + idStr + "/science-nin/storm-rider/regalia/delete",
	}
	for _, k := range sr.Regalia {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/science-nin-picks/regalia/" + k.Slug})
	}
	if sr.RegaliaUsed < sr.RegaliaCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/storm-rider/regalia/add"
		sec.AddLabel = tr("subclass_tracker.storm_rider.regalia.add_label")
		for _, o := range sr.AvailableRegalia {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}
