package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Ever Evolving (class/science-nin/group/scientific-inquiry/shinobi-ware/
// feature/ever-evolving, 14th level): "You can spend Creation Points equal
// to the rank of an Armor Seal to instantly apply it to your Full Metal
// Shinobi armor. You can change this seal with a Full turn Action,
// including choosing no seal to gain the creation points back and remove
// the seal."
//
// Shaped like Titan Upgrades (titan.go) rather than every OTHER pick in
// science_nin_subclasses.go: a single slot (cap 1 — "apply IT to your Full
// Metal Shinobi armor" names exactly one seal at a time), gated by the SAME
// shared Creation Points budget as the base Scientific Ninja Tools catalog
// and every other CP-spending pick, rather than a flat slot-count cap
// against its own independent pool the way Shinobi-Ware Upgrades/Glorious
// Evolution/every generic handleScienceNinSubclassPickAdd catalog works.
// Cost equals the seal's own rank (D=1 ... S=5, jutsuRankOrder) — confirmed
// live against dist/rules.db that every rank D through S actually exists
// among enhancement_seal/armor rows, and the book's own text states no tier
// ceiling the way Martial Defense's guard slots do (contrast
// guardSealTierCap, martial_defense.go), so every armor seal in the catalog
// is offered regardless of character level.
//
// "Change this seal with a Full Turn Action, including choosing no seal to
// gain the creation points back" needs no extra tracked state: deleting the
// current seal (a free refund, same "trust the player" boundary every other
// pick removal in this codebase draws — e.g. handleMartialDefenseSealDelete,
// handleTitanPickDelete) and then adding a new one already models a swap —
// there is nothing left for a dedicated "change" route to do. The action-
// economy cost of the Full Turn Action itself stays manual/narrated, same
// restraint this app already applies everywhere action economy isn't
// separately tracked.
const scienceNinEverEvolvingFeatureSlug = "class/science-nin/group/scientific-inquiry/shinobi-ware/feature/ever-evolving"

// everEvolvingSealCost is the seal's own rank read as a Creation Points
// cost — D=1, C=2, B=3, A=4, S=5, the exact table jutsuRankOrder already
// encodes for jutsu ranks and reused here verbatim ("the rank of an Armor
// Seal" runs the same D-through-S scale).
func everEvolvingSealCost(seal guardSealOption) int {
	return jutsuRankOrder[seal.Rank]
}

// everEvolvingSealOption pairs an Armor Seal with its own Creation Points
// cost (everEvolvingSealCost), precomputed once for display — kept as its
// own type rather than a field added to guardSealOption itself, since Cost
// has no meaning for Martial Defense's own guard-slot picker (guard slots
// spend no Creation Points at all; see martial_defense.go's own header
// doc).
type everEvolvingSealOption struct {
	guardSealOption
	Cost int
}

// loadEverEvolvingSealCatalog lists every Armor Seal Ever Evolving can
// apply — the same base catalog loadGuardSealCatalog draws from
// (martial_defense.go), but with no tier-cap filter: Ever Evolving's own
// text places no level-gated ceiling on which rank is reachable, only a
// Creation Points cost that scales with it.
func (s *server) loadEverEvolvingSealCatalog() ([]everEvolvingSealOption, error) {
	all, err := s.loadArmorSealCatalog()
	if err != nil {
		return nil, err
	}
	out := make([]everEvolvingSealOption, len(all))
	for i, o := range all {
		out[i] = everEvolvingSealOption{guardSealOption: o, Cost: everEvolvingSealCost(o)}
	}
	return out, nil
}

// addEverEvolvingSealPick validates and installs Ever Evolving's own single
// seal, gated by the character's own remaining Creation Points budget —
// modeled directly on handleTitanUpgradeAdd (titan.go), the closest existing
// precedent for a CP-BUDGET-gated pick, rather than
// handleScienceNinSubclassPickAdd's shared factory (which only supports a
// flat used>=cap check). Shared by handleEverEvolvingSealAdd's own
// Core-sheet AJAX route and the Shinobi-Ware subclass tracker popup's own
// redirect-based equivalent (science_nin_shinobi_ware_popup.go).
func (s *server) addEverEvolvingSealPick(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing seal"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for ever evolving seal add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		log.Println("load science-nin for ever evolving seal add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.ShinobiWare == nil {
		return http.StatusBadRequest, "character has no Ever Evolving yet"
	}
	if data.ShinobiWare.EverEvolvingSeal != nil {
		return http.StatusBadRequest, "already have a seal applied — remove it first to change it"
	}
	var picked *everEvolvingSealOption
	for i, o := range data.ShinobiWare.AvailableEverEvolvingSeal {
		if o.Slug == slug {
			picked = &data.ShinobiWare.AvailableEverEvolvingSeal[i]
			break
		}
	}
	if picked == nil {
		return http.StatusBadRequest, "not a valid armor seal"
	}
	if data.CreationPointsUsed+picked.Cost > data.CreationPointsCap {
		return http.StatusBadRequest, "not enough Creation Points remaining"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickEverEvolvingSeal, slug, ""); err != nil {
		log.Println("add ever evolving seal:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleEverEvolvingSealAdd is addEverEvolvingSealPick's own Core-sheet AJAX
// route.
func (s *server) handleEverEvolvingSealAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("seal_slug"))
	if status, msg := s.addEverEvolvingSealPick(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleEverEvolvingSealDelete removes Ever Evolving's own seal, refunding
// its Creation Points for free — freely, at any time, matching this
// codebase's "trust the player" precedent every other pick removal draws.
// This IS "a Full Turn Action, including choosing no seal to gain the
// creation points back" — see this file's header doc for why no separate
// "change" state is needed on top of delete-then-add.
func (s *server) handleEverEvolvingSealDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("seal_slug"))
	if slug == "" {
		http.Error(w, "missing seal", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickEverEvolvingSeal, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove ever evolving seal:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}
