package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/i18n"
)

// science_nin_shinobi_ware_popup.go: the "Shinobi-Ware" subclass tracker
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
