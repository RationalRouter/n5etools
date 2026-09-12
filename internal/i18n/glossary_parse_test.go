package i18n

import (
	"strings"
	"testing"
)

// Regression guard for the "Elemento X" bug (fixed 2026-09-12): DeepL only
// honors a glossary constraint for terms actually present in glossary.md's
// parsed tables, and the 5 base elements + Yin/Yang had no standalone entry
// at all -- only compound, field-specific phrases like "Ninjutsu / Fire
// Release" existed, which never matched "Fire Release" appearing on its own
// in clan_features/clan_traits/feats/subclass_features prose. That gap let
// DeepL choose a different, wrong noun ("Elemento") in some tables while
// getting it right in others. This test fails loudly if glossary.md is ever
// edited in a way that drops or breaks one of these entries, so the gap
// can't silently reopen for a future retranslation pass or a new locale.
func TestElementReleaseGlossaryCoverage(t *testing.T) {
	want := map[string]map[string]string{
		"Spanish": {
			"Earth Release":     "descarga de Tierra",
			"Fire Release":      "descarga de Fuego",
			"Water Release":     "descarga de Agua",
			"Wind Release":      "descarga del Viento",
			"Lightning Release": "descarga del Rayo",
			"Yin Release":       "descarga Yin",
			"Yang Release":      "descarga Yang",
		},
		"Portuguese": {
			"Earth Release":     "descarga da Terra",
			"Fire Release":      "descarga de Fogo",
			"Water Release":     "descarga de Água",
			"Wind Release":      "descarga do Vento",
			"Lightning Release": "descarga do Relâmpago",
			"Yin Release":       "descarga Yin",
			"Yang Release":      "descarga Yang",
		},
	}

	for locale, terms := range want {
		got, err := ParseGlossaryTables("glossary.md", locale)
		if err != nil {
			t.Fatalf("[%s] ParseGlossaryTables: %v", locale, err)
		}
		for source, wantTarget := range terms {
			gotTarget, ok := got[source]
			if !ok {
				t.Errorf("[%s] glossary.md has no entry for %q -- a future DeepL translate/retranslate run has no constraint stopping it from picking the wrong noun (\"Elemento X\") or wrong connotation (\"Liberación X\") again", locale, source)
				continue
			}
			if gotTarget != wantTarget {
				t.Errorf("[%s] %q -> %q, want %q", locale, source, gotTarget, wantTarget)
			}
		}
	}
}

// Regression guard for the same bug class from the other direction: even if
// a glossary entry exists, it must not itself encode "Elemento X" or
// "Liberación/Liberação X" for a base element -- that was literally how the
// bug got into the glossary in the first place (a stale "Yin Release ->
// Elemento Yin" row, justified by over-generalizing the unrelated combo-type
// convention).
func TestElementReleaseGlossaryRejectsKnownWrongTerms(t *testing.T) {
	baseElements := []string{"Earth Release", "Fire Release", "Water Release", "Wind Release", "Lightning Release", "Yin Release", "Yang Release"}
	wrongSubstrings := []string{"Elemento", "Liberaci", "Liberaç", "Libertaç"}

	for _, locale := range []string{"Spanish", "Portuguese"} {
		got, err := ParseGlossaryTables("glossary.md", locale)
		if err != nil {
			t.Fatalf("[%s] ParseGlossaryTables: %v", locale, err)
		}
		for _, source := range baseElements {
			target, ok := got[source]
			if !ok {
				continue // reported by TestElementReleaseGlossaryCoverage
			}
			for _, bad := range wrongSubstrings {
				if strings.Contains(strings.ToLower(target), strings.ToLower(bad)) {
					t.Errorf("[%s] %q -> %q contains the known-wrong substring %q -- this is the exact bug fixed 2026-09-12, do not reintroduce it", locale, source, target, bad)
				}
			}
		}
	}
}
