package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

var classBook = SourceBook{
	Slug: "book/class-compendium", Title: "Orochimaru's Observation Compendium",
	Version: "3.12", FileName: "classes.pdf", FileSHA256: "cafe",
}

func lvl(n int) *int { return &n }

func sampleClass() parse.Class {
	return parse.Class{
		Name: "Scout-Nin", SourcePage: 90, HitDie: 8, ChakraDie: 10,
		Description: "A fast shinobi.", QuickBuild: "Dex first.",
		Proficiencies: []parse.ClassProficiency{
			{Kind: "armor", Value: "Light armor"},
			{Kind: "skill_choice", Value: "Stealth", ChooseN: 3},
		},
		Equipment: []string{"1 Simple weapon"},
		Casting:   []parse.ClassCasting{{Discipline: "ninjutsu", Ability: "int"}},
		Features: []parse.ClassFeature{
			{Name: "Fighting Stance", Level: lvl(1), Description: "Pick a stance.", SourcePage: 90},
		},
		Group: &parse.SubclassGroup{
			DisplayName: "Scouting Technique", SelectionLevels: []int{3, 6, 9},
			Description: "Choose a technique.", SourcePage: 91,
			Subclasses: []parse.Subclass{{
				Name: "Arbiter Scout", Description: "Directs the battle.", SourcePage: 91,
				Features: []parse.ClassFeature{
					{Name: "Superior Arbitration", Level: lvl(3), Description: "Gain dice.", SourcePage: 91},
				},
			}},
		},
		OptionLists: []parse.OptionList{{
			Name: "Arbiter Maneuvers", SubclassName: "Arbiter Scout", SourcePage: 92,
			Options: []parse.ClassOption{
				{Name: "Assisted Accuracy", Description: "Spend a die.", SourcePage: 92},
			},
		}},
	}
}

func TestLoadClassBookCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	r, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// class + feature + group + subclass + subclass feature + option = 6.
	if r.Created != 6 || len(r.Duplicates) != 0 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Updated != 0 || r.Unchanged != 6 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var sel string
	if err := db.QueryRow(`SELECT selection_levels FROM subclass_groups
		WHERE slug = 'class/scout-nin/group/scouting-technique'`).Scan(&sel); err != nil {
		t.Fatal(err)
	}
	if sel != "3,6,9" {
		t.Errorf("selection_levels = %q", sel)
	}
	var subclassRef string
	if err := db.QueryRow(`SELECT subclass_slug FROM class_options
		WHERE slug = 'class/scout-nin/option/arbiter-maneuvers/assisted-accuracy'`).Scan(&subclassRef); err != nil {
		t.Fatal(err)
	}
	if subclassRef != "class/scout-nin/group/scouting-technique/arbiter-scout" {
		t.Errorf("option subclass_slug = %q", subclassRef)
	}
	var nCast, nProf int
	db.QueryRow(`SELECT COUNT(*) FROM class_casting WHERE class_slug='class/scout-nin'`).Scan(&nCast)
	db.QueryRow(`SELECT COUNT(*) FROM class_proficiencies WHERE class_slug='class/scout-nin'`).Scan(&nProf)
	if nCast != 1 || nProf != 2 {
		t.Errorf("detail rows: casting=%d profs=%d", nCast, nProf)
	}
}

func TestLoadClassBookOverridesAndDemote(t *testing.T) {
	db := testDB(t)
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}
	// A human corrects the feature's level and blesses the row.
	if _, err := db.Exec(`UPDATE class_features
		SET level_override = 2, detection_status = 'verified'
		WHERE slug = 'class/scout-nin/feature/fighting-stance'`); err != nil {
		t.Fatal(err)
	}

	// Unchanged re-run: override and status survive.
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}
	var status string
	var override int
	if err := db.QueryRow(`SELECT detection_status, level_override FROM class_features
		WHERE slug = 'class/scout-nin/feature/fighting-stance'`).Scan(&status, &override); err != nil {
		t.Fatal(err)
	}
	if status != "verified" || override != 2 {
		t.Errorf("after no-op reload: status=%s override=%d", status, override)
	}

	// Content change in print: parsed column updates, status demotes,
	// override untouched.
	changed := sampleClass()
	changed.Features[0].Description = "Pick a stance from the core book."
	r, err := LoadClassBook(db, classBook, []parse.Class{changed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("demoted = %v", r.Demoted)
	}
	if err := db.QueryRow(`SELECT detection_status, level_override FROM class_features
		WHERE slug = 'class/scout-nin/feature/fighting-stance'`).Scan(&status, &override); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" || override != 2 {
		t.Errorf("after content change: status=%s override=%d", status, override)
	}
}

func TestLoadClassBookDuplicateNamesSuffixed(t *testing.T) {
	db := testDB(t)
	c := sampleClass()
	c.Group.Subclasses[0].Features = append(c.Group.Subclasses[0].Features,
		parse.ClassFeature{Name: "Superior Arbitration", Level: lvl(9),
			Description: "The dice grow.", SourcePage: 93})
	r, err := LoadClassBook(db, classBook, []parse.Class{c}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Duplicates) != 1 {
		t.Fatalf("duplicates = %v", r.Duplicates)
	}
	var desc string
	if err := db.QueryRow(`SELECT description FROM subclass_features
		WHERE slug = 'class/scout-nin/group/scouting-technique/arbiter-scout/feature/superior-arbitration-2'`).Scan(&desc); err != nil {
		t.Fatal(err)
	}
	if desc != "The dice grow." {
		t.Errorf("suffixed row description = %q", desc)
	}
	// Suffixes are deterministic: a re-run is a no-op, nothing vanishes.
	r, err = LoadClassBook(db, classBook, []parse.Class{c}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || len(r.Vanished) != 0 {
		t.Errorf("re-run: %+v", r)
	}
}
