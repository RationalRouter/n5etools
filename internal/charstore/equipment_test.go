package charstore

import "testing"

// TestSetEquipmentMergesSameSlugAcrossSources covers the reported bug this
// merge exists to fix: a class starting-equipment group's kit choice and a
// background's own pack text can both resolve to the same item_slug (e.g.
// an Armorsmith Kit from both a Mech Crafter pick and a Background) — they
// must land as one row with a summed quantity, not two separate rows.
func TestSetEquipmentMergesSameSlugAcrossSources(t *testing.T) {
	db := testCharDB(t)

	if err := SetEquipment(db, 1, []EquipmentLine{
		{Slug: "toolkit/armorsmith-kit", Quantity: 1},
		{Slug: "toolkit/armorsmith-kit", Quantity: 1},
		{Slug: "weapon/kunai", Quantity: 3},
	}); err != nil {
		t.Fatal(err)
	}

	var kitRows, kitQty int
	if err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(quantity), 0) FROM character_inventory WHERE character_id = 1 AND item_slug = 'toolkit/armorsmith-kit'`,
	).Scan(&kitRows, &kitQty); err != nil {
		t.Fatal(err)
	}
	if kitRows != 1 {
		t.Errorf("armorsmith kit rows = %d, want 1 (two sources should merge into one row)", kitRows)
	}
	if kitQty != 2 {
		t.Errorf("armorsmith kit total quantity = %d, want 2", kitQty)
	}
}

// TestSetEquipmentResubmitDoesNotCompoundExternalRow guards the edge case
// found while reviewing the merge fix above: the character sheet has no
// creation-status gate, so a same-slug row can already exist from outside
// this creation step (e.g. a manual sheet edit) by the time a player
// revisits and resubmits the Equipment step. The merge must never touch
// that row — only a row this same function tagged 'creation-equipment' —
// or every resubmission would silently inflate the external row's quantity
// forever, since the leading DELETE never clears anything but its own tag.
func TestSetEquipmentResubmitDoesNotCompoundExternalRow(t *testing.T) {
	db := testCharDB(t)

	if _, err := db.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, quantity, notes) VALUES (1, 'toolkit/armorsmith-kit', 5, 'manual')`,
	); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := SetEquipment(db, 1, []EquipmentLine{
			{Slug: "toolkit/armorsmith-kit", Quantity: 1},
		}); err != nil {
			t.Fatal(err)
		}
	}

	var externalQty int
	if err := db.QueryRow(
		`SELECT quantity FROM character_inventory WHERE character_id = 1 AND item_slug = 'toolkit/armorsmith-kit' AND notes = 'manual'`,
	).Scan(&externalQty); err != nil {
		t.Fatal(err)
	}
	if externalQty != 5 {
		t.Errorf("manually-added row quantity = %d after 3 resubmissions, want unchanged 5", externalQty)
	}

	var creationQty int
	if err := db.QueryRow(
		`SELECT quantity FROM character_inventory WHERE character_id = 1 AND item_slug = 'toolkit/armorsmith-kit' AND notes = 'creation-equipment'`,
	).Scan(&creationQty); err != nil {
		t.Fatal(err)
	}
	if creationQty != 1 {
		t.Errorf("creation-equipment row quantity = %d after 3 resubmissions, want 1 (each resubmission rebuilds from scratch)", creationQty)
	}
}
