package main

import (
	"log"
	"net/http"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// hoshiKujakuModeFeatureSlug identifies Hoshi Clan's Kujaku Mode
// (clan/hoshi/feature/kujaku-mode), granted at 3rd level to every Hoshi
// Clan character — gates whether the toggle shows on the sheet at all.
// hoshiKujakuFlourishFeatSlug/hoshiCosmicKujakuFeatSlug identify the two
// feats named in this feature's own upgrade text, checked here (not just in
// internal/charsheet, which only needs Cosmic Kujaku for its own AC block)
// to build the narrated info panel's feat-conditional lines. Same literal
// slugs as internal/charsheet/charsheet.go's own copies — duplicated rather
// than exported across the package boundary, since nothing else in this
// codebase shares a feature-slug constant between the two packages either.
const (
	hoshiKujakuModeFeatureSlug  = "clan/hoshi/feature/kujaku-mode"
	hoshiKujakuFlourishFeatSlug = "feat/hoshi/kujaku-flourish"
	hoshiCosmicKujakuFeatSlug   = "feat/hoshi/cosmic-kujaku"
)

// kujakuModeView is the sheet's display shape for Hoshi Clan's Kujaku Mode
// toggle and its narrated bonuses. Granted gates whether the toggle shows
// at all (false for every non-Hoshi character, and for a Hoshi character
// below 3rd level); Active is the player's own stored toggle
// (charsheet.Sheet.KujakuModeActive). The rest are computed fresh on every
// render, never stored:
//   - ACBonus is already folded into Sheet.AC by internal/charsheet's own
//     Kujaku Mode block when Active — kept here too only so the info panel
//     can say what it did.
//   - TempHP ("you gain temp hit points equal to your maximum Star
//     Chakra") is read from the character's own star_chakra custom
//     resource rather than written into the editable Sheet.TempHP box —
//     Sheet.TempHP is stored player state that can already hold temp HP
//     from other sources, and this ruleset's temp-HP-doesn't-stack
//     convention means the right auto-behavior (keep the higher value, and
//     don't claw it back the instant the mode's 1-minute duration ends)
//     has no existing mechanism to hook into. Narrated for the player to
//     apply by hand instead, the same "trust the player" boundary
//     wow_whelp.go's own one-time stat prefill already draws.
//   - ChakraControlDie ("+1d4" at 3rd, "+1d6" at 11th, "+1d8" at 18th) and
//     the reach/HS-component/reaction-damage-reduction clauses have no
//     rollable-bonus-die or reaction-trigger mechanism on this sheet to
//     automate into, so they're narrated text too — see
//     hoshiKujakuModeFeatureSlug's own doc comment in charsheet.go for the
//     same boundary drawn from the other side of the package split.
type kujakuModeView struct {
	Granted          bool
	Active           bool
	ACBonus          int
	ChakraControlDie string
	TempHP           int
	FlourishKnown    bool
	CosmicKnown      bool
}

// loadKujakuModeView builds the Kujaku Mode display for one already-
// computed sheet. grantedFeatures must already include the character's
// taken feats (e.g. via loadMergedGrantedFeatures) so FlourishKnown/
// CosmicKnown resolve correctly; customResources must already include the
// character's Star Chakra pool (loadCustomResources) so TempHP resolves.
func loadKujakuModeView(sheet *charsheet.Sheet, grantedFeatures []grantedFeatureRow, customResources []CustomResourceEntry) kujakuModeView {
	if !hasFeature(grantedFeatures, hoshiKujakuModeFeatureSlug) {
		return kujakuModeView{}
	}
	cosmic := hasFeature(grantedFeatures, hoshiCosmicKujakuFeatSlug)
	acBonus := 1
	if cosmic {
		acBonus = 2
	}
	die := "1d4"
	switch {
	case sheet.Level >= 18:
		die = "1d8"
	case sheet.Level >= 11:
		die = "1d6"
	}
	tempHP := 0
	for _, res := range customResources {
		if res.Key == "star_chakra" {
			tempHP = res.Max
			break
		}
	}
	return kujakuModeView{
		Granted:          true,
		Active:           sheet.KujakuModeActive,
		ACBonus:          acBonus,
		ChakraControlDie: die,
		TempHP:           tempHP,
		FlourishKnown:    hasFeature(grantedFeatures, hoshiKujakuFlourishFeatSlug),
		CosmicKnown:      cosmic,
	}
}

// handleSheetKujakuMode toggles Hoshi Clan's Kujaku Mode (form field "on",
// "1" or "0") — same shape as handleSheetInspiration, except it answers
// with the re-rendered "sheet_kujaku_mode" fragment (via respondSheet)
// rather than an unconditional redirect: unlike Inspiration, turning this
// on changes AC (charsheet.Compute's own Kujaku Mode block) and the info
// panel's own narrated numbers, both of which need to update live without
// a full page reload.
func (s *server) handleSheetKujakuMode(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetKujakuModeActive(s.charDB, id, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set kujaku mode active:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_kujaku_mode")
}
