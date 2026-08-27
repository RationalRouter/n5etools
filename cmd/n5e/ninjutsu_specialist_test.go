package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// TestNinjutsuMasterAlwaysMaxDamage covers Ninjutsu Master (L20, base class):
// "select one Ninjutsu of C-Rank or lower. You always deal maximum damage
// with the chosen Jutsu." The pick itself (which jutsu) is already tracked
// via character_ninjutsu_jutsu_picks/NinjutsuPickMaster — this only covers
// the newly-added AlwaysMaxDamage annotation ninjutsuMasterAlwaysMaxDamageSlug
// resolves it to, surfaced on jutsuSheetRow for both the Known Jutsu list and
// the Attacks & Jutsu table (they share the same loadCharacterJutsuSheet
// rows).
func TestNinjutsuMasterAlwaysMaxDamage(t *testing.T) {
	s := testServer(t)
	for _, stmt := range []string{
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		 VALUES ('jutsu/water-bullet', 'Water Bullet', 'Ninjutsu', 'C', '1 Action', '30 ft', 'Instant', 'CM', 'Cost: 3', 'Ninjutsu',
		         'Make a Ninjutsu Attack against one target.')`,
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		 VALUES ('jutsu/great-waterfall', 'Great Waterfall Technique', 'Ninjutsu', 'B', '1 Action', '60 ft', 'Instant', 'CM', 'Cost: 5', 'Ninjutsu',
		         'Make a Ninjutsu Attack against every target in a line.')`,
	} {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Mizu')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/ninjutsu-specialist', 20, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (1, 'jutsu/water-bullet'), (1, 'jutsu/great-waterfall')`,
	); err != nil {
		t.Fatal(err)
	}
	var waterBulletID int64
	if err := s.charDB.QueryRow(
		`SELECT id FROM character_jutsu WHERE character_id = 1 AND jutsu_slug = 'jutsu/water-bullet'`,
	).Scan(&waterBulletID); err != nil {
		t.Fatal(err)
	}
	if err := charstore.AddNinjutsuJutsuPick(s.charDB, 1, charstore.NinjutsuPickMaster, waterBulletID); err != nil {
		t.Fatal(err)
	}

	sheet := &charsheet.Sheet{Level: 20}
	rows, err := s.loadCharacterJutsuSheet(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]jutsuSheetRow{}
	for _, r := range rows {
		byName[r.Slug] = r
	}
	if !byName["jutsu/water-bullet"].AlwaysMaxDamage {
		t.Error("Water Bullet (the Ninjutsu Master pick) AlwaysMaxDamage = false, want true")
	}
	if byName["jutsu/great-waterfall"].AlwaysMaxDamage {
		t.Error("Great Waterfall Technique (not picked) AlwaysMaxDamage = true, want false")
	}

	// Drop below 20th level (e.g. a multiclass respec) without removing the
	// stored pick — ninjutsuMasterCap gates the badge the same way
	// loadNinjutsuSpecialistTabData already hides KnownMaster below 20th.
	if _, err := s.charDB.Exec(
		`UPDATE character_classes SET levels = 19 WHERE character_id = 1 AND class_slug = 'class/ninjutsu-specialist'`,
	); err != nil {
		t.Fatal(err)
	}
	rows, err = s.loadCharacterJutsuSheet(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.AlwaysMaxDamage {
			t.Errorf("AlwaysMaxDamage still set on %s below 20th Ninjutsu Specialist level", r.Slug)
		}
	}
}

// TestNinjutsuMasterAlwaysMaxDamageNoPick covers the common case (no pick
// made yet, or no Ninjutsu Specialist levels at all) — the helper must
// return "" rather than erroring so loadCharacterJutsuSheet's own call site
// doesn't need a special no-pick branch.
func TestNinjutsuMasterAlwaysMaxDamageNoPick(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Nopick')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/ninjutsu-specialist', 20, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	slug, err := s.ninjutsuMasterAlwaysMaxDamageSlug(1)
	if err != nil {
		t.Fatal(err)
	}
	if slug != "" {
		t.Errorf("ninjutsuMasterAlwaysMaxDamageSlug = %q, want \"\" (no pick made)", slug)
	}
}
