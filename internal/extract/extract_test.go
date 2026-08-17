package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The sourcebook PDFs are not committed (size + copyright); extraction tests
// run only on machines that have them. CI-safe: they skip when absent.
const sourcebookDir = "/home/sergio/Documents/N5E"

func openBook(t *testing.T, name string) *Document {
	t.Helper()
	path := filepath.Join(sourcebookDir, name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook %s not available: %v", name, err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	return doc
}

// Physical page 4 of the jutsu compendium is the first jutsu listing page.
// This locks in the two properties the parsers depend on: reading order
// (whole left column before right column) and clean key:value lines.
func TestJutsuCompendiumPage4ReadingOrder(t *testing.T) {
	doc := openBook(t, "Jiraiyas_Jutsu_Compendium.pdf")
	lines, err := doc.PageLines(4)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")

	// Decoding: apostrophes and full words intact.
	for _, want := range []string{
		"hand can’t attack, activate chakra items, or carry more than",
		"Classification: Ninjutsu",
		"Cost: 1 Chakra",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("page text missing %q", want)
		}
	}

	// Reading order: the left column's three E-rank jutsu come before the
	// right column's first (Chakra Blow), and the full-width intro line keeps
	// its tail (the bug that killed the coordinate-based approach).
	order := []string{
		"elemental chakra. Non-Elemental Ninjutsu",
		"CHAKRA HANDS",
		"CHAKRA MOVEMENT",
		"CHAKRA PULSE",
		"CHAKRA BLOW",
	}
	pos := -1
	for _, want := range order {
		i := strings.Index(text, want)
		if i < 0 {
			t.Fatalf("page text missing %q", want)
		}
		if i < pos {
			t.Errorf("%q appears before the previous marker — reading order broken", want)
		}
		pos = i
	}
}

func TestClanCompendiumPage6(t *testing.T) {
	doc := openBook(t, "Tsunades_Studies_Compendium.pdf")
	lines, err := doc.PageLines(6)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"ABURAME CLAN",
		"CREEPY CRAWLY",
		"Bug Host:",
		"Beginning at 1st level",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("page text missing %q", want)
		}
	}
}

func TestClassCompendiumPage7(t *testing.T) {
	doc := openBook(t, "Orochimarus_Observation_Compendium.pdf")
	lines, err := doc.PageLines(7)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"CLASS FEATURES",
		"Hit Dice: 1d6 per Genjutsu Specialist level",
		"Chakra Dice: 1d12 per Genjutsu Specialist level",
		"Saving Throws: Constitution, Wisdom, Charisma",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("page text missing %q", want)
		}
	}
}
