package correct

import "testing"

func TestPenaltyPluralFix_SingularArticlePluralNoun(t *testing.T) {
	got, diffs := penaltyPluralFix("they have a -3 penalties on all Constitution Saving Throws.")
	if want := "they have a -3 penalty on all Constitution Saving Throws."; got != want {
		t.Errorf("penaltyPluralFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}

func TestPenaltyPluralFix_NoMatch(t *testing.T) {
	got, diffs := penaltyPluralFix("they have a -3 penalty on all Constitution Saving Throws.")
	if want := "they have a -3 penalty on all Constitution Saving Throws."; got != want {
		t.Errorf("penaltyPluralFix() = %q, want %q", got, want)
	}
	if diffs != nil {
		t.Errorf("diffs = %v, want nil", diffs)
	}
}
