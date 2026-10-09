package i18n

import "testing"

func TestPlaceholderRoundTrip(t *testing.T) {
	cases := []string{
		"A, B, C's of Defeat",
		"Drag to reorder %s",
		`Q&A <notes> "quoted" %d of %d`,
		"Under 50% hit points",
	}
	for _, in := range cases {
		if got := RestorePlaceholders(ProtectPlaceholders(in)); got != in {
			t.Errorf("round trip %q = %q", in, got)
		}
	}
}

// DeepL's html tag handling returns entity-escaped text.
func TestRestorePlaceholdersDecodesEntities(t *testing.T) {
	got := RestorePlaceholders("C&#x27;s &amp; <ph>%d</ph> d&#39;arme")
	if want := "C's & %d d'arme"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
