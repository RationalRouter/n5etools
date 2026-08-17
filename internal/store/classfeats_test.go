package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

func TestLoadClassFeats(t *testing.T) {
	db := testDB(t)
	// The class rows must exist for linking.
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}

	feats := []parse.Feat{
		{Name: "Scouting Training", Category: "Archetype", ClassName: "Scout-Nin",
			Prerequisites: "Level 5+", Description: "You train as a scout.", SourcePage: 260},
		{Name: "Witches Training", Category: "Archetype", ClassName: "Witch",
			Description: "No such class exists in the books.", SourcePage: 261},
	}
	r, err := LoadClassFeats(db, classBook, feats)
	if err != nil {
		t.Fatal(err)
	}
	// Both load; the unknown class is flagged, not dropped.
	if r.Created != 2 || len(r.Skipped) != 1 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadClassFeats(db, classBook, feats)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 2 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var classRef any
	if err := db.QueryRow(`SELECT class_slug FROM feats
		WHERE slug = 'feat/class/scouting-training'`).Scan(&classRef); err != nil {
		t.Fatal(err)
	}
	if classRef != "class/scout-nin" {
		t.Errorf("linked feat class_slug = %v", classRef)
	}
	if err := db.QueryRow(`SELECT class_slug FROM feats
		WHERE slug = 'feat/class/witches-training'`).Scan(&classRef); err != nil {
		t.Fatal(err)
	}
	if classRef != nil {
		t.Errorf("unknown-class feat class_slug = %v, want NULL", classRef)
	}
}
