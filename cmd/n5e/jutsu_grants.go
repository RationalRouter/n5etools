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

// jutsuGrantNoRankPattern matches a "you learn/gain the [Name] Genjutsu"
// sentence with NEITHER an explicit rank token NOR a level restated in the
// same sentence, e.g. "you learn the Bane Genjutsu, if you do not know it
// already" (Systematic Breakdown, Intelligence Operative). A full-corpus
// grep confirmed this shape recurs well beyond that one instance (several
// Hunter-Nin subclasses' own free-jutsu grants — Necrosis, Weapons of
// Darkness, Shadow Bite, Chakra Mark, Summoning Technique — use the
// identical rankless phrasing), so a shared regex is the right fix here
// rather than a one-off constant for Bane alone.
//
// This pattern is deliberately broader than the other three (no rank token
// required at all), so it also fires on sentences the other patterns
// already handle correctly — for "the Mending E-Rank Ninjutsu", nothing
// stops the non-greedy capture from swallowing "Mending E-Rank" as one
// name, since RE2 has no lookahead to forbid it. rankTokenSuffixPattern
// below discards any match whose captured name ends in an explicit rank
// token, leaving this pattern to only ever contribute genuinely rank-less
// grants that the other three patterns can't reach.
var jutsuGrantNoRankPattern = regexp.MustCompile(
	`[Yy]ou (?:learns?|gains?) (?:the |a )?([A-Z][A-Za-z' -]*?) (?i:Ninjutsu|Genjutsu|Taijutsu|Bukijutsu)`,
)

// rankTokenSuffixPattern matches a jutsuGrantNoRankPattern capture that
// actually ends in an explicit rank token ("... E-Rank") — meaning the
// sentence was already correctly handled by jutsuGrantNoLevelPattern (or is
// the rank-first shape jutsuGrantRankFirstPattern owns), and this broader,
// rank-agnostic match should be discarded rather than added as a second,
// wrongly-named grant.
var rankTokenSuffixPattern = regexp.MustCompile(`(?i)[EDCBAS]-Rank$`)

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
	for _, m := range jutsuGrantNoRankPattern.FindAllStringSubmatch(description, -1) {
		name := strings.TrimSpace(m[1])
		if rankTokenSuffixPattern.MatchString(name) {
			continue // already-ranked sentence; one of the three patterns above owns it
		}
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

// heatMasterFireAccessSlug is Heat Master's "If You Can't Handle the
// Heat..." subclass feature — near-word-for-word identical in shape to
// Water and Oil, Do Mix above, just for a single release: "You gain the
// Fire release Keyword and may learn Fire release Jutsu. If you already
// have access to Fire release Jutsu, you instead learn a number of Fire
// release Jutsu equal to half your Proficiency Bonus."
const heatMasterFireAccessSlug = "class/cooking-nin/group/cooking-focus/heat-master/feature/if-you-cant-handle-the-heat"

// hasNatureReleaseAccess reports whether a character's class or clan already
// lets them learn jutsu carrying the given keyword — the same union
// loadJutsuOrigins uses to badge a jutsu "class" or "clan" on the sheet.
//
// This is deliberately NOT used for the five elemental release keywords
// (Fire/Water/Earth/Wind/Lightning Release) any more — see
// natureReleaseBonusJutsuSlots' own comment for why a discipline-based
// check can't tell "already has access" for those apart from "just gained
// it from this same feature." It stays correct for non-elemental keywords
// like "Medical", which jutsuEligible (jutsu_eligibility.go) never gates
// behind an affinity in the first place — any class whose discipline list
// includes Medical jutsu can already learn them freely, so this broad
// class/clan check is the right (and only) test for those.
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

// natureReleaseBonusJutsuSlots implements the "if you already have access
// to [release] Jutsu, you instead learn a number of [release] Jutsu equal
// to half your Proficiency Bonus" shape shared by Fry Cooks' Water and Oil,
// Do Mix and Heat Master's If You Can't Handle the Heat... — gated on the
// named granting feature being active AND the character already having
// access to at least one of the given keywords from some other source.
//
// For an elemental keyword (Fire/Water/Earth/Wind/Lightning Release), "from
// some other source" means the character's own resolved elemental
// affinities (elemental_affinity.go's characterElementalAffinities — clan
// trait, the Nature Release feat, or a Professor subclass slot), NOT the
// broad classJutsuPredicate-based hasNatureReleaseAccess check: that check
// only tests whether a jutsu of this keyword falls within the character's
// class DISCIPLINE (Ninjutsu/Genjutsu/Taijutsu/Bukijutsu) at all, which for
// any Ninjutsu-casting class is true for nearly every element regardless of
// whether that class or clan actually grants the keyword — confirmed live
// against dist/rules.db: a clanless, featless Cooking-Nin already "passes"
// hasNatureReleaseAccess for Fire/Water/Medical purely because Cooking-Nin
// casts Ninjutsu and the book has plenty of Ninjutsu-classified jutsu
// tagged with each of those keywords. Using that check here would make the
// "gain the keyword fresh, no bonus" branch of both features unreachable
// for any Ninjutsu-casting class — always paying out the bonus regardless
// of whether the character actually had prior access, which is not what
// either feature's text says. Non-elemental keywords ("Medical") have no
// affinity-system equivalent to check, so those still go through the
// broad class/clan check, which is accurate for them (see
// hasNatureReleaseAccess's own comment).
func (s *server) natureReleaseBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug, grantSlug string, keywords []string, proficiencyBonus int) (int, error) {
	has := false
	for _, f := range features {
		if f.Slug == grantSlug {
			has = true
			break
		}
	}
	if !has {
		return 0, nil
	}

	grantedSlugs := make(map[string]bool, len(features))
	for _, f := range features {
		grantedSlugs[f.Slug] = true
	}
	affinities, err := s.characterElementalAffinities(characterID, clanSlug, grantedSlugs)
	if err != nil {
		return 0, err
	}
	affinitySet := make(map[string]bool, len(affinities))
	for _, a := range affinities {
		affinitySet[a.Element] = true
	}

	for _, kw := range keywords {
		if el := elementFromReleaseKeyword(kw); el != "" {
			if affinitySet[el] {
				return proficiencyBonus / 2, nil
			}
			continue
		}
		ok, err := s.hasNatureReleaseAccess(classSlug, clanSlug, kw)
		if err != nil {
			return 0, err
		}
		if ok {
			return proficiencyBonus / 2, nil
		}
	}
	return 0, nil
}

// waterAndOilBonusJutsuSlots returns the bonus known-jutsu count Water and
// Oil, Do Mix adds to JutsuKnownCap — half the character's proficiency
// bonus, rounded down, gated on Water or Medical release access from
// elsewhere. See natureReleaseBonusJutsuSlots.
func (s *server) waterAndOilBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug string, proficiencyBonus int) (int, error) {
	return s.natureReleaseBonusJutsuSlots(characterID, features, classSlug, clanSlug, waterAndOilDoMixSlug, []string{"Water Release", "Medical"}, proficiencyBonus)
}

// heatMasterBonusJutsuSlots returns the bonus known-jutsu count If You
// Can't Handle the Heat... adds to JutsuKnownCap — half the character's
// proficiency bonus, rounded down, gated on Fire release access from
// elsewhere. See natureReleaseBonusJutsuSlots.
func (s *server) heatMasterBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug string, proficiencyBonus int) (int, error) {
	return s.natureReleaseBonusJutsuSlots(characterID, features, classSlug, clanSlug, heatMasterFireAccessSlug, []string{"Fire Release"}, proficiencyBonus)
}

// chakraCellEnhancementFeatureSlug is Science-Nin's base 1st-level feature
// granting bonus known-jutsu slots.
const chakraCellEnhancementFeatureSlug = "class/science-nin/feature/chakra-cell-enhancement"

// chakraCellEnhancementBonusJutsuSlots returns the bonus known-jutsu count
// Chakra Cell Enhancement adds to JutsuKnownCap — the character's own
// Intelligence modifier (never negative), gated purely on having the
// granted feature at all (already level/class gated by
// features.LoadGrantedFeatures, same as waterAndOilBonusJutsuSlots's own
// grant check). Book text: "You learn E-Rank jutsu equal to your
// Intelligence Modifier. You can change these jutsu over the course of a
// rest" — same "flat cap addition, rank not separately enforced"
// simplification waterAndOilBonusJutsuSlots already accepts for its own
// bonus slots: this app's known-jutsu cap has no per-rank sub-limit to
// restrict these bonus slots to E-Rank jutsu specifically against.
func chakraCellEnhancementBonusJutsuSlots(features []grantedFeatureRow, intModifier int) int {
	for _, f := range features {
		if f.Slug == chakraCellEnhancementFeatureSlug {
			if intModifier < 0 {
				return 0
			}
			return intModifier
		}
	}
	return 0
}
