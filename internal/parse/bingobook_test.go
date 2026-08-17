package parse

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

func TestParseAdversaryRolesFixture(t *testing.T) {
	lines := mkLines(19,
		"APPLYING ROLES",
		"Roles are meaningfully applied to Standards and Elites.",
		"ROLE DEFINITIONS",
		"STRIKER",
		"A Striker exists to deal damage.",
		"DEFENDER",
		"A Defender exists to survive.",
		"ROLE TRAITS",
		"The following traits are traits available to Adversaries.",
	)
	roles, anomalies := ParseAdversaryRoles(lines)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(roles) != 2 {
		t.Fatalf("roles = %+v", roles)
	}
	if roles[0].Name != "Striker" || roles[0].Description != "A Striker exists to deal damage." {
		t.Errorf("role 0 = %+v", roles[0])
	}
	if roles[1].Name != "Defender" || roles[1].Description != "A Defender exists to survive." {
		t.Errorf("role 1 = %+v", roles[1])
	}
}

func TestParseAdversaryRoleTraitsFixture(t *testing.T) {
	lines := mkLines(20,
		"ROLE TRAITS",
		"The following traits are traits available to Adversaries.",
		"CLARIFICATIONS",
		"Auras. Traits classified as Aura's all originate from the Adversary.",
		"Ambush. Traits classified as Ambushes trigger on a hit.",
		"STRIKER",
		"AGGRESSIVE",
		"D-Rank",
		"If this Adversary triggers an attack of opportunity, it strikes back.",
		"BRUTAL MOMENTUM",
		"D-Rank",
		"When this Adversary moves at least 15 feet toward a creature before",
		"hitting them, the target must succeed on a save or fall Prone.",
		"LURKER",
		"AMBUSH: HAMSTRUNG",
		"D-Rank",
		"The target's movement speed is halved.",
		"DEFENDER",
		"STALWART",
		"D-Rank",
		"When reduced below half HP, gain Temporary Hit Points.",
	)
	traits, anomalies := ParseAdversaryRoleTraits(lines)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(traits) != 4 {
		t.Fatalf("traits = %+v", traits)
	}
	if traits[0].Name != "Aggressive" || traits[0].RoleName != "Striker" || traits[0].Rank != "D" {
		t.Errorf("trait 0 = %+v", traits[0])
	}
	if traits[1].Name != "Brutal Momentum" || traits[1].RoleName != "Striker" {
		t.Errorf("trait 1 = %+v", traits[1])
	}
	// The CLARIFICATIONS glossary text must never leak into a trait's
	// description — traits[0]'s description shouldn't contain "Aura's".
	for _, tr := range traits {
		if got := tr.Description; got == "" {
			t.Errorf("%s: empty description", tr.Name)
		}
	}
	if traits[2].Name != "Ambush: Hamstrung" || traits[2].RoleName != "Lurker" {
		t.Errorf("trait 2 (Lurker) = %+v", traits[2])
	}
	if traits[3].Name != "Stalwart" || traits[3].RoleName != "Defender" {
		t.Errorf("trait 3 (Defender) = %+v", traits[3])
	}
}

func TestParseAdversaryRanksFixture(t *testing.T) {
	lines := mkLines(1,
		"MINIONS",
		"A Minion is weaker than other adversaries.",
		"MINION ADVERSARY TEMPLATE",
		"Hit Points: 10 + Level Initiative Bonus: -2",
		"NOTE: Minions Automatically Fail Saving Throws",
		"MINION FREEFORM TEMPLATE",
		"Level Freeform Slots",
		"1+ 1",
		"5+ 2",
		"STANDARDS",
		"A Standard is an adversary built to stand toe to toe.",
		"STANDARD FREEFORM TEMPLATE",
		"Level Freeform Slots",
		"1+ 2",
		"5+ 3",
		"ELITES",
		"An Elite is a commander.",
		"SOLOS",
		"A Solo is the capstone of a mission.",
		"ELITE/SOLO FREEFORM TEMPLATE",
		"Level Elite Freeform Slots  Level Solo Freeform Slots",
		"3+ 3 5+ 3",
		"- - 19+ 7",
		"ELITE ADVERSARY TEMPLATE",
		"Hit/Chakra Point Modifier: +1.25",
		"AC Bonus +1 Saving Throw Bonus +1",
		"Save DC Bonus +1 Initiative +2",
		"SOLO ADVERSARY TEMPLATE",
		"Hit/Chakra Point Modifier: +1.5 + 0.5 x Per Player",
		"AC Bonus +2 Saving Throw Bonus +2",
		"Save DC Bonus +2 Initiative +3",
		"ELITE ENHANCEMENTS",
		"This section covers Elite Actions.",
	)
	ranks, slots, anomalies := ParseAdversaryRanks(lines)
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(ranks) != 4 {
		t.Fatalf("ranks = %+v", ranks)
	}
	minion, standard, elite, solo := ranks[0], ranks[1], ranks[2], ranks[3]
	if minion.Name != "Minion" || minion.HPFormula != "10 + Level" || minion.InitBonus != -2 {
		t.Errorf("minion = %+v", minion)
	}
	if standard.Name != "Standard" || standard.HPFormula != "" || standard.ACBonus != 0 {
		t.Errorf("standard = %+v", standard)
	}
	if elite.HPFormula != "+1.25" || elite.ACBonus != 1 || elite.SaveBonus != 1 ||
		elite.SaveDCBonus != 1 || elite.InitBonus != 2 {
		t.Errorf("elite = %+v", elite)
	}
	if solo.HPFormula != "+1.5 + 0.5 x Per Player" || solo.ACBonus != 2 || solo.InitBonus != 3 {
		t.Errorf("solo = %+v", solo)
	}

	// Minion(2) + Standard(2) + Elite(1) + Solo(2, incl. the "- -" row that
	// only has a Solo entry) = 7.
	if len(slots) != 7 {
		t.Fatalf("slots = %+v", slots)
	}
	var eliteCount, soloCount int
	for _, s := range slots {
		switch s.RankName {
		case "Elite":
			eliteCount++
		case "Solo":
			soloCount++
		}
	}
	if eliteCount != 1 || soloCount != 2 {
		t.Errorf("eliteCount=%d soloCount=%d, want 1, 2", eliteCount, soloCount)
	}
}

// loadBingoBookLines opens the real Bingo Book and returns its lines, or
// nil after calling t.Skip if the sourcebook isn't present on this machine.
func loadBingoBookLines(t *testing.T) []Line {
	t.Helper()
	path := filepath.Join("/home/sergio/Documents/N5E",
		"Bingo Book Pack 1 - Adversaries - Class, Freeform, Roles, Role Traits.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook not available: %v", err)
		return nil
	}
	doc, err := extract.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []Line
	for n := 1; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, Line{Page: n, Text: ln})
		}
	}
	return lines
}

// Whole-book regression (verified 2026-07-27 against Pack 1). Skips when
// the sourcebook is absent.
func TestParseBingoBookWholeBook(t *testing.T) {
	lines := loadBingoBookLines(t)
	if lines == nil {
		return
	}

	roles, ra := ParseAdversaryRoles(lines)
	if len(ra) != 0 {
		t.Errorf("role anomalies: %+v", ra)
	}
	if len(roles) != 5 {
		t.Errorf("roles = %d, want 5", len(roles))
	}
	wantRoles := map[string]bool{"Striker": true, "Lurker": true, "Defender": true, "Controller": true, "Supporter": true}
	for _, r := range roles {
		if !wantRoles[r.Name] {
			t.Errorf("unexpected role name %q", r.Name)
		}
		if r.Description == "" {
			t.Errorf("%s: empty description", r.Name)
		}
	}

	traits, ta := ParseAdversaryRoleTraits(lines)
	if len(ta) != 0 {
		t.Errorf("trait anomalies: %+v", ta)
	}
	if len(traits) != 150 {
		t.Errorf("role traits = %d, want 150", len(traits))
	}
	byRole := map[string]int{}
	byRank := map[string]int{}
	seen := map[string]bool{}
	for _, tr := range traits {
		byRole[tr.RoleName]++
		byRank[tr.Rank]++
		if tr.Description == "" {
			t.Errorf("%s: empty description", tr.Name)
		}
		if seen[tr.Name] {
			t.Errorf("duplicate trait name %q", tr.Name)
		}
		seen[tr.Name] = true
	}
	for role := range wantRoles {
		if byRole[role] != 30 {
			t.Errorf("role %s has %d traits, want 30", role, byRole[role])
		}
	}
	wantRanks := map[string]int{"D": 50, "C": 40, "B": 30, "A": 20, "S": 10}
	for rank, want := range wantRanks {
		if byRank[rank] != want {
			t.Errorf("rank %s has %d traits, want %d", rank, byRank[rank], want)
		}
	}

	ranks, slots, ka := ParseAdversaryRanks(lines)
	if len(ka) != 0 {
		t.Errorf("rank anomalies: %+v", ka)
	}
	if len(ranks) != 4 {
		t.Errorf("ranks = %d, want 4", len(ranks))
	}
	if len(slots) != 16 {
		t.Errorf("freeform slots = %d, want 16", len(slots))
	}
}
