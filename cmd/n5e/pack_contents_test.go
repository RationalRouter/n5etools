package main

import (
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// The five packs' descriptions, verbatim from rules.db.
const travelersPack = "Contents: Commoner's clothes, a Shinobi Waist Bag, a Shinobi Belt Pouch, " +
	"a Shinobi Leg Pouch, 1 bedroll, 2 glow rods, and 1 blank map scroll."

const explorersPack = "Contents: 1 bedroll, 5 glow rods, 5 heating pads, 1 Radio Link, " +
	"Commoner's clothes, 7 days of field rations, a Shinobi Backpack, and 50 feet of rope sealed in a scroll."

func seedPackContentItems(t *testing.T, s *server) {
	t.Helper()
	rows := []struct{ slug, name string }{
		{"gear/ordinary-clothing", "Ordinary Clothing"},
		{"gear/shinobi-waist-bag", "Shinobi Waist Bag"},
		{"gear/shinobi-belt-pouch", "Shinobi Belt Pouch"},
		{"gear/shinobi-leg-pouch", "Shinobi Leg Pouch"},
		{"gear/shinobi-backpack", "Shinobi Backpack"},
		{"gear/bedroll", "Bedroll"},
		{"gear/glow-rod", "Glow Rod"},
		{"gear/heating-pad", "Heating Pad"},
		{"gear/field-rations-1-day", "Field Rations (1 Day)"},
		{"gear/rope-50ft", "Rope (50 ft)"},
		{"tool/radio-link", "Radio Link"},
	}
	for _, r := range rows {
		if _, err := s.rulesDB.Exec(
			`INSERT OR IGNORE INTO equipment (slug, name, kind) VALUES (?, ?, 'gear')`, r.slug, r.name,
		); err != nil {
			t.Fatalf("seed %s: %v", r.slug, err)
		}
	}
}

func TestParsePackContents(t *testing.T) {
	srv := testServer(t)
	seedPackContentItems(t, srv)

	got, err := srv.parsePackContents(travelersPack)
	if err != nil {
		t.Fatal(err)
	}
	want := []startingEquipmentLine{
		{Text: "Commoner's clothes", Slug: "gear/ordinary-clothing", Quantity: 1},
		{Text: "a Shinobi Waist Bag", Slug: "gear/shinobi-waist-bag", Quantity: 1},
		{Text: "a Shinobi Belt Pouch", Slug: "gear/shinobi-belt-pouch", Quantity: 1},
		{Text: "a Shinobi Leg Pouch", Slug: "gear/shinobi-leg-pouch", Quantity: 1},
		{Text: "1 bedroll", Slug: "gear/bedroll", Quantity: 1},
		{Text: "2 glow rods", Slug: "gear/glow-rod", Quantity: 2},
		// No "map scroll" row exists, so this stays honest free text.
		{Text: "1 blank map scroll", Slug: "", Quantity: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The two segments whose printed number is not a count of items: seven days
// of one-day rations really is seven rows' worth, but "50 feet of rope" is one
// Rope (50 ft), not fifty of them.
func TestParsePackContentsCountOverrides(t *testing.T) {
	srv := testServer(t)
	seedPackContentItems(t, srv)

	got, err := srv.parsePackContents(explorersPack)
	if err != nil {
		t.Fatal(err)
	}
	byText := map[string]startingEquipmentLine{}
	for _, line := range got {
		byText[line.Text] = line
	}

	rations := byText["7 days of field rations"]
	if rations.Slug != "gear/field-rations-1-day" || rations.Quantity != 7 {
		t.Errorf("rations = %+v, want gear/field-rations-1-day x7", rations)
	}
	rope := byText["50 feet of rope sealed in a scroll"]
	if rope.Slug != "gear/rope-50ft" || rope.Quantity != 1 {
		t.Errorf("rope = %+v, want gear/rope-50ft x1", rope)
	}
}

// An item that is not a container must not offer to unpack into anything.
func TestParsePackContentsIgnoresNonContainers(t *testing.T) {
	srv := testServer(t)

	got, err := srv.parsePackContents("Grants +10 Bulk of carrying capacity.")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("non-container parsed as %d contents: %+v", len(got), got)
	}
}

// Unpacking replaces the pack: the container row goes, its contents arrive,
// and a second helping of the same item deepens the stack rather than adding
// a duplicate row.
func TestUnpackInventoryItem(t *testing.T) {
	srv := testServer(t)
	seedPackContentItems(t, srv)

	if _, err := srv.charDB.Exec(
		`INSERT INTO characters (id, name) VALUES (1, 'Unpacker')`); err != nil {
		t.Fatal(err)
	}
	res, err := srv.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'gear/travelers-pack', 1, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	rowID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'gear/bedroll', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	contents := []charstore.UnpackedItem{
		{Slug: "gear/bedroll", Quantity: 1},
		{Slug: "gear/glow-rod", Quantity: 2},
		{Text: "1 blank map scroll", Quantity: 1},
	}
	if err := charstore.UnpackInventoryItem(srv.charDB, 1, rowID, contents); err != nil {
		t.Fatal(err)
	}

	inventory, err := srv.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]inventoryRow{}
	var freeText []inventoryRow
	for _, row := range inventory {
		// A contents line with no matching catalogue slug now becomes its
		// own custom/ library entry (part 11) rather than a bare item_slug
		// IS NULL row — see UnpackInventoryItem's item.Slug == "" branch.
		if strings.HasPrefix(row.Slug, "custom/") {
			freeText = append(freeText, row)
			continue
		}
		if _, dup := bySlug[row.Slug]; dup {
			t.Errorf("%s appears in two rows — contents did not merge into the existing stack", row.Slug)
		}
		bySlug[row.Slug] = row
	}

	if _, still := bySlug["gear/travelers-pack"]; still {
		t.Error("the pack is still in the inventory after being unpacked")
	}
	if got := bySlug["gear/bedroll"].Quantity; got != 2 {
		t.Errorf("bedroll quantity = %d, want 2 (the one already held plus the one unpacked)", got)
	}
	if got := bySlug["gear/glow-rod"].Quantity; got != 2 {
		t.Errorf("glow rod quantity = %d, want 2", got)
	}
	if len(freeText) != 1 || !strings.Contains(freeText[0].Name, "map scroll") {
		t.Errorf("free-text contents = %+v, want the map scroll line", freeText)
	}
}
