package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/i18n"
)

// science_nin_ninjaneer_popup.go: the "Ninjaneer" subclass tracker popup —
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
