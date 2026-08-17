package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

// coreBook is the shared fixture for the core rulebook's loaders (stances,
// feats, backgrounds, seals).
var coreBook = SourceBook{
	Slug: "book/core", Title: "Naruto 5e Full Document",
	Version: "3.11", FileName: "core.pdf", FileSHA256: "beef",
}

func sampleSeal() parse.EnhancementSeal {
	return parse.EnhancementSeal{
		Name: "Bane Seal", Tier: "Minor", Rank: "D", AppliesTo: "weapon",
		CostRyo: 350, Description: "It begins to vibrate.", SourcePage: 76,
	}
}

func TestLoadEnhancementSealsCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	seals := []parse.EnhancementSeal{sampleSeal()}
	r, err := LoadEnhancementSeals(db, coreBook, seals)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 1 {
		t.Errorf("first load: %+v", r)
	}
	r, err = LoadEnhancementSeals(db, coreBook, seals)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 1 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var name, kind, rank, appliesTo string
	var cost float64
	if err := db.QueryRow(`SELECT name, kind, seal_rank, seal_applies_to, cost_ryo
		FROM equipment WHERE slug = 'seal/weapon/minor/bane-seal'`).
		Scan(&name, &kind, &rank, &appliesTo, &cost); err != nil {
		t.Fatal(err)
	}
	if name != "Bane Seal (Minor)" || kind != "enhancement_seal" || rank != "D" ||
		appliesTo != "weapon" || cost != 350 {
		t.Errorf("bane seal row: name=%q kind=%q rank=%q applies=%q cost=%v",
			name, kind, rank, appliesTo, cost)
	}
}

func TestLoadEnhancementSealsSameNameDifferentTierNoCollision(t *testing.T) {
	db := testDB(t)
	seals := []parse.EnhancementSeal{
		sampleSeal(),
		{Name: "Bane Seal", Tier: "Refined", Rank: "C", AppliesTo: "weapon",
			CostRyo: 800, Description: "A stronger vibration.", SourcePage: 77},
	}
	r, err := LoadEnhancementSeals(db, coreBook, seals)
	if err != nil {
		t.Fatal(err)
	}
	// Same name, different tier/rank — distinct slugs, no duplicate flagged.
	if r.Created != 2 || len(r.Duplicates) != 0 {
		t.Errorf("load: %+v", r)
	}
}

func TestLoadEnhancementSealsDemoteOnChange(t *testing.T) {
	db := testDB(t)
	seals := []parse.EnhancementSeal{sampleSeal()}
	if _, err := LoadEnhancementSeals(db, coreBook, seals); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE equipment SET detection_status = 'verified'
		WHERE slug = 'seal/weapon/minor/bane-seal'`); err != nil {
		t.Fatal(err)
	}
	changed := []parse.EnhancementSeal{sampleSeal()}
	changed[0].CostRyo = 400
	r, err := LoadEnhancementSeals(db, coreBook, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("demoted = %v", r.Demoted)
	}
	var status string
	if err := db.QueryRow(`SELECT detection_status FROM equipment
		WHERE slug = 'seal/weapon/minor/bane-seal'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" {
		t.Errorf("status after change = %s", status)
	}
}
