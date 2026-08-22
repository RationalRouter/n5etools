package main

import "testing"

// TestCharacterMedicalRankCap covers characterMedicalRankCap's three access
// tiers: unrestricted for Medical-Nin, level-gated for Science-Nin's Mad
// Scientist (D at 3rd, C at 9th, B at 14th, per Biotic Mastery's own text),
// and no access at all otherwise — including a plain Ninjutsu-casting class
// with neither, which is the exact shape the audit found silently treated
// as unrestricted before this gate existed.
func TestCharacterMedicalRankCap(t *testing.T) {
	madScientist := map[string]bool{madScientistBioticMasteryFeatureSlug: true}
	cases := []struct {
		name       string
		classSlugs []string
		granted    map[string]bool
		level      int
		want       string
	}{
		{"Medical-Nin is unrestricted regardless of level", []string{medicalNinSlug}, nil, 1, "S"},
		{"plain Ninjutsu class with no special access gets none", []string{"class/cooking-nin"}, nil, 20, ""},
		{"Mad Scientist below 3rd (feature not yet granted) gets none", []string{"class/science-nin"}, nil, 2, ""},
		{"Mad Scientist at 3rd is capped at D", []string{"class/science-nin"}, madScientist, 3, "D"},
		{"Mad Scientist at 8th is still capped at D", []string{"class/science-nin"}, madScientist, 8, "D"},
		{"Mad Scientist at 9th is capped at C", []string{"class/science-nin"}, madScientist, 9, "C"},
		{"Mad Scientist at 13th is still capped at C", []string{"class/science-nin"}, madScientist, 13, "C"},
		{"Mad Scientist at 14th is capped at B", []string{"class/science-nin"}, madScientist, 14, "B"},
		{"Mad Scientist at 20th is still capped at B, not S", []string{"class/science-nin"}, madScientist, 20, "B"},
	}
	for _, c := range cases {
		if got := characterMedicalRankCap(c.classSlugs, c.granted, c.level); got != c.want {
			t.Errorf("%s: characterMedicalRankCap() = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestJutsuEligibleMedicalGate reproduces the audit's own finding: every
// Ninjutsu-casting class (all eleven, since Medical jutsu are classified
// Ninjutsu like everything else those classes cast) used to pass
// jutsuEligible for ANY Medical jutsu purely on class-discipline origin,
// with no regard for rank or whether the class has real Medical training at
// all. It also covers the fix's own carve-out: a clan-curated Medical
// jutsu (origin == "clan" — Hanami, Hyuga, Shakuton, and Uzumaki all have
// signature Medical Hijutsu seeded directly into clan_jutsu) is real,
// specific access and must not be blocked by a class-side rank cap.
func TestJutsuEligibleMedicalGate(t *testing.T) {
	medicalKeywords := "Ninjutsu, Medical"
	noAffinities := map[string]bool{}

	cases := []struct {
		name           string
		origin         string
		medicalRankCap string
		rank           string
		want           bool
	}{
		{"class-origin Medical jutsu blocked with no Medical access at all", "class", "", "E", false},
		{"class-origin Medical jutsu within a Mad Scientist's D-Rank cap passes", "class", "D", "D", true},
		{"class-origin Medical jutsu above a Mad Scientist's D-Rank cap blocks", "class", "D", "C", false},
		{"class-origin Medical jutsu allowed at exactly the cap's rank", "class", "C", "C", true},
		{"Medical-Nin's S cap allows every rank", "class", "S", "S", true},
		{"clan-origin Medical jutsu passes with no class-side access at all — the clan curated it directly", "clan", "", "S", true},
	}
	for _, c := range cases {
		if got := jutsuEligible(c.origin, medicalKeywords, noAffinities, false, c.medicalRankCap, c.rank, false); got != c.want {
			t.Errorf("%s: jutsuEligible() = %v, want %v", c.name, got, c.want)
		}
	}

	// A non-Medical jutsu is entirely unaffected by medicalRankCap, even
	// when it's empty (no Medical access at all) — the gate only applies
	// once jutsuIsMedical(keywords) is true.
	if !jutsuEligible("class", "Ninjutsu", noAffinities, false, "", "S", false) {
		t.Error("a plain non-Medical Ninjutsu jutsu should be unaffected by an empty medicalRankCap")
	}
}

// TestJutsuEligibleByTheBookGrant covers Patissier Chef's own narrow bypass
// of the Medical gate above: byTheBookGrant lets a specific curated healing
// jutsu through with no Medical access at all, but only when the caller
// actually asserts the grant — jutsuEligible itself doesn't know which slug
// is being checked, so a false byTheBookGrant must still block exactly like
// the ungated case above.
func TestJutsuEligibleByTheBookGrant(t *testing.T) {
	medicalKeywords := "Ninjutsu, Medical"
	noAffinities := map[string]bool{}

	if !jutsuEligible("class", medicalKeywords, noAffinities, false, "", "C", true) {
		t.Error("byTheBookGrant should bypass the Medical rank-cap gate entirely")
	}
	if jutsuEligible("class", medicalKeywords, noAffinities, false, "", "C", false) {
		t.Error("without byTheBookGrant, a class-origin Medical jutsu with no rank cap should still block")
	}
}
