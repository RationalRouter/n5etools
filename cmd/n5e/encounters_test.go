package main

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// seedAdversaryClasses adds Pack 1's class rows, one official role trait
// and two sheet traits, the ingest-populated rows the generator needs on top of rules
// migration 0082's seeded charts.
func seedAdversaryClasses(t *testing.T, s *server) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO adversary_ranks (slug, name, hp_formula, ac_bonus, save_bonus, save_dc_bonus, init_bonus) VALUES
			('adversary-rank/minion', 'Minion', '10 + Level', 0, 0, 0, -2),
			('adversary-rank/standard', 'Standard', '', 0, 0, 0, 0),
			('adversary-rank/elite', 'Elite', '+1.25', 1, 1, 1, 2),
			('adversary-rank/solo', 'Solo', '+1.5 + 0.5 x Per Player', 2, 2, 2, 3)`,
		`INSERT INTO adversary_roles (slug, name, description) VALUES ('adversary-role/defender', 'Defender', '')`,
		`INSERT INTO adversary_role_traits (slug, name, role_slug, rank, description) VALUES
			('adversary-role-trait/sturdy', 'Sturdy', 'adversary-role/defender', 'D', 'Test.')`,
		`INSERT INTO adversary_builder_sheet_traits (slug, category, category_tag, name, rank, description, sort_order, source_book) VALUES
			('adversary-builder-sheet/leaf-will', 'affiliation', 'Leaf Shinobi', 'Will of Fire', 'C', 'Test.', 1, 'book/adversary-builder-sheet'),
			('adversary-builder-sheet/sharingan', 'clan', 'Uchiha', 'Sharingan', '', 'Test.', 2, 'book/adversary-builder-sheet')`,
	} {
		if _, err := s.rulesDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

var rosterJSONField = regexp.MustCompile(`name="roster_json" value="([^"]*)"`)

func getPage(t *testing.T, s *server, path string) string {
	t.Helper()
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

// TestEncounterGenerateSaveAndTrack runs a generated roster through save,
// the saved page, a reroll and the turn order, checking that the builder
// choices are stored and the computed stats reach the tracker.
func TestEncounterGenerateSaveAndTrack(t *testing.T) {
	s := testServer(t)
	seedAdversaryClasses(t, s)

	getPage(t, s, "/encounters")
	w := postPopupForm(t, s, "/encounters/generate", url.Values{
		"party_size": {"4"}, "party_level": {"8"}, "difficulty": {"hard"},
		"clan_tag": {"Uchiha"}, "affiliation_tag": {"random"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("generate: status %d: %s", w.Code, w.Body.String())
	}
	m := rosterJSONField.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatal("generated page has no roster_json field")
	}
	rosterJSON := html.UnescapeString(m[1])

	w = postPopupForm(t, s, "/encounters", url.Values{
		"name": {"Valley Ambush"}, "roster_json": {rosterJSON},
		"party_size": {"4"}, "party_level": {"8"}, "difficulty": {"hard"}, "budget": {"128"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("save: status %d: %s", w.Code, w.Body.String())
	}
	encID, err := strconv.ParseInt(strings.TrimPrefix(w.Header().Get("Location"), "/encounters/"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}

	var n, wrongRank, wrongClan, noAffiliation, elite int
	if err := s.charDB.QueryRow(`
		SELECT COUNT(*),
		       SUM(adv_rank <> 'C'), SUM(clan_tag <> 'Uchiha'), SUM(affiliation_tag = 'Factionless'),
		       SUM(rank_slug = 'adversary-rank/elite')
		FROM encounter_adversaries WHERE encounter_id = ?`, encID).Scan(&n, &wrongRank, &wrongClan, &noAffiliation, &elite); err != nil {
		t.Fatal(err)
	}
	if n == 0 || wrongRank != 0 || wrongClan != 0 || noAffiliation != 0 || elite != 1 {
		t.Errorf("stored roster: %d rows, %d not C-Rank, %d not Uchiha, %d factionless, %d elite", n, wrongRank, wrongClan, noAffiliation, elite)
	}

	page := getPage(t, s, "/encounters/"+strconv.FormatInt(encID, 10))
	if !strings.Contains(page, "encounter-terms") {
		t.Error("saved roster renders no computed-stat tooltips")
	}

	view, err := loadEncounterRosterView(s.charDB, encID)
	if err != nil {
		t.Fatal(err)
	}
	boss := view.Adversaries[0]
	postTurn(t, s, "/encounters/"+strconv.FormatInt(encID, 10)+"/reroll", url.Values{"adversary_id": {strconv.FormatInt(boss.ID, 10)}})
	var clan, rank string
	if err := s.charDB.QueryRow(`SELECT clan_tag, adv_rank FROM encounter_adversaries WHERE id = ?`, boss.ID).Scan(&clan, &rank); err != nil {
		t.Fatal(err)
	}
	if clan != "Uchiha" || rank != "C" {
		t.Errorf("reroll changed clan/rank to %q/%q", clan, rank)
	}

	stats, err := s.encounterStats(httptest.NewRequest(http.MethodGet, "/", nil), encID)
	if err != nil {
		t.Fatal(err)
	}
	postTurn(t, s, turnPath(encID, "add-adversary"), url.Values{"adversary_id": {strconv.FormatInt(boss.ID, 10)}})
	var hpMax, hpCur, init int
	if err := s.charDB.QueryRow(`SELECT hp_max, hp_current, initiative FROM encounter_turn_entries WHERE adversary_id = ?`, boss.ID).Scan(&hpMax, &hpCur, &init); err != nil {
		t.Fatal(err)
	}
	want := stats[boss.ID]
	if hpMax != want.HPMax || hpCur != want.HPMax || want.HPMax < 50 {
		t.Errorf("turn entry HP %d/%d, want the computed %d", hpCur, hpMax, want.HPMax)
	}
	if init < 1+want.Init || init > 20+want.Init {
		t.Errorf("initiative %d outside 1d20%+d", init, want.Init)
	}

	if err := deleteEncounter(s.charDB, encID); err != nil {
		t.Fatal(err)
	}
	var left int
	s.charDB.QueryRow(`SELECT COUNT(*) FROM encounter_adversary_traits`).Scan(&left)
	if left != 0 {
		t.Errorf("%d trait rows left after deleting the encounter", left)
	}
}
