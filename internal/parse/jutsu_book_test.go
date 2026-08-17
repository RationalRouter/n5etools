package parse

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

// Whole-book regression test against the real jutsu compendium (v3.1).
// Skips when the PDF isn't on this machine (not committed: size + copyright).
// The counts below were verified by hand against the book's own structure;
// if a re-ingest changes them, something changed — the book or the parser —
// and either way a human should look.
func TestParseJutsuBookFullCompendium(t *testing.T) {
	path := filepath.Join("/home/sergio/Documents/N5E", "Jiraiyas_Jutsu_Compendium.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook not available: %v", err)
	}
	doc, err := extract.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	var lines []Line
	for n := 4; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, Line{Page: n, Text: ln})
		}
	}

	jutsu, anomalies := ParseJutsuBook(lines)

	if len(jutsu) != 1400 {
		t.Errorf("parsed %d jutsu, want 1400", len(jutsu))
	}

	byGroup := map[string]int{}
	byRank := map[string]int{}
	for _, j := range jutsu {
		byGroup[j.CategoryGroup]++
		byRank[j.Rank]++
	}

	wantGroups := map[string]int{
		"Ninjutsu / Non-Elemental":     202,
		"Ninjutsu / Earth Release":     75,
		"Ninjutsu / Wind Release":      74,
		"Ninjutsu / Fire Release":      75,
		"Ninjutsu / Water Release":     74,
		"Ninjutsu / Lightning Release": 74,
		"Genjutsu":                     268,
		"Taijutsu":                     175,
		"Bukijutsu":                    383,
	}
	for g, want := range wantGroups {
		if byGroup[g] != want {
			t.Errorf("section %q: %d jutsu, want %d", g, byGroup[g], want)
		}
	}
	for g := range byGroup {
		if _, ok := wantGroups[g]; !ok {
			t.Errorf("unexpected section %q with %d jutsu", g, byGroup[g])
		}
	}

	wantRanks := map[string]int{"E": 31, "D": 468, "C": 351, "B": 267, "A": 183, "S": 100}
	for r, want := range wantRanks {
		if byRank[r] != want {
			t.Errorf("rank %s: %d jutsu, want %d", r, byRank[r], want)
		}
	}
	if byRank[""] != 0 {
		t.Errorf("%d jutsu with no rank detected", byRank[""])
	}

	// Every entry must be complete: the required-field audit inside the
	// parser reports gaps as anomalies, so the full anomaly list doubles as
	// the completeness check. The book's known typos produce exactly these
	// nine (hyphen-wrapped "…B-\nRank:" lines inside descriptions are
	// recovered silently and no longer flagged).
	if len(anomalies) != 9 {
		t.Errorf("got %d anomalies, want 9:", len(anomalies))
		for _, a := range anomalies {
			t.Logf("  p%d %s: %s", a.Page, a.Subject, a.Problem)
		}
	}
}
