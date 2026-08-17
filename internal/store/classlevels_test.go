package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
	"github.com/sergio/n5e/internal/sheet"
)

var mastersheet = SourceBook{
	Slug: "sheet/mastersheet", Title: "N5E Community Mastersheet",
	Version: "3.1", FileName: "mastersheet.xlsx", FileSHA256: "feed",
}

func sampleChart() sheet.ClassChart {
	known := 6
	c := sheet.ClassChart{Name: "Scout-Nin", HitDie: 8, ChakraDie: 10}
	for lvl := 1; lvl <= 20; lvl++ {
		lr := sheet.LevelRow{Level: lvl, ProfBonus: 3 + (lvl-1)/3, JutsuKnown: &known}
		if lvl >= 3 {
			lr.Resources = []sheet.Resource{{Name: "Superiority Dice", Value: "1d6"}}
		}
		c.Levels = append(c.Levels, lr)
	}
	return c
}

func TestLoadClassLevels(t *testing.T) {
	db := testDB(t)
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}

	charts := []sheet.ClassChart{sampleChart(), {Name: "Nowhere", HitDie: 8, ChakraDie: 8}}
	r, err := LoadClassLevels(db, mastersheet, charts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 20 || len(r.Skipped) != 1 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadClassLevels(db, mastersheet, charts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 20 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var prof, known int
	if err := db.QueryRow(`SELECT proficiency_bonus, jutsu_known FROM class_levels
		WHERE class_slug='class/scout-nin' AND level=20`).Scan(&prof, &known); err != nil {
		t.Fatal(err)
	}
	if prof != 9 || known != 6 {
		t.Errorf("L20 = prof %d known %d", prof, known)
	}
	var nRes int
	db.QueryRow(`SELECT COUNT(*) FROM class_level_resources
		WHERE class_slug='class/scout-nin'`).Scan(&nRes)
	if nRes != 18 {
		t.Errorf("resource rows = %d, want 18", nRes)
	}

	// Override + demote round trip on a level row.
	if _, err := db.Exec(`UPDATE class_levels
		SET jutsu_known_override = 7, detection_status = 'verified'
		WHERE class_slug='class/scout-nin' AND level=1`); err != nil {
		t.Fatal(err)
	}
	changed := []sheet.ClassChart{sampleChart()}
	changed[0].Levels[0].ProfBonus = 4
	r, err = LoadClassLevels(db, mastersheet, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("demoted = %v", r.Demoted)
	}
	var status string
	var override int
	if err := db.QueryRow(`SELECT detection_status, jutsu_known_override FROM class_levels
		WHERE class_slug='class/scout-nin' AND level=1`).Scan(&status, &override); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" || override != 7 {
		t.Errorf("after change: status=%s override=%d", status, override)
	}
}
