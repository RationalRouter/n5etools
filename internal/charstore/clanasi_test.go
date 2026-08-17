package charstore

import (
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

// TestEveryClanGrantsAbilityIncreases is the regression guard for the bug
// this file exists to fix: every clan in the real rules database must yield
// at least one usable variant, either from clan_ability_increases or from
// parsing clans.ability_increase_text. Before this, 16 of the 45 produced
// nothing and silently granted a character no clan bonus at all.
//
// Skips when the maintainer-built database isn't present, matching the
// whole-book regression tests elsewhere in this repo — out/rules.db is a
// build artifact, not committed source.
func TestEveryClanGrantsAbilityIncreases(t *testing.T) {
	const dbPath = "../../out/rules.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("out/rules.db not built; skipping whole-database clan check")
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT slug FROM clans ORDER BY slug`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var checked int
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			t.Fatal(err)
		}
		checked++
		variants, err := ClanAbilityVariants(db, slug)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		if len(variants) == 0 {
			t.Errorf("%s: no ability-increase variants — the clan would grant nothing", slug)
			continue
		}
		for vi, variant := range variants {
			if len(variant.Slots) == 0 {
				t.Errorf("%s variant %d: no slots", slug, vi)
			}
			// What the step's dropdowns pre-select must be a legal answer,
			// so pressing the button without touching anything works.
			if _, err := ResolveAbilityPicks(variant, DefaultPicks(variant)); err != nil {
				t.Errorf("%s variant %d: default picks rejected: %v", slug, vi, err)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no clans read from the database")
	}
}

// TestParseAbilityIncreaseText runs the parser over every distinct
// clans.ability_increase_text value that has no clan_ability_increases rows
// — the 16 choice-granting clans whose bonuses were silently never applied
// — plus two of the fixed-text ones for contrast.
func TestParseAbilityIncreaseText(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []AbilityVariant
	}{
		{"chinoike", "+2 Wis or Int, +1 Dex", []AbilityVariant{{Slots: []AbilitySlot{
			{Amount: 2, Options: []string{"wis", "int"}},
			{Amount: 1, Options: []string{"dex"}},
		}}}},
		{"hanami", "+2 Str, +1 Wis or Int", []AbilityVariant{{Slots: []AbilitySlot{
			{Amount: 2, Options: []string{"str"}},
			{Amount: 1, Options: []string{"wis", "int"}},
		}}}},
		{"konjiki", "+2 Int, +1 Str or Con", []AbilityVariant{{Slots: []AbilitySlot{
			{Amount: 2, Options: []string{"int"}},
			{Amount: 1, Options: []string{"str", "con"}},
		}}}},
		// The parenthetical restates the all-picks-distinct rule
		// ResolveAbilityPicks applies to every clan, so it is stripped
		// rather than parsed.
		{"uchiha", "+2 Dex or Int, +1 Int or Wis (You cannot choose Int for both ASI’s)", []AbilityVariant{{Slots: []AbilitySlot{
			{Amount: 2, Options: []string{"dex", "int"}},
			{Amount: 1, Options: []string{"int", "wis"}},
		}}}},
		{"yuki", "+2 Dex or Int, +1 Int or Cha (Int cannot be chosen as the +1 ASI, if it was chosen for the +2 ASI.)", []AbilityVariant{{Slots: []AbilitySlot{
			{Amount: 2, Options: []string{"dex", "int"}},
			{Amount: 1, Options: []string{"int", "cha"}},
		}}}},
		// Full ability names, not abbreviations — both spellings appear in
		// the same column for different clans.
		{"full names", "+2 Intelligence, +1 Wisdom", []AbilityVariant{{Slots: []AbilitySlot{
			{Amount: 2, Options: []string{"int"}},
			{Amount: 1, Options: []string{"wis"}},
		}}}},
		// The only clan with a genuine alternative spread. Both variants get
		// a label, since a radio with an unlabelled option is useless.
		{"non-clan", "+2 to any Ability Score, +1 to any other Ability Score not selected previously (or +1 to any 3 Ability Scores.).", []AbilityVariant{
			{Label: "+2 to any, +1 to any", Slots: []AbilitySlot{
				{Amount: 2, Options: []string{"str", "dex", "con", "int", "wis", "cha"}},
				{Amount: 1, Options: []string{"str", "dex", "con", "int", "wis", "cha"}},
			}},
			{Label: "+1 to any 3 abilities", Slots: []AbilitySlot{
				{Amount: 1, Options: []string{"str", "dex", "con", "int", "wis", "cha"}},
				{Amount: 1, Options: []string{"str", "dex", "con", "int", "wis", "cha"}},
				{Amount: 1, Options: []string{"str", "dex", "con", "int", "wis", "cha"}},
			}},
		}},
		// Nothing understood yields nothing, so a clan silently granting no
		// bonus is visible rather than a plausible-looking wrong bonus.
		{"empty", "", nil},
		{"unparseable", "See the clan's entry", nil},
	}

	for _, c := range cases {
		got := ParseAbilityIncreaseText(c.text)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ParseAbilityIncreaseText(%q)\n got %+v\nwant %+v", c.name, c.text, got, c.want)
		}
	}
}

func TestResolveAbilityPicks(t *testing.T) {
	uchiha := ParseAbilityIncreaseText("+2 Dex or Int, +1 Int or Wis (You cannot choose Int for both ASI’s)")[0]

	got, err := ResolveAbilityPicks(uchiha, []string{"dex", "int"})
	if err != nil {
		t.Fatalf("valid picks rejected: %v", err)
	}
	want := []AbilityGrant{{Ability: "dex", Amount: 2}, {Ability: "int", Amount: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// The book's own restriction for this clan, enforced as the general
	// "every increase goes to a different ability" rule.
	if _, err := ResolveAbilityPicks(uchiha, []string{"int", "int"}); !errors.Is(err, ErrInvalidPick) {
		t.Errorf("duplicate pick error = %v, want ErrInvalidPick", err)
	}
	// An ability the slot never offered — what a hand-crafted POST would send.
	if _, err := ResolveAbilityPicks(uchiha, []string{"str", "wis"}); !errors.Is(err, ErrInvalidPick) {
		t.Errorf("off-menu pick error = %v, want ErrInvalidPick", err)
	}
	if _, err := ResolveAbilityPicks(uchiha, []string{"dex"}); !errors.Is(err, ErrInvalidPick) {
		t.Errorf("short pick list error = %v, want ErrInvalidPick", err)
	}
}
