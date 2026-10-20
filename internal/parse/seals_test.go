package parse

import (
	"strings"
	"testing"
)

func TestParseEnhancementSealsFixture(t *testing.T) {
	lines := mkLines(76,
		"WEAPON ENHANCEMENT SEALS",
		"D-RANK SEALS",
		"BANE SEAL (MINOR)",
		"Ryo Cost: 350",
		"When this seal is placed on a weapon, it begins to vibrate.",
		"C-RANK SEALS",
		"BARRIER SEAL (REFINED)",
		"Ryo Cost: 800",
		"This seal grants a barrier.",
		"HUNTER SEAL (SUPERIOR)",
		"Ryo Cost: 650",
		"Weapons imbued with this seal deal an additional 2 die.",
		"ARMOR ENHANCEMENT SEALS",
		"D-RANK SEALS",
		"BLACKENED SEAL (MINOR)",
		"Ryo Cost: 250",
		"Armor imbued with this seal grants a stealth bonus.",
		"LEARNING/ CREATING A JUTSU",
		"You can spend time between adventures learning a new jutsu.",
	)
	seals, anomalies := ParseEnhancementSeals(lines)
	if len(seals) != 4 {
		t.Fatalf("got %d seals, want 4: %+v", len(seals), seals)
	}
	if len(anomalies) != 1 || !strings.Contains(anomalies[0].Problem, "SUPERIOR") {
		t.Fatalf("anomalies = %+v, want only the Hunter Seal tier mismatch", anomalies)
	}

	bane := seals[0]
	if bane.Name != "Bane Seal" || bane.Tier != "Minor" || bane.Rank != "D" ||
		bane.AppliesTo != "weapon" || bane.CostRyo != 350 {
		t.Errorf("Bane Seal = %+v", bane)
	}
	hunter := seals[2]
	if hunter.Rank != "C" || hunter.Tier != "Refined" { // a D/C/B heading wins over a misprinted tier word
		t.Errorf("Hunter Seal = rank %q tier %q, want C / Refined (from its C-RANK heading)", hunter.Rank, hunter.Tier)
	}
	armorSeal := seals[3]
	if armorSeal.AppliesTo != "armor" || armorSeal.Name != "Blackened Seal" {
		t.Errorf("armor seal = %+v", armorSeal)
	}
	// The next section's prose must not leak into the last seal.
	if strings.Contains(armorSeal.Description, "Learning") {
		t.Errorf("next-section text leaked in: %q", armorSeal.Description)
	}
}

// Whole-book regression (verified 2026-07-18 against v3.11): 149 seals
// (69 weapon, 80 armor), exactly one anomaly — the genuine Hunter Seal
// tier/heading mismatch in the printed book, now read as two distinct
// Hunter Seals (Refined, C-Rank and Superior, A-Rank).
func TestParseEnhancementSealsFullCorebook(t *testing.T) {
	lines := loadCoreBookLines(t)
	if lines == nil {
		return
	}
	seals, anomalies := ParseEnhancementSeals(lines)
	if len(seals) != 149 {
		t.Errorf("seals = %d, want 149", len(seals))
	}
	byApplies := map[string]int{}
	for _, s := range seals {
		byApplies[s.AppliesTo]++
		if s.Name == "" || s.Description == "" || s.Rank == "" || s.CostRyo == 0 {
			t.Errorf("incomplete seal: %+v", s)
		}
	}
	if byApplies["weapon"] != 69 || byApplies["armor"] != 80 {
		t.Errorf("applies-to split = %v, want weapon 69 / armor 80", byApplies)
	}
	if len(anomalies) != 1 || !strings.Contains(anomalies[0].Problem, "SUPERIOR") {
		t.Errorf("anomalies = %+v, want only the Hunter Seal mismatch", anomalies)
	}
	hunters := map[string]int{}
	for _, s := range seals {
		if s.Name == "Hunter Seal" {
			hunters[s.Tier+"/"+s.Rank] = s.CostRyo
		}
	}
	for _, s := range seals {
		if strings.Contains(s.Description, "RANK SEALS") {
			t.Errorf("%s (%s): section heading leaked into description", s.Name, s.Tier)
		}
	}
	if hunters["Refined/C"] != 650 || hunters["Superior/A"] != 1800 || len(hunters) != 2 {
		t.Errorf("Hunter Seals = %v, want Refined/C 650 and Superior/A 1800", hunters)
	}
}
