package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/features"
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

// jutsuGrantNoRankPattern matches a "you learn/gain/add the [Name] Genjutsu"
// sentence with NEITHER an explicit rank token NOR a level restated in the
// same sentence, e.g. "you learn the Bane Genjutsu, if you do not know it
// already" (Systematic Breakdown, Intelligence Operative). A full-corpus
// grep confirmed this shape recurs well beyond that one instance (several
// Hunter-Nin subclasses' own free-jutsu grants — Necrosis, Weapons of
// Darkness, Shadow Bite, Chakra Mark, Summoning Technique — use the
// identical rankless phrasing), so a shared regex is the right fix here
// rather than a one-off constant for Bane alone.
//
// The verb also matches "add(s)?" alongside "learn(s)?"/"gain(s)?" —
// Scout-Nin's Tajū Kage Bunshin (class/scout-nin/group/scouting-technique/
// cloning-scout/feature/taju-kage-bunshin) is phrased "you add the Shadow
// Clone Technique Ninjutsu to your known jutsu list", rank-less like the
// rest of this pattern's matches, and "add" wasn't previously covered by
// any of the four regexes' verb alternations. A corpus-wide re-scan with
// "adds?" included confirmed this feature is the only new match across
// class_features/subclass_features/clan_features — the one other
// "add...jutsu list" phrasing in the book (clan/kurama/feature/
// genjutsu-conversions) doesn't have a capitalized jutsu name immediately
// following "add", so it still doesn't match.
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
	`[Yy]ou (?:learns?|gains?|adds?) (?:the |a )?([A-Z][A-Za-z' -]*?) (?i:Ninjutsu|Genjutsu|Taijutsu|Bukijutsu)`,
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

// jutsuGrantNameAliases maps a normalized grant shorthand name (as it
// appears inside feature prose) to the jutsu slug it actually refers to,
// for the rare case where the two don't match after normalizeJutsuGrantName
// — a small, individually hand-verified list, not a blanket prefix-strip
// rule, the same discipline internal/parse/subclasses.go's
// knownExtractionSquishes already uses. A blanket "strip any X Release:/X:
// prefix" rule was considered and rejected: a corpus check found 900 of
// 1818 jutsu rows have a colon in their name, many different "X Release:"
// families sharing bare suffixes, so that shortcut risks ambiguous
// collisions across unrelated jutsu.
var jutsuGrantNameAliases = map[string]string{
	// Necrotic Hand's Medical Proficiency (class/hunter-nin/group/
	// hunters-creeds/necrotic-hand/feature/medical-proficiency, 3rd level)
	// grants "the Necrosis Ninjutsu", but the jutsu's own printed name is
	// "Medical Release: Necrosis" — confirmed the only jutsu row in
	// rules.db whose name contains "Necrosis".
	"necrosis": "jutsu/medical-release-necrosis",
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
	grantedFeatures, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	if len(grantedFeatures) == 0 {
		return nil, nil
	}
	index, err := s.jutsuNameIndex()
	if err != nil {
		return nil, err
	}

	labels := map[string]string{}
	for _, f := range grantedFeatures {
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
			normalized := normalizeJutsuGrantName(grant.Name)
			slug, ok := index[normalized]
			if !ok {
				if aliasSlug, aliasOK := jutsuGrantNameAliases[normalized]; aliasOK {
					slug, ok = aliasSlug, true
				}
			}
			if !ok {
				continue // rules update renamed/removed the jutsu — skip rather than invent a row
			}
			if _, exists := labels[slug]; !exists {
				labels[slug] = label
			}
		}
	}

	// Controlled Chakra Flow's own choice-of-2 pick (Firecracker Flash or
	// Feather Burst) isn't a fixed sentence parseJutsuGrants can extract —
	// it's a player CHOICE (features.ChoiceNamedJutsuGrant) — so it's
	// resolved separately and merged into the same label map.
	if pickedSlug, err := s.controlledChakraFlowGrantedJutsu(characterID, grantedFeatures); err != nil {
		return nil, err
	} else if pickedSlug != "" {
		if _, exists := labels[pickedSlug]; !exists {
			labels[pickedSlug] = "Subclass Feature"
		}
	}

	// Mech Crafter's Adaptive Movement grants two named jutsu in one
	// sentence ("You learn the Body Flicker and Chakra Leaping Ninjutsu") —
	// jutsuGrantNoRankPattern captures the whole "Body Flicker and Chakra
	// Leaping" span as one bogus name, which resolves against neither jutsu
	// and is silently skipped, so both are hand-added here instead of
	// generalizing the regex to split on "and" (see jutsuGrantNameAliases'
	// own doc for why this file prefers a small hand-verified list over a
	// broad heuristic). Only the free known-jutsu grant reaching the Known
	// Jutsu list is tracked here — casting either jutsu off CCD chakra
	// instead of the normal pool, the combined-casting rider ("automatically
	// gain the benefits of the other jutsu you did not cast"), and the
	// terrain-ignoring/wall-walking/jump-distance clauses while under either
	// jutsu's effect all stay entirely manual/narrated; no combined-cast or
	// terrain-ignoring mechanic exists anywhere in this app.
	if hasFeature(grantedFeatures, scienceNinAdaptiveMovementFeatureSlug) {
		for _, slug := range []string{"jutsu/body-flicker", "jutsu/chakra-leaping"} {
			if _, exists := labels[slug]; !exists {
				labels[slug] = "Subclass Feature"
			}
		}
	}

	return labels, nil
}

// hasFeature reports whether grantedFeatures contains a feature with the
// given slug — same shape as internal/charsheet's own unexported hasFeature,
// duplicated here since that package can't be imported back into this one
// (see scienceNinExoskeletonFeatureSlug's own comment for this project's
// established precedent on small cross-package duplication like this).
func hasFeature(grantedFeatures []grantedFeatureRow, slug string) bool {
	for _, f := range grantedFeatures {
		if f.Slug == slug {
			return true
		}
	}
	return false
}

// puppetControlledChakraFlowFeatureSlug is Green Technique Marionettist's
// 6th-level subclass feature: "you learn one of the following E-Rank
// jutsu... Firecracker Flash (Ninjutsu)... Feather Burst (Genjutsu)..." —
// see internal/features' own ChoiceNamedJutsuGrant doc for the choice
// mechanism backing this pick.
const puppetControlledChakraFlowFeatureSlug = "class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/controlled-chakra-flow"

// controlledChakraFlowGrantedJutsu resolves Controlled Chakra Flow's own
// resolved pick (features.ChoiceNamedJutsuGrant, ChoiceIndex 0 — Firecracker
// Flash or Feather Burst) into a jutsu slug, or "" if the feature isn't
// currently granted or no pick has been made yet. Gated on grantedFeatures
// (already level-filtered by loadGrantedFeatures) the same way every other
// single-feature grant check in this file works, rather than re-deriving
// MinLevel through features.ResolveChoiceSlots. Only WHICH of the two named
// jutsu was learned is tracked — each jutsu's own rider effect (bundling
// with a C-Rank Ninjutsu cast, or applying to all Puppets at once) is
// reference text with no computed field to land in.
func (s *server) controlledChakraFlowGrantedJutsu(characterID int64, grantedFeatures []grantedFeatureRow) (string, error) {
	has := false
	for _, f := range grantedFeatures {
		if f.Slug == puppetControlledChakraFlowFeatureSlug {
			has = true
			break
		}
	}
	if !has {
		return "", nil
	}
	resolved, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return "", err
	}
	return resolved[features.ChoiceKey{FeatureSlug: puppetControlledChakraFlowFeatureSlug, ChoiceIndex: 0}], nil
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
// Origin (loadJutsuOrigins' "class"/"clan" badges) is non-gating flavor —
// the library never blocks a drag based on it. jutsuEligible
// (jutsu_eligibility.go) DOES gate elemental jutsu on a separate, real
// affinities map, though, and this feature's own "gain the Water Nature
// release" clause is now registered there (subclassFlatAffinity,
// elemental_affinity.go) — without it, a Fry Cook with no other Water
// source could never actually add a Water-release jutsu to their known
// list, contradicting the printed text. The second branch (the bonus
// known-jutsu-count math below) is a separate payload of the same feature.
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
// to [release] Jutsu, you instead learn N additional [release] Jutsu that
// you qualify for" shape shared by Fry Cooks' Water and Oil, Do Mix, Heat
// Master's If You Can't Handle the Heat..., and the five Ninjutsu Focus
// "[Element] Release" subclass features — gated on the named granting
// feature being active AND the character already having access to at
// least one of the given keywords from some other source. bonusAmount is
// caller-supplied rather than computed here, since it varies by feature:
// half proficiency bonus for the two Cooking-Nin features (computed at
// their own call sites), a flat 2 for the five Ninjutsu Focus features.
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
func (s *server) natureReleaseBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug, grantSlug string, keywords []string, bonusAmount int) (int, error) {
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
	// grantSlug is now itself a subclassFlatAffinity source (see
	// waterAndOilDoMixSlug's own comment), so it must be excluded from this
	// local copy before resolving affinities here — otherwise the feature's
	// own flat grant would always satisfy its own "already have access"
	// pre-check below, making the "gain the keyword fresh, no bonus" branch
	// unreachable again, the same bug this function's own comment already
	// describes hasNatureReleaseAccess having had. jutsuEligibleForCharacter
	// and characters.go's own eligibility check build their own separate,
	// unmodified grantedSlugs maps and are unaffected by this local delete —
	// they must keep the full set so the flat grant actually opens
	// eligibility there.
	delete(grantedSlugs, grantSlug)
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
				return bonusAmount, nil
			}
			continue
		}
		ok, err := s.hasNatureReleaseAccess(classSlug, clanSlug, kw)
		if err != nil {
			return 0, err
		}
		if ok {
			return bonusAmount, nil
		}
	}
	return 0, nil
}

// waterAndOilBonusJutsuSlots returns the bonus known-jutsu count Water and
// Oil, Do Mix adds to JutsuKnownCap — half the character's proficiency
// bonus, rounded down, gated on Water or Medical release access from
// elsewhere. See natureReleaseBonusJutsuSlots.
func (s *server) waterAndOilBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug string, proficiencyBonus int) (int, error) {
	return s.natureReleaseBonusJutsuSlots(characterID, features, classSlug, clanSlug, waterAndOilDoMixSlug, []string{"Water Release", "Medical"}, proficiencyBonus/2)
}

// heatMasterBonusJutsuSlots returns the bonus known-jutsu count If You
// Can't Handle the Heat... adds to JutsuKnownCap — half the character's
// proficiency bonus, rounded down, gated on Fire release access from
// elsewhere. See natureReleaseBonusJutsuSlots.
func (s *server) heatMasterBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug string, proficiencyBonus int) (int, error) {
	return s.natureReleaseBonusJutsuSlots(characterID, features, classSlug, clanSlug, heatMasterFireAccessSlug, []string{"Fire Release"}, proficiencyBonus/2)
}

// ninjutsuFocusReleaseBonusSlugs: the five Ninjutsu Focus "[Element]
// Release" 2nd-level subclass features (Blaze Walker/Fire, Lightning
// Breaker/Lightning, Stone Crusher/Earth, Storm Terror/Wind, Tsunami/Water)
// each carry two separate payloads in the same sentence: "you gain the
// ability to learn ninjutsu with the [Element] Release Keyword" (the flat
// affinity grant itself — already handled by elemental_affinity.go's
// subclassFlatAffinity) and "if you can already do this you learn 2
// additional [Element] Release Ninjutsu that you qualify for" (a SEPARATE
// bonus known-jutsu count, gated on already having that element from some
// other source, keyed here). This map only covers the second half; the
// first half needs no entry of its own here.
var ninjutsuFocusReleaseBonusSlugs = map[string]string{
	"class/ninjutsu-specialist/group/ninjutsu-focus/blaze-walker/feature/fire-release":           "Fire Release",
	"class/ninjutsu-specialist/group/ninjutsu-focus/lightning-breaker/feature/lightning-release": "Lightning Release",
	"class/ninjutsu-specialist/group/ninjutsu-focus/stone-crusher/feature/earth-release":         "Earth Release",
	"class/ninjutsu-specialist/group/ninjutsu-focus/storm-terror/feature/wind-release":           "Wind Release",
	"class/ninjutsu-specialist/group/ninjutsu-focus/tsunami/feature/water-release":               "Water Release",
}

// ninjutsuFocusReleaseBonusJutsuSlots returns the bonus known-jutsu count
// summed across every Ninjutsu Focus Release feature the character holds —
// in practice at most one, since a character can only take one Ninjutsu
// Focus subclass, but summed rather than a single lookup for the same
// multiclass-safety reason featArchivistBonusJutsuSlots sums its own four
// feats. Each feature grants a flat 2, not half proficiency bonus like
// Water and Oil/Heat Master, so bonusAmount is passed as a literal 2. Book
// text's "one of which can be C-Rank or lower" per-rank restriction is not
// separately enforced — same "flat cap addition, rank not separately
// enforced" simplification chakraCellEnhancementBonusJutsuSlots's own
// comment documents, since JutsuKnownCap has no per-rank sub-limit. Which
// specific jutsu fill the extra 2 slots stays the player's own pick via the
// existing known-jutsu picker.
func (s *server) ninjutsuFocusReleaseBonusJutsuSlots(characterID int64, features []grantedFeatureRow, classSlug, clanSlug string) (int, error) {
	total := 0
	for slug, keyword := range ninjutsuFocusReleaseBonusSlugs {
		bonus, err := s.natureReleaseBonusJutsuSlots(characterID, features, classSlug, clanSlug, slug, []string{keyword}, 2)
		if err != nil {
			return 0, err
		}
		total += bonus
	}
	return total, nil
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

// clanBonusDRankJutsuKnown: clans whose own clan_traits row grants a flat,
// always-on increase to known jutsu — "You know 1 additional [Clan]
// D-Rank [Hi]jutsu" (e.g. clan/aburame's Parasitic Technique) — with no
// player choice involved. clan_traits carries no feature slug (its
// primary key is clan_slug+name, not a slug column the way clan_features
// has), so this can't be gated off a grantedFeatureRow the way
// waterAndOilBonusJutsuSlots/heatMasterBonusJutsuSlots/
// chakraCellEnhancementBonusJutsuSlots all are above — it's keyed
// directly off the clan slug instead, the same "clan-slug-keyed flat
// grant, no picker needed" shape clanFlatAffinity (elemental_affinity.go)
// already uses for clan_traits rows that grant an element outright.
// Confirmed against dist/rules.db: every clan below has an identically
// shaped clan_traits row. Same "flat cap addition, rank not separately
// enforced" simplification chakraCellEnhancementBonusJutsuSlots documents
// above — the extra slot is unrestricted, filled from whatever eligible
// jutsu list the character already draws from (which for every clan
// below already includes that clan's own jutsu list, per each clan's
// separate always-on "Clan Jutsu" feature).
var clanBonusDRankJutsuKnown = map[string]int{
	"clan/aburame":         1,
	"clan/hoshi":           1,
	"clan/hyuga":           1,
	"clan/jugo":            1,
	"clan/kashu":           1,
	"clan/nara":            1,
	"clan/shikigami":       1,
	"clan/synthetic-human": 1,
	"clan/uzumaki":         1,
	"clan/yamanaka":        1,
}

// clanBonusDRankJutsuSlots returns the bonus known-jutsu count a
// character's clan_traits row adds to JutsuKnownCap.
func clanBonusDRankJutsuSlots(clanSlug string) int {
	return clanBonusDRankJutsuKnown[clanSlug]
}

// sarutobiAdvancedNatureProficiencyFeatureSlug is Sarutobi's 1st-level clan
// feature granting escalating bonus known-jutsu slots at 1st, 3rd, 7th, and
// 15th level.
const sarutobiAdvancedNatureProficiencyFeatureSlug = "clan/sarutobi/feature/advanced-nature-proficiency"

// sarutobiAdvancedNatureProficiencyBonusJutsuSlots returns the bonus
// known-jutsu count Advanced Nature Proficiency adds to JutsuKnownCap: one
// slot at 1st level, plus one more (cumulative) at 3rd, 7th, and 15th level.
// Gated on the character's TOTAL level, not any one class's own level,
// since this is a clan feature (see jutsuKnownCapForCharacter's own comment
// on sheet.Level). Book text restricts each slot's rank (the 1st-level slot
// must be a D-Rank jutsu carrying a Nature Release keyword chosen via the
// sibling Advanced Nature Transformation clan feature, then C/B/A-Rank at
// 3rd/7th/15th) — not separately enforced, the same "flat cap addition,
// rank not separately enforced" simplification chakraCellEnhancementBonusJutsuSlots
// already accepts, since this app's known-jutsu cap has no per-rank
// sub-limit. The Nature Release prerequisite needs no separate access
// check either: Advanced Nature Transformation is itself an always-granted
// 1st-level Sarutobi feature, so every Sarutobi carrying this feature
// already has it.
func sarutobiAdvancedNatureProficiencyBonusJutsuSlots(features []grantedFeatureRow, totalLevel int) int {
	has := false
	for _, f := range features {
		if f.Slug == sarutobiAdvancedNatureProficiencyFeatureSlug {
			has = true
			break
		}
	}
	if !has {
		return 0
	}
	bonus := 1
	if totalLevel >= 3 {
		bonus++
	}
	if totalLevel >= 7 {
		bonus++
	}
	if totalLevel >= 15 {
		bonus++
	}
	return bonus
}

// whiteTechniqueChakraStringAugmentsFeatureSlug is White Technique Weaver's
// 2nd-level subclass feature granting escalating bonus known-jutsu slots at
// Puppet Master levels 2, 6, 10, and 14.
const whiteTechniqueChakraStringAugmentsFeatureSlug = "class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/chakra-string-augments"

// whiteTechniqueChakraStringBonusJutsuSlots returns the bonus known-jutsu
// count Chakra String Augments adds to JutsuKnownCap: two additional jutsu
// of a rank the character qualifies for, plus one more (cumulative) at
// Puppet Master levels 6, 10, and 14 — for a cap of 5. Gated on the
// character's own Puppet Master class level specifically, not sheet.Level,
// the same "scoped to the granting class's own level" rule
// waterAndOilBonusJutsuSlots/heatMasterBonusJutsuSlots already follow,
// since the book's "at 6th, 10th, and 14th levels" means levels in this
// class. Book text: "you gain two additional Jutsu of a rank you qualify
// for, not counting against your known. You gain 1 more jutsu this way at
// 6th, 10th, and 14th levels" — not separately enforced by rank, the same
// "flat cap addition, rank not separately enforced" simplification
// chakraCellEnhancementBonusJutsuSlots already accepts.
func whiteTechniqueChakraStringBonusJutsuSlots(features []grantedFeatureRow, puppetMasterLevel int) int {
	has := false
	for _, f := range features {
		if f.Slug == whiteTechniqueChakraStringAugmentsFeatureSlug {
			has = true
			break
		}
	}
	if !has {
		return 0
	}
	bonus := 2
	if puppetMasterLevel >= 6 {
		bonus++
	}
	if puppetMasterLevel >= 10 {
		bonus++
	}
	if puppetMasterLevel >= 14 {
		bonus++
	}
	return bonus
}

// archivistFeatBonusJutsuSlots: the four discipline-specific Archivist
// feats (Ninjutsu/Genjutsu/Taijutsu/Bukijutsu) each grant an identically
// shaped flat +1 known-jutsu slot — "You learn an additional [discipline]
// that you qualify for. This does not count against your Jutsu Known." —
// confirmed identical across all four rows in dist/rules.db, discipline
// word the only difference. Keyed by the feat's own slug (as set on
// grantedFeatureRow by mergeFeatFeatures) rather than a clan or class
// feature slug, the same "flat cap addition" shape clanBonusDRankJutsuKnown
// uses above, just feat-sourced instead of clan-sourced.
//
// Each feat's separate one-time "next time you hit 5th/9th/13th level,
// learn one additional [rank] jutsu" trigger is NOT modeled here — that
// needs a level-gated picker, not a flat cap addition, and is left as an
// unmodeled follow-up.
var archivistFeatBonusJutsuSlots = map[string]int{
	"feat/ninjutsu-archivist":  1,
	"feat/genjutsu-archivist":  1,
	"feat/taijutsu-archivist":  1,
	"feat/bukijutsu-archivist": 1,
}

// featArchivistBonusJutsuSlots returns the bonus known-jutsu count the
// Archivist feats add to JutsuKnownCap, summed rather than a single lookup
// since a character can hold more than one of the four (e.g. a
// multiclassed character qualifying for both Ninjutsu Archivist and
// Genjutsu Archivist).
func featArchivistBonusJutsuSlots(features []grantedFeatureRow) int {
	bonus := 0
	for _, f := range features {
		bonus += archivistFeatBonusJutsuSlots[f.Slug]
	}
	return bonus
}

// ninjutsuSpecializationBonusJutsuKnown: the four non-elemental Ninjutsu
// Focus "[Discipline] Specialization" 2nd-level subclass features each
// grant an UNCONDITIONAL flat bonus known-jutsu count — unlike the five
// "[Element] Release" features above (ninjutsuFocusReleaseBonusSlugs), none
// of these four are gated on "if you already have access", so no separate
// affinity/access check is needed here, just the flat cap addition. Book
// text's own per-feature keyword/rank restriction (Hijutsu-only,
// Fuinjutsu-only, Medical-Acid/Necrotic/Poison-only, no-nature-release-only;
// plus a C-Rank-or-lower carve-out on Hemomantic and Void Specialization) is
// not separately enforced — same "flat cap addition, rank not separately
// enforced" simplification chakraCellEnhancementBonusJutsuSlots's own
// comment documents, since JutsuKnownCap has no per-rank sub-limit. Which
// jutsu fill the bonus slots stays the player's own pick via the existing
// known-jutsu picker.
var ninjutsuSpecializationBonusJutsuKnown = map[string]int{
	"class/ninjutsu-specialist/group/ninjutsu-focus/hijutsu-elitist/feature/hijutsu-specialization":    1,
	"class/ninjutsu-specialist/group/ninjutsu-focus/scribe-master/feature/fuinjutsu-specialization":    2,
	"class/ninjutsu-specialist/group/ninjutsu-focus/sanguine-master/feature/hemomantic-specialization": 1,
	"class/ninjutsu-specialist/group/ninjutsu-focus/trace-talent/feature/void-specialization":          2,
}

// ninjutsuSpecializationBonusJutsuSlots returns the bonus known-jutsu count
// summed across every Ninjutsu Specialization feature the character holds —
// in practice at most one, since a character can only take one Ninjutsu
// Focus subclass, but summed for the same multiclass-safety reason
// featArchivistBonusJutsuSlots sums its own four feats.
func ninjutsuSpecializationBonusJutsuSlots(features []grantedFeatureRow) int {
	bonus := 0
	for _, f := range features {
		bonus += ninjutsuSpecializationBonusJutsuKnown[f.Slug]
	}
	return bonus
}
