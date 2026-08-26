package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// titan_slots_popup.go: the "Titan Slots" subclass tracker popup — the
// second of this pattern's two initial users (see subclass_tracker_popup.
// go's header doc for the pattern itself). Mech Crafter's own Titan Slots,
// Exo-Suit, and Specialist Crafting, formerly the tail end of
// titan_reference (companion_fields.html), sharing the same Creation
// Points budget the Core tab's Scientific Ninja Tools box still shows.
//
// Unlike S.N.B Upgrades, this data is character-scoped, not tied to any
// one companion row (titanUpgradesData's own header doc: "independent of
// which specific companion row... the picks belong to") — Ordnance
// Training's own text caps a character at one live Titan at a time, so a
// single fixed sidebar button maps cleanly onto it exactly the way it does
// for S.N.B Upgrades, with no per-companion-instance wrinkle to design
// around.

// titanSlotsPopupPath is the Titan Slots popup's own URL.
func titanSlotsPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/titan-slots"
}

// handleTitanSlotsPopup serves the "Titan Slots" popup. Renders with empty
// Sections and an explanatory hint for a character with no Ordnance
// Training — reachable only by visiting this URL directly after a respec,
// since the sidebar button that links here is itself gated on the same
// granted feature (see characters.go's handleCharacterSheet, which passes
// through the "ShowTitanSlotsPopup" the button checks).
func (s *server) handleTitanSlotsPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for titan slots popup:", err)
		return
	}
	data, err := s.loadTitanUpgradesData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load titan upgrades for titan slots popup:", err)
		return
	}

	popup := subclassTrackerPopupData{
		Title:               "Titan Slots",
		CharacterID:         id,
		CharacterName:       sheet.Name,
		RefreshOpenerBlocks: "sheet-summon-tab sheet-science-nin",
	}
	if data == nil {
		popup.EmptyHint = "This character has no Titan Slots yet — Ordnance Training grants this at 3rd-level Mech Crafter."
		s.renderSubclassTrackerPopup(w, "character_titan_slots.html", popup, nil)
		return
	}

	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, titanSlotsSection(id, data))
	if data.ExoSuit != nil {
		popup.Sections = append(popup.Sections, titanExoSuitSection(id, data))
	}
	// Specialist Crafting (14th level) isn't a catalog picker at all — a
	// single "Mech or Weapon" <select>, not part of subclassTrackerSection
	// — so it rides along as an "extra" template field instead.
	s.renderSubclassTrackerPopup(w, "character_titan_slots.html", popup, map[string]any{
		"SpecialistCraftingAvailable": data.SpecialistCraftingAvailable,
		"SpecialistCraftingKeyword":   data.SpecialistCraftingKeyword,
	})
}

// titanSlotsSection builds the popup's own main Titan Slots Known/Available
// section — see titanUpgradesData's own doc for TitanSlotsCap/Used (flat
// slot count, Ordnance Training: "a number of Titan Slots equal to your
// Proficiency Bonus") and the shared Creation Points budget each Known
// upgrade's own effective Cost also draws from. The add-form's own gate
// below checks data.CreationPointsUsed/Cap alongside the slot count,
// matching the original inline template's own "if and (lt TitanSlotsUsed
// TitanSlotsCap) (lt CreationPointsUsed CreationPointsCap)" double gate
// (titan_reference, companion_fields.html) — the slot check alone isn't
// enough, since a Known upgrade's own Cost can exhaust the shared budget
// well before every slot is filled.
func titanSlotsSection(characterID int64, data *titanUpgradesData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Titan Slots (" + strconv.Itoa(data.TitanSlotsUsed) + "/" + strconv.Itoa(data.TitanSlotsCap) + " filled)",
		Hint:         "Mech- or Weapon-keyword upgrades only — installed or replaced during a long rest. Spends from the same Creation Points budget as the Scientific Ninja Tools box on the Core tab.",
		DeleteAction: "/characters/" + idStr + "/titan-slots/delete",
	}
	for _, k := range data.KnownUpgrades {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug:       k.Slug,
			Name:       k.Name,
			Epithet:    titanUpgradeEpithet(k.Tier, k.Keyword, k.Cost, k.Drain),
			DetailHref: "/titan-upgrades/" + k.Slug,
		})
	}
	if data.TitanSlotsUsed < data.TitanSlotsCap && data.CreationPointsUsed < data.CreationPointsCap {
		sec.AddAction = "/characters/" + idStr + "/titan-slots/add"
		sec.AddLabel = "Install Upgrade"
		for _, o := range data.AvailableUpgrades {
			// Cost is the SAME titanEffectiveUpgradeCost result addTitanSlotPick
			// itself checks against the budget (titan.go) — Specialist
			// Crafting's own per-keyword discount has to be folded in before
			// comparing against CreationPointsUsed/Cap, not just before
			// formatting the display Epithet, or a discounted upgrade would
			// show disabled by its pre-discount cost alone.
			cost := titanEffectiveUpgradeCost(o, data.SpecialistCraftingKeyword)
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name,
				Epithet:     titanUpgradeEpithet(o.Tier, o.Keyword, cost, o.Drain),
				Description: o.Description,
				Disabled:    data.CreationPointsUsed+cost > data.CreationPointsCap,
			})
		}
	}
	return sec
}

// titanExoSuitSection builds the popup's own Exo-Suit section (Endless
// Work, 6th level) — a second, separate Mech-keyword-only slot restricted
// to Cost 8 or lower, usable even outside the Titan, still spending from
// the same shared Creation Points budget. Same double gate as
// titanSlotsSection above (slot count AND the shared Creation Points
// budget), matching the original inline template.
func titanExoSuitSection(characterID int64, data *titanUpgradesData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	exo := data.ExoSuit
	sec := subclassTrackerSection{
		Title:        "Exo-Suit (" + strconv.Itoa(exo.Used) + "/" + strconv.Itoa(exo.Cap) + " filled)",
		Hint:         "Mech-keyword upgrades of Cost 8 or lower, usable even while not inside the Titan. Installed during a long rest; a second slot opens at 14th level.",
		DeleteAction: "/characters/" + idStr + "/titan-slots/exosuit/delete",
	}
	for _, k := range exo.Known {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug:       k.Slug,
			Name:       k.Name,
			Epithet:    titanUpgradeEpithet(k.Tier, k.Keyword, k.Cost, k.Drain),
			DetailHref: "/titan-upgrades/" + k.Slug,
		})
	}
	if exo.Used < exo.Cap && data.CreationPointsUsed < data.CreationPointsCap {
		sec.AddAction = "/characters/" + idStr + "/titan-slots/exosuit/add"
		sec.AddLabel = "Install Exo-Suit Upgrade"
		for _, o := range exo.Available {
			// See titanSlotsSection's own comment just above on why the
			// Specialist-Crafting-discounted cost, not the raw catalog Cost,
			// is what has to be compared against the shared budget here.
			cost := titanEffectiveUpgradeCost(o, data.SpecialistCraftingKeyword)
			sec.Available = append(sec.Available, subclassTrackerOption{
				Slug: o.Slug, Name: o.Name,
				Epithet:     titanUpgradeEpithet(o.Tier, o.Keyword, cost, o.Drain),
				Description: o.Description,
				Disabled:    data.CreationPointsUsed+cost > data.CreationPointsCap,
			})
		}
	}
	return sec
}

// titanUpgradeEpithet formats one Titan Upgrade's badge text exactly as
// companion_fields.html's own inline markup did: "Tier · KEYWORD · Cost N"
// plus " · Drain N" only when the upgrade actually drains CCD Chakra.
func titanUpgradeEpithet(tier, keyword string, cost, drain int) string {
	epithet := tier + " · " + strings.ToUpper(keyword) + " · Cost " + strconv.Itoa(cost)
	if drain != 0 {
		epithet += " · Drain " + strconv.Itoa(drain)
	}
	return epithet
}

// handleTitanSlotsAdd installs one Titan Upgrade into an ordinary Titan
// Slot from the Titan Slots popup — reuses addTitanSlotPick's own
// validation (titan.go), which the Companions-tab's own AJAX route also
// uses, then redirects back to the popup instead of swapping a sheet
// fragment.
func (s *server) handleTitanSlotsAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addTitanSlotPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, titanSlotsPopupPath(id), http.StatusSeeOther)
}

// handleTitanSlotsExoSuitAdd installs one Titan Upgrade into the Exo-Suit
// slot from the Titan Slots popup — reuses addTitanExoSuitPick's own
// validation (titan.go).
func (s *server) handleTitanSlotsExoSuitAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addTitanExoSuitPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, titanSlotsPopupPath(id), http.StatusSeeOther)
}

// handleTitanSlotsSpecialistCraftingSet sets Specialist Crafting's own
// Mech/Weapon designation from the Titan Slots popup — reuses
// setTitanSpecialistCraftingKeyword's own validation (titan.go).
func (s *server) handleTitanSlotsSpecialistCraftingSet(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	if status, msg := s.setTitanSpecialistCraftingKeyword(id, keyword); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, titanSlotsPopupPath(id), http.StatusSeeOther)
}
