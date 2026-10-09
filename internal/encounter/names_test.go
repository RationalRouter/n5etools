package encounter

import (
	"math/rand"
	"strings"
	"testing"
)

func inList(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestRandomNameNeverPairsTwoAnimeHalves(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seen := map[string]bool{}
	for i := 0; i < 5000; i++ {
		name := RandomName(rng, NoClan)
		parts := strings.Split(name, " ")
		if len(parts) != 2 {
			t.Fatalf("%q is not a given and a family name", name)
		}
		given, family := parts[0], parts[1]
		if !inList(modernGivenNames, given) && !inList(animeGivenNames, given) {
			t.Fatalf("%q: unknown given name", name)
		}
		if !inList(modernFamilyNames, family) && !inList(animeFamilyNames, family) {
			t.Fatalf("%q: unknown family name", name)
		}
		if inList(animeGivenNames, given) && inList(animeFamilyNames, family) {
			t.Fatalf("%q pairs two anime halves", name)
		}
		if famousNames[name] || given == family {
			t.Fatalf("%q is excluded", name)
		}
		seen[name] = true
	}
	if len(seen) < 1000 {
		t.Errorf("only %d distinct names in 5000 draws", len(seen))
	}
}

func TestRandomNameUsesClanFamilyName(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 2000; i++ {
		name := RandomName(rng, "Hyuga")
		given, family, _ := strings.Cut(name, " ")
		if family != "Hyuga" || !inList(modernGivenNames, given) {
			t.Fatalf("Hyuga adversary named %q", name)
		}
		if name == "Hinata Hyuga" {
			t.Fatal("generated a famous character's name")
		}
	}
	if got := ClanFamilyName("Kageyama T7"); got != "Kageyama" {
		t.Errorf("ClanFamilyName(Kageyama T7) = %q", got)
	}
	if got := ClanFamilyName("Synthetic Human"); got != "" {
		t.Errorf("ClanFamilyName(Synthetic Human) = %q, want none", got)
	}
}

func TestNameListsHaveNoDuplicates(t *testing.T) {
	for _, list := range [][]string{modernGivenNames, modernFamilyNames, animeGivenNames, animeFamilyNames} {
		seen := map[string]bool{}
		for _, n := range list {
			if seen[n] {
				t.Errorf("%q listed twice", n)
			}
			seen[n] = true
		}
	}
	for _, n := range animeGivenNames {
		if inList(modernGivenNames, n) {
			t.Errorf("%q is in both given-name lists", n)
		}
	}
	for _, n := range animeFamilyNames {
		if inList(modernFamilyNames, n) {
			t.Errorf("%q is in both family-name lists", n)
		}
	}
}
