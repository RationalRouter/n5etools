package main

// puppetUpgradeFeatureBonusTierSlots: a granted subclass feature slug ->
// the extra material-tier Upgrade slots it grants for free, by tier name
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
var puppetUpgradeFeatureBonusTierSlots = map[string]map[string]int{
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/nearly-perfected-architecture": {"Silver Tier": 2},
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design":                 {"Bronze Tier": 1, "Silver Tier": 1, "Gold Tier": 1},
}

// puppetUpgradeFeatureBonusTierSlotsGate: each grant's subclass color and
// minimum Puppet Master level, keyed the same way as
// puppetUpgradeFeatureBonusTierSlots — both entries happen to share level
// 17 today, but are recorded per-feature rather than hardcoded once so a
// future addition isn't forced to share it.
var puppetUpgradeFeatureBonusTierSlotsGate = map[string]struct {
	SubclassColor string
	MinLevel      int
}{
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/nearly-perfected-architecture": {SubclassColor: "Purple", MinLevel: 17},
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design":                 {SubclassColor: "Black", MinLevel: 17},
}

// puppetUpgradeFeatureBonusTierCaps resolves puppetUpgradeFeatureBonusTierSlots
// for the character's current subclass/level — same array shape as
// puppetUpgradeMaterialTierCaps' own return (index 0 unused, index = tier
// rank) so a caller can add this element-wise onto that array, the same way
// puppetFoundationBonusTierCapsFromPicks' result already is.
func puppetUpgradeFeatureBonusTierCaps(subclassColor string, puppetMasterLevel int) [puppetUpgradeTierCount + 1]int {
	var caps [puppetUpgradeTierCount + 1]int
	for featureSlug, tierSlots := range puppetUpgradeFeatureBonusTierSlots {
		gate := puppetUpgradeFeatureBonusTierSlotsGate[featureSlug]
		if gate.SubclassColor != subclassColor || puppetMasterLevel < gate.MinLevel {
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
