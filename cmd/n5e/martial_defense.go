package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// Martial Defense (class/taijutsu-specialist/feature/martial-defense) is one
// feature row in rules.db bundling two gated effects (see
// class_features.description for the exact text):
//
//   - 1st level: the unarmored AC formula becomes
//     10 + Proficiency + Dexterity Modifier — see computeEquippedAC's own
//     Taijutsu-Specialist branch (internal/charsheet/charsheet.go).
//   - 5th level onward: a guard-slot system that infuses the character with
//     taijutsu techniques "meant to mimic" an armor seal, structurally the
//     same shape as Purple Technique's Battle Ready Armor upgrade (see
//     puppets.go's battleReadyArmorSealOptions) — this file mirrors that
//     picker, keyed to the character directly rather than a companion.
//
// The guard-slot system's own numeric seal effect isn't computed anywhere:
// like battleReadyArmorSealOptions, this app has no field an armor seal's
// actual bonus reaches (equipment carries no parsed numeric-effect column
// for enhancement_seal rows, only reference description text) — the PICK is
// tracked and level-gated, matching Weapon Focus's UI pattern, but no
// numeric AC/save/etc. bonus is derived from it. Documented Group 2/3 gap.

// guardSlotCap is Martial Defense's own slot schedule: 2 slots at 5th level,
// a 3rd at 9th, a 4th at 13th, a 5th at 17th. Zero below 5th — the guard-
// slot half of this feature doesn't exist yet at 1st-4th level, only the AC
// formula does.
func guardSlotCap(taijutsuLevel int) int {
	switch {
	case taijutsuLevel >= 17:
		return 5
	case taijutsuLevel >= 13:
		return 4
	case taijutsuLevel >= 9:
		return 3
	case taijutsuLevel >= 5:
		return 2
	default:
		return 0
	}
}

// guardSealTierCap is the highest armor-seal tier (equipment.seal_rank,
// D/C/B/A) a guard slot can currently be infused with — minor (D) from 5th,
// refined (C) from 9th, greater (B) from 13th, Superior (A) from 17th. Every
// tier at or below the cap stays available, same cumulative reading
// battleReadyArmorSealOptions' own rank gate uses — the book never says
// access to a lower tier is lost once a higher one unlocks. "" (no tier)
// below 5th level. Mastercraft (S) armor seals exist in the catalog but are
// never granted by this feature at any level — the book's own progression
// tops out at Superior.
func guardSealTierCap(taijutsuLevel int) string {
	switch {
	case taijutsuLevel >= 17:
		return "A"
	case taijutsuLevel >= 13:
		return "B"
	case taijutsuLevel >= 9:
		return "C"
	case taijutsuLevel >= 5:
		return "D"
	default:
		return ""
	}
}

// guardSealOption is one Armor Seal (equipment.kind = 'enhancement_seal',
// seal_applies_to = 'armor') offered — or already chosen — as a Martial
// Defense guard-slot infusion.
type guardSealOption struct {
	Slug, Name, Description, Rank string
	DetailHref                    string
}

// loadArmorSealCatalog lists every Armor Seal in the equipment catalog
// (kind='enhancement_seal', seal_applies_to='armor'), D through S, with no
// tier filtering of its own — the shared base both Martial Defense's own
// tier-capped guard slots (loadGuardSealCatalog) and Shinobi-Ware's Ever
// Evolving (loadEverEvolvingSealCatalog, ever_evolving.go), whose own text
// places no level-gated tier ceiling, build on.
func (s *server) loadArmorSealCatalog() ([]guardSealOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(description, ''), seal_rank FROM equipment
		WHERE kind = 'enhancement_seal' AND seal_applies_to = 'armor' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []guardSealOption
	for rows.Next() {
		var o guardSealOption
		var rank sql.NullString
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description, &rank); err != nil {
			return nil, err
		}
		o.Rank = rank.String
		o.DetailHref = "/seals/" + o.Slug
		out = append(out, o)
	}
	return out, rows.Err()
}

// loadGuardSealCatalog lists every armor seal the character currently
// qualifies for by tier, gated by guardSealTierCap — same underlying
// catalog loadArmorSealCatalog reads, just capped against this feature's
// own level schedule instead of the universal core-book rank-by-character-
// level formula, since Martial Defense's own text ties each tier to its own
// specific class level rather than the standard D/C/B/A/S-by-character-
// level table.
func (s *server) loadGuardSealCatalog(tierCap string) ([]guardSealOption, error) {
	if tierCap == "" {
		return nil, nil
	}
	all, err := s.loadArmorSealCatalog()
	if err != nil {
		return nil, err
	}
	tierCapOrder := jutsuRankOrder[tierCap]
	var out []guardSealOption
	for _, o := range all {
		if jutsuRankOrder[o.Rank] <= tierCapOrder {
			out = append(out, o)
		}
	}
	return out, nil
}

// martialDefenseTabData is the sheet_martial_defense box's full data.
type martialDefenseTabData struct {
	Cap       int
	Used      int
	TierCap   string
	Known     []guardSealOption
	Available []guardSealOption
}

// loadMartialDefenseTabData returns nil for a character with no Taijutsu
// Specialist levels, or one below 5th level in it — the template gates the
// whole box's existence on this being non-nil, same treatment
// WeaponFocus/MartialTechniques get.
func (s *server) loadMartialDefenseTabData(characterID int64) (*martialDefenseTabData, error) {
	level, err := s.taijutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	slotCap := guardSlotCap(level)
	if slotCap == 0 {
		return nil, nil
	}
	tierCap := guardSealTierCap(level)

	catalog, err := s.loadGuardSealCatalog(tierCap)
	if err != nil {
		return nil, err
	}
	picks, err := charstore.ListMartialDefenseSeals(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	pickedSet := make(map[string]bool, len(picks))
	for _, slug := range picks {
		pickedSet[slug] = true
	}

	var known, available []guardSealOption
	for _, o := range catalog {
		if pickedSet[o.Slug] {
			known = append(known, o)
		} else {
			available = append(available, o)
		}
	}
	// A previously-infused seal whose tier has since fallen out of the
	// current catalog window (shouldn't happen — tiers are cumulative — but
	// mirrors the defensive stale-slug tolerance the rest of this app
	// applies) still counts against Used even if it can't be re-picked.
	used := len(known)
	if used < len(picks) {
		used = len(picks)
	}

	return &martialDefenseTabData{
		Cap:       slotCap,
		Used:      used,
		TierCap:   tierCap,
		Known:     known,
		Available: available,
	}, nil
}

// handleMartialDefenseSealAdd infuses one guard slot with an armor seal,
// gated by the character's own current slot cap and qualifying tier —
// server-side, defense in depth regardless of what the UI already disables.
func (s *server) handleMartialDefenseSealAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("seal_slug"))
	if slug == "" {
		http.Error(w, "missing seal", http.StatusBadRequest)
		return
	}
	data, err := s.loadMartialDefenseTabData(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load martial defense for add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no guard slots", http.StatusBadRequest)
		return
	}
	if data.Used >= data.Cap {
		http.Error(w, "no guard slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.Available {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid armor seal for your guard slots", http.StatusBadRequest)
		return
	}
	if err := charstore.AddMartialDefenseSeal(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add martial defense seal:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_defense")
}

// handleMartialDefenseSealDelete drops one infused guard-slot seal. The slug
// is a form field, not a URL path segment — equipment slugs contain slashes
// (e.g. "seal/armor/minor/..."), same reason handleWeaponFocusDelete takes
// its slug as a form field instead of a {slug} path wildcard.
func (s *server) handleMartialDefenseSealDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("seal_slug"))
	if slug == "" {
		http.Error(w, "missing seal", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveMartialDefenseSeal(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove martial defense seal:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_martial_defense")
}
