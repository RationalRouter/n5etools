package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// weapon_form_popup.go: the "Weapon Form" subclass tracker popup — each of
// the 8 Weapon Forms' own auto-granted Techniques, cap-gated Styles picker,
// and Slayer Form's own Stalking Predator, plus the base class's own
// Superior Weapon Flurry, pulled out of sheet_weapon_form (character_
// sheet.html) into one popup. See subclass_tracker_popup.go's header doc
// for the shared pattern this is built from.
//
// Unlike every other user of this pattern, ONE popup covers all 8
// subclass-gated Weapon Forms rather than one popup per subclass —
// sheet_weapon_form itself only ever shows content for whichever ONE Form
// the character has chosen (loadWeaponFormTabData returns nil with no Form
// picked), so a single fixed sidebar button gated on .WeaponForm (like
// Titan Slots' own feature-grant gate, rather than a specific
// .ScienceNin.<Subclass> flag) maps cleanly onto it.
//
// Techniques, Styles, and Stalking Predator are genuinely subclass-specific
// (which Techniques/Styles catalog, keyed off the chosen Form's own
// subclass_slug; Stalking Predator is Slayer Form-exclusive), so all three
// render inside a nested <div class="subclass-scope"> for this project's
// blue subclass-specific-content convention (character_weapon_form.html's
// own doc comment has the CSS selector details). Superior Weapon Flurry is
// the base class's own 14th/18th-level pick, independent of which Form is
// chosen (weaponFormTabData's own doc comment), so it renders OUTSIDE that
// wrapper in the page's default (gold) text — the first user of this
// pattern to mix subclass-specific and base-class content in one popup.
//
// Styles and Superior Weapon Flurry are both catalog Known/Available
// pickers with no static per-item detail-popup route (unlike every
// Science-Nin catalog this pattern has converted so far) — Known picks
// render as plain text rather than a sheet-popup-trigger link, the
// documented fallback subclassTrackerKnownPick.DetailHref's own doc comment
// already anticipates ("every current catalog has a static detail route,
// but the field stays optional for a future one that might not"). The
// Available list still shows each option's full description via a hover
// tooltip (subclass_tracker_section's own markup), so only a Known pick's
// detail becomes unavailable to re-read, the same trade-off Martial
// Techniques (staying inline, not converted) already lives with today.

func weaponFormPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/weapon-specialist/weapon-form"
}

func (s *server) handleWeaponFormPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for weapon form popup:", err)
		return
	}
	data, err := s.loadWeaponFormTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon form for popup:", err)
		return
	}

	popup := subclassTrackerPopupData{Title: "Weapon Form", CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-weapon-form"}
	if data == nil {
		popup.EmptyHint = "This character has no Weapon Form chosen yet — every Weapon Specialist picks one of the 8 Weapon Forms at 3rd level."
		s.renderSubclassTrackerPopup(w, "character_weapon_form.html", popup, nil)
		return
	}

	popup.Title = data.FormName
	var superiorWeaponFlurry *subclassTrackerSection
	if data.SuperiorWeaponFlurry != nil {
		sec := superiorWeaponFlurrySection(id, data.SuperiorWeaponFlurry)
		superiorWeaponFlurry = &sec
	}
	stylesSection := weaponFormStylesSection(id, data)
	s.renderSubclassTrackerPopup(w, "character_weapon_form.html", popup, map[string]any{
		"FormName":             data.FormName,
		"Techniques":           data.Techniques,
		"StalkingPredator":     data.StalkingPredator,
		"Styles":               stylesSection,
		"SuperiorWeaponFlurry": superiorWeaponFlurry,
	})
}

func weaponFormStylesSection(characterID int64, data *weaponFormTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        data.FormName + " Styles (" + strconv.Itoa(data.Used) + "/" + strconv.Itoa(data.Cap) + " chosen)",
		Hint:         "Changeable any time.",
		DeleteAction: "/characters/" + idStr + "/weapon-specialist/weapon-form/style/delete",
	}
	for _, k := range data.Known {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.Used < data.Cap {
		sec.AddAction = "/characters/" + idStr + "/weapon-specialist/weapon-form/style/add"
		sec.AddLabel = "Learn Style"
		for _, o := range data.Available {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

func superiorWeaponFlurrySection(characterID int64, data *superiorWeaponFlurryTabData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Superior Weapon Flurry (" + strconv.Itoa(data.Used) + "/" + strconv.Itoa(data.Cap) + " chosen)",
		Hint:         "Each benefit modifies an existing Flurry Technique's own duration or effect — applying that change stays manual/narrated, only which benefits are selected is tracked here.",
		DeleteAction: "/characters/" + idStr + "/weapon-specialist/weapon-form/superior-weapon-flurry/delete",
	}
	for _, k := range data.Known {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{Slug: k.Slug, Name: k.Name})
	}
	if data.Used < data.Cap {
		sec.AddAction = "/characters/" + idStr + "/weapon-specialist/weapon-form/superior-weapon-flurry/add"
		sec.AddLabel = "Select Benefit"
		for _, o := range data.Available {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Description: o.Description})
		}
	}
	return sec
}

// handleWeaponFormStylePopupAdd is addWeaponFormStyle's own redirect-based
// equivalent for the Weapon Form popup.
func (s *server) handleWeaponFormStylePopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addWeaponFormStyle(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, weaponFormPopupPath(id), http.StatusSeeOther)
}

// handleWeaponFormStylePopupDelete drops one known Style from the Weapon
// Form popup — fully generic removal (RemoveWeaponFormStyle needs no
// budget/cap check), same reasoning subclassTrackerPopupDelete's own doc
// comment gives for why the delete side never needs a hand-written core
// function the way the add side does.
func (s *server) handleWeaponFormStylePopupDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing style", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveWeaponFormStyle(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove weapon form style (popup):", err)
		return
	}
	http.Redirect(w, r, weaponFormPopupPath(id), http.StatusSeeOther)
}

// handleStalkingPredatorPopup is setStalkingPredator's own redirect-based
// equivalent for the Weapon Form popup.
func (s *server) handleStalkingPredatorPopup(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setStalkingPredator(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, weaponFormPopupPath(id), http.StatusSeeOther)
}

// handleSuperiorWeaponFlurryPopupAdd is addSuperiorWeaponFlurryBenefit's
// own redirect-based equivalent for the Weapon Form popup.
func (s *server) handleSuperiorWeaponFlurryPopupAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addSuperiorWeaponFlurryBenefit(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, weaponFormPopupPath(id), http.StatusSeeOther)
}

// handleSuperiorWeaponFlurryPopupDelete drops one selected benefit from the
// Weapon Form popup — fully generic removal, same reasoning
// handleWeaponFormStylePopupDelete's own doc comment gives.
func (s *server) handleSuperiorWeaponFlurryPopupDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing benefit", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveSuperiorWeaponFlurryBenefit(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove superior weapon flurry benefit (popup):", err)
		return
	}
	http.Redirect(w, r, weaponFormPopupPath(id), http.StatusSeeOther)
}
