package charstore

import "testing"

func TestCompanionUpgradesCRUD(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Test Puppet")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := AddCompanionUpgrade(db, 1, companionID,
		"class/puppet-master/option/black-iron-upgrades/wood-tier",
		"class/puppet-master/option/black-iron-upgrades/wood-tier/entry/chakra-disruption-blade"); err != nil {
		t.Fatal(err)
	}
	upID, err := AddCompanionUpgrade(db, 1, companionID,
		"class/puppet-master/option/black-iron-upgrades/wood-tier",
		"class/puppet-master/option/black-iron-upgrades/wood-tier/entry/hidden-blades")
	if err != nil {
		t.Fatal(err)
	}

	got, err := ListCompanionUpgrades(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d upgrades, want 2: %+v", len(got), got)
	}

	if err := DeleteCompanionUpgrade(db, 1, companionID, upID); err != nil {
		t.Fatal(err)
	}
	got, err = ListCompanionUpgrades(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after delete, got %d upgrades, want 1: %+v", len(got), got)
	}
}

// A forged companionID belonging to a DIFFERENT character must not be
// readable/writable/deletable — same ownership-scoping guarantee
// GetCompanion/DeleteCompanion already provide for the companion row
// itself.
func TestCompanionUpgradesOwnershipScoping(t *testing.T) {
	db := testCharDB(t)
	if _, err := db.Exec(`INSERT INTO characters (id, name) VALUES (2, 'Other')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := AddCompanion(db, 1, "puppet", "Character 1's Puppet")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddCompanionUpgrade(db, 1, companionID, "tier", "entry/real"); err != nil {
		t.Fatal(err)
	}

	// Character 2 tries to add/list/delete against character 1's companion.
	if _, err := AddCompanionUpgrade(db, 2, companionID, "tier", "entry/forged"); err == nil {
		t.Error("AddCompanionUpgrade across characters should fail, got nil error")
	}
	got, err := ListCompanionUpgrades(db, 2, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListCompanionUpgrades across characters returned %d rows, want 0", len(got))
	}

	real, err := ListCompanionUpgrades(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(real) != 1 {
		t.Fatalf("owner's own list = %d, want 1", len(real))
	}
	if err := DeleteCompanionUpgrade(db, 2, companionID, real[0].ID); err != nil {
		t.Fatal(err)
	}
	stillThere, err := ListCompanionUpgrades(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stillThere) != 1 {
		t.Errorf("cross-character delete removed the row, want it untouched: %+v", stillThere)
	}
}
