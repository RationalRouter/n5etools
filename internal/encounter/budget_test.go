package encounter

import "testing"

func TestBudget(t *testing.T) {
	cases := []struct {
		size, level int
		difficulty  string
		want        int
	}{
		{4, 5, "standard", 60},
		{4, 5, "easy", 40},
		{4, 5, "hard", 80},
		{4, 5, "deadly", 100},
		{1, 1, "standard", 3},
		{0, 0, "standard", 3}, // clamped to size=1, level=1
	}
	for _, c := range cases {
		if got := Budget(c.size, c.level, c.difficulty); got != c.want {
			t.Errorf("Budget(%d, %d, %q) = %d, want %d", c.size, c.level, c.difficulty, got, c.want)
		}
	}
}

func TestBudgetUnknownDifficultyFallsBackToStandard(t *testing.T) {
	if got, want := Budget(4, 5, "nonsense"), Budget(4, 5, "standard"); got != want {
		t.Errorf("Budget with unknown difficulty = %d, want fallback %d", got, want)
	}
}

func TestRankCostEscalatesWithRankAndLevel(t *testing.T) {
	if rankCost("minion", 2) >= rankCost("standard", 2) {
		t.Error("Minion should cost less than Standard at the same level")
	}
	if rankCost("standard", 2) >= rankCost("elite", 2) {
		t.Error("Standard should cost less than Elite at the same level")
	}
	if rankCost("elite", 2) >= rankCost("solo", 2) {
		t.Error("Elite should cost less than Solo at the same level")
	}
	if rankCost("standard", 2) >= rankCost("standard", 10) {
		t.Error("higher level should cost more at the same rank")
	}
}

func TestValidDifficulty(t *testing.T) {
	for _, d := range Difficulties {
		if !ValidDifficulty(d) {
			t.Errorf("ValidDifficulty(%q) = false, want true", d)
		}
	}
	if ValidDifficulty("nonsense") {
		t.Error("ValidDifficulty(\"nonsense\") = true, want false")
	}
}
