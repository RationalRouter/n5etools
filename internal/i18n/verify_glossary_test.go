package i18n

import (
	"slices"
	"testing"
)

// glossaryFixture mirrors the real shape that matters here: short terms
// that are also ordinary English words, and a longer term that contains one
// of them.
func glossaryFixture() *GlossaryChecker {
	return NewGlossaryChecker(map[string]string{
		"Light":        "Ligera",
		"Fire":         "Fuego",
		"Thrown":       "Arrojadiza",
		"Fire Release": "descarga de Fuego",
	})
}

// TestMissingTermsFlagsRealDrift is the check's whole reason to exist: a
// pinned mechanic used in its game sense, rendered as something other than
// the approved term, must still be reported. Every relaxation below is only
// acceptable while this keeps passing.
func TestMissingTermsFlagsRealDrift(t *testing.T) {
	gc := glossaryFixture()
	cases := []struct {
		name, en, target, want string
	}{
		{"capitalized mechanic followed by prose", "a Light weapon deals less", "un arma liviana inflige menos", "Light"},
		{"property named in prose", "gains the Thrown property", "gana la propiedad lanzadiza", "Thrown"},
		{"multi-word term", "casting a Fire Release jutsu", "lanzar un jutsu de Técnica de Fuego", "Fire Release"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := gc.MissingTerms(c.en, c.target)
			if !slices.Contains(got, c.want) {
				t.Errorf("MissingTerms(%q, %q) = %v, want it to contain %q", c.en, c.target, got, c.want)
			}
		})
	}
}

// TestMissingTermsIgnoresOrdinaryProse covers the case-sensitivity rule
// (see sourceTermRe). These are the false positives that made up the bulk
// of the Spanish findings: correct translations of ordinary English words
// that happen to collide with a pinned mechanic's name.
func TestMissingTermsIgnoresOrdinaryProse(t *testing.T) {
	gc := glossaryFixture()
	cases := []struct{ name, en, target string }{
		{"lowercase light", "in bright light you can see", "con luz brillante puedes ver"},
		{"lowercase fire", "the building was on fire", "el edificio estaba en llamas"},
		{"lowercase thrown", "the rock is thrown aside", "la roca es apartada"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gc.MissingTerms(c.en, c.target); len(got) != 0 {
				t.Errorf("MissingTerms(%q, %q) = %v, want none (ordinary prose, not the pinned mechanic)", c.en, c.target, got)
			}
		})
	}
}

// TestMissingTermsIgnoresProperNames covers the title-case-run rule (see
// termPresent). A capitalized term flanked by another capitalized word is
// part of a name, and a name's approved translation legitimately need not
// contain the glossary rendering -- "Fire Bomb" is "Bomba incendiaria".
func TestMissingTermsIgnoresProperNames(t *testing.T) {
	gc := glossaryFixture()
	cases := []struct{ name, en, target string }{
		{"trailing capital", "Fire Bomb", "Bomba incendiaria"},
		{"leading capital", "Mechanical Light Shield Seal", "Sello de Escudo Mecánico"},
		{"both sides", "No Light at the End", "Sin Salida"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gc.MissingTerms(c.en, c.target); len(got) != 0 {
				t.Errorf("MissingTerms(%q, %q) = %v, want none (proper name, not the pinned mechanic)", c.en, c.target, got)
			}
		})
	}
}

// TestMissingTermsIgnoresSentenceStart covers atSentenceStart. English
// capitalizes the first word of every sentence and bullet item, so a
// capital in that position says nothing about whether the pinned mechanic
// was meant.
func TestMissingTermsIgnoresSentenceStart(t *testing.T) {
	gc := glossaryFixture()
	cases := []struct{ name, en, target string }{
		{"bullet item", "• Light created by your Enlightening Grasp feature now lasts longer.",
			"• La luz creada por tu Agarre Iluminador ahora dura más."},
		{"after a period", "The orb glows. Light spills out for 20 feet.",
			"El orbe brilla. La luz se derrama 20 pies."},
		{"after a colon", "Effect: Fire spreads to the next space.",
			"Efecto: el fuego se extiende al siguiente espacio."},
		{"very start of field", "Thrown aside by the blast, it lands prone.",
			"Apartada por la explosión, cae derribada."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gc.MissingTerms(c.en, c.target); len(got) != 0 {
				t.Errorf("MissingTerms(%q, %q) = %v, want none (sentence-initial capital)", c.en, c.target, got)
			}
		})
	}
}

// TestMissingTermsSentenceStartDoesNotHideRealUse is the mirror of
// TestMissingTermsProperNameDoesNotHideRealUse: a sentence-initial
// occurrence must not excuse a genuine mid-sentence use in the same row.
func TestMissingTermsSentenceStartDoesNotHideRealUse(t *testing.T) {
	gc := glossaryFixture()
	en := "Light spills out for 20 feet. It is also a Light weapon."
	got := gc.MissingTerms(en, "La luz se derrama 20 pies. También es un arma liviana.")
	if !slices.Contains(got, "Light") {
		t.Errorf("MissingTerms(%q, ...) = %v, want it to contain %q", en, got, "Light")
	}
}

// TestMissingTermsProperNameDoesNotHideRealUse guards the obvious way the
// rule above could go wrong: one proper-name occurrence must not excuse a
// separate, genuine use of the same term in the same row.
func TestMissingTermsProperNameDoesNotHideRealUse(t *testing.T) {
	gc := glossaryFixture()
	en := "The Fire Bomb is a Light weapon."
	got := gc.MissingTerms(en, "La Bomba incendiaria es un arma liviana.")
	if !slices.Contains(got, "Light") {
		t.Errorf("MissingTerms(%q, ...) = %v, want it to contain %q", en, got, "Light")
	}
}

// TestMissingTermsLongestMatchWins covers dropSubsumedTerms: when both
// "Fire" and "Fire Release" match, only the specific one is reported --
// the bare "Fuego" the short entry wants is not even the right rendering
// there.
func TestMissingTermsLongestMatchWins(t *testing.T) {
	gc := glossaryFixture()
	got := gc.MissingTerms("a Fire Release technique", "una técnica de Técnica de Fuego")
	if slices.Contains(got, "Fire") {
		t.Errorf("MissingTerms = %v, want %q suppressed by the containing %q entry", got, "Fire", "Fire Release")
	}
}
