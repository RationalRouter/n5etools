package charstore

import (
	"database/sql"
	"testing"
)

func TestSplitCompanionIntoBodiesDividesHPAndClonesStats(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Sandman")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCompanionStatDefaults(db, 1, companionID,
		14, 31, 30, sql.NullInt64{},
		16, 12, 14, 10, 8, 6, "Medium",
	); err != nil {
		t.Fatal(err)
	}
	if err := SetCompanionHP(db, 1, companionID, sql.NullInt64{Int64: 31, Valid: true}); err != nil {
		t.Fatal(err)
	}

	if err := SplitCompanionIntoBodies(db, 1, companionID, 3); err != nil {
		t.Fatal(err)
	}

	all, err := ListCompanions(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}

	primary, err := GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if !primary.MatryoshkaGroupID.Valid || primary.MatryoshkaGroupID.Int64 != companionID {
		t.Fatalf("primary.MatryoshkaGroupID = %+v, want valid=%d", primary.MatryoshkaGroupID, companionID)
	}
	// 31 / 3 = 10 remainder 1 — the remainder goes to the primary body.
	if primary.HPMax.Int64 != 11 {
		t.Errorf("primary.HPMax = %d, want 11", primary.HPMax.Int64)
	}

	var siblingCount int
	var sawFullHP bool
	for _, c := range all {
		if c.ID == companionID {
			continue
		}
		siblingCount++
		if !c.MatryoshkaGroupID.Valid || c.MatryoshkaGroupID.Int64 != companionID {
			t.Errorf("sibling %d MatryoshkaGroupID = %+v, want valid=%d", c.ID, c.MatryoshkaGroupID, companionID)
		}
		if c.HPMax.Int64 == 10 {
			sawFullHP = true
		}
		if c.HPCurrent.Valid {
			t.Errorf("sibling %d HPCurrent = %+v, want NULL", c.ID, c.HPCurrent)
		}
		if c.Str.Int64 != 16 || c.Size != "Medium" {
			t.Errorf("sibling %d stats not cloned: Str=%v Size=%q", c.ID, c.Str, c.Size)
		}
	}
	if siblingCount != 2 {
		t.Fatalf("siblingCount = %d, want 2", siblingCount)
	}
	if !sawFullHP {
		t.Error("expected at least one sibling body with HPMax = 10")
	}
}

func TestSplitCompanionIntoBodiesRejectsInvalidCountAndAlreadySplit(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Sandman")
	if err != nil {
		t.Fatal(err)
	}

	for _, count := range []int{0, 1, 4} {
		if err := SplitCompanionIntoBodies(db, 1, companionID, count); err == nil {
			t.Errorf("count=%d: expected an error, got nil", count)
		}
	}

	if err := SplitCompanionIntoBodies(db, 1, companionID, 2); err != nil {
		t.Fatal(err)
	}
	if err := SplitCompanionIntoBodies(db, 1, companionID, 2); err == nil {
		t.Error("expected an error re-splitting an already-split companion, got nil")
	}
}

func TestMergeCompanionBodiesRestoresHPAndDeletesSiblings(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Sandman")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCompanionIntField(db, 1, companionID, "hp_max", sql.NullInt64{Int64: 30, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := SplitCompanionIntoBodies(db, 1, companionID, 3); err != nil {
		t.Fatal(err)
	}

	if err := MergeCompanionBodies(db, 1, companionID); err != nil {
		t.Fatal(err)
	}

	all, err := ListCompanions(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1", len(all))
	}
	merged, err := GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.MatryoshkaGroupID.Valid {
		t.Errorf("merged.MatryoshkaGroupID = %+v, want NULL", merged.MatryoshkaGroupID)
	}
	if merged.HPMax.Int64 != 30 {
		t.Errorf("merged.HPMax = %d, want 30 (lossless restore)", merged.HPMax.Int64)
	}
}

func TestMatryoshkaJutsuSlotsIsInIntFieldWhitelist(t *testing.T) {
	db := testCharDB(t)
	companionID, err := AddCompanion(db, 1, "puppet", "Sandman")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddCompanionIntField(db, 1, companionID, "matryoshka_jutsu_slots", 3); err != nil {
		t.Fatal(err)
	}
	c, err := GetCompanion(db, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.MatryoshkaJutsuSlots != 3 {
		t.Errorf("MatryoshkaJutsuSlots = %d, want 3", c.MatryoshkaJutsuSlots)
	}
}
