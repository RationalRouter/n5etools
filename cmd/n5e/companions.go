package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file is the whole Puppet/Summon/custom companion feature: a small,
// separate popup-window sheet (companion_sheet.html) a character can open
// per Puppet Tool or summoned creature they field, alongside the main sheet
// but never nested inside it.
//
// charstore.Companion's stat fields are player-editable, but that no longer
// means "manual-only": kinds with a documented, kind-specific formula
// (kind="titan" via titanAC/titanMaxHP/titanBashAttack, kind="nin-dog" via
// ninDogAC/ninDogMaxHP/ninDogBiteAttack, kind="snb" via snbAC/snbMaxHP/
// snbBiteAttack, and Puppet Tools via puppetToolDefaultAC and the other
// puppet*Attack functions) get those fields auto-computed at creation and on
// the events that would change them (level-up, chassis/specialization pick,
// etc.), same as the main sheet. A player can still overwrite any of those
// fields afterward — the computed value is only ever a starting default, not
// a locked value — which is what actually accommodates the "every chassis/
// tribe is different" variance this comment used to cite as the reason nothing
// was computed at all. Only kind="summon" (a chosen Summon Tribe's creature)
// and kind="custom" remain fully manual: no per-tribe formula is modeled for
// generic summons, so every stat on those two kinds is still a plain
// player-entered value with no computed default behind it.
//
// The only other thing this file computes is which of the character's OWN
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

// companionAttacksTabLabel names the main-sheet tab a companion's own
// Attacks are added/removed from, for the popup's read-only empty-state
// hint (companion_fields.html's ReadOnlyAttacks branch) to point at — the
// same puppet-vs-everything-else split as companionAttacksFragment above,
// just spelled out as the tab's display name instead of its fragment id.
func companionAttacksTabLabel(kind string) string {
	if kind == "puppet" {
		return "Puppets"
	}
	return "Companions"
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
	if kind == "snb" {
		if err := s.prefillSNBStatDefaults(id, companionID); err != nil {
			log.Println("prefill snb stat defaults:", err)
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
	if err := charstore.SetCompanionStatDefaults(s.charDB, characterID, companionID,
		ac, hpMax, int64(baseline.Speed), sql.NullInt64{},
		int64(baseline.Str), int64(baseline.Dex), int64(baseline.Con),
		int64(baseline.Int), int64(baseline.Wis), int64(baseline.Cha), "",
	); err != nil {
		return err
	}
	return charstore.SetCompanionHP(s.charDB, characterID, companionID, sql.NullInt64{Int64: hpMax, Valid: true})
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

	// Resistances/Immunities/ConditionImmunities: comma-joined free text,
	// matching titanReference/snbReference/ninDogReference's own
	// identically-named/-shaped fields exactly. Unlike Titan's own two
	// hardcoded upgrade checks, the generic Summon Tribe catalog spans 17
	// tribes with dozens of rank-gated features, so these are resolved via
	// summonTribeResistanceCatalog — a hand-transcribed lookup table, not a
	// live text parse of each feature's own prose — see that table's own
	// header doc for exactly which rows across the whole catalog qualify
	// and which don't. "" until this tribe has at least one currently-
	// unlocked (see Features' own Locked flag) qualifying feature, which the
	// "summon_tribe_reference" template's own {{if}} guards read as "no row
	// at all" rather than an empty one — same convention titan_reference
	// already uses.
	Resistances         string
	Immunities          string
	ConditionImmunities string
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
	ref.Resistances, ref.Immunities, ref.ConditionImmunities = summonTribeResistancesImmunities(tribeSlug, ref.Features)

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

// summonTribeResistanceGrant is one summon_tribe_features row's own
// contribution to summonTribeReference's Resistances/Immunities/
// ConditionImmunities fields (and ninDogReference's identically-shaped
// fields — nindog.go's own ninDogResistanceCatalog reuses this same struct
// for its own, currently-empty, Dog/Wolf-scoped table). Resistance/
// Immunity/ConditionImmunity each hold the exact damage type(s) or
// condition name(s) that ONE named feature grants the summon itself, ""
// for whichever of the three that feature doesn't touch.
type summonTribeResistanceGrant struct {
	Resistance        string
	Immunity          string
	ConditionImmunity string
}

// summonTribeResistanceCatalog: tribe slug -> feature name -> what that
// feature grants the summon itself, hand-transcribed (2026-08-25) from
// every summon_tribe_features row across the FULL catalog (all 17 tribes,
// every rank) whose own name or description mentions "resist"/"immun":
//
//	SELECT tribe_slug, name, rank, description FROM summon_tribe_features
//	WHERE description LIKE '%resist%' OR description LIKE '%immun%'
//	   OR name LIKE '%resist%' OR name LIKE '%immun%'
//
// That query returned 36 rows. Roughly half of them are deliberately NOT
// in this table, because they don't grant a genuine personal, passive
// damage resistance, damage immunity, or condition immunity to the summon
// itself:
//   - offensive "ignores the TARGET's own Resistance/Immunity/Damage
//     Reduction" clauses grant the summon nothing defensive at all (Boar's
//     Unstoppable Inoshishi, Dog/Wolf's own Iron Fangs — "cannot have its
//     damage resisted", confirmed the same offensive shape ninDogReference's
//     own investigation already excluded it for — Monkey/Primate's Power
//     Stance, Rat's Viral Contagion and Extreme Poison, Shark/Predator
//     Fish's Unstoppable Violence, Tiger/Lion's Presence of Power, Turtle's
//     Long Standing Focus);
//   - a couple grant immunity to something OTHER than the summon itself
//     (Hare/Rabbit's Usagi Tribalism shields its ALLIES from its own
//     Ninjutsu; Snake's Devils Glare makes a creature it just Stunned
//     immune to that same feature for 24 hours, not the summon);
//   - a few name a real passive defensive trait that still isn't a damage
//     type or a named condition, so it doesn't fit any of the three tracked
//     fields (Insect Swarm's The Swarm — immune to being targeted at all;
//     Snake's Hiss — immune to penalties on its own attack rolls; Spider's
//     Spider Climb — immune to difficult terrain; Turtle's Kame Shell
//     Armor — immune to critical hits);
//   - Toad's Gama Courage ("advantage on saving throws to resist the Fear
//     condition") is the same shape as titanReference's own
//     ResistanceAdvantageText, but building that whole side-channel
//     mechanism for exactly one summon-tribe row isn't justified any more
//     here than titan.go's own doc says it was for a second, wider Titan
//     mechanic — see that field's own doc.
//
// Every entry that IS here reads verbatim off that feature's own
// description. summonTribeResistancesImmunities below only folds in an
// entry whose feature is currently UNLOCKED (loadSummonTribeReference's own
// per-feature rank gate, already computed before this table is ever
// consulted) — the same "cumulative once unlocked" model the rest of this
// panel's own Features list already uses, so a tribe with more than one
// qualifying feature unlocked at once (e.g. Slug's own Resistance at
// C-Rank plus Immune at A-Rank) shows both simultaneously rather than only
// the highest-rank one.
var summonTribeResistanceCatalog = map[string]map[string]summonTribeResistanceGrant{
	"summon/bear": {
		"Kuma King":  {ConditionImmunity: "Physical Conditions"},
		"Kuma Queen": {ConditionImmunity: "Sensory Conditions"},
	},
	"summon/boar": {
		"Inoshishi Force": {ConditionImmunity: "Fear, Charm"},
	},
	"summon/deer": {
		"Peace": {ConditionImmunity: "All conditions from hostile sources (cannot cast damaging jutsu while active)"},
	},
	"summon/hare-rabbit": {
		"Usagi Flexibility": {ConditionImmunity: "Grappled, Restrained"},
	},
	"summon/insect-swarm": {
		"The Plague": {ConditionImmunity: "All conditions (and penalties to its own attacks or damage)"},
	},
	"summon/lizard": {
		"Tokage’s Venomous Flesh": {Immunity: "Poison, Acid"},
		"Tokage’s Will":           {ConditionImmunity: "Mental, Sensory"},
	},
	"summon/monkey-primate": {
		"Monkey Business": {ConditionImmunity: "Physical or Mental (chosen when it triggers)"},
	},
	"summon/ox-ram": {
		"Ushi Calmness": {ConditionImmunity: "Mental"},
	},
	"summon/shark-predator-fish": {
		// "immune to the fear condition and effects that would push it" —
		// the forced-movement half is labeled "Movement Effects", the same
		// label titanResistancesImmunities' own Hulking Strength entry
		// already uses for the identical "immune to being forcibly moved"
		// shape, so the two read consistently wherever a player sees them.
		"Shaaku Violence": {ConditionImmunity: "Fear", Immunity: "Movement Effects (forced push)"},
		"Shark Skin":      {Immunity: "Cold"},
	},
	"summon/slug": {
		"Namekuji Slimy Body": {ConditionImmunity: "Grappled, Restrained"},
		"Resistance":          {Resistance: "Slashing, Piercing"},
		"Immune":              {Immunity: "Cold, Poison"},
	},
	"summon/spider": {
		"Golden Web": {ConditionImmunity: "Blinded, Dazzled, Dazed"},
	},
	"summon/tiger-lion": {
		"Tora Anger": {ConditionImmunity: "Fear, Charmed"},
	},
	"summon/toad": {
		"Gama Slime": {ConditionImmunity: "Grappled, Restrained"},
		"Gama Wart":  {ConditionImmunity: "Envenomed, Bleeding"},
	},
	"summon/turtle": {
		"Impregnable":      {Resistance: "Bludgeoning, Slashing, Piercing"},
		"Unbreakable":      {ConditionImmunity: "Physical, Elemental"},
		"Unending Dignity": {ConditionImmunity: "Mental"},
	},
}

// summonTribeResistancesImmunities folds summonTribeResistanceCatalog's
// entries for tribeSlug into the same Resistances/Immunities/
// ConditionImmunities shape titanReference/snbReference already give their
// own companion kinds, respecting each feature's own Locked flag (already
// computed by loadSummonTribeReference's rank-gate loop just above its own
// call site) so a resistance/immunity from a not-yet-reached rank doesn't
// show before the character's own summon rank actually reaches it. Returns
// "" for a field neither this tribe nor its current rank grants anything
// for yet.
func summonTribeResistancesImmunities(tribeSlug string, features []companionFeatureRef) (resistances, immunities, conditionImmunities string) {
	grants := summonTribeResistanceCatalog[tribeSlug]
	if len(grants) == 0 {
		return "", "", ""
	}
	var resistanceParts, immunityParts, conditionParts []string
	for _, f := range features {
		if f.Locked {
			continue
		}
		g, ok := grants[f.Name]
		if !ok {
			continue
		}
		if g.Resistance != "" {
			resistanceParts = append(resistanceParts, g.Resistance)
		}
		if g.Immunity != "" {
			immunityParts = append(immunityParts, g.Immunity)
		}
		if g.ConditionImmunity != "" {
			conditionParts = append(conditionParts, g.ConditionImmunity)
		}
	}
	return strings.Join(resistanceParts, ", "), strings.Join(immunityParts, ", "), strings.Join(conditionParts, ", ")
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
	SNBReference    *snbReference
	// Attacks: only populated for a kind companionSupportsStructuredAttacks
	// reports true for (nin-dog, titan, summon, custom — puppet has its own
	// richer card on the Puppets tab instead, see sheet_puppet_tab's own
	// doc). nil for any kind that hasn't reached structured attacks yet,
	// which keeps rendering the plain freeform textarea.
	Attacks []companionAttackRow
	// Saves: computed for every kind (cheap — six subtractions and a map
	// lookup), even "puppet", which this view struct can also represent when
	// a Puppet Tool shows up in the Companions tab's own no-kind-filter list
	// (see loadSummonsTabData's own doc) — companion_fields.html's own
	// {{if ne .Companion.Kind "puppet"}} guard is what actually keeps the
	// Saving Throws box from rendering for that kind, not this field being
	// left unset, so there's no per-kind branching needed here at all.
	Saves companionSavesView
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
	// Collapsed: which companion ids are currently minimized on the
	// Companions tab (state_key "summons:collapsed" in
	// character_sheet_ui_state — see sheet-companion-collapse.js's own doc
	// for why this lives server-side rather than in localStorage). Read
	// fresh on every render, including a live fragment refresh, rather than
	// only at the initial page load, so a card's [open] attribute never
	// drifts from what was last actually saved — unlike sheet-layout.js's
	// own grid state, this needs no client-side reapply-after-swap pass at
	// all (see that file's own header comment).
	Collapsed map[int64]bool
}

func (s *server) loadSummonsTabData(characterID int64, sheet *charsheet.Sheet) (summonsTabData, error) {
	var data summonsTabData
	tribes, err := s.loadSummonTribeOptions()
	if err != nil {
		return data, err
	}
	data.Tribes = tribes

	uiState, err := charstore.GetSheetUIState(s.charDB, characterID)
	if err != nil {
		return data, err
	}
	data.Collapsed = parseCollapsedCompanionIDs(uiState["summons:collapsed"])

	all, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return data, err
	}
	for _, c := range all {
		view := summonCompanionView{Companion: c, Saves: companionSaves(c, sheet.ProficiencyBonus, sheet.Level)}
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

			// Re-read: loadNinDogReference just wrote fresh AC/HP-max/Speed/
			// Jutsu-Slots-Max/ability scores (auto-then-pin resolved) to this
			// companion's own row — c and view.Saves must reflect that same
			// write before Bite's own strMod (below) and companionSaves both
			// read off them, the same re-read-after-write pattern
			// loadPuppetsTabData already uses (puppets.go).
			c, err = charstore.GetCompanion(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Companion = c
			view.Saves = companionSaves(c, sheet.ProficiencyBonus, sheet.Level)

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

			// Re-read for the same reason as the nin-dog block above —
			// loadTitanReference just wrote fresh effective values this
			// render, and titanBashAttack/titanKnownWeaponUpgradeAttacks/
			// titanSpecialWeaponUpgradeAttacks all read the Titan's own
			// ability scores straight off c.
			c, err = charstore.GetCompanion(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Companion = c
			view.Saves = applyTitanSturdyFrameSaves(
				companionSaves(c, sheet.ProficiencyBonus, sheet.Level), ref, sheet.Abilities["int"].Modifier)

			attacks, err := charstore.ListCompanionAttacks(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			// titanSpecialWeaponUpgradeAttacks: Xo-16 Gatling/Greater
			// Missile Racks' own single-target mode, the separate bespoke
			// mechanism titanWeaponAttackClausePattern's own doc calls for
			// alongside titanKnownWeaponUpgradeAttacks' regex-driven
			// extraction — see titan.go's own header doc.
			// titanSaveOnlyUpgradeAttacks: Shinobifall/Thermite Launcher/
			// Critical Ejection's own NoAttackRoll damage-only rows.
			view.Attacks = append(append(append(append(composeCompanionAttacks(attacks, c, sheet.ProficiencyBonus),
				titanBashAttack(c, ref, sheet.ProficiencyBonus)),
				titanKnownWeaponUpgradeAttacks(ref, c, sheet.ProficiencyBonus, c.IsDemonFoe)...),
				titanSpecialWeaponUpgradeAttacks(ref, c, sheet, sheet.ProficiencyBonus, c.IsDemonFoe)...),
				titanSaveOnlyUpgradeAttacks(ref, sheet, c.IsDemonFoe)...)
		}
		if c.Kind == "snb" {
			ref, err := s.loadSNBReference(characterID, c, sheet)
			if err != nil {
				return data, err
			}
			view.SNBReference = ref

			// Re-read for the same reason as the nin-dog/titan blocks above.
			c, err = charstore.GetCompanion(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Companion = c
			view.Saves = companionSaves(c, sheet.ProficiencyBonus, sheet.Level)

			attacks, err := charstore.ListCompanionAttacks(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
			playerIntMod := sheet.Abilities["int"].Modifier
			view.Attacks = append(composeCompanionAttacks(attacks, c, sheet.ProficiencyBonus),
				snbBiteAttack(sheet.Level, playerIntMod, sheet.ProficiencyBonus, ref.AccuracyBonus))
		}
		if c.Kind == "summon" || c.Kind == "custom" {
			// Neither of these two kinds has a computed baseline attack the
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

// parseCollapsedCompanionIDs decodes the "summons:collapsed" UI-state blob
// (a plain JSON array of companion ids, e.g. "[3,5]" — written by
// sheet-companion-collapse.js) into a lookup set. Malformed or absent state
// (raw == "", a brand-new character with nothing ever minimized) yields a
// nil map, which index/lookup treats the same as "nothing collapsed" —
// there is no meaningful distinction between "never saved" and "saved as
// empty" for this particular blob, unlike save_proficiencies' own similar-
// looking ambiguity (see migration 0077's doc), since collapsing is freely
// re-toggleable state with no rules-text default to accidentally reapply.
func parseCollapsedCompanionIDs(raw string) map[int64]bool {
	if raw == "" {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
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
		"Title":          companion.Name + " — " + sheet.Name,
		"CharacterID":    id,
		"CharacterName":  sheet.Name,
		"Companion":      companion,
		"AttacksTabName": companionAttacksTabLabel(companion.Kind),
		// Saves: same "computed for every kind, hidden only by the
		// template's own kind check" treatment loadSummonsTabData's own
		// summonCompanionView.Saves field gets — see that field's doc.
		// ShowSaveToggles is deliberately never set here: the popup is a
		// read-only quick reference for combat (ReadOnlyAttacks' own doc
		// above states the same rule for structured Attacks), and its bare
		// layout (layout_bare.html) doesn't load sheet-toggles.js at all, so
		// a toggle form rendered here would silently fall through to a real,
		// unhandled page navigation on click instead of doing nothing.
		"Saves": companionSaves(companion, sheet.ProficiencyBonus, sheet.Level),
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
		//
		// Reuses loadPuppetsTabData wholesale rather than hand-reconstructing
		// this one companion's own view a second time — that used to leave
		// AC/Max HP/Speed/ability scores permanently stuck at zero here (this
		// handler never computed Expected* at all), which was harmless back
		// when those fields were still editable inputs bound to a stored
		// value, but is a real bug now that companion_fields.html renders
		// them as pure computed display (see this file's own header doc on
		// the Sync dropdown's removal). loadPuppetsTabData is also the
		// single place SetCompanionStatDefaults gets called to keep the
		// stored row in sync, so calling it here means opening this popup
		// after visiting the Puppets tab (the only way to reach it — see
		// companion-popup.js) always sees the same fresh numbers the tab
		// itself just wrote.
		tabData, err := s.loadPuppetsTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load puppets tab data for popup:", err)
			return
		}
		data["ArmorChassisOptions"] = tabData.ArmorChassisOptions
		data["BaseTraits"] = tabData.BaseTraits
		data["ReadOnlyAttacks"] = true
		var view *puppetCompanionView
		for i := range tabData.Companions {
			if tabData.Companions[i].ID == cid {
				view = &tabData.Companions[i]
				break
			}
		}
		if view != nil {
			data["PuppetView"] = view
			data["Attacks"] = view.Attacks
			data["FoundationCatalogLabel"] = view.FoundationCatalogLabel
			data["FoundationEntryName"] = view.FoundationEntryName
			data["FoundationTraits"] = view.FoundationTraits
			data["PuppetSkills"] = view.PuppetSkills
			data["SymphonyEnhancement"] = view.SymphonyEnhancement
		}
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
		ref, err := s.loadSNBReference(id, companion, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load snb reference:", err)
			return
		}
		data["SNBReference"] = ref

		// Re-read: loadSNBReference just wrote fresh effective values this
		// render (same auto-then-pin write loadSummonsTabData's own snb
		// block re-reads after) — companion/data["Companion"]/data["Saves"]
		// must reflect that same write before snbBiteAttack reads off it.
		companion, err = charstore.GetCompanion(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("reload companion after snb reference:", err)
			return
		}
		data["Companion"] = companion
		data["Saves"] = companionSaves(companion, sheet.ProficiencyBonus, sheet.Level)

		// Same "read-only quick reference, editing happens on the tab"
		// treatment nin-dog's own popup case gives structured Attacks above.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for snb popup:", err)
			return
		}
		playerIntMod := sheet.Abilities["int"].Modifier
		data["Attacks"] = append(composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus),
			snbBiteAttack(sheet.Level, playerIntMod, sheet.ProficiencyBonus, ref.AccuracyBonus))
	case "nin-dog":
		ref, err := s.loadNinDogReference(id, companion, sheet.Level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load nin-dog reference:", err)
			return
		}
		data["NinDogReference"] = ref

		// Re-read for the same reason as the snb case above.
		companion, err = charstore.GetCompanion(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("reload companion after nin-dog reference:", err)
			return
		}
		data["Companion"] = companion
		data["Saves"] = companionSaves(companion, sheet.ProficiencyBonus, sheet.Level)

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

		// Re-read for the same reason as the snb/nin-dog cases above.
		companion, err = charstore.GetCompanion(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("reload companion after titan reference:", err)
			return
		}
		data["Companion"] = companion
		data["Saves"] = applyTitanSturdyFrameSaves(
			companionSaves(companion, sheet.ProficiencyBonus, sheet.Level), ref, sheet.Abilities["int"].Modifier)

		// Same "read-only quick reference, editing happens on the tab"
		// treatment nin-dog's own popup case gives structured Attacks above.
		data["ReadOnlyAttacks"] = true
		attacks, err := charstore.ListCompanionAttacks(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion attacks for titan popup:", err)
			return
		}
		// titanSpecialWeaponUpgradeAttacks: see the identical call in
		// loadSummonsTabData's own titan branch above for why this is a
		// separate call from titanKnownWeaponUpgradeAttacks rather than
		// folded into it. titanSaveOnlyUpgradeAttacks: Shinobifall/Thermite
		// Launcher/Critical Ejection's own NoAttackRoll damage-only rows.
		data["Attacks"] = append(append(append(append(composeCompanionAttacks(attacks, companion, sheet.ProficiencyBonus),
			titanBashAttack(companion, ref, sheet.ProficiencyBonus)),
			titanKnownWeaponUpgradeAttacks(ref, companion, sheet.ProficiencyBonus, companion.IsDemonFoe)...),
			titanSpecialWeaponUpgradeAttacks(ref, companion, sheet, sheet.ProficiencyBonus, companion.IsDemonFoe)...),
			titanSaveOnlyUpgradeAttacks(ref, sheet, companion.IsDemonFoe)...)
	}

	s.render(w, "companion_sheet.html", data)
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

	// hp_current/ac/hp_max/speed/fly_speed/the six ability scores/size are
	// deliberately not read here — each has its own endpoint
	// (handleCompanionHP/handleCompanionIntField/handleCompanionSize) and
	// its own <form> in the template. See charstore.SetCompanionFields' doc
	// for why folding any of them into this whole-form save would silently
	// wipe it on every unrelated field's blur.
	//
	// resistances/immunities/condition_immunities: only ever rendered as
	// real input fields for kind="custom" (companion_fields.html's own
	// {{if eq .Companion.Kind "custom"}} guard around them) — reading them
	// unconditionally here is harmless for every other kind, since their own
	// form never includes these field names in the first place, so
	// r.FormValue reads "" for them, matching SetCompanionFields' own doc on
	// why that's a safe no-op.
	err := charstore.SetCompanionFields(s.charDB, id, cid, name, summonTribeSlug,
		r.FormValue("attacks"), r.FormValue("traits"), r.FormValue("notes"),
		armorChassis, r.FormValue("is_armor_form") == "1",
		strings.TrimSpace(r.FormValue("nin_dog_breed")),
		strings.TrimSpace(r.FormValue("titan_specialization")),
		r.FormValue("resistances"), r.FormValue("immunities"), r.FormValue("condition_immunities"),
		r.FormValue("is_demon_foe") == "1",
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

// companionOverrideFields whitelists which companion stat fields support a
// manual pin overriding their auto-computed default, mirroring
// sheetOverrideFields' own role for the main sheet — AC, Max HP, Speed, Fly
// Speed, the six ability scores, a Nin-Dog's Jutsu Slots Max, a Titan's
// Barrier Max. Only meaningful for the four kinds with a formula behind
// these fields at all (puppet/nin-dog/titan/snb — see charstore.Companion's
// own header doc); pinning one of these on a summon/custom companion is
// harmless but inert, since nothing ever recomputes a default for those two
// kinds to override in the first place. Size is pin-capable too (kind =
// "puppet" only, via handleCompanionSize) but isn't listed here since it
// goes through its own dedicated string setter (charstore.SetCompanionSize),
// not the int-only companionIntFields path this map gates.
var companionOverrideFields = map[string]bool{
	"ac": true, "hp_max": true, "speed": true, "fly_speed": true,
	"str_score": true, "dex_score": true, "con_score": true,
	"int_score": true, "wis_score": true, "cha_score": true,
	"jutsu_slots_max": true, "barrier_max": true,
}

// companionOverrideInt reads field out of a companion's own override map
// (charstore.GetCompanionOverrides) as an int64 — ok is false if the field
// isn't pinned, or (defensively, should never happen through this app's own
// writers) if the stored text fails to parse. Every kind-specific loader
// (loadPuppetsTabData, loadNinDogReference, loadTitanReference,
// loadSNBReference) calls this once per pinnable field, ahead of computing
// that field's own auto default, to decide which one actually lands in the
// row it writes back via that kind's own per-render stat-defaults writer.
func companionOverrideInt(overrides map[string]string, field string) (int64, bool) {
	raw, ok := overrides[field]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// handleCompanionIntField returns the endpoint for one of a companion's own
// delta-editable int fields (AC, HP-max, Speed, ability scores, ...) — same
// request/response shape as handleCompanionHP just above (form field
// "value": a leading "+"/"-" adjusts, a bare number sets outright, empty
// clears, and the response is always the new plain value so the field never
// carries a stale "+1" into a later, unrelated blur), generalized via
// charstore.AddCompanionIntField/SetCompanionIntField's own field
// whitelist.
//
// pin controls whether this field also gets a row in
// character_companion_overrides (charstore.SetCompanionOverride) — true for
// every field in companionOverrideFields (AC/Max HP/Speed/Fly Speed/ability
// scores/Jutsu Slots Max/Barrier Max: fields a kind-specific loader
// recomputes and overwrites on every render, so without a pin recorded here
// that recompute would silently discard whatever the player just typed the
// very next time the page renders — the same "auto-then-pin" contract the
// main sheet's own Max HP/Max Chakra boxes use), false for a pure resource
// pool with no computed default to protect against at all (HP-current's own
// handleCompanionHP, Jutsu-Slots-current, Barrier-current, Matryoshka's
// known-jutsu-slots count, Temp HP).
func (s *server) handleCompanionIntField(field string, pin bool) http.HandlerFunc {
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
			if pin {
				if err := charstore.SetCompanionOverride(s.charDB, cid, field, ""); err != nil {
					http.Error(w, "database error", http.StatusInternalServerError)
					log.Println("clear companion override "+field+":", err)
					return
				}
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
		if pin {
			if err := charstore.SetCompanionOverride(s.charDB, cid, field, strconv.FormatInt(newValue, 10)); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("set companion override "+field+":", err)
				return
			}
		}
		w.Write([]byte(strconv.FormatInt(newValue, 10)))
	}
}

// handleCompanionSize is Size's own dedicated endpoint (form field
// "value") — pin-capable like the fields handleCompanionIntField serves,
// but a plain string (a companion's size category, "Medium"/"Large"/...)
// rather than a number, so it can't reuse that handler's numeric delta/set/
// clear contract. Blank clears both the raw column and the pin, matching
// every other pinnable field's own "blank un-pins" behavior. Size is
// currently only ever rendered as an input for kind="puppet" (see
// companion_fields.html's $autoComputed Size branch), but this handler
// doesn't re-check kind — writing a size override for a kind with no Size
// formula to override is simply never read back by anything.
func (s *server) handleCompanionSize(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if err := charstore.SetCompanionSize(s.charDB, id, cid, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set companion size:", err)
		return
	}
	if err := charstore.SetCompanionOverride(s.charDB, cid, "size", value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set companion override size:", err)
		return
	}
	w.Write([]byte(value))
}
