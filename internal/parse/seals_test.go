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
	if hunter.Rank != "A" { // tier word wins over the C-RANK heading it's under
		t.Errorf("Hunter Seal rank = %q, want A (from its SUPERIOR tier)", hunter.Rank)
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
// tier/heading mismatch in the printed book.
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
}
