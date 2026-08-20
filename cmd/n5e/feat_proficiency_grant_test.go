package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// TestApplyFeatProficiencyGrant covers applyFeatProficiencyGrant's own
// branch logic directly (feat_skill_proficiency.go) — the two call sites
// (handleSheetFeatAdd, the ASI breakpoint's Feat alternative, and
// handleSheetFeatSkillOrToolChoice) all delegate to this one function, so a
// single focused test here covers the shared behavior all three depend on,
// without needing a real feat row or rules.db content.
func TestApplyFeatProficiencyGrant(t *testing.T) {
	const noMasteryClause = "You gain proficiency in Deception."
	const masteryClause = "You gain proficiency in the Insight skill. If you are already proficient, you instead gain Mastery."

	t.Run("not already proficient: ordinary grant, no Mastery row", func(t *testing.T) {
		s := testServer(t)
		res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()

		if err := s.applyFeatProficiencyGrant(id, "feat/empathic", "skill", "Insight", masteryClause); err != nil {
			t.Fatal(err)
		}
		has, err := charstore.HasProficiency(s.charDB, id, "skill", "Insight")
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("expected an ordinary Insight proficiency row")
		}
		rank, err := charstore.MasteryRankFor(s.charDB, id, "Insight")
		if err != nil {
			t.Fatal(err)
		}
		if rank != 0 {
			t.Fatalf("rank = %d, want 0 (no Mastery entry should exist yet)", rank)
		}
	})

	t.Run("already proficient, no Mastery clause: duplicate grant is a harmless no-op write", func(t *testing.T) {
		s := testServer(t)
		res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if err := charstore.ApplyFeatSkillProficiency(s.charDB, id, "feat/class/other", "Deception"); err != nil {
			t.Fatal(err)
		}

		if err := s.applyFeatProficiencyGrant(id, "feat/feigned-confidence", "skill", "Deception", noMasteryClause); err != nil {
			t.Fatal(err)
		}
		rank, err := charstore.MasteryRankFor(s.charDB, id, "Deception")
		if err != nil {
			t.Fatal(err)
		}
		if rank != 0 {
			t.Fatalf("rank = %d, want 0 (no Mastery clause in this feat's text)", rank)
		}
	})

	t.Run("already proficient with Mastery clause: upgrades to Rank 1 Mastery instead of a duplicate row", func(t *testing.T) {
		s := testServer(t)
		res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if err := charstore.ApplyFeatSkillProficiency(s.charDB, id, "feat/class/other", "Insight"); err != nil {
			t.Fatal(err)
		}

		if err := s.applyFeatProficiencyGrant(id, "feat/empathic", "skill", "Insight", masteryClause); err != nil {
			t.Fatal(err)
		}
		rank, err := charstore.MasteryRankFor(s.charDB, id, "Insight")
		if err != nil {
			t.Fatal(err)
		}
		if rank != 1 {
			t.Fatalf("rank = %d, want 1", rank)
		}
		// The feat itself must not also have written its own duplicate
		// character_proficiencies row — the Mastery entry is the only trace
		// of this grant.
		var featOwnRow int
		if err := s.charDB.QueryRow(
			`SELECT COUNT(*) FROM character_proficiencies WHERE character_id = ? AND source_kind = 'feat' AND source_ref = 'feat/empathic'`,
			id,
		).Scan(&featOwnRow); err != nil {
			t.Fatal(err)
		}
		if featOwnRow != 0 {
			t.Fatalf("feat/empathic wrote %d of its own proficiency rows, want 0", featOwnRow)
		}
	})

	t.Run("already proficient with an existing Mastery rank: increases it by one", func(t *testing.T) {
		s := testServer(t)
		res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if err := charstore.ApplyFeatSkillProficiency(s.charDB, id, "feat/class/other", "Insight"); err != nil {
			t.Fatal(err)
		}
		if err := charstore.SetMastery(s.charDB, id, "Insight", 1); err != nil {
			t.Fatal(err)
		}

		if err := s.applyFeatProficiencyGrant(id, "feat/empathic", "skill", "Insight", masteryClause); err != nil {
			t.Fatal(err)
		}
		rank, err := charstore.MasteryRankFor(s.charDB, id, "Insight")
		if err != nil {
			t.Fatal(err)
		}
		if rank != 2 {
			t.Fatalf("rank = %d, want 2", rank)
		}
	})

	t.Run("already at the absolute Mastery cap: no further increase", func(t *testing.T) {
		s := testServer(t)
		res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if err := charstore.ApplyFeatSkillProficiency(s.charDB, id, "feat/class/other", "Insight"); err != nil {
			t.Fatal(err)
		}
		if err := charstore.SetMastery(s.charDB, id, "Insight", 3); err != nil {
			t.Fatal(err)
		}

		if err := s.applyFeatProficiencyGrant(id, "feat/empathic", "skill", "Insight", masteryClause); err != nil {
			t.Fatal(err)
		}
		rank, err := charstore.MasteryRankFor(s.charDB, id, "Insight")
		if err != nil {
			t.Fatal(err)
		}
		if rank != 3 {
			t.Fatalf("rank = %d, want 3 (already at MaxMasteryRank)", rank)
		}
	})

	t.Run("tool kind: already proficient with a kit and a Mastery clause upgrades the same way", func(t *testing.T) {
		s := testServer(t)
		res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test')`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if err := charstore.ApplyFeatToolProficiency(s.charDB, id, "feat/class/other", "Security Kit"); err != nil {
			t.Fatal(err)
		}
		const toolMasteryClause = "You gain proficiency with a Security Kit or Sleight of Hand (Pick one). If you are already proficient, you instead gain Mastery."

		if err := s.applyFeatProficiencyGrant(id, "feat/burglar", "tool", "Security Kit", toolMasteryClause); err != nil {
			t.Fatal(err)
		}
		rank, err := charstore.MasteryRankFor(s.charDB, id, "Security Kit")
		if err != nil {
			t.Fatal(err)
		}
		if rank != 1 {
			t.Fatalf("rank = %d, want 1", rank)
		}
	})
}
