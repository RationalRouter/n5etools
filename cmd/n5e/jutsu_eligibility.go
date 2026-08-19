package main

import (
	"database/sql"

	"github.com/sergio/n5e/internal/charstore"
)

// jutsuEligible reports whether a jutsu whose origin/keywords are given can
// be learned: it must come from the character's own class discipline or
// clan (origin non-empty, see loadJutsuOrigins), AND if it names an
// element, the character needs a matching affinity for it (or, for the rare
// "Any Nature Release" keyword, just needs to have at least one affinity at
// all — see jutsuNeedsAnyAffinity). A jutsu naming more than one element (a
// combo-affinity clan's own jutsu) is eligible on a match against ANY one of
// them, not all — see jutsuElements.
func jutsuEligible(origin, keywords string, affinities map[string]bool, hasAnyAffinity bool) bool {
	if origin == "" {
		return false
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

// jutsuEligibleForCharacter is handleSheetJutsuAdd's own server-side gate —
// the library row's disabled state (the main sheet render) is what a
// player actually sees, but a POST reaching the add handler is trusted no
// further than any other form submission on this app, so the exact same
// check (class/clan origin, elemental affinity) runs again here against
// the database's own current state.
func (s *server) jutsuEligibleForCharacter(characterID int64, slug string) (bool, error) {
	var keywords string
	err := s.rulesDB.QueryRow(`SELECT keywords FROM jutsu WHERE slug = ?`, slug).Scan(&keywords)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	classes, err := s.loadCharacterClassLevels(characterID)
	if err != nil {
		return false, err
	}
	classSlugs := make([]string, len(classes))
	for i, c := range classes {
		classSlugs[i] = c.Slug
	}

	var clanSlug sql.NullString
	if err := s.charDB.QueryRow(`SELECT clan_slug FROM characters WHERE id = ?`, characterID).Scan(&clanSlug); err != nil {
		return false, err
	}

	origins, err := s.loadJutsuOrigins(classSlugs, clanSlug.String)
	if err != nil {
		return false, err
	}

	level := s.characterLevel(characterID)
	grantedFeatures, err := s.loadGrantedFeatures(characterID, clanSlug.String, level)
	if err != nil {
		return false, err
	}
	grantedSlugs := map[string]bool{}
	for _, f := range grantedFeatures {
		grantedSlugs[f.Slug] = true
	}
	affinityList, err := s.characterElementalAffinities(characterID, clanSlug.String, grantedSlugs)
	if err != nil {
		return false, err
	}
	affinitySet := map[string]bool{}
	for _, a := range affinityList {
		affinitySet[a.Element] = true
	}

	return jutsuEligible(origins[slug], keywords, affinitySet, len(affinityList) > 0), nil
}
