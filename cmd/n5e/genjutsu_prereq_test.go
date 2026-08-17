package main

import "testing"

func TestGenjutsuMiragePrereqMet(t *testing.T) {
	allInception := map[string]bool{
		"reality marble":     true,
		"temporal stopwatch": true,
		"phantasmal force":   true,
	}
	allMirages := map[string]bool{
		"improved illusionary weapon": true,
		"supreme illusionary weapon":  true,
	}

	cases := []struct {
		name      string
		prereq    string
		level     int
		inception map[string]bool
		mirages   map[string]bool
		wantMet   bool
	}{
		{"blank prereq always passes", "", 1, nil, nil, true},
		{"level met (with period)", "9th level.", 9, nil, nil, true},
		{"level unmet", "9th level.", 8, nil, nil, false},
		{"level met (capital L, no period)", "13th Level", 13, nil, nil, true},
		{"known inception exact name", "Reality Marble", 1, map[string]bool{"reality marble": true}, nil, true},
		{"unknown inception exact name blocks", "Reality Marble", 1, nil, nil, false},
		{"inception name with trailing pledge artifact, known", "Reality Marble Genjutsu pledge", 1, map[string]bool{"reality marble": true}, nil, true},
		{"inception name with trailing pledge artifact, unknown blocks", "Phantasmal Force Genjutsu pledge", 1, nil, nil, false},
		{"known mirage name", "Improved Illusionary Weapon", 1, nil, map[string]bool{"improved illusionary weapon": true}, true},
		{"unknown mirage name blocks", "Supreme Illusionary Weapon", 1, nil, nil, false},
		// Combined clauses, both orderings — the real corpus has both.
		{"name-first combined, both met", "Temporal Stopwatch, 10th level", 10, map[string]bool{"temporal stopwatch": true}, nil, true},
		{"name-first combined, level unmet", "Temporal Stopwatch, 10th level", 9, map[string]bool{"temporal stopwatch": true}, nil, false},
		{"name-first combined, name unmet", "Temporal Stopwatch, 10th level", 10, nil, nil, false},
		{"level-first combined (Magnum Opus), both met", "13th level, Reality marble", 13, map[string]bool{"reality marble": true}, nil, true},
		{"level-first combined, level unmet", "13th level, Reality marble", 12, map[string]bool{"reality marble": true}, nil, false},
		// The 3 real unenforceable rows (Maddening Pain, Relentless Pain,
		// Vampiric Pain) — neither clause resolves to a known shape, so
		// nothing blocks.
		{"Maddening/Relentless Pain's own unresolvable clause", "Pain, Doubled Pain or Unlimited Pain", 1, nil, nil, true},
		{"Vampiric Pain's own unresolvable clause", "Pain, Doubled Pain or Unlimited Pain Genjutsu", 1, nil, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := genjutsuMiragePrereqMet(c.prereq, c.level, allInception, allMirages, c.inception, c.mirages)
			if got != c.wantMet {
				t.Errorf("genjutsuMiragePrereqMet(%q, level=%d) = %v, want %v", c.prereq, c.level, got, c.wantMet)
			}
		})
	}
}
