package store

import (
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

func TestLoadBloodlineLatents(t *testing.T) {
	db := testDB(t)
	// The latent tables reference clans — load one first.
	if _, err := LoadClanBook(db, clanBook, []parse.Clan{sampleClan()}, nil); err != nil {
		t.Fatal(err)
	}

	feat := &parse.Feat{
		Name: "Bloodline, Latent", Category: "Clan, Rare",
		Prerequisites: "Levels 1-4 only", Description: "You gain 10 Bloodline Points.",
		SourcePage: 218,
	}
	latents := []parse.BloodlineLatent{
		{ClanName: "Aburame", Name: "Latent Bug Host I", PointCost: 2,
			Description: "Beginning at 1st level, twice per long rest, add 1d4.", SourcePage: 219},
		{ClanName: "Aburame", Name: "Latent Bug Host II", PointCost: 3,
			Prerequisites: "You must have Latent Bug Host I",
			Description:   "Beginning at 7th level, the bonus increases.", SourcePage: 219},
		{ClanName: "Nowhere", Name: "Orphan Ability", PointCost: 2,
			Description: "References a clan that was never loaded.", SourcePage: 220},
	}

	r, err := LoadBloodlineLatents(db, clanBook, feat, latents)
	if err != nil {
		t.Fatal(err)
	}
	// 1 feat + 2 latents created; the orphan is skipped with a reason.
	if r.Created != 3 || len(r.Skipped) != 1 {
		t.Errorf("first load: %+v", r)
	}

	r, err = LoadBloodlineLatents(db, clanBook, feat, latents)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Unchanged != 3 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var cost int
	var prereq string
	if err := db.QueryRow(`
		SELECT point_cost, prerequisites FROM bloodline_latents
		WHERE slug = 'latent/aburame/latent-bug-host-ii'`).Scan(&cost, &prereq); err != nil {
		t.Fatal(err)
	}
	if cost != 3 || prereq != "You must have Latent Bug Host I" {
		t.Errorf("latent row: cost=%d prereq=%q", cost, prereq)
	}

	// The section feat is unscoped (clan_slug NULL).
	var scoped any
	if err := db.QueryRow(`SELECT clan_slug FROM feats WHERE slug = 'feat/bloodline-latent'`).Scan(&scoped); err != nil {
		t.Fatal(err)
	}
	if scoped != nil {
		t.Errorf("bloodline feat clan_slug = %v, want NULL", scoped)
	}

	// Re-running the clan-book load must not report the bloodline feat as
	// vanished — it lives in the feats table but belongs to this loader.
	cr, err := LoadClanBook(db, clanBook, []parse.Clan{sampleClan()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cr.Vanished) != 0 {
		t.Errorf("clan-book reload reported vanished rows: %v", cr.Vanished)
	}
}
