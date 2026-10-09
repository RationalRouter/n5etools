package main

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// seedTurnOrderEncounter inserts a saved encounter with two adversaries
// sharing a name (the generator names by level/rank/role, so repeats are
// normal) and returns the encounter id and adversary ids.
func seedTurnOrderEncounter(t *testing.T, s *server) (encID int64, advIDs []int64) {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO encounters (name, party_size, party_level, difficulty, budget) VALUES ('Bridge Ambush', 4, 3, 'standard', 10)`)
	if err != nil {
		t.Fatal(err)
	}
	encID, _ = res.LastInsertId()
	for i := 0; i < 2; i++ {
		res, err := s.charDB.Exec(`
			INSERT INTO encounter_adversaries (encounter_id, sort_order, name, rank_slug, rank_name, role_slug, role_name, level, init_bonus)
			VALUES (?, ?, 'Level 3 Minion Striker', 'adversary-rank/minion', 'Minion', 'adversary-role/striker', 'Striker', 3, 2)`, encID, i)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		advIDs = append(advIDs, id)
	}
	return encID, advIDs
}

func turnPath(encID int64, rest string) string {
	return "/encounters/" + strconv.FormatInt(encID, 10) + "/turns/" + rest
}

func postTurn(t *testing.T, s *server, path string, form url.Values) string {
	t.Helper()
	w := postPopupForm(t, s, path, form)
	if w.Code != http.StatusOK {
		t.Fatalf("POST %s: status %d: %s", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

type turnRow struct {
	ID         int64
	Name       string
	Initiative sql.NullInt64
}

func turnRows(t *testing.T, s *server, encID int64) []turnRow {
	t.Helper()
	entries, err := loadTurnEntries(s.charDB, encID)
	if err != nil {
		t.Fatal(err)
	}
	var out []turnRow
	for _, e := range entries {
		out = append(out, turnRow{e.ID, e.Name, e.Initiative})
	}
	return out
}

func currentTurn(t *testing.T, s *server, encID int64) (sql.NullInt64, int) {
	t.Helper()
	var cur sql.NullInt64
	var round int
	if err := s.charDB.QueryRow(`SELECT current_turn_entry_id, round_number FROM encounters WHERE id = ?`, encID).Scan(&cur, &round); err != nil {
		t.Fatal(err)
	}
	return cur, round
}

func TestTurnOrderAddSortAndNumbering(t *testing.T) {
	s := testServer(t)
	encID, _ := seedTurnOrderEncounter(t, s)

	body := postTurn(t, s, turnPath(encID, "add-adversary"), url.Values{"adversary_id": {"all"}})
	if !strings.Contains(body, `id="encounter-turn-order"`) {
		t.Fatalf("add-adversary should answer with the tracker fragment, got:\n%s", body)
	}
	rows := turnRows(t, s, encID)
	if len(rows) != 2 {
		t.Fatalf("want 2 adversary entries, got %d", len(rows))
	}
	names := map[string]bool{}
	for _, r := range rows {
		names[r.Name] = true
		if !r.Initiative.Valid || r.Initiative.Int64 < 3 || r.Initiative.Int64 > 22 {
			t.Errorf("%s: initiative %v outside 1d20+2", r.Name, r.Initiative)
		}
	}
	if !names["Level 3 Minion Striker"] || !names["Level 3 Minion Striker #2"] {
		t.Errorf("repeat names should be numbered, got %v", names)
	}
	// A level-3 Minion has 10 + level Hit Points.
	var hpMax, hpCur int
	if err := s.charDB.QueryRow(`SELECT hp_max, hp_current FROM encounter_turn_entries WHERE encounter_id = ? LIMIT 1`, encID).Scan(&hpMax, &hpCur); err != nil {
		t.Fatal(err)
	}
	if hpMax != 13 || hpCur != 13 {
		t.Errorf("Minion entry HP %d/%d, want 13/13", hpCur, hpMax)
	}

	// "all" again adds nothing: both are already in the tracker.
	postTurn(t, s, turnPath(encID, "add-adversary"), url.Values{"adversary_id": {"all"}})
	if n := len(turnRows(t, s, encID)); n != 2 {
		t.Fatalf("re-adding all should skip tracked adversaries, got %d entries", n)
	}

	// Pin known initiatives, then add a player with none and an ally tied
	// with the first adversary: blank sorts last, ties keep insertion order.
	rows = turnRows(t, s, encID)
	postTurn(t, s, turnPath(encID, strconv.FormatInt(rows[0].ID, 10)+"/initiative"), url.Values{"value": {"15"}})
	postTurn(t, s, turnPath(encID, strconv.FormatInt(rows[1].ID, 10)+"/initiative"), url.Values{"value": {"9"}})
	postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"Sakura"}, "kind": {"player"}, "initiative": {""}})
	postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"Pakkun"}, "kind": {"ally"}, "initiative": {"15"}})

	var got []string
	for _, r := range turnRows(t, s, encID) {
		got = append(got, r.Name)
	}
	want := []string{rows[0].Name, "Pakkun", rows[1].Name, "Sakura"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("order = %v, want %v", got, want)
	}

	if w := postPopupForm(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"X"}, "kind": {"adversary"}}); w.Code != http.StatusBadRequest {
		t.Errorf("manual add with kind=adversary: status %d, want 400", w.Code)
	}
}

func TestTurnOrderNextWrapsAndDeletePassesTurn(t *testing.T) {
	s := testServer(t)
	encID, _ := seedTurnOrderEncounter(t, s)
	for _, p := range []struct{ name, init string }{{"A", "20"}, {"B", "10"}, {"C", "5"}} {
		postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {p.name}, "kind": {"player"}, "initiative": {p.init}})
	}
	rows := turnRows(t, s, encID)
	a, b, c := rows[0].ID, rows[1].ID, rows[2].ID

	step := func(wantID int64, wantRound int) {
		t.Helper()
		postTurn(t, s, turnPath(encID, "next"), nil)
		cur, round := currentTurn(t, s, encID)
		if !cur.Valid || cur.Int64 != wantID || round != wantRound {
			t.Fatalf("after next: current=%v round=%d, want %d round %d", cur, round, wantID, wantRound)
		}
	}
	step(a, 1)
	step(b, 1)
	step(c, 1)
	step(a, 2)

	// Removing the entry whose turn it is passes the turn on.
	postTurn(t, s, turnPath(encID, strconv.FormatInt(a, 10)+"/delete"), nil)
	if cur, round := currentTurn(t, s, encID); cur.Int64 != b || round != 2 {
		t.Fatalf("after deleting current A: current=%v round=%d, want B round 2", cur, round)
	}
	// Removing the last entry while it's current wraps to the first and
	// starts a new round.
	step(c, 2)
	postTurn(t, s, turnPath(encID, strconv.FormatInt(c, 10)+"/delete"), nil)
	if cur, round := currentTurn(t, s, encID); cur.Int64 != b || round != 3 {
		t.Fatalf("after deleting current C: current=%v round=%d, want B round 3", cur, round)
	}
	// Removing the only entry clears the current turn instead of wrapping
	// onto itself.
	postTurn(t, s, turnPath(encID, strconv.FormatInt(b, 10)+"/delete"), nil)
	if cur, round := currentTurn(t, s, encID); cur.Valid || round != 3 {
		t.Fatalf("after deleting the only entry: current=%v round=%d, want none, round 3", cur, round)
	}

	postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"D"}, "kind": {"ally"}})
	postTurn(t, s, turnPath(encID, "next"), nil)
	postTurn(t, s, turnPath(encID, "reset"), nil)
	if cur, round := currentTurn(t, s, encID); cur.Valid || round != 1 || len(turnRows(t, s, encID)) != 0 {
		t.Fatalf("after reset: current=%v round=%d entries=%d", cur, round, len(turnRows(t, s, encID)))
	}
}

func TestTurnOrderHPAndNote(t *testing.T) {
	s := testServer(t)
	encID, _ := seedTurnOrderEncounter(t, s)
	postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"Zabuza"}, "kind": {"ally"}})
	id := strconv.FormatInt(turnRows(t, s, encID)[0].ID, 10)

	// Max HP fills a blank current HP; damage then counts down from it.
	postTurn(t, s, turnPath(encID, id+"/hp_max"), url.Values{"value": {"40"}})
	for _, step := range []struct{ in, want string }{{"-7", "33"}, {"+5", "38"}, {"12", "12"}, {"-50", "0"}} {
		if got := postTurn(t, s, turnPath(encID, id+"/hp"), url.Values{"value": {step.in}}); got != step.want {
			t.Errorf("hp %q -> %q, want %q", step.in, got, step.want)
		}
	}
	if got := postTurn(t, s, turnPath(encID, id+"/hp"), url.Values{"value": {""}}); got != "" {
		t.Errorf("blank hp should clear, got %q", got)
	}
	// With current HP cleared, a delta counts from max HP again.
	if got := postTurn(t, s, turnPath(encID, id+"/hp"), url.Values{"value": {"-10"}}); got != "30" {
		t.Errorf("delta from max after clear = %q, want 30", got)
	}
	if w := postPopupForm(t, s, turnPath(encID, id+"/hp"), url.Values{"value": {"abc"}}); w.Code != http.StatusBadRequest {
		t.Errorf("bad hp value: status %d, want 400", w.Code)
	}

	if got := postTurn(t, s, turnPath(encID, id+"/note"), url.Values{"value": {"  Concentrating  "}}); got != "Concentrating" {
		t.Errorf("note = %q", got)
	}

	// An entry id from another encounter is rejected.
	other, _ := seedTurnOrderEncounter(t, s)
	if w := postPopupForm(t, s, turnPath(other, id+"/hp"), url.Values{"value": {"1"}}); w.Code != http.StatusNotFound {
		t.Errorf("cross-encounter entry: status %d, want 404", w.Code)
	}
}

func TestTurnOrderSurvivesAdversaryAndEncounterDelete(t *testing.T) {
	s := testServer(t)
	encID, advIDs := seedTurnOrderEncounter(t, s)
	postTurn(t, s, turnPath(encID, "add-adversary"), url.Values{"adversary_id": {strconv.FormatInt(advIDs[0], 10)}})

	w := postPopupForm(t, s, "/encounters/"+strconv.FormatInt(encID, 10)+"/adversary/"+strconv.FormatInt(advIDs[0], 10)+"/delete", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete adversary: status %d", w.Code)
	}
	var advRef sql.NullInt64
	if err := s.charDB.QueryRow(`SELECT adversary_id FROM encounter_turn_entries WHERE encounter_id = ?`, encID).Scan(&advRef); err != nil {
		t.Fatalf("turn entry should survive its adversary's removal: %v", err)
	}
	if advRef.Valid {
		t.Errorf("adversary_id should be cleared, got %d", advRef.Int64)
	}

	page := getPopup(t, s, "/encounters/turn-order/"+strconv.FormatInt(encID, 10))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Level 3 Minion Striker") {
		t.Fatalf("fragment route: status %d body:\n%s", page.Code, page.Body.String())
	}

	detail := getPopup(t, s, "/encounters/"+strconv.FormatInt(encID, 10))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `id="encounter-turn-order"`) {
		t.Fatalf("encounter page should include the tracker: status %d", detail.Code)
	}

	if w := postPopupForm(t, s, "/encounters/"+strconv.FormatInt(encID, 10)+"/delete", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("delete encounter: status %d", w.Code)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM encounter_turn_entries WHERE encounter_id = ?`,
		`SELECT COUNT(*) FROM encounter_adversaries WHERE encounter_id = ?`,
	} {
		var n int
		if err := s.charDB.QueryRow(q, encID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d rows left (err %v)", q, n, err)
		}
	}
}

func turnNames(t *testing.T, s *server, encID int64) string {
	t.Helper()
	var names []string
	for _, r := range turnRows(t, s, encID) {
		names = append(names, r.Name)
	}
	return strings.Join(names, "|")
}

func TestTurnOrderManualReorder(t *testing.T) {
	s := testServer(t)
	encID, _ := seedTurnOrderEncounter(t, s)
	for _, p := range []struct{ name, init string }{{"A", "20"}, {"B", "12"}, {"C", "12"}, {"D", ""}} {
		postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {p.name}, "kind": {"player"}, "initiative": {p.init}})
	}
	if got := turnNames(t, s, encID); got != "A|B|C|D" {
		t.Fatalf("initial order = %s", got)
	}
	ids := map[string]string{}
	for _, r := range turnRows(t, s, encID) {
		ids[r.Name] = strconv.FormatInt(r.ID, 10)
	}

	// Break the B/C tie and send D (no initiative) to the top.
	postTurn(t, s, turnPath(encID, "reorder"), url.Values{"order": {ids["D"] + "," + ids["A"] + "," + ids["C"] + "," + ids["B"]}})
	if got := turnNames(t, s, encID); got != "D|A|C|B" {
		t.Fatalf("after reorder = %s", got)
	}
	for _, r := range turnRows(t, s, encID) {
		if r.Name == "D" && r.Initiative.Valid {
			t.Errorf("reorder must not touch initiative, D has %d", r.Initiative.Int64)
		}
	}

	// A new entry lands by initiative among the existing order, after ties
	// and without undoing the manual moves.
	postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"E"}, "kind": {"ally"}, "initiative": {"12"}})
	if got := turnNames(t, s, encID); got != "D|A|C|B|E" {
		t.Fatalf("after adding E at 12 = %s", got)
	}
	// Editing one initiative moves only that entry. Nothing has 25 or more,
	// so B goes just above the highest initiative, below the hand-placed D.
	postTurn(t, s, turnPath(encID, ids["B"]+"/initiative"), url.Values{"value": {"25"}})
	if got := turnNames(t, s, encID); got != "D|B|A|C|E" {
		t.Fatalf("after B -> 25 = %s", got)
	}

	// Next Turn follows the stored order.
	postTurn(t, s, turnPath(encID, "next"), nil)
	postTurn(t, s, turnPath(encID, "next"), nil)
	if cur, _ := currentTurn(t, s, encID); strconv.FormatInt(cur.Int64, 10) != ids["B"] {
		t.Errorf("second turn should be B, got entry %d", cur.Int64)
	}

	// A list that doesn't match the tracker is refused, not half-applied.
	for _, bad := range []struct {
		order string
		code  int
	}{
		{ids["A"] + "," + ids["B"], http.StatusConflict},
		{ids["A"] + "," + ids["A"] + "," + ids["B"] + "," + ids["C"] + "," + ids["D"], http.StatusBadRequest},
		{"x", http.StatusBadRequest},
	} {
		if w := postPopupForm(t, s, turnPath(encID, "reorder"), url.Values{"order": {bad.order}}); w.Code != bad.code {
			t.Errorf("reorder %q: status %d, want %d", bad.order, w.Code, bad.code)
		}
	}
	if got := turnNames(t, s, encID); got != "D|B|A|C|E" {
		t.Fatalf("rejected reorders changed the order: %s", got)
	}
}

func TestTurnOrderUnitCardFollowsCurrentTurn(t *testing.T) {
	s := testServer(t)
	encID, advIDs := seedTurnOrderEncounter(t, s)
	postTurn(t, s, turnPath(encID, "add-manual"), url.Values{"name": {"Sakura"}, "kind": {"player"}, "initiative": {"99"}})
	body := postTurn(t, s, turnPath(encID, "add-adversary"), url.Values{"adversary_id": {strconv.FormatInt(advIDs[0], 10)}})
	if strings.Contains(body, "encounter-unit-card") {
		t.Error("unit card shown before the first turn")
	}

	body = postTurn(t, s, turnPath(encID, "next"), nil)
	if !strings.Contains(body, "encounter-unit-card") || !strings.Contains(body, "Only adversaries from the roster have a unit card.") || strings.Contains(body, "encounter-unit-stats") {
		t.Errorf("a player's turn should show the no-card note:\n%s", body)
	}

	body = postTurn(t, s, turnPath(encID, "next"), nil)
	edit := `/encounters/builder/` + strconv.FormatInt(encID, 10) + `/` + strconv.FormatInt(advIDs[0], 10)
	if !strings.Contains(body, "encounter-unit-stats") || !strings.Contains(body, edit) {
		t.Errorf("an adversary's turn should show its stat block and Edit in Builder link:\n%s", body)
	}

	page := getPage(t, s, "/encounters/"+strconv.FormatInt(encID, 10))
	if !strings.Contains(page, `<details id="encounter-roster" class="encounter-roster" open>`) || !strings.Contains(page, "encounter-unit-stats") {
		t.Error("encounter page lacks the collapsible roster or the unit card")
	}
}
