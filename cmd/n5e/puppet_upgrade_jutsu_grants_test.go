package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

func TestLoadPuppetUpgradeJutsuGrantsLevelFilter(t *testing.T) {
	s := testServer(t)

	got, err := s.loadPuppetUpgradeJutsuGrants(elementalReactorEntrySlug, "Fire", 6)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"jutsu/fire-release-fox-fire": true, "jutsu/fire-release-fire-dragon-bullet": true}
	if len(got) != len(want) {
		t.Fatalf("Fire at level 6 = %v, want %v", got, want)
	}
	for _, slug := range got {
		if !want[slug] {
			t.Errorf("unexpected grant %q at level 6", slug)
		}
	}

	got14, err := s.loadPuppetUpgradeJutsuGrants(elementalReactorEntrySlug, "Fire", 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(got14) != 4 {
		t.Fatalf("Fire at level 14 = %v, want all 4 grants", got14)
	}

	gotNone, err := s.loadPuppetUpgradeJutsuGrants(elementalReactorEntrySlug, "Fire", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotNone) != 0 {
		t.Fatalf("Fire below level 2 = %v, want none", gotNone)
	}

	entangling, err := s.loadPuppetUpgradeJutsuGrants(
		"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads", "", 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(entangling) != 3 {
		t.Fatalf("Entangling Threads at level 9 = %v, want 3 (levels 1/5/9)", entangling)
	}
}

// TestPuppetUpgradeTableGrantedJutsuEndToEnd exercises the full companion
// pick -> grant lookup path: Elemental Reactor needs its own sub-choice,
// Weaponized Jutsu Casting needs a SEPARATE Puppet Weapon Type pick on the
// same companion (not a sub-choice of its own), and a second companion
// picking the same element must not double the result.
func TestPuppetUpgradeTableGrantedJutsuEndToEnd(t *testing.T) {
	s := testServer(t)
	weaponizedSlug := "class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting"
	droneSlug := "class/puppet-master/option/puppet-weapon-types/drone-weapon"

	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Kankuro')`); err != nil {
		t.Fatal(err)
	}

	companion1, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Chibi")
	if err != nil {
		t.Fatal(err)
	}
	reactorPick, err := charstore.AddCompanionUpgrade(s.charDB, 1, companion1, elementalReactorEntrySlug, elementalReactorEntrySlug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionUpgradeChoice(s.charDB, 1, companion1, reactorPick, "Fire"); err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionUpgrade(s.charDB, 1, companion1, weaponizedSlug, weaponizedSlug); err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionUpgrade(s.charDB, 1, companion1, droneSlug, droneSlug); err != nil {
		t.Fatal(err)
	}

	// A second companion also picking Fire must not duplicate the result.
	companion2, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}
	reactorPick2, err := charstore.AddCompanionUpgrade(s.charDB, 1, companion2, elementalReactorEntrySlug, elementalReactorEntrySlug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCompanionUpgradeChoice(s.charDB, 1, companion2, reactorPick2, "Fire"); err != nil {
		t.Fatal(err)
	}

	got, err := s.puppetUpgradeTableGrantedJutsu(1, 14)
	if err != nil {
		t.Fatal(err)
	}

	wantFire := []string{
		"jutsu/fire-release-fox-fire", "jutsu/fire-release-fire-dragon-bullet",
		"jutsu/fire-release-fire-wall", "jutsu/fire-release-great-fire-absorption",
	}
	wantDrone := []string{
		"jutsu/prepared-shot", "jutsu/sealing-art-mark-of-finding",
		"jutsu/kaguras-mind-eye", "jutsu/medical-release-aura-of-power",
	}
	if len(got) != len(wantFire)+len(wantDrone) {
		t.Fatalf("got %d grants, want %d: %v", len(got), len(wantFire)+len(wantDrone), got)
	}
	for _, slug := range append(wantFire, wantDrone...) {
		if label, ok := got[slug]; !ok || label != "Puppet Upgrade" {
			t.Errorf("missing or mislabeled grant %q: %q", slug, label)
		}
	}
}
