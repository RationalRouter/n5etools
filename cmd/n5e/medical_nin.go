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

// Medical-Nin's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Medical-Nin entry documents that don't already fit the
// generic customResourceGrants pool (see custom_resources.go for Chakra
// Scalpel Charges, Preserve/Take Life, Yin Motes, Competent Combatant's
// double-modifier uses, Passive Regeneration's own use-cap, Natural
// Healing Dice, Shaman's Hex Marks, Transfigured Technique's uses, and Not
// Allowed to Die/Until Their Heart Stops' own per-rest use-counts — the
// latter two gated through medicalDoctrinePickedRows below, since Medical
// Doctrine's 4 options are NULL-level class_features rows loadGrantedFeatures
// returns unconditionally for every Medical-Nin): three size/bonus-only
// informational readouts (Chakra Scalpel damage, Channeled Healing bonus,
// Rejuvenating Rest extra dice), Medical Doctrine's own 4-option cap+catalog
// pick (cap 1@3rd -> 2@13th), and Combat Medic's own Expert Combatant pick
// (cap 1@9th -> 2@13th -> 3@17th, sourced from the character's own known
// Taijutsu/Bukijutsu rather than a rules-database catalog).
//
// Combat Medic's Martial Competency (2nd level) grants the identical
// Taijutsu Stance pick Taijutsu Specialist's own Unarmed Technique grants
// — see taijutsu.go's buildFightingStanceView/storeFightingStanceChoice,
// generalized here as a second granting feature rather than duplicated.
//
// Everything else this pass deferred — the 6 subclasses' own
// jutsu-plus-rider-effect charts (the same missing system Hunter-Nin's own
// audit already flagged as a cross-class future build), Medical Doctrine's
// remaining 2 triggered sub-effects (Long Life Short Death, a per-recipient
// cap rather than a per-caster pool; Never on the Front Lines, no count at
// all), Expert Combatant's own "gains the Medical Keyword" downstream
// effect and its separate "attack twice instead of once" clause (no
// attacks-per-action field exists anywhere in this app for any class), and
// every other conditional/triggered payload needing a subsystem this app
// has no shape for yet — is documented, not modeled; see CLASS_AUDIT.md's
// Medical-Nin detail entry.
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

// Three archetype-training feats (feat/class/medical-training ->
// medical-expert -> medical-specialist) grant a taste of Medical-Nin's own
// mechanics to a character with zero real Medical-Nin levels — each feat's
// own prerequisites text ("You cannot have class levels in Medical-Nin", on
// Medical Training) makes real levels and this feat progression mutually
// exclusive. Confirmed live against rules.db's feats table:
//   - Medical Training (Level 5+, Int or Wis 15+): "the Rejuvenating class
//     feature as if you were a 2nd level Medical-Nin. You do not gain any
//     benefits as a result of being higher level."
//   - Medical Expert (Medical Training, Level 10+): "the Chakra Scalpel
//     feature as if you were a 3rd level Medical-Nin. You gain 3 charges per
//     long rest... the Preserve/Take Life class feature as if you were a 5th
//     level Medical Nin... twice per rest."
//   - Medical Specialist (Medical Expert, Level 15+): "the 7th level
//     benefits of the Chakra Scalpel class feature" plus "one doctrine from
//     the Medical Doctrine class feature."
//
// A character holding a deeper feat is assumed to also carry every
// shallower one's own benefits (this app doesn't enforce feat prerequisites
// in code, same "trust the player" boundary genjutsuArchetypeFeats/
// huntersArchetypeGrants already draw). Bad Medicine's separate "Take
// Life... additional damage" clause and Medical Training's own Proficiency-
// in-Medicine/jutsu-keyword-access clauses are out of scope here — no Take
// Life damage field exists on the sheet at all (a genuine but separate gap),
// and Proficiency-in-Medicine is already handled generically by
// feat_skill_proficiency.go.
const (
	medicalTrainingFeatSlug   = "feat/class/medical-training"
	medicalExpertFeatSlug     = "feat/class/medical-expert"
	medicalSpecialistFeatSlug = "feat/class/medical-specialist"
)

// medicalArchetypeFeats reports which of Medical Training/Expert/Specialist
// a character holds — mirrors genjutsuArchetypeFeats (genjutsu.go) exactly.
type medicalArchetypeFeats struct {
	Training, Expert, Specialist bool
}

// loadMedicalArchetypeFeats scans a character's merged granted-features list
// (feats included, via mergeFeatFeatures) for the 3 archetype-training feat
// slugs.
func loadMedicalArchetypeFeats(features []grantedFeatureRow) medicalArchetypeFeats {
	var m medicalArchetypeFeats
	for _, f := range features {
		switch f.Slug {
		case medicalTrainingFeatSlug:
			m.Training = true
		case medicalExpertFeatSlug:
			m.Expert = true
		case medicalSpecialistFeatSlug:
			m.Specialist = true
		}
	}
	return m
}

func (m medicalArchetypeFeats) any() bool {
	return m.Training || m.Expert || m.Specialist
}

// rejuvenatingLevel is the "as if you were a 2nd level Medical-Nin"
// qualification Medical Training grants for Rejuvenating specifically — the
// caller combines this with the character's own real Medical-Nin level via
// max().
func (m medicalArchetypeFeats) rejuvenatingLevel() int {
	if m.Training {
		return 2
	}
	return 0
}

// chakraScalpelLevel is the "as if 3rd level" (Medical Expert) / "7th level
// benefits" (Medical Specialist) qualification these feats grant for Chakra
// Scalpel's own damage-die readout — confirmed against
// v_class_level_resources: level 3 -> "1d4", level 7 -> "2d4", matching each
// feat's own framing. Specialist supersedes Expert (a character holding
// Specialist also holds Expert per the feat's own prerequisite, but the
// higher tier's benefit is what actually applies).
func (m medicalArchetypeFeats) chakraScalpelLevel() int {
	switch {
	case m.Specialist:
		return 7
	case m.Expert:
		return 3
	default:
		return 0
	}
}

// doctrineBonus: Medical Specialist grants "one doctrine from the Medical
// Doctrine class feature" outside the class's own level-gated cap.
func (m medicalArchetypeFeats) doctrineBonus() int {
	if m.Specialist {
		return 1
	}
	return 0
}

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

// medicalDoctrinePickedRows returns synthetic grantedFeatureRow entries for
// the character's picked Medical Doctrine options that carry a per-rest
// use-count pool (Not Allowed to Die, Until Their Heart Stops — see
// customResourceGrants). Medical Doctrine's 4 options are class_features
// rows with a NULL level column (medicalDoctrineSlugs above), so
// loadGrantedFeatures returns all 4 unconditionally for every Medical-Nin
// regardless of level or pick — a customResourceGrants entry keyed directly
// off one of those slugs would incorrectly grant its pool to every
// Medical-Nin at MinLevel, regardless of which doctrine (if any) was
// chosen. These synthetic rows are what actually scope each pool to a
// character who picked that specific doctrine, mirroring
// hunterNinPatternPassiveRows' identical "append, don't merge into the
// stored list" shape (hunter_nin.go) — the caller
// (custom_resources.go's loadCustomResources) appends this slice to the
// merged granted-features list before resolving customResourceGrants.
// Level/Description are left zero, same as the hunter-nin precedent —
// computeCustomResources only reads Slug. Long Life Short Death (a
// per-recipient cap, not per-caster) and Never on the Front Lines (no count
// at all) have no entry here and stay Group 2/3.
func (s *server) medicalDoctrinePickedRows(characterID int64) ([]grantedFeatureRow, error) {
	picks, err := charstore.ListMedicalDoctrinePicks(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	names := map[string]string{
		"class/medical-nin/feature/not-allowed-to-die":      "Not Allowed to Die",
		"class/medical-nin/feature/until-their-heart-stops": "Until Their Heart Stops",
	}
	var rows []grantedFeatureRow
	for _, slug := range picks {
		if name, ok := names[slug]; ok {
			rows = append(rows, grantedFeatureRow{Slug: slug, Name: name, SourceLabel: "Medical Doctrine"})
		}
	}
	return rows, nil
}

// expertCombatantCap: Combat Medic's own 9th-level feature: "select one
// taijutsu or bukijutsu you know. The selected jutsu gains the Medical
// Keyword. You may select an additional taijutsu or bukijutsu at 13th and
// 17th level... You may switch which jutsu this feature affects when you
// would complete a Long Rest. Also, you can attack twice, instead of once,
// whenever you take the Attack action on your turn." Cap: 1 at 9th level, 2
// at 13th, 3 at 17th — no class_level_resources chart exists for this
// bracket, hand-curated the same way ninjutsuMasterCap
// (ninjutsu_specialist.go) is. Only which jutsu is currently selected is
// tracked; the Medical Keyword's downstream effect and the "attack twice"
// clause both stay manual — no attacks-per-action field exists anywhere in
// this app for any class.
func expertCombatantCap(level int) int {
	switch {
	case level >= 17:
		return 3
	case level >= 13:
		return 2
	case level >= 9:
		return 1
	default:
		return 0
	}
}

// loadKnownExpertCombatantJutsu reads every jutsu of classification
// "Taijutsu" or "Bukijutsu" the character currently knows (character_jutsu,
// both a published jutsu_slug resolved against rules.db's v_jutsu and a
// player-created custom_jutsu resolved entirely within charDB), for Expert
// Combatant's own picker to filter and offer. A sibling of
// ninjutsu_specialist.go's loadKnownNinjutsu (same body, different
// classification filter — "Taijutsu"/"Bukijutsu" here instead of
// "Ninjutsu") rather than a shared/generalized helper, kept in this file
// since Expert Combatant is Medical-Nin's own feature. A stale published
// jutsu_slug (removed in a rules update) is silently skipped, same
// tolerance loadKnownNinjutsu already extends.
func (s *server) loadKnownExpertCombatantJutsu(characterID int64) ([]knownJutsuOption, error) {
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
		var name, classification, rank, keywords, description string
		if r.jutsuSlug.Valid {
			err := s.rulesDB.QueryRow(
				`SELECT name, classification, rank, keywords, description FROM v_jutsu WHERE slug = ?`, r.jutsuSlug.String,
			).Scan(&name, &classification, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue // stale slug (rules update) — skip rather than break the picker
			}
			if err != nil {
				return nil, err
			}
		} else if r.customID.Valid {
			err := s.charDB.QueryRow(
				`SELECT name, classification, rank, keywords, description FROM custom_jutsu WHERE id = ?`, r.customID.Int64,
			).Scan(&name, &classification, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else {
			continue
		}
		if classification != "Taijutsu" && classification != "Bukijutsu" {
			continue
		}
		out = append(out, knownJutsuOption{
			JutsuID:        r.id,
			Name:           name,
			Rank:           rank,
			Description:    description,
			HasCombination: strings.Contains(keywords, "Combination"),
		})
	}
	return out, nil
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

	// ExpertCombatantCap/Used/Known/Available are Combat Medic's own Expert
	// Combatant pick (9th/13th/17th level) — zero/nil for every other
	// subclass, same gating shape Stance above uses. Sourced from the
	// character's own known jutsu (loadKnownExpertCombatantJutsu), not a
	// rules-database catalog.
	ExpertCombatantCap       int
	ExpertCombatantUsed      int
	KnownExpertCombatant     []knownNinjutsuJutsuPick
	AvailableExpertCombatant []knownJutsuOption
}

// loadMedicalNinTabData returns nil for a character with no Medical-Nin
// levels and none of the 3 archetype-training feats — mirrors
// loadHunterTechniquesTabData's identical widened gate.
func (s *server) loadMedicalNinTabData(characterID int64, sheet *charsheet.Sheet) (*medicalNinTabData, error) {
	level, err := s.medicalNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	arch := loadMedicalArchetypeFeats(grantedFeatures)
	if level == 0 && !arch.any() {
		return nil, nil
	}

	scalpelDie, err := s.chakraScalpelDamageDie(max(level, arch.chakraScalpelLevel()))
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
		RejuvenatingRestDice:   rejuvenatingRestDice(max(level, arch.rejuvenatingLevel())),
	}

	data.DoctrineCap = medicalDoctrineCap(level) + arch.doctrineBonus()
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

	if subclassSlug == combatMedicSubclassSlug {
		data.ExpertCombatantCap = expertCombatantCap(level)
		if data.ExpertCombatantCap > 0 {
			known, err := s.loadKnownExpertCombatantJutsu(characterID)
			if err != nil {
				return nil, err
			}
			picks, err := charstore.ListMedicalNinExpertCombatantPicks(s.charDB, characterID)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[int64]bool, len(picks))
			for _, id := range picks {
				pickedSet[id] = true
			}
			data.ExpertCombatantUsed = len(picks)
			for _, o := range known {
				if pickedSet[o.JutsuID] {
					data.KnownExpertCombatant = append(data.KnownExpertCombatant, knownNinjutsuJutsuPick{JutsuID: o.JutsuID, Name: o.Name, Rank: o.Rank})
				} else {
					data.AvailableExpertCombatant = append(data.AvailableExpertCombatant, o)
				}
			}
		}
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

// handleExpertCombatantAdd records one of Combat Medic's Expert Combatant
// picks (a specific known Taijutsu or Bukijutsu), gated by the character's
// own current cap — server-side, defense in depth regardless of what the UI
// already disables. Mirrors handleMedicalDoctrineAdd's shape, keyed by a
// character_jutsu row id (form field "jutsu_id") instead of a rules-database
// option_slug, same "jutsu_id" convention handleNinjutsuJutsuPickAdd
// (ninjutsu_specialist.go) uses.
func (s *server) handleExpertCombatantAdd(w http.ResponseWriter, r *http.Request) {
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
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for expert combatant add:", err)
		return
	}
	data, err := s.loadMedicalNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load medical-nin for expert combatant add:", err)
		return
	}
	if data == nil || data.ExpertCombatantCap == 0 {
		http.Error(w, "character has not reached Expert Combatant", http.StatusBadRequest)
		return
	}
	if data.ExpertCombatantUsed >= data.ExpertCombatantCap {
		http.Error(w, "no Expert Combatant slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.AvailableExpertCombatant {
		if o.JutsuID == jutsuID {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	if err := charstore.AddMedicalNinExpertCombatantPick(s.charDB, id, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add expert combatant pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_medical_nin")
}

// handleExpertCombatantDelete drops one Expert Combatant pick — freely, at
// any time. RAW allows switching which jutsu this feature affects on a Long
// Rest, but this app doesn't enforce pick-timing anywhere else either, same
// "trust the player" boundary handleMedicalDoctrineDelete draws.
func (s *server) handleExpertCombatantDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.RemoveMedicalNinExpertCombatantPick(s.charDB, id, jutsuID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove expert combatant pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_medical_nin")
}
