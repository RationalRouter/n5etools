package charstore

import (
	"database/sql"
	"testing"
)

func TestMeetsMulticlassPrereqAllClasses(t *testing.T) {
	cases := []struct {
		name      string
		classSlug string
		scores    map[string]int
		want      bool
	}{
		{"genjutsu meets", "class/genjutsu-specialist", map[string]int{"wis": 14, "cha": 14}, true},
		{"genjutsu fails cha", "class/genjutsu-specialist", map[string]int{"wis": 14, "cha": 13}, false},
		{"hunter-nin meets", "class/hunter-nin", map[string]int{"dex": 14, "int": 14}, true},
		{"hunter-nin fails dex", "class/hunter-nin", map[string]int{"dex": 13, "int": 14}, false},
		{"intel-op meets", "class/intelligence-operative", map[string]int{"int": 14, "cha": 14}, true},
		{"medical-nin meets", "class/medical-nin", map[string]int{"int": 14, "wis": 14}, true},
		{"ninjutsu meets", "class/ninjutsu-specialist", map[string]int{"con": 14, "int": 14}, true},
		{"scout-nin meets all three", "class/scout-nin", map[string]int{"str": 13, "int": 13, "wis": 13}, true},
		{"scout-nin fails one of three", "class/scout-nin", map[string]int{"str": 13, "int": 13, "wis": 12}, false},
		{"taijutsu meets via str", "class/taijutsu-specialist", map[string]int{"str": 14, "dex": 8, "con": 14}, true},
		{"taijutsu meets via dex", "class/taijutsu-specialist", map[string]int{"str": 8, "dex": 14, "con": 14}, true},
		{"taijutsu fails con even with str", "class/taijutsu-specialist", map[string]int{"str": 14, "dex": 8, "con": 13}, false},
		{"weapon-specialist meets", "class/weapon-specialist", map[string]int{"str": 14, "dex": 14}, true},
		{"puppet-master meets (typo-repaired clause)", "class/puppet-master", map[string]int{"con": 14, "int": 14}, true},
		{"puppet-master fails con", "class/puppet-master", map[string]int{"con": 13, "int": 14}, false},
		{"cooking-nin meets", "class/cooking-nin", map[string]int{"int": 14, "cha": 14}, true},
		{"science-nin meets", "class/science-nin", map[string]int{"int": 16}, true},
		{"science-nin fails at 15", "class/science-nin", map[string]int{"int": 15}, false},
		{"unknown class slug defaults to met", "class/does-not-exist", map[string]int{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MeetsMulticlassPrereq(c.scores, c.classSlug); got != c.want {
				t.Errorf("MeetsMulticlassPrereq(%v, %q) = %v, want %v", c.scores, c.classSlug, got, c.want)
			}
		})
	}
}

func TestUnmetMulticlassClasses(t *testing.T) {
	scores := map[string]int{"str": 10, "dex": 10, "con": 10, "int": 16, "wis": 10, "cha": 10}
	unmet := UnmetMulticlassClasses(scores, []string{"class/science-nin", "class/weapon-specialist"})
	if len(unmet) != 1 || unmet[0] != "class/weapon-specialist" {
		t.Errorf("unmet = %v, want just class/weapon-specialist", unmet)
	}
}

func addPrimaryClass(t *testing.T, db *sql.DB, characterID int64, classSlug string, level int) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, ?, ?, 0)`,
		characterID, classSlug, level,
	); err != nil {
		t.Fatal(err)
	}
}

func TestAddMulticlassFixedGrants(t *testing.T) {
	db := testCharDB(t)
	addPrimaryClass(t, db, 1, "class/weapon-specialist", 5)

	if err := AddMulticlass(db, 1, "class/ninjutsu-specialist", 3, "", "", ""); err != nil {
		t.Fatal(err)
	}

	var levels, orderIndex int
	if err := db.QueryRow(
		`SELECT levels, order_index FROM character_classes WHERE character_id = 1 AND class_slug = 'class/ninjutsu-specialist'`,
	).Scan(&levels, &orderIndex); err != nil {
		t.Fatal(err)
	}
	if levels != 3 || orderIndex != 1 {
		t.Errorf("levels=%d orderIndex=%d, want 3/1", levels, orderIndex)
	}

	rows, err := db.Query(
		`SELECT kind, value FROM character_proficiencies WHERE character_id = 1 AND source_ref = 'class/ninjutsu-specialist' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got [][2]string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		got = append(got, [2]string{k, v})
	}
	want := [][2]string{{"skill", "Ninshou"}, {"skill", "Chakra Control"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("grant %d = %v, want %v", i, got[i], want[i])
		}
	}

	// No level-1-max hit/chakra gain row should have been written for the
	// second class — Compute's own fallback handles it since it isn't the
	// character's first class.
	var gainCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM character_level_gains WHERE character_id = 1 AND class_slug = 'class/ninjutsu-specialist'`,
	).Scan(&gainCount); err != nil {
		t.Fatal(err)
	}
	if gainCount != 0 {
		t.Errorf("expected no stored level gains for a fresh multiclass add, got %d", gainCount)
	}
}

func TestAddMulticlassChoiceGrants(t *testing.T) {
	db := testCharDB(t)
	addPrimaryClass(t, db, 1, "class/medical-nin", 1)

	if err := AddMulticlass(db, 1, "class/hunter-nin", 1, "Stealth", "", ""); err != nil {
		t.Fatal(err)
	}
	var skillCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = 1 AND source_ref = 'class/hunter-nin' AND kind = 'skill' AND value = 'Stealth'`,
	).Scan(&skillCount); err != nil {
		t.Fatal(err)
	}
	if skillCount != 1 {
		t.Errorf("expected the chosen skill Stealth to be granted, got count %d", skillCount)
	}
	var trackersCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = 1 AND source_ref = 'class/hunter-nin' AND kind = 'tool' AND value = 'Trackers Kit'`,
	).Scan(&trackersCount); err != nil {
		t.Fatal(err)
	}
	if trackersCount != 1 {
		t.Error("expected the fixed Trackers Kit grant")
	}
}

func TestAddMulticlassMissingChoiceErrors(t *testing.T) {
	db := testCharDB(t)
	addPrimaryClass(t, db, 1, "class/medical-nin", 1)

	if err := AddMulticlass(db, 1, "class/hunter-nin", 1, "", "", ""); err != ErrMissingChoice {
		t.Errorf("err = %v, want ErrMissingChoice", err)
	}
}

func TestAddMulticlassLevelCap(t *testing.T) {
	db := testCharDB(t)
	addPrimaryClass(t, db, 1, "class/weapon-specialist", 18)

	if err := AddMulticlass(db, 1, "class/ninjutsu-specialist", 3, "", "", ""); err != ErrLevelCapExceeded {
		t.Errorf("err = %v, want ErrLevelCapExceeded", err)
	}
	// Exactly at the cap should succeed.
	if err := AddMulticlass(db, 1, "class/ninjutsu-specialist", 2, "", "", ""); err != nil {
		t.Fatalf("expected success at exactly 20 total, got %v", err)
	}
}

func TestSetClassLevelRespectsTotalCap(t *testing.T) {
	db := testCharDB(t)
	addPrimaryClass(t, db, 1, "class/weapon-specialist", 10)
	if err := AddMulticlass(db, 1, "class/ninjutsu-specialist", 5, "", "", ""); err != nil {
		t.Fatal(err)
	}

	if err := SetClassLevel(db, 1, "class/weapon-specialist", 16); err != ErrLevelCapExceeded {
		t.Errorf("err = %v, want ErrLevelCapExceeded (16 + 5 in the other class = 21)", err)
	}
	if err := SetClassLevel(db, 1, "class/weapon-specialist", 15); err != nil {
		t.Fatalf("expected success at exactly 20 total, got %v", err)
	}
}

func TestRemoveClassResequencesAndCleansUp(t *testing.T) {
	db := testCharDB(t)
	addPrimaryClass(t, db, 1, "class/weapon-specialist", 5)
	if err := AddMulticlass(db, 1, "class/ninjutsu-specialist", 3, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := AddMulticlass(db, 1, "class/medical-nin", 2, "", "", ""); err != nil {
		t.Fatal(err)
	}

	if err := RemoveClass(db, 1, "class/ninjutsu-specialist", nil); err != nil {
		t.Fatal(err)
	}

	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = 1`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("expected 2 remaining classes, got %d", remaining)
	}
	var medicalOrder int
	if err := db.QueryRow(
		`SELECT order_index FROM character_classes WHERE character_id = 1 AND class_slug = 'class/medical-nin'`,
	).Scan(&medicalOrder); err != nil {
		t.Fatal(err)
	}
	if medicalOrder != 1 {
		t.Errorf("medical-nin order_index = %d, want 1 (resequenced down from 2)", medicalOrder)
	}
	var profCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = 1 AND source_ref = 'class/ninjutsu-specialist'`,
	).Scan(&profCount); err != nil {
		t.Fatal(err)
	}
	if profCount != 0 {
		t.Errorf("expected removed class's proficiencies cleared, got %d rows", profCount)
	}
}
