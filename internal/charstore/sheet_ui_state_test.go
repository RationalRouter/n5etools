package charstore

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/schema"
)

func testCharDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := schema.Apply(db, schema.Characters); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Test')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSheetUIStateRoundTrip(t *testing.T) {
	db := testCharDB(t)

	state, err := GetSheetUIState(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 0 {
		t.Fatalf("expected no saved state yet, got %v", state)
	}

	if err := SetSheetUIState(db, 1, "grid:core", `{"version":2,"cols":12,"boxes":{}}`); err != nil {
		t.Fatal(err)
	}
	if err := SetSheetUIState(db, 1, "subgrid:squares", `{"order":["hitdice","prof"],"vertical":false}`); err != nil {
		t.Fatal(err)
	}

	state, err = GetSheetUIState(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 2 {
		t.Fatalf("expected 2 saved keys, got %d: %v", len(state), state)
	}
	if state["grid:core"] != `{"version":2,"cols":12,"boxes":{}}` {
		t.Errorf("grid:core = %q", state["grid:core"])
	}

	// Overwriting an existing key updates it in place rather than erroring
	// or duplicating.
	if err := SetSheetUIState(db, 1, "grid:core", `{"version":2,"cols":12,"boxes":{"hitdice":{"x":0,"y":0,"w":4,"h":4}}}`); err != nil {
		t.Fatal(err)
	}
	state, err = GetSheetUIState(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 2 {
		t.Fatalf("expected still 2 keys after overwrite, got %d", len(state))
	}
	if state["grid:core"] == `{"version":2,"cols":12,"boxes":{}}` {
		t.Error("grid:core was not updated by the second SetSheetUIState call")
	}

	if err := DeleteSheetUIState(db, 1, "grid:core"); err != nil {
		t.Fatal(err)
	}
	state, err = GetSheetUIState(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state["grid:core"]; ok {
		t.Error("grid:core still present after DeleteSheetUIState")
	}
	if _, ok := state["subgrid:squares"]; !ok {
		t.Error("deleting grid:core should not have touched subgrid:squares")
	}
}
