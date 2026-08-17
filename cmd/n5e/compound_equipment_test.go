package main

import "testing"

// seedCompoundEquipmentItems inserts the equipment rows these cases name.
//
// testServer's rules DB is migrations-only, so it holds whatever the
// migrations INSERT and nothing that arrived through `n5e-ingest sheet` —
// armor/padded-cloth is one of the latter. Seeding explicitly keeps the test
// about the parser rather than about which rows happen to be born in a
// migration this month.
func seedCompoundEquipmentItems(t *testing.T, s *server) {
	t.Helper()
	rows := []struct{ slug, name, kind string }{
		{"toolkit/cooking-kit", "Cooking Kit", "toolkit"},
		{"toolkit/medicine-kit", "Medicine Kit", "toolkit"},
		{"toolkit/poison-kit", "Poison Kit", "toolkit"},
		{"tool/flash-tag", "Flash Tag", "tool"},
		{"tool/paper-bombs", "Paper Bombs", "tool"},
		{"tool/smoke-bomb", "Smoke Bomb", "tool"},
		{"armor/padded-cloth", "Padded Cloth", "armor"},
		{"gear/crafters-pack", "Crafter's Pack", "gear"},
		{"weapon/crossbow-hand", "Crossbow, Hand", "weapon"},
		{"scroll/d-rank-jutsu-scroll", "D-Rank Jutsu Scroll", "scroll"},
	}
	for _, r := range rows {
		if _, err := s.rulesDB.Exec(
			`INSERT OR IGNORE INTO equipment (slug, name, kind) VALUES (?, ?, ?)`,
			r.slug, r.name, r.kind,
		); err != nil {
			t.Fatalf("seed %s: %v", r.slug, err)
		}
	}
}

// Every input below is a verbatim class_equipment_options.description that
// the ingest left unresolved (item_slug NULL). These landed on the sheet as
// prose rows instead of resolved items; the expectations here are the real
// equipment rows each line names.
func TestParseCompoundEquipment(t *testing.T) {
	srv := testServer(t)
	seedCompoundEquipmentItems(t, srv)

	cases := []struct {
		desc  string
		slugs []string // "" for a part that legitimately stays free text
		texts []string
	}{
		{
			desc:  "Cooking Tools, Flash Tag, Paper Bomb",
			slugs: []string{"toolkit/cooking-kit", "tool/flash-tag", "tool/paper-bombs"},
			texts: []string{"Cooking Tools", "Flash Tag", "Paper Bomb"},
		},
		{
			desc:  "Padded Cloth, Poison Kit, and 1 smoke bombs",
			slugs: []string{"armor/padded-cloth", "toolkit/poison-kit", "tool/smoke-bomb"},
			texts: []string{"Padded Cloth", "Poison Kit", "1 smoke bombs"},
		},
		{
			desc:  "1 Flash tags, 1 Medicine Kit",
			slugs: []string{"tool/flash-tag", "toolkit/medicine-kit"},
			texts: []string{"1 Flash tags", "1 Medicine Kit"},
		},
		{
			desc:  "Crafter’s pack, and 1 paper bomb",
			slugs: []string{"gear/crafters-pack", "tool/paper-bombs"},
			texts: []string{"Crafter’s pack", "1 paper bomb"},
		},
		{
			desc:  "Ninjutsu Scroll (D-Rank)",
			slugs: []string{"scroll/d-rank-jutsu-scroll"},
			texts: []string{"Ninjutsu Scroll (D-Rank)"},
		},
		{
			// The rules have no ammunition row, so the second half is
			// honest free text rather than a wrong item.
			desc:  "a Hand crossbow and one stack of bolts",
			slugs: []string{"weapon/crossbow-hand", ""},
			texts: []string{"a Hand crossbow", "one stack of bolts"},
		},
		{
			// Not a possession at all — must not resolve to anything.
			desc:  "No Armor",
			slugs: []string{""},
			texts: []string{"No Armor"},
		},
	}

	for _, tc := range cases {
		lines, err := srv.parseCompoundEquipment(tc.desc)
		if err != nil {
			t.Fatalf("parseCompoundEquipment(%q): %v", tc.desc, err)
		}
		if len(lines) != len(tc.slugs) {
			t.Errorf("%q split into %d lines, want %d: %+v", tc.desc, len(lines), len(tc.slugs), lines)
			continue
		}
		for i, line := range lines {
			if line.Slug != tc.slugs[i] {
				t.Errorf("%q part %d (%q): slug = %q, want %q", tc.desc, i, line.Text, line.Slug, tc.slugs[i])
			}
			if line.Text != tc.texts[i] {
				t.Errorf("%q part %d: text = %q, want %q", tc.desc, i, line.Text, tc.texts[i])
			}
		}
	}
}

// Quantities come from the printed count on each part, not from the option's
// quantity column — "1 smoke bombs" is one smoke bomb even though the plural
// invites reading it as more.
func TestParseCompoundEquipmentQuantities(t *testing.T) {
	srv := testServer(t)
	seedCompoundEquipmentItems(t, srv)

	lines, err := srv.parseCompoundEquipment("Padded Cloth, Poison Kit, and 1 smoke bombs")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if line.Quantity != 1 {
			t.Errorf("%q: quantity = %d, want 1", line.Text, line.Quantity)
		}
	}
}
