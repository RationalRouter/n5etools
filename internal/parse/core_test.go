package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

func TestParseFightingStancesFixture(t *testing.T) {
	lines := mkLines(140,
		"FIGHTING STANCES",
		"Certain class features offer your choice of Fighting Style.",
		"You can only gain the benefit of one stance at a time.",
		"TAIJUTSU STANCES",
		"DRAGON FIST STANCE",
		"You have learned a defensive stance.",
		"• Your Unarmed Damage die becomes a d6.",
		"BUKIJUTSU STANCES",
		"SHARPSHOOTER STANCE",
		"Prerequisite: Dexterity 13+",
		"Your ranged attacks sharpen.",
		"FEATS",
		"ACTION SURGE",
		"Category: General",
		"Not part of the stances section.",
	)
	stances, anomalies := ParseFightingStances(lines)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(stances) != 2 {
		t.Fatalf("stances = %+v", stances)
	}
	if stances[0].Name != "Dragon Fist Stance" || stances[0].StanceType != "taijutsu" ||
		!strings.Contains(stances[0].Description, "d6") {
		t.Errorf("stance 0 = %+v", stances[0])
	}
	if stances[1].Name != "Sharpshooter Stance" || stances[1].StanceType != "bukijutsu" ||
		stances[1].Prerequisites != "Dexterity 13+" {
		t.Errorf("stance 1 = %+v", stances[1])
	}
}

// loadCoreBookLines opens the real core book and returns its lines (from
// physical page 2 on), or nil after calling t.Skip if the sourcebook isn't
// present on this machine. Shared by every core-book whole-book regression.
func loadCoreBookLines(t *testing.T) []Line {
	t.Helper()
	path := filepath.Join("/home/sergio/Documents/N5E", "Naruto 5e - Full Document.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook not available: %v", err)
		return nil
	}
	doc, err := extract.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []Line
	for n := 2; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, Line{Page: n, Text: ln})
		}
	}
	return lines
}

// Whole-book regression (verified 2026-07-18 against v3.11). Skips when the
// sourcebook is absent.
func TestParseCoreBookChapter13(t *testing.T) {
	lines := loadCoreBookLines(t)
	if lines == nil {
		return
	}

	stances, sa := ParseFightingStances(lines)
	if len(sa) != 0 {
		t.Errorf("stance anomalies: %+v", sa)
	}
	byKind := map[string]int{}
	for _, s := range stances {
		byKind[s.StanceType]++
		if s.Description == "" {
			t.Errorf("%s: empty description", s.Name)
		}
	}
	if len(stances) != 21 || byKind["taijutsu"] != 9 || byKind["bukijutsu"] != 12 {
		t.Errorf("stances = %d (%v), want 21 (9 taijutsu, 12 bukijutsu)", len(stances), byKind)
	}

	feats, fa := ParseCoreFeats(lines)
	if len(feats) != 142 {
		t.Errorf("core feats = %d, want 142", len(feats))
	}
	for _, f := range feats {
		if f.Category == "" || f.Description == "" {
			t.Errorf("feat %q: empty category or description", f.Name)
		}
	}
	// Two known chart sidebars are appended to their feats and flagged.
	if len(fa) != 2 {
		t.Errorf("feat anomalies = %+v, want the 2 chart appends", fa)
	}
	for _, a := range fa {
		if !strings.Contains(a.Problem, "sidebar/table") {
			t.Errorf("unexpected anomaly: %+v", a)
		}
	}

	backgrounds, ba := ParseBackgrounds(lines)
	if len(ba) != 0 {
		t.Errorf("background anomalies: %+v", ba)
	}
	if len(backgrounds) != 10 {
		t.Fatalf("backgrounds = %d, want 10", len(backgrounds))
	}
	for _, b := range backgrounds {
		if b.Description == "" || b.SkillProfs == "" || b.FeatureName == "" ||
			b.FeatureText == "" || b.ASIText == "" || b.EquipmentPack == "" {
			t.Errorf("%s: incomplete background: %+v", b.Name, b)
		}
	}
	// Noble prints the singular "Tool Proficiency:" label — must still land.
	for _, b := range backgrounds {
		if b.Name == "Noble" && b.ToolProfs != "Security Kit" {
			t.Errorf("Noble tool profs = %q", b.ToolProfs)
		}
	}
}

func TestParseBackgroundsFixture(t *testing.T) {
	lines := mkLines(20,
		"BACKGROUNDS",
		"Every story has a beginning.",
		"PROFICIENCIES",
		"Each background gives proficiency in two skills.",
		"ENTERTAINER",
		"You thrived in front of an audience.",
		"Skill Proficiencies: Acrobatics, Performance",
		"Tool Proficiencies: Disguise kit",
		"Equipment: A love letter, Wallet containing 100",
		"Ryo.",
		"Equipment Pack: Infiltrator’s or Captain’s Pack (Choose one).",
		"FEATURE: BACK BY POPULAR DEMAND",
		"You can always find a place to perform.",
		"ENTICING PERSONALITY",
		"Select one:",
		"• Increase any ability score by +1.",
		"(Recommended: Intelligence, Charisma)",
		"CHAPTER 4: CLASSES",
		"Class chapter text.",
	)
	backgrounds, anomalies := ParseBackgrounds(lines)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(backgrounds) != 1 {
		t.Fatalf("backgrounds = %+v", backgrounds)
	}
	b := backgrounds[0]
	if b.Name != "Entertainer" || b.FeatureName != "Back By Popular Demand" {
		t.Errorf("background = %+v", b)
	}
	if b.Equipment != "A love letter, Wallet containing 100 Ryo." &&
		b.Equipment != "A love letter, Wallet containing 100 Ryo" {
		t.Errorf("wrapped equipment = %q", b.Equipment)
	}
	if !strings.Contains(b.ASIText, "Select one:") ||
		!strings.Contains(b.ASIText, "Recommended: Intelligence") {
		t.Errorf("asi text = %q", b.ASIText)
	}
	if strings.Contains(b.Description, "beginning") || strings.Contains(b.ASIText, "CHAPTER") {
		t.Errorf("section intro or next chapter leaked in: %+v", b)
	}
}
