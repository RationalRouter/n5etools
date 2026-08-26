package charstore

import (
	"testing"
)

// TestSetCompanionFieldsLocksArmorChassis: Armor Chassis (Purple Technique's
// one-time crafting choice at 2nd level) must never change once set, even
// if a later save sends a different value — see SetCompanionFields' own
// doc for why several Purple Upgrades gating on a specific chassis makes an
// editable pick an exploit, not just a UX nicety.
func TestSetCompanionFieldsLocksArmorChassis(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Test Puppet")
	if err != nil {
		t.Fatal(err)
	}
	set := func(chassis string) {
		t.Helper()
		if err := SetCompanionFields(db, 1, companionID, "Test Puppet", "",
			"", "", "", chassis, false, "", "",
			"", "", "",
			false,
		); err != nil {
			t.Fatal(err)
		}
	}

	set("Bulwark")
	c, err := GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.ArmorChassis != "Bulwark" {
		t.Fatalf("ArmorChassis = %q, want Bulwark", c.ArmorChassis)
	}

	set("Camouflage")
	c, err = GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.ArmorChassis != "Bulwark" {
		t.Errorf("ArmorChassis changed to %q after already being set — should have stayed Bulwark", c.ArmorChassis)
	}
}
