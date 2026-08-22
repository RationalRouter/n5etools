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

// This file is the whole Puppet/Summon/custom companion feature: a small,
// separate popup-window sheet (companion_sheet.html) a character can open
// per Puppet Tool or summoned creature they field, alongside the main sheet
// but never nested inside it.
//
// Deliberately NOT computed like the main sheet. A Puppet Master's puppet
// and a summon tribe's creature each follow bespoke stat rules (see
// summon_tribes.toughness/defensive_ability, which print as formulas, not
// values) that vary by chassis/tribe far more than this app tries to model.
// Every stat here is a plain player-entered field (charstore.Companion);
// the only thing this file computes is which of the character's OWN
// already-level-gated rules content is worth showing as read-only
// reference next to those fields — the character's Puppet Master subclass
// features (kind="puppet") or a chosen summon tribe's rank-gated content
// (kind="summon"), gated by the character's real level/rank exactly the
// way the main sheet's own granted-features panel already is.

// companionRespondFragment reads the optional "respond_fragment" form field
// shared by every add/delete-companion form (see character_sheet.html's own
// comments on the 3 forms that set it): handleSheetCompanionAdd/
// handleSheetCompanionDelete are each reused by 3 different callers (the
// Core tab's generic Companions box, the Puppets tab's own card/add-form,
// the Summons tab's own card) whose own data-target wants a different
// fragment swapped back — "sheet_companions" for the Core tab (this
// function's default, matching every caller that predates this field and
// so never sends it), "sheet_puppet_tab"/"sheet_summon_tab" for the other
// two. Whitelisted rather than passed straight to respondSheet, the same
// "don't trust a form field with an unbounded string" caution
// handleSheetCompanionAdd's own kind validation already applies.
func companionRespondFragment(r *http.Request) string {
	switch r.FormValue("respond_fragment") {
	case "sheet_puppet_tab":
		return "sheet_puppet_tab"
	case "sheet_summon_tab":
		return "sheet_summon_tab"
	default:
		return "sheet_companions"
	}
}

// companionKindLabels maps a companion's raw stored Kind value to its
// display label — the same small ordered-lookup-table shape items.go's own
// itemKindLabel/itemKindLabels use for a different "kind" concept (item
// categories), kept as a separate table since companion kind and item kind
// are unrelated vocabularies, but the same "small table, fallback to the
// raw value" pattern applies. "custom" maps to "Other" specifically to
// match the Add Companion dropdown's own existing option text
// (character_sheet.html's sheet_companions define block), not a generic
// Title Case of "custom", so a companion's kind badge and the dropdown that
// created it never disagree on what to call it.
var companionKindLabels = []struct{ kind, label string }{
	{"puppet", "Puppet"},
	{"summon", "Summon"},
	{"nin-dog", "Nin-Dog"},
	{"titan", "Titan"},
	{"snb", "S.N.B"},
	{"custom", "Other"},
}

// companionKindLabel resolves one companion kind's display label — shared
// by every place a companion's raw kind string would otherwise render
// verbatim as user-facing text (the Core tab's Companions box, the
// Companions tab's per-companion card header, the standalone companion
// popup's own header), so a future new kind only needs adding here once
// instead of at each render site individually, and so those render sites
// can never drift on what a given kind is called. Falls back to the raw
// kind string for anything not in the table (defensive; every kind
// handleSheetCompanionAdd accepts already has an entry here).
func companionKindLabel(kind string) string {
	for _, k := range companionKindLabels {
		if k.kind == kind {
			return k.label
		}
	}
	return kind
}

// companionStructuredAttackKinds whitelists which companion kinds have
// reached the structured, rollable Attacks presentation (as opposed to the
// plain freeform textarea a not-yet-covered kind falls back to) — puppet
// (the original), nin-dog, and titan (see the "Attacks section should be
// rollable, not typed" fix, which had the identical free-text bug and fix
// shape for both companion kinds — companion_fields.html's own doc on the
// shared {{else}} textarea branch), plus summon, custom, and snb, extended
// to cover every remaining companion kind so all of them can carry rollable
// attacks — including a manually-added jutsu-shaped row, per 0020_companion_
// attacks.sql's own doc on that being the intended path for a companion's
// jutsu absent a real per-companion casting-economy system. Kept as a map
// (rather than just checking kind != "" or similar) so a brand-new future
// kind still starts on the textarea fallback until deliberately added here.
var companionStructuredAttackKinds = map[string]bool{
	"puppet":  true,
	"nin-dog": true,
	"titan":   true,
	"summon":  true,
	"custom":  true,
	"snb":     true,
}

// companionSupportsStructuredAttacks reports whether kind has reached the
// structured Attacks presentation — used both to gate the template data a
// popup/tab card is given (ShowStructuredAttacks/ReadOnlyAttacks) and to
// guard the attack add/delete handlers server-side, so a stale or
// hand-crafted form POST can't add a structured attack row to a kind whose
// own card still renders the plain textarea.
func companionSupportsStructuredAttacks(kind string) bool {
	return companionStructuredAttackKinds[kind]
}

// companionAttacksFragment returns which sheet fragment holds a companion's
// own structured-attacks card, so the attack add/delete handlers can
// respond with the fragment that actually contains the row that changed
// instead of a fragment name hardcoded to one kind. Puppet-kind companions
// only ever render their structured Attacks on the Puppets tab
// (sheet_puppet_tab); every other kind that supports structured attacks
// (nin-dog, titan) renders on the Companions tab instead (sheet_summon_tab
// — see loadSummonsTabData's own no-kind-filter doc for why every non-puppet
// kind lands there).
func companionAttacksFragment(kind string) string {
	if kind == "puppet" {
		return "sheet_puppet_tab"
	}
	return "sheet_summon_tab"
}

// parseCharacterAndCompanionID reads the {id}/{cid} path values shared by
// every companion route, the same "NotFound rather than a parse error"
// contract parseCharacterAndRowID uses for inventory rows.
func parseCharacterAndCompanionID(w http.ResponseWriter, r *http.Request) (characterID, companionID int64, ok bool) {
	characterID, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	companionID, err = strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	return characterID, companionID, true
}

// handleSheetCompanionAdd creates one companion (form fields "name", "kind")
// and re-renders the main sheet's Companions box.
func (s *server) handleSheetCompanionAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	if kind != "puppet" && kind != "summon" && kind != "custom" && kind != "nin-dog" && kind != "titan" && kind != "snb" {
		http.Error(w, "bad kind", http.StatusBadRequest)
		return
	}
	companionID, err := charstore.AddCompanion(s.charDB, id, kind, name)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add companion:", err)
		return
	}
	if kind == "puppet" {
		if err := s.prefillPuppetStatDefaults(id, companionID); err != nil {
			// The companion itself was created successfully; a prefill
			// failure (e.g. no baseline row, no Puppet Master level yet)
			// just leaves it starting blank like any other companion —
			// not worth failing the whole add over.
			log.Println("prefill puppet stat defaults:", err)
		}
	}
	if kind == "nin-dog" {
		if err := s.prefillNinDogStatDefaults(id, companionID); err != nil {
			log.Println("prefill nin-dog stat defaults:", err)
		}
	}
	if kind == "titan" {
		if err := s.prefillTitanStatDefaults(id, companionID); err != nil {
			// A character with no Ordnance Training yet just leaves the
			// companion starting blank, same as prefillNinDogStatDefaults'
			// own treatment of a load failure just above.
			log.Println("prefill titan stat defaults:", err)
		}
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// prefillPuppetStatDefaults populates a freshly-created puppet's AC/HP-max/
// Speed/six ability scores from the Puppet Tool baseline, and starts it at
// full computed HP — called exactly once, right after creation. Uses
// charstore.SetCompanionStatDefaults (never SetCompanionFields) and a
// single explicit SetCompanionHP call, the same shape the dedicated HP
// endpoint already uses on a normal edit — see SetCompanionStatDefaults'
// own doc for why this can't reintroduce the whole-form-save/hp_current
// regression.
func (s *server) prefillPuppetStatDefaults(characterID, companionID int64) error {
	baseline, err := s.loadPuppetToolStatBlock()
	if err != nil || baseline == nil {
		return err
	}
	level, err := s.puppetMasterClassLevel(characterID)
	if err != nil || level == 0 {
		return err
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return err
	}
	masterConMod := sheet.Abilities["con"].Modifier

	ac := int64(puppetToolDefaultAC(baseline.ACBase, sheet.ProficiencyBonus))
	hpMax := int64(puppetToolMaxHP(*baseline, level, masterConMod))
	// No flying speed here: a brand-new puppet has no upgrades yet, so
	// nothing grants one (see hoveringMechanismBonus). The per-render
	// backfill in loadPuppetsTabData fills it in if one is ever taken.
	// size "" here: a brand-new puppet has no Puppeteer Chassis/Puppet
	// Framework/Puppet Role/Puppet Weapon Type pick yet either, so there is
	// nothing to backfill it with — same reasoning as flying speed above.
	// loadPuppetsTabData's own per-render backfill fills it in once a
	// Foundation catalog pick exists.
	return charstore.SetCompanionStatDefaults(s.charDB, characterID, companionID,
		ac, hpMax, hpMax, int64(baseline.Speed), sql.NullInt64{},
		int64(baseline.Str), int64(baseline.Dex), int64(baseline.Con),
		int64(baseline.Int), int64(baseline.Wis), int64(baseline.Cha), "",
	)
}

// handleSheetCompanionDelete removes one companion and re-renders the main
// sheet's Companions box. The companion's own popup window, if still open,
// is left as-is (its own save calls will 404 harmlessly) — there is no way
// to reach into another window from here, and closing it isn't this app's
// job.
func (s *server) handleSheetCompanionDelete(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := charstore.DeleteCompanion(s.charDB, id, cid); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete companion:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// handlePuppetMatryoshkaSplit implements Matryoshka Framework's own "split
// into 1 to 3 bodies (on a rest)" action — see
// charstore.SplitCompanionIntoBodies' own doc for the split itself. Scoped
// to kind="puppet" the same way every other Puppet Master-only action on
// this companion is (e.g. handlePuppetUpgradeAdd), since only a Puppet Tool
// can hold a Matryoshka Framework pick at all.
func (s *server) handlePuppetMatryoshkaSplit(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for matryoshka split:", err)
		return
	}
	if companion.Kind != "puppet" {
		http.Error(w, "not a puppet", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	count, err := strconv.Atoi(strings.TrimSpace(r.FormValue("count")))
	if err != nil || count < 2 || count > 3 {
		http.Error(w, "count must be 2 or 3", http.StatusBadRequest)
		return
	}
	if err := charstore.SplitCompanionIntoBodies(s.charDB, id, cid, count); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("split companion into bodies:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetMatryoshkaMerge implements Matryoshka Framework's own
// "re-merge on a rest" action — see charstore.MergeCompanionBodies' own
// doc. The template renders a Merge button on every body's own card, but
// each one's form always posts to the group's primary id (its own
// MatryoshkaGroupID), never its own possibly-sibling id — so this handler
// never has to resolve "which body in the group is the primary" itself.
func (s *server) handlePuppetMatryoshkaMerge(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := charstore.MergeCompanionBodies(s.charDB, id, cid); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("merge companion bodies:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// companionFeatureRef is one read-only rules-reference row shown in a
// companion's popup — a Puppet Master subclass feature (Level set, Rank
// empty) or a summon tribe's rank-gated special feature (Rank set, Level
// zero). Locked mirrors the main sheet's own level gating: shown either way
// so the player can see what is coming, not hidden until reached.
type companionFeatureRef struct {
	Name        string
	Level       int
	Rank        string
	Description string
	Locked      bool
}

// puppetMasterSubclassSlug resolves the character's own chosen Puppet
// Master subclass, if any — shared by loadPuppetMasterReference (the
// popup/reference panel) and loadPuppetUpgradeTiers (the Puppets tab's
// upgrade catalog, which needs the same slug to scope the subclass-
// exclusive upgrade list). Returns ("", "", nil) if the character has no
// Puppet Master subclass chosen yet.
func (s *server) puppetMasterSubclassSlug(characterID int64) (slug, name string, err error) {
	const puppetMasterSlug = "class/puppet-master"

	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return "", "", err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var s string
		if err := subRows.Scan(&s); err != nil {
			subRows.Close()
			return "", "", err
		}
		subclassSlugs = append(subclassSlugs, s)
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
		if classSlug == puppetMasterSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// summonRankOrder ranks jutsu_ranks.rank for comparison without a DB round
// trip — the same six letters that table has always had (see
// 0008_summon_tribes.sql), cheap enough to hardcode rather than query.
var summonRankOrder = map[string]int{"E": 0, "D": 1, "C": 2, "B": 3, "A": 4, "S": 5}

// highestRankForLevel is the universal, level-only, zero-exception D/C/B/A/S
// formula confirmed by reading every class's own printed level table (see
// this project's own notes on v_class_levels.highest_rank_known): levels
// 1-4 D-Rank, 5-8 C-Rank, 9-12 B-Rank, 13-16 A-Rank, 17-20 S-Rank. Used here
// to gate a kind="summon" companion's tribe features/progression by the
// character's total level, the same rank a Summoning Technique cast at that
// level could actually reach — no per-class query needed since the formula
// doesn't vary by class.
func highestRankForLevel(level int) string {
	switch {
	case level >= 17:
		return "S"
	case level >= 13:
		return "A"
	case level >= 9:
		return "B"
	case level >= 5:
		return "C"
	default:
		return "D"
	}
}

type summonNamedText struct {
	Name        string
	Description string
}

type summonProgressionRow struct {
	Rank      string
	SizeText  string
	StatsText string
	Locked    bool
}

// summonTribeReference is the read-only panel shown on a kind="summon"
// companion's popup once a tribe is picked: the tribe's own printed stat
// formulas/attacks/roles (always shown, ungated — they describe the tribe
// itself, not something leveled into) plus its rank-gated special features
// and stat progression, gated by the character's current
// highestRankForLevel.
type summonTribeReference struct {
	Name                 string
	SummonType           string
	Description          string
	DefensiveAbility     string
	SavingThrows         string
	Skills               string
	Senses               string
	JutsuSaveDCText      string
	JutsuAttackBonusText string
	JutsuSpecialtyText   string
	Attacks              []summonNamedText
	Roles                []summonNamedText
	Features             []companionFeatureRef
	Progression          []summonProgressionRow
}

func (s *server) loadSummonTribeReference(tribeSlug string, characterLevel int) (*summonTribeReference, error) {
	var ref summonTribeReference
	err := s.rulesDB.QueryRow(`
		SELECT name, summon_type, COALESCE(description, ''), COALESCE(defensive_ability, ''),
		       COALESCE(saving_throws, ''), COALESCE(skills, ''), COALESCE(senses, ''),
		       COALESCE(jutsu_save_dc_text, ''), COALESCE(jutsu_attack_bonus_text, ''), COALESCE(jutsu_specialty_text, '')
		FROM summon_tribes WHERE slug = ?`, tribeSlug,
	).Scan(&ref.Name, &ref.SummonType, &ref.Description, &ref.DefensiveAbility,
		&ref.SavingThrows, &ref.Skills, &ref.Senses,
		&ref.JutsuSaveDCText, &ref.JutsuAttackBonusText, &ref.JutsuSpecialtyText)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	currentOrder := summonRankOrder[highestRankForLevel(characterLevel)]

	attackRows, err := s.rulesDB.Query(
		`SELECT name, description FROM summon_tribe_attacks WHERE tribe_slug = ? ORDER BY name`, tribeSlug)
	if err != nil {
		return nil, err
	}
	for attackRows.Next() {
		var t summonNamedText
		if err := attackRows.Scan(&t.Name, &t.Description); err != nil {
			attackRows.Close()
			return nil, err
		}
		ref.Attacks = append(ref.Attacks, t)
	}
	attackRows.Close()
	if err := attackRows.Err(); err != nil {
		return nil, err
	}

	roleRows, err := s.rulesDB.Query(
		`SELECT role_name, description FROM summon_tribe_roles WHERE tribe_slug = ? ORDER BY role_name`, tribeSlug)
	if err != nil {
		return nil, err
	}
	for roleRows.Next() {
		var t summonNamedText
		if err := roleRows.Scan(&t.Name, &t.Description); err != nil {
			roleRows.Close()
			return nil, err
		}
		ref.Roles = append(ref.Roles, t)
	}
	roleRows.Close()
	if err := roleRows.Err(); err != nil {
		return nil, err
	}

	featRows, err := s.rulesDB.Query(
		`SELECT name, rank, description FROM summon_tribe_features WHERE tribe_slug = ? ORDER BY sort_order`, tribeSlug)
	if err != nil {
		return nil, err
	}
	for featRows.Next() {
		var f companionFeatureRef
		if err := featRows.Scan(&f.Name, &f.Rank, &f.Description); err != nil {
			featRows.Close()
			return nil, err
		}
		f.Locked = summonRankOrder[f.Rank] > currentOrder
		ref.Features = append(ref.Features, f)
	}
	featRows.Close()
	if err := featRows.Err(); err != nil {
		return nil, err
	}

	progRows, err := s.rulesDB.Query(
		`SELECT rank, COALESCE(size_text, ''), stats_text FROM summon_tribe_progression WHERE tribe_slug = ?`, tribeSlug)
	if err != nil {
		return nil, err
	}
	for progRows.Next() {
		var p summonProgressionRow
		if err := progRows.Scan(&p.Rank, &p.SizeText, &p.StatsText); err != nil {
			progRows.Close()
			return nil, err
		}
		p.Locked = summonRankOrder[p.Rank] > currentOrder
		ref.Progression = append(ref.Progression, p)
	}
	progRows.Close()
	if err := progRows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(ref.Progression, func(i, k int) bool {
		return summonRankOrder[ref.Progression[i].Rank] < summonRankOrder[ref.Progression[k].Rank]
	})

	return &ref, nil
}

// summonTribeOption is one entry in the companion popup's tribe picker.
type summonTribeOption struct {
	Slug string
	Name string
}

func (s *server) loadSummonTribeOptions() ([]summonTribeOption, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name FROM summon_tribes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []summonTribeOption
	for rows.Next() {
		var t summonTribeOption
		if err := rows.Scan(&t.Slug, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// summonCompanionView is one companion (any kind) plus its own summon-tribe
// reference (nil for a companion with no tribe chosen — which in practice
// means every kind except "summon", since only that kind's picker ever
// writes SummonTribeSlug) — each summon can have a different tribe, so
// unlike Puppets' single character-wide subclass reference, this is
// resolved per companion.
type summonCompanionView struct {
	charstore.Companion
	Reference       *summonTribeReference
	NinDogReference *ninDogReference
	TitanReference  *titanReference
	// Attacks: only populated for a kind companionSupportsStructuredAttacks
	// reports true for (nin-dog, titan, summon, custom — puppet has its own
	// richer card on the Puppets tab instead, see sheet_puppet_tab's own
	// doc). nil for any kind that hasn't reached structured attacks yet,
	// which keeps rendering the plain freeform textarea.
	Attacks []companionAttackRow
}

// summonsTabData is everything the main sheet's Companions tab (labeled
// "Companions" in the UI; internal identifiers here and in
// character_sheet.html/sheet-puppets.js keep the older "summons"/"summon"
// naming from before the tab was broadened, rather than a repo-wide rename)
// and its "sheet_summon_tab" fragment need. Companions is the one
// companion-related tab shown regardless of class — unlike the Puppets tab
// (gated on Puppet Master levels), it lists every companion the character
// has, of any kind, so a companion is never invisible on every tab-level
// view just because of which kind got picked when it was added (see
// handleSheetCompanionAdd's own kind whitelist for the full kind list).
// Deliberately minimal beyond that — see the tab panel's own comment for
// what's explicitly deferred.
type summonsTabData struct {
	Tribes     []summonTribeOption
	Companions []summonCompanionView
}

func (s *server) loadSummonsTabData(characterID int64, sheet *charsheet.Sheet) (summonsTabData, error) {
	var data summonsTabData
	tribes, err := s.loadSummonTribeOptions()
	if err != nil {
		return data, err
	}
	data.Tribes = tribes

	all, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return data, err
	}
	for _, c := range all {
		view := summonCompanionView{Companion: c}
		if c.Kind == "summon" && c.SummonTribeSlug != "" {
			ref, err := s.loadSummonTribeReference(c.SummonTribeSlug, sheet.Level)
			if err != nil {
				return data, err
			}
			view.Reference = ref
		}
		if c.Kind == "nin-dog" {
			ref, err := s.loadNinDogReference(characterID, c, sheet.Level)
			if err != nil {
				return data, err
			}
			view.NinDogReference = ref

			attacks, err := charstore.ListCompanionAttacks(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Attacks = append(composeCompanionAttacks(attacks, c, sheet.ProficiencyBonus),
				ninDogBiteAttack(c, sheet.ProficiencyBonus))
		}
		if c.Kind == "titan" {
			ref, err := s.loadTitanReference(characterID, sheet, c)
			if err != nil {
				return data, err
			}
			view.TitanReference = ref

			attacks, err := charstore.ListCompanionAttacks(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Attacks = append(composeCompanionAttacks(attacks, c, sheet.ProficiencyBonus),
				titanBashAttack(c, sheet.ProficiencyBonus))
		}
		if c.Kind == "summon" || c.Kind == "custom" || c.Kind == "snb" {
			// None of these three kinds has a computed baseline attack the
			// way Bite/Bash do (no rules-defined natural weapon or stat
			// block to derive one from — see companionStructuredAttackKinds'
			// own doc), so this is just the player-added rows, with nothing
			// appended.
			attacks, err := charstore.ListCompanionAttacks(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Attacks = composeCompanionAttacks(attacks, c, sheet.ProficiencyBonus)
		}
		data.Companions = append(data.Companions, view)
	}
	return data, nil
}

// handleCompanionSheet serves one companion's own standalone popup page —
// opened via companion-popup.js's window.open, not plain navigation, so it
// renders through the bare layout (no main nav/dice-roller/etc — see
// companion_sheet.html's own doc comment).
func (s *server) handleCompanionSheet(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
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
		log.Println("compute sheet for companion page:", err)
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion:", err)
		return
	}

	data := map[string]any{
		"Title":         companion.Name + " — " + sheet.Name,
		"CharacterID":   id,
		"CharacterName": sheet.Name,
		"Companion":     companion,
	}

	switch companion.Kind {
	case "puppet":
		// The popup is a quick reference for combat, not the full editing
		// experience (upgrade picking, the subclass feature catalog) — that
		// lives on the main sheet's Puppets tab, and the class-wide
		// "Puppet Master Reference" catalog now lives in its own minimized
		// sidebar panel there instead (see character_sheet.html's
		// sheet-puppet-reference-panel). What the popup DOES show, read-
		// only and rollable: the resolved Puppet Skills list and whatever
		// Attacks are already on this puppet (from the tab or an upgrade).
		// Armor Chassis (Juggernaut Armor) is Purple Technique Juggernaut's
		// own 2nd-level subclass feature — every other subclass's popup
		// gets no chassis options at all, matching loadPuppetsTabData's
		// identical gate on the main sheet's Puppets tab.
		subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load puppet master subclass for popup:", err)
			return
		}
		var chassisOptions []puppetArmorChassis
		if puppetSubclassColorBySlug[subclassSlug] == "Purple" {
			chassisOptions, err = s.loadPuppetArmorChassisOptions()
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load armor chassis options:", err)
				return
			}
		}
		data["ArmorChassisOptions"] = chassisOptions

		baseTraits, err := s.loadPuppetToolStatBlock()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load puppet tool stat block:", err)
			return
		}
		data["BaseTraits"] = baseTraits

		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for popup:", err)
			return
		}
		view := composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus)

		upgradePicks, err := charstore.ListCompanionUpgrades(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion upgrades for popup:", err)
			return
		}
		upgradeChoices, err := charstore.ListCompanionUpgradeChoices(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion upgrade choices for popup:", err)
			return
		}
		view = append(view, puppetUpgradeGrantedAttacks(sheet, upgradePicks, upgradeChoices)...)

		foundationPicks := resolvePuppetFoundationPicks(upgradePicks, upgradeChoices)
		expectedAbilityScores := puppetFoundationExpectedAbilityScores(baseTraits, foundationPicks)
		attackRollBonus, critRangeThreshold := 0, 20
		critDamageBonus := 0
		var foundationTraits []string
		for _, fp := range foundationPicks {
			if row := puppetFoundationWeaponAttack(expectedAbilityScores, sheet.ProficiencyBonus, fp); row != nil {
				view = append(view, *row)
			}
			attackRollBonus += fp.Entry.FlatAttackRollBonus
			if fp.Entry.RoleEffect != nil {
				if fp.Entry.RoleEffect.AttackRollBonus != nil {
					attackRollBonus += fp.Entry.RoleEffect.AttackRollBonus(sheet.ProficiencyBonus)
				}
				if fp.Entry.RoleEffect.CritRangeBonus > 0 && 20-fp.Entry.RoleEffect.CritRangeBonus < critRangeThreshold {
					critRangeThreshold = 20 - fp.Entry.RoleEffect.CritRangeBonus
				}
				if fp.Entry.RoleEffect.CritDamageBonus != nil {
					critDamageBonus += fp.Entry.RoleEffect.CritDamageBonus(sheet.ProficiencyBonus)
				}
				if fp.Entry.RoleEffect.TextBonus != nil {
					foundationTraits = append(foundationTraits, fp.Entry.RoleEffect.TextBonus(sheet.ProficiencyBonus))
				}
			}
			foundationTraits = append(foundationTraits, fp.Entry.ReferenceTraits...)
			if fp.Entry.SageCreatureFramework {
				bestialTraits, err := s.bestialFrameworkReferenceTraits(fp)
				if err != nil {
					http.Error(w, "database error", http.StatusInternalServerError)
					log.Println("load bestial framework reference traits for popup:", err)
					return
				}
				foundationTraits = append(foundationTraits, bestialTraits...)
			}
			data["FoundationCatalogLabel"] = fp.Entry.Catalog
			data["FoundationEntryName"] = fp.Entry.Name
		}
		for i := range view {
			view[i].AttackTotal += attackRollBonus
			view[i].CritRangeThreshold = critRangeThreshold
			view[i].CritDamageBonus = critDamageBonus
		}
		data["Attacks"] = view
		data["FoundationTraits"] = foundationTraits

		generalizedPicks, err := charstore.ListGeneralizedSkills(s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load generalized skills for popup:", err)
			return
		}
		upgradeSlugs := make([]string, len(upgradePicks))
		for i, u := range upgradePicks {
			upgradeSlugs[i] = u.UpgradeEntrySlug
		}
		data["PuppetSkills"] = resolvePuppetSkills(sheet, companion, generalizedPicks, upgradeSlugs,
			puppetFoundationSkillAbilityOverride(foundationPicks))

		resolvedFeatureChoices, err := features.LoadFeatureChoices(s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load feature choices for companion popup:", err)
			return
		}
		if bonus := puppetElevatedDesignAbilityBonus(resolvedFeatureChoices); len(bonus) > 0 {
			data["ElevatedDesignBonus"] = bonus
		}
		if bonus := puppetToolASIAbilityBonus(resolvedFeatureChoices); len(bonus) > 0 {
			data["PuppetToolASIBonus"] = bonus
		}
		allCompanions, err := charstore.ListCompanions(s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companions for symphony enhancement view:", err)
			return
		}
		symphonyEnhancementViews, err := s.puppetSymphonyEnhancementViews(id, resolvedFeatureChoices, allCompanions)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load symphony enhancement view for companion popup:", err)
			return
		}
		data["SymphonyEnhancement"] = symphonyEnhancementViews[cid]
	case "summon":
		tribes, err := s.loadSummonTribeOptions()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load summon tribe options:", err)
			return
		}
		data["SummonTribes"] = tribes
		if companion.SummonTribeSlug != "" {
			ref, err := s.loadSummonTribeReference(companion.SummonTribeSlug, sheet.Level)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load summon tribe reference:", err)
				return
			}
			data["SummonReference"] = ref
		}

		// Same "read-only quick reference, editing happens on the tab"
		// treatment puppet/nin-dog/titan's own popup cases give structured
		// Attacks — no computed baseline attack to append here (see
		// loadSummonsTabData's identical summon/custom branch), just
		// whatever the player has added.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for summon popup:", err)
			return
		}
		data["Attacks"] = composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus)
	case "custom":
		// "Custom" shows no rules-reference panel of any kind (see
		// 0017_companions.sql's own doc) — structured Attacks is the only
		// popup content this kind gets beyond the shared stat fields.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for custom popup:", err)
			return
		}
		data["Attacks"] = composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus)
	case "snb":
		// Same bare "no rules-reference panel" treatment as "custom" above
		// — a Scientific Ninja Beast has real rules-defined stats (see
		// scienceNinSNBSpecialistData), but this app doesn't model a second
		// full stat block the way it does for Puppet/Nin-Dog/Titan; it gets
		// the shared plain stat fields plus structured Attacks like any
		// other bare companion kind.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for snb popup:", err)
			return
		}
		data["Attacks"] = composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus)
	case "nin-dog":
		ref, err := s.loadNinDogReference(id, companion, sheet.Level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load nin-dog reference:", err)
			return
		}
		data["NinDogReference"] = ref

		// Same "read-only quick reference, editing happens on the tab"
		// treatment puppet's own popup case gives structured Attacks above.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for nin-dog popup:", err)
			return
		}
		data["Attacks"] = append(composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus),
			ninDogBiteAttack(companion, sheet.ProficiencyBonus))
	case "titan":
		ref, err := s.loadTitanReference(id, sheet, companion)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load titan reference:", err)
			return
		}
		data["TitanReference"] = ref

		// Same "read-only quick reference, editing happens on the tab"
		// treatment nin-dog's own popup case gives structured Attacks above.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for titan popup:", err)
			return
		}
		data["Attacks"] = append(composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus),
			titanBashAttack(companion, sheet.ProficiencyBonus))
	}

	s.render(w, "companion_sheet.html", data)
}

// parseOptionalInt turns a companion popup number field's raw text into a
// sql.NullInt64 — blank means "not entered yet" (SQL NULL), matching every
// stat on a fresh companion. A non-blank value that doesn't parse is a
// client bug (the inputs are type="number"), not a normal empty state, so
// it's rejected rather than silently dropped.
func parseOptionalInt(s string) (sql.NullInt64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullInt64{}, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: v, Valid: true}, nil
}

// handleCompanionSave is the companion popup's whole-form autosave target —
// companion-sheet.js resubmits every field on any one field's blur, the
// same shape sheet-bio.js already uses for the main sheet's Bio/Notes
// fields. Answers 204 with no body: the popup never navigates.
func (s *server) handleCompanionSave(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	summonTribeSlug := strings.TrimSpace(r.FormValue("summon_tribe_slug"))

	// Armor Chassis is Purple Technique Juggernaut's own 2nd-level subclass
	// feature — the popup's own picker already only offers it to a Purple
	// character (see handleCompanionSheet's identical gate), but this is
	// the real enforcement: a raw POST bypassing that UI must not be able
	// to give a non-Purple Puppet Tool the Juggernaut Armor AC formula.
	armorChassis := strings.TrimSpace(r.FormValue("armor_chassis"))
	if armorChassis != "" {
		companion, err := charstore.GetCompanion(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion for armor chassis gate:", err)
			return
		}
		if companion.Kind != "puppet" {
			armorChassis = ""
		} else {
			subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load puppet master subclass for armor chassis gate:", err)
				return
			}
			if puppetSubclassColorBySlug[subclassSlug] != "Purple" {
				armorChassis = ""
			}
		}
	}

	// hp_current/ac/hp_max are deliberately not read here — each has its own
	// endpoint (handleCompanionHP/handleCompanionIntField) and its own
	// <form> in the template. See charstore.SetCompanionFields' doc for why
	// folding any of them into this whole-form save would silently wipe it
	// on every unrelated field's blur.
	var speed, flySpeed, str, dex, con, intScore, wis, cha sql.NullInt64
	for field, dest := range map[string]*sql.NullInt64{
		"speed": &speed, "fly_speed": &flySpeed,
		"str_score": &str, "dex_score": &dex, "con_score": &con,
		"int_score": &intScore, "wis_score": &wis, "cha_score": &cha,
	} {
		v, err := parseOptionalInt(r.FormValue(field))
		if err != nil {
			http.Error(w, "bad "+field, http.StatusBadRequest)
			return
		}
		*dest = v
	}

	err := charstore.SetCompanionFields(s.charDB, id, cid, name, summonTribeSlug,
		speed, flySpeed, str, dex, con, intScore, wis, cha,
		r.FormValue("attacks"), r.FormValue("traits"), r.FormValue("notes"),
		armorChassis, r.FormValue("is_armor_form") == "1",
		strings.TrimSpace(r.FormValue("size")), strings.TrimSpace(r.FormValue("nin_dog_breed")),
		strings.TrimSpace(r.FormValue("titan_specialization")),
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("save companion:", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCompanionHP is the companion popup's HP box endpoint (form field
// "value") — deliberately its own request, separate from
// handleCompanionSave's whole-form autosave, mirroring the main sheet's
// Ryo/HP boxes: a leading "+"/"-" adjusts hp_current by that amount
// (floored at 0, via charstore.AddCompanionHP), a bare number sets it
// outright, and an empty value clears it back to blank. Splitting this out
// is load-bearing, not just parallel structure — the whole-form save
// resubmits every field's CURRENT typed text on every field's blur, so if
// "+3" stayed in the HP input after being applied once, the next unrelated
// field blur (e.g. leaving the Notes box) would resubmit "+3" and apply the
// delta again. Answering with the new plain numeric value (or "" if
// cleared) lets companion-sheet.js overwrite the input so it never carries
// a stale "+3" into a later save.
func (s *server) handleCompanionHP(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("value"))

	if raw == "" {
		if err := charstore.SetCompanionHP(s.charDB, id, cid, sql.NullInt64{}); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("clear companion hp:", err)
			return
		}
		w.Write(nil)
		return
	}

	relative := raw[0] == '+' || raw[0] == '-'
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}

	var newValue int64
	if relative {
		newValue, err = charstore.AddCompanionHP(s.charDB, id, cid, value)
	} else {
		newValue = value
		err = charstore.SetCompanionHP(s.charDB, id, cid, sql.NullInt64{Int64: value, Valid: true})
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set companion hp:", err)
		return
	}
	w.Write([]byte(strconv.FormatInt(newValue, 10)))
}

// handleCompanionIntField returns the endpoint for one of a companion's own
// delta-editable int fields (AC, HP-max) — same request/response shape as
// handleCompanionHP just above (form field "value": a leading "+"/"-"
// adjusts, a bare number sets outright, empty clears, and the response is
// always the new plain value so the field never carries a stale "+1" into
// a later, unrelated blur), generalized via charstore.AddCompanionIntField/
// SetCompanionIntField's own field whitelist. This is what replaced the
// "Use computed" hint button: a fresh puppet's AC/HP-max already start at
// the computed baseline (see prefillPuppetStatDefaults/loadPuppetsTabData's
// backfill), and this lets a player nudge them from there — typing "+1"
// after an Armor Chassis pick, for instance — without doing the
// arithmetic themselves.
func (s *server) handleCompanionIntField(field string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, cid, ok := parseCharacterAndCompanionID(w, r)
		if !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		raw := strings.TrimSpace(r.FormValue("value"))

		if raw == "" {
			if err := charstore.SetCompanionIntField(s.charDB, id, cid, field, sql.NullInt64{}); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("clear companion "+field+":", err)
				return
			}
			w.Write(nil)
			return
		}

		relative := raw[0] == '+' || raw[0] == '-'
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "bad value", http.StatusBadRequest)
			return
		}

		var newValue int64
		if relative {
			newValue, err = charstore.AddCompanionIntField(s.charDB, id, cid, field, value)
		} else {
			newValue = value
			err = charstore.SetCompanionIntField(s.charDB, id, cid, field, sql.NullInt64{Int64: value, Valid: true})
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set companion "+field+":", err)
			return
		}
		w.Write([]byte(strconv.FormatInt(newValue, 10)))
	}
}
