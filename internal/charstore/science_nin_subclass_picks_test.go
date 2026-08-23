package charstore

import "testing"

// TestAddScienceNinSubclassPickBIMIncrementsQuantity covers Grenadier's own
// exception to this table's usual "a character either holds a given pick or
// doesn't" semantics: "you can pick any modification with 'B.I.M' in the
// name more than once, other than the Barrier B.I.M." A repeat add against
// the same 'bim' option_slug must increment the stored row's quantity
// rather than no-op.
func TestAddScienceNinSubclassPickBIMIncrementsQuantity(t *testing.T) {
	db := testCharDB(t)
	slug := "class/science-nin/option/explosive-modifications/minor/entry/timer-b-i-m"

	if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug, ""); err != nil {
		t.Fatal(err)
	}
	picks, err := ListScienceNinSubclassPicks(db, 1, ScienceNinPickBIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Quantity != 1 {
		t.Fatalf("after first pick: picks = %+v, want exactly one row with quantity 1", picks)
	}

	if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug, ""); err != nil {
		t.Fatal(err)
	}
	picks, err = ListScienceNinSubclassPicks(db, 1, ScienceNinPickBIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Quantity != 2 {
		t.Fatalf("after second pick of the same type: picks = %+v, want exactly one row with quantity 2 (still a single row, not two)", picks)
	}

	if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug, ""); err != nil {
		t.Fatal(err)
	}
	picks, err = ListScienceNinSubclassPicks(db, 1, ScienceNinPickBIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Quantity != 3 {
		t.Fatalf("after third pick of the same type: picks = %+v, want quantity 3", picks)
	}
}

// TestRemoveScienceNinSubclassPickBIMDecrementsThenDeletes covers the
// removal-side counterpart: each removal drops the held quantity by one,
// and only the removal that brings it to zero actually deletes the row.
func TestRemoveScienceNinSubclassPickBIMDecrementsThenDeletes(t *testing.T) {
	db := testCharDB(t)
	slug := "class/science-nin/option/explosive-modifications/minor/entry/timer-b-i-m"
	for i := 0; i < 3; i++ {
		if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug, ""); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemoveScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug); err != nil {
		t.Fatal(err)
	}
	picks, err := ListScienceNinSubclassPicks(db, 1, ScienceNinPickBIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Quantity != 2 {
		t.Fatalf("after removing one of three: picks = %+v, want quantity 2, row still present", picks)
	}

	if err := RemoveScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug); err != nil {
		t.Fatal(err)
	}
	picks, err = ListScienceNinSubclassPicks(db, 1, ScienceNinPickBIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Quantity != 1 {
		t.Fatalf("after removing two of three: picks = %+v, want quantity 1, row still present", picks)
	}

	if err := RemoveScienceNinSubclassPick(db, 1, ScienceNinPickBIM, slug); err != nil {
		t.Fatal(err)
	}
	picks, err = ListScienceNinSubclassPicks(db, 1, ScienceNinPickBIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("after removing the last held copy: picks = %+v, want the row gone entirely", picks)
	}
}

// TestAddScienceNinSubclassPickNonBIMStaysSingleRow confirms every category
// OTHER than 'bim' keeps its existing no-duplicate semantics unchanged: a
// repeat add against the same option_slug still updates in place (pool,
// for Inversion Serums) rather than incrementing any count, and quantity
// stays at its schema default of 1 throughout.
func TestAddScienceNinSubclassPickNonBIMStaysSingleRow(t *testing.T) {
	db := testCharDB(t)
	slug := "class/science-nin/option/inversion-serums/minor/entry/some-serum"

	if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickInversionSerum, slug, "mending"); err != nil {
		t.Fatal(err)
	}
	picks, err := ListScienceNinSubclassPicks(db, 1, ScienceNinPickInversionSerum)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Pool != "mending" || picks[0].Quantity != 1 {
		t.Fatalf("after first pick: picks = %+v, want one row, pool mending, quantity 1", picks)
	}

	// A repeat pick with a different pool must switch the pool, not add a
	// second row or touch quantity — the pre-existing Inversion Serums
	// "re-submit to change pool" contract, untouched by the B.I.M-specific
	// quantity change.
	if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickInversionSerum, slug, "maiming"); err != nil {
		t.Fatal(err)
	}
	picks, err = ListScienceNinSubclassPicks(db, 1, ScienceNinPickInversionSerum)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 || picks[0].Pool != "maiming" || picks[0].Quantity != 1 {
		t.Fatalf("after re-picking with a different pool: picks = %+v, want still one row, pool switched to maiming, quantity still 1", picks)
	}
}

// TestRemoveScienceNinSubclassPickNonBIMDeletesWhole confirms every
// category other than 'bim' still deletes its row outright on removal — no
// decrement step, since quantity never leaves 1 there.
func TestRemoveScienceNinSubclassPickNonBIMDeletesWhole(t *testing.T) {
	db := testCharDB(t)
	slug := "class/science-nin/option/arsenal-modifications/minor/entry/some-mod"
	if err := AddScienceNinSubclassPick(db, 1, ScienceNinPickArsenalMod, slug, ""); err != nil {
		t.Fatal(err)
	}
	if err := RemoveScienceNinSubclassPick(db, 1, ScienceNinPickArsenalMod, slug); err != nil {
		t.Fatal(err)
	}
	picks, err := ListScienceNinSubclassPicks(db, 1, ScienceNinPickArsenalMod)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("after removing a single-copy pick: picks = %+v, want gone entirely", picks)
	}
}
