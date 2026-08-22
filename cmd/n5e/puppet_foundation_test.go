package main

import (
	"fmt"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/puppetupgrades"
)

// TestPuppetFoundationAbilityAdjust covers the trickiest part of
// puppet_foundation.go: AbilityChoiceGroups matched positionally against a
// pick's own stored sub-choices, and the Floor/Set/Delta ordering each
// group applies.
func TestPuppetFoundationAbilityAdjust(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppet-roles/controller"]
	if entry.EntrySlug == "" {
		t.Fatal("Controller entry not found in FoundationEntries")
	}

	// No sub-choices made yet: neither group has a matching pick, so
	// nothing is added.
	pick := puppetFoundationPick{Entry: entry}
	sets, deltas := puppetFoundationAbilityAdjust(pick)
	if len(sets) != 0 || len(deltas) != 0 {
		t.Fatalf("with no sub-choices, got sets=%v deltas=%v, want both empty", sets, deltas)
	}

	// Both picks made: group 0 (+2, Floor 10) chose "con" (baseline would be
	// low), group 1 (+1, no floor) chose "wis" — "Negative scores chosen
	// become 10 before the +2 is added" is Floor:10 on group 0 only.
	pick.SubChoices = []charstore.CompanionUpgradeChoice{
		{ID: 1, ChoiceSlug: "con"},
		{ID: 2, ChoiceSlug: "wis"},
	}
	sets, deltas = puppetFoundationAbilityAdjust(pick)
	if sets["con"] != 10 {
		t.Errorf("sets[con] = %d, want 10 (the +2 group's own Floor)", sets["con"])
	}
	if deltas["con"] != 2 {
		t.Errorf("deltas[con] = %d, want 2", deltas["con"])
	}
	if deltas["wis"] != 1 {
		t.Errorf("deltas[wis] = %d, want 1 (the +1 group, no floor)", deltas["wis"])
	}
	if _, ok := sets["wis"]; ok {
		t.Errorf("sets[wis] should be unset (group 1 has no Floor), got %v", sets)
	}
}

// TestPuppetFoundationAbilityAdjustSet covers Supporter's "Intelligence or
// Wisdom start at 10" — Set, not Delta: the chosen ability floors to 10
// with nothing added on top.
func TestPuppetFoundationAbilityAdjustSet(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppet-roles/supporter"]
	pick := puppetFoundationPick{
		Entry:      entry,
		SubChoices: []charstore.CompanionUpgradeChoice{{ID: 1, ChoiceSlug: "wis"}},
	}
	sets, deltas := puppetFoundationAbilityAdjust(pick)
	if sets["wis"] != 10 {
		t.Errorf("sets[wis] = %d, want 10", sets["wis"])
	}
	if deltas["wis"] != 0 {
		t.Errorf("deltas[wis] = %d, want 0 (Set, not Delta)", deltas["wis"])
	}
	// Supporter's fixed AbilityDeltas["con"] = 2 always applies regardless
	// of the sub-choice.
	if deltas["con"] != 2 {
		t.Errorf("deltas[con] = %d, want 2 (fixed, not choice-gated)", deltas["con"])
	}
}

// TestPuppetFoundationSubChoiceSpecMixedKinds covers Specialized, the one
// entry with 3 different KINDS of sub-choice under a single pick (ability,
// size, weapon damage type) — every option's slug must carry a kind prefix
// so handlePuppetUpgradeChoiceAdd's own per-kind cap can tell them apart.
func TestPuppetFoundationSubChoiceSpecMixedKinds(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppeteer-chassis/specialized"]
	spec, ok := puppetFoundationSubChoiceSpec(entry)
	if !ok {
		t.Fatal("expected a sub-choice spec for Specialized")
	}
	if spec.Quantity != 3 {
		t.Errorf("Quantity = %d, want 3 (1 ability + 1 size + 1 weapon damage type)", spec.Quantity)
	}
	kinds := map[string]bool{}
	for _, o := range spec.FixedOptions {
		prefix, _, ok := cutColon(o.Slug)
		if !ok {
			t.Errorf("option slug %q has no kind prefix, want one (Specialized mixes kinds)", o.Slug)
			continue
		}
		kinds[prefix] = true
	}
	for _, want := range []string{"ability", "size", "weapon"} {
		if !kinds[want] {
			t.Errorf("no %q-prefixed option found among %+v", want, spec.FixedOptions)
		}
	}
}

// TestPuppetFoundationSubChoiceSpecSingleKind covers the common case
// (Controller: 2 ability-only picks) — no prefix needed since there's only
// one kind of choice under the pick at all.
func TestPuppetFoundationSubChoiceSpecSingleKind(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppet-roles/controller"]
	spec, ok := puppetFoundationSubChoiceSpec(entry)
	if !ok {
		t.Fatal("expected a sub-choice spec for Controller")
	}
	if spec.Quantity != 2 {
		t.Errorf("Quantity = %d, want 2", spec.Quantity)
	}
	for _, o := range spec.FixedOptions {
		if _, _, ok := cutColon(o.Slug); ok {
			t.Errorf("option slug %q has a kind prefix, want none (Controller is ability-only)", o.Slug)
		}
	}
}

// TestPuppetFoundationExpectedAbilityScores reproduces the audit's live
// scenario: Quadrupedal (Puppeteer Chassis) sets STR/CON to 16, floored
// against the class baseline — an ability the pick doesn't touch (DEX) must
// pass through the baseline untouched.
func TestPuppetFoundationExpectedAbilityScores(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppeteer-chassis/quadrupedal"]
	if entry.EntrySlug == "" {
		t.Fatal("Quadrupedal entry not found in FoundationEntries")
	}
	baseline := &puppetToolStatBlock{Str: 8, Dex: 12, Con: 8, Int: 3, Wis: 8, Cha: 6}
	picks := []puppetFoundationPick{{Entry: entry}}

	got := puppetFoundationExpectedAbilityScores(baseline, picks)
	if got["str"] != 16 {
		t.Errorf("str = %d, want 16 (Quadrupedal's own Set, baseline 8 is lower)", got["str"])
	}
	if got["con"] != 16 {
		t.Errorf("con = %d, want 16", got["con"])
	}
	if got["dex"] != 12 {
		t.Errorf("dex = %d, want 12 (untouched, baseline passes through)", got["dex"])
	}

	if got := puppetFoundationExpectedAbilityScores(nil, picks); got != nil {
		t.Errorf("nil baseline should yield a nil map, got %v", got)
	}
}

// TestPuppetFoundationWeaponAttackUsesExpectedScore reproduces the audit's
// core defect: the natural weapon's own attack/damage modifier must come
// from the CORRECTED (Foundation-adjusted) ability score, not a stale value
// — Quadrupedal's Bite (DamageAbility "str", AddProficiency true) at STR 16
// (mod +3) with a level-20 Puppet Master's +9 proficiency bonus.
func TestPuppetFoundationWeaponAttackUsesExpectedScore(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppeteer-chassis/quadrupedal"]
	pick := puppetFoundationPick{Entry: entry}

	// The stale/pre-sync scenario: STR 16 corrected, but nothing in the
	// expectedScores map claims a LOWER stored score — this is exactly what
	// the audit reproduced (companion.Str stuck at 15, mod +2, versus the
	// corrected mod +3 the Foundation pick actually grants).
	row := puppetFoundationWeaponAttack(map[string]int{"str": 16}, 9, pick)
	if row == nil {
		t.Fatal("Quadrupedal has a NaturalWeapon (Bite); got nil row")
	}
	if row.AttackTotal != 12 {
		t.Errorf("AttackTotal = %d, want 12 (str mod +3 from the corrected score, + prof +9)", row.AttackTotal)
	}
	if row.DamageTotal != 12 {
		t.Errorf("DamageTotal = %d, want 12 (str mod +3, + prof +9 since Quadrupedal's Bite has AddProficiency)", row.DamageTotal)
	}

	// An ability key absent from expectedScores (e.g. a nil map, same as a
	// nil baseline upstream) must fall back to a flat 0 modifier, not a
	// bogus AbilityModifier(0) == -5 from treating the zero value as a real
	// score — the proficiency bonus alone should remain.
	row = puppetFoundationWeaponAttack(nil, 9, pick)
	if row.AttackTotal != 9 || row.DamageTotal != 9 {
		t.Errorf("with no expected score at all, AttackTotal/DamageTotal = %d/%d, want 9/9 (no ability modifier, not -5)", row.AttackTotal, row.DamageTotal)
	}
}

// TestPuppetFoundationSize covers the level-gated escalation (Ogre: Large
// at 2nd, Huge at 14th) on top of a fixed base size.
func TestPuppetFoundationSize(t *testing.T) {
	entry := puppetupgrades.FoundationEntries["class/puppet-master/option/puppet-weapon-types/ogre-weapon"]
	pick := puppetFoundationPick{Entry: entry}
	if got := puppetFoundationSize(pick, 10); got != "Large" {
		t.Errorf("size at level 10 = %q, want Large", got)
	}
	if got := puppetFoundationSize(pick, 14); got != "Huge" {
		t.Errorf("size at level 14 = %q, want Huge", got)
	}
}

// cutColon is a small test-only helper mirroring the "kind:value" prefix
// parsing puppetFoundationSubChoiceValues/handlePuppetUpgradeChoiceAdd both
// do inline via strings.CutPrefix/strings.Cut.
func cutColon(s string) (prefix, rest string, ok bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return "", s, false
}

// seedFrameworkJutsu inserts one minimal jutsu row of the given
// classification/rank — rulesDB carries no jutsu content of its own in
// tests (unlike dist/rules.db, that table is populated by a separate
// ingest tool, not the schema migrations testServer applies), so every
// Shade/Spellblade Framework test seeds exactly the ranks/classifications
// it needs.
func seedFrameworkJutsu(t *testing.T, s *server, slug, classification, rank string) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES (?, ?, ?, ?, '1 Action', '30 feet', 'Instant', 'HS', '', '', '')`,
		slug, slug, classification, rank,
	); err != nil {
		t.Fatal(err)
	}
}

// TestPuppetFrameworkJutsuOptionsLevelGating covers Shade/Spellblade
// Framework's own "1 D-Rank jutsu at 2nd level, 1 C-Rank at 10th, 1 B-Rank
// at 17th" clause: slot 0 (D) is open as soon as the character has the
// Puppet Master class at all, slot 1 (C) needs level 10, slot 2 (B) needs
// level 17 — a still-locked slot returns nil, nil rather than an empty (but
// technically open) list.
func TestPuppetFrameworkJutsuOptionsLevelGating(t *testing.T) {
	cases := []struct {
		level               int
		wantD, wantC, wantB bool
	}{
		{1, true, false, false},
		{9, true, false, false},
		{10, true, true, false},
		{16, true, true, false},
		{17, true, true, true},
		{20, true, true, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("level_%d", c.level), func(t *testing.T) {
			s := testServer(t)
			if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Framework Jutsu Test')`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(
				`INSERT INTO character_classes (character_id, class_slug, levels, order_index)
				 VALUES (1, 'class/puppet-master', ?, 0)`, c.level,
			); err != nil {
				t.Fatal(err)
			}
			seedFrameworkJutsu(t, s, "jutsu/test-genjutsu-d", "Genjutsu", "D")
			seedFrameworkJutsu(t, s, "jutsu/test-genjutsu-c", "Genjutsu", "C")
			seedFrameworkJutsu(t, s, "jutsu/test-genjutsu-b", "Genjutsu", "B")
			for slot, want := range []bool{c.wantD, c.wantC, c.wantB} {
				opts, err := shadeFrameworkJutsuOptions(s, 1, 1, slot)
				if err != nil {
					t.Fatalf("shadeFrameworkJutsuOptions(slot %d): %v", slot, err)
				}
				if got := len(opts) > 0; got != want {
					t.Errorf("level %d, slot %d: got options=%v (len %d), want present=%v", c.level, slot, got, len(opts), want)
				}
			}
		})
	}
}

// TestPuppetFrameworkJutsuOptionsClassification confirms Shade filters to
// Genjutsu and Spellblade to Ninjutsu — the one difference between the two
// resolver functions, both backed by the same puppetFrameworkJutsuOptions.
func TestPuppetFrameworkJutsuOptionsClassification(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Framework Jutsu Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		 VALUES (1, 'class/puppet-master', 20, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	seedFrameworkJutsu(t, s, "jutsu/test-genjutsu-d", "Genjutsu", "D")
	seedFrameworkJutsu(t, s, "jutsu/test-ninjutsu-d", "Ninjutsu", "D")
	seedFrameworkJutsu(t, s, "jutsu/test-taijutsu-d", "Taijutsu", "D")

	shadeOpts, err := shadeFrameworkJutsuOptions(s, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(shadeOpts) == 0 {
		t.Fatal("shadeFrameworkJutsuOptions returned no D-Rank Genjutsu options")
	}
	for _, o := range shadeOpts {
		var classification string
		if err := s.rulesDB.QueryRow(`SELECT classification FROM jutsu WHERE slug = ?`, o.Slug).Scan(&classification); err != nil {
			t.Fatal(err)
		}
		if classification != "Genjutsu" {
			t.Errorf("shadeFrameworkJutsuOptions returned %q, classification %q, want Genjutsu", o.Slug, classification)
		}
	}

	spellbladeOpts, err := spellbladeFrameworkJutsuOptions(s, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(spellbladeOpts) == 0 {
		t.Fatal("spellbladeFrameworkJutsuOptions returned no D-Rank Ninjutsu options")
	}
	for _, o := range spellbladeOpts {
		var classification string
		if err := s.rulesDB.QueryRow(`SELECT classification FROM jutsu WHERE slug = ?`, o.Slug).Scan(&classification); err != nil {
			t.Fatal(err)
		}
		if classification != "Ninjutsu" {
			t.Errorf("spellbladeFrameworkJutsuOptions returned %q, classification %q, want Ninjutsu", o.Slug, classification)
		}
	}
}

// TestPuppetFrameworkJutsuOptionsOutOfRange confirms an out-of-range slot
// index (defensive — the picker UI never offers more than Quantity: 3
// slots) is treated as locked rather than panicking on the fixed-size
// puppetFrameworkJutsuRankBySlot/puppetFrameworkJutsuMinLevelBySlot arrays.
func TestPuppetFrameworkJutsuOptionsOutOfRange(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Framework Jutsu Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		 VALUES (1, 'class/puppet-master', 20, 0)`,
	); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []int{-1, 3, 4} {
		opts, err := shadeFrameworkJutsuOptions(s, 1, 1, slot)
		if err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
		if opts != nil {
			t.Errorf("slot %d: got %v, want nil", slot, opts)
		}
	}
}
