package textentries

import "testing"

// Real shape from Puppet Master's Black Iron Upgrades / Wood Tier (queried
// directly from the shipped rules.db) — the exact motivating case for this
// package's ingest-time reuse (see internal/store/classoptionentries.go).
func TestFindEntriesSplitsBundledCapsTier(t *testing.T) {
	raw := "CHAKRA DISRUPTION BLADE Techniques: Black, Perfect You fit your Puppet Tool with multiple small compartments of Black Iron Sand. HIDDEN BLADES Techniques: Black, Perfect You install blades within your Puppet."
	entries := FindEntries(raw)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if name := raw[entries[0].NameStart:entries[0].NameEnd]; name != "CHAKRA DISRUPTION BLADE" {
		t.Errorf("entry 0 name = %q", name)
	}
	if name := raw[entries[1].NameStart:entries[1].NameEnd]; name != "HIDDEN BLADES" {
		t.Errorf("entry 1 name = %q", name)
	}
	if entries[0].Kind != EntryKindCaps || entries[1].Kind != EntryKindCaps {
		t.Errorf("kinds = %v, %v, want both EntryKindCaps", entries[0].Kind, entries[1].Kind)
	}
}

// The untiered "Puppet Weapon Types" list (Drone/Ogre/Sentinel) is already
// one row per option in class_options — a single option's own description
// text has no second bundled name to find, so FindEntries must return fewer
// than 2 matches (the caller's own >=2 threshold is what actually decides
// not to split, but the underlying detection must not manufacture a second
// entry out of thin air either).
func TestFindEntriesSingleOptionDoesNotOverSplit(t *testing.T) {
	raw := "Drone Weapon. Your Puppet Tool gains a ranged natural weapon that deals 1d8 piercing damage."
	entries := FindEntries(raw)
	if len(entries) > 1 {
		t.Errorf("got %d entries for a single-option field, want at most 1: %+v", len(entries), entries)
	}
}

// Real shape from Puppet Master's Black Iron Upgrades / Gold Tier (queried
// directly from the shipped rules.db) — "Triple Iron Maiden"'s body used to
// swallow "A Thousand Hands Manipulation Force" whole, because the leading
// single-letter "A" broke capsEntryPattern's anchor. See capsEntryPattern's
// doc comment for the full root-cause explanation.
func TestFindEntriesSplitsCapsHeaderStartingWithA(t *testing.T) {
	raw := "Restrained Aberrations, Demons, Monstrosities, or Undead have disadvantage on all rolls to break free from your Puppet Tool's Entrapment Mechanism. A THOUSAND HANDS MANIPULATION FORCE Techniques: Black You have placed an incredible amount of seals in a secret compartment on one part of your Puppet Tool."
	entries := FindEntries(raw)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if name := raw[entries[0].NameStart:entries[0].NameEnd]; name != "THOUSAND HANDS MANIPULATION FORCE" {
		t.Errorf("entry 0 name = %q", name)
	}
	if entries[0].Kind != EntryKindCaps {
		t.Errorf("kind = %v, want EntryKindCaps", entries[0].Kind)
	}
}

// Real shape from Puppet Master's Armorer's Upgrades / Gold Tier (queried
// directly from the shipped rules.db) — "Full Metal Jacket"'s body used to
// swallow "Infiltrator" whole, because a trailing ")" landed between the
// sentence-ending period and the required space: "...armor remains
// intact.) INFILTRATOR Techniques: ..." — the anchor's [.!?;] class only
// ever expected the very next byte to be that space.
func TestFindEntriesSplitsCapsHeaderAfterTrailingParen(t *testing.T) {
	raw := "FULL METAL JACKET Techniques: Purple Prerequisite: Heavy Plating You have upgraded your armor to the point that techniques that would have turned it to dust, now barely scratch its surface. Your AC cannot be lowered in any way from hostile effects and jutsu, features, or effects that would destroy your armor are ineffective (your armor remains intact.) INFILTRATOR Techniques: Purple Prerequisite: Deep-Learning Analysis There is no system that can outsmart the hacking design of your armor."
	entries := FindEntries(raw)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if name := raw[entries[0].NameStart:entries[0].NameEnd]; name != "FULL METAL JACKET" {
		t.Errorf("entry 0 name = %q", name)
	}
	if name := raw[entries[1].NameStart:entries[1].NameEnd]; name != "INFILTRATOR" {
		t.Errorf("entry 1 name = %q, want INFILTRATOR", name)
	}
}

// Real shape from Puppet Master's Puppeteer Chassis (Black Technique, queried
// directly from the shipped rules.db) — a parenthetical clarifying list ends
// its own sentence with no period before the closing paren at all, so
// neither the bare "[.!?;]" anchor nor its "\)?" suffix (which still
// requires a PRECEDING [.!?;]) matched, and "QUADRUPEDAL"'s whole entry
// swallowed into "SPECIALIZED"'s body.
func TestFindEntriesSplitsCapsHeaderAfterBareParenNoPeriod(t *testing.T) {
	raw := "On a roll of a 17-20, your Puppet Tool inflicts 1 rank of a condition associated with the damage type of its weapon (Bludgeoning=Bruised / Piercing=Weakened (until end of your next turn) / Slashing=Bleeding) QUADRUPEDAL ASI: Strength & Constitution scores become 16. Your Puppet Tool takes on a quadrupedal design."
	entries := FindEntries(raw)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if name := raw[entries[0].NameStart:entries[0].NameEnd]; name != "QUADRUPEDAL" {
		t.Errorf("entry 0 name = %q, want QUADRUPEDAL", name)
	}
}

// Real shape from the same Gold Tier field — "G30" is a real book heading
// (a codename, not a typo) that the caps patterns never matched at all,
// because \p{Lu} only covers letters: a name containing a digit couldn't
// satisfy the "2+ uppercase letters" run the pattern requires anywhere in
// its own name, silently swallowing the whole entry into whatever came
// immediately before it ("Fortified Defenses", in the shipped data).
func TestFindEntriesSplitsCapsHeaderWithDigit(t *testing.T) {
	raw := "Additionally, you have advantage on saving throws that would lower your strength score in any way. G30 Techniques: Purple Your armor is internally coated with a non-Newtonian slime that hardens in response to physical trauma."
	entries := FindEntries(raw)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if name := raw[entries[0].NameStart:entries[0].NameEnd]; name != "G30" {
		t.Errorf("entry 0 name = %q, want G30", name)
	}
}

// Real shape from Puppet Master's Armorer's Upgrades / Wood Tier (queried
// directly from the shipped rules.db) — "ELEMENTAL REACTOR TABLE" (a data
// table's caption, its own tabular data unrecoverable as PDF text the same
// way Armor Chassis's page-image table is) ran straight into the next real
// entry, "FARADAY FACEPLATE", with no punctuation between them — both were
// captured as one 5-word name, "Elemental Reactor Table Faraday Faceplate",
// glued onto Faraday Faceplate's own body text. The caption should be
// dropped, not stored as (part of) any entry's name.
func TestFindEntriesDropsTableCaptionPrefix(t *testing.T) {
	raw := "You can take this upgrade multiple times, each time selecting a different element. ELEMENTAL REACTOR TABLE FARADAY FACEPLATE Techniques: Purple After casting a Jutsu of D-Rank or higher, until the start of your next turn, you have advantage on saving throws against being charmed, mind controlled, stunned, or dazed from Genjutsu."
	entries := FindEntries(raw)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if name := raw[entries[0].NameStart:entries[0].NameEnd]; name != "FARADAY FACEPLATE" {
		t.Errorf("entry 0 name = %q, want FARADAY FACEPLATE", name)
	}
}

// Real shape from Puppet Master's Armorer's Upgrades / Wood Tier (queried
// directly from the shipped rules.db) — "Chakra Blast" and "Power Fist" both
// grant a "natural weapon" whose own stat block restates the upgrade's own
// name right before it ("...natural weapon, Chakra Blast. This weapon
// counts as... Chakra Blast. Ranged Weapon Attack: ..."), which
// namedEntryPattern misread as a SECOND, distinct entry named "Chakra
// Blast" — splitting the real entry into two pieces: the truncated original
// (losing its own attack stat block) and a nameless duplicate holding just
// the stat block. Confirmed live: rules.db shipped with both
// .../wood-tier/entry/chakra-blast and .../wood-tier/entry/chakra-blast-2,
// each named "Chakra Blast" (see LoadClassOptionEntries' own duplicate-name
// suffixing), and the Puppets tab showed the same tile name twice.
func TestFindEntriesDoesNotSplitOnWeaponStatBlockSelfReference(t *testing.T) {
	raw := "CHAKRA BLAST Techniques: Purple, Perfect You infuse your armor with a gauntlet that can conduct intense blasts of chakra. You gain the following natural weapon, Chakra Blast. This weapon counts as any Ranged weapon, any Bow, or Senbon. This weapon can be used as a component in Jutsu. Chakra Blast. Ranged Weapon Attack: Ninjutsu or Taijutsu attack bonus to hit, Range 60ft, one target. Hit: 1d8 + Ninjutsu or Taijutsu ability modifier in force damage. ELEMENTAL REACTOR Techniques: Purple You fit your Puppet Tool with a special elemental reactor."
	entries := FindEntries(raw)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (CHAKRA BLAST and ELEMENTAL REACTOR only): %+v", len(entries), entries)
	}
	for _, e := range entries {
		if name := raw[e.NameStart:e.NameEnd]; name == "Chakra Blast" {
			t.Errorf("the weapon's own restated name before its stat block should not be read as a second entry, got one named %q", name)
		}
	}
}

func TestTitleCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CHAKRA DISRUPTION BLADE", "Chakra Disruption Blade"},
		{"FIRE AND WATER BLASTER", "Fire and Water Blaster"},
		{"ARMORY: NEEDLE WAVE", "Armory: Needle Wave"},
		{"STURDY (RENAMED)", "Sturdy (Renamed)"},
		{"SELF-DESTRUCT", "Self-Destruct"},
	}
	for _, c := range cases {
		if got := TitleCase(c.in); got != c.want {
			t.Errorf("TitleCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
