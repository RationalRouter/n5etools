package main

import "testing"

func TestMedicalDoctrineCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 1}, {12, 1}, {13, 2}, {20, 2},
	}
	for _, c := range cases {
		if got := medicalDoctrineCap(c.level); got != c.want {
			t.Errorf("medicalDoctrineCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestRejuvenatingRestDice(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{0, ""}, {1, "1d6"}, {6, "1d6"}, {7, "2d6"}, {10, "2d6"}, {11, "3d6"}, {16, "3d6"}, {17, "4d6"}, {20, "4d6"},
	}
	for _, c := range cases {
		if got := rejuvenatingRestDice(c.level); got != c.want {
			t.Errorf("rejuvenatingRestDice(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestSplitMedicalDoctrinePicks(t *testing.T) {
	catalog := []medicalDoctrineOption{
		{Slug: "class/medical-nin/feature/long-life-short-death", Name: "Long Life, Short Death", Description: "d1"},
		{Slug: "class/medical-nin/feature/never-on-the-front-lines", Name: "Never on the Front Lines", Description: "d2"},
		{Slug: "class/medical-nin/feature/not-allowed-to-die", Name: "Not Allowed to Die", Description: "d3"},
		{Slug: "class/medical-nin/feature/until-their-heart-stops", Name: "Until Their Heart Stops", Description: "d4"},
	}
	picked := map[string]bool{"class/medical-nin/feature/not-allowed-to-die": true}

	known, available := splitMedicalDoctrinePicks(catalog, picked)
	if len(known) != 1 || known[0].Slug != "class/medical-nin/feature/not-allowed-to-die" {
		t.Fatalf("known = %+v, want exactly Not Allowed to Die", known)
	}
	if len(available) != 3 {
		t.Fatalf("available = %+v, want 3 remaining doctrines", available)
	}
	for _, o := range available {
		if picked[o.Slug] {
			t.Errorf("available still contains already-picked doctrine %s", o.Slug)
		}
	}
}
