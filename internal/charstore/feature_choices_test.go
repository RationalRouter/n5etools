package charstore

import "testing"

func TestSetAbilityScoreImprovementSplitAndDouble(t *testing.T) {
	db := testCharDB(t)

	if err := SetAbilityScoreImprovement(db, 1, "class/hunter-nin@4",
		[]AbilityPick{{Ability: "str", Amount: 1}, {Ability: "dex", Amount: 1}}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT ability, amount FROM character_ability_bonuses WHERE character_id = 1 AND source_ref = 'class/hunter-nin@4'`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for rows.Next() {
		var ab string
		var amt int
		if err := rows.Scan(&ab, &amt); err != nil {
			t.Fatal(err)
		}
		got[ab] = amt
	}
	rows.Close()
	if got["str"] != 1 || got["dex"] != 1 || len(got) != 2 {
		t.Fatalf("split pick: got %v, want str:1 dex:1", got)
	}

	// Re-resolving the same ref as the "double" variant replaces the split
	// pick entirely rather than stacking on top of it.
	if err := SetAbilityScoreImprovement(db, 1, "class/hunter-nin@4",
		[]AbilityPick{{Ability: "con", Amount: 2}}); err != nil {
		t.Fatal(err)
	}
	rows, err = db.Query(`SELECT ability, amount FROM character_ability_bonuses WHERE character_id = 1 AND source_ref = 'class/hunter-nin@4'`)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]int{}
	for rows.Next() {
		var ab string
		var amt int
		if err := rows.Scan(&ab, &amt); err != nil {
			t.Fatal(err)
		}
		got[ab] = amt
	}
	rows.Close()
	if got["con"] != 2 || len(got) != 1 {
		t.Fatalf("double pick after re-resolve: got %v, want only con:2", got)
	}
}

func TestSetAbilityScoreImprovementFeatSwitching(t *testing.T) {
	db := testCharDB(t)
	ref := "class/hunter-nin@4"

	if err := SetAbilityScoreImprovementFeat(db, 1, ref, "feat/nature-release", 4); err != nil {
		t.Fatal(err)
	}
	var owns int
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1 AND feat_slug = 'feat/nature-release'`).Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if owns != 1 {
		t.Fatalf("expected the feat to be granted, got count %d", owns)
	}
	var linkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_asi_feat_choices WHERE character_id = 1 AND ref = ?`, ref).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Fatalf("expected one linkage row, got %d", linkCount)
	}

	// Switching to a DIFFERENT feat for the same ref must drop the old grant,
	// not stack a second feat on top of it.
	if err := SetAbilityScoreImprovementFeat(db, 1, ref, "feat/action-surge", 4); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1 AND feat_slug = 'feat/nature-release'`).Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if owns != 0 {
		t.Fatalf("expected the old feat grant to be removed, got count %d", owns)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1 AND feat_slug = 'feat/action-surge'`).Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if owns != 1 {
		t.Fatalf("expected the new feat to be granted, got count %d", owns)
	}

	// Switching back to an ability-score pick must clean up the feat side
	// entirely — no dangling linkage row, no dangling feat grant.
	if err := SetAbilityScoreImprovement(db, 1, ref, []AbilityPick{{Ability: "str", Amount: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1 AND feat_slug = 'feat/action-surge'`).Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if owns != 0 {
		t.Fatalf("expected the feat grant to be removed on switch back to ASI, got count %d", owns)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_asi_feat_choices WHERE character_id = 1 AND ref = ?`, ref).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 0 {
		t.Fatalf("expected the linkage row to be removed on switch back to ASI, got %d", linkCount)
	}
}

func TestClearASIFeatChoiceByFeatSlug(t *testing.T) {
	db := testCharDB(t)
	ref := "class/hunter-nin@4"
	if err := SetAbilityScoreImprovementFeat(db, 1, ref, "feat/nature-release", 4); err != nil {
		t.Fatal(err)
	}
	if err := ClearASIFeatChoiceByFeatSlug(db, 1, "feat/nature-release"); err != nil {
		t.Fatal(err)
	}
	var linkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM character_asi_feat_choices WHERE character_id = 1 AND ref = ?`, ref).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 0 {
		t.Fatalf("expected the linkage row to be gone after a direct Feats-tab delete, got %d", linkCount)
	}
}
