package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

func sampleMulticlassRules() []parse.MulticlassRule {
	return []parse.MulticlassRule{
		{ClassName: "Scout-Nin", AbilityPrereq: "Strength 13 & Intelligence 13 & Wisdom 13",
			ProficienciesGained: "Light and Medium armor, one skill", JutsuPerLevel: "+1 Every 2 Levels.",
			SourcePage: 179},
		{ClassName: "Nowhere", AbilityPrereq: "Charisma 99",
			ProficienciesGained: "Nothing", JutsuPerLevel: "+1 Every Level.", SourcePage: 179},
	}
}

func TestLoadMulticlassRulesCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}
	rules := sampleMulticlassRules()
	r, err := LoadMulticlassRules(db, coreBook, rules)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 1 || len(r.Skipped) != 1 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadMulticlassRules(db, coreBook, rules)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 1 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var ability string
	if err := db.QueryRow(`SELECT ability_prereq_text FROM class_multiclass_rules
		WHERE class_slug = 'class/scout-nin'`).Scan(&ability); err != nil {
		t.Fatal(err)
	}
	if ability != "Strength 13 & Intelligence 13 & Wisdom 13" {
		t.Errorf("ability text = %q", ability)
	}
}

func TestLoadMulticlassRulesDemoteOnChange(t *testing.T) {
	db := testDB(t)
	if _, err := LoadClassBook(db, classBook, []parse.Class{sampleClass()}, nil); err != nil {
		t.Fatal(err)
	}
	rules := sampleMulticlassRules()
	if _, err := LoadMulticlassRules(db, coreBook, rules); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE class_multiclass_rules SET detection_status = 'verified'
		WHERE class_slug = 'class/scout-nin'`); err != nil {
		t.Fatal(err)
	}
	changed := sampleMulticlassRules()
	changed[0].JutsuPerLevel = "+1 Every 3 Levels."
	r, err := LoadMulticlassRules(db, coreBook, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("demoted = %v", r.Demoted)
	}
	var status string
	if err := db.QueryRow(`SELECT detection_status FROM class_multiclass_rules
		WHERE class_slug = 'class/scout-nin'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" {
		t.Errorf("status after change = %s", status)
	}
}
