package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/extract"
)

// Fixture covering the real quirks: a bulleted role whose second entry
// glues an unbulleted definition ("Multiattack:") onto it, a two-part
// attack name format ("Claws. Melee..."), a curly-apostrophe feature name
// ("Tokage's..."), a JUTSU SPECIALTY heading that wraps across two lines,
// and a Jutsu Specialty bullet that prints AFTER the stat table (a genuine
// book layout artifact — see Deer in the real chapter).
func TestParseSummonTribesFixture(t *testing.T) {
	lines := mkLines(139,
		"LIZARD",
		"Cold-blooded and patient, lizards strike without warning.",
		"Summon Type: Dragon",
		"Toughness: 8",
		"Defensive Ability Score: Dexterity",
		"Saving Throws: Dexterity, Constitution, Wisdom",
		"Creature Skills: Stealth, Perception, Survival",
		"Creature Senses: Darkvision (60ft)",
		"ROLES",
		"Lizards are ambush predators. Select one of the following roles;",
		"• Lurker: This summon gains the Lethal Attack trait.",
		"• Striker: This Summon has the Multiattack trait.",
		"Multiattack: You can make up to two attacks using your Tail.",
		"NATURAL/WEAPONS",
		"Tail. Melee Weapon Attack: 10ft., one target. Dex + Prof to",
		"hit, +Dex Bludgeoning Damage.",
		"SAVE DC’S & ATTACK BONUSES:",
		"All Jutsu Save DC’s: 8 + Dexterity modifier + Summoner’s",
		"Proficiency Bonus.",
		"All Jutsu Attack bonus: Dexterity modifier + Summoner’s",
		"Proficiency Bonus.",
		"SPECIAL FEATURES:",
		"D-RANK",
		"Tokage’s Ferocity: When this summon inflicts a condition, it",
		"gains a +2 bonus to its AC.",
		"C-RANK",
		"Tokage’s Gullet: If this summon casts a line jutsu, widen it.",
		"B-RANK",
		"Tokage’s Martial Skill. This summon can wield a weapon.",
		"A-RANK",
		"Tokage’s Power. This summon cannot be interrupted.",
		"S-RANK",
		"Tokage’s Control: Affected creatures gain twice the ranks.",
		"LIZARD JUTSU",
		"SPECIALTY",
		"Lizards have access to any Jutsu with the following keywords;",
		"• Earth Release keyword.",
		"LIZARD",
		"Rank Level Size STR DEX CON INT WIS CHA Jutsu Slots Jutsu Speed",
		"D-Rank 4",
		"th",
		"S 10 16 12 10 14 10 5 2 D-Rank 40ft",
		"C-Rank 8",
		"th",
		"S-M +6 Ability Score Increases up to 20. 7 2 D-Rank, 2 C-Rank 40ft",
		"B-Rank 12",
		"th",
		"S-M +6 Ability Score Increases up to 22. 10 2 C-Rank (or Lower), 2 B-Rank 50ft",
		"A-Rank 16",
		"th",
		"S-L +6 Ability Score Increases up to 24. 13 3 B-Rank (or Lower), 1 A-Rank. 50ft",
		"S-Rank 20",
		"th",
		"S-L +6 Ability Score Increases up to 26. 16 3 A-Rank (or Lower), 1 S-Rank. 60ft",
		"• Wind Release, without a nature release.",
	)
	tribes, anomalies := ParseSummonTribes(mkLinesFor(lines, "SUMMONING JUTSU"))
	if len(anomalies) != 0 {
		t.Fatalf("anomalies: %+v", anomalies)
	}
	if len(tribes) != 1 {
		t.Fatalf("got %d tribes, want 1: %+v", len(tribes), tribes)
	}
	lz := tribes[0]

	if lz.Name != "Lizard" || lz.SummonType != "Dragon" || lz.Toughness != 8 {
		t.Errorf("header = %+v", lz)
	}
	if len(lz.Roles) != 2 || lz.Roles[1].Name != "Striker" ||
		!strings.Contains(lz.Roles[1].Description, "Multiattack: You can make") {
		t.Errorf("unbulleted continuation not absorbed into role: %+v", lz.Roles)
	}
	if len(lz.Attacks) != 1 || lz.Attacks[0].Name != "Tail" ||
		!strings.Contains(lz.Attacks[0].Description, "Bludgeoning Damage") {
		t.Errorf("attacks = %+v", lz.Attacks)
	}
	if lz.JutsuSaveDCText != "8 + Dexterity modifier + Summoner’s Proficiency Bonus." {
		t.Errorf("save dc = %q", lz.JutsuSaveDCText)
	}
	if len(lz.Features) != 5 {
		t.Fatalf("got %d features, want 5: %+v", len(lz.Features), lz.Features)
	}
	if lz.Features[0].Name != "Tokage’s Ferocity" || lz.Features[0].Rank != "D" {
		t.Errorf("curly-apostrophe feature name not matched: %+v", lz.Features[0])
	}
	// The wrapped "LIZARD JUTSU" / "SPECIALTY" heading must still open
	// specialty mode, and the bullet displaced after the table must land
	// in the same field, not get flagged as a broken progression row.
	if !strings.Contains(lz.JutsuSpecialty, "Earth Release keyword") ||
		!strings.Contains(lz.JutsuSpecialty, "Wind Release, without a nature release") {
		t.Errorf("jutsu specialty (incl. displaced bullet) = %q", lz.JutsuSpecialty)
	}
	if len(lz.Progression) != 5 {
		t.Fatalf("got %d progression rows, want 5: %+v", len(lz.Progression), lz.Progression)
	}
	if lz.Progression[0].Level != 4 || lz.Progression[0].SizeText != "S" ||
		lz.Progression[0].StatsText != "10 16 12 10 14 10 5 2 D-Rank 40ft" {
		t.Errorf("D-Rank progression row = %+v", lz.Progression[0])
	}
	if lz.Progression[4].StatsText != "+6 Ability Score Increases up to 26. 16 3 A-Rank (or Lower), 1 S-Rank. 60ft" {
		t.Errorf("S-Rank progression row = %+v", lz.Progression[4])
	}
}

// mkLinesFor prepends a section-start marker so ParseSummonTribes'
// "SUMMONING JUTSU" anchor is present, without polluting every fixture
// call site above with it.
func mkLinesFor(lines []Line, marker string) []Line {
	return append([]Line{{Page: lines[0].Page, Text: marker}}, lines...)
}

// Whole-book regression (verified 2026-07-18 against v3.1): 20 tribes, 40
// roles, 27 attacks, 295 rank-gated features, 100 progression rows (5 per
// tribe), zero anomalies. Skips when the sourcebook is absent.
func TestParseSummonTribesFullJutsuBook(t *testing.T) {
	path := filepath.Join("/home/sergio/Documents/N5E", "Jiraiyas_Jutsu_Compendium.pdf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sourcebook not available: %v", err)
	}
	doc, err := extract.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []Line
	for n := 4; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			t.Fatalf("page %d: %v", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, Line{Page: n, Text: ln})
		}
	}

	tribes, anomalies := ParseSummonTribes(lines)
	for _, a := range anomalies {
		t.Errorf("anomaly p%d %s: %s", a.Page, a.Subject, a.Problem)
	}
	if len(tribes) != 20 {
		t.Fatalf("tribes = %d, want 20", len(tribes))
	}
	totRoles, totAttacks, totFeatures, totProg := 0, 0, 0, 0
	for _, tr := range tribes {
		totRoles += len(tr.Roles)
		totAttacks += len(tr.Attacks)
		totFeatures += len(tr.Features)
		totProg += len(tr.Progression)
		if tr.SummonType == "" || tr.Description == "" || tr.JutsuSpecialty == "" {
			t.Errorf("%s: incomplete tribe: %+v", tr.Name, tr)
		}
		if len(tr.Progression) != 5 {
			t.Errorf("%s: %d progression rows, want 5", tr.Name, len(tr.Progression))
		}
	}
	if totRoles != 40 {
		t.Errorf("total roles = %d, want 40", totRoles)
	}
	if totAttacks != 27 {
		t.Errorf("total attacks = %d, want 27", totAttacks)
	}
	if totFeatures != 295 {
		t.Errorf("total features = %d, want 295", totFeatures)
	}
	if totProg != 100 {
		t.Errorf("total progression rows = %d, want 100", totProg)
	}
}
