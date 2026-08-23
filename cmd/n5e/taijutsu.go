package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Taijutsu Specialist's Martial Dice economy: Martial Adept (class_features,
// level 2) grants a pool of Martial Dice that resets at the start of every
// turn (not a rest-tier resource, so it deliberately does NOT go through
// customResourceGrants — see that file's own doc comment) and a cap on how
// many Martial Techniques (class_options, list_name "Martial Techniques") a
// character can know at once. All 20 Martial Techniques rows ingested so
// far are class-wide (subclass_slug IS NULL), not tied to any one Taijutsu
// Style. loadMartialTechniquesTabData's own query also matches a specific
// subclass_slug, so a genuinely per-Style entry is still picked up if one
// is ever ingested. Two Talent & Focus
// subclass features layer curated overlays on top of both: Unnatural
// Talent (+1/+2/+3 Martial Dice at 3rd/10th/17th Taijutsu level — the
// "Unnatural Talent Martial Dice Chart" text is misfiled at the tail of
// Focused Technique's L20 description in the book, not Unnatural Talent's
// own row) and Talented Legacy (+1 known-technique cap at 14th, +1 more at
// 20th).
//
// Everything else in this class is Group 2 (activated/conditional,
// spend-a-die-for-a-temporary-effect) in the same classification
// internal/puppetupgrades established — see CLASS_AUDIT.md's Taijutsu
// Specialist detail entry for the full per-subclass breakdown of what's
// deliberately not automated here and why.
const taijutsuSpecialistSlug = "class/taijutsu-specialist"

const (
	unarmedTechniqueFeatureSlug     = "class/taijutsu-specialist/feature/unarmed-technique"
	unnaturalTalentFeatureSlug      = "class/taijutsu-specialist/group/taijutsu-style/talent-focus/feature/unnatural-talent"
	talentedLegacyFeatureSlug       = "class/taijutsu-specialist/group/taijutsu-style/talent-focus/feature/talented-legacy"
	fistsOfIronFeatureSlug          = "class/taijutsu-specialist/group/taijutsu-style/passionate-flame/feature/fists-of-iron"
	handWrapsOfPassionFeatureSlug   = "class/taijutsu-specialist/group/taijutsu-style/passionate-flame/feature/hand-wraps-of-passion"
	antiChakraWavelengthFeatureSlug = "class/taijutsu-specialist/group/taijutsu-style/ruin/feature/anti-chakra-wavelength"
	mixedMartialArtsFeatureSlug     = "class/taijutsu-specialist/group/taijutsu-style/stancer/feature/mixed-martial-arts"
	stanceBlendingFeatureSlug       = "class/taijutsu-specialist/group/taijutsu-style/stancer/feature/stance-blending"
	passionateFlameSubclassSlug     = "class/taijutsu-specialist/group/taijutsu-style/passionate-flame"
	ruinSubclassSlug                = "class/taijutsu-specialist/group/taijutsu-style/ruin"
	stancerSubclassSlug             = "class/taijutsu-specialist/group/taijutsu-style/stancer"

	martialDiceResourceKey = "martial_dice"
)

// The 4 archetype-training feats below give a non-Taijutsu-Specialist
// character a taste of the class's own mechanics with no real class level:
// Martial Arts Training grants a base Martial Dice/Technique and a Fighting
// Stance pick (mutually exclusive with real levels — its own prerequisite
// reads "You cannot have class levels in Taijutsu Specialist"); Expert and
// Specialist each grant "1 additional" Martial Dice and Martial Technique on
// top of whatever came before; Enthusiast grants a chosen Style's 3rd-level
// Martial Techniques (see martialTechniqueAutoGrants below — not modeled
// here, see loadMartialTechniquesTabData's own doc comment for why).
const (
	martialArtsTrainingFeatSlug   = "feat/class/martial-arts-training"
	martialArtsExpertFeatSlug     = "feat/class/martial-arts-expert"
	martialArtsEnthusiastFeatSlug = "feat/class/martial-arts-enthusiast"
	martialArtsSpecialistFeatSlug = "feat/class/martial-arts-specialist"
)

// taijutsuArchetypeFeatSlugs answers "does this character have a taste of
// Taijutsu Specialist mechanics at all" for a character with no real class
// level — every loader/handler below that currently gates on taijutsuLevel
// != 0 also checks this set before deciding to hide its whole section.
var taijutsuArchetypeFeatSlugs = map[string]bool{
	martialArtsTrainingFeatSlug:   true,
	martialArtsExpertFeatSlug:     true,
	martialArtsEnthusiastFeatSlug: true,
	martialArtsSpecialistFeatSlug: true,
}

// martialTechniqueAutoGrants: feature slug -> the bespoke Martial Technique
// names it grants for free. Every one of the 8 Taijutsu Style subclasses
// has a 3rd-level feature reading "you learn the following two Martial
// Techniques" (Passionate Flame's own Enhanced Flurry included) — these 16
// techniques have no class_options catalog row of their own; their full
// text already renders wherever the granting feature's own description is
// shown (the Features & Traits panel), so this map only needs names, for
// marking them Granted on the Known Martial Techniques list: auto-known,
// don't count against the cap, no delete button — same SourceLabel
// boundary loadGrantedJutsuLabels already established for free jutsu.
var martialTechniqueAutoGrants = map[string][]string{
	"class/taijutsu-specialist/group/taijutsu-style/disturbance/feature/blinding-speed":       {"Shatter", "Unstable Core"},
	"class/taijutsu-specialist/group/taijutsu-style/ironclad/feature/iron-combat":             {"Interpose", "Shield Bash"},
	"class/taijutsu-specialist/group/taijutsu-style/nin-tai/feature/elemental-combat":         {"Elemental Rush", "Elemental Crush"},
	"class/taijutsu-specialist/group/taijutsu-style/passionate-flame/feature/enhanced-flurry": {"Chakra Enhanced Blows", "Dynamic Set-Up"},
	"class/taijutsu-specialist/group/taijutsu-style/righteous-fury/feature/frenzied-assault":  {"Savage Force", "Tyrants Savagery"},
	"class/taijutsu-specialist/group/taijutsu-style/ruin/feature/anti-chakra-wavelength":      {"Chakra Break", "Focus Break"},
	"class/taijutsu-specialist/group/taijutsu-style/stancer/feature/combo-breaker":            {"Alpha Counter", "Dodge Cancel"},
	"class/taijutsu-specialist/group/taijutsu-style/talent-focus/feature/focused-talent":      {"Redirected Aggression", "Shatterpoint"},
}

// handWrapsOfPassionOptions: Passionate Flame's Hand Wraps of Passion
// choice. RAW gates re-picking to a Full Rest; this app doesn't enforce
// rest timing, same "trust the player" boundary Mastery already draws —
// freely re-editable any time.
var handWrapsOfPassionOptions = []featureChoiceOption{
	{Value: "mighty-blows", Label: "Mighty Blows", Description: "Taijutsu attacks you make that deal [Unarmed Damage] ignores resistance and treat immunity as resistance. Beginning at 14th level when you deal [Unarmed Damage] twice to the same creature on your turn, they are unable to take Reactions against you until the beginning of your next turn."},
	{Value: "mighty-guards", Label: "Mighty Guards", Description: "You gain damage reduction equal to half of your Proficiency Bonus. Beginning at 14th level that increases to your full proficiency."},
	{Value: "mighty-mobility", Label: "Mighty Mobility", Description: "You cannot have your speed reduced by any means, and you ignore difficult terrain. Beginning at 14th level, you gain immunity to the Slowed and Grappled conditions."},
}

// antiChakraWavelengthKeywords: Ruin's Anti-Chakra Wavelength lets the
// player choose 2 keywords from this fixed 10-item list, freely
// re-editable (same boundary as Hand Wraps of Passion above).
var antiChakraWavelengthKeywords = []string{
	"Sensory", "Fuinjutsu", "Earth Release", "Wind Release", "Fire Release",
	"Water Release", "Lightning Release", "Tactical", "Visual", "Auditory",
}

// taijutsuSpecialistClassLevel returns the character's own Taijutsu
// Specialist class level, or 0 if they have none — mirrors
// puppetMasterClassLevel.
func (s *server) taijutsuSpecialistClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, taijutsuSpecialistSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// taijutsuSpecialistSubclassSlug resolves the character's own chosen
// Taijutsu Style, if any — mirrors puppetMasterSubclassSlug exactly (a
// character can have subclasses from more than one class under
// multiclassing, so this scans character_subclasses for the one whose
// parent class is Taijutsu Specialist rather than assuming there's only
// one row).
func (s *server) taijutsuSpecialistSubclassSlug(characterID int64) (slug, name string, err error) {
	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return "", "", err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var sc string
		if err := subRows.Scan(&sc); err != nil {
			subRows.Close()
			return "", "", err
		}
		subclassSlugs = append(subclassSlugs, sc)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return "", "", err
	}

	for _, sc := range subclassSlugs {
		var n, classSlug string
		if err := s.rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, sc,
		).Scan(&n, &classSlug); err != nil {
			continue // a stale/removed subclass slug just isn't a match
		}
		if classSlug == taijutsuSpecialistSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// classLevelResourceInt reads one v_class_level_resources cell as an int —
// same lookup shape puppetUpgradeSlotCap already uses for Puppet Master's
// own level-gated tables.
func (s *server) classLevelResourceInt(classSlug, resourceName string, level int) (int, error) {
	var valueText sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = ?`,
		classSlug, level, resourceName,
	).Scan(&valueText)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(valueText.String))
	if convErr != nil {
		return 0, nil
	}
	return n, nil
}

func hasGrantedFeature(rows []grantedFeatureRow, slug string) bool {
	for _, f := range rows {
		if f.Slug == slug {
			return true
		}
	}
	return false
}

// hasAnyGrantedFeature is hasGrantedFeature generalized to a set of slugs —
// used to gate a whole section on "any one of these feats/features".
func hasAnyGrantedFeature(rows []grantedFeatureRow, slugs map[string]bool) bool {
	for _, f := range rows {
		if slugs[f.Slug] {
			return true
		}
	}
	return false
}

// martialDiceFeatGrants: archetype-training feat slug -> the flat Martial
// Dice pool bonus its own text grants, independent of (and additional to)
// the level-based class_level_resources chart martialDiceMax already reads.
// Martial Arts Training grants the base 1 die; Expert and Specialist each
// grant "1 additional" — modeled as three independent flat additions rather
// than a running total, since nothing here assumes a character was granted
// every earlier feat in the chain too. Martial Arts Enthusiast grants no
// Martial Dice of its own.
var martialDiceFeatGrants = map[string]int{
	martialArtsTrainingFeatSlug:   1,
	martialArtsExpertFeatSlug:     1,
	martialArtsSpecialistFeatSlug: 1,
}

// martialTechniqueCapFeatGrants: feat slug -> the flat Martial Technique cap
// bonus its own text grants ("1 Martial Technique of your choice" / "learn
// one Martial Technique" / "1 additional Martial Technique"). Covers both
// the archetype-training feats (which apply even at taijutsuLevel 0) and
// Combo Expert / Martial Master (real Taijutsu Specialist levels required by
// their own prerequisites, 4+ and 8+ respectively) — each of the latter two
// also carries its own per-technique clause (a die-cost removal for Combo
// Expert, a two-step die-size bump for Martial Master) that has no field to
// flow into and is left as reference text only. loadMartialTechniquesTabData's
// own class_options query already matches on class_slug alone (independent
// of subclass or real class level), so raising the cap is enough to let a
// character pick and store a technique through the existing Available/Known
// Add/Delete handlers — no new picker UI needed.
var martialTechniqueCapFeatGrants = map[string]int{
	martialArtsTrainingFeatSlug:   1,
	martialArtsExpertFeatSlug:     1,
	martialArtsSpecialistFeatSlug: 1,
	"feat/class/combo-expert":     1,
	"feat/class/martial-master":   1,
}

// taijutsuStanceFeatureSlug picks which ChoiceKey a character's Fighting
// Stance pick is stored under: a real Taijutsu Specialist uses Unarmed
// Technique's own slug (existing behavior, unchanged); a character with no
// real levels uses Martial Arts Training's own slug instead, so the two
// grants — mutually exclusive under RAW, see taijutsuArchetypeFeatSlugs'
// doc comment — never collide under one shared key, matching
// buildFightingStanceView's own "each caller passes its own featureSlug"
// convention.
func taijutsuStanceFeatureSlug(taijutsuLevel int) string {
	if taijutsuLevel > 0 {
		return unarmedTechniqueFeatureSlug
	}
	return martialArtsTrainingFeatSlug
}

// martialDiceMax computes a character's Martial Dice pool size: the base
// class_level_resources chart ("Martial Die": 1 at 2nd-5th, 2 at 6th-9th, 3
// at 10th-13th, 4 at 14th-18th, 5 at 19th-20th) plus Talent & Focus's
// Unnatural Talent overlay, plus any archetype-training feat's own flat
// bonus (martialDiceFeatGrants) — the latter applies even at taijutsuLevel
// 0, for a character with no real Taijutsu Specialist levels at all.
func (s *server) martialDiceMax(grantedFeatures []grantedFeatureRow, taijutsuLevel int) (int, error) {
	base, err := s.classLevelResourceInt(taijutsuSpecialistSlug, "Martial Die", taijutsuLevel)
	if err != nil {
		return 0, err
	}
	if hasGrantedFeature(grantedFeatures, unnaturalTalentFeatureSlug) {
		switch {
		case taijutsuLevel >= 17:
			base += 3
		case taijutsuLevel >= 10:
			base += 2
		case taijutsuLevel >= 3:
			base += 1
		}
	}
	for slug, bonus := range martialDiceFeatGrants {
		if hasGrantedFeature(grantedFeatures, slug) {
			base += bonus
		}
	}
	return base, nil
}

// martialDieSize is the die a player rolls when spending a Martial Die —
// distinct from the pool COUNT above. Martial Adept's own text: d4 base,
// d6 at 9th level, d8 at 17th.
func martialDieSize(taijutsuLevel int) string {
	switch {
	case taijutsuLevel >= 17:
		return "d8"
	case taijutsuLevel >= 9:
		return "d6"
	default:
		return "d4"
	}
}

// stanceUnarmedDieGrant pairs the Fighting Stance a die-size override
// requires with the die size it grants while that stance is the character's
// current pick.
type stanceUnarmedDieGrant struct {
	StanceSlug string
	DieSize    string
}

// stanceUnarmedDieGrants covers the nine "X Fist Expert" archetype feats,
// each reading "while you are in the [X] Fist Stance, your [Unarmed Damage]
// die becomes a d8" — verified against rules.db's own feats table
// description text (sqlite3 dist/rules.db "SELECT slug, description FROM
// feats WHERE slug LIKE '%-fist-expert'"), not assumed from the naming
// pattern. Each feat's own prerequisite only requires already having the
// matching Stance, not any Taijutsu Specialist class level, so this table is
// checked independent of taijutsuLevel. Iron Fist Expert and Silent Fist
// Expert further escalate to a d10 conditioned on also owning a specific
// piece of equipment (Combat Bracers / Iron Claws respectively) while a
// matching ability score is the character's Taijutsu ability — that
// equipment-and-ability check has no mechanism here to hook into, so only
// each feat's unconditional (stance-only) d8 clause is modeled.
var stanceUnarmedDieGrants = map[string]stanceUnarmedDieGrant{
	"feat/dragon-fist-expert":  {StanceSlug: "stance/dragon-fist-stance", DieSize: "d8"},
	"feat/drunken-fist-expert": {StanceSlug: "stance/drunken-fist-stance", DieSize: "d8"},
	"feat/frog-fist-expert":    {StanceSlug: "stance/frog-fist-stance", DieSize: "d8"},
	"feat/iron-fist-expert":    {StanceSlug: "stance/iron-fist-stance", DieSize: "d8"},
	"feat/lion-fist-expert":    {StanceSlug: "stance/lion-fist-stance", DieSize: "d8"},
	"feat/rabbit-fist-expert":  {StanceSlug: "stance/rabbit-fist-stance", DieSize: "d8"},
	"feat/serpent-fist-expert": {StanceSlug: "stance/serpent-fist-stance", DieSize: "d8"},
	"feat/silent-fist-expert":  {StanceSlug: "stance/silent-fist-stance", DieSize: "d8"},
	"feat/wolf-fist-expert":    {StanceSlug: "stance/wolf-fist-stance", DieSize: "d8"},
}

// dieSizeValue parses a "dN" die-size string into N, for comparing which of
// two die sizes is larger. Returns 0 for "" or anything unparseable, which
// always loses a comparison against a real die.
func dieSizeValue(die string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(die, "d"))
	if err != nil {
		return 0
	}
	return n
}

// unarmedDamageDieSize is Unarmed Technique's own die progression (base
// class feature, level 1, every Taijutsu Specialist): "you can roll a d6 in
// place of the normal damage of your unarmed strike or Weapons that you are
// proficient with. This increases to a d8 at 6th level and a d10 at 11th
// level." Unlike Martial Dice's chart, this one is printed correctly in the
// feature's own description (not misfiled elsewhere). Rendered as a
// computed informational line only, same boundary Fists of Iron's bonus
// dice already draw — the [Unarmed Damage] bracket token appears in over a
// hundred jutsu descriptions' prose, and no bonus-damage-composition
// mechanism exists anywhere in the attack tables to wire a computed value
// into; building one for this single feature would be scope creep.
//
// currentStanceSlug (the character's own current Fighting Stance pick, from
// fightingStanceView.Current) is checked against stanceUnarmedDieGrants —
// whichever of the level-based progression or a matching stance grant is
// LARGER wins, since a stance grant is a floor a high-level character's own
// progression can already exceed (e.g. an 11th-level character's d10
// already beats any fist-expert feat's d8). Returns "" — hidden by the
// template's own {{if}} guard — when taijutsuLevel is 0 and no stance grant
// applies: a feat-only character with, say, only Martial Arts Training has
// no Unarmed Damage die at all until a stance-conditional feat like Dragon
// Fist Expert grants one.
func unarmedDamageDieSize(taijutsuLevel int, grantedFeatures []grantedFeatureRow, currentStanceSlug string) string {
	die := ""
	switch {
	case taijutsuLevel >= 11:
		die = "d10"
	case taijutsuLevel >= 6:
		die = "d8"
	case taijutsuLevel >= 1:
		die = "d6"
	}
	for _, f := range grantedFeatures {
		grant, ok := stanceUnarmedDieGrants[f.Slug]
		if !ok || grant.StanceSlug != currentStanceSlug {
			continue
		}
		if dieSizeValue(grant.DieSize) > dieSizeValue(die) {
			die = grant.DieSize
		}
	}
	return die
}

// martialTechniqueCap computes how many Martial Techniques a character can
// know at once: the base class_level_resources chart ("Martial
// Techniques": 4 -> 5 at 7th -> 6 at 13th -> 7 at 19th) plus Talent &
// Focus's Talented Legacy overlay (+1 at 14th, +1 more — total +2 — at
// 20th), plus any archetype-training feat's own flat bonus
// (martialTechniqueCapFeatGrants) — the latter applies even at
// taijutsuLevel 0.
func (s *server) martialTechniqueCap(grantedFeatures []grantedFeatureRow, taijutsuLevel int) (int, error) {
	base, err := s.classLevelResourceInt(taijutsuSpecialistSlug, "Martial Techniques", taijutsuLevel)
	if err != nil {
		return 0, err
	}
	if hasGrantedFeature(grantedFeatures, talentedLegacyFeatureSlug) {
		switch {
		case taijutsuLevel >= 20:
			base += 2
		case taijutsuLevel >= 14:
			base += 1
		}
	}
	for slug, bonus := range martialTechniqueCapFeatGrants {
		if hasGrantedFeature(grantedFeatures, slug) {
			base += bonus
		}
	}
	return base, nil
}

// fistsOfIronBonusDice is Passionate Flame's Fists of Iron bonus-damage
// scale (in Martial Dice): 1 at 3rd-8th, 2 at 9th-16th, 3 at 17th-20th. The
// "FIST OF IRON BONUS DAMAGE CHART" text is misfiled at the tail of Flaming
// Finishers' (L9) description in the book, not Fists of Iron's own row —
// confirmed against the shipped rules.db, not a copy error here.
func fistsOfIronBonusDice(taijutsuLevel int) int {
	switch {
	case taijutsuLevel >= 17:
		return 3
	case taijutsuLevel >= 9:
		return 2
	default:
		return 1
	}
}

// martialDiceView is the sheet_vitals tile's data — see
// CustomResourceEntry for the shape it deliberately mirrors, minus the
// rest-regen fields (Martial Dice resets manually via "New Turn", not on a
// rest tier).
type martialDiceView struct {
	Current int
	Max     int
	DieSize string
}

// loadMartialDice returns nil for a character with no Taijutsu Specialist
// levels AND none of the archetype-training feats (taijutsuArchetypeFeatSlugs)
// — the template gates the whole tile on this being non-nil, same "real DOM
// removal" treatment CustomResources/PuppetTactics already get.
func (s *server) loadMartialDice(characterID int64, sheet *charsheet.Sheet) (*martialDiceView, error) {
	taijutsuLevel, err := s.taijutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	if taijutsuLevel == 0 && !hasAnyGrantedFeature(grantedFeatures, taijutsuArchetypeFeatSlugs) {
		return nil, nil
	}
	max, err := s.martialDiceMax(grantedFeatures, taijutsuLevel)
	if err != nil {
		return nil, err
	}
	stored, err := charstore.GetCustomResources(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	current, ok := stored[martialDiceResourceKey]
	if !ok || current > max {
		current = max
	}
	if current < 0 {
		current = 0
	}
	return &martialDiceView{Current: current, Max: max, DieSize: martialDieSize(taijutsuLevel)}, nil
}

// handleSheetMartialDice adjusts the Martial Dice pool by a signed delta —
// same click-to-edit-box pattern as HP/Chakra/every custom resource.
func (s *server) handleSheetMartialDice(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	delta, err := strconv.Atoi(strings.TrimSpace(r.FormValue("delta")))
	if err != nil {
		http.Error(w, "bad delta", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for martial dice:", err)
		return
	}
	dice, err := s.loadMartialDice(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial dice:", err)
		return
	}
	if dice == nil {
		http.Error(w, "character has no Taijutsu Specialist mechanics", http.StatusBadRequest)
		return
	}
	newValue := dice.Current + delta
	if newValue < 0 {
		newValue = 0
	}
	if newValue > dice.Max {
		newValue = dice.Max
	}
	if err := charstore.SetCustomResourceValue(s.charDB, id, martialDiceResourceKey, newValue); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set martial dice:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetMartialDiceNewTurn resets the pool to its max — Martial
// Adept's own text: "at the beginning of each of your turns you manifest a
// pool of martial die" (unspent dice don't carry over). A manual button,
// since this app has no combat/initiative tracker to fire this
// automatically.
func (s *server) handleSheetMartialDiceNewTurn(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for martial dice new turn:", err)
		return
	}
	dice, err := s.loadMartialDice(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial dice for new turn:", err)
		return
	}
	if dice == nil {
		http.Error(w, "character has no Taijutsu Specialist mechanics", http.StatusBadRequest)
		return
	}
	if err := charstore.SetCustomResourceValue(s.charDB, id, martialDiceResourceKey, dice.Max); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("reset martial dice:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// martialTechniqueOption is one not-yet-known class-wide Martial Technique,
// offered in the picker.
type martialTechniqueOption struct {
	Slug        string
	Name        string
	Description string
}

// knownMartialTechnique is one entry on the Known list — either a player
// pick (Slug set, has a remove button, counts against the cap) or an
// auto-granted bespoke technique (Granted true, no Slug, no remove button,
// doesn't count against the cap). Description is always populated (the
// class_options row's own text for a player pick; the granting feature's
// own description — which already contains the bespoke technique's full
// text — for a Granted one) so the Known list can carry the same
// .tooltip/.tooltip-content the Available list already has instead of
// going bare once learned.
type knownMartialTechnique struct {
	Slug        string
	Name        string
	Granted     bool
	Description string
}

// passionateFlameView holds Passionate Flame's own sub-section of the
// Martial Techniques box — nil unless the character's chosen Taijutsu
// Style is Passionate Flame.
type passionateFlameView struct {
	FistsOfIronDice  int
	HandWraps        string // current pick value, "" if unpicked
	HandWrapsLabel   string
	HandWrapsOptions []featureChoiceOption
}

// ruinAntiChakraView holds Ruin's own sub-section — nil unless the
// character's chosen Taijutsu Style is Ruin. Keyword1/Keyword2 (rather than
// a slice) so the template can bind each <select> to its own value without
// an out-of-range risk from indexing a short slice.
type ruinAntiChakraView struct {
	Keyword1, Keyword2 string // "" if unset
	Options            []string
}

// stanceOption is one Taijutsu Stance offered in the picker (Chapter 13
// reference data — class-agnostic in fighting_stances, filtered here to
// stance_type='taijutsu' since Unarmed Technique only grants a choice among
// those, not the Bukijutsu stances).
type stanceOption struct {
	Slug        string
	Name        string
	Description string
}

// fightingStanceView holds Unarmed Technique's own stance pick — every
// Taijutsu Specialist has one (base class feature, level 1, not subclass-
// gated), freely re-editable same as Hand Wraps of Passion/Anti-Chakra
// Wavelength (RAW doesn't gate this pick to a rest either).
type fightingStanceView struct {
	Current            string // stance slug, "" if unpicked
	CurrentName        string
	CurrentDescription string
	Options            []stanceOption
}

// martialTechniquesTabData is the sheet_martial_techniques box's full data.
type martialTechniquesTabData struct {
	Cap                     int
	Used                    int
	Known                   []knownMartialTechnique
	Available               []martialTechniqueOption
	UnarmedDamageDie        string
	Stance                  *fightingStanceView
	PassionateFlame         *passionateFlameView
	Ruin                    *ruinAntiChakraView
	StancerMixedMartialArts *fightingStanceView
	StancerStanceBlending   *fightingStanceView
}

// buildFightingStanceView assembles one granting feature's own Taijutsu
// Stance pick view from the character's stored feature choices — shared by
// Taijutsu Specialist's Unarmed Technique, the Martial Arts Training feat
// (taijutsuStanceFeatureSlug picks whichever of the two applies), and Combat
// Medic's Martial Competency (medical_nin.go), since a character with more
// than one of these grants gets INDEPENDENT stance picks (Martial
// Competency's own text: "You can't take a stance more than once even if
// you gain a stance choice again" implies exactly this — a second grant is
// a second, separate choice), not one shared pick. Each caller passes its
// own featureSlug so the picks are stored under different ChoiceKeys.
func buildFightingStanceView(choices map[features.ChoiceKey]string, options []stanceOption, featureSlug string) *fightingStanceView {
	stance := &fightingStanceView{
		Current: choices[features.ChoiceKey{FeatureSlug: featureSlug, ChoiceIndex: 0}],
		Options: options,
	}
	for _, o := range options {
		if o.Slug == stance.Current {
			stance.CurrentName = o.Name
			stance.CurrentDescription = o.Description
		}
	}
	return stance
}

// errInvalidStance signals storeFightingStanceChoice's slug wasn't found in
// the options catalog it was given — distinguished from a database error so
// callers can answer with the right HTTP status.
var errInvalidStance = errors.New("not a valid stance")

// storeFightingStanceChoice validates slug against the given options
// catalog and records it under featureSlug's own ChoiceIndex 0 — shared by
// every feature that grants a Fighting Stance pick (Unarmed Technique below
// and Martial Competency in medical_nin.go's handleMedicalNinFightingStance,
// both Taijutsu-Stance-only; Scout-Nin's Fighting Stance in scout_nin.go,
// which offers both Taijutsu and Weapon Stances) so the validation logic
// lives in exactly one place instead of being duplicated per granting
// feature. options is a parameter rather than always re-deriving it from
// loadTaijutsuStanceOptions so a caller can pass a wider stance_type filter
// without a second, duplicated validate-and-store function.
func (s *server) storeFightingStanceChoice(characterID int64, featureSlug string, options []stanceOption, slug string) error {
	valid := false
	for _, o := range options {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		return errInvalidStance
	}
	return charstore.SetFeatureChoice(s.charDB, characterID, featureSlug, 0, slug)
}

// loadStanceOptionsByType catalogs Fighting Stances from fighting_stances
// (Chapter 13 reference data, class-agnostic) filtered to one or more
// stance_type values — generalizes what used to be loadTaijutsuStanceOptions's
// own hardcoded single-type query so Scout-Nin's Fighting Stance (which
// offers both Taijutsu Stance and Weapon Stance, per its own text: "between
// Taijutsu Stance and Weapon Stance") can reuse the same loader instead of a
// third, near-duplicate query.
func (s *server) loadStanceOptionsByType(stanceTypes ...string) ([]stanceOption, error) {
	placeholders := make([]string, len(stanceTypes))
	args := make([]any, len(stanceTypes))
	for i, t := range stanceTypes {
		placeholders[i] = "?"
		args[i] = t
	}
	rows, err := s.rulesDB.Query(fmt.Sprintf(`
		SELECT slug, name, description FROM fighting_stances
		WHERE stance_type IN (%s) ORDER BY name`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []stanceOption
	for rows.Next() {
		var o stanceOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		options = append(options, o)
	}
	return options, rows.Err()
}

// loadTaijutsuStanceOptions catalogs the 9 Taijutsu Stances only — Taijutsu
// Specialist's Unarmed Technique and Medical-Nin's Martial Competency both
// only ever offer a Taijutsu Stance pick, never a Weapon Stance.
func (s *server) loadTaijutsuStanceOptions() ([]stanceOption, error) {
	return s.loadStanceOptionsByType("taijutsu")
}

// loadMartialTechniquesTabData returns nil for a character with no Taijutsu
// Specialist levels AND none of the archetype-training feats
// (taijutsuArchetypeFeatSlugs) — the template gates the whole box's
// existence on this being non-nil, same treatment PuppetTactics gets.
//
// Martial Arts Enthusiast's own "Select one Taijutsu Specialist Class,
// Taijutsu Style (Subclass). You gain its 3rd Level Martial Techniques." is
// NOT modeled here: it needs a genuinely new picker (8 Taijutsu Styles, none
// of which this feat-only character has actually chosen a real subclass
// for) with its own template markup and route, which is more than this pass
// safely covers without the ability to build/run the app to verify a new
// template renders correctly. The feat is still recognized by
// taijutsuArchetypeFeatSlugs (so a character who has ONLY this feat still
// sees the rest of the box — Martial Dice, if any other archetype feat also
// applies, and the class-wide technique picker), it just grants none of its
// own bespoke techniques yet.
func (s *server) loadMartialTechniquesTabData(characterID int64, sheet *charsheet.Sheet) (*martialTechniquesTabData, error) {
	taijutsuLevel, err := s.taijutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	if taijutsuLevel == 0 && !hasAnyGrantedFeature(grantedFeatures, taijutsuArchetypeFeatSlugs) {
		return nil, nil
	}

	cap, err := s.martialTechniqueCap(grantedFeatures, taijutsuLevel)
	if err != nil {
		return nil, err
	}

	picks, err := charstore.ListMartialTechniques(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	pickedSet := make(map[string]bool, len(picks))
	for _, slug := range picks {
		pickedSet[slug] = true
	}

	subclassSlug, _, err := s.taijutsuSpecialistSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}

	// All 20 Martial Techniques rows ingested so far are class-wide
	// (subclass_slug IS NULL), matching CLASS_AUDIT.md's documented fix for
	// what had mistagged these rows to Passionate Flame. Matching a specific
	// subclass_slug too keeps this correct if a genuinely per-Style entry is
	// ever ingested later.
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND list_name = 'Martial Techniques'
		  AND (subclass_slug IS NULL OR subclass_slug = ?)
		ORDER BY name`, taijutsuSpecialistSlug, subclassSlug)
	if err != nil {
		return nil, err
	}
	var known []knownMartialTechnique
	var available []martialTechniqueOption
	for rows.Next() {
		var opt martialTechniqueOption
		if err := rows.Scan(&opt.Slug, &opt.Name, &opt.Description); err != nil {
			rows.Close()
			return nil, err
		}
		if pickedSet[opt.Slug] {
			known = append(known, knownMartialTechnique{Slug: opt.Slug, Name: opt.Name, Description: opt.Description})
		} else {
			available = append(available, opt)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, f := range grantedFeatures {
		if names, ok := martialTechniqueAutoGrants[f.Slug]; ok {
			for _, name := range names {
				known = append(known, knownMartialTechnique{Name: name, Granted: true, Description: f.Description})
			}
		}
	}
	sort.Slice(known, func(i, j int) bool { return known[i].Name < known[j].Name })

	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}

	stanceOptions, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		return nil, err
	}
	stance := buildFightingStanceView(choices, stanceOptions, taijutsuStanceFeatureSlug(taijutsuLevel))

	data := &martialTechniquesTabData{
		Cap:              cap,
		Used:             len(picks),
		Known:            known,
		Available:        available,
		UnarmedDamageDie: unarmedDamageDieSize(taijutsuLevel, grantedFeatures, stance.Current),
		Stance:           stance,
	}

	if subclassSlug == passionateFlameSubclassSlug || subclassSlug == ruinSubclassSlug {
		if subclassSlug == passionateFlameSubclassSlug {
			pf := &passionateFlameView{HandWrapsOptions: handWrapsOfPassionOptions}
			if hasGrantedFeature(grantedFeatures, fistsOfIronFeatureSlug) {
				pf.FistsOfIronDice = fistsOfIronBonusDice(taijutsuLevel)
			}
			wraps := choices[features.ChoiceKey{FeatureSlug: handWrapsOfPassionFeatureSlug, ChoiceIndex: 0}]
			pf.HandWraps = wraps
			for _, o := range handWrapsOfPassionOptions {
				if o.Value == wraps {
					pf.HandWrapsLabel = o.Label
				}
			}
			data.PassionateFlame = pf
		}
		if subclassSlug == ruinSubclassSlug {
			data.Ruin = &ruinAntiChakraView{
				Keyword1: choices[features.ChoiceKey{FeatureSlug: antiChakraWavelengthFeatureSlug, ChoiceIndex: 0}],
				Keyword2: choices[features.ChoiceKey{FeatureSlug: antiChakraWavelengthFeatureSlug, ChoiceIndex: 1}],
				Options:  antiChakraWavelengthKeywords,
			}
		}
	}

	// Stancer's Mixed Martial Arts (3rd level) and Stance Blending (9th
	// level) each independently grant one more "learn a Taijutsu Stance you
	// don't know" pick, on top of the base Unarmed Technique stance above —
	// same buildFightingStanceView shape as Medical-Nin's Martial
	// Competency (medical_nin.go), just two more independent grants under
	// their own feature slugs rather than a shared capped list. Only the
	// KNOWN-stance pick is tracked here; actually holding more than one
	// stance's benefit at once by spending Martial Dice stays manual (see
	// CLASS_AUDIT.md's Stancer row).
	if subclassSlug == stancerSubclassSlug {
		if hasGrantedFeature(grantedFeatures, mixedMartialArtsFeatureSlug) {
			data.StancerMixedMartialArts = buildFightingStanceView(choices, stanceOptions, mixedMartialArtsFeatureSlug)
		}
		if hasGrantedFeature(grantedFeatures, stanceBlendingFeatureSlug) {
			data.StancerStanceBlending = buildFightingStanceView(choices, stanceOptions, stanceBlendingFeatureSlug)
		}
	}

	return data, nil
}

// handleMartialTechniqueAdd learns one Martial Technique, gated by the
// character's own current cap — server-side, defense in depth regardless
// of what the UI already disables.
func (s *server) handleMartialTechniqueAdd(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing technique", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for martial technique add:", err)
		return
	}
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no Taijutsu Specialist mechanics", http.StatusBadRequest)
		return
	}
	if data.Used >= data.Cap {
		http.Error(w, "no technique slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.Available {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid technique to learn", http.StatusBadRequest)
		return
	}
	if err := charstore.AddMartialTechnique(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add martial technique:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// handleMartialTechniqueDelete drops one known technique. The slug is a
// form field, not a URL path segment — class_options slugs contain slashes
// (e.g. "class/taijutsu-specialist/option/martial-techniques/brutal-
// taijutsu"), which a single-segment {slug} path wildcard cannot carry
// (confirmed empirically: html/template does not percent-encode "/" in a
// URL-path substitution, and net/http's ServeMux only treats a %2F-encoded
// slash as part of a wildcard segment, not a literal one — this same shape
// bit handlePuppetTacticDelete elsewhere in this codebase, fixed the same
// way once found during the Puppet Master audit).
func (s *server) handleMartialTechniqueDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing technique", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveMartialTechnique(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove martial technique:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// handleFightingStance records Unarmed Technique's Taijutsu Stance pick —
// class-wide (not subclass-gated), freely re-editable same boundary as Hand
// Wraps of Passion below. Also serves Martial Arts Training's own identical
// stance pick for a character with no real Taijutsu Specialist levels
// (taijutsuStanceFeatureSlug picks the right ChoiceKey for either case) —
// the gate below reuses loadMartialTechniquesTabData's own check (data.Stance
// non-nil) rather than re-deriving the "does this character have Taijutsu
// mechanics" condition a second time.
func (s *server) handleFightingStance(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("stance_slug"))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for fighting stance:", err)
		return
	}
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques for fighting stance:", err)
		return
	}
	if data == nil || data.Stance == nil {
		http.Error(w, "character has no Taijutsu Specialist mechanics", http.StatusBadRequest)
		return
	}
	taijutsuLevel, err := s.taijutsuSpecialistClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load taijutsu level for fighting stance:", err)
		return
	}
	options, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load taijutsu stance options:", err)
		return
	}
	if err := s.storeFightingStanceChoice(id, taijutsuStanceFeatureSlug(taijutsuLevel), options, slug); err != nil {
		if err == errInvalidStance {
			http.Error(w, "not a valid stance", http.StatusBadRequest)
		} else {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set fighting stance:", err)
		}
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// setStancerMixedMartialArtsStance validates and stores Stancer's own Mixed
// Martial Arts (3rd level) known-stance pick — an independent grant from
// the base Unarmed Technique stance, stored under its own feature slug so a
// Stancer's two picks never collide (see buildFightingStanceView's own doc
// comment). Shared by handleStancerMixedMartialArtsStance's own Core-sheet
// AJAX route and the Stancer popup's own route (taijutsu_stancer_popup.go),
// so both share one validation path and cannot drift apart.
func (s *server) setStancerMixedMartialArtsStance(id int64, slug string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for mixed martial arts stance:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		log.Println("load martial techniques for mixed martial arts stance:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.StancerMixedMartialArts == nil {
		return http.StatusBadRequest, "character has no Mixed Martial Arts stance choice available"
	}
	options, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		log.Println("load taijutsu stance options:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := s.storeFightingStanceChoice(id, mixedMartialArtsFeatureSlug, options, slug); err != nil {
		if err == errInvalidStance {
			return http.StatusBadRequest, "not a valid stance"
		}
		log.Println("set mixed martial arts stance:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleStancerMixedMartialArtsStance is setStancerMixedMartialArtsStance's
// own Core-sheet AJAX wrapper.
func (s *server) handleStancerMixedMartialArtsStance(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("stance_slug"))
	if status, msg := s.setStancerMixedMartialArtsStance(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// setStancerStanceBlendingStance validates and stores Stancer's own Stance
// Blending (9th level) known-stance pick — a third independent stance grant
// on the same character (base Unarmed Technique + Mixed Martial Arts),
// stored under its own feature slug. Shared by
// handleStancerStanceBlendingStance's own Core-sheet AJAX route and the
// Stancer popup's own route (taijutsu_stancer_popup.go).
func (s *server) setStancerStanceBlendingStance(id int64, slug string) (int, string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for stance blending stance:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		log.Println("load martial techniques for stance blending stance:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil || data.StancerStanceBlending == nil {
		return http.StatusBadRequest, "character has no Stance Blending stance choice available"
	}
	options, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		log.Println("load taijutsu stance options:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := s.storeFightingStanceChoice(id, stanceBlendingFeatureSlug, options, slug); err != nil {
		if err == errInvalidStance {
			return http.StatusBadRequest, "not a valid stance"
		}
		log.Println("set stance blending stance:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleStancerStanceBlendingStance is setStancerStanceBlendingStance's own
// Core-sheet AJAX wrapper.
func (s *server) handleStancerStanceBlendingStance(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("stance_slug"))
	if status, msg := s.setStancerStanceBlendingStance(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// setHandWrapsOfPassion validates and stores Passionate Flame's
// re-selectable Hand Wraps of Passion pick. RAW gates re-picking to a Full
// Rest — not enforced here, same "trust the player" boundary Mastery
// already draws. Shared by handleHandWrapsOfPassion's own Core-sheet AJAX
// route and the Passionate Flame popup's own route (taijutsu_passionate_
// flame_popup.go).
func (s *server) setHandWrapsOfPassion(id int64, value string) (int, string) {
	valid := false
	for _, o := range handWrapsOfPassionOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid pick"
	}
	subclassSlug, _, err := s.taijutsuSpecialistSubclassSlug(id)
	if err != nil {
		log.Println("load subclass for hand wraps of passion:", err)
		return http.StatusInternalServerError, "database error"
	}
	if subclassSlug != passionateFlameSubclassSlug {
		return http.StatusBadRequest, "not a Passionate Flame character"
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, handWrapsOfPassionFeatureSlug, 0, value); err != nil {
		log.Println("set hand wraps of passion:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleHandWrapsOfPassion is setHandWrapsOfPassion's own Core-sheet AJAX
// wrapper.
func (s *server) handleHandWrapsOfPassion(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := s.setHandWrapsOfPassion(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// setAntiChakraWavelength validates and stores Ruin's 2-of-10 keyword pick,
// one choice_index slot each — freely re-editable, same boundary as Hand
// Wraps of Passion above. Shared by handleAntiChakraWavelength's own
// Core-sheet AJAX route and the Ruin popup's own route (taijutsu_ruin_
// popup.go).
func (s *server) setAntiChakraWavelength(id int64, first, second string) (int, string) {
	subclassSlug, _, err := s.taijutsuSpecialistSubclassSlug(id)
	if err != nil {
		log.Println("load subclass for anti-chakra wavelength:", err)
		return http.StatusInternalServerError, "database error"
	}
	if subclassSlug != ruinSubclassSlug {
		return http.StatusBadRequest, "not a Ruin character"
	}
	validKeyword := func(v string) bool {
		for _, k := range antiChakraWavelengthKeywords {
			if k == v {
				return true
			}
		}
		return false
	}
	if !validKeyword(first) || !validKeyword(second) || first == second {
		return http.StatusBadRequest, "choose two different keywords"
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, antiChakraWavelengthFeatureSlug, 0, first); err != nil {
		log.Println("set anti-chakra wavelength keyword 1:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, antiChakraWavelengthFeatureSlug, 1, second); err != nil {
		log.Println("set anti-chakra wavelength keyword 2:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleAntiChakraWavelength is setAntiChakraWavelength's own Core-sheet
// AJAX wrapper.
func (s *server) handleAntiChakraWavelength(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	first := strings.TrimSpace(r.FormValue("keyword1"))
	second := strings.TrimSpace(r.FormValue("keyword2"))
	if status, msg := s.setAntiChakraWavelength(id, first, second); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}
