package schema

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openMemDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestApplyRulesFromEmpty(t *testing.T) {
	db := openMemDB(t)
	if err := Apply(db, Rules); err != nil {
		t.Fatalf("applying rules migrations: %v", err)
	}

	// Seed data sanity: 6 ranks, 20 advancement rows, the elemental circle.
	checks := []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM jutsu_ranks`, 6},
		{`SELECT COUNT(*) FROM xp_levels`, 20},
		{`SELECT COUNT(*) FROM elemental_advantage`, 5},
		{`SELECT COUNT(*) FROM ability_point_costs`, 8},
	}
	for _, c := range checks {
		var got int
		if err := db.QueryRow(c.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.query, err)
		}
		if got != c.want {
			t.Errorf("%s = %d, want %d", c.query, got, c.want)
		}
	}

	// The core book's worked values: level 1 costs 0 XP at +3 proficiency,
	// level 20 costs 3000 XP at +9.
	var prof int
	if err := db.QueryRow(`SELECT proficiency_bonus FROM xp_levels WHERE level = 1`).Scan(&prof); err != nil {
		t.Fatal(err)
	}
	if prof != 3 {
		t.Errorf("level 1 proficiency = %d, want 3", prof)
	}

	// Override views: an override wins over the parsed value.
	mustExec(t, db, `INSERT INTO clans (slug, name, speed_feet) VALUES ('clan/test', 'Test', 30)`)
	mustExec(t, db, `UPDATE clans SET speed_feet_override = 35 WHERE slug = 'clan/test'`)
	var speed int
	if err := db.QueryRow(`SELECT speed_feet FROM v_clans WHERE slug = 'clan/test'`).Scan(&speed); err != nil {
		t.Fatal(err)
	}
	if speed != 35 {
		t.Errorf("v_clans speed_feet = %d, want override value 35", speed)
	}
}

func TestApplyCharactersFromEmpty(t *testing.T) {
	db := openMemDB(t)
	if err := Apply(db, Characters); err != nil {
		t.Fatalf("applying character migrations: %v", err)
	}

	// A minimal character round-trips.
	mustExec(t, db, `INSERT INTO characters (name, clan_slug, background_slug) VALUES ('Naruto Uzumaki', 'clan/uzumaki', 'background/trouble-maker')`)
	mustExec(t, db, `INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/scout-nin', 1, 0)`)

	// The jutsu XOR constraint: exactly one of jutsu_slug / custom_jutsu_id.
	mustExec(t, db, `INSERT INTO character_jutsu (character_id, jutsu_slug, source) VALUES (1, 'jutsu/clone-technique', 'learned')`)
	if _, err := db.Exec(`INSERT INTO character_jutsu (character_id, source) VALUES (1, 'learned')`); err == nil {
		t.Error("character_jutsu with neither slug nor custom id should be rejected")
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	db := openMemDB(t)
	for i := 0; i < 2; i++ {
		if err := Apply(db, Rules); err != nil {
			t.Fatalf("apply #%d: %v", i+1, err)
		}
	}
}

func TestLatestMigration(t *testing.T) {
	names, err := migrationNames(Rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no rules migrations found — test environment is broken")
	}
	want := names[len(names)-1]

	got, err := LatestMigration(Rules)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("LatestMigration(Rules) = %q, want %q (the highest-sorted filename)", got, want)
	}

	if !strings.HasSuffix(got, ".sql") {
		t.Errorf("LatestMigration(Rules) = %q, doesn't look like a real migration filename", got)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}
