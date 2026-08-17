package validate

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/schema"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := schema.Apply(db, schema.Rules); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"Boil Release: Corrosive Viper Fang": "boilreleasecorrosiveviperfang",
		"WITCHES ENTHUSIAST[NEW]":            "witchesenthusiastnew",
		"  Padded  Name  ":                   "paddedname",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Errorf("normalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiff(t *testing.T) {
	sheet := map[string]string{"bear": "BEAR", "boar": "BOAR"}
	db := map[string]string{"bear": "Bear", "deer": "Deer"}
	r := diff("Test", sheet, db)
	if r.SheetCount != 2 || r.DBCount != 2 {
		t.Fatalf("counts = %+v", r)
	}
	if len(r.InDBNotSheet) != 1 || r.InDBNotSheet[0] != "Deer" {
		t.Errorf("InDBNotSheet = %v, want [Deer]", r.InDBNotSheet)
	}
	if len(r.InSheetNotDB) != 1 || r.InSheetNotDB[0] != "BOAR" {
		t.Errorf("InSheetNotDB = %v, want [BOAR]", r.InSheetNotDB)
	}
}

func TestRunAggregatesNeedsReview(t *testing.T) {
	db := testDB(t)
	// Insert directly — this test only cares whether Run() correctly
	// aggregates across whatever tables have needs_review rows, not how
	// those rows got there.
	if _, err := db.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range,
		                   duration, components, cost_text, keywords, description,
		                   detection_status)
		VALUES ('jutsu/a', 'A', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', '1 Chakra', 'Ninjutsu', 'x', 'needs_review'),
		       ('jutsu/b', 'B', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', '1 Chakra', 'Ninjutsu', 'x', 'needs_review'),
		       ('jutsu/c', 'C', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', '1 Chakra', 'Ninjutsu', 'x', 'auto')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO classes (slug, name, detection_status)
		VALUES ('class/a', 'A', 'needs_review')`); err != nil {
		t.Fatal(err)
	}

	report, err := Run(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalNeedsReview != 3 {
		t.Errorf("total = %d, want 3", report.TotalNeedsReview)
	}
	byTable := map[string]int{}
	for _, tc := range report.NeedsReview {
		byTable[tc.Table] = tc.Count
	}
	if byTable["jutsu"] != 2 || byTable["classes"] != 1 {
		t.Errorf("per-table = %+v", byTable)
	}
	if report.CrossCheck != nil {
		t.Errorf("cross-check should be nil when sheetPath is empty: %+v", report.CrossCheck)
	}

	var buf strings.Builder
	if err := report.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "needs_review: 3 rows across 2 tables") ||
		!strings.Contains(out, "jutsu") || !strings.Contains(out, "classes") {
		t.Errorf("Write output missing expected content: %s", out)
	}
}
