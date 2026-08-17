package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Hunter-Nin's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Hunter-Nin entry documents: Lethal Attack (class_features
// level 1, a Sneak-Attack-shaped bonus-damage die scaling 1d8->10d8, already
// used by the class-browsing reference table via v_class_level_resources
// but never surfaced per-character before) rendered as a computed
// informational line, and three independent cap-gated catalog picks —
// Hunters Patterns (level 2, class_options list_name "Hunters Patterns", cap
// 1->2@9th->3@15th), Hunters Exploits (level 3, "Hunters Exploits", cap
// 2->3@10th->4@17th, plus a Proficiency-Bonus-per-Short-Rest spend pool
// handled by customResourceGrants — see custom_resources.go), and Defensive
// Tactics (level 6, a hand-curated 4-option catalog with no class_options
// rows of its own — the book embeds its options directly in the granting
// feature's own text — cap 1->2@11th->3@17th).
//
// Every one of the 8 Hunter's Creeds subclasses grants exclusive, free
// access to exactly one Hunters Exploit via its own 3rd-level feature ("you
// gain exclusive access to the X Hunter Exploit. This does not count
// against your Exploit Limit.") — huntersExploitAutoGrants/
// huntersExploitSubclassLocks below mirror Taijutsu's
// martialTechniqueAutoGrants for the "known for free, outside the cap"
// half, plus a lock so the other 7 subclasses can never manually pick that
// exploit from the shared catalog.
//
// Everything else (per-subclass numeric pools like Undertaker's Poisonous
// Embrace or Vice Agent's Greed's Shell, the 7 subclasses' own
// jutsu-plus-technique-rider grants, all Group 2/3 conditional/triggered
// features) is documented, not modeled — see CLASS_AUDIT.md's Hunter-Nin
// detail entry for the full per-subclass breakdown.
const hunterNinSlug = "class/hunter-nin"

// huntersExploitsResourceKey must match custom_resources.go's own Key for
// the "class/hunter-nin/feature/hunters-exploits" grant — the spend pool
// behind whichever exploits a character knows.
const huntersExploitsResourceKey = "hunter_exploits"

const (
	bladeWardenSubclassSlug  = "class/hunter-nin/group/hunters-creeds/blade-warden"
	necroticHandSubclassSlug = "class/hunter-nin/group/hunters-creeds/necrotic-hand"
	graveStalkerSubclassSlug = "class/hunter-nin/group/hunters-creeds/grave-stalker"
	arsenalistSubclassSlug   = "class/hunter-nin/group/hunters-creeds/arsenalist"
	undertakerSubclassSlug   = "class/hunter-nin/group/hunters-creeds/undertaker"
	viceAgentSubclassSlug    = "class/hunter-nin/group/hunters-creeds/vice-agent"
	voidWalkerSubclassSlug   = "class/hunter-nin/group/hunters-creeds/void-walker"
	wolvesLegacySubclassSlug = "class/hunter-nin/group/hunters-creeds/wolves-legacy"
)

// huntersExploitAutoGrants: the granting feature slug -> the one Hunters
// Exploit option_slug it grants for free, outside the known-exploit cap.
var huntersExploitAutoGrants = map[string]string{
	"class/hunter-nin/group/hunters-creeds/blade-warden/feature/blades-prey":          "class/hunter-nin/option/hunters-exploits/wardens-assault",
	"class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/medicinal-blade":     "class/hunter-nin/option/hunters-exploits/festering-siphonage",
	"class/hunter-nin/group/hunters-creeds/grave-stalker/feature/shadow-stalker":      "class/hunter-nin/option/hunters-exploits/shadow-step",
	"class/hunter-nin/group/hunters-creeds/arsenalist/feature/tools-of-the-trade":     "class/hunter-nin/option/hunters-exploits/sharp-rain",
	"class/hunter-nin/group/hunters-creeds/undertaker/feature/poisonous-embrace":      "class/hunter-nin/option/hunters-exploits/incurable-affliction",
	"class/hunter-nin/group/hunters-creeds/vice-agent/feature/greeds-shell":           "class/hunter-nin/option/hunters-exploits/foxs-sin",
	"class/hunter-nin/group/hunters-creeds/void-walker/feature/blink":                 "class/hunter-nin/option/hunters-exploits/void-assault",
	"class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-body": "class/hunter-nin/option/hunters-exploits/deflection",
}

// huntersExploitSubclassLocks: the same 8 exclusive exploits, keyed the
// other direction — an option_slug here can only ever be manually picked
// (i.e. appear in a character's own Available list) by a character whose
// own subclass matches. A character of the matching subclass never needs
// to pick it manually — huntersExploitAutoGrants already grants it for
// free — so in practice this only ever filters the option OUT of every
// OTHER subclass's picker.
var huntersExploitSubclassLocks = map[string]string{
	"class/hunter-nin/option/hunters-exploits/wardens-assault":      bladeWardenSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/festering-siphonage":  necroticHandSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/shadow-step":          graveStalkerSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/sharp-rain":           arsenalistSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/incurable-affliction": undertakerSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/foxs-sin":             viceAgentSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/void-assault":         voidWalkerSubclassSlug,
	"class/hunter-nin/option/hunters-exploits/deflection":           wolvesLegacySubclassSlug,
}

// defensiveTacticsOptions: Defensive Tactics' own 4-option catalog, hand-
// curated straight from class_features (the book embeds these options
// directly in the granting feature's text, not as class_options rows) —
// same reasoning as Taijutsu's handWrapsOfPassionOptions.
var defensiveTacticsOptions = []hunterPickOption{
	{Slug: "escaping-danger", Name: "Escaping Danger", Description: "Attacks of opportunity and attacks made as a result of a creature's Reaction, against you are made at disadvantage."},
	{Slug: "unbroken-will", Name: "Unbroken Will", Description: "You have advantage on saving throws to resist any Mental or Sensory Condition."},
	{Slug: "hunters-revenge", Name: "Hunter's Revenge", Description: "When you are hit by a creature's attack, the next time you deal damage to that creature you can Lethal Attack ignoring its normal trigger requirements."},
	{Slug: "evasion", Name: "Evasion", Description: "When you are subjected to an effect that allows you to make a Dexterity saving throw to take only half damage, you instead take no damage if you succeed on a saving throw, and only half damage if you fail."},
}

// hunterNinClassLevel returns the character's own Hunter-Nin class level, or
// 0 if they have none — mirrors taijutsuSpecialistClassLevel.
func (s *server) hunterNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, hunterNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// hunterNinSubclassSlug resolves the character's own chosen Hunter's Creed,
// if any — mirrors taijutsuSpecialistSubclassSlug/puppetMasterSubclassSlug
// exactly.
func (s *server) hunterNinSubclassSlug(characterID int64) (slug, name string, err error) {
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
		if classSlug == hunterNinSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// lethalAttackDice reads Lethal Attack's own live chart (v_class_level_resources,
// resource_name "Lethal Attack") rather than hand-transcribing it — same
// "read the chart, don't retype it" precedent martialDiceMax established.
func (s *server) lethalAttackDice(level int) (string, error) {
	var value sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = ? AND level = ? AND resource_name = 'Lethal Attack'`,
		hunterNinSlug, level,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// huntersPatternsCap: Hunters Patterns' own chart (1 base, 2 at 9th, 3 at
// 15th) — no v_class_level_resources row exists for this one (confirmed:
// only "Lethal Attack" is charted for this class), so it's hand-curated
// the same way martialDieSize is.
func huntersPatternsCap(level int) int {
	switch {
	case level >= 15:
		return 3
	case level >= 9:
		return 2
	case level >= 2:
		return 1
	default:
		return 0
	}
}

// huntersExploitsCap: Hunters Exploits' own known-count chart (2 at 3rd, 3
// at 10th, 4 at 17th) — the SEPARATE "how many times per short rest can you
// use one" number lives in customResourceGrants (custom_resources.go),
// keyed off Proficiency Bonus instead of a level chart.
func huntersExploitsCap(level int) int {
	switch {
	case level >= 17:
		return 4
	case level >= 10:
		return 3
	case level >= 3:
		return 2
	default:
		return 0
	}
}

// defensiveTacticsCap: 1 at 6th, 2 at 11th, 3 at 17th.
func defensiveTacticsCap(level int) int {
	switch {
	case level >= 17:
		return 3
	case level >= 11:
		return 2
	case level >= 6:
		return 1
	default:
		return 0
	}
}

// hunterPickOption is one entry in any of Hunter-Nin's three catalog picks.
type hunterPickOption struct {
	Slug        string
	Name        string
	Description string
}

// knownHunterPick is one entry on a Known list — either a player pick
// (has a remove button, counts against its own cap) or Granted true (one
// of the 8 subclass-exclusive Hunters Exploits, no remove button, doesn't
// count against the cap) — same shape Taijutsu's knownMartialTechnique
// already established.
type knownHunterPick struct {
	Slug    string
	Name    string
	Granted bool
}

// loadHunterOptionCatalog reads one of Hunter-Nin's two DB-backed catalogs
// (list_name "Hunters Patterns" or "Hunters Exploits") in full.
func (s *server) loadHunterOptionCatalog(listName string) ([]hunterPickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND list_name = ? ORDER BY name`, hunterNinSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hunterPickOption
	for rows.Next() {
		var o hunterPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// splitHunterPicks classifies a catalog against a character's stored picks
// for one category — shared by Patterns and Defensive Tactics; Exploits
// needs its own version below since it also merges subclass auto-grants
// and filters out the other 7 subclasses' locked exploits.
func splitHunterPicks(catalog []hunterPickOption, picked map[string]bool) (known []knownHunterPick, available []hunterPickOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownHunterPick{Slug: o.Slug, Name: o.Name})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// hunterTechniquesTabData is the sheet_hunter_techniques box's full data.
type hunterTechniquesTabData struct {
	LethalAttackDice string

	PatternsCap       int
	PatternsUsed      int
	KnownPatterns     []knownHunterPick
	AvailablePatterns []hunterPickOption

	ExploitsCap       int
	ExploitsUsed      int
	KnownExploits     []knownHunterPick
	AvailableExploits []hunterPickOption

	DefensiveTacticsCap       int
	DefensiveTacticsUsed      int
	KnownDefensiveTactics     []knownHunterPick
	AvailableDefensiveTactics []hunterPickOption
}

// loadHunterTechniquesTabData returns nil for a character with no
// Hunter-Nin levels — the template gates the whole box's existence on this
// being non-nil, same treatment MartialTechniques gets. Each of the three
// sub-sections independently gates itself on its own Cap being > 0 (a
// level 1-2 Hunter-Nin has Lethal Attack but neither Patterns nor
// Exploits yet).
func (s *server) loadHunterTechniquesTabData(characterID int64, sheet *charsheet.Sheet) (*hunterTechniquesTabData, error) {
	hunterLevel, err := s.hunterNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if hunterLevel == 0 {
		return nil, nil
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	subclassSlug, _, err := s.hunterNinSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	lethalDice, err := s.lethalAttackDice(hunterLevel)
	if err != nil {
		return nil, err
	}

	data := &hunterTechniquesTabData{LethalAttackDice: lethalDice}

	data.PatternsCap = huntersPatternsCap(hunterLevel)
	if data.PatternsCap > 0 {
		catalog, err := s.loadHunterOptionCatalog("Hunters Patterns")
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickPattern)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.PatternsUsed = len(picks)
		data.KnownPatterns, data.AvailablePatterns = splitHunterPicks(catalog, pickedSet)
	}

	data.ExploitsCap = huntersExploitsCap(hunterLevel)
	if data.ExploitsCap > 0 {
		catalog, err := s.loadHunterOptionCatalog("Hunters Exploits")
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickExploit)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.ExploitsUsed = len(picks)

		grantedExploitSlugs := map[string]bool{}
		for _, f := range grantedFeatures {
			if slug, ok := huntersExploitAutoGrants[f.Slug]; ok {
				grantedExploitSlugs[slug] = true
			}
		}

		for _, o := range catalog {
			switch {
			case grantedExploitSlugs[o.Slug]:
				data.KnownExploits = append(data.KnownExploits, knownHunterPick{Slug: o.Slug, Name: o.Name, Granted: true})
			case pickedSet[o.Slug]:
				data.KnownExploits = append(data.KnownExploits, knownHunterPick{Slug: o.Slug, Name: o.Name})
			default:
				if lock, locked := huntersExploitSubclassLocks[o.Slug]; locked && lock != subclassSlug {
					continue // exclusive to a different Hunter's Creed
				}
				data.AvailableExploits = append(data.AvailableExploits, o)
			}
		}
	}

	data.DefensiveTacticsCap = defensiveTacticsCap(hunterLevel)
	if data.DefensiveTacticsCap > 0 {
		picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickDefensiveTactic)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.DefensiveTacticsUsed = len(picks)
		data.KnownDefensiveTactics, data.AvailableDefensiveTactics = splitHunterPicks(defensiveTacticsOptions, pickedSet)
	}

	return data, nil
}

// handleHunterPickAdd builds one category's "learn a pick" route — shared
// since all three catalogs validate/store identically, differing only in
// which of hunterTechniquesTabData's own fields govern the cap and the
// currently-pickable list.
func (s *server) handleHunterPickAdd(
	category charstore.HunterNinPickCategory,
	used func(*hunterTechniquesTabData) int,
	cap func(*hunterTechniquesTabData) int,
	available func(*hunterTechniquesTabData) []hunterPickOption,
) http.HandlerFunc {
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
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for hunter pick add:", err)
			return
		}
		data, err := s.loadHunterTechniquesTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter techniques for add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Hunter-Nin", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		valid := false
		for _, o := range available(data) {
			if o.Slug == slug {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if err := charstore.AddHunterNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add hunter pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_hunter_techniques")
	}
}

// handleHunterPickDelete builds one category's "forget a pick" route —
// freely, at any time, same "trust the player" boundary Martial Technique
// forgetting already draws.
func (s *server) handleHunterPickDelete(category charstore.HunterNinPickCategory) http.HandlerFunc {
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
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveHunterNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove hunter pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_hunter_techniques")
	}
}

// handleHunterPickDetail serves the click-to-open popup for a Known
// Hunters Pattern, Hunters Exploit, or Defensive Tactic (see
// sheet_hunter_techniques' KnownPatterns/KnownExploits/
// KnownDefensiveTactics — both player-picked and the 8 subclass-exclusive
// Granted exploits carry a real Slug, so all three link here) — same
// "sheet-popup-trigger fetches its own href + ?fragment=1 and drops the
// result into the shared dialog" mechanism sheet-popup.js already uses
// for feats/items/jutsu, just without those pages' full standalone-page
// half: nothing else on the site ever links to a single Hunters Pattern/
// Exploit/Defensive Tactic, so this only ever needs to answer the
// fragment request, not a real full-page view. Not character-scoped —
// all three catalogs are static rules content, same as /feats/{slug...}.
func (s *server) handleHunterPickDetail(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	slug := r.PathValue("slug")

	var name, description string
	switch category {
	case "pattern":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = 'Hunters Patterns'`,
			slug, hunterNinSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter pattern detail:", err)
			return
		}
	case "exploit":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = 'Hunters Exploits'`,
			slug, hunterNinSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter exploit detail:", err)
			return
		}
	case "defensive-tactic":
		found := false
		for _, o := range defensiveTacticsOptions {
			if o.Slug == slug {
				name, description, found = o.Name, o.Description, true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}

	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render hunter pick detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render hunter pick detail:", err)
	}
}
