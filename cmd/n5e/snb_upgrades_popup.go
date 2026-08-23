package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// snb_upgrades_popup.go: the "S.N.B Upgrades" subclass tracker popup — the
// first of this pattern's two initial users (see subclass_tracker_popup.go's
// header doc for the pattern itself). S.N.B Specialist's own S.N.B Upgrades
// slot picker and Future of Shinobi: S.N.B's permanent picks, formerly
// inline in sheet_science_nin (character_sheet.html), both share the same
// Creation Points budget the Core tab's Scientific Ninja Tools box still
// shows — see science_nin_creation_points.html's own doc on why that tile
// is duplicated here rather than shared live across windows.

// snbUpgradesPopupPath is the S.N.B Upgrades popup's own URL.
func snbUpgradesPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/snb-upgrades"
}

// handleSNBUpgradesPopup serves the "S.N.B Upgrades" popup. Renders with
// empty Sections and an explanatory hint for a character with no S.N.B
// Specialist levels — same "why is this empty" treatment
// loadCharacterReference's own "no class levels yet" case gets; reachable
// only by visiting this URL directly after a respec, since the sidebar
// button that links here is itself gated on .ScienceNin.SNBSpecialist.
func (s *server) handleSNBUpgradesPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for snb upgrades popup:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for snb upgrades popup:", err)
		return
	}

	popup := subclassTrackerPopupData{
		Title:         "S.N.B Upgrades",
		CharacterID:   id,
		CharacterName: sheet.Name,
	}
	if data == nil || data.SNBSpecialist == nil {
		popup.EmptyHint = "This character has no S.N.B Upgrades yet — Combat Programming's Scientific Ninja Beast grants this at 3rd-level S.N.B Specialist."
		s.renderSubclassTrackerPopup(w, "character_snb_upgrades.html", popup, nil)
		return
	}

	snb := data.SNBSpecialist
	popup.ShowCreationPoints = true
	popup.CreationPointsUsed = data.CreationPointsUsed
	popup.CreationPointsCap = data.CreationPointsCap
	popup.Sections = append(popup.Sections, snbUpgradesSection(id, snb, data.CreationPointsUsed, data.CreationPointsCap))
	// Matches sheet_science_nin's own {{if or $snb.KnownPermanent
	// $snb.AvailablePermanent}} gate exactly — Future of Shinobi: S.N.B
	// only shows once there is something to see, not merely once
	// PermanentCap is set (a freshly-granted capstone with zero known
	// upgrades yet has nothing AvailablePermanent could offer either).
	if len(snb.KnownPermanent) > 0 || len(snb.AvailablePermanent) > 0 {
		popup.Sections = append(popup.Sections, snbUpgradesPermanentSection(id, snb))
	}
	s.renderSubclassTrackerPopup(w, "character_snb_upgrades.html", popup, nil)
}

// snbUpgradesSection builds the popup's own main Known/Available section —
// see scienceNinSNBSpecialistData's own doc for the underlying data
// (UpgradeCap/Used is the flat slot count; each Known upgrade's own Cost
// also draws from the shared Creation Points budget the popup's own CP
// tile shows above this section). creationPointsUsed/Cap gate the add-form
// alongside the slot count, matching the original inline template's own
// "if and (lt UpgradeUsed UpgradeCap) (lt CreationPointsUsed
// CreationPointsCap)" double gate (sheet_science_nin, character_sheet.
// html) — the slot check alone isn't enough, since a Known upgrade's own
// Cost can exhaust the shared budget well before every slot is filled.
func snbUpgradesSection(characterID int64, snb *scienceNinSNBSpecialistData, creationPointsUsed, creationPointsCap int) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "S.N.B Upgrades (" + strconv.Itoa(snb.UpgradeUsed) + "/" + strconv.Itoa(snb.UpgradeCap) + " slots)",
		Hint:         "Installed into your Scientific Ninja Beast — drawn from the same shared Creation Points budget shown above, so an upgrade can be blocked by either the slot count or the remaining Creation Points, whichever runs out first.",
		DeleteAction: "/characters/" + idStr + "/snb-upgrades/delete",
	}
	for _, k := range snb.KnownUpgrades {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug:       k.Slug,
			Name:       k.Name,
			Epithet:    k.Tier + " · Cost " + strconv.Itoa(k.Cost),
			DetailHref: "/science-nin-picks/snb-upgrade/" + k.Slug,
		})
	}
	if snb.UpgradeUsed < snb.UpgradeCap && creationPointsUsed < creationPointsCap {
		sec.AddAction = "/characters/" + idStr + "/snb-upgrades/add"
		sec.AddLabel = "Install Upgrade"
		for _, o := range snb.AvailableUpgrades {
			epithet := o.Tier
			if o.Epithet != "" {
				epithet += " · " + o.Epithet
			}
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: epithet, Description: o.Description})
		}
	}
	return sec
}

// snbUpgradesPermanentSection builds the popup's own Future of Shinobi:
// S.N.B section (20th-level capstone) — permanent picks no longer take an
// upgrade slot or cost Creation Points, so unlike snbUpgradesSection above,
// neither its Known rows nor its Available options carry a Cost badge.
func snbUpgradesPermanentSection(characterID int64, snb *scienceNinSNBSpecialistData) subclassTrackerSection {
	idStr := strconv.FormatInt(characterID, 10)
	sec := subclassTrackerSection{
		Title:        "Future of Shinobi: S.N.B (" + strconv.Itoa(snb.PermanentUsed) + "/" + strconv.Itoa(snb.PermanentCap) + " chosen)",
		Hint:         "Three known S.N.B Upgrades made permanent — they no longer take an upgrade slot or cost Creation Points.",
		DeleteAction: "/characters/" + idStr + "/snb-upgrades/permanent/delete",
	}
	for _, k := range snb.KnownPermanent {
		sec.Known = append(sec.Known, subclassTrackerKnownPick{
			Slug:       k.Slug,
			Name:       k.Name,
			Epithet:    k.Tier,
			DetailHref: "/science-nin-picks/snb-upgrade/" + k.Slug,
		})
	}
	if snb.PermanentUsed < snb.PermanentCap {
		sec.AddAction = "/characters/" + idStr + "/snb-upgrades/permanent/add"
		sec.AddLabel = "Make Permanent"
		for _, o := range snb.AvailablePermanent {
			sec.Available = append(sec.Available, subclassTrackerOption{Slug: o.Slug, Name: o.Name, Epithet: o.Tier, Description: o.Description})
		}
	}
	return sec
}

// handleSNBUpgradesAdd installs one S.N.B Upgrade from the S.N.B Upgrades
// popup — reuses addSNBUpgradePick's own validation (science_nin_
// subclasses.go), which the Core-sheet's own AJAX route also uses, then
// redirects back to the popup instead of swapping a sheet fragment.
func (s *server) handleSNBUpgradesAdd(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.addSNBUpgradePick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, snbUpgradesPopupPath(id), http.StatusSeeOther)
}

// handleSNBUpgradesPermanentAdd makes one known S.N.B Upgrade permanent
// (Future of Shinobi: S.N.B) from the popup — same cap-only validation
// shape handleScienceNinSubclassPickAdd's generic factory already applies
// for the Core-sheet's own AJAX route to this same category
// (charstore.ScienceNinPickSNBUpgradePermanent, science_nin_subclasses.go),
// reimplemented directly here rather than reused since that factory always
// ends in respondSheet — see subclass_tracker_popup.go's header doc on why
// popup routes need their own handlers instead.
func (s *server) handleSNBUpgradesPermanentAdd(w http.ResponseWriter, r *http.Request) {
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
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for snb upgrade permanent add:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for snb upgrade permanent add:", err)
		return
	}
	if data == nil || data.SNBSpecialist == nil {
		http.Error(w, "character has no S.N.B Upgrades yet", http.StatusBadRequest)
		return
	}
	snb := data.SNBSpecialist
	if snb.PermanentUsed >= snb.PermanentCap {
		http.Error(w, "no Future of Shinobi: S.N.B slots remaining", http.StatusBadRequest)
		return
	}
	var picked *scienceNinSubclassOption
	for i, o := range snb.AvailablePermanent {
		if o.Slug == slug {
			picked = &snb.AvailablePermanent[i]
			break
		}
	}
	if picked == nil {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickSNBUpgradePermanent, slug, ""); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add snb upgrade permanent pick:", err)
		return
	}
	http.Redirect(w, r, snbUpgradesPopupPath(id), http.StatusSeeOther)
}
