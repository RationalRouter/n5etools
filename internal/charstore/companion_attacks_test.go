package charstore

import "testing"

func TestCompanionAttacksCRUD(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Test Puppet")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := AddCompanionAttack(db, 1, companionID, CompanionAttack{
		Name: "Bash", AttackAbility: "str", AttackProf: "full",
		DamageCount: 1, DamageSides: 6, DamageAbility: "str", DamageType: "bludgeoning",
	}); err != nil {
		t.Fatal(err)
	}
	attackID, err := AddCompanionAttack(db, 1, companionID, CompanionAttack{Name: "Buzzsaw"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := ListCompanionAttacks(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d attacks, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Bash" || got[0].DamageDice() != "1d6" {
		t.Errorf("first attack = %+v, damage dice %q", got[0], got[0].DamageDice())
	}
	if got[1].DamageDice() != "" {
		t.Errorf("attack with no damage should render no dice, got %q", got[1].DamageDice())
	}

	if err := DeleteCompanionAttack(db, 1, companionID, attackID); err != nil {
		t.Fatal(err)
	}
	got, err = ListCompanionAttacks(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after delete, got %d attacks, want 1: %+v", len(got), got)
	}
}

// Same ownership-scoping guarantee as companion upgrades: a forged
// companionID belonging to a different character must not be
// readable/writable/deletable.
func TestCompanionAttacksOwnershipScoping(t *testing.T) {
	db := testCharDB(t)
	if _, err := db.Exec(`INSERT INTO characters (id, name) VALUES (2, 'Other')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := AddCompanion(db, 1, "puppet", "Character 1's Puppet")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddCompanionAttack(db, 1, companionID, CompanionAttack{Name: "Real Attack"}); err != nil {
		t.Fatal(err)
	}

	if _, err := AddCompanionAttack(db, 2, companionID, CompanionAttack{Name: "Forged Attack"}); err == nil {
		t.Error("AddCompanionAttack across characters should fail, got nil error")
	}
	got, err := ListCompanionAttacks(db, 2, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListCompanionAttacks across characters returned %d rows, want 0", len(got))
	}

	real, err := ListCompanionAttacks(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(real) != 1 {
		t.Fatalf("owner's own list = %d, want 1", len(real))
	}
	if err := DeleteCompanionAttack(db, 2, companionID, real[0].ID); err != nil {
		t.Fatal(err)
	}
	stillThere, err := ListCompanionAttacks(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stillThere) != 1 {
		t.Errorf("cross-character delete removed the row, want it untouched: %+v", stillThere)
	}
}
