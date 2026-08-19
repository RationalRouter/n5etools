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

// Medical-Nin's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Medical-Nin entry documents that don't already fit the
// generic customResourceGrants pool (see custom_resources.go for Chakra
// Scalpel Charges, Preserve/Take Life, Yin Motes, Competent Combatant's
// double-modifier uses, Passive Regeneration's own use-cap, Natural
// Healing Dice, Shaman's Hex Marks, and Transfigured Technique's uses):
// three size/bonus-only informational readouts (Chakra Scalpel damage,
// Channeled Healing bonus, Rejuvenating Rest extra dice) and Medical
// Doctrine's own 4-option cap+catalog pick (cap 1@3rd -> 2@13th).
//
// Combat Medic's Martial Competency (2nd level) grants the identical
// Taijutsu Stance pick Taijutsu Specialist's own Unarmed Technique grants
// — see taijutsu.go's buildFightingStanceView/storeFightingStanceChoice,
// generalized here as a second granting feature rather than duplicated.
//
// Everything else this pass deferred — the 6 subclasses' own
// jutsu-plus-rider-effect charts (the same missing system Hunter-Nin's own
// audit already flagged as a cross-class future build), Medical Doctrine's
// 4 individual triggered sub-effects, and every other conditional/
// triggered payload needing a subsystem this app has no shape for yet — is
// documented, not modeled; see CLASS_AUDIT.md's Medical-Nin detail entry.
const medicalNinSlug = "class/medical-nin"

// combatMedicSubclassSlug identifies Combat Medic among Medical-Nin's 6
// Tenets of Medicine subclasses — the only one that grants a Taijutsu
// Stance pick (Martial Competency).
const combatMedicSubclassSlug = "class/medical-nin/group/tenets-of-medicine/combat-medic"

// martialCompetencyFeatureSlug is Combat Medic's own 2nd-level feature
// granting a Taijutsu Stance pick, mirroring Taijutsu Specialist's
// unarmedTechniqueFeatureSlug (taijutsu.go). Its own text: "you also adopt
// a particular fighting stance. Choose one of the Taijutsu Stance located
// in Chapter 13: Customization Options."
const martialCompetencyFeatureSlug = "class/medical-nin/group/tenets-of-medicine/combat-medic/feature/martial-competency"

// medicalDoctrineSlugs: Medical Doctrine's 4 named options exist as
// separate class_features rows (NULL level, sort_order 6-9, sitting
// between Chakra Scalpel and ASI/Feat) rather than being embedded in the
// parent feature's own text — read live here rather than hand-transcribed,
// same "read the chart, don't retype it" discipline
// loadGenjutsuConversionCatalog already established for Real World
// Conversion's own live-read class_features catalog.
var medicalDoctrineSlugs = []string{
	"class/medical-nin/feature/long-life-short-death",
	"class/medical-nin/feature/never-on-the-front-lines",
	"class/medical-nin/feature/not-allowed-to-die",
	"class/medical-nin/feature/until-their-heart-stops",
}

// medicalDoctrineOption is one of Medical Doctrine's 4 catalog entries.
type medicalDoctrineOption struct {
	Slug        string
	Name        string
	Description string
}

// knownMedicalDoctrine is one chosen Medical Doctrine — no Granted variant
// exists (no subclass auto-grants a doctrine for free), unlike Hunter-Nin's
// knownHunterPick. Carries its own Description (unlike knownHunterPick,
// which links out to a click-through popup instead) so the sheet can show
// a plain rollover tooltip on the known pick, same treatment Martial
// Technique's own Known list gives.
type knownMedicalDoctrine struct {
	Slug        string
	Name        string
	Description string
}

// medicalNinClassLevel returns the character's own Medical-Nin class
// level, or 0 if they have none — mirrors cookingNinClassLevel.
func (s *server) medicalNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, medicalNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// medicalNinSubclassSlug resolves the character's own chosen Tenet of
// Medicine, if any — mirrors hunterNinSubclassSlug/
// taijutsuSpecialistSubclassSlug exactly.
func (s *server) medicalNinSubclassSlug(characterID int64) (slug, name string, err error) {
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
		if classSlug == medicalNinSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// chakraScalpelDamageDie reads Chakra Scalpel's own live chart
// (v_class_level_resources, resource_name "Chakra Scalpel damage") rather
// than hand-transcribing it — same "read the chart, don't retype it"
// precedent lethalAttackDice/cookingDieSize established. The pool of
// activation charges this die rides on (3->9 by level) is a separate
// customResourceGrants entry ("chakra_scalpel_charges").
func (s *server) chakraScalpelDamageDie(level int) (string, error) {
	var value sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = 'Chakra Scalpel damage'`,
		medicalNinSlug, level,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// channeledHealingBonus reads Channeled Healing's own live chart
// (v_class_level_resources, resource_name "Channeled Healing") — a flat
// numeric bonus healing readout, not a pool. The same feature's separate
// failed-death-save-removal clause is a real spend pool, tracked in
// custom_resources.go under Key "channeled_healing_death_save_removal".
func (s *server) channeledHealingBonus(level int) (string, error) {
	var value sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = 'Channeled Healing'`,
		medicalNinSlug, level,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// rejuvenatingRestDice: "they regain an extra 1d6 Hit points. This amount
// of extra healing increases to 2d6 at 7th level, 3d6 at 11th, and 4d6 at
// 17th Level." Not charted in v_class_level_resources (confirmed — only
// Chakra Scalpel Charges/damage and Channeled Healing are), so this is
// hand-curated the same way martialDieSize/rejuvenatingRestDice's sibling
// size-only readouts already are.
func rejuvenatingRestDice(level int) string {
	switch {
	case level >= 17:
		return "4d6"
	case level >= 11:
		return "3d6"
	case level >= 7:
		return "2d6"
	case level >= 1:
		return "1d6"
	default:
		return ""
	}
}

// medicalDoctrineCap: 1 at 3rd level, 2 at 13th.
func medicalDoctrineCap(level int) int {
	switch {
	case level >= 13:
		return 2
	case level >= 3:
		return 1
	default:
		return 0
	}
}

// loadMedicalDoctrineCatalog reads Medical Doctrine's 4 named
// class_features rows live (blank level column — these aren't gated by
// their own level, medicalDoctrineCap handles that) rather than
// hand-transcribing their names/text, mirroring
// loadGenjutsuConversionCatalog's identical shape.
func (s *server) loadMedicalDoctrineCatalog() ([]medicalDoctrineOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_features
		WHERE class_slug = ? AND slug IN (?, ?, ?, ?)
		ORDER BY sort_order`,
		medicalNinSlug,
		medicalDoctrineSlugs[0], medicalDoctrineSlugs[1], medicalDoctrineSlugs[2], medicalDoctrineSlugs[3])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []medicalDoctrineOption
	for rows.Next() {
		var o medicalDoctrineOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// splitMedicalDoctrinePicks classifies the 4-option catalog against a
// character's stored picks — mirrors splitHunterPicks/splitGenjutsuPicks.
func splitMedicalDoctrinePicks(catalog []medicalDoctrineOption, picked map[string]bool) (known []knownMedicalDoctrine, available []medicalDoctrineOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownMedicalDoctrine{Slug: o.Slug, Name: o.Name, Description: o.Description})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// medicalNinTabData is the sheet_medical_nin box's full data — nil for a
// character with no Medical-Nin levels, same "no empty state" treatment
// HunterTechniques/CookingNin already establish.
type medicalNinTabData struct {
	ChakraScalpelDamageDie string
	ChanneledHealingBonus  string
	RejuvenatingRestDice   string

	DoctrineCap       int
	DoctrineUsed      int
	KnownDoctrines    []knownMedicalDoctrine
	AvailableDoctrine []medicalDoctrineOption

	// Stance is Combat Medic's own Martial Competency Taijutsu Stance pick
	// — nil for every other subclass (and for a Combat Medic below 2nd
	// level, though the subclass's own 2nd-level gate makes that
	// practically unreachable once the subclass is chosen at all).
	Stance *fightingStanceView
}

// loadMedicalNinTabData returns nil for a character with no Medical-Nin
// levels.
func (s *server) loadMedicalNinTabData(characterID int64, sheet *charsheet.Sheet) (*medicalNinTabData, error) {
	level, err := s.medicalNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}

	scalpelDie, err := s.chakraScalpelDamageDie(level)
	if err != nil {
		return nil, err
	}
	healingBonus, err := s.channeledHealingBonus(level)
	if err != nil {
		return nil, err
	}

	data := &medicalNinTabData{
		ChakraScalpelDamageDie: scalpelDie,
		ChanneledHealingBonus:  healingBonus,
		RejuvenatingRestDice:   rejuvenatingRestDice(level),
	}

	data.DoctrineCap = medicalDoctrineCap(level)
	if data.DoctrineCap > 0 {
		catalog, err := s.loadMedicalDoctrineCatalog()
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListMedicalDoctrinePicks(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.DoctrineUsed = len(picks)
		data.KnownDoctrines, data.AvailableDoctrine = splitMedicalDoctrinePicks(catalog, pickedSet)
	}

	subclassSlug, _, err := s.medicalNinSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	if subclassSlug == combatMedicSubclassSlug && level >= 2 {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		stanceOptions, err := s.loadTaijutsuStanceOptions()
		if err != nil {
			return nil, err
		}
		data.Stance = buildFightingStanceView(choices, stanceOptions, martialCompetencyFeatureSlug)
	}

	return data, nil
}

// handleMedicalDoctrineAdd learns one Medical Doctrine, gated by the
// character's own current cap — server-side, defense in depth regardless
// of what the UI already disables.
func (s *server) handleMedicalDoctrineAdd(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing doctrine", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for medical doctrine add:", err)
		return
	}
	data, err := s.loadMedicalNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load medical-nin for doctrine add:", err)
		return
	}
	if data == nil || data.DoctrineCap == 0 {
		http.Error(w, "character has not reached Medical Doctrine", http.StatusBadRequest)
		return
	}
	if data.DoctrineUsed >= data.DoctrineCap {
		http.Error(w, "no doctrine slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.AvailableDoctrine {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid doctrine", http.StatusBadRequest)
		return
	}
	if err := charstore.AddMedicalDoctrinePick(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add medical doctrine pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_medical_nin")
}

// handleMedicalDoctrineDelete drops one chosen doctrine — freely, at any
// time. RAW says "You cannot change this choice one made" (a typo for
// "once made" in the source text), but this app doesn't enforce pick-timing
// anywhere else either, same "trust the player" boundary Hunters
// Exploits/Malleable Mirages/Martial Techniques all draw.
func (s *server) handleMedicalDoctrineDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing doctrine", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveMedicalDoctrinePick(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove medical doctrine pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_medical_nin")
}

// handleMedicalNinFightingStance records Combat Medic's own Martial
// Competency Taijutsu Stance pick — generalizes taijutsu.go's
// storeFightingStanceChoice to a second granting feature instead of
// hard-gating the whole mechanic to Taijutsu Specialist. A character with
// levels in both Taijutsu Specialist and Combat Medic gets two independent
// picks, stored under two different feature slugs — see
// buildFightingStanceView's own doc comment.
func (s *server) handleMedicalNinFightingStance(w http.ResponseWriter, r *http.Request) {
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
	level, err := s.medicalNinClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load medical-nin level for fighting stance:", err)
		return
	}
	subclassSlug, _, err := s.medicalNinSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load medical-nin subclass for fighting stance:", err)
		return
	}
	if level < 2 || subclassSlug != combatMedicSubclassSlug {
		http.Error(w, "character has no Martial Competency stance choice available", http.StatusBadRequest)
		return
	}
	options, err := s.loadTaijutsuStanceOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load taijutsu stance options:", err)
		return
	}
	if err := s.storeFightingStanceChoice(id, martialCompetencyFeatureSlug, options, slug); err != nil {
		if err == errInvalidStance {
			http.Error(w, "not a valid stance", http.StatusBadRequest)
		} else {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set medical-nin fighting stance:", err)
		}
		return
	}
	s.respondSheet(w, r, id, "sheet_medical_nin")
}
