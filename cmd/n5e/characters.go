// Character creation flow: multi-page, server-rendered, no client-side
// wizard state (matches dice-roller.js's own comment that this app is a
// plain multi-page site, not an SPA). The character row is created
// immediately as a draft and every later step is an ordinary
// read-then-update against that real character_id — closing the browser
// mid-creation loses nothing.
package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/backup"
	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

func parseCharacterID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// levelOptions backs the Class step's starting-level dropdown — a fixed 1-20
// range, same bound SetLevel enforces.
var levelOptions = func() []int {
	opts := make([]int, 20)
	for i := range opts {
		opts[i] = i + 1
	}
	return opts
}()

// ---- Character list & new -------------------------------------------------

type characterListRow struct {
	ID             int64
	Name           string
	CreationStatus string
	ClassSummary   string // "Level 1 Genjutsu Specialist", or "" pre-class
	Sheet          *charsheet.Sheet
}

func (s *server) handleCharacters(w http.ResponseWriter, r *http.Request) {
	rows, err := s.charDB.Query(`SELECT id, name, creation_status FROM characters`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query characters:", err)
		return
	}
	var characters []characterListRow
	for rows.Next() {
		var c characterListRow
		if err := rows.Scan(&c.ID, &c.Name, &c.CreationStatus); err != nil {
			rows.Close()
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan character:", err)
			return
		}
		characters = append(characters, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("characters rows:", err)
		return
	}
	sort.Slice(characters, func(i, k int) bool { return sortKey(characters[i].Name) < sortKey(characters[k].Name) })

	// One query per character for its class summary — same small-N, N+1
	// shape classes.go's loadClassLevels uses; character counts here are
	// realistically single digits to low tens, not worth batching.
	for i := range characters {
		classes, err := s.loadCharacterClassLevels(characters[i].ID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("query character class summary:", err)
			return
		}
		if len(classes) == 0 {
			continue
		}
		if len(classes) == 1 {
			characters[i].ClassSummary = "Level " + strconv.Itoa(classes[0].Levels) + " " + s.className(classes[0].Slug)
			continue
		}
		// Multiclassed: "Level 8 (Weapon Specialist 5 / Ninjutsu
		// Specialist 3)" — total first since that's what governs
		// proficiency bonus, the per-class breakdown after it.
		total := 0
		parts := make([]string, len(classes))
		for j, c := range classes {
			total += c.Levels
			parts[j] = s.className(c.Slug) + " " + strconv.Itoa(c.Levels)
		}
		characters[i].ClassSummary = "Level " + strconv.Itoa(total) + " (" + strings.Join(parts, " / ") + ")"
	}

	// Compute is read-only and safe against a mid-creation draft (level 0,
	// default 10s, no equipped armor) — reused as-is here so the list shows
	// real derived numbers even before creation finishes, no engine changes
	// needed. Same N+1-per-character shape as the ClassSummary loop above.
	for i := range characters {
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characters[i].ID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute character sheet for list:", err)
			return
		}
		characters[i].Sheet = sheet
	}

	s.render(w, "characters.html", map[string]any{"Title": "Characters", "Characters": characters})
}

func (s *server) handleNewCharacter(w http.ResponseWriter, r *http.Request) {
	s.render(w, "character_new.html", map[string]any{"Title": "New Character"})
}

func (s *server) handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.render(w, "character_new.html", map[string]any{
			"Title": "New Character", "Error": "Name can't be empty.",
		})
		return
	}
	id, err := charstore.CreateDraft(s.charDB, name)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("create character draft:", err)
		return
	}
	http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
}

// handleDeleteCharacter permanently removes a character. POST-only (a GET
// would let a stray link or a prefetch destroy a character), and the two
// buttons that reach it are guarded by confirm-submit.js — the confirm
// dialog is the safety net, not this handler, which does exactly what it's
// asked. A delete of an id that no longer exists is not an error worth
// showing: the end state the caller wanted is the end state they get.
func (s *server) handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := charstore.DeleteCharacter(s.charDB, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete character:", err)
		return
	}
	http.Redirect(w, r, "/characters", http.StatusSeeOther)
}

// handleResetCharacterCreation is the "Reset Levels" button — a full retcon
// back to the start of creation (see charstore.ResetCharacterCreation for
// exactly what's kept vs. wiped). POST-only and guarded by
// confirm-submit.js, same reasoning as handleDeleteCharacter above: the
// confirm dialog is the safety net, not this handler. A manual backup
// snapshot is taken first regardless, since this is far more destructive
// than a typical sheet edit and easy to fat-finger.
func (s *server) handleResetCharacterCreation(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := backup.Create(s.charDB, s.backupDir, backup.ReasonPreReset); err != nil {
		log.Println("pre-reset backup:", err)
	} else if err := backup.Prune(s.backupDir); err != nil {
		log.Println("pruning backups:", err)
	}
	if err := charstore.ResetCharacterCreation(s.charDB, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("reset character creation:", err)
		return
	}
	http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
}

// ---- Creation hub -----------------------------------------------------

type creationStep struct {
	Label string
	Href  string
	Done  bool
}

func (s *server) handleCreationHub(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := s.buildCreationHubData(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build creation hub data:", err)
		return
	}
	s.render(w, "creation_hub.html", data)
}

// buildCreationHubData assembles the hub's template data — factored out of
// handleCreationHub so handleCreateFinish's own subclass gate (below) can
// re-render the same page with an added "Error" message instead of
// duplicating the steps/summary-building logic.
func (s *server) buildCreationHubData(id int64) (map[string]any, error) {
	var name, creationStatus string
	var str, dex, con, intel, wis, cha int
	var clanSlug, backgroundSlug sql.NullString
	err := s.charDB.QueryRow(`
		SELECT name, creation_status, clan_slug, background_slug,
		       base_str, base_dex, base_con, base_int, base_wis, base_cha
		FROM characters WHERE id = ?`, id,
	).Scan(&name, &creationStatus, &clanSlug, &backgroundSlug, &str, &dex, &con, &intel, &wis, &cha)
	if err != nil {
		return nil, err
	}

	base := strconv.FormatInt(id, 10)
	exists := func(query string) bool {
		var n int
		if err := s.charDB.QueryRow(query, id).Scan(&n); err != nil {
			return false
		}
		return n > 0
	}
	// Abilities has no dedicated "done" signal to check — creation never
	// stores a flag for it, only the six scores themselves — so this
	// heuristic (any score no longer at the schema's default of 10) is a
	// deliberate approximation: a character who genuinely wants all-10s
	// reads as "not done" on the checklist even after saving. Harmless
	// (the step stays freely revisitable either way), just imprecise.
	abilitiesDone := str != 10 || dex != 10 || con != 10 || intel != 10 || wis != 10 || cha != 10

	classSummary, err := s.loadClassSummary(id)
	if err != nil {
		return nil, err
	}

	steps := []creationStep{
		{Label: "Clan", Href: "/characters/" + base + "/create/clan", Done: clanSlug.Valid},
		{Label: "Class", Href: "/characters/" + base + "/create/class",
			Done: exists(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`)},
		{Label: "Subclass", Href: "/characters/" + base + "/create/class", Done: subclassGateSatisfied(classSummary)},
		{Label: "Ability Scores", Href: "/characters/" + base + "/create/abilities", Done: abilitiesDone},
		{Label: "Background", Href: "/characters/" + base + "/create/background", Done: backgroundSlug.Valid},
		{Label: "Equipment", Href: "/characters/" + base + "/create/equipment",
			Done: exists(`SELECT COUNT(*) FROM character_inventory WHERE character_id = ? AND notes = 'creation-equipment'`)},
		{Label: "Jutsu", Href: "/characters/" + base + "/create/jutsu",
			Done: exists(`SELECT COUNT(*) FROM character_jutsu WHERE character_id = ? AND source = 'learned'`)},
		{Label: "Ambitions", Href: "/characters/" + base + "/create/ambitions",
			Done: exists(`SELECT COUNT(*) FROM character_ambitions WHERE character_id = ?`)},
	}

	return map[string]any{
		"Title": "Creating " + name, "ID": id, "Name": name,
		"CreationStatus": creationStatus, "Steps": steps,
	}, nil
}

// renderCreationHubError re-renders the creation hub with an inline error —
// the subclass gate's own "can't finish yet" path, same "rebuild the page,
// add Error" shape handleCreateClan already uses for its own step.
func (s *server) renderCreationHubError(w http.ResponseWriter, id int64, message string) {
	data, err := s.buildCreationHubData(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build creation hub data for error:", err)
		return
	}
	data["Error"] = message
	s.render(w, "creation_hub.html", data)
}

// handleCreateFinish is also reachable from an already-complete character
// (the hub's own subtitle: "revisited any time before or after
// finishing"), so the draft->complete transition is captured before
// calling charstore.Finish — current_hp/current_chakra are seeded to their
// computed maximum only on that first transition. Re-finishing an
// already-complete character (e.g. after only editing Ambitions) must not
// heal current_hp/current_chakra back to full; a character who has taken
// damage or spent chakra since finishing keeps that state.
func (s *server) handleCreateFinish(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var creationStatus string
	if err := s.charDB.QueryRow(
		`SELECT creation_status FROM characters WHERE id = ?`, id,
	).Scan(&creationStatus); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query creation status:", err)
		return
	}

	classSummary, err := s.loadClassSummary(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load class summary for finish gate:", err)
		return
	}
	if missing := subclassGateFailures(classSummary); len(missing) > 0 {
		names := make([]string, len(missing))
		for i, row := range missing {
			names[i] = row.ClassName
		}
		s.renderCreationHubError(w, id,
			"Choose a subclass before finishing — "+strings.Join(names, ", ")+" reached 3rd level without one.")
		return
	}

	if err := charstore.Finish(s.charDB, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("finish creation:", err)
		return
	}

	if creationStatus != "complete" {
		// A brand-new character's current_hp/current_chakra are still at
		// the schema's default of 0 (nothing in any prior creation step
		// sets them) — seed them to the same computed maximum a long/full
		// rest heals to, via the same charsheet.Compute +
		// charstore.SetRestGains pair handleSheetRest uses, rather than
		// re-deriving the Max HP/Max Chakra formula here. Temp HP is
		// deliberately left untouched: it starts at 0 and should stay
		// there for a new character.
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for creation finish:", err)
			return
		}
		hp := sheet.MaxHP - sheet.CurrentHP
		chakra := sheet.MaxChakra - sheet.CurrentChakra
		if err := charstore.SetRestGains(s.charDB, id, hp, chakra, 0, 0); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("seed starting hp/chakra:", err)
			return
		}
	}

	http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// ---- Clan step ----------------------------------------------------------

type clanOption struct {
	Slug, Name, Epithet string
}

func (s *server) handleCreateClan(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		clanSlug := r.FormValue("clan_slug")
		if clanSlug == "" {
			http.Error(w, "clan_slug required", http.StatusBadRequest)
			return
		}
		variant, _ := strconv.Atoi(r.FormValue("asi_variant"))
		variants, err := charstore.ClanAbilityVariants(s.rulesDB, clanSlug)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("clan ability variants:", err)
			return
		}
		var picks []string
		if variant >= 0 && variant < len(variants) {
			for slot := range variants[variant].Slots {
				picks = append(picks, r.FormValue("asi_"+strconv.Itoa(variant)+"_"+strconv.Itoa(slot)))
			}
		}
		if err := charstore.SetClan(s.charDB, s.rulesDB, id, clanSlug, variant, picks); err != nil {
			// A rejected pick (an ability chosen twice, or one not offered
			// by its slot) is the player's input, not a server fault — the
			// step re-renders with the message rather than dead-ending on
			// a 500, same as the background step's duplicate-choice path.
			// Anything else really is a server fault and stays a 500.
			if !errors.Is(err, charstore.ErrInvalidPick) {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("set clan:", err)
				return
			}
			data, buildErr := s.buildClanStepData(id, clanSlug)
			if buildErr != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("set clan:", err, "| rebuild clan step:", buildErr)
				return
			}
			data["Error"] = strings.TrimPrefix(err.Error(), charstore.ErrInvalidPick.Error()+": ")
			s.render(w, "create_clan.html", data)
			return
		}
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
		return
	}

	focus := r.URL.Query().Get("clan")
	data, err := s.buildClanStepData(id, focus)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build clan step data:", err)
		return
	}

	// The picker's row hrefs point back at THIS route (not /clans/{slug}
	// directly) so a JS-off click still lands inside the creation flow —
	// creation-picker.js's fragment fetch therefore hits this same GET
	// handler with ?fragment=1, not handleClanDetail's.
	//
	// The fragment is the detail card PLUS this character's ability-increase
	// picker, not the bare card: which abilities a clan's increases may go
	// to is a per-character choice that has to move with the focused clan,
	// so it cannot live in a fixed confirm form outside the swapped pane.
	// Same reasoning (and same shape) as the class and background steps.
	if r.URL.Query().Get("fragment") == "1" {
		if data["FocusDetail"] == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["create_clan.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render clan step fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "create_clan_focus", data); err != nil {
			log.Println("render clan step fragment:", err)
		}
		return
	}

	s.render(w, "create_clan.html", data)
}

// buildClanStepData assembles the clan step's page context — the clan list,
// and (when focus names a clan) that clan's detail card plus the
// ability-increase picker for it. Shared by the GET render, the ?fragment=1
// pane swap and the POST rejected-pick re-render, same as
// buildClassStepData/buildBackgroundStepData.
func (s *server) buildClanStepData(characterID int64, focus string) (map[string]any, error) {
	var current sql.NullString
	if err := s.charDB.QueryRow(`SELECT clan_slug FROM characters WHERE id = ?`, characterID).Scan(&current); err != nil {
		return nil, err
	}

	rows, err := s.rulesDB.Query(`SELECT slug, name, epithet FROM clans`)
	if err != nil {
		return nil, err
	}
	var clans []clanOption
	for rows.Next() {
		var c clanOption
		var epithet sql.NullString
		if err := rows.Scan(&c.Slug, &c.Name, &epithet); err != nil {
			rows.Close()
			return nil, err
		}
		c.Epithet = epithet.String
		clans = append(clans, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(clans, func(i, k int) bool { return sortKey(clans[i].Name) < sortKey(clans[k].Name) })

	if focus == "" {
		focus = current.String
	}
	data := map[string]any{
		"Title": "Choose a Clan", "ID": characterID, "Clans": clans,
		"Selected": current.String, "Focus": focus,
	}
	if focus != "" {
		if detail, err := loadClanDetail(s.rulesDB, focus); err == nil {
			data["FocusDetail"] = detail
		}
		// unknown slug (stale query param) — fall through with no FocusDetail,
		// same graceful-degrade convention buildClassStepData already uses.
		variants, err := charstore.ClanAbilityVariants(s.rulesDB, focus)
		if err != nil {
			return nil, err
		}
		data["ASIVariants"] = variants
		data["ASIChosen"] = s.loadClanAbilityPicks(characterID, focus, variants)
	}
	return data, nil
}

// loadClanAbilityPicks reads back the increases already stored for this
// character's clan so revisiting the step shows what was picked rather than
// silently resetting every dropdown to its first option.
//
// Returned as one map per variant slot index, keyed by variant: a stored
// bonus is matched to the first slot of the same amount that offers it and
// hasn't already been claimed. That is enough because a variant never has
// two slots of the same amount offering the same ability.
func (s *server) loadClanAbilityPicks(characterID int64, clanSlug string, variants []charstore.AbilityVariant) map[int][]string {
	// Start from the defaults so every dropdown is pre-selected with a
	// legal answer even for a clan this character has never chosen —
	// see charstore.DefaultPicks for why "whatever the browser picks
	// first" is not good enough.
	defaults := map[int][]string{}
	for vi, variant := range variants {
		defaults[vi] = charstore.DefaultPicks(variant)
	}

	rows, err := s.charDB.Query(
		`SELECT ability, amount FROM character_ability_bonuses
		 WHERE character_id = ? AND source_kind = 'clan' AND source_ref = ?`, characterID, clanSlug)
	if err != nil {
		return defaults
	}
	defer rows.Close()
	type stored struct {
		ability string
		amount  int
	}
	var have []stored
	for rows.Next() {
		var st stored
		if err := rows.Scan(&st.ability, &st.amount); err != nil {
			return defaults
		}
		have = append(have, st)
	}
	if len(have) == 0 {
		return defaults
	}

	out := map[int][]string{}
	for vi, variant := range variants {
		picks := append([]string(nil), defaults[vi]...)
		used := make([]bool, len(have))
		for si, slot := range variant.Slots {
			matched := false
			for hi, st := range have {
				if used[hi] || st.amount != slot.Amount {
					continue
				}
				for _, option := range slot.Options {
					if option == st.ability {
						picks[si] = st.ability
						used[hi] = true
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}
		out[vi] = picks
	}
	return out
}

// ---- Class step -----------------------------------------------------------

type classOption struct {
	Slug, Name string
	HitDie     sql.NullInt64
	ChakraDie  sql.NullInt64
}

type skillChoiceOption struct {
	Skill    string
	Selected bool
}

// toolkitChoiceSlot is one "pick a toolkit" dropdown on the class step.
// Prompt is the book's own wording for the row it came from ("Select Any
// two Toolkits"), repeated on every slot that row produced so the player
// can see which instruction they're answering; Chosen is the already-saved
// pick, so revisiting the step doesn't quietly reset it.
type toolkitChoiceSlot struct {
	Prompt string
	Chosen string
}

// loadToolkitChoiceSlots expands a class's choose-a-toolkit proficiency
// rows into one slot per pick. See charstore.ToolkitChoiceCount for why
// these rows need expanding at all: the ingest stored the book's prose
// verbatim, so a class that grants two toolkits of the player's choice has
// a single tool proficiency whose value is the instruction itself.
func (s *server) loadToolkitChoiceSlots(classSlug string) ([]toolkitChoiceSlot, error) {
	if classSlug == "" {
		return nil, nil
	}
	rows, err := s.rulesDB.Query(`
		SELECT value FROM class_proficiencies
		WHERE class_slug = ? AND kind = 'tool' ORDER BY rowid`, classSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slots []toolkitChoiceSlot
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		for n := charstore.ToolkitChoiceCount(value); n > 0; n-- {
			slots = append(slots, toolkitChoiceSlot{Prompt: value})
		}
	}
	return slots, rows.Err()
}

// handleCreateClass is the creation flow's Class step — a thin wrapper
// around the shared renderClassStep/submitClassStep, which also back the
// sheet's own Add a Class page (handleSheetClass). The two differ only in
// which template renders and where a successful pick sends the player.
func (s *server) handleCreateClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	idStr := strconv.FormatInt(id, 10)
	actionBase := "/characters/" + idStr + "/create/class"

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		s.submitClassStep(w, r, id, "create_class.html", actionBase, "/characters/"+idStr+"/create")
		return
	}
	s.renderClassStep(w, r, id, "create_class.html", actionBase)
}

// handleSheetClass is the sheet's own "Add a Class" page, reached from the
// sheet header once creation is finished — the same picker
// (renderClassStep/submitClassStep) the creation flow's Class step uses,
// just landing back on the sheet instead of the creation checklist. Also
// covers the edge case of a character with zero classes (e.g. their only
// class was just removed) by falling through to the same first-class
// SetClass flow submitClassStep already has.
func (s *server) handleSheetClass(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	idStr := strconv.FormatInt(id, 10)
	actionBase := "/characters/" + idStr + "/sheet/class"

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		s.submitClassStep(w, r, id, "sheet_class.html", actionBase, "/characters/"+idStr)
		return
	}
	s.renderClassStep(w, r, id, "sheet_class.html", actionBase)
}

// handleSheetClassRemove/Subclass/Level are the sheet's Add a Class page's
// routes for classPickerRemove/Subclass/Level — see
// handleCreateClassRemove/Subclass/Level's own doc for the shared core.
func (s *server) handleSheetClassRemove(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.classPickerRemove(w, r, id, "/characters/"+strconv.FormatInt(id, 10)+"/sheet/class")
}

func (s *server) handleSheetClassSubclass(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.classPickerSubclass(w, r, id, "/characters/"+strconv.FormatInt(id, 10)+"/sheet/class")
}

func (s *server) handleSheetClassLevel(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.classPickerLevel(w, r, id, "/characters/"+strconv.FormatInt(id, 10)+"/sheet/class")
}

// renderClassStep is the GET side of the class picker, shared by
// handleCreateClass and handleSheetClass — same buildClassStepData, same
// fragment-render branch for JS-driven picker clicks (creation-picker.js),
// just a different template name/action base.
func (s *server) renderClassStep(w http.ResponseWriter, r *http.Request, characterID int64, templateName, actionBase string) {
	currentClass, err := s.primaryClassSlug(characterID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query current class:", err)
		return
	}
	var currentLevel sql.NullInt64
	if currentClass != "" {
		if err := s.charDB.QueryRow(
			`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`, characterID, currentClass,
		).Scan(&currentLevel); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("query current level:", err)
			return
		}
	}

	focus := r.URL.Query().Get("class")
	if focus == "" {
		focus = currentClass
	}
	data, err := s.buildClassStepData(characterID, currentClass, focus, nil, actionBase)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build class step data:", err)
		return
	}
	// Defaults to 1 for a character with no class yet, same as SetClass's
	// own default — the dropdown should read 1 rather than 0 the first
	// time this step is ever opened. Only pre-fills the PRIMARY class's
	// current level when focus IS the primary (re-editing it) — a
	// multiclass-add's level field must start at 1 regardless of what the
	// primary class's own level happens to be, not silently default to a
	// number that may not even fit under MaxLevel.
	startLevel := 1
	if focus == currentClass && currentLevel.Valid {
		startLevel = int(currentLevel.Int64)
	}
	data["CurrentLevel"] = startLevel

	// Same reasoning as handleCreateClan's fragment branch: the picker's
	// row hrefs point back at this route (not /classes/{slug}) so a JS-off
	// click stays inside the creation flow, so creation-picker.js's fetch
	// hits this handler with ?fragment=1 too. Unlike Clan, the fragment
	// needed here is a combined block (rich class detail + this
	// character's skill-choice form for that class), not the bare detail
	// card alone — see create_class_focus in partials/class_picker.html.
	if r.URL.Query().Get("fragment") == "1" {
		if data["FocusDetail"] == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates[templateName]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render class step fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "create_class_focus", data); err != nil {
			log.Println("render class step fragment:", err)
		}
		return
	}

	s.render(w, templateName, data)
}

// submitClassStep handles POSTing a class pick on either the creation
// flow's Class step or the sheet's own Add a Class page — the character's
// first class (the original full SetClass grant) or an additional one
// (submitMulticlassAdd), whichever the submitted class_slug calls for.
// templateName/actionBase pick which page's chrome a validation failure
// re-renders with; successRedirect is where a successful pick sends the
// player. r.ParseForm must already have been called.
func (s *server) submitClassStep(w http.ResponseWriter, r *http.Request, characterID int64, templateName, actionBase, successRedirect string) {
	classSlug := r.FormValue("class_slug")
	if classSlug == "" {
		http.Error(w, "class_slug required", http.StatusBadRequest)
		return
	}

	currentClass, err := s.primaryClassSlug(characterID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query current class:", err)
		return
	}
	existingClasses, err := s.loadCharacterClassLevels(characterID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load existing classes:", err)
		return
	}
	// The primary class stays fully re-editable through the original
	// SetClass flow below even though it's technically "held" — only a
	// held SECONDARY class's form is a no-op (change it by removing and
	// re-adding instead), and only a not-yet-held class routes into the
	// multiclass-add path.
	isPrimary := classSlug == currentClass
	held := false
	for _, c := range existingClasses {
		if c.Slug == classSlug {
			held = true
		}
	}
	if held && !isPrimary {
		// A stale/double-submitted form for an already-held secondary
		// class is a no-op, not an error.
		http.Redirect(w, r, actionBase, http.StatusSeeOther)
		return
	}

	if len(existingClasses) > 0 && !isPrimary {
		ok, errMsg, err := s.submitMulticlassAdd(r, characterID, classSlug, existingClasses)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add multiclass:", err)
			return
		}
		if !ok {
			data, buildErr := s.buildClassStepData(characterID, currentClass, classSlug, nil, actionBase)
			if buildErr != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("build class step data:", buildErr)
				return
			}
			if lvl, convErr := strconv.Atoi(strings.TrimSpace(r.FormValue("level"))); convErr == nil {
				data["CurrentLevel"] = lvl
			}
			data["ChosenSkill"] = r.FormValue("mc_skill")
			data["ChosenToolkit"] = r.FormValue("mc_toolkit")
			data["ChosenWeapon"] = r.FormValue("mc_weapon")
			data["Error"] = errMsg
			s.render(w, templateName, data)
			return
		}
		http.Redirect(w, r, successRedirect, http.StatusSeeOther)
		return
	}

	// ---- first class (or re-editing the primary): unchanged full-grant flow
	// Defaults to 1 (an empty/missing field, e.g. a pre-existing bookmarked
	// form) rather than rejecting the submission — starting level is a
	// convenience for campaigns that don't begin at 1st level, not a
	// required decision.
	levelStr := strings.TrimSpace(r.FormValue("level"))
	level := 1
	if levelStr != "" {
		var convErr error
		level, convErr = strconv.Atoi(levelStr)
		if convErr != nil || level < 1 || level > 20 {
			data, buildErr := s.buildClassStepData(characterID, currentClass, classSlug, nil, actionBase)
			if buildErr != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("build class step data:", buildErr)
				return
			}
			data["CurrentLevel"] = 1
			data["Error"] = "Starting level must be a whole number from 1 to 20."
			s.render(w, templateName, data)
			return
		}
	}
	chosenSkills := r.Form["skills"]

	var chooseN int
	if err := s.rulesDB.QueryRow(`
		SELECT choose_n FROM class_proficiencies
		WHERE class_slug = ? AND kind = 'skill_choice' LIMIT 1`, classSlug,
	).Scan(&chooseN); err != nil && err != sql.ErrNoRows {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query skill choice_n:", err)
		return
	}

	// Toolkit picks, one per slot the class's prose asks for. Read
	// positionally (toolkit_0, toolkit_1, …) rather than as a repeated
	// field so a blank slot stays a blank at its own index instead of
	// shifting the ones after it.
	toolkitSlots, err := s.loadToolkitChoiceSlots(classSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load toolkit choice slots:", err)
		return
	}
	// Checked against the offered list, not just against "non-empty". The
	// dropdown only lists starting-tier kits, but a hand-built POST is not
	// bound by the dropdown, and the whole point of the restriction is
	// that a character cannot begin play holding a 1200-Ryo Supreme kit.
	// Same validation standard the background step's duplicate-pick check
	// already holds to.
	offeredToolkits, err := s.loadToolkitOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load toolkit options:", err)
		return
	}
	offeredNames := make(map[string]bool, len(offeredToolkits))
	for _, t := range offeredToolkits {
		offeredNames[t.Name] = true
	}
	chosenToolkits := make([]string, len(toolkitSlots))
	missingToolkit := false
	badToolkit := false
	for i := range toolkitSlots {
		chosenToolkits[i] = strings.TrimSpace(r.FormValue("toolkit_" + strconv.Itoa(i)))
		switch {
		case chosenToolkits[i] == "":
			missingToolkit = true
		case !offeredNames[chosenToolkits[i]]:
			badToolkit = true
		}
	}

	if (chooseN > 0 && len(chosenSkills) != chooseN) || missingToolkit || badToolkit {
		chosenSet := make(map[string]bool, len(chosenSkills))
		for _, sk := range chosenSkills {
			chosenSet[sk] = true
		}
		data, err := s.buildClassStepData(characterID, currentClass, classSlug, chosenSet, actionBase)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build class step data:", err)
			return
		}
		data["CurrentLevel"] = level
		// Keep the player's partial toolkit picks visible instead of
		// wiping the ones they did fill in.
		if slots, ok := data["ToolkitChoices"].([]toolkitChoiceSlot); ok {
			for i := range slots {
				if i < len(chosenToolkits) && chosenToolkits[i] != "" {
					slots[i].Chosen = chosenToolkits[i]
				}
			}
		}
		switch {
		case chooseN > 0 && len(chosenSkills) != chooseN:
			data["Error"] = "Choose exactly " + strconv.Itoa(chooseN) + " skills."
		case badToolkit:
			data["Error"] = "Characters start with basic toolkits only — Greater, Superior and Supreme kits are bought in play."
		default:
			data["Error"] = "Pick a toolkit for every slot."
		}
		s.render(w, templateName, data)
		return
	}

	if err := charstore.SetClass(s.charDB, s.rulesDB, characterID, classSlug, chosenSkills, chosenToolkits); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set class:", err)
		return
	}
	// SetClass always starts a class at level 1; raise it here, same call
	// the sheet's own per-class level control uses, so a level-2+ start
	// gains HP/Chakra/features exactly as if the character had levelled up
	// that far normally (no separate creation-time formula to keep in sync
	// with charsheet.Compute's).
	if level != 1 {
		if err := charstore.SetLevel(s.charDB, characterID, level); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set starting level:", err)
			return
		}
	}
	http.Redirect(w, r, successRedirect, http.StatusSeeOther)
}

// removeCharacterClass drops classSlug entirely from the character —
// resolving its subclass siblings against rules.db here (charstore never
// touches rules.db) before handing off to charstore.RemoveClass. Shared by
// the creation flow's Class step and the sheet's "Your Classes" summary,
// which both offer the same Remove action.
func (s *server) removeCharacterClass(characterID int64, classSlug string) error {
	siblings, err := s.loadSubclassOptions(classSlug)
	if err != nil {
		return err
	}
	siblingSlugs := make([]string, len(siblings))
	for i, opt := range siblings {
		siblingSlugs[i] = opt.Slug
	}
	return charstore.RemoveClass(s.charDB, characterID, classSlug, siblingSlugs)
}

// classPickerRemove drops one class from the character entirely (form
// field "class_slug") — shared core behind the creation flow's and the
// sheet's own "Your Classes" Remove button, which both just redirect back
// to their own page afterward. POST-only and confirm-guarded client-side
// (confirm-submit.js), same convention handleDeleteCharacter already
// follows for a destructive action reached from a plain list.
func (s *server) classPickerRemove(w http.ResponseWriter, r *http.Request, characterID int64, redirectTo string) {
	classSlug := strings.TrimSpace(r.FormValue("class_slug"))
	if classSlug == "" {
		http.Error(w, "class_slug required", http.StatusBadRequest)
		return
	}
	if err := s.removeCharacterClass(characterID, classSlug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove class:", err)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// classPickerSubclass is the shared core behind the creation flow's and the
// sheet's own "Your Classes" subclass picker (form fields "class_slug",
// "subclass_slug") — setCharacterSubclass plus a plain redirect back to
// whichever page called it, rather than the sheet's main subclass control's
// fetch-and-swap fragment (handleSheetSubclass), since both these pages
// stay plain multi-page forms throughout.
func (s *server) classPickerSubclass(w http.ResponseWriter, r *http.Request, characterID int64, redirectTo string) {
	classSlug := strings.TrimSpace(r.FormValue("class_slug"))
	subclassSlug := strings.TrimSpace(r.FormValue("subclass_slug"))
	if err := s.setCharacterSubclass(characterID, classSlug, subclassSlug); err != nil {
		writeSubclassError(w, err)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// classPickerLevel changes one already-held class's level in place (form
// fields "class_slug", "level") — the shared core behind the creation
// flow's and the sheet's own "Your Classes" inline level field, reached so
// a player who added a class at the wrong level isn't forced to remove and
// re-add it.
func (s *server) classPickerLevel(w http.ResponseWriter, r *http.Request, characterID int64, redirectTo string) {
	classSlug := strings.TrimSpace(r.FormValue("class_slug"))
	level, convErr := strconv.Atoi(strings.TrimSpace(r.FormValue("level")))
	if classSlug == "" || convErr != nil || level < 1 || level > 20 {
		http.Error(w, "class_slug and a level from 1 to 20 are required", http.StatusBadRequest)
		return
	}
	if err := charstore.SetClassLevel(s.charDB, characterID, classSlug, level); err != nil {
		if errors.Is(err, charstore.ErrLevelCapExceeded) {
			http.Error(w, "that would push the character's total level above 20", http.StatusBadRequest)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set class level:", err)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// handleCreateClassRemove/Subclass/Level are the creation flow's Class
// step routes for classPickerRemove/Subclass/Level; handleSheetClassRemove/
// Subclass/Level (below, near the other sheet handlers) are the sheet's own
// Add a Class page's equivalents — same core, different redirect target.
func (s *server) handleCreateClassRemove(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.classPickerRemove(w, r, id, "/characters/"+strconv.FormatInt(id, 10)+"/create/class")
}

func (s *server) handleCreateClassSubclass(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.classPickerSubclass(w, r, id, "/characters/"+strconv.FormatInt(id, 10)+"/create/class")
}

func (s *server) handleCreateClassLevel(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.classPickerLevel(w, r, id, "/characters/"+strconv.FormatInt(id, 10)+"/create/class")
}

// buildClassStepData assembles the full page context for create_class.html
// — the class list, and (when focus is set) that class's skill-choice
// pool — shared by the GET render and the POST validation-error re-render
// so a rejected skill count doesn't strand the user on a page missing the
// class list and pool it needs to correct the mistake. selectedOverride,
// when non-nil, marks which skills should show checked (the user's
// just-submitted, rejected attempt); when nil, checked state comes from
// whatever's already saved for the character's current class.
//
// actionBase is the form action prefix the shared class_picker.html
// partial (your_classes_panel/create_class_focus/multiclass_add_form)
// posts every control to — "/characters/{id}/create/class" from the
// creation flow, "/characters/{id}/sheet/class" from the sheet's own Add a
// Class page — the two pages this data map feeds.
func (s *server) buildClassStepData(characterID int64, currentClassSlug, focus string, selectedOverride map[string]bool, actionBase string) (map[string]any, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, hit_die, chakra_die FROM classes`)
	if err != nil {
		return nil, err
	}
	var classes []classOption
	for rows.Next() {
		var c classOption
		if err := rows.Scan(&c.Slug, &c.Name, &c.HitDie, &c.ChakraDie); err != nil {
			rows.Close()
			return nil, err
		}
		classes = append(classes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(classes, func(i, k int) bool { return sortKey(classes[i].Name) < sortKey(classes[k].Name) })

	// Every class the character already has, not just the primary — the
	// "Your Classes" summary block and the picker's "already yours"
	// highlighting/gating both need the full set once multiclassing exists.
	yourClasses, err := s.loadClassSummary(characterID)
	if err != nil {
		return nil, err
	}
	heldSlugs := make(map[string]bool, len(yourClasses))
	for _, c := range yourClasses {
		heldSlugs[c.ClassSlug] = true
	}

	data := map[string]any{
		"Title": "Choose a Class", "ID": characterID, "Classes": classes, "Current": currentClassSlug, "Focus": focus,
		"YourClasses": yourClasses, "HeldSlugs": heldSlugs, "ActionBase": actionBase,
		// A fixed 1-20 range (same bound SetClassLevel enforces) needed by
		// every level dropdown this page renders — the first-class picker,
		// the multiclass-add picker (capped further per-render via
		// MaxLevel), and each "Your Classes" row's own inline level field —
		// set once here instead of by every caller that builds this data.
		"LevelOptions": levelOptions,
	}
	if focus == "" {
		return data, nil
	}

	detail, err := s.loadClassDetail(focus)
	if err != nil {
		return data, nil // unknown slug (e.g. stale query param) — show the list with no focus panel
	}
	data["FocusDetail"] = detail
	data["FocusName"] = detail.Name

	alreadyHeld := heldSlugs[focus]
	// A held SECONDARY class shows read-only ("remove it above to change
	// your picks"); the PRIMARY class stays fully re-editable below exactly
	// as before multiclassing existed — focus == currentClassSlug is what
	// the original hydrate-for-editing logic already keyed off, so that
	// path is untouched, just gated out of the new read-only branch.
	alreadyHeldSecondary := alreadyHeld && focus != currentClassSlug
	data["AlreadyHeld"] = alreadyHeldSecondary

	// Once the character has a class, focusing a DIFFERENT, not-yet-held
	// class reads as "add this as an additional class" — a much smaller
	// grant (charstore.MulticlassGrantChoices) than the full first-level
	// pool below, which stays reserved for choosing/re-editing the primary
	// class itself.
	isMulticlassAdd := len(yourClasses) > 0 && !alreadyHeld
	data["IsMulticlassAdd"] = isMulticlassAdd
	if isMulticlassAdd {
		// Always present (even unused ones) so the template's <select>
		// pickers can compare against them without a missing-map-key
		// template error; the POST error-rerender path overwrites these
		// with what the player actually submitted.
		data["ChosenSkill"] = ""
		data["ChosenToolkit"] = ""
		data["ChosenWeapon"] = ""
		fields, err := s.loadClassAddGrantFields(characterID, focus)
		if err != nil {
			return nil, err
		}
		for k, v := range fields {
			data[k] = v
		}
		// Best-effort inline prerequisite warning so the player sees it
		// before submitting — the raw ability_prereq_text already shown by
		// FocusDetail's MulticlassRules block says what's required, this
		// says whether it's currently met. The server-side check on submit
		// (submitMulticlassAdd -> charstore.UnmetMulticlassClasses) is
		// authoritative either way.
		if scores, err := s.characterAbilityScores(characterID); err == nil {
			checkSlugs := make([]string, 0, len(yourClasses)+1)
			for _, c := range yourClasses {
				checkSlugs = append(checkSlugs, c.ClassSlug)
			}
			checkSlugs = append(checkSlugs, focus)
			unmet := charstore.UnmetMulticlassClasses(scores, checkSlugs)
			unmetNames := make([]string, len(unmet))
			for i, slug := range unmet {
				unmetNames[i] = s.className(slug)
			}
			data["UnmetPrereqClasses"] = unmetNames
		}
		return data, nil
	}
	if alreadyHeldSecondary {
		return data, nil
	}

	chooseRows, err := s.rulesDB.Query(`
		SELECT value, choose_n FROM class_proficiencies
		WHERE class_slug = ? AND kind = 'skill_choice'`, focus)
	if err != nil {
		return nil, err
	}
	already := selectedOverride
	if already == nil && focus == currentClassSlug {
		already = s.loadCharacterClassSkillChoices(characterID, focus)
	}
	var chooseN int
	var choices []skillChoiceOption
	for chooseRows.Next() {
		var skill string
		var n int
		if err := chooseRows.Scan(&skill, &n); err != nil {
			chooseRows.Close()
			return nil, err
		}
		chooseN = n
		choices = append(choices, skillChoiceOption{Skill: skill, Selected: already[skill]})
	}
	chooseRows.Close()
	if err := chooseRows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(choices, func(i, k int) bool { return sortKey(choices[i].Skill) < sortKey(choices[k].Skill) })
	data["SkillChoices"] = choices
	data["ChooseN"] = chooseN

	slots, err := s.loadToolkitChoiceSlots(focus)
	if err != nil {
		return nil, err
	}
	if len(slots) > 0 {
		// Only hydrate when this is the class the character actually has —
		// browsing a different class's detail pane must not show another
		// class's saved picks.
		if focus == currentClassSlug {
			saved := s.loadCharacterClassToolkits(characterID, focus)
			// SetClass writes the fixed tool grants first and the player's
			// slot picks last, so the picks are the tail of this list —
			// taking the tail keeps a class that has BOTH (a named kit plus
			// "and one of your choice") from hydrating the dropdowns with
			// its own automatic grants.
			if len(saved) > len(slots) {
				saved = saved[len(saved)-len(slots):]
			}
			for i := range saved {
				slots[i].Chosen = saved[i]
			}
		}
		toolkits, err := s.loadToolkitOptions()
		if err != nil {
			return nil, err
		}
		data["ToolkitChoices"] = slots
		data["Toolkits"] = toolkits
	}
	return data, nil
}

// loadCharacterClassToolkits returns this character's class-granted tool
// proficiencies in insertion order, which is the order SetClass wrote the
// player's slot picks in. Best-effort: a query error here just means the
// dropdowns start empty, which is recoverable, unlike failing the page.
func (s *server) loadCharacterClassToolkits(characterID int64, classSlug string) []string {
	rows, err := s.charDB.Query(`
		SELECT value FROM character_proficiencies
		WHERE character_id = ? AND source_kind = 'class' AND source_ref = ? AND kind = 'tool'
		ORDER BY rowid`, characterID, classSlug)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return out
		}
		out = append(out, v)
	}
	return out
}

func (s *server) loadCharacterClassSkillChoices(characterID int64, classSlug string) map[string]bool {
	out := map[string]bool{}
	rows, err := s.charDB.Query(`
		SELECT value FROM character_proficiencies
		WHERE character_id = ? AND source_kind = 'class' AND source_ref = ? AND kind = 'skill'`, characterID, classSlug)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil {
			out[v] = true
		}
	}
	return out
}

// ---- Ability scores step --------------------------------------------------

func (s *server) handleCreateAbilities(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		method := r.FormValue("method")
		scores := map[string]int{}
		valid := true
		for _, ab := range [6]string{"str", "dex", "con", "int", "wis", "cha"} {
			n, err := strconv.Atoi(r.FormValue(ab))
			if err != nil {
				valid = false
				break
			}
			scores[ab] = n
		}

		var errMsg string
		switch {
		case !valid:
			errMsg = "Every ability score must be a whole number."
		case method == "standard":
			got := []int{scores["str"], scores["dex"], scores["con"], scores["int"], scores["wis"], scores["cha"]}
			want := []int{15, 14, 13, 12, 11, 10}
			sort.Ints(got)
			sort.Ints(want)
			for i := range got {
				if got[i] != want[i] {
					errMsg = "Standard Array must use exactly 15/14/13/12/11/10, one per ability."
					break
				}
			}
		case method == "pointbuy":
			costs, err := s.loadAbilityPointCosts()
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load ability point costs:", err)
				return
			}
			total := 0
			for _, ab := range [6]string{"str", "dex", "con", "int", "wis", "cha"} {
				score := scores[ab]
				cost, ok := costs[score]
				if !ok {
					errMsg = "Point Buy scores must be between 8 and 15."
					break
				}
				total += cost
			}
			if errMsg == "" && total > 30 {
				errMsg = "Point Buy budget is 30 points; that totals " + strconv.Itoa(total) + "."
			}
		}
		// method == "manual" (or anything else): no validation, trust the player.

		if errMsg != "" {
			costs, err := s.loadAbilityPointCosts()
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load ability point costs:", err)
				return
			}
			var costRows []struct{ Score, Cost int }
			for score := 8; score <= 15; score++ {
				costRows = append(costRows, struct{ Score, Cost int }{score, costs[score]})
			}
			s.render(w, "create_abilities.html", map[string]any{
				"Title": "Ability Scores", "ID": id, "Error": errMsg, "Values": scores, "Costs": costRows, "CostMap": costs,
			})
			return
		}

		if err := charstore.SetAbilities(s.charDB, id,
			scores["str"], scores["dex"], scores["con"], scores["int"], scores["wis"], scores["cha"],
		); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set abilities:", err)
			return
		}
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
		return
	}

	var str, dex, con, intel, wis, cha int
	err = s.charDB.QueryRow(`
		SELECT base_str, base_dex, base_con, base_int, base_wis, base_cha
		FROM characters WHERE id = ?`, id,
	).Scan(&str, &dex, &con, &intel, &wis, &cha)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query character abilities:", err)
		return
	}

	costs, err := s.loadAbilityPointCosts()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load ability point costs:", err)
		return
	}
	var costRows []struct{ Score, Cost int }
	for score := 8; score <= 15; score++ {
		costRows = append(costRows, struct{ Score, Cost int }{score, costs[score]})
	}

	s.render(w, "create_abilities.html", map[string]any{
		"Title": "Ability Scores", "ID": id,
		"Values":  map[string]int{"str": str, "dex": dex, "con": con, "int": intel, "wis": wis, "cha": cha},
		"Costs":   costRows,
		"CostMap": costs,
	})
}

func (s *server) loadAbilityPointCosts() (map[int]int, error) {
	rows, err := s.rulesDB.Query(`SELECT score, cost FROM ability_point_costs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	costs := map[int]int{}
	for rows.Next() {
		var score, cost int
		if err := rows.Scan(&score, &cost); err != nil {
			return nil, err
		}
		costs[score] = cost
	}
	return costs, rows.Err()
}

// ---- Background step --------------------------------------------------

// chooseFromPattern matches the two clean "choice" shapes found in
// background_proficiencies.value: "Choose two from Ninshou, Martial Arts,
// Illusions" and "Choose one between Poison Kit, Medicine Kit, Trappers
// Kit" — anything else falls back to free text, per the plan.
var chooseFromPattern = regexp.MustCompile(`^Choose (\w+)(?: from| between)? (.+)$`)

// compoundGrantPattern splits a value that grants something outright AND
// then asks for a choice: "Acrobatics, Choose one Ninshou, Martial Arts,
// Illusions". Read as one string, the whole sentence became the name of a
// single proficiency — the character ended up proficient with "Acrobatics,
// Choose one Ninshou, Martial Arts, Illusions" and was never asked to
// choose. Same class of ingest artefact as charstore.ToolkitChoiceCount's.
var compoundGrantPattern = regexp.MustCompile(`^(.+?),\s*(Choose\s+.+)$`)

// andOrGrantPattern covers the other compound shape in this column:
// "History and Deception or Persuasion" — History is granted, then one of
// the remaining two is chosen. Anchored on " and " followed by an " or ",
// so a plain "X or Y" (a pure choice) and a plain "X and Y" (two grants)
// both fall through untouched.
var andOrGrantPattern = regexp.MustCompile(`^([^,]+?)\s+and\s+(.+?\s+or\s+.+)$`)

// toolYourChoicePattern matches the open-ended tool grant shape ("One of
// your choice") — unlike chooseFromPattern's named list, this has no fixed
// option set, so classifyBackgroundProfs fills Options from the full
// toolkit catalog instead of parsing names out of the value text.
var toolYourChoicePattern = regexp.MustCompile(`(?i)your choice`)

var chooseWordToN = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5}

type backgroundOption struct {
	Slug, Name, Description string
}

// backgroundProfRow is one background_proficiencies row, classified for
// rendering: Options non-empty means a real choose-N-from-pool picker;
// otherwise IsChoice+empty Options means the unparsed "between" fallback
// (free text); otherwise it's an automatic grant with nothing to pick.
type backgroundProfRow struct {
	Index       int
	Kind, Value string
	IsChoice    bool
	ChooseN     int
	Options     []string
	OptionDescs []string // parallel to Options; "" where Kind isn't "skill" or no blurb exists
	OptionSlugs []string // parallel to Options; "" where Kind isn't "tool" or no catalog match exists
	// SlotValues is exactly ChooseN long — SlotValues[slot] is the
	// already-saved pick for that dropdown slot on revisit, or "" if that
	// slot was never filled. Distributing already-chosen names across
	// slots (rather than just flagging "is this option chosen somewhere")
	// matters because each slot is its own independent <select>: marking
	// the same option "selected" in every slot's dropdown would make them
	// all show the same value instead of each showing its own pick.
	SlotValues []string
	ValueDesc  string // skill blurb for the non-choice/automatic-grant case
}

// backgroundSkillOption pairs an option name with its skill blurb for the
// template's dropdown rendering — Go templates can't zip parallel slices
// by index on their own.
type backgroundSkillOption struct {
	Name string
	Desc string
	Slug string // toolkit catalog slug for "View stats", "" for skill options
}

// loadCharacterBackgroundChoices returns which (kind,value) pairs are
// already saved for this character's background — same purpose as
// loadCharacterClassSkillChoices, generalized to cover both skill and tool
// picks (background choices aren't skill-only the way class skill_choice
// rows are). Keyed "kind|value" since a tool and a skill could in
// principle share a display name.
func (s *server) loadCharacterBackgroundChoices(characterID int64, backgroundSlug string) map[string]bool {
	out := map[string]bool{}
	rows, err := s.charDB.Query(`
		SELECT kind, value FROM character_proficiencies
		WHERE character_id = ? AND source_kind = 'background' AND source_ref = ?`, characterID, backgroundSlug)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if rows.Scan(&kind, &value) == nil {
			out[kind+"|"+value] = true
		}
	}
	return out
}

// classifyBackgroundProfs classifies background_proficiencies rows for
// rendering. alreadyChosen (keyed "kind|value", from
// loadCharacterBackgroundChoices) marks which pool options should render
// pre-selected on revisit — nil is fine (e.g. the POST validation path,
// which only needs the pool/ChooseN shape, not hydration). toolkits backs
// two tool-kind cases: chooseFromPattern's named list (for "View stats"
// slugs) and the open "One of your choice" grant, whose Options is the
// full catalog rather than names parsed out of the value text.
func classifyBackgroundProfs(rulesDB *sql.DB, backgroundSlug string, alreadyChosen map[string]bool, toolkits []toolkitOption) ([]backgroundProfRow, error) {
	toolkitSlugByName := make(map[string]string, len(toolkits))
	for _, t := range toolkits {
		toolkitSlugByName[strings.ToLower(t.Name)] = t.Slug
	}

	rows, err := rulesDB.Query(`
		SELECT kind, value FROM background_proficiencies WHERE background_slug = ? ORDER BY rowid`, backgroundSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []backgroundProfRow
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, err
		}
		for _, part := range splitCompoundGrant(value) {
			out = append(out, classifyBackgroundProfValue(kind, part, len(out), alreadyChosen, toolkits, toolkitSlugByName))
		}
	}
	return out, rows.Err()
}

// splitCompoundGrant breaks a background_proficiencies value that mixes an
// automatic grant with a choice into its separate clauses, so each becomes
// its own row (an "(automatic)" line and a real dropdown) instead of one
// proficiency named after the whole sentence.
//
// Returns the value unchanged — a one-element slice — for every value that
// is purely one thing, which is all but two of them today.
func splitCompoundGrant(value string) []string {
	value = strings.TrimSpace(value)
	if m := compoundGrantPattern.FindStringSubmatch(value); m != nil {
		return []string{strings.TrimSpace(m[1]), strings.TrimSpace(m[2])}
	}
	if m := andOrGrantPattern.FindStringSubmatch(value); m != nil {
		// Restated in the "Choose one from A, B" shape the parser below
		// already understands, rather than teaching that parser a second
		// grammar for the same idea.
		options := strings.ReplaceAll(m[2], " or ", ", ")
		return []string{strings.TrimSpace(m[1]), "Choose one from " + strings.TrimSpace(options)}
	}
	return []string{value}
}

// classifyBackgroundProfValue classifies one clause. Split out of
// classifyBackgroundProfs so a compound value can be run through it once per
// clause.
func classifyBackgroundProfValue(kind, value string, index int, alreadyChosen map[string]bool, toolkits []toolkitOption, toolkitSlugByName map[string]string) backgroundProfRow {
	row := backgroundProfRow{Index: index, Kind: kind, Value: value}
	switch {
	case chooseFromPattern.MatchString(value):
		m := chooseFromPattern.FindStringSubmatch(value)
		row.IsChoice = true
		row.ChooseN = chooseWordToN[strings.ToLower(m[1])]
		for _, opt := range strings.Split(m[2], ",") {
			row.Options = append(row.Options, strings.TrimSpace(opt))
		}
	case kind == "tool" && toolYourChoicePattern.MatchString(value):
		row.IsChoice = true
		row.ChooseN = 1
		for _, t := range toolkits {
			row.Options = append(row.Options, t.Name)
		}
	case strings.HasPrefix(value, "Choose"):
		row.IsChoice = true // unparsed shape — free-text fallback
	case kind == "skill":
		row.ValueDesc = charsheet.SkillDescriptions[value]
	}
	if len(row.Options) > 0 {
		for _, name := range row.Options {
			desc, slug := "", ""
			switch kind {
			case "skill":
				desc = charsheet.SkillDescriptions[name]
			case "tool":
				slug = toolkitSlugByName[strings.ToLower(name)]
			}
			row.OptionDescs = append(row.OptionDescs, desc)
			row.OptionSlugs = append(row.OptionSlugs, slug)
		}
		row.SlotValues = make([]string, row.ChooseN)
		slot := 0
		for _, name := range row.Options {
			if slot >= row.ChooseN {
				break
			}
			if alreadyChosen[kind+"|"+name] {
				row.SlotValues[slot] = name
				slot++
			}
		}
	}
	return row
}

// skillOptionPairs zips backgroundProfRow's parallel Options/OptionDescs/
// OptionSlugs slices for the dropdown template, which needs {Name, Desc,
// Slug} triples, not three separately-indexed slices.
func skillOptionPairs(row backgroundProfRow) []backgroundSkillOption {
	pairs := make([]backgroundSkillOption, len(row.Options))
	for i, name := range row.Options {
		desc, slug := "", ""
		if i < len(row.OptionDescs) {
			desc = row.OptionDescs[i]
		}
		if i < len(row.OptionSlugs) {
			slug = row.OptionSlugs[i]
		}
		pairs[i] = backgroundSkillOption{Name: name, Desc: desc, Slug: slug}
	}
	return pairs
}

func (s *server) handleCreateBackground(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		backgroundSlug := r.FormValue("background_slug")
		if backgroundSlug == "" {
			http.Error(w, "background_slug required", http.StatusBadRequest)
			return
		}
		toolkits, err := s.loadToolkitOptions()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load toolkit options:", err)
			return
		}
		profs, err := classifyBackgroundProfs(s.rulesDB, backgroundSlug, nil, toolkits) // validation only — hydration doesn't matter here
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("classify background profs:", err)
			return
		}
		var grants []struct{ Kind, Value string }
		var dupErr string
		for _, p := range profs {
			switch {
			case p.IsChoice && len(p.Options) > 0:
				// One dropdown per pick-slot (choice_{idx}_0..N-1) rather
				// than a multi-value checkbox group — exactly N selections
				// by construction. Still checked server-side (not just
				// relying on the dropdowns' own client-side mutual-exclusion
				// JS) since a direct POST or JS-disabled client could submit
				// the same option twice, same validation standard every
				// other step in this flow holds to.
				seen := map[string]bool{}
				for slot := 0; slot < p.ChooseN; slot++ {
					chosen := strings.TrimSpace(r.FormValue("choice_" + strconv.Itoa(p.Index) + "_" + strconv.Itoa(slot)))
					if chosen == "" {
						continue
					}
					if seen[chosen] {
						dupErr = "You picked " + chosen + " twice for " + p.Kind + " — choose " + strconv.Itoa(p.ChooseN) + " different options."
						continue
					}
					seen[chosen] = true
					grants = append(grants, struct{ Kind, Value string }{p.Kind, chosen})
				}
			case p.IsChoice:
				key := "choice_" + strconv.Itoa(p.Index)
				if text := strings.TrimSpace(r.FormValue(key)); text != "" {
					grants = append(grants, struct{ Kind, Value string }{p.Kind, text})
				}
			default:
				grants = append(grants, struct{ Kind, Value string }{p.Kind, p.Value})
			}
		}
		if dupErr != "" {
			data, err := s.buildBackgroundStepData(id, backgroundSlug)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("build background step data:", err)
				return
			}
			data["Error"] = dupErr
			s.render(w, "create_background.html", data)
			return
		}
		if err := charstore.SetBackground(s.charDB, id, backgroundSlug, grants); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set background:", err)
			return
		}
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
		return
	}

	focus := r.URL.Query().Get("background")
	data, err := s.buildBackgroundStepData(id, focus)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build background step data:", err)
		return
	}

	// Same reasoning as handleCreateClass's fragment branch: the grants
	// form is character-scoped (which choices are already saved), so it
	// can't be served by the generic /backgrounds/{slug} route — return
	// the combined detail+form block instead of the bare detail card.
	if r.URL.Query().Get("fragment") == "1" {
		if data["FocusDetail"] == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["create_background.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render background step fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "create_background_focus", data); err != nil {
			log.Println("render background step fragment:", err)
		}
		return
	}

	s.render(w, "create_background.html", data)
}

// buildBackgroundStepData assembles the full page context for
// create_background.html — the background list, and (when focus is set)
// that background's rich detail card plus this character's grant/choice
// form — shared by the GET render and the POST duplicate-choice
// re-render, same reasoning as buildClassStepData.
func (s *server) buildBackgroundStepData(characterID int64, focus string) (map[string]any, error) {
	var currentBackground sql.NullString
	if err := s.charDB.QueryRow(`SELECT background_slug FROM characters WHERE id = ?`, characterID).Scan(&currentBackground); err != nil {
		return nil, err
	}
	if focus == "" {
		focus = currentBackground.String
	}

	rows, err := s.rulesDB.Query(`SELECT slug, name, description FROM backgrounds`)
	if err != nil {
		return nil, err
	}
	var backgrounds []backgroundOption
	for rows.Next() {
		var b backgroundOption
		if err := rows.Scan(&b.Slug, &b.Name, &b.Description); err != nil {
			rows.Close()
			return nil, err
		}
		backgrounds = append(backgrounds, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(backgrounds, func(i, k int) bool { return sortKey(backgrounds[i].Name) < sortKey(backgrounds[k].Name) })

	data := map[string]any{
		"Title": "Choose a Background", "ID": characterID, "Backgrounds": backgrounds,
		"Current": currentBackground.String, "Focus": focus,
	}
	if focus == "" {
		return data, nil
	}

	detail, err := s.loadBackgroundDetail(focus)
	if err != nil {
		return data, nil // unknown slug (e.g. stale query param) — show the list with no focus panel
	}
	data["FocusDetail"] = detail
	data["FocusName"] = detail.Name
	var alreadyChosen map[string]bool
	if focus == currentBackground.String {
		alreadyChosen = s.loadCharacterBackgroundChoices(characterID, focus)
	}
	toolkits, err := s.loadToolkitOptions()
	if err != nil {
		return nil, err
	}
	profs, err := classifyBackgroundProfs(s.rulesDB, focus, alreadyChosen, toolkits)
	if err != nil {
		return nil, err
	}
	data["Profs"] = profs
	return data, nil
}

// ---- Equipment step ------------------------------------------------------

type equipmentChoiceOption struct {
	ChoiceIdx   int
	Description string
	ItemSlug    string
	Quantity    int
	Selected    bool // already saved for this character — hydrated on revisit, see hydrateEquipmentSelections

	// IsKitChoice/KitSlots cover printed bundles like "2 Kits of your
	// Choice" — resolveEquipmentSlug deliberately leaves these unresolved
	// (see equipmentNameLookup's doc comment) since they name a category,
	// not one specific item. Rather than saving the bundle text verbatim,
	// the equipment step renders Quantity real dropdowns of the toolkit
	// catalog (kind='toolkit') so the player picks actual items; KitSlots
	// holds the previously-saved slug per dropdown, hydrated on revisit.
	IsKitChoice bool
	KitSlots    []string

	// WeaponCategory/WeaponCount/WeaponSlots are the same idea for the
	// printed weapon allowances — "1 Simple Weapon", "any two simple
	// weapons", "1 Martial Weapon". When character creation allows picking a
	// simple or martial weapon, it renders a dropdown of the matching
	// category's weapons. WeaponCategory is "" when the option is not one of
	// those.
	//
	// WeaponCount comes from the description's own number word, NOT from
	// Quantity: the row for "any two simple weapons" carries quantity 1, so
	// trusting the column would silently offer one dropdown where the book
	// grants two.
	WeaponCategory string
	WeaponCount    int
	WeaponSlots    []string
}

type equipmentChoiceGroup struct {
	GroupIdx int
	Options  []equipmentChoiceOption
}

type toolkitOption struct {
	Slug, Name string
}

// isKitChoiceOption flags an unresolved equipment-choice bullet that's
// really "pick any toolkit(s)" rather than one specific item or an
// unresolvable multi-item bundle — every real instance in the book data
// pairs the word "kit" with either "choice" or "select" ("2 Kits of your
// Choice", "One Tool Kit of your choice", "Select any one Toolkit"), while
// genuine bundles ("Padded Cloth, Poison Kit, and 1 smoke bombs") name a
// specific kit with neither word, so this stays narrow on purpose instead
// of matching "kit" alone.
func isKitChoiceOption(desc string) bool {
	d := strings.ToLower(desc)
	return strings.Contains(d, "kit") && (strings.Contains(d, "choice") || strings.Contains(d, "select"))
}

// weaponChoicePattern matches the printed weapon allowances that name a
// category rather than a specific weapon: "1 Simple Weapon", "One Simple
// weapon", "2 Simple Weapons", "any two simple weapons", "1 Martial Weapon".
// Every one of the 15 such rows in class_equipment_options fits this shape.
var weaponChoicePattern = regexp.MustCompile(`(?i)^(?:any\s+)?(\d+|one|two|three)\s+(simple|martial|exotic)\s+weapons?\.?$`)

var weaponCountWords = map[string]int{"one": 1, "two": 2, "three": 3}

// weaponChoiceOption reads a "pick N weapons of category C" line. ok is false
// for anything else, including a line naming a specific weapon (those arrive
// with item_slug already resolved and never reach here).
func weaponChoiceOption(desc string) (category string, count int, ok bool) {
	m := weaponChoicePattern.FindStringSubmatch(strings.TrimSpace(desc))
	if m == nil {
		return "", 0, false
	}
	if n, err := strconv.Atoi(m[1]); err == nil {
		count = n
	} else {
		count = weaponCountWords[strings.ToLower(m[1])]
	}
	if count < 1 {
		return "", 0, false
	}
	return strings.ToLower(m[2]), count, true
}

// loadWeaponOptions returns the weapons of one category, for a creation-step
// dropdown.
//
// The "(Two Hands)" rows are excluded: they are Mastersheet-only duplicates
// of a weapon already in the list, recording the versatile grip rather than a
// second object (migration 0021's header spells this out). Offering both
// "Spear" and "Spear (Two Hands)" as separate picks would read as two
// different weapons when the character is choosing one spear.
func (s *server) loadWeaponOptions(category string) ([]toolkitOption, error) {
	rows, err := s.rulesDB.Query(
		`SELECT slug, name FROM equipment WHERE kind = 'weapon' AND weapon_category = ?`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []toolkitOption
	for rows.Next() {
		var w toolkitOption
		if err := rows.Scan(&w.Slug, &w.Name); err != nil {
			return nil, err
		}
		if strings.HasSuffix(w.Name, "(Two Hands)") {
			continue
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}

// savedCreationWeapons groups the character's saved creation-equipment rows
// by weapon category, in insertion order, so hydrateEquipmentSelections can
// hand each weapon dropdown back the pick it was showing before.
//
// Weapons named outright by an option (a class that simply grants a Kunai)
// land in character_inventory the same way, and would be indistinguishable
// here — so those slugs are excluded by the caller's own bookkeeping: an
// option with a resolved ItemSlug never has WeaponSlots to fill.
func (s *server) savedCreationWeapons(characterID int64) (map[string][]string, error) {
	category := map[string]string{}
	rows, err := s.rulesDB.Query(
		`SELECT slug, weapon_category FROM equipment WHERE kind = 'weapon' AND weapon_category IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug, cat string
		if err := rows.Scan(&slug, &cat); err != nil {
			rows.Close()
			return nil, err
		}
		category[slug] = cat
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	saved := map[string][]string{}
	invRows, err := s.charDB.Query(`
		SELECT item_slug FROM character_inventory
		WHERE character_id = ? AND notes = 'creation-equipment' ORDER BY id`, characterID)
	if err != nil {
		return nil, err
	}
	defer invRows.Close()
	for invRows.Next() {
		var slug sql.NullString
		if err := invRows.Scan(&slug); err != nil {
			return nil, err
		}
		if slug.Valid {
			if cat, ok := category[slug.String]; ok {
				saved[cat] = append(saved[cat], slug.String)
			}
		}
	}
	return saved, invRows.Err()
}

// weaponChoiceLists builds the per-category weapon lists the equipment
// template's dropdowns render from, one query per distinct category the
// class's options actually ask for — Weapon Specialist grants four weapon
// choices across two categories, so that is two queries rather than four.
func (s *server) weaponChoiceLists(groups []equipmentChoiceGroup) (map[string][]toolkitOption, error) {
	weapons := map[string][]toolkitOption{}
	for _, g := range groups {
		for _, opt := range g.Options {
			if opt.WeaponCategory == "" || weapons[opt.WeaponCategory] != nil {
				continue
			}
			list, err := s.loadWeaponOptions(opt.WeaponCategory)
			if err != nil {
				return nil, err
			}
			weapons[opt.WeaponCategory] = list
		}
	}
	return weapons, nil
}

// weaponNamesByCategory is loadWeaponOptions keyed by slug, for validating a
// submitted dropdown value against the list it was rendered from.
func (s *server) weaponNamesByCategory(category string) (map[string]string, error) {
	options, err := s.loadWeaponOptions(category)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(options))
	for _, o := range options {
		names[o.Slug] = o.Name
	}
	return names, nil
}

// gearTierPrefixes are the words the book puts in front of an upgraded
// version of a piece of gear. A name carrying one is not starting
// equipment: the Supreme Alchemist Kit costs 1200 Ryo against the base
// kit's 200, and every one of these tiers is something a character buys or
// earns in play, never something they begin with.
var gearTierPrefixes = []string{"Greater ", "Superior ", "Supreme "}

// isStartingTierGear reports whether an item is the base version of its
// kind, i.e. the only version a character may start with.
//
// This is what stops character creation handing out thousands of Ryo of
// advanced gear for free — without this filter, character creation would let
// a player pick advanced-tier gear worth thousands of Ryo right from the
// start. Every choose-a-toolkit control in the creation flow (class
// proficiencies, background tool grants, the equipment step's "2 Kits of
// your Choice") reads its options through loadToolkitOptions, so filtering
// there covers all three.
//
// Matched on the name rather than the slug because the name is what the
// book prints and what a re-ingest is guaranteed to reproduce; slugs are
// generated and could change shape.
func isStartingTierGear(name string) bool {
	for _, prefix := range gearTierPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

// loadToolkitOptions returns the toolkits a character may start with — every
// equipment.kind='toolkit' row EXCEPT the Greater/Superior/Supreme tiers
// (see isStartingTierGear) — folding-sorted the same way every other browse
// list in this app is (see sortKey).
//
// This is creation-facing only. The sheet's own add-an-item dropdown still
// offers the full catalogue through loadEquipmentOptions: buying a Supreme
// kit at 1200 Ryo mid-campaign is exactly what it's for.
func (s *server) loadToolkitOptions() ([]toolkitOption, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name FROM equipment WHERE kind = 'toolkit'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []toolkitOption
	for rows.Next() {
		var t toolkitOption
		if err := rows.Scan(&t.Slug, &t.Name); err != nil {
			return nil, err
		}
		if !isStartingTierGear(t.Name) {
			continue
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}

// loadMartialWeaponOptions backs Scout-Nin's multiclass "One Martial
// Weapon" choice — every weapon the catalog tags weapon_category='martial',
// unlike toolkits there is no Greater/Superior/Supreme tiering to filter
// out here.
func (s *server) loadMartialWeaponOptions() ([]toolkitOption, error) {
	rows, err := s.rulesDB.Query(
		`SELECT slug, name FROM equipment WHERE kind = 'weapon' AND weapon_category = 'martial' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []toolkitOption
	for rows.Next() {
		var t toolkitOption
		if err := rows.Scan(&t.Slug, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loadClassAddGrantFields is what an "add this as an additional class"
// confirm form needs beyond the class detail card already in
// buildClassStepData: whichever extra choice pickers
// charstore.MulticlassGrantChoices says classSlug's multiclass grant
// requires, and the highest level selectable without breaking the
// total-level-20 cap (charstore.TotalClassLevels). Shared by the creation
// flow's Class step and the sheet's Add a Class page — the two forms differ
// only in chrome, not in what they ask for.
func (s *server) loadClassAddGrantFields(characterID int64, classSlug string) (map[string]any, error) {
	fields := map[string]any{}
	for _, choice := range charstore.MulticlassGrantChoices(classSlug) {
		switch choice {
		case charstore.ChoiceClassSkillPool:
			rows, err := s.rulesDB.Query(`
				SELECT value FROM class_proficiencies WHERE class_slug = ? AND kind = 'skill_choice' ORDER BY value`, classSlug)
			if err != nil {
				return nil, err
			}
			var opts []string
			for rows.Next() {
				var v string
				if err := rows.Scan(&v); err != nil {
					rows.Close()
					return nil, err
				}
				opts = append(opts, v)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
			fields["NeedsSkill"] = true
			fields["SkillOptions"] = opts
		case charstore.ChoiceToolkit:
			toolkits, err := s.loadToolkitOptions()
			if err != nil {
				return nil, err
			}
			fields["NeedsToolkit"] = true
			fields["ToolkitOptions"] = toolkits
		case charstore.ChoiceMartialWeapon:
			weapons, err := s.loadMartialWeaponOptions()
			if err != nil {
				return nil, err
			}
			fields["NeedsWeapon"] = true
			fields["WeaponOptions"] = weapons
		}
	}
	total, err := charstore.TotalClassLevels(s.charDB, characterID, "")
	if err != nil {
		return nil, err
	}
	fields["MaxLevel"] = 20 - total
	return fields, nil
}

// className looks up a class's display name, falling back to the slug
// itself on a stale/unrecognized value — same tolerance every other
// class-name lookup in this file already applies.
func (s *server) className(classSlug string) string {
	var name string
	if err := s.rulesDB.QueryRow(`SELECT name FROM classes WHERE slug = ?`, classSlug).Scan(&name); err != nil {
		return classSlug
	}
	return name
}

// characterAbilityScores returns the character's current FINAL ability
// scores (base + clan/background/ASI bonuses), keyed by 3-letter code —
// what multiclass prerequisites check against, per the book's own
// "ability score minimums" phrasing (a plain reading of the character
// sheet's actual scores, not the pre-bonus base numbers).
func (s *server) characterAbilityScores(characterID int64) (map[string]int, error) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	scores := make(map[string]int, len(sheet.Abilities))
	for ab, v := range sheet.Abilities {
		scores[ab] = v.Score
	}
	return scores, nil
}

// submitMulticlassAdd validates and applies "add classSlug as a further
// class" from an already-parsed form (fields "level", "mc_skill",
// "mc_toolkit", "mc_weapon" — only whichever of the last three
// charstore.MulticlassGrantChoices(classSlug) actually needs are read).
// Shared by the creation flow's Class step and the sheet's Add a Class
// page so the two can never validate differently. existingClasses is the
// character's classes before this add (used for the "current classes must
// still qualify too" prerequisite rule and the total-level cap).
//
// ok is false for anything the player can fix by changing their submission
// (bad level, unmet prerequisite, invalid choice) — errMsg is meant to be
// shown back on the picker. A non-nil err is a real database failure.
func (s *server) submitMulticlassAdd(r *http.Request, characterID int64, classSlug string, existingClasses []characterClassLevel) (ok bool, errMsg string, err error) {
	level, convErr := strconv.Atoi(strings.TrimSpace(r.FormValue("level")))
	if convErr != nil || level < 1 || level > 20 {
		return false, "Starting level must be a whole number from 1 to 20.", nil
	}

	scores, err := s.characterAbilityScores(characterID)
	if err != nil {
		return false, "", err
	}
	checkSlugs := make([]string, 0, len(existingClasses)+1)
	for _, c := range existingClasses {
		checkSlugs = append(checkSlugs, c.Slug)
	}
	checkSlugs = append(checkSlugs, classSlug)
	if unmet := charstore.UnmetMulticlassClasses(scores, checkSlugs); len(unmet) > 0 {
		names := make([]string, len(unmet))
		for i, slug := range unmet {
			names[i] = s.className(slug)
		}
		return false, "Ability score prerequisites not met for: " + strings.Join(names, ", ") + ".", nil
	}

	fields, err := s.loadClassAddGrantFields(characterID, classSlug)
	if err != nil {
		return false, "", err
	}
	chosenSkill := strings.TrimSpace(r.FormValue("mc_skill"))
	chosenToolkit := strings.TrimSpace(r.FormValue("mc_toolkit"))
	chosenWeapon := strings.TrimSpace(r.FormValue("mc_weapon"))
	if needsSkill, _ := fields["NeedsSkill"].(bool); needsSkill {
		valid := false
		for _, opt := range fields["SkillOptions"].([]string) {
			if opt == chosenSkill {
				valid = true
			}
		}
		if !valid {
			return false, "Pick a valid skill.", nil
		}
	}
	if needsToolkit, _ := fields["NeedsToolkit"].(bool); needsToolkit {
		valid := false
		for _, opt := range fields["ToolkitOptions"].([]toolkitOption) {
			if opt.Name == chosenToolkit {
				valid = true
			}
		}
		if !valid {
			return false, "Pick a valid toolkit.", nil
		}
	}
	if needsWeapon, _ := fields["NeedsWeapon"].(bool); needsWeapon {
		valid := false
		for _, opt := range fields["WeaponOptions"].([]toolkitOption) {
			if opt.Name == chosenWeapon {
				valid = true
			}
		}
		if !valid {
			return false, "Pick a valid weapon.", nil
		}
	}

	if err := charstore.AddMulticlass(s.charDB, characterID, classSlug, level, chosenSkill, chosenToolkit, chosenWeapon); err != nil {
		if errors.Is(err, charstore.ErrLevelCapExceeded) {
			return false, "That would push the character's total level above 20.", nil
		}
		return false, "", err
	}
	return true, "", nil
}

// hydrateEquipmentSelections marks each group's previously-saved choice as
// Selected (matched by item_slug when resolved, else by description text —
// character_inventory has no group/choice_idx column to key off directly),
// falling back to the first option in any group with no match at all (a
// fresh, never-submitted visit) — same "first option checked by default"
// behavior as before, just no longer clobbering a real saved choice.
func (s *server) hydrateEquipmentSelections(characterID int64, groups []equipmentChoiceGroup) error {
	rows, err := s.charDB.Query(`
		SELECT item_slug, custom_name FROM character_inventory
		WHERE character_id = ? AND notes = 'creation-equipment'`, characterID)
	if err != nil {
		return err
	}
	defer rows.Close()
	saved := map[string]bool{}
	for rows.Next() {
		var slug, text sql.NullString
		if err := rows.Scan(&slug, &text); err != nil {
			return err
		}
		if slug.Valid {
			saved["slug:"+slug.String] = true
		} else {
			saved["text:"+text.String] = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Kit-choice options don't save their bundle text verbatim (see
	// IsKitChoice's doc comment) — they save one character_inventory row
	// per chosen toolkit item instead. Pull those back in insertion order
	// and hand them out to each kit-choice option's slots as encountered
	// below; in every real class today there's at most one such option, so
	// this ordering concern is mostly theoretical, but it's cheap to get
	// right for a hypothetical class with two.
	// character_inventory (charDB) has no FK to equipment (rulesDB) — separate
	// SQLite files, so this can't be one JOINed query. Load the toolkit slug
	// set from rulesDB and filter in Go instead.
	// Every toolkit, not just the starting-tier ones loadToolkitOptions
	// offers: this is recognising what was already saved, and a draft
	// created before the starting-tier restriction (or edited by hand)
	// could still hold an upgraded kit. Failing to recognise it would
	// silently drop it out of its slot on revisit.
	toolkitRows, err := s.rulesDB.Query(`SELECT slug FROM equipment WHERE kind = 'toolkit'`)
	if err != nil {
		return err
	}
	isToolkit := map[string]bool{}
	for toolkitRows.Next() {
		var slug string
		if err := toolkitRows.Scan(&slug); err != nil {
			toolkitRows.Close()
			return err
		}
		isToolkit[slug] = true
	}
	toolkitRows.Close()
	if err := toolkitRows.Err(); err != nil {
		return err
	}
	kitRows, err := s.charDB.Query(`
		SELECT item_slug FROM character_inventory
		WHERE character_id = ? AND notes = 'creation-equipment' ORDER BY id`, characterID)
	if err != nil {
		return err
	}
	defer kitRows.Close()
	var savedKits []string
	for kitRows.Next() {
		var slug sql.NullString
		if err := kitRows.Scan(&slug); err != nil {
			return err
		}
		if slug.Valid && isToolkit[slug.String] {
			savedKits = append(savedKits, slug.String)
		}
	}
	if err := kitRows.Err(); err != nil {
		return err
	}

	for gi := range groups {
		for oi := range groups[gi].Options {
			opt := &groups[gi].Options[oi]
			if !opt.IsKitChoice || opt.Quantity <= 0 {
				continue
			}
			take := opt.Quantity
			if take > len(savedKits) {
				take = len(savedKits)
			}
			opt.KitSlots = make([]string, opt.Quantity)
			copy(opt.KitSlots, savedKits[:take])
			savedKits = savedKits[take:]
		}
	}

	// The weapon dropdowns save the same way and are recognised the same
	// way: the saved rows are real weapon items, so a saved pick is any
	// creation-equipment row whose slug is a weapon of the right category.
	// Matched per category rather than in one flat queue like the kits
	// above, because a class can hand out both a simple and a martial
	// choice (Hunter-Nin, Scout-Nin and Weapon Specialist all do) and a
	// martial pick must not fall into a simple slot.
	savedWeapons, err := s.savedCreationWeapons(characterID)
	if err != nil {
		return err
	}
	for gi := range groups {
		for oi := range groups[gi].Options {
			opt := &groups[gi].Options[oi]
			if opt.WeaponCategory == "" || opt.WeaponCount <= 0 {
				continue
			}
			queue := savedWeapons[opt.WeaponCategory]
			take := opt.WeaponCount
			if take > len(queue) {
				take = len(queue)
			}
			opt.WeaponSlots = make([]string, opt.WeaponCount)
			copy(opt.WeaponSlots, queue[:take])
			savedWeapons[opt.WeaponCategory] = queue[take:]
		}
	}

	for gi := range groups {
		anySelected := false
		for oi := range groups[gi].Options {
			opt := &groups[gi].Options[oi]
			key := "text:" + opt.Description
			if opt.ItemSlug != "" {
				key = "slug:" + opt.ItemSlug
			}
			if saved[key] {
				opt.Selected = true
				anySelected = true
			}
		}
		if !anySelected && len(groups[gi].Options) > 0 {
			groups[gi].Options[0].Selected = true
		}
	}
	return nil
}

func (s *server) handleCreateEquipment(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var classSlug sql.NullString
	if err := s.charDB.QueryRow(`
		SELECT class_slug FROM character_classes WHERE character_id = ? ORDER BY order_index LIMIT 1`, id,
	).Scan(&classSlug); err != nil && err != sql.ErrNoRows {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query character class for equipment:", err)
		return
	}
	var backgroundSlug sql.NullString
	if err := s.charDB.QueryRow(`SELECT background_slug FROM characters WHERE id = ?`, id).Scan(&backgroundSlug); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query character background for equipment:", err)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		groups, err := s.loadEquipmentChoiceGroups(classSlug.String)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load equipment groups:", err)
			return
		}
		toolkits, err := s.loadToolkitOptions()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load toolkit options:", err)
			return
		}
		toolkitNames := make(map[string]string, len(toolkits))
		for _, t := range toolkits {
			toolkitNames[t.Slug] = t.Name
		}

		var lines []charstore.EquipmentLine
		var kitErr string
		for _, g := range groups {
			chosen, err := strconv.Atoi(r.FormValue("group_" + strconv.Itoa(g.GroupIdx)))
			if err != nil {
				continue
			}
			for _, opt := range g.Options {
				if opt.ChoiceIdx != chosen {
					continue
				}
				if opt.IsKitChoice {
					for slot := 0; slot < opt.Quantity; slot++ {
						slug := r.FormValue("kit_" + strconv.Itoa(g.GroupIdx) + "_" + strconv.Itoa(opt.ChoiceIdx) + "_" + strconv.Itoa(slot))
						name, ok := toolkitNames[slug]
						if !ok {
							kitErr = "Pick a toolkit for every slot under \"" + opt.Description + "\"."
							continue
						}
						lines = append(lines, charstore.EquipmentLine{Slug: slug, Text: name, Quantity: 1})
					}
				} else if opt.WeaponCategory != "" {
					// Same slot-per-pick shape as the kit choices: one real
					// weapon per dropdown, so the inventory gets equippable
					// items with damage dice instead of the sentence "1
					// Simple Weapon".
					names, err := s.weaponNamesByCategory(opt.WeaponCategory)
					if err != nil {
						http.Error(w, "database error", http.StatusInternalServerError)
						log.Println("load weapon options:", err)
						return
					}
					for slot := 0; slot < opt.WeaponCount; slot++ {
						slug := r.FormValue("weapon_" + strconv.Itoa(g.GroupIdx) + "_" + strconv.Itoa(opt.ChoiceIdx) + "_" + strconv.Itoa(slot))
						name, ok := names[slug]
						if !ok {
							kitErr = "Pick a weapon for every slot under \"" + opt.Description + "\"."
							continue
						}
						lines = append(lines, charstore.EquipmentLine{Slug: slug, Text: name, Quantity: 1})
					}
				} else if opt.ItemSlug == "" {
					// An option the ingest could not resolve to one item is
					// usually several items in one printed line ("Cooking
					// Tools, Flash Tag, Paper Bomb"). Split it and look each
					// part up, so the inventory gets real equippable rows
					// instead of the sentence. Parts that name nothing in
					// the rules stay as their own free-text line — the same
					// bargain parseStartingEquipment already makes for the
					// backgrounds.
					parsed, err := s.parseCompoundEquipment(opt.Description)
					if err != nil {
						http.Error(w, "database error", http.StatusInternalServerError)
						log.Println("parse class equipment option:", err)
						return
					}
					if len(parsed) == 0 {
						parsed = []startingEquipmentLine{{Text: opt.Description, Quantity: opt.Quantity}}
					}
					for _, line := range parsed {
						lines = append(lines, charstore.EquipmentLine{
							Slug: line.Slug, Text: line.Text, Quantity: line.Quantity,
						})
					}
				} else {
					lines = append(lines, charstore.EquipmentLine{Slug: opt.ItemSlug, Text: opt.Description, Quantity: opt.Quantity})
				}
				break
			}
		}
		if kitErr != "" {
			if err := s.hydrateEquipmentSelections(id, groups); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("hydrate equipment selections:", err)
				return
			}
			weapons, err := s.weaponChoiceLists(groups)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load weapon options:", err)
				return
			}
			data := map[string]any{
				"Title": "Starting Equipment", "ID": id, "Groups": groups,
				"Toolkits": toolkits, "Weapons": weapons, "Error": kitErr,
			}
			if backgroundSlug.Valid {
				s.addBackgroundEquipmentData(data, id, backgroundSlug.String)
			}
			s.render(w, "create_equipment.html", data)
			return
		}
		startingRyo := 0.0
		if backgroundSlug.Valid {
			var eqText, packText sql.NullString
			if err := s.rulesDB.QueryRow(`
				SELECT equipment_text, equipment_pack_text FROM backgrounds WHERE slug = ?`, backgroundSlug.String,
			).Scan(&eqText, &packText); err == nil {
				if eqText.Valid && eqText.String != "" {
					// One inventory row per possession, not one row for the
					// whole printed sentence — see starting_equipment.go.
					// Anything the sentence names that the rules have a row
					// for arrives as a real, linkable, equippable item; the
					// rest stays as its own short free-text line.
					parsed, err := s.parseStartingEquipment(eqText.String)
					if err != nil {
						http.Error(w, "database error", http.StatusInternalServerError)
						log.Println("parse background starting equipment:", err)
						return
					}
					for _, line := range parsed.Lines {
						lines = append(lines, charstore.EquipmentLine{
							Slug: line.Slug, Text: line.Text, Quantity: line.Quantity,
						})
					}
					startingRyo = parsed.Ryo
				}
				if packText.Valid && packText.String != "" {
					// A resolved pack choice is saved as the real item, so
					// it lands in the inventory as something with stats and
					// a detail page rather than as a sentence. The prose
					// line is still the fallback when the player hasn't
					// picked, or when the line names nothing recognisable.
					saved := false
					if choices, err := s.packChoices(packText.String); err == nil {
						picked := strings.TrimSpace(r.FormValue("background_pack"))
						for _, choice := range choices {
							if choice.Slug == picked {
								lines = append(lines, charstore.EquipmentLine{
									Slug: choice.Slug, Text: choice.Name, Quantity: 1,
								})
								saved = true
								break
							}
						}
					}
					if !saved {
						lines = append(lines, charstore.EquipmentLine{Text: packText.String, Quantity: 1})
					}
				}
			}
		}
		if err := charstore.SetEquipment(s.charDB, id, lines); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set equipment:", err)
			return
		}
		// The background's printed purse ("a wallet containing 100 Ryo")
		// now reaches the sheet's Ryo box instead of being narrated in an
		// inventory line while the purse showed 0. Idempotent on resubmit —
		// see charstore.SetStartingRyo.
		if err := charstore.SetStartingRyo(s.charDB, id, startingRyo); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set starting ryo:", err)
			return
		}
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
		return
	}

	data := map[string]any{"Title": "Starting Equipment", "ID": id}
	if classSlug.Valid {
		groups, err := s.loadEquipmentChoiceGroups(classSlug.String)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load equipment groups:", err)
			return
		}
		if err := s.hydrateEquipmentSelections(id, groups); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("hydrate equipment selections:", err)
			return
		}
		data["Groups"] = groups
		toolkits, err := s.loadToolkitOptions()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load toolkit options:", err)
			return
		}
		data["Toolkits"] = toolkits
		weapons, err := s.weaponChoiceLists(groups)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load weapon options:", err)
			return
		}
		data["Weapons"] = weapons
	}
	if backgroundSlug.Valid {
		s.addBackgroundEquipmentData(data, id, backgroundSlug.String)
	}
	s.render(w, "create_equipment.html", data)
}

// packChoiceOption is one selectable adventuring pack in a background's
// printed "X or Y Pack (Choose one)." line.
type packChoiceOption struct {
	Slug, Name string
}

// packChoices resolves a background's equipment_pack_text into the real
// gear items it names, so the player gets a dropdown instead of a sentence
// they have to interpret and then track by hand.
//
// Every one of the ten distinct pack lines in the book data has the same
// shape — "Infiltrator's or Crafter's Pack (Choose one)." — and every pack
// named in them exists in the equipment table as gear/<name>-pack. Rather
// than pattern-match the sentence, this walks the actual pack items and
// asks which of them the sentence mentions, so a rules update that adds a
// pack or rewords a line needs nothing here. Apostrophes are stripped
// before comparing because the book text uses a typographic apostrophe
// and the item names use a plain one.
//
// Returns nil when nothing matches, which is the signal to fall back to
// showing the raw text (a line this can't resolve is still information the
// player needs).
func (s *server) packChoices(packText string) ([]packChoiceOption, error) {
	if strings.TrimSpace(packText) == "" {
		return nil, nil
	}
	normalize := func(v string) string {
		v = strings.ReplaceAll(v, "\u2019", "")
		v = strings.ReplaceAll(v, "'", "")
		return strings.ToLower(v)
	}
	haystack := normalize(packText)

	rows, err := s.rulesDB.Query(
		`SELECT slug, name FROM equipment WHERE kind = 'gear' AND name LIKE '%Pack' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var found []packChoiceOption
	for rows.Next() {
		var opt packChoiceOption
		if err := rows.Scan(&opt.Slug, &opt.Name); err != nil {
			return nil, err
		}
		// Match on the owner word alone ("Infiltrator"), since the text
		// writes the word "Pack" only once for both choices.
		owner := normalize(strings.TrimSuffix(opt.Name, " Pack"))
		if owner == "" || !strings.Contains(haystack, owner) {
			continue
		}
		found = append(found, opt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Ordered as the sentence names them, so the dropdown reads the way
	// the book line does.
	sort.Slice(found, func(i, k int) bool {
		return strings.Index(haystack, normalize(strings.TrimSuffix(found[i].Name, " Pack"))) <
			strings.Index(haystack, normalize(strings.TrimSuffix(found[k].Name, " Pack")))
	})
	return found, nil
}

// addBackgroundEquipmentData fills in the background half of the equipment
// step: the flat granted-items line, the pack line, and — when the pack
// line resolves to real items — the choices for its dropdown plus whichever
// one is already saved. Shared by the three places that render this page.
func (s *server) addBackgroundEquipmentData(data map[string]any, characterID int64, backgroundSlug string) {
	var eqText, packText sql.NullString
	if err := s.rulesDB.QueryRow(`
		SELECT equipment_text, equipment_pack_text FROM backgrounds WHERE slug = ?`, backgroundSlug,
	).Scan(&eqText, &packText); err != nil {
		return
	}
	data["BackgroundEquipmentText"] = eqText.String
	data["BackgroundPackText"] = packText.String

	choices, err := s.packChoices(packText.String)
	if err != nil {
		log.Println("resolve background pack choices:", err)
		return
	}
	if len(choices) == 0 {
		return
	}
	data["BackgroundPackChoices"] = choices

	// Which one is already chosen, so revisiting the step doesn't silently
	// reset it — the same "every creation choice must survive navigating
	// away" rule the rest of this flow follows.
	for _, choice := range choices {
		var n int
		if err := s.charDB.QueryRow(`
			SELECT COUNT(*) FROM character_inventory
			WHERE character_id = ? AND item_slug = ?`, characterID, choice.Slug,
		).Scan(&n); err == nil && n > 0 {
			data["BackgroundPackChosen"] = choice.Slug
			break
		}
	}
}

func (s *server) loadEquipmentChoiceGroups(classSlug string) ([]equipmentChoiceGroup, error) {
	if classSlug == "" {
		return nil, nil
	}
	rows, err := s.rulesDB.Query(`
		SELECT group_idx, choice_idx, description, item_slug, quantity
		FROM class_equipment_options WHERE class_slug = ? ORDER BY group_idx, choice_idx`, classSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []equipmentChoiceGroup
	for rows.Next() {
		var groupIdx, choiceIdx, quantity int
		var description string
		var itemSlug sql.NullString
		if err := rows.Scan(&groupIdx, &choiceIdx, &description, &itemSlug, &quantity); err != nil {
			return nil, err
		}
		if n := len(groups); n == 0 || groups[n-1].GroupIdx != groupIdx {
			groups = append(groups, equipmentChoiceGroup{GroupIdx: groupIdx})
		}
		g := &groups[len(groups)-1]
		opt := equipmentChoiceOption{
			ChoiceIdx: choiceIdx, Description: description, ItemSlug: itemSlug.String, Quantity: quantity,
			IsKitChoice: itemSlug.String == "" && isKitChoiceOption(description),
		}
		if itemSlug.String == "" && !opt.IsKitChoice {
			if category, count, ok := weaponChoiceOption(description); ok {
				opt.WeaponCategory, opt.WeaponCount = category, count
			}
		}
		g.Options = append(g.Options, opt)
	}
	return groups, rows.Err()
}

// ---- Jutsu step -------------------------------------------------------

// jutsuChoiceOption is one selectable jutsu on the creation step.
//
// It embeds the library page's jutsuListEntry rather than carrying its own
// four fields, because the step now renders the same filter UI as /jutsu
// (Categories, Rank, Casting Action, Duration, Components, Range) and every
// one of those filters reads a data- attribute derived from that struct. A
// second, smaller struct would mean a second copy of jutsu_filters.go's
// bucketing rules that could drift from the library's.
type jutsuChoiceOption struct {
	jutsuListEntry
	Selected bool
	// Source is "class" (castable through the class's disciplines) or
	// "clan" (from the character's own clan's list). Rendered as a badge
	// and counted separately, and it is also what the clan-jutsu counter
	// reads.
	Source string
}

func (s *server) handleCreateJutsu(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	classes, err := s.loadCharacterClassLevels(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query character classes for jutsu:", err)
		return
	}
	// Total character level (sum across every class), for display and for
	// stamping "learned at level" — the per-class levels in `classes`
	// itself drive the known-cap/eligible-jutsu math below.
	level := 1
	if len(classes) > 0 {
		level = 0
		for _, c := range classes {
			level += c.Levels
		}
	}

	// The character's clan decides which clan jutsu list (if any) joins the
	// class list below. A clanless draft simply gets no clan half.
	var clanSlug sql.NullString
	if err := s.charDB.QueryRow(`SELECT clan_slug FROM characters WHERE id = ?`, id).Scan(&clanSlug); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query character clan for jutsu:", err)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		slugs := r.Form["jutsu"]
		jutsuKnown, _, err := s.computeJutsuKnown(classes)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load jutsu known:", err)
			return
		}
		if jutsuKnown >= 0 && len(slugs) > jutsuKnown {
			options, err := s.loadEligibleJutsuAllClasses(classes, clanSlug.String, slugs)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load eligible jutsu:", err)
				return
			}
			s.render(w, "create_jutsu.html", map[string]any{
				"Title": "Learn Jutsu", "ID": id, "Jutsu": options,
				"JutsuKnown": jutsuKnown, "Level": level,
				"SourceTiles": s.jutsuSourceTilesOrNil(),
				"Error": "You can only know " + strconv.Itoa(jutsuKnown) + " jutsu at level " + strconv.Itoa(level) +
					" — clan jutsu are picked instead of normal ones, not on top of them.",
			})
			return
		}
		if err := charstore.SetJutsu(s.charDB, id, level, slugs); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set jutsu:", err)
			return
		}
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
		return
	}

	if len(classes) == 0 {
		s.render(w, "create_jutsu.html", map[string]any{"Title": "Learn Jutsu", "ID": id})
		return
	}

	var selected []string
	selRows, err := s.charDB.Query(`
		SELECT jutsu_slug FROM character_jutsu WHERE character_id = ? AND source = 'learned'`, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query selected jutsu:", err)
		return
	}
	for selRows.Next() {
		var slug string
		if err := selRows.Scan(&slug); err != nil {
			selRows.Close()
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan selected jutsu:", err)
			return
		}
		selected = append(selected, slug)
	}
	selRows.Close()

	jutsuKnown, _, err := s.computeJutsuKnown(classes)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu known:", err)
		return
	}
	options, err := s.loadEligibleJutsuAllClasses(classes, clanSlug.String, selected)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load eligible jutsu:", err)
		return
	}

	data := map[string]any{
		"Title": "Learn Jutsu", "ID": id, "Jutsu": options, "JutsuKnown": jutsuKnown, "Level": level,
		"SourceTiles": s.jutsuSourceTilesOrNil(),
	}
	// Server-render the first row's detail card so the pane isn't empty
	// before any click — same progressive-enhancement precedent
	// handleJutsuList follows for the standalone /jutsu page.
	if len(options) > 0 {
		if detail, err := loadJutsuDetail(s.rulesDB, options[0].Slug); err == nil {
			data["FocusDetail"] = detail
		}
	}

	s.render(w, "create_jutsu.html", data)
}

// jutsuSourceTilesOrNil supplies the creation step's sourcebook filter tiles.
// A failure here costs one filter control on a step that still works
// without it, so it degrades to no tiles rather than failing the page —
// unlike the library page, where the tiles are part of the point.
func (s *server) jutsuSourceTilesOrNil() []jutsuSourceTile {
	tiles, err := loadJutsuSourceTiles(s.rulesDB)
	if err != nil {
		log.Println("load jutsu source tiles for creation step:", err)
		return nil
	}
	return tiles
}

// loadJutsuKnown returns v_class_levels.jutsu_known for the given class and
// level, or -1 if unknown (either NULL in the data, meaning "no cap
// recorded" here, or no matching row at all — both left unlimited rather
// than silently blocking selection).
func (s *server) loadJutsuKnown(classSlug string, level int) (int, error) {
	var jutsuKnown sql.NullInt64
	err := s.rulesDB.QueryRow(`
		SELECT jutsu_known FROM v_class_levels WHERE class_slug = ? AND level = ?`, classSlug, level,
	).Scan(&jutsuKnown)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	if err != nil {
		return 0, err
	}
	if !jutsuKnown.Valid {
		return -1, nil
	}
	return int(jutsuKnown.Int64), nil
}

// classJutsuPredicate is the SQL test for "this class can learn this jutsu",
// shared by the creation step and the sheet's jutsu library so the two lists
// can never disagree about what a class may take. Expects the class slug as
// its one bound parameter.
//
// class_casting is the book's own per-class discipline table, and it lists
// Ninjutsu, Genjutsu and Taijutsu for all eleven classes — and Bukijutsu for
// none of them, not even the Weapon Specialist. Gating purely on it therefore
// hid all 384 Bukijutsu jutsu from every character in the game. Bukijutsu is
// admitted unconditionally instead: it is the weapon
// discipline, every class already has a Bukijutsu to-hit modifier (see
// charsheet.AttackKinds), and there is no class the book bars from it.
//
// LIKE rather than equality catches the one jutsu classified "Hijutsu,
// Bukijutsu"; no other classification contains the substring, so it cannot
// over-match. Hijutsu on its own stays out deliberately — those are secret
// techniques, and the ones a character can actually have reach them through
// the clan_jutsu half of the union rather than through their class.
const classJutsuPredicate = `(
		LOWER(classification) IN (SELECT discipline FROM class_casting WHERE class_slug = ?)
		OR LOWER(classification) LIKE '%bukijutsu%'
	)`

// loadEligibleJutsu lists jutsu the character can learn at this level, from
// two sources:
//
//   - the class list: any discipline the class casts (class_casting), plus
//     Bukijutsu — see classJutsuPredicate;
//   - the character's own clan's list (clan_jutsu).
//
// Clan jutsu were missing entirely before, so a Hyuga could not pick a
// single Hyuga technique during creation. The book's rule for them (stated
// per clan, e.g. Bakuton's Explosive Techniques) is that a clan's jutsu are
// added "instead of selecting jutsu from the Normal jutsu list(s)" — one
// allowance covering both lists, not a bonus pool — so they are merged into
// the same list here and tagged with their source rather than counted apart.
//
// Both sources are capped at the highest rank the class knows at this level
// (v_class_levels.highest_rank_known) when that's recorded, and left
// unfiltered by rank when it's NULL: same "don't silently block on missing
// data" call as loadJutsuKnown. Not gated by nature release — confirmed
// during planning that no such structured gate exists anywhere in the
// schema.
//
// A per-clan restriction this deliberately does NOT model: a few clans
// (Bakuton's Branch Style, for one) further gate their own list behind a
// keyword chosen at 1st level. That gate lives in clan feature prose, not
// in any column, so enforcing it would mean parsing free text — the step
// offers the clan's full list and leaves that call to the player and GM.
func (s *server) loadEligibleJutsu(classSlug string, level int, clanSlug string, selected []string) ([]jutsuChoiceOption, error) {
	selectedSet := map[string]bool{}
	for _, s := range selected {
		selectedSet[s] = true
	}

	var highestRank sql.NullString
	if err := s.rulesDB.QueryRow(`
		SELECT highest_rank_known FROM v_class_levels WHERE class_slug = ? AND level = ?`, classSlug, level,
	).Scan(&highestRank); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// The class and clan halves are unioned in SQL rather than queried
	// twice and merged in Go, so a jutsu that is BOTH (a clan technique
	// whose classification the class also casts) appears exactly once. The
	// clan tag wins for those, via MAX over a 0/1 flag rather than over the
	// source strings themselves — "clan" < "class" lexically, so a MAX on
	// the strings would silently pick the wrong one.
	query := `
		SELECT v.slug, v.name, v.rank, v.classification, v.category_group,
		       v.casting_time, v.range, v.duration, v.components, v.description,
		       COALESCE(j.source_book, ''), MAX(src.is_clan) AS is_clan
		FROM (
			SELECT slug, 0 AS is_clan FROM v_jutsu
			WHERE ` + classJutsuPredicate + `
			UNION ALL
			SELECT jutsu_slug, 1 FROM clan_jutsu WHERE clan_slug = ?
		) src
		JOIN v_jutsu v ON v.slug = src.slug
		JOIN jutsu j ON j.slug = v.slug
		JOIN jutsu_ranks jr ON jr.rank = v.rank`
	args := []any{classSlug, clanSlug}
	if highestRank.Valid {
		query += ` WHERE jr.sort_order <= (SELECT sort_order FROM jutsu_ranks WHERE rank = ?)`
		args = append(args, highestRank.String)
	}
	query += ` GROUP BY v.slug`

	rows, err := s.rulesDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []jutsuChoiceOption
	for rows.Next() {
		var j jutsuChoiceOption
		var components string
		var isClan bool
		if err := rows.Scan(&j.Slug, &j.Name, &j.Rank, &j.Classification, &j.CategoryGroup,
			&j.CastingTime, &j.Range, &j.Duration, &components, &j.Description,
			&j.SourceBook, &isClan); err != nil {
			return nil, err
		}
		j.Source = "class"
		if isClan {
			j.Source = "clan"
		}
		// Same derivation the library list does, so the filter panel's
		// buckets mean exactly the same thing on both pages.
		j.CastingAction = castingActionBucket(j.CastingTime)
		j.DurationLabel = durationBucket(j.Duration)
		j.DurationOrder = durationOrder[j.DurationLabel]
		rng := parseJutsuRange(j.Range)
		j.RangeFeet, j.RangeNumeric, j.RangeSpecial = rng.Feet, rng.Numeric, rng.Special
		j.Components = componentCodes(components)

		j.Selected = selectedSet[j.Slug]
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}

// loadEligibleJutsuAllClasses unions loadEligibleJutsu across every class a
// (possibly multiclassed) character has — each class still gated by its
// OWN highest-rank-known at its OWN level, so a jutsu that only one of the
// character's classes has grown into isn't hidden by a lower-level second
// class, and vice versa. A jutsu eligible through more than one class (or
// through the clan list, which loadEligibleJutsu re-queries identically
// every time) is deduplicated by slug, first occurrence wins.
func (s *server) loadEligibleJutsuAllClasses(classes []characterClassLevel, clanSlug string, selected []string) ([]jutsuChoiceOption, error) {
	seen := map[string]bool{}
	var out []jutsuChoiceOption
	for _, c := range classes {
		opts, err := s.loadEligibleJutsu(c.Slug, c.Levels, clanSlug, selected)
		if err != nil {
			return nil, err
		}
		for _, o := range opts {
			if seen[o.Slug] {
				continue
			}
			seen[o.Slug] = true
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}

// characterClassLevel is one character_classes row (class slug + level in
// that class), in creation order — the shared shape every multiclass-aware
// jutsu/level computation loops over.
type characterClassLevel struct {
	Slug   string
	Levels int
}

// loadCharacterClassLevels returns every class the character has, in the
// order they were taken (classes[0] is always the primary class).
func (s *server) loadCharacterClassLevels(characterID int64) ([]characterClassLevel, error) {
	rows, err := s.charDB.Query(`
		SELECT class_slug, levels FROM character_classes WHERE character_id = ? ORDER BY order_index`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []characterClassLevel
	for rows.Next() {
		var c characterClassLevel
		if err := rows.Scan(&c.Slug, &c.Levels); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// computeJutsuKnown aggregates known-jutsu count and highest rank known
// across every class a character has, per the core book's Multiclassing
// rules (page 179): the count is the first class's own single-class
// jutsu_known at its level, plus floor(level/rate) for every additional
// class using that class's own Multiclassing Jutsu Known rate
// (charstore.MulticlassJutsuRate) — the book's own worked example pools two
// classes' dice "as described for levels after 1st", and this is the jutsu
// equivalent for a stat with no book-native pooling rule of its own.
// Highest rank known is whichever single class's own table gives the best
// rank at THAT class's own level — equivalent to using the character's
// single highest class level, since the underlying table is level-only and
// monotonic (confirmed identical shape across all 11 classes).
//
// known is -1 (matching loadJutsuKnown's own "no cap recorded" sentinel) if
// the first class has no recorded value — an unknown base means the whole
// aggregate is unknown rather than silently starting from 0.
func (s *server) computeJutsuKnown(classes []characterClassLevel) (known int, highestRank string, err error) {
	if len(classes) == 0 {
		return -1, "", nil
	}
	known, err = s.loadJutsuKnown(classes[0].Slug, classes[0].Levels)
	if err != nil {
		return 0, "", err
	}
	if known >= 0 {
		for _, c := range classes[1:] {
			if rate := charstore.MulticlassJutsuRate(c.Slug); rate > 0 {
				known += c.Levels / rate
			}
		}
	}
	for _, c := range classes {
		var rank sql.NullString
		if err := s.rulesDB.QueryRow(
			`SELECT highest_rank_known FROM v_class_levels WHERE class_slug = ? AND level = ?`, c.Slug, c.Levels,
		).Scan(&rank); err != nil && err != sql.ErrNoRows {
			return 0, "", err
		}
		if rank.Valid && jutsuRankOrder[rank.String] >= jutsuRankOrder[highestRank] {
			highestRank = rank.String
		}
	}
	return known, highestRank, nil
}

// jutsuKnownCapForCharacter is the sheet-facing known-jutsu cap: the
// classes[]-aggregated computeJutsuKnown, plus any Water and Oil/Heat
// Master bonus slots the primary class/clan combo grants (that bonus is a
// narrow clan+class feature check, not a core multiclass rule, so it
// deliberately stays scoped to the primary class only — the two never
// stack in practice, since a character can only take one Cooking Focus),
// plus Science-Nin's own Chakra Cell Enhancement bonus (Intelligence
// modifier, gated purely on the granted feature so it applies regardless
// of which class slot Science-Nin occupies in a multiclass), plus the flat
// +1 several clans' own clan_traits rows grant outright
// (clanBonusDRankJutsuSlots), plus Sarutobi's own escalating Advanced
// Nature Proficiency clan feature
// (sarutobiAdvancedNatureProficiencyBonusJutsuSlots), plus White Technique
// Weaver's own escalating Chakra String Augments subclass feature
// (whiteTechniqueChakraStringBonusJutsuSlots, scoped to the character's own
// Puppet Master class level), plus the flat +1 each of the four Archivist
// feats grants outright (featArchivistBonusJutsuSlots, summed since a
// character can hold more than one), plus the five Ninjutsu Focus
// "[Element] Release" subclass features' own conditional +2 known-jutsu
// bonus (ninjutsuFocusReleaseBonusJutsuSlots, same primary-class-only
// scoping as Water and Oil/Heat Master above, since it shares that same
// "already have access from elsewhere" gate), plus the four Ninjutsu Focus
// "[Discipline] Specialization" subclass features' own unconditional flat
// bonus (ninjutsuSpecializationBonusJutsuSlots). Shared by
// handleCharacterSheet and the sheet_jutsu_known/sheet_attack_jutsu_table
// fragment refresh, which used to each run their own copy of this off the
// primary class's level alone — wrong the moment a character has a second
// class, since sheet.Level is the character's TOTAL level, not that
// class's own.
func (s *server) jutsuKnownCapForCharacter(characterID int64, sheet *charsheet.Sheet, grantedFeatures []grantedFeatureRow) (int, error) {
	classes, err := s.loadCharacterClassLevels(characterID)
	if err != nil {
		return -1, err
	}
	if len(classes) == 0 {
		return -1, nil
	}
	known, _, err := s.computeJutsuKnown(classes)
	if err != nil {
		return -1, err
	}
	if known < 0 {
		return -1, nil
	}
	waterAndOilBonus, err := s.waterAndOilBonusJutsuSlots(characterID, grantedFeatures, classes[0].Slug, sheet.ClanSlug, sheet.ProficiencyBonus)
	if err != nil {
		return -1, err
	}
	heatMasterBonus, err := s.heatMasterBonusJutsuSlots(characterID, grantedFeatures, classes[0].Slug, sheet.ClanSlug, sheet.ProficiencyBonus)
	if err != nil {
		return -1, err
	}
	chakraCellBonus := chakraCellEnhancementBonusJutsuSlots(grantedFeatures, sheet.Abilities["int"].Modifier)
	clanBonus := clanBonusDRankJutsuSlots(sheet.ClanSlug)
	sarutobiBonus := sarutobiAdvancedNatureProficiencyBonusJutsuSlots(grantedFeatures, sheet.Level)
	puppetMasterLevel, err := s.puppetMasterClassLevel(characterID)
	if err != nil {
		return -1, err
	}
	whiteTechniqueBonus := whiteTechniqueChakraStringBonusJutsuSlots(grantedFeatures, puppetMasterLevel)
	archivistBonus := featArchivistBonusJutsuSlots(grantedFeatures)
	ninjutsuFocusReleaseBonus, err := s.ninjutsuFocusReleaseBonusJutsuSlots(characterID, grantedFeatures, classes[0].Slug, sheet.ClanSlug)
	if err != nil {
		return -1, err
	}
	ninjutsuSpecializationBonus := ninjutsuSpecializationBonusJutsuSlots(grantedFeatures)
	return known + waterAndOilBonus + heatMasterBonus + chakraCellBonus + clanBonus + sarutobiBonus + whiteTechniqueBonus + archivistBonus + ninjutsuFocusReleaseBonus + ninjutsuSpecializationBonus, nil
}

// ---- Ambitions step ---------------------------------------------------

func (s *server) handleCreateAmbitions(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		drive := strings.TrimSpace(r.FormValue("drive"))
		goal := strings.TrimSpace(r.FormValue("goal"))
		fear := strings.TrimSpace(r.FormValue("fear"))
		if err := charstore.SetAmbitions(s.charDB, id, drive, goal, fear); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set ambitions:", err)
			return
		}
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/create", http.StatusSeeOther)
		return
	}

	var backgroundSlug sql.NullString
	if err := s.charDB.QueryRow(`SELECT background_slug FROM characters WHERE id = ?`, id).Scan(&backgroundSlug); err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query character background for ambitions:", err)
		return
	}

	values := map[string]string{"drive": "", "goal": "", "fear": ""}
	savedRows, err := s.charDB.Query(`SELECT kind, text FROM character_ambitions WHERE character_id = ?`, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query saved ambitions:", err)
		return
	}
	for savedRows.Next() {
		var kind, text string
		if err := savedRows.Scan(&kind, &text); err != nil {
			savedRows.Close()
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan saved ambition:", err)
			return
		}
		values[kind] = text
	}
	savedRows.Close()

	if backgroundSlug.Valid {
		promptRows, err := s.rulesDB.Query(`
			SELECT kind, text FROM ambition_prompts WHERE background_slug = ?`, backgroundSlug.String)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("query ambition prompts:", err)
			return
		}
		for promptRows.Next() {
			var kind, text string
			if err := promptRows.Scan(&kind, &text); err != nil {
				promptRows.Close()
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("scan ambition prompt:", err)
				return
			}
			if values[kind] == "" {
				values[kind] = text
			}
		}
		promptRows.Close()
	}

	s.render(w, "create_ambitions.html", map[string]any{
		"Title": "Ambitions", "ID": id, "Drive": values["drive"], "Goal": values["goal"], "Fear": values["fear"],
	})
}

// ---- Character sheet (read-only, post-creation) ------------------------

type inventoryRow struct {
	ID   int64
	Name string
	// Slug always points somewhere now (part 11): a catalogue row into
	// rules.db, a custom row into the local item library (custom_items,
	// characters.db) — see lookupCarriedItem and DetailHref.
	Slug     string
	Kind     string // rules kind ("weapon", "armor", ...); player-typed Type for a custom row
	Quantity int
	Equipped bool

	// Bulk is this row's per-unit bulk, invalid when the rules have no
	// bulk for the item (weapon/net, the six Lock rows) or the row is
	// free text with no slug at all. BulkTotal is Bulk*Quantity, and is
	// 0 for both of those cases — an unknown bulk is counted as nothing
	// rather than guessed at, which the summary says out loud rather
	// than quietly folding into the total.
	Bulk      sql.NullFloat64
	BulkTotal float64
	// StorageBonus is non-zero only for the four Shinobi storage tools,
	// whose equipment.bulk column holds the capacity they GRANT rather
	// than the space they take (see bulkStorageBonus).
	StorageBonus float64
	// Unpackable marks a container — an item whose description opens
	// "Contents:" — so the inventory can offer an Unpack button beside it
	// (see handleSheetInventoryUnpack).
	Unpackable bool
}

// DetailHref is the rules page this row's name links to, or "" for a
// free-text row with nothing to link to.
//
// Three tables mean three URL spaces: linking everything at /items/{slug}
// was right while the bag only held equipment, but the Inventory tab now
// takes poisons and traps too, and those 404 there. The slug's own prefix
// is what routes it, exactly as carriedItemKinds does on the read side.
func (r inventoryRow) DetailHref() string {
	switch {
	case r.Slug == "":
		return ""
	case strings.HasPrefix(r.Slug, "poison/"):
		return "/poisons/" + r.Slug
	case strings.HasPrefix(r.Slug, "trap/"):
		return "/traps/" + r.Slug
	}
	return "/items/" + r.Slug
}

// bulkStorageBonus is the capacity each of the four Shinobi storage tools
// adds to a character's inventory limit.
//
// These four rows are the one place equipment.bulk does not mean what it
// means everywhere else: the core book's Utility Kits table prints a "Bulk
// Bonus" column for them, not a Bulk column, and the ingest loaded that
// column into the same field. Treating those values as ordinary bulk would
// have a backpack costing 10 of the 10 capacity it is supposed to grant, so
// they are pulled out here by slug rather than read from the column.
//
// The book allows only one storage tool of a given type to benefit you at
// once, which is why this is keyed by slug and quantity is ignored: a
// second backpack is dead weight, not +20.
var bulkStorageBonus = map[string]float64{
	"gear/shinobi-backpack":   10,
	"gear/shinobi-waist-bag":  5,
	"gear/shinobi-belt-pouch": 3,
	"gear/shinobi-leg-pouch":  2,
}

// featBulkBonusGrants is flat max-bulk increases from feats:
//   - feat/f-athlete: "Increase your maximum bulk by +5."
//   - feat/brawny: "Increase your maximum bulk by +10."
var featBulkBonusGrants = map[string]float64{
	"feat/f-athlete": 5,
	"feat/brawny":    10,
}

// featBulkBonus sums featBulkBonusGrants across a character's taken feats.
func (s *server) featBulkBonus(characterID int64) (float64, error) {
	feats, err := s.loadCharacterFeats(characterID)
	if err != nil {
		return 0, err
	}
	var total float64
	for _, f := range feats {
		total += featBulkBonusGrants[f.Slug]
	}
	return total, nil
}

// bulkSummary is the inventory's carrying-capacity readout.
//
// Capacity is the core book's formula: 10 base inventory slots, plus 2 per
// point of Strength modifier, plus whatever storage tools are carried. A
// character over capacity is Encumbered (speed halved, disadvantage on all
// Strength/Dexterity/Constitution checks, attacks and saves) — the sheet
// says so rather than leaving the player to compare two numbers.
type bulkSummary struct {
	Carried  float64
	Capacity float64
	Base     float64 // 10 + 2 * Strength modifier + FeatureBonus
	Storage  float64 // sum of the carried storage tools' bonuses
	StrBonus float64 // the "+2 per Strength modifier" part, shown in the hint
	// FeatureBonus is extra max bulk from a class feature (currently just
	// Puppet Master's Always Prepared) — shown in the hint the same way
	// StrBonus is, so a raised number is never unexplained.
	FeatureBonus float64
	Unknown      int // rows whose bulk the rules don't record, so aren't counted
	Encumbered   bool
}

// computeInventoryBulk fills in each row's bulk figures and returns the
// whole-inventory summary. It mutates inv in place because the per-row and
// whole-sheet numbers come from exactly the same pass over the same rows,
// and splitting them into two functions means two places to keep the
// storage-tool special case right. featureBulkBonus is extra max bulk from
// a class feature (0 for a character with none) — computed by the caller
// since it needs a characterID query this function has no access to.
func computeInventoryBulk(inv []inventoryRow, sheet *charsheet.Sheet, featureBulkBonus float64) bulkSummary {
	summary := bulkSummary{Base: 10, FeatureBonus: featureBulkBonus}
	summary.Base += featureBulkBonus
	if sheet != nil {
		if str, ok := sheet.Abilities["str"]; ok {
			summary.StrBonus = float64(2 * str.Modifier)
			summary.Base += summary.StrBonus
		}
	}
	for i := range inv {
		row := &inv[i]
		if bonus, ok := bulkStorageBonus[row.Slug]; ok && row.Quantity > 0 {
			// Part 10: a storage tool only grants its capacity while worn —
			// one sitting unequipped in the bag doesn't expand what you can
			// carry, same as armor only contributing to AC and a weapon only
			// getting an attack row while equipped (buildAttacks above).
			// Previously, equipping a storage tool like the Shinobi Waist Bag
			// didn't adjust carry weight at all: Equipped was never checked,
			// so the bonus applied unconditionally and toggling equip made
			// no difference either way.
			if row.Equipped {
				row.StorageBonus = bonus
				summary.Storage += bonus
			}
			continue
		}
		if !row.Bulk.Valid {
			summary.Unknown++
			continue
		}
		row.BulkTotal = row.Bulk.Float64 * float64(row.Quantity)
		summary.Carried += row.BulkTotal
	}
	summary.Capacity = summary.Base + summary.Storage
	summary.Encumbered = summary.Carried > summary.Capacity
	return summary
}

// equipmentOption is one entry in the Inventory tab's "add an item"
// dropdown — every item in the rules, grouped by kind.
type equipmentOption struct {
	Slug, Name, Kind string
}

// equipmentOptionGroup is one <optgroup> of the add-item dropdown. Named
// apart from classes.go's equipmentGroup, which is an unrelated thing —
// one (a)/(b)/(c) starting-equipment alternative from a class entry.
type equipmentOptionGroup struct {
	Kind    string
	Label   string
	Options []equipmentOption
}

// equipmentKindLabels turns the equipment table's kind values into the
// headings the dropdown shows.
var equipmentKindLabels = map[string]string{
	"weapon":           "Weapons",
	"armor":            "Armor",
	"gear":             "Gear",
	"tool":             "Tools",
	"toolkit":          "Toolkits",
	"scroll":           "Scrolls",
	"enhancement_seal": "Enhancement Seals",
}

// equipmentKindOrder is the order the dropdown's groups appear in —
// weapons and armor first because those are the ones that change a
// character's numbers, then the rest.
var equipmentKindOrder = []string{"weapon", "armor", "gear", "tool", "toolkit", "scroll", "enhancement_seal"}

// loadEquipmentOptions reads every item in the rules for the Inventory
// tab's add-item dropdown. All 399 of them are sent with the page rather
// than searched over the wire: this is a local, offline app, the whole
// list is a few tens of kilobytes of markup, and a plain <select> the
// browser can type-ahead through needs no JavaScript at all.
func (s *server) loadEquipmentOptions() ([]equipmentOptionGroup, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, kind FROM equipment ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byKind := map[string][]equipmentOption{}
	for rows.Next() {
		var opt equipmentOption
		if err := rows.Scan(&opt.Slug, &opt.Name, &opt.Kind); err != nil {
			return nil, err
		}
		byKind[opt.Kind] = append(byKind[opt.Kind], opt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []equipmentOptionGroup
	for _, kind := range equipmentKindOrder {
		if len(byKind[kind]) == 0 {
			continue
		}
		label := equipmentKindLabels[kind]
		if label == "" {
			label = kind
		}
		out = append(out, equipmentOptionGroup{Kind: kind, Label: label, Options: byKind[kind]})
		delete(byKind, kind)
	}
	// Anything with a kind this code doesn't know about still has to show
	// up, rather than silently vanishing from the dropdown after a rules
	// update adds a category.
	var extra []string
	for kind := range byKind {
		extra = append(extra, kind)
	}
	sort.Strings(extra)
	for _, kind := range extra {
		out = append(out, equipmentOptionGroup{Kind: kind, Label: kind, Options: byKind[kind]})
	}
	return out, nil
}

// attackRow is one equipped weapon rendered as a pair of click-to-roll
// entries in the sheet's Attacks & Jutsu table: an attack roll (1d20 +
// AttackBonus) and a damage roll (DamageCount d DamageSides + DamageBonus).
type attackRow struct {
	Name        string
	Slug        string
	Ability     string // "STR"/"DEX" — shown so the chosen ability is visible, not guessed at
	AttackBonus int
	DamageCount int // 0 when damage_dice couldn't be parsed; the damage roll is then omitted
	DamageSides int
	DamageBonus int
	DamageDice  string // as printed in the rules, e.g. "1d8"
	DamageType  string
	// InventoryID is the character_inventory row this attack came from, so
	// the row's delete control can remove the weapon itself. An attack
	// only exists because a weapon is equipped, so "delete this attack"
	// can only sensibly mean "drop this weapon".
	InventoryID int64
	// CritRangeThreshold: the lowest d20 result that counts as a critical
	// hit for THIS attack roll — 0 means no widening (default 20), same
	// "0 means untouched" shape companionAttackRow's own field
	// (puppets.go) already uses. Weapon Specialist's Critical Focus
	// (class_features level 7, +1/+2/+3 ranks at 7/11/17) is the only
	// source right now — see weaponSpecialistCritRangeThreshold.
	CritRangeThreshold int

	// The composition behind AttackBonus/DamageBonus, so the row's "Adjust"
	// form can show what is actually selected. AttackAbility/DamageAbility are
	// the three-letter codes in use after any override; Derived is true while
	// the character has no override at all, which is what lets the row say the
	// numbers came from the weapon rather than from a choice.
	AttackAbility string
	AttackProf    string
	AttackFlat    int
	DamageAbility string
	DamageFlat    int
	Derived       bool
}

// customAttackRow is a stored custom attack plus the totals its parts add up
// to right now. The stored row keeps the FLAT parts (see migration 0009), so
// the roll buttons need the composition done at render time — and the edit
// form needs the parts, which is why this embeds rather than replaces.
type customAttackRow struct {
	charstore.CustomAttack
	AttackTotal int
	DamageTotal int
	// CritRangeThreshold: same shape as attackRow's own field of the same
	// name — 0 means no widening (default 20). Callers pass the character's
	// current Weapon Specialist Critical Focus threshold (see
	// weaponSpecialistCritRangeThreshold) for a "weapon"-kind list and 0 for
	// "jutsu"/"item" lists, since the book text grants the widening to
	// weapons (and Bukijutsu jutsu casts, which this custom-attack list
	// does not distinguish from other jutsu kinds and so is left untouched
	// here).
	CritRangeThreshold int
}

// composeCustomAttacks resolves each row's ability + proficiency + flat parts
// against the character as they are now, so a level-up or an ability change
// moves every custom attack by itself. A row with no ability chosen composes
// to exactly its flat number, which is what every row written before this
// existed does. critRangeThreshold is applied unconditionally to every row
// in list — pass 0 for a list that shouldn't widen (see CritRangeThreshold's
// own doc comment above).
func composeCustomAttacks(list []charstore.CustomAttack, sheet *charsheet.Sheet, critRangeThreshold int) []customAttackRow {
	out := make([]customAttackRow, 0, len(list))
	for _, a := range list {
		row := customAttackRow{
			CustomAttack: a,
			AttackTotal: charsheet.ComposeModifier(
				sheet.Abilities[a.AttackAbility].Modifier, sheet.ProficiencyBonus,
				a.AttackProf, a.AttackBonus),
			DamageTotal:        sheet.Abilities[a.DamageAbility].Modifier + a.DamageBonus,
			CritRangeThreshold: critRangeThreshold,
		}
		out = append(out, row)
	}
	return out
}

// weaponSpecialistCritFocusRank returns Weapon Specialist's own Critical
// Focus class feature rank (class_features row
// 'class/weapon-specialist/feature/critical-focus', level 7: "all weapons
// you are proficient in gain +1 rank of the Critical property which now
// applies to Bukijutsu you cast. This increases to +2 ranks at 11th level
// and +3 at 17th level") for the given Weapon Specialist class level — 0
// below 7th. The weapon_properties table's own "Critical" row converts each
// rank into a flat +1 widening of the crit threat range (max +3), so this
// rank feeds directly into a CritRangeThreshold of 20-rank.
func weaponSpecialistCritFocusRank(level int) int {
	switch {
	case level >= 17:
		return 3
	case level >= 11:
		return 2
	case level >= 7:
		return 1
	default:
		return 0
	}
}

// weaponSpecialistCritRangeThreshold resolves Critical Focus into the
// lowest d20 result that counts as a critical hit for this character's own
// weapon attacks right now — 0 means no widening (attackRow/customAttackRow
// both treat 0 as "use the default 20"). Applies unconditionally to every
// equipped/custom weapon attack once the character has the rank — the book
// text says "all weapons you are proficient in", not gated to a Weapon
// Focus pick the way weaponFocusBonusSet's flat bonus is.
func (s *server) weaponSpecialistCritRangeThreshold(characterID int64) (int, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil || level == 0 {
		return 0, err
	}
	rank := weaponSpecialistCritFocusRank(level)
	if rank == 0 {
		return 0, nil
	}
	return 20 - rank, nil
}

var damageDicePattern = regexp.MustCompile(`^(\d*)d(\d+)$`)

// buildAttacks turns the character's equipped weapons — and equipped
// explosive tags/bombs, see below — into rollable attack rows. Two rules
// decide the ability used, both read off the weapon's printed properties
// text because the ingested equipment rows have weapon_category set to NULL
// throughout — the source book prints properties but no simple/martial
// column, so properties are the only signal actually present in the data:
//
//   - Finesse: the better of Strength and Dexterity, the player's choice in
//     the book, resolved here as "whichever is higher" since no sheet-side
//     preference exists to consult.
//   - Otherwise a weapon that is shot rather than thrown (Ammunition or a
//     Range band, with no Thrown property) uses Dexterity. A thrown weapon
//     keeps Strength — throwing is a melee weapon's own ability unless the
//     weapon is also Finesse, which the first rule already caught.
//   - Everything else is Strength.
//
// The full proficiency bonus is included by default. This app does not model
// weapon proficiency anywhere — character_proficiencies only covers skills,
// tools, languages and saving throws — so there is nothing to check against,
// and a character equipping a weapon they cannot use is the rare case. The
// template says so beside the table rather than leaving the number
// unexplained.
//
// Every one of those decisions is a default, not a verdict: character_weapon_
// attack_options (migration 0009) can override the attack ability, the share
// of proficiency, the damage ability, and add a flat extra to either. A weapon
// with no override row stays entirely derived, so nothing has to be configured
// for the common case.
//
// equipment.kind is not always "weapon" for something that belongs in this
// table: Paper Bombs, Flash Tags, Fire/Ice/Shock Bombs, Breaching Tags and
// Poison Tags (and every upgrade tier of each) are catalogued as kind='tool'
// — the book prints them as a tool-slot item, not a weapon — but they are
// thrown/planted consumables with a printed damage roll and/or Save DC, same
// shape as any other rollable attack. equipment.save_dc (added by migration
// 0017, "explosive tags/bombs' own Save DC") is the signal that separates
// this family from an ordinary tool: it is NULL for every ordinary tool row
// (lockpicks, radios, kits) and set for exactly these bomb/tag rows, so
// "kind='tool' AND save_dc IS NOT NULL" is included below alongside
// "kind='weapon'". A row with no printed damage_dice (Flash Tag, Poison Tag)
// still gets an attack row with an empty damage column ("—", the same
// fallback an ordinary weapon with no damage_dice would get) — its Save DC
// and effect are on the item's own card, linked from the row's name.
func (s *server) buildAttacks(characterID int64, inventory []inventoryRow, sheet *charsheet.Sheet) ([]attackRow, error) {
	options, err := charstore.ListWeaponAttackOptions(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	focusSlugs, focusBonus, err := s.weaponFocusBonusSet(characterID, sheet)
	if err != nil {
		return nil, err
	}
	critRangeThreshold, err := s.weaponSpecialistCritRangeThreshold(characterID)
	if err != nil {
		return nil, err
	}
	wardenWeaponSlug, wardenAggressiveAttack, wardenBladesAggression, err := s.hunterNinWardenWeaponBonuses(characterID, sheet)
	if err != nil {
		return nil, err
	}
	arsenalWeaponKeywords, arsenalWeaponBonus, err := s.hunterNinArsenalWeaponBonus(characterID)
	if err != nil {
		return nil, err
	}
	cookingToolSlug, cookingToolDieSize, cookingToolDamageType, cookingToolCritBonus, err := s.cookingToolInfusionAttackOverrides(characterID, sheet)
	if err != nil {
		return nil, err
	}
	cookingToolPipeSlug, cookingToolPipeDieSize, cookingToolPipeDamageType, err := s.cookingToolInfusionPipeAttackOverrides(characterID, sheet)
	if err != nil {
		return nil, err
	}
	sentWeaponSlug, err := s.sentAttackOverrides(characterID, sheet)
	if err != nil {
		return nil, err
	}
	var out []attackRow
	for _, item := range inventory {
		if !item.Equipped || item.Slug == "" {
			continue
		}
		var kind string
		var damageDice, damageType, properties sql.NullString
		var saveDC sql.NullInt64
		if strings.HasPrefix(item.Slug, "custom/") {
			// A custom item's rollable flag (custom_items.rollable_kind) is
			// the gate here, not its free-text Kind/Type — see CustomItem's
			// doc comment for why those are kept apart.
			ci, err := charstore.GetCustomItemBySlug(s.charDB, item.Slug)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
			if ci.RollableKind != "weapon" {
				continue
			}
			kind = "weapon"
			damageDice = sql.NullString{String: ci.DamageDice, Valid: ci.DamageDice != ""}
			damageType = sql.NullString{String: ci.DamageType, Valid: ci.DamageType != ""}
			properties = sql.NullString{String: ci.Properties, Valid: ci.Properties != ""}
		} else {
			err := s.rulesDB.QueryRow(
				`SELECT kind, damage_dice, damage_type, properties, save_dc FROM equipment WHERE slug = ?`,
				item.Slug,
			).Scan(&kind, &damageDice, &damageType, &properties, &saveDC)
			if err == sql.ErrNoRows {
				continue // stale slug after a rules update — already handled the same way in loadCharacterInventory
			}
			if err != nil {
				return nil, err
			}
			// See the doc comment above: a tool-kind row only qualifies when
			// it is one of the explosive tag/bomb family (save_dc set).
			// Every other tool (and every armor/gear/toolkit/scroll/
			// enhancement_seal row) is excluded, same as before.
			if kind != "weapon" && !(kind == "tool" && saveDC.Valid) {
				continue
			}
		}

		props := strings.ToLower(properties.String)
		str := sheet.Abilities["str"].Modifier
		dex := sheet.Abilities["dex"].Modifier
		ability := "str"
		switch {
		case strings.Contains(props, "finesse"):
			if dex > str {
				ability = "dex"
			}
		case !strings.Contains(props, "thrown") &&
			(strings.Contains(props, "ammunition") || strings.Contains(props, "range")):
			ability = "dex"
		}

		// Cooking Tool Infusion's own text: "you can choose to use
		// Intelligence in place of Strength when determining your attack
		// and damage rolls" — applied unconditionally once the implement is
		// equipped, same "always-on once known" treatment Storm Rider's Air
		// Trecks weapon gets for its own identical clause. This replaces the
		// derived ability outright (still overridable below by a manual
		// per-weapon Adjust, same as every other bonus in this function).
		isCookingTool := cookingToolSlug != "" && item.Slug == cookingToolSlug
		if isCookingTool {
			ability = "int"
			if cookingToolDamageType != "" {
				damageType.String = cookingToolDamageType
			}
		}

		// Bonus Tool Infusion: Pipe's own text (Herbalist, 3rd level): "you
		// can also use Intelligence in place of Strength when determining
		// your attack and damage rolls" — the identical clause to the base
		// Cooking Tool Infusion above, applied independently to whichever
		// equipped item matches the character's own Pipe pick instead. The
		// two implement catalogs live under disjoint slug prefixes
		// (weapon/cooking-tool-% vs weapon/cooking-pipe-%, see
		// 0064_cooking_tool_infusion_pipe_implements.sql), so isCookingTool
		// and isCookingToolPipe can never both be true for the same item —
		// a character holding both features simultaneously gets two
		// independently-overridden equipped weapons, never one item with
		// both bonuses stacked.
		isCookingToolPipe := cookingToolPipeSlug != "" && item.Slug == cookingToolPipeSlug
		if isCookingToolPipe {
			ability = "int"
			if cookingToolPipeDamageType != "" {
				damageType.String = cookingToolPipeDamageType
			}
		}

		// S.E.N.Ts' own text: "you can also use your Intelligence Modifier in
		// place of Dexterity for all weapon attack and damage rolls using
		// S.E.N. Ts" — same "always-on once equipped, still overridable
		// below" treatment as Cooking Tool Infusion's identical clause.
		isSENT := sentWeaponSlug != "" && item.Slug == sentWeaponSlug
		if isSENT {
			ability = "int"
		}

		// Apply the character's override for this weapon, if any. Each part
		// falls back independently to what the weapon itself implies.
		opt, overridden := options[item.ID]
		attackAbility, damageAbility := ability, ability
		prof := charsheet.ProfFull
		if overridden {
			if opt.AttackAbility != "" {
				attackAbility = opt.AttackAbility
				damageAbility = opt.AttackAbility
			}
			if opt.AttackProf != "" {
				prof = opt.AttackProf
			}
			if opt.DamageAbility != "" {
				damageAbility = opt.DamageAbility
			}
		}

		// Weapon Specialist's Weapon Focus: a flat Attack & Damage bonus
		// (+1/+2/+3, level-gated — see charstore.WeaponFocusBonus)
		// automatically applied to every equipped weapon of a chosen focus
		// type, on top of any per-weapon override above rather than
		// replacing it.
		weaponFocusBonus := 0
		if focusSlugs[item.Slug] {
			weaponFocusBonus = focusBonus
		}

		// Blade Warden's Aggressive Attack (7th level, always-on once
		// known) swaps the single-ability damage sum for BOTH Strength and
		// Dexterity, and Blade's Aggression (10th) separately steps the
		// damage die up by 1 — both flat, ever-on bonuses gated purely on
		// "this equipped item is the character's own Warden Weapon", never
		// spent or triggered, so nothing beyond the match itself needs
		// tracking (the pick that IS tracked, and gates both bonuses in
		// the first place, is the Warden Weapon TYPE choice itself — see
		// hunterNinWardenWeaponBonuses). Deliberately left out of
		// DamageFlat/DamageAbility below: those feed the per-weapon
		// "Adjust" form's single-ability composition display, which has no
		// way to represent "both Str and Dex" — the roll button's own
		// DamageBonus total is correct either way.
		damageAbilityBonus := sheet.Abilities[damageAbility].Modifier
		effectiveDamageDice := damageDice.String
		if wardenWeaponSlug != "" && item.Slug == wardenWeaponSlug {
			if wardenAggressiveAttack {
				damageAbilityBonus = sheet.Abilities["str"].Modifier + sheet.Abilities["dex"].Modifier
			}
			if wardenBladesAggression {
				effectiveDamageDice = stepWeaponDie(effectiveDamageDice)
			}
		}

		// Arsenalist's Arsenal Weapons (3rd level, always-on once known):
		// any equipped weapon whose properties match one of the character's
		// picked Arsenal keywords (thrown/multiattack/light/finesse) steps
		// its damage die up by 1 and gains a flat +2/+4/+6 damage bonus —
		// see hunterNinArsenalWeaponBonus. The Hidden Weapon property tag
		// and the Lethal-Attack trigger override stay manual/narrated; only
		// the die-step and flat damage bonus reach the Attacks table.
		arsenalBonus := 0
		for kw := range arsenalWeaponKeywords {
			if strings.Contains(props, kw) {
				effectiveDamageDice = stepWeaponDie(effectiveDamageDice)
				arsenalBonus = arsenalWeaponBonus
				break
			}
		}

		// Cooking Tool Infusion's own text: "Your cooking tool deals damage
		// equal to your Cooking Dice" — the level-scaling chart
		// (cookingDieSize, 1d4 at 1st-4th up to 1d12 at 17th+) replaces the
		// catalog row's own inert 1d4 outright, and the Critical/Critical 2
		// property picks widen this weapon's own crit-threat range (+1 per
		// rank, same "0 means untouched" shape weaponSpecialistCritRangeThreshold
		// already uses) — the narrower of the two thresholds (Critical Focus's
		// own class-wide threshold, if any) wins, since either alone widens
		// the range and they don't stack per weapon_properties' own text.
		rowCritRangeThreshold := critRangeThreshold
		if isCookingTool {
			if cookingToolDieSize != "" {
				effectiveDamageDice = cookingToolDieSize
			}
			if cookingToolCritBonus > 0 {
				if ct := 20 - cookingToolCritBonus; rowCritRangeThreshold == 0 || ct < rowCritRangeThreshold {
					rowCritRangeThreshold = ct
				}
			}
		}

		// Bonus Tool Infusion: Pipe's own text: "your cooking tool deals
		// damage equal to your Cooking Dice" — the same live-scaling
		// Cooking Die chart the base Cooking Tool Infusion weapon reads
		// (cookingToolInfusionPipeAttackOverrides calls the same
		// cookingDieSize helper), applied to the Pipe's own equipped item
		// instead. Pipe's own property catalog has no Critical/Critical 2
		// entry (see cookingToolPipePropertyL6Options/L11Options), so there
		// is no crit-range-threshold bonus to fold in here.
		if isCookingToolPipe && cookingToolPipeDieSize != "" {
			effectiveDamageDice = cookingToolPipeDieSize
		}

		// S.E.N.Ts' own text: "It immediately becomes a d10 stack and its
		// damage die increases by a step" — replaces the catalog row's own
		// damage dice outright with 1d12 (1d10, stepped once via
		// stepWeaponDie), same "override replaces the inert catalog value"
		// treatment Cooking Tool Infusion's own die-size chart gets above.
		if isSENT {
			effectiveDamageDice = stepWeaponDie("1d10")
		}

		row := attackRow{
			Name:               item.Name,
			Slug:               item.Slug,
			InventoryID:        item.ID,
			CritRangeThreshold: rowCritRangeThreshold,
			Ability:            strings.ToUpper(attackAbility),
			AttackBonus: charsheet.ComposeModifier(
				sheet.Abilities[attackAbility].Modifier, sheet.ProficiencyBonus, prof, opt.AttackBonus+weaponFocusBonus),
			DamageBonus:   damageAbilityBonus + opt.DamageBonus + weaponFocusBonus + arsenalBonus,
			DamageDice:    effectiveDamageDice,
			DamageType:    damageType.String,
			AttackAbility: attackAbility,
			AttackProf:    prof,
			AttackFlat:    opt.AttackBonus + weaponFocusBonus,
			DamageAbility: damageAbility,
			DamageFlat:    opt.DamageBonus + weaponFocusBonus + arsenalBonus,
			Derived:       !overridden,
		}
		if m := damageDicePattern.FindStringSubmatch(strings.TrimSpace(effectiveDamageDice)); m != nil {
			count := 1
			if m[1] != "" {
				count, _ = strconv.Atoi(m[1])
			}
			sides, _ := strconv.Atoi(m[2])
			row.DamageCount, row.DamageSides = count, sides
		}
		out = append(out, row)
	}
	return out, nil
}

type jutsuSheetRow struct {
	Slug, Name, Rank, CostText string
	CostChakra                 *int   // nil when the jutsu's cost isn't a fixed number (e.g. "Special")
	AttackKind                 string // "Ninjutsu"/"Genjutsu"/"Taijutsu"/"Bukijutsu" when this jutsu makes an attack roll, "" otherwise
	AttackBonus                int    // the to-hit modifier actually rolled, override included
	IsConcentration            bool   // true when this jutsu's duration requires concentration (see isConcentrationDuration)

	// CritRangeThreshold: Weapon Specialist's Critical Focus widens the crit
	// range specifically for "Bukijutsu you cast" (class_features, 7th/11th/
	// 17th level) — same field/meaning as attackRow's own CritRangeThreshold,
	// but gated on this jutsu's own classification column being Bukijutsu
	// rather than applied to every jutsu attack roll, since Critical Focus's
	// text does not extend to Ninjutsu/Genjutsu/plain-Taijutsu jutsu. 0 means
	// no widening (the client falls back to a plain 20).
	CritRangeThreshold int

	// UpcastOptions is every rank this jutsu can be cast at (base rank
	// through S) with the chakra cost each one spends, from a parsed
	// jutsu_upcast_rules.chakra_per_rank. len <= 1 means "nothing to
	// upcast to" — the sheet renders today's plain fixed-cost Cast button
	// in that case, same as a jutsu with no upcast rule at all.
	UpcastOptions []jutsuUpcastOption

	// Damage, which a jutsu only has if the player pinned it: the book prints
	// it in prose, never as dice. DamageCount == 0 means no damage roll.
	DamageCount, DamageSides int
	DamageBonus              int
	DamageDice               string // "2d6", for display
	DamageType               string

	// The composition behind the numbers, so the row's edit form can show what
	// is selected. Derived is true while the character has no override, which
	// is what lets the row say the to-hit came from the jutsu's own text.
	AttackAbility string
	AttackProf    string
	AttackFlat    int
	DamageAbility string
	DamageFlat    int
	Derived       bool

	// SourceLabel is non-empty for a jutsu granted for free by a class
	// feature or clan trait ("Class Feature"/"Clan") rather than one the
	// player chose — it has no character_jutsu row of its own, doesn't
	// count against JutsuKnownCap, and can't be forgotten from the sheet.
	SourceLabel string

	// CostOverride is the player's own manual override of this jutsu's
	// Chakra cost (feats, clan, or class features that cast it for less —
	// or more — than the printed cost_chakra), separate from CostChakra
	// (the resulting EFFECTIVE cost after every automatic and manual
	// override is applied) so the modify-jutsu box's input can show only
	// what the player actually typed, not e.g. Martial Technique's own
	// automatic value. nil means no manual override is set.
	CostOverride *int

	// FreeCast is set when a Malleable Mirage the character has picked
	// grants a LIMITED free-or-half-cost cast of this specific jutsu
	// (genjutsuMirageJutsuGrants, genjutsu.go) — nil for a jutsu with no
	// such grant, and also nil for an UNLIMITED grant (Beast Speech/Myriad
	// Forms/Piece of Mind), which instead applies straight to CostChakra
	// above with no separate use-limit to choose between. A limited grant
	// can't just overwrite CostChakra the way the unlimited ones do: the
	// player must be free to spend a normal chakra-cost cast instead of a
	// scarce Mirage use, so the sheet renders a SEPARATE "Cast via
	// <Mirage>" button alongside the normal Cast button rather than
	// replacing it.
	FreeCast *jutsuFreeCastGrant
}

// jutsuFreeCastGrant is the per-row, per-character view of a limited
// alternate-cost jutsu grant paid out of a rest-scoped pool instead of
// Chakra — originally Malleable Mirages' own free/half-cost grants
// (genjutsu.go), now also Wolves Legacy's Wolf Techniques (hunter_nin.go),
// Interrogationist's Unerring Eye/Perfect Mind (intelligence_operative.go),
// and Mech Crafter's Adaptive Movement (science_nin.go). MirageName/
// ResourceKey/Uses/Max come from the matching customResourceGrants entry
// (custom_resources.go) via loadCustomResources, the same rest-scoped pool
// already tracked on the sheet's Resources list; Cost is the Chakra still
// paid alongside the pool spend (0 for a free grant, the jutsu's own book
// cost halved — rounded down — for a half-cost one, always 0 for Wolf
// Techniques and Adaptive Movement, both of which pay the pool spend
// instead of Chakra rather than alongside it). UsesPerCast is how many pool
// uses one cast spends (1 for every Mirage grant; Wolf Techniques spends a
// flat 2; Adaptive Movement spends the jutsu's own printed cost_chakra,
// since the pool it draws from — CCD — is itself measured in Chakra rather
// than a discrete use count). CastRank overrides the rank the cast (and its
// concentration tracking) is recorded at — "" means the jutsu's own printed
// Rank, the only value any Mirage or Adaptive Movement grant ever needs;
// Wolf Techniques fixes this to "B" regardless of the jutsu's own rank.
type jutsuFreeCastGrant struct {
	MirageName  string
	ResourceKey string
	Cost        int
	Uses, Max   int
	UsesPerCast int
	CastRank    string
}

// jutsuUpcastOption is one selectable rank a jutsu can be cast at, paired
// with the chakra cost that rank spends.
type jutsuUpcastOption struct {
	Rank string
	Cost int
}

// taijutsuMartialTechniqueFlatCost is Martial Technique's (class/taijutsu-
// specialist/feature/martial-technique) flat per-rank Chakra cost table: a
// Taijutsu Specialist casts any Taijutsu-classification jutsu for this fixed
// amount instead of its printed cost_chakra, regardless of what the jutsu
// would otherwise cost. Only D through S appear because no Taijutsu jutsu in
// the rules DB is E-rank (confirmed against v_jutsu). A jutsu whose cost is
// textual ("Special") is excluded by the caller rather than here — those
// rows have no numeric cost_chakra to begin with.
//
// RAW also grants an opt-out ("you can choose to not use this feature when
// you would cast a Taijutsu") and forbids stacking with other cost-reduction
// sources while this is active. Neither is implemented: this override is
// always-on, matching how every other always-on Group 1 feature in this
// codebase works (e.g. Weapon Focus's always-on bonus). Documented Group 2/3
// gap — a per-cast opt-out toggle and cost-reduction-source interaction
// would need their own UI and are out of scope for this pass.
var taijutsuMartialTechniqueFlatCost = map[string]int{
	"D": 3,
	"C": 5,
	"B": 9,
	"A": 14,
	"S": 20,
}

// buildUpcastOptions enumerates every rank from baseRank through S (the
// top of jutsuRankOrder — see jutsu.go), each costing baseCost plus
// perRank chakra for every rank above base.
//
// Deliberately NOT capped at the character's own Highest Rank Known:
// confirmed with the project's rules authority that some in-game situations
// let a character cast above what they'd normally know, and enforcing a
// ceiling here would be inventing a restriction the book doesn't state for
// casting specifically — that table only gates what can be LEARNED. DM/
// player judgment covers any narrower limit in practice.
func buildUpcastOptions(baseRank string, baseCost, perRank int) []jutsuUpcastOption {
	baseOrder, ok := jutsuRankOrder[baseRank]
	if !ok {
		return nil
	}
	var opts []jutsuUpcastOption
	for rank, order := range jutsuRankOrder {
		if order < baseOrder {
			continue
		}
		opts = append(opts, jutsuUpcastOption{
			Rank: rank,
			Cost: baseCost + perRank*(order-baseOrder),
		})
	}
	sort.Slice(opts, func(i, k int) bool {
		return jutsuRankOrder[opts[i].Rank] < jutsuRankOrder[opts[k].Rank]
	})
	return opts
}

// jutsuAttackPattern picks the attack roll out of a jutsu's printed
// description — "Make a Ranged Ninjutsu Attack", "Make one Melee Taijutsu
// Attack", "Make two Genjutsu Attacks". The description is the only place
// this lives: the jutsu table's own `classification` column says what
// SCHOOL a jutsu belongs to, not whether casting it involves an attack roll
// at all, and most jutsu resolve with a saving throw instead.
//
// Anchoring on "Make ... Attack" rather than the bare words is what keeps
// buff jutsu out. Several jutsu describe what happens to attacks you make
// later ("Weapon and Taijutsu attacks made with this weapon deal an
// additional 2d4 Poison Damage"); those are not themselves attacks, and a
// bare `(ninjutsu|genjutsu|taijutsu) attack` match would give every one of
// them a to-hit button that rolls nothing real.
//
// Bukijutsu is included: it is one of charsheet.AttackKinds, so there is a
// real number to put on the button. Hijutsu is still absent — no jutsu
// describes a "Hijutsu Attack" — and falls through with no attack bonus
// rather than borrowing another kind's number.
var jutsuAttackPattern = regexp.MustCompile(
	`(?i)\bmakes?\s+(?:a|an|one|two|three|four|five|\d+)?\s*(?:melee\s+|ranged\s+|unarmed\s+)*(ninjutsu|genjutsu|taijutsu|bukijutsu)\s+attacks?\b`)

// jutsuAttackKind returns the capitalised attack kind a jutsu's description
// calls for, or "" when it isn't an attack-roll jutsu.
func jutsuAttackKind(description string) string {
	m := jutsuAttackPattern.FindStringSubmatch(description)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1][:1]) + strings.ToLower(m[1][1:])
}

// resolveJutsuAttackKind reroutes a Bukijutsu-classified jutsu whose
// description uses the book's shared physical-attack wording ("Make a Melee
// Taijutsu Attack") from the Taijutsu bucket to the Bukijutsu one.
//
// Every real Bukijutsu-classified jutsu describes its attack roll with that
// same "Taijutsu Attack" phrasing rather than "Bukijutsu Attack" — the book
// never actually prints the latter — so jutsuAttackKind's text-only reading
// alone sends every one of them through the Taijutsu bucket. That silently
// starves three mechanics that only apply to the Bukijutsu bucket
// (charsheet.go's Weapon Focus attack bonus, Spirited Fighter's Bukijutsu
// CHA override, and Lethal Precision's Bukijutsu DEX override) of any real
// jutsu to ever apply to. classification (the jutsu table's own SCHOOL
// column, unlike the free-text kind) is what actually says a jutsu is a
// weapon jutsu, so it — not the shared wording — decides the bucket here.
func resolveJutsuAttackKind(kind, classification string) string {
	if kind == "Taijutsu" && strings.Contains(strings.ToLower(classification), "bukijutsu") {
		return "Bukijutsu"
	}
	return kind
}

func (s *server) handleCharacterSheet(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		// Compute's own doc: returns a plain error (not sql.ErrNoRows
		// directly) for "character not found" — same substring check
		// charsheet.go's own callers would need, since it wraps with
		// fmt.Errorf. Simplest reliable signal here: charDB has no such
		// row at all.
		var exists int
		if s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&exists) == nil && exists == 0 {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute character sheet:", err)
		return
	}

	if err := s.ensureScienceNinAutoGrants(id, sheet); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("ensure science-nin auto grants:", err)
		return
	}
	inventory, err := s.loadCharacterInventory(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character inventory:", err)
		return
	}
	featureBulkBonus, err := s.puppetAlwaysPreparedBulkBonus(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load always prepared bulk bonus:", err)
		return
	}
	chassisBulkBonus, err := s.puppetChassisPropertyBulkBonus(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load chassis property bulk bonus:", err)
		return
	}
	featBulk, err := s.featBulkBonus(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feat bulk bonus:", err)
		return
	}
	backupPlanBonus, err := s.backupPlanBulkBonus(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load backup plan bulk bonus:", err)
		return
	}
	bulk := computeInventoryBulk(inventory, sheet, featureBulkBonus+chassisBulkBonus+featBulk+backupPlanBonus)
	attacks, err := s.buildAttacks(id, inventory, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build attacks:", err)
		return
	}
	jutsu, err := s.loadCharacterJutsuSheet(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character jutsu:", err)
		return
	}
	customWeaponAttacks, err := charstore.ListCustomAttacks(s.charDB, id, "weapon")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom weapon attacks:", err)
		return
	}
	customJutsuAttacks, err := charstore.ListCustomAttacks(s.charDB, id, "jutsu")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom jutsu attacks:", err)
		return
	}
	// "Other rollable" custom items (custom_items.rollable_kind='other') land
	// here too, same as a hand-written one — see applyCustomItemRollableWiring.
	customItemAttacks, err := charstore.ListCustomAttacks(s.charDB, id, "item")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom item attacks:", err)
		return
	}
	critRangeThreshold, err := s.weaponSpecialistCritRangeThreshold(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon specialist crit range threshold:", err)
		return
	}
	customWeaponRows := composeCustomAttacks(customWeaponAttacks, sheet, critRangeThreshold)
	customJutsuRows := composeCustomAttacks(customJutsuAttacks, sheet, 0)
	customItemRows := composeCustomAttacks(customItemAttacks, sheet, 0)
	concentration, err := s.loadConcentrationView(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load concentration:", err)
		return
	}
	drive, goal, fear, err := s.loadCharacterAmbitions(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character ambitions:", err)
		return
	}
	// Only feeds Passive Traits/Resistances below (and, further down,
	// free jutsu grants and mergeFeatFeatures) — the Core tab no longer
	// shows this list itself; see reference.go's Class Reference popup for
	// the player-facing "what do I have" view.
	grantedFeatures, err := s.loadGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load granted features:", err)
		return
	}
	chatLog, err := s.loadChatLog(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load chat log:", err)
		return
	}
	toolProfs, err := s.loadCharacterProficiencyValues(id, "tool")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load tool proficiencies:", err)
		return
	}
	languages, err := s.loadCharacterProficiencyValues(id, "language")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load languages:", err)
		return
	}
	customSkills, err := s.loadCustomSkills(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom skills:", err)
		return
	}
	toolMods, err := s.loadProficiencyMods(id, "tool")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load tool proficiency mods:", err)
		return
	}
	skillMods, err := s.loadProficiencyMods(id, "skill")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load skill proficiency mods:", err)
		return
	}
	toolRows := buildProficiencyRows(toolProfs, "tool", toolMods, sheet)
	customSkillRows := buildProficiencyRows(customSkills, "skill", skillMods, sheet)
	allTools, err := s.loadAllToolNames()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load all tool names:", err)
		return
	}
	allLanguages, err := s.loadAllLanguageNames()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load all language names:", err)
		return
	}
	classSummary, err := s.loadClassSummary(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load class summary:", err)
		return
	}
	companions, err := charstore.ListCompanions(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companions:", err)
		return
	}
	puppetsTab, err := s.loadPuppetsTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppets tab:", err)
		return
	}
	puppetTactics, err := s.loadPuppetTacticsTabData(id, puppetsTab.PuppetMasterLevel)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet tactics:", err)
		return
	}
	mastery, err := s.loadMasteryData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load mastery:", err)
		return
	}
	summonsTab, err := s.loadSummonsTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load summons tab:", err)
		return
	}
	equipmentOptions, err := s.loadEquipmentOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load equipment options:", err)
		return
	}
	characterFeats, err := s.loadCharacterFeats(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character feats:", err)
		return
	}
	// Taken feats belong in the Core tab's Features & Traits list too, in
	// the same level order as everything else there.
	grantedFeatures = mergeFeatFeatures(grantedFeatures, characterFeats)
	patternRows, err := s.hunterNinPatternPassiveRows(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load hunter-nin pattern rows:", err)
		return
	}
	fullMetalShinobiRows, err := s.fullMetalShinobiPassiveRows(id, sheet.Level, grantedFeatures)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load full-metal shinobi resistance rows:", err)
		return
	}
	passiveTraitFeatures := append(grantedFeatures, patternRows...)
	passiveTraitFeatures = append(passiveTraitFeatures, fullMetalShinobiRows...)
	if demonSightRow, err := s.genjutsuMirageDemonSightPassiveRow(id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load genjutsu mirage demon sight row:", err)
		return
	} else if demonSightRow != nil {
		passiveTraitFeatures = append(passiveTraitFeatures, *demonSightRow)
	}
	passiveTraits := computePassiveTraits(passiveTraitFeatures, sheet.Level)
	customResources, err := s.loadCustomResources(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom resources:", err)
		return
	}
	martialDice, err := s.loadMartialDice(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial dice:", err)
		return
	}
	martialTechniques, err := s.loadMartialTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial techniques:", err)
		return
	}
	weaponFocus, err := s.loadWeaponFocusTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon focus:", err)
		return
	}
	weaponForm, err := s.loadWeaponFormTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon form:", err)
		return
	}
	martialDefense, err := s.loadMartialDefenseTabData(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial defense:", err)
		return
	}
	hunterTechniques, err := s.loadHunterTechniquesTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load hunter techniques:", err)
		return
	}
	cookingNin, err := s.loadCookingNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load cooking-nin:", err)
		return
	}
	genjutsu, err := s.loadGenjutsuTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load genjutsu:", err)
		return
	}
	medicalNin, err := s.loadMedicalNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load medical-nin:", err)
		return
	}
	scoutNin, err := s.loadScoutNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin:", err)
		return
	}
	intelligenceOperative, err := s.loadIntelligenceOperativeTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load intelligence operative:", err)
		return
	}
	ninjutsuSpecialist, err := s.loadNinjutsuSpecialistTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load ninjutsu specialist:", err)
		return
	}
	scienceNin, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin:", err)
		return
	}

	// The three library panes. Each is told what the character already has
	// so an owned row can be dimmed rather than offered again.
	ownedItems := map[string]bool{}
	for _, item := range inventory {
		if item.Slug != "" {
			ownedItems[item.Slug] = true
		}
	}
	itemLibrary, err := s.loadSheetItemLibrary(ownedItems)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load item library:", err)
		return
	}
	ownedFeats := map[string]bool{}
	for _, feat := range characterFeats {
		ownedFeats[feat.Slug] = true
	}
	featCharacter, err := s.buildFeatCharacter(id, sheet, characterFeats)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build feat eligibility context:", err)
		return
	}
	featLibrary, err := s.loadSheetFeatLibrary(ownedFeats, featCharacter)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feat library:", err)
		return
	}
	ownedJutsu := map[string]bool{}
	for _, j := range jutsu {
		ownedJutsu[j.Slug] = true
	}
	jutsuLibrary, err := loadJutsuList(s.rulesDB)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu library:", err)
		return
	}
	jutsuSourceTiles := s.jutsuSourceTilesOrNil()

	// "Jutsu Known": the class's own progression table (v_class_levels) says
	// how many jutsu a character of this level knows, and because Level is a
	// real class level the allowance grows on its own at every level-up —
	// nothing here has to be re-run by hand. -1 means "no number recorded for
	// this class/level", which the templates render as a bare count with no
	// denominator rather than as a cap of zero. Aggregated across every class
	// the character has (multiclassing) — see jutsuKnownCapForCharacter.
	classes, err := s.loadCharacterClassLevels(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load classes for jutsu known:", err)
		return
	}
	jutsuKnownCap, err := s.jutsuKnownCapForCharacter(id, sheet, grantedFeatures)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu known cap for sheet:", err)
		return
	}
	classSlugs := make([]string, len(classes))
	for i, c := range classes {
		classSlugs[i] = c.Slug
	}
	jutsuOrigins, err := s.loadJutsuOrigins(classSlugs, sheet.ClanSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu origins:", err)
		return
	}

	// No rule anywhere lets a character benefit from a second clan's jutsu
	// (unlike a second class, reachable via multiclassing) — dropped from
	// the library outright rather than left origin-tagged "other" and
	// merely filterable, since there's never a legitimate reason to show
	// them here at all.
	otherClans, err := otherClansJutsu(s.rulesDB, sheet.ClanSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load other-clan jutsu:", err)
		return
	}
	filteredLibrary := jutsuLibrary[:0]
	for _, j := range jutsuLibrary {
		if !otherClans[j.Slug] {
			filteredLibrary = append(filteredLibrary, j)
		}
	}
	jutsuLibrary = filteredLibrary

	// Elemental (Nature Release) affinities — see elemental_affinity.go.
	// grantedFeatureSlugs drives both the combo-clan "second release at 7th
	// level" grant and the Professor subclass's own three picks; reusing
	// the already-merged grantedFeatures (class/subclass/clan/feats) here
	// rather than recomputing it a second time.
	grantedFeatureSlugs := map[string]bool{}
	for _, f := range grantedFeatures {
		grantedFeatureSlugs[f.Slug] = true
	}
	elementalAffinityPicks, err := charstore.ListElementalAffinities(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load elemental affinity picks:", err)
		return
	}
	hasNatureReleaseFeat, err := s.characterHasFeat(id, natureReleaseFeatSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("check nature release feat:", err)
		return
	}
	elementalAffinities := resolveElementalAffinities(sheet.ClanSlug, hasNatureReleaseFeat, grantedFeatureSlugs, elementalAffinityPicks)
	elementalAffinitySlotsData := elementalAffinitySlots(sheet.ClanSlug, hasNatureReleaseFeat, grantedFeatureSlugs, elementalAffinityPicks)
	// Elemental Resistance (Elemental Scout, 6th level) isn't in the
	// static passiveTraitGrants table computePassiveTraits already
	// resolved above — its Target is the character's own Elemental
	// Knowledge pick, only known once elementalAffinityPicks is loaded.
	passiveTraits = mergePassiveResistance(passiveTraits,
		scoutNinElementalResistanceEntry(grantedFeatures, elementalAffinityPicks["elemental-knowledge"]))
	affinitySet := map[string]bool{}
	for _, a := range elementalAffinities {
		affinitySet[a.Element] = true
	}
	medicalRankCap := characterMedicalRankCap(classSlugs, grantedFeatureSlugs, sheet.Level)
	hasByTheBook := grantedFeatureSlugs[patissierChefByTheBookFeatureSlug]
	eligibleJutsu := map[string]bool{}
	for _, j := range jutsuLibrary {
		byTheBookGrant := hasByTheBook && patissierChefByTheBookHealingJutsuSlugs[j.Slug]
		eligibleJutsu[j.Slug] = jutsuEligible(jutsuOrigins[j.Slug], j.Keywords, affinitySet, len(elementalAffinities) > 0, medicalRankCap, j.Rank.String, byTheBookGrant)
	}

	// sheet-layout.js's grid ("grid:core"/"grid:bio") and sheet-subgrid.js's
	// tile order/orientation ("subgrid:<name>") — see 0015_sheet_ui_state.sql
	// for why this moved server-side. Decoded once here (rather than handed
	// to the template as raw strings) so jsonify's HTML-safe re-encoding
	// covers it the same way every other embedded-JSON block on this page
	// already works — a single {{jsonify .UIState}} blob keyed the same way
	// as the table, so a new subgrid never needs a new script tag added here.
	uiStateRaw, err := charstore.GetSheetUIState(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load sheet ui state:", err)
		return
	}
	uiState := map[string]any{}
	for key, raw := range uiStateRaw {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err == nil {
			uiState[key] = v
		}
	}

	pendingASI, err := s.buildPendingASIRows(id, sheet.PendingASISlots, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending ASI rows:", err)
		return
	}

	pendingFeatureChoices, err := s.buildPendingFeatureChoiceRows(sheet.PendingFeatureChoices)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending feature choice rows:", err)
		return
	}

	pendingFeatAbilityChoices, err := s.buildPendingFeatAbilityChoiceRows(id, characterFeats)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending feat ability choice rows:", err)
		return
	}

	pendingFeatSkillOrToolChoices, err := s.buildPendingFeatSkillOrToolChoiceRows(id, characterFeats)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending feat skill-or-tool choice rows:", err)
		return
	}

	pendingHunterPatternChoices, err := s.buildPendingHunterPatternChoiceRows(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending hunter pattern choice rows:", err)
		return
	}

	s.render(w, "character_sheet.html", map[string]any{
		"Title": sheet.Name, "ID": id, "Sheet": sheet,
		"AbilityOrder": charsheet.Abilities, // Sheet.Abilities is a map — fixed display order comes from here
		"SkillGroups":  groupSkillsByAbility(sheet.Skills),
		"LevelOptions": levelOptions, // sheet_level_row's per-class level <select>s
		"Inventory":    inventory, "Bulk": bulk, "Jutsu": jutsu, "Attacks": attacks,
		"Concentration":       concentration,
		"CustomWeaponAttacks": customWeaponRows, "CustomJutsuAttacks": customJutsuRows, "CustomItemAttacks": customItemRows,
		"Feats":       characterFeats,
		"ItemLibrary": itemLibrary, "FeatLibrary": featLibrary,
		"JutsuLibrary": groupJutsu(jutsuLibrary), "JutsuSourceTiles": jutsuSourceTiles,
		"OwnedJutsu": ownedJutsu, "JutsuKnownCap": jutsuKnownCap, "JutsuOrigins": jutsuOrigins,
		"EligibleJutsu": eligibleJutsu, "ElementalAffinities": elementalAffinities, "ElementalAffinitySlots": elementalAffinitySlotsData,
		"UIState": uiState,
		"Drive":   drive, "Goal": goal, "Fear": fear,
		"PassiveTraits": passiveTraits, "CustomResources": customResources, "PuppetTactics": puppetTactics,
		"MartialDice": martialDice, "MartialTechniques": martialTechniques, "WeaponFocus": weaponFocus,
		"WeaponForm":                    weaponForm,
		"MartialDefense":                martialDefense,
		"HunterTechniques":              hunterTechniques,
		"CookingNin":                    cookingNin,
		"Genjutsu":                      genjutsu,
		"MedicalNin":                    medicalNin,
		"ScoutNin":                      scoutNin,
		"IntelligenceOperative":         intelligenceOperative,
		"NinjutsuSpecialist":            ninjutsuSpecialist,
		"ScienceNin":                    scienceNin,
		"Mastery":                       mastery,
		"PendingFeatureChoices":         pendingFeatureChoices,
		"PendingASI":                    pendingASI,
		"PendingFeatAbilityChoices":     pendingFeatAbilityChoices,
		"PendingFeatSkillOrToolChoices": pendingFeatSkillOrToolChoices,
		"PendingHunterPatternChoices":   pendingHunterPatternChoices,
		"ChatLog":                       chatLog,
		"ToolProficiencies":             toolRows, "Languages": languages, "CustomSkills": customSkillRows,
		"AllTools": allTools, "AllLanguages": allLanguages,
		"ClassSummary":     classSummary,
		"EquipmentOptions": equipmentOptions,
		"Companions":       companions,
		"PuppetsTab":       puppetsTab,
		"SummonsTab":       summonsTab,
	})
}

type skillGroupRow struct {
	Ability string
	Skills  []charsheet.SkillEntry
}

// groupSkillsByAbility folds Sheet.Skills (already sorted by ability then
// name, per Compute's own doc) into one group per ability — a single
// linear pass, not a re-sort, same "rely on already-sorted input" pattern
// jutsu.go's groupJutsu uses.
func groupSkillsByAbility(skills []charsheet.SkillEntry) []skillGroupRow {
	var groups []skillGroupRow
	for _, sk := range skills {
		if n := len(groups); n == 0 || groups[n-1].Ability != sk.Ability {
			groups = append(groups, skillGroupRow{Ability: sk.Ability})
		}
		g := &groups[len(groups)-1]
		g.Skills = append(g.Skills, sk)
	}
	return groups
}

// loadCharacterInventory joins to equipment where a real item_slug is set
// (for the /items/{slug} link) and falls back to custom_name otherwise —
// same either-or shape character_inventory's own CHECK constraint enforces.
func (s *server) loadCharacterInventory(characterID int64) ([]inventoryRow, error) {
	// Every row carries a real item_slug now — a custom row's slug points
	// into the local custom_items library instead of rules.db (see
	// lookupCarriedItem), rather than storing its name/kind/bulk/description
	// inline the way a pre-migration row used to (0013_custom_items.sql
	// backfilled every old inline row into the library and pointed it at a
	// slug too).
	rows, err := s.charDB.Query(`
		SELECT id, item_slug, quantity, equipped
		FROM character_inventory WHERE character_id = ? ORDER BY id`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []inventoryRow
	for rows.Next() {
		var id int64
		var itemSlug sql.NullString
		var quantity int
		var equipped bool
		if err := rows.Scan(&id, &itemSlug, &quantity, &equipped); err != nil {
			return nil, err
		}
		row := inventoryRow{ID: id, Quantity: quantity, Equipped: equipped, Slug: itemSlug.String}
		if err := s.lookupCarriedItem(&row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// carriedItemKinds routes an inventory slug to the rules table that
// describes it.
//
// character_inventory holds three sorts of thing now: ordinary equipment,
// and — since the sheet's Inventory tab merged the Items, Traps and Poisons
// libraries into one "things you carry" pane — poisons and trap templates
// too. Those live in their own tables with their
// own columns, not in `equipment`, so a single query cannot serve all three.
// The slug's own prefix says which table to ask, and the prefixes are
// generated by the ingest, so they are reliable.
var carriedItemKinds = []struct {
	prefix string
	kind   string
	query  string
}{
	{"poison/", "Poison", `SELECT name, bulk FROM poisons WHERE slug = ?`},
	// Traps have no bulk column at all — they are built in place, not
	// carried — so the query supplies a NULL to keep one scan shape.
	{"trap/", "Trap", `SELECT name, NULL FROM trap_templates WHERE slug = ?`},
}

// lookupCarriedItem fills in an inventory row's name, kind and bulk from
// whichever rules table its slug belongs to. A slug that resolves nowhere
// (a stale reference after a rules update) keeps the slug as its name rather
// than vanishing, the same graceful degrade this function has always had.
func (s *server) lookupCarriedItem(row *inventoryRow) error {
	// A custom/ slug points at the local library (custom_items in
	// characters.db, see 0013_custom_items.sql), never at rules.db — the
	// same "slug prefix picks the table" dispatch carriedItemKinds does
	// below, just against a different database.
	if strings.HasPrefix(row.Slug, "custom/") {
		item, err := charstore.GetCustomItemBySlug(s.charDB, row.Slug)
		if err == sql.ErrNoRows {
			row.Name = row.Slug
			return nil
		}
		if err != nil {
			return err
		}
		row.Name, row.Kind, row.Bulk = item.Name, item.Kind, item.Bulk
		return nil
	}
	for _, source := range carriedItemKinds {
		if !strings.HasPrefix(row.Slug, source.prefix) {
			continue
		}
		err := s.rulesDB.QueryRow(source.query, row.Slug).Scan(&row.Name, &row.Bulk)
		if err == sql.ErrNoRows {
			row.Name = row.Slug
			return nil
		}
		if err != nil {
			return err
		}
		row.Kind = source.kind
		return nil
	}
	var description sql.NullString
	if err := s.rulesDB.QueryRow(
		`SELECT name, kind, bulk, description FROM equipment WHERE slug = ?`, row.Slug,
	).Scan(&row.Name, &row.Kind, &row.Bulk, &description); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		row.Name = row.Slug
		return nil
	}
	// Read straight off the description rather than from a list of pack
	// slugs, so a rules update that adds a sixth pack gets an Unpack button
	// with no code change. parsePackContents applies the same test.
	row.Unpackable = strings.HasPrefix(strings.TrimSpace(description.String), packContentsPrefix)
	return nil
}

// loadCharacterJutsuSheet links each known jutsu out to its existing
// /jutsu/{slug} detail page rather than embedding another two-pane
// preview on a page that isn't itself a browse view.
//
// sheet supplies the character's attack modifiers — Ninjutsu, Genjutsu,
// Taijutsu and Bukijutsu — so a jutsu whose description calls for an attack
// roll can be rolled straight from the sheet at the right bonus, rather than
// leaving the player to work out which one applies and add it up.
func (s *server) loadCharacterJutsuSheet(characterID int64, sheet *charsheet.Sheet) ([]jutsuSheetRow, error) {
	attackBonus := map[string]int{}
	attackAbility := map[string]string{}
	for _, a := range sheet.JutsuAttacks {
		attackBonus[a.Kind] = a.Modifier
		attackAbility[a.Kind] = a.Ability
	}

	// Martial Technique (1st level): a Taijutsu Specialist's Taijutsu jutsu
	// always cost a fixed amount by rank instead of their printed cost —
	// see taijutsuMartialTechniqueFlatCost.
	taijutsuSpecialistLevel, err := s.taijutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	// Critical Focus (Weapon Specialist, 7th/11th/17th level): widens the
	// crit range on Bukijutsu-classified jutsu the same way it already does
	// for weapon attacks (buildAttacks) — applied per-row below, gated on
	// classification rather than unconditionally, since it only reaches
	// Bukijutsu jutsu-casting.
	critRangeThreshold, err := s.weaponSpecialistCritRangeThreshold(characterID)
	if err != nil {
		return nil, err
	}
	// Patissier Chef's Gotta Do the Cooking By the Book (5th level): "may
	// use Charisma as your Jutsu Modifier for" the curated Medical-release
	// healing/temp-HP subset (patissierChefByTheBookHealingJutsuSlugs) —
	// applied per-row below, alongside the same feature's jutsu-access
	// grant (jutsuEligible/jutsuEligibilityContext).
	byTheBookFeatures, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	hasByTheBook := hasFeature(byTheBookFeatures, patissierChefByTheBookFeatureSlug)
	options, err := charstore.ListJutsuOptions(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	// Malleable Mirages (Genjutsu Specialist): a picked Mirage can grant a
	// specific jutsu the character may not otherwise know — see
	// genjutsuMirageJutsuGrants' own doc comment for which of the 79
	// Mirages this covers and why the rest are excluded. poolResources is
	// only needed for the LIMITED grants (genjutsuGrantFreeLimited/
	// genjutsuGrantHalfCostLimited), to read each Mirage's own current/max
	// use count off the same rest-scoped pool already tracked on the
	// sheet's Resources list.
	mirageGrants, err := s.genjutsuMirageJutsuGrantsForCharacter(characterID)
	if err != nil {
		return nil, err
	}
	// Genjutsu Pledges' own unconditional free-jutsu base features (Inspired
	// Appearance, Shaping Your World): same genjutsuGrantFreeUnlimited
	// CostChakra-override shape as the three unconditional Malleable Mirage
	// grants above, just always-on rather than gated behind a Mirage pick —
	// see genjutsuPledgeJutsuGrants' own doc comment (genjutsu.go). Merged
	// directly into mirageGrants so both sources feed the same
	// CostChakra-override block below uniformly; no slug collides between
	// the two maps (no Mirage grants Transform or Minor Illusion).
	pledgeGrants, err := s.genjutsuPledgeJutsuGrantsForCharacter(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	for slug, grant := range pledgeGrants {
		mirageGrants[slug] = grant
	}
	// Wolves Legacy's Wolf Techniques (Hunter-Nin): a picked Wolf Technique
	// can be cast by spending Prosthetic Attachment uses instead of Chakra —
	// same "read the pool's own current/max off the Resources list" need as
	// the Mirage grants above, see hunterNinJutsuGrantsForCharacter's own
	// doc comment (hunter_nin.go).
	hunterGrants, err := s.hunterNinJutsuGrantsForCharacter(characterID)
	if err != nil {
		return nil, err
	}
	// Necrotic Hand's Dr. Death (Hunter-Nin, 17th level): an unconditional
	// rank-threshold override on Necrosis's own Chakra cost, not a spendable
	// pool — see hunterNinRankCeilingGrantsForCharacter's own doc comment
	// (hunter_nin.go) for why this needs no customResourceGrants entry at
	// all, unlike the two grant maps above.
	rankCeilingGrants, err := s.hunterNinRankCeilingGrantsForCharacter(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	// Interrogationist's Unerring Eye/Perfect Mind (Intelligence Operative):
	// spend a Brave Order to cast a specific named jutsu instead of Chakra —
	// same "read the pool's own current/max off the Resources list" need as
	// the Mirage/Wolf Technique grants above, see
	// intelligenceOperativeJutsuGrantsForCharacter's own doc comment
	// (intelligence_operative.go).
	ioGrants, err := s.intelligenceOperativeJutsuGrantsForCharacter(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	// Mech Crafter's Adaptive Movement (Science-Nin): Body Flicker/Chakra
	// Leaping can be cast paying their own full printed Chakra cost out of
	// CCD instead of the normal Chakra pool — same "read the pool's own
	// current/max off the Resources list" need as the grants above, see
	// scienceNinAdaptiveMovementJutsuGrantsForCharacter's own doc comment
	// (science_nin.go).
	amGrants, err := s.scienceNinAdaptiveMovementJutsuGrantsForCharacter(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	var poolResources []CustomResourceEntry
	if len(mirageGrants) > 0 || len(hunterGrants) > 0 || len(ioGrants) > 0 || len(amGrants) > 0 {
		poolResources, err = s.loadCustomResources(characterID, sheet)
		if err != nil {
			return nil, err
		}
	}
	rows, err := s.charDB.Query(`
		SELECT jutsu_slug FROM character_jutsu
		WHERE character_id = ? AND jutsu_slug IS NOT NULL`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	grantLabels, err := s.loadGrantedJutsuLabels(characterID, sheet)
	if err != nil {
		return nil, err
	}
	// loadGrantedJutsuLabels returns a nil map for a character with no
	// granted features at all (e.g. a class with no class_features rows of
	// its own) — every block below writes into grantLabels regardless of
	// whether loadGrantedJutsuLabels itself found anything to seed it with,
	// so it must be non-nil before any of them run.
	if grantLabels == nil {
		grantLabels = map[string]string{}
	}
	if pmLevel, err := s.puppetMasterClassLevel(characterID); err != nil {
		return nil, err
	} else if pmLevel > 0 {
		upgradeGrants, err := s.puppetUpgradeTableGrantedJutsu(characterID, pmLevel)
		if err != nil {
			return nil, err
		}
		for slug, label := range upgradeGrants {
			if _, exists := grantLabels[slug]; !exists {
				grantLabels[slug] = label
			}
		}
	}
	if scoutNinLevel, err := s.scoutNinClassLevel(characterID); err != nil {
		return nil, err
	} else if scoutNinLevel > 0 {
		mobileSavantGrants, err := s.scoutNinMobileSavantGrantedJutsu(characterID)
		if err != nil {
			return nil, err
		}
		for slug, label := range mobileSavantGrants {
			if _, exists := grantLabels[slug]; !exists {
				grantLabels[slug] = label
			}
		}
	}
	if medicalNinLevel, err := s.medicalNinClassLevel(characterID); err != nil {
		return nil, err
	} else if medicalNinLevel > 0 {
		chartGrants, err := s.medicalNinJutsuChartGrantedJutsu(characterID, medicalNinLevel)
		if err != nil {
			return nil, err
		}
		for slug, label := range chartGrants {
			if _, exists := grantLabels[slug]; !exists {
				grantLabels[slug] = label
			}
		}
	}
	if hunterNinLevel, err := s.hunterNinClassLevel(characterID); err != nil {
		return nil, err
	} else if hunterNinLevel > 0 {
		// Arsenal Item (Arsenalist) and Wolf Technique (Wolves Legacy) both
		// grant a specific named jutsu as a side effect of a catalog pick —
		// same "compute from the picks table, merge into the label map"
		// shape as Puppet Upgrade/Mobile Savant/Medical Doctrine above, see
		// hunterNinArsenalItemGrantedJutsu/hunterNinWolfTechniqueGrantedJutsu's
		// own doc comments (hunter_nin.go).
		arsenalGrants, err := s.hunterNinArsenalItemGrantedJutsu(characterID)
		if err != nil {
			return nil, err
		}
		for slug, label := range arsenalGrants {
			if _, exists := grantLabels[slug]; !exists {
				grantLabels[slug] = label
			}
		}
		wolfGrants, err := s.hunterNinWolfTechniqueGrantedJutsu(characterID)
		if err != nil {
			return nil, err
		}
		for slug, label := range wolfGrants {
			if _, exists := grantLabels[slug]; !exists {
				grantLabels[slug] = label
			}
		}
	}
	known := map[string]bool{}
	for _, slug := range slugs {
		known[slug] = true
	}
	for slug := range grantLabels {
		if !known[slug] {
			slugs = append(slugs, slug)
		}
	}

	var out []jutsuSheetRow
	for _, slug := range slugs {
		var j jutsuSheetRow
		j.Slug = slug
		j.SourceLabel = grantLabels[slug]
		var rank sql.NullString
		var description, duration, classification string
		var costChakra, chakraPerRank sql.NullInt64
		if err := s.rulesDB.QueryRow(
			`SELECT name, rank, cost_text, cost_chakra, description, chakra_per_rank, duration, classification FROM v_jutsu WHERE slug = ?`, slug,
		).Scan(&j.Name, &rank, &j.CostText, &costChakra, &description, &chakraPerRank, &duration, &classification); err != nil {
			continue // stale slug (rules update) — skip rather than break the whole sheet
		}
		j.Rank = rank.String
		j.IsConcentration = isConcentrationDuration(duration)
		if costChakra.Valid {
			v := int(costChakra.Int64)
			j.CostChakra = &v
		}
		// Martial Technique's flat cost overrides the printed cost_chakra
		// outright for a Taijutsu Specialist's own Taijutsu jutsu — "Special"
		// cost jutsu (costChakra.Valid == false) are excluded, matching the
		// RAW carve-out.
		effectiveBaseCost := costChakra
		if taijutsuSpecialistLevel > 0 && classification == "Taijutsu" && costChakra.Valid {
			if flat, ok := taijutsuMartialTechniqueFlatCost[j.Rank]; ok {
				v := flat
				j.CostChakra = &v
				effectiveBaseCost = sql.NullInt64{Int64: int64(flat), Valid: true}
			}
		}
		// A Malleable Mirage's own jutsu grant sits at the same precedence
		// spot as Martial Technique above — after the printed cost, before
		// the player's own manual override. genjutsuGrantFreeUnlimited
		// applies permanently, the same way Martial Technique's flat cost
		// does, since the book prints no rest-scoped limit for it to
		// respect. The limited modes deliberately do NOT touch CostChakra/
		// effectiveBaseCost here — j.FreeCast surfaces them as a separate
		// "Cast via <Mirage>" button instead (see character_sheet.html),
		// so the player keeps the choice between spending a scarce Mirage
		// use and casting normally.
		if grant, ok := mirageGrants[slug]; ok {
			switch grant.Mode {
			case genjutsuGrantFreeUnlimited:
				v := 0
				j.CostChakra = &v
				effectiveBaseCost = sql.NullInt64{Int64: 0, Valid: true}
			case genjutsuGrantFreeLimited, genjutsuGrantHalfCostLimited:
				if def, ok := customResourceGrants["genjutsu-mirage/"+grant.ResourceSuffix]; ok {
					for _, entry := range poolResources {
						if entry.Key != def.Key {
							continue
						}
						cost := 0
						if grant.Mode == genjutsuGrantHalfCostLimited && costChakra.Valid {
							cost = int(costChakra.Int64) / 2
						}
						j.FreeCast = &jutsuFreeCastGrant{
							MirageName:  def.Name,
							ResourceKey: def.Key,
							Cost:        cost,
							Uses:        entry.Current,
							Max:         entry.Max,
							UsesPerCast: 1,
						}
						break
					}
				}
			}
		}
		// Wolves Legacy's Wolf Techniques (Hunter-Nin): the alternate-cost
		// clause spends Prosthetic Attachment uses instead of Chakra, so this
		// sits at the same "separate Cast-via-<Pool> button, don't touch
		// CostChakra" precedence spot as the Mirage limited grants above —
		// the player keeps the choice between spending Prosthetic Attachment
		// uses and casting normally. A jutsu can only ever carry one of
		// mirageGrants/hunterGrants (no jutsu is both a Malleable Mirage
		// grant and a Wolf Technique pick), so overwriting j.FreeCast here
		// rather than checking it's still nil is safe.
		//
		// Read straight off poolResources' own Key/Name rather than through
		// customResourceGrants, unlike the Mirage branch above:
		// customResourceGrants is keyed by the GRANTING FEATURE's slug, not
		// by its own Key field's value, and grant.ResourceKey here already
		// IS that Key value ("prosthetic_attachments") — genjutsu.go's own
		// synthetic "genjutsu-mirage/<suffix>" slugs happen to equal their
		// matching customResourceGrants map key directly, which is what lets
		// the Mirage branch index the map by ResourceSuffix in the first
		// place; Hunter-Nin's grant has no such synthetic-slug-as-map-key
		// relationship to lean on.
		if grant, ok := hunterGrants[slug]; ok {
			for _, entry := range poolResources {
				if entry.Key != grant.ResourceKey {
					continue
				}
				j.FreeCast = &jutsuFreeCastGrant{
					MirageName:  entry.Name,
					ResourceKey: entry.Key,
					Cost:        grant.Cost,
					Uses:        entry.Current,
					Max:         entry.Max,
					UsesPerCast: grant.UsesPerCast,
					CastRank:    grant.CastRank,
				}
				break
			}
		}
		// Interrogationist's Unerring Eye/Perfect Mind: same alternate-cost
		// shape as the Wolf Techniques branch above — spend a Brave Order
		// instead of Chakra. A jutsu can only ever carry one of
		// mirageGrants/hunterGrants/ioGrants (no jutsu overlaps two of these
		// three classes' own grant sources), so overwriting j.FreeCast here
		// rather than checking it's still nil is safe.
		if grant, ok := ioGrants[slug]; ok {
			for _, entry := range poolResources {
				if entry.Key != grant.ResourceKey {
					continue
				}
				j.FreeCast = &jutsuFreeCastGrant{
					MirageName:  entry.Name,
					ResourceKey: entry.Key,
					Cost:        grant.Cost,
					Uses:        entry.Current,
					Max:         entry.Max,
					UsesPerCast: grant.UsesPerCast,
				}
				break
			}
		}
		// Mech Crafter's Adaptive Movement (Science-Nin): Body Flicker/Chakra
		// Leaping can be cast paying their own full printed Chakra cost out of
		// CCD instead of Chakra — same alternate-cost shape as the three
		// branches above, except the pool spent (CCD) is itself measured in
		// Chakra, so UsesPerCast (the amount decremented from it) is the
		// jutsu's own printed cost_chakra rather than a fixed per-cast use
		// count, and Cost (paid from the normal Chakra pool alongside the
		// pool spend) is always 0 — the full cost comes out of CCD instead,
		// nothing is paid twice. A jutsu can only ever carry one of
		// mirageGrants/hunterGrants/ioGrants/amGrants (no jutsu overlaps two
		// of these four classes' own grant sources), so overwriting
		// j.FreeCast here rather than checking it's still nil is safe.
		if amGrants[slug] && costChakra.Valid {
			for _, entry := range poolResources {
				if entry.Key != scienceNinAdaptiveMovementResourceKey {
					continue
				}
				j.FreeCast = &jutsuFreeCastGrant{
					MirageName:  entry.Name,
					ResourceKey: entry.Key,
					Cost:        0,
					Uses:        entry.Current,
					Max:         entry.Max,
					UsesPerCast: int(costChakra.Int64),
				}
				break
			}
		}
		// A manual cost override (feats/clan/class features that let a jutsu
		// be cast for less — or more — than its printed cost) takes
		// precedence over even Martial Technique's automatic flat cost,
		// since it is the player's own explicit call from the modify-jutsu
		// box. Unlike Martial Technique, this IS allowed on a "Special"-cost
		// jutsu: a class feature that grants a fixed-cost cast of an
		// otherwise textual-cost jutsu needs exactly this to become
		// castable at all. This is a separate mechanic from upcasting
		// (jutsu_upcast_rules/buildUpcastOptions) — it moves the anchor
		// buildUpcastOptions starts from, the same way Martial Technique's
		// override already does above, but nothing here changes chakraPerRank.
		if opt, ok := options[slug]; ok && opt.CostChakraOverride != nil {
			v := *opt.CostChakraOverride
			j.CostChakra = &v
			j.CostOverride = opt.CostChakraOverride
			effectiveBaseCost = sql.NullInt64{Int64: int64(v), Valid: true}
		}
		// Only jutsu with BOTH a fixed base cost and a parsed per-rank delta
		// get upcast options — a jutsu with no cost_chakra already has no
		// Cast button at all (see character_sheet.html), and one whose
		// at_higher_ranks text didn't parse to a number falls back to the
		// sheet's existing manual chakra-edit path, same as it does today.
		// effectiveBaseCost (rather than the raw costChakra) anchors this so
		// Martial Technique's override also lands on every upcast rank, not
		// just the jutsu's base rank — "Upcasting still increases the cost
		// of the jutsu as listed in its text" keeps chakraPerRank (the per-
		// rank delta) unmodified, only the anchor moves.
		if effectiveBaseCost.Valid && chakraPerRank.Valid {
			j.UpcastOptions = buildUpcastOptions(j.Rank, int(effectiveBaseCost.Int64), int(chakraPerRank.Int64))
		}
		// Necrotic Hand's Dr. Death (Hunter-Nin, 17th level): "Casting
		// Necrosis at C-Rank or fewer costs 0 Chakra" — applied AFTER
		// buildUpcastOptions above computes the jutsu's real per-rank cost
		// table (from its own unmodified printed cost/per-rank delta, or
		// Martial Technique's/the player's own override if either applies —
		// this always runs last), zeroing only the ranks at or below the
		// threshold. Every rank above the threshold keeps the cost
		// buildUpcastOptions already computed, unlike a flat
		// genjutsuGrantFreeUnlimited-style override, which would zero the
		// anchor BEFORE buildUpcastOptions runs and undercharge every rank
		// above the threshold too.
		if ceiling, ok := rankCeilingGrants[slug]; ok {
			thresholdOrder := jutsuRankOrder[ceiling.MaxFreeRank]
			for i := range j.UpcastOptions {
				if jutsuRankOrder[j.UpcastOptions[i].Rank] <= thresholdOrder {
					j.UpcastOptions[i].Cost = 0
				}
			}
			if j.CostChakra != nil && jutsuRankOrder[j.Rank] <= thresholdOrder {
				v := 0
				j.CostChakra = &v
			}
		}

		// What the jutsu's own description implies, before any override.
		// resolveJutsuAttackKind reroutes Bukijutsu-classified jutsu onto the
		// Bukijutsu bucket even though their text uses the shared Taijutsu
		// wording — see that function's own doc comment.
		kind := resolveJutsuAttackKind(jutsuAttackKind(description), classification)
		j.AttackKind = kind
		j.AttackBonus = attackBonus[kind]
		j.AttackAbility = attackAbility[kind]
		j.AttackProf = charsheet.ProfFull
		// By the Book's Charisma substitution (see hasByTheBook's own doc
		// comment above). AttackAbility is set regardless of whether this
		// jutsu's text calls for an attack roll (kind == "" for 17 of the
		// 18 curated jutsu) because DamageAbility inherits it just below —
		// the only place this app tracks a jutsu's own healing dice is the
		// row's ✎ edit control, and its default ability should read
		// Charisma too instead of silently falling back to Intelligence.
		// AttackBonus is only recomputed when kind != "" (only Medical
		// Release: Vampiric Touch today) since attackBonus[kind] — built
		// off the class's baseline Ninjutsu ability — has nothing to
		// override when there is no attack roll to begin with.
		if hasByTheBook && patissierChefByTheBookHealingJutsuSlugs[slug] {
			j.AttackAbility = "cha"
			if kind != "" {
				j.AttackBonus = sheet.Abilities["cha"].Modifier + sheet.ProficiencyBonus + sheet.JackOfAllCombatBonus
			}
		}
		j.DamageAbility = j.AttackAbility
		j.Derived = true
		if strings.Contains(strings.ToLower(classification), "bukijutsu") {
			j.CritRangeThreshold = critRangeThreshold
		}

		if opt, ok := options[slug]; ok {
			j.Derived = false
			// An override gives a jutsu a to-hit even when its text never asked
			// for one — a homebrew or higher-rank casting that does attack. The
			// column still reads "—" only when there is genuinely nothing set.
			if opt.AttackAbility != "" {
				j.AttackAbility = opt.AttackAbility
				if j.AttackKind == "" {
					j.AttackKind = "Custom"
				}
			}
			if opt.AttackProf != "" {
				j.AttackProf = opt.AttackProf
			}
			j.AttackFlat = opt.AttackBonus
			if j.AttackKind != "" {
				// This recompute discards attackBonus[kind] (and with it
				// Weapon Focus's Bukijutsu bonus, folded into JutsuAttacks'
				// own Modifier upstream) for ANY jutsu with an option row at
				// all, not just one with a custom attack ability — a
				// pre-existing gap this change doesn't widen. Combat's own
				// bonus is re-added explicitly just below instead of being
				// left to the same fate, since it's reachable here for
				// every jutsu this loop's DamageBonus branch also reaches.
				j.AttackBonus = charsheet.ComposeModifier(
					sheet.Abilities[j.AttackAbility].Modifier, sheet.ProficiencyBonus,
					j.AttackProf, opt.AttackBonus)
			}
			if opt.DamageAbility != "" {
				j.DamageAbility = opt.DamageAbility
			}
			j.DamageFlat = opt.DamageBonus
			j.DamageCount, j.DamageSides = opt.DamageCount, opt.DamageSides
			j.DamageType = opt.DamageType
			if j.DamageCount > 0 && j.DamageSides > 0 {
				j.DamageDice = strconv.Itoa(j.DamageCount) + "d" + strconv.Itoa(j.DamageSides)
				j.DamageBonus = sheet.Abilities[j.DamageAbility].Modifier + opt.DamageBonus
			}
			// Combat's own "+1/+2 bonus to attack & damage rolls made with
			// Ninjutsu, Taijutsu, Genjutsu and Bukijutsu you cast" — applied
			// to both halves here since jutsu damage is otherwise never
			// computed at all (see Sheet.JackOfAllCombatBonus's own doc
			// comment), and this override block always recomputes
			// AttackBonus from scratch above regardless of what
			// attackBonus[kind] already carried.
			switch kind {
			case "Ninjutsu", "Genjutsu", "Taijutsu", "Bukijutsu":
				if j.AttackKind != "" {
					j.AttackBonus += sheet.JackOfAllCombatBonus
				}
				if j.DamageCount > 0 && j.DamageSides > 0 {
					j.DamageBonus += sheet.JackOfAllCombatBonus
				}
			}
		}
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}

// concentrationView is the sheet's display shape for the character's one
// active concentration slot (see character_concentration/charstore's
// StartConcentration). ChakraControlMod is the same modifier already shown
// on the Chakra Control skill row — a Concentration Check is that same
// roll, not a separate formula.
type concentrationView struct {
	JutsuName        string
	Rank             string
	ChakraControlMod int
}

// loadConcentrationView returns the character's active concentration for
// display, or nil when nothing is active. Shared by handleCharacterSheet and
// renderSheetFragment's "sheet_vitals" case, same as loadCharacterJutsuSheet
// is shared between them.
func (s *server) loadConcentrationView(characterID int64, sheet *charsheet.Sheet) (*concentrationView, error) {
	slug, rank, ok, err := charstore.GetConcentration(s.charDB, characterID)
	if err != nil || !ok {
		return nil, err
	}
	var name string
	if err := s.rulesDB.QueryRow(`SELECT name FROM v_jutsu WHERE slug = ?`, slug).Scan(&name); err != nil {
		name = slug // stale slug (rules update) — show something rather than break
	}
	mod, _ := charsheet.SkillModifier(sheet, "Chakra Control")
	return &concentrationView{JutsuName: name, Rank: rank, ChakraControlMod: mod}, nil
}

func (s *server) loadCharacterAmbitions(characterID int64) (drive, goal, fear string, err error) {
	rows, err := s.charDB.Query(`SELECT kind, text FROM character_ambitions WHERE character_id = ?`, characterID)
	if err != nil {
		return "", "", "", err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var kind, text string
		if err := rows.Scan(&kind, &text); err != nil {
			return "", "", "", err
		}
		values[kind] = text
	}
	return values["drive"], values["goal"], values["fear"], rows.Err()
}

// loadCharacterProficiencyValues returns the distinct values granted for
// one character_proficiencies kind — backs the sheet's Tool Proficiencies
// and Languages panels, sorted for a stable display order regardless of
// insertion order (class grants, background grants, and manual "+"
// additions can arrive in any sequence).
func (s *server) loadCharacterProficiencyValues(characterID int64, kind string) ([]string, error) {
	rows, err := s.charDB.Query(
		`SELECT DISTINCT value FROM character_proficiencies WHERE character_id = ? AND kind = ? ORDER BY value`,
		characterID, kind,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// loadCustomSkills returns the character's skill proficiencies that are
// not one of the rules' own skills.
//
// A player-invented skill ("Calligraphy", a homebrew specialty) is stored
// as an ordinary character_proficiencies row with kind='skill', which is
// the right place for it, but charsheet.Sheet.Skills is built by walking
// the fixed SkillAbility map — so a name the rules don't know would be
// written and then never displayed anywhere. This is what puts those rows
// back on the sheet, in the "Tool Proficiencies & Custom Skills" panel
// beside the tools, exactly where the reference sheet has them.
func (s *server) loadCustomSkills(characterID int64) ([]string, error) {
	values, err := s.loadCharacterProficiencyValues(characterID, "skill")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, value := range values {
		if _, known := charsheet.SkillAbility[value]; !known {
			out = append(out, value)
		}
	}
	return out, nil
}

// profRow is one line in the reworked Tool Proficiencies & Custom Skills box
// (part 9): a proficiency's name plus the ability/proficiency-share/flat-
// bonus tweak stored in character_proficiency_mods, composed into one
// rollable Modifier the same way Initiative composes its own total
// (charsheet.ComposeModifier). Kind says which endpoint an edit posts to.
type profRow struct {
	Kind     string
	Name     string
	Ability  string // "" = no ability term
	ProfMode string
	Bonus    int
	Modifier int
	// MasteryRank is this row's own Mastery rank (0 = none), already folded
	// into Modifier. Mastery is granted "with a given skill or toolkit", so
	// a toolkit's own d20 roll gets it just like a skill's does — it is
	// shown here as well as added because a bonus that only appears inside
	// a composed total is indistinguishable from a hand-typed one.
	MasteryRank int
}

// loadProficiencyMods returns every character_proficiency_mods row for one
// kind ("tool" or "skill"), keyed by the proficiency's displayed name — the
// join key 0011_proficiency_mods.sql's own comment explains.
func (s *server) loadProficiencyMods(characterID int64, kind string) (map[string]profRow, error) {
	rows, err := s.charDB.Query(
		`SELECT value, ability, prof_mode, bonus FROM character_proficiency_mods
		 WHERE character_id = ? AND kind = ?`, characterID, kind,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]profRow{}
	for rows.Next() {
		var r profRow
		if err := rows.Scan(&r.Name, &r.Ability, &r.ProfMode, &r.Bonus); err != nil {
			return nil, err
		}
		out[r.Name] = r
	}
	return out, rows.Err()
}

// buildProficiencyRows composes the display list for one T&CS section: every
// name gets its stored tweak if it has one, or the neutral default (no
// ability, no proficiency share, no bonus — Modifier 0) if it has never been
// adjusted. A stale/unrecognised stored ability falls back to "no ability
// term" rather than computing off a zero score, the same rule Initiative's
// own override reading follows.
//
// Mastery is added on top of the composed total rather than being folded
// into the player-editable flat Bonus: the bonus field is the player's own
// hand-tuned number, and a rule-derived term written into it would be
// overwritten the next time they adjust the row (and would double up the
// moment their rank changed).
func buildProficiencyRows(names []string, kind string, mods map[string]profRow, sheet *charsheet.Sheet) []profRow {
	out := make([]profRow, 0, len(names))
	for _, name := range names {
		row := profRow{Kind: kind, Name: name, ProfMode: charsheet.ProfNone}
		if m, ok := mods[name]; ok {
			row.Ability, row.ProfMode, row.Bonus = m.Ability, m.ProfMode, m.Bonus
		}
		abilityMod := 0
		if ab, ok := sheet.Abilities[row.Ability]; ok && row.Ability != "" {
			abilityMod = ab.Modifier
		}
		row.MasteryRank = sheet.MasteryRanks[name]
		row.Modifier = charsheet.ComposeModifier(abilityMod, sheet.ProficiencyBonus, row.ProfMode, row.Bonus) +
			charsheet.MasteryBonus(row.MasteryRank)
		out = append(out, row)
	}
	return out
}

// loadAllToolNames returns the game's tool/toolkit proficiency names for the
// T&CS box's "add a tool" dropdown — the base tier of each toolkit only.
// Greater/Superior/Supreme are rarity upgrades of the same physical kit
// (better bonus, same proficiency), not separate tool proficiencies, so
// listing all four tiers would offer the same tool four times.
func (s *server) loadAllToolNames() ([]string, error) {
	rows, err := s.rulesDB.Query(`
		SELECT name FROM equipment
		WHERE kind = 'toolkit'
		  AND name NOT LIKE 'Greater %' AND name NOT LIKE 'Superior %' AND name NOT LIKE 'Supreme %'
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// loadAllLanguageNames returns every language the rules actually grant, for
// the Languages box's dropdown. There is no dedicated languages table — the
// four special tongues (Insect-Speak, Dog-Speak, Snake-Speak, Machine-Speak)
// each live in one clan's extra_language column, parsed the same way
// charstore.SetClan folds them into a new character's proficiencies.
func (s *server) loadAllLanguageNames() ([]string, error) {
	rows, err := s.rulesDB.Query(
		`SELECT extra_language FROM clans WHERE extra_language IS NOT NULL AND extra_language != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		if name := charstore.ClanLanguageName(text); name != "" {
			out = append(out, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// subclassPickOption is one choice in a classSummaryRow's subclass picker.
type subclassPickOption struct {
	Slug string
	Name string
}

// classSummaryRow is one line of the header's class/subclass summary — one
// per character_classes row, in creation order, each with its own subclass
// name if one has been picked for that specific class. Multiclassing means
// more than one of these can exist and each can have a different subclass,
// which is why Subclass is looked up per class rather than once for the
// character.
type classSummaryRow struct {
	ClassSlug string
	ClassName string
	Levels    int
	Subclass  string // "" if no subclass chosen for this class yet

	// SubclassSlug/SubclassOptions back the inline subclass picker: empty
	// SubclassOptions means this class has no subclass group at all
	// (nothing to pick), non-empty means the player can choose or change
	// their pick right here rather than needing a dedicated page — there
	// has never been anywhere else in the app a subclass could be chosen,
	// not even during character creation.
	SubclassSlug    string
	SubclassOptions []subclassPickOption
}

// loadClassSummary backs the header's "under the name" class/subclass line.
// subclass_groups.class_slug is what ties a chosen subclass back to the
// class it belongs to — character_subclasses itself only stores the
// subclass slug, not which class it modifies.
func (s *server) loadClassSummary(characterID int64) ([]classSummaryRow, error) {
	rows, err := s.charDB.Query(
		`SELECT class_slug, levels FROM character_classes WHERE character_id = ? ORDER BY order_index`, characterID)
	if err != nil {
		return nil, err
	}
	var classSlugs []string
	var levels []int
	for rows.Next() {
		var slug string
		var lvl int
		if err := rows.Scan(&slug, &lvl); err != nil {
			rows.Close()
			return nil, err
		}
		classSlugs = append(classSlugs, slug)
		levels = append(levels, lvl)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var slug string
		if err := subRows.Scan(&slug); err != nil {
			subRows.Close()
			return nil, err
		}
		subclassSlugs = append(subclassSlugs, slug)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return nil, err
	}

	// class_slug -> chosen subclass's display name/slug.
	subclassNameByClass := map[string]string{}
	subclassSlugByClass := map[string]string{}
	for _, slug := range subclassSlugs {
		var name, classSlug string
		if err := s.rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, slug,
		).Scan(&name, &classSlug); err != nil {
			continue // a stale/removed subclass slug just shows no subclass
		}
		subclassNameByClass[classSlug] = name
		subclassSlugByClass[classSlug] = slug
	}

	out := make([]classSummaryRow, 0, len(classSlugs))
	for i, slug := range classSlugs {
		row := classSummaryRow{
			ClassSlug: slug, Levels: levels[i],
			Subclass: subclassNameByClass[slug], SubclassSlug: subclassSlugByClass[slug],
		}
		if err := s.rulesDB.QueryRow(`SELECT name FROM classes WHERE slug = ?`, slug).Scan(&row.ClassName); err != nil {
			row.ClassName = slug
		}
		options, err := s.loadSubclassOptions(slug)
		if err != nil {
			return nil, err
		}
		row.SubclassOptions = options
		out = append(out, row)
	}
	return out, nil
}

// subclassGateSatisfied reports whether every class a character holds at
// 3rd level or higher, and that actually has a subclass group to pick from,
// has had a subclass chosen — the gate handleCreateFinish enforces so a
// character can't be locked in with an unresolved subclass pick. A class
// below 3rd level, or one with no subclass group at all (SubclassOptions
// empty), is never blocking.
func subclassGateSatisfied(classSummary []classSummaryRow) bool {
	return len(subclassGateFailures(classSummary)) == 0
}

// subclassGateFailures returns the classSummaryRow entries subclassGateSatisfied
// would reject, so the caller can name them in an error message.
func subclassGateFailures(classSummary []classSummaryRow) []classSummaryRow {
	var out []classSummaryRow
	for _, row := range classSummary {
		if row.Levels >= 3 && len(row.SubclassOptions) > 0 && row.SubclassSlug == "" {
			out = append(out, row)
		}
	}
	return out
}

// loadSubclassOptions returns every subclass in classSlug's one subclass
// group (see 0011_subclass_rename.sql — each class has exactly one), for
// the sheet's inline subclass picker. Empty, not an error, for a class with
// no subclass group at all.
func (s *server) loadSubclassOptions(classSlug string) ([]subclassPickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT sc.slug, sc.name FROM subclasses sc
		JOIN subclass_groups g ON g.slug = sc.group_slug
		WHERE g.class_slug = ?
		ORDER BY sc.name`, classSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []subclassPickOption
	for rows.Next() {
		var opt subclassPickOption
		if err := rows.Scan(&opt.Slug, &opt.Name); err != nil {
			return nil, err
		}
		out = append(out, opt)
	}
	return out, rows.Err()
}

// grantedFeatureRow is one auto-seeded, real class/clan feature shown
// alongside character_custom_features rows in the Core tab's features
// panel — read-only (editing these means changing class/clan/level, not
// this panel), same "Racial: X/1st Level" style source label the reference
// screenshots use.
//
// A type alias, not a new type: internal/features.GrantedFeatureRow is the
// same struct internal/charsheet.Compute now resolves numeric grants
// against (proficiency/AC/Speed/Mastery — see internal/features' own doc
// comment for why that logic moved out of this package). Every existing
// cmd/n5e consumer (passive_traits.go, jutsu_grants.go, the tests) keeps
// compiling and working unchanged against the local name.
type grantedFeatureRow = features.GrantedFeatureRow

// loadMergedGrantedFeatures is loadGrantedFeatures immediately folded
// through mergeFeatFeatures — the combination every consumer that needs the
// character's full active-feature list (passive traits, the Water and Oil
// bonus check) actually wants, so callers added after the Core tab's own
// inline version don't have to remember to chain the two themselves.
func (s *server) loadMergedGrantedFeatures(characterID int64, clanSlug string, classLevel int) ([]grantedFeatureRow, error) {
	granted, err := s.loadGrantedFeatures(characterID, clanSlug, classLevel)
	if err != nil {
		return nil, err
	}
	feats, err := s.loadCharacterFeats(characterID)
	if err != nil {
		return nil, err
	}
	return mergeFeatFeatures(granted, feats), nil
}

// loadGrantedFeatures assembles the character's real class + subclass + clan
// features out of rules.db, gated by the level each was actually gained at —
// one query per character_classes row (multiclassing means more than one
// class can each unlock its own features up to its own levels), one per
// chosen subclass (gated by its parent class's own level, resolved through
// subclass_groups.class_slug since character_subclasses only stores the
// subclass slug), plus one for the clan gated by the character's total class
// level (Sheet.Level, which is that total — there is no display-only level
// any more for the two to disagree about).
func (s *server) loadGrantedFeatures(characterID int64, clanSlug string, classLevel int) ([]grantedFeatureRow, error) {
	all, err := features.LoadGrantedFeatures(s.rulesDB, s.charDB, characterID, clanSlug, classLevel)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, f := range all {
		if !puppetMasterTacticSlugs[f.Slug] {
			out = append(out, f)
		}
	}
	// Mixed Studies (18th level, base Science-Nin): folds the picked
	// Inquiry's own 3rd Level feature rows in here — the single funnel
	// every cmd/n5e consumer of this function (science_nin_subclasses.go's
	// own has[slug] catalog gates, jutsu_grants.go, custom_resources.go,
	// loadMergedGrantedFeatures) ultimately reads through. See
	// mergeMixedStudiesFeatures's own doc (science_nin.go).
	return s.mergeMixedStudiesFeatures(characterID, out)
}

// puppetMasterTacticSlugs excludes Puppet Master's 5 named Tactics
// (Agile/Defensive/Helpful/Offensive/Resourceful) from the automatic
// granted-features list — rules.db has them tagged level 11 (their own
// 11th-level enhancement text is bundled into the same row as their base
// 2nd-level effect, with no separate row for the earlier level), so the
// plain level-gated list would otherwise show all 5 simultaneously, and
// only starting at level 11 — no player choice at all, and invisible
// between levels 2 and 10 despite "Tactics of the Craft" (kept in the
// list; it's just the explanatory umbrella feature) granting one at level
// 2. See puppets.go's own Tactics picker for where these are shown
// instead. "Tactics of the Craft" itself is not in this set.
var puppetMasterTacticSlugs = map[string]bool{
	"class/puppet-master/feature/puppet-master-tactics-agile-tactics": true,
	"class/puppet-master/feature/defensive-tactics":                   true,
	"class/puppet-master/feature/helpful-tactics-changed-new":         true,
	"class/puppet-master/feature/offensive-tactics":                   true,
	"class/puppet-master/feature/resourceful-tactics":                 true,
}

// mergeFeatFeatures folds the character's taken feats into the granted
// class/clan features and orders the whole list by the level each thing was
// gained at.
//
// A feat dragged onto the Feats tab is a feature of the character like any
// other, and reading the Core tab should show it without having to remember
// to check a second tab. The Feats tab stays the place to
// add and remove them — these rows are read-only, exactly like the class and
// clan ones they sit among.
//
// The sort is stable, so within one level the order the loaders produced
// survives: class features in the book's own sort_order, then clan traits,
// then feats. Always-on features (level IS NULL, so Level 0) sort to the top,
// which is where a "you always have this" line belongs.
func mergeFeatFeatures(granted []grantedFeatureRow, feats []characterFeat) []grantedFeatureRow {
	for _, feat := range feats {
		prefix := "Feat"
		if feat.Category != "" {
			prefix = "Feat: " + feat.Category
		}
		granted = append(granted, grantedFeatureRow{
			Slug:        feat.Slug,
			Name:        feat.Name,
			Description: feat.Description,
			SourceLabel: features.FeatureSourceLabel(prefix, feat.ChosenAtLevel),
			Level:       int(feat.ChosenAtLevel.Int64),
		})
	}
	sort.SliceStable(granted, func(i, k int) bool { return granted[i].Level < granted[k].Level })
	return granted
}

// loadCharacterCustomFeatures is a thin wrapper over charstore's own
// reader, kept here so handleCharacterSheet's data-loading block reads as
// one flat list of loadX calls like every other section of this file.
func (s *server) loadCharacterCustomFeatures(characterID int64) ([]charstore.CustomFeature, error) {
	return charstore.ListCustomFeatures(s.charDB, characterID)
}

// chatLogRow is one persisted chat/dice-log line, oldest first (reversed
// from the DESC-limited query so the template can render top-to-bottom
// without the JS needing to flip it before appending new rows at the
// bottom).
type chatLogRow struct {
	ID        int64
	Kind      string
	Text      string
	Crit      string
	CreatedAt string
}

// loadChatLog returns the most recent 300 rows (AppendChatLog's own trim
// bound), oldest first.
func (s *server) loadChatLog(characterID int64) ([]chatLogRow, error) {
	rows, err := s.charDB.Query(`
		SELECT id, kind, text, crit, created_at FROM (
			SELECT id, kind, text, crit, created_at FROM character_chat_log
			WHERE character_id = ? ORDER BY id DESC LIMIT 300
		) ORDER BY id ASC`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatLogRow
	for rows.Next() {
		var c chatLogRow
		if err := rows.Scan(&c.ID, &c.Kind, &c.Text, &c.Crit, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- Character sheet writes (post-creation, editable play state) -------
//
// Every handler below follows the create-flow's own pattern (parse form,
// call the matching charstore function, then respond) with one split:
// handleSheetHP/handleSheetBaseTempHP/handleSheetRest/handleSheetChat are
// called via fetch from sheet-hp.js/sheet-chat.js and return just the
// affected fragment so the page never reloads for a roll or a hit-point
// change; everything else is a plain form POST that redirects back to the
// sheet, same as the create flow's own handlers, since those fields don't
// need sub-second feedback.

// renderSheetFragment recomputes the sheet fresh and renders one named
// {{define}} block from character_sheet.html on its own — "sheet_vitals"
// (the HP/THP/Chakra/Hit-Dice block) after an HP edit, a Base Temp HP
// edit, or a rest, and "sheet_ryo" after a currency edit. Every such block
// takes the same {ID, Sheet} data, so the handlers that swap a live chunk
// of the page in place share this rather than each assembling their own
// partial view of the same numbers.
//
// "sheet_tools_skills" and "sheet_languages" are the exception: they need
// several more queries (proficiency lists, tweaks, the full tool/language
// catalogues) that every other fragment here has no use for, so those are
// loaded only when one of those two names is asked for rather than paid on
// every HP tick or dice roll.
func (s *server) renderSheetFragment(w http.ResponseWriter, characterID int64, name string) {
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for "+name+" fragment:", err)
		return
	}
	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render " + name + " fragment: no template registered")
		return
	}
	data := map[string]any{
		"ID":    characterID,
		"Sheet": sheet,
		// Only a couple of fragments read these (SkillGroups: sheet_skills;
		// AbilityOrder: sheet_attack_mods' ability pickers; LevelOptions:
		// sheet_level_row's per-class level <select>s), but every fragment
		// is rendered through here with the same data, so it costs one map
		// entry each to keep callers from having to know what they need.
		"SkillGroups":  groupSkillsByAbility(sheet.Skills),
		"AbilityOrder": charsheet.Abilities,
		"LevelOptions": levelOptions,
	}
	switch name {
	case "sheet_vitals":
		concentration, err := s.loadConcentrationView(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load concentration for fragment:", err)
			return
		}
		data["Concentration"] = concentration
		customResources, err := s.loadCustomResources(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load custom resources for fragment:", err)
			return
		}
		data["CustomResources"] = customResources
		martialDice, err := s.loadMartialDice(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load martial dice for fragment:", err)
			return
		}
		data["MartialDice"] = martialDice
	case "sheet_weapon_attacks":
		if err := s.ensureScienceNinAutoGrants(characterID, sheet); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("ensure science-nin auto grants for fragment:", err)
			return
		}
		inventory, err := s.loadCharacterInventory(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load inventory for fragment:", err)
			return
		}
		attacks, err := s.buildAttacks(characterID, inventory, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build attacks for fragment:", err)
			return
		}
		customWeaponAttacks, err := charstore.ListCustomAttacks(s.charDB, characterID, "weapon")
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load custom weapon attacks for fragment:", err)
			return
		}
		critRangeThreshold, err := s.weaponSpecialistCritRangeThreshold(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load weapon specialist crit range threshold for fragment:", err)
			return
		}
		data["Attacks"] = attacks
		data["CustomWeaponAttacks"] = composeCustomAttacks(customWeaponAttacks, sheet, critRangeThreshold)
	case "sheet_other_rollables":
		customItemAttacks, err := charstore.ListCustomAttacks(s.charDB, characterID, "item")
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load custom item attacks for fragment:", err)
			return
		}
		data["CustomItemAttacks"] = composeCustomAttacks(customItemAttacks, sheet, 0)
	case "sheet_inventory", "sheet_inventory_full":
		inventory, err := s.loadCharacterInventory(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load inventory for "+name+" fragment:", err)
			return
		}
		data["Inventory"] = inventory
		featureBulkBonus, err := s.puppetAlwaysPreparedBulkBonus(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load always prepared bulk bonus for "+name+" fragment:", err)
			return
		}
		chassisBulkBonus, err := s.puppetChassisPropertyBulkBonus(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load chassis property bulk bonus for "+name+" fragment:", err)
			return
		}
		featBulk, err := s.featBulkBonus(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load feat bulk bonus for "+name+" fragment:", err)
			return
		}
		backupPlanBonus, err := s.backupPlanBulkBonus(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load backup plan bulk bonus for "+name+" fragment:", err)
			return
		}
		data["Bulk"] = computeInventoryBulk(inventory, sheet, featureBulkBonus+chassisBulkBonus+featBulk+backupPlanBonus)
	case "sheet_elemental_affinities":
		grantedFeatures, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load granted features for elemental affinities fragment:", err)
			return
		}
		grantedFeatureSlugs := map[string]bool{}
		for _, f := range grantedFeatures {
			grantedFeatureSlugs[f.Slug] = true
		}
		hasFeat, err := s.characterHasFeat(characterID, natureReleaseFeatSlug)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("check nature release feat for fragment:", err)
			return
		}
		picks, err := charstore.ListElementalAffinities(s.charDB, characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load elemental affinity picks for fragment:", err)
			return
		}
		data["ElementalAffinities"] = resolveElementalAffinities(sheet.ClanSlug, hasFeat, grantedFeatureSlugs, picks)
		data["ElementalAffinitySlots"] = elementalAffinitySlots(sheet.ClanSlug, hasFeat, grantedFeatureSlugs, picks)
	case "sheet_puppet_tactics":
		level, err := s.puppetMasterClassLevel(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load puppet master level for tactics fragment:", err)
			return
		}
		tactics, err := s.loadPuppetTacticsTabData(characterID, level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load puppet tactics for fragment:", err)
			return
		}
		data["PuppetTactics"] = tactics
	case "sheet_martial_techniques":
		martialTechniques, err := s.loadMartialTechniquesTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load martial techniques for fragment:", err)
			return
		}
		data["MartialTechniques"] = martialTechniques
	case "sheet_weapon_focus":
		weaponFocus, err := s.loadWeaponFocusTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load weapon focus for fragment:", err)
			return
		}
		data["WeaponFocus"] = weaponFocus
	case "sheet_weapon_form":
		weaponForm, err := s.loadWeaponFormTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load weapon form for fragment:", err)
			return
		}
		data["WeaponForm"] = weaponForm
	case "sheet_martial_defense":
		martialDefense, err := s.loadMartialDefenseTabData(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load martial defense for fragment:", err)
			return
		}
		data["MartialDefense"] = martialDefense
	case "sheet_hunter_techniques":
		hunterTechniques, err := s.loadHunterTechniquesTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter techniques for fragment:", err)
			return
		}
		data["HunterTechniques"] = hunterTechniques
	case "sheet_cooking_nin":
		cookingNin, err := s.loadCookingNinTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load cooking-nin for fragment:", err)
			return
		}
		data["CookingNin"] = cookingNin
	case "sheet_genjutsu":
		genjutsu, err := s.loadGenjutsuTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu for fragment:", err)
			return
		}
		data["Genjutsu"] = genjutsu
	case "sheet_medical_nin":
		medicalNin, err := s.loadMedicalNinTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load medical-nin for fragment:", err)
			return
		}
		data["MedicalNin"] = medicalNin
	case "sheet_scout_nin":
		scoutNin, err := s.loadScoutNinTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load scout-nin for fragment:", err)
			return
		}
		data["ScoutNin"] = scoutNin
	case "sheet_intelligence_operative":
		intelligenceOperative, err := s.loadIntelligenceOperativeTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load intelligence operative for fragment:", err)
			return
		}
		data["IntelligenceOperative"] = intelligenceOperative
	case "sheet_ninjutsu_specialist":
		ninjutsuSpecialist, err := s.loadNinjutsuSpecialistTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load ninjutsu specialist for fragment:", err)
			return
		}
		data["NinjutsuSpecialist"] = ninjutsuSpecialist
	case "sheet_science_nin":
		scienceNin, err := s.loadScienceNinTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load science-nin for fragment:", err)
			return
		}
		data["ScienceNin"] = scienceNin
	case "sheet_mastery":
		mastery, err := s.loadMasteryData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load mastery for fragment:", err)
			return
		}
		data["Mastery"] = mastery
	case "sheet_feature_choices":
		pendingASI, err := s.buildPendingASIRows(characterID, sheet.PendingASISlots, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build pending ASI rows for fragment:", err)
			return
		}
		pendingFeatureChoices, err := s.buildPendingFeatureChoiceRows(sheet.PendingFeatureChoices)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build pending feature choice rows for fragment:", err)
			return
		}
		characterFeats, err := s.loadCharacterFeats(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load character feats for feature choices fragment:", err)
			return
		}
		pendingFeatAbilityChoices, err := s.buildPendingFeatAbilityChoiceRows(characterID, characterFeats)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build pending feat ability choice rows for fragment:", err)
			return
		}
		pendingFeatSkillOrToolChoices, err := s.buildPendingFeatSkillOrToolChoiceRows(characterID, characterFeats)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build pending feat skill-or-tool choice rows for fragment:", err)
			return
		}
		pendingHunterPatternChoices, err := s.buildPendingHunterPatternChoiceRows(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build pending hunter pattern choice rows for fragment:", err)
			return
		}
		data["PendingFeatureChoices"] = pendingFeatureChoices
		data["PendingASI"] = pendingASI
		data["PendingFeatAbilityChoices"] = pendingFeatAbilityChoices
		data["PendingFeatSkillOrToolChoices"] = pendingFeatSkillOrToolChoices
		data["PendingHunterPatternChoices"] = pendingHunterPatternChoices
	case "sheet_companions":
		companions, err := charstore.ListCompanions(s.charDB, characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companions for fragment:", err)
			return
		}
		data["Companions"] = companions
	case "sheet_puppet_tab":
		puppetsTab, err := s.loadPuppetsTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load puppets tab for fragment:", err)
			return
		}
		data["PuppetsTab"] = puppetsTab
	case "sheet_summon_tab":
		summonsTab, err := s.loadSummonsTabData(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load summons tab for fragment:", err)
			return
		}
		data["SummonsTab"] = summonsTab
	case "sheet_passive_traits":
		grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load granted features for passive traits fragment:", err)
			return
		}
		patternRows, err := s.hunterNinPatternPassiveRows(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load hunter-nin pattern rows for passive traits fragment:", err)
			return
		}
		fullMetalShinobiRows, err := s.fullMetalShinobiPassiveRows(characterID, sheet.Level, grantedFeatures)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load full-metal shinobi resistance rows for passive traits fragment:", err)
			return
		}
		passiveTraitFeatures := append(grantedFeatures, patternRows...)
		passiveTraitFeatures = append(passiveTraitFeatures, fullMetalShinobiRows...)
		if demonSightRow, err := s.genjutsuMirageDemonSightPassiveRow(characterID); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu mirage demon sight row for passive traits fragment:", err)
			return
		} else if demonSightRow != nil {
			passiveTraitFeatures = append(passiveTraitFeatures, *demonSightRow)
		}
		traits := computePassiveTraits(passiveTraitFeatures, sheet.Level)
		// Elemental Resistance (Elemental Scout, 6th level) — see the main
		// sheet render's identical merge for why this can't live in the
		// static passiveTraitGrants table itself.
		elementalPicks, err := charstore.ListElementalAffinities(s.charDB, characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load elemental affinity picks for passive traits fragment:", err)
			return
		}
		traits = mergePassiveResistance(traits, scoutNinElementalResistanceEntry(grantedFeatures, elementalPicks["elemental-knowledge"]))
		data["PassiveTraits"] = traits
	case "sheet_feats":
		characterFeats, err := s.loadCharacterFeats(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load character feats for fragment:", err)
			return
		}
		data["Feats"] = characterFeats
	case "sheet_ambitions":
		drive, goal, fear, err := s.loadCharacterAmbitions(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load ambitions for fragment:", err)
			return
		}
		data["Drive"], data["Goal"], data["Fear"] = drive, goal, fear
	case "sheet_level_row":
		classSummary, err := s.loadClassSummary(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load class summary for fragment:", err)
			return
		}
		data["ClassSummary"] = classSummary
	case "sheet_tools_skills":
		toolProfs, err := s.loadCharacterProficiencyValues(characterID, "tool")
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load tool proficiencies for fragment:", err)
			return
		}
		customSkills, err := s.loadCustomSkills(characterID)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load custom skills for fragment:", err)
			return
		}
		toolMods, err := s.loadProficiencyMods(characterID, "tool")
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load tool proficiency mods for fragment:", err)
			return
		}
		skillMods, err := s.loadProficiencyMods(characterID, "skill")
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load skill proficiency mods for fragment:", err)
			return
		}
		allTools, err := s.loadAllToolNames()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load all tool names for fragment:", err)
			return
		}
		data["ToolProficiencies"] = buildProficiencyRows(toolProfs, "tool", toolMods, sheet)
		data["CustomSkills"] = buildProficiencyRows(customSkills, "skill", skillMods, sheet)
		data["AllTools"] = allTools
	case "sheet_languages":
		languages, err := s.loadCharacterProficiencyValues(characterID, "language")
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load languages for fragment:", err)
			return
		}
		allLanguages, err := s.loadAllLanguageNames()
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load all language names for fragment:", err)
			return
		}
		data["Languages"] = languages
		data["AllLanguages"] = allLanguages
	case "sheet_jutsu_known", "sheet_attack_jutsu_table":
		jutsu, err := s.loadCharacterJutsuSheet(characterID, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load character jutsu for "+name+" fragment:", err)
			return
		}
		grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load granted features for "+name+" fragment:", err)
			return
		}
		jutsuKnownCap, err := s.jutsuKnownCapForCharacter(characterID, sheet, grantedFeatures)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load jutsu known cap for "+name+" fragment:", err)
			return
		}
		data["Jutsu"] = jutsu
		data["JutsuKnownCap"] = jutsuKnownCap
		if name == "sheet_attack_jutsu_table" {
			customJutsuAttacks, err := charstore.ListCustomAttacks(s.charDB, characterID, "jutsu")
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("load custom jutsu attacks for fragment:", err)
				return
			}
			data["CustomJutsuAttacks"] = composeCustomAttacks(customJutsuAttacks, sheet, 0)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Println("render "+name+" fragment:", err)
	}
}

// renderSheetChatFragment renders just the chat/dice-log list
// ("sheet_chat_log", defined in character_sheet.html) — shared by the GET
// (initial/poll) and POST (after appending a line) branches of
// handleSheetChat.
func (s *server) renderSheetChatFragment(w http.ResponseWriter, characterID int64) {
	chatLog, err := s.loadChatLog(characterID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load chat log for fragment:", err)
		return
	}
	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render chat fragment: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "sheet_chat_log", map[string]any{"ID": characterID, "ChatLog": chatLog}); err != nil {
		log.Println("render chat fragment:", err)
	}
}

// respondSheet answers a sheet mutation: the re-rendered fragment when the
// request came from the page's own fetch(), an ordinary redirect back to
// the sheet when it came from a native form submission. See wantsFragment
// in server.go for why both paths have to exist — without the redirect
// branch, a form that JavaScript failed to intercept lands the player on a
// bare scrap of HTML with no styling, no navigation, and no sign that
// anything was saved.
func (s *server) respondSheet(w http.ResponseWriter, r *http.Request, characterID int64, fragment string) {
	if !wantsFragment(r) {
		redirectToSheet(w, r, characterID)
		return
	}
	s.renderSheetFragment(w, characterID, fragment)
}

// redirectToSheet is the shared "plain form POST, no live feedback needed"
// response every non-fetch sheet handler below ends with.
func redirectToSheet(w http.ResponseWriter, r *http.Request, characterID int64) {
	http.Redirect(w, r, "/characters/"+strconv.FormatInt(characterID, 10), http.StatusSeeOther)
}

// handleSheetUIStateSave persists one sheet-layout.js grid or
// sheet-subgrid.js order/orientation blob (form fields "key", "data") via
// charstore.SetSheetUIState. AJAX-only by nature — dragging a box or
// flipping a subgrid's orientation has no plain-HTML-form equivalent to
// fall back to, unlike the rest of this file's handlers — so this never
// redirects, just answers 204 on success. No fragment to re-render either:
// the box the player just dragged already shows its new position/order
// live, this call only makes it survive a reload.
func (s *server) handleSheetUIStateSave(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	data := r.FormValue("data")
	if key == "" || data == "" {
		http.Error(w, "key and data required", http.StatusBadRequest)
		return
	}
	if !json.Valid([]byte(data)) {
		http.Error(w, "data must be valid JSON", http.StatusBadRequest)
		return
	}
	if err := charstore.SetSheetUIState(s.charDB, id, key, data); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set sheet ui state:", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSheetUIStateReset clears one saved blob (form field "key") — the
// server-side half of "Reset Layout"/"Reset [subgrid]": the client reloads
// right after this succeeds, and the initial render embeds whatever's left
// in character_sheet_ui_state, so a cleared key comes back as the
// template's own computed default rather than reloading straight into the
// state that was just reset away from.
func (s *server) handleSheetUIStateReset(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	if err := charstore.DeleteSheetUIState(s.charDB, id, key); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete sheet ui state:", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSheetHP applies a signed HP delta (form field "delta") via
// charstore.SetHP — sheet-hp.js posts this after the player confirms a
// click-to-edit "+2"/"-5" entry on the HP fraction box.
func (s *server) handleSheetHP(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.SetHP(s.charDB, id, delta); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set hp:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetChakra applies a signed chakra delta (form field "delta") via
// charstore.SetChakra — the Chakra box is a click-to-edit box exactly like
// HP and Temp HP, so it posts through the same path and gets the same
// "sheet_vitals" fragment back.
func (s *server) handleSheetChakra(w http.ResponseWriter, r *http.Request) {
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
	if err := charstore.SetChakra(s.charDB, id, delta); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set chakra:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetJutsuCast is what the sheet's per-jutsu Cast button actually
// posts to — unlike the generic chakra-delta endpoint above, this one knows
// WHICH jutsu was cast (form fields "slug" and "rank", the rank actually
// cast at, alongside the same signed "delta" chakra cost the button already
// computed client-side per buildUpcastOptions). That's what lets a
// concentration jutsu start tracking itself: if the jutsu's own duration
// text requires concentration, casting it replaces whatever concentration
// slot the character had (see charstore.StartConcentration's upsert), same
// as the book's single-slot rule.
func (s *server) handleSheetJutsuCast(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	rank := strings.TrimSpace(r.FormValue("rank"))
	if slug == "" || rank == "" {
		http.Error(w, "missing slug or rank", http.StatusBadRequest)
		return
	}
	delta, err := strconv.Atoi(strings.TrimSpace(r.FormValue("delta")))
	if err != nil {
		http.Error(w, "bad delta", http.StatusBadRequest)
		return
	}
	// A Malleable Mirage's own "Cast via <Mirage>" button (jutsuFreeCastGrant,
	// character_sheet.html) carries a resource_key naming which of that
	// Mirage's rest-scoped uses (customResourceGrants) to spend — decremented
	// here, alongside the chakra delta and concentration tracking below,
	// rather than through handleSheetCustomResource's own generic spend
	// endpoint, since routing it through there would silently skip the
	// concentration-tracking side effect a normal cast of a concentration
	// jutsu needs. resource_uses is how many uses that one cast spends —
	// optional, defaulting to 1 (every Mirage grant's own shape); Wolves
	// Legacy's Wolf Techniques (jutsuFreeCastGrant.UsesPerCast, hunter_nin.go)
	// is the one grant that spends more than one use per cast, so the
	// button submits its own count explicitly rather than this endpoint
	// assuming a fixed spend everywhere.
	if resourceKey := strings.TrimSpace(r.FormValue("resource_key")); resourceKey != "" {
		if !validCustomResourceKey(resourceKey) {
			http.NotFound(w, r)
			return
		}
		uses := 1
		if rawUses := strings.TrimSpace(r.FormValue("resource_uses")); rawUses != "" {
			uses, err = strconv.Atoi(rawUses)
			if err != nil || uses < 1 {
				http.Error(w, "bad resource_uses", http.StatusBadRequest)
				return
			}
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for jutsu cast resource:", err)
			return
		}
		entries, err := s.loadCustomResources(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load custom resources for jutsu cast:", err)
			return
		}
		found := false
		for _, e := range entries {
			if e.Key != resourceKey {
				continue
			}
			found = true
			if e.Current < uses {
				http.Error(w, "no uses left", http.StatusBadRequest)
				return
			}
			if err := charstore.SetCustomResourceValue(s.charDB, id, resourceKey, e.Current-uses); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("set custom resource for jutsu cast:", err)
				return
			}
			break
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	}
	if err := charstore.SetChakra(s.charDB, id, delta); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set chakra for cast:", err)
		return
	}
	var duration string
	if err := s.rulesDB.QueryRow(`SELECT duration FROM v_jutsu WHERE slug = ?`, slug).Scan(&duration); err != nil {
		// Stale/unknown slug — the chakra spend above already happened
		// (matches how the sheet already treats this jutsu as spendable),
		// there's just nothing to check for concentration.
		log.Println("load jutsu duration for cast:", err)
	} else if isConcentrationDuration(duration) {
		if err := charstore.StartConcentration(s.charDB, id, slug, rank); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("start concentration:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetConcentrationBreak clears the character's active concentration
// slot — the sheet's "Break Concentration" button, a plain no-input
// sheet-fetch-form.
func (s *server) handleSheetConcentrationBreak(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := charstore.BreakConcentration(s.charDB, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("break concentration:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetBaseTempHP sets a new Base Temp HP ceiling (form field
// "value") via charstore.SetBaseTempHP — the plain-input box that
// deliberately does NOT follow the click-to-edit-delta rule the HP/THP
// fraction boxes use.
func (s *server) handleSheetBaseTempHP(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value, err := strconv.Atoi(strings.TrimSpace(r.FormValue("value")))
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	if err := charstore.SetBaseTempHP(s.charDB, id, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set base temp hp:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetRyo updates the character's ryo total from the currency box's
// single free-text entry (form field "value"), which carries its own
// meaning in its leading character: an explicit sign ("+200", "-50")
// adjusts the running total, and a bare number ("2000") sets it outright.
// That distinction has to be made on the raw string, before parsing — once
// through ParseFloat, "+200" and "200" are the same float64 and the intent
// is gone. Commas are stripped first so the value the player sees in the
// box ("1,001,800") can be typed straight back in.
//
// Like the HP/THP boxes, this answers with the re-rendered fragment rather
// than a redirect: a full-page redirect is exactly what was throwing the
// player back to the top of the sheet on every entry.
func (s *server) handleSheetRyo(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := strings.ReplaceAll(strings.TrimSpace(r.FormValue("value")), ",", "")
	if raw == "" {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	relative := raw[0] == '+' || raw[0] == '-'
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	if relative {
		err = charstore.AddRyo(s.charDB, id, value)
	} else {
		err = charstore.SetRyo(s.charDB, id, value)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set ryo:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_ryo")
}

// handleSheetSpeed pins a manual Speed override (form field "value"), same
// click-to-edit widget and "absolute number sets it, +N/-N adjusts it"
// convention as Ryo (part 10's "make Speed editable like the Ryo counter"
// ask). The relative form needs the character's CURRENT effective speed —
// clan default or an earlier override — so it's computed fresh before adding
// the delta rather than trusting whatever the client last rendered.
func (s *server) handleSheetSpeed(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("value"))
	if raw == "" {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	relative := raw[0] == '+' || raw[0] == '-'
	n, err := strconv.Atoi(raw)
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	value := n
	if relative {
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load sheet for speed adjust:", err)
			return
		}
		value = sheet.Speed + n
	}
	if value < 0 {
		http.Error(w, "speed cannot be negative", http.StatusBadRequest)
		return
	}
	if err := charstore.SetOverride(s.charDB, id, "speed", strconv.Itoa(value), sheetOverrideFields["speed"]); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set speed:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_squares")
}

// handleSheetAbility overwrites one base ability score from the sheet's
// ability editor (form fields "ability" and "value").
//
// It answers 204 rather than a fragment, and the caller reloads. An ability
// score is the one number on this sheet that almost everything else is
// derived from — its own modifier, the saving throw, every skill under it,
// passive perception, initiative, AC, and every weapon attack and damage
// bonus — so there is no single chunk of the page that could be swapped and
// still leave the sheet self-consistent. Reloading is the honest answer,
// and browsers restore scroll position across a reload, so it does not
// throw the player back to the top the way a redirect would.
func (s *server) handleSheetAbility(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ability := strings.TrimSpace(r.FormValue("ability"))
	value, err := strconv.Atoi(strings.TrimSpace(r.FormValue("value")))
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	// The stored value is the base score, before clan/background/ASI
	// bonuses, so the ceiling here is deliberately generous rather than
	// the usual 20 — a final score of 20 can sit on a lower base.
	if value < 1 || value > 30 {
		http.Error(w, "ability scores must be between 1 and 30", http.StatusBadRequest)
		return
	}
	if err := charstore.SetBaseAbility(s.charDB, id, ability, value); err != nil {
		http.Error(w, "bad ability", http.StatusBadRequest)
		log.Println("set base ability:", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// The three inventory handlers all answer 204 and let the page reload,
// rather than swapping an inventory fragment in place.
//
// Inventory is the one part of the sheet whose changes reach clean across
// it: equipping armor moves AC, equipping a weapon adds or removes a row
// in Attacks & Jutsu on a different tab, and both live outside anything
// an inventory-shaped fragment could contain. Re-rendering the page is the
// only way to leave every number agreeing with every other one. The tab
// the player was on is remembered across the reload by sheet-tabs.js, and
// browsers restore scroll position, so it isn't the jump-to-top a redirect
// would be.

// handleSheetInventoryAdd puts one rules item into the character's bag.
func (s *server) handleSheetInventoryAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("item_slug"))
	if slug == "" {
		http.Error(w, "pick an item", http.StatusBadRequest)
		return
	}
	// Verified against the rules rather than trusted: a slug that isn't a
	// real item would sit in the bag forever displaying its own raw slug,
	// since loadCharacterInventory falls back to that when the lookup
	// misses. Checked across all three carriable tables, since the sheet's
	// Inventory tab now offers poisons and traps alongside equipment.
	known, err := s.carriedItemExists(slug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("verify item slug:", err)
		return
	}
	if !known {
		http.Error(w, "no such item", http.StatusBadRequest)
		return
	}
	quantity, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
	if err != nil {
		quantity = 1
	}
	if err := charstore.AddInventoryItem(s.charDB, id, slug, quantity); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add inventory item:", err)
		return
	}
	// Only ever posted from the Inventory tab's library pane (the Core tab's
	// condensed box has no add-from-library form of its own).
	s.respondSheet(w, r, id, "sheet_inventory_full")
}

// carriedItemExists reports whether a slug names something a character can
// carry — an equipment row, a poison, or a trap template.
func (s *server) carriedItemExists(slug string) (bool, error) {
	table := "equipment"
	for _, source := range carriedItemKinds {
		if strings.HasPrefix(slug, source.prefix) {
			switch source.kind {
			case "Poison":
				table = "poisons"
			case "Trap":
				table = "trap_templates"
			}
			break
		}
	}
	var count int
	// The table name is chosen from this fixed set above, never taken from
	// the request, so interpolating it carries no injection risk; the slug
	// itself stays a bound parameter.
	if err := s.rulesDB.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE slug = ?`, slug).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// inventoryViewFragment reads the "view" form field the condensed (Core
// tab) and full (Inventory tab) inventory renderings each set on every one
// of their shared update/delete/unpack forms (see sheet_inventory and
// sheet_inventory_full) — both renderings post to the exact same endpoints,
// so this is the only way the handler knows which fragment the requester
// actually wants back. Defaults to the condensed view for anything else
// (an old cached page, a form with the field stripped), never a 400 — the
// mutation itself still has to succeed either way.
func inventoryViewFragment(r *http.Request) string {
	if r.FormValue("view") == "sheet_inventory_full" {
		return "sheet_inventory_full"
	}
	return "sheet_inventory"
}

// handleSheetInventoryUpdate sets a row's quantity and equipped flag.
func (s *server) handleSheetInventoryUpdate(w http.ResponseWriter, r *http.Request) {
	id, rowID, ok := parseCharacterAndRowID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	quantity, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity")))
	if err != nil {
		quantity = 1
	}
	if err := charstore.UpdateInventoryItem(
		s.charDB, id, rowID, quantity, r.FormValue("equipped") == "1",
	); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("update inventory item:", err)
		return
	}
	s.respondSheet(w, r, id, inventoryViewFragment(r))
}

// handleSheetInventoryDelete drops a row from the bag.
func (s *server) handleSheetInventoryDelete(w http.ResponseWriter, r *http.Request) {
	id, rowID, ok := parseCharacterAndRowID(w, r)
	if !ok {
		return
	}
	if err := charstore.DeleteInventoryItem(s.charDB, id, rowID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete inventory item:", err)
		return
	}
	s.respondSheet(w, r, id, inventoryViewFragment(r))
}

// handleSheetInventoryUnpack empties a container row into the inventory: the
// pack goes away and everything its description lists arrives in its place.
//
// The contents are re-read from the rules here rather than trusted from the
// form, so the request carries nothing but "unpack row N".
func (s *server) handleSheetInventoryUnpack(w http.ResponseWriter, r *http.Request) {
	id, rowID, ok := parseCharacterAndRowID(w, r)
	if !ok {
		return
	}
	var slug sql.NullString
	err := s.charDB.QueryRow(
		`SELECT item_slug FROM character_inventory WHERE id = ? AND character_id = ?`, rowID, id,
	).Scan(&slug)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load inventory row for unpack:", err)
		return
	}
	if !slug.Valid || slug.String == "" {
		http.Error(w, "that row is free text, not an item that can be unpacked", http.StatusBadRequest)
		return
	}

	var description sql.NullString
	if err := s.rulesDB.QueryRow(
		`SELECT description FROM equipment WHERE slug = ?`, slug.String,
	).Scan(&description); err != nil && err != sql.ErrNoRows {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load pack description:", err)
		return
	}
	contents, err := s.parsePackContents(description.String)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("parse pack contents:", err)
		return
	}
	if len(contents) == 0 {
		http.Error(w, "that item doesn't contain anything", http.StatusBadRequest)
		return
	}

	unpacked := make([]charstore.UnpackedItem, 0, len(contents))
	for _, line := range contents {
		unpacked = append(unpacked, charstore.UnpackedItem{
			Slug: line.Slug, Text: line.Text, Quantity: line.Quantity,
		})
	}
	if err := charstore.UnpackInventoryItem(s.charDB, id, rowID, unpacked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("unpack inventory item:", err)
		return
	}
	s.respondSheet(w, r, id, inventoryViewFragment(r))
}

// parseCharacterAndRowID pulls both path values for the per-row inventory
// routes, answering 404 itself if either is malformed.
func parseCharacterAndRowID(w http.ResponseWriter, r *http.Request) (characterID, rowID int64, ok bool) {
	characterID, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	rowID, err = strconv.ParseInt(r.PathValue("rowID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	return characterID, rowID, true
}

// respondSheetReload is the answer for a change with effects too broad to
// express as one fragment: nothing at all for the page's own fetch (which
// reloads itself), and an ordinary redirect for a native form submission,
// which reloads by definition.
func (s *server) respondSheetReload(w http.ResponseWriter, r *http.Request, characterID int64) {
	if !wantsFragment(r) {
		redirectToSheet(w, r, characterID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxPortraitBytes caps an uploaded portrait. The image is base64'd into
// the character's row, so this is a cap on how large a single characters.db
// row can get — 2 MiB of source bytes becomes roughly 2.7 MiB of text,
// which is still comfortably under SQLite's default 1 GB string limit and
// still fast to read on every sheet render. It is also far more than a
// portrait needs; anything bigger is a photo that wandered in by accident.
const maxPortraitBytes = 2 << 20

// handleSheetPortrait accepts a portrait image upload and stores it inline
// as a data: URL (see migration 0005 for why inline rather than a file
// path). This is the one sheet endpoint that genuinely wants a
// multipart/form-data body — it carries a file, which urlencoded bodies
// cannot — so unlike every other handler here it parses with
// ParseMultipartForm. Everything else on the sheet must stay urlencoded;
// r.ParseForm() silently ignores multipart bodies and reads every field
// back as empty.
func (s *server) handleSheetPortrait(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// MaxBytesReader rejects an oversized upload as it streams rather than
	// after buffering the whole thing; the +1024 leaves room for the
	// multipart envelope itself so a file right at the limit still fits.
	r.Body = http.MaxBytesReader(w, r.Body, maxPortraitBytes+1024)
	if err := r.ParseMultipartForm(maxPortraitBytes); err != nil {
		http.Error(w, "image too large (2 MB maximum)", http.StatusRequestEntityTooLarge)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("portrait")
	if err != nil {
		http.Error(w, "no image supplied", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPortraitBytes+1))
	if err != nil {
		http.Error(w, "could not read image", http.StatusBadRequest)
		return
	}
	if len(data) > maxPortraitBytes {
		http.Error(w, "image too large (2 MB maximum)", http.StatusRequestEntityTooLarge)
		return
	}

	// Sniff the real content rather than trusting the browser-supplied
	// Content-Type: the stored string goes straight into an <img src>, and
	// a mislabeled file would render as a broken image with no explanation.
	// Only the raster formats a portrait would actually be are accepted —
	// notably not SVG, which is a script-execution vector.
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		http.Error(w, "unsupported image type "+contentType+" (use PNG, JPEG, GIF, or WebP)", http.StatusBadRequest)
		log.Printf("portrait upload rejected: %s reported %s, sniffed %s",
			header.Filename, header.Header.Get("Content-Type"), contentType)
		return
	}

	dataURL := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
	if err := charstore.SetPortrait(s.charDB, id, dataURL); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set portrait:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_portrait")
}

// handleSheetPortraitDelete clears the stored portrait, putting the box
// back to its empty "Upload portrait" state.
func (s *server) handleSheetPortraitDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := charstore.SetPortrait(s.charDB, id, ""); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("clear portrait:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_portrait")
}

// handleSheetRest applies a rest's outcome via charstore.SetRestGains.
//
// A Long Rest ("mode=long") takes no input at all and restores everything:
// HP and Chakra back to their maxima, and every spent hit die and chakra
// die back to the pool. This is deliberately not a partial recovery — no
// 5e-style "regain half your hit dice" — so there is nothing to type and
// nothing to decide.
//
// A Full Rest ("mode=full") is a superset of a Long Rest — everything
// above, plus every custom resource's own FullRegen (e.g. Science-Nin's
// CCD, which only reaches full charge on a Full Rest, staying at half
// after a mere Long Rest).
//
// A Short Rest is the one that still needs input, because it is the one
// with a real choice in it: how many dice to spend. sheet-rest.js rolls
// them through the shared dice tray and posts the totals; the values are
// clamped below so a client cannot heal past the maximum.
func (s *server) handleSheetRest(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	atoiField := func(name string) (int, error) {
		v := strings.TrimSpace(r.FormValue(name))
		if v == "" {
			return 0, nil
		}
		return strconv.Atoi(v)
	}

	mode := r.FormValue("mode")
	var hp, chakra int
	if mode == "long" || mode == "full" {
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for long/full rest:", err)
			return
		}
		hp = sheet.MaxHP - sheet.CurrentHP
		chakra = sheet.MaxChakra - sheet.CurrentChakra
		// Negative deltas return every spent die to the pool. Posted
		// hit-dice fields are ignored on a long/full rest — full recovery
		// is full recovery, there is nothing for the client to choose.
		if err := charstore.SetRestGains(s.charDB, id, hp, chakra,
			-sheet.HitDiceSpent, -sheet.ChakraDiceSpent); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set long/full rest gains:", err)
			return
		}
		if err := s.applyCustomResourceRest(id, sheet, mode); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("apply custom resource rest:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_vitals")
		return
	}

	hp, err = atoiField("hp")
	if err != nil {
		http.Error(w, "bad hp", http.StatusBadRequest)
		return
	}
	chakra, err = atoiField("chakra")
	if err != nil {
		http.Error(w, "bad chakra", http.StatusBadRequest)
		return
	}
	// A short rest's gain comes from dice, so it can easily roll past the
	// character's maximum — healing never exceeds it. Clamped here rather
	// than in charstore.SetRestGains, which stays a plain "apply the
	// delta" setter like every other one, and rather than in the browser,
	// so the ceiling holds regardless of what a client posts.
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for short rest:", err)
		return
	}
	if room := sheet.MaxHP - sheet.CurrentHP; hp > room {
		hp = room
	}
	if room := sheet.MaxChakra - sheet.CurrentChakra; chakra > room {
		chakra = room
	}
	if hp < 0 {
		hp = 0
	}
	if chakra < 0 {
		chakra = 0
	}
	hitDiceDelta, err := atoiField("hit_dice_delta")
	if err != nil {
		http.Error(w, "bad hit_dice_delta", http.StatusBadRequest)
		return
	}
	chakraDiceDelta, err := atoiField("chakra_dice_delta")
	if err != nil {
		http.Error(w, "bad chakra_dice_delta", http.StatusBadRequest)
		return
	}
	if err := charstore.SetRestGains(s.charDB, id, hp, chakra, hitDiceDelta, chakraDiceDelta); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set rest gains:", err)
		return
	}
	if err := s.applyCustomResourceRest(id, sheet, "short"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("apply custom resource rest:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetCustomResource applies a signed delta (form field "delta") to
// one of a character's custom resource pools (CCD, White Chakra, ...) —
// same click-to-edit-box shape as HP/Chakra, keyed by the {key} path
// segment instead of a fixed column. The key is validated against
// customResourceGrants first so a crafted request can't insert an
// arbitrary resource_key.
//
// Unlike HP/Chakra (real columns on characters, always present with a
// real stored value), a custom resource with no row yet is meant to start
// at its own Max (see computeCustomResources) rather than 0 — so the new
// absolute value is computed here, in Go, off loadCustomResources' own
// Current (which already accounts for that default) and persisted via
// SetCustomResourceValue, rather than applying the delta as a raw SQL
// update against a row that might not exist. Applying the delta in SQL
// against a nonexistent row has no baseline to add against and silently
// lands wrong — confirmed live-testing this feature, where a fresh CCD's
// first -20 spend produced 0 instead of Max-20.
func (s *server) handleSheetCustomResource(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	key := r.PathValue("key")
	if !validCustomResourceKey(key) {
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
		log.Println("compute sheet for custom resource:", err)
		return
	}
	entries, err := s.loadCustomResources(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom resources:", err)
		return
	}
	found := false
	for _, e := range entries {
		if e.Key != key {
			continue
		}
		found = true
		newValue := e.Current + delta
		if newValue < 0 {
			newValue = 0
		}
		if newValue > e.Max {
			newValue = e.Max
		}
		if err := charstore.SetCustomResourceValue(s.charDB, id, key, newValue); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set custom resource:", err)
			return
		}
		break
	}
	if !found {
		// The character doesn't actually have this resource (e.g. a stale
		// request from before a respec) — nothing to spend against.
		http.NotFound(w, r)
		return
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetInspiration toggles the inspiration flag (form field "on",
// "1" or "0").
func (s *server) handleSheetInspiration(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetInspiration(s.charDB, id, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set inspiration:", err)
		return
	}
	redirectToSheet(w, r, id)
}

// handleSheetAmbitions replaces the sheet-side Drive/Goal/Fear editor's
// values — same charstore.SetAmbitions the create flow's own ambitions
// step already uses, just redirecting back to the sheet instead of the
// creation hub.
func (s *server) handleSheetAmbitions(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	drive := strings.TrimSpace(r.FormValue("drive"))
	goal := strings.TrimSpace(r.FormValue("goal"))
	fear := strings.TrimSpace(r.FormValue("fear"))
	if err := charstore.SetAmbitions(s.charDB, id, drive, goal, fear); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set sheet ambitions:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_ambitions")
}

// handleSheetBio replaces the Bio tab's five free-text fields —
// sheet-tabs.js's Bio textareas autosave on blur straight to this.
func (s *server) handleSheetBio(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	err = charstore.SetBio(s.charDB, id,
		r.FormValue("appearance"), r.FormValue("backstory"),
		r.FormValue("allies_organizations"), r.FormValue("additional_features_text"),
		r.FormValue("treasure"),
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set bio:", err)
		return
	}
	redirectToSheet(w, r, id)
}

// handleSheetNotes replaces the Core tab's Notes scratchpad — the same
// blur-autosave path as the Bio tab, on its own route so a notes save never
// rewrites the Bio fields and vice versa.
func (s *server) handleSheetNotes(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetNotes(s.charDB, id, r.FormValue("notes")); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set notes:", err)
		return
	}
	redirectToSheet(w, r, id)
}

// handleSheetLevel sets one of the character's classes' level (form fields
// "level", 1-20, and "class_slug" — defaults to the primary class when
// omitted, so a stale bookmarked form from before multiclassing existed
// still works).
//
// This is a real level change: charstore.SetClassLevel raises that class's
// level, and everything derived from the character's TOTAL level —
// proficiency bonus, hit dice, unlocked features, Max HP and Max Chakra —
// follows on the next render. Swaps the sheet_level_row fragment back in
// along with every also-refresh block that depends on level or
// proficiency bonus, rather than a full page reload.
func (s *server) handleSheetLevel(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	level, err := strconv.Atoi(strings.TrimSpace(r.FormValue("level")))
	if err != nil || level < 1 || level > 20 {
		http.Error(w, "level must be a whole number from 1 to 20", http.StatusBadRequest)
		return
	}
	classSlug := strings.TrimSpace(r.FormValue("class_slug"))
	if classSlug == "" {
		classSlug, err = s.primaryClassSlug(id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("query primary class for level:", err)
			return
		}
		if classSlug == "" {
			http.Error(w, "pick a class before setting a level", http.StatusBadRequest)
			return
		}
	}
	if err := charstore.SetClassLevel(s.charDB, id, classSlug, level); err != nil {
		if errors.Is(err, charstore.ErrLevelCapExceeded) {
			http.Error(w, "that would push the character's total level above 20", http.StatusBadRequest)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set level:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_level_row")
}

// errNotCharactersClass and errNotClassSubclass are returned by
// setCharacterSubclass for a class_slug that isn't one of the character's
// own, or a subclass_slug that isn't a real option for that class.
var errNotCharactersClass = errors.New("not one of this character's classes")
var errNotClassSubclass = errors.New("not a subclass of that class")

// setCharacterSubclass validates and applies one subclass pick for one of
// the character's classes — the shared logic behind both the sheet's inline
// subclass picker (handleSheetSubclass) and the creation flow's own
// (handleCreateClassSubclass), so the two forms can never validate
// differently. subclassSlug == "" clears the pick.
func (s *server) setCharacterSubclass(characterID int64, classSlug, subclassSlug string) error {
	var classLevel int
	if err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`, characterID, classSlug,
	).Scan(&classLevel); err != nil {
		if err == sql.ErrNoRows {
			return errNotCharactersClass
		}
		return err
	}

	siblings, err := s.loadSubclassOptions(classSlug)
	if err != nil {
		return err
	}
	siblingSlugs := make([]string, len(siblings))
	valid := subclassSlug == ""
	for i, opt := range siblings {
		siblingSlugs[i] = opt.Slug
		if opt.Slug == subclassSlug {
			valid = true
		}
	}
	if !valid {
		return errNotClassSubclass
	}
	return charstore.SetSubclass(s.charDB, characterID, siblingSlugs, subclassSlug, classLevel)
}

// writeSubclassError maps setCharacterSubclass's sentinel errors to the
// right HTTP status, shared by every caller so a bad class_slug/
// subclass_slug pair always reads the same way regardless of which page
// submitted it.
func writeSubclassError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotCharactersClass):
		http.Error(w, "not one of this character's classes", http.StatusBadRequest)
	case errors.Is(err, errNotClassSubclass):
		http.Error(w, "not a subclass of that class", http.StatusBadRequest)
	default:
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set subclass:", err)
	}
}

// handleSheetSubclass sets (or clears, if subclass_slug is blank) which
// subclass a character has chosen for one of their classes (form fields
// "class_slug", "subclass_slug") — the sheet's own inline picker next to
// the class/subclass summary line. Refreshed through the same
// "sheet_level_row" fragment and also-refresh list as a level change, since
// picking a subclass can unlock features/passive traits exactly the way
// levelling up does.
func (s *server) handleSheetSubclass(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	classSlug := strings.TrimSpace(r.FormValue("class_slug"))
	subclassSlug := strings.TrimSpace(r.FormValue("subclass_slug"))
	if err := s.setCharacterSubclass(id, classSlug, subclassSlug); err != nil {
		writeSubclassError(w, err)
		return
	}
	s.respondSheet(w, r, id, "sheet_level_row")
}

// sheetOverrideFields whitelists which character_overrides fields the sheet
// may write, mapped to the note stored alongside the value. A form field
// name is arbitrary input right up until it's checked against this — the
// column is a free-text key, so an unchecked one would let a request invent
// override rows charsheet.Compute never reads and nothing ever cleans up.
var sheetOverrideFields = map[string]string{
	"maxhp":     "manual max HP",
	"maxchakra": "manual max chakra",
	"ac":        "manual AC",
	"speed":     "manual speed override",

	"initiative_ability": "initiative ability",
	"initiative_prof":    "initiative proficiency mode",
	"initiative_bonus":   "initiative flat bonus",

	"ac_ability": "AC adjustment ability",
	"ac_prof":    "AC adjustment proficiency mode",
	"ac_bonus":   "AC adjustment flat bonus",
}

func init() {
	// The self-governing jutsu attack abilities (Ninjutsu, Genjutsu, Taijutsu,
	// Bukijutsu — see charsheet.AttackKinds) are whitelisted from the same list
	// charsheet.Compute reads them back with, so adding another one can't leave
	// its override silently unwritable.
	for _, k := range charsheet.AttackKinds {
		sheetOverrideFields[charsheet.AttackAbilityField(k.Kind)] = "manual " + k.Kind + " attack ability"
	}
	// Clash Checks' own, independent ability overrides — same 4 discipline
	// names as AttackKinds (Hijutsu has no Clash entry either), different
	// character_overrides key (charsheet.ClashAbilityField).
	for _, k := range charsheet.AttackKinds {
		sheetOverrideFields[charsheet.ClashAbilityField(k.Kind)] = "manual " + k.Kind + " clash ability"
	}
}

// handleSheetMaxima pins or unpins Max HP and Max Chakra by hand (form
// fields "maxhp" and "maxchakra"; an empty field clears that pin and hands
// the number back to the automatic level-based calculation).
//
// This is the "or do it manually" half of level-up support: a player who
// rolled their hit dice instead of taking the fixed value types the total
// they actually rolled, and the sheet stops computing it.
func (s *server) handleSheetMaxima(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	for _, field := range []string{"maxhp", "maxchakra"} {
		raw := strings.TrimSpace(r.FormValue(field))
		if raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 0 {
				http.Error(w, "bad "+field, http.StatusBadRequest)
				return
			}
			raw = strconv.Itoa(v)
		}
		if err := charstore.SetOverride(s.charDB, id, field, raw, sheetOverrideFields[field]); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set "+field+" override:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_vitals")
}

// handleSheetAttackAbility overrides which ability one jutsu attack type
// rolls off (form fields "kind" — Ninjutsu/Genjutsu/Taijutsu — and
// "ability", empty meaning "back to the default").
//
// Class features and feats really do move these (Genjutsu off Charisma,
// Taijutsu off Dexterity), so the pick is per character rather than a
// global rule. Reloads the page rather than swapping a fragment: the
// change moves both the attack tile and every jutsu row's to-hit number,
// and it happens once per character rather than once per turn.
func (s *server) handleSheetAttackAbility(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	field := charsheet.AttackAbilityField(kind)
	if _, ok := sheetOverrideFields[field]; !ok {
		http.Error(w, "unknown attack kind", http.StatusBadRequest)
		return
	}
	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if ability != "" && !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "unknown ability", http.StatusBadRequest)
		return
	}
	if err := charstore.SetOverride(s.charDB, id, field, ability, sheetOverrideFields[field]); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set attack ability override:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_attack_mods")
}

// handleSheetClashAbility overrides which ability one discipline's Clash
// Check rolls off (form fields "kind" — Ninjutsu/Genjutsu/Taijutsu/Bukijutsu
// — and "ability", empty meaning "back to the default"). Same shape as
// handleSheetAttackAbility just above, and a deliberately separate
// character_overrides key (charsheet.ClashAbilityField) — a class feature
// or feat could plausibly move a Clash check's governing ability without
// also moving that discipline's Attack ability, or vice versa.
func (s *server) handleSheetClashAbility(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	field := charsheet.ClashAbilityField(kind)
	if _, ok := sheetOverrideFields[field]; !ok {
		http.Error(w, "unknown clash discipline", http.StatusBadRequest)
		return
	}
	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if ability != "" && !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "unknown ability", http.StatusBadRequest)
		return
	}
	if err := charstore.SetOverride(s.charDB, id, field, ability, sheetOverrideFields[field]); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set clash ability override:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_attack_mods")
}

// handleSheetInitiative sets the three parts of a character's initiative roll
// (form fields "ability", "prof" and "bonus"). Empty or absent fields clear
// back to the N5E defaults: Dexterity, half proficiency bonus, no flat extra.
//
// One endpoint for all three rather than three, because they are one decision
// — a feature that moves initiative onto Wisdom usually changes the
// proficiency share too, and posting them separately would render an
// intermediate state that is nobody's actual rule.
func (s *server) handleSheetInitiative(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if ability != "" && !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "unknown ability", http.StatusBadRequest)
		return
	}
	prof := strings.ToLower(strings.TrimSpace(r.FormValue("prof")))
	if prof != "" && !slices.Contains(charsheet.ProfModes, prof) {
		http.Error(w, "unknown proficiency mode", http.StatusBadRequest)
		return
	}
	// Stored as text like every other override, but validated as a number here
	// so a typo becomes a 400 rather than a silently-ignored value that leaves
	// the player staring at an unchanged total.
	bonus := strings.TrimSpace(r.FormValue("bonus"))
	if bonus != "" {
		n, err := strconv.Atoi(bonus)
		if err != nil {
			http.Error(w, "bonus must be a whole number", http.StatusBadRequest)
			return
		}
		if n == 0 {
			bonus = "" // clear rather than store a no-op override row
		} else {
			bonus = strconv.Itoa(n)
		}
	}

	for _, set := range []struct{ field, value string }{
		{"initiative_ability", ability},
		{"initiative_prof", prof},
		{"initiative_bonus", bonus},
	} {
		if err := charstore.SetOverride(s.charDB, id, set.field, set.value, sheetOverrideFields[set.field]); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set initiative override:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_squares")
}

// handleSheetAC sets AC's own Adjust menu (form fields "ability", "prof" and
// "bonus") — the same three-field composed modifier Initiative already has.
// Unlike Initiative this stacks on top of the
// armor-derived AC rather than replacing it, but the endpoint shape —
// validate, clear-on-blank, one write for all three — is deliberately
// identical to handleSheetInitiative's.
func (s *server) handleSheetAC(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if ability != "" && !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "unknown ability", http.StatusBadRequest)
		return
	}
	prof := strings.ToLower(strings.TrimSpace(r.FormValue("prof")))
	if prof != "" && !slices.Contains(charsheet.ProfModes, prof) {
		http.Error(w, "unknown proficiency mode", http.StatusBadRequest)
		return
	}
	bonus := strings.TrimSpace(r.FormValue("bonus"))
	if bonus != "" {
		n, err := strconv.Atoi(bonus)
		if err != nil {
			http.Error(w, "bonus must be a whole number", http.StatusBadRequest)
			return
		}
		if n == 0 {
			bonus = ""
		} else {
			bonus = strconv.Itoa(n)
		}
	}

	for _, set := range []struct{ field, value string }{
		{"ac_ability", ability},
		{"ac_prof", prof},
		{"ac_bonus", bonus},
	} {
		if err := charstore.SetOverride(s.charDB, id, set.field, set.value, sheetOverrideFields[set.field]); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set AC override:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_ac")
}

// handleSheetProficiencyMod sets one Tool Proficiencies & Custom Skills
// line's roll tweak (form fields "kind", "value", "ability", "prof",
// "bonus") — the per-item version of the same ability/proficiency/bonus
// composer Initiative and AC use, added so every line in the reworked T&CS
// box can be "fully customizable" rather than a flat, unrollable name.
func (s *server) handleSheetProficiencyMod(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	kind := strings.ToLower(strings.TrimSpace(r.FormValue("kind")))
	if kind != "tool" && kind != "skill" {
		http.Error(w, "unknown kind", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if value == "" {
		http.Error(w, "missing value", http.StatusBadRequest)
		return
	}
	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if ability != "" && !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "unknown ability", http.StatusBadRequest)
		return
	}
	prof := strings.ToLower(strings.TrimSpace(r.FormValue("prof")))
	if prof == "" || !slices.Contains(charsheet.ProfModes, prof) {
		prof = charsheet.ProfNone
	}
	bonus, err := strconv.Atoi(strings.TrimSpace(r.FormValue("bonus")))
	if err != nil {
		bonus = 0
	}

	if err := charstore.SetProficiencyMod(s.charDB, id, kind, value, ability, prof, bonus); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set proficiency mod:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_tools_skills")
}

// parseCustomItemForm reads the field set shared by "Add custom item" and a
// library entry's own edit form (see handleCustomItemUpdate in items.go) —
// everything about the item itself. Quantity and Equipped are NOT part of
// this: those describe one character's inventory row, not the shared
// library entry, and only the Add form (below) parses them.
func parseCustomItemForm(r *http.Request) (charstore.CustomItem, error) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return charstore.CustomItem{}, errors.New("name the item")
	}
	item := charstore.CustomItem{
		Name:        name,
		Kind:        strings.TrimSpace(r.FormValue("kind")),
		Description: strings.TrimSpace(r.FormValue("notes")),
	}
	switch r.FormValue("rollable_kind") {
	case "weapon", "toolkit", "other":
		item.RollableKind = r.FormValue("rollable_kind")
	}
	if item.RollableKind == "weapon" {
		item.DamageType = strings.TrimSpace(r.FormValue("damage_type"))
		item.Properties = strings.TrimSpace(r.FormValue("properties"))
		count, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("damage_count")))
		sides, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("damage_sides")))
		if count > 0 && sides > 0 {
			item.DamageDice = fmt.Sprintf("%dd%d", count, sides)
		}
	}
	if raw := strings.TrimSpace(r.FormValue("bulk")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			return charstore.CustomItem{}, errors.New("bad bulk")
		}
		item.Bulk = sql.NullFloat64{Float64: v, Valid: true}
	}
	return item, nil
}

// applyCustomItemRollableWiring hooks a freshly-created rollable custom item
// into whichever existing per-character mechanism its kind already has, so
// it is ready to roll immediately — no new override table for either case
// (see custom_items.rollable_kind's doc comment on why weapon needs none at
// all: buildAttacks reads custom_items directly).
func (s *server) applyCustomItemRollableWiring(characterID int64, item charstore.CustomItem) error {
	switch item.RollableKind {
	case "toolkit":
		// Same as picking a toolkit from a class/background choice slot —
		// a character_proficiencies row, so it shows up in Tool
		// Proficiencies & Custom Skills with the existing per-item override
		// (character_proficiency_mods) already available. Guarded against a
		// duplicate: unlike SetProficiencyMod, the insert isn't an upsert.
		existing, err := s.loadCharacterProficiencyValues(characterID, "tool")
		if err != nil {
			return err
		}
		if slices.Contains(existing, item.Name) {
			return nil
		}
		return charstore.AddCustomProficiency(s.charDB, characterID, "tool", item.Name)
	case "other":
		// A flat-bonus custom attack in its own "Other Rollables" section —
		// the simplest of the three existing override shapes, which fits a
		// generic rollable trinket that isn't a weapon or a tool.
		_, err := charstore.AddCustomAttack(s.charDB, characterID, charstore.CustomAttack{
			Kind: "item",
			Name: item.Name,
		})
		return err
	}
	return nil
}

// handleSheetInventoryAddCustom adds a new library entry (custom_items) and
// carries it into this character's bag — the "+ Add custom item" escape
// hatch for anything the rules catalogue has no row for. Every field but the
// name is optional. Unlike a catalogue pick, this always creates a brand new
// row: a freshly minted custom_items slug can never already be in this
// character's inventory, so there's nothing to merge quantity into.
func (s *server) handleSheetInventoryAddCustom(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	item, err := parseCustomItemForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	quantity := 1
	if q, err := strconv.Atoi(strings.TrimSpace(r.FormValue("quantity"))); err == nil && q > 0 {
		quantity = q
	}
	equipped := r.FormValue("equipped") == "1"

	created, err := charstore.AddCustomItem(s.charDB, item)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add custom item:", err)
		return
	}
	if err := charstore.AddInventoryItemWithEquipped(s.charDB, id, created.Slug, quantity, equipped); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add custom item to inventory:", err)
		return
	}
	if err := s.applyCustomItemRollableWiring(id, created); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("wire custom item rollable:", err)
		return
	}
	// Only ever posted from the Inventory tab (the Core tab's condensed box
	// has no "Add custom item" form of its own).
	s.respondSheet(w, r, id, "sheet_inventory_full")
}

// sheetLiveFragments whitelists the {{define}} blocks in
// character_sheet.html that handleSheetFragment will re-render on demand.
// Anything a fetch can name has to be listed here, both so a request can't
// ask for an arbitrary template and so the set of blocks that are allowed
// to refresh independently stays visible in one place.
var sheetLiveFragments = map[string]bool{
	"sheet_vitals":                 true,
	"sheet_squares":                true,
	"sheet_skills":                 true,
	"sheet_ryo":                    true,
	"sheet_tools_skills":           true,
	"sheet_languages":              true,
	"sheet_jutsu_known":            true,
	"sheet_attack_jutsu_table":     true,
	"sheet_ac":                     true,
	"sheet_attack_mods":            true,
	"sheet_saves":                  true,
	"sheet_weapon_attacks":         true,
	"sheet_inventory":              true,
	"sheet_inventory_full":         true,
	"sheet_passive_traits":         true,
	"sheet_feats":                  true,
	"sheet_ambitions":              true,
	"sheet_level_row":              true,
	"sheet_companions":             true,
	"sheet_puppet_tab":             true,
	"sheet_summon_tab":             true,
	"sheet_puppet_tactics":         true,
	"sheet_martial_techniques":     true,
	"sheet_weapon_focus":           true,
	"sheet_weapon_form":            true,
	"sheet_martial_defense":        true,
	"sheet_hunter_techniques":      true,
	"sheet_cooking_nin":            true,
	"sheet_genjutsu":               true,
	"sheet_medical_nin":            true,
	"sheet_scout_nin":              true,
	"sheet_intelligence_operative": true,
	"sheet_ninjutsu_specialist":    true,
	"sheet_science_nin":            true,
	"sheet_elemental_affinities":   true,
	"sheet_mastery":                true,
	"sheet_feature_choices":        true,
	"sheet_other_rollables":        true,
}

// handleSheetFragment re-renders one live block of the sheet on request.
//
// It exists because the sheet's blocks are no longer in one-to-one
// correspondence with the actions that change them. A rest posts to
// /sheet/rest and swaps "sheet_vitals" in reply, but it also changes the
// hit-dice counter, which now lives in the square row at the top of the
// left column — a different element, in a different column, that the reply
// to one POST cannot contain. Rather than teach every endpoint to return
// several fragments at once, the client names the extra blocks it wants
// refreshed (data-also-refresh) and fetches each from here.
func (s *server) handleSheetFragment(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if !sheetLiveFragments[name] {
		http.NotFound(w, r)
		return
	}
	s.renderSheetFragment(w, id, name)
}

// handleSheetCustomAttack adds one player-defined attack row (form fields
// "kind", "name", "attack_bonus", "damage_count", "damage_sides",
// "damage_bonus", "damage_type", "notes").
//
// Every number is optional except the name: an attack with no damage roll is
// a real thing (a shove, a grapple, a control jutsu), and a row with a to-hit
// of +0 is a legitimate answer, so a blank field means zero rather than an
// error. See migration 0007_custom_attacks.sql for why these rows exist.
func (s *server) handleSheetCustomAttack(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	attack, ok := parseCustomAttackForm(w, r)
	if !ok {
		return
	}
	if _, err := charstore.AddCustomAttack(s.charDB, id, attack); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add custom attack:", err)
		return
	}
	// Always sheet_other_rollables, matching sheet_custom_attack_fields'
	// own hardcoded data-target — the form's kind radio can land the new
	// row in any of the three tables, and data-also-refresh (also hardcoded
	// there) picks up whichever of the other two actually changed.
	s.respondSheet(w, r, id, "sheet_other_rollables")
}

// parseCustomAttackForm reads the fields the add and edit forms share, and
// writes the 400 itself when something required is missing — the two handlers
// only differ in what they do with the result.
//
// "kind" is the weapon/jutsu/item choice, which the edit form exposes as a
// real control: it decides which of the three attack tables (Weapons,
// Jutsu, Other Rollables) the row renders in, so changing it there is how a
// row filed under the wrong one gets moved.
func parseCustomAttackForm(w http.ResponseWriter, r *http.Request) (charstore.CustomAttack, bool) {
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind != "weapon" && kind != "jutsu" && kind != "item" {
		http.Error(w, "kind must be weapon, jutsu or item", http.StatusBadRequest)
		return charstore.CustomAttack{}, false
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return charstore.CustomAttack{}, false
	}
	// A custom row has nothing to fall back to, so an absent proficiency mode
	// means none — unlike a weapon or a jutsu, whose derived numbers already
	// carry the full bonus.
	prof := strings.ToLower(strings.TrimSpace(r.FormValue("attack_prof")))
	if !slices.Contains(charsheet.ProfModes, prof) {
		prof = charsheet.ProfNone
	}
	return charstore.CustomAttack{
		Kind:          kind,
		Name:          name,
		AttackAbility: formAbility(r, "attack_ability"),
		AttackProf:    prof,
		AttackBonus:   formInt(r, "attack_bonus"),
		DamageCount:   formInt(r, "damage_count"),
		DamageSides:   formInt(r, "damage_sides"),
		DamageAbility: formAbility(r, "damage_ability"),
		DamageBonus:   formInt(r, "damage_bonus"),
		DamageType:    strings.TrimSpace(r.FormValue("damage_type")),
		Notes:         strings.TrimSpace(r.FormValue("notes")),
	}, true
}

// handleSheetJutsuOptions overrides how one known jutsu's attack and damage
// are rolled (form field "slug", plus the same modifier fields the weapon and
// custom-attack forms use, and damage_count/damage_sides/damage_type).
//
// Jutsu are identified by slug rather than by a row id because that is how
// character_jutsu stores them and how the forget button already addresses
// them. Clearing every field back to the defaults deletes the row, so the
// jutsu returns to being derived from its own printed description.
func (s *server) handleSheetJutsuOptions(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}
	prof := strings.ToLower(strings.TrimSpace(r.FormValue("attack_prof")))
	if !slices.Contains(charsheet.ProfModes, prof) {
		prof = charsheet.ProfFull
	}
	opts := charstore.JutsuOptions{
		Slug:               slug,
		AttackAbility:      formAbility(r, "attack_ability"),
		AttackProf:         prof,
		AttackBonus:        formInt(r, "attack_bonus"),
		DamageCount:        formInt(r, "damage_count"),
		DamageSides:        formInt(r, "damage_sides"),
		DamageAbility:      formAbility(r, "damage_ability"),
		DamageBonus:        formInt(r, "damage_bonus"),
		DamageType:         strings.TrimSpace(r.FormValue("damage_type")),
		CostChakraOverride: formIntPtr(r, "cost_chakra_override"),
	}
	if err := charstore.SetJutsuOptions(s.charDB, id, opts); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set jutsu options:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_attack_jutsu_table")
}

// formAbility reads a three-letter ability code from a form, dropping anything
// that is not one. These come from <select>s with fixed options, so a bad value
// is a malformed request, and composing without that term is closer to intent
// than a 400 on a form the player can see is fine.
func formAbility(r *http.Request, field string) string {
	v := strings.ToLower(strings.TrimSpace(r.FormValue(field)))
	if slices.Contains(charsheet.Abilities, v) {
		return v
	}
	return ""
}

// formInt reads a signed whole number, tolerating a leading "+" the way the
// hand-typed modifier boxes have always accepted it. A blank or unparseable
// value is zero, which for every field using this means "no contribution".
func formInt(r *http.Request, field string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(r.FormValue(field), "+")))
	return n
}

// formIntPtr reads a signed whole number the same way formInt does, but
// returns nil for a blank or unparseable field instead of treating blank as
// zero — for fields where 0 is itself a meaningful value (e.g. a jutsu that
// costs no Chakra to cast) and must stay distinguishable from "leave unset".
func formIntPtr(r *http.Request, field string) *int {
	raw := strings.TrimSpace(strings.TrimPrefix(r.FormValue(field), "+"))
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

// handleSheetWeaponAttackOptions overrides how one equipped weapon's attack
// row is computed (path value "rid" — the inventory row; form fields
// "attack_ability", "attack_prof", "attack_bonus", "damage_ability",
// "damage_bonus").
//
// Clearing everything back to the defaults deletes the row rather than storing
// a no-op, so a weapon returns to being fully derived from its printed
// properties — see charstore.SetWeaponAttackOptions.
func (s *server) handleSheetWeaponAttackOptions(w http.ResponseWriter, r *http.Request) {
	id, inventoryID, ok := parseCharacterAndRowID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	prof := strings.ToLower(strings.TrimSpace(r.FormValue("attack_prof")))
	if !slices.Contains(charsheet.ProfModes, prof) {
		prof = charsheet.ProfFull
	}
	opts := charstore.WeaponAttackOptions{
		InventoryID:   inventoryID,
		AttackAbility: formAbility(r, "attack_ability"),
		AttackProf:    prof,
		AttackBonus:   formInt(r, "attack_bonus"),
		DamageAbility: formAbility(r, "damage_ability"),
		DamageBonus:   formInt(r, "damage_bonus"),
	}
	if err := charstore.SetWeaponAttackOptions(s.charDB, id, opts); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set weapon attack options:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_attacks")
}

// handleSheetCustomAttackUpdate rewrites one custom attack in place (path
// value "rowID"), letting the player tweak it as needed after it's been
// added (for example, upon level up) — a custom attack's to-hit and damage
// are hand-entered numbers, so they go stale the moment anything they were
// derived from changes, and re-typing the whole row to fix a +1 was the
// only way to do it.
func (s *server) handleSheetCustomAttackUpdate(w http.ResponseWriter, r *http.Request) {
	id, rowID, ok := parseCharacterAndRowID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	attack, ok := parseCustomAttackForm(w, r)
	if !ok {
		return
	}
	attack.ID = rowID
	if err := charstore.UpdateCustomAttack(s.charDB, id, attack); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("update custom attack:", err)
		return
	}
	// Same reasoning as handleSheetCustomAttack: fixed target, matching
	// sheet_custom_attack_fields' own hardcoded data-target/also-refresh.
	s.respondSheet(w, r, id, "sheet_other_rollables")
}

// customAttackFragment maps a character_custom_attacks row's kind to the
// live fragment that renders its table — used only by delete, since that's
// the one custom-attack action whose form (sheet_custom_attack_row) sets a
// per-instance data-target rather than the fixed one add/update share.
func customAttackFragment(kind string) string {
	switch kind {
	case "weapon":
		return "sheet_weapon_attacks"
	case "jutsu":
		return "sheet_attack_jutsu_table"
	default:
		return "sheet_other_rollables"
	}
}

// handleSheetCustomAttackDelete removes one custom attack by ID (path value
// "rid"), scoped to this character.
func (s *server) handleSheetCustomAttackDelete(w http.ResponseWriter, r *http.Request) {
	id, rowID, ok := parseCharacterAndRowID(w, r)
	if !ok {
		return
	}
	var kind string
	if err := s.charDB.QueryRow(
		`SELECT kind FROM character_custom_attacks WHERE id = ? AND character_id = ?`, rowID, id,
	).Scan(&kind); err != nil && err != sql.ErrNoRows {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom attack kind for delete:", err)
		return
	}
	if err := charstore.DeleteCustomAttack(s.charDB, id, rowID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete custom attack:", err)
		return
	}
	s.respondSheet(w, r, id, customAttackFragment(kind))
}

// primaryClassSlug is the first class the character took (order_index 0) —
// the one whose progression table the sheet quotes for jutsu known, hit dice
// and the rest. "" when the character has no class yet.
func (s *server) primaryClassSlug(characterID int64) (string, error) {
	var slug sql.NullString
	err := s.charDB.QueryRow(`
		SELECT class_slug FROM character_classes WHERE character_id = ? ORDER BY order_index LIMIT 1`,
		characterID,
	).Scan(&slug)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return slug.String, err
}

// loadJutsuOrigins tags each jutsu slug the character has a claim on:
// "clan" for the ones on their clan's list, "class" for the ones whose
// classification their class casts. Everything else in the book is left out
// of the map, and the sheet's library renders those as "other".
//
// The same union handleCreateJutsu's loadEligibleJutsu builds, minus the rank
// ceiling: on the sheet this drives a filter and a badge, not what may be
// picked, and a level-9 player looking for the A-rank they are about to earn
// should still be able to find it.
func (s *server) loadJutsuOrigins(classSlugs []string, clanSlug string) (map[string]string, error) {
	origins := map[string]string{}
	if len(classSlugs) == 0 {
		classSlugs = []string{""}
	}
	for _, classSlug := range classSlugs {
		if classSlug == "" && clanSlug == "" {
			continue
		}
		rows, err := s.rulesDB.Query(`
			SELECT slug, MAX(is_clan) FROM (
				SELECT slug, 0 AS is_clan FROM v_jutsu
				WHERE `+classJutsuPredicate+`
				UNION ALL
				SELECT jutsu_slug, 1 FROM clan_jutsu WHERE clan_slug = ?
			) GROUP BY slug`, classSlug, clanSlug)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var slug string
			var isClan bool
			if err := rows.Scan(&slug, &isClan); err != nil {
				rows.Close()
				return nil, err
			}
			if isClan {
				origins[slug] = "clan"
			} else if origins[slug] != "clan" {
				origins[slug] = "class"
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return origins, nil
}

// characterLevel is the character's total level — the sum of their class
// levels, which for a single-classed character is just that class's level.
//
// Used to stamp "chosen at level" on things learned from the sheet rather
// than during creation. A character with no class rows yet (mid-creation, or
// a rules row that went away) counts as level 1 rather than 0, so the stamp
// is always a level that exists.
func (s *server) characterLevel(characterID int64) int {
	var level int
	if err := s.charDB.QueryRow(
		`SELECT COALESCE(SUM(levels), 1) FROM character_classes WHERE character_id = ?`, characterID,
	).Scan(&level); err != nil || level < 1 {
		return 1
	}
	return level
}

// handleSheetJutsuAdd learns one jutsu straight from the sheet's Jutsu tab
// (form field "slug"), which is what a drop onto the "known jutsu" pane
// posts.
//
// Deliberately does NOT enforce the class's jutsu-known cap. The creation
// step does enforce it, because that is where a starting character is built;
// after that, class features, feats, scrolls and GM rulings all add jutsu
// outside the table, and a sheet that refused them would be wrong more often
// than right. The Jutsu tab shows the count against the cap instead, so the
// player can see when they are over it.
func (s *server) handleSheetJutsuAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		http.Error(w, "slug required", http.StatusBadRequest)
		return
	}
	var exists int
	if err := s.rulesDB.QueryRow(`SELECT COUNT(*) FROM jutsu WHERE slug = ?`, slug).Scan(&exists); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("verify jutsu slug:", err)
		return
	}
	if exists == 0 {
		http.Error(w, "no such jutsu", http.StatusBadRequest)
		return
	}
	eligible, err := s.jutsuEligibleForCharacter(id, slug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("check jutsu eligibility:", err)
		return
	}
	if !eligible {
		http.Error(w, "this character does not qualify for this jutsu", http.StatusBadRequest)
		return
	}
	if err := charstore.AddJutsu(s.charDB, id, slug, s.characterLevel(id)); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add character jutsu:", err)
		return
	}
	// sheet_jutsu_known rather than a full reload: learning a jutsu only
	// ever touches that list and the Attacks & Jutsu table (refreshed
	// alongside it via data-also-refresh on the form — see
	// sheet-jutsu-fetch.js), a bounded, known pair of regions unlike
	// equipping gear (AC, encumbrance, scattered everywhere), which is why
	// sheet-inventory.js's other forms still reload.
	s.respondSheet(w, r, id, "sheet_jutsu_known")
}

// handleSheetFeatAdd records one feat from the sheet's Feats tab (form field
// "slug"). Prerequisites are shown on the feat's card but not enforced —
// they are printed as prose ("Aburame Clan, Level 8+") with no structured
// form to check against, and a GM waiving one is normal play.
func (s *server) handleSheetFeatAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		http.Error(w, "slug required", http.StatusBadRequest)
		return
	}
	var description string
	if err := s.rulesDB.QueryRow(`SELECT description FROM feats WHERE slug = ?`, slug).Scan(&description); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "no such feat", http.StatusBadRequest)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("verify feat slug:", err)
		return
	}
	if err := charstore.AddFeat(s.charDB, id, slug, s.characterLevel(id)); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add character feat:", err)
		return
	}
	// A fixed single-ability clause applies immediately; a multi-option one
	// (e.g. Hive Minded's "Intelligence or Wisdom") is left unresolved for
	// the Pending Choices picker instead of guessing.
	if abilities, amount, ok := parseFeatAbilityIncrease(slug, description); ok && len(abilities) == 1 {
		if err := charstore.ApplyFeatAbilityBonus(s.charDB, id, slug, abilities[0], amount); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("apply feat ability bonus:", err)
			return
		}
	}
	// A clean fixed single-skill clause applies immediately, same as a fixed
	// single-ability clause above — upgrading to Mastery instead of a
	// duplicate grant when the character is already proficient, for the
	// feats whose text calls for that (see applyFeatProficiencyGrant). Tool/
	// choice-of-N proficiency clauses are left unparsed and documented in
	// FEAT_AUDIT.md instead.
	if skill, ok := parseFeatSkillProficiency(slug, description); ok {
		if err := s.applyFeatProficiencyGrant(id, slug, "skill", skill, description); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("apply feat skill proficiency:", err)
			return
		}
	}
	// Same fixed-single-category auto-apply as the skill clause above, for a
	// weapon-category grant instead (e.g. "You gain proficiency in Martial
	// Weapons").
	if category, ok := parseFeatWeaponProficiency(description); ok {
		if err := charstore.ApplyFeatWeaponProficiency(s.charDB, id, slug, category); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("apply feat weapon proficiency:", err)
			return
		}
	}
	s.respondSheet(w, r, id, "sheet_feats")
}

// handleSheetFeatDelete drops one taken feat (form field "slug"). Same
// body-not-path reasoning as handleSheetJutsuDelete: feat slugs contain
// slashes.
func (s *server) handleSheetFeatDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		http.Error(w, "slug required", http.StatusBadRequest)
		return
	}
	if err := charstore.DeleteFeat(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete character feat:", err)
		return
	}
	// Safe no-op if this feat never had an ability bonus applied (no clause,
	// or a multi-option clause whose pending choice was never resolved).
	if err := charstore.RemoveFeatAbilityBonus(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove feat ability bonus:", err)
		return
	}
	// Same safe-no-op reasoning as the ability bonus removal above.
	if err := charstore.RemoveFeatSkillProficiency(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove feat skill proficiency:", err)
		return
	}
	// Same safe-no-op reasoning, for the tool/kit side of a "Kit or Skill
	// (Pick one)" feat resolved via handleSheetFeatSkillOrToolChoice.
	if err := charstore.RemoveFeatToolProficiency(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove feat tool proficiency:", err)
		return
	}
	// Same safe-no-op reasoning again, for a weapon-category grant.
	if err := charstore.RemoveFeatWeaponProficiency(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove feat weapon proficiency:", err)
		return
	}
	// If this feat was granted through an ASI breakpoint's Feat alternative
	// (see charstore.SetAbilityScoreImprovementFeat), dropping it from this
	// tab must also clear that linkage — otherwise the breakpoint would stay
	// marked resolved with no feat anywhere to show for it.
	if err := charstore.ClearASIFeatChoiceByFeatSlug(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("clear asi feat choice on feat delete:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_feats")
}

// handleSheetJutsuDelete removes one known jutsu from the sheet (form field
// "slug").
//
// Keyed by slug rather than by row id because character_jutsu is written by
// the creation step as a set of slugs, and the sheet's jutsu rows carry the
// slug already — there is no id in that markup to send. The slug travels in
// the body rather than the path because jutsu slugs contain slashes. Scoped
// to the character, so a forged slug can only ever remove that character's
// own row.
func (s *server) handleSheetJutsuDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		http.Error(w, "slug required", http.StatusBadRequest)
		return
	}
	if _, err := s.charDB.Exec(
		`DELETE FROM character_jutsu WHERE character_id = ? AND jutsu_slug = ?`, id, slug,
	); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete character jutsu:", err)
		return
	}
	// character_jutsu_options is keyed by slug, not by a character_jutsu row
	// id, so there is nothing for SQLite to cascade from — forgetting a jutsu
	// has to take its overrides with it explicitly, or relearning it later
	// would silently come back pre-tuned. Not fatal: the jutsu is already gone,
	// and a stranded override row is invisible until that same jutsu returns.
	if err := charstore.DeleteJutsuOptions(s.charDB, id, slug); err != nil {
		log.Println("delete jutsu options:", err)
	}
	// See handleSheetJutsuAdd's comment: a bounded pair of regions, not a
	// full reload.
	s.respondSheet(w, r, id, "sheet_jutsu_known")
}

// handleCharacterCustomFeatures serves the "Custom Features" popup — the
// character's own homebrew/DM-granted features, opened the same real-
// separate-window way as reference.go's Class Reference and
// clan_reference.go's Clan Reference (reference-popup.js,
// [data-reference-popup]). Unlike those two, this list is player-authored,
// not derived from rules.db, so the popup is also where features are added
// and removed — handleCustomFeatureAdd/handleCustomFeatureDelete below,
// each a plain POST-and-redirect back to this same URL, matching this
// project's convention for rarely-used, popup-scoped forms rather than the
// main sheet's fetch-and-swap fragments.
func (s *server) handleCharacterCustomFeatures(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var name string
	if err := s.charDB.QueryRow(`SELECT name FROM characters WHERE id = ?`, id).Scan(&name); err != nil {
		http.NotFound(w, r)
		return
	}
	customFeatures, err := s.loadCharacterCustomFeatures(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom features for popup:", err)
		return
	}
	s.render(w, "character_custom_features.html", map[string]any{
		"Title":          name + " — Custom Features",
		"CharacterID":    id,
		"CharacterName":  name,
		"CustomFeatures": customFeatures,
	})
}

// handleCustomFeatureAdd adds one custom feature (form fields "name",
// "source_label", "description") from the Custom Features popup.
func (s *server) handleCustomFeatureAdd(w http.ResponseWriter, r *http.Request) {
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
	sourceLabel := strings.TrimSpace(r.FormValue("source_label"))
	description := strings.TrimSpace(r.FormValue("description"))
	if _, err := charstore.AddCustomFeature(s.charDB, id, name, sourceLabel, description); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add custom feature:", err)
		return
	}
	http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/custom-features", http.StatusSeeOther)
}

// handleCustomFeatureDelete removes one custom feature by ID (path value
// "fid"), scoped to this character.
func (s *server) handleCustomFeatureDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	featureID, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := charstore.DeleteCustomFeature(s.charDB, id, featureID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete custom feature:", err)
		return
	}
	http.Redirect(w, r, "/characters/"+strconv.FormatInt(id, 10)+"/custom-features", http.StatusSeeOther)
}

// handleSheetProficiency toggles one character_proficiencies row on/off
// from the sheet's proficiency bullets (form fields "kind", "value", "on").
func (s *server) handleSheetProficiency(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	value := strings.TrimSpace(r.FormValue("value"))
	if kind == "" || value == "" {
		http.Error(w, "kind and value are required", http.StatusBadRequest)
		return
	}
	if err := charstore.SetProficiencyToggle(s.charDB, id, kind, value, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set proficiency toggle:", err)
		return
	}
	// This one handler now answers three different boxes (part 10 added
	// remove buttons to Tool Proficiencies & Custom Skills and Languages,
	// both of which reuse this same on/off toggle rather than duplicating
	// it) — so which fragment comes back has to follow "kind" and, for
	// "skill", whether the name is one of the rules' own skills (the Skills
	// box's bullet toggle) or not (a custom skill living in the T&CS box;
	// see loadCustomSkills). Answering with the whole block, not just an
	// acknowledgement, matters here for the same reason it always has:
	// toggling a skill's proficiency changes its modifier and, if it was
	// Perception, passive Perception too — flipping only the bullet's
	// colour client-side would leave every number beside it stale.
	fragment := "sheet_skills"
	switch kind {
	case "tool":
		fragment = "sheet_tools_skills"
	case "language":
		fragment = "sheet_languages"
	case "skill":
		if _, isBookSkill := charsheet.SkillAbility[value]; !isBookSkill {
			fragment = "sheet_tools_skills"
		}
	}
	s.respondSheet(w, r, id, fragment)
}

// handleSheetCustomProf adds one player-typed tool/skill/language
// proficiency (form fields "kind", "value") from the sheet's "+" panels.
func (s *server) handleSheetCustomProf(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	value := strings.TrimSpace(r.FormValue("value"))
	if kind == "" || value == "" {
		http.Error(w, "kind and value are required", http.StatusBadRequest)
		return
	}
	if err := charstore.AddCustomProficiency(s.charDB, id, kind, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add custom proficiency:", err)
		return
	}
	fragment := "sheet_tools_skills"
	if kind == "language" {
		fragment = "sheet_languages"
	}
	s.respondSheet(w, r, id, fragment)
}

// handleSheetChat serves the chat/dice-log panel: GET returns the current
// log fragment (sheet-chat.js doesn't currently poll with it, but this
// keeps the same "GET renders, POST mutates then renders" shape every
// other fragment endpoint in this file uses); POST appends one line (form
// fields "kind" — 'message' or 'roll', "text", and "crit" for roll rows)
// via charstore.AppendChatLog, then returns the same fragment.
func (s *server) handleSheetChat(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		kind := r.FormValue("kind")
		if kind != "message" && kind != "roll" {
			http.Error(w, "bad kind", http.StatusBadRequest)
			return
		}
		text := strings.TrimSpace(r.FormValue("text"))
		if text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		crit := r.FormValue("crit")
		if crit == "" {
			crit = "none"
		}
		if err := charstore.AppendChatLog(s.charDB, id, kind, text, crit); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("append chat log:", err)
			return
		}
	}
	s.renderSheetChatFragment(w, id)
}

// handleSheetChatClear wipes one character's whole chat/dice log — reachable
// from every tab, since the panel itself (character_sheet.html's
// .sheet-chat-panel) is a single sidebar shared by every tab, not per-tab
// markup. Returns the same now-empty "sheet_chat_log" fragment
// handleSheetChat itself renders, so the existing sheet-fetch-form swap
// wiring (sheet-vitals.js) needs nothing new to handle it.
func (s *server) handleSheetChatClear(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := charstore.ClearChatLog(s.charDB, id); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("clear chat log:", err)
		return
	}
	s.renderSheetChatFragment(w, id)
}
