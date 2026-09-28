package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/charsheet"
	"github.com/RationalRouter/n5e/internal/charstore"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// genjutsu_subclass_popups.go: every Genjutsu Specialist subclass tracker
// popup. Each section below is one subclass's popup, all built from the
// shared pattern documented in subclass_tracker_popup.go. Tested by
// genjutsu_subclass_popups_test.go.

// The "Psyche Breaker" subclass tracker
// popup — Corrupt Thoughts' own 10th-level "select two Genjutsu you know
// that deal damage" pick, pulled out of sheet_genjutsu (character_sheet.
// html) into its own popup. See subclass_tracker_popup.go's header doc for
// the shared pattern this is built from; see genjutsu_twisted_casting_popup.
// go's own header doc for why this is a separate popup rather than bundled
// with Twisted Casting.
//
// Psyche Breaker sources its Available list from the character's own known
// Genjutsu (knownJutsuOption) rather than a rules-database catalog — same
// "jutsu_id" hand-rolled shape as Twisted Casting. The "that deal damage"
// qualifier is not enforced here either (see psycheBreakerCap's own doc
// comment in genjutsu.go).

// psycheBreakerPopupPath is the Psyche Breaker popup's own URL.
func psycheBreakerPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/genjutsu-specialist/psyche-breaker"
}

// handlePsycheBreakerPopup serves the "Psyche Breaker" popup. Renders with
// an explanatory hint for a character with no Psyche Breaker picks
// available yet (below 10th-level Corrupt Thoughts, or a different Genjutsu
// Pledge entirely) — reachable only by visiting this URL directly after a
// respec, since the sidebar button that links here is itself gated on the
// same PsycheBreakerCap check.
func (s *server) handlePsycheBreakerPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for psyche breaker popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadGenjutsuTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load genjutsu for psyche breaker popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.psyche_breaker"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-genjutsu"}
	if data == nil || data.PsycheBreakerCap == 0 {
		popup.EmptyHint = tr("subclass_tracker.psyche_breaker.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_genjutsu_psyche_breaker.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_genjutsu_psyche_breaker.html", popup, map[string]any{
		"Genjutsu": data,
	})
}

// handlePsycheBreakerPopupAdd is addGenjutsuJutsuPick's own redirect-based
// wrapper for the Psyche Breaker popup.
func (s *server) handlePsycheBreakerPopupAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	status, msg := s.addGenjutsuJutsuPick(id, charstore.GenjutsuPickPsycheBreaker, jutsuID,
		func(d *genjutsuTabData) int { return d.PsycheBreakerUsed },
		func(d *genjutsuTabData) int { return d.PsycheBreakerCap },
		func(d *genjutsuTabData) []knownJutsuOption { return d.AvailablePsycheBreaker })
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, psycheBreakerPopupPath(id), http.StatusSeeOther)
}

// handlePsycheBreakerPopupDelete is a thin redirect-based wrapper around
// RemoveGenjutsuJutsuPick, matching handleGenjutsuJutsuPickDelete's own
// Core-sheet route — removing a pick never needs a budget/cap check, so
// this stays a thin wrapper rather than sharing a "core" method with that
// route.
func (s *server) handlePsycheBreakerPopupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveGenjutsuJutsuPick(s.charDB, id, charstore.GenjutsuPickPsycheBreaker, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove psyche breaker pick (popup):", err)
		return
	}
	http.Redirect(w, r, psycheBreakerPopupPath(id), http.StatusSeeOther)
}

// The "Twisted Casting" subclass tracker
// popup — Beguiler's own 10th-level "select two Genjutsu you know" pick,
// pulled out of sheet_genjutsu (character_sheet.html) into its own popup.
// See subclass_tracker_popup.go's header doc for the shared pattern this is
// built from. Twisted Casting and Psyche Breaker (genjutsu_psyche_breaker_
// popup.go) are two separate popups, not one bundled popup, since each
// belongs to a different Genjutsu Pledge (Beguiler vs Corrupt Thoughts) —
// same "one popup per subclass" shape Stancer/Passionate Flame/Ruin already
// establish for Taijutsu Specialist's own three subclasses.
//
// Twisted Casting sources its Available list from the character's own known
// Genjutsu (knownJutsuOption) rather than a rules-database catalog, so its
// picker form fields are "jutsu_id", not "option_slug" — this doesn't fit
// subclassTrackerSection's shared markup (hard-coded to "option_slug"), so
// the section is hand-rolled here, same shape character_ninjutsu_specialist_
// awakened_scroll.html's own inline markup already uses.

// twistedCastingPopupPath is the Twisted Casting popup's own URL.
func twistedCastingPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/genjutsu-specialist/twisted-casting"
}

// handleTwistedCastingPopup serves the "Twisted Casting" popup. Renders
// with an explanatory hint for a character with no Twisted Casting picks
// available yet (below 10th-level Beguiler, or a different Genjutsu Pledge
// entirely) — reachable only by visiting this URL directly after a respec,
// since the sidebar button that links here is itself gated on the same
// TwistedCastingCap check.
func (s *server) handleTwistedCastingPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for twisted casting popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadGenjutsuTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load genjutsu for twisted casting popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.twisted_casting"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-genjutsu"}
	if data == nil || data.TwistedCastingCap == 0 {
		popup.EmptyHint = tr("subclass_tracker.twisted_casting.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_genjutsu_twisted_casting.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_genjutsu_twisted_casting.html", popup, map[string]any{
		"Genjutsu": data,
	})
}

// handleTwistedCastingPopupAdd is addGenjutsuJutsuPick's own redirect-based
// wrapper for the Twisted Casting popup.
func (s *server) handleTwistedCastingPopupAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	status, msg := s.addGenjutsuJutsuPick(id, charstore.GenjutsuPickTwistedCasting, jutsuID,
		func(d *genjutsuTabData) int { return d.TwistedCastingUsed },
		func(d *genjutsuTabData) int { return d.TwistedCastingCap },
		func(d *genjutsuTabData) []knownJutsuOption { return d.AvailableTwistedCasting })
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, twistedCastingPopupPath(id), http.StatusSeeOther)
}

// handleTwistedCastingPopupDelete is a thin redirect-based wrapper around
// RemoveGenjutsuJutsuPick, matching handleGenjutsuJutsuPickDelete's own
// Core-sheet route — removing a pick never needs a budget/cap check, so
// this stays a thin wrapper rather than sharing a "core" method with that
// route.
func (s *server) handleTwistedCastingPopupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveGenjutsuJutsuPick(s.charDB, id, charstore.GenjutsuPickTwistedCasting, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove twisted casting pick (popup):", err)
		return
	}
	http.Redirect(w, r, twistedCastingPopupPath(id), http.StatusSeeOther)
}
