package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// timerBIMSlug/barrierBIMSlug are the two Explosive Modifications entries
// seedGrenadierBIMCatalog seeds — real class_option_entries slugs, matching
// the shape confirmed live against dist/rules.db (see
// splitScienceNinBIMPicks' own doc for why Barrier B.I.M specifically is
// matched by name rather than slug in the production code; the test seeds
// both anyway so a slug-based regression would also be caught).
const (
	timerBIMSlug   = "class/science-nin/option/explosive-modifications/minor/entry/timer-b-i-m"
	barrierBIMSlug = "class/science-nin/option/explosive-modifications/superior/entry/barrier-b-i-m"
)

// seedGrenadierBIMCatalog seeds enough rules.db content for a Grenadier's
// B.I.M box to render: the class itself, a class_levels/class_level_resources
// "Creation Points" row (loadScienceNinTabData's own early-exit gate),
// Grenadier's own subclass chain, its 3rd-level B.I.M granting feature, and
// a two-entry Explosive Modifications catalog (one ordinary B.I.M, plus the
// one book-named exception, Barrier B.I.M).
func seedGrenadierBIMCatalog(t *testing.T, s *server, level int) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/science-nin', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', ?, 'Creation Points', '20')`, level)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/grenadier', 'class/science-nin/group/scientific-inquiry', 'Grenadier')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/grenadier', 'B.I.M', 3,
		 'Your bandolier holds a number of B.I.Ms equal to your proficiency bonus.', 1)`, scienceNinBIMFeatureSlug)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/explosive-modifications/minor', 'class/science-nin', NULL, 'Explosive Modifications', 'Minor', '', 1),
		('class/science-nin/option/explosive-modifications/superior', 'class/science-nin', NULL, 'Explosive Modifications', 'Superior', '', 2)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/explosive-modifications/minor', 'Timer B.I.M', 'Set a delay before this B.I.M detonates.', 1),
		(?, 'class/science-nin/option/explosive-modifications/superior', 'Barrier B.I.M', 'Creates a barrier on detonation.', 1)`,
		timerBIMSlug, barrierBIMSlug)
}

// seedGrenadierCharacter inserts a character with the given Science-Nin
// level, already 3rd-level-subclassed into Grenadier.
func seedGrenadierCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/grenadier', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func loadGrenadierData(t *testing.T, s *server, characterID int64) *scienceNinGrenadierData {
	t.Helper()
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadScienceNinTabData(characterID, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil || data.Grenadier == nil {
		t.Fatalf("loadScienceNinTabData: want a rendered Grenadier box, got data=%+v", data)
	}
	return data.Grenadier
}

// bimAddRequest/bimDeleteRequest build a request against the real,
// server.go-registered science-nin-bim route(s) and run it through
// s.routes() end to end — the same dispatch a live browser POST goes
// through, not a hand-copied stand-in for it.
func bimAddRequest(t *testing.T, s *server, characterID int64, optionSlug string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/characters/"+strconv.FormatInt(characterID, 10)+"/sheet/science-nin-bim",
		strings.NewReader(url.Values{"option_slug": {optionSlug}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// sheet-fetch-form's own JS always sends this on a live sheet POST
	// (see sheet-refresh.js et al.) — matching it here is what makes a
	// successful pick answer 200 with a rendered fragment instead of a 303
	// redirect, so the test can assert on status codes cleanly.
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

// TestGrenadierBIMRepeatPicksAllowed covers the book's own text ("you can
// pick any modification with 'B.I.M' in the name more than once, other
// than the Barrier B.I.M"): picking the same non-Barrier B.I.M type twice
// must succeed both times, with BIMUsed reflecting the total held count
// (2), not the distinct-types-known count (which would read 1).
func TestGrenadierBIMRepeatPicksAllowed(t *testing.T) {
	s := testServer(t)
	seedGrenadierBIMCatalog(t, s, 10)
	id := seedGrenadierCharacter(t, s, "Deidara", 10)

	gr := loadGrenadierData(t, s, id)
	if gr.BIMUsed != 0 {
		t.Fatalf("BIMUsed before any pick = %d, want 0", gr.BIMUsed)
	}
	if len(gr.AvailableBIM) != 2 {
		t.Fatalf("AvailableBIM before any pick = %+v, want both seeded entries", gr.AvailableBIM)
	}

	if w := bimAddRequest(t, s, id, timerBIMSlug); w.Code != http.StatusOK {
		t.Fatalf("first Timer B.I.M pick: status %d, body %q", w.Code, w.Body.String())
	}
	gr = loadGrenadierData(t, s, id)
	if gr.BIMUsed != 1 {
		t.Fatalf("BIMUsed after one pick = %d, want 1", gr.BIMUsed)
	}
	if len(gr.KnownBIM) != 1 || gr.KnownBIM[0].Quantity != 1 {
		t.Fatalf("KnownBIM after one pick = %+v, want one entry with Quantity 1", gr.KnownBIM)
	}
	timerStillAvailable := false
	for _, o := range gr.AvailableBIM {
		if o.Slug == timerBIMSlug {
			timerStillAvailable = true
		}
	}
	if !timerStillAvailable {
		t.Fatalf("AvailableBIM after one pick = %+v, want Timer B.I.M to remain pickable again", gr.AvailableBIM)
	}

	// Pick the same type a second time.
	if w := bimAddRequest(t, s, id, timerBIMSlug); w.Code != http.StatusOK {
		t.Fatalf("second Timer B.I.M pick: status %d, body %q", w.Code, w.Body.String())
	}
	gr = loadGrenadierData(t, s, id)
	if gr.BIMUsed != 2 {
		t.Fatalf("BIMUsed after picking the same type twice = %d, want 2 (total held, not distinct types known)", gr.BIMUsed)
	}
	if len(gr.KnownBIM) != 1 || gr.KnownBIM[0].Quantity != 2 {
		t.Fatalf("KnownBIM after picking the same type twice = %+v, want exactly one entry with Quantity 2", gr.KnownBIM)
	}
}

// TestGrenadierBarrierBIMRejectsSecondPick covers the book's one explicitly
// named exception: Barrier B.I.M can be picked once but never a second
// time, even though every other B.I.M type in the catalog now allows
// repeats.
func TestGrenadierBarrierBIMRejectsSecondPick(t *testing.T) {
	s := testServer(t)
	seedGrenadierBIMCatalog(t, s, 10)
	id := seedGrenadierCharacter(t, s, "Deidara", 10)

	if w := bimAddRequest(t, s, id, barrierBIMSlug); w.Code != http.StatusOK {
		t.Fatalf("first Barrier B.I.M pick: status %d, body %q", w.Code, w.Body.String())
	}
	gr := loadGrenadierData(t, s, id)
	if gr.BIMUsed != 1 || len(gr.KnownBIM) != 1 || gr.KnownBIM[0].Name != "Barrier B.I.M" {
		t.Fatalf("after first Barrier B.I.M pick: BIMUsed=%d KnownBIM=%+v, want exactly one Barrier B.I.M held", gr.BIMUsed, gr.KnownBIM)
	}
	for _, o := range gr.AvailableBIM {
		if o.Slug == barrierBIMSlug {
			t.Fatalf("AvailableBIM after picking Barrier B.I.M = %+v, want Barrier B.I.M excluded", gr.AvailableBIM)
		}
	}

	// A second Barrier B.I.M pick must be rejected outright — it is no
	// longer a valid pick at all, matching every other category's own
	// "known picks leave Available" treatment.
	w := bimAddRequest(t, s, id, barrierBIMSlug)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("second Barrier B.I.M pick: status %d, body %q, want 400", w.Code, w.Body.String())
	}
	gr = loadGrenadierData(t, s, id)
	if gr.BIMUsed != 1 || len(gr.KnownBIM) != 1 || gr.KnownBIM[0].Quantity != 1 {
		t.Fatalf("after rejected second Barrier B.I.M pick: BIMUsed=%d KnownBIM=%+v, want unchanged at quantity 1", gr.BIMUsed, gr.KnownBIM)
	}
}

// TestGrenadierBIMCapStillEnforced confirms the bandolier's own overall cap
// (Proficiency Bonus) still gates the TOTAL held count once repeats are
// allowed — a character can't pick past their cap just because a type can
// now be picked more than once.
func TestGrenadierBIMCapStillEnforced(t *testing.T) {
	s := testServer(t)
	seedGrenadierBIMCatalog(t, s, 3) // 3rd level: Proficiency Bonus 3 in this system's own chart (xp_levels), and Grenadier's earliest level for the B.I.M feature itself
	id := seedGrenadierCharacter(t, s, "Deidara", 3)

	gr := loadGrenadierData(t, s, id)
	if gr.BIMCap != 3 {
		t.Fatalf("BIMCap at a 3-Proficiency-Bonus level = %d, want 3 (test assumes this xp_levels chart value; adjust the seeded level if the chart changes)", gr.BIMCap)
	}

	for i := 0; i < gr.BIMCap; i++ {
		if w := bimAddRequest(t, s, id, timerBIMSlug); w.Code != http.StatusOK {
			t.Fatalf("pick %d/%d: status %d, body %q", i+1, gr.BIMCap, w.Code, w.Body.String())
		}
	}
	gr = loadGrenadierData(t, s, id)
	if gr.BIMUsed != gr.BIMCap {
		t.Fatalf("BIMUsed = %d, want equal to BIMCap (%d) once fully loaded", gr.BIMUsed, gr.BIMCap)
	}

	// A third pick of the same (still catalog-available) type must now be
	// rejected purely on the cap, not on validity.
	w := bimAddRequest(t, s, id, timerBIMSlug)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("pick past cap: status %d, body %q, want 400", w.Code, w.Body.String())
	}
	gr = loadGrenadierData(t, s, id)
	if gr.BIMUsed != gr.BIMCap {
		t.Fatalf("BIMUsed after rejected over-cap pick = %d, want still %d", gr.BIMUsed, gr.BIMCap)
	}
}

// TestScienceNinInversionSerumUnaffectedByBIMChange confirms a sibling
// Science-Nin subclass catalog (Mad Scientist's Inversion Serums) keeps its
// existing no-duplicate behavior: picking the same Serum twice still
// results in exactly one held copy, unlike B.I.M.
func TestScienceNinInversionSerumUnaffectedByBIMChange(t *testing.T) {
	s := testServer(t)
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	level := 10
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/science-nin', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', ?, 'Creation Points', '20')`, level)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/mad-scientist', 'class/science-nin/group/scientific-inquiry', 'Mad Scientist')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/mad-scientist', 'Inversion Serums', 3,
		 'You can hold a number of Serums equal to your Intelligence modifier.', 1)`, scienceNinInversionSerumsFeatureSlug)
	serumSlug := "class/science-nin/option/inversion-serums/minor/entry/test-serum"
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/inversion-serums/minor', 'class/science-nin', NULL, 'Inversion Serums', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/inversion-serums/minor', 'Test Serum', 'Drain: 2 Mending CCD Chakra. A test serum.', 1)`, serumSlug)

	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kabuto', 10, 10, 10, 16, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/mad-scientist', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}

	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickInversionSerum, serumSlug, "mending"); err != nil {
		t.Fatal(err)
	}
	// A direct repeat storage-layer call (the same shape a second "Brew
	// Serum" submission would make) must stay a single-row update, not add
	// a second copy.
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, charstore.ScienceNinPickInversionSerum, serumSlug, "mending"); err != nil {
		t.Fatal(err)
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, charstore.ScienceNinPickInversionSerum)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Quantity != 1 {
		t.Fatalf("Inversion Serum picks after a repeat add = %+v, want exactly one row, still quantity 1", picks)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil || data.MadScientist == nil {
		t.Fatalf("loadScienceNinTabData: want a rendered Mad Scientist box, got %+v", data)
	}
	if data.MadScientist.SerumUsed != 1 {
		t.Fatalf("SerumUsed = %d, want 1 (a repeat pick must not double-count)", data.MadScientist.SerumUsed)
	}
	for _, o := range data.MadScientist.AvailableSerums {
		if o.Slug == serumSlug {
			t.Fatalf("AvailableSerums = %+v, want the already-known Serum excluded, same as before this change", data.MadScientist.AvailableSerums)
		}
	}
}
