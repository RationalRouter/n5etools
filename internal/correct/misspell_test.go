package correct

import "testing"

func TestMisspellFix(t *testing.T) {
	fixed, diffs := misspellFix("You will recieve the item.")
	if fixed != "You will receive the item." {
		t.Errorf("fixed = %q, want %q", fixed, "You will receive the item.")
	}
	if len(diffs) != 1 || diffs[0].tool != "misspell" || diffs[0].original != "recieve" || diffs[0].corrected != "receive" {
		t.Errorf("diffs = %+v", diffs)
	}
}

func TestMisspellFix_LeavesJargonAlone(t *testing.T) {
	s := "The Aburame clan channels chakra through hijutsu and ninjutsu."
	fixed, diffs := misspellFix(s)
	if fixed != s {
		t.Errorf("fixed = %q, want unchanged %q", fixed, s)
	}
	if len(diffs) != 0 {
		t.Errorf("diffs = %+v, want none", diffs)
	}
}
