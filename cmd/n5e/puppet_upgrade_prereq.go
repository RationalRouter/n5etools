package main

import (
	"regexp"
	"sort"
	"strings"
)

// Puppet Upgrade prerequisites are stored the same way feat prerequisites
// are (see feat_prereq.go's own doc): as the book's own prose, embedded right
// inside the upgrade's own description ("Prerequisite: Chakra Blast You
// install one of two unique augments..."), not a structured field. There is
// no line break between the prerequisite clause and the upgrade's own
// mechanical text in the extracted PDF text, so the two run together.
//
// Same philosophy as feat_prereq.go: a clause this can't confidently resolve
// to a real, known upgrade (or armor chassis) name is dropped, never treated
// as unmet. Hiding an upgrade the player is entitled to take is a bug they
// cannot see or work around; leaving one extra, unlocked tile costs nothing
// (this is exactly why the parser stops the instant it can't match anything
// recognizable, rather than trying to skip ahead past an unparseable clause
// to find something further on — skipping ahead risks matching some OTHER
// upgrade's name mentioned purely descriptively deeper in the same entry's
// own flavor text, e.g. Mech Pilot's own body mentions "Power Fist" by name
// with no prerequisite meaning at all).
//
// This intentionally does NOT try to handle every clause shape found in the
// corpus — only two: "requires upgrade(s) X" (by far the common case) and
// "Juggernaut Armor must be chassis X" (Purple Technique's Phase
// Suit/Mech Pilot/Exploding Puppet Mechanism). Ability-score conditions
// ("Puppet Tool Constitution score of 16+"), exclusions ("cannot have
// Mechanical Light Shield Block"), and one-off references to a name rules.db
// doesn't actually have under that exact spelling ("Improved Architecture I
// (Blue)", "Chakra Sealing Mechanism") are left unenforced for the same
// reason — same tradeoff feat_prereq.go already made, just applied here.

var rePrereqMarker = regexp.MustCompile(`(?i)Prerequisites?:\s*`)

// puppetUpgradeNameEntry pairs a known name with its lowercased form —
// matching is case-insensitive, but the original casing is what gets shown
// back to the player (a "Requires: Chakra Blast" hint, not "requires:
// chakra blast").
type puppetUpgradeNameEntry struct {
	Name  string
	Lower string
}

// puppetUpgradeSortedNames wraps a []string of real names into
// length-descending puppetUpgradeNameEntry values, longest first — the
// scanner below tries candidates in this order so a longer name is never
// mistaken for a shorter one that happens to be its own prefix (no such
// overlap is known to exist in this corpus, but sorting costs nothing and
// removes the need to ever worry about it).
func puppetUpgradeSortedNames(names []string) []puppetUpgradeNameEntry {
	out := make([]puppetUpgradeNameEntry, len(names))
	for i, n := range names {
		out[i] = puppetUpgradeNameEntry{Name: n, Lower: normalizeApostrophes(strings.ToLower(n))}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].Lower) > len(out[j].Lower) })
	return out
}

// normalizeApostrophes maps the curly right-single-quotation-mark (’,
// U+2019) — how an entry's own canonical name spells a possessive, e.g.
// "Spider’s Web" — onto a plain ASCII apostrophe. A different upgrade's
// inline reference to that same name ("Prerequisites: ... Spider's Web
// ...") was extracted with the plain form instead, a real byte-level
// inconsistency confirmed directly against rules.db, not a guess — without
// this, the two spellings never compare equal and the reference silently
// fails to match at all.
func normalizeApostrophes(s string) string {
	return strings.ReplaceAll(s, "’", "'")
}

// puppetUpgradePrereqGroup is one AND'd requirement: satisfied if the
// character has ANY ONE of Names (a bare single-name requirement is just a
// group of one). Kind is "upgrade" or "chassis" — the two vocabularies
// checked against different parts of a companion (its own upgrade picks vs.
// its ArmorChassis field), so a group is never a mix of both.
type puppetUpgradePrereqGroup struct {
	Kind  string
	Names []string
}

func isNameContinuation(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// matchNameAt tries every candidate at the very start of text (after
// trimming leading spaces), longest first, tolerating one bare trailing "s"
// (Resilient Strings' prose says "Thread Weapons", the real entry is "Thread
// Weapon"). A candidate only matches if it ends at a real word boundary —
// otherwise "Thread Weapon" would wrongly match the start of some unrelated
// longer word.
func matchNameAt(text string, names []puppetUpgradeNameEntry) (name, rest string, ok bool) {
	trimmed := strings.TrimLeft(text, " ")
	low := strings.ToLower(trimmed)
	for _, n := range names {
		for _, cand := range [2]string{n.Lower, n.Lower + "s"} {
			if !strings.HasPrefix(low, cand) {
				continue
			}
			after := trimmed[len(cand):]
			if after == "" || !isNameContinuation(after[0]) {
				return n.Name, after, true
			}
		}
	}
	return "", text, false
}

// scanPrereqGroups greedily consumes known upgrade names and the connector
// words joining them ("and", "&", "/", "one of the following: ..."),
// stopping the instant something doesn't match either — which is what
// naturally draws the line between the prerequisite clause and the
// upgrade's own run-on mechanical description, without ever having to
// guess where a sentence "really" ends.
func scanPrereqGroups(text string, names []puppetUpgradeNameEntry) []puppetUpgradePrereqGroup {
	var groups []puppetUpgradePrereqGroup
	rest := text
	for {
		rest = strings.TrimLeft(rest, " ,;")
		low := strings.ToLower(rest)
		switch {
		case strings.HasPrefix(low, "and "):
			rest = rest[len("and "):]
			continue
		case strings.HasPrefix(low, "& "):
			rest = rest[len("& "):]
			continue
		case strings.HasPrefix(low, "you must have "):
			// "You must have Entrapment Mechanism on one of your Puppet
			// Tools." (Iron Maiden) — a full sentence rather than a bare
			// name, but still names a real upgrade. Skipping this filler
			// phrase and falling through to the normal name match below
			// handles it without a separate code path.
			rest = rest[len("you must have "):]
			continue
		case strings.HasPrefix(low, "one of the following:"):
			rest = rest[len("one of the following:"):]
			var alts []string
			for {
				rest = strings.TrimLeft(rest, " ,")
				if strings.HasPrefix(strings.ToLower(rest), "or ") {
					rest = rest[len("or "):]
				}
				name, r2, ok := matchNameAt(rest, names)
				if !ok {
					break
				}
				alts = append(alts, name)
				rest = r2
			}
			if len(alts) > 0 {
				groups = append(groups, puppetUpgradePrereqGroup{Kind: "upgrade", Names: alts})
			}
			continue
		}

		name, r2, ok := matchNameAt(rest, names)
		if !ok {
			return groups
		}
		alts := []string{name}
		rest = r2
		for strings.HasPrefix(rest, "/") {
			name2, r3, ok2 := matchNameAt(rest[1:], names)
			if !ok2 {
				break
			}
			alts = append(alts, name2)
			rest = r3
		}
		groups = append(groups, puppetUpgradePrereqGroup{Kind: "upgrade", Names: alts})
	}
}

// reChassisPrereq matches Purple Technique's own prerequisite shape:
// "Your Juggernaut Armor must be a Steel Fortress." / "...must be Weaved
// Mail, a Wooden Suit, or an Iron Shell." / "your Armor Type must be a Steel
// Fortress." (Exploding Puppet Mechanism's leading "For those who practice
// the Purple technique," clause is simply not matched by this regex at all,
// which is fine — every upgrade on Purple Technique's own list already only
// applies to a Purple Technique puppet in the first place).
var (
	reChassisPrereq = regexp.MustCompile(`(?i)armor(?:\s+type)?\s+must\s+be\s+([^.]+)\.`)
	reChassisSplit  = regexp.MustCompile(`(?i)\s*,\s*|\s+or\s+|\s+and\s+`)
	// A comma-then-"or" boundary ("Suit, or an Iron Shell") only ever gets
	// split on the comma — by the time reChassisSplit reaches "or", the
	// leading space that its own "\s+or\s+" branch requires has already
	// been consumed as part of the comma match, so a trailing "or "/"and "
	// (and any article after it) has to be stripped from each part
	// separately rather than relying on the splitter alone.
	reChassisLeadWord = regexp.MustCompile(`(?i)^(?:or|and)\s+(?:an?\s+)?|^an?\s+`)
)

func scanChassisGroup(text string, chassisNames []puppetUpgradeNameEntry) *puppetUpgradePrereqGroup {
	m := reChassisPrereq.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	var names []string
	for _, part := range reChassisSplit.Split(m[1], -1) {
		part = strings.TrimSpace(reChassisLeadWord.ReplaceAllString(strings.TrimSpace(part), ""))
		low := strings.ToLower(part)
		for _, cn := range chassisNames {
			if cn.Lower == low {
				names = append(names, cn.Name)
				break
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	return &puppetUpgradePrereqGroup{Kind: "chassis", Names: names}
}

// parsePuppetUpgradePrereq reads one upgrade entry's own description and
// returns every AND'd requirement group it can confidently resolve —
// nil for an upgrade with no prerequisite (or one this can't parse at all).
// Every "Prerequisite(s):" marker in the text is scanned independently and
// ANY groups found are combined (Full Metal Jacket has two separate
// "Prerequisite: X" sentences, each gating its own name — both apply).
func parsePuppetUpgradePrereq(description string, upgradeNames, chassisNames []puppetUpgradeNameEntry) []puppetUpgradePrereqGroup {
	description = normalizeApostrophes(description)
	var groups []puppetUpgradePrereqGroup
	for _, loc := range rePrereqMarker.FindAllStringIndex(description, -1) {
		rest := description[loc[1]:]
		if found := scanPrereqGroups(rest, upgradeNames); len(found) > 0 {
			groups = append(groups, found...)
			continue
		}
		if g := scanChassisGroup(rest, chassisNames); g != nil {
			groups = append(groups, *g)
		}
	}
	return groups
}

// puppetUpgradePrereqHintText renders parsed groups back into a short
// human-readable string for the tile's own "Requires: ..." hint — e.g.
// "Chakra Blast", "Chakra Draining Trap or Chakra Sealing Trap", or
// "Antagonistic Connection and one of Forceful Suppression, Spider’s Web,
// Judgement Chains". Chassis groups append "armor chassis" so the hint
// reads as a requirement about the puppet's own chassis, not another
// upgrade.
func puppetUpgradePrereqHintText(groups []puppetUpgradePrereqGroup) string {
	var parts []string
	for _, g := range groups {
		var text string
		switch len(g.Names) {
		case 1:
			text = g.Names[0]
		case 2:
			text = g.Names[0] + " or " + g.Names[1]
		default:
			text = "one of " + strings.Join(g.Names, ", ")
		}
		if g.Kind == "chassis" {
			text += " armor chassis"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " and ")
}

// puppetUpgradePrereqMet reports whether a companion's current upgrade picks
// (by name, e.g. from classOptionNamesBySlug-adjacent lookups — see
// callers) and armor chassis satisfy every group (AND across groups, OR
// within a group's own Names).
func puppetUpgradePrereqMet(groups []puppetUpgradePrereqGroup, haveUpgradeName map[string]bool, chassis string) bool {
	chassisLower := strings.ToLower(chassis)
	for _, g := range groups {
		met := false
		for _, name := range g.Names {
			if g.Kind == "chassis" {
				if strings.ToLower(name) == chassisLower {
					met = true
					break
				}
			} else if haveUpgradeName[strings.ToLower(name)] {
				met = true
				break
			}
		}
		if !met {
			return false
		}
	}
	return true
}

// puppetUpgradeRepeatLimit: a Puppet Upgrade entry slug -> how many times
// ONE companion may take it. Everything defaults to 1 (a plain radio pick,
// once taken, is done). A full sweep of every "take this upgrade
// multiple/a second time" clause across the whole class (re-run after
// discovering Mastered Armament had been missed the first time — its own
// "you can take this upgrade a second time to gain the other option" text
// is the identical shape as Integrated Weapon's, just never wired in) found
// four upgrades that allow more than one take on the SAME Puppet Tool:
// Piercing Chakra ("You can take this upgrade multiple times. A single
// Puppet can only acquire this upgrade twice."), Elemental Reactor
// ("...each time selecting a different element" — capped at 5, one per
// element), Integrated Weapon and Mastered Armament (both "...a second
// time, to gain the other option" — capped at 2; see
// puppetUpgradeUniqueSubChoiceEntries for the per-pick uniqueness check
// this count cap alone doesn't enforce). Every other "multiple times"
// clause in this class turns out to read "once for each Puppet Tool you
// have" (repeatable across DIFFERENT companions, not the same one) —
// already handled for free by this app's own per-companion pick tracking,
// with no entry here needed.
var puppetUpgradeRepeatLimit = map[string]int{
	"class/puppet-master/option/magus-upgrades/wood-tier/entry/piercing-chakra": 2,
	elementalReactorEntrySlug: len(elementNames),
	integratedWeaponEntrySlug: 2,
	masteredArmamentEntrySlug: 2,
}

func init() {
	// Every "At Higher Levels: you may take this upgrade again as a
	// [higher tier]..." entry (puppetUpgradeTierEscalation, below) is also
	// a repeatable entry — 1 base take plus 1 per listed higher tier — so
	// its cap folds into the same map rather than needing a second gate.
	for slug, spec := range puppetUpgradeTierEscalation {
		puppetUpgradeRepeatLimit[slug] = 1 + len(spec.HigherTiers)
	}
}

// puppetUpgradeTierEscalationSpec names the higher-tier Upgrade slots an
// entry can ALSO be taken from, in ascending order — see
// puppetUpgradeTierEscalation's own doc.
type puppetUpgradeTierEscalationSpec struct {
	HigherTiers []string
}

// puppetUpgradeTierEscalation: entries whose own text reads "At Higher
// Levels: you may take this upgrade again using a [higher tier] Upgrade
// slot, for [a bigger/different effect]" — the entry's own Description
// (already pulled in full from rules.db) already spells out what changes
// at each tier, so no separate per-tier effect text is modeled here; this
// map only says WHICH additional tiers the same named entry can be taken
// from. A full read of every Puppet Master upgrade's own text found 6 of
// these, spanning Purple Technique, Blue Technique, AND two class-wide
// lists (Puppet Master Upgrades) — this is not a Purple-Technique-only
// mechanic. loadPuppetUpgradeTiers injects the entry into each of its own
// higher tier's own option list (in addition to its home tier), and
// puppetUpgradeMaxTakes (via the init() above) allows one take per listed
// tier plus the original.
//
//   - Stonecold Stronghold (Bronze, Purple Technique): "may take this
//     upgrade as a Silver tier upgrade... Furthermore, you can take it as
//     a Gold upgrade."
//   - Faraday Faceplate (Wood, Purple Technique — a distinct entry from
//     "Elemental Reactor" elsewhere on this same tier; the two used to ship
//     mis-merged into one entry by a since-fixed ingest bug, see
//     textentries.go's tableCaptionWord): "can take this Upgrade as a
//     Silver tier Upgrade."
//   - Chakra Draining Mechanism (Bronze, class-wide): "using a higher tier
//     Upgrade slot. For each tier after Bronze, increase the chakra
//     drained by 1d6." Open-ended text; the book confirms Silver and Gold
//     by the closing Puppet Upgrades rule's own tier ladder cap (Platinum
//     is the last tier that ladder reaches), so Platinum is included on
//     the same linear reading even though it isn't separately spelled out.
//   - Tag-Team (Bronze, class-wide): "using higher tier Upgrade slots...
//     If taken using a Gold tier Upgrade slot, your Puppets can perform
//     Finishers in place of any allied creature within 15 feet." Platinum
//     included on the same "higher tier slots" open-ended wording as
//     Chakra Draining Mechanism, though the book's own worked examples
//     stop at Gold.
//   - Armory: Explosive Launcher (Wood, class-wide): "using higher tier
//     upgrade slots. For each tier this upgrade is taken at above Wood
//     tier, increase the number of explosive tools this upgrade can hold
//     by +1."
//   - Ingrained Strength (Bronze, Blue Technique): "using higher tier
//     Upgrade slots. For each tier this Upgrade is taken at above Bronze
//     tier, increase the number of conditions you can select by +1."
var puppetUpgradeTierEscalation = map[string]puppetUpgradeTierEscalationSpec{
	"class/puppet-master/option/armorers-upgrades/bronze-tier/entry/stonecold-stronghold": {
		HigherTiers: []string{"Silver Tier", "Gold Tier"},
	},
	"class/puppet-master/option/armorers-upgrades/wood-tier/entry/faraday-faceplate": {
		HigherTiers: []string{"Silver Tier"},
	},
	"class/puppet-master/option/puppet-master-upgrades/bronze-tier/entry/chakra-draining-mechanism": {
		HigherTiers: []string{"Silver Tier", "Gold Tier", "Platinum Tier"},
	},
	"class/puppet-master/option/puppet-master-upgrades/bronze-tier/entry/tag-team": {
		HigherTiers: []string{"Silver Tier", "Gold Tier", "Platinum Tier"},
	},
	"class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/armory-explosive-launcher": {
		HigherTiers: []string{"Bronze Tier", "Silver Tier", "Gold Tier", "Platinum Tier"},
	},
	"class/puppet-master/option/upgrades-of-war/bronze-tier/entry/ingrained-strength": {
		HigherTiers: []string{"Silver Tier", "Gold Tier", "Platinum Tier"},
	},
}

// puppetUpgradeMaxTakes returns how many times one companion may pick the
// given upgrade entry — see puppetUpgradeRepeatLimit.
func puppetUpgradeMaxTakes(entrySlug string) int {
	if n, ok := puppetUpgradeRepeatLimit[entrySlug]; ok {
		return n
	}
	return 1
}
