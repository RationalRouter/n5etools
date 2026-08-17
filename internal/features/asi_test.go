package features

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/schema"
)

func TestResolveASISlotsBreakpointsAndRefs(t *testing.T) {
	granted := []GrantedFeatureRow{{Slug: "class/hunter-nin/feature/ability-score-improvement-feat"}}
	classLevels := map[string]int{"class/hunter-nin": 9}

	got := ResolveASISlots(granted, classLevels)
	// Level 9 clears breakpoints 4 and 8 only.
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2: %+v", len(got), got)
	}
	want := map[int]string{
		4: "class/hunter-nin@4",
		8: "class/hunter-nin@8",
	}
	for _, s := range got {
		if s.Ref != want[s.Level] {
			t.Errorf("level %d: ref = %q, want %q", s.Level, s.Ref, want[s.Level])
		}
		if s.ClassSlug != "class/hunter-nin" {
			t.Errorf("ClassSlug = %q, want class/hunter-nin", s.ClassSlug)
		}
	}
}

func TestResolveASISlotsMulticlassIndependentRefs(t *testing.T) {
	// Two different classes both at level 4 must not collide on one ref.
	granted := []GrantedFeatureRow{
		{Slug: "class/hunter-nin/feature/ability-score-improvement-feat"},
		{Slug: "class/puppet-master/feature/ability-score-improvement-feat"},
	}
	classLevels := map[string]int{"class/hunter-nin": 4, "class/puppet-master": 4}

	got := ResolveASISlots(granted, classLevels)
	if len(got) != 2 {
		t.Fatalf("got %d slots, want 2: %+v", len(got), got)
	}
	refs := map[string]bool{}
	for _, s := range got {
		refs[s.Ref] = true
	}
	if !refs["class/hunter-nin@4"] || !refs["class/puppet-master@4"] {
		t.Errorf("refs = %v, want both classes' own level-4 ref present and distinct", refs)
	}
}

func TestResolveASISlotsIgnoresUnrelatedFeatures(t *testing.T) {
	granted := []GrantedFeatureRow{{Slug: "class/hunter-nin/feature/expertise"}}
	if got := ResolveASISlots(granted, map[string]int{"class/hunter-nin": 20}); len(got) != 0 {
		t.Fatalf("got %d slots for a non-ASI feature, want 0: %+v", len(got), got)
	}
}

func TestPendingASISlots(t *testing.T) {
	slots := []ASISlot{
		{Ref: "class/hunter-nin@4"},
		{Ref: "class/hunter-nin@8"},
	}
	resolved := map[string]bool{"class/hunter-nin@4": true}
	got := PendingASISlots(slots, resolved)
	if len(got) != 1 || got[0].Ref != "class/hunter-nin@8" {
		t.Fatalf("got %+v, want only the unresolved level-8 slot", got)
	}
}

func TestValidASIPicks(t *testing.T) {
	scores := map[string]int{"str": 10, "dex": 15, "con": 19, "wis": 20}
	maxScores := map[string]int{"str": 20, "dex": 20, "con": 20, "wis": 20}
	cases := []struct {
		name  string
		picks []AbilityPick
		want  bool
	}{
		{"two distinct scores +1 each", []AbilityPick{{"str", 1}, {"dex", 1}}, true},
		{"one score +2", []AbilityPick{{"str", 2}}, true},
		{"same ability twice is not a real split", []AbilityPick{{"str", 1}, {"str", 1}}, false},
		{"+2 to a score already at 19 exceeds the cap", []AbilityPick{{"con", 2}}, false},
		{"+1 to a score at 19 is still fine", []AbilityPick{{"con", 1}, {"str", 1}}, true},
		{"any increase to a score already at 20 is rejected", []AbilityPick{{"wis", 1}, {"str", 1}}, false},
		{"three picks is not a shape the book allows", []AbilityPick{{"str", 1}, {"dex", 1}, {"con", 1}}, false},
		{"a lone +1 (no partner) is not a real ASI", []AbilityPick{{"str", 1}}, false},
		{"an unknown ability code", []AbilityPick{{"nonsense", 2}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidASIPicks(c.picks, scores, maxScores); got != c.want {
				t.Errorf("ValidASIPicks(%+v) = %v, want %v", c.picks, got, c.want)
			}
		})
	}
}

// TestValidASIPicksRaisedMax covers the Item 11 interaction: a character
// with a RaisesMax grant (e.g. Master of Focus) sitting at a current score
// above the flat-20 cap must still be able to take a further legal ASI up
// to their REAL max, and must still be rejected past that real max.
func TestValidASIPicksRaisedMax(t *testing.T) {
	// A Master-of-Focus character: Str 22 (20 base/ASI'd up + 2 from the
	// grant), max also raised to 22 by that same grant.
	scores := map[string]int{"str": 22, "dex": 15}
	maxScores := map[string]int{"str": 22, "dex": 20}

	if !ValidASIPicks([]AbilityPick{{Ability: "dex", Amount: 2}}, scores, maxScores) {
		t.Errorf("dex +2 (15->17, max 20) should be valid")
	}
	if ValidASIPicks([]AbilityPick{{Ability: "str", Amount: 1}, {Ability: "dex", Amount: 1}}, scores, maxScores) {
		t.Errorf("str is already at its real max of 22 — a further +1 must be rejected")
	}

	// A missing maxScores entry defaults to 20, same as the old flat cap —
	// a plain, never-boosted character's ValidASIPicks call site can pass
	// an empty/nil map with no special-casing.
	if !ValidASIPicks([]AbilityPick{{Ability: "dex", Amount: 1}, {Ability: "str", Amount: 1}}, map[string]int{"dex": 19, "str": 10}, nil) {
		t.Errorf("missing maxScores entries should default to 20, same as the old flat cap")
	}
}

func TestLoadResolvedASIRefsUnionsFeatChoice(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := schema.Apply(db, schema.Characters); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO character_ability_bonuses (character_id, source_kind, source_ref, ability, amount)
		 VALUES (1, 'asi', 'class/hunter-nin@4', 'str', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO character_asi_feat_choices (character_id, ref, feat_slug) VALUES (1, 'class/hunter-nin@8', 'feat/nature-release')`); err != nil {
		t.Fatal(err)
	}

	got, err := LoadResolvedASIRefs(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got["class/hunter-nin@4"] || !got["class/hunter-nin@8"] || len(got) != 2 {
		t.Fatalf("got %v, want both the ability-branch and feat-branch refs resolved", got)
	}
}
