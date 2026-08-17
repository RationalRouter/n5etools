package correct

import "testing"

func TestHyphenSpaceFix_ClosesConfirmedSplits(t *testing.T) {
	cases := map[string]string{
		"D- Rank":              "D-Rank",
		"a 30- foot cube":      "a 30-foot cube",
		"hand-to- hand combat": "hand-to-hand combat",
		"a Nin- Dog":           "a Nin-Dog",
		"Will- O-Wisp":         "Will-O-Wisp",
		"Will-O- Wisps":        "Will-O-Wisps",
		"ever- so-slightly":    "ever-so-slightly",
		"head- to-head match":  "head-to-head match",
	}
	for in, want := range cases {
		got, diffs := hyphenSpaceFix(in)
		if got != want {
			t.Errorf("hyphenSpaceFix(%q) = %q, want %q", in, got, want)
		}
		if len(diffs) == 0 {
			t.Errorf("hyphenSpaceFix(%q) produced no diffs", in)
		}
	}
}

func TestHyphenSpaceFix_LeavesDashPunctuationAlone(t *testing.T) {
	// Both of these are real corpus sentences using a bare hyphen as
	// dash-style punctuation, not a split compound word — closing them
	// would create a nonsense fused word ("alone-for", "foe-the").
	cases := []string{
		"alone- for a formative part of your life",
		"adjacent to it-friend or foe- the affected creature",
		"critical hit on a roll of 19- Tiger Grapple",
	}
	for _, in := range cases {
		got, diffs := hyphenSpaceFix(in)
		if got != in || len(diffs) != 0 {
			t.Errorf("hyphenSpaceFix(%q) = %q, diffs=%d; want unchanged", in, got, len(diffs))
		}
	}
}
