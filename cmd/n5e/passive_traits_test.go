package main

import "testing"

// TestComputePassiveTraitsBattleReadinessAdvantage pins Weapon Specialist's
// Battle Readiness (base class, 7th level) as the live demonstration of
// advantageGrants: "Starting at 7th level, you have fully learned how to
// instantly switch from a neutral stance to that of a combat one. You have
// advantage on Initiative Checks." (rules.db class_features, confirmed
// verbatim 2026-08-26).
func TestComputePassiveTraitsBattleReadinessAdvantage(t *testing.T) {
	battleReadiness := grantedFeatureRow{
		Slug: "class/weapon-specialist/feature/battle-readiness",
		Name: "Battle Readiness",
	}

	traits := computePassiveTraits([]grantedFeatureRow{battleReadiness}, 7)
	if len(traits.Advantages) != 1 {
		t.Fatalf("Advantages = %+v, want exactly 1 entry", traits.Advantages)
	}
	if traits.Advantages[0].RollType != "Initiative checks" {
		t.Errorf("Advantages[0].RollType = %q, want %q", traits.Advantages[0].RollType, "Initiative checks")
	}
	if len(traits.Advantages[0].Sources) != 1 || traits.Advantages[0].Sources[0] != "Battle Readiness" {
		t.Errorf("Advantages[0].Sources = %+v, want just Battle Readiness", traits.Advantages[0].Sources)
	}
	if len(traits.Disadvantages) != 0 {
		t.Errorf("Disadvantages = %+v, want none", traits.Disadvantages)
	}
}

// TestComputePassiveTraitsAdvantageNoGrant confirms a character with no
// registered advantageGrants slug gets an empty Advantages/Disadvantages —
// this mechanism must never fabricate an entry from an unrelated feature.
func TestComputePassiveTraitsAdvantageNoGrant(t *testing.T) {
	unrelated := grantedFeatureRow{
		Slug: "class/taijutsu-specialist/feature/perfect-mind",
		Name: "Perfect Mind",
	}
	traits := computePassiveTraits([]grantedFeatureRow{unrelated}, 10)
	if len(traits.Advantages) != 0 {
		t.Errorf("Advantages = %+v, want none", traits.Advantages)
	}
	if len(traits.Disadvantages) != 0 {
		t.Errorf("Disadvantages = %+v, want none", traits.Disadvantages)
	}
}

// withTestAdvantageGrant temporarily registers a synthetic advantageGrants
// entry under a fake slug for the duration of a test, restoring the real
// table afterward — the generic MinLevel-gating, Disadvantage-direction, and
// multi-source-merge behavior computePassiveTraits applies to advantageGrants
// has no second real registered feature to exercise it against yet (only
// Battle Readiness is wired so far; the other four dependent features are
// separate follow-up work), so this fixture exercises the mechanism itself
// directly rather than waiting on that follow-up work to land.
func withTestAdvantageGrant(t *testing.T, slug string, grant advantageGrant) {
	t.Helper()
	prev, existed := advantageGrants[slug]
	advantageGrants[slug] = append([]advantageGrant{}, grant)
	t.Cleanup(func() {
		if existed {
			advantageGrants[slug] = prev
		} else {
			delete(advantageGrants, slug)
		}
	})
}

// TestComputePassiveTraitsAdvantageMinLevelGate pins the MinLevel gate the
// same way TestComputePassiveTraitsEscalation pins Ashen Resilience's own
// MinLevel-gated clause: a grant below its MinLevel must not appear at all.
func TestComputePassiveTraitsAdvantageMinLevelGate(t *testing.T) {
	const slug = "test/synthetic/feature/gated-advantage"
	withTestAdvantageGrant(t, slug, advantageGrant{
		RollType:  "Perception checks",
		Direction: directionAdvantage,
		MinLevel:  10,
	})
	feature := grantedFeatureRow{Slug: slug, Name: "Gated Advantage"}

	below := computePassiveTraits([]grantedFeatureRow{feature}, 9)
	if len(below.Advantages) != 0 {
		t.Errorf("Advantages below MinLevel = %+v, want none", below.Advantages)
	}

	atLevel := computePassiveTraits([]grantedFeatureRow{feature}, 10)
	if len(atLevel.Advantages) != 1 || atLevel.Advantages[0].RollType != "Perception checks" {
		t.Errorf("Advantages at MinLevel = %+v, want Perception checks", atLevel.Advantages)
	}
}

// TestComputePassiveTraitsDisadvantageDirection confirms a Disadvantage
// grant lands in PassiveTraitSummary.Disadvantages, not Advantages — the
// shape Untouchable (disadvantage on reaction attacks made against the
// character) will need once it's wired up.
func TestComputePassiveTraitsDisadvantageDirection(t *testing.T) {
	const slug = "test/synthetic/feature/disadvantage-on-reactions"
	withTestAdvantageGrant(t, slug, advantageGrant{
		RollType:  "reaction attacks made against you",
		Direction: directionDisadvantage,
	})
	feature := grantedFeatureRow{Slug: slug, Name: "Synthetic Disadvantage Feature"}

	traits := computePassiveTraits([]grantedFeatureRow{feature}, 6)
	if len(traits.Advantages) != 0 {
		t.Errorf("Advantages = %+v, want none", traits.Advantages)
	}
	if len(traits.Disadvantages) != 1 || traits.Disadvantages[0].RollType != "reaction attacks made against you" {
		t.Fatalf("Disadvantages = %+v, want reaction attacks made against you", traits.Disadvantages)
	}
	if len(traits.Disadvantages[0].Sources) != 1 || traits.Disadvantages[0].Sources[0] != "Synthetic Disadvantage Feature" {
		t.Errorf("Disadvantages[0].Sources = %+v, want just the granting feature's name", traits.Disadvantages[0].Sources)
	}
}

// TestComputePassiveTraitsAdvantageMergesSources confirms two different
// features granting Advantage on the SAME roll type collapse into one
// entry whose Sources lists both — the same de-duplication shape
// Resistances/Immunities already use for two sources granting the same
// Target.
func TestComputePassiveTraitsAdvantageMergesSources(t *testing.T) {
	const slugA = "test/synthetic/feature/advantage-source-a"
	const slugB = "test/synthetic/feature/advantage-source-b"
	withTestAdvantageGrant(t, slugA, advantageGrant{RollType: "Stealth checks", Direction: directionAdvantage})
	withTestAdvantageGrant(t, slugB, advantageGrant{RollType: "Stealth checks", Direction: directionAdvantage})

	features := []grantedFeatureRow{
		{Slug: slugA, Name: "Source A"},
		{Slug: slugB, Name: "Source B"},
	}
	traits := computePassiveTraits(features, 1)
	if len(traits.Advantages) != 1 {
		t.Fatalf("Advantages = %+v, want exactly 1 merged entry", traits.Advantages)
	}
	if len(traits.Advantages[0].Sources) != 2 {
		t.Errorf("Advantages[0].Sources = %+v, want both Source A and Source B", traits.Advantages[0].Sources)
	}
}

// TestComputePassiveTraitsNoxiousHandiworkAdvantage pins Puppet Master/Black
// Technique's Noxious Handiwork (14th level): "...you gain resistance to
// poison damage and have advantage on saving throws that would inflict the
// envenomed condition" (rules.db subclass_features, confirmed verbatim
// 2026-08-26). The flat Poison resistance half is already covered by
// TestComputePassiveTraits* elsewhere via passiveTraitGrants; this pins the
// advantageGrants half specifically.
func TestComputePassiveTraitsNoxiousHandiworkAdvantage(t *testing.T) {
	feature := grantedFeatureRow{
		Slug: "class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/noxious-handiwork",
		Name: "Noxious Handiwork",
	}
	traits := computePassiveTraits([]grantedFeatureRow{feature}, 14)

	if len(traits.Advantages) != 1 || traits.Advantages[0].RollType != "saving throws against becoming Envenomed" {
		t.Fatalf("Advantages = %+v, want saving throws against becoming Envenomed", traits.Advantages)
	}
	if len(traits.Resistances) != 1 || traits.Resistances[0].Target != "Poison" {
		t.Errorf("Resistances = %+v, want Poison resistance alongside the advantage grant", traits.Resistances)
	}
}

// withTestPassiveNoteGrant is passiveNoteGrants' counterpart to
// withTestAdvantageGrant — registers a synthetic entry under a fake slug for
// the duration of a test, restoring the real table afterward.
func withTestPassiveNoteGrant(t *testing.T, slug string, grant passiveNoteGrant) {
	t.Helper()
	prev, existed := passiveNoteGrants[slug]
	passiveNoteGrants[slug] = append([]passiveNoteGrant{}, grant)
	t.Cleanup(func() {
		if existed {
			passiveNoteGrants[slug] = prev
		} else {
			delete(passiveNoteGrants, slug)
		}
	})
}

// TestComputePassiveTraitsEnhancedVisionNote pins Puppet Master/Purple
// Technique's Enhanced Vision (6th level): the same sentence that stacks 60ft
// of Darkvision (senseGrants, already covered elsewhere) also "doubles your
// normal sight range... You can accurately make out the details of things
// within 1 mile of you" (rules.db subclass_features, confirmed verbatim
// 2026-08-26) — no sight-range field exists to double, so this resolves as a
// Notes entry alongside the Darkvision sense.
func TestComputePassiveTraitsEnhancedVisionNote(t *testing.T) {
	feature := grantedFeatureRow{
		Slug: "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/enhanced-vision",
		Name: "Enhanced Vision",
	}
	traits := computePassiveTraits([]grantedFeatureRow{feature}, 6)

	if len(traits.Notes) != 1 || traits.Notes[0].Text != "Sight range doubled; 1-mile detail" {
		t.Fatalf("Notes = %+v, want the sight-range note", traits.Notes)
	}
	if len(traits.Notes[0].Sources) != 1 || traits.Notes[0].Sources[0] != "Enhanced Vision" {
		t.Errorf("Notes[0].Sources = %+v, want just Enhanced Vision", traits.Notes[0].Sources)
	}
	if len(traits.Senses) != 1 || traits.Senses[0].Sense != "Darkvision" || traits.Senses[0].Feet != 60 {
		t.Errorf("Senses = %+v, want 60ft Darkvision alongside the note", traits.Senses)
	}
}

// TestComputePassiveTraitsMasterOfWhiteTechniqueNote pins Puppet Master/
// White Technique's capstone (20th level): "...double the specified ranges
// of any White Technique Feature, as well as your Chakra Threads" (rules.db
// subclass_features, confirmed verbatim 2026-08-26) — no range field exists
// anywhere in this app for Chakra Threads or any other White Technique
// feature to double, so this resolves as a Notes entry.
func TestComputePassiveTraitsMasterOfWhiteTechniqueNote(t *testing.T) {
	feature := grantedFeatureRow{
		Slug: "class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/master-of-the-white-technique",
		Name: "Master of the White Technique",
	}
	traits := computePassiveTraits([]grantedFeatureRow{feature}, 20)

	if len(traits.Notes) != 1 || traits.Notes[0].Text != "Chakra Threads & Feature ranges doubled" {
		t.Fatalf("Notes = %+v, want the range-doubling note", traits.Notes)
	}
}

// TestComputePassiveTraitsNoteMinLevelGate mirrors
// TestComputePassiveTraitsAdvantageMinLevelGate for passiveNoteGrants: a
// grant below its MinLevel must not appear at all.
func TestComputePassiveTraitsNoteMinLevelGate(t *testing.T) {
	const slug = "test/synthetic/feature/gated-note"
	withTestPassiveNoteGrant(t, slug, passiveNoteGrant{Text: "Synthetic gated note", MinLevel: 10})
	feature := grantedFeatureRow{Slug: slug, Name: "Gated Note Feature"}

	below := computePassiveTraits([]grantedFeatureRow{feature}, 9)
	if len(below.Notes) != 0 {
		t.Errorf("Notes below MinLevel = %+v, want none", below.Notes)
	}

	atLevel := computePassiveTraits([]grantedFeatureRow{feature}, 10)
	if len(atLevel.Notes) != 1 || atLevel.Notes[0].Text != "Synthetic gated note" {
		t.Errorf("Notes at MinLevel = %+v, want the synthetic note", atLevel.Notes)
	}
}

// TestComputePassiveTraitsNoteMergesSources mirrors
// TestComputePassiveTraitsAdvantageMergesSources for passiveNoteGrants: two
// different features granting the identical Text collapse into one entry
// whose Sources lists both.
func TestComputePassiveTraitsNoteMergesSources(t *testing.T) {
	const slugA = "test/synthetic/feature/note-source-a"
	const slugB = "test/synthetic/feature/note-source-b"
	withTestPassiveNoteGrant(t, slugA, passiveNoteGrant{Text: "Shared note"})
	withTestPassiveNoteGrant(t, slugB, passiveNoteGrant{Text: "Shared note"})

	features := []grantedFeatureRow{
		{Slug: slugA, Name: "Source A"},
		{Slug: slugB, Name: "Source B"},
	}
	traits := computePassiveTraits(features, 1)
	if len(traits.Notes) != 1 {
		t.Fatalf("Notes = %+v, want exactly 1 merged entry", traits.Notes)
	}
	if len(traits.Notes[0].Sources) != 2 {
		t.Errorf("Notes[0].Sources = %+v, want both Source A and Source B", traits.Notes[0].Sources)
	}
}

// TestComputePassiveTraitsUntouchableDisadvantage pins Disturbance's
// Untouchable (6th level) as a real registered advantageGrants entry:
// "creatures who would spend their Reaction to make an attack of any type
// targeting you, is made at disadvantage" (rules.db subclass_features,
// confirmed verbatim 2026-08-26).
func TestComputePassiveTraitsUntouchableDisadvantage(t *testing.T) {
	untouchable := grantedFeatureRow{
		Slug: "class/taijutsu-specialist/group/taijutsu-style/disturbance/feature/untouchable",
		Name: "Untouchable",
	}

	traits := computePassiveTraits([]grantedFeatureRow{untouchable}, 6)
	if len(traits.Disadvantages) != 1 {
		t.Fatalf("Disadvantages = %+v, want exactly 1 entry", traits.Disadvantages)
	}
	if traits.Disadvantages[0].RollType != "reaction attacks made against you" {
		t.Errorf("Disadvantages[0].RollType = %q, want %q", traits.Disadvantages[0].RollType, "reaction attacks made against you")
	}
	if len(traits.Disadvantages[0].Sources) != 1 || traits.Disadvantages[0].Sources[0] != "Untouchable" {
		t.Errorf("Disadvantages[0].Sources = %+v, want just Untouchable", traits.Disadvantages[0].Sources)
	}
	if len(traits.Advantages) != 0 {
		t.Errorf("Advantages = %+v, want none", traits.Advantages)
	}
}

// TestComputePassiveTraitsShinobisKarmaBody pins Hunter-Nin/Wolves Legacy's
// Shinobi's Karma: Body (3rd level): "Increase the number of failed death
// saves you need by 2, before you die, and you gain advantage on death
// saves" (rules.db subclass_features, confirmed verbatim 2026-08-26). The
// advantage half resolves as an Advantages entry; the death-save-count
// increase (no death-save-tracking mechanism exists anywhere in this app)
// resolves as a Notes entry instead.
func TestComputePassiveTraitsShinobisKarmaBody(t *testing.T) {
	feature := grantedFeatureRow{
		Slug: "class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-body",
		Name: "Shinobi's Karma: Body",
	}
	traits := computePassiveTraits([]grantedFeatureRow{feature}, 3)

	if len(traits.Advantages) != 1 || traits.Advantages[0].RollType != "Death saving throws" {
		t.Fatalf("Advantages = %+v, want Advantage on Death saving throws", traits.Advantages)
	}
	if len(traits.Advantages[0].Sources) != 1 || traits.Advantages[0].Sources[0] != "Shinobi's Karma: Body" {
		t.Errorf("Advantages[0].Sources = %+v, want just Shinobi's Karma: Body", traits.Advantages[0].Sources)
	}
	if len(traits.Notes) != 1 || traits.Notes[0].Text != "You need 5 failed death saving throws (instead of 3) before you die" {
		t.Fatalf("Notes = %+v, want the death-save-cap note", traits.Notes)
	}
}

// TestComputePassiveTraitsShinobisKarmaWill pins Hunter-Nin/Wolves Legacy's
// Shinobi's Karma: Will (14th level): "saving throws you make against a
// Genjutsu that would Restrain, incapacitate, slow or stun you are made at
// advantage" (rules.db subclass_features, confirmed verbatim 2026-08-26).
// The unconditional Charisma save proficiency this same feature also grants
// is covered by internal/features' own fixedProficiencyGrants table, not
// this one.
func TestComputePassiveTraitsShinobisKarmaWill(t *testing.T) {
	feature := grantedFeatureRow{
		Slug: "class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/shinobis-karma-will",
		Name: "Shinobi's Karma: Will",
	}
	traits := computePassiveTraits([]grantedFeatureRow{feature}, 14)

	if len(traits.Advantages) != 1 {
		t.Fatalf("Advantages = %+v, want exactly 1 entry", traits.Advantages)
	}
	want := "saving throws against a Genjutsu that would Restrain, Incapacitate, Slow, or Stun you"
	if traits.Advantages[0].RollType != want {
		t.Errorf("Advantages[0].RollType = %q, want %q", traits.Advantages[0].RollType, want)
	}
	if len(traits.Notes) != 0 {
		t.Errorf("Notes = %+v, want none (Will has no death-save clause)", traits.Notes)
	}
}

// TestMergePassiveAdvantageNilEntry confirms mergePassiveAdvantage is a
// no-op when passed a nil entry — the same "nothing to add" shape
// mergePassiveResistance already establishes for a character who hasn't
// made the underlying pick yet (e.g. Food For the Soul before its first
// selection).
func TestMergePassiveAdvantageNilEntry(t *testing.T) {
	summary := PassiveTraitSummary{Advantages: []AdvantageEntry{{RollType: "Initiative checks", Sources: []string{"Battle Readiness"}}}}
	merged := mergePassiveAdvantage(summary, nil)
	if len(merged.Advantages) != 1 || merged.Advantages[0].RollType != "Initiative checks" {
		t.Errorf("merged.Advantages = %+v, want the original entry untouched", merged.Advantages)
	}
}

// TestMergePassiveAdvantageNewRollType confirms a dynamically-resolved entry
// (RollType unknown to the static advantageGrants table, e.g. Food For the
// Soul's player-picked ability score) is appended as its own row.
func TestMergePassiveAdvantageNewRollType(t *testing.T) {
	summary := PassiveTraitSummary{Advantages: []AdvantageEntry{{RollType: "Initiative checks", Sources: []string{"Battle Readiness"}}}}
	merged := mergePassiveAdvantage(summary, &AdvantageEntry{RollType: "Strength checks", Sources: []string{"Food For the Soul"}})
	if len(merged.Advantages) != 2 {
		t.Fatalf("merged.Advantages = %+v, want 2 entries", merged.Advantages)
	}
}

// TestMergePassiveAdvantageSameRollTypeMergesSources confirms a
// dynamically-resolved entry that happens to share its RollType with an
// already-computed entry folds into it (Sources combined) instead of
// producing a duplicate row — the Advantage/Disadvantage counterpart to
// mergePassiveResistance's own same-Target merge behavior.
func TestMergePassiveAdvantageSameRollTypeMergesSources(t *testing.T) {
	summary := PassiveTraitSummary{Advantages: []AdvantageEntry{{RollType: "Perception checks", Sources: []string{"Source A"}}}}
	merged := mergePassiveAdvantage(summary, &AdvantageEntry{RollType: "Perception checks", Sources: []string{"Source B"}})
	if len(merged.Advantages) != 1 {
		t.Fatalf("merged.Advantages = %+v, want exactly 1 merged entry", merged.Advantages)
	}
	if len(merged.Advantages[0].Sources) != 2 {
		t.Errorf("merged.Advantages[0].Sources = %+v, want both Source A and Source B", merged.Advantages[0].Sources)
	}
}
