package features

import "testing"

func TestResolveChoiceSlotsGating(t *testing.T) {
	nara := []GrantedFeatureRow{{Slug: "clan/nara/feature/masterwork-skill"}}

	// Below the first breakpoint: no slots at all.
	if got := ResolveChoiceSlots(nara, nil, 6); len(got) != 0 {
		t.Fatalf("level 6: got %d slots, want 0: %+v", len(got), got)
	}
	// At 7th level: the two 7th-level slots only.
	got := ResolveChoiceSlots(nara, nil, 7)
	if len(got) != 2 {
		t.Fatalf("level 7: got %d slots, want 2: %+v", len(got), got)
	}
	for _, s := range got {
		if s.FeatureSlug != "clan/nara/feature/masterwork-skill" || s.ChoiceIndex >= 2 {
			t.Errorf("unexpected slot at level 7: %+v", s)
		}
	}
	// At 18th level: all four.
	if got := ResolveChoiceSlots(nara, nil, 18); len(got) != 4 {
		t.Fatalf("level 18: got %d slots, want 4: %+v", len(got), got)
	}

	// The feature must actually be granted, not just level-eligible.
	if got := ResolveChoiceSlots(nil, nil, 18); len(got) != 0 {
		t.Fatalf("no granted features: got %d slots, want 0: %+v", len(got), got)
	}
}

func TestResolveChoiceSlotsUsesOwningClassLevel(t *testing.T) {
	// Perfect Mind is a subclass feature under intelligence-operative,
	// gated at 17th level of THAT class specifically — a high total
	// (multiclass) level must not unlock it early.
	granted := []GrantedFeatureRow{{
		Slug: "class/intelligence-operative/group/master-strategies/interrogationist/feature/perfect-mind",
	}}
	classLevels := map[string]int{"class/intelligence-operative": 10}
	if got := ResolveChoiceSlots(granted, classLevels, 20); len(got) != 0 {
		t.Fatalf("class level 10 (total 20): got %d slots, want 0: %+v", len(got), got)
	}
	classLevels["class/intelligence-operative"] = 17
	got := ResolveChoiceSlots(granted, classLevels, 5) // low total level from other classes
	if len(got) != 1 {
		t.Fatalf("class level 17: got %d slots, want 1: %+v", len(got), got)
	}
	if got[0].Kind != ChoiceSavingThrowProficiency || len(got[0].Options) != 2 {
		t.Errorf("unexpected slot shape: %+v", got[0])
	}
}

func TestPendingAndResolveChoiceGrants(t *testing.T) {
	slots := []ChoiceSlot{
		{FeatureSlug: "clan/non-clan/feature/self-taught-skills", ChoiceIndex: 0, Kind: ChoiceSkillProficiency},
		{FeatureSlug: "class/scout-nin/feature/canny-1-st-level", ChoiceIndex: 0, Kind: ChoiceSkillProficiency},
	}
	resolved := map[ChoiceKey]string{
		{FeatureSlug: "clan/non-clan/feature/self-taught-skills", ChoiceIndex: 0}: "Stealth",
	}

	pending := PendingChoiceSlots(slots, resolved)
	if len(pending) != 1 || pending[0].FeatureSlug != "class/scout-nin/feature/canny-1-st-level" {
		t.Fatalf("got pending %+v, want only the unresolved Canny slot", pending)
	}

	grants := ResolveChoiceGrants(slots, resolved)
	if len(grants) != 1 || grants[0] != (ProficiencyGrant{GrantSkill, "Stealth"}) {
		t.Fatalf("got grants %+v, want just the resolved Self-Taught Skills pick", grants)
	}
}

func TestResolveChoiceSlotsElevatedDesign(t *testing.T) {
	const featureSlug = "class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design"
	granted := []GrantedFeatureRow{{Slug: featureSlug}}
	classLevels := map[string]int{"class/puppet-master": 16}

	if got := ResolveChoiceSlots(granted, classLevels, 16); len(got) != 0 {
		t.Fatalf("level 16: got %d slots, want 0 (gated at 17): %+v", len(got), got)
	}

	classLevels["class/puppet-master"] = 17
	got := ResolveChoiceSlots(granted, classLevels, 17)
	if len(got) != 2 {
		t.Fatalf("level 17: got %d slots, want 2: %+v", len(got), got)
	}
	for _, s := range got {
		if s.FeatureSlug != featureSlug || s.Kind != ChoiceCompanionAbilityScoreIncrease || s.Amount != 2 || len(s.Options) != 6 {
			t.Errorf("unexpected slot shape: %+v", s)
		}
	}
	if got[0].ChoiceIndex == got[1].ChoiceIndex {
		t.Fatalf("both slots share ChoiceIndex %d, want two independent slots", got[0].ChoiceIndex)
	}
}

func TestClassSlugOf(t *testing.T) {
	tests := []struct{ slug, want string }{
		{"class/hunter-nin/feature/expertise", "class/hunter-nin"},
		{"class/intelligence-operative/group/master-strategies/interrogationist/feature/perfect-mind", "class/intelligence-operative"},
		{"clan/nara/feature/masterwork-skill", ""},
	}
	for _, tt := range tests {
		if got := classSlugOf(tt.slug); got != tt.want {
			t.Errorf("classSlugOf(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}
