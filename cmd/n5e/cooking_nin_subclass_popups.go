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

// cooking_nin_subclass_popups.go: every Cooking-Nin subclass tracker popup.
// Each section below is one subclass's popup, all built from the shared
// pattern documented in subclass_tracker_popup.go. Tested by
// cooking_nin_subclass_popups_test.go.

// The "Nature's Blend Enhancements"
// subclass tracker popup — Gastrochemist's own Enhancement picker, pulled
// out of sheet_cooking_nin (character_sheet.html) into its own popup. See
// subclass_tracker_popup.go's header doc for the shared pattern this is
// built from.
//
// Sources its Available list from the character's own known jutsu of their
// Nature's Blend release element(s) (knownJutsuOption) rather than a
// rules-database catalog, and pairs each pick with a second field
// (enhancement_type) — this doesn't fit subclassTrackerSection's shared
// markup (hard-coded to a single "option_slug"), so the section is
// hand-rolled here, same shape character_genjutsu_twisted_casting.html's
// own hand-rolled section uses for its own single extra field.

// blendEnhancementsPopupPath is the Nature's Blend Enhancements popup's own
// URL.
func blendEnhancementsPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/blend-enhancements"
}

// handleBlendEnhancementsPopup serves the "Nature's Blend Enhancements"
// popup. Renders with an explanatory hint for a character with no
// Enhancement slots yet (not a Gastrochemist, or below the level granting
// the first slot) — reachable only by visiting this URL directly after a
// respec, since the sidebar button that links here is itself gated on the
// same BlendEnhancementCap check.
func (s *server) handleBlendEnhancementsPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for blend enhancements popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadCookingNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for blend enhancements popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.natures_blend_enhancements"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-cooking-nin"}
	if data == nil || data.BlendEnhancementCap == 0 {
		popup.EmptyHint = tr("subclass_tracker.cooking_blend_enhancements.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_blend_enhancements.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_blend_enhancements.html", popup, map[string]any{
		"CookingNin": data,
	})
}

// handleBlendEnhancementsPopupAdd is addCookingNinBlendEnhancementPick's
// own redirect-based wrapper for the Blend Enhancements popup
// (cooking_nin.go).
func (s *server) handleBlendEnhancementsPopupAdd(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing jutsu", http.StatusBadRequest)
		return
	}
	enhancementType := strings.TrimSpace(r.FormValue("enhancement_type"))
	if status, msg := s.addCookingNinBlendEnhancementPick(id, jutsuID, enhancementType); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, blendEnhancementsPopupPath(id), http.StatusSeeOther)
}

// handleBlendEnhancementsPopupDelete is a thin redirect-based wrapper
// around RemoveCookingNinBlendEnhancementPick, matching
// handleCookingNinBlendEnhancementDelete's own Core-sheet route — removing
// a pick never needs a budget/cap check, so this stays a thin wrapper
// rather than sharing a "core" method with that route.
func (s *server) handleBlendEnhancementsPopupDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing jutsu", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveCookingNinBlendEnhancementPick(s.charDB, id, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove cooking-nin blend enhancement pick (popup):", err)
		return
	}
	http.Redirect(w, r, blendEnhancementsPopupPath(id), http.StatusSeeOther)
}

// The "Expert Combatant (Battle
// Cook)" subclass tracker popup — Battle Cook's own 2nd-level
// weapon-classification pick, pulled out of sheet_cooking_nin
// (character_sheet.html) into its own popup. See subclass_tracker_popup.go's
// header doc for the shared pattern this is built from. Distinct from
// Combat Medic's own unrelated "Expert Combatant" feature (Medical-Nin,
// medical_nin_combat_medic_popup.go's handleExpertCombatantPopupAdd/Delete)
// — the two classes' features simply share a printed name, nothing else —
// so every identifier here says "BattleCook" explicitly rather than reusing
// the bare "ExpertCombatant" names Combat Medic's own popup already claims.
//
// A single lock-once-chosen <select>, not a catalog Known/Available list,
// so this is hand-rolled rather than routed through the shared
// subclass_tracker_section partial — same shape Combat Medic's own Fighting
// Stance <select> already establishes.

// battleCookExpertCombatantPopupPath is the Expert Combatant (Battle Cook)
// popup's own URL.
func battleCookExpertCombatantPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/expert-combatant"
}

// handleBattleCookExpertCombatantPopup serves the "Expert Combatant (Battle
// Cook)" popup. Renders with an explanatory hint for a character with no
// Expert Combatant yet (below 2nd-level Battle Cook, or a different Cooking
// Focus entirely) — reachable only by visiting this URL directly after a
// respec, since the sidebar button that links here is itself gated on the
// same .CookingNin.ExpertCombatant check.
func (s *server) handleBattleCookExpertCombatantPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for expert combatant (battle cook) popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadCookingNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for expert combatant (battle cook) popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.expert_combatant_battle_cook"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-cooking-nin"}
	if data == nil || data.ExpertCombatant == nil {
		popup.EmptyHint = tr("subclass_tracker.cooking_expert_combatant.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_expert_combatant.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_expert_combatant.html", popup, map[string]any{
		"ExpertCombatant": data.ExpertCombatant,
	})
}

// handleBattleCookExpertCombatantPopupSet is
// setBattleCookExpertCombatantWeapon's own redirect-based wrapper for the
// Expert Combatant (Battle Cook) popup (cooking_nin.go).
func (s *server) handleBattleCookExpertCombatantPopupSet(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setBattleCookExpertCombatantWeapon(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, battleCookExpertCombatantPopupPath(id), http.StatusSeeOther)
}

// The "Fast and Furious" subclass
// tracker popup — Entremetier Chef's own 2nd-level Int-or-Cha-for-Initiative
// pick, pulled out of sheet_cooking_nin (character_sheet.html) into its own
// popup. See subclass_tracker_popup.go's header doc for the shared pattern
// this is built from.
//
// A single, freely re-editable <select>, not a catalog Known/Available
// list, so this is hand-rolled rather than routed through the shared
// subclass_tracker_section partial — same shape Combat Medic's own Fighting
// Stance <select> already establishes.

// fastAndFuriousPopupPath is the Fast and Furious popup's own URL.
func fastAndFuriousPopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/fast-and-furious"
}

// handleFastAndFuriousPopup serves the "Fast and Furious" popup. Renders
// with an explanatory hint for a character without the feature (below
// 2nd-level Entremetier Chef, or a different Cooking Focus entirely) —
// reachable only by visiting this URL directly after a respec, since the
// sidebar button that links here is itself gated on the same
// .CookingNin.FastAndFurious check.
func (s *server) handleFastAndFuriousPopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for fast and furious popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadCookingNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for fast and furious popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.fast_and_furious"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-cooking-nin"}
	if data == nil || data.FastAndFurious == nil {
		popup.EmptyHint = tr("subclass_tracker.cooking_fast_and_furious.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_fast_and_furious.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_fast_and_furious.html", popup, map[string]any{
		"FastAndFurious": data.FastAndFurious,
	})
}

// handleFastAndFuriousPopupSet is setFastAndFurious's own redirect-based
// wrapper for the Fast and Furious popup (cooking_nin.go).
func (s *server) handleFastAndFuriousPopupSet(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setFastAndFurious(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, fastAndFuriousPopupPath(id), http.StatusSeeOther)
}

// The "Bonus Tool Infusion: Pipe" subclass
// tracker popup — Herbalist's own second Cooking Tool Infusion weapon
// (implement, damage type, and one weapon property each at 3rd/6th/11th
// level), pulled out of sheet_cooking_nin (character_sheet.html) into its
// own popup. See subclass_tracker_popup.go's header doc for the shared
// pattern this is built from.
//
// Every pick here is a single-value, lock-once-chosen <select> — the same
// shape Cooking Tool Infusion's own (base-class, still inline) picks use —
// not a catalog Known/Available list, so this is hand-rolled rather than
// routed through the shared subclass_tracker_section partial, same
// reasoning character_titan_slots.html's own Specialist Crafting <select>
// and character_medical_nin_combat_medic.html's own Fighting Stance
// <select> already establish. pipePopupSetHandler is a small local factory
// (not subclassTrackerPopupDelete's generic, cross-file one) since all 5
// picks share the identical (id, value) -> (status, msg) shape — unlike the
// Add side elsewhere in this pattern, there is no per-catalog budget/cap
// check to differ here.

// pipePopupPath is the Pipe popup's own URL.
func pipePopupPath(id int64) string {
	return "/characters/" + strconv.FormatInt(id, 10) + "/cooking-nin/pipe"
}

// handlePipePopup serves the "Bonus Tool Infusion: Pipe" popup. Renders
// with an explanatory hint for a character with no Pipe yet (below
// 3rd-level Herbalist, or a different Cooking Focus entirely) — reachable
// only by visiting this URL directly after a respec, since the sidebar
// button that links here is itself gated on the same
// .CookingNin.BonusToolInfusionPipe check.
func (s *server) handlePipePopup(w http.ResponseWriter, r *http.Request) {
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
		log.Println("compute sheet for pipe popup:", err)
		return
	}
	locale := localeFromContext(r.Context())
	data, err := s.loadCookingNinTabData(id, sheet, locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for pipe popup:", err)
		return
	}
	tr := i18n.Translator(locale)

	popup := subclassTrackerPopupData{Title: tr("sheet.bonus_tool_infusion_pipe"), CharacterID: id, CharacterName: sheet.Name, RefreshOpenerBlocks: "sheet-cooking-nin"}
	if data == nil || data.BonusToolInfusionPipe == nil {
		popup.EmptyHint = tr("subclass_tracker.cooking_pipe.empty_hint")
		s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_pipe.html", popup, nil)
		return
	}

	s.renderSubclassTrackerPopup(w, r, "character_cooking_nin_pipe.html", popup, map[string]any{
		"Pipe": data.BonusToolInfusionPipe,
	})
}

// pipePopupSetHandler builds one of the Pipe popup's 5 redirect-based
// "Set" routes, each a thin wrapper around one of cooking_nin.go's own
// setCookingToolPipeImplement/DamageType/PropertyL3/L6/L11 — the identical
// validation path the Core-sheet's own AJAX routes already use.
func (s *server) pipePopupSetHandler(setFn func(id int64, value string) (int, string)) http.HandlerFunc {
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
		value := strings.TrimSpace(r.FormValue("value"))
		if status, msg := setFn(id, value); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		http.Redirect(w, r, pipePopupPath(id), http.StatusSeeOther)
	}
}
