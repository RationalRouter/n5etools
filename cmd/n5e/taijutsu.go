package main

import (
	"database/sql"
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
// character can know at once. Each Taijutsu Style has its own techniques
// (subclass_slug set on every row, not a single class-wide pool as an
// earlier version of this comment assumed) — 20 ingested so far, all under
// Passionate Flame, since the other Styles' own techniques haven't been
// ingested yet. loadMartialTechniquesTabData's own query matches
// subclass_slug IS NULL too, so a genuinely class-wide entry (if one is
// ever ingested) is still picked up. Two Talent & Focus
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
	passionateFlameSubclassSlug     = "class/taijutsu-specialist/group/taijutsu-style/passionate-flame"
	ruinSubclassSlug                = "class/taijutsu-specialist/group/taijutsu-style/ruin"

	martialDiceResourceKey = "martial_dice"
)

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

// martialDiceMax computes a character's Martial Dice pool size: the base
// class_level_resources chart ("Martial Die": 1 at 2nd-5th, 2 at 6th-9th, 3
// at 10th-13th, 4 at 14th-18th, 5 at 19th-20th) plus Talent & Focus's
// Unnatural Talent overlay.
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
func unarmedDamageDieSize(taijutsuLevel int) string {
	switch {
	case taijutsuLevel >= 11:
		return "d10"
	case taijutsuLevel >= 6:
		return "d8"
	default:
		return "d6"
	}
}

// martialTechniqueCap computes how many Martial Techniques a character can
// know at once: the base class_level_resources chart ("Martial
// Techniques": 4 -> 5 at 7th -> 6 at 13th -> 7 at 19th) plus Talent &
// Focus's Talented Legacy overlay (+1 at 14th, +1 more — total +2 — at
// 20th).
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
// levels — the template gates the whole tile on this being non-nil, same
// "real DOM removal" treatment CustomResources/PuppetTactics already get.
func (s *server) loadMartialDice(characterID int64, sheet *charsheet.Sheet) (*martialDiceView, error) {
	taijutsuLevel, err := s.taijutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if taijutsuLevel == 0 {
		return nil, nil
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
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
		http.Error(w, "character has no levels in Taijutsu Specialist", http.StatusBadRequest)
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
		http.Error(w, "character has no levels in Taijutsu Specialist", http.StatusBadRequest)
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
// doesn't count against the cap).
type knownMartialTechnique struct {
	Slug    string
	Name    string
	Granted bool
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
	Cap              int
	Used             int
	Known            []knownMartialTechnique
	Available        []martialTechniqueOption
	UnarmedDamageDie string
	Stance           *fightingStanceView
	PassionateFlame  *passionateFlameView
	Ruin             *ruinAntiChakraView
}

// loadTaijutsuStanceOptions catalogs the 9 Taijutsu Stances from
// fighting_stances (Chapter 13 reference data, class-agnostic — also holds
// 12 Bukijutsu stances this class's own Unarmed Technique pick has no
// bearing on, hence the stance_type filter).
func (s *server) loadTaijutsuStanceOptions() ([]stanceOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM fighting_stances
		WHERE stance_type = 'taijutsu' ORDER BY name`)
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

// loadMartialTechniquesTabData returns nil for a character with no
// Taijutsu Specialist levels — the template gates the whole box's
// existence on this being non-nil, same treatment PuppetTactics gets.
func (s *server) loadMartialTechniquesTabData(characterID int64, sheet *charsheet.Sheet) (*martialTechniquesTabData, error) {
	taijutsuLevel, err := s.taijutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if taijutsuLevel == 0 {
		return nil, nil
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
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

	// Despite the "class-wide catalog" framing this box's own doc comment
	// still uses (a holdover from before any Style's own Martial Techniques
	// had actually been ingested), every row ingested so far carries a real
	// subclass_slug (each Style has its own techniques, not one shared
	// pool) — subclass_slug IS NULL matched zero rows for every character,
	// making "Learn Technique" permanently empty regardless of picks or
	// cap. Matching NULL too keeps this correct if a genuinely class-wide
	// entry is ever ingested later.
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
			known = append(known, knownMartialTechnique{Slug: opt.Slug, Name: opt.Name})
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
				known = append(known, knownMartialTechnique{Name: name, Granted: true})
			}
		}
	}
	sort.Slice(known, func(i, j int) bool { return known[i].Name < known[j].Name })

	data := &martialTechniquesTabData{
		Cap:              cap,
		Used:             len(picks),
		Known:            known,
		Available:        available,
		UnarmedDamageDie: unarmedDamageDieSize(taijutsuLevel),
	}

	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}

	stanceOptions, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		return nil, err
	}
	stance := &fightingStanceView{
		Current: choices[features.ChoiceKey{FeatureSlug: unarmedTechniqueFeatureSlug, ChoiceIndex: 0}],
		Options: stanceOptions,
	}
	for _, o := range stanceOptions {
		if o.Slug == stance.Current {
			stance.CurrentName = o.Name
			stance.CurrentDescription = o.Description
		}
	}
	data.Stance = stance

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
		http.Error(w, "character has no levels in Taijutsu Specialist", http.StatusBadRequest)
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
// Wraps of Passion below.
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
	taijutsuLevel, err := s.taijutsuSpecialistClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load taijutsu level for fighting stance:", err)
		return
	}
	if taijutsuLevel == 0 {
		http.Error(w, "character has no levels in Taijutsu Specialist", http.StatusBadRequest)
		return
	}
	options, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load stance options for fighting stance:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid stance", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, unarmedTechniqueFeatureSlug, 0, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set fighting stance:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// handleHandWrapsOfPassion records Passionate Flame's re-selectable Hand
// Wraps of Passion pick. RAW gates re-picking to a Full Rest — not
// enforced here, same "trust the player" boundary Mastery already draws.
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
	valid := false
	for _, o := range handWrapsOfPassionOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	subclassSlug, _, err := s.taijutsuSpecialistSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load subclass for hand wraps of passion:", err)
		return
	}
	if subclassSlug != passionateFlameSubclassSlug {
		http.Error(w, "not a Passionate Flame character", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, handWrapsOfPassionFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set hand wraps of passion:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}

// handleAntiChakraWavelength records Ruin's 2-of-10 keyword pick, one
// choice_index slot each — freely re-editable, same boundary as above.
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
	subclassSlug, _, err := s.taijutsuSpecialistSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load subclass for anti-chakra wavelength:", err)
		return
	}
	if subclassSlug != ruinSubclassSlug {
		http.Error(w, "not a Ruin character", http.StatusBadRequest)
		return
	}
	validKeyword := func(v string) bool {
		for _, k := range antiChakraWavelengthKeywords {
			if k == v {
				return true
			}
		}
		return false
	}
	first := strings.TrimSpace(r.FormValue("keyword1"))
	second := strings.TrimSpace(r.FormValue("keyword2"))
	if !validKeyword(first) || !validKeyword(second) || first == second {
		http.Error(w, "choose two different keywords", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, antiChakraWavelengthFeatureSlug, 0, first); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set anti-chakra wavelength keyword 1:", err)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, antiChakraWavelengthFeatureSlug, 1, second); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set anti-chakra wavelength keyword 2:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_techniques")
}
