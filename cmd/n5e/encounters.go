// Dungeon Master Tools: the Encounter Generator, a procedural adversary
// roster built from the Bingo Book's abstract Rank/Role/Role Trait/
// Freeform Slot toolkit (internal/encounter, internal/schema/migrations/
// rules/0024_bingo_book.sql). A generated roster is previewed unsaved
// (roster state round-tripped through a hidden JSON form field — see
// encounter_roster_card in templates/partials/encounter_roster.html) and
// only written to characters.db's encounters/encounter_adversaries/
// encounter_adversary_traits tables (migration 0089) once the GM names and
// saves it.
package main

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/encounter"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// newRand returns a math/rand source seeded from crypto/rand, so each
// generate/reroll gets independent, unpredictable results -- there's no
// need to reproduce a specific roll later (a saved roster is a snapshot,
// not a replay), just to avoid every request producing the same sequence.
func newRand() *rand.Rand {
	var seed int64
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err == nil {
		seed = int64(binary.LittleEndian.Uint64(buf[:]))
	}
	return rand.New(rand.NewSource(seed))
}

// loadBingoBookToolkit reads Pack 1's class (adversary_ranks), Role Trait
// and Freeform Slot tables from rules.db, with the standard
// content_translations overlay applied to class and Role Trait names.
func loadBingoBookToolkit(rulesDB *sql.DB, locale string) (ranks []encounter.Rank, traits []encounter.Trait, slots []encounter.FreeformSlot, err error) {
	rankRows, err := rulesDB.Query(`SELECT slug, name, hp_formula, ac_bonus, save_bonus, save_dc_bonus, init_bonus FROM adversary_ranks`)
	if err != nil {
		return nil, nil, nil, err
	}
	var rankSlugs []string
	for rankRows.Next() {
		var rk encounter.Rank
		if err := rankRows.Scan(&rk.Slug, &rk.Name, &rk.HPFormula, &rk.ACBonus, &rk.SaveBonus, &rk.SaveDCBonus, &rk.InitBonus); err != nil {
			rankRows.Close()
			return nil, nil, nil, err
		}
		ranks = append(ranks, rk)
		rankSlugs = append(rankSlugs, rk.Slug)
	}
	rankRows.Close()
	if err := rankRows.Err(); err != nil {
		return nil, nil, nil, err
	}
	sort.Slice(ranks, func(i, j int) bool {
		return encounterClassOrder[encounter.ClassKind(ranks[i].Slug)] < encounterClassOrder[encounter.ClassKind(ranks[j].Slug)]
	})
	if tr, err := i18n.Batch(rulesDB, locale, "adversary_ranks", rankSlugs); err == nil {
		for i := range ranks {
			if v, ok := tr[ranks[i].Slug]["name"]; ok {
				ranks[i].Name = v
			}
		}
	}

	traitRows, err := rulesDB.Query(`SELECT slug, name, role_slug, rank, description FROM adversary_role_traits`)
	if err != nil {
		return nil, nil, nil, err
	}
	var traitSlugs []string
	for traitRows.Next() {
		t := encounter.Trait{Source: "official", Category: "role"}
		if err := traitRows.Scan(&t.Slug, &t.Name, &t.RoleSlug, &t.Rank, &t.Description); err != nil {
			traitRows.Close()
			return nil, nil, nil, err
		}
		traits = append(traits, t)
		traitSlugs = append(traitSlugs, t.Slug)
	}
	traitRows.Close()
	if err := traitRows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if tr, err := i18n.Batch(rulesDB, locale, "adversary_role_traits", traitSlugs); err == nil {
		for i := range traits {
			if v, ok := tr[traits[i].Slug]["name"]; ok {
				traits[i].Name = v
			}
			if v, ok := tr[traits[i].Slug]["description"]; ok {
				traits[i].Description = v
			}
		}
	}

	slotRows, err := rulesDB.Query(`SELECT rank_slug, level_min, slots FROM adversary_freeform_slots`)
	if err != nil {
		return nil, nil, nil, err
	}
	for slotRows.Next() {
		var fs encounter.FreeformSlot
		if err := slotRows.Scan(&fs.RankSlug, &fs.LevelMin, &fs.Slots); err != nil {
			slotRows.Close()
			return nil, nil, nil, err
		}
		slots = append(slots, fs)
	}
	slotRows.Close()
	if err := slotRows.Err(); err != nil {
		return nil, nil, nil, err
	}

	return ranks, traits, slots, nil
}

// encounterClassOrder sorts the classes weakest first on every picker.
var encounterClassOrder = map[string]int{"minion": 0, "standard": 1, "elite": 2, "solo": 3}

// rankBySlug is a small linear lookup -- there are four classes, never
// worth a map for.
func rankBySlug(ranks []encounter.Rank, slug string) (encounter.Rank, bool) {
	for _, rk := range ranks {
		if rk.Slug == slug {
			return rk, true
		}
	}
	return encounter.Rank{}, false
}

// loadClassificationToolkit reads the Optional Classification modifiers and
// the Solo-only Transformation/Battlefield Domain example catalogs from
// rules.db, with the standard content_translations overlay applied to their
// names/descriptions. Kept separate from loadBingoBookToolkit so every
// existing call site of that function (roster generation itself never needs
// this data -- only the add/reroll/classify forms do) doesn't have to grow
// two more return values it wouldn't use.
func loadClassificationToolkit(rulesDB *sql.DB, locale string) (mods []encounter.ClassificationModifier, transformations []encounter.TransformationExample, domains []encounter.BattlefieldDomainExample, err error) {
	modRows, err := rulesDB.Query(`SELECT kind, hp_chakra_modifier, ac_bonus, attack_save_bonus, ability_points, initiative_bonus FROM adversary_classification_modifiers`)
	if err != nil {
		return nil, nil, nil, err
	}
	for modRows.Next() {
		var m encounter.ClassificationModifier
		if err := modRows.Scan(&m.Kind, &m.HPChakraModifier, &m.ACBonus, &m.AttackSaveBonus, &m.AbilityPoints, &m.InitiativeBonus); err != nil {
			modRows.Close()
			return nil, nil, nil, err
		}
		mods = append(mods, m)
	}
	modRows.Close()
	if err := modRows.Err(); err != nil {
		return nil, nil, nil, err
	}

	transRows, err := rulesDB.Query(`SELECT slug, name FROM adversary_transformation_examples ORDER BY sort_order`)
	if err != nil {
		return nil, nil, nil, err
	}
	var transSlugs []string
	for transRows.Next() {
		var t encounter.TransformationExample
		if err := transRows.Scan(&t.Slug, &t.Name); err != nil {
			transRows.Close()
			return nil, nil, nil, err
		}
		transformations = append(transformations, t)
		transSlugs = append(transSlugs, t.Slug)
	}
	transRows.Close()
	if err := transRows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if tr, err := i18n.Batch(rulesDB, locale, "adversary_transformation_examples", transSlugs); err == nil {
		for i := range transformations {
			if v, ok := tr[transformations[i].Slug]["name"]; ok {
				transformations[i].Name = v
			}
		}
	}

	domainRows, err := rulesDB.Query(`SELECT slug, name FROM adversary_battlefield_domains ORDER BY sort_order`)
	if err != nil {
		return nil, nil, nil, err
	}
	var domainSlugs []string
	for domainRows.Next() {
		var d encounter.BattlefieldDomainExample
		if err := domainRows.Scan(&d.Slug, &d.Name); err != nil {
			domainRows.Close()
			return nil, nil, nil, err
		}
		domains = append(domains, d)
		domainSlugs = append(domainSlugs, d.Slug)
	}
	domainRows.Close()
	if err := domainRows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if tr, err := i18n.Batch(rulesDB, locale, "adversary_battlefield_domains", domainSlugs); err == nil {
		for i := range domains {
			if v, ok := tr[domains[i].Slug]["name"]; ok {
				domains[i].Name = v
			}
		}
	}

	return mods, transformations, domains, nil
}

// applyClassificationForm reads a "classification"/"transformation_slug"/
// "battlefield_domain_slug" form triple and applies it to adv -- shared by
// every handler that generates or edits one adversary, so the same
// mutual-exclusivity and no-op-when-blank behavior applies everywhere.
// Transformation/Battlefield Domain fields are silently ignored for a
// non-Solo adversary, matching the book's own Solo-only framing rather than
// rejecting the request outright.
func applyClassificationForm(r *http.Request, adv encounter.GeneratedAdversary, mods []encounter.ClassificationModifier, transformations []encounter.TransformationExample, domains []encounter.BattlefieldDomainExample) encounter.GeneratedAdversary {
	adv = encounter.ApplyClassification(adv, r.FormValue("classification"), mods)
	if adv.RankSlug != "adversary-rank/solo" {
		return adv
	}
	transformationSlug := r.FormValue("transformation_slug")
	domainSlug := r.FormValue("battlefield_domain_slug")
	transformationName, domainName := "", ""
	for _, t := range transformations {
		if t.Slug == transformationSlug {
			transformationName = t.Name
		}
	}
	for _, d := range domains {
		if d.Slug == domainSlug {
			domainName = d.Name
		}
	}
	return encounter.WithNarrativeAttachment(adv, transformationSlug, transformationName, domainSlug, domainName)
}

// encounterRosterView is what encounter_roster_card (templates/partials/
// encounter_roster.html) renders -- either an unsaved preview (EncounterID
// == 0, RosterJSON carries the roster forward through the save/reroll
// forms) or a saved encounter's own adversaries read back from
// characters.db (EncounterID != 0, IDs on each adversary target the
// per-row reroll endpoint instead). encounterKit.fill sets the picker rows
// and each adversary's computed stats.
type encounterRosterView struct {
	EncounterID int64
	Name        string
	PartySize   int
	PartyLevel  int
	Difficulty  string
	Budget      int
	RosterJSON  string
	Adversaries []encounterAdversaryView
	// Ranks populates the manual "Add Adversary" class picker.
	Ranks []encounter.Rank
	// Transformations/BattlefieldDomains populate the Solo-only narrative
	// attachment picker.
	Transformations    []encounter.TransformationExample
	BattlefieldDomains []encounter.BattlefieldDomainExample
}

// encounterAdversaryView is one roster row: the stored adversary plus what
// the roster computes for display.
type encounterAdversaryView struct {
	encounter.GeneratedAdversary
	Index int
	ID    int64 // 0 for an unsaved preview row

	Stats           encounter.StatBlock
	ClassName       string
	RoleName        string
	ClanName        string
	AffiliationName string
	Discipline      string // the role's jutsu discipline, or ""
}

func rosterToView(roster []encounter.GeneratedAdversary) []encounterAdversaryView {
	views := make([]encounterAdversaryView, len(roster))
	for i, a := range roster {
		views[i] = encounterAdversaryView{GeneratedAdversary: a, Index: i}
	}
	return views
}

func marshalRoster(roster []encounter.GeneratedAdversary) string {
	b, err := json.Marshal(roster)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalRoster(s string) ([]encounter.GeneratedAdversary, error) {
	var roster []encounter.GeneratedAdversary
	if err := json.Unmarshal([]byte(s), &roster); err != nil {
		return nil, err
	}
	for i := range roster {
		roster[i] = roster[i].Normalize()
	}
	return roster, nil
}

// ---- Saved encounters: list --------------------------------------------

type encounterListRow struct {
	ID         int64
	Name       string
	PartySize  int
	PartyLevel int
	Difficulty string
	CreatedAt  string
}

// generatorFormData is what the generator form's clan and affiliation
// pickers need.
func (s *server) generatorFormData(r *http.Request, kit encounterKit, data map[string]any) map[string]any {
	tr := i18n.Translator(localeFromContext(r.Context()))
	data["Difficulties"] = encounter.Difficulties
	data["ClanOptions"] = kit.clanOptions(tr)
	data["AffiliationOptions"] = kit.affiliationOptions(tr)
	data["GenClan"] = r.FormValue("clan_tag")
	data["GenAffiliation"] = r.FormValue("affiliation_tag")
	return data
}

func (s *server) handleEncounters(w http.ResponseWriter, r *http.Request) {
	rows, err := s.charDB.Query(`
		SELECT id, name, party_size, party_level, difficulty, created_at
		FROM encounters ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query encounters:", err)
		return
	}
	defer rows.Close()

	var list []encounterListRow
	for rows.Next() {
		var e encounterListRow
		if err := rows.Scan(&e.ID, &e.Name, &e.PartySize, &e.PartyLevel, &e.Difficulty, &e.CreatedAt); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan encounter:", err)
			return
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("encounters rows:", err)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}

	s.render(w, r, "encounters.html", s.generatorFormData(r, kit, map[string]any{
		"Title":      s.t(r, "encounters.title"),
		"Encounters": list,
	}))
}

// ---- Generate (unsaved preview) ----------------------------------------

func parseEncounterForm(r *http.Request) (partySize, partyLevel int, difficulty string) {
	partySize, _ = strconv.Atoi(r.FormValue("party_size"))
	partyLevel, _ = strconv.Atoi(r.FormValue("party_level"))
	difficulty = r.FormValue("difficulty")
	if partySize < 1 {
		partySize = 1
	}
	if partyLevel < 1 {
		partyLevel = 1
	}
	if !encounter.ValidDifficulty(difficulty) {
		difficulty = "standard"
	}
	return
}

// previewForm is the unsaved roster plus the party fields every preview
// form round-trips.
type previewForm struct {
	roster                []encounter.GeneratedAdversary
	partySize, partyLevel int
	difficulty            string
	budget                int
}

// parsePreviewForm reads a preview action's roster_json and party fields,
// answering 400 itself on bad input. withIndex also reads and checks
// adversary_index.
func parsePreviewForm(w http.ResponseWriter, r *http.Request, withIndex bool) (f previewForm, idx int, ok bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return f, 0, false
	}
	roster, err := unmarshalRoster(r.FormValue("roster_json"))
	if err != nil {
		http.Error(w, "bad roster data", http.StatusBadRequest)
		return f, 0, false
	}
	f.roster = roster
	f.partySize, f.partyLevel, f.difficulty = parseEncounterForm(r)
	f.budget, _ = strconv.Atoi(r.FormValue("budget"))
	if withIndex {
		idx, err = strconv.Atoi(r.FormValue("adversary_index"))
		if err != nil || idx < 0 || idx >= len(roster) {
			http.Error(w, "bad adversary index", http.StatusBadRequest)
			return f, 0, false
		}
	}
	return f, idx, true
}

func (s *server) previewView(r *http.Request, kit encounterKit, f previewForm) encounterRosterView {
	view := encounterRosterView{
		PartySize: f.partySize, PartyLevel: f.partyLevel, Difficulty: f.difficulty, Budget: f.budget,
		RosterJSON: marshalRoster(f.roster), Adversaries: rosterToView(f.roster),
	}
	kit.fill(&view, i18n.Translator(localeFromContext(r.Context())))
	return view
}

func (s *server) handleGenerateEncounter(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	partySize, partyLevel, difficulty := parseEncounterForm(r)
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	opt := encounter.GenOptions{Clan: r.FormValue("clan_tag"), Affiliation: r.FormValue("affiliation_tag")}
	roster, budget, err := encounter.GenerateRoster(newRand(), partySize, partyLevel, difficulty, kit.Toolkit, opt)
	if err != nil {
		http.Error(w, "generator error", http.StatusInternalServerError)
		log.Println("generate roster:", err)
		return
	}

	view := s.previewView(r, kit, previewForm{roster, partySize, partyLevel, difficulty, budget})
	if wantsFragment(r) {
		s.renderCard(w, r, "encounters.html", "encounter_roster_card", view)
		return
	}
	s.render(w, r, "encounters.html", s.generatorFormData(r, kit, map[string]any{
		"Title": s.t(r, "encounters.title"), "Preview": view,
	}))
}

// handleRerollAdversary rerolls one row of an UNSAVED preview roster --
// the current roster is round-tripped through the request's own
// roster_json hidden field rather than read from any DB, since nothing
// has been saved yet.
func (s *server) handleRerollAdversary(w http.ResponseWriter, r *http.Request) {
	f, idx, ok := parsePreviewForm(w, r, true)
	if !ok {
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	rk, ok := kit.class(f.roster[idx].RankSlug)
	if !ok {
		http.Error(w, "unknown class", http.StatusBadRequest)
		return
	}
	f.roster[idx] = encounter.Reroll(newRand(), kit.Toolkit, rk, f.roster[idx])
	s.renderCard(w, r, "encounters.html", "encounter_roster_card", s.previewView(r, kit, f))
}

// handleAddPreviewAdversary appends one freshly generated adversary (of the
// GM's chosen class, defaulting to the party's own level) to an UNSAVED
// preview roster. The GM's optional Classification/Transformation/
// Battlefield Domain picks on the add form are applied immediately.
func (s *server) handleAddPreviewAdversary(w http.ResponseWriter, r *http.Request) {
	f, _, ok := parsePreviewForm(w, r, false)
	if !ok {
		return
	}
	level, err := strconv.Atoi(r.FormValue("level"))
	if err != nil || level < 1 {
		level = f.partyLevel
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	rk, ok := kit.class(r.FormValue("rank_slug"))
	if !ok {
		http.Error(w, "unknown class", http.StatusBadRequest)
		return
	}
	fresh := encounter.GenerateOne(newRand(), kit.Toolkit, rk, level, encounter.GenOptions{})
	fresh = applyClassificationForm(r, fresh, kit.Tables.Classifications, kit.Transformations, kit.Domains)
	f.roster = append(f.roster, fresh)
	s.renderCard(w, r, "encounters.html", "encounter_roster_card", s.previewView(r, kit, f))
}

// handleClassifyPreviewAdversary sets (or clears) one row's Optional
// Classification and/or Transformation/Battlefield Domain attachment on an
// UNSAVED preview roster, without regenerating its mechanics -- a
// deliberate, independent GM decision (see ApplyClassification's own
// comment).
func (s *server) handleClassifyPreviewAdversary(w http.ResponseWriter, r *http.Request) {
	f, idx, ok := parsePreviewForm(w, r, true)
	if !ok {
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	f.roster[idx] = applyClassificationForm(r, f.roster[idx], kit.Tables.Classifications, kit.Transformations, kit.Domains)
	s.renderCard(w, r, "encounters.html", "encounter_roster_card", s.previewView(r, kit, f))
}

// handleDeletePreviewAdversary removes one row from an UNSAVED preview
// roster by index.
func (s *server) handleDeletePreviewAdversary(w http.ResponseWriter, r *http.Request) {
	f, idx, ok := parsePreviewForm(w, r, true)
	if !ok {
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	f.roster = append(f.roster[:idx], f.roster[idx+1:]...)
	s.renderCard(w, r, "encounters.html", "encounter_roster_card", s.previewView(r, kit, f))
}

// ---- Save / view / reroll / delete a saved encounter --------------------

func (s *server) handleSaveEncounter(w http.ResponseWriter, r *http.Request) {
	f, _, ok := parsePreviewForm(w, r, false)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}

	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin save encounter tx:", err)
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO encounters (name, party_size, party_level, difficulty, budget)
		VALUES (?, ?, ?, ?, ?)`, name, f.partySize, f.partyLevel, f.difficulty, f.budget)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("insert encounter:", err)
		return
	}
	encID, _ := res.LastInsertId()

	for i, a := range f.roster {
		if _, err := insertAdversary(tx, encID, i, a, kit); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("insert encounter adversaries:", err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit save encounter:", err)
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(encID, 10), http.StatusSeeOther)
}

// adversaryColumns are the encounter_adversaries columns one
// GeneratedAdversary is stored in, in adversaryArgs' order.
var adversaryColumns = []string{
	"name", "rank_slug", "rank_name", "role_slug", "role_name", "level",
	"hp_formula", "ac_bonus", "save_bonus", "save_dc_bonus", "init_bonus",
	"freeform_slots", "budget_cost", "tenacity_max", "classification",
	"classification_hp_chakra_modifier", "classification_ability_points",
	"transformation_slug", "transformation_name", "battlefield_domain_slug", "battlefield_domain_name",
	"adv_rank", "clan_tag", "affiliation_tag", "class_mod_tag", "armor",
	"str_mod", "dex_mod", "con_mod", "int_mod", "wis_mod", "cha_mod", "save_profs",
	"adj_hp", "adj_cp", "adj_ac", "adj_init", "adj_speed", "adj_dr",
}

func adversaryArgs(a encounter.GeneratedAdversary, kit encounterKit) []any {
	a = a.Normalize()
	args := []any{
		a.Name, a.RankSlug, kit.className(a.RankSlug), a.RoleSlug, kit.roleName(a.RoleSlug), a.Level,
		a.HPFormula, a.ACBonus, a.SaveBonus, a.SaveDCBonus, a.InitBonus,
		a.FreeformSlots, a.BudgetCost, a.TenacityMax, a.Classification,
		a.ClassificationHPChakraModifier, a.ClassificationAbilityPoints,
		a.TransformationSlug, a.TransformationName, a.BattlefieldDomainSlug, a.BattlefieldDomainName,
		a.AdvRank, a.ClanTag, a.AffiliationTag, a.ClassModTag, a.Armor,
	}
	for _, m := range a.AbilityMods {
		if m == nil {
			args = append(args, nil)
		} else {
			args = append(args, *m)
		}
	}
	return append(args, strings.Join(a.SaveProfs, ","),
		a.Adjust.HP, a.Adjust.CP, a.Adjust.AC, a.Adjust.Init, a.Adjust.Speed, a.Adjust.DR)
}

// insertAdversary writes one adversary, its traits and jutsu under encID at
// sortOrder, returning its id.
func insertAdversary(tx *sql.Tx, encID int64, sortOrder int, a encounter.GeneratedAdversary, kit encounterKit) (int64, error) {
	marks := strings.Repeat(", ?", len(adversaryColumns))
	res, err := tx.Exec(`INSERT INTO encounter_adversaries (encounter_id, sort_order, `+strings.Join(adversaryColumns, ", ")+`)
		VALUES (?, ?`+marks+`)`, append([]any{encID, sortOrder}, adversaryArgs(a, kit)...)...)
	if err != nil {
		return 0, err
	}
	advID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := writeAdversaryTraits(tx, advID, a.Traits); err != nil {
		return 0, err
	}
	return advID, writeAdversaryJutsu(tx, advID, a.Jutsu)
}

// appendAdversary inserts one adversary onto the end of a saved encounter's
// roster, at the gap-tolerant sort_order convention already used elsewhere
// in the app (character_custom_features and similar) -- MAX(sort_order)+1
// on insert, no renumbering of existing rows on delete, so a saved
// adversary's id/position never shifts under a GM mid-review.
func appendAdversary(tx *sql.Tx, encID int64, a encounter.GeneratedAdversary, kit encounterKit) (int64, error) {
	var sortOrder int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM encounter_adversaries WHERE encounter_id = ?`, encID).Scan(&sortOrder); err != nil {
		return 0, err
	}
	return insertAdversary(tx, encID, sortOrder, a, kit)
}

// replaceAdversary overwrites one saved adversary row (and its traits and
// jutsu) in
// place, keeping its id/sort_order, so the row doesn't jump position in the
// table.
func replaceAdversary(tx *sql.Tx, advID int64, a encounter.GeneratedAdversary, kit encounterKit) error {
	if _, err := tx.Exec(`UPDATE encounter_adversaries SET `+strings.Join(adversaryColumns, " = ?, ")+` = ? WHERE id = ?`,
		append(adversaryArgs(a, kit), advID)...); err != nil {
		return err
	}
	if err := writeAdversaryTraits(tx, advID, a.Traits); err != nil {
		return err
	}
	return writeAdversaryJutsu(tx, advID, a.Jutsu)
}

// writeAdversaryTraits replaces a saved adversary's trait rows.
func writeAdversaryTraits(tx *sql.Tx, advID int64, traits []encounter.GeneratedTrait) error {
	if _, err := tx.Exec(`DELETE FROM encounter_adversary_traits WHERE adversary_id = ?`, advID); err != nil {
		return err
	}
	for ti, t := range traits {
		source, category := t.Source, t.Category
		if source == "" {
			source, category = "official", "role"
		}
		if _, err := tx.Exec(`
			INSERT INTO encounter_adversary_traits (adversary_id, trait_slug, trait_name, trait_rank, trait_description, sort_order, trait_source, trait_category)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, advID, t.Slug, t.Name, t.Rank, t.Description, ti, source, category); err != nil {
			return err
		}
	}
	return nil
}

// writeAdversaryJutsu replaces a saved adversary's jutsu rows.
func writeAdversaryJutsu(tx *sql.Tx, advID int64, jutsu []encounter.GeneratedJutsu) error {
	if _, err := tx.Exec(`DELETE FROM encounter_adversary_jutsu WHERE adversary_id = ?`, advID); err != nil {
		return err
	}
	for i, j := range jutsu {
		if _, err := tx.Exec(`
			INSERT INTO encounter_adversary_jutsu (adversary_id, jutsu_slug, jutsu_name, jutsu_rank, classification, cost_text, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, advID, j.Slug, j.Name, j.Rank, j.Classification, j.CostText, i); err != nil {
			return err
		}
	}
	return nil
}

// deleteAdversary removes one saved adversary and its child rows. Children
// are deleted explicitly rather than relied on to cascade -- characters.db
// is opened without a _pragma=foreign_keys(1) DSN param, and PRAGMA
// foreign_keys is per-connection in SQLite, so ON DELETE CASCADE can't be
// trusted against a pooled *sql.DB (see internal/charstore/sheet.go's own
// deleteCompanionChildRows comment for the same reasoning elsewhere).
func deleteAdversary(tx *sql.Tx, encID, advID int64) error {
	for _, q := range []string{
		`DELETE FROM encounter_adversary_traits WHERE adversary_id = ?`,
		`DELETE FROM encounter_adversary_jutsu WHERE adversary_id = ?`,
		// Its turn-order entry stays (the adversary may already be
		// mid-fight); it just stops pointing at a roster row.
		`UPDATE encounter_turn_entries SET adversary_id = NULL WHERE adversary_id = ?`,
	} {
		if _, err := tx.Exec(q, advID); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`DELETE FROM encounter_adversaries WHERE id = ? AND encounter_id = ?`, advID, encID)
	return err
}

// setAdversaryClassification updates only a saved adversary's Optional
// Classification and Transformation/Battlefield Domain attachment, leaving
// its class/role/traits/mechanics untouched -- backs handleClassifySaved
// Adversary, a deliberately narrower action than a full reroll.
func setAdversaryClassification(tx *sql.Tx, advID int64, a encounter.GeneratedAdversary) error {
	_, err := tx.Exec(`
		UPDATE encounter_adversaries SET
			ac_bonus = ?, save_dc_bonus = ?, init_bonus = ?,
			classification = ?, classification_hp_chakra_modifier = ?, classification_ability_points = ?,
			transformation_slug = ?, transformation_name = ?, battlefield_domain_slug = ?, battlefield_domain_name = ?
		WHERE id = ?`,
		a.ACBonus, a.SaveDCBonus, a.InitBonus,
		a.Classification, a.ClassificationHPChakraModifier, a.ClassificationAbilityPoints,
		a.TransformationSlug, a.TransformationName, a.BattlefieldDomainSlug, a.BattlefieldDomainName, advID)
	return err
}

// rowScanner is *sql.Row or *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanAdversary reads "id, sort_order, <adversaryColumns>" into a view row
// (traits not included).
func scanAdversary(sc rowScanner) (encounterAdversaryView, error) {
	var v encounterAdversaryView
	a := &v.GeneratedAdversary
	var rankName, roleName, saveProfs string
	var mods [6]sql.NullInt64
	err := sc.Scan(&v.ID, &v.Index,
		&a.Name, &a.RankSlug, &rankName, &a.RoleSlug, &roleName, &a.Level,
		&a.HPFormula, &a.ACBonus, &a.SaveBonus, &a.SaveDCBonus, &a.InitBonus,
		&a.FreeformSlots, &a.BudgetCost, &a.TenacityMax, &a.Classification,
		&a.ClassificationHPChakraModifier, &a.ClassificationAbilityPoints,
		&a.TransformationSlug, &a.TransformationName, &a.BattlefieldDomainSlug, &a.BattlefieldDomainName,
		&a.AdvRank, &a.ClanTag, &a.AffiliationTag, &a.ClassModTag, &a.Armor,
		&mods[0], &mods[1], &mods[2], &mods[3], &mods[4], &mods[5], &saveProfs,
		&a.Adjust.HP, &a.Adjust.CP, &a.Adjust.AC, &a.Adjust.Init, &a.Adjust.Speed, &a.Adjust.DR)
	if err != nil {
		return v, err
	}
	for i, m := range mods {
		if m.Valid {
			n := int(m.Int64)
			a.AbilityMods[i] = &n
		}
	}
	for _, p := range strings.Split(saveProfs, ",") {
		if p = strings.TrimSpace(p); p != "" {
			a.SaveProfs = append(a.SaveProfs, p)
		}
	}
	return v, nil
}

const adversarySelect = `SELECT id, sort_order, ` // + adversaryColumns

func adversarySelectSQL(where string) string {
	return adversarySelect + strings.Join(adversaryColumns, ", ") + ` FROM encounter_adversaries ` + where
}

// loadAdversaryTraits reads one saved adversary's traits.
func loadAdversaryTraits(charDB *sql.DB, advID int64) ([]encounter.GeneratedTrait, error) {
	rows, err := charDB.Query(`
		SELECT trait_slug, trait_name, trait_rank, trait_description, trait_source, trait_category
		FROM encounter_adversary_traits WHERE adversary_id = ? ORDER BY sort_order`, advID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []encounter.GeneratedTrait
	for rows.Next() {
		var t encounter.GeneratedTrait
		if err := rows.Scan(&t.Slug, &t.Name, &t.Rank, &t.Description, &t.Source, &t.Category); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loadAdversaryJutsu reads one saved adversary's jutsu.
func loadAdversaryJutsu(charDB *sql.DB, advID int64) ([]encounter.GeneratedJutsu, error) {
	rows, err := charDB.Query(`
		SELECT jutsu_slug, jutsu_name, jutsu_rank, classification, cost_text
		FROM encounter_adversary_jutsu WHERE adversary_id = ? ORDER BY sort_order`, advID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []encounter.GeneratedJutsu
	for rows.Next() {
		var j encounter.GeneratedJutsu
		if err := rows.Scan(&j.Slug, &j.Name, &j.Rank, &j.Classification, &j.CostText); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// loadSavedAdversary reads one saved adversary with its traits and jutsu.
func loadSavedAdversary(charDB *sql.DB, encID, advID int64) (encounter.GeneratedAdversary, error) {
	v, err := scanAdversary(charDB.QueryRow(adversarySelectSQL(`WHERE id = ? AND encounter_id = ?`), advID, encID))
	if err != nil {
		return v.GeneratedAdversary, err
	}
	if v.Traits, err = loadAdversaryTraits(charDB, advID); err != nil {
		return v.GeneratedAdversary, err
	}
	v.Jutsu, err = loadAdversaryJutsu(charDB, advID)
	return v.GeneratedAdversary, err
}

func loadEncounterRosterView(charDB *sql.DB, encID int64) (encounterRosterView, error) {
	var view encounterRosterView
	view.EncounterID = encID
	err := charDB.QueryRow(`
		SELECT name, party_size, party_level, difficulty, budget
		FROM encounters WHERE id = ?`, encID).
		Scan(&view.Name, &view.PartySize, &view.PartyLevel, &view.Difficulty, &view.Budget)
	if err != nil {
		return view, err
	}

	rows, err := charDB.Query(adversarySelectSQL(`WHERE encounter_id = ? ORDER BY sort_order`), encID)
	if err != nil {
		return view, err
	}
	var advs []encounterAdversaryView
	for rows.Next() {
		a, err := scanAdversary(rows)
		if err != nil {
			rows.Close()
			return view, err
		}
		advs = append(advs, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return view, err
	}

	for i := range advs {
		if advs[i].Traits, err = loadAdversaryTraits(charDB, advs[i].ID); err != nil {
			return view, err
		}
		if advs[i].Jutsu, err = loadAdversaryJutsu(charDB, advs[i].ID); err != nil {
			return view, err
		}
	}
	view.Adversaries = advs
	return view, nil
}

// renderSavedRoster re-reads a saved encounter's roster and renders the
// roster card.
func (s *server) renderSavedRoster(w http.ResponseWriter, r *http.Request, kit encounterKit, encID int64) {
	view, err := loadEncounterRosterView(s.charDB, encID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("reload encounter roster:", err)
		return
	}
	kit.fill(&view, i18n.Translator(localeFromContext(r.Context())))
	s.renderCard(w, r, "encounters.html", "encounter_roster_card", view)
}

func (s *server) handleEncounterDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	view, err := loadEncounterRosterView(s.charDB, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load encounter detail:", err)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	kit.fill(&view, i18n.Translator(localeFromContext(r.Context())))
	turns, err := loadTurnOrderView(s.charDB, id, kit, view.Adversaries, i18n.Translator(localeFromContext(r.Context())))
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load turn order for encounter detail:", err)
		return
	}
	s.render(w, r, "encounters.html", map[string]any{
		"Title": view.Name, "Detail": view, "TurnOrder": turns,
	})
}

// rosterStats indexes a filled roster view's stat blocks by adversary id.
func rosterStats(view encounterRosterView) map[int64]encounter.StatBlock {
	out := make(map[int64]encounter.StatBlock, len(view.Adversaries))
	for _, a := range view.Adversaries {
		out[a.ID] = a.Stats
	}
	return out
}

// savedAdversaryPath reads {id} and, from the form or the path, the
// adversary id, answering 404 itself on bad input.
func savedAdversaryPath(w http.ResponseWriter, r *http.Request, fromForm bool) (encID, advID int64, ok bool) {
	encID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return 0, 0, false
	}
	raw := r.PathValue("advId")
	if fromForm {
		raw = r.FormValue("adversary_id")
	}
	advID, err = strconv.ParseInt(raw, 10, 64)
	if err != nil {
		http.Error(w, "bad adversary id", http.StatusBadRequest)
		return 0, 0, false
	}
	return encID, advID, true
}

func (s *server) handleRerollSavedAdversary(w http.ResponseWriter, r *http.Request) {
	encID, advID, ok := savedAdversaryPath(w, r, true)
	if !ok {
		return
	}
	prev, err := loadSavedAdversary(s.charDB, encID, advID)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("lookup adversary for reroll:", err)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	rk, ok := kit.class(prev.RankSlug)
	if !ok {
		http.Error(w, "unknown class", http.StatusBadRequest)
		return
	}
	fresh := encounter.Reroll(newRand(), kit.Toolkit, rk, prev)

	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin reroll tx:", err)
		return
	}
	defer tx.Rollback()
	if err := replaceAdversary(tx, advID, fresh, kit); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("replace adversary:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit reroll:", err)
		return
	}
	s.renderSavedRoster(w, r, kit, encID)
}

// handleClassifySavedAdversary sets (or clears) one saved adversary's
// Optional Classification and/or Transformation/Battlefield Domain
// attachment without regenerating its mechanics -- the saved-encounter
// counterpart of handleClassifyPreviewAdversary.
func (s *server) handleClassifySavedAdversary(w http.ResponseWriter, r *http.Request) {
	encID, advID, ok := savedAdversaryPath(w, r, false)
	if !ok {
		return
	}
	var current encounter.GeneratedAdversary
	err := s.charDB.QueryRow(`
		SELECT rank_slug, ac_bonus, save_dc_bonus, init_bonus, classification
		FROM encounter_adversaries WHERE id = ? AND encounter_id = ?`, advID, encID).
		Scan(&current.RankSlug, &current.ACBonus, &current.SaveDCBonus, &current.InitBonus, &current.Classification)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("lookup adversary for classify:", err)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	adv := applyClassificationForm(r, current, kit.Tables.Classifications, kit.Transformations, kit.Domains)

	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin classify tx:", err)
		return
	}
	defer tx.Rollback()
	if err := setAdversaryClassification(tx, advID, adv); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set adversary classification:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit classify:", err)
		return
	}
	s.renderSavedRoster(w, r, kit, encID)
}

// handleAddSavedAdversary appends one freshly generated adversary (of the
// GM's chosen class, defaulting to the encounter's own party level) onto a
// saved encounter's roster.
func (s *server) handleAddSavedAdversary(w http.ResponseWriter, r *http.Request) {
	encID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	var partyLevel int
	err = s.charDB.QueryRow(`SELECT party_level FROM encounters WHERE id = ?`, encID).Scan(&partyLevel)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("lookup encounter for add:", err)
		return
	}
	level, err := strconv.Atoi(r.FormValue("level"))
	if err != nil || level < 1 {
		level = partyLevel
	}

	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	rk, ok := kit.class(r.FormValue("rank_slug"))
	if !ok {
		http.Error(w, "unknown class", http.StatusBadRequest)
		return
	}
	fresh := encounter.GenerateOne(newRand(), kit.Toolkit, rk, level, encounter.GenOptions{})
	fresh = applyClassificationForm(r, fresh, kit.Tables.Classifications, kit.Transformations, kit.Domains)

	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin add adversary tx:", err)
		return
	}
	defer tx.Rollback()
	if _, err := appendAdversary(tx, encID, fresh, kit); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("append adversary:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit add adversary:", err)
		return
	}
	s.renderSavedRoster(w, r, kit, encID)
}

// handleDeleteSavedAdversary removes one adversary (and its child rows)
// from a saved encounter. POST-only, guarded client-side by
// confirm-submit.js's data-confirm, same as handleDeleteEncounter.
func (s *server) handleDeleteSavedAdversary(w http.ResponseWriter, r *http.Request) {
	encID, advID, ok := savedAdversaryPath(w, r, false)
	if !ok {
		return
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin delete adversary tx:", err)
		return
	}
	defer tx.Rollback()
	if err := deleteAdversary(tx, encID, advID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete adversary:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit delete adversary:", err)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	s.renderSavedRoster(w, r, kit, encID)
}

// handleDeleteEncounter permanently removes a saved encounter. POST-only,
// guarded client-side by confirm-submit.js -- same reasoning as
// handleDeleteCharacter (characters.go): the confirm dialog is the safety
// net, not this handler. Child rows are deleted explicitly, for the same
// no-reliable-foreign-keys reason as deleteAdversary.
func (s *server) handleDeleteEncounter(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := deleteEncounter(s.charDB, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete encounter:", err)
		return
	}
	http.Redirect(w, r, "/encounters", http.StatusSeeOther)
}

func deleteEncounter(charDB *sql.DB, encID int64) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM encounter_turn_entries WHERE encounter_id = ?`,
		`DELETE FROM encounter_adversary_traits WHERE adversary_id IN (SELECT id FROM encounter_adversaries WHERE encounter_id = ?)`,
		`DELETE FROM encounter_adversary_jutsu WHERE adversary_id IN (SELECT id FROM encounter_adversaries WHERE encounter_id = ?)`,
		`DELETE FROM encounter_adversaries WHERE encounter_id = ?`,
		`DELETE FROM encounters WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, encID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
