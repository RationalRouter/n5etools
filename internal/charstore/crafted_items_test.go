package charstore

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/RationalRouter/n5e/internal/crafting"
)

func TestCraftedItemLifecycle(t *testing.T) {
	db := testCharDB(t)

	cr := crafting.DefaultCrafting()
	cr.Participants, cr.HireSealsmith = 3, true
	it, err := AddCraftedItem(db, CraftedItem{Notes: "for Sakura", Recipe: crafting.Recipe{
		Name: "Storm Katana", BaseSlug: "weapon/katana", Quality: "greater", Crafting: cr,
		Seals: []crafting.SealPick{
			{Slug: "seal/weapon/minor/martial-seal"},
			{Slug: "seal/weapon/greater/elemental-seal", Choices: []string{"lightning"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if it.Slug != "crafted/storm-katana-1" {
		t.Errorf("slug = %q", it.Slug)
	}

	got, err := GetCraftedItemBySlug(db, it.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if got.Recipe.Name != "Storm Katana" || got.Recipe.Quality != "greater" || got.Notes != "for Sakura" ||
		got.Recipe.Crafting != cr || len(got.Recipe.Seals) != 2 ||
		!reflect.DeepEqual(got.Recipe.Seals[1].Choices, []string{"lightning"}) || got.Recipe.Seals[0].Choices != nil &&
		len(got.Recipe.Seals[0].Choices) != 0 {
		t.Errorf("round trip = %+v", got)
	}

	got.Recipe.Name = "Thunder Katana"
	got.Recipe.Seals = got.Recipe.Seals[1:]
	if err := UpdateCraftedItem(db, it.ID, got); err != nil {
		t.Fatal(err)
	}
	got, _ = GetCraftedItem(db, it.ID)
	if got.Slug != it.Slug || got.Recipe.Name != "Thunder Katana" || len(got.Recipe.Seals) != 1 {
		t.Errorf("after update = %+v", got)
	}

	dup, err := DuplicateCraftedItem(db, it.ID, "Thunder Katana II")
	if err != nil {
		t.Fatal(err)
	}
	if dup.Slug == it.Slug || len(dup.Recipe.Seals) != 1 {
		t.Errorf("duplicate = %+v", dup)
	}

	if err := SetCraftedItemArchived(db, it.ID, true); err != nil {
		t.Fatal(err)
	}
	active, _ := ListCraftedItems(db, false)
	archived, _ := ListCraftedItems(db, true)
	if len(active) != 1 || active[0].ID != dup.ID || len(archived) != 1 || len(archived[0].Recipe.Seals) != 1 {
		t.Errorf("active %+v archived %+v", active, archived)
	}

	if base, err := ResolveMechanicalSlug(db, it.Slug); err != nil || base != "weapon/katana" {
		t.Errorf("resolve crafted = %q, %v", base, err)
	}
	if base, _ := ResolveMechanicalSlug(db, "weapon/kunai"); base != "weapon/kunai" {
		t.Errorf("resolve plain = %q", base)
	}
	if base, err := ResolveMechanicalSlug(db, "crafted/gone-99"); err != nil || base != "" {
		t.Errorf("resolve missing = %q, %v", base, err)
	}
	if err := UpdateCraftedItem(db, 99, got); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("update missing = %v", err)
	}
}
