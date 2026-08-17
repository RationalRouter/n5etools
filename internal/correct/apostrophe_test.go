package correct

import "testing"

func TestApostropheFix_UserExample(t *testing.T) {
	in := "You are proficient with Katana's, Broadswords and Odachi's."
	want := "You are proficient with Katanas, Broadswords and Odachis."
	got, diffs := apostropheFix(in)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(diffs) != 2 {
		t.Fatalf("diffs = %+v, want 2", diffs)
	}
	if diffs[0].original != "Katana's," || diffs[0].corrected != "Katanas," {
		t.Errorf("diffs[0] = %+v", diffs[0])
	}
	if diffs[1].original != "Odachi's." || diffs[1].corrected != "Odachis." {
		t.Errorf("diffs[1] = %+v", diffs[1])
	}
}

func TestApostropheFix_CurlyApostrophe(t *testing.T) {
	got, diffs := apostropheFix("proficient with Odachi’s.")
	if got != "proficient with Odachis." {
		t.Errorf("got %q", got)
	}
	if len(diffs) != 1 {
		t.Fatalf("diffs = %+v, want 1", diffs)
	}
}

func TestApostropheFix_LeavesGenuinePossessivesAlone(t *testing.T) {
	s := "The Hokage's decision was final."
	got, diffs := apostropheFix(s)
	if got != s {
		t.Errorf("got %q, want unchanged %q", got, s)
	}
	if len(diffs) != 0 {
		t.Errorf("diffs = %+v, want none", diffs)
	}
}

func TestApostropheFix_LeavesContractionsAlone(t *testing.T) {
	for _, s := range []string{
		"It's, admittedly, a stretch.",
		"That's, in short, the plan.",
	} {
		got, diffs := apostropheFix(s)
		if got != s {
			t.Errorf("got %q, want unchanged %q", got, s)
		}
		if len(diffs) != 0 {
			t.Errorf("s=%q diffs = %+v, want none", s, diffs)
		}
	}
}
