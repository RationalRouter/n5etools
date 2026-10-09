// Package encounter computes an encounter's point budget from party size,
// party level, and desired difficulty, and procedurally builds an adversary
// roster against that budget from the Bingo Book's abstract Rank/Role/
// Trait/Freeform-Slot toolkit (internal/parse.AdversaryRole and friends,
// loaded from rules.db by internal/store/bingobook.go). This is not a
// derivation of any external system's encounter-building math — a simple,
// tunable formula scaled to this project's own adversary_ranks bonuses.
package encounter

// difficultyMultiplier scales the per-player-per-level budget contribution.
var difficultyMultiplier = map[string]int{
	"easy":     2,
	"standard": 3,
	"hard":     4,
	"deadly":   5,
}

// Difficulties lists the valid difficulty values, in the order shown on the
// generator form.
var Difficulties = []string{"easy", "standard", "hard", "deadly"}

// ValidDifficulty reports whether d is one of Difficulties.
func ValidDifficulty(d string) bool {
	_, ok := difficultyMultiplier[d]
	return ok
}

// Budget returns the total point budget available to spend on a roster for
// a party of partySize players at partyLevel, at the given difficulty.
func Budget(partySize, partyLevel int, difficulty string) int {
	mult, ok := difficultyMultiplier[difficulty]
	if !ok {
		mult = difficultyMultiplier["standard"]
	}
	if partySize < 1 {
		partySize = 1
	}
	if partyLevel < 1 {
		partyLevel = 1
	}
	return partySize * partyLevel * mult
}

// rankBaseCost is each rank's budget cost at level 1, mirroring the
// escalating AC/Save/Init bonuses adversary_ranks already assigns each rank
// (Minion has none, Standard is the baseline, Elite and Solo roughly
// double each step up).
var rankBaseCost = map[string]int{
	"minion":   1,
	"standard": 4,
	"elite":    8,
	"solo":     16,
}

// rankCost returns a class's budget cost for an adversary of the given
// level — the base cost scaled by roughly half the level, so a higher-level
// adversary of the same class costs more to field. kind is ClassKind's
// result, so a translated class name never changes the cost.
func rankCost(kind string, level int) int {
	base, ok := rankBaseCost[kind]
	if !ok {
		base = rankBaseCost["standard"]
	}
	scale := level / 2
	if scale < 1 {
		scale = 1
	}
	return base * scale
}
