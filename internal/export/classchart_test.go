package export

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/schema"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := schema.Apply(db, schema.Rules); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWriteClassChart(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec(`INSERT INTO classes (slug, name) VALUES ('class/bear-nin', 'Bear-Nin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO class_levels (class_slug, level, proficiency_bonus, jutsu_known)
		VALUES ('class/bear-nin', 1, 3, 6), ('class/bear-nin', 2, 3, 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description, sort_order)
		VALUES ('class/bear-nin/feature/claws', 'class/bear-nin', 'Claws', 1, 'x', 0),
		       ('class/bear-nin/feature/asi', 'class/bear-nin', 'Ability Score Improvement/Feat', NULL, 'x', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO class_level_resources (class_slug, level, resource_name, value)
		VALUES ('class/bear-nin', 1, 'Chakra Die', '1d8'),
		       ('class/bear-nin', 2, 'Chakra Die', '2d8')`); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if err := WriteClassChart(db, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// header + 2 levels + 1 unleveled row.
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), out)
	}
	if lines[0] != "Class,Level,Proficiency Bonus,Features,Resources,Jutsu Known" {
		t.Errorf("header = %q", lines[0])
	}
	if lines[1] != "Bear-Nin,1,3,Claws,Chakra Die: 1d8,6" {
		t.Errorf("L1 row = %q", lines[1])
	}
	if lines[2] != "Bear-Nin,2,3,,Chakra Die: 2d8,7" {
		t.Errorf("L2 row (no feature that level) = %q", lines[2])
	}
	if lines[3] != "Bear-Nin,(unleveled),,Ability Score Improvement/Feat,," {
		t.Errorf("unleveled row = %q", lines[3])
	}
}
