package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

var jutsuBook2 = SourceBook{
	Slug: "book/jutsu-compendium", Title: "Jiraiya's Jutsu Compendium",
	Version: "3.1", FileName: "jutsu.pdf", FileSHA256: "dead",
}

func sampleTribe() parse.SummonTribe {
	return parse.SummonTribe{
		Name: "Bear", SummonType: "Carnivoran", Description: "Powerful and proud.",
		Toughness: 10, DefensiveAbility: "Constitution",
		SavingThrows: "Strength, Constitution, Wisdom", Skills: "Athletics, Perception, Survival",
		Senses: "Darkvision (60ft)", JutsuSaveDCText: "8 + Strength modifier + Summoner's Proficiency Bonus.",
		JutsuAttackText: "Strength modifier + Summoner's Proficiency Bonus.",
		JutsuSpecialty:  "Bears have access to any Jutsu with the Earth Release keyword.",
		SourcePage:      139,
		Roles: []parse.SummonRole{
			{Name: "Defender", Description: "Gains a bonus to its AC."},
			{Name: "Striker", Description: "Has the Multiattack trait."},
		},
		Attacks: []parse.SummonAttack{
			{Name: "Claws", Description: "Melee Weapon Attack: 5ft."},
		},
		Features: []parse.SummonFeature{
			{Name: "Kuma Flex", Rank: "D", Description: "Gains 3 DR.", SourcePage: 139},
			{Name: "Kuma King", Rank: "S", Description: "Immune to Physical Conditions.", SourcePage: 139},
		},
		Progression: []parse.SummonProgressionRow{
			{Rank: "D", Level: 4, SizeText: "M", StatsText: "16 10 14 10 12 10 4 2 D-Rank 30ft"},
			{Rank: "C", Level: 8, SizeText: "M-L", StatsText: "+6 Ability Score Increases up to 20. 6 2 D-Rank, 2 C-Rank 30ft"},
			{Rank: "B", Level: 12, SizeText: "M-L", StatsText: "+6 Ability Score Increases up to 22. 9 2 C-Rank (or Lower), 2 B-Rank 40ft"},
			{Rank: "A", Level: 16, SizeText: "M-H", StatsText: "+6 Ability Score Increases up to 24. 12 3 B-Rank (or Lower), 1 A-Rank. 40ft"},
			{Rank: "S", Level: 20, SizeText: "M-G", StatsText: "+6 Ability Score Increases up to 26. 15 3 A-Rank (or Lower), 1 S-Rank. 50ft"},
		},
	}
}

func TestLoadSummonTribesCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	tribes := []parse.SummonTribe{sampleTribe()}
	r, err := LoadSummonTribes(db, jutsuBook2, tribes)
	if err != nil {
		t.Fatal(err)
	}
	// 1 tribe + 2 features = 3.
	if r.Created != 3 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadSummonTribes(db, jutsuBook2, tribes)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 3 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var summonType string
	var toughness int
	if err := db.QueryRow(`SELECT summon_type, toughness FROM summon_tribes
		WHERE slug = 'summon/bear'`).Scan(&summonType, &toughness); err != nil {
		t.Fatal(err)
	}
	if summonType != "Carnivoran" || toughness != 10 {
		t.Errorf("tribe row: type=%s toughness=%d", summonType, toughness)
	}

	var nRoles, nAttacks, nProg int
	db.QueryRow(`SELECT COUNT(*) FROM summon_tribe_roles WHERE tribe_slug='summon/bear'`).Scan(&nRoles)
	db.QueryRow(`SELECT COUNT(*) FROM summon_tribe_attacks WHERE tribe_slug='summon/bear'`).Scan(&nAttacks)
	db.QueryRow(`SELECT COUNT(*) FROM summon_tribe_progression WHERE tribe_slug='summon/bear'`).Scan(&nProg)
	if nRoles != 2 || nAttacks != 1 || nProg != 5 {
		t.Errorf("detail rows: roles=%d attacks=%d prog=%d", nRoles, nAttacks, nProg)
	}

	var featRank, featDesc string
	if err := db.QueryRow(`SELECT rank, description FROM summon_tribe_features
		WHERE slug = 'summon/bear/feature/kuma-king'`).Scan(&featRank, &featDesc); err != nil {
		t.Fatal(err)
	}
	if featRank != "S" || featDesc != "Immune to Physical Conditions." {
		t.Errorf("feature row: rank=%s desc=%q", featRank, featDesc)
	}
}

func TestLoadSummonTribesOverridesAndDemote(t *testing.T) {
	db := testDB(t)
	tribes := []parse.SummonTribe{sampleTribe()}
	if _, err := LoadSummonTribes(db, jutsuBook2, tribes); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE summon_tribes SET toughness_override = 12,
		detection_status = 'verified' WHERE slug = 'summon/bear'`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSummonTribes(db, jutsuBook2, tribes); err != nil {
		t.Fatal(err)
	}
	var status string
	var override int
	if err := db.QueryRow(`SELECT detection_status, toughness_override FROM summon_tribes
		WHERE slug = 'summon/bear'`).Scan(&status, &override); err != nil {
		t.Fatal(err)
	}
	if status != "verified" || override != 12 {
		t.Errorf("after no-op reload: status=%s override=%d", status, override)
	}

	changed := []parse.SummonTribe{sampleTribe()}
	changed[0].Toughness = 11
	r, err := LoadSummonTribes(db, jutsuBook2, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("demoted = %v", r.Demoted)
	}
	if err := db.QueryRow(`SELECT detection_status, toughness_override FROM summon_tribes
		WHERE slug = 'summon/bear'`).Scan(&status, &override); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" || override != 12 {
		t.Errorf("after content change: status=%s override=%d", status, override)
	}
}
