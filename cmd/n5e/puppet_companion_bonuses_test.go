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

// TestPuppetToolASIAbilityBonus mirrors TestPuppetElevatedDesignAbilityBonus
// but for Puppet Tool's own base-class ASI bonus — Amount 1 per slot
// instead of Elevated Design's 2, so the "same ability picked on both
// slots of one breakpoint" case must sum to +2 (not +4), reproducing the
// book's own "+2 to one ability score, or +1 to two ability scores" branch.
// Also confirms two different breakpoints' own picks accumulate onto the
// same ability rather than only the most recent breakpoint winning.
func TestPuppetToolASIAbilityBonus(t *testing.T) {
	if got := puppetToolASIAbilityBonus(nil); len(got) != 0 {
		t.Errorf("no resolved choices: got %v, want empty", got)
	}

	// 4th-level breakpoint, same ability picked on both of its slots ("+2
	// to one ability score").
	resolved := map[features.ChoiceKey]string{
		{FeatureSlug: puppetToolFeatureSlug, ChoiceIndex: 4 * 2}:   "str",
		{FeatureSlug: puppetToolFeatureSlug, ChoiceIndex: 4*2 + 1}: "str",
	}
	got := puppetToolASIAbilityBonus(resolved)
	want := map[string]int{"str": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("same ability picked twice at one breakpoint: got %v, want %v", got, want)
	}

	// 4th-level breakpoint split across two different abilities ("+1 to
	// two ability scores"), plus an 8th-level breakpoint stacking more of
	// the same "str" bonus from a different breakpoint.
	resolved = map[features.ChoiceKey]string{
		{FeatureSlug: puppetToolFeatureSlug, ChoiceIndex: 4 * 2}:   "str",
		{FeatureSlug: puppetToolFeatureSlug, ChoiceIndex: 4*2 + 1}: "dex",
		{FeatureSlug: puppetToolFeatureSlug, ChoiceIndex: 8 * 2}:   "str",
	}
	got = puppetToolASIAbilityBonus(resolved)
	want = map[string]int{"str": 2, "dex": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("split pick plus a second breakpoint: got %v, want %v", got, want)
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
// cap array shape (index = puppetUpgradeTierRanks value). Both are class
// features, not feats, so hasFeat is passed nil throughout — neither has a
// FeatSlug set, and a nil map is a valid "nothing taken" hasFeat.
func TestPuppetUpgradeFeatureBonusTierCaps(t *testing.T) {
	if got := puppetUpgradeFeatureBonusTierCaps("Purple", 16, nil); got != ([puppetUpgradeTierCount + 1]int{}) {
		t.Errorf("Purple below level 17: got %v, want all zero", got)
	}
	if got := puppetUpgradeFeatureBonusTierCaps("Red", 20, nil); got != ([puppetUpgradeTierCount + 1]int{}) {
		t.Errorf("Red (no bonus grant at all): got %v, want all zero", got)
	}

	purple := puppetUpgradeFeatureBonusTierCaps("Purple", 17, nil)
	var wantPurple [puppetUpgradeTierCount + 1]int
	wantPurple[puppetUpgradeTierRanks["Silver Tier"]] = 2
	if purple != wantPurple {
		t.Errorf("Purple level 17: got %v, want %v (2 Silver Tier slots)", purple, wantPurple)
	}

	black := puppetUpgradeFeatureBonusTierCaps("Black", 17, nil)
	var wantBlack [puppetUpgradeTierCount + 1]int
	wantBlack[puppetUpgradeTierRanks["Bronze Tier"]] = 1
	wantBlack[puppetUpgradeTierRanks["Silver Tier"]] = 1
	wantBlack[puppetUpgradeTierRanks["Gold Tier"]] = 1
	if black != wantBlack {
		t.Errorf("Black level 17: got %v, want %v (1 Bronze/Silver/Gold each)", black, wantBlack)
	}
}

// TestPuppetUpgradeFeatureBonusTierCapsFeatDriven covers the Puppet Master
// Archetype feat chain and Tools to Carry on a Legacy: each is gated on
// hasFeat rather than subclass color (so a character of ANY subclass, or
// none at all, still qualifies), and — unlike the two class features above
// — is NOT granted just from satisfying level/subclass; the specific feat
// must actually be present in hasFeat.
func TestPuppetUpgradeFeatureBonusTierCapsFeatDriven(t *testing.T) {
	// Right subclass/level for Elevated Design, but no feats taken at all:
	// none of the feat-driven entries should fire, only the class feature.
	noFeats := puppetUpgradeFeatureBonusTierCaps("Black", 20, nil)
	var wantNoFeats [puppetUpgradeTierCount + 1]int
	wantNoFeats[puppetUpgradeTierRanks["Bronze Tier"]] = 1
	wantNoFeats[puppetUpgradeTierRanks["Silver Tier"]] = 1
	wantNoFeats[puppetUpgradeTierRanks["Gold Tier"]] = 1
	if noFeats != wantNoFeats {
		t.Errorf("Black level 20, no feats: got %v, want %v (Elevated Design only)", noFeats, wantNoFeats)
	}

	// Puppeteer Training: fires at level 0 with no subclass at all, purely
	// off hasFeat — the archetype chain's whole point is working without
	// real Puppet Master class levels.
	training := puppetUpgradeFeatureBonusTierCaps("", 0, map[string]bool{"feat/class/puppeteer-training": true})
	var wantTraining [puppetUpgradeTierCount + 1]int
	wantTraining[puppetUpgradeTierRanks["Wood Tier"]] = 2
	if training != wantTraining {
		t.Errorf("Puppeteer Training only: got %v, want %v (2 Wood Tier slots)", training, wantTraining)
	}

	// Tools to Carry on a Legacy below every one of its own level
	// thresholds: only the unconditional Wood-tier grant fires.
	toolsLow := puppetUpgradeFeatureBonusTierCaps("Red", 5, map[string]bool{"feat/class/tools-to-carry-on-a-legacy": true})
	var wantToolsLow [puppetUpgradeTierCount + 1]int
	wantToolsLow[puppetUpgradeTierRanks["Wood Tier"]] = 1
	if toolsLow != wantToolsLow {
		t.Errorf("Tools to Carry on a Legacy at PM level 5: got %v, want %v (Wood Tier only)", toolsLow, wantToolsLow)
	}

	// Tools to Carry on a Legacy at 19th level of Puppet Master: every one
	// of its 4 thresholds has been crossed.
	toolsHigh := puppetUpgradeFeatureBonusTierCaps("Red", 19, map[string]bool{"feat/class/tools-to-carry-on-a-legacy": true})
	var wantToolsHigh [puppetUpgradeTierCount + 1]int
	wantToolsHigh[puppetUpgradeTierRanks["Wood Tier"]] = 1
	wantToolsHigh[puppetUpgradeTierRanks["Bronze Tier"]] = 1
	wantToolsHigh[puppetUpgradeTierRanks["Silver Tier"]] = 1
	wantToolsHigh[puppetUpgradeTierRanks["Platinum Tier"]] = 1
	if toolsHigh != wantToolsHigh {
		t.Errorf("Tools to Carry on a Legacy at PM level 19: got %v, want %v (all 4 thresholds)", toolsHigh, wantToolsHigh)
	}

	// Meeting level 19 but never having taken the feat at all: nothing
	// fires. This is the regression case the FeatSlug check exists for —
	// without it, any Puppet Master at level 19+ would get these slots for
	// free regardless of their actual feats.
	noFeatHighLevel := puppetUpgradeFeatureBonusTierCaps("Red", 19, nil)
	if noFeatHighLevel != ([puppetUpgradeTierCount + 1]int{}) {
		t.Errorf("PM level 19, feat never taken: got %v, want all zero", noFeatHighLevel)
	}
}
