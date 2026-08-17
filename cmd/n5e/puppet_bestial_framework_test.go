package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/puppetupgrades"
)

// seedSummonTribe inserts one minimal summon_tribes row plus an optional
// D-Rank summon_tribe_features row and a summon_tribe_attacks row — rulesDB
// carries no summon content of its own in tests (same reason
// seedFrameworkJutsu exists for jutsu), so every Bestial Framework test
// seeds exactly the tribe/trait/attack rows it needs.
func seedSummonTribe(t *testing.T, s *server, tribeSlug, tribeName string) {
	t.Helper()
	if _, err := s.rulesDB.Exec(
		`INSERT INTO summon_tribes (slug, name, summon_type) VALUES (?, ?, 'Carnivoran')`,
		tribeSlug, tribeName,
	); err != nil {
		t.Fatal(err)
	}
}

func seedSummonTribeDRankTrait(t *testing.T, s *server, tribeSlug, traitSlug, traitName string) {
	t.Helper()
	if _, err := s.rulesDB.Exec(
		`INSERT INTO summon_tribe_features (slug, tribe_slug, name, rank, description, sort_order)
		 VALUES (?, ?, ?, 'D', 'test trait description', 0)`,
		traitSlug, tribeSlug, traitName,
	); err != nil {
		t.Fatal(err)
	}
}

func seedSummonTribeAttack(t *testing.T, s *server, tribeSlug, name, description string) {
	t.Helper()
	if _, err := s.rulesDB.Exec(
		`INSERT INTO summon_tribe_attacks (tribe_slug, name, description) VALUES (?, ?, ?)`,
		tribeSlug, name, description,
	); err != nil {
		t.Fatal(err)
	}
}

// bestialFrameworkTestSetup builds a character at the given Puppet Master
// level with one puppet companion holding a Bestial Framework pick, seeded
// with one tribe (Bear) and its D-Rank trait/attack. Returns the companion
// ID and the upgrade pick ID (needed to record sub-choices).
func bestialFrameworkTestSetup(t *testing.T, level int) (s *server, companionID, upgradeID int64) {
	t.Helper()
	s = testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Bestial Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		 VALUES (1, 'class/puppet-master', ?, 0)`, level,
	); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}
	upgradeID, err = charstore.AddCompanionUpgrade(s.charDB, 1, companionID, bestialFrameworkEntrySlug, bestialFrameworkEntrySlug)
	if err != nil {
		t.Fatal(err)
	}
	seedSummonTribe(t, s, "summon/bear", "Bear")
	seedSummonTribeDRankTrait(t, s, "summon/bear", "summon/bear/feature/kuma-flex", "Kuma Flex")
	seedSummonTribeAttack(t, s, "summon/bear", "Claws", "Str + Prof to hit, +Str Slashing Damage.")
	return s, companionID, upgradeID
}

// TestBestialFrameworkSubChoiceOptionsAbility covers slots 0/1: both offer
// the full 6-ability list, unconditionally (no level gate — the entry's own
// ability picks are made at the same 2nd-level moment the Framework itself
// is chosen).
func TestBestialFrameworkSubChoiceOptionsAbility(t *testing.T) {
	s, companionID, _ := bestialFrameworkTestSetup(t, 2)
	for _, slot := range []int{0, 1} {
		opts, err := bestialFrameworkSubChoiceOptions(s, 1, companionID, slot)
		if err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
		if len(opts) != 6 {
			t.Errorf("slot %d: got %d options, want 6 abilities", slot, len(opts))
		}
		for _, o := range opts {
			if !strings.HasPrefix(o.Slug, "ability:") {
				t.Errorf("slot %d: option slug %q has no \"ability:\" prefix", slot, o.Slug)
			}
		}
	}
}

// TestBestialFrameworkSubChoiceOptionsTribe covers slot 2: every seeded
// summon_tribes row, prefixed "tribe:".
func TestBestialFrameworkSubChoiceOptionsTribe(t *testing.T) {
	s, companionID, _ := bestialFrameworkTestSetup(t, 2)
	opts, err := bestialFrameworkSubChoiceOptions(s, 1, companionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 1 || opts[0].Slug != "tribe:summon/bear" {
		t.Errorf("got %+v, want exactly one tribe:summon/bear option", opts)
	}
}

// TestBestialFrameworkSubChoiceOptionsTraitGating covers the 2nd/10th/17th
// level gates on slots 3/4/5, and confirms a still-locked slot returns
// nil, nil (not an empty-but-technically-open list) — same contract
// puppetFrameworkJutsuOptions already uses for Shade/Spellblade.
func TestBestialFrameworkSubChoiceOptionsTraitGating(t *testing.T) {
	cases := []struct {
		level               int
		want3, want4, want5 bool
	}{
		{1, false, false, false},
		{2, true, false, false},
		{9, true, false, false},
		{10, true, true, false},
		{16, true, true, false},
		{17, true, true, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("level_%d", c.level), func(t *testing.T) {
			s, companionID, upgradeID := bestialFrameworkTestSetup(t, c.level)
			if _, err := charstore.AddCompanionUpgradeChoice(s.charDB, 1, companionID, upgradeID, "tribe:summon/bear"); err != nil {
				t.Fatal(err)
			}
			for slot, want := range []bool{c.want3, c.want4, c.want5} {
				opts, err := bestialFrameworkSubChoiceOptions(s, 1, companionID, 3+slot)
				if err != nil {
					t.Fatalf("level %d slot %d: %v", c.level, 3+slot, err)
				}
				if got := len(opts) > 0; got != want {
					t.Errorf("level %d slot %d: got options=%v (len %d), want present=%v", c.level, 3+slot, got, len(opts), want)
				}
			}
		})
	}
}

// TestBestialFrameworkSubChoiceOptionsTraitNoTribeYet covers the "no Sage
// Creature chosen yet" case — even at 17th level, a trait slot offers
// nothing until a tribe is on file to filter by.
func TestBestialFrameworkSubChoiceOptionsTraitNoTribeYet(t *testing.T) {
	s, companionID, _ := bestialFrameworkTestSetup(t, 17)
	opts, err := bestialFrameworkSubChoiceOptions(s, 1, companionID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 0 {
		t.Errorf("got %d options with no tribe chosen, want 0", len(opts))
	}
}

// TestBestialFrameworkTribeSlug covers the read-back helper both before and
// after the tribe sub-choice is recorded.
func TestBestialFrameworkTribeSlug(t *testing.T) {
	s, companionID, upgradeID := bestialFrameworkTestSetup(t, 2)
	got, err := s.bestialFrameworkTribeSlug(1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("before any tribe choice: got %q, want empty", got)
	}
	if _, err := charstore.AddCompanionUpgradeChoice(s.charDB, 1, companionID, upgradeID, "tribe:summon/bear"); err != nil {
		t.Fatal(err)
	}
	got, err = s.bestialFrameworkTribeSlug(1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "summon/bear" {
		t.Errorf("after choosing summon/bear: got %q, want summon/bear", got)
	}
}

// TestBestialFrameworkReferenceTraits covers the reference-text builder:
// unresolved (no tribe yet), then resolved with its natural weapon(s) and
// one chosen D-Rank trait folded in by name/description, not just slug.
func TestBestialFrameworkReferenceTraits(t *testing.T) {
	s, _, _ := bestialFrameworkTestSetup(t, 2)
	entry, ok := puppetupgrades.FoundationEntries[bestialFrameworkEntrySlug]
	if !ok {
		t.Fatal("Bestial Framework entry not found in FoundationEntries")
	}

	unresolved := puppetFoundationPick{Entry: entry}
	got, err := s.bestialFrameworkReferenceTraits(unresolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "Sage Creature: not yet chosen." {
		t.Errorf("unresolved pick: got %v, want a single \"not yet chosen\" line", got)
	}

	resolved := puppetFoundationPick{
		Entry: entry,
		SubChoices: []charstore.CompanionUpgradeChoice{
			{ID: 1, ChoiceSlug: "tribe:summon/bear"},
			{ID: 2, ChoiceSlug: "trait:summon/bear/feature/kuma-flex"},
		},
	}
	got, err = s.bestialFrameworkReferenceTraits(resolved)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "Sage Creature: Bear") {
		t.Errorf("got %v, want a line naming the chosen tribe (Bear)", got)
	}
	if !strings.Contains(joined, "Natural Weapon — Claws") {
		t.Errorf("got %v, want the tribe's own Claws attack as reference text", got)
	}
	if !strings.Contains(joined, "D-Rank Trait — Kuma Flex") {
		t.Errorf("got %v, want the chosen Kuma Flex trait resolved to its own name", got)
	}
}

// TestHandlePuppetUpgradeChoiceAddAllowsRepeatedAbilityKind confirms the
// generic "one pick per kind" collision check (written for Specialized/
// Sentinel, where every kind really is capped at 1) is correctly relaxed
// for Bestial Framework's own "ability" kind — a second "ability:"-prefixed
// pick must succeed, not be rejected as "this category has already been
// chosen."
func TestHandlePuppetUpgradeChoiceAddAllowsRepeatedAbilityKind(t *testing.T) {
	s, companionID, upgradeID := bestialFrameworkTestSetup(t, 2)

	postChoice := func(choiceSlug string) int {
		req := httptest.NewRequest(http.MethodPost,
			"/characters/1/companions/1/upgrades/1/choices",
			strings.NewReader(url.Values{"choice_slug": {choiceSlug}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		req.SetPathValue("cid", strconv.FormatInt(companionID, 10))
		req.SetPathValue("uid", strconv.FormatInt(upgradeID, 10))
		rr := httptest.NewRecorder()
		s.handlePuppetUpgradeChoiceAdd(rr, req)
		return rr.Code
	}

	if code := postChoice("ability:str"); code != http.StatusSeeOther && code != http.StatusOK {
		t.Fatalf("first ability pick: status %d, want a success redirect/OK", code)
	}
	if code := postChoice("ability:dex"); code != http.StatusSeeOther && code != http.StatusOK {
		t.Fatalf("second ability pick: status %d, want a success redirect/OK (not blocked as a duplicate kind)", code)
	}

	choices, err := charstore.ListCompanionUpgradeChoices(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 2 {
		t.Fatalf("recorded choices = %d, want 2 (both ability picks kept)", len(choices))
	}
}
