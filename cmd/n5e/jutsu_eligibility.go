package main

import (
	"database/sql"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// jutsuIsMedical reports whether a jutsu's keywords column names the
// Medical keyword — the same substring-match convention jutsuElements/
// jutsuNeedsAnyAffinity already use for the element keywords.
func jutsuIsMedical(keywords string) bool {
	return strings.Contains(keywords, "Medical")
}

// characterMedicalRankCap resolves the highest-rank Medical-keyword jutsu a
// character may learn or cast, or "" for no Medical access at all. Medical
// jutsu are classified "Ninjutsu" like everything else Ninjutsu-casting
// classes learn — class_casting alone (classJutsuPredicate) can't tell them
// apart, so this is a second, independent gate jutsuEligible applies only
// when a candidate's own keywords actually name Medical.
//
//   - class/medical-nin: unrestricted (the "S" ceiling never actually binds
//     — every other class's own jutsu gets the same uncapped treatment at
//     the sheet level, see loadJutsuOrigins' doc comment; a real rank
//     ceiling below that only ever applies at creation, via the class's own
//     highest_rank_known).
//   - Science-Nin's Mad Scientist inquiry, Biotic Mastery (3rd level):
//     "You can learn and cast any D-Rank Medical Ninjutsu. This increases
//     to any C-Rank Medical Ninjutsu at Level 9 and B-Rank Medical Ninjutsu
//     at Level 14" — a real, fixed textual ceiling (unlike the sheet's
//     usual "no ceiling, just a badge" treatment), so it applies everywhere
//     jutsuEligible is checked, not just at creation.
//   - every other class/subclass: no access at all.
func characterMedicalRankCap(classSlugs []string, grantedFeatureSlugs map[string]bool, level int) string {
	for _, slug := range classSlugs {
		if slug == medicalNinSlug {
			return "S"
		}
	}
	if !grantedFeatureSlugs[madScientistBioticMasteryFeatureSlug] {
		return ""
	}
	switch {
	case level >= 14:
		return "B"
	case level >= 9:
		return "C"
	default:
		return "D"
	}
}

// jutsuEligible reports whether a jutsu whose origin/keywords/rank are given
// can be learned: it must come from the character's own class discipline or
// clan (origin non-empty, see loadJutsuOrigins), AND if it names an
// element, the character needs a matching affinity for it (or, for the rare
// "Any Nature Release" keyword, just needs to have at least one affinity at
// all — see jutsuNeedsAnyAffinity). A jutsu naming more than one element (a
// combo-affinity clan's own jutsu) is eligible on a match against ANY one of
// them, not all — see jutsuElements. Separately, a jutsu naming the Medical
// keyword AND reachable only via the broad class-discipline union
// (origin == "class") needs the character's own medicalRankCap
// (characterMedicalRankCap) to be non-empty and at least the jutsu's own
// rank — every Ninjutsu-casting class (all eleven) otherwise matches
// Medical jutsu on discipline alone, which is not the book's rule for any
// of them except Medical-Nin itself. A jutsu reachable via origin == "clan"
// skips this gate: several real clans (Hanami, Hyuga, Shakuton, Uzumaki)
// have their own Medical-tagged Hijutsu curated directly into clan_jutsu —
// that curation IS the access grant, the same way a combo-affinity clan's
// own jutsu doesn't need a second, independent qualification beyond having
// the clan itself. byTheBookGrant (Patissier Chef's own curated healing/
// temp-HP subset, patissierChefByTheBookHealingJutsuSlugs) is a second,
// narrower bypass of that same Medical gate — true only when the candidate
// slug is itself in that curated set AND the character actually has the
// feature, resolved once by the caller rather than re-checked per jutsu.
func jutsuEligible(origin, keywords string, affinities map[string]bool, hasAnyAffinity bool, medicalRankCap, rank string, byTheBookGrant bool) bool {
	if origin == "" {
		return false
	}
	if origin == "class" && jutsuIsMedical(keywords) && !byTheBookGrant {
		if medicalRankCap == "" || jutsuRankOrder[rank] > jutsuRankOrder[medicalRankCap] {
			return false
		}
	}
	if jutsuNeedsAnyAffinity(keywords) {
		return hasAnyAffinity
	}
	if els := jutsuElements(keywords); len(els) > 0 {
		for _, el := range els {
			if affinities[el] {
				return true
			}
		}
		return false
	}
	return true
}

// characterHasFeat reports whether a character has taken a specific feat —
// a plain existence check, not a count (character_feats is keyed
// (character_id, feat_slug), so it can never be more than one row anyway;
// see natureReleaseFeatSlug's own doc for what that means for a feat the
// book itself allows taking more than once).
func (s *server) characterHasFeat(characterID int64, featSlug string) (bool, error) {
	var exists int
	err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_feats WHERE character_id = ? AND feat_slug = ?`,
		characterID, featSlug,
	).Scan(&exists)
	return exists > 0, err
}

// characterElementalAffinities resolves everything the character currently
// has, folding together the curated clan tables (elemental_affinity.go),
// the Nature Release feat, and Professor's three subclass slots, plus
// whatever choices are already stored for the slots that need one
// (charstore.ListElementalAffinities).
func (s *server) characterElementalAffinities(characterID int64, clanSlug string, grantedFeatureSlugs map[string]bool) ([]elementalAffinity, error) {
	hasFeat, err := s.characterHasFeat(characterID, natureReleaseFeatSlug)
	if err != nil {
		return nil, err
	}
	picks, err := charstore.ListElementalAffinities(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	return resolveElementalAffinities(clanSlug, hasFeat, grantedFeatureSlugs, picks), nil
}

// jutsuEligibilityContext bundles everything jutsuEligible needs about one
// character (class/clan origin map, elemental affinity set) — computed
// once via loadJutsuEligibilityContext and reused across every candidate
// jutsu a caller needs to filter, rather than each candidate re-deriving
// origins/affinities from scratch the way a naive per-slug loop over
// jutsuEligibleForCharacter would (Master of the Green Technique's own
// jutsu picker, cmd/n5e/puppets.go's loadGreenTechniqueJutsuOptions, is
// exactly this shape — several hundred candidate jutsu, one eligibility
// check each).
type jutsuEligibilityContext struct {
	origins        map[string]string
	affinities     map[string]bool
	hasAnyAffinity bool
	medicalRankCap string
	hasByTheBook   bool
}

// eligible reports whether the jutsu named by slug/keywords/rank is eligible
// for the character this context was built for.
func (c jutsuEligibilityContext) eligible(slug, keywords, rank string) bool {
	byTheBookGrant := c.hasByTheBook && patissierChefByTheBookHealingJutsuSlugs[slug]
	return jutsuEligible(c.origins[slug], keywords, c.affinities, c.hasAnyAffinity, c.medicalRankCap, rank, byTheBookGrant)
}

// loadJutsuEligibilityContext computes a character's own class/clan jutsu
// origins and elemental affinity set once — the same setup
// jutsuEligibleForCharacter below performs for a single jutsu, factored out
// so a caller filtering many candidates against the same character pays
// this cost once, not once per candidate.
func (s *server) loadJutsuEligibilityContext(characterID int64) (jutsuEligibilityContext, error) {
	var ctx jutsuEligibilityContext

	classes, err := s.loadCharacterClassLevels(characterID)
	if err != nil {
		return ctx, err
	}
	classSlugs := make([]string, len(classes))
	for i, c := range classes {
		classSlugs[i] = c.Slug
	}

	var clanSlug sql.NullString
	if err := s.charDB.QueryRow(`SELECT clan_slug FROM characters WHERE id = ?`, characterID).Scan(&clanSlug); err != nil {
		return ctx, err
	}

	origins, err := s.loadJutsuOrigins(classSlugs, clanSlug.String)
	if err != nil {
		return ctx, err
	}

	level := s.characterLevel(characterID)
	grantedFeatures, err := s.loadGrantedFeatures(characterID, clanSlug.String, level)
	if err != nil {
		return ctx, err
	}
	grantedSlugs := map[string]bool{}
	for _, f := range grantedFeatures {
		grantedSlugs[f.Slug] = true
	}
	affinityList, err := s.characterElementalAffinities(characterID, clanSlug.String, grantedSlugs)
	if err != nil {
		return ctx, err
	}
	affinitySet := map[string]bool{}
	for _, a := range affinityList {
		affinitySet[a.Element] = true
	}

	ctx.origins = origins
	ctx.affinities = affinitySet
	ctx.hasAnyAffinity = len(affinityList) > 0
	ctx.medicalRankCap = characterMedicalRankCap(classSlugs, grantedSlugs, level)
	ctx.hasByTheBook = grantedSlugs[patissierChefByTheBookFeatureSlug]
	return ctx, nil
}

// jutsuEligibleForCharacter is handleSheetJutsuAdd's own server-side gate —
// the library row's disabled state (the main sheet render) is what a
// player actually sees, but a POST reaching the add handler is trusted no
// further than any other form submission on this app, so the exact same
// check (class/clan origin, elemental affinity) runs again here against
// the database's own current state.
func (s *server) jutsuEligibleForCharacter(characterID int64, slug string) (bool, error) {
	var keywords, rank string
	err := s.rulesDB.QueryRow(`SELECT keywords, rank FROM v_jutsu WHERE slug = ?`, slug).Scan(&keywords, &rank)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	ctx, err := s.loadJutsuEligibilityContext(characterID)
	if err != nil {
		return false, err
	}
	return ctx.eligible(slug, keywords, rank), nil
}
