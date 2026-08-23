package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// This file covers the three subclass tracker popups converted from
// single/small subclass-gated Core-sheet sections: Intelligence Operative's
// "Operative Traps" (Tactical Strategist only), Ninjutsu Specialist's
// "Awakened Scroll" (Scribe Master only), and Medical-Nin's "Combat Medic"
// (Fighting Stance + Expert Combatant, Combat Medic only) — mirroring
// science_nin_more_subclass_popups_test.go/taijutsu_subclass_popups_test.go's
// own EmptyHint + render/round-trip + sidebar-gating shape. getPopup/
// postPopupForm are shared helpers already defined in subclass_tracker_
// popup_test.go.

// --- Operative Traps -------------------------------------------------------

const operativeTrapsTestSlug = "class/intelligence-operative/option/operative-traps/test-trap"

// seedOperativeTrapsCatalog seeds enough rules.db content for a Tactical
// Strategist's own Operative Traps picker to render: the class itself, its
// Master Strategies subclass chain, and a one-row Operative Traps catalog
// (list_name-keyed, via loadIntelligenceOperativeOptionCatalog's own shape —
// operativeTrapsCap itself is hand-curated by level, no class_level_resources
// chart needed).
func seedOperativeTrapsCatalog(t *testing.T, s *server) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/intelligence-operative', 'Intelligence Operative', 8, 6)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/intelligence-operative/group/master-strategies', 'class/intelligence-operative', 'Master Strategies')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/intelligence-operative/group/master-strategies/tactical-strategist', 'class/intelligence-operative/group/master-strategies', 'Tactical Strategist')`)
	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		(?, 'class/intelligence-operative', NULL, 'Operative Traps', 'Test Trap', 'A test operative trap.', 1)`, operativeTrapsTestSlug)
}

// seedTacticalStrategistCharacter inserts a character with the given
// Intelligence Operative level, already 3rd-level-subclassed into Tactical
// Strategist.
func seedTacticalStrategistCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/intelligence-operative', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/intelligence-operative/group/master-strategies/tactical-strategist', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedPlainIntelligenceOperativeCharacter inserts a plain Intelligence
// Operative character with no subclass chosen — used by the EmptyHint test,
// which only needs the base class seeded without triggering Tactical
// Strategist's own Operative Traps gate.
func seedPlainIntelligenceOperativeCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/intelligence-operative', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOperativeTrapsPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedOperativeTrapsCatalog(t, s)
	id := seedPlainIntelligenceOperativeCharacter(t, s, "Shikamaru", 1) // no subclass -> Cap 0

	w := getPopup(t, s, operativeTrapsPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Operative Traps yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestOperativeTrapsPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedOperativeTrapsCatalog(t, s)
	id := seedTacticalStrategistCharacter(t, s, "Shikamaru", 6)
	popupPath := operativeTrapsPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Trap") {
		t.Errorf("initial render missing the available Operative Trap option:\n%s", body)
	}

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"option_slug": {operativeTrapsTestSlug}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListIntelligenceOperativePicks(s.charDB, id, charstore.IntelligenceOperativePickOperativeTrap)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0] != operativeTrapsTestSlug {
		t.Fatalf("picks after add = %+v, want one row for %q", picks, operativeTrapsTestSlug)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Trap") {
		t.Errorf("popup after add missing Test Trap in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"option_slug": {operativeTrapsTestSlug}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListIntelligenceOperativePicks(s.charDB, id, charstore.IntelligenceOperativePickOperativeTrap)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Awakened Scroll ---------------------------------------------------

// seedScribeMasterCatalog seeds enough rules.db content for a Scribe
// Master's own Awakened Scroll picker to render: the class itself, its
// Ninjutsu Focus subclass chain, and the Awakened Scroll granting feature
// (awakenedScrollFeatureSlug, ninjutsu_specialist.go) — awakenedScrollCap
// itself is hand-curated by level, no class_level_resources chart needed.
func seedScribeMasterCatalog(t *testing.T, s *server) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/ninjutsu-specialist', 'Ninjutsu Specialist', 8, 10)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/ninjutsu-specialist/group/ninjutsu-focus', 'class/ninjutsu-specialist', 'Ninjutsu Focus')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master', 'class/ninjutsu-specialist/group/ninjutsu-focus', 'Scribe Master')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master', 'Awakened Scroll', 2, 'Your scroll has open jutsu seals.', 1)`,
		awakenedScrollFeatureSlug)
}

// seedScribeMasterCharacter inserts a character with the given Ninjutsu
// Specialist level, already 3rd-level-subclassed into Scribe Master, plus
// one known Ninjutsu (jutsu/test-ninjutsu) for the Awakened Scroll picker
// to offer.
func seedScribeMasterCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/test-ninjutsu', 'Test Ninjutsu', 'Ninjutsu', 'C', '1 Action', '30 ft', 'Instant', 'HS', 'Cost: 2', 'Ninjutsu',
		        'A test ninjutsu.')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/ninjutsu-specialist', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (?, 'jutsu/test-ninjutsu')`, id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedPlainNinjutsuSpecialistCharacter inserts a plain Ninjutsu Specialist
// character with no subclass chosen — used by the EmptyHint test, which
// only needs the base class seeded without triggering Scribe Master's own
// Awakened Scroll gate.
func seedPlainNinjutsuSpecialistCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/ninjutsu-specialist', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAwakenedScrollPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedScribeMasterCatalog(t, s)
	id := seedPlainNinjutsuSpecialistCharacter(t, s, "Sai", 1) // no subclass -> feature not granted

	w := getPopup(t, s, awakenedScrollPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no Awakened Scroll seals yet") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

func TestAwakenedScrollPopupRenderAddDelete(t *testing.T) {
	s := testServer(t)
	seedScribeMasterCatalog(t, s)
	id := seedScribeMasterCharacter(t, s, "Sai", 6)
	popupPath := awakenedScrollPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Test Ninjutsu") {
		t.Errorf("initial render missing the available known Ninjutsu option:\n%s", body)
	}

	jutsuIDStr := knownJutsuIDFromBody(t, body, "Test Ninjutsu")

	addW := postPopupForm(t, s, popupPath+"/add", url.Values{"jutsu_id": {jutsuIDStr}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListNinjutsuJutsuPicks(s.charDB, id, charstore.NinjutsuPickAwakenedScroll)
	if err != nil {
		t.Fatal(err)
	}
	jutsuID, _ := strconv.ParseInt(jutsuIDStr, 10, 64)
	if len(picks) != 1 || picks[0] != jutsuID {
		t.Fatalf("picks after add = %+v, want one row for jutsu id %d", picks, jutsuID)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Ninjutsu") {
		t.Errorf("popup after add missing Test Ninjutsu in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/delete", url.Values{"jutsu_id": {jutsuIDStr}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListNinjutsuJutsuPicks(s.charDB, id, charstore.NinjutsuPickAwakenedScroll)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// knownJutsuIDFromBody extracts the jutsu_id radio value tied to name from a
// rendered popup body — Awakened Scroll/Expert Combatant both key their
// picker off the character_jutsu row id (assigned by SQLite, not a fixed
// constant), so tests read it back out of the rendered HTML rather than
// hard-coding it.
func knownJutsuIDFromBody(t *testing.T, body, name string) string {
	t.Helper()
	marker := `value="`
	nameIdx := strings.Index(body, name)
	if nameIdx == -1 {
		t.Fatalf("body missing jutsu name %q:\n%s", name, body)
	}
	// The radio's own value="N" attribute sits just before the jutsu's name
	// in this popup's markup (<input type="radio" name="jutsu_id" value="N"
	// required> Name ...) — search backward from the name for the nearest
	// preceding value="...".
	head := body[:nameIdx]
	idx := strings.LastIndex(head, marker)
	if idx == -1 {
		t.Fatalf("no value=\"...\" found before jutsu name %q:\n%s", name, body)
	}
	rest := head[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		t.Fatalf("malformed value attribute before jutsu name %q:\n%s", name, body)
	}
	return rest[:end]
}

// --- Combat Medic --------------------------------------------------------

// seedCombatMedicCatalog seeds enough rules.db content for a Combat
// Medic's own Fighting Stance/Expert Combatant pickers to render: the class
// itself, its Tenets of Medicine subclass chain, and one known Taijutsu
// jutsu (Expert Combatant's own catalog source, loadKnownExpertCombatantJutsu)
// — expertCombatantCap itself is hand-curated by level, no
// class_level_resources chart needed. seedTaijutsuStanceCatalog
// (taijutsu_subclass_popups_test.go, same package) seeds the fighting_
// stances catalog Martial Competency's own picker reads.
func seedCombatMedicCatalog(t *testing.T, s *server) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/medical-nin', 'Medical-Nin', 8, 8)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/medical-nin/group/tenets-of-medicine', 'class/medical-nin', 'Tenets of Medicine')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/medical-nin/group/tenets-of-medicine/combat-medic', 'class/medical-nin/group/tenets-of-medicine', 'Combat Medic')`)
	mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/test-taijutsu', 'Test Taijutsu Strike', 'Taijutsu', 'C', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1', 'Taijutsu',
		        'A test taijutsu strike.')`)
	seedTaijutsuStanceCatalog(t, s)
}

// seedCombatMedicCharacter inserts a character with the given Medical-Nin
// level, already 2nd-level-subclassed into Combat Medic, plus one known
// Taijutsu jutsu for Expert Combatant's own picker to offer.
func seedCombatMedicCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 10, 14, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/medical-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/medical-nin/group/tenets-of-medicine/combat-medic', 2)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (?, 'jutsu/test-taijutsu')`, id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedPlainMedicalNinCharacter inserts a plain Medical-Nin character with
// no subclass chosen — used by the EmptyHint test, which only needs the
// base class seeded without triggering Combat Medic's own gate.
func seedPlainMedicalNinCharacter(t *testing.T, s *server, name string, level int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 10, 14, 10)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/medical-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCombatMedicPopupEmptyHint(t *testing.T) {
	s := testServer(t)
	seedCombatMedicCatalog(t, s)
	id := seedPlainMedicalNinCharacter(t, s, "Sakura", 1) // no subclass -> not a Combat Medic

	w := getPopup(t, s, combatMedicPopupPath(id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Martial Competency and Expert Combatant are Combat Medic") {
		t.Errorf("body missing empty hint:\n%s", w.Body.String())
	}
}

// TestCombatMedicPopupRenderSetAddDelete covers the round trip through the
// real registered routes for both Fighting Stance (a freely re-editable
// <select>, set not add/delete) and Expert Combatant (a Known/Available
// jutsu_id picker) — the two independently level-gated sub-panels this
// popup bundles.
func TestCombatMedicPopupRenderSetAddDelete(t *testing.T) {
	s := testServer(t)
	seedCombatMedicCatalog(t, s)
	id := seedCombatMedicCharacter(t, s, "Sakura", 9) // 9th level: Stance (2nd) and Expert Combatant (9th) both open
	popupPath := combatMedicPopupPath(id)

	body := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(body, `class="character-reference-page subclass-scope"`) {
		t.Errorf("popup page missing the subclass-scope wrapper:\n%s", body)
	}
	if !strings.Contains(body, "Iron Fist Stance") || !strings.Contains(body, "Whirlwind Stance") {
		t.Errorf("initial render missing the Fighting Stance options:\n%s", body)
	}
	if !strings.Contains(body, "Test Taijutsu Strike") {
		t.Errorf("initial render missing the available Expert Combatant option:\n%s", body)
	}

	stanceW := postPopupForm(t, s, popupPath+"/fighting-stance", url.Values{"stance_slug": {"stance/iron-fist-stance"}})
	if stanceW.Code != http.StatusSeeOther {
		t.Fatalf("fighting stance set: status %d, body %q", stanceW.Code, stanceW.Body.String())
	}
	afterStance := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterStance, `value="stance/iron-fist-stance" data-description="Your unarmed strikes deal extra damage." selected`) {
		t.Errorf("Fighting Stance select should show its pick pre-selected:\n%s", afterStance)
	}

	jutsuIDStr := knownJutsuIDFromBody(t, afterStance, "Test Taijutsu Strike")

	addW := postPopupForm(t, s, popupPath+"/expert-combatant/add", url.Values{"jutsu_id": {jutsuIDStr}})
	if addW.Code != http.StatusSeeOther {
		t.Fatalf("expert combatant add: status %d, body %q", addW.Code, addW.Body.String())
	}
	picks, err := charstore.ListMedicalNinExpertCombatantPicks(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	jutsuID, _ := strconv.ParseInt(jutsuIDStr, 10, 64)
	if len(picks) != 1 || picks[0] != jutsuID {
		t.Fatalf("picks after add = %+v, want one row for jutsu id %d", picks, jutsuID)
	}

	afterAdd := getPopup(t, s, popupPath).Body.String()
	if !strings.Contains(afterAdd, "Test Taijutsu Strike") {
		t.Errorf("popup after add missing Test Taijutsu Strike in Known:\n%s", afterAdd)
	}

	delW := postPopupForm(t, s, popupPath+"/expert-combatant/delete", url.Values{"jutsu_id": {jutsuIDStr}})
	if delW.Code != http.StatusSeeOther {
		t.Fatalf("expert combatant delete: status %d, body %q", delW.Code, delW.Body.String())
	}
	picks, err = charstore.ListMedicalNinExpertCombatantPicks(s.charDB, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("picks after delete = %+v, want none", picks)
	}
}

// --- Sidebar buttons for all 3 new popups -------------------------------

// TestOperativeTrapsAwakenedScrollCombatMedicSidebarButtons covers
// character_sheet.html's own gating for the 3 sidebar buttons this
// conversion added, and confirms the old inline pickers/routes they
// replaced are gone from the Core sheet — the same two checks
// TestNewSubclassTrackerSidebarButtons already makes for Science-Nin's own
// 5 popups.
func TestOperativeTrapsAwakenedScrollCombatMedicSidebarButtons(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, s *server) int64
		popupPath   func(int64) string
		buttonText  string
		goneAddText string // an old inline form's own action path — must not leak into the Core sheet, since the inline picker/route it replaced is gone
	}{
		{"Operative Traps", func(t *testing.T, s *server) int64 {
			seedOperativeTrapsCatalog(t, s)
			return seedTacticalStrategistCharacter(t, s, "Shikamaru", 6)
		}, operativeTrapsPopupPath, `tracked in the "Operative Traps" popup`, `/sheet/operative-traps"`},
		{"Awakened Scroll", func(t *testing.T, s *server) int64 {
			seedScribeMasterCatalog(t, s)
			return seedScribeMasterCharacter(t, s, "Sai", 6)
		}, awakenedScrollPopupPath, `tracked in the "Awakened Scroll" popup`, `/sheet/awakened-scroll"`},
		{"Combat Medic", func(t *testing.T, s *server) int64 {
			seedCombatMedicCatalog(t, s)
			return seedCombatMedicCharacter(t, s, "Sakura", 9)
		}, combatMedicPopupPath, `tracked in the "Combat Medic" popup`, `/sheet/expert-combatant"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			id := c.seed(t, s)
			idStr := strconv.FormatInt(id, 10)
			sheetBody := getPopup(t, s, "/characters/"+idStr).Body.String()
			if !strings.Contains(sheetBody, `href="`+c.popupPath(id)+`"`) {
				t.Errorf("Core sheet missing its own sidebar button (href=%q):\n%s", c.popupPath(id), sheetBody)
			}
			if !strings.Contains(sheetBody, c.buttonText) {
				t.Errorf("Core sheet missing pointer text %q:\n%s", c.buttonText, sheetBody)
			}
			if strings.Contains(sheetBody, c.goneAddText) {
				t.Errorf("Core sheet still contains inline picker form action %q; it should only point at the popup now", c.goneAddText)
			}
		})
	}
}
