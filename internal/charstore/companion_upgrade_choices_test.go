package charstore

import "testing"

func TestCompanionUpgradeChoicesCRUD(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Test Puppet")
	if err != nil {
		t.Fatal(err)
	}
	upgradeID, err := AddCompanionUpgrade(db, 1, companionID,
		"class/puppet-master/option/black-iron-upgrades/wood-tier",
		"class/puppet-master/option/black-iron-upgrades/wood-tier/entry/poison-mist-hell")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := AddCompanionUpgradeChoice(db, 1, companionID, upgradeID, "tool/poison-tag"); err != nil {
		t.Fatal(err)
	}
	choiceID, err := AddCompanionUpgradeChoice(db, 1, companionID, upgradeID, "tool/greater-poison-tag")
	if err != nil {
		t.Fatal(err)
	}

	got, err := ListCompanionUpgradeChoices(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d choices, want 2: %+v", len(got), got)
	}

	if err := DeleteCompanionUpgradeChoice(db, 1, companionID, choiceID); err != nil {
		t.Fatal(err)
	}
	got, err = ListCompanionUpgradeChoices(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after delete, got %d choices, want 1: %+v", len(got), got)
	}
}

// A forged companionID belonging to a different character must not be
// readable/writable/deletable, same guarantee as companion upgrades/attacks.
func TestCompanionUpgradeChoicesOwnershipScoping(t *testing.T) {
	db := testCharDB(t)
	if _, err := db.Exec(`INSERT INTO characters (id, name) VALUES (2, 'Other')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := AddCompanion(db, 1, "puppet", "Character 1's Puppet")
	if err != nil {
		t.Fatal(err)
	}
	upgradeID, err := AddCompanionUpgrade(db, 1, companionID, "tier", "entry/poison-mist-hell")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddCompanionUpgradeChoice(db, 1, companionID, upgradeID, "tool/poison-tag"); err != nil {
		t.Fatal(err)
	}

	if _, err := AddCompanionUpgradeChoice(db, 2, companionID, upgradeID, "tool/greater-poison-tag"); err == nil {
		t.Error("AddCompanionUpgradeChoice across characters should fail, got nil error")
	}
	got, err := ListCompanionUpgradeChoices(db, 2, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListCompanionUpgradeChoices across characters returned %d rows, want 0", len(got))
	}

	real, err := ListCompanionUpgradeChoices(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(real) != 1 {
		t.Fatalf("owner's own list = %d, want 1", len(real))
	}
	if err := DeleteCompanionUpgradeChoice(db, 2, companionID, real[0].ID); err != nil {
		t.Fatal(err)
	}
	stillThere, err := ListCompanionUpgradeChoices(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stillThere) != 1 {
		t.Errorf("cross-character delete removed the row, want it untouched: %+v", stillThere)
	}
}
