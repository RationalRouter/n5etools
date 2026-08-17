package main

import "testing"

// TestOtherClansJutsu: a jutsu belonging to a DIFFERENT clan's own list must
// be excluded, but the character's own clan's jutsu (and any jutsu not tied
// to any clan at all) must not be — there is no rule letting a character
// benefit from a second clan's jutsu.
func TestOtherClansJutsu(t *testing.T) {
	s := testServer(t)
	for _, clan := range []string{"clan/uchiha", "clan/hyuga"} {
		if _, err := s.rulesDB.Exec(`INSERT INTO clans (slug, name) VALUES (?, ?)`, clan, clan); err != nil {
			t.Fatal(err)
		}
	}
	seed := []struct{ clan, jutsu string }{
		{"clan/uchiha", "jutsu/fire-release-fireball"},
		{"clan/hyuga", "jutsu/gentle-fist"},
		{"clan/hyuga", "jutsu/byakugan"},
	}
	for _, r := range seed {
		if _, err := s.rulesDB.Exec(`
			INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
			VALUES (?, ?, 'Ninjutsu', 'D', '1 Action', '30 feet', 'Instant', 'HS', '', '', '')`, r.jutsu, r.jutsu); err != nil {
			t.Fatal(err)
		}
		if _, err := s.rulesDB.Exec(
			`INSERT INTO clan_jutsu (clan_slug, jutsu_slug) VALUES (?, ?)`, r.clan, r.jutsu); err != nil {
			t.Fatal(err)
		}
	}

	got, err := otherClansJutsu(s.rulesDB, "clan/uchiha")
	if err != nil {
		t.Fatal(err)
	}
	if got["jutsu/fire-release-fireball"] {
		t.Error("own clan's jutsu (Uchiha) must not be excluded")
	}
	if !got["jutsu/gentle-fist"] || !got["jutsu/byakugan"] {
		t.Errorf("Hyuga's jutsu must be excluded for an Uchiha character, got %v", got)
	}
	if got["jutsu/something-not-clan-affiliated"] {
		t.Error("a jutsu with no clan_jutsu row at all was somehow marked excluded")
	}

	gotNoClan, err := otherClansJutsu(s.rulesDB, "")
	if err != nil {
		t.Fatal(err)
	}
	if !gotNoClan["jutsu/fire-release-fireball"] || !gotNoClan["jutsu/gentle-fist"] {
		t.Errorf("a character with no clan must have every clan's jutsu excluded, got %v", gotNoClan)
	}
}
