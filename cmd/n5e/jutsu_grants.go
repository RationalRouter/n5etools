package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// jutsuGrantWithLevelPattern matches a class/clan feature sentence that
// names both the level a jutsu is granted at and the jutsu itself, e.g.
// "Starting at 7th level you learn the Summoning Technique D-Rank ninjutsu"
// (clan/hoshigaki/feature/commander-of-the-deep) or "Beginning at 1st
// level, as a Puppet Master you learn the Chakra Hands E-Rank Ninjutsu for
// free" (class/puppet-master/feature/chakra-threads). The gap between
// "level" and "you learn" is bounded rather than open-ended so this can't
// jump across sentence boundaries into an unrelated jutsu mention later in
// the same feature's (often paragraph-long) description.
//
// The verb also matches "gain(s)" alongside "learn(s)": 6 of the 7
// Genjutsu Pledge subclasses' 2nd-level free-jutsu features are printed as
// "you gain the E-Rank Genjutsu X" rather than "you learn" (e.g. "When you
// choose this path at 2nd Level, you gain the E-Rank Genjutsu Transform" —
// class/genjutsu-specialist/.../beguiler/feature/inspired-appearance).
// "gain" alone is far too common in this sourcebook's prose to match
// broadly, so the same tight "-Rank <Discipline> <Name>" shape immediately
// following is still required — only the verb widened, not the specificity.
var jutsuGrantWithLevelPattern = regexp.MustCompile(
	`(?:Beginning|Starting) at (\d+)(?:st|nd|rd|th) level.{0,60}?you (?:learns?|gains?) (?:the |a )?([A-Z][A-Za-z' -]*?) ([EDCBAS])-Rank (?i:Ninjutsu|Genjutsu|Taijutsu|Bukijutsu)`,
)

// jutsuGrantNoLevelPattern matches the same "you learn/gain the X (Rank)
// (School)" shape without a level restated in the same sentence, e.g.
// "You learn the Mending E-Rank Ninjutsu, which does not count against
// your known" (class/puppet-master/feature/puppet-tool) — that grant's
// level is the level the whole feature is gained at, not a separate one.
// See jutsuGrantWithLevelPattern's comment for why "gain(s)?" is also
// matched here, with the same specificity guardrail.
var jutsuGrantNoLevelPattern = regexp.MustCompile(
	`[Yy]ou (?:learns?|gains?) (?:the |a )?([A-Z][A-Za-z' -]*?) ([EDCBAS])-Rank (?i:Ninjutsu|Genjutsu|Taijutsu|Bukijutsu)`,
)

// jutsuGrantRankFirstPattern matches a class/clan feature sentence phrased
// rank-first — "you learn the E-Rank Ninjutsu Enhanced Defense." — as
// opposed to jutsuGrantNoLevelPattern's name-first phrasing ("you learn
// the Enhanced Defense E-Rank Ninjutsu"). Confirmed via a full corpus grep
// of class_features/subclass_features/clan_features: exactly 4 rows use
// this word order, all Puppet Master 6th-level subclass features (Enhanced
// Vision, Battle Pressure, Overture, Combat Alertness) — previously
// silently dropped because neither of the other two patterns matches this
// word order. The name is terminated by the sentence's own period rather
// than a following school keyword, since nothing follows the name here;
// the character class includes ':' for names like "Sealing Art: String
// Light Formation" and "Medical Release: Virtue".
//
// This is also the shape all 6 "gain"-phrased Genjutsu Pledge grants use —
// e.g. "you gain the E-Rank Genjutsu Transform." (Beguiler's Inspired
// Appearance) — see jutsuGrantWithLevelPattern's comment for why
// "gain(s)?" is matched alongside "learn(s)?" here too. Those sentences
// differ from the Puppet Master ones in two ways this pattern also
// accounts for: an optional comma can sit between the school keyword and
// the name ("the E-Rank Genjutsu, Minor Illusion." — Illusionist's Shaping
// Your World), and the name itself is usually followed by a comma rather
// than a period ("the E-Rank Genjutsu Doubt, if you already know this
// Genjutsu..." — Layered Reality's Synchronous Technique), since the
// sentence continues rather than ending there. The name's own character
// class therefore no longer includes ',' (previously harmless only because
// every matched name happened to be comma-free) — comma is now exclusively
// the terminator alternative alongside the sentence-ending period.
var jutsuGrantRankFirstPattern = regexp.MustCompile(
	`[Yy]ou (?:learns?|gains?) (?:the |a )?([EDCBAS])-Rank (?i:Ninjutsu|Genjutsu|Taijutsu|Bukijutsu),? ([A-Z][A-Za-z':\- ]*?)[,.]`,
)

// jutsuGrantMatch is one "you learn X for free" instance pulled out of a
// class or clan feature's description, before the printed name is resolved
// against the rules database.
type jutsuGrantMatch struct {
	Name  string
	Level int
}

// parseJutsuGrants finds every named-jutsu grant in a feature's description.
// fallbackLevel is used when the sentence granting the jutsu doesn't restate
// its own level (i.e. it's granted at the same level the whole feature is);
// jutsuGrantWithLevelPattern is checked first so a sentence that DOES name
// its own level (a jutsu granted later than the feature's base level, as in
// the Hoshigaki case) is never mistakenly attributed to fallbackLevel.
func parseJutsuGrants(description string, fallbackLevel int) []jutsuGrantMatch {
	var out []jutsuGrantMatch
	seen := map[string]bool{}
	for _, m := range jutsuGrantWithLevelPattern.FindAllStringSubmatch(description, -1) {
		level, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		name := strings.TrimSpace(m[2])
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, jutsuGrantMatch{Name: name, Level: level})
	}
	for _, m := range jutsuGrantNoLevelPattern.FindAllStringSubmatch(description, -1) {
		name := strings.TrimSpace(m[1])
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, jutsuGrantMatch{Name: name, Level: fallbackLevel})
	}
	for _, m := range jutsuGrantRankFirstPattern.FindAllStringSubmatch(description, -1) {
		name := strings.TrimSpace(m[2])
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, jutsuGrantMatch{Name: name, Level: fallbackLevel})
	}
	return out
}

// jutsuNameIndex maps a jutsu's printed name (lowercased, whitespace
// collapsed) to its slug, for resolving the plain-English names that
// appear inside feature prose rather than as a rules-database reference.
func (s *server) jutsuNameIndex() (map[string]string, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name FROM jutsu`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := map[string]string{}
	for rows.Next() {
		var slug, name string
		if err := rows.Scan(&slug, &name); err != nil {
			return nil, err
		}
		index[normalizeJutsuGrantName(name)] = slug
	}
	return index, rows.Err()
}

func normalizeJutsuGrantName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// loadGrantedJutsuLabels finds every jutsu a character's class, subclass, or
// clan features grant for free at their current level — "Beginning at 1st
// level, as a Puppet Master you learn the Chakra Hands E-Rank Ninjutsu for
// free" and its like — and returns them as slug -> a short badge label
// ("Class Feature", "Subclass Feature", or "Clan").
//
// Reuses loadGrantedFeatures rather than querying class_features/
// clan_features again: that function already resolves the real level each
// feature is gained at (COALESCE(level_override, level) via v_class_features
// / v_clan_features) and already filters to features the character has
// actually reached. A feature's own level is only the FALLBACK level for a
// jutsu it grants, though — Hoshigaki's Commander of the Deep is itself a
// 1st-level feature whose Summoning Technique grant explicitly starts at
// 7th level, so the two can't be conflated.
func (s *server) loadGrantedJutsuLabels(characterID int64, sheet *charsheet.Sheet) (map[string]string, error) {
	features, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	if len(features) == 0 {
		return nil, nil
	}
	index, err := s.jutsuNameIndex()
	if err != nil {
		return nil, err
	}

	labels := map[string]string{}
	for _, f := range features {
		label := ""
		switch {
		case strings.HasPrefix(f.SourceLabel, "Class:"):
			label = "Class Feature"
		case strings.HasPrefix(f.SourceLabel, "Subclass:"):
			label = "Subclass Feature"
		case strings.HasPrefix(f.SourceLabel, "Racial:"):
			label = "Clan"
		default:
			continue
		}
		for _, grant := range parseJutsuGrants(f.Description, f.Level) {
			if grant.Level > sheet.Level {
				continue
			}
			slug, ok := index[normalizeJutsuGrantName(grant.Name)]
			if !ok {
				continue // rules update renamed/removed the jutsu — skip rather than invent a row
			}
			if _, exists := labels[slug]; !exists {
				labels[slug] = label
			}
		}
	}
	return labels, nil
}

// jutsuKnownCount counts how many of a character's jutsu count against
// JutsuKnownCap — every row except the free class-feature/clan grants
// loadGrantedJutsuLabels adds, which the book states explicitly don't
// count against what a character knows.
func jutsuKnownCount(jutsu []jutsuSheetRow) int {
	n := 0
	for _, j := range jutsu {
		if j.SourceLabel == "" {
			n++
		}
	}
	return n
}

// waterAndOilDoMixSlug is Fry Cook's "Water and Oil, Do Mix" subclass
// feature: "You gain the Water Nature release and can add Jutsu with this
// Water release Keyword to your Jutsu known list. You also gain the ability
// to learn and cast Medical release Jutsu... If you already have access to
// Water or Medical release Jutsu, you instead learn a combined number of
// Jutsu of these releases equal to half your Proficiency Bonus."
//
// Only the second branch has a number this app can add anywhere: nothing
// here restricts which jutsu a character may add to their known list by
// nature release in the first place (loadJutsuOrigins' "class"/"clan"
// badges are informational, not a gate — the library never blocks a drag),
// so the first branch's "gain access" has no restriction left to lift.
const waterAndOilDoMixSlug = "class/cooking-nin/group/cooking-focus/fry-cooks/feature/water-and-oil-do-mix"

// hasNatureReleaseAccess reports whether a character's class or clan already
// lets them learn jutsu carrying the given keyword — the same union
// loadJutsuOrigins uses to badge a jutsu "class" or "clan" on the sheet.
func (s *server) hasNatureReleaseAccess(classSlug, clanSlug, keyword string) (bool, error) {
	var n int
	err := s.rulesDB.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT slug FROM v_jutsu WHERE `+classJutsuPredicate+`
			UNION
			SELECT jutsu_slug AS slug FROM clan_jutsu WHERE clan_slug = ?
		) j
		JOIN jutsu_keywords jk ON jk.jutsu_slug = j.slug
		WHERE jk.keyword = ?`, classSlug, clanSlug, keyword,
	).Scan(&n)
	return n > 0, err
}

// waterAndOilBonusJutsuSlots returns the bonus known-jutsu count Water and
// Oil, Do Mix adds to JutsuKnownCap — half the character's proficiency
// bonus, rounded down, gated on the feature being active AND the character
// already having Water or Medical release access from some other source
// (their clan's jutsu list, or another class feature). 0 if either doesn't
// hold.
func (s *server) waterAndOilBonusJutsuSlots(features []grantedFeatureRow, classSlug, clanSlug string, proficiencyBonus int) (int, error) {
	has := false
	for _, f := range features {
		if f.Slug == waterAndOilDoMixSlug {
			has = true
			break
		}
	}
	if !has {
		return 0, nil
	}
	water, err := s.hasNatureReleaseAccess(classSlug, clanSlug, "Water Release")
	if err != nil {
		return 0, err
	}
	medical, err := s.hasNatureReleaseAccess(classSlug, clanSlug, "Medical")
	if err != nil {
		return 0, err
	}
	if !water && !medical {
		return 0, nil
	}
	return proficiencyBonus / 2, nil
}
