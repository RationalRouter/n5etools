package features

import "testing"

func TestResolveProficiencyGrants(t *testing.T) {
	granted := []GrantedFeatureRow{
		{Slug: "clan/hoshigaki/feature/brute-strength"},                                          // grants Intimidation
		{Slug: "class/hunter-nin/group/hunters-creeds/blade-warden/feature/wardens-proficiency"}, // grants Athletics, Intimidation
		{Slug: "class/some-feature/with-no-grant"},                                               // must be ignored, not panic
	}
	got := ResolveProficiencyGrants(granted)
	// Intimidation legitimately appears twice — two different sources each
	// grant it, same as two character_proficiencies rows for the same skill
	// from different sources would (SetClan/SetClass never dedupe either).
	wantCounts := map[ProficiencyGrant]int{
		{GrantSkill, "Intimidation"}: 2,
		{GrantSkill, "Athletics"}:    1,
	}
	gotCounts := map[ProficiencyGrant]int{}
	for _, g := range got {
		gotCounts[g]++
	}
	if len(got) != 3 {
		t.Fatalf("got %d grants, want 3: %+v", len(got), got)
	}
	for grant, want := range wantCounts {
		if gotCounts[grant] != want {
			t.Errorf("count of %+v = %d, want %d", grant, gotCounts[grant], want)
		}
	}
}

func TestResolveProficiencyGrantsSavingThrow(t *testing.T) {
	got := ResolveProficiencyGrants([]GrantedFeatureRow{{Slug: "clan/uzumaki/feature/inhuman-lifeforce"}})
	if len(got) != 1 || got[0] != (ProficiencyGrant{GrantSavingThrow, "con"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveACSwapAbility(t *testing.T) {
	tests := []struct {
		name        string
		slug        string
		armorCat    string
		wantAbility string
	}{
		{"Hoshigaki always applies", "clan/hoshigaki/feature/shark-skinned-predator", "", "con"},
		{"Hoshigaki applies even in heavy armor", "clan/hoshigaki/feature/shark-skinned-predator", "heavy", "con"},
		{"Medical-Nin always applies", "class/medical-nin/group/tenets-of-medicine/combat-medic/feature/competent-combatant", "", "wis"},
		{"Konjiki applies in light armor", "clan/konjiki/feature/blood-of-the-earth", "light", "int"},
		{"Konjiki applies in medium armor", "clan/konjiki/feature/blood-of-the-earth", "medium", "int"},
		{"Konjiki does not apply unarmored", "clan/konjiki/feature/blood-of-the-earth", "", ""},
		{"Konjiki does not apply in heavy armor", "clan/konjiki/feature/blood-of-the-earth", "heavy", ""},
		{"no grant at all", "class/unrelated/feature", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveACSwapAbility([]GrantedFeatureRow{{Slug: tt.slug}}, tt.armorCat)
			if got != tt.wantAbility {
				t.Errorf("got %q, want %q", got, tt.wantAbility)
			}
		})
	}
}

func TestResolveSpeedBonus(t *testing.T) {
	namikaze := []GrantedFeatureRow{{Slug: "clan/namikaze/feature/supernatural-speed"}}
	if got := ResolveSpeedBonus(namikaze, 1, ""); got != 5 {
		t.Errorf("level 1: got %d, want 5", got)
	}
	if got := ResolveSpeedBonus(namikaze, 10, ""); got != 5 {
		t.Errorf("level 10 (below next tier): got %d, want 5", got)
	}
	if got := ResolveSpeedBonus(namikaze, 11, ""); got != 10 {
		t.Errorf("level 11: got %d, want 10 (not additive with tier 1's 5)", got)
	}
	if got := ResolveSpeedBonus(namikaze, 20, ""); got != 15 {
		t.Errorf("level 20: got %d, want 15", got)
	}

	taijutsu := []GrantedFeatureRow{{Slug: "class/taijutsu-specialist/feature/enhanced-movement"}}
	if got := ResolveSpeedBonus(taijutsu, 6, ""); got != 15 {
		t.Errorf("taijutsu level 6, no armor: got %d, want 15", got)
	}
	if got := ResolveSpeedBonus(taijutsu, 6, "heavy"); got != 0 {
		t.Errorf("taijutsu level 6, heavy armor: got %d, want 0", got)
	}
	if got := ResolveSpeedBonus(taijutsu, 1, ""); got != 0 {
		t.Errorf("taijutsu level 1 (below first tier): got %d, want 0", got)
	}

	// Ironclad's own feature text overrides Enhanced Movement's heavy-armor
	// gate: "While wearing heavy armor, you still gain the benefits of
	// Enhanced Movement." The exception is keyed off the presence of any
	// granted Ironclad-subclass feature, not a separate parameter.
	ironclad := []GrantedFeatureRow{
		{Slug: "class/taijutsu-specialist/feature/enhanced-movement"},
		{Slug: "class/taijutsu-specialist/group/taijutsu-style/ironclad/feature/ironclad"},
	}
	if got := ResolveSpeedBonus(ironclad, 6, "heavy"); got != 15 {
		t.Errorf("ironclad level 6, heavy armor: got %d, want 15 (Ironclad keeps Enhanced Movement in heavy armor)", got)
	}
	if got := ResolveSpeedBonus(ironclad, 6, ""); got != 15 {
		t.Errorf("ironclad level 6, no armor: got %d, want 15", got)
	}

	// A different Taijutsu Style subclass (no Ironclad feature granted)
	// keeps the normal heavy-armor gate.
	nonIronclad := []GrantedFeatureRow{
		{Slug: "class/taijutsu-specialist/feature/enhanced-movement"},
		{Slug: "class/taijutsu-specialist/group/taijutsu-style/stancer/feature/stancer"},
	}
	if got := ResolveSpeedBonus(nonIronclad, 6, "heavy"); got != 0 {
		t.Errorf("non-ironclad level 6, heavy armor: got %d, want 0", got)
	}

	// Two different sources stack.
	both := []GrantedFeatureRow{
		{Slug: "clan/namikaze/feature/supernatural-speed"},
		{Slug: "class/taijutsu-specialist/feature/enhanced-movement"},
	}
	if got := ResolveSpeedBonus(both, 11, ""); got != 30 { // namikaze's level-11 tier (10) + taijutsu's level-9 tier (20)
		t.Errorf("stacked sources at level 11: got %d, want 30", got)
	}
}
