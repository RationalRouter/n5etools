package charsheet

import "testing"

// TestSkillDescriptionsComplete checks structural completeness (every
// skill SkillAbility knows about has a non-empty blurb) — the prose
// accuracy itself is a one-time human transcription check against the
// book, same as SkillAbility's own mapping.
func TestSkillDescriptionsComplete(t *testing.T) {
	for skill := range SkillAbility {
		desc, ok := SkillDescriptions[skill]
		if !ok || desc == "" {
			t.Errorf("SkillDescriptions missing an entry for %q", skill)
		}
	}
	if len(SkillDescriptions) != len(SkillAbility) {
		t.Errorf("SkillDescriptions has %d entries, SkillAbility has %d — expected the same 21 skills",
			len(SkillDescriptions), len(SkillAbility))
	}
}
