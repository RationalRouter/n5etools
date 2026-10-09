package bingobookdata

import "testing"

func TestDataIsConsistent(t *testing.T) {
	blocks, err := StatBlocks()
	if err != nil {
		t.Fatal(err)
	}
	traits, err := GeneralTraits()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SpecialistLists(); err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 {
		t.Fatal("no stat blocks loaded")
	}
	for _, p := range Check(blocks, traits) {
		t.Error(p)
	}
}

func TestModFor(t *testing.T) {
	for score, want := range map[int]int{1: -5, 7: -2, 8: -1, 9: -1, 10: 0, 11: 0, 12: 1, 21: 5} {
		if got := modFor(score); got != want {
			t.Errorf("modFor(%d) = %d, want %d", score, got, want)
		}
	}
}

func TestTraitSlug(t *testing.T) {
	for name, want := range map[string]string{
		"Abandon a Friend":                "adversary-beta-general-trait/abandon-a-friend",
		"Aura: Destabilized Chakra Field": "adversary-beta-general-trait/aura-destabilized-chakra-field",
		"Believe it!":                     "adversary-beta-general-trait/believe-it",
		"Can't Hold Me Down":              "adversary-beta-general-trait/cant-hold-me-down",
	} {
		if got := TraitSlug(name); got != want {
			t.Errorf("TraitSlug(%q) = %q, want %q", name, got, want)
		}
	}
}
