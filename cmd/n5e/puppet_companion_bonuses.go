package main

import (
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// puppetElevatedDesignFeatureSlug: Black Technique Puppeteer's L17 feature.
// "All Puppet Tools you possess gain a +2 to two different ability scores.
// The maximums for these chosen scores are increased to 22." The free-
// Upgrade half of this same feature (Bronze/Silver/Gold, one each) is a
// bonus tier-cap slot, not a choice slot — see puppet_upgrade_bonus_tiers.go.
const puppetElevatedDesignFeatureSlug = "class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/elevated-design"

// puppetSymphonyOfPuppetryFeatureSlug: Red Technique Performer's L17
// either/or feature — see choices.go's own ChoiceSymphonyBranch doc.
const puppetSymphonyOfPuppetryFeatureSlug = "class/puppet-master/group/puppet-techniques/red-technique-performer/feature/symphony-of-puppetry"

// puppetExpandedPuppetryEntrySlug: the Silver Tier Upgrade Symphony of
// Puppetry's Expansion branch grants for free (or converts into a bonus
// Bronze-tier-or-lower slot, once already taken twice) — see
// puppetSymphonyExpansionGrant.
const puppetExpandedPuppetryEntrySlug = "class/puppet-master/option/puppet-master-upgrades/silver-tier/entry/expanded-puppetry"

// puppetElevatedDesignAbilityBonus resolves Elevated Design's own choice
// (two independent ChoiceCompanionAbilityScoreIncrease slots) into a
// per-ability +2 bonus, applied to every Puppet Tool the character
// possesses. Deliberately never folded into a companion's own Str/Dex/...
// fields — those are plain player-editable <input>s that resend their
// CURRENT typed value on every other field's blur (see
// charstore.SetCompanionFields' own doc); silently pre-adding +2 there
// would get autosaved right back into the stored score on the next
// unrelated edit, permanently baking in a bonus that's supposed to stay
// reversible (e.g. if a level-down or subclass swap ever removed the
// feature). Callers show this as separate annotation text next to the
// field instead — see companion_fields.html.
func puppetElevatedDesignAbilityBonus(resolved map[features.ChoiceKey]string) map[string]int {
	bonus := map[string]int{}
	for i := 0; i < 2; i++ {
		if ab, ok := resolved[features.ChoiceKey{FeatureSlug: puppetElevatedDesignFeatureSlug, ChoiceIndex: i}]; ok && ab != "" {
			bonus[ab] += 2
		}
	}
	return bonus
}

// puppetSymphonyExpansionGrant resolves Symphony of Puppetry's Expansion
// branch's own further effect, once picked: "You gain the Expanded
// Puppetry upgrade for free. If you have already taken this upgrade
// twice, you gain an upgrade of bronze tier or lower." Computed, not
// chosen a second time — pickedCountByEntrySlug is this COMPANION's own
// count (Upgrades are tracked per companion throughout this app; Symphony
// of Puppetry's own free grant is scoped the same way as every other
// per-companion free-Upgrade merge in loadPuppetUpgradeTiers).
func puppetSymphonyExpansionGrant(resolved map[features.ChoiceKey]string, pickedCountByEntrySlug map[string]int) (entrySlug string, bronzeOrLowerInstead bool) {
	if resolved[features.ChoiceKey{FeatureSlug: puppetSymphonyOfPuppetryFeatureSlug, ChoiceIndex: 0}] != "expansion" {
		return "", false
	}
	if pickedCountByEntrySlug[puppetExpandedPuppetryEntrySlug] >= 2 {
		return "", true
	}
	return puppetExpandedPuppetryEntrySlug, false
}

// puppetSymphonyEnhancementCompanionIDs resolves which two companions
// Symphony of Puppetry's Enhancement branch applies to: "The two Puppets
// you started with at 2nd level." This app tracks no other notion of
// "started with" a companion, so the two earliest-created (lowest id)
// kind="puppet" companions are used as a documented proxy — companions is
// expected already sorted in creation order (charstore.ListCompanions'
// own default order).
func puppetSymphonyEnhancementCompanionIDs(companions []charstore.Companion) []int64 {
	var out []int64
	for _, c := range companions {
		if c.Kind != "puppet" {
			continue
		}
		out = append(out, c.ID)
		if len(out) == 2 {
			break
		}
	}
	return out
}

// puppetSymphonyEnhancementView is one companion's own resolved/pending
// Symphony of Puppetry Enhancement state — puppetCompanionView leaves this
// nil for every companion the branch doesn't apply to (branch not taken,
// or not one of the two targeted companions).
type puppetSymphonyEnhancementView struct {
	// Ability is this companion's own resolved ability pick ("str"/"dex"/
	// ...), or "" if not yet chosen.
	Ability string
}

// puppetSymphonyEnhancementViews resolves Symphony of Puppetry's
// Enhancement branch for every targeted companion at once — nil if the
// branch hasn't been chosen at all, otherwise one entry per id in
// puppetSymphonyEnhancementCompanionIDs.
func (s *server) puppetSymphonyEnhancementViews(characterID int64, resolved map[features.ChoiceKey]string, companions []charstore.Companion) (map[int64]*puppetSymphonyEnhancementView, error) {
	if resolved[features.ChoiceKey{FeatureSlug: puppetSymphonyOfPuppetryFeatureSlug, ChoiceIndex: 0}] != "enhancement" {
		return nil, nil
	}
	ids := puppetSymphonyEnhancementCompanionIDs(companions)
	if len(ids) == 0 {
		return nil, nil
	}
	picks, err := charstore.ListFeatureCompanionChoices(s.charDB, characterID, puppetSymphonyOfPuppetryFeatureSlug, 0)
	if err != nil {
		return nil, err
	}
	out := map[int64]*puppetSymphonyEnhancementView{}
	for _, id := range ids {
		out[id] = &puppetSymphonyEnhancementView{Ability: picks[id]}
	}
	return out, nil
}

// handleSheetPuppetSymphonyEnhancementAbility records a companion's own
// Symphony of Puppetry Enhancement ability pick (form field "ability") —
// re-derives eligibility from the character's currently-resolved choices
// rather than trusting the posted companion id blindly, the same
// "stale/hand-crafted request can't record a pick for a slot the character
// doesn't actually qualify for" rule handleSheetFeatureChoice already
// follows.
func (s *server) handleSheetPuppetSymphonyEnhancementAbility(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if !slices.Contains(charsheet.Abilities, ability) {
		http.Error(w, "not a valid ability pick", http.StatusBadRequest)
		return
	}

	resolved, err := features.LoadFeatureChoices(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load feature choices for symphony enhancement pick:", err)
		return
	}
	if resolved[features.ChoiceKey{FeatureSlug: puppetSymphonyOfPuppetryFeatureSlug, ChoiceIndex: 0}] != "enhancement" {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}
	companions, err := charstore.ListCompanions(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companions for symphony enhancement pick:", err)
		return
	}
	if !slices.Contains(puppetSymphonyEnhancementCompanionIDs(companions), cid) {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureCompanionChoice(s.charDB, id, puppetSymphonyOfPuppetryFeatureSlug, cid, 0, ability); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set symphony enhancement ability pick:", err)
		return
	}
	s.respondCompanionAction(w, r, id, cid)
}

// respondCompanionAction is respondSheet's companion-popup-aware sibling —
// a companion-scoped action can be submitted from either the Puppets tab
// (fragment-swapped, like every other sheet action) or the companion's own
// standalone popup window (companion_sheet.html), which loads none of the
// main sheet's fetch-form JS and so always falls through to a plain form
// submit. redirectToSheet's own target (the main character sheet) would be
// jarring from the popup — it would navigate the small popup window away
// from the very companion it was showing — so the plain-submit fallback
// here goes back to that companion's own popup URL instead.
func (s *server) respondCompanionAction(w http.ResponseWriter, r *http.Request, characterID, companionID int64) {
	if !wantsFragment(r) {
		http.Redirect(w, r, "/characters/"+strconv.FormatInt(characterID, 10)+"/companions/"+strconv.FormatInt(companionID, 10), http.StatusSeeOther)
		return
	}
	s.renderSheetFragment(w, characterID, "sheet_puppet_tab")
}
