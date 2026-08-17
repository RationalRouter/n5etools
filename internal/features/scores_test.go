package features

import "testing"

func TestResolveAbilityScoreGrants(t *testing.T) {
	granted := []GrantedFeatureRow{
		{Slug: "class/weapon-specialist/group/weapon-forms/samurai-form/feature/master-of-focus"},
		{Slug: "class/weapon-specialist/feature/unrelated"},
	}
	got := ResolveAbilityScoreGrants(granted)
	want := []AbilityScoreGrant{{Ability: "str", Amount: 2, RaisesMax: true}, {Ability: "con", Amount: 2, RaisesMax: true}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("grant %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if extra := ResolveAbilityScoreGrants(nil); len(extra) != 0 {
		t.Errorf("no granted features should mean no grants, got %+v", extra)
	}
}

func TestResolveConditionalAbilityScoreGrants(t *testing.T) {
	granted := []GrantedFeatureRow{
		{Slug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/master-of-the-purple-technique"},
	}

	if got := ResolveConditionalAbilityScoreGrants(granted, false); len(got) != 0 {
		t.Errorf("no armor worn should grant nothing, got %+v", got)
	}

	got := ResolveConditionalAbilityScoreGrants(granted, true)
	want := AbilityScoreGrant{Ability: "int", Amount: 2}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want one %+v (deliberately RaisesMax:false — see the grant's own comment)", got, want)
	}

	// A character without the feature at all gets nothing even while
	// "wearing armor" is true — the condition only gates characters who
	// already have the feature.
	if got := ResolveConditionalAbilityScoreGrants(nil, true); len(got) != 0 {
		t.Errorf("no granted feature should mean no grant regardless of armor state, got %+v", got)
	}
}

func TestResolveSaveMasteryRanks(t *testing.T) {
	granted := []GrantedFeatureRow{
		{Slug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design"},
	}
	got := ResolveSaveMasteryRanks(granted)
	if got["str"] != 1 {
		t.Errorf("str mastery rank = %d, want 1 (one source = Rank 1)", got["str"])
	}
	if got["dex"] != 0 {
		t.Errorf("dex mastery rank = %d, want 0", got["dex"])
	}
}

func TestResolveChoiceAbilityScoreGrants(t *testing.T) {
	slot := ChoiceSlot{
		FeatureSlug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design",
		ChoiceIndex: 0, Kind: ChoiceAbilityScoreIncrease,
		Options: []string{"str", "dex"}, Amount: 2, RaisesMax: true,
	}

	// Unresolved: contributes nothing.
	if got := ResolveChoiceAbilityScoreGrants([]ChoiceSlot{slot}, map[ChoiceKey]string{}); len(got) != 0 {
		t.Errorf("unresolved slot should grant nothing, got %+v", got)
	}

	// Resolved to a valid option — RaisesMax carries through from the slot
	// into the resulting grant.
	resolved := map[ChoiceKey]string{{slot.FeatureSlug, 0}: "dex"}
	got := ResolveChoiceAbilityScoreGrants([]ChoiceSlot{slot}, resolved)
	if len(got) != 1 || got[0] != (AbilityScoreGrant{Ability: "dex", Amount: 2, RaisesMax: true}) {
		t.Errorf("got %+v, want one dex +2 grant with RaisesMax", got)
	}

	// A stored value outside the slot's own options contributes nothing
	// rather than raising an arbitrary score.
	resolved[ChoiceKey{slot.FeatureSlug, 0}] = "cha"
	if got := ResolveChoiceAbilityScoreGrants([]ChoiceSlot{slot}, resolved); len(got) != 0 {
		t.Errorf("out-of-options value should grant nothing, got %+v", got)
	}

	// A non-ability slot with a resolved value is ignored by this resolver.
	skillSlot := ChoiceSlot{FeatureSlug: "clan/nara/feature/masterwork-skill", ChoiceIndex: 0, Kind: ChoiceSkillProficiency}
	skillResolved := map[ChoiceKey]string{{skillSlot.FeatureSlug, 0}: "Stealth"}
	if got := ResolveChoiceAbilityScoreGrants([]ChoiceSlot{skillSlot}, skillResolved); len(got) != 0 {
		t.Errorf("skill slot should grant no ability scores, got %+v", got)
	}
}
