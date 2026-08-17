package correct

import "testing"

func TestPossessiveFix_DropsFixed(t *testing.T) {
	cases := map[string]string{
		"a creatures cells":            "a creature’s cells",
		"your weapons range":           "your weapon’s range",
		"the targets Hit Points":       "the target’s Hit Points",
		"the casters next turn":        "the caster’s next turn",
		"the clones body":              "the clone’s body",
		"a creatures Damage Reduction": "a creature’s Damage Reduction",
		"this Adversaries hit points":  "this Adversary’s hit points",
		"these Adversaries next turn":  "these Adversary’s next turn",
		"these Adversaries allies":     "these Adversary’s allies",
	}
	for in, want := range cases {
		got, diffs := possessiveFix(in)
		if got != want {
			t.Errorf("possessiveFix(%q) = %q, want %q", in, got, want)
		}
		if len(diffs) != 1 {
			t.Errorf("possessiveFix(%q) diffs = %d, want 1", in, len(diffs))
		}
	}
}

func TestPossessiveFix_LeavesGenuinePluralsAlone(t *testing.T) {
	cases := []string{
		"the creatures gain advantage",
		"all creatures within 10 feet",
		"your weapons are ready",
		"these Adversaries deal damage",
	}
	for _, in := range cases {
		got, diffs := possessiveFix(in)
		if got != in || len(diffs) != 0 {
			t.Errorf("possessiveFix(%q) = %q, diffs=%d; want unchanged", in, got, len(diffs))
		}
	}
}
