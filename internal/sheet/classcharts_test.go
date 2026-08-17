package sheet

import (
	"os"
	"testing"
)

// Regression against the real Mastersheet (verified 2026-07-18 against the
// v3.1 sheet). Skips when the sheet is absent.
func TestParseClassChartsMastersheet(t *testing.T) {
	path := "/home/sergio/Documents/N5E/Mastersheet - N5E v3.1.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("mastersheet not available: %v", err)
	}
	charts, anomalies, err := ParseClassCharts(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range anomalies {
		t.Errorf("anomaly %s: %s", a.Subject, a.Problem)
	}
	if len(charts) != 11 {
		t.Fatalf("charts = %d, want 11", len(charts))
	}
	for _, c := range charts {
		if len(c.Levels) != 20 {
			t.Errorf("%s: %d levels, want 20", c.Name, len(c.Levels))
		}
		if c.HitDie == 0 || c.ChakraDie == 0 {
			t.Errorf("%s: dice d%d/d%d", c.Name, c.HitDie, c.ChakraDie)
		}
		for _, lr := range c.Levels {
			if lr.ProfBonus < 3 || lr.ProfBonus > 9 {
				t.Errorf("%s L%d: proficiency bonus %d out of range", c.Name, lr.Level, lr.ProfBonus)
			}
		}
	}

	// Pin one chart in detail.
	g := charts[0]
	if g.Name != "Genjutsu-Specialist" || g.HitDie != 6 || g.ChakraDie != 12 {
		t.Errorf("first chart = %+v", g)
	}
	l20 := g.Levels[19]
	if l20.ProfBonus != 9 || l20.JutsuKnown == nil || *l20.JutsuKnown != 20 {
		t.Errorf("Genjutsu L20 = %+v", l20)
	}
	if len(l20.Resources) != 1 || l20.Resources[0] != (Resource{Name: "Malleable Mirages", Value: "11"}) {
		t.Errorf("Genjutsu L20 resources = %+v", l20.Resources)
	}
	if l20.FeaturesText != "The Prestige, Master of Illusion (2)" {
		t.Errorf("Genjutsu L20 features = %q", l20.FeaturesText)
	}
	// Hunter's resource column holds dice values.
	h := charts[1]
	if h.Levels[19].Resources[0] != (Resource{Name: "Lethal Attack", Value: "10d8"}) {
		t.Errorf("Hunter L20 resources = %+v", h.Levels[19].Resources)
	}
}
