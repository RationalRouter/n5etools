package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// hunter_creed_popup.go: the "Hunter's Creed" subclass tracker popup — each
// of the 8 Hunter's Creeds' own subclass-exclusive cap+catalog pick(s)
// (Blade Warden's Warden Weapon + Warden Weapon Property, Necrotic Hand's
// Medical Assassination Technique, Grave Stalker's Shadow Assassination
// Technique, Arsenalist's Arsenal, Undertaker's Toxic Assassination
// Technique, Vice Agent's Vice Assassination Technique, Void Walker's Void
// Assassination Technique, Wolves Legacy's Prosthetic Attachments + Wolf
// Techniques), pulled out of sheet_hunter_techniques (character_sheet.html)
// into one popup. See subclass_tracker_popup.go's header doc for the shared
// pattern this is built from.
//
// Like Weapon Form (weapon_form_popup.go), ONE popup covers all 8 Creeds
// rather than one popup per Creed — a character only ever has one Hunter's
// Creed active at a time (hunterNinSubclassSlug), so handleHunterCreedPopup
// switches on that single slug to decide which of the 10 section-builder
// functions below actually run, the same "one popup, per-subclass gating
// inside" shape sheet_hunter_techniques' own {{if gt .XxxCap 0}} gates
// already used inline. Unlike Weapon Form, nothing in this popup is
// base-class content shared by every Creed — Lethal Attack, Lethal
// Precision, Hunters Patterns, Hunters Exploits, Defensive Tactics, and
// Practiced Combatant's Fighting Stance sub-picker all stay inline on the
// Core sheet (every Creed shares them, none is subclass-exclusive), so
// character_hunter_creed.html needs none of Weapon Form's split-wrapper
// trick for mixing gold/blue content — the whole popup is subclass-specific
// blue text, the same single-.subclass-scope-wrapper shape character_snb_
// upgrades.html/character_science_nin_shinobi_ware.html already use.
//
// Arsenal Item and Wolf Technique keep their own bespoke add-validation
// (hunterArsenalItemAddCore/hunterWolfTechniqueAddCore, hunter_nin.go)
// rather than hunterNinTrackerPopupAdd's generic factory, since part of
// each pick's own effect (learning the matching jutsu, resolved off the
// picks table by hunterNinArsenalItemGrantedJutsu/hunterNinWolfTechnique
// GrantedJutsu) is computed elsewhere — the exact same "own handler, not
// the shared factory" split the Core-sheet's own handleHunterArsenalItemAdd/
// handleHunterWolfTechniqueAdd already draw, not something new this popup
// introduces.

func hunterCreedPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/hunter-nin/creed"
}

func (s *server) handleHunterCreedPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for hunter creed popup:", err)
		return
	}
	data, err := s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load hunter techniques for creed popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Hunter's Creed", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-hunter-techniques sheet-weapon-attacks"}
	if data == nil || data.CreedName == "" {
		popup.EmptyHint = "This character hasn't chosen a Hunter's Creed yet — every Hunter-Nin picks one of the 8 Creeds at 3rd level."
		s.renderSubclassTrackerPopup(w, "character_hunter_creed.html", popup, nil)
		return
	}

	popup.Title = data.CreedName
	subclassSlug, _, err := s.hunterNinSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load hunter subclass for creed popup:", err)
		return
	}
	switch subclassSlug {
	case bladeWardenSubclassSlug:
		if data.WardenWeaponCap > 0 {
			popup.Sections = append(popup.Sections, wardenWeaponSection(id, data))
		}
		if data.WardenWeaponPropertyCap > 0 {
			popup.Sections = append(popup.Sections, wardenWeaponPropertySection(id, data))
		}
	case necroticHandSubclassSlug:
		if data.MedicalTechniqueCap > 0 {
			popup.Sections = append(popup.Sections, medicalTechniqueSection(id, data))
		}
	case graveStalkerSubclassSlug:
		if data.ShadowTechniqueCap > 0 {
			popup.Sections = append(popup.Sections, shadowTechniqueSection(id, data))
		}
	case arsenalistSubclassSlug:
		if data.ArsenalItemCap > 0 {
			popup.Sections = append(popup.Sections, arsenalItemSection(id, data))
		}
	case undertakerSubclassSlug:
		if data.ToxicTechniqueCap > 0 {
			popup.Sections = append(popup.Sections, toxicTechniqueSection(id, data))
		}
	case viceAgentSubclassSlug:
		if data.ViceTechniqueCap > 0 {
			popup.Sections = append(popup.Sections, viceTechniqueSection(id, data))
		}
	case voidWalkerSubclassSlug:
		if data.VoidTechniqueCap > 0 {
			popup.Sections = append(popup.Sections, voidTechniqueSection(id, data))
		}
	case wolvesLegacySubclassSlug:
		if data.ProstheticAttachmentCap > 0 {
			popup.Sections = append(popup.Sections, prostheticAttachmentSection(id, data))
		}
		if data.WolfTechniqueCap > 0 {
			popup.Sections = append(popup.Sections, wolfTechniqueSection(id, data))
		}
	}
	s.renderSubclassTrackerPopup(w, "character_hunter_creed.html", popup, nil)
}

func wardenWeaponSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Warden Weapon (" + strconv.Itoa(data.WardenWeaponUsed) + "/" + strconv.Itoa(data.WardenWeaponCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/warden-weapon/delete",
	}
	for _, k := range data.KnownWardenWeapon {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/warden-weapon/" + k.Slug})
	}
	if data.WardenWeaponUsed < data.WardenWeaponCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/warden-weapon/add"
		sec.AddLabel = "Choose Warden Weapon"
		for _, o := range data.AvailableWardenWeapon {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func wardenWeaponPropertySection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Warden Weapon Property (" + strconv.Itoa(data.WardenWeaponPropertyUsed) + "/" + strconv.Itoa(data.WardenWeaponPropertyCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/warden-weapon-property/delete",
	}
	for _, k := range data.KnownWardenWeaponProperty {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/warden-weapon-property/" + k.Slug})
	}
	if data.WardenWeaponPropertyUsed < data.WardenWeaponPropertyCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/warden-weapon-property/add"
		sec.AddLabel = "Learn Property"
		for _, o := range data.AvailableWardenWeaponProperty {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func medicalTechniqueSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Medical Assassination Technique (" + strconv.Itoa(data.MedicalTechniqueUsed) + "/" + strconv.Itoa(data.MedicalTechniqueCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/medical-technique/delete",
	}
	for _, k := range data.KnownMedicalTechnique {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/medical-technique/" + k.Slug})
	}
	if data.MedicalTechniqueUsed < data.MedicalTechniqueCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/medical-technique/add"
		sec.AddLabel = "Learn Technique"
		for _, o := range data.AvailableMedicalTechnique {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func shadowTechniqueSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Shadow Assassination Technique (" + strconv.Itoa(data.ShadowTechniqueUsed) + "/" + strconv.Itoa(data.ShadowTechniqueCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/shadow-technique/delete",
	}
	for _, k := range data.KnownShadowTechnique {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/shadow-technique/" + k.Slug})
	}
	if data.ShadowTechniqueUsed < data.ShadowTechniqueCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/shadow-technique/add"
		sec.AddLabel = "Learn Technique"
		for _, o := range data.AvailableShadowTechnique {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func arsenalItemSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Arsenal (" + strconv.Itoa(data.ArsenalItemUsed) + "/" + strconv.Itoa(data.ArsenalItemCap) + " chosen)",
		Hint:         "Manipulated Tools items also learn the matching jutsu automatically.",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/arsenal-item/delete",
	}
	for _, k := range data.KnownArsenalItem {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/arsenal-item/" + k.Slug})
	}
	if data.ArsenalItemUsed < data.ArsenalItemCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/arsenal-item/add"
		sec.AddLabel = "Add to Arsenal"
		for _, o := range data.AvailableArsenalItem {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func toxicTechniqueSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Toxic Assassination Technique (" + strconv.Itoa(data.ToxicTechniqueUsed) + "/" + strconv.Itoa(data.ToxicTechniqueCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/toxic-technique/delete",
	}
	for _, k := range data.KnownToxicTechnique {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/toxic-technique/" + k.Slug})
	}
	if data.ToxicTechniqueUsed < data.ToxicTechniqueCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/toxic-technique/add"
		sec.AddLabel = "Learn Technique"
		for _, o := range data.AvailableToxicTechnique {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func viceTechniqueSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Vice Assassination Technique (" + strconv.Itoa(data.ViceTechniqueUsed) + "/" + strconv.Itoa(data.ViceTechniqueCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/vice-technique/delete",
	}
	for _, k := range data.KnownViceTechnique {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/vice-technique/" + k.Slug})
	}
	if data.ViceTechniqueUsed < data.ViceTechniqueCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/vice-technique/add"
		sec.AddLabel = "Learn Technique"
		for _, o := range data.AvailableViceTechnique {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func voidTechniqueSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Void Assassination Technique (" + strconv.Itoa(data.VoidTechniqueUsed) + "/" + strconv.Itoa(data.VoidTechniqueCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/void-technique/delete",
	}
	for _, k := range data.KnownVoidTechnique {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/void-technique/" + k.Slug})
	}
	if data.VoidTechniqueUsed < data.VoidTechniqueCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/void-technique/add"
		sec.AddLabel = "Learn Technique"
		for _, o := range data.AvailableVoidTechnique {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// prostheticAttachmentSection's own Hint references the Core tab, not
// "above" — unlike the inline block this replaced, the Special Resources
// tile behind these attachments' per-rest use pool isn't shown anywhere in
// this popup, matching titan_slots_popup.go's own "on the Core tab" phrasing
// for the identical shape of cross-reference.
func prostheticAttachmentSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Prosthetic Attachments (" + strconv.Itoa(data.ProstheticAttachmentUsed) + "/" + strconv.Itoa(data.ProstheticAttachmentCap) + " chosen)",
		Hint:         "Spent from your Prosthetic Attachments pool in Special Resources on the Core tab.",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/prosthetic-attachment/delete",
	}
	for _, k := range data.KnownProstheticAttachment {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/prosthetic-attachment/" + k.Slug})
	}
	if data.ProstheticAttachmentUsed < data.ProstheticAttachmentCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/prosthetic-attachment/add"
		sec.AddLabel = "Learn Attachment"
		for _, o := range data.AvailableProstheticAttachment {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func wolfTechniqueSection(characterID int64, data *hunterTechniquesTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Wolf Techniques (" + strconv.Itoa(data.WolfTechniqueUsed) + "/" + strconv.Itoa(data.WolfTechniqueCap) + " chosen)",
		Hint:         "Learning a Wolf Technique also learns its jutsu. Casting it by spending two uses of your Prosthetic Attachments instead of Chakra (always B-Rank, no component requirement) stays a manual choice at cast time — nothing here automates that alternate cost.",
		DeleteAction: "/characters/" + idStr + "/hunter-nin/creed/wolf-technique/delete",
	}
	for _, k := range data.KnownWolfTechnique {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name, DetailHref: "/hunter-techniques/wolf-technique/" + k.Slug})
	}
	if data.WolfTechniqueUsed < data.WolfTechniqueCap {
		sec.AddAction = "/characters/" + idStr + "/hunter-nin/creed/wolf-technique/add"
		sec.AddLabel = "Learn Technique"
		for _, o := range data.AvailableWolfTechnique {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// hunterNinTrackerPopupDelete is this popup's own "forget a pick" factory —
// subclass_tracker_popup.go's own subclassTrackerPopupDelete can't be reused
// directly here, since it's hard-typed to charstore.ScienceNinSubclassPickCategory/
// RemoveScienceNinSubclassPick (Science-Nin's own picks table), while Hunter-
// Nin's picks live in a separate table under charstore.HunterNinPickCategory/
// RemoveHunterNinPick. Same shape otherwise: fully generic across all 10 of
// this popup's own categories, since removing a pick never needs a
// budget/cap check.
func (s *server) hunterNinTrackerPopupDelete(category charstore.HunterNinPickCategory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if err := charstore.RemoveHunterNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove hunter pick for creed popup:", err)
			return
		}
		http.Redirect(w, r, hunterCreedPopupPath(id), http.StatusSeeOther)
	}
}

// handleHunterArsenalItemPopupAdd is hunterArsenalItemAddCore's own
// redirect-based wrapper for the Hunter's Creed popup.
func (s *server) handleHunterArsenalItemPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.hunterArsenalItemAddCore(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, hunterCreedPopupPath(id), http.StatusSeeOther)
}

// handleHunterWolfTechniquePopupAdd is hunterWolfTechniqueAddCore's own
// redirect-based wrapper for the Hunter's Creed popup.
func (s *server) handleHunterWolfTechniquePopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.hunterWolfTechniqueAddCore(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, hunterCreedPopupPath(id), http.StatusSeeOther)
}
