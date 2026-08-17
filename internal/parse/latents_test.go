package parse

import (
	"strings"
	"testing"
)

// Fixture mirroring the real table layout: wrapped ability names, split
// superscript ordinals, prerequisite parentheticals, the section-opening
// feat, and the LEGACY CONTENT terminator.
func TestParseBloodlineLatentsFixture(t *testing.T) {
	lines := mkLines(218,
		"YUKI CLAN", // preceding clan content is ignored
		"Some clan text.",
		"BLOODLINE LATENTS",
		"BLOODLINE, LATENT",
		"Category: Clan, Rare",
		"Prerequisite: You must take your first instance of this",
		"feat between levels 1 and 4.",
		"You have the blood of a famous clan. You gain benefits:",
		"• You gain 10 Bloodline Points.",
		"LATENT ABURAME",
		"Bloodline Ability",
		"Name",
		"Bloodline",
		"Point Cost Ability description",
		"Latent Bug Host I 2 Beginning at 1",
		"st",
		"level, twice per long rest, you can add 1d4 to any constitution saving throw.",
		"Latent Bug Host II 3 (You must have Latent Bug Host I) Beginning at 7",
		"th",
		"level, the bonus increases to a d6.",
		"Latent Chakra",
		"Consumption I",
		"2 Beginning at 3",
		"rd",
		"level, when you would cast an Aburame Clan Hijutsu, that deals damage, you",
		"can choose to instead reduce its damage by half and deal Chakra damage instead.",
		"D-Rank Hijutsu 2 You gain the ability to learn D-Rank Aburame Clan Hijutsu",
		"Clan Feats 5 You gain the ability to learn Clan Feats, with Aburame Clan as a Prerequisite",
		// Patterns from other tables: zero-cost rows (Fuma, Hyūga), the cost
		// digit ending the name's wrapped line (Hoshi "Chakra I 2"), digit
		// name words (Genwa "1s and 0s"), lowercase "with" (Shoton).
		"Latent Branch Family  0 Being born with only a fraction of the gift.",
		"Latent Star",
		"Chakra I 2",
		"Beginning at 1st level, you have a pool of Star Chakra.",
		"Latent 1s and",
		"0s I",
		"2 Beginning at 1st level, you gain proficiency in a kit.",
		"Latent One with Earth 5 Beginning at 7",
		"th",
		"level, You may use Charisma as your Ninjutsu ability modifier.",
		// Four-line name wrap (Synthetic Human "Latent Corrupted Chakra Mode II").
		"Latent",
		"Corrupted",
		"Chakra Mode",
		"II",
		"3 (You must have Latent Corrupted Chakra Mode I) Beginning at 15th level, you gain benefits.",
		"LEGACY CONTENT",
		"BLOODLINE, LATENT",
		"Category: Clan (legacy, must not be parsed)",
	)
	feat, latents, anomalies := ParseBloodlineLatents(lines)
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if feat == nil || feat.Name != "Bloodline, Latent" || feat.Category != "Clan, Rare" {
		t.Fatalf("feat = %+v", feat)
	}
	if len(latents) != 10 {
		t.Fatalf("got %d latents, want 10: %+v", len(latents), latents)
	}
	l := latents[0]
	if l.ClanName != "Aburame" || l.Name != "Latent Bug Host I" || l.PointCost != 2 {
		t.Errorf("latent 0 = %+v", l)
	}
	if !strings.Contains(l.Description, "Beginning at 1st level, twice per long rest") {
		t.Errorf("superscript ordinal not glued: %q", l.Description)
	}
	if latents[1].Prerequisites != "You must have Latent Bug Host I" {
		t.Errorf("prereq = %q", latents[1].Prerequisites)
	}
	if strings.HasPrefix(latents[1].Description, "(") {
		t.Errorf("prereq not stripped from description: %q", latents[1].Description)
	}
	if latents[2].Name != "Latent Chakra Consumption I" {
		t.Errorf("wrapped name = %q", latents[2].Name)
	}
	if !strings.Contains(latents[2].Description, "Beginning at 3rd level") {
		t.Errorf("wrapped-name row description: %q", latents[2].Description)
	}
	if latents[3].Name != "D-Rank Hijutsu" || latents[4].Name != "Clan Feats" || latents[4].PointCost != 5 {
		t.Errorf("standard rows: %+v %+v", latents[3], latents[4])
	}
	if latents[5].Name != "Latent Branch Family" || latents[5].PointCost != 0 {
		t.Errorf("zero-cost row: %+v", latents[5])
	}
	if latents[6].Name != "Latent Star Chakra I" || latents[6].PointCost != 2 ||
		!strings.Contains(latents[6].Description, "pool of Star Chakra") {
		t.Errorf("cost-ends-name-line row: %+v", latents[6])
	}
	if latents[7].Name != "Latent 1s and 0s I" || latents[7].PointCost != 2 {
		t.Errorf("digit-word name row: %+v", latents[7])
	}
	if latents[8].Name != "Latent One with Earth" || latents[8].PointCost != 5 ||
		!strings.Contains(latents[8].Description, "Beginning at 7th level") {
		t.Errorf("lowercase-connector row: %+v", latents[8])
	}
	if latents[9].Name != "Latent Corrupted Chakra Mode II" || latents[9].PointCost != 3 ||
		latents[9].Prerequisites != "You must have Latent Corrupted Chakra Mode I" {
		t.Errorf("four-line name wrap row: %+v", latents[9])
	}
	// The stray "Latent" must not leak into the preceding row's description.
	if strings.Contains(latents[8].Description, "Latent") {
		t.Errorf("name fragment leaked into previous description: %q", latents[8].Description)
	}
}
