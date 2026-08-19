package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Cooking-Nin's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Cooking-Nin entry documents that don't already fit the
// generic customResourceGrants pool (see custom_resources.go for Shinobi
// Snacks, Wandering Aroma, War and Food, the 9 subclasses' own bonus-Aura
// pools, and the other Group 1 use-count pools): the Cooking Die's own
// size readout (1d4->1d12, referenced by nearly every base and subclass
// feature but never surfaced per-character before) and Food For the
// Soul's freely re-editable ability-score pick.
//
// Everything else this pass deferred — Revv Up Those Friers' fried-pill
// size readout, the always-on debuff/buff payloads behind the various
// use-count gates above, per-Snack-ingredient effect automation, and any
// mechanic needing a subsystem this app has no shape for yet — is
// documented, not modeled; see CLASS_AUDIT.md's Cooking-Nin detail entry.
const cookingNinSlug = "class/cooking-nin"

// foodForTheSoulFeatureSlug is the base class feature ("Whenever you would
// complete a short or Long Rest, select one ability score of your
// choice...") whose pick this box records.
const foodForTheSoulFeatureSlug = "class/cooking-nin/feature/food-for-the-soul"

// foodForTheSoulOptions: freely re-editable any time (RAW re-picks on every
// short or Long Rest; this app doesn't enforce rest timing, same "trust the
// player" boundary Hand Wraps of Passion/Mastery already draw). The applied
// effect itself (Advantage on allies' first check/save with the chosen
// score) stays a manual/Group 2-3 boundary — no advantage-flag tracking
// exists anywhere in this app — so this only records WHICH score is
// currently selected.
var foodForTheSoulOptions = []featureChoiceOption{
	{Value: "str", Label: "Strength", Description: "Allied creatures gain Advantage on their first Strength Ability Check and Strength saving throw before your next rest."},
	{Value: "dex", Label: "Dexterity", Description: "Allied creatures gain Advantage on their first Dexterity Ability Check and Dexterity saving throw before your next rest."},
	{Value: "con", Label: "Constitution", Description: "Allied creatures gain Advantage on their first Constitution Ability Check and Constitution saving throw before your next rest."},
	{Value: "int", Label: "Intelligence", Description: "Allied creatures gain Advantage on their first Intelligence Ability Check and Intelligence saving throw before your next rest."},
	{Value: "wis", Label: "Wisdom", Description: "Allied creatures gain Advantage on their first Wisdom Ability Check and Wisdom saving throw before your next rest."},
	{Value: "cha", Label: "Charisma", Description: "Allied creatures gain Advantage on their first Charisma Ability Check and Charisma saving throw before your next rest."},
}

// cookingNinClassLevel returns the character's own Cooking-Nin class
// level, or 0 if they have none — mirrors hunterNinClassLevel.
func (s *server) cookingNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, cookingNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// cookingDieSize reads the Cooking Die's own live chart
// (v_class_level_resources, resource_name "Cooking Die") rather than
// hand-transcribing it — same "read the chart, don't retype it" precedent
// lethalAttackDice/flurryDieSize already established. The Cooking Die is
// the Cooking Tool weapon's own damage die and the base die nearly every
// Snack ingredient and Aura effect scales off of.
func (s *server) cookingDieSize(level int) (string, error) {
	var value sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = 'Cooking Die'`,
		cookingNinSlug, level,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// foodForTheSoulView holds Food For the Soul's current pick — nil unless
// the character has reached 2nd level in Cooking-Nin.
type foodForTheSoulView struct {
	Current string // ability key ("str".."cha"), "" if unpicked
	Options []featureChoiceOption
}

// cookingNinTabData is the sheet_cooking_nin box's full data — nil for a
// character with no Cooking-Nin levels, same "no empty state" treatment
// HunterTechniques/MartialTechniques already establish.
type cookingNinTabData struct {
	CookingDieSize string
	FoodForTheSoul *foodForTheSoulView
}

// loadCookingNinTabData returns nil for a character with no Cooking-Nin
// levels.
func (s *server) loadCookingNinTabData(characterID int64, sheet *charsheet.Sheet) (*cookingNinTabData, error) {
	level, err := s.cookingNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}
	dieSize, err := s.cookingDieSize(level)
	if err != nil {
		return nil, err
	}
	data := &cookingNinTabData{CookingDieSize: dieSize}

	if level >= 2 {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		data.FoodForTheSoul = &foodForTheSoulView{
			Current: choices[features.ChoiceKey{FeatureSlug: foodForTheSoulFeatureSlug, ChoiceIndex: 0}],
			Options: foodForTheSoulOptions,
		}
	}

	return data, nil
}

// handleFoodForTheSoul records Food For the Soul's re-selectable
// ability-score pick.
func (s *server) handleFoodForTheSoul(w http.ResponseWriter, r *http.Request) {
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
	valid := false
	for _, o := range foodForTheSoulOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	level, err := s.cookingNinClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin level for food for the soul:", err)
		return
	}
	if level < 2 {
		http.Error(w, "character has not reached 2nd level in Cooking-Nin", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, foodForTheSoulFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set food for the soul:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}
