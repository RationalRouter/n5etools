// Turn-order (initiative) tracker for a saved encounter (migrations
// 0092_encounter_turn_order.sql and 0093_encounter_turn_order_manual.sql).
// Entries are adversaries pulled from the encounter's own roster, with
// initiative rolled server-side, or player/ally rows the GM types in.
//
// The order is stored (sort_order), not computed: a new entry, or one whose
// initiative is edited, is placed by initiative (descending, blank last,
// after any equal ones), and the GM can then drag entries anywhere, e.g. to
// break a tie or let the party's heavy hitter go first. Nothing re-sorts the
// other entries behind the GM's back.
//
// Every structural change (add, remove, initiative edit, Next Turn, reset)
// answers with the re-rendered #encounter-turn-order fragment, swapped in by
// encounters.js. The HP and note fields answer with their resolved plain
// value instead, so tabbing between fields never loses focus to a swap.
package main

import (
	"database/sql"
	"github.com/RationalRouter/n5e/internal/i18n"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type turnEntryView struct {
	ID          int64
	Kind        string
	AdversaryID int64 // 0 for a player/ally row, or once its adversary is removed
	Name        string
	Initiative  sql.NullInt64
	HPCurrent   sql.NullInt64
	HPMax       sql.NullInt64
	Note        string
	Current     bool
}

type turnAdversaryOption struct {
	ID        int64
	Name      string
	InitBonus int // the adversary's computed initiative modifier
	HPMax     int // its computed Hit Points
}

type turnOrderView struct {
	EncounterID int64
	Round       int
	Entries     []turnEntryView
	Adversaries []turnAdversaryOption
	Unit        *turnUnitView // the current entry's card; nil before the first turn
}

// turnUnitView is the unit card under the tracker for the entry whose turn
// it is. Row is nil for a player or ally, or an adversary since removed
// from the roster.
type turnUnitView struct {
	Name     string
	Kind     string
	Row      *encounterAdversaryView
	Passives []builderPassive
}

// orderedTurnEntriesSQL is the one place the tracker's order is read;
// loadTurnOrderView and the Next Turn/remove logic both go through it so
// "whose turn is next" always matches what the GM sees on screen.
const orderedTurnEntriesSQL = `
	SELECT id, kind, COALESCE(adversary_id, 0), name, initiative, hp_current, hp_max, note
	FROM encounter_turn_entries WHERE encounter_id = ?
	ORDER BY sort_order, id`

type turnQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func loadTurnEntries(q turnQuerier, encID int64) ([]turnEntryView, error) {
	rows, err := q.Query(orderedTurnEntriesSQL, encID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []turnEntryView
	for rows.Next() {
		var e turnEntryView
		if err := rows.Scan(&e.ID, &e.Kind, &e.AdversaryID, &e.Name, &e.Initiative, &e.HPCurrent, &e.HPMax, &e.Note); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// loadTurnOrderView reads the tracker. roster is the encounter's filled
// roster (encounterKit.fill), for the add picker's hints and the current
// adversary's unit card.
func loadTurnOrderView(charDB *sql.DB, encID int64, kit encounterKit, roster []encounterAdversaryView, tr i18n.TranslateFunc) (turnOrderView, error) {
	view := turnOrderView{EncounterID: encID}
	var current sql.NullInt64
	if err := charDB.QueryRow(`SELECT current_turn_entry_id, round_number FROM encounters WHERE id = ?`, encID).
		Scan(&current, &view.Round); err != nil {
		return view, err
	}
	entries, err := loadTurnEntries(charDB, encID)
	if err != nil {
		return view, err
	}
	for i := range entries {
		e := &entries[i]
		e.Current = current.Valid && e.ID == current.Int64
		if !e.Current {
			continue
		}
		view.Unit = &turnUnitView{Name: e.Name, Kind: e.Kind}
		for j := range roster {
			if e.AdversaryID != 0 && roster[j].ID == e.AdversaryID {
				row := roster[j]
				view.Unit.Row = &row
				view.Unit.Passives = kit.builderPassives(row.GeneratedAdversary, tr)
			}
		}
	}
	view.Entries = entries

	for _, a := range roster {
		view.Adversaries = append(view.Adversaries, turnAdversaryOption{
			ID: a.ID, Name: a.Name, InitBonus: a.Stats.Init, HPMax: a.Stats.HPMax,
		})
	}
	return view, nil
}

func (s *server) renderTurnOrder(w http.ResponseWriter, r *http.Request, encID int64) {
	roster, kit, err := s.filledRoster(r, encID)
	var view turnOrderView
	if err == nil {
		view, err = loadTurnOrderView(s.charDB, encID, kit, roster.Adversaries, i18n.Translator(localeFromContext(r.Context())))
	}
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load turn order:", err)
		return
	}
	s.renderCard(w, r, "encounters.html", "encounter_turn_order", view)
}

// parseTurnPath reads {id} and, when withTurn is set, {turnId}, and confirms
// the encounter exists (and owns that entry) before any write touches it.
func (s *server) parseTurnPath(w http.ResponseWriter, r *http.Request, withTurn bool) (encID, turnID int64, ok bool) {
	encID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, 0, false
	}
	if withTurn {
		turnID, err = strconv.ParseInt(r.PathValue("turnId"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return 0, 0, false
		}
		err = s.charDB.QueryRow(`SELECT 1 FROM encounter_turn_entries WHERE id = ? AND encounter_id = ?`, turnID, encID).Scan(new(int))
	} else {
		err = s.charDB.QueryRow(`SELECT 1 FROM encounters WHERE id = ?`, encID).Scan(new(int))
	}
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return 0, 0, false
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("lookup turn order target:", err)
		return 0, 0, false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return 0, 0, false
	}
	return encID, turnID, true
}

// uniqueTurnName numbers repeat names ("Level 3 Minion Striker #2") so the
// GM can tell identical generated adversaries apart in the tracker.
func uniqueTurnName(tx *sql.Tx, encID int64, base string) (string, error) {
	var n int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM encounter_turn_entries
		WHERE encounter_id = ? AND (name = ? OR name LIKE ? ESCAPE '\')`,
		encID, base, likeEscape(base)+" #%").Scan(&n)
	if err != nil || n == 0 {
		return base, err
	}
	return base + " #" + strconv.Itoa(n+1), nil
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func insertTurnEntry(tx *sql.Tx, encID int64, kind string, adversaryID sql.NullInt64, name string, initiative, hp sql.NullInt64) error {
	res, err := tx.Exec(`
		INSERT INTO encounter_turn_entries (encounter_id, kind, adversary_id, name, initiative, hp_current, hp_max, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, encID, kind, adversaryID, name, initiative, hp, hp, time.Now().Unix())
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	return placeByInitiative(tx, encID, id)
}

// initiativeSlot returns where an entry with this initiative belongs in an
// already-ordered list: right after the last entry whose initiative is at
// least as high (so it lands after any it ties with), or, if none is, just
// before the first entry that has an initiative. Blank-initiative entries
// the GM dragged somewhere are stepped over rather than treated as lowest,
// and a blank initiative itself goes last.
func initiativeSlot(others []turnEntryView, init sql.NullInt64) int {
	if !init.Valid {
		return len(others)
	}
	lastAtLeast, firstRanked := -1, -1
	for i, e := range others {
		if !e.Initiative.Valid {
			continue
		}
		if firstRanked < 0 {
			firstRanked = i
		}
		if e.Initiative.Int64 >= init.Int64 {
			lastAtLeast = i
		}
	}
	switch {
	case lastAtLeast >= 0:
		return lastAtLeast + 1
	case firstRanked >= 0:
		return firstRanked
	default:
		return len(others)
	}
}

// placeByInitiative moves entryID to its initiative slot among the other
// entries (keeping their current order) and renumbers sort_order.
func placeByInitiative(tx *sql.Tx, encID, entryID int64) error {
	entries, err := loadTurnEntries(tx, encID)
	if err != nil {
		return err
	}
	var self turnEntryView
	others := make([]turnEntryView, 0, len(entries))
	for _, e := range entries {
		if e.ID == entryID {
			self = e
			continue
		}
		others = append(others, e)
	}
	at := initiativeSlot(others, self.Initiative)
	ids := make([]int64, 0, len(entries))
	for _, e := range others[:at] {
		ids = append(ids, e.ID)
	}
	ids = append(ids, entryID)
	for _, e := range others[at:] {
		ids = append(ids, e.ID)
	}
	return renumberTurns(tx, ids)
}

func renumberTurns(tx *sql.Tx, ids []int64) error {
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE encounter_turn_entries SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return nil
}

// handleTurnAddAdversary adds one roster adversary (adversary_id), or every
// roster adversary not yet in the tracker (adversary_id=all), rolling
// 1d20 + its computed initiative for each and starting it at its computed
// Hit Points.
func (s *server) handleTurnAddAdversary(w http.ResponseWriter, r *http.Request) {
	encID, _, ok := s.parseTurnPath(w, r, false)
	if !ok {
		return
	}
	raw := r.FormValue("adversary_id")
	query := `SELECT id, name FROM encounter_adversaries WHERE encounter_id = ?`
	args := []any{encID}
	if raw == "all" {
		query += ` AND id NOT IN (SELECT adversary_id FROM encounter_turn_entries WHERE encounter_id = ? AND adversary_id IS NOT NULL)`
		args = append(args, encID)
	} else {
		advID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "bad adversary id", http.StatusBadRequest)
			return
		}
		query += ` AND id = ?`
		args = append(args, advID)
	}
	query += ` ORDER BY sort_order`

	stats, err := s.encounterStats(r, encID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute adversaries for turn order:", err)
		return
	}
	rows, err := s.charDB.Query(query, args...)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load adversaries for turn order:", err)
		return
	}
	var picks []turnAdversaryOption
	for rows.Next() {
		var a turnAdversaryOption
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			rows.Close()
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan adversary for turn order:", err)
			return
		}
		a.InitBonus, a.HPMax = stats[a.ID].Init, stats[a.ID].HPMax
		picks = append(picks, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load adversaries for turn order:", err)
		return
	}
	if raw != "all" && len(picks) == 0 {
		http.Error(w, "unknown adversary", http.StatusBadRequest)
		return
	}

	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin turn add tx:", err)
		return
	}
	defer tx.Rollback()
	rng := newRand()
	for _, a := range picks {
		name, err := uniqueTurnName(tx, encID, a.Name)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("number turn entry:", err)
			return
		}
		init := int64(rng.Intn(20) + 1 + a.InitBonus)
		hp := sql.NullInt64{Int64: int64(a.HPMax), Valid: a.HPMax > 0}
		if err := insertTurnEntry(tx, encID, "adversary", sql.NullInt64{Int64: a.ID, Valid: true}, name, sql.NullInt64{Int64: init, Valid: true}, hp); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("insert adversary turn entry:", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit turn add:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// parseOptionalInt reads a blank-or-integer form value.
func parseOptionalInt(raw string) (sql.NullInt64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sql.NullInt64{}, true
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return sql.NullInt64{}, false
	}
	return sql.NullInt64{Int64: v, Valid: true}, true
}

// handleTurnAddManual adds a player character or ally the GM types in.
func (s *server) handleTurnAddManual(w http.ResponseWriter, r *http.Request) {
	encID, _, ok := s.parseTurnPath(w, r, false)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	kind := r.FormValue("kind")
	if name == "" || (kind != "player" && kind != "ally") {
		http.Error(w, "name and kind (player or ally) are required", http.StatusBadRequest)
		return
	}
	init, ok := parseOptionalInt(r.FormValue("initiative"))
	if !ok {
		http.Error(w, "bad initiative", http.StatusBadRequest)
		return
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin manual turn add tx:", err)
		return
	}
	defer tx.Rollback()
	if err := insertTurnEntry(tx, encID, kind, sql.NullInt64{}, name, init, sql.NullInt64{}); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("insert manual turn entry:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit manual turn add:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// handleTurnInitiative sets (or, blank, clears) one entry's initiative and
// re-places that entry by it. The response is the whole tracker, since the
// row may move.
func (s *server) handleTurnInitiative(w http.ResponseWriter, r *http.Request) {
	encID, turnID, ok := s.parseTurnPath(w, r, true)
	if !ok {
		return
	}
	init, ok := parseOptionalInt(r.FormValue("value"))
	if !ok {
		http.Error(w, "bad initiative", http.StatusBadRequest)
		return
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin turn initiative tx:", err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE encounter_turn_entries SET initiative = ? WHERE id = ?`, init, turnID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set turn initiative:", err)
		return
	}
	if err := placeByInitiative(tx, encID, turnID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("place turn by initiative:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit turn initiative:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// handleTurnReorder stores a GM-chosen order: "order" is every entry id in
// the encounter, comma-separated, top to bottom. Initiative values are left
// as they are. A list that doesn't name exactly this encounter's entries
// (stale page, another tab changed the tracker) is rejected with 409 so the
// client can reload the tracker instead of saving half an order.
func (s *server) handleTurnReorder(w http.ResponseWriter, r *http.Request) {
	encID, _, ok := s.parseTurnPath(w, r, false)
	if !ok {
		return
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, part := range strings.Split(r.FormValue("order"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || seen[id] {
			http.Error(w, "bad order", http.StatusBadRequest)
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin turn reorder tx:", err)
		return
	}
	defer tx.Rollback()
	entries, err := loadTurnEntries(tx, encID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load turn entries for reorder:", err)
		return
	}
	if len(entries) != len(ids) {
		http.Error(w, "turn order changed; reload it", http.StatusConflict)
		return
	}
	for _, e := range entries {
		if !seen[e.ID] {
			http.Error(w, "turn order changed; reload it", http.StatusConflict)
			return
		}
	}
	if err := renumberTurns(tx, ids); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("reorder turns:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit turn reorder:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// handleTurnHP is the delta box for current HP: a leading +/- adjusts
// (floored at 0; an unset current HP counts from max HP, so "-7" against a
// fresh 40-HP adversary leaves 33), a bare number sets, blank clears. The
// response is the resolved value as plain text, written back into the field.
func (s *server) handleTurnHP(w http.ResponseWriter, r *http.Request) {
	_, turnID, ok := s.parseTurnPath(w, r, true)
	if !ok {
		return
	}
	raw := strings.TrimSpace(r.FormValue("value"))
	if raw == "" {
		if _, err := s.charDB.Exec(`UPDATE encounter_turn_entries SET hp_current = NULL WHERE id = ?`, turnID); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("clear turn hp:", err)
		}
		return
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	if raw[0] == '+' || raw[0] == '-' {
		var base int64
		if err := s.charDB.QueryRow(`SELECT COALESCE(hp_current, hp_max, 0) FROM encounter_turn_entries WHERE id = ?`, turnID).Scan(&base); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load turn hp:", err)
			return
		}
		value = max(base+value, 0)
	}
	if _, err := s.charDB.Exec(`UPDATE encounter_turn_entries SET hp_current = ? WHERE id = ?`, value, turnID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set turn hp:", err)
		return
	}
	w.Write([]byte(strconv.FormatInt(value, 10)))
}

// handleTurnHPMax sets (or, blank, clears) max HP. Setting a max while
// current HP is still unset fills current HP too, so a freshly added
// adversary only needs one number typed in. Answers with the tracker,
// since current HP may have changed alongside it.
func (s *server) handleTurnHPMax(w http.ResponseWriter, r *http.Request) {
	encID, turnID, ok := s.parseTurnPath(w, r, true)
	if !ok {
		return
	}
	value, ok := parseOptionalInt(r.FormValue("value"))
	if !ok || (value.Valid && value.Int64 < 0) {
		http.Error(w, "bad value", http.StatusBadRequest)
		return
	}
	if _, err := s.charDB.Exec(`
		UPDATE encounter_turn_entries
		SET hp_max = ?1, hp_current = COALESCE(hp_current, ?1)
		WHERE id = ?2`, value, turnID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set turn hp max:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// handleTurnNote saves an entry's free-text note (conditions, concentration,
// whatever the GM wants to track). Answers with the saved text.
func (s *server) handleTurnNote(w http.ResponseWriter, r *http.Request) {
	_, turnID, ok := s.parseTurnPath(w, r, true)
	if !ok {
		return
	}
	note := strings.TrimSpace(r.FormValue("value"))
	if _, err := s.charDB.Exec(`UPDATE encounter_turn_entries SET note = ? WHERE id = ?`, note, turnID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set turn note:", err)
		return
	}
	w.Write([]byte(note))
}

// nextTurn returns the entry after current in tracker order, wrapping to the
// first entry (and reporting wrapped=true) after the last one. With no
// current entry yet, the first entry starts the round without wrapping.
func nextTurn(entries []turnEntryView, current sql.NullInt64) (next int64, wrapped, ok bool) {
	if len(entries) == 0 {
		return 0, false, false
	}
	if current.Valid {
		for i, e := range entries {
			if e.ID == current.Int64 {
				if i+1 < len(entries) {
					return entries[i+1].ID, false, true
				}
				return entries[0].ID, true, true
			}
		}
	}
	return entries[0].ID, false, true
}

// advanceTurn moves current_turn_entry_id one step and bumps round_number on
// wrap.
func advanceTurn(tx *sql.Tx, encID int64) error {
	var current sql.NullInt64
	if err := tx.QueryRow(`SELECT current_turn_entry_id FROM encounters WHERE id = ?`, encID).Scan(&current); err != nil {
		return err
	}
	entries, err := loadTurnEntries(tx, encID)
	if err != nil {
		return err
	}
	next, wrapped, ok := nextTurn(entries, current)
	if !ok {
		_, err := tx.Exec(`UPDATE encounters SET current_turn_entry_id = NULL WHERE id = ?`, encID)
		return err
	}
	round := 0
	if wrapped {
		round = 1
	}
	_, err = tx.Exec(`UPDATE encounters SET current_turn_entry_id = ?, round_number = round_number + ? WHERE id = ?`, next, round, encID)
	return err
}

func (s *server) handleTurnNext(w http.ResponseWriter, r *http.Request) {
	encID, _, ok := s.parseTurnPath(w, r, false)
	if !ok {
		return
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin next turn tx:", err)
		return
	}
	defer tx.Rollback()
	if err := advanceTurn(tx, encID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("advance turn:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit next turn:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// handleTurnDelete removes one entry, passing the turn on first if it was
// that entry's turn.
func (s *server) handleTurnDelete(w http.ResponseWriter, r *http.Request) {
	encID, turnID, ok := s.parseTurnPath(w, r, true)
	if !ok {
		return
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin turn delete tx:", err)
		return
	}
	defer tx.Rollback()
	if err := passTurnIfCurrent(tx, encID, turnID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("pass turn before delete:", err)
		return
	}
	if _, err := tx.Exec(`DELETE FROM encounter_turn_entries WHERE id = ? AND encounter_id = ?`, turnID, encID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete turn entry:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit turn delete:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

// passTurnIfCurrent hands the turn to the next entry when turnID is about to
// be removed while it's that entry's turn. Removing the only entry clears
// the current turn instead of wrapping onto itself.
func passTurnIfCurrent(tx *sql.Tx, encID, turnID int64) error {
	var current sql.NullInt64
	if err := tx.QueryRow(`SELECT current_turn_entry_id FROM encounters WHERE id = ?`, encID).Scan(&current); err != nil {
		return err
	}
	if !current.Valid || current.Int64 != turnID {
		return nil
	}
	entries, err := loadTurnEntries(tx, encID)
	if err != nil {
		return err
	}
	next, wrapped, ok := nextTurn(entries, current)
	if !ok || next == turnID {
		_, err := tx.Exec(`UPDATE encounters SET current_turn_entry_id = NULL WHERE id = ?`, encID)
		return err
	}
	round := 0
	if wrapped {
		round = 1
	}
	_, err = tx.Exec(`UPDATE encounters SET current_turn_entry_id = ?, round_number = round_number + ? WHERE id = ?`, next, round, encID)
	return err
}

// handleTurnReset clears every entry and starts again at round 1.
func (s *server) handleTurnReset(w http.ResponseWriter, r *http.Request) {
	encID, _, ok := s.parseTurnPath(w, r, false)
	if !ok {
		return
	}
	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin turn reset tx:", err)
		return
	}
	defer tx.Rollback()
	if err := clearTurnOrder(tx, encID); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("reset turn order:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit turn reset:", err)
		return
	}
	s.renderTurnOrder(w, r, encID)
}

func clearTurnOrder(tx *sql.Tx, encID int64) error {
	if _, err := tx.Exec(`DELETE FROM encounter_turn_entries WHERE encounter_id = ?`, encID); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE encounters SET current_turn_entry_id = NULL, round_number = 1 WHERE id = ?`, encID)
	return err
}

// handleTurnOrderFragment re-renders the tracker on its own. encounters.js
// calls it after a roster change, so the "add adversary" picker never lists
// a removed adversary or misses an added one. The route is
// /encounters/turn-order/{id} rather than /encounters/{id}/turns, which
// would be ambiguous with GET /encounters/npc-templates/{slug}.
func (s *server) handleTurnOrderFragment(w http.ResponseWriter, r *http.Request) {
	encID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderTurnOrder(w, r, encID)
}
