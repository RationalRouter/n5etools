package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
)

// everEvolvingTestSeals: five Armor Seals, one per rank D through S —
// confirmed live against dist/rules.db that all five ranks actually exist
// among real enhancement_seal/armor rows (see ever_evolving.go's own header
// doc), seeded here too so a rank-filtering regression would be caught.
var everEvolvingTestSeals = []struct{ slug, name, rank string }{
	{"seal/armor/test/minor-test-seal", "Minor Test Seal", "D"},
	{"seal/armor/test/refined-test-seal", "Refined Test Seal", "C"},
	{"seal/armor/test/greater-test-seal", "Greater Test Seal", "B"},
	{"seal/armor/test/superior-test-seal", "Superior Test Seal", "A"},
	{"seal/armor/test/mastercraft-test-seal", "Mastercraft Test Seal", "S"},
}

// seedEverEvolvingRules seeds enough rules.db content for Ever Evolving's
// own seal picker to render: the Science-Nin class, a class_levels/
// class_level_resources "Creation Points" row (loadScienceNinTabData's own
// early-exit gate), Shinobi-Ware's own subclass chain, its 14th-level Ever
// Evolving granting feature, and everEvolvingTestSeals above.
func seedEverEvolvingRules(t *testing.T, s *server, level, creationPointsCap int) {
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
		('class/science-nin', ?, 'Creation Points', ?)`, level, strconv.Itoa(creationPointsCap))
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/shinobi-ware', 'class/science-nin/group/scientific-inquiry', 'Shinobi-Ware')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/shinobi-ware', 'Ever Evolving', 14,
		 'You can spend Creation Points equal to the rank of an Armor Seal to instantly apply it to your Full Metal Shinobi armor.', 1)`,
		scienceNinEverEvolvingFeatureSlug)
	for _, sl := range everEvolvingTestSeals {
		mustExecRules(`INSERT INTO equipment (slug, name, kind, seal_applies_to, seal_rank, description) VALUES
			(?, ?, 'enhancement_seal', 'armor', ?, 'A test armor seal.')`, sl.slug, sl.name, sl.rank)
	}
}

// seedEverEvolvingCharacter inserts a character with the given Science-Nin
// level, already 3rd-level-subclassed into Shinobi-Ware.
func seedEverEvolvingCharacter(t *testing.T, s *server, name string, level int) int64 {
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
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/shinobi-ware', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func loadEverEvolvingData(t *testing.T, s *server, characterID int64) *scienceNinShinobiWareData {
	t.Helper()
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadScienceNinTabData(characterID, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil || data.ShinobiWare == nil {
		t.Fatalf("loadScienceNinTabData: want a rendered Shinobi-Ware box, got %+v", data)
	}
	return data.ShinobiWare
}

// everEvolvingAddRequest/everEvolvingDeleteRequest build requests against
// the real, server.go-registered science-nin-ever-evolving-seal route(s)
// and run them through s.routes() end to end — the same dispatch a live
// browser POST goes through, not a hand-copied stand-in for it (mirrors
// science_nin_bim_test.go's own bimAddRequest).
func everEvolvingAddRequest(t *testing.T, s *server, characterID int64, sealSlug string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/characters/"+strconv.FormatInt(characterID, 10)+"/sheet/science-nin-ever-evolving-seal",
		strings.NewReader(url.Values{"seal_slug": {sealSlug}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

func everEvolvingDeleteRequest(t *testing.T, s *server, characterID int64, sealSlug string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/characters/"+strconv.FormatInt(characterID, 10)+"/sheet/science-nin-ever-evolving-seal/delete",
		strings.NewReader(url.Values{"seal_slug": {sealSlug}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

// TestEverEvolvingSealCatalogNoTierFilter pins the one deliberate
// difference from Martial Defense's own guard-slot catalog (contrast
// TestBattleReadyArmorSealOptions, puppets_test.go, and
// martial_defense.go's guardSealTierCap): Ever Evolving offers every armor
// seal rank D through S regardless of character level, with Cost equal to
// jutsuRankOrder's own D=1..S=5 scale.
func TestEverEvolvingSealCatalogNoTierFilter(t *testing.T) {
	s := testServer(t)
	seedEverEvolvingRules(t, s, 14, 20)

	catalog, err := s.loadEverEvolvingSealCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(everEvolvingTestSeals) {
		t.Fatalf("loadEverEvolvingSealCatalog = %d entries, want all %d ranks with no tier filtering", len(catalog), len(everEvolvingTestSeals))
	}
	wantCost := map[string]int{"D": 1, "C": 2, "B": 3, "A": 4, "S": 5}
	for _, o := range catalog {
		if want := wantCost[o.Rank]; o.Cost != want {
			t.Errorf("%s (rank %s): Cost = %d, want %d", o.Name, o.Rank, o.Cost, want)
		}
	}
}

// TestEverEvolvingSealAddRejectsOverBudget covers the CP-BUDGET gate
// (data.CreationPointsUsed+cost > data.CreationPointsCap) handleTitanUpgradeAdd
// already establishes the shape for: a character with too few remaining
// Creation Points to afford a given seal's rank must be rejected, even
// though Ever Evolving's own slot count (cap 1) is otherwise wide open.
func TestEverEvolvingSealAddRejectsOverBudget(t *testing.T) {
	s := testServer(t)
	// Cap 3: enough for the D/C/B ranks (cost 1/2/3) but not A (cost 4) or S
	// (cost 5).
	seedEverEvolvingRules(t, s, 14, 3)
	id := seedEverEvolvingCharacter(t, s, "Kabuto", 14)

	sw := loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal != nil {
		t.Fatalf("before any pick: EverEvolvingSeal = %+v, want nil", sw.EverEvolvingSeal)
	}
	if len(sw.AvailableEverEvolvingSeal) != len(everEvolvingTestSeals) {
		t.Fatalf("AvailableEverEvolvingSeal before any pick = %+v, want all %d seals offered (cap 1 has no picks yet)", sw.AvailableEverEvolvingSeal, len(everEvolvingTestSeals))
	}

	// Superior Test Seal (Rank A, cost 4) exceeds the 3-CP budget.
	w := everEvolvingAddRequest(t, s, id, "seal/armor/test/superior-test-seal")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("add over-budget seal (cost 4, cap 3): status %d, body %q, want 400", w.Code, w.Body.String())
	}
	sw = loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal != nil {
		t.Fatalf("after rejected over-budget add: EverEvolvingSeal = %+v, want still nil", sw.EverEvolvingSeal)
	}

	// Greater Test Seal (Rank B, cost 3) exactly fits.
	w = everEvolvingAddRequest(t, s, id, "seal/armor/test/greater-test-seal")
	if w.Code != http.StatusOK {
		t.Fatalf("add in-budget seal (cost 3, cap 3): status %d, body %q", w.Code, w.Body.String())
	}
	sw = loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal == nil || sw.EverEvolvingSeal.Slug != "seal/armor/test/greater-test-seal" {
		t.Fatalf("after successful add: EverEvolvingSeal = %+v, want Greater Test Seal", sw.EverEvolvingSeal)
	}
}

// TestEverEvolvingSealSingleSlotRejectsSecondAdd covers the cap: Ever
// Evolving is a single slot ("apply IT to your Full Metal Shinobi armor"),
// so a second Add while one is already applied must be rejected even when
// Creation Points remain — the player must delete the current seal first,
// which is what models "changing" it.
func TestEverEvolvingSealSingleSlotRejectsSecondAdd(t *testing.T) {
	s := testServer(t)
	seedEverEvolvingRules(t, s, 14, 20)
	id := seedEverEvolvingCharacter(t, s, "Kabuto", 14)

	if w := everEvolvingAddRequest(t, s, id, "seal/armor/test/minor-test-seal"); w.Code != http.StatusOK {
		t.Fatalf("first add: status %d, body %q", w.Code, w.Body.String())
	}
	w := everEvolvingAddRequest(t, s, id, "seal/armor/test/refined-test-seal")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("second add while one seal is already applied: status %d, body %q, want 400", w.Code, w.Body.String())
	}
	sw := loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal == nil || sw.EverEvolvingSeal.Slug != "seal/armor/test/minor-test-seal" {
		t.Fatalf("after rejected second add: EverEvolvingSeal = %+v, want unchanged at Minor Test Seal", sw.EverEvolvingSeal)
	}
}

// TestEverEvolvingSealDeleteRefundsCreationPoints covers the book's own
// "choosing no seal to gain the creation points back" clause: deleting the
// held seal must free its cost back into the shared Creation Points budget,
// and (since delete-then-add already models "changing" the seal, per
// ever_evolving.go's own header doc) leave the slot open for a fresh Add.
func TestEverEvolvingSealDeleteRefundsCreationPoints(t *testing.T) {
	s := testServer(t)
	seedEverEvolvingRules(t, s, 14, 5)
	id := seedEverEvolvingCharacter(t, s, "Kabuto", 14)

	// Mastercraft Test Seal (Rank S, cost 5) spends the entire budget.
	if w := everEvolvingAddRequest(t, s, id, "seal/armor/test/mastercraft-test-seal"); w.Code != http.StatusOK {
		t.Fatalf("add: status %d, body %q", w.Code, w.Body.String())
	}
	sw := loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal == nil {
		t.Fatal("after add: EverEvolvingSeal is nil")
	}

	if w := everEvolvingDeleteRequest(t, s, id, "seal/armor/test/mastercraft-test-seal"); w.Code != http.StatusOK {
		t.Fatalf("delete: status %d, body %q", w.Code, w.Body.String())
	}
	sw = loadEverEvolvingData(t, s, id)
	if sw.EverEvolvingSeal != nil {
		t.Fatalf("after delete: EverEvolvingSeal = %+v, want nil", sw.EverEvolvingSeal)
	}
	if len(sw.AvailableEverEvolvingSeal) != len(everEvolvingTestSeals) {
		t.Fatalf("AvailableEverEvolvingSeal after delete = %+v, want all %d seals offered again", sw.AvailableEverEvolvingSeal, len(everEvolvingTestSeals))
	}

	// The full budget must be free again — re-adding the same Rank S seal
	// (cost 5) must succeed against the same cap 5 that only just fit it
	// once already.
	if w := everEvolvingAddRequest(t, s, id, "seal/armor/test/mastercraft-test-seal"); w.Code != http.StatusOK {
		t.Fatalf("re-add after refund: status %d, body %q, want 200 (Creation Points were not actually refunded)", w.Code, w.Body.String())
	}
}
