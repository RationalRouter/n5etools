package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

// A compact clan following the book's template, exercising: wrapped ability
// label, trait parsing, level-gated features, a clan jutsu section, a feat,
// and the epithet/overview split.
func TestParseClanBookFixture(t *testing.T) {
	lines := mkLines(6,
		"ABURAME CLAN",
		"Some flavor fiction that is skipped entirely.",
		"CREEPY CRAWLY",
		"The Aburame Clan is one of the four noble clans.",
		"ABURAME TRAITS",
		"Recommended Recommended Ability Score",
		"Increase:",
		"+2 Intelligence, +1 Wisdom",
		"Speed: Your base walking speed is 30 feet",
		"Skill Proficiencies: Nature, Animal Handling",
		"Extra Language: Insect-Speak, you can",
		"understand and speak to insects.",
		"Parasitic Technique: You know 1 additional",
		"Aburame Clan D-Rank Ninjutsu.",
		"ABURAME FEATURES",
		"Bug Host: Beginning at 1st level, you gain a +1 bonus",
		"to Constitution saving throws. This increases at 11th level.",
		"Insect Focus: Starting at 3rd Level you learn to focus.",
		"ABURAME CLAN JUTSU",
		"D-RANK:",
		"HUMAN COCOON",
		"Classification: Hijutsu",
		"Rank: D-Rank",
		"Casting Time: 1 Action",
		"Range: Self",
		"Duration: Special",
		"Components: HS, CM",
		"Cost: Special",
		"Keywords: Hijutsu, Ninjutsu",
		"Description: You create a cocoon.",
		"CLAN FEATS",
		"HIVE MINDED",
		"Category: Clan",
		"Prerequisite: Aburame Clan, Level 8+",
		"Your insects treat you as a hive. You gain benefits;",
		"• Increase your Intelligence or Wisdom score by 1.",
	)
	clans, anomalies := ParseClanBook(lines)
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if len(clans) != 1 {
		t.Fatalf("got %d clans, want 1", len(clans))
	}
	c := clans[0]
	if c.Name != "Aburame Clan" || c.Epithet != "Creepy Crawly" {
		t.Errorf("Name=%q Epithet=%q", c.Name, c.Epithet)
	}
	if !strings.Contains(c.Overview, "four noble clans") {
		t.Errorf("Overview = %q", c.Overview)
	}
	if c.AbilityRaw != "+2 Intelligence, +1 Wisdom" {
		t.Errorf("AbilityRaw = %q", c.AbilityRaw)
	}
	wantASI := []AbilityIncrease{{"int", 2}, {"wis", 1}}
	if len(c.AbilityIncreases) != 2 || c.AbilityIncreases[0] != wantASI[0] || c.AbilityIncreases[1] != wantASI[1] {
		t.Errorf("AbilityIncreases = %+v", c.AbilityIncreases)
	}
	if c.SpeedFeet == nil || *c.SpeedFeet != 30 {
		t.Errorf("SpeedFeet = %v", c.SpeedFeet)
	}
	if len(c.SkillProfs) != 2 || c.SkillProfs[0] != "Nature" {
		t.Errorf("SkillProfs = %v", c.SkillProfs)
	}
	if !strings.HasPrefix(c.ExtraLanguage, "Insect-Speak") || !strings.Contains(c.ExtraLanguage, "speak to insects") {
		t.Errorf("ExtraLanguage lost its wrapped tail: %q", c.ExtraLanguage)
	}
	if len(c.Traits) != 1 || c.Traits[0].Name != "Parasitic Technique" {
		t.Errorf("Traits = %+v", c.Traits)
	}
	if len(c.Features) != 2 {
		t.Fatalf("Features = %+v", c.Features)
	}
	if c.Features[0].Name != "Bug Host" || c.Features[0].Level == nil || *c.Features[0].Level != 1 {
		t.Errorf("feature 0 = %+v", c.Features[0])
	}
	if c.Features[1].Level == nil || *c.Features[1].Level != 3 {
		t.Errorf("Insect Focus level = %+v", c.Features[1].Level)
	}
	if len(c.Jutsu) != 1 || c.Jutsu[0].Name != "Human Cocoon" || c.Jutsu[0].CategoryGroup != "Clan: Aburame" {
		t.Errorf("Jutsu = %+v", c.Jutsu)
	}
	if len(c.Feats) != 1 || c.Feats[0].Prerequisites != "Aburame Clan, Level 8+" {
		t.Errorf("Feats = %+v", c.Feats)
	}
}

// Choice-based ability increases stay raw-only; no fixed rows are invented.
func TestParseClanBookChoiceASINotFixed(t *testing.T) {
	lines := mkLines(21,
		"CHINOIKE CLAN",
		"BLOOD ARCHITECTS",
		"Overview prose here.",
		"CHINOIKE TRAITS",
		"Recommended Recommended Ability Score Increase: +2 Wis or Int, +1 Dex",
		"Speed: Your base walking speed is 30 feet.",
	)
	clans, _ := ParseClanBook(lines)
	if len(clans) != 1 {
		t.Fatalf("got %d clans", len(clans))
	}
	if len(clans[0].AbilityIncreases) != 0 {
		t.Errorf("choice ASI must not produce fixed increases: %+v", clans[0].AbilityIncreases)
	}
	if clans[0].AbilityRaw != "+2 Wis or Int, +1 Dex" {
		t.Errorf("AbilityRaw = %q", clans[0].AbilityRaw)
	}
}

// Whole-book regression against the real clan compendium (v3.11). The counts
// were eyeballed against the book's TOC and per-clan sections; all 37
// anomalies are genuine missing-Keywords omissions in print.
func TestParseClanBookFullCompendium(t *testing.T) {
	path := filepath.Join("/home/sergio/Documents/N5E", "Tsunades_Studies_Compendium.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook not available: %v", err)
	}
	doc, err := extract.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []Line
	for n := 5; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, Line{Page: n, Text: ln})
		}
	}
	clans, anomalies := ParseClanBook(lines)

	if len(clans) != 45 { // 44 clans + Non-Clan
		t.Errorf("parsed %d clans, want 45", len(clans))
	}
	totJutsu, totFeatures, totFeats := 0, 0, 0
	for _, c := range clans {
		totJutsu += len(c.Jutsu)
		totFeatures += len(c.Features)
		totFeats += len(c.Feats)
		if c.SpeedFeet == nil {
			t.Errorf("%s: no walking speed parsed", c.Name)
		}
		if c.AbilityRaw == "" {
			t.Errorf("%s: no ability increase text", c.Name)
		}
		if c.Epithet == "" {
			t.Errorf("%s: no epithet", c.Name)
		}
	}
	if totJutsu != 418 {
		t.Errorf("total clan jutsu = %d, want 418", totJutsu)
	}
	if totFeatures != 239 {
		t.Errorf("total features = %d, want 239", totFeatures)
	}
	if totFeats != 150 {
		t.Errorf("total clan feats = %d, want 150", totFeats)
	}
	for _, a := range anomalies {
		if !strings.Contains(a.Problem, "missing field Keywords") {
			t.Errorf("unexpected anomaly p%d %s: %s", a.Page, a.Subject, a.Problem)
		}
	}
	if len(anomalies) != 37 {
		t.Errorf("got %d anomalies, want 37 known missing-Keywords omissions", len(anomalies))
	}

	// Bloodline Latents section of the same book (verified 2026-07-17 against
	// v3.11: 44 per-clan tables — Non-Clan has none — 575 rows total).
	feat, latents, latentAnomalies := ParseBloodlineLatents(lines)
	if len(latentAnomalies) != 0 {
		for _, a := range latentAnomalies {
			t.Errorf("latent anomaly p%d %s: %s", a.Page, a.Subject, a.Problem)
		}
	}
	if feat == nil || feat.Name != "Bloodline, Latent" {
		t.Fatalf("section feat = %+v", feat)
	}
	if len(latents) != 575 {
		t.Errorf("parsed %d latents, want 575", len(latents))
	}
	latentClans := map[string]bool{}
	prereqs, zeroCost := 0, 0
	for _, l := range latents {
		latentClans[l.ClanName] = true
		if l.Prerequisites != "" {
			prereqs++
		}
		if l.PointCost == 0 {
			zeroCost++
		}
		if l.Name == "" || l.Description == "" {
			t.Errorf("%s/%s: empty name or description", l.ClanName, l.Name)
		}
	}
	if len(latentClans) != 44 {
		t.Errorf("latent tables cover %d clans, want 44", len(latentClans))
	}
	if prereqs != 371 {
		t.Errorf("latents with prerequisites = %d, want 371", prereqs)
	}
	if zeroCost != 9 {
		t.Errorf("zero-cost latents = %d, want 9", zeroCost)
	}
}
