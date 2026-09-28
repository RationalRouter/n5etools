package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/charsheet"
	"github.com/RationalRouter/n5e/internal/charstore"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// science_nin_subclass_popups.go: every Science-Nin subclass tracker popup.
// Each section below is one subclass's popup, all built from the shared
// pattern documented in subclass_tracker_popup.go. Their tests live in
// science_nin_more_subclass_popups_test.go and
// science_nin_last_subclass_popups_test.go.

// The "Grenadier" subclass tracker popup —
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

// The "Mad Scientist" subclass tracker
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

// The "Spyware" subclass tracker popup —
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
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for spyware popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.spyware"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.Spyware == nil {
		popup.EmptyHint = tr("subclass_tracker.spyware.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_spyware.html", popup, nil)
		return
	}

	sp := data.Spyware
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, spywareProgramSection(id, sp, data.CreationPointsUsed, data.CreationPointsCap, tr))
	if sp.QuickHack != nil || len(sp.AvailableQuickHack) > 0 {
		popup.Sections = append(popup.Sections, quickHackSection(id, sp, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_spyware.html", popup, nil)
}

// spywareProgramSection builds the popup's own main Spyware Programs
// Known/Available section — Cruel Angel's Thesis' own Programs draw from
// the same shared Creation Points budget S.N.B Upgrades/E.I.Ps do, so
// creationPointsUsed/Cap gate each Available program's own Disabled flag
// the identical way snbUpgradesSection's own doc explains (snb_upgrades_
// popup.go), re-parsing each option's raw Cost via parseScienceNinToolStatLine
// rather than reading a dedicated field (scienceNinSubclassOption carries
// none — see that struct's own doc, science_nin_subclasses.go).
func spywareProgramSection(characterID int64, sp *scienceNinSpywareData, creationPointsUsed, creationPointsCap int, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.spyware.program.title", sp.ProgramUsed, sp.ProgramCap),
		DeleteAction: "/characters/" + idStr + "/science-nin/spyware/program/delete",
	}
	for _, k := range sp.KnownPrograms {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/spyware-program/" + k.Slug})
	}
	if sp.ProgramUsed < sp.ProgramCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/spyware/program/add"
		sec.AddLabel = tr("subclass_tracker.spyware.program.add_label")
		for _, o := range sp.AvailablePrograms {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			cost := o.Cost
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description,
				Disabled: creationPointsUsed+cost > creationPointsCap,
			})
		}
	}
	return sec
}

// quickHackSection has no slot count in its title — Netrunner grants
// exactly one Quick Hack designation, the same single-slot shape
// bimSpecialistSection/ascendedWowSection/permaPerkSection already use.
func quickHackSection(characterID int64, sp *scienceNinSpywareData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.spyware.quick_hack.title"),
		Hint:         tr("subclass_tracker.spyware.quick_hack.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/spyware/quick-hack/delete",
	}
	if sp.QuickHack != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: sp.QuickHack.Slug, Name: sp.QuickHack.Name, DetailHref: "/science-nin-picks/spyware-program/" + sp.QuickHack.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/spyware/quick-hack/add"
		sec.AddLabel = tr("subclass_tracker.spyware.quick_hack.add_label")
		for _, o := range sp.AvailableQuickHack {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}

// The "Storm Rider" subclass tracker
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

// The "Technobi" subclass tracker popup —
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
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for technobi popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.technobi"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.Technobi == nil {
		popup.EmptyHint = tr("subclass_tracker.technobi.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_technobi.html", popup, nil)
		return
	}

	tb := data.Technobi
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, technobiMechanizationSection(id, tb, tr))
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_technobi.html", popup, map[string]any{
		"Technobi": tb,
	})
}

func technobiMechanizationSection(characterID int64, tb *scienceNinTechnobiData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.technobi.mechanization.title", tb.MechanizationUsed, tb.MechanizationCap),
		DeleteAction: "/characters/" + idStr + "/science-nin/technobi/mechanization/delete",
	}
	for _, k := range tb.KnownMechanizations {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/technobi-mechanization/" + k.Slug})
	}
	if tb.MechanizationUsed < tb.MechanizationCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/technobi/mechanization/add"
		sec.AddLabel = tr("subclass_tracker.technobi.mechanization.add_label")
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

// The "Shinobi-Ware" subclass tracker
// popup — Shinobi-Ware Upgrades, Full-Metal Shinobi, Glorious Evolution, In
// His Image, and Ever Evolving, pulled out of sheet_science_nin
// (character_sheet.html) into their own popup. See subclass_tracker_popup.
// go's header doc for the shared pattern this is built from.

func shinobiWarePopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/shinobi-ware"
}

func (s *server) handleShinobiWarePopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for shinobi-ware popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for shinobi-ware popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.shinobi_ware"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.ShinobiWare == nil {
		popup.EmptyHint = tr("subclass_tracker.shinobi_ware.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_shinobi_ware.html", popup, nil)
		return
	}

	sw := data.ShinobiWare
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, shinobiWareUpgradeSection(id, sw, tr))
	if sw.EvolvedCap > 0 {
		popup.Sections = append(popup.Sections, gloriousEvolutionSection(id, sw, tr))
	}
	if sw.ShinjutsuUpgrade != nil || len(sw.AvailableShinjutsuUpgrade) > 0 {
		popup.Sections = append(popup.Sections, inHisImageSection(id, sw, tr))
	}
	if sw.EverEvolvingSeal != nil || len(sw.AvailableEverEvolvingSeal) > 0 {
		popup.Sections = append(popup.Sections, everEvolvingSection(id, sw, data.CreationPointsUsed, data.CreationPointsCap, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_shinobi_ware.html", popup, map[string]any{
		"ShinobiWare": sw,
	})
}

func shinobiWareUpgradeSection(characterID int64, sw *scienceNinShinobiWareData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.shinobi_ware.upgrade.title", sw.UpgradeUsed, sw.UpgradeCap),
		Hint:         tr("subclass_tracker.shinobi_ware.upgrade.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/shinobi-ware/upgrade/delete",
	}
	for _, k := range sw.KnownUpgrades {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/shinobi-ware-upgrade/" + k.Slug})
	}
	if sw.UpgradeUsed < sw.UpgradeCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/upgrade/add"
		sec.AddLabel = tr("subclass_tracker.shinobi_ware.upgrade.add_label")
		for _, o := range sw.AvailableUpgrades {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

func gloriousEvolutionSection(characterID int64, sw *scienceNinShinobiWareData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.shinobi_ware.glorious_evolution.title", sw.EvolvedUsed, sw.EvolvedCap),
		Hint:         tr("subclass_tracker.shinobi_ware.glorious_evolution.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/shinobi-ware/evolved/delete",
	}
	for _, k := range sw.KnownEvolved {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/evolved-upgrade/" + k.Slug})
	}
	if sw.EvolvedUsed < sw.EvolvedCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/evolved/add"
		sec.AddLabel = tr("subclass_tracker.shinobi_ware.glorious_evolution.add_label")
		for _, o := range sw.AvailableEvolved {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

// inHisImageSection has no DeleteAction — Shinjutsu Upgrade is permanent
// ("You can not change this later"), matching the Core sheet's own missing
// delete route for this category.
func inHisImageSection(characterID int64, sw *scienceNinShinobiWareData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title: tr("subclass_tracker.shinobi_ware.in_his_image.title"),
		Hint:  tr("subclass_tracker.shinobi_ware.in_his_image.hint"),
	}
	if sw.ShinjutsuUpgrade != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: sw.ShinjutsuUpgrade.Slug, Name: sw.ShinjutsuUpgrade.Name, DetailHref: "/science-nin-picks/shinobi-ware-upgrade/" + sw.ShinjutsuUpgrade.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/shinjutsu-upgrade/add"
		sec.AddLabel = tr("subclass_tracker.shinobi_ware.in_his_image.add_label")
		for _, o := range sw.AvailableShinjutsuUpgrade {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// everEvolvingSection builds the popup's own Ever Evolving Known/Available
// section — creationPointsUsed/Cap (the same shared budget every other
// Science-Nin CP-spending catalog draws from, threaded in from the caller's
// own *scienceNinToolsTabData rather than read off sw, which doesn't carry
// them) gate each Available seal's own Disabled flag below: a seal's Cost
// equals its own rank (everEvolvingSealCost, ever_evolving.go), so a
// character already close to their cap can find some ranks pickable and
// higher ones not, all inside this single one-slot section.
func everEvolvingSection(characterID int64, sw *scienceNinShinobiWareData, creationPointsUsed, creationPointsCap int, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.shinobi_ware.ever_evolving.title"),
		Hint:         tr("subclass_tracker.shinobi_ware.ever_evolving.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/shinobi-ware/ever-evolving-seal/delete",
	}
	if sw.EverEvolvingSeal != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug: sw.EverEvolvingSeal.Slug, Name: sw.EverEvolvingSeal.Name,
			Epithet: "Rank " + sw.EverEvolvingSeal.Rank + " · " + strconv.Itoa(sw.EverEvolvingSeal.Cost) + " CP",
		})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/ever-evolving-seal/add"
		sec.AddLabel = tr("subclass_tracker.shinobi_ware.ever_evolving.add_label")
		for _, o := range sw.AvailableEverEvolvingSeal {
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name, Epithet: "Rank " + o.Rank + " · " + strconv.Itoa(o.Cost) + " CP", Description: o.Description,
				Disabled: creationPointsUsed+o.Cost > creationPointsCap,
			})
		}
	}
	return sec
}

// handleFullMetalShinobiResistancePopupAdd is setFullMetalShinobiResistancePick's
// own redirect-based wrapper for the Shinobi-Ware popup.
func (s *server) handleFullMetalShinobiResistancePopupAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slotKey := strings.TrimSpace(r.FormValue("slot_key"))
	damageType := strings.TrimSpace(r.FormValue("damage_type"))
	if status, msg := s.setFullMetalShinobiResistancePick(id, slotKey, damageType); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, shinobiWarePopupPath(id), http.StatusSeeOther)
}

// handleEverEvolvingSealPopupAdd is addEverEvolvingSealPick's own
// redirect-based wrapper for the Shinobi-Ware popup.
func (s *server) handleEverEvolvingSealPopupAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addEverEvolvingSealPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, shinobiWarePopupPath(id), http.StatusSeeOther)
}

// handleEverEvolvingSealPopupDelete is a thin redirect-based wrapper around
// RemoveScienceNinSubclassPick, matching handleEverEvolvingSealDelete's own
// Core-sheet route — not subclassTrackerPopupDelete's generic factory, since
// this category's own form field is "option_slug" here (matching
// subclass_tracker_section's shared markup) rather than "seal_slug" (the
// Core sheet's own bespoke inline form field name for this one section).
func (s *server) handleEverEvolvingSealPopupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if slug == "" {
		http.Error(w, "missing seal", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickEverEvolvingSeal, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove ever evolving seal (popup):", err)
		return
	}
	http.Redirect(w, r, shinobiWarePopupPath(id), http.StatusSeeOther)
}

// The "Ninjaneer" subclass tracker popup —
// Arsenal Modifications, Enhanced Weapon, Warrior of Science's Legendary
// Weapon, Perfected Weapons, and A Weapon to Surpass's Perfected Weapon
// Mark, pulled out of sheet_science_nin (character_sheet.html) into their
// own popup. See subclass_tracker_popup.go's header doc for the shared
// pattern this is built from.

func ninjaneerPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/science-nin/ninjaneer"
}

func (s *server) handleNinjaneerPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for ninjaneer popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for ninjaneer popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.ninjaneer"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.Ninjaneer == nil {
		popup.EmptyHint = tr("subclass_tracker.ninjaneer.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_ninjaneer.html", popup, nil)
		return
	}

	nj := data.Ninjaneer
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, arsenalModSection(id, nj, tr))
	if nj.EnhancedWeapon != nil || len(nj.AvailableEnhancedWeapons) > 0 {
		popup.Sections = append(popup.Sections, enhancedWeaponSection(id, nj, tr))
	}
	if nj.WarriorOfScience != nil {
		popup.Sections = append(popup.Sections, legendaryWeaponSection(id, nj.WarriorOfScience, tr))
	}
	if nj.PerfectedWeaponCap > 0 {
		popup.Sections = append(popup.Sections, perfectedWeaponSection(id, nj, tr))
	}
	if nj.PerfectedWeaponMark != nil || len(nj.AvailablePerfectedWeaponMarks) > 0 {
		popup.Sections = append(popup.Sections, perfectedWeaponMarkSection(id, nj, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_ninjaneer.html", popup, map[string]any{
		"WarriorOfScience": nj.WarriorOfScience,
	})
}

func arsenalModSection(characterID int64, nj *scienceNinNinjaneerData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.ninjaneer.arsenal_mod.title", nj.ArsenalModUsed, nj.ArsenalModCap),
		Hint:         tr("subclass_tracker.ninjaneer.arsenal_mod.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/ninjaneer/arsenal-mod/delete",
	}
	for _, k := range nj.KnownArsenalMods {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/arsenal-mod/" + k.Slug})
	}
	if nj.ArsenalModUsed < nj.ArsenalModCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/ninjaneer/arsenal-mod/add"
		sec.AddLabel = tr("subclass_tracker.ninjaneer.arsenal_mod.add_label")
		for _, o := range nj.AvailableArsenalMods {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

func perfectedWeaponSection(characterID int64, nj *scienceNinNinjaneerData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.ninjaneer.perfected_weapon.title", nj.PerfectedWeaponUsed, nj.PerfectedWeaponCap),
		Hint:         tr("subclass_tracker.ninjaneer.perfected_weapon.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/ninjaneer/perfected-weapon/delete",
	}
	for _, k := range nj.KnownPerfectedWeapons {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/science-nin-picks/perfected-weapon/" + k.Slug})
	}
	if nj.PerfectedWeaponUsed < nj.PerfectedWeaponCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/ninjaneer/perfected-weapon/add"
		sec.AddLabel = tr("subclass_tracker.ninjaneer.perfected_weapon.add_label")
		for _, o := range nj.AvailablePerfectedWeapons {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// ninjaneerWeaponDesignationSection builds the shared shape Enhanced
// Weapon/Legendary Weapon/Perfected Weapon Mark all use: a single-slot
// "which owned inventory row" designation, InventoryID standing in for a
// catalog Slug.
func ninjaneerWeaponDesignationSection(title, hint, addAction, addLabel, deleteAction string, current *ninjaneerWeaponOption, available []ninjaneerWeaponOption) subclassTrackerSection {
	sec := subclassTrackerSection{Title: title, Hint: hint, DeleteAction: deleteAction}
	if current != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug: strconv.FormatInt(current.InventoryID, 10), Name: current.Name, DetailHref: current.ItemHref,
		})
	} else if len(available) > 0 {
		sec.AddAction = addAction
		sec.AddLabel = addLabel
		for _, o := range available {
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: strconv.FormatInt(o.InventoryID, 10), Name: o.Name, Description: o.Description,
			})
		}
	}
	return sec
}

// legendaryWeaponSection builds Warrior of Science's own Legendary Weapon
// designation — the toggle itself renders separately in the template (extra
// field WarriorOfScience), since it isn't part of the generic Known/
// Available shape.
func legendaryWeaponSection(characterID int64, wos *scienceNinWarriorOfScienceData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	return ninjaneerWeaponDesignationSection(
		tr("subclass_tracker.ninjaneer.legendary_weapon.title"),
		tr("subclass_tracker.ninjaneer.legendary_weapon.hint"),
		"/characters/"+idStr+"/science-nin/ninjaneer/legendary-weapon/add", tr("subclass_tracker.ninjaneer.legendary_weapon.add_label"),
		"/characters/"+idStr+"/science-nin/ninjaneer/legendary-weapon/delete",
		wos.LegendaryWeapon, wos.AvailableLegendaryWeapons)
}

func enhancedWeaponSection(characterID int64, nj *scienceNinNinjaneerData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	return ninjaneerWeaponDesignationSection(
		tr("subclass_tracker.ninjaneer.enhanced_weapon.title"),
		tr("subclass_tracker.ninjaneer.enhanced_weapon.hint"),
		"/characters/"+idStr+"/science-nin/ninjaneer/enhanced-weapon/add", tr("subclass_tracker.ninjaneer.enhanced_weapon.add_label"),
		"/characters/"+idStr+"/science-nin/ninjaneer/enhanced-weapon/delete",
		nj.EnhancedWeapon, nj.AvailableEnhancedWeapons)
}

func perfectedWeaponMarkSection(characterID int64, nj *scienceNinNinjaneerData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	return ninjaneerWeaponDesignationSection(
		tr("subclass_tracker.ninjaneer.perfected_weapon_mark.title"),
		tr("subclass_tracker.ninjaneer.perfected_weapon_mark.hint"),
		"/characters/"+idStr+"/science-nin/ninjaneer/perfected-weapon-mark/add", tr("subclass_tracker.ninjaneer.perfected_weapon_mark.add_label"),
		"/characters/"+idStr+"/science-nin/ninjaneer/perfected-weapon-mark/delete",
		nj.PerfectedWeaponMark, nj.AvailablePerfectedWeaponMarks)
}

// handleSheetNinjaneerLegendaryWeaponPopupToggle is
// handleSheetNinjaneerLegendaryWeaponToggle's own redirect-based equivalent
// for the Ninjaneer popup — a bare boolean set with no real validation to
// extract into a shared core.
func (s *server) handleSheetNinjaneerLegendaryWeaponPopupToggle(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetNinjaneerLegendaryWeaponActive(s.charDB, id, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set ninjaneer legendary weapon active (popup):", err)
		return
	}
	http.Redirect(w, r, ninjaneerPopupPath(id), http.StatusSeeOther)
}

// The "Elemental
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
	locale := localeFromContext(r.Context())
	data, err := s.loadScienceNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for elemental innovationist popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{
		Title:         tr("sheet.elemental_innovationist"),
		CharacterID:   id,
		CharacterName: sheet.Name,
		// sheet-ac: Exoskeleton's Donned/Doffed toggle changes AC (it swaps
		// the character's own armor-category AC formula), so the Core Sheet's
		// AC box needs the same refresh every other AC-affecting popup action
		// already gets — see the same "sheet-ac" name in the data-also-refresh
		// lists throughout character_sheet.html (inventory equip/unequip,
		// feat grant/remove).
		// sheet-squares/sheet-vitals/sheet-skills: individual E.I.P picks
		// (Speed Demon, Stamina, etc.) change Speed, Max HP, and Passive
		// Perception respectively — those live in these three blocks, not
		// sheet-science-nin, and were missing here entirely, which is why
		// picking/removing an E.I.P auto-calculated correctly server-side but
		// never displayed the new value without a manual page refresh.
		// sheet-companions/sheet-summon-tab: the Angel E.I.P auto-creates a
		// Spectre companion on pick and auto-deletes it on removal — both the
		// Core tab's condensed companion list and the full Companions tab
		// card need to reflect that immediately, same reason.
		RefreshOpenerBlocks: "sheet-science-nin sheet-weapon-attacks sheet-inventory sheet-inventory-full sheet-ac sheet-squares sheet-vitals sheet-skills sheet-companions sheet-summon-tab",
	}
	if data == nil || data.ElementalInnovationist == nil {
		popup.EmptyHint = tr("subclass_tracker.elemental_innovationist.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_science_nin_elemental_innovationist.html", popup, nil)
		return
	}

	ei := data.ElementalInnovationist
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, eipSection(id, ei, data.CreationPointsUsed, data.CreationPointsCap, tr))
	if ei.WOWCap > 0 {
		popup.Sections = append(popup.Sections, wowSection(id, ei, tr))
		if ei.DesignatedWoW != nil || len(ei.AvailableDesignatedWoW) > 0 {
			popup.Sections = append(popup.Sections, ascendedWowSection(id, ei, tr))
		}
	}
	if ei.PermaPerk != nil || len(ei.AvailablePermaPerk) > 0 {
		popup.Sections = append(popup.Sections, permaPerkSection(id, ei, tr))
	}
	s.renderSubclassTrackerPopup(w, r, "character_science_nin_elemental_innovationist.html", popup, map[string]any{
		"Exoskeleton": data.Exoskeleton,
	})
}

// eipSection builds the popup's own main E.I.Ps Known/Available section —
// draws from the same shared Creation Points budget S.N.B Upgrades/Spyware
// Programs do (this file's own header doc), so creationPointsUsed/Cap gate
// each Available perk's own Disabled flag the identical way
// snbUpgradesSection's own doc explains (snb_upgrades_popup.go), re-parsing
// each option's raw Cost via parseScienceNinToolStatLine rather than
// reading a dedicated field (scienceNinSubclassOption carries none — see
// that struct's own doc, science_nin_subclasses.go).
func eipSection(characterID int64, ei *scienceNinElementalInnovationistData, creationPointsUsed, creationPointsCap int, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.elemental_innovationist.eip.title", ei.EIPUsed, ei.EIPCap),
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/eip/delete",
	}
	for _, k := range ei.KnownEIPs {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/eip/" + k.Slug})
	}
	if ei.EIPUsed < ei.EIPCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/eip/add"
		sec.AddLabel = tr("subclass_tracker.elemental_innovationist.eip.add_label")
		for _, o := range ei.AvailableEIPs {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			cost := o.Cost
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description,
				Disabled: creationPointsUsed+cost > creationPointsCap,
			})
		}
	}
	return sec
}

func wowSection(characterID int64, ei *scienceNinElementalInnovationistData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.elemental_innovationist.wow.title", ei.WOWUsed, ei.WOWCap),
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/wow/delete",
	}
	for _, k := range ei.KnownWOW {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/science-nin-picks/wow/" + k.Slug})
	}
	if ei.WOWUsed < ei.WOWCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/wow/add"
		sec.AddLabel = tr("subclass_tracker.elemental_innovationist.wow.add_label")
		for _, o := range ei.AvailableWOW {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description, DescriptionHTML: formatWoWDescription(o.Description)})
		}
	}
	return sec
}

func ascendedWowSection(characterID int64, ei *scienceNinElementalInnovationistData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.elemental_innovationist.ascended_wow.title"),
		Hint:         tr("subclass_tracker.elemental_innovationist.ascended_wow.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/ascended-wow/delete",
	}
	if ei.DesignatedWoW != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: ei.DesignatedWoW.Slug, Name: ei.DesignatedWoW.Name, DetailHref: "/science-nin-picks/wow/" + ei.DesignatedWoW.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/ascended-wow/add"
		sec.AddLabel = tr("subclass_tracker.elemental_innovationist.ascended_wow.add_label")
		for _, o := range ei.AvailableDesignatedWoW {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description, DescriptionHTML: formatWoWDescription(o.Description)})
		}
	}
	return sec
}

func permaPerkSection(characterID int64, ei *scienceNinElementalInnovationistData, tr i18n.TranslateFunc) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        tr("subclass_tracker.elemental_innovationist.perma_perk.title"),
		Hint:         tr("subclass_tracker.elemental_innovationist.perma_perk.hint"),
		DeleteAction: "/characters/" + idStr + "/science-nin/elemental-innovationist/perma-perk/delete",
	}
	if ei.PermaPerk != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: ei.PermaPerk.Slug, Name: ei.PermaPerk.Name, DetailHref: "/science-nin-picks/eip/" + ei.PermaPerk.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/elemental-innovationist/perma-perk/add"
		sec.AddLabel = tr("subclass_tracker.elemental_innovationist.perma_perk.add_label")
		for _, o := range ei.AvailablePermaPerk {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}

// addWoWPick validates and stores a W.O.W pick exactly like
// scienceNinSubclassPickAddCore already does for every other flat-slot
// catalog, then additionally forges the weapon into the character's own
// equipped Inventory (see grantWoWWeapon, wow_weapons.go) — the one thing a
// W.o.W pick needs beyond every other subclass tracker's plain "record the
// pick" add, since a W.o.W is a real weapon the player carries, not an
// abstract upgrade slot. Shared by both wow/add routes (Core-sheet AJAX and
// this popup), same "one core, two thin wrappers" shape addEIPPick already
// uses for its own dedicated add.
func (s *server) addWoWPick(id int64, rawSlug, locale string) (int, string) {
	status, msg := s.scienceNinSubclassPickAddCore(id, charstore.ScienceNinPickWOW,
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil {
				return 0
			}
			return d.ElementalInnovationist.WOWUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil {
				return 0
			}
			return d.ElementalInnovationist.WOWCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ElementalInnovationist == nil {
				return nil
			}
			return d.ElementalInnovationist.AvailableWOW
		},
		false, rawSlug, locale)
	if status != http.StatusOK {
		return status, msg
	}
	name, description, err := s.scienceNinOptionNameAndDescription(rawSlug, locale)
	if err != nil {
		log.Println("look up wow option name:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := s.grantWoWWeapon(id, rawSlug, name, description, locale); err != nil {
		log.Println("grant wow weapon:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleScienceNinWOWAdd is addWoWPick's own Core-sheet AJAX wrapper,
// matching handleScienceNinEIPAdd's shape (science_nin_subclasses.go).
func (s *server) handleScienceNinWOWAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addWoWPick(id, slug, localeFromContext(r.Context())); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleWoWPopupAdd is addWoWPick's own redirect-based wrapper for the
// Elemental Innovationist popup, matching handleEIPPopupAdd's shape.
func (s *server) handleWoWPopupAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addWoWPick(id, slug, localeFromContext(r.Context())); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, elementalInnovationistPopupPath(id), http.StatusSeeOther)
}

// removeWoWPick clears one W.o.W pick and the equipped weapon it granted
// (see revokeWoWWeapon, wow_weapons.go) — freely, at any time, same "trust
// the player" boundary every other pick removal in this codebase draws. A
// forgotten W.o.W can no longer be the Ascended designee either, so a
// matching Ascended pick is cleared alongside it rather than left dangling.
func (s *server) removeWoWPick(id int64, slug, locale string) error {
	name, err := s.scienceNinOptionName(slug)
	if err != nil {
		return err
	}
	if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickWOW, slug); err != nil {
		return err
	}
	if name != "" {
		if err := s.revokeWoWWeapon(id, name); err != nil {
			return err
		}
	}
	ascended, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickAscendedWoW)
	if err != nil {
		return err
	}
	for _, p := range ascended {
		if p.OptionSlug == slug {
			if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickAscendedWoW, slug); err != nil {
				return err
			}
		}
	}
	if slug == scienceNinDraconicGauntletWoWSlug {
		logWhelpSyncErr(s.syncDraconicGauntletWhelp(id, locale))
	}
	return nil
}

// handleScienceNinWOWDelete is removeWoWPick's own Core-sheet AJAX wrapper.
func (s *server) handleScienceNinWOWDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if slug == "" {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	if err := s.removeWoWPick(id, slug, localeFromContext(r.Context())); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove wow pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleWoWPopupDelete is removeWoWPick's own redirect-based wrapper for the
// Elemental Innovationist popup — not built on subclassTrackerPopupDelete's
// shared factory, since a W.o.W's own delete also needs to revoke the
// granted weapon (and any dangling Ascended designation), unlike every
// other category that factory covers (see subclassTrackerPopupDelete's own
// doc, and ninjaneerWeaponDesignationPopupDelete for the identical shape of
// exception).
func (s *server) handleWoWPopupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if slug == "" {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	if err := s.removeWoWPick(id, slug, localeFromContext(r.Context())); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove wow pick:", err)
		return
	}
	http.Redirect(w, r, elementalInnovationistPopupPath(id), http.StatusSeeOther)
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
