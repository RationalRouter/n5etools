package main

import "testing"

// The description strings below are copied verbatim out of rules.db
// (SELECT description FROM class_option_entries WHERE class_option_slug LIKE
// 'class/puppet-master/%' AND description LIKE '%Prerequisite%') — the
// parser's whole job is coping with the book's prose as it actually reads,
// run-on sentences and all.
func TestParsePuppetUpgradePrereq(t *testing.T) {
	upgradeNames := puppetUpgradeSortedNames([]string{
		"Chakra Blast", "Heavy Plating", "Adaptive Camouflage", "Deep-Learning Analysis",
		"Entrapment Mechanism", "Chakra Draining Trap", "Chakra Sealing Trap", "Iron Maiden",
		"Poison Mist Hell", "Armory: Needle Wave", "Thread Weapon", "Antagonistic Connection",
		"Forceful Suppression", "Spider’s Web", "Judgement Chains", "Mechanical Light Shield Block",
		"Grappling Threads", "Autobot", "Transforming Apparatus", "Paladin", "Power Fist",
		"Chakra Regulators",
	})
	chassisNames := puppetUpgradeSortedNames([]string{"Weaved Mail", "Wooden Suit", "Iron Shell", "Steel Fortress"})

	parse := func(desc string) []puppetUpgradePrereqGroup {
		return parsePuppetUpgradePrereq(desc, upgradeNames, chassisNames)
	}

	t.Run("bare single name, no punctuation before the run-on flavor text", func(t *testing.T) {
		desc := "Techniques: Purple Prerequisite: Chakra Blast You install one of two unique augments to your Chakra Blast that allow it to incapacitate your foes."
		groups := parse(desc)
		if len(groups) != 1 || groups[0].Kind != "upgrade" || len(groups[0].Names) != 1 || groups[0].Names[0] != "Chakra Blast" {
			t.Fatalf("Incapacitating Blasts: got %+v, want one group [Chakra Blast]", groups)
		}
	})

	t.Run("two separate Prerequisite: sentences AND together", func(t *testing.T) {
		desc := "Prerequisite: Heavy Plating You have upgraded your armor to the point that techniques that would have turned it to dust, now barely scratch its surface. Prerequisite: Deep-Learning Analysis There is no system that can outsmart the hacking design of your armor."
		groups := parse(desc)
		if len(groups) != 2 {
			t.Fatalf("Full Metal Jacket: got %d groups, want 2: %+v", len(groups), groups)
		}
		if groups[0].Names[0] != "Heavy Plating" || groups[1].Names[0] != "Deep-Learning Analysis" {
			t.Errorf("Full Metal Jacket: got %+v, want [Heavy Plating] and [Deep-Learning Analysis]", groups)
		}
	})

	t.Run("slash-separated OR", func(t *testing.T) {
		desc := "Prerequisite: Chakra Draining Trap/Chakra Sealing Trap You learn more about the secret techniques from Sunagakure and begin to integrate your Puppet Tool with your new knowledge."
		groups := parse(desc)
		if len(groups) != 1 || len(groups[0].Names) != 2 {
			t.Fatalf("Black Iron: got %+v, want one OR-group of 2", groups)
		}
	})

	t.Run("named prerequisite phrased as You must have X", func(t *testing.T) {
		desc := "Prerequisite: You must have Entrapment Mechanism on one of your Puppet Tools."
		groups := parse(desc)
		if len(groups) != 1 || groups[0].Names[0] != "Entrapment Mechanism" {
			t.Fatalf("Iron Maiden: got %+v, want [Entrapment Mechanism]", groups)
		}
	})

	t.Run("AND plus a one-of-the-following OR group", func(t *testing.T) {
		desc := "Prerequisites: Antagonistic Connection and one of the following: Forceful Suppression, Spider’s Web, Judgement Chains While you have a creature restrained by your threads you can force them to atone."
		groups := parse(desc)
		if len(groups) != 2 {
			t.Fatalf("Dead Mans Atonement: got %d groups, want 2: %+v", len(groups), groups)
		}
		if len(groups[0].Names) != 1 || groups[0].Names[0] != "Antagonistic Connection" {
			t.Errorf("Dead Mans Atonement group 1 = %+v, want [Antagonistic Connection]", groups[0])
		}
		if len(groups[1].Names) != 3 {
			t.Errorf("Dead Mans Atonement group 2 = %+v, want 3 alternatives", groups[1])
		}
	})

	t.Run("chassis prerequisite, single chassis", func(t *testing.T) {
		desc := "Techniques: Purple Prerequisite: Your Juggernaut Armor must be a Steel Fortress. You have created something truly special."
		groups := parse(desc)
		if len(groups) != 1 || groups[0].Kind != "chassis" || groups[0].Names[0] != "Steel Fortress" {
			t.Fatalf("Mech Pilot: got %+v, want chassis [Steel Fortress]", groups)
		}
	})

	t.Run("chassis prerequisite, multiple alternatives with articles", func(t *testing.T) {
		desc := "Prerequisite: Your Juggernaut Armor must be Weaved Mail, a Wooden Suit, or an Iron Shell."
		groups := parse(desc)
		if len(groups) != 1 || groups[0].Kind != "chassis" || len(groups[0].Names) != 3 {
			t.Fatalf("Phase Suit: got %+v, want chassis OR-group of 3", groups)
		}
	})

	t.Run("ability-score and exclusion clauses are dropped, not inverted", func(t *testing.T) {
		desc := "Prerequisite: Puppet Tool Constitution score of 16+, Puppet Tool cannot have Mechanical Light Shield Block."
		groups := parse(desc)
		if len(groups) != 0 {
			t.Fatalf("Iron Fortress: got %+v, want no enforced groups (neither clause is a positive named requirement this parser understands)", groups)
		}
	})

	t.Run("unrecognized referenced name (not in rules.db under this spelling) is dropped", func(t *testing.T) {
		desc := "Prerequisite: Improved Architecture I (Blue) You improve upon the design of your Puppet Tool with pure innovation."
		groups := parse(desc)
		if len(groups) != 0 {
			t.Fatalf("Kill Command: got %+v, want none — \"Improved Architecture I\" isn't a real known name", groups)
		}
	})

	t.Run("no Prerequisite marker at all", func(t *testing.T) {
		if groups := parse("A simple upgrade with no strings attached."); len(groups) != 0 {
			t.Fatalf("got %+v, want none", groups)
		}
	})
}

func TestPuppetUpgradePrereqMet(t *testing.T) {
	groups := []puppetUpgradePrereqGroup{
		{Kind: "upgrade", Names: []string{"Chakra Blast"}},
	}
	if puppetUpgradePrereqMet(groups, map[string]bool{}, "") {
		t.Error("no picks at all should not satisfy a Chakra Blast requirement")
	}
	if !puppetUpgradePrereqMet(groups, map[string]bool{"chakra blast": true}, "") {
		t.Error("having Chakra Blast should satisfy its own requirement, case-insensitively")
	}

	orGroup := []puppetUpgradePrereqGroup{
		{Kind: "upgrade", Names: []string{"Chakra Draining Trap", "Chakra Sealing Trap"}},
	}
	if !puppetUpgradePrereqMet(orGroup, map[string]bool{"chakra sealing trap": true}, "") {
		t.Error("either alternative in an OR-group should satisfy it")
	}

	chassisGroup := []puppetUpgradePrereqGroup{{Kind: "chassis", Names: []string{"Steel Fortress"}}}
	if puppetUpgradePrereqMet(chassisGroup, nil, "Iron Shell") {
		t.Error("wrong chassis should not satisfy a chassis requirement")
	}
	if !puppetUpgradePrereqMet(chassisGroup, nil, "Steel Fortress") {
		t.Error("matching chassis should satisfy the requirement")
	}

	andGroups := []puppetUpgradePrereqGroup{
		{Kind: "upgrade", Names: []string{"Antagonistic Connection"}},
		{Kind: "upgrade", Names: []string{"Forceful Suppression", "Spider’s Web", "Judgement Chains"}},
	}
	partial := map[string]bool{"antagonistic connection": true}
	if puppetUpgradePrereqMet(andGroups, partial, "") {
		t.Error("AND across groups: having only the first group's requirement should not be enough")
	}
	partial["spider’s web"] = true
	if !puppetUpgradePrereqMet(andGroups, partial, "") {
		t.Error("AND across groups: satisfying both groups (one via its OR) should pass")
	}
}

// puppetUpgradeMaxTakes: everything defaults to a single pick per companion
// except Piercing Chakra, the sole upgrade whose own text allows a second
// pick on the SAME Puppet Tool ("A single Puppet can only acquire this
// upgrade twice").
func TestPuppetUpgradeMaxTakes(t *testing.T) {
	if got := puppetUpgradeMaxTakes("class/puppet-master/option/magus-upgrades/wood-tier/entry/piercing-chakra"); got != 2 {
		t.Errorf("Piercing Chakra max takes = %d, want 2", got)
	}
	if got := puppetUpgradeMaxTakes("class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/ghillie-coating"); got != 1 {
		t.Errorf("Ghillie Coating max takes = %d, want 1", got)
	}
	if got := puppetUpgradeMaxTakes(elementalReactorEntrySlug); got != 5 {
		t.Errorf("Elemental Reactor max takes = %d, want 5 (one per element)", got)
	}
	// Stonecold Stronghold: base (Bronze) + Silver + Gold = 3 total takes.
	if got := puppetUpgradeMaxTakes("class/puppet-master/option/armorers-upgrades/bronze-tier/entry/stonecold-stronghold"); got != 3 {
		t.Errorf("Stonecold Stronghold max takes = %d, want 3", got)
	}
	// Armory: Explosive Launcher: base (Wood) + Bronze/Silver/Gold/Platinum = 5.
	if got := puppetUpgradeMaxTakes("class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/armory-explosive-launcher"); got != 5 {
		t.Errorf("Armory: Explosive Launcher max takes = %d, want 5", got)
	}
}
