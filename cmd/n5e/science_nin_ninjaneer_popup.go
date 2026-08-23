package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
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
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for ninjaneer popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Ninjaneer", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.Ninjaneer == nil {
		popup.EmptyHint = "This character has no Arsenal Modifications yet — Arsenal Modifications grants this at 3rd-level Ninjaneer."
		s.renderSubclassTrackerPopup(w, "character_science_nin_ninjaneer.html", popup, nil)
		return
	}

	nj := data.Ninjaneer
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, arsenalModSection(id, nj))
	if nj.EnhancedWeapon != nil || len(nj.AvailableEnhancedWeapons) > 0 {
		popup.Sections = append(popup.Sections, enhancedWeaponSection(id, nj))
	}
	if nj.WarriorOfScience != nil {
		popup.Sections = append(popup.Sections, legendaryWeaponSection(id, nj.WarriorOfScience))
	}
	if nj.PerfectedWeaponCap > 0 {
		popup.Sections = append(popup.Sections, perfectedWeaponSection(id, nj))
	}
	if nj.PerfectedWeaponMark != nil || len(nj.AvailablePerfectedWeaponMarks) > 0 {
		popup.Sections = append(popup.Sections, perfectedWeaponMarkSection(id, nj))
	}
	s.renderSubclassTrackerPopup(w, "character_science_nin_ninjaneer.html", popup, map[string]any{
		"WarriorOfScience": nj.WarriorOfScience,
	})
}

func arsenalModSection(characterID int64, nj *scienceNinNinjaneerData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Arsenal Modifications (" + strconv.Itoa(nj.ArsenalModUsed) + "/" + strconv.Itoa(nj.ArsenalModCap) + " slots)",
		Hint:         "Cost badges are informational — Creation Points spent installing modifications aren't tracked separately from Upgrade Slots.",
		DeleteAction: "/characters/" + idStr + "/science-nin/ninjaneer/arsenal-mod/delete",
	}
	for _, k := range nj.KnownArsenalMods {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, Epithet: k.Tier, DetailHref: "/science-nin-picks/arsenal-mod/" + k.Slug})
	}
	if nj.ArsenalModUsed < nj.ArsenalModCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/ninjaneer/arsenal-mod/add"
		sec.AddLabel = "Install Modification"
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

func perfectedWeaponSection(characterID int64, nj *scienceNinNinjaneerData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Perfected Weapons (" + strconv.Itoa(nj.PerfectedWeaponUsed) + "/" + strconv.Itoa(nj.PerfectedWeaponCap) + " held)",
		Hint:         "Restricted to Minor-tier Arsenal Modifications, granted free of Creation Points — one per Perfected Weapon.",
		DeleteAction: "/characters/" + idStr + "/science-nin/ninjaneer/perfected-weapon/delete",
	}
	for _, k := range nj.KnownPerfectedWeapons {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/science-nin-picks/perfected-weapon/" + k.Slug})
	}
	if nj.PerfectedWeaponUsed < nj.PerfectedWeaponCap {
		sec.AddAction = "/characters/" + idStr + "/science-nin/ninjaneer/perfected-weapon/add"
		sec.AddLabel = "Perfect Weapon"
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
func legendaryWeaponSection(characterID int64, wos *scienceNinWarriorOfScienceData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	return ninjaneerWeaponDesignationSection(
		"Warrior of Science: Legendary Weapon",
		"20 CCD chakra (Action) to activate, 10 CCD chakra upkeep at the start of each turn — spend both manually. While active, a held Legendary Weapon gains +2 to Weapon/Taijutsu attack rolls, ignores THP and damage reduction once per turn, doubles the benefit of one non-Heavy/Two-Handed Weapon Property, and can convert a Bukijutsu's damage to force at double CCD cost — all of which stays on paper; only this on/off state and which weapon is Legendary are tracked.",
		"/characters/"+idStr+"/science-nin/ninjaneer/legendary-weapon/add", "Designate Legendary Weapon",
		"/characters/"+idStr+"/science-nin/ninjaneer/legendary-weapon/delete",
		wos.LegendaryWeapon, wos.AvailableLegendaryWeapons)
}

func enhancedWeaponSection(characterID int64, nj *scienceNinNinjaneerData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	return ninjaneerWeaponDesignationSection(
		"Enhanced Arsenal: Enhanced Weapon",
		"Which owned weapon is your Enhanced Weapon. An Enhanced Finesse weapon uses Intelligence in place of Dexterity for attack and damage rolls, applied automatically.",
		"/characters/"+idStr+"/science-nin/ninjaneer/enhanced-weapon/add", "Designate Enhanced Weapon",
		"/characters/"+idStr+"/science-nin/ninjaneer/enhanced-weapon/delete",
		nj.EnhancedWeapon, nj.AvailableEnhancedWeapons)
}

func perfectedWeaponMarkSection(characterID int64, nj *scienceNinNinjaneerData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	return ninjaneerWeaponDesignationSection(
		"A Weapon to Surpass: Perfected Weapon",
		"Which owned weapon is your (single, tracked) Perfected Weapon — for The Future of Shinobi: Weapons' own flat Intelligence-Modifier damage bonus, applied automatically once that capstone is granted. The book allows working on an ally's weapon; this app only tracks one of your own.",
		"/characters/"+idStr+"/science-nin/ninjaneer/perfected-weapon-mark/add", "Mark Perfected Weapon",
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
