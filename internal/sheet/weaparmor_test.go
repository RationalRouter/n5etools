package sheet

import (
	"os"
	"testing"
)

// Regression against the real Mastersheet (verified 2026-07-18 against the
// v3.1 sheet: 32 official armor rows, 71 official weapon rows — the
// homebrew sections past the "[[...]]" dividers are excluded). Skips when
// the sheet is absent.
func TestParseWeapArmorMastersheet(t *testing.T) {
	path := "/home/sergio/Documents/N5E/Mastersheet - N5E v3.1.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("mastersheet not available: %v", err)
	}
	armors, weapons, anomalies, err := ParseWeapArmor(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range anomalies {
		t.Errorf("anomaly %s: %s", a.Subject, a.Problem)
	}
	if len(armors) != 32 {
		t.Errorf("armors = %d, want 32", len(armors))
	}
	if len(weapons) != 71 {
		t.Errorf("weapons = %d, want 71", len(weapons))
	}
	for _, a := range armors {
		if a.Name == "" || a.CalcType == "" {
			t.Errorf("armor with empty name/calc type: %+v", a)
		}
	}
	for _, w := range weapons {
		if w.Name == "" {
			t.Errorf("weapon with empty name: %+v", w)
		}
	}

	// Pin a couple of known rows.
	var skin, dustCoat *Armor
	for i := range armors {
		switch armors[i].Name {
		case "Skin":
			skin = &armors[i]
		case "Dust Coat":
			dustCoat = &armors[i]
		}
	}
	if skin == nil || skin.CalcType != "Custom" || skin.MaxMod != nil {
		t.Errorf("Skin = %+v", skin)
	}
	if dustCoat == nil || dustCoat.Ability1 != "INT" || dustCoat.Ability2 != "PROF" {
		t.Errorf("Dust Coat = %+v", dustCoat)
	}

	var battleWire *Weapon
	for i := range weapons {
		if weapons[i].Name == "Battle Wire" {
			battleWire = &weapons[i]
		}
	}
	if battleWire == nil || battleWire.DamageDice != "1d4" || battleWire.DamageType != "Slashing" {
		t.Errorf("Battle Wire = %+v", battleWire)
	}

	// Homebrew content past the dividers must never appear.
	for _, a := range armors {
		if a.Name == "[[Custom Armor Below Here]]" {
			t.Error("divider row leaked into armor list")
		}
	}
	for _, w := range weapons {
		if w.Name == "[[Custom Weapons]]" {
			t.Error("divider row leaked into weapon list")
		}
	}
}
