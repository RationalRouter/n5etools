package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

var bingoBook = SourceBook{
	Slug: "book/bingo-book", Title: "Bingo Book Pack 1 — Adversaries",
	Version: "1", FileName: "test.pdf", FileSHA256: "bingo",
}

func sampleAdversaryRole() parse.AdversaryRole {
	return parse.AdversaryRole{Name: "Striker", Description: "A Striker exists to deal damage.", SourcePage: 19}
}

func sampleAdversaryRoleTrait() parse.AdversaryRoleTrait {
	return parse.AdversaryRoleTrait{
		Name: "Aggressive", RoleName: "Striker", Rank: "D",
		Description: "If this Adversary triggers an attack of opportunity...", SourcePage: 20,
	}
}

func TestLoadAdversaryRolesCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	batch := []parse.AdversaryRole{sampleAdversaryRole()}

	r, err := LoadAdversaryRoles(db, bingoBook, batch)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 1 || r.Updated != 0 {
		t.Errorf("first load: %+v", r)
	}

	r, err = LoadAdversaryRoles(db, bingoBook, batch)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 1 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var slug string
	if err := db.QueryRow(`SELECT slug FROM adversary_roles WHERE name = 'Striker'`).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "adversary-role/striker" {
		t.Errorf("slug = %q, want adversary-role/striker", slug)
	}
}

func TestLoadAdversaryRoleTraitsCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	if _, err := LoadAdversaryRoles(db, bingoBook, []parse.AdversaryRole{sampleAdversaryRole()}); err != nil {
		t.Fatal(err)
	}
	batch := []parse.AdversaryRoleTrait{sampleAdversaryRoleTrait()}

	r, err := LoadAdversaryRoleTraits(db, bingoBook, batch)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 1 {
		t.Errorf("first load: %+v", r)
	}

	r, err = LoadAdversaryRoleTraits(db, bingoBook, batch)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 1 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var roleSlug, rank string
	if err := db.QueryRow(`SELECT role_slug, rank FROM adversary_role_traits WHERE name = 'Aggressive'`).
		Scan(&roleSlug, &rank); err != nil {
		t.Fatal(err)
	}
	if roleSlug != "adversary-role/striker" || rank != "D" {
		t.Errorf("role_slug=%q rank=%q, want adversary-role/striker, D", roleSlug, rank)
	}
}

// A trait's rank changing must demote a human-'verified' row back to
// needs_review, same override-preserving rule every other loader follows.
func TestLoadAdversaryRoleTraitsDemotesVerified(t *testing.T) {
	db := testDB(t)
	if _, err := LoadAdversaryRoles(db, bingoBook, []parse.AdversaryRole{sampleAdversaryRole()}); err != nil {
		t.Fatal(err)
	}
	trait := sampleAdversaryRoleTrait()
	if _, err := LoadAdversaryRoleTraits(db, bingoBook, []parse.AdversaryRoleTrait{trait}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE adversary_role_traits SET detection_status = 'verified', notes = 'checked'
		WHERE slug = 'adversary-role-trait/aggressive'`); err != nil {
		t.Fatal(err)
	}

	trait.Rank = "C"
	r, err := LoadAdversaryRoleTraits(db, bingoBook, []parse.AdversaryRoleTrait{trait})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("Demoted = %v, want the one changed slug", r.Demoted)
	}
	var status, notes, rank string
	if err := db.QueryRow(`SELECT detection_status, notes, rank FROM adversary_role_traits WHERE slug = 'adversary-role-trait/aggressive'`).
		Scan(&status, &notes, &rank); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" || notes != "checked" || rank != "C" {
		t.Errorf("status=%q notes=%q rank=%q", status, notes, rank)
	}
}

func TestLoadAdversaryRanksAndFreeformSlots(t *testing.T) {
	db := testDB(t)
	ranks := []parse.AdversaryRank{
		{Name: "Minion", HPFormula: "10 + Level", InitBonus: -2, Notes: "Minions automatically fail saving throws."},
		{Name: "Standard"},
		{Name: "Elite", HPFormula: "+1.25", ACBonus: 1, SaveBonus: 1, SaveDCBonus: 1, InitBonus: 2},
		{Name: "Solo", HPFormula: "+1.5 + 0.5 x Per Player", ACBonus: 2, SaveBonus: 2, SaveDCBonus: 2, InitBonus: 3},
	}
	slots := []parse.AdversaryFreeformSlot{
		{RankName: "Minion", LevelMin: 1, Slots: 1},
		{RankName: "Minion", LevelMin: 5, Slots: 2},
		{RankName: "Elite", LevelMin: 3, Slots: 3},
		{RankName: "Solo", LevelMin: 5, Slots: 3},
	}

	r, err := LoadAdversaryRanks(db, bingoBook, ranks, slots)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 4 {
		t.Errorf("Created = %d, want 4", r.Created)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM adversary_freeform_slots`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("adversary_freeform_slots rows = %d, want 4", n)
	}
	var eliteSlots int
	if err := db.QueryRow(`SELECT slots FROM adversary_freeform_slots WHERE rank_slug = 'adversary-rank/elite' AND level_min = 3`).
		Scan(&eliteSlots); err != nil {
		t.Fatal(err)
	}
	if eliteSlots != 3 {
		t.Errorf("elite level-3 slots = %d, want 3", eliteSlots)
	}

	// Freeform slots are a pure derived table: a second load with a
	// different set must fully replace the old rows, not merge with them.
	slots2 := []parse.AdversaryFreeformSlot{{RankName: "Minion", LevelMin: 1, Slots: 1}}
	if _, err := LoadAdversaryRanks(db, bingoBook, ranks, slots2); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM adversary_freeform_slots`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("after replace, adversary_freeform_slots rows = %d, want 1", n)
	}

	var hpFormula string
	var acBonus int
	if err := db.QueryRow(`SELECT hp_formula, ac_bonus FROM adversary_ranks WHERE slug = 'adversary-rank/standard'`).
		Scan(&hpFormula, &acBonus); err != nil {
		t.Fatal(err)
	}
	if hpFormula != "" || acBonus != 0 {
		t.Errorf("Standard rank should be the zero-bonus baseline: hp_formula=%q ac_bonus=%d", hpFormula, acBonus)
	}
}
