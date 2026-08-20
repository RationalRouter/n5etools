package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// featureChoiceOption is one selectable value in a pending choice's <select>.
// Description is optional — set only where a caller wants a rollover
// tooltip on its own option (e.g. taijutsu.go's handWrapsOfPassionOptions);
// left blank everywhere else.
type featureChoiceOption struct {
	Value       string
	Label       string
	Description string
}

// hasOptionDescription reports whether any option in a pending choice row
// carries a Description — the sheet_feature_choices template uses this to
// decide whether a row's <select> needs the rollover-tooltip info icon at
// all (Skill/Saving Throw proficiency options never do; a free Upgrade or
// Armor Property pick's options always do).
func hasOptionDescription(options []featureChoiceOption) bool {
	for _, o := range options {
		if o.Description != "" {
			return true
		}
	}
	return false
}

// pendingFeatureChoiceRow is one unresolved choice slot as shown on the
// sheet — see internal/features' own choiceSlotDefs doc comment for what
// FeatureSlug/ChoiceIndex mean together and why each entry here exists.
type pendingFeatureChoiceRow struct {
	FeatureSlug string
	ChoiceIndex int
	Label       string
	Options     []featureChoiceOption
	// IsAbilityScore marks a ChoiceAbilityScoreIncrease row — the template
	// renders it as a plain (non-fetch) form with a confirm gate, because
	// an ability-score change ripples into Skills/Saves/AC/HP/Chakra/attack
	// modifiers all at once, the same breadth that already makes
	// handleSheetASI reload instead of fragment-swapping.
	IsAbilityScore bool
}

// puppetUpgradeEntryChoiceLabels: display names for the small, fixed set of
// Upgrade entries a ChoiceUpgradeEntry slot can offer (Purple Technique
// Proficiency's 3, Chakra String Augments' 4) — hardcoded rather than a
// rules.db lookup since the list is fixed, tiny, and known ahead of time,
// the same closed-vocabulary precedent abilityLabels already sets.
var puppetUpgradeEntryChoiceLabels = map[string]string{
	"class/puppet-master/option/armorers-upgrades/wood-tier/entry/chakra-blast":             "Chakra Blast",
	"class/puppet-master/option/armorers-upgrades/wood-tier/entry/integrated-weapon":        "Integrated Weapon",
	"class/puppet-master/option/armorers-upgrades/wood-tier/entry/power-fist":               "Power Fist",
	"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/better-bonds":           "Better Bonds",
	"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/chakra-pathway":         "Chakra Pathway",
	"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/defensive-manipulation": "Defensive Manipulation",
	"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/thread-weapon":          "Thread Weapon",
}

// symphonyBranchChoiceLabels: display names for Symphony of Puppetry's own
// fixed 2-value branch pick.
var symphonyBranchChoiceLabels = map[string]string{
	"expansion":   "Expansion — gain the Expanded Puppetry Upgrade for free (or a Bronze tier or lower Upgrade, once already taken twice)",
	"enhancement": "Enhancement — double the max HP, and +2 to one ability score, of the two Puppets you started with",
}

// buildPendingFeatureChoiceRows turns Sheet.PendingFeatureChoices into the
// display shape sheet_feature_choices needs — resolving each slot's real
// option list. internal/features can't import charsheet.SkillAbility itself
// (charsheet imports features, not the other way around), so a skill pick's
// full option list is resolved here instead; a saving-throw pick already
// carries its fixed ability-code options straight from the slot. A method
// (not a plain function) because ChoiceArmorProperty's own options come
// from rules.db's armor_properties table, which only this layer can reach.
func (s *server) buildPendingFeatureChoiceRows(slots []features.ChoiceSlot) ([]pendingFeatureChoiceRow, error) {
	skillNames := make([]string, 0, len(charsheet.SkillAbility))
	for name := range charsheet.SkillAbility {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)

	// Loaded at most once, lazily — only Nearly Perfected Architecture's 2
	// slots ever need it, and most renders have zero pending
	// ChoiceArmorProperty slots at all.
	var armorProperties []featureChoiceOption
	var armorPropertiesLoaded bool

	rows := make([]pendingFeatureChoiceRow, 0, len(slots))
	for _, slot := range slots {
		row := pendingFeatureChoiceRow{FeatureSlug: slot.FeatureSlug, ChoiceIndex: slot.ChoiceIndex, Label: slot.Label}
		switch slot.Kind {
		case features.ChoiceSkillProficiency:
			for _, name := range skillNames {
				row.Options = append(row.Options, featureChoiceOption{Value: name, Label: name})
			}
		case features.ChoiceSavingThrowProficiency:
			for _, ab := range slot.Options {
				row.Options = append(row.Options, featureChoiceOption{Value: ab, Label: abilityLabels[ab] + " Saving Throw"})
			}
		case features.ChoiceAbilityScoreIncrease:
			row.IsAbilityScore = true
			for _, ab := range slot.Options {
				row.Options = append(row.Options, featureChoiceOption{
					Value: ab,
					Label: fmt.Sprintf("%s (+%d)", abilityLabels[ab], slot.Amount),
					Description: fmt.Sprintf(
						"Permanently increases your %s score by +%d. This pick cannot be changed later.",
						abilityLabels[ab], slot.Amount),
				})
			}
		case features.ChoiceUpgradeEntry:
			for _, entrySlug := range slot.Options {
				var description string
				if err := s.rulesDB.QueryRow(`SELECT description FROM class_option_entries WHERE slug = ?`, entrySlug).Scan(&description); err != nil && err != sql.ErrNoRows {
					return nil, err
				}
				row.Options = append(row.Options, featureChoiceOption{
					Value: entrySlug, Label: puppetUpgradeEntryChoiceLabels[entrySlug], Description: description,
				})
			}
		case features.ChoiceCompanionAbilityScoreIncrease:
			for _, ab := range slot.Options {
				row.Options = append(row.Options, featureChoiceOption{
					Value: ab,
					Label: fmt.Sprintf("%s (+%d)", abilityLabels[ab], slot.Amount),
					Description: fmt.Sprintf(
						"Increases every Puppet Tool you possess's %s score by +%d (max %d). This pick cannot be changed later.",
						abilityLabels[ab], slot.Amount, slot.Cap),
				})
			}
		case features.ChoiceNamedJutsuGrant:
			for _, jutsuSlug := range slot.Options {
				var name, description string
				if err := s.rulesDB.QueryRow(`SELECT name, description FROM jutsu WHERE slug = ?`, jutsuSlug).Scan(&name, &description); err != nil && err != sql.ErrNoRows {
					return nil, err
				}
				row.Options = append(row.Options, featureChoiceOption{
					Value: jutsuSlug, Label: name, Description: description,
				})
			}
		case features.ChoiceArmorProperty:
			if !armorPropertiesLoaded {
				var err error
				armorProperties, err = s.armorPropertyChoiceOptions()
				if err != nil {
					return nil, err
				}
				armorPropertiesLoaded = true
			}
			row.Options = armorProperties
		case features.ChoiceSymphonyBranch:
			for _, branch := range slot.Options {
				row.Options = append(row.Options, featureChoiceOption{Value: branch, Label: symphonyBranchChoiceLabels[branch]})
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// armorPropertyChoiceOptions loads every Armor Property from rules.db as a
// ChoiceArmorProperty slot's own option list — resolved once per call to
// buildPendingFeatureChoiceRows (at most one ChoiceArmorProperty feature
// exists today, Nearly Perfected Architecture's own 2 slots, so this never
// runs more than twice per render).
func (s *server) armorPropertyChoiceOptions() ([]featureChoiceOption, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, description FROM armor_properties ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []featureChoiceOption
	for rows.Next() {
		var o featureChoiceOption
		if err := rows.Scan(&o.Value, &o.Label, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// armorPropertyValid reports whether slug is a real armor_properties row —
// ChoiceArmorProperty's own server-side validation, since its real option
// list lives in rules.db rather than a fixed slice featureChoiceOptionValid
// can loop over.
func (s *server) armorPropertyValid(slug string) (bool, error) {
	var exists bool
	err := s.rulesDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM armor_properties WHERE slug = ?)`, slug).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// featureChoiceOptionValid re-derives the same option list
// buildPendingFeatureChoiceRows offers, for handleSheetFeatureChoice's own
// server-side validation — the picker's <option> list is a UI convenience,
// not the real enforcement, same rule every other picker on this sheet
// follows (see e.g. masteryNameEligible). ChoiceArmorProperty is validated
// against rules.db directly (armorPropertyValid) rather than here, since
// its real option list isn't a fixed slice this function can loop over.
func featureChoiceOptionValid(kind features.ChoiceKind, options []string, value string) bool {
	switch kind {
	case features.ChoiceSkillProficiency:
		_, ok := charsheet.SkillAbility[value]
		return ok
	case features.ChoiceUpgradeEntry:
		_, ok := puppetUpgradeEntryChoiceLabels[value]
		return ok
	case features.ChoiceSavingThrowProficiency, features.ChoiceAbilityScoreIncrease, features.ChoiceSymphonyBranch, features.ChoiceCompanionAbilityScoreIncrease, features.ChoiceNamedJutsuGrant:
		for _, o := range options {
			if o == value {
				return true
			}
		}
	}
	return false
}

// handleSheetFeatureChoice records a player's pick for one pending choice
// slot (form fields "feature_slug", "choice_index", "value"). It re-derives
// the character's currently-unlocked slots from a fresh Compute rather than
// trusting the posted feature_slug/choice_index blindly, so a stale or
// hand-crafted request can't record a pick for a slot the character doesn't
// actually qualify for (e.g. a level that's since been edited back down).
func (s *server) handleSheetFeatureChoice(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	featureSlug := strings.TrimSpace(r.FormValue("feature_slug"))
	choiceIndex, convErr := strconv.Atoi(r.FormValue("choice_index"))
	value := strings.TrimSpace(r.FormValue("value"))
	if featureSlug == "" || convErr != nil || value == "" {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for feature choice:", err)
		return
	}
	var slot *features.ChoiceSlot
	for i := range sheet.PendingFeatureChoices {
		if sheet.PendingFeatureChoices[i].FeatureSlug == featureSlug && sheet.PendingFeatureChoices[i].ChoiceIndex == choiceIndex {
			slot = &sheet.PendingFeatureChoices[i]
			break
		}
	}
	if slot == nil {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}
	if slot.Kind == features.ChoiceArmorProperty {
		valid, err := s.armorPropertyValid(value)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("validate armor property choice:", err)
			return
		}
		if !valid {
			http.Error(w, "not a valid pick for this choice", http.StatusBadRequest)
			return
		}
	} else if !featureChoiceOptionValid(slot.Kind, slot.Options, value) {
		http.Error(w, "not a valid pick for this choice", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, featureSlug, choiceIndex, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set feature choice:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_feature_choices")
}

// pendingASIRow is one unresolved Ability Score Improvement-or-Feat pick as
// shown on the sheet. Options is the ability list for the "one score +2"
// method (any ability under 20); SplitOptions is the same list widened to
// include anything under 20 that a +1 (rather than +2) can still land on —
// the two lists differ only at a score of exactly 19, which can take a +1
// but not a +2. FeatOptions is every feat this character currently qualifies
// for and does not already have, so this breakpoint's Feat alternative can
// never offer something a player couldn't actually take.
type pendingASIRow struct {
	Ref          string
	ClassName    string
	Level        int
	Options      []featureChoiceOption // one score +2; excludes anything at 19 or 20
	SplitOptions []featureChoiceOption // two scores +1 each; excludes only a score already at 20
	FeatOptions  []featOptionWithDescription
}

// featOptionWithDescription is one qualifying feat offered by an ASI
// breakpoint's Feat alternative — carries its description alongside the
// name/slug so the picker can show a rollover tooltip without a second
// round trip, the same "server renders it once, JS just toggles what's
// already there" approach the rest of this sheet's tooltips already use.
type featOptionWithDescription struct {
	Slug        string
	Name        string
	Description string
	// Highlight marks a Clan/Class-category feat, same blue treatment and
	// same featIsClanOrClassCategory check loadSheetFeatLibrary's own
	// entries get (sheet_library.go) — kept in sync by construction since
	// both read the same feats.category column.
	Highlight bool
}

// buildPendingASIRows turns Sheet.PendingASISlots into the display shape
// sheet_feature_choices needs. Ability options exclude anything the book's
// own cap already forbids ("you can't increase an ability score above 20
// using this feature") for whichever method they back; feat options reuse
// the exact eligibility check the Feats tab's own library pane already
// applies (buildFeatCharacter + parseFeatPrereq + Meets, see
// loadSheetFeatLibrary), so a feat this picker offers is always one the
// Feats tab would also consider the character qualifies for.
func (s *server) buildPendingASIRows(characterID int64, slots []features.ASISlot, sheet *charsheet.Sheet) ([]pendingASIRow, error) {
	if len(slots) == 0 {
		return nil, nil
	}
	characterFeats, err := s.loadCharacterFeats(characterID)
	if err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(characterFeats))
	for _, f := range characterFeats {
		owned[f.Slug] = true
	}
	featChar, err := s.buildFeatCharacter(characterID, sheet, characterFeats)
	if err != nil {
		return nil, err
	}
	featOptions, err := s.eligibleFeatOptions(owned, featChar)
	if err != nil {
		return nil, err
	}

	rows := make([]pendingASIRow, 0, len(slots))
	for _, slot := range slots {
		row := pendingASIRow{Ref: slot.Ref, ClassName: s.className(slot.ClassSlug), Level: slot.Level, FeatOptions: featOptions}
		for _, ab := range charsheet.Abilities {
			score := sheet.Abilities[ab].Score
			max := sheet.AbilityMax[ab]
			if max == 0 {
				max = 20
			}
			labelText := fmt.Sprintf("%s (%d)", abilityLabels[ab], score)
			if max != 20 {
				labelText = fmt.Sprintf("%s (%d, max %d)", abilityLabels[ab], score, max)
			}
			label := featureChoiceOption{Value: ab, Label: labelText}
			if score < max-1 {
				row.Options = append(row.Options, label)
			}
			if score < max {
				row.SplitOptions = append(row.SplitOptions, label)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// eligibleFeatOptions lists every feat the character qualifies for and does
// not already have, for the Feat alternative on an ASI breakpoint. Mirrors
// loadSheetFeatLibrary's own eligibility check (cmd/n5e/sheet_library.go)
// exactly, rather than a second, potentially-divergent notion of "qualifies
// for" — feats named in one another's prerequisites still need the full
// knownFeats set built first, same reasoning as that function's own doc
// comment.
func (s *server) eligibleFeatOptions(owned map[string]bool, character featCharacter) ([]featOptionWithDescription, error) {
	knownClasses, err := s.knownClassNames()
	if err != nil {
		return nil, err
	}
	knownClans, err := s.knownClanNames()
	if err != nil {
		return nil, err
	}

	rows, err := s.rulesDB.Query(`SELECT slug, name, category, COALESCE(prerequisites, ''), COALESCE(description, '') FROM feats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type featRow struct{ slug, name, category, prerequisites, description string }
	var featRows []featRow
	knownFeats := map[string]bool{}
	for rows.Next() {
		var f featRow
		if err := rows.Scan(&f.slug, &f.name, &f.category, &f.prerequisites, &f.description); err != nil {
			return nil, err
		}
		featRows = append(featRows, f)
		knownFeats[strings.ToLower(f.name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []featOptionWithDescription
	for _, f := range featRows {
		if owned[f.slug] {
			continue
		}
		if f.prerequisites != "" && !character.Meets(parseFeatPrereq(f.prerequisites, character.AllSkills, knownFeats, knownClasses, knownClans)) {
			continue
		}
		out = append(out, featOptionWithDescription{Slug: f.slug, Name: f.name, Description: f.description, Highlight: featIsClanOrClassCategory(f.category)})
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}

// handleSheetASI records a player's pick for one Ability Score Improvement
// breakpoint — either the ability-increase branch (form field "mode"="asi",
// default when omitted for old bookmarked forms: "ability1" and, for the
// "two scores +1 each" method, "ability2") or the Feat branch
// (mode="feat", "feat_slug"). A plain form POST either way, not a
// sheet-fetch-form: an ability increase ripples into Skills/Saves/AC/HP/
// Chakra/attack modifiers/tool proficiencies all at once — the same breadth
// sheet-abilities.js's own ability-score editor already decided isn't safe
// to enumerate as a data-also-refresh list — and a granted Feat can be
// exactly as broad (a proficiency or AC-swap grant), so both branches fall
// back to the browser's own real page reload after the form submits.
func (s *server) handleSheetASI(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(r.FormValue("ref"))
	if ref == "" {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for asi:", err)
		return
	}
	qualifies := false
	for _, slot := range sheet.PendingASISlots {
		if slot.Ref == ref {
			qualifies = true
			break
		}
	}
	if !qualifies {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.FormValue("mode")))
	if mode == "" {
		mode = "asi"
	}

	switch mode {
	case "asi":
		currentScores := make(map[string]int, len(charsheet.Abilities))
		for _, ab := range charsheet.Abilities {
			currentScores[ab] = sheet.Abilities[ab].Score
		}
		var picks []features.AbilityPick
		ability1 := strings.ToLower(strings.TrimSpace(r.FormValue("ability1")))
		ability2 := strings.ToLower(strings.TrimSpace(r.FormValue("ability2")))
		if !slices.Contains(charsheet.Abilities, ability1) {
			http.Error(w, "not a valid ability pick", http.StatusBadRequest)
			return
		}
		if ability2 == "" {
			picks = []features.AbilityPick{{Ability: ability1, Amount: 2}}
		} else {
			if !slices.Contains(charsheet.Abilities, ability2) {
				http.Error(w, "not a valid ability pick", http.StatusBadRequest)
				return
			}
			picks = []features.AbilityPick{{Ability: ability1, Amount: 1}, {Ability: ability2, Amount: 1}}
		}
		if !features.ValidASIPicks(picks, currentScores, sheet.AbilityMax) {
			http.Error(w, "not a valid ability score improvement", http.StatusBadRequest)
			return
		}
		storePicks := make([]charstore.AbilityPick, len(picks))
		for i, p := range picks {
			storePicks[i] = charstore.AbilityPick{Ability: p.Ability, Amount: p.Amount}
		}
		if err := charstore.SetAbilityScoreImprovement(s.charDB, id, ref, storePicks); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set ability score improvement:", err)
			return
		}
	case "feat":
		featSlug := strings.TrimSpace(r.FormValue("feat_slug"))
		if featSlug == "" {
			http.Error(w, "missing or invalid choice", http.StatusBadRequest)
			return
		}
		characterFeats, err := s.loadCharacterFeats(id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load character feats for asi feat pick:", err)
			return
		}
		owned := make(map[string]bool, len(characterFeats))
		for _, f := range characterFeats {
			owned[f.Slug] = true
		}
		featChar, err := s.buildFeatCharacter(id, sheet, characterFeats)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("build feat eligibility context for asi feat pick:", err)
			return
		}
		eligible, err := s.eligibleFeatOptions(owned, featChar)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load eligible feats for asi feat pick:", err)
			return
		}
		var featDescription string
		found := false
		for _, f := range eligible {
			if f.Slug == featSlug {
				found = true
				featDescription = f.Description
				break
			}
		}
		if !found {
			http.Error(w, "not a feat you currently qualify for", http.StatusBadRequest)
			return
		}
		if err := charstore.SetAbilityScoreImprovementFeat(s.charDB, id, ref, featSlug, s.characterLevel(id)); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set ability score improvement feat:", err)
			return
		}
		// Same fixed-single-ability auto-apply as handleSheetFeatAdd — a feat
		// granted through this ASI breakpoint's Feat alternative is still a
		// feat, and still owes its own ability bonus on top of whatever the
		// breakpoint itself grants.
		if abilities, amount, ok := parseFeatAbilityIncrease(featSlug, featDescription); ok && len(abilities) == 1 {
			if err := charstore.ApplyFeatAbilityBonus(s.charDB, id, featSlug, abilities[0], amount); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("apply feat ability bonus (asi feat alternative):", err)
				return
			}
		}
		// Same fixed-single-skill auto-apply as handleSheetFeatAdd, for the
		// same reason as the ability bonus above.
		if skill, ok := parseFeatSkillProficiency(featSlug, featDescription); ok {
			if err := s.applyFeatProficiencyGrant(id, featSlug, "skill", skill, featDescription); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("apply feat skill proficiency (asi feat alternative):", err)
				return
			}
		}
		// Same fixed-single-category auto-apply as handleSheetFeatAdd, for a
		// weapon-category grant instead.
		if category, ok := parseFeatWeaponProficiency(featDescription); ok {
			if err := charstore.ApplyFeatWeaponProficiency(s.charDB, id, featSlug, category); err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("apply feat weapon proficiency (asi feat alternative):", err)
				return
			}
		}
	default:
		http.Error(w, "unknown choice mode", http.StatusBadRequest)
		return
	}
	redirectToSheet(w, r, id)
}
