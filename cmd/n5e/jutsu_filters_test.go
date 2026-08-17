package main

import (
	"reflect"
	"testing"
)

func TestCastingActionBucket(t *testing.T) {
	cases := map[string]string{
		"1 Action":           "Action",
		"1 Action.":          "Action",
		"1Action":            "Action", // no space before "Action" — no \b, Contains still finds it
		"Full Turn Action":   "Action",
		"1 Full Turn Action": "Action",
		"1 Bonus Action":     "Bonus Action",
		"1 Bonus action":     "Bonus Action",
		"1 Bonus Action.":    "Bonus Action",
		"1 Reaction, which you take when you would take damage.": "Reaction",
		"Special":    "Special",
		"1 Minute":   "Special", // a non-turn casting time, names none of the 3 real action types
		"10 Minutes": "Special",
		"1 Hour":     "Special",
	}
	for raw, want := range cases {
		if got := castingActionBucket(raw); got != want {
			t.Errorf("castingActionBucket(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestDurationBucket(t *testing.T) {
	cases := map[string]string{
		"Instant":                         "Instant",
		"Instant.":                        "Instant",
		"instant":                         "Instant",
		"Instantaneous":                   "Instant",
		"1 Round":                         "1 Round",
		"1 round.":                        "1 Round",
		"1 Minute":                        "1 Minute",
		"1 minute":                        "1 Minute",
		"10 Minutes":                      "10 Minutes",
		"Concentration, up to 1 minute":   "1 Minute",
		"Concentration, Up to 1 minute":   "1 Minute",
		"Concentration, up to 1 minute.":  "1 Minute",
		"Concentration, up to 1 Minute":   "1 Minute",
		"Concentration, up to 1 minutes":  "1 Minute", // real plural typo in the data
		"Concentration, up to 10 minutes": "10 Minutes",
		"Concentration, up to 10 Minutes": "10 Minutes",
		"Concentration, up to 1 hour":     "1 Hour",
		"Concentration, Special":          "Special",
		"1 Hour":                          "1 Hour",
		"8 Hours":                         "8 Hours",
		"24 Hours":                        "24 Hours",
		"1 Day":                           "1 Day",
		"10 Days":                         "10 Days",
		"1 Year":                          "1 Year",
		"Permanent":                       "Permanent",
		"Until Dispelled":                 "Until Dispelled",
		"Until Short Rest":                "Until Short Rest",
		"Special":                         "Special",
		"Up to 10 Minute":                 "10 Minutes",
	}
	for raw, want := range cases {
		if got := durationBucket(raw); got != want {
			t.Errorf("durationBucket(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestIsConcentrationDuration(t *testing.T) {
	cases := map[string]bool{
		"Concentration, up to 1 minute": true,
		"concentration, up to 1 minute": true,
		"Concentration, Special":        true,
		"  Concentration, up to 1 hour": true,
		"Instant":                       false,
		"Instantaneous":                 false,
		"1 Minute":                      false,
		"Permanent":                     false,
		"Special":                       false,
		"":                              false,
	}
	for raw, want := range cases {
		if got := isConcentrationDuration(raw); got != want {
			t.Errorf("isConcentrationDuration(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestParseJutsuRange(t *testing.T) {
	cases := []struct {
		raw     string
		feet    int
		numeric bool
		special string
	}{
		{"Self", 0, true, ""},
		{"self", 0, true, ""},
		{"Self (30-foot radius sphere)", 0, true, ""}, // AoE size in parens is NOT the jutsu's own range
		{"Self (120-foot line)", 0, true, ""},
		{"Touch", 0, true, ""},
		{"Touch (60 feet)", 0, true, ""},
		{"60 feet", 60, true, ""},
		{"60 Feet", 60, true, ""},
		{"60ft", 60, true, ""},
		{"10ft Cube", 10, true, ""},
		{"120 Feet (30-foot-cube)", 120, true, ""}, // primary range, not the parenthetical AoE
		{"30-feet", 30, true, ""},
		{"1 Mile", feetPerMile, true, ""},
		{"10 Miles", 10 * feetPerMile, true, ""},
		{"1-Mile", feetPerMile, true, ""},
		{"Weapons Range", 0, false, "Weapon Range"},
		{"Weapon's Range", 0, false, "Weapon Range"},
		{"Weapon’s Range", 0, false, "Weapon Range"},
		{"Double Weapon Range", 0, false, "Weapon Range"},
		{"Movement Speed", 0, false, "Movement Speed"},
		{"Full Movement", 0, false, "Movement Speed"},
		{"Special", 0, false, "Special"},
		{"Special (Cone)", 0, false, "Special"},
		{"X-Foot Cone", 0, false, "Special"}, // unparseable garbage falls to the catch-all, doesn't crash
	}
	for _, c := range cases {
		got := parseJutsuRange(c.raw)
		if got.Feet != c.feet || got.Numeric != c.numeric || got.Special != c.special {
			t.Errorf("parseJutsuRange(%q) = %+v, want {Feet:%d Numeric:%v Special:%q}", c.raw, got, c.feet, c.numeric, c.special)
		}
	}
}

func TestComponentCodes(t *testing.T) {
	cases := map[string][]string{
		"HS, CM":                             {"HS", "CM"},
		"HS, CM, CS":                         {"HS", "CM", "CS"},
		"CM, M":                              {"CM", "M"},
		"M":                                  {"M"},
		"W (Melee Bludgeoning), M":           {"W", "M"},
		"W(any), M":                          {"W", "M"},
		"HS, CM, NT (Poison Kit, 1 Charges)": {"HS", "CM", "NT"}, // comma INSIDE parens must not split the token
		"M, W (Any Flail).":                  {"M", "W"},         // trailing period
		"HS, CM, CS, NT (Empty Scroll)":      {"HS", "CM", "CS", "NT"},
	}
	for raw, want := range cases {
		got := componentCodes(raw)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("componentCodes(%q) = %v, want %v", raw, got, want)
		}
	}
}
