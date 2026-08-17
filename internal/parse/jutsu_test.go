package parse

import (
	"strings"
	"testing"
)

// mkLines builds a Line slice on a single page — fixtures stay readable.
func mkLines(page int, texts ...string) []Line {
	out := make([]Line, 0, len(texts))
	for _, t := range texts {
		out = append(out, Line{Page: page, Text: t})
	}
	return out
}

// One well-formed entry, exercising section context, wrapped field values,
// and name title-casing.
func TestParseJutsuBookBasicEntry(t *testing.T) {
	lines := mkLines(4,
		"NINJUTSU",
		"FIRE RELEASE",
		"D-RANK:",
		"FIRE RELEASE: EMBER TOSS",
		"Classification: Ninjutsu",
		"Rank: D-Rank",
		"Casting Time: 1 Action",
		"Range: 30 Feet",
		"Duration: Instant",
		"Components: HS, CM",
		"Cost: 4 Chakra",
		"Keywords: Fire Release, Ninjutsu",
		"Description: You toss a mote of flame that",
		"scorches one target it hits.",
		"At Higher Ranks: For each rank above D-Rank,",
		"increase the damage by 1d6.",
	)
	jutsu, anomalies := ParseJutsuBook(lines)
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if len(jutsu) != 1 {
		t.Fatalf("got %d jutsu, want 1", len(jutsu))
	}
	j := jutsu[0]
	if j.Name != "Fire Release: Ember Toss" {
		t.Errorf("Name = %q", j.Name)
	}
	if j.CategoryGroup != "Ninjutsu / Fire Release" {
		t.Errorf("CategoryGroup = %q", j.CategoryGroup)
	}
	if j.Rank != "D" {
		t.Errorf("Rank = %q", j.Rank)
	}
	if j.CostChakra == nil || *j.CostChakra != 4 {
		t.Errorf("CostChakra = %v, want 4", j.CostChakra)
	}
	if want := "You toss a mote of flame that scorches one target it hits."; j.Description != want {
		t.Errorf("Description = %q\nwant %q", j.Description, want)
	}
	if !strings.Contains(j.AtHigherRanks, "increase the damage by 1d6") {
		t.Errorf("AtHigherRanks = %q", j.AtHigherRanks)
	}
	if j.SourcePage != 4 {
		t.Errorf("SourcePage = %d", j.SourcePage)
	}
}

// The book sometimes prints a field label without its colon ("Rank C-Rank",
// "Cost 12 Chakra"). The parser recovers the value and flags the typo.
func TestParseJutsuBookColonlessFieldRecovered(t *testing.T) {
	lines := mkLines(75,
		"NINJUTSU",
		"WIND RELEASE",
		"C-RANK:",
		"WIND RELEASE: WHIRLWIND MOVEMENT",
		"Classification: Ninjutsu",
		"Rank C-Rank", // book typo: no colon
		"Casting Time: 1 Bonus Action",
		"Range: 60 Feet",
		"Duration: Instant",
		"Components: HS, CM, M",
		"Cost: 6 Chakra",
		"Keywords: Wind Release, Ninjutsu",
		"Description: You teleport to one space within 60 feet.",
	)
	jutsu, anomalies := ParseJutsuBook(lines)
	if len(jutsu) != 1 {
		t.Fatalf("got %d jutsu, want 1", len(jutsu))
	}
	if jutsu[0].Rank != "C" {
		t.Errorf("Rank = %q, want C (recovered from colon-less label)", jutsu[0].Rank)
	}
	if len(anomalies) != 1 || !strings.Contains(anomalies[0].Problem, "without a colon") {
		t.Errorf("anomalies = %+v, want one colon-typo flag", anomalies)
	}
}

// A description line that happens to START with a field word ("Rank: 3,
// A-Rank: 4" after a hyphen line break) must be absorbed as prose, not
// hijack the already-parsed field. Fields appear in fixed print order, so an
// out-of-order label is never a real field.
func TestParseJutsuBookWrappedLabelAbsorbed(t *testing.T) {
	lines := mkLines(76,
		"NINJUTSU",
		"WIND RELEASE",
		"B-RANK:",
		"WIND RELEASE: BACKLASH",
		"Classification: Ninjutsu",
		"Rank: B-Rank",
		"Casting Time: 1 Reaction",
		"Range: Self",
		"Duration: Instant",
		"Components: HS, CM",
		"Cost: 14 Chakra",
		"Keywords: Wind Release, Ninjutsu",
		"Description: The Rank DC equals 13 + the Jutsu's Rank (D-Rank: 1, C-Rank: 2, B-",
		"Rank: 3, A-Rank: 4, S-Rank: 5). If you roll higher,",
		"the triggering jutsu is nullified.",
	)
	jutsu, anomalies := ParseJutsuBook(lines)
	if len(jutsu) != 1 {
		t.Fatalf("got %d jutsu, want 1", len(jutsu))
	}
	j := jutsu[0]
	if j.Rank != "B" {
		t.Errorf("Rank = %q, want B (wrapped label must not overwrite it)", j.Rank)
	}
	if !strings.Contains(j.Description, "Rank: 3, A-Rank: 4, S-Rank: 5") ||
		!strings.Contains(j.Description, "nullified") {
		t.Errorf("Description lost wrapped text: %q", j.Description)
	}
	// A "Rank:" line right after a line ending in "-" is provably a
	// hyphen-wrapped rank table — recovered silently, no anomaly.
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %+v, want none for a benign hyphen wrap", anomalies)
	}
}

// Entry names may contain square brackets ("AMMO HEART [NAME/ CHANGED]");
// they must still be recognized as entry starts so consecutive entries don't
// merge into one.
func TestParseJutsuBookBracketedNameStartsEntry(t *testing.T) {
	lines := mkLines(304,
		"BUKIJUTSU",
		"B-RANK:",
		"ALL-IN ATTACK",
		"Classification: Bukijutsu",
		"Rank: B-Rank",
		"Casting Time: 1 Action",
		"Range: Weapons Range",
		"Duration: Instant",
		"Components: W (Any)",
		"Cost: 36 Chakra",
		"Keywords: Bukijutsu",
		"Description: You lead your allies.",
		"AMMO HEART [NAME/ CHANGED]",
		"Classification: Bukijutsu",
		"Rank: B-Rank",
		"Casting Time: 1 Action",
		"Range: Weapons Range",
		"Duration: Instant",
		"Components: W (Any Ammo)",
		"Cost: 12 Chakra",
		"Keywords: Bukijutsu",
		"Description: You fire a shot.",
	)
	jutsu, anomalies := ParseJutsuBook(lines)
	if len(anomalies) != 0 {
		t.Fatalf("unexpected anomalies: %+v", anomalies)
	}
	if len(jutsu) != 2 {
		t.Fatalf("got %d jutsu, want 2 — bracketed name failed to start an entry", len(jutsu))
	}
	if jutsu[0].Description != "You lead your allies." {
		t.Errorf("first entry absorbed the second: %q", jutsu[0].Description)
	}
	if jutsu[1].Name != "Ammo Heart [Name/ Changed]" {
		t.Errorf("second entry Name = %q", jutsu[1].Name)
	}
}

// An entry whose Rank disagrees with the running rank-section header is a
// book error the validation report must surface.
func TestParseJutsuBookRankSectionMismatchFlagged(t *testing.T) {
	lines := mkLines(244,
		"TAIJUTSU",
		"B-RANK:",
		"COMBO BREAK",
		"Classification: Taijutsu",
		"Rank: A-Rank", // disagrees with the B-RANK header above
		"Casting Time: 1 Reaction",
		"Range: Self",
		"Duration: Instant",
		"Components: M",
		"Cost: 12 Chakra",
		"Keywords: Taijutsu, Finisher",
		"Description: You break the combo.",
	)
	jutsu, anomalies := ParseJutsuBook(lines)
	if len(jutsu) != 1 {
		t.Fatalf("got %d jutsu, want 1", len(jutsu))
	}
	if len(anomalies) != 1 || !strings.Contains(anomalies[0].Problem, "does not match the running B-Rank") {
		t.Errorf("anomalies = %+v, want one rank-mismatch flag", anomalies)
	}
}

// A missing required field (Sunbeam's description is mislabeled in print) is
// flagged, and the mislabel itself is flagged too — never silently merged.
func TestParseJutsuBookDuplicateLabelFlagged(t *testing.T) {
	lines := mkLines(96,
		"NINJUTSU",
		"FIRE RELEASE",
		"B-RANK:",
		"FIRE RELEASE: SUNBEAM",
		"Classification: Ninjutsu",
		"Rank: B-Rank",
		"Casting Time: 1 Action",
		"Range: Self (60-foot line)",
		"Duration: Concentration, up to 1 minute",
		"Components: HS, CM, CS",
		"Cost: 14 Chakra",
		"Keywords: Fire Release, Ninjutsu",
		"Keywords: You create a beam of brilliant white-hot light.", // book typo: should say Description
	)
	jutsu, anomalies := ParseJutsuBook(lines)
	if len(jutsu) != 1 {
		t.Fatalf("got %d jutsu, want 1", len(jutsu))
	}
	var sawDup, sawMissing bool
	for _, a := range anomalies {
		if strings.Contains(a.Problem, "kept as text") {
			sawDup = true
		}
		if a.Problem == "missing field Description" {
			sawMissing = true
		}
	}
	if !sawDup || !sawMissing {
		t.Errorf("anomalies = %+v, want duplicate-label and missing-Description flags", anomalies)
	}
}
