package store

import (
	"testing"

	"github.com/sergio/n5e/internal/classcharts"
	"github.com/sergio/n5e/internal/parse"
)

var curated = SourceBook{
	Slug: "curated/class-charts", Title: "N5E Toolkit curated charts",
	Version: "1.0", FileName: "internal/classcharts",
}

func sampleChart() classcharts.Class {
	known := 6
	c := classcharts.Class{Slug: "class/scout-nin"}
	for lvl := 1; lvl <= 20; lvl++ {
		prof := 3 + (lvl-1)/3
		lr := classcharts.Level{Level: lvl, ProficiencyBonus: &prof, JutsuKnown: &known}
		if lvl >= 3 {
			lr.Resources = []classcharts.Resource{{Name: "Superiority Dice", Value: "1d6", Status: "auto"}}
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

	charts := []classcharts.Class{sampleChart(), {Slug: "class/nowhere"}}
	r, err := LoadClassLevels(db, curated, charts)
	if err != nil {
		t.Fatal(err)
	}
	// 20 level rows + 18 charted resource rows (levels 3-20).
	if r.Created != 38 || len(r.Skipped) != 1 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadClassLevels(db, curated, charts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 38 {
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
	changed := []classcharts.Class{sampleChart()}
	newProf := 4
	changed[0].Levels[0].ProficiencyBonus = &newProf
	r, err = LoadClassLevels(db, curated, changed)
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

// The Mastersheet has real column gaps filled in by hand as 'manual' rows,
// and v_class_level_resources prefers value_override over value. Neither may
// be touched by a re-ingest, and a row the chart never carries must be
// reported as vanished rather than deleted.
func TestLoadClassLevelsPreservesManualResources(t *testing.T) {
	db := testDB(t)
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}
	charts := []classcharts.Class{sampleChart()}
	if _, err := LoadClassLevels(db, curated, charts); err != nil {
		t.Fatal(err)
	}

	// A hand-added row for a column the sheet does not carry at all.
	if _, err := db.Exec(`INSERT INTO class_level_resources
		(class_slug, level, resource_name, value, detection_status)
		VALUES ('class/scout-nin', 5, 'Wood Tier', '3', 'manual')`); err != nil {
		t.Fatal(err)
	}
	// An override on a row the sheet DOES carry.
	if _, err := db.Exec(`UPDATE class_level_resources
		SET value_override = '9d9', detection_status = 'verified'
		WHERE class_slug='class/scout-nin' AND level=3 AND resource_name='Superiority Dice'`); err != nil {
		t.Fatal(err)
	}

	r, err := LoadClassLevels(db, curated, charts)
	if err != nil {
		t.Fatal(err)
	}

	var value, status string
	if err := db.QueryRow(`SELECT value, detection_status FROM class_level_resources
		WHERE class_slug='class/scout-nin' AND level=5 AND resource_name='Wood Tier'`).
		Scan(&value, &status); err != nil {
		t.Fatalf("manual row destroyed by re-ingest: %v", err)
	}
	if value != "3" || status != "manual" {
		t.Errorf("manual row = %q/%s, want 3/manual", value, status)
	}

	var override string
	if err := db.QueryRow(`SELECT value_override, detection_status FROM class_level_resources
		WHERE class_slug='class/scout-nin' AND level=3 AND resource_name='Superiority Dice'`).
		Scan(&override, &status); err != nil {
		t.Fatal(err)
	}
	if override != "9d9" || status != "verified" {
		t.Errorf("override row = %q/%s, want 9d9/verified", override, status)
	}

	// The hand-added row is not in the chart, so it reports as vanished.
	if len(r.Vanished) != 1 || r.Vanished[0] != "class/scout-nin/level/5/Wood Tier" {
		t.Errorf("vanished = %v", r.Vanished)
	}
}
