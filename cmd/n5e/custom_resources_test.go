package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

func TestComputeCustomResourcesCCDUsesOwnClassLevel(t *testing.T) {
	features := []grantedFeatureRow{
		{Slug: "class/science-nin/feature/chakra-containment-device", Name: "Chakra Containment Device", Level: 2},
	}
	// Multiclass character: 4 levels of Science-Nin, 6 of something else.
	// CCD's Max must scale off Science-Nin's own level (4*15=60), not the
	// character's total level (10).
	classLevels := map[string]int{"class/science-nin": 4, "class/other-class": 6}

	entries := computeCustomResources(features, classLevels, 2 /* conMod */, 0, 10 /* characterLevel */, nil)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want exactly 1", entries)
	}
	if entries[0].Key != "ccd" || entries[0].Max != 60 {
		t.Errorf("got %+v, want ccd with Max=60", entries[0])
	}
	if entries[0].Current != 60 {
		t.Errorf("Current = %d, want Max (no stored value yet) = 60", entries[0].Current)
	}
}

func TestComputeCustomResourcesCCDGatedByMinLevel(t *testing.T) {
	features := []grantedFeatureRow{
		{Slug: "class/science-nin/feature/chakra-containment-device", Name: "Chakra Containment Device", Level: 2},
	}
	classLevels := map[string]int{"class/science-nin": 1}

	entries := computeCustomResources(features, classLevels, 0, 0, 1, nil)
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want none below CCD's MinLevel 2", entries)
	}
}

func TestComputeCustomResourcesPerformanceScroll(t *testing.T) {
	features := []grantedFeatureRow{
		{Slug: "class/puppet-master/group/puppet-techniques/red-technique-performer/feature/performance-of-10-puppets", Name: "Performance of 10 Puppets", Level: 10},
	}
	classLevels := map[string]int{"class/puppet-master": 10}

	// Below MinLevel 10: no entry at all.
	entries := computeCustomResources(features, classLevels, 0, 0, 9, nil)
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want none below MinLevel 10", entries)
	}

	// At MinLevel 10: a single-charge resource, "once per long rest."
	entries = computeCustomResources(features, classLevels, 0, 0, 10, nil)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want exactly 1", entries)
	}
	if entries[0].Key != "performance_scroll" || entries[0].Max != 1 {
		t.Errorf("got %+v, want performance_scroll with Max=1", entries[0])
	}
	if entries[0].ShortRegen != regenNone || entries[0].LongRegen != regenFull {
		t.Errorf("regen = short:%v long:%v, want short:regenNone long:regenFull", entries[0].ShortRegen, entries[0].LongRegen)
	}
}

func TestComputeCustomResourcesWhiteChakraSurgeStacksOntoBaseGrant(t *testing.T) {
	base := grantedFeatureRow{Slug: "clan/hatake/feature/white-chakra", Name: "White Chakra", Level: 1}
	surge := grantedFeatureRow{Slug: "feat/hatake/white-chakra-surge", Name: "White Chakra Surge", Level: 4}
	classLevels := map[string]int{"class/hatake-something": 10}

	// Base grant alone: 5 + level.
	entries := computeCustomResources([]grantedFeatureRow{base}, classLevels, 0, 0, 10, nil)
	if len(entries) != 1 || entries[0].Key != "white_chakra" || entries[0].Max != 15 {
		t.Fatalf("base alone: got %+v, want white_chakra Max=15", entries)
	}

	// Base + Surge: Surge's own formula (5 + 2*level = 25) wins since it's
	// higher, but there is still only ONE white_chakra entry, not two.
	entries = computeCustomResources([]grantedFeatureRow{base, surge}, classLevels, 0, 0, 10, nil)
	if len(entries) != 1 {
		t.Fatalf("base+surge: got %d entries, want 1 combined white_chakra entry: %+v", len(entries), entries)
	}
	if entries[0].Max != 25 {
		t.Errorf("base+surge: Max = %d, want 25 (Surge's higher formula wins)", entries[0].Max)
	}
	if entries[0].Restriction == "" {
		t.Errorf("Restriction should carry over from the winning grant, got empty")
	}
}

func TestComputeCustomResourcesStarChakraAndCalories(t *testing.T) {
	star := grantedFeatureRow{Slug: "clan/hoshi/feature/star-chakra", Name: "Star Chakra", Level: 1}
	classLevels := map[string]int{"class/whatever": 5}

	// CON mod 3, level 5 -> Max 8.
	entries := computeCustomResources([]grantedFeatureRow{star}, classLevels, 3, 0, 5, nil)
	if len(entries) != 1 || entries[0].Max != 8 {
		t.Fatalf("got %+v, want star_chakra Max=8", entries)
	}

	// A negative CON mod is floored at 1 for the Max formula (book text:
	// "your constitution modifier (min 1)").
	entries = computeCustomResources([]grantedFeatureRow{star}, classLevels, -2, 0, 5, nil)
	if entries[0].Max != 6 {
		t.Errorf("Max = %d, want 6 (CON mod floored to 1, +5 level)", entries[0].Max)
	}

	calories := grantedFeatureRow{Slug: "clan/akimichi/feature/calories", Name: "Calories", Level: 1}
	entries = computeCustomResources([]grantedFeatureRow{calories}, classLevels, 3, 0, 5, nil)
	if len(entries) != 1 || entries[0].Max != 8 {
		t.Fatalf("got %+v, want calories Max=8", entries)
	}
}

func TestComputeCustomResourcesReserveCellsAndChakraBarrier(t *testing.T) {
	reserve := grantedFeatureRow{Slug: "clan/uzumaki/feature/chakra-reserves", Name: "Chakra Reserves", Level: 1}
	classLevels := map[string]int{"class/whatever": 7}

	entries := computeCustomResources([]grantedFeatureRow{reserve}, classLevels, 0, 0, 7, nil)
	if len(entries) != 1 || entries[0].Max != 7 {
		t.Fatalf("got %+v, want reserve_cells Max=7", entries)
	}

	barrier := grantedFeatureRow{
		Slug:  "class/scout-nin/group/scouting-technique/barrier-scout/feature/chakra-barrier",
		Name:  "Chakra Barrier",
		Level: 3,
	}
	// Below MinLevel 3, not granted at all.
	entries = computeCustomResources([]grantedFeatureRow{barrier}, classLevels, 0, 0, 2, nil)
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want none below Chakra Barrier's MinLevel 3", entries)
	}
	entries = computeCustomResources([]grantedFeatureRow{barrier}, classLevels, 0, 0, 7, nil)
	if len(entries) != 1 || entries[0].Max != 14 {
		t.Fatalf("got %+v, want chakra_barrier Max=14 (2x level 7)", entries)
	}
}

func TestComputeCustomResourcesStoredCurrentClampsToMax(t *testing.T) {
	star := grantedFeatureRow{Slug: "clan/hoshi/feature/star-chakra", Name: "Star Chakra", Level: 1}
	classLevels := map[string]int{"class/whatever": 1}
	// Max is 1 (con 0 floored to 1) + level 1 = 2, but a stale stored value
	// from before a level/ability change claims 99 — must clamp to Max.
	entries := computeCustomResources([]grantedFeatureRow{star}, classLevels, 0, 0, 1, map[string]int{"star_chakra": 99})
	if entries[0].Current != entries[0].Max {
		t.Errorf("Current = %d, want clamped to Max = %d", entries[0].Current, entries[0].Max)
	}
}

func TestComputeCustomResourcesHuntersExploitsUsesProfBonus(t *testing.T) {
	exploits := grantedFeatureRow{Slug: "class/hunter-nin/feature/hunters-exploits", Name: "Hunters Exploits", Level: 3}
	classLevels := map[string]int{"class/hunter-nin": 10}

	entries := computeCustomResources([]grantedFeatureRow{exploits}, classLevels, 0, 6 /* profBonus */, 10, nil)
	if len(entries) != 1 || entries[0].Key != huntersExploitsResourceKey || entries[0].Max != 6 {
		t.Fatalf("got %+v, want %s Max=6 (Proficiency Bonus, not level-derived)", entries, huntersExploitsResourceKey)
	}
	if entries[0].ShortRegen != regenFull {
		t.Errorf("ShortRegen = %v, want regenFull — \"a number of times... per Short Rest\"", entries[0].ShortRegen)
	}
}

func TestApplyRestRegen(t *testing.T) {
	tests := []struct {
		name              string
		kind              restRegen
		current, max, con int
		want              int
	}{
		{"conMod gains and clamps", regenConMod, 2, 10, 3, 5},
		{"conMod clamps at max", regenConMod, 9, 10, 5, 10},
		{"negative conMod gains nothing", regenConMod, 2, 10, -2, 2},
		{"halfSpent regains half of what's missing", regenHalfSpent, 4, 10, 0, 7}, // spent=6, +3
		{"halfSpent floors odd amounts", regenHalfSpent, 5, 10, 0, 7},             // spent=5, +2 (floor)
		{"halfMax resets to half of max", regenHalfMax, 0, 20, 0, 10},
		{"halfMax never decreases an already-higher current", regenHalfMax, 15, 20, 0, 15},
		{"full resets straight to max", regenFull, 3, 20, 0, 20},
		{"none leaves current untouched", regenNone, 7, 20, 5, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyRestRegen(tt.kind, tt.current, tt.max, tt.con)
			if got != tt.want {
				t.Errorf("applyRestRegen(%v, %d, %d, %d) = %d, want %d", tt.kind, tt.current, tt.max, tt.con, got, tt.want)
			}
		})
	}
}

func TestValidCustomResourceKey(t *testing.T) {
	if !validCustomResourceKey("ccd") {
		t.Error("ccd should be a valid key")
	}
	if validCustomResourceKey("not-a-real-resource") {
		t.Error("an unknown key should not validate")
	}
}

// TestHandleSheetCustomResourceFirstSpendSeedsFromMax is a regression test
// for a bug caught live-testing this feature: a resource's very first spend
// (no character_custom_resources row yet) must land relative to its own Max,
// not 0. A prior version applied the delta as a raw SQL upsert with no
// baseline to add against, so a level-3 Science-Nin's first -20 CCD spend
// (Max 45) produced a stored value of 0 instead of 25.
func TestHandleSheetCustomResourceFirstSpendSeedsFromMax(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 10)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description, sort_order) VALUES
		('class/science-nin/feature/chakra-containment-device', 'class/science-nin',
		 'Chakra Containment Device', 2, 'Starting at Level 2 you learn to create a CCD.', 1)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Test Science-Nin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/science-nin', 3, 0)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/resource/ccd",
		strings.NewReader(url.Values{"delta": {"-20"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	req.SetPathValue("key", "ccd")
	w := httptest.NewRecorder()
	s.handleSheetCustomResource(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("spend ccd: status %d, body %s", w.Code, w.Body.String())
	}

	stored, err := charstore.GetCustomResources(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored["ccd"] != 25 {
		t.Errorf("stored ccd = %d, want 25 (Max 45 - 20)", stored["ccd"])
	}

	// A second spend now has a real baseline to add against.
	req2 := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/resource/ccd",
		strings.NewReader(url.Values{"delta": {"-10"}}.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("X-Requested-With", "fetch")
	req2.SetPathValue("id", "1")
	req2.SetPathValue("key", "ccd")
	w2 := httptest.NewRecorder()
	s.handleSheetCustomResource(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second spend ccd: status %d, body %s", w2.Code, w2.Body.String())
	}
	stored, err = charstore.GetCustomResources(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored["ccd"] != 15 {
		t.Errorf("stored ccd after second spend = %d, want 15", stored["ccd"])
	}

	// An unknown key 404s rather than inserting an arbitrary resource_key.
	badReq := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/resource/not-a-real-resource",
		strings.NewReader(url.Values{"delta": {"-1"}}.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.SetPathValue("id", "1")
	badReq.SetPathValue("key", "not-a-real-resource")
	badW := httptest.NewRecorder()
	s.handleSheetCustomResource(badW, badReq)
	if badW.Code != http.StatusNotFound {
		t.Errorf("spend on unknown key: status %d, want 404", badW.Code)
	}
}

// TestCustomResourceAppearsOnFullPageLoad is a regression test for a bug
// caught live-testing this feature: character_sheet.html's initial full-page
// render invokes {{template "sheet_vitals" (dict "ID" .ID "Sheet" .Sheet)}}
// — dict builds a brand-new map, so anything not explicitly listed (here,
// Concentration and CustomResources) is invisible inside that template
// even though handleCharacterSheet's own top-level data map has it. A
// granted CCD computed correctly (confirmed via computeCustomResources
// directly) but never appeared in the actual rendered HTML on a fresh page
// load — only after some OTHER action triggered a "sheet_vitals" fragment
// refresh, which goes through a different code path
// (renderSheetFragment) that does pass it through. This test goes through
// the real full-page handler, not the fragment one, so it would have
// caught the gap.
func TestCustomResourceAppearsOnFullPageLoad(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 10)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description, sort_order) VALUES
		('class/science-nin/feature/chakra-containment-device', 'class/science-nin',
		 'Chakra Containment Device', 2, 'Starting at Level 2 you learn to create a CCD.', 1)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Test Science-Nin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/science-nin', 3, 0)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "/characters/1/sheet/resource/ccd") {
		t.Errorf("full page render is missing the CCD resource box entirely; body did not contain its endpoint")
	}
	if !strings.Contains(body, "Chakra Containment Device") {
		t.Errorf("full page render is missing the CCD resource's label")
	}
}
