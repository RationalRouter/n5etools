package charstore

import "testing"

// TestSetNinDogStatDefaultsSeedsJutsuSlotsCurrent covers the "max right,
// current zero" bug: a brand-new Nin-Dog's jutsu_slots_max was always set
// correctly, but jutsu_slots_current was never written by this function at
// all, leaving it NULL until the player manually adjusted the +/- control —
// the same shape the player-character "zero vitals" bug had for HP/Chakra.
func TestSetNinDogStatDefaultsSeedsJutsuSlotsCurrent(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "nin-dog", "Test Nin-Dog")
	if err != nil {
		t.Fatal(err)
	}

	if err := SetNinDogStatDefaults(db, 1, companionID,
		14, 20, 20, 30, 5, 5,
		12, 14, 12, 8, 10, 10,
	); err != nil {
		t.Fatal(err)
	}

	companion, err := GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if !companion.JutsuSlotsCurrent.Valid || companion.JutsuSlotsCurrent.Int64 != 5 {
		t.Errorf("JutsuSlotsCurrent = %+v, want valid 5 (seeded to max on creation)", companion.JutsuSlotsCurrent)
	}
	if !companion.JutsuSlotsMax.Valid || companion.JutsuSlotsMax.Int64 != 5 {
		t.Errorf("JutsuSlotsMax = %+v, want valid 5", companion.JutsuSlotsMax)
	}
}

// TestSetNinDogStatDefaultsNeverOverwritesManualEdit confirms the
// COALESCE-based contract still holds for jutsu_slots_current specifically:
// once a player has spent slots (a non-NULL value already present), a
// second prefill call (e.g. a hypothetical future backfill) must never
// reset it back up to max.
func TestSetNinDogStatDefaultsNeverOverwritesManualEdit(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "nin-dog", "Test Nin-Dog")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetNinDogStatDefaults(db, 1, companionID,
		14, 20, 20, 30, 5, 5,
		12, 14, 12, 8, 10, 10,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE character_companions SET jutsu_slots_current = 2 WHERE id = ?`, companionID); err != nil {
		t.Fatal(err)
	}

	if err := SetNinDogStatDefaults(db, 1, companionID,
		14, 20, 20, 30, 5, 5,
		12, 14, 12, 8, 10, 10,
	); err != nil {
		t.Fatal(err)
	}

	companion, err := GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if !companion.JutsuSlotsCurrent.Valid || companion.JutsuSlotsCurrent.Int64 != 2 {
		t.Errorf("JutsuSlotsCurrent = %+v after re-prefill, want unchanged valid 2", companion.JutsuSlotsCurrent)
	}
}
