package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

const (
	captainsPack     = "Contents: 1 Toolkit (pick one), Commoner's clothes, an empty book, writing utensils, " + "7 days of field rations, a compass, a Shinobi Leg Pouch, and a thermos."
	craftersPack     = "Contents: 3 Toolkits (pick three)."
	infiltratorsPack = "Contents: 1 Hackers Kit or Security Kit (pick one), 1 blank Keycard, " + "1 blank Data Scroll, and a Shinobi Waist Bag."
)

// seedPackToolkitChoiceCatalog seeds a small, real toolkit catalog — enough
// for detectPackToolkitChoice to validate the Infiltrator's Pack's scoped
// "Hackers Kit or Security Kit" choice against real rows, plus one extra
// toolkit (Poison Kit) so a generic "any toolkit" pick has more than one
// option to choose from.
func seedPackToolkitChoiceCatalog(t *testing.T, s *server) {
	t.Helper()
	for _, stmt := range []string{
		`INSERT OR IGNORE INTO equipment (slug, name, kind, description) VALUES
		 ('toolkit/hackers-kit', 'Hackers Kit', 'toolkit', 'Grants proficiency bonus to checks to hack electronic systems.')`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, description) VALUES
		 ('toolkit/security-kit', 'Security Kit', 'toolkit', 'Grants proficiency bonus to checks to bypass locks and security systems.')`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, description) VALUES
		 ('toolkit/poison-kit', 'Poison Kit', 'toolkit', 'Grants proficiency bonus to checks to craft or identify poisons.')`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, description) VALUES
		 ('toolkit/supreme-poison-kit', 'Supreme Poison Kit', 'toolkit', 'See Poison Kit.')`,
	} {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// TestParsePackContentsToolkitChoice covers the three packs the class/
// full-class audit flagged as still leaving a toolkit pick as inert free
// text: Crafter's (a bare "N Toolkits (pick N)"), Captain's (the same
// clause alongside otherwise-resolved items), and Infiltrator's (a scoped
// two-option "X or Y (pick one)").
func TestParsePackContentsToolkitChoice(t *testing.T) {
	s := testServer(t)
	seedPackContentItems(t, s)
	seedPackToolkitChoiceCatalog(t, s)

	t.Run("crafters pack: generic pick-three", func(t *testing.T) {
		got, err := s.parsePackContents(craftersPack)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d lines, want 1: %+v", len(got), got)
		}
		line := got[0]
		if !line.IsToolkitChoice || line.Slug != "" || line.Quantity != 3 || line.ToolkitChoiceOptions != "" {
			t.Errorf("crafters pack line = %+v, want IsToolkitChoice=true Quantity=3 ToolkitChoiceOptions=\"\"", line)
		}
	})

	t.Run("captains pack: pick-one alongside resolved items", func(t *testing.T) {
		got, err := s.parsePackContents(captainsPack)
		if err != nil {
			t.Fatal(err)
		}
		var found *startingEquipmentLine
		for i := range got {
			if got[i].Text == "1 Toolkit (pick one)" {
				found = &got[i]
			}
		}
		if found == nil {
			t.Fatalf("no \"1 Toolkit (pick one)\" line in %+v", got)
		}
		if !found.IsToolkitChoice || found.Slug != "" || found.Quantity != 1 || found.ToolkitChoiceOptions != "" {
			t.Errorf("captains pack toolkit line = %+v, want IsToolkitChoice=true Quantity=1 ToolkitChoiceOptions=\"\"", *found)
		}
		// The rest of the pack still resolves as before — this mechanism
		// must not regress a segment that already worked.
		var clothes bool
		for _, l := range got {
			if l.Slug == "gear/ordinary-clothing" {
				clothes = true
			}
		}
		if !clothes {
			t.Errorf("captains pack no longer resolves Commoner's clothes: %+v", got)
		}
	})

	t.Run("infiltrators pack: scoped two-option pick", func(t *testing.T) {
		got, err := s.parsePackContents(infiltratorsPack)
		if err != nil {
			t.Fatal(err)
		}
		var found *startingEquipmentLine
		for i := range got {
			if strings.Contains(got[i].Text, "Hackers Kit or Security Kit") {
				found = &got[i]
			}
		}
		if found == nil {
			t.Fatalf("no Hackers-Kit-or-Security-Kit line in %+v", got)
		}
		if !found.IsToolkitChoice || found.Slug != "" || found.Quantity != 1 {
			t.Errorf("infiltrators pack toolkit line = %+v, want IsToolkitChoice=true Quantity=1", *found)
		}
		if found.ToolkitChoiceOptions != "Hackers Kit|Security Kit" {
			t.Errorf("ToolkitChoiceOptions = %q, want \"Hackers Kit|Security Kit\"", found.ToolkitChoiceOptions)
		}
	})
}

// TestParsePackContentsToolkitOrChoiceRequiresRealToolkits confirms
// detectPackToolkitChoice's honesty guard: a structural "X or Y (pick one)"
// match whose two names are NOT both real toolkits must stay ordinary,
// unresolved free text rather than being silently treated as a toolkit
// pick — an "X or Y" pick about anything else stays honest, same bargain
// resolveStartingItem already makes for a possession with no matching row.
func TestParsePackContentsToolkitOrChoiceRequiresRealToolkits(t *testing.T) {
	s := testServer(t)

	got, err := s.parsePackContents("Contents: 1 Frobnicator or Turboencabulator (pick one).")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1: %+v", len(got), got)
	}
	if got[0].IsToolkitChoice {
		t.Errorf("line %+v flagged as a toolkit choice with no real toolkit rows behind either name", got[0])
	}
}

// TestUnpackInventoryItemToolkitChoicePlaceholder confirms
// UnpackInventoryItem tags a toolkit-choice line's placeholder row so it can
// be found later (charstore.DecodePackToolkitChoiceNotes), for both the
// generic and scoped shapes.
func TestUnpackInventoryItemToolkitChoicePlaceholder(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Unpacker')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'gear/crafters-pack', 1, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	rowID, _ := res.LastInsertId()

	contents := []charstore.UnpackedItem{
		{Text: "3 Toolkits (pick three)", Quantity: 3, IsToolkitChoice: true},
	}
	if err := charstore.UnpackInventoryItem(s.charDB, 1, rowID, contents); err != nil {
		t.Fatal(err)
	}

	var itemSlug, notes string
	var quantity int
	if err := s.charDB.QueryRow(
		`SELECT item_slug, quantity, notes FROM character_inventory WHERE character_id = 1`,
	).Scan(&itemSlug, &quantity, &notes); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(itemSlug, "custom/") {
		t.Errorf("item_slug = %q, want a custom/ slug", itemSlug)
	}
	if quantity != 3 {
		t.Errorf("quantity = %d, want 3", quantity)
	}
	options, ok := charstore.DecodePackToolkitChoiceNotes(notes)
	if !ok || options != "" {
		t.Errorf("notes = %q decoded to (%q, %v), want (\"\", true)", notes, options, ok)
	}
}

// TestResolvePackToolkitChoice drives the whole generic pick-three placeholder
// (Crafter's Pack's shape) down to zero, one slot at a time, verifying each
// resolution both lands a real, mergeable inventory item and grants a
// character_proficiencies row — the two effects a bare free-text line never
// had.
func TestResolvePackToolkitChoice(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Unpacker')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'gear/crafters-pack', 1, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	rowID, _ := res.LastInsertId()
	if err := charstore.UnpackInventoryItem(s.charDB, 1, rowID,
		[]charstore.UnpackedItem{{Text: "3 Toolkits (pick three)", Quantity: 3, IsToolkitChoice: true}},
	); err != nil {
		t.Fatal(err)
	}

	var placeholderID int64
	if err := s.charDB.QueryRow(
		`SELECT id FROM character_inventory WHERE character_id = 1 AND notes = ?`, charstore.PackToolkitChoiceNotesTag,
	).Scan(&placeholderID); err != nil {
		t.Fatal(err)
	}

	picks := []struct{ slug, name string }{
		{"toolkit/poison-kit", "Poison Kit"},
		{"toolkit/hackers-kit", "Hackers Kit"},
		{"toolkit/poison-kit", "Poison Kit"}, // repeat pick — must deepen the stack, not duplicate the row
	}
	for i, p := range picks {
		if err := charstore.ResolvePackToolkitChoice(s.charDB, 1, placeholderID, p.slug, p.name); err != nil {
			t.Fatalf("pick %d (%s): %v", i, p.slug, err)
		}
	}

	// The placeholder is exhausted (3 picks consumed) and gone.
	var remaining int
	err = s.charDB.QueryRow(`SELECT COUNT(*) FROM character_inventory WHERE id = ?`, placeholderID).Scan(&remaining)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("placeholder row still present after its last pick")
	}

	var poisonQty int
	if err := s.charDB.QueryRow(
		`SELECT quantity FROM character_inventory WHERE character_id = 1 AND item_slug = 'toolkit/poison-kit'`,
	).Scan(&poisonQty); err != nil {
		t.Fatal(err)
	}
	if poisonQty != 2 {
		t.Errorf("poison kit quantity = %d, want 2 (picked twice, merged into one stack)", poisonQty)
	}

	rows, err := s.charDB.Query(
		`SELECT value, source_kind, source_ref FROM character_proficiencies WHERE character_id = 1 AND kind = 'tool' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var value, sourceKind, sourceRef string
		if err := rows.Scan(&value, &sourceKind, &sourceRef); err != nil {
			t.Fatal(err)
		}
		if sourceKind != "pack" {
			t.Errorf("source_kind = %q, want \"pack\"", sourceKind)
		}
		got = append(got, value+"|"+sourceRef)
	}
	want := []string{"Poison Kit|toolkit/poison-kit", "Hackers Kit|toolkit/hackers-kit", "Poison Kit|toolkit/poison-kit"}
	if len(got) != len(want) {
		t.Fatalf("got %d proficiency rows %v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("proficiency row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestResolvePackToolkitChoiceRejectsNonPlaceholderRow confirms the
// resolver's own guard: it must refuse to touch a character_inventory row
// that isn't actually a pending pack toolkit choice (no notes tag at all,
// or already resolved/deleted), the same "row wasn't there" contract
// UnpackInventoryItem's own container check uses.
func TestResolvePackToolkitChoiceRejectsNonPlaceholderRow(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Unpacker')`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'gear/bedroll', 1, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	rowID, _ := res.LastInsertId()

	if err := charstore.ResolvePackToolkitChoice(s.charDB, 1, rowID, "toolkit/poison-kit", "Poison Kit"); err == nil {
		t.Fatal("expected an error resolving against an ordinary inventory row, got nil")
	}

	if err := charstore.ResolvePackToolkitChoice(s.charDB, 1, 99999, "toolkit/poison-kit", "Poison Kit"); err == nil {
		t.Fatal("expected an error resolving against a nonexistent row, got nil")
	}
}

// TestHandleSheetPackToolkitChoiceEndToEnd drives the full HTTP path: unpack
// a Crafter's Pack via the real handler, confirm the pending picker sees
// three remaining picks with the already-held toolkit excluded, resolve one
// pick via the real handler, and confirm both the inventory item and the
// tool proficiency landed and the pending list now shows two remaining.
func TestHandleSheetPackToolkitChoiceEndToEnd(t *testing.T) {
	s := testServer(t)
	seedPackToolkitChoiceCatalog(t, s)
	// gear/crafters-pack is real seeded rules content (see
	// internal/schema/migrations/rules/0017_adventuring_gear.sql) — no need
	// to insert it, and re-inserting it would collide with that row.

	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Crafter')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	// The character already has Hackers Kit proficiency from another
	// source — the pending picker's options must exclude it.
	if _, err := s.charDB.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
		 VALUES (?, 'tool', 'Hackers Kit', 'class', 'class/science-nin')`, id,
	); err != nil {
		t.Fatal(err)
	}

	invRes, err := s.charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES (?, 'gear/crafters-pack', 1, 0)`, id)
	if err != nil {
		t.Fatal(err)
	}
	packRowID, _ := invRes.LastInsertId()

	unpackReq := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/sheet/inventory/"+strconv.FormatInt(packRowID, 10)+"/unpack", nil)
	unpackReq.SetPathValue("id", idStr)
	unpackReq.SetPathValue("rowID", strconv.FormatInt(packRowID, 10))
	unpackW := httptest.NewRecorder()
	s.handleSheetInventoryUnpack(unpackW, unpackReq)
	if unpackW.Code != http.StatusSeeOther {
		t.Fatalf("unpack status = %d, body:\n%s", unpackW.Code, unpackW.Body.String())
	}

	pending, err := s.buildPendingPackToolkitChoiceRows(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d pending rows, want 1: %+v", len(pending), pending)
	}
	if !strings.Contains(pending[0].Label, "3 remaining") {
		t.Errorf("label = %q, want it to mention 3 remaining", pending[0].Label)
	}
	for _, o := range pending[0].Options {
		if o.Label == "Hackers Kit" {
			t.Errorf("options %+v still offer the already-held Hackers Kit", pending[0].Options)
		}
	}
	var poisonOption featureChoiceOption
	found := false
	for _, o := range pending[0].Options {
		if o.Value == "toolkit/poison-kit" {
			poisonOption, found = o, true
		}
	}
	if !found {
		t.Fatalf("Poison Kit not offered: %+v", pending[0].Options)
	}
	if poisonOption.Description == "" {
		t.Error("Poison Kit option carries no rollover-tooltip description")
	}

	// An unoffered toolkit must be rejected server-side even if the request
	// is hand-built — mirrors the equipment step's own held-toolkit re-check.
	badReq := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/sheet/inventory/pack-toolkit-choice",
		strings.NewReader(url.Values{"row_id": {strconv.FormatInt(pending[0].RowID, 10)}, "toolkit_slug": {"toolkit/hackers-kit"}}.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.SetPathValue("id", idStr)
	badW := httptest.NewRecorder()
	s.handleSheetPackToolkitChoice(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Errorf("rejecting an unoffered toolkit: status = %d, want 400", badW.Code)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/sheet/inventory/pack-toolkit-choice",
		strings.NewReader(url.Values{"row_id": {strconv.FormatInt(pending[0].RowID, 10)}, "toolkit_slug": {"toolkit/poison-kit"}}.Encode()))
	resolveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resolveReq.SetPathValue("id", idStr)
	resolveW := httptest.NewRecorder()
	s.handleSheetPackToolkitChoice(resolveW, resolveReq)
	if resolveW.Code != http.StatusSeeOther {
		t.Fatalf("resolve status = %d, body:\n%s", resolveW.Code, resolveW.Body.String())
	}

	inventory, err := s.loadCharacterInventory(id)
	if err != nil {
		t.Fatal(err)
	}
	var gotPoisonKit bool
	for _, row := range inventory {
		if row.Slug == "toolkit/poison-kit" {
			gotPoisonKit = true
		}
	}
	if !gotPoisonKit {
		t.Errorf("Poison Kit never landed as a real inventory row: %+v", inventory)
	}

	var profCount int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = ? AND kind = 'tool' AND value = 'Poison Kit' AND source_kind = 'pack'`,
		id,
	).Scan(&profCount); err != nil {
		t.Fatal(err)
	}
	if profCount != 1 {
		t.Errorf("Poison Kit tool proficiency rows = %d, want 1", profCount)
	}

	pending, err = s.buildPendingPackToolkitChoiceRows(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || !strings.Contains(pending[0].Label, "2 remaining") {
		t.Fatalf("pending after one pick = %+v, want a single row mentioning 2 remaining", pending)
	}
}

// TestPendingPackToolkitChoiceAppearsOnFullPageLoad is a regression test for
// a bug caught live-testing this feature: character_sheet.html's top-level
// {{template "sheet_feature_choices" (dict ...)}} call builds a brand-new
// map for that sub-template, so a field not explicitly listed in the dict is
// invisible inside it even though handleCharacterSheet's own top-level data
// map has it under the same key — the exact shape TestCustomResourceAppearsOnFullPageLoad
// (custom_resources_test.go) already regression-tests for a different field.
// A Crafter's Pack unpack correctly left a pending placeholder row
// (buildPendingPackToolkitChoiceRows saw it fine, confirmed by every other
// test in this file, which all call that function directly or drive the
// fragment-render path) but the picker never appeared in the actual
// rendered HTML on a plain page load — only immediately after the unpack
// action's own fetch-based fragment refresh, which goes through
// renderSheetFragment's "sheet_feature_choices" case, a different code path
// that does pass the field through. This test goes through the real
// full-page handler, not the fragment one, so it would have caught the gap.
func TestPendingPackToolkitChoiceAppearsOnFullPageLoad(t *testing.T) {
	s := testServer(t)
	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Unpacker')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	invRes, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (?, 'gear/crafters-pack', 1, 0)`, id)
	if err != nil {
		t.Fatal(err)
	}
	packRowID, _ := invRes.LastInsertId()
	if err := charstore.UnpackInventoryItem(s.charDB, id, packRowID,
		[]charstore.UnpackedItem{{Text: "3 Toolkits (pick three)", Quantity: 3, IsToolkitChoice: true}},
	); err != nil {
		t.Fatal(err)
	}

	idStr := strconv.FormatInt(id, 10)
	req := httptest.NewRequest(http.MethodGet, "/characters/"+idStr, nil)
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Pick a Toolkit") {
		t.Error("full-page render doesn't show the pending pack toolkit choice picker")
	}
	if !strings.Contains(w.Body.String(), "3 remaining") {
		t.Error("full-page render doesn't show the pending pick's remaining count")
	}
}
