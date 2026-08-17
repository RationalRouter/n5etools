package puppetupgrades

// This file is the Armor Chassis property counterpart to bonuses.go: the two
// named chassis properties (Mobile, Powerful Build) that carry a real,
// always-on numeric bonus this app already has a field for, keyed by the
// rules.db puppet_armor_chassis slug of every chassis that carries them —
// read directly against the 4 real chassis rows (migration
// 0031_puppet_armor_chassis.sql), same hand-curated convention as the rest
// of this package: this package never queries rulesDB itself, the caller
// (internal/charsheet, cmd/n5e) resolves which chassis is currently worn and
// looks it up here.
//
// Mobile's own "+1 bonus to Dexterity skill checks and saving throws" clause
// stays reference-only text on the chassis card, matching every other
// skill-check-bonus clause elsewhere in this codebase (none of them have a
// tracked field). Powerful Build's "counts as one size larger for carrying
// capacity" clause is likewise reference-only — this app has no carrying-
// capacity field at all, only max bulk.

// MobileSpeedBonus is the flat Speed bonus a Mobile chassis grants.
const MobileSpeedBonus = 5

// PowerfulBuildBulkBonus is the flat max-bulk bonus a Powerful Build chassis
// grants.
const PowerfulBuildBulkBonus = 10

// PowerfulBuildStrengthCap is the ceiling a Powerful Build chassis's own
// Strength bonus cannot push a character's Strength score past ("increase
// your Strength Score by +2, up to the maximum of 22") — distinct from (and
// lower than) the ordinary ASI-eligible max, since this is an equipment
// bonus, not a permanent score increase.
const PowerfulBuildStrengthCap = 22

// chassisMobileSlugs is every puppet_armor_chassis.slug whose properties
// include Mobile.
var chassisMobileSlugs = map[string]bool{
	"puppet-armor-chassis/weaved-mail": true,
	"puppet-armor-chassis/wooden-suit": true,
}

// chassisPowerfulBuildSlugs is every puppet_armor_chassis.slug whose
// properties include Powerful Build.
var chassisPowerfulBuildSlugs = map[string]bool{
	"puppet-armor-chassis/steel-fortress": true,
}

// ChassisHasMobile reports whether the given worn chassis slug carries the
// Mobile property. A blank slug (nothing worn, or nothing chosen yet)
// reports false.
func ChassisHasMobile(chassisSlug string) bool {
	return chassisMobileSlugs[chassisSlug]
}

// ChassisHasPowerfulBuild reports whether the given worn chassis slug
// carries the Powerful Build property. A blank slug (nothing worn, or
// nothing chosen yet) reports false.
func ChassisHasPowerfulBuild(chassisSlug string) bool {
	return chassisPowerfulBuildSlugs[chassisSlug]
}
