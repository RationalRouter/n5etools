package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Technobi's S.E.N.Ts (3rd level): "During a Short Rest you can work on a
// stack of Arrows, Bolts, Kunai, Shuriken, or Senbon. This stack becomes
// your S.E.N.T. It immediately becomes a d10 stack and its damage die
// increases by a step. You can also use your Intelligence Modifier in
// place of Dexterity for all weapon attack and damage rolls using S.E.N.
// Ts." Modeled the same shape as Cooking Tool Infusion's own implement
// pick (cooking_nin.go): a single free-form catalog choice stored under
// the feature's own slug via the generic character_feature_choices store,
// with buildAttacks applying the ability/die-size override to whichever
// equipped item matches the pick — except this pick stays freely
// re-selectable every short rest (no lock), the same "current value,
// changeable any time" shape Fast and Furious/Food For the Soul already
// use, rather than Cooking Tool Implement's permanent-once-chosen one.

// scienceNinSENTWeaponSlugs is every Ammunition-tagged weapon matching the
// feature's own named ammo types (Arrows, Bolts, Kunai, Shuriken, Senbon) —
// deliberately excludes Fuma-Shuriken/Monster Shuriken (Returning, not
// Ammunition — not a consumable stack) and every other ammunition weapon
// whose flavor isn't one of the five named types (Blowgun, Bola, Sling,
// firearms).
var scienceNinSENTWeaponSlugs = []string{
	"weapon/longbow",
	"weapon/short-bow",
	"weapon/crossbow-hand",
	"weapon/crossbow-heavy",
	"weapon/light-crossbow",
	"weapon/kunai",
	"weapon/shuriken",
	"weapon/senbon",
}

// loadSENTWeaponCatalog reads the S.E.N.T-eligible weapons straight off the
// equipment table by slug, same "read the catalog, don't hand-transcribe
// it" shape loadCookingToolImplementCatalog already establishes, just
// filtered by an explicit slug list instead of a shared prefix (these 8
// weapons share no slug pattern of their own).
func (s *server) loadSENTWeaponCatalog() ([]featureChoiceOption, error) {
	placeholders := make([]string, len(scienceNinSENTWeaponSlugs))
	args := make([]any, len(scienceNinSENTWeaponSlugs))
	for i, slug := range scienceNinSENTWeaponSlugs {
		placeholders[i] = "?"
		args[i] = slug
	}
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, COALESCE(description, '') FROM equipment WHERE slug IN (`+strings.Join(placeholders, ",")+`) ORDER BY name`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []featureChoiceOption
	for rows.Next() {
		var o featureChoiceOption
		if err := rows.Scan(&o.Value, &o.Label, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// sentChoiceKey is the single ChoiceIndex slot S.E.N.T's pick lives under —
// one feature slug, one pick, same shape fastAndFuriousFeatureSlug/
// foodForTheSoulFeatureSlug already use with a bare 0.
var sentChoiceKey = features.ChoiceKey{FeatureSlug: scienceNinSENTsFeatureSlug, ChoiceIndex: 0}

// sentAttackOverrides resolves the equipped-item slug the character has
// currently worked into their S.E.N.T, gated on actually holding the
// feature. weaponSlug == "" means "no pick yet, or no S.E.N.Ts at all" —
// buildAttacks treats that as "never matches any equipped item", same
// "empty means untouched" shape cookingToolInfusionAttackOverrides already
// uses.
func (s *server) sentAttackOverrides(characterID int64, sheet *charsheet.Sheet) (weaponSlug string, err error) {
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return "", err
	}
	if !hasGrantedFeature(grantedFeatures, scienceNinSENTsFeatureSlug) {
		return "", nil
	}
	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return "", err
	}
	return choices[sentChoiceKey], nil
}

// handleSENTPick records (or changes) which catalog weapon the character
// has currently worked into their S.E.N.T. Freely re-selectable, matching
// the feature's own "during a Short Rest you can work on a stack" text —
// unlike Cooking Tool Implement, nothing about this pick is worded as
// permanent.
func (s *server) handleSENTPick(w http.ResponseWriter, r *http.Request) {
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
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for sent pick:", err)
		return
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load granted features for sent pick:", err)
		return
	}
	if !hasGrantedFeature(grantedFeatures, scienceNinSENTsFeatureSlug) {
		http.Error(w, "character does not have S.E.N.Ts", http.StatusBadRequest)
		return
	}
	catalog, err := s.loadSENTWeaponCatalog()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load sent weapon catalog:", err)
		return
	}
	valid := false
	for _, o := range catalog {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid S.E.N.T weapon", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, scienceNinSENTsFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set sent pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}
