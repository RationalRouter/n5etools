package main

import "testing"

// puppetTacticCap reads directly off "Tactics of the Craft"'s own printed
// text: "Beginning at 2nd level, you gain one tactic. At 11th level, you
// gain a second tactic." Black Technique Puppeteer stacks its own bonus on
// top ("Lastly, you gain an additional Tactic from the Tactics of the Craft
// feature. Gain another Tactic at 10th level.", Black Technique
// Proficiency, L2).
func TestPuppetTacticCap(t *testing.T) {
	cases := []struct {
		level int
		color string
		want  int
	}{
		{1, "", 0}, {2, "", 1}, {10, "", 1}, {11, "", 2}, {20, "", 2},
		{1, "Black", 0}, {2, "Black", 2}, {9, "Black", 2}, {10, "Black", 3}, {11, "Black", 4}, {20, "Black", 4},
		{2, "White", 1}, {11, "Purple", 2},
	}
	for _, c := range cases {
		if got := puppetTacticCap(c.level, c.color); got != c.want {
			t.Errorf("puppetTacticCap(%d, %q) = %d, want %d", c.level, c.color, got, c.want)
		}
	}
}
