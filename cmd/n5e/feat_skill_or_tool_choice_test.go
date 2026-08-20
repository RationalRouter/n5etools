package main

import "testing"

// TestParseFeatSkillOrToolChoice pins the four real "Kit or Skill (Pick
// one)" feats in the live corpus (verified against dist/rules.db) plus
// feat/crafter's exclusion.
func TestParseFeatSkillOrToolChoice(t *testing.T) {
	cases := []struct {
		name        string
		slug        string
		description string
		want        [2]featChoiceCandidate
		wantOK      bool
	}{
		{
			name:        "feat/burglar: Security Kit or Sleight of Hand",
			slug:        "feat/burglar",
			description: "You gain proficiency with a Security Kit or Sleight of Hand (Pick one). If you are already proficient, you instead gain Mastery.",
			want: [2]featChoiceCandidate{
				{Kind: "tool", Name: "Security Kit"},
				{Kind: "skill", Name: "Sleight of Hand"},
			},
			wantOK: true,
		},
		{
			name:        "feat/chef: Cooking Kit or Survival",
			slug:        "feat/chef",
			description: "You gain proficiency with a Cooking Kit or Survival (Pick one). If you are already proficient, you instead gain Mastery.",
			want: [2]featChoiceCandidate{
				{Kind: "tool", Name: "Cooking Kit"},
				{Kind: "skill", Name: "Survival"},
			},
			wantOK: true,
		},
		{
			name:        "feat/chemist: Alchemist kit or Investigation (lowercase 'kit' in the source text)",
			slug:        "feat/chemist",
			description: "You gain proficiency with the Alchemist kit or Investigation. (Pick One). If you are already Proficient, you instead gain Mastery.",
			want: [2]featChoiceCandidate{
				{Kind: "tool", Name: "Alchemist Kit"},
				{Kind: "skill", Name: "Investigation"},
			},
			wantOK: true,
		},
		{
			name:        "feat/investigator: Investigation or Forensics Kit",
			slug:        "feat/investigator",
			description: "You gain proficiency in Investigation or the Forensics Kit (Pick one). If you are already proficient, you instead gain Mastery.",
			want: [2]featChoiceCandidate{
				{Kind: "skill", Name: "Investigation"},
				{Kind: "tool", Name: "Forensics Kit"},
			},
			wantOK: true,
		},
		{
			name:        "feat/crafter excluded: Crafting-and-kit-choice compound clause",
			slug:        "feat/crafter",
			description: "You gain proficiency in Crafting and either Weaponsmiths or Armorsmith Kit (Pick one). If you are already proficient, you instead gain Mastery.",
			wantOK:      false,
		},
		{
			name:        "no compound clause at all",
			slug:        "feat/acrobat",
			description: "You gain proficiency in the Acrobatics skill. If you are already proficient, you instead gain Mastery.",
			wantOK:      false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseFeatSkillOrToolChoice(c.slug, c.description)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, c.wantOK, got)
			}
			if ok && got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestTitleCaseWords(t *testing.T) {
	cases := map[string]string{
		"Alchemist kit":   "Alchemist Kit",
		"security kit":    "Security Kit",
		"Sleight of Hand": "Sleight Of Hand",
		"":                "",
	}
	for in, want := range cases {
		if got := titleCaseWords(in); got != want {
			t.Errorf("titleCaseWords(%q) = %q, want %q", in, got, want)
		}
	}
}
