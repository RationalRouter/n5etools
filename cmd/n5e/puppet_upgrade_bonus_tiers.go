package main

// puppetUpgradeFeatureBonusTierSlots: a granted subclass feature slug, or a
// taken feat's own slug (e.g. "feat/class/puppeteer-training"), -> the extra
// material-tier Upgrade slots it grants for free, by tier name
// (puppetUpgradeTierRanks' own vocabulary) — the character-level sibling of
// puppetFoundationBonusTierCapsFromPicks' per-companion-pick version.
//
// Nearly Perfected Architecture (Purple, L17): "Gain 2 Upgrades of Silver
// tier or lower for free." puppetUpgradeEffectiveCap already lets a higher-
// tier slot fund a lower tier, so "Silver tier or lower" needs no extra
// modeling beyond adding 2 Silver-tier slots.
//
// Elevated Design (Black, L17): "...you gain an additional Bronze, Silver,
// and Gold tier Upgrade." One free slot in each of those three tiers.
//
// Puppeteer Training/Expert/Enthusiast/Specialist (the Puppet Master
// Archetype feat chain): each grants exactly one flat "N Upgrade(s) of
// [tier] or lower" line, the same shape as the two subclass features above,
// with no Puppet Master class-level requirement of its own — see the gate
// map's MinLevel 0 (omitted, the zero value) for these.
//
// Tools to Carry on a Legacy (a plain class feat, prerequisite 4+ levels in
// Puppet Master): bundles FOUR separate grants at THREE different Puppet
// Master levels in one feat ("gain one Wood tier upgrade" immediately, "an
// additional Bronze tier upgrade at 9th Level", "...Silver tier upgrade at
// 14th Level", "...Gold or Platinum tier upgrade at 19th Level") — one
// gate entry only carries a single MinLevel, so this feat is split across 4
// map keys (one real slug + 3 synthetic "#tier-level" suffixes), all
// sharing the same FeatSlug in the gate map for the actual "was this feat
// taken" check. "Gold or Platinum" is modeled as 1 Platinum-tier slot, not
// two separate slots — Platinum outranks Gold in puppetUpgradeTierRanks, and
// puppetUpgradeEffectiveCap already lets a higher tier fund a lower one, so
// a Platinum slot already covers taking it as Gold instead, same "or lower"
// simplification as Nearly Perfected Architecture's Silver-tier grant above.
//
// Master of the White Technique (White, L20): "You gain an extra 2 Puppet
// Upgrades that you qualify for" — an unrestricted-tier grant, modeled the
// same way as Nearly Perfected Architecture's own "Silver tier or lower"
// simplification above, at the top of the ladder (Platinum) so
// puppetUpgradeEffectiveCap's tier-funds-lower-tiers rule covers any tier the
// slots actually get spent on. The same sentence's "double the specified
// ranges of any White Technique Feature, as well as your Chakra Threads"
// clause stays Group 3: no range field exists anywhere in this app for
// Chakra Threads or White Technique features to double.
var puppetUpgradeFeatureBonusTierSlots = map[string]map[string]int{
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/nearly-perfected-architecture": {"Silver Tier": 2},
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design":                 {"Bronze Tier": 1, "Silver Tier": 1, "Gold Tier": 1},
	"class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/master-of-the-white-technique":      {"Platinum Tier": 2},

	"feat/class/puppeteer-training":   {"Wood Tier": 2},
	"feat/class/puppeteer-expert":     {"Bronze Tier": 1},
	"feat/class/puppeteer-enthusiast": {"Silver Tier": 1},
	"feat/class/puppeteer-specialist": {"Gold Tier": 1},

	"feat/class/tools-to-carry-on-a-legacy":           {"Wood Tier": 1},
	"feat/class/tools-to-carry-on-a-legacy#bronze-9":  {"Bronze Tier": 1},
	"feat/class/tools-to-carry-on-a-legacy#silver-14": {"Silver Tier": 1},
	"feat/class/tools-to-carry-on-a-legacy#gold-19":   {"Platinum Tier": 1},
}

// puppetUpgradeFeatureBonusTierSlotsGate: each grant's subclass color,
// minimum Puppet Master level, and (for a feat-driven grant) the feat slug
// that must actually be taken — keyed the same way as
// puppetUpgradeFeatureBonusTierSlots.
//
// SubclassColor empty means "applies regardless of subclass" (every feat-
// driven entry below): a class feature is only ever granted to characters of
// its own subclass, so the two curated subclass-feature entries can gate
// purely on SubclassColor+MinLevel and know that pair is a complete proxy
// for "the character actually has this feature" — nothing else grants
// Nearly Perfected Architecture or Elevated Design. A feat carries no such
// guarantee (any subclass, or none at all yet, can take it), so
// puppetUpgradeFeatureBonusTierCaps treats a blank SubclassColor as a
// wildcard rather than comparing it literally (a literal compare would only
// ever match a character whose own subclassColor also happens to be "").
//
// FeatSlug is the real feat_slug row a feat-driven entry needs present in
// character_feats — unlike SubclassColor+MinLevel, this can't be inferred
// from anything else already available, since taking an optional feat is a
// player choice, not an automatic consequence of level/subclass the way the
// two curated class features are. Left blank (the zero value) for the two
// subclass-feature entries, which need no such check.
var puppetUpgradeFeatureBonusTierSlotsGate = map[string]struct {
	SubclassColor string
	MinLevel      int
	FeatSlug      string
}{
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/nearly-perfected-architecture": {SubclassColor: "Purple", MinLevel: 17},
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design":                 {SubclassColor: "Black", MinLevel: 17},
	"class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/master-of-the-white-technique":      {SubclassColor: "White", MinLevel: 20},

	"feat/class/puppeteer-training":   {FeatSlug: "feat/class/puppeteer-training"},
	"feat/class/puppeteer-expert":     {FeatSlug: "feat/class/puppeteer-expert"},
	"feat/class/puppeteer-enthusiast": {FeatSlug: "feat/class/puppeteer-enthusiast"},
	"feat/class/puppeteer-specialist": {FeatSlug: "feat/class/puppeteer-specialist"},

	"feat/class/tools-to-carry-on-a-legacy":           {FeatSlug: "feat/class/tools-to-carry-on-a-legacy"},
	"feat/class/tools-to-carry-on-a-legacy#bronze-9":  {FeatSlug: "feat/class/tools-to-carry-on-a-legacy", MinLevel: 9},
	"feat/class/tools-to-carry-on-a-legacy#silver-14": {FeatSlug: "feat/class/tools-to-carry-on-a-legacy", MinLevel: 14},
	"feat/class/tools-to-carry-on-a-legacy#gold-19":   {FeatSlug: "feat/class/tools-to-carry-on-a-legacy", MinLevel: 19},
}

// puppetUpgradeFeatureBonusTierCaps resolves puppetUpgradeFeatureBonusTierSlots
// for the character's current subclass/level/taken-feats — same array shape
// as puppetUpgradeMaterialTierCaps' own return (index 0 unused, index = tier
// rank) so a caller can add this element-wise onto that array, the same way
// puppetFoundationBonusTierCapsFromPicks' result already is.
//
// hasFeat is keyed by feat slug (character_feats.feat_slug); a nil/empty map
// is fine and simply means every FeatSlug-gated entry is skipped, which is
// exactly what a character with no matching feat should see.
func puppetUpgradeFeatureBonusTierCaps(subclassColor string, puppetMasterLevel int, hasFeat map[string]bool) [puppetUpgradeTierCount + 1]int {
	var caps [puppetUpgradeTierCount + 1]int
	for featureSlug, tierSlots := range puppetUpgradeFeatureBonusTierSlots {
		gate := puppetUpgradeFeatureBonusTierSlotsGate[featureSlug]
		if gate.SubclassColor != "" && gate.SubclassColor != subclassColor {
			continue
		}
		if puppetMasterLevel < gate.MinLevel {
			continue
		}
		if gate.FeatSlug != "" && !hasFeat[gate.FeatSlug] {
			continue
		}
		for tierName, n := range tierSlots {
			if rank, ok := puppetUpgradeTierRanks[tierName]; ok {
				caps[rank] += n
			}
		}
	}
	return caps
}

// puppetUpgradeFeatureBonusTierCapsForCharacter is
// puppetUpgradeFeatureBonusTierCaps for a caller that hasn't already loaded
// which of the FeatSlug-gated entries' feats the character has taken —
// derives the exact set of feat slugs worth checking from the gate map
// itself (never hardcoded here), so a future FeatSlug-gated entry needs no
// matching update in this function.
func (s *server) puppetUpgradeFeatureBonusTierCapsForCharacter(characterID int64, subclassColor string, puppetMasterLevel int) ([puppetUpgradeTierCount + 1]int, error) {
	hasFeat := map[string]bool{}
	checked := map[string]bool{}
	for _, gate := range puppetUpgradeFeatureBonusTierSlotsGate {
		if gate.FeatSlug == "" || checked[gate.FeatSlug] {
			continue
		}
		checked[gate.FeatSlug] = true
		has, err := s.characterHasFeat(characterID, gate.FeatSlug)
		if err != nil {
			return [puppetUpgradeTierCount + 1]int{}, err
		}
		hasFeat[gate.FeatSlug] = has
	}
	return puppetUpgradeFeatureBonusTierCaps(subclassColor, puppetMasterLevel, hasFeat), nil
}
