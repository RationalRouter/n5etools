package main

import (
	"regexp"
	"strconv"
)

// scalingLevelRe mirrors internal/parse's own ordinalLevelRe — a separate
// copy since this one runs at render time over already-stored description
// text, not at ingest time over raw PDF lines.
var scalingLevelRe = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)[ \n]+[Ll]evel`)

// highestScalingLevel returns the highest ordinal level mentioned anywhere
// in description, or 0 if none is found. Many class/clan features in this
// book are gated at one level but keep escalating within their own prose
// at later levels (e.g. Ryu Clan's "Dragon's Rage", gained at 3rd level,
// whose own text describes further scaling at 7th/11th/15th/18th level) —
// callers compare this against the feature's own gate level to flag that
// in a reference popup, without the ingest pipeline needing to split one
// named feature into several rows.
func highestScalingLevel(description string) int {
	matches := scalingLevelRe.FindAllStringSubmatch(description, -1)
	max := 0
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max
}
