package parse

import "testing"

func TestParseEquipmentPropertiesBasic(t *testing.T) {
	props, anomalies := ParseEquipmentProperties("Finesse, Critical, Deadly", "Cleaver Sword")
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	want := []EquipmentProperty{
		{PropertySlug: "property/finesse", RawName: "Finesse"},
		{PropertySlug: "property/critical", RawName: "Critical"},
		{PropertySlug: "property/deadly", RawName: "Deadly"},
	}
	if len(props) != len(want) {
		t.Fatalf("got %d props, want %d: %+v", len(props), len(want), props)
	}
	for i, w := range want {
		if props[i] != w {
			t.Errorf("prop %d = %+v, want %+v", i, props[i], w)
		}
	}
}

func TestParseEquipmentPropertiesParenDetail(t *testing.T) {
	// Covers both "Name (detail)" and the no-space "Name(detail)" spelling
	// seen on some weapons in the live data.
	props, anomalies := ParseEquipmentProperties("Thrown (30/60), Multiattack, Ammunition", "Kunai")
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if props[0].PropertySlug != "property/thrown" || props[0].Detail != "30/60" {
		t.Errorf("thrown = %+v", props[0])
	}

	props2, anomalies2 := ParseEquipmentProperties("Thrown(30/60), Light, Returning", "Shuriken")
	if len(anomalies2) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies2)
	}
	if props2[0].PropertySlug != "property/thrown" || props2[0].Detail != "30/60" {
		t.Errorf("no-space thrown = %+v", props2[0])
	}
}

func TestParseEquipmentPropertiesOCRPeriod(t *testing.T) {
	// A stray OCR period stands in for a comma on some rows.
	props, anomalies := ParseEquipmentProperties("Thrown(60/120). Hidden, Returning", "Kunai (Explosive Tag)")
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	want := []string{"property/thrown", "property/hidden", "property/returning"}
	if len(props) != len(want) {
		t.Fatalf("got %d props, want %d: %+v", len(props), len(want), props)
	}
	for i, w := range want {
		if props[i].PropertySlug != w {
			t.Errorf("prop %d slug = %q, want %q", i, props[i].PropertySlug, w)
		}
	}
}

func TestParseEquipmentPropertiesTrailingNumber(t *testing.T) {
	props, anomalies := ParseEquipmentProperties("Reach 1, Deadly, Heavy, Two-Handed", "Scythe")
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if props[0].PropertySlug != "property/reach" || props[0].Detail != "1" {
		t.Errorf("reach = %+v", props[0])
	}
	if props[1].PropertySlug != "property/deadly" || props[1].Detail != "" {
		t.Errorf("bare deadly = %+v", props[1])
	}
}

func TestParseEquipmentPropertiesDeadlyLethalDistinct(t *testing.T) {
	// The Hidden Blade's real properties list — proves Deadly and Lethal are
	// different mechanics (both present on the same weapon), not a spelling
	// inconsistency to merge.
	props, anomalies := ParseEquipmentProperties("Deadly, Lethal 5, Hidden, Finesse", "Hidden Blade")
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if props[0].PropertySlug != "property/deadly" {
		t.Errorf("deadly = %+v", props[0])
	}
	if props[1].PropertySlug != "property/lethal" || props[1].Detail != "5" {
		t.Errorf("lethal 5 = %+v", props[1])
	}
}

func TestParseEquipmentPropertiesSynonyms(t *testing.T) {
	// "Grappling" (the Net) and "Two Handed" (no hyphen) are inconsistent
	// spellings of Grapple / Two-Handed elsewhere in the live data.
	props, _ := ParseEquipmentProperties("Grappling, Range (10/15)", "Net")
	if props[0].PropertySlug != "property/grapple" {
		t.Errorf("grappling = %+v, want property/grapple", props[0])
	}

	props2, _ := ParseEquipmentProperties("Critical, Heavy, Two Handed", "Great Sword")
	if props2[2].PropertySlug != "property/two-handed" {
		t.Errorf("two handed = %+v, want property/two-handed", props2[2])
	}
}

func TestParseEquipmentPropertiesUnrecognizedFlagged(t *testing.T) {
	props, anomalies := ParseEquipmentProperties("Finesse, Flooble", "Made-Up Weapon")
	if len(anomalies) != 1 {
		t.Fatalf("got %d anomalies, want 1: %+v", len(anomalies), anomalies)
	}
	if props[1].PropertySlug != "" || props[1].RawName != "Flooble" {
		t.Errorf("unrecognized token = %+v", props[1])
	}
}

func TestParseEquipmentPropertiesEmpty(t *testing.T) {
	props, anomalies := ParseEquipmentProperties("", "Fists")
	if props != nil || anomalies != nil {
		t.Errorf("empty input = %+v, %+v, want nil, nil", props, anomalies)
	}
}
