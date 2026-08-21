package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Intelligence Operative's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Intelligence Operative entry documents: Brave Orders
// (the class's core spendable resource, plus every activation-gate pool
// layered on top of it — Checkmate, Tsume, Momentary Pause, Converging
// Timelines, Day Ahead, Omniscient Clairvoyance, Sapphire Insights, War
// Cry, In Perfect Sync, Tactical Scheme — all handled by
// customResourceGrants, see custom_resources.go) and two independent
// cap-gated catalog picks — Plans (level 2, base class, class_options
// list_name "Plans", cap 2->3@5th->4@8th->5@11th->6@14th->7@17th->8@20th)
// and Operative Traps (level 3, Tactical Strategist only, class_options
// list_name "Operative Traps", cap 2->3@6th->4@9th; the SEPARATE
// Proficiency-Bonus "traps set" spend pool lives in customResourceGrants,
// same "pool here, known-list there" split Hunters Exploits/Hunters
// Patterns already established). Also built: Interrogationist's Unerring
// Eye and Perfect Mind, each of which lets its owner cast one specific
// named jutsu by spending a Brave Order instead of chakra, "as if you know
// it" — see intelligenceOperativeJutsuGrants below, the same
// FreeCast-button pattern Genjutsu Specialist's Malleable Mirages
// (genjutsu.go) and Hunter-Nin's Wolf Techniques (hunter_nin.go) already
// establish.
//
// Plans' own 21-row catalog was mistagged to the Tactical Strategist
// subclass in class_options — the same PDF-outline-bookmarking artifact
// already fixed for Taijutsu's Martial Techniques and Ninjutsu Specialist's
// Efficient Molding — corrected by internal/parse/subclasses.go's own
// Intelligence Operative special case plus a migration for the
// already-shipped rows.
//
// Everything else (each Plan's own in-combat effect text, Exploit
// Weakness's "currently studied target" state, Tactical Scheme's Scheme Die
// swap mechanic, Grave Controller's Dead Soul Technique/Corpse Collector and
// Shadowhand's Shadowhand Manifestation companion subsystems, Perfect
// Mind's conditional-Mastery edge case, and the rest of the class's
// Brave-Order-gated activated/reaction text) is documented, not modeled —
// see CLASS_AUDIT.md's Intelligence Operative detail entry for the full
// per-subclass breakdown.
const intelligenceOperativeSlug = "class/intelligence-operative"

const tacticalStrategistSubclassSlug = "class/intelligence-operative/group/master-strategies/tactical-strategist"

// unerringEyeFeatureSlug/perfectMindFeatureSlug are Interrogationist's two
// subclass features whose own text lets their owner cast a specific named
// jutsu by spending a Brave Order instead of chakra — see
// intelligenceOperativeJutsuGrants below.
const (
	unerringEyeFeatureSlug = "class/intelligence-operative/group/master-strategies/interrogationist/feature/unerring-eye"
	perfectMindFeatureSlug = "class/intelligence-operative/group/master-strategies/interrogationist/feature/perfect-mind"
)

// intelligenceOperativeJutsuGrant names one jutsu an Intelligence Operative
// subclass feature lets its owner cast by spending a Brave Order instead of
// Chakra — Intelligence Operative's own version of genjutsuMirageJutsuGrant
// (genjutsu.go)/hunterNinJutsuGrant (hunter_nin.go), keyed to an always-on
// subclass feature rather than a player pick. ResourceKey points straight
// at the class's own existing "brave_orders" customResourceGrants pool
// (Master Planner, custom_resources.go) rather than a new dedicated pool —
// neither feature below prints its own separate use-limit; both simply
// spend from the character's shared Brave Order pool.
type intelligenceOperativeJutsuGrant struct {
	JutsuSlug   string
	ResourceKey string
	Cost        int
	UsesPerCast int
}

// intelligenceOperativeJutsuGrants: Interrogationist's own two subclass
// features that each let their owner cast a specific named jutsu by
// spending a Brave Order.
//
// Unerring Eye (13th level): "...you can, as a Reaction, spend one Brave
// Order to Cast Genjutsu Break at the highest rank you can cast based on
// your class table, without expending chakra, as if you know it." Genjutsu
// Break resolves to exactly one v_jutsu row (jutsu/genjutsu-break,
// Genjutsu, C-Rank) — only the rank-agnostic "spend 1 Brave Order, no
// chakra" grant is modeled; casting at "the highest rank you can cast" is
// the jutsu's own printed rank here, the same simplification every other
// FreeCast grant in this codebase already makes (no grant tracks a
// player's own Highest Rank Known against the cast rank).
//
// Perfect Mind (17th level): "...you can, as a Reaction, spend one Brave
// Order to cast Chakra Shatter and automatically mark the triggering
// creature with your Exploit Weakness class feature." Less explicit than
// Unerring Eye (no "without expending chakra"/"as if you know it" stated
// outright), but modeled the same way — a Brave-Order-gated cast of a
// jutsu the character need not already know — consistent with every other
// "spend 1 Brave Order to activate X" feature elsewhere in this class,
// none of which layer an additional chakra cost on top of the Brave Order
// spend. Chakra Shatter resolves to exactly one v_jutsu row
// (jutsu/chakra-shatter, Genjutsu, C-Rank, printed cost 9 chakra); Cost is
// 0 here rather than that printed cost, matching that free-cast reading.
// Only the use-count gate (spending a Brave Order) is tracked — marking
// the triggering creature with Exploit Weakness stays manual/narrated,
// same as every other triggered side-effect clause elsewhere in this
// class's own features.
var intelligenceOperativeJutsuGrants = map[string]intelligenceOperativeJutsuGrant{
	unerringEyeFeatureSlug: {
		JutsuSlug:   "jutsu/genjutsu-break",
		ResourceKey: "brave_orders",
		Cost:        0,
		UsesPerCast: 1,
	},
	perfectMindFeatureSlug: {
		JutsuSlug:   "jutsu/chakra-shatter",
		ResourceKey: "brave_orders",
		Cost:        0,
		UsesPerCast: 1,
	},
}

// intelligenceOperativeJutsuGrantsForCharacter resolves every
// intelligenceOperativeJutsuGrants entry the character actually qualifies
// for — i.e. currently holds the granting subclass feature at their
// current Intelligence Operative level — keyed by the granted jutsu's own
// slug, the same output shape genjutsuMirageJutsuGrantsForCharacter/
// hunterNinJutsuGrantsForCharacter return, so it can feed the same
// FreeCast-population spot in loadCharacterJutsuSheet (characters.go).
// Reads loadGrantedFeatures directly rather than a stored pick — mirrors
// genjutsuPledgeJutsuGrantsForCharacter's own "always-on feature, not a
// player choice" shape (genjutsu.go); loadGrantedFeatures already gates a
// subclass feature by its own subclass's class level, so no separate
// Interrogationist/level check is needed here.
func (s *server) intelligenceOperativeJutsuGrantsForCharacter(characterID int64, clanSlug string, classLevel int) (map[string]intelligenceOperativeJutsuGrant, error) {
	granted, err := s.loadGrantedFeatures(characterID, clanSlug, classLevel)
	if err != nil {
		return nil, err
	}
	out := map[string]intelligenceOperativeJutsuGrant{}
	for _, f := range granted {
		if grant, ok := intelligenceOperativeJutsuGrants[f.Slug]; ok {
			out[grant.JutsuSlug] = grant
		}
	}
	return out, nil
}

// intelligenceOperativeClassLevel returns the character's own Intelligence
// Operative class level, or 0 if they have none — mirrors
// hunterNinClassLevel/genjutsuSpecialistClassLevel.
func (s *server) intelligenceOperativeClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, intelligenceOperativeSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// intelligenceOperativeSubclassSlug resolves the character's own chosen
// Master Strategy, if any — mirrors hunterNinSubclassSlug/
// genjutsuSpecialistSubclassSlug exactly.
func (s *server) intelligenceOperativeSubclassSlug(characterID int64) (slug, name string, err error) {
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
		if classSlug == intelligenceOperativeSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// plansKnownCap: Plans Known's own chart, confirmed via SQL against
// class_level_resources' "Plans Known" column: 2@2, 3@5, 4@8, 5@11, 6@14,
// 7@17, 8@20.
func plansKnownCap(level int) int {
	switch {
	case level >= 20:
		return 8
	case level >= 17:
		return 7
	case level >= 14:
		return 6
	case level >= 11:
		return 5
	case level >= 8:
		return 4
	case level >= 5:
		return 3
	case level >= 2:
		return 2
	default:
		return 0
	}
}

// operativeTrapsCap: Trap Setter's own known-traps chart ("Select 2
// Operative Traps... You learn one more trap at 6th Level, and another at
// 9th Level") — 2 at 3rd, 3 at 6th, 4 at 9th.
func operativeTrapsCap(level int) int {
	switch {
	case level >= 9:
		return 4
	case level >= 6:
		return 3
	case level >= 3:
		return 2
	default:
		return 0
	}
}

// intelligenceOperativePickOption is one entry in either of Intelligence
// Operative's two catalog picks.
type intelligenceOperativePickOption struct {
	Slug        string
	Name        string
	Description string
}

// knownIntelligenceOperativePick is one entry on a Known list — same shape
// knownHunterPick/knownGenjutsuPick already establish, kept here even
// though neither Plans nor Operative Traps has a subclass auto-grant (so
// Granted is always false) for consistency with every other catalog-pick
// popup link on the sheet, all of which route through the shared
// hunter_pick_detail_card template.
type knownIntelligenceOperativePick struct {
	Slug string
	Name string
}

// loadIntelligenceOperativeOptionCatalog reads one of Intelligence
// Operative's two DB-backed catalogs (list_name "Plans" or "Operative
// Traps") in full.
func (s *server) loadIntelligenceOperativeOptionCatalog(listName string) ([]intelligenceOperativePickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND list_name = ? ORDER BY name`, intelligenceOperativeSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []intelligenceOperativePickOption
	for rows.Next() {
		var o intelligenceOperativePickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// splitIntelligenceOperativePicks classifies a catalog against a
// character's stored picks for one category — same shape
// splitHunterPicks/splitGenjutsuPicks already establish.
func splitIntelligenceOperativePicks(catalog []intelligenceOperativePickOption, picked map[string]bool) (known []knownIntelligenceOperativePick, available []intelligenceOperativePickOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownIntelligenceOperativePick{Slug: o.Slug, Name: o.Name})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// intelligenceOperativeTabData is the sheet_intelligence_operative box's
// full data.
type intelligenceOperativeTabData struct {
	PlansCap   int
	PlansUsed  int
	KnownPlans []knownIntelligenceOperativePick
	// AvailablePlans deliberately isn't length-limited to (PlansCap -
	// PlansUsed) options here — the picker itself gates on the cap, same as
	// every other catalog pick on this sheet.
	AvailablePlans []intelligenceOperativePickOption

	OperativeTrapsCap       int
	OperativeTrapsUsed      int
	KnownOperativeTraps     []knownIntelligenceOperativePick
	AvailableOperativeTraps []intelligenceOperativePickOption
}

// loadIntelligenceOperativeTabData returns nil for a character with no
// Intelligence Operative levels — the template gates the whole box's
// existence on this being non-nil, same treatment HunterTechniques/Genjutsu
// get. Operative Traps only ever appears for a Tactical Strategist (its own
// Cap stays 0 for every other Master Strategy, hiding that sub-section the
// same way every other Cap-gated sub-section on this sheet hides itself).
func (s *server) loadIntelligenceOperativeTabData(characterID int64, sheet *charsheet.Sheet) (*intelligenceOperativeTabData, error) {
	level, err := s.intelligenceOperativeClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}

	data := &intelligenceOperativeTabData{}

	data.PlansCap = plansKnownCap(level)
	if data.PlansCap > 0 {
		catalog, err := s.loadIntelligenceOperativeOptionCatalog("Plans")
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListIntelligenceOperativePicks(s.charDB, characterID, charstore.IntelligenceOperativePickPlan)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.PlansUsed = len(picks)
		data.KnownPlans, data.AvailablePlans = splitIntelligenceOperativePicks(catalog, pickedSet)
	}

	subclassSlug, _, err := s.intelligenceOperativeSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	if subclassSlug == tacticalStrategistSubclassSlug {
		data.OperativeTrapsCap = operativeTrapsCap(level)
		if data.OperativeTrapsCap > 0 {
			catalog, err := s.loadIntelligenceOperativeOptionCatalog("Operative Traps")
			if err != nil {
				return nil, err
			}
			picks, err := charstore.ListIntelligenceOperativePicks(s.charDB, characterID, charstore.IntelligenceOperativePickOperativeTrap)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.OperativeTrapsUsed = len(picks)
			data.KnownOperativeTraps, data.AvailableOperativeTraps = splitIntelligenceOperativePicks(catalog, pickedSet)
		}
	}

	return data, nil
}

// handleIntelligenceOperativePickAdd builds one category's "learn a pick"
// route — shared since both catalogs validate/store identically, differing
// only in which of intelligenceOperativeTabData's own fields govern the cap
// and the currently-pickable list. Same shape
// handleHunterPickAdd/handleGenjutsuPickAdd already establish.
func (s *server) handleIntelligenceOperativePickAdd(
	category charstore.IntelligenceOperativePickCategory,
	used func(*intelligenceOperativeTabData) int,
	cap func(*intelligenceOperativeTabData) int,
	available func(*intelligenceOperativeTabData) []intelligenceOperativePickOption,
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
			log.Println("compute sheet for intelligence operative pick add:", err)
			return
		}
		data, err := s.loadIntelligenceOperativeTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load intelligence operative for add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Intelligence Operative", http.StatusBadRequest)
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
		if err := charstore.AddIntelligenceOperativePick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add intelligence operative pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_intelligence_operative")
	}
}

// handleIntelligenceOperativePickDelete builds one category's "forget a
// pick" route — freely, at any time, same "trust the player" boundary
// every other pick removal in this app already draws.
func (s *server) handleIntelligenceOperativePickDelete(category charstore.IntelligenceOperativePickCategory) http.HandlerFunc {
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
		if err := charstore.RemoveIntelligenceOperativePick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove intelligence operative pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_intelligence_operative")
	}
}

// handleIntelligenceOperativePickDetail serves the click-to-open popup for
// a Known Plan or Operative Trap — same mechanism
// handleHunterPickDetail/handleGenjutsuPickDetail already use, reusing the
// same hunter_pick_detail_card template (a bare Name/Description card with
// nothing Hunter-specific in it). Not character-scoped — both catalogs are
// static rules content.
func (s *server) handleIntelligenceOperativePickDetail(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	slug := r.PathValue("slug")

	var listName string
	switch category {
	case "plan":
		listName = "Plans"
	case "operative-trap":
		listName = "Operative Traps"
	default:
		http.NotFound(w, r)
		return
	}

	var name, description string
	err := s.rulesDB.QueryRow(`
		SELECT name, description FROM class_options
		WHERE slug = ? AND class_slug = ? AND list_name = ?`,
		slug, intelligenceOperativeSlug, listName,
	).Scan(&name, &description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load intelligence operative pick detail:", err)
		return
	}

	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render intelligence operative pick detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render intelligence operative pick detail:", err)
	}
}
