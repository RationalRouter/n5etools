package main

import "testing"

// jutsuElement/jutsuNeedsAnyAffinity read straight off the jutsu.keywords
// column as actually shipped in rules.db — including the two rows that
// glue two keywords together with no comma ("Bukijutsu Earth Release").
func TestJutsuElement(t *testing.T) {
	cases := []struct {
		keywords string
		want     string
	}{
		{"Fire Release, Ninjutsu", "Fire"},
		{"Water Release, Ninjutsu, Clash", "Water"},
		{"Bukijutsu Earth Release", "Earth"},
		{"Ninjutsu", ""},
		{"Any Nature Release", ""},
	}
	for _, c := range cases {
		if got := jutsuElement(c.keywords); got != c.want {
			t.Errorf("jutsuElement(%q) = %q, want %q", c.keywords, got, c.want)
		}
	}
	if !jutsuNeedsAnyAffinity("Any Nature Release, Ninjutsu") {
		t.Error("jutsuNeedsAnyAffinity should be true for 'Any Nature Release'")
	}
	if jutsuNeedsAnyAffinity("Fire Release, Ninjutsu") {
		t.Error("jutsuNeedsAnyAffinity should be false for a specific element")
	}
}

// jutsuElements must report BOTH elements a combo-affinity clan's own jutsu
// names (e.g. Bakuton's "Earth Release, Lightning Release, Ninjutsu") — a
// single-match jutsuElement would only ever see the first, Earth.
func TestJutsuElements(t *testing.T) {
	cases := []struct {
		keywords string
		want     []string
	}{
		{"Earth Release, Lightning Release, Ninjutsu", []string{"Earth", "Lightning"}},
		{"Fire Release, Ninjutsu", []string{"Fire"}},
		{"Ninjutsu", nil},
	}
	for _, c := range cases {
		got := jutsuElements(c.keywords)
		if len(got) != len(c.want) {
			t.Errorf("jutsuElements(%q) = %v, want %v", c.keywords, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("jutsuElements(%q) = %v, want %v", c.keywords, got, c.want)
				break
			}
		}
	}
}

func TestJutsuEligible(t *testing.T) {
	affinities := map[string]bool{"Fire": true}
	cases := []struct {
		name     string
		origin   string
		keywords string
		hasAny   bool
		want     bool
	}{
		{"wrong class/clan blocks regardless of element", "", "Fire Release, Ninjutsu", true, false},
		{"non-elemental class jutsu is fine", "class", "Ninjutsu", false, true},
		{"matching element passes", "class", "Fire Release, Ninjutsu", false, true},
		{"mismatched element blocks", "class", "Water Release, Ninjutsu", false, false},
		{"Any Nature Release needs at least one affinity", "clan", "Any Nature Release", true, true},
		{"Any Nature Release blocks with none", "clan", "Any Nature Release", false, false},
		{"combo clan jutsu blocks when neither half is held", "clan", "Earth Release, Lightning Release, Ninjutsu", false, false},
	}
	for _, c := range cases {
		if got := jutsuEligible(c.origin, c.keywords, affinities, c.hasAny, "", "E", false); got != c.want {
			t.Errorf("%s: jutsuEligible() = %v, want %v", c.name, got, c.want)
		}
	}
}

// A combo-affinity clan (e.g. Bakuton, Earth+Lightning) grants its own jutsu
// on EITHER half of the pair — a character who picked Lightning, not Earth,
// at 1st level must still be able to learn the clan's own jutsu, since
// jutsuElement alone would only ever check Earth (first in elementNames) and
// wrongly block them.
func TestJutsuEligibleComboAffinityEitherHalf(t *testing.T) {
	bakutonKeywords := "Earth Release, Lightning Release, Ninjutsu"
	if !jutsuEligible("clan", bakutonKeywords, map[string]bool{"Lightning": true}, false, "", "E", false) {
		t.Error("holding only the second-checked element (Lightning) should still pass")
	}
	if !jutsuEligible("clan", bakutonKeywords, map[string]bool{"Earth": true}, false, "", "E", false) {
		t.Error("holding only the first-checked element (Earth) should pass")
	}
	if jutsuEligible("clan", bakutonKeywords, map[string]bool{"Fire": true}, false, "", "E", false) {
		t.Error("holding neither half of the pair should block")
	}
}

// resolveElementalAffinities: a flat clan (Uchiha) needs no pick at all; a
// combo clan (Yuki) needs the 1st-level pick to show up, and gains the
// OTHER half automatically once the 7th-level feature is granted.
func TestResolveElementalAffinities(t *testing.T) {
	uchiha := resolveElementalAffinities("clan/uchiha", false, nil, nil)
	if len(uchiha) != 1 || uchiha[0].Element != "Fire" {
		t.Fatalf("Uchiha affinities = %+v, want just Fire", uchiha)
	}

	yukiNoPick := resolveElementalAffinities("clan/yuki", false, nil, nil)
	if len(yukiNoPick) != 0 {
		t.Fatalf("Yuki with no pick and no 7th-level feature = %+v, want none yet", yukiNoPick)
	}

	yukiPicked := resolveElementalAffinities("clan/yuki", false, nil, map[string]string{"clan": "Water"})
	if len(yukiPicked) != 1 || yukiPicked[0].Element != "Water" {
		t.Fatalf("Yuki after picking Water = %+v, want just Water", yukiPicked)
	}

	granted := map[string]bool{"clan/yuki/feature/ice-release": true}
	yukiBoth := resolveElementalAffinities("clan/yuki", false, granted, map[string]string{"clan": "Water"})
	if len(yukiBoth) != 2 {
		t.Fatalf("Yuki at 7th level = %+v, want both Wind and Water", yukiBoth)
	}
}
