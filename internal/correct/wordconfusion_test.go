package correct

import "testing"

func TestWordConfusionFix_AllayToAlly(t *testing.T) {
	got, diffs := wordConfusionFix("your Bonded allay gain advantage")
	if want := "your Bonded ally gain advantage"; got != want {
		t.Errorf("wordConfusionFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}
