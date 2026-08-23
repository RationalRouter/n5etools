package main

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// TestParseStartingEquipment covers the part-7 report that starting
// equipment "is showing in the inventory as a single extracted line from the
// text": one sentence must become one line per possession, with the ones the
// rules know about resolved to real items and the money pulled out into the
// character's purse.
func TestParseStartingEquipment(t *testing.T) {
	s := testServer(t)
	for _, stmt := range []string{
		`INSERT OR IGNORE INTO equipment (slug, name, kind, cost_ryo) VALUES ('gear/wallet', 'Wallet', 'gear', 5)`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, cost_ryo) VALUES ('gear/ordinary-clothing', 'Ordinary Clothing', 'gear', 5)`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, cost_ryo) VALUES ('gear/fine-clothing', 'Fine Clothing', 'gear', 200)`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, cost_ryo) VALUES ('toolkit/poison-kit', 'Poison Kit', 'toolkit', 200)`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, cost_ryo) VALUES ('toolkit/supreme-poison-kit', 'Supreme Poison Kit', 'toolkit', 1200)`,
		`INSERT OR IGNORE INTO equipment (slug, name, kind, cost_ryo) VALUES ('scroll/blank-scroll', 'Blank Scroll', 'scroll', 75)`,
	} {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	cases := []struct {
		name    string
		text    string
		wantRyo float64
		want    []startingEquipmentLine
	}{
		{
			name:    "urchin",
			text:    "A Map of the Village you grew up in, A Token of intimate value to you, Set of Basic Clothing, Wallet containing 100 Ryo",
			wantRyo: 100,
			want: []startingEquipmentLine{
				{Text: "A Map of the Village you grew up in", Quantity: 1},
				{Text: "A Token of intimate value to you", Quantity: 1},
				{Text: "Set of Basic Clothing", Slug: "gear/ordinary-clothing", Quantity: 1},
				{Text: "Wallet", Slug: "gear/wallet", Quantity: 1},
			},
		},
		{
			// No container word at all — the money is its own segment.
			name:    "hermit",
			text:    "A diary of your experiences, a winter blanket, a set of common clothes, a poison kit, and 100 Ryo",
			wantRyo: 100,
			want: []startingEquipmentLine{
				{Text: "A diary of your experiences", Quantity: 1},
				{Text: "a winter blanket", Quantity: 1},
				{Text: "a set of common clothes", Slug: "gear/ordinary-clothing", Quantity: 1},
				{Text: "a poison kit", Slug: "toolkit/poison-kit", Quantity: 1},
			},
		},
		{
			// The 10 Ryo here is what a piece of jewellery is WORTH, not
			// money in hand: only the pouch's 100 may reach the purse.
			name:    "traveler",
			text:    "a small piece of jewelry worth 10 Ryo in the style of your homeland's craftsmanship, and a pouch containing 100 Ryo",
			wantRyo: 100,
			want: []startingEquipmentLine{
				{Text: "a small piece of jewelry worth 10 Ryo in the style of your homeland's craftsmanship", Quantity: 1},
			},
		},
		{
			// A leading count is carried onto the row, and "containing"
			// without any Ryo is just prose.
			name:    "counts",
			text:    "3 Books containing training strategies, 1 Blank Jutsu Scrolls",
			wantRyo: 0,
			want: []startingEquipmentLine{
				{Text: "3 Books containing training strategies", Quantity: 3},
				{Text: "1 Blank Jutsu Scrolls", Slug: "scroll/blank-scroll", Quantity: 1},
			},
		},
	}

	for _, c := range cases {
		got, err := s.parseStartingEquipment(c.text)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Ryo != c.wantRyo {
			t.Errorf("%s: Ryo = %v, want %v", c.name, got.Ryo, c.wantRyo)
		}
		if len(got.Lines) != len(c.want) {
			t.Errorf("%s: %d lines, want %d: %+v", c.name, len(got.Lines), len(c.want), got.Lines)
			continue
		}
		for i, want := range c.want {
			if got.Lines[i] != want {
				t.Errorf("%s line %d = %+v, want %+v", c.name, i, got.Lines[i], want)
			}
		}
	}

	// Upgraded gear must never be reachable from a background's prose, the
	// same rule isStartingTierGear enforces on the toolkit dropdowns.
	got, err := s.parseStartingEquipment("a Supreme Poison Kit")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 1 || got.Lines[0].Slug != "" {
		t.Errorf("Supreme kit resolved to %+v, want an unresolved free-text line", got.Lines)
	}
}

// TestEveryBackgroundStartingEquipmentParses is the whole-database guard:
// every background's printed equipment must yield its starting money and at
// least one resolved item, so a rules re-ingest that rewords a line shows up
// as a failure here rather than as a character silently starting broke.
func TestEveryBackgroundStartingEquipmentParses(t *testing.T) {
	const dbPath = "../../out/rules.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("out/rules.db not built; skipping whole-database background check")
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &server{rulesDB: db}

	rows, err := db.Query(`SELECT slug, equipment_text FROM backgrounds ORDER BY slug`)
	if err != nil {
		t.Fatal(err)
	}
	type background struct{ slug, text string }
	var backgrounds []background
	for rows.Next() {
		var b background
		var text sql.NullString
		if err := rows.Scan(&b.slug, &text); err != nil {
			t.Fatal(err)
		}
		b.text = text.String
		backgrounds = append(backgrounds, b)
	}
	rows.Close()
	if len(backgrounds) == 0 {
		t.Fatal("no backgrounds read from the database")
	}

	for _, b := range backgrounds {
		got, err := s.parseStartingEquipment(b.text)
		if err != nil {
			t.Fatalf("%s: %v", b.slug, err)
		}
		if got.Ryo <= 0 {
			t.Errorf("%s: starting Ryo = %v, want the amount its text names", b.slug, got.Ryo)
		}
		if len(got.Lines) < 2 {
			t.Errorf("%s: %d lines — the sentence was not broken up", b.slug, len(got.Lines))
		}
		resolved := 0
		for _, line := range got.Lines {
			if line.Slug != "" {
				resolved++
			}
		}
		if resolved == 0 {
			t.Errorf("%s: nothing resolved to a real item: %+v", b.slug, got.Lines)
		}
	}
}

// TestSplitCompoundGrant covers the two background_proficiencies values that
// grant something outright AND then ask for a choice. Stored whole, each
// became a single proficiency named after its own instructions — the
// character was "proficient with Acrobatics, Choose one Ninshou, Martial
// Arts, Illusions" and was never asked to choose.
func TestSplitCompoundGrant(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			"Acrobatics, Choose one Ninshou, Martial Arts, Illusions",
			[]string{"Acrobatics", "Choose one Ninshou, Martial Arts, Illusions"},
		},
		{
			"History and Deception or Persuasion",
			[]string{"History", "Choose one from Deception, Persuasion"},
		},
		// Everything else passes through untouched.
		{"Acrobatics", []string{"Acrobatics"}},
		{"Choose two from Ninshou, Martial Arts, Illusions", []string{"Choose two from Ninshou, Martial Arts, Illusions"}},
		{"Choose one between Poison Kit, Medicine Kit, Trappers Kit", []string{"Choose one between Poison Kit, Medicine Kit, Trappers Kit"}},
		{"One of your choice", []string{"One of your choice"}},
		{"Sleight of Hand", []string{"Sleight of Hand"}},
	}
	for _, c := range cases {
		got := splitCompoundGrant(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCompoundGrant(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCompoundGrant(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}

	// And the choice half really does become a pickable dropdown, not
	// another free-text fallback.
	row := classifyBackgroundProfValue("skill", "Choose one Ninshou, Martial Arts, Illusions", 1, nil, nil, nil, nil)
	if !row.IsChoice || row.ChooseN != 1 || len(row.Options) != 3 {
		t.Errorf("choice clause = %+v, want a 1-of-3 picker", row)
	}
}
