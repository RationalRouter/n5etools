package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

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
		popup.EmptyHint = "This character has no Elemental Innovationist picks yet — E.I.Ps grants this at 3rd-level Elemental Innovationist."
		s.renderSubclassTrackerPopup(w, "character_science_nin_elemental_innovationist.html", popup, nil)
		return
	}

	ei := data.ElementalInnovationist
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, eipSection(id, ei, data.CreationPointsUsed, data.CreationPointsCap))
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

// eipSection builds the popup's own main E.I.Ps Known/Available section —
// draws from the same shared Creation Points budget S.N.B Upgrades/Spyware
// Programs do (this file's own header doc), so creationPointsUsed/Cap gate
// each Available perk's own Disabled flag the identical way
// snbUpgradesSection's own doc explains (snb_upgrades_popup.go), re-parsing
// each option's raw Cost via parseScienceNinToolStatLine rather than
// reading a dedicated field (scienceNinSubclassOption carries none — see
// that struct's own doc, science_nin_subclasses.go).
func eipSection(characterID int64, ei *scienceNinElementalInnovationistData, creationPointsUsed, creationPointsCap int) subclassTrackerSection {
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
			cost, _, _, _ := parseScienceNinToolStatLine(o.Description)
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description,
				Disabled: creationPointsUsed+cost > creationPointsCap,
			})
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
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description, DescriptionHTML: formatWoWDescription(o.Description)})
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
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description, DescriptionHTML: formatWoWDescription(o.Description)})
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

// addWoWPick validates and stores a W.O.W pick exactly like
// scienceNinSubclassPickAddCore already does for every other flat-slot
// catalog, then additionally forges the weapon into the character's own
// equipped Inventory (see grantWoWWeapon, wow_weapons.go) — the one thing a
// W.o.W pick needs beyond every other subclass tracker's plain "record the
// pick" add, since a W.o.W is a real weapon the player carries, not an
// abstract upgrade slot. Shared by both wow/add routes (Core-sheet AJAX and
// this popup), same "one core, two thin wrappers" shape addEIPPick already
// uses for its own dedicated add.
func (s *server) addWoWPick(id int64, rawSlug string) (int, string) {
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
		false, rawSlug)
	if status != http.StatusOK {
		return status, msg
	}
	name, description, err := s.scienceNinOptionNameAndDescription(rawSlug)
	if err != nil {
		log.Println("look up wow option name:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := s.grantWoWWeapon(id, rawSlug, name, description); err != nil {
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
	if status, msg := s.addWoWPick(id, slug); status != http.StatusOK {
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
	if status, msg := s.addWoWPick(id, slug); status != http.StatusOK {
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
func (s *server) removeWoWPick(id int64, slug string) error {
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
		logWhelpSyncErr(s.syncDraconicGauntletWhelp(id))
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
	if err := s.removeWoWPick(id, slug); err != nil {
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
	if err := s.removeWoWPick(id, slug); err != nil {
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
