package main

import (
	"reflect"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// TestFullMetalShinobiResistances covers the feature's own staged
// progression: "At Level 6 you choose to gain resistance to either
// Bludgeoning, Piercing, or Slashing damage. At Level 9 you can choose
// another. At Level 14 you gain the last that you did not choose."
func TestFullMetalShinobiResistances(t *testing.T) {
	cases := []struct {
		name  string
		level int
		picks map[string]string
		want  []string
	}{
		{"below 6th: no resistances at all, even with picks stored ahead of time", 5, map[string]string{"sixth-level": "Bludgeoning"}, nil},
		{"6th with a pick: just that one", 6, map[string]string{"sixth-level": "Piercing"}, []string{"Piercing"}},
		{"6th with no pick yet: nothing", 6, nil, nil},
		{"8th (past 6th, before 9th): still just the 6th-level pick", 8, map[string]string{"sixth-level": "Slashing"}, []string{"Slashing"}},
		{"9th with both picks: both", 9, map[string]string{"sixth-level": "Bludgeoning", "ninth-level": "Piercing"}, []string{"Bludgeoning", "Piercing"}},
		{"13th with only the 6th-level pick made (9th skipped): just the one", 13, map[string]string{"sixth-level": "Bludgeoning"}, []string{"Bludgeoning"}},
		{"14th completes the last one automatically", 14, map[string]string{"sixth-level": "Bludgeoning", "ninth-level": "Piercing"}, []string{"Bludgeoning", "Piercing", "Slashing"}},
		{"14th with no picks ever made still ends up with all three", 14, nil, []string{"Bludgeoning", "Piercing", "Slashing"}},
		{"20th behaves the same as 14th", 20, map[string]string{"ninth-level": "Slashing"}, []string{"Slashing", "Bludgeoning", "Piercing"}},
	}
	for _, c := range cases {
		got := fullMetalShinobiResistances(c.level, c.picks)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: fullMetalShinobiResistances(%d, %v) = %v, want %v", c.name, c.level, c.picks, got, c.want)
		}
	}
}

// TestFullMetalShinobiSlots covers the picker's own visibility and option
// exclusion: a slot only appears once its level is reached, and each slot's
// options exclude whatever the OTHER slot has already picked (but not its
// own current pick, so it stays changeable).
func TestFullMetalShinobiSlots(t *testing.T) {
	if got := fullMetalShinobiSlots(5, nil); got != nil {
		t.Errorf("below 6th level: want no open slots, got %v", got)
	}

	slots := fullMetalShinobiSlots(6, nil)
	if len(slots) != 1 || slots[0].SlotKey != "sixth-level" {
		t.Fatalf("at 6th level: want just the sixth-level slot open, got %v", slots)
	}
	if len(slots[0].Options) != 3 {
		t.Errorf("6th-level slot with nothing picked yet: want all 3 damage types offered, got %v", slots[0].Options)
	}

	slots = fullMetalShinobiSlots(10, map[string]string{"sixth-level": "Bludgeoning"})
	if len(slots) != 2 {
		t.Fatalf("at 10th level: want both slots open, got %v", slots)
	}
	var sixth, ninth fullMetalShinobiSlot
	for _, s := range slots {
		switch s.SlotKey {
		case "sixth-level":
			sixth = s
		case "ninth-level":
			ninth = s
		}
	}
	if !reflect.DeepEqual(sixth.Options, []string{"Bludgeoning", "Piercing", "Slashing"}) {
		t.Errorf("sixth-level's own current pick must stay selectable (free to change): got options %v", sixth.Options)
	}
	if sixth.Current != "Bludgeoning" {
		t.Errorf("sixth-level Current = %q, want Bludgeoning", sixth.Current)
	}
	if !reflect.DeepEqual(ninth.Options, []string{"Piercing", "Slashing"}) {
		t.Errorf("ninth-level must exclude Bludgeoning (already taken by the sixth-level slot): got options %v", ninth.Options)
	}
	if ninth.Current != "" {
		t.Errorf("ninth-level Current = %q, want empty (no pick made yet)", ninth.Current)
	}
}

// TestFullMetalShinobiPassiveRowsEndToEnd covers the full path from stored
// picks through to the sheet's own Resistances panel: fullMetalShinobiPassiveRows
// reads the character's picks and emits synthetic rows, which
// computePassiveTraits then resolves against passiveTraitGrants into actual
// damage resistances. Only reachable at all once the character actually
// holds Full-Metal Shinobi (Shinobi-Ware) — see
// TestFullMetalShinobiPassiveRowsGatedOnFeature for the regression this
// guards against.
func TestFullMetalShinobiPassiveRowsEndToEnd(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Kaito')`); err != nil {
		t.Fatal(err)
	}
	hasFeature := []grantedFeatureRow{{Slug: scienceNinFullMetalShinobiFeatureSlug, Name: "Full-Metal Shinobi"}}

	// Before 6th level: no rows, no resistances.
	rows, err := s.fullMetalShinobiPassiveRows(1, 5, hasFeature)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("below 6th level: want no synthetic rows, got %v", rows)
	}

	if err := charstore.SetFullMetalShinobiResistance(s.charDB, 1, "sixth-level", "Piercing"); err != nil {
		t.Fatal(err)
	}
	rows, err = s.fullMetalShinobiPassiveRows(1, 8, hasFeature)
	if err != nil {
		t.Fatal(err)
	}
	traits := computePassiveTraits(rows, 8)
	if len(traits.Resistances) != 1 || traits.Resistances[0].Target != "Piercing" {
		t.Fatalf("at 8th level with only the 6th-level pick made: want just Piercing resistance, got %v", traits.Resistances)
	}

	// At 14th, the last damage type (Slashing) completes automatically even
	// though the 9th-level slot was never picked.
	rows, err = s.fullMetalShinobiPassiveRows(1, 14, hasFeature)
	if err != nil {
		t.Fatal(err)
	}
	traits = computePassiveTraits(rows, 14)
	if len(traits.Resistances) != 3 {
		t.Fatalf("at 14th level: want all 3 damage resistances, got %v", traits.Resistances)
	}
	targets := map[string]bool{}
	for _, r := range traits.Resistances {
		targets[r.Target] = true
	}
	for _, dt := range []string{"Bludgeoning", "Piercing", "Slashing"} {
		if !targets[dt] {
			t.Errorf("at 14th level: missing %s resistance, got %v", dt, traits.Resistances)
		}
	}
}

// TestFullMetalShinobiPassiveRowsGatedOnFeature guards against the bug
// found while building S.E.N.Ts: fullMetalShinobiResistances' own
// 14th-level auto-complete clause produces all 3 damage types from an
// EMPTY picks map, so without a has-the-feature gate, ANY character of ANY
// class simply reaching character level 14 would silently gain full
// Full-Metal Shinobi resistances.
func TestFullMetalShinobiPassiveRowsGatedOnFeature(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (2, 'Not Shinobi-Ware')`); err != nil {
		t.Fatal(err)
	}
	rows, err := s.fullMetalShinobiPassiveRows(2, 14, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("14th-level character without Full-Metal Shinobi: want no synthetic rows, got %v", rows)
	}
}
