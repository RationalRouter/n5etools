package correct

import "testing"

func TestCuratedFix_AppliesKnownEntry(t *testing.T) {
	in := "The jutsu gains gains the benefits of being Overcharged."
	got, diffs := curatedFix(in)
	if want := "The jutsu gains the benefits of being Overcharged."; got != want {
		t.Errorf("curatedFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}

func TestCuratedFix_StaleKeyIsANoOp(t *testing.T) {
	in := "some unrelated sentence that matches nothing in the table"
	got, diffs := curatedFix(in)
	if got != in || len(diffs) != 0 {
		t.Errorf("curatedFix(%q) = %q, diffs=%d; want unchanged", in, got, len(diffs))
	}
}
