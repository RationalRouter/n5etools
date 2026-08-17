package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

func sampleClan() parse.Clan {
	speed := 30
	level1 := 1
	j := sampleJutsu()
	j.Name = "Human Cocoon"
	j.CategoryGroup = "Clan: Aburame"
	return parse.Clan{
		Name: "Aburame Clan", Epithet: "Creepy Crawly",
		Overview: "One of the four noble clans.", SourcePage: 6,
		AbilityRaw:       "+2 Intelligence, +1 Wisdom",
		AbilityIncreases: []parse.AbilityIncrease{{Ability: "int", Amount: 2}, {Ability: "wis", Amount: 1}},
		SpeedText:        "Your base walking speed is 30 feet", SpeedFeet: &speed,
		SkillProfs: []string{"Nature", "Animal Handling"},
		Traits:     []parse.ClanTrait{{Name: "Parasitic Technique", Description: "You know 1 additional jutsu."}},
		Features:   []parse.ClanFeature{{Name: "Bug Host", Level: &level1, Description: "Beginning at 1st level...", SourcePage: 6}},
		Jutsu:      []parse.Jutsu{j},
		Feats:      []parse.Feat{{Name: "Hive Minded", Category: "Clan", Prerequisites: "Aburame Clan, Level 8+", Description: "Benefits.", SourcePage: 9}},
	}
}

var clanBook = SourceBook{
	Slug: "book/clan-compendium", Title: "Tsunade's Studies Compendium",
	Version: "3.11", FileName: "test.pdf", FileSHA256: "def",
}

func TestLoadClanBookCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	batch := []parse.Clan{sampleClan()}

	r, err := LoadClanBook(db, clanBook, batch, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 1 clan + 1 feature + 1 jutsu + 1 feat
	if r.Created != 4 || r.Updated != 0 {
		t.Errorf("first load: %+v", r)
	}

	r, err = LoadClanBook(db, clanBook, batch, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Updated != 0 || r.Unchanged != 4 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	// Clan jutsu are slug-scoped under the clan and linked in the junction.
	var linked int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM clan_jutsu
		WHERE clan_slug = 'clan/aburame' AND jutsu_slug = 'jutsu/aburame/human-cocoon'`,
	).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked != 1 {
		t.Errorf("clan_jutsu link missing")
	}
	var asi int
	if err := db.QueryRow(`SELECT COUNT(*) FROM clan_ability_increases WHERE clan_slug='clan/aburame'`).Scan(&asi); err != nil {
		t.Fatal(err)
	}
	if asi != 2 {
		t.Errorf("ability increases = %d, want 2", asi)
	}
}

func TestLoadClanBookPreservesOverridesAndDemotes(t *testing.T) {
	db := testDB(t)
	if _, err := LoadClanBook(db, clanBook, []parse.Clan{sampleClan()}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE clans SET speed_feet_override = 35, detection_status = 'verified',
		                 notes = 'confirmed with maintainer'
		WHERE slug = 'clan/aburame'`); err != nil {
		t.Fatal(err)
	}

	// Unchanged re-ingest: everything human survives.
	if _, err := LoadClanBook(db, clanBook, []parse.Clan{sampleClan()}, nil); err != nil {
		t.Fatal(err)
	}
	var status, notes string
	var override int
	if err := db.QueryRow(`SELECT detection_status, speed_feet_override, notes FROM clans WHERE slug='clan/aburame'`).
		Scan(&status, &override, &notes); err != nil {
		t.Fatal(err)
	}
	if status != "verified" || override != 35 || notes != "confirmed with maintainer" {
		t.Errorf("re-ingest disturbed human data: %q %d %q", status, override, notes)
	}
	// The override wins in the effective view.
	var speed int
	if err := db.QueryRow(`SELECT speed_feet FROM v_clans WHERE slug='clan/aburame'`).Scan(&speed); err != nil {
		t.Fatal(err)
	}
	if speed != 35 {
		t.Errorf("v_clans speed = %d, want override 35", speed)
	}

	// Changed content: demote to needs_review, override untouched.
	changed := sampleClan()
	changed.Overview = "Rewritten overview in a new book version."
	r, err := LoadClanBook(db, clanBook, []parse.Clan{changed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 || r.Demoted[0] != "clan/aburame" {
		t.Errorf("Demoted = %v", r.Demoted)
	}
	if err := db.QueryRow(`SELECT detection_status, speed_feet_override FROM clans WHERE slug='clan/aburame'`).
		Scan(&status, &override); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" || override != 35 {
		t.Errorf("demote wrong: %q %d", status, override)
	}
}
