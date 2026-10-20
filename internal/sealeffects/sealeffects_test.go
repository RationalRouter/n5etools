package sealeffects

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/RationalRouter/n5e/internal/schema"
	_ "modernc.org/sqlite"
)

func openDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestEverySealClassified fails when the built rules.db holds a seal that is
// neither auto nor reference, so a newly ingested seal can't silently fall
// through, and when a table entry names no real seal.
func TestEverySealClassified(t *testing.T) {
	src, err := os.Open(filepath.Join("..", "..", "out", "rules.db"))
	if err != nil {
		t.Skipf("no built rules.db: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rules.db")
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	src.Close()
	dst.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := schema.Apply(db, schema.Rules); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`SELECT slug FROM equipment WHERE kind = 'enhancement_seal'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	autoSeen := map[string]bool{}
	familySeen := map[string]bool{}
	n := 0
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			t.Fatal(err)
		}
		n++
		if _, _, known := Classify(slug); !known {
			t.Errorf("seal %s is neither auto nor reference", slug)
		}
		applies, tier, family, _ := parts(slug)
		autoSeen[applies+"/"+tier+"/"+family] = true
		familySeen[applies+"/"+family] = true
	}
	if n < 140 {
		t.Fatalf("only %d seals in rules.db", n)
	}
	for k := range auto {
		if !autoSeen[k] {
			t.Errorf("auto entry %s names no seal", k)
		}
	}
	for k := range reference {
		if !familySeen[k] {
			t.Errorf("reference entry %s names no seal family", k)
		}
	}
}

// TestLoad totals equipped crafted items: per-family non-stacking, the
// level gate, seals on the wrong kind of base, and unequipped items.
func TestLoad(t *testing.T) {
	rulesDB := openDB(t, "rules.db")
	charDB := openDB(t, "characters.db")
	if err := schema.Apply(rulesDB, schema.Rules); err != nil {
		t.Fatal(err)
	}
	if err := schema.Apply(charDB, schema.Characters); err != nil {
		t.Fatal(err)
	}
	if _, err := rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, seal_rank) VALUES
		('weapon/katana', 'Katana', 'weapon', NULL),
		('armor/vest', 'Vest', 'armor', NULL),
		('seal/weapon/minor/martial-seal', 'Martial Seal (Minor)', 'enhancement_seal', 'D'),
		('seal/weapon/greater/martial-seal', 'Martial Seal (Greater)', 'enhancement_seal', 'B'),
		('seal/weapon/minor/extending-seal', 'Extending Seal (Minor)', 'enhancement_seal', 'D'),
		('seal/weapon/minor/bane-seal', 'Bane Seal (Minor)', 'enhancement_seal', 'D'),
		('seal/armor/refined/swift-seal', 'Swift Seal (Refined)', 'enhancement_seal', 'C'),
		('seal/armor/refined/dexterity-seal', 'Dexterity Seal (Refined)', 'enhancement_seal', 'C'),
		('seal/armor/greater/elemental-seal', 'Elemental Seal (Greater)', 'enhancement_seal', 'B')`); err != nil {
		t.Fatal(err)
	}
	if _, err := charDB.Exec(`
		INSERT INTO characters (id, name) VALUES (1, 'Tester');
		INSERT INTO crafted_items (id, slug, name, base_slug, quality) VALUES
			(1, 'crafted/a-1', 'Blade A', 'weapon/katana', 'standard'),
			(2, 'crafted/b-2', 'Blade B', 'weapon/katana', 'standard'),
			(3, 'crafted/c-3', 'Vest C', 'armor/vest', 'standard'),
			(4, 'crafted/d-4', 'Blade D', 'weapon/katana', 'standard'),
			(5, 'crafted/e-5', 'Vest E', 'armor/vest', 'standard');
		INSERT INTO crafted_item_seals (crafted_item_id, seal_slug, sort_order) VALUES
			(1, 'seal/weapon/minor/martial-seal', 0),
			(1, 'seal/weapon/minor/extending-seal', 1),
			(1, 'seal/weapon/minor/bane-seal', 2),
			(2, 'seal/weapon/minor/martial-seal', 0),
			(3, 'seal/armor/refined/swift-seal', 0),
			(3, 'seal/armor/refined/dexterity-seal', 1),
			(3, 'seal/weapon/minor/martial-seal', 2),
			(4, 'seal/weapon/greater/martial-seal', 0),
			(5, 'seal/armor/greater/elemental-seal', 0);
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES
			(1, 'crafted/a-1', 1, 1),
			(1, 'crafted/b-2', 1, 1),
			(1, 'crafted/c-3', 1, 1),
			(1, 'crafted/d-4', 1, 1),
			(1, 'crafted/e-5', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	tot, err := Load(rulesDB, charDB, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	// Two Minor Martial weapons give +1, not +2; the level-9 Greater one is
	// gated at level 5; the Martial seal on the vest is ignored.
	if got := tot.JutsuAttackBonus("Taijutsu"); got != 1 {
		t.Errorf("Taijutsu = %d, want 1", got)
	}
	if tot.Speed != 10 || tot.Saves["dex"] != 1 || tot.Checks["dex"] != 1 || tot.AC != 0 {
		t.Errorf("totals = %+v", tot.Effect)
	}
	if len(tot.Items) != 4 {
		t.Fatalf("items = %+v", tot.Items)
	}
	a, d := tot.Items[0], tot.Items[3]
	if a.MeleeRange != 5 || a.RangedRange != 20 || a.Rank != "D" || len(a.Seals) != 3 || a.Seals[2].Auto {
		t.Errorf("blade A = %+v", a)
	}
	if !d.Gated || d.MinLevel != 9 || d.Rank != "B" {
		t.Errorf("blade D = %+v", d)
	}

	tot, err = Load(rulesDB, charDB, 1, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got := tot.JutsuAttackBonus("Taijutsu"); got != 2 {
		t.Errorf("level 9 Taijutsu = %d, want 2", got)
	}
}
