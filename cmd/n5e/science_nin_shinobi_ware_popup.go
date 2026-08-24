package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
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
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for shinobi-ware popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Shinobi-Ware", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-science-nin"}
	if data == nil || data.ShinobiWare == nil {
		popup.EmptyHint = "This character has no Shinobi-Ware Upgrades yet — Edge Runner grants this at 3rd-level Shinobi-Ware."
		s.renderSubclassTrackerPopup(w, "character_science_nin_shinobi_ware.html", popup, nil)
		return
	}

	sw := data.ShinobiWare
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, shinobiWareUpgradeSection(id, sw))
	if sw.EvolvedCap > 0 {
		popup.Sections = append(popup.Sections, gloriousEvolutionSection(id, sw))
	}
	if sw.ShinjutsuUpgrade != nil || len(sw.AvailableShinjutsuUpgrade) > 0 {
		popup.Sections = append(popup.Sections, inHisImageSection(id, sw))
	}
	if sw.EverEvolvingSeal != nil || len(sw.AvailableEverEvolvingSeal) > 0 {
		popup.Sections = append(popup.Sections, everEvolvingSection(id, sw))
	}
	s.renderSubclassTrackerPopup(w, "character_science_nin_shinobi_ware.html", popup, map[string]any{
		"ShinobiWare": sw,
	})
}

func shinobiWareUpgradeSection(characterID int64, sw *scienceNinShinobiWareData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Shinobi-Ware Upgrades (" + strconv.Itoa(sw.UpgradeUsed) + "/" + strconv.Itoa(sw.UpgradeCap) + " slots)",
		Hint:         "Cost badges are informational — Creation Points spent installing an upgrade aren't tracked separately from Upgrade Slots.",
		DeleteAction: "/characters/" + idStr + "/science-nin/shinobi-ware/upgrade/delete",
	}
	for _, k := range sw.KnownUpgrades {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/shinobi-ware-upgrade/" + k.Slug})
	}
	if sw.UpgradeUsed < sw.UpgradeCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/upgrade/add"
		sec.AddLabel = "Install Upgrade"
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

func gloriousEvolutionSection(characterID int64, sw *scienceNinShinobiWareData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Glorious Evolution (" + strconv.Itoa(sw.EvolvedUsed) + "/" + strconv.Itoa(sw.EvolvedCap) + " evolved)",
		Hint:         "Slot-free upgrades — don't count against Upgrade Slots above. Higher tiers unlock at 9th and 14th level.",
		DeleteAction: "/characters/" + idStr + "/science-nin/shinobi-ware/evolved/delete",
	}
	for _, k := range sw.KnownEvolved {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/evolved-upgrade/" + k.Slug})
	}
	if sw.EvolvedUsed < sw.EvolvedCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/evolved/add"
		sec.AddLabel = "Evolve Upgrade"
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
func inHisImageSection(characterID int64, sw *scienceNinShinobiWareData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title: "In His Image",
		Hint:  "One Shinjutsu Upgrade, granted permanently the first time your seal activates — this pick cannot be changed once made. Activating and maintaining the seal, and its +2 AC/+1 crit-range effects while sealed, stay manual.",
	}
	if sw.ShinjutsuUpgrade != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: sw.ShinjutsuUpgrade.Slug, Name: sw.ShinjutsuUpgrade.Name, DetailHref: "/science-nin-picks/shinobi-ware-upgrade/" + sw.ShinjutsuUpgrade.Slug})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/shinjutsu-upgrade/add"
		sec.AddLabel = "Bind Shinjutsu"
		for _, o := range sw.AvailableShinjutsuUpgrade {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func everEvolvingSection(characterID int64, sw *scienceNinShinobiWareData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Ever Evolving",
		Hint:         "Instantly apply an Armor Seal to your Full Metal Shinobi armor for Creation Points equal to its rank. Changing seals (including removing it for no seal) is a Full Turn Action — this app doesn't gate the pick behind that action cost, only the shared Creation Points budget.",
		DeleteAction: "/characters/" + idStr + "/science-nin/shinobi-ware/ever-evolving-seal/delete",
	}
	if sw.EverEvolvingSeal != nil {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug: sw.EverEvolvingSeal.Slug, Name: sw.EverEvolvingSeal.Name,
			Epithet: "Rank " + sw.EverEvolvingSeal.Rank + " · " + strconv.Itoa(sw.EverEvolvingSeal.Cost) + " CP",
		})
	} else {
		sec.AddAction = "/characters/" + idStr + "/science-nin/shinobi-ware/ever-evolving-seal/add"
		sec.AddLabel = "Apply Seal"
		for _, o := range sw.AvailableEverEvolvingSeal {
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name, Epithet: "Rank " + o.Rank + " · " + strconv.Itoa(o.Cost) + " CP", Description: o.Description,
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
