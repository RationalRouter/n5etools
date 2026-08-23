package schema

import (
	"database/sql"
	"testing"
)

// TestMigration0076PackToolkitChoiceDataCleanup seeds the exact row shapes
// migration 0076 was written against — two characters whose Infiltrator's
// Pack toolkit pick never landed a tool proficiency (one still an inert
// custom row, one already a real item), an unsplit background equipment
// sentence, and a Crafter's Pack toolkit pick with no resolvable signal for
// which toolkits to grant — and confirms the migration turns each into the
// same shape the pack-unpack mechanism (cmd/n5e/pack_toolkit_choice.go,
// internal/charstore/sheet.go's ResolvePackToolkitChoice) produces when a
// pack is unpacked and resolved correctly today.
func TestMigration0076PackToolkitChoiceDataCleanup(t *testing.T) {
	db := openMemDB(t)
	if err := Apply(db, Characters); err != nil {
		t.Fatalf("applying character migrations: %v", err)
	}

	seedMigration0076Fixtures(t, db)
	rerunMigration(t, db, "0076_pack_toolkit_choice_data_cleanup.sql")
	assertMigration0076Result(t, db)

	// A second raw application of the same migration body must change
	// nothing — every statement's guard must fail once the fix is in place.
	rerunMigration(t, db, "0076_pack_toolkit_choice_data_cleanup.sql")
	assertMigration0076Result(t, db)
}

func seedMigration0076Fixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO characters (id, name) VALUES (2, 'Sasuke'), (5, 'Hon'), (10, 'Konnichiwa')`)

	// Character 5: Infiltrator's Pack toolkit pick left as a renamed but
	// still-custom row — the player's own rename to "Hackers Kit" is the
	// only signal of intent, and no real item or proficiency exists yet.
	mustExec(t, db, `INSERT INTO custom_items (slug, name) VALUES
		('custom/1-hackers-kit-or-security-kit-(pick-one)-13', 'Hackers Kit')`)
	mustExec(t, db, `INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (5, 'custom/1-hackers-kit-or-security-kit-(pick-one)-13', 1, 0)`)

	// Character 2: same pack, worked around by adding the real item
	// directly — the proficiency half is still missing.
	mustExec(t, db, `INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (2, 'toolkit/hackers-kit', 1, 1)`)

	// Character 2: background/urchin's whole equipment sentence stored as
	// one unsplit free-text row.
	mustExec(t, db, `INSERT INTO custom_items (slug, name) VALUES
		('custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13',
		 'A Map of the Village you grew up in, A Token of intimate value to you, Set of Basic Clothing, Wallet containing 100 Ryo')`)
	mustExec(t, db, `INSERT INTO character_inventory (character_id, item_slug, quantity, notes)
		VALUES (2,
		 'custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13',
		 1, 'creation-equipment')`)

	// Character 10: Crafter's Pack's "3 Toolkits (pick three)", never
	// resolved and with no signal for which three toolkits to grant.
	mustExec(t, db, `INSERT INTO custom_items (slug, name) VALUES
		('custom/3-toolkits-(pick-three)-29', '3 Toolkits (pick three)')`)
	mustExec(t, db, `INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (10, 'custom/3-toolkits-(pick-three)-29', 3, 0)`)
}

func assertMigration0076Result(t *testing.T, db *sql.DB) {
	t.Helper()

	// Character 5: the renamed custom row now points at the real item, and
	// carries the tool proficiency the original pick never granted.
	var slug string
	if err := db.QueryRow(`SELECT item_slug FROM character_inventory WHERE character_id = 5`).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "toolkit/hackers-kit" {
		t.Errorf("character 5 inventory item_slug = %q, want toolkit/hackers-kit", slug)
	}
	assertToolProficiency(t, db, 5, "Hackers Kit", "pack", "toolkit/hackers-kit")

	// Character 2: the already-real item now has its matching proficiency.
	assertToolProficiency(t, db, 2, "Hackers Kit", "pack", "toolkit/hackers-kit")

	// Character 2: the unsplit background sentence is now four lines — two
	// flavor lines with no item_slug match, plus real Ordinary Clothing and
	// a real Wallet.
	var flavorLines int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM character_inventory ci
		JOIN custom_items c ON c.slug = ci.item_slug
		WHERE ci.character_id = 2 AND c.name IN
			('A Map of the Village you grew up in', 'A Token of intimate value to you')
	`).Scan(&flavorLines); err != nil {
		t.Fatal(err)
	}
	if flavorLines != 2 {
		t.Errorf("character 2 flavor lines = %d, want 2 (Map + Token, split apart)", flavorLines)
	}
	for _, want := range []string{"gear/ordinary-clothing", "gear/wallet"} {
		var qty int
		err := db.QueryRow(`SELECT quantity FROM character_inventory WHERE character_id = 2 AND item_slug = ?`, want).Scan(&qty)
		if err == sql.ErrNoRows {
			t.Errorf("character 2 missing real item %s", want)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if qty != 1 {
			t.Errorf("character 2 %s quantity = %d, want 1", want, qty)
		}
	}

	// Character 10: the placeholder is now a live pending pack-toolkit
	// choice (same notes tag charstore.PackToolkitChoiceNotesTag/
	// UnpackInventoryItem write for an unresolved "any toolkit" pick),
	// still offering all 3 picks.
	var notes sql.NullString
	var quantity int
	if err := db.QueryRow(
		`SELECT notes, quantity FROM character_inventory WHERE character_id = 10`,
	).Scan(&notes, &quantity); err != nil {
		t.Fatal(err)
	}
	if !notes.Valid || notes.String != "pack-toolkit-choice" {
		t.Errorf("character 10 notes = %v, want \"pack-toolkit-choice\"", notes)
	}
	if quantity != 3 {
		t.Errorf("character 10 quantity = %d, want 3 (all picks still open)", quantity)
	}
}

func assertToolProficiency(t *testing.T, db *sql.DB, characterID int64, value, sourceKind, sourceRef string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM character_proficiencies
		WHERE character_id = ? AND kind = 'tool' AND value = ? AND source_kind = ? AND source_ref = ?`,
		characterID, value, sourceKind, sourceRef,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("character %d %q tool proficiency rows = %d, want 1", characterID, value, count)
	}
}

// rerunMigration re-executes one already-applied migration's SQL body by
// clearing its own schema_migrations tracking row and calling Apply again —
// the same tx.Exec(body) path production uses, just re-triggered against
// data seeded to look like a pre-migration database instead of empty. Only
// safe for a migration whose own guards make it idempotent, which is
// exactly what this file's tests are checking.
func rerunMigration(t *testing.T, db *sql.DB, filename string) {
	t.Helper()
	mustExec(t, db, `DELETE FROM schema_migrations WHERE filename = ?`, filename)
	if err := Apply(db, Characters); err != nil {
		t.Fatalf("re-applying %s: %v", filename, err)
	}
}
