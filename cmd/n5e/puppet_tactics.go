package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// Puppet Master Tactics: "Tactics of the Craft" (class_features, level 2)
// grants one of 5 named Tactics — Agile/Defensive/Helpful/Offensive/
// Resourceful — as a real player CHOICE, with a second slot opening at
// level 11 and the choice freely re-pickable at any time (see
// charstore.RemovePuppetTactic's own doc for why this app doesn't try to
// enforce the "only on a level-up" real-game pacing rule, the same
// tradeoff already made for Puppet Upgrades). rules.db has no per-choice
// structure for this the way Puppet Upgrades got via class_option_entries
// — each Tactic is just its own class_features row, tagged level 11 (its
// OWN row bundles both the level-2 base effect and the level-11
// enhancement in one description, with no earlier row for level 2 at all).
// Read literally, that would make the granted-features list show all 5
// simultaneously and only from level 11 onward — see
// puppetMasterTacticSlugs in characters.go for the exclusion that stops
// that, and this file's own picker for what replaces it, placed on the
// Core tab per its own ask (not the Puppets tab — a Tactic is the
// player's OWN backup plan for when their Puppet Tool is destroyed, not
// something the puppet has).
var puppetMasterTacticSlugOrder = []string{
	"class/puppet-master/feature/puppet-master-tactics-agile-tactics",
	"class/puppet-master/feature/defensive-tactics",
	"class/puppet-master/feature/helpful-tactics-changed-new",
	"class/puppet-master/feature/offensive-tactics",
	"class/puppet-master/feature/resourceful-tactics",
}

// puppetTacticSlugSet mirrors puppetMasterTacticSlugOrder as a set, for the
// add handler's own validation — a posted slug must be one of these 5, not
// an arbitrary class_features row from some other class entirely.
var puppetTacticSlugSet = func() map[string]bool {
	set := make(map[string]bool, len(puppetMasterTacticSlugOrder))
	for _, slug := range puppetMasterTacticSlugOrder {
		set[slug] = true
	}
	return set
}()

// puppetTacticCap returns how many Tactics a character can have chosen at
// once: 0 before level 2 (not unlocked yet), 1 from level 2, 2 from level
// 11 — read directly off "Tactics of the Craft"'s own printed text ("you
// gain one tactic. At 11th level, you gain a second tactic"), since
// rules.db has no class_level_resources row for this the way Puppet
// Upgrades' material tiers do. subclassColor adds Black Technique
// Puppeteer's own bonus on top ("Lastly, you gain an additional Tactic from
// the Tactics of the Craft feature. Gain another Tactic at 10th level.",
// Black Technique Proficiency, L2) — +1 at 2nd level, +1 more at 10th,
// stacking with the base progression; every other color gets none.
func puppetTacticCap(level int, subclassColor string) int {
	total := 0
	switch {
	case level >= 11:
		total = 2
	case level >= 2:
		total = 1
	}
	if subclassColor == "Black" && level >= 2 {
		total++
	}
	if subclassColor == "Black" && level >= 10 {
		total++
	}
	return total
}

type puppetTactic struct {
	Slug        string
	Name        string
	Description string
	Selected    bool
}

type puppetTacticsTabData struct {
	Cap     int
	Used    int
	Tactics []puppetTactic
}

// loadPuppetTacticsTabData returns nil (not a zero value) for a character
// with no Puppet Master levels at all — the template gates the whole box's
// existence on this being non-nil, the same "real DOM removal, not just
// hidden" treatment the Puppets tab itself gets in the tab bar.
func (s *server) loadPuppetTacticsTabData(characterID int64, puppetMasterLevel int) (*puppetTacticsTabData, error) {
	if puppetMasterLevel == 0 {
		return nil, nil
	}

	subclassSlug, _, err := s.puppetMasterSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	subclassColor := puppetSubclassColorBySlug[subclassSlug]

	picks, err := charstore.ListPuppetTactics(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	picked := make(map[string]bool, len(picks))
	for _, slug := range picks {
		picked[slug] = true
	}

	tactics := make([]puppetTactic, 0, len(puppetMasterTacticSlugOrder))
	for _, slug := range puppetMasterTacticSlugOrder {
		var name, description string
		err := s.rulesDB.QueryRow(`SELECT name, description FROM class_features WHERE slug = ?`, slug).Scan(&name, &description)
		if err == sql.ErrNoRows {
			continue // a rules update renamed/removed the row — degrade gracefully, same as everywhere else in this app
		}
		if err != nil {
			return nil, err
		}
		// "Puppet Master Tactics Agile Tactics" is how rules.db actually
		// stores Agile Tactics' own name (a doubled prefix, confirmed
		// directly against the shipped data) — every other Tactic's name
		// is clean, so this is a one-off strip rather than a general rule.
		name = strings.TrimPrefix(name, "Puppet Master Tactics ")
		tactics = append(tactics, puppetTactic{
			Slug: slug, Name: name, Description: description, Selected: picked[slug],
		})
	}

	return &puppetTacticsTabData{Cap: puppetTacticCap(puppetMasterLevel, subclassColor), Used: len(picks), Tactics: tactics}, nil
}

// handlePuppetTacticAdd chooses one Tactic, gated by the character's own
// current cap — server-side, the same defense-in-depth every other cap
// check on this sheet gets regardless of what the UI already disables.
func (s *server) handlePuppetTacticAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("tactic_slug"))
	if !puppetTacticSlugSet[slug] {
		http.Error(w, "unknown tactic", http.StatusBadRequest)
		return
	}

	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for tactic add:", err)
		return
	}
	if level == 0 {
		http.Error(w, "character has no levels in Puppet Master", http.StatusBadRequest)
		return
	}
	subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master subclass for tactic add:", err)
		return
	}
	picks, err := charstore.ListPuppetTactics(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet tactics for add:", err)
		return
	}
	if len(picks) >= puppetTacticCap(level, puppetSubclassColorBySlug[subclassSlug]) {
		http.Error(w, "no tactic slots remaining", http.StatusBadRequest)
		return
	}

	if err := charstore.AddPuppetTactic(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add puppet tactic:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tactics")
}

// handlePuppetTacticDelete drops a chosen Tactic — no cap check needed,
// removing is always allowed. The slug is a form field, not a URL path
// segment — class_features slugs contain slashes (e.g.
// "class/puppet-master/feature/defensive-tactics"), which a single-segment
// {slug} path wildcard cannot carry (html/template does not percent-encode
// "/" in a URL-path substitution, and net/http's ServeMux only treats a
// %2F-encoded slash as part of a wildcard segment, not a literal one — this
// route 404'd on every real Tactic before this fix). Same pattern as
// handleMartialTechniqueDelete in taijutsu.go.
func (s *server) handlePuppetTacticDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("tactic_slug"))
	if slug == "" {
		http.Error(w, "missing tactic", http.StatusBadRequest)
		return
	}
	if err := charstore.RemovePuppetTactic(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove puppet tactic:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tactics")
}
