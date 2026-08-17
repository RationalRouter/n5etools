package main

import (
	"reflect"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// TestPuppetElevatedDesignAbilityBonus locks in Elevated Design's two
// independent ability picks folding into a single per-ability +2 map (both
// picks land on the same ability => +4, matching how a player who picks the
// same ability twice should stack, not overwrite).
func TestPuppetElevatedDesignAbilityBonus(t *testing.T) {
	if got := puppetElevatedDesignAbilityBonus(nil); len(got) != 0 {
		t.Errorf("no resolved choices: got %v, want empty", got)
	}

	resolved := map[features.ChoiceKey]string{
		{FeatureSlug: puppetElevatedDesignFeatureSlug, ChoiceIndex: 0}: "int",
		{FeatureSlug: puppetElevatedDesignFeatureSlug, ChoiceIndex: 1}: "wis",
	}
	got := puppetElevatedDesignAbilityBonus(resolved)
	want := map[string]int{"int": 2, "wis": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("two distinct abilities: got %v, want %v", got, want)
	}

	resolved = map[features.ChoiceKey]string{
		{FeatureSlug: puppetElevatedDesignFeatureSlug, ChoiceIndex: 0}: "str",
		{FeatureSlug: puppetElevatedDesignFeatureSlug, ChoiceIndex: 1}: "str",
	}
	got = puppetElevatedDesignAbilityBonus(resolved)
	want = map[string]int{"str": 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("same ability picked twice: got %v, want %v (should stack)", got, want)
	}

	// Only ChoiceIndex 0 resolved: partial pick still returns what's there.
	resolved = map[features.ChoiceKey]string{
		{FeatureSlug: puppetElevatedDesignFeatureSlug, ChoiceIndex: 0}: "dex",
	}
	got = puppetElevatedDesignAbilityBonus(resolved)
	want = map[string]int{"dex": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("only first slot resolved: got %v, want %v", got, want)
	}
}

// TestPuppetSymphonyExpansionGrant covers all three branches: not the
// Expansion branch at all, Expansion with room left (grants the entry),
// and Expansion already taken twice (falls back to the Bronze-or-lower
// Upgrade slot instead of a third copy of the named entry).
func TestPuppetSymphonyExpansionGrant(t *testing.T) {
	notExpansion := map[features.ChoiceKey]string{
		{FeatureSlug: puppetSymphonyOfPuppetryFeatureSlug, ChoiceIndex: 0}: "enhancement",
	}
	if slug, bonus := puppetSymphonyExpansionGrant(notExpansion, nil); slug != "" || bonus {
		t.Errorf("enhancement branch: got (%q, %v), want (\"\", false)", slug, bonus)
	}

	expansion := map[features.ChoiceKey]string{
		{FeatureSlug: puppetSymphonyOfPuppetryFeatureSlug, ChoiceIndex: 0}: "expansion",
	}
	slug, bonus := puppetSymphonyExpansionGrant(expansion, map[string]int{})
	if slug != puppetExpandedPuppetryEntrySlug || bonus {
		t.Errorf("expansion, none picked yet: got (%q, %v), want (%q, false)", slug, bonus, puppetExpandedPuppetryEntrySlug)
	}

	slug, bonus = puppetSymphonyExpansionGrant(expansion, map[string]int{puppetExpandedPuppetryEntrySlug: 1})
	if slug != puppetExpandedPuppetryEntrySlug || bonus {
		t.Errorf("expansion, picked once already: got (%q, %v), want (%q, false)", slug, bonus, puppetExpandedPuppetryEntrySlug)
	}

	slug, bonus = puppetSymphonyExpansionGrant(expansion, map[string]int{puppetExpandedPuppetryEntrySlug: 2})
	if slug != "" || !bonus {
		t.Errorf("expansion, picked twice already: got (%q, %v), want (\"\", true)", slug, bonus)
	}
}

// TestPuppetSymphonyEnhancementCompanionIDs confirms the "two earliest
// kind='puppet' companions" proxy: lowest IDs win, non-puppet companions are
// skipped, and the result is capped at 2 regardless of how many puppets
// exist.
func TestPuppetSymphonyEnhancementCompanionIDs(t *testing.T) {
	if got := puppetSymphonyEnhancementCompanionIDs(nil); len(got) != 0 {
		t.Errorf("no companions: got %v, want empty", got)
	}

	companions := []charstore.Companion{
		{ID: 5, Kind: "summon"},
		{ID: 2, Kind: "puppet"},
		{ID: 9, Kind: "puppet"},
		{ID: 3, Kind: "puppet"},
	}
	got := puppetSymphonyEnhancementCompanionIDs(companions)
	want := []int64{2, 9}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mixed companions in id order: got %v, want %v (first two puppets encountered, summon skipped)", got, want)
	}

	single := []charstore.Companion{{ID: 7, Kind: "puppet"}}
	if got := puppetSymphonyEnhancementCompanionIDs(single); !reflect.DeepEqual(got, []int64{7}) {
		t.Errorf("single puppet: got %v, want [7]", got)
	}
}

// TestPuppetUpgradeFeatureBonusTierCaps confirms the two curated bonus-slot
// grants (Nearly Perfected Architecture, Elevated Design) are gated on both
// subclass color AND Puppet Master level, and produce the right per-tier
// cap array shape (index = puppetUpgradeTierRanks value).
func TestPuppetUpgradeFeatureBonusTierCaps(t *testing.T) {
	if got := puppetUpgradeFeatureBonusTierCaps("Purple", 16); got != ([puppetUpgradeTierCount + 1]int{}) {
		t.Errorf("Purple below level 17: got %v, want all zero", got)
	}
	if got := puppetUpgradeFeatureBonusTierCaps("Red", 20); got != ([puppetUpgradeTierCount + 1]int{}) {
		t.Errorf("Red (no bonus grant at all): got %v, want all zero", got)
	}

	purple := puppetUpgradeFeatureBonusTierCaps("Purple", 17)
	var wantPurple [puppetUpgradeTierCount + 1]int
	wantPurple[puppetUpgradeTierRanks["Silver Tier"]] = 2
	if purple != wantPurple {
		t.Errorf("Purple level 17: got %v, want %v (2 Silver Tier slots)", purple, wantPurple)
	}

	black := puppetUpgradeFeatureBonusTierCaps("Black", 17)
	var wantBlack [puppetUpgradeTierCount + 1]int
	wantBlack[puppetUpgradeTierRanks["Bronze Tier"]] = 1
	wantBlack[puppetUpgradeTierRanks["Silver Tier"]] = 1
	wantBlack[puppetUpgradeTierRanks["Gold Tier"]] = 1
	if black != wantBlack {
		t.Errorf("Black level 17: got %v, want %v (1 Bronze/Silver/Gold each)", black, wantBlack)
	}
}
