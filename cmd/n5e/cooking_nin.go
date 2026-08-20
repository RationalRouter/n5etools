package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
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
// feature but never surfaced per-character before), Food For the Soul's
// freely re-editable ability-score pick, and the Gastrochemist subclass's
// own Nature's Blend Enhancement pick — a cap-gated (jutsu, Enhancement
// type) choice sourced from the character's own known jutsu, filtered by
// their Nature's Blend release element(s) (elemental_affinity.go), same
// "known-jutsu-as-catalog" shape ninjutsu_specialist.go's Refined Ninjutsu
// pioneered, with one extra dimension (the Enhancement type) alongside it.
//
// Everything else this pass deferred — Revv Up Those Friers' fried-pill
// size readout, the always-on debuff/buff payloads behind the various
// use-count gates above (including the Enhancements' own chakra-spend-to-
// apply mechanic once picked), per-Snack-ingredient effect automation, and
// any mechanic needing a subsystem this app has no shape for yet — is
// documented, not modeled; see CLASS_AUDIT.md's Cooking-Nin detail entry.
const cookingNinSlug = "class/cooking-nin"

// foodForTheSoulFeatureSlug is the base class feature ("Whenever you would
// complete a short or Long Rest, select one ability score of your
// choice...") whose pick this box records.
const foodForTheSoulFeatureSlug = "class/cooking-nin/feature/food-for-the-soul"

// Three archetype-training feats (Chef Trainee -> Chefs Expert -> Chefs
// Specialist) grant a taste of these mechanics to a character with zero
// real Cooking-Nin class levels — each feat's own prerequisites text ("You
// cannot have Levels in the Cooking-Nin Class") makes real levels and this
// feat progression mutually exclusive, same shape as Hunter-Nin's own
// archetype-training feats (see hunter_nin.go). loadCookingNinTabData's own
// level gate widens to also open for these feats; the matching Shinobi
// Snacks pool bump lives in custom_resources.go, keyed by the same feat
// slugs. Chef Trainee's own "Cooking Tool Infusion feature as though you
// were a Level 1 Cooking Nin" clause and its Cooking Tools proficiency
// clause are not modeled — no Cooking Tool Infusion mechanism exists
// anywhere in this app yet, and tool proficiencies are an established
// out-of-scope boundary.
const (
	chefTraineeFeatSlug     = "feat/class/chef-trainee"
	chefsExpertFeatSlug     = "feat/class/chefs-expert"
	chefsSpecialistFeatSlug = "feat/class/chefs-specialist"
)

// cookingArchetypeFeats reports which of Chef Trainee/Chefs Expert/Chefs
// Specialist a character holds. Specialist requires Expert as its own
// prerequisite, which requires Trainee — a character with a deeper tier is
// assumed to also carry every shallower tier's own benefits, same
// "trust the player" boundary huntersArchetypeGrants already draws for its
// own tiered archetype chain.
type cookingArchetypeFeats struct {
	Trainee, Expert, Specialist bool
}

// loadCookingArchetypeFeats scans a character's merged granted-features
// list (feats included, via mergeFeatFeatures) for the 3 archetype-training
// slugs. Named "granted", not "features", so it doesn't shadow this file's
// own internal/features package import.
func loadCookingArchetypeFeats(granted []grantedFeatureRow) cookingArchetypeFeats {
	var a cookingArchetypeFeats
	for _, f := range granted {
		switch f.Slug {
		case chefTraineeFeatSlug:
			a.Trainee = true
		case chefsExpertFeatSlug:
			a.Expert = true
		case chefsSpecialistFeatSlug:
			a.Specialist = true
		}
	}
	return a
}

// any reports whether the character holds any of the 3 archetype-training
// feats — the caller's own gating signal for widening the sheet_cooking_nin
// box's existence beyond real Cooking-Nin levels.
func (a cookingArchetypeFeats) any() bool {
	return a.Trainee || a.Expert || a.Specialist
}

// dieSize returns the flat Cooking Die size the highest archetype tier held
// grants in place of the real class's level-scaled chart (cookingDieSize)
// — "a Cooking Dice Equal to 1d4" (Chef Trainee), "Your Cooking Dice
// becomes 1d6" (Chefs Expert), "...becomes 1d8" (Chefs Specialist). ""
// if the character holds none of the three.
func (a cookingArchetypeFeats) dieSize() string {
	switch {
	case a.Specialist:
		return "1d8"
	case a.Expert:
		return "1d6"
	case a.Trainee:
		return "1d4"
	default:
		return ""
	}
}

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

// fastAndFuriousFeatureSlug is Entremetier Chef's 2nd-level feature slug —
// "you can use your intelligence or charisma instead of your dexterity for
// Initiative rolls." This is a separate, same-named constant from
// internal/charsheet's own copy — that package can't import cmd/n5e (cmd/n5e
// imports charsheet, not the reverse), so the literal is duplicated rather
// than shared, same precedent as gastrochemistNaturesBlendFeatureSlug's own
// cross-package siblings.
const fastAndFuriousFeatureSlug = "class/cooking-nin/group/cooking-focus/entremetier-chef/feature/fast-and-furious"

// fastAndFuriousOptions: freely re-editable any time, same "trust the
// player" boundary foodForTheSoulOptions draws above — the book gives no
// rest-timing restriction on this pick at all, unlike Food For the Soul's
// own per-rest re-pick. Only WHICH ability is picked is tracked; charsheet.go
// reads it back as InitiativeAbility's own default (still overridable by the
// sheet's existing Adjust-Initiative ability dropdown).
var fastAndFuriousOptions = []featureChoiceOption{
	{Value: "int", Label: "Intelligence", Description: "Use Intelligence instead of Dexterity for Initiative rolls."},
	{Value: "cha", Label: "Charisma", Description: "Use Charisma instead of Dexterity for Initiative rolls."},
}

// gastrochemistNaturesBlendFeatureSlug is Nature's Blend's own 2nd-level
// feature slug — the same slug gastrochemistAffinitySlots
// (elemental_affinity.go) keys its own "natures-blend" elemental-release
// pick to. Reused here to gate the Enhancement picker below on the
// character actually holding the Gastrochemist subclass, not merely
// Cooking-Nin levels of any subclass.
const gastrochemistNaturesBlendFeatureSlug = "class/cooking-nin/group/cooking-focus/gastrochemist/feature/natures-blend"

// gastrochemistEnhancementCap: Nature's Blend's own pick count — "choose a
// Jutsu of your Nature's Blend release, and give it one of the following
// Enhancements" at 2nd level, "You may choose an additional Jutsu to gain
// an Enhancement at 3rd, 5th, 9th, 13th, and 17th Level." No
// class_level_resources chart exists for this (Cooking-Nin's only chart row
// is "Cooking Die") — hand-curated the same way ninjutsuMasterCap is.
func gastrochemistEnhancementCap(level int) int {
	switch {
	case level >= 17:
		return 6
	case level >= 13:
		return 5
	case level >= 9:
		return 4
	case level >= 5:
		return 3
	case level >= 3:
		return 2
	case level >= 2:
		return 1
	default:
		return 0
	}
}

// gastrochemistEnhancementOptions: the 4 Enhancement types Nature's Blend
// lets a chosen Jutsu be given. Only WHICH type was chosen for a given Jutsu
// is tracked by this pick — the applied effect (spend Chakra equal to half
// the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to
// boost the Jutsu's DC/damage/healing-THP-DR/range at cast time, scaled by
// Cooking Dice and the Jutsu's Rank) stays fully Group 2/3, same boundary
// Food For the Soul's own applied Advantage effect draws above — no
// chakra-spend-at-cast-time mechanism exists anywhere in this app.
var gastrochemistEnhancementOptions = []featureChoiceOption{
	{Value: "texture", Label: "Enhance Texture", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase the Jutsu's DC by half your Cooking Dice result — capped at 1 Cooking Die for D/C-Rank Jutsu, 2 for B/A-Rank, 3 for S-Rank."},
	{Value: "kick", Label: "Enhance Kick", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase the Jutsu's damage dice by a number of Cooking Dice set by its Rank (1 for D/C, 2 for B/A, 3 for S) — or deal that much damage to a creature that fails its save if the Jutsu has no damage dice — once per casting."},
	{Value: "temperature", Label: "Enhance Temperature", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase Healing, THP, or DR the Jutsu provides by twice your Cooking Dice result times its Rank tier, or by half a Cooking Die if the Jutsu's duration is longer than instant."},
	{Value: "aroma", Label: "Enhance Aroma", Description: "Spend Chakra equal to half the Jutsu's casting cost, plus a Snack you or an ally within 5ft has, to increase the Jutsu's Range, including any Area of Effect it creates, by 5 times half your Cooking Dice result."},
}

// gastrochemistEnhancementLabel resolves a stored enhancement_type value
// back to its display label, for the Known list.
func gastrochemistEnhancementLabel(value string) string {
	for _, o := range gastrochemistEnhancementOptions {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

// knownBlendEnhancementPick is one entry on the Nature's Blend Enhancement
// Known list — a known jutsu (see knownJutsuOption, ninjutsu_specialist.go)
// paired with which of the 4 Enhancement types it was given.
type knownBlendEnhancementPick struct {
	JutsuID          int64
	Name             string
	Rank             string
	EnhancementType  string
	EnhancementLabel string
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

// loadKnownBlendJutsu reads every jutsu the character currently knows whose
// keywords name at least one of the given elements (the character's own
// Nature's Blend release element(s), from the "natures-blend"/
// "many-colored-blend" affinity picks — elemental_affinity.go), for the
// Gastrochemist's own Enhancement picker to filter and offer. Same "resolve
// a published jutsu_slug or a player-created custom_jutsu, tolerate a stale
// slug" shape as loadKnownNinjutsu (ninjutsu_specialist.go), just filtered
// by element via jutsuElements instead of by classification == "Ninjutsu".
func (s *server) loadKnownBlendJutsu(characterID int64, blendElements map[string]bool) ([]knownJutsuOption, error) {
	if len(blendElements) == 0 {
		return nil, nil
	}
	rows, err := s.charDB.Query(`
		SELECT id, jutsu_slug, custom_jutsu_id FROM character_jutsu WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id        int64
		jutsuSlug sql.NullString
		customID  sql.NullInt64
	}
	var stored []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.jutsuSlug, &r.customID); err != nil {
			rows.Close()
			return nil, err
		}
		stored = append(stored, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []knownJutsuOption
	for _, r := range stored {
		var name, rank, keywords, description string
		if r.jutsuSlug.Valid {
			err := s.rulesDB.QueryRow(
				`SELECT name, rank, keywords, description FROM v_jutsu WHERE slug = ?`, r.jutsuSlug.String,
			).Scan(&name, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue // stale slug (rules update) — skip rather than break the picker
			}
			if err != nil {
				return nil, err
			}
		} else if r.customID.Valid {
			err := s.charDB.QueryRow(
				`SELECT name, rank, keywords, description FROM custom_jutsu WHERE id = ?`, r.customID.Int64,
			).Scan(&name, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else {
			continue
		}
		matched := false
		for _, el := range jutsuElements(keywords) {
			if blendElements[el] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		out = append(out, knownJutsuOption{JutsuID: r.id, Name: name, Rank: rank, Description: description})
	}
	return out, nil
}

// foodForTheSoulView holds Food For the Soul's current pick — nil unless
// the character has reached 2nd level in Cooking-Nin.
type foodForTheSoulView struct {
	Current string // ability key ("str".."cha"), "" if unpicked
	Options []featureChoiceOption
}

// fastAndFuriousView holds Fast and Furious's current pick — nil unless the
// character holds Entremetier Chef's own 2nd-level feature.
type fastAndFuriousView struct {
	Current string // "int" or "cha", "" if unpicked
	Options []featureChoiceOption
}

// cookingNinTabData is the sheet_cooking_nin box's full data — nil for a
// character with no Cooking-Nin levels, same "no empty state" treatment
// HunterTechniques/MartialTechniques already establish.
type cookingNinTabData struct {
	CookingDieSize string
	FoodForTheSoul *foodForTheSoulView
	FastAndFurious *fastAndFuriousView

	// Gastrochemist's Nature's Blend Enhancement pick (2nd level, cap
	// 1->6 across 2nd/3rd/5th/9th/13th/17th level) — only rendered when
	// BlendEnhancementCap > 0, i.e. the character actually holds the
	// Gastrochemist subclass.
	BlendEnhancementCap     int
	BlendEnhancementUsed    int
	KnownBlendEnhancements  []knownBlendEnhancementPick
	AvailableBlendJutsu     []knownJutsuOption
	BlendEnhancementOptions []featureChoiceOption
}

// loadCookingNinTabData returns nil for a character with no Cooking-Nin
// levels and none of the 3 archetype-training feats — the template gates
// the whole box's existence on this being non-nil, same treatment
// HunterTechniques gets for its own archetype feats.
func (s *server) loadCookingNinTabData(characterID int64, sheet *charsheet.Sheet) (*cookingNinTabData, error) {
	level, err := s.cookingNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	arch := loadCookingArchetypeFeats(grantedFeatures)
	if level == 0 && !arch.any() {
		return nil, nil
	}
	var dieSize string
	if level > 0 {
		dieSize, err = s.cookingDieSize(level)
		if err != nil {
			return nil, err
		}
	} else {
		// Real levels and the archetype feats are mutually exclusive by the
		// feats' own prerequisites ("You cannot have Levels in the
		// Cooking-Nin Class"), so this only ever fills in an empty
		// real-level readout, never overrides a real chart value.
		dieSize = arch.dieSize()
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

	if hasGrantedFeature(grantedFeatures, fastAndFuriousFeatureSlug) {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		data.FastAndFurious = &fastAndFuriousView{
			Current: choices[features.ChoiceKey{FeatureSlug: fastAndFuriousFeatureSlug, ChoiceIndex: 0}],
			Options: fastAndFuriousOptions,
		}
	}

	if hasGrantedFeature(grantedFeatures, gastrochemistNaturesBlendFeatureSlug) {
		data.BlendEnhancementCap = gastrochemistEnhancementCap(level)
		if data.BlendEnhancementCap > 0 {
			affinityPicks, err := charstore.ListElementalAffinities(s.charDB, characterID)
			if err != nil {
				return nil, err
			}
			blendElements := map[string]bool{}
			if el := affinityPicks["natures-blend"]; el != "" {
				blendElements[el] = true
			}
			if el := affinityPicks["many-colored-blend"]; el != "" {
				blendElements[el] = true
			}
			known, err := s.loadKnownBlendJutsu(characterID, blendElements)
			if err != nil {
				return nil, err
			}
			enhancementPicks, err := charstore.ListCookingNinBlendEnhancementPicks(s.charDB, characterID)
			if err != nil {
				return nil, err
			}
			pickedByJutsu := make(map[int64]string, len(enhancementPicks))
			for _, p := range enhancementPicks {
				pickedByJutsu[p.JutsuID] = p.EnhancementType
			}
			data.BlendEnhancementUsed = len(enhancementPicks)
			data.BlendEnhancementOptions = gastrochemistEnhancementOptions
			for _, o := range known {
				if enhType, ok := pickedByJutsu[o.JutsuID]; ok {
					data.KnownBlendEnhancements = append(data.KnownBlendEnhancements, knownBlendEnhancementPick{
						JutsuID:          o.JutsuID,
						Name:             o.Name,
						Rank:             o.Rank,
						EnhancementType:  enhType,
						EnhancementLabel: gastrochemistEnhancementLabel(enhType),
					})
				} else {
					data.AvailableBlendJutsu = append(data.AvailableBlendJutsu, o)
				}
			}
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

// handleFastAndFurious records Fast and Furious's re-selectable Initiative
// ability pick (Intelligence or Charisma in place of Dexterity) — only the
// pick is tracked; internal/charsheet.Compute reads it back as
// InitiativeAbility's own default (still overridable by the sheet's
// existing Adjust-Initiative dropdown, same override-beats-default shape
// every other Initiative source on this sheet already follows).
func (s *server) handleFastAndFurious(w http.ResponseWriter, r *http.Request) {
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
	for _, o := range fastAndFuriousOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for fast and furious:", err)
		return
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load granted features for fast and furious:", err)
		return
	}
	if !hasGrantedFeature(grantedFeatures, fastAndFuriousFeatureSlug) {
		http.Error(w, "character does not have Fast and Furious", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, fastAndFuriousFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set fast and furious:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingNinBlendEnhancementAdd records one Nature's Blend Enhancement
// pick (form fields "jutsu_id", "enhancement_type") — a known jutsu of the
// character's own Nature's Blend release element(s) paired with one of the
// 4 Enhancement types. Only the PICK is tracked; the Enhancement's actual
// chakra-plus-Snack-spend-to-apply mechanic stays fully Group 2/3 — see
// gastrochemistEnhancementOptions' own comment.
func (s *server) handleCookingNinBlendEnhancementAdd(w http.ResponseWriter, r *http.Request) {
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
	valid := false
	for _, o := range gastrochemistEnhancementOptions {
		if o.Value == enhancementType {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid Enhancement type", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for blend enhancement add:", err)
		return
	}
	data, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin for blend enhancement add:", err)
		return
	}
	if data == nil || data.BlendEnhancementUsed >= data.BlendEnhancementCap {
		http.Error(w, "no Enhancement slots remaining", http.StatusBadRequest)
		return
	}
	jutsuValid := false
	for _, o := range data.AvailableBlendJutsu {
		if o.JutsuID == jutsuID {
			jutsuValid = true
			break
		}
	}
	if !jutsuValid {
		http.Error(w, "not a valid jutsu to enhance", http.StatusBadRequest)
		return
	}
	if err := charstore.AddCookingNinBlendEnhancementPick(s.charDB, id, jutsuID, enhancementType); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add cooking-nin blend enhancement pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}

// handleCookingNinBlendEnhancementDelete drops one Enhancement pick —
// freely, at any time, same "trust the player" boundary every other pick
// removal on this sheet draws.
func (s *server) handleCookingNinBlendEnhancementDelete(w http.ResponseWriter, r *http.Request) {
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
		log.Println("remove cooking-nin blend enhancement pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_cooking_nin")
}
