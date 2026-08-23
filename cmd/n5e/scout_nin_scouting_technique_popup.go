package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// scout_nin_scouting_technique_popup.go: the "Scouting Technique" subclass
// tracker popup — the Maneuvers Known cap+catalog pick every one of the 9
// Scouting Techniques shares (scoped to whichever one the character has
// chosen), plus Tactical Scout's own Tactical Superiority and Signature
// Maneuver, Cloning Scout's own Supreme Clones, Pathfinder Scout's own
// Mobile Savant, Trickster Scout's own freely re-editable Change of Heart
// pick, and Arbiter Scout's own freely re-editable Paragon's Presence pick
// — pulled out of sheet_scout_nin (character_sheet.html) into one popup.
// See subclass_tracker_popup.go's header doc for the shared pattern this
// is built from.
//
// Like Hunter's Creed (hunter_creed_popup.go) and Weapon Form
// (weapon_form_popup.go), ONE popup covers all 9 Scouting Techniques
// rather than one popup per subclass — a character only ever has one
// Scouting Technique active at a time (scoutNinSubclassSlug), so
// handleScoutingTechniquePopup switches on that single slug to decide
// which of the section-builder functions below actually run, the same
// "one popup, per-subclass gating inside" shape sheet_scout_nin's own
// {{if gt .XxxCap 0}} gates already used inline. Assault Scout, Barrier
// Scout, Elemental Scout, and Phantom Scout have no subclass-exclusive
// pick beyond Maneuvers Known at all (confirmed against scout_nin.go — no
// Cap function anywhere else switches on any of their 4 subclass slugs),
// so their own popup only ever shows that one section.
//
// Jack of All Master of None, Signature Technique, Shinobi Adept, Signature
// Jutsu, the base Fighting Stance pick, and Deft Explorer's Mobile readout
// all stay inline on the Core sheet instead of moving here, despite sitting
// beside these subclass-exclusive picks in the book's own text — every one
// of them is a BASE-CLASS feature every Scouting Technique shares
// (jackOfAllCap/signatureTechniqueCap/shinobiAdeptCap/signatureJutsuCap
// take no subclassSlug parameter at all; Tactical Scout only extends Jack
// of All's and Signature Technique's own cap further via its own
// granted-feature checks, the same shape masteredSignatureJutsuFeatureSlug
// extends Signature Jutsu's cap). This is the identical "shared across
// every subclass, stays inline" boundary hunter_creed_popup.go's own
// header doc draws around Hunter Techniques' Lethal Attack/Lethal
// Precision/Hunters Patterns/Hunters Exploits/Defensive Tactics.
//
// Every Known pick here renders as plain text rather than a
// sheet-popup-trigger link (subclassTrackerKnownPick.DetailHref left
// "") — Scout-Nin has no static per-pick detail-popup route for any of
// its catalogs (Maneuvers, Tactical Superiority, Signature Maneuver,
// Supreme Clones, Mobile Savant all draw from hand-curated Go catalogs or
// live class_options/jutsu queries, never a dedicated detail page), the
// same accepted trade-off weapon_form_popup.go's own header doc already
// documents for Styles/Superior Weapon Flurry.

func scoutingTechniquePopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/scout-nin/scouting-technique"
}

func (s *server) handleScoutingTechniquePopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for scouting technique popup:", err)
		return
	}
	data, err := s.loadScoutNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin for scouting technique popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Scouting Technique", CharacterID: id, CharacterName: sheet.Name}
	if data == nil || data.TechniqueName == "" {
		popup.EmptyHint = "This character hasn't chosen a Scouting Technique yet — every Scout-Nin picks one of the 9 Scouting Techniques at 3rd level."
		s.renderSubclassTrackerPopup(w, "character_scout_nin_scouting_technique.html", popup, nil)
		return
	}
	popup.Title = data.TechniqueName

	subclassSlug, _, err := s.scoutNinSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin subclass for scouting technique popup:", err)
		return
	}

	if data.ManeuversCap > 0 {
		popup.Sections = append(popup.Sections, scoutManeuversSection(id, data))
	}
	switch subclassSlug {
	case tacticalScoutSubclassSlug:
		if data.TacticalSuperiorityCap > 0 {
			popup.Sections = append(popup.Sections, tacticalSuperioritySection(id, data))
		}
		if data.SignatureManeuverCap > 0 {
			popup.Sections = append(popup.Sections, signatureManeuverSection(id, data))
		}
	case cloningScoutSubclassSlug:
		if data.SupremeClonesCap > 0 {
			popup.Sections = append(popup.Sections, supremeClonesSection(id, data))
		}
	case pathfinderScoutSubclassSlug:
		if data.MobileSavantCap > 0 {
			popup.Sections = append(popup.Sections, mobileSavantSection(id, data))
		}
	}

	s.renderSubclassTrackerPopup(w, "character_scout_nin_scouting_technique.html", popup, map[string]any{
		"ChangeOfHeart":    data.ChangeOfHeart,
		"ParagonsPresence": data.ParagonsPresence,
	})
}

// scoutManeuversSection builds the popup's own Maneuvers Known section —
// shared by all 9 Scouting Techniques, titled off the character's own
// subclass Maneuvers list name (e.g. "Arbiter Maneuvers"), same field the
// Core sheet's own inline heading used to read.
func scoutManeuversSection(characterID int64, data *scoutNinTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        data.ManeuversListName + " (" + strconv.Itoa(data.ManeuversUsed) + "/" + strconv.Itoa(data.ManeuversCap) + " known)",
		DeleteAction: "/characters/" + idStr + "/scout-nin/scouting-technique/maneuvers/delete",
	}
	for _, k := range data.KnownManeuvers {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.ManeuversUsed < data.ManeuversCap {
		sec.AddAction = "/characters/" + idStr + "/scout-nin/scouting-technique/maneuvers/add"
		sec.AddLabel = "Learn Maneuver"
		for _, o := range data.AvailableManeuvers {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func tacticalSuperioritySection(characterID int64, data *scoutNinTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Tactical Superiority (" + strconv.Itoa(data.TacticalSuperiorityUsed) + "/" + strconv.Itoa(data.TacticalSuperiorityCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/scout-nin/scouting-technique/tactical-superiority/delete",
	}
	for _, k := range data.KnownTacticalSuperiority {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.TacticalSuperiorityUsed < data.TacticalSuperiorityCap {
		sec.AddAction = "/characters/" + idStr + "/scout-nin/scouting-technique/tactical-superiority/add"
		sec.AddLabel = "Learn Maneuver"
		for _, o := range data.AvailableTacticalSuperiority {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func signatureManeuverSection(characterID int64, data *scoutNinTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Signature Maneuver (" + strconv.Itoa(data.SignatureManeuverUsed) + "/" + strconv.Itoa(data.SignatureManeuverCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/scout-nin/scouting-technique/signature-maneuver/delete",
	}
	for _, k := range data.KnownSignatureManeuver {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.SignatureManeuverUsed < data.SignatureManeuverCap {
		sec.AddAction = "/characters/" + idStr + "/scout-nin/scouting-technique/signature-maneuver/add"
		sec.AddLabel = "Mark Signature"
		for _, o := range data.AvailableSignatureManeuver {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func supremeClonesSection(characterID int64, data *scoutNinTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Supreme Clones (" + strconv.Itoa(data.SupremeClonesUsed) + "/" + strconv.Itoa(data.SupremeClonesCap) + " chosen)",
		Hint:         "Your clones can perform the marked Maneuver without expending a Superiority Die once per turn — actually having a clone do so stays a manual call.",
		DeleteAction: "/characters/" + idStr + "/scout-nin/scouting-technique/supreme-clones/delete",
	}
	for _, k := range data.KnownSupremeClones {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.SupremeClonesUsed < data.SupremeClonesCap {
		sec.AddAction = "/characters/" + idStr + "/scout-nin/scouting-technique/supreme-clones/add"
		sec.AddLabel = "Mark Supreme Clone"
		for _, o := range data.AvailableSupremeClones {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func mobileSavantSection(characterID int64, data *scoutNinTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Mobile Savant (" + strconv.Itoa(data.MobileSavantUsed) + "/" + strconv.Itoa(data.MobileSavantCap) + " chosen)",
		DeleteAction: "/characters/" + idStr + "/scout-nin/scouting-technique/mobile-savant/delete",
	}
	for _, k := range data.KnownMobileSavant {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.MobileSavantUsed < data.MobileSavantCap {
		sec.AddAction = "/characters/" + idStr + "/scout-nin/scouting-technique/mobile-savant/add"
		sec.AddLabel = "Learn Jutsu"
		for _, o := range data.AvailableMobileSavant {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// scoutNinTrackerPopupDelete is this popup's own "forget a pick" factory —
// subclass_tracker_popup.go's own subclassTrackerPopupDelete can't be
// reused directly here, since it's hard-typed to charstore.
// ScienceNinSubclassPickCategory/RemoveScienceNinSubclassPick (Science-
// Nin's own picks table), while Scout-Nin's picks live in a separate table
// under charstore.ScoutNinPickCategory/RemoveScoutNinPick. Same shape
// otherwise: fully generic across all 5 of this popup's own catalog
// categories, since removing a pick never needs a budget/cap check.
// Mirrors hunterNinTrackerPopupDelete exactly (hunter_creed_popup.go).
func (s *server) scoutNinTrackerPopupDelete(category charstore.ScoutNinPickCategory) http.HandlerFunc {
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
		if err := charstore.RemoveScoutNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove scout-nin pick for scouting technique popup:", err)
			return
		}
		http.Redirect(w, r, scoutingTechniquePopupPath(id), http.StatusSeeOther)
	}
}

// handleChangeOfHeartPopup is setChangeOfHeart's own redirect-based
// wrapper for the Scouting Technique popup.
func (s *server) handleChangeOfHeartPopup(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setChangeOfHeart(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, scoutingTechniquePopupPath(id), http.StatusSeeOther)
}

// handleParagonsPresencePopup is setParagonsPresence's own redirect-based
// wrapper for the Scouting Technique popup.
func (s *server) handleParagonsPresencePopup(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setParagonsPresence(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, scoutingTechniquePopupPath(id), http.StatusSeeOther)
}
