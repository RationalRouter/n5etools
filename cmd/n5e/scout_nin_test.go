package main

import "testing"

// TestScoutNinManeuversKnownCap checks every subclass's own "Maneuvers
// Known" column against values read directly off its own printed
// "Superior <Name> Table" text (see scoutNinManeuversKnownCap's doc
// comment for where each table lives) — one row per subclass at a level
// picked to distinguish it from a neighboring tier, plus the below-3rd-level
// zero case.
func TestScoutNinManeuversKnownCap(t *testing.T) {
	cases := []struct {
		subclass string
		level    int
		want     int
	}{
		// Below 3rd level: no subclass has any maneuvers yet.
		{arbiterScoutSubclassSlug, 2, 0},

		// Arbiter (Dice shape "a", Maneuvers shape "b"): 3rd->3, 6th->3, 8th->4, 18th->6.
		{arbiterScoutSubclassSlug, 3, 3},
		{arbiterScoutSubclassSlug, 6, 3},
		{arbiterScoutSubclassSlug, 8, 4},
		{arbiterScoutSubclassSlug, 18, 6},
		// Cloning and Trickster share Arbiter's own shape.
		{cloningScoutSubclassSlug, 8, 4},
		{tricksterScoutSubclassSlug, 18, 6},

		// Elemental (Dice shape "b", Maneuvers shape "a"): 3rd->3, 6th->4, 9th->5, 18th->8.
		{elementalScoutSubclassSlug, 3, 3},
		{elementalScoutSubclassSlug, 6, 4},
		{elementalScoutSubclassSlug, 9, 5},
		{elementalScoutSubclassSlug, 18, 8},
		// Barrier and Phantom share Elemental's own shape.
		{barrierScoutSubclassSlug, 9, 5},
		{phantomScoutSubclassSlug, 18, 8},

		// Assault and Pathfinder (Dice shape "c"): Maneuvers Known is
		// IDENTICAL to the Dice column, per both subclasses' own printed
		// tables — 3rd->3, 7th->4, 19th->7.
		{assaultScoutSubclassSlug, 3, 3},
		{assaultScoutSubclassSlug, 7, 4},
		{assaultScoutSubclassSlug, 19, 7},
		{pathfinderScoutSubclassSlug, 7, 4},

		// Tactical's own unique table: Maneuvers Known is identical to its
		// own Dice column — 3rd->3, 6th->4, 20th->10.
		{tacticalScoutSubclassSlug, 3, 3},
		{tacticalScoutSubclassSlug, 6, 4},
		{tacticalScoutSubclassSlug, 20, 10},
	}
	for _, c := range cases {
		if got := scoutNinManeuversKnownCap(c.subclass, c.level); got != c.want {
			t.Errorf("scoutNinManeuversKnownCap(%q, %d) = %d, want %d", c.subclass, c.level, got, c.want)
		}
	}
}

// An unrecognized/empty subclass slug (no subclass chosen yet) must never
// panic or silently pick some default shape — it's simply 0 maneuvers
// known, which the tab-data loader uses to hide the whole section.
func TestScoutNinManeuversKnownCapUnknownSubclass(t *testing.T) {
	if got := scoutNinManeuversKnownCap("", 20); got != 0 {
		t.Errorf("scoutNinManeuversKnownCap(\"\", 20) = %d, want 0", got)
	}
}
