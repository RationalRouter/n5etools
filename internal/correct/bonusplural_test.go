package correct

import "testing"

func TestBonusPluralFix_SingularArticlePluralNoun(t *testing.T) {
	got, diffs := bonusPluralFix("You gain a +2 bonuses to your AC.")
	if want := "You gain a +2 bonus to your AC."; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
	if len(diffs) != 1 {
		t.Errorf("diffs = %d, want 1", len(diffs))
	}
}

func TestBonusPluralFix_NoSign(t *testing.T) {
	got, _ := bonusPluralFix("the weapon has a 10 bonuses to trip and grapple checks")
	if want := "the weapon has a 10 bonus to trip and grapple checks"; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
}

func TestBonusPluralFix_MultipleOccurrences(t *testing.T) {
	got, diffs := bonusPluralFix("gains a +2 bonuses to AC, deals +1 extra and a +3 bonuses to the damage roll.")
	if want := "gains a +2 bonus to AC, deals +1 extra and a +3 bonus to the damage roll."; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
	if len(diffs) != 2 {
		t.Errorf("diffs = %d, want 2", len(diffs))
	}
}

func TestBonusPluralFix_NoMatch(t *testing.T) {
	got, diffs := bonusPluralFix("You gain a +2 bonus to your AC.")
	if want := "You gain a +2 bonus to your AC."; got != want {
		t.Errorf("bonusPluralFix() = %q, want %q", got, want)
	}
	if diffs != nil {
		t.Errorf("diffs = %v, want nil", diffs)
	}
}
