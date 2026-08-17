package main

import (
	"database/sql"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// Elemental (Nature Release) affinities gate which elemental jutsu a
// character can learn — a jutsu's own keywords column (e.g. "Fire Release,
// Ninjutsu") names its element, and a character can only learn it with a
// matching affinity, from one of three sources: a clan trait, the Nature
// Release feat, or (for one specific subclass) a class feature. Hand-
// verified against dist/rules.db's clan_traits/clan_features/
// subclass_features tables in full — every clan trait naming a Release
// Affinity is accounted for below, not a sample.
//
// elementNames is every element a jutsu's keywords column can name.
var elementNames = []string{"Fire", "Water", "Earth", "Wind", "Lightning"}

// elementReleaseKeyword is the exact substring a jutsu's own keywords
// column uses for one element (e.g. "Fire" -> "Fire Release") — matched
// with strings.Contains rather than an exact comma-split token match, since
// a couple of rows in the shipped data glue two keywords together with no
// separating comma (e.g. "Bukijutsu Earth Release").
func elementReleaseKeyword(element string) string {
	return element + " Release"
}

// jutsuElement returns the element a jutsu's keywords name, or "" if it
// names none (a non-elemental jutsu) — "Any Nature Release" is reported as
// "" too (handled separately by jutsuElementRequirementMet, since it's
// satisfied by ANY affinity rather than one specific element).
func jutsuElement(keywords string) string {
	for _, el := range elementNames {
		if strings.Contains(keywords, elementReleaseKeyword(el)) {
			return el
		}
	}
	return ""
}

// jutsuNeedsAnyAffinity reports the "Any Nature Release" keyword some
// jutsu use (e.g. a Puppet Master upgrade note, or a versatile technique) —
// satisfied by having at least one affinity at all, not one specific
// element.
func jutsuNeedsAnyAffinity(keywords string) bool {
	return strings.Contains(keywords, "Any Nature Release")
}

// clanFlatAffinity: clans whose own trait grants one element outright, no
// player choice involved (clan_traits, description NOT phrased "(Pick one)").
var clanFlatAffinity = map[string]string{
	"clan/chinoike":  "Water",
	"clan/fushin":    "Wind",
	"clan/genwa":     "Lightning",
	"clan/hatake":    "Lightning",
	"clan/hoshigaki": "Water",
	"clan/hozuki":    "Water",
	"clan/iburi":     "Fire",
	"clan/konjiki":   "Earth",
	"clan/shoton":    "Earth",
	"clan/uchiha":    "Fire",
}

// clanComboAffinityOptions: "kekkei genkai combo" clans whose own trait
// grants a 1st-level CHOICE of one of two elements (clan_traits,
// "(Pick one)"), with the other element of the pair granted automatically
// once the clan's own 7th-level feature (clanComboSecondReleaseFeature)
// unlocks — "you gain the second Nature release you didn't select," in
// every one of these clans' own printed text.
var clanComboAffinityOptions = map[string][2]string{
	"clan/bakuton":  {"Earth", "Lightning"},
	"clan/futton":   {"Water", "Fire"},
	"clan/jiton":    {"Earth", "Wind"},
	"clan/keton":    {"Fire", "Lightning"},
	"clan/namikaze": {"Wind", "Lightning"},
	"clan/ranton":   {"Water", "Lightning"},
	"clan/senju":    {"Earth", "Water"},
	"clan/shakuton": {"Wind", "Fire"},
	"clan/yoton":    {"Earth", "Fire"},
	"clan/yuki":     {"Wind", "Water"},
}

// clanComboSecondReleaseFeature: combo clan slug -> the clan_features slug
// whose own text grants the character the OTHER half of its pair, once
// loadGrantedFeatures says the character actually has it (that function
// already gates on the feature's own level, currently 7th for every one of
// these — reused here rather than re-deriving the level check by hand).
var clanComboSecondReleaseFeature = map[string]string{
	"clan/bakuton":  "clan/bakuton/feature/explosion-release",
	"clan/futton":   "clan/futton/feature/boil-release",
	"clan/jiton":    "clan/jiton/feature/magnet-release",
	"clan/keton":    "clan/keton/feature/plasma-release",
	"clan/namikaze": "clan/namikaze/feature/swift-release",
	"clan/ranton":   "clan/ranton/feature/storm-release",
	"clan/senju":    "clan/senju/feature/wood-release",
	"clan/shakuton": "clan/shakuton/feature/scorch-release",
	"clan/yoton":    "clan/yoton/feature/lava-release",
	"clan/yuki":     "clan/yuki/feature/ice-release",
}

// natureReleaseFeatSlug: the Nature Release feat ("You select one of the 5
// Following Nature Releases... You can take this feat more than once, each
// time selecting a different nature release"). Repeated takes aren't
// representable — character_feats is keyed (character_id, feat_slug), one
// row per feat, so a second take is a no-op today — a real, pre-existing
// gap in feat storage generally, not something invented for this feature.
// Only a single pick is supported here as a result.
const natureReleaseFeatSlug = "feat/nature-release"

// professorAffinitySlots: Ninjutsu Specialist's "The Professor" subclass
// grants an independent elemental pick from each of these three features,
// in order — "Versatile Release" (2nd level, one pick), "Twin Cast" (6th,
// a second pick, must differ from the first), "Soshikage" (14th, a third,
// must differ from both). Gated on loadGrantedFeatures rather than a
// hand-checked level number, same reasoning as the clan combo grant above.
var professorAffinitySlots = []struct {
	SlotKey      string
	FeatureSlug  string
	FeatureLabel string
}{
	{"versatile-release", "class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/versatile-release", "Versatile Release"},
	{"twin-cast", "class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/twin-cast", "Twin Cast"},
	{"soshikage", "class/ninjutsu-specialist/group/ninjutsu-focus/the-professor/feature/soshikage", "Soshikage"},
}

// elementalAffinity is one resolved element the character currently has,
// with the feature/trait/feat name it came from (for the library panel's
// own tooltip — see the Resistances & Senses panel for the same pattern).
type elementalAffinity struct {
	Element string
	Source  string
}

// elementalAffinitySlot is one open or filled "choose an element" slot the
// sheet's picker needs to render — Options is empty for the Nature Release
// feat slot's own free choice of any of the 5.
type elementalAffinitySlot struct {
	SlotKey string
	Label   string
	Options []string // empty means "any of the 5"
	Current string   // "" if not yet chosen
}

// resolveElementalAffinities computes every element the character
// currently has and where each came from — flat clan grants need no picks
// at all; combo clans/the feat/Professor need whatever's already stored in
// picks (charstore.ListElementalAffinities).
//
// Elemental Reactor is deliberately NOT a source here: its own book table
// (see puppet_upgrade_jutsu_grants.go) grants specific named jutsu directly,
// not a standing elemental affinity — corrected after the first reading
// ("gaining the jutsu associated with it") turned out to mean literal named
// jutsu grants, confirmed by the printed table once it was actually found
// and hand-transcribed (it isn't PDF-extractable text, same as Armor
// Chassis), not "now eligible to learn any jutsu of this element."
func resolveElementalAffinities(clanSlug string, hasNatureReleaseFeat bool, grantedFeatureSlugs map[string]bool, picks map[string]string) []elementalAffinity {
	var out []elementalAffinity
	if el, ok := clanFlatAffinity[clanSlug]; ok {
		out = append(out, elementalAffinity{Element: el, Source: "Clan trait"})
	}
	if pair, ok := clanComboAffinityOptions[clanSlug]; ok {
		if picked := picks["clan"]; picked == pair[0] || picked == pair[1] {
			out = append(out, elementalAffinity{Element: picked, Source: "Clan trait"})
		}
		if grantedFeatureSlugs[clanComboSecondReleaseFeature[clanSlug]] {
			out = append(out, elementalAffinity{Element: pair[0], Source: "Clan feature (7th level)"})
			out = append(out, elementalAffinity{Element: pair[1], Source: "Clan feature (7th level)"})
		}
	}
	if hasNatureReleaseFeat {
		if picked := picks[natureReleaseFeatSlug]; picked != "" {
			out = append(out, elementalAffinity{Element: picked, Source: "Nature Release feat"})
		}
	}
	for _, slot := range professorAffinitySlots {
		if grantedFeatureSlugs[slot.FeatureSlug] {
			if picked := picks[slot.SlotKey]; picked != "" {
				out = append(out, elementalAffinity{Element: picked, Source: slot.FeatureLabel})
			}
		}
	}

	// Dedupe (a combo clan's own 7th-level grant can repeat an element the
	// character's own 1st-level pick already named), keeping the FIRST
	// source seen for a given element as the one worth showing.
	seen := map[string]bool{}
	deduped := out[:0]
	for _, a := range out {
		if seen[a.Element] {
			continue
		}
		seen[a.Element] = true
		deduped = append(deduped, a)
	}
	sort.Slice(deduped, func(i, k int) bool { return deduped[i].Element < deduped[k].Element })
	return deduped
}

// elementalAffinitySlots lists every slot the sheet's picker should show
// for this character right now — a combo clan's own 1st-level choice, the
// Nature Release feat's pick, and any Professor subclass slot the
// character has actually reached. Empty entries (nothing applicable) are
// simply omitted, the same "no box for nothing to configure" rule
// Puppet Tactics/Generalized Skill already follow.
func elementalAffinitySlots(clanSlug string, hasNatureReleaseFeat bool, grantedFeatureSlugs map[string]bool, picks map[string]string) []elementalAffinitySlot {
	var out []elementalAffinitySlot
	if pair, ok := clanComboAffinityOptions[clanSlug]; ok {
		out = append(out, elementalAffinitySlot{
			SlotKey: "clan", Label: "Clan Affinity (pick one)",
			Options: []string{pair[0], pair[1]}, Current: picks["clan"],
		})
	}
	if hasNatureReleaseFeat {
		out = append(out, elementalAffinitySlot{
			SlotKey: natureReleaseFeatSlug, Label: "Nature Release (feat)",
			Options: append([]string(nil), elementNames...), Current: picks[natureReleaseFeatSlug],
		})
	}
	for _, slot := range professorAffinitySlots {
		if grantedFeatureSlugs[slot.FeatureSlug] {
			out = append(out, elementalAffinitySlot{
				SlotKey: slot.SlotKey, Label: slot.FeatureLabel,
				Options: append([]string(nil), elementNames...), Current: picks[slot.SlotKey],
			})
		}
	}
	return out
}

// elementalAffinitySlotKeys is every valid slot key the picker's own add
// handler may write to — a whitelist, since slot_key comes straight off a
// form field.
var elementalAffinitySlotKeys = func() map[string]bool {
	set := map[string]bool{"clan": true, natureReleaseFeatSlug: true}
	for _, slot := range professorAffinitySlots {
		set[slot.SlotKey] = true
	}
	return set
}()

// handleElementalAffinityAdd sets (or changes) one affinity slot's pick
// (form fields "slot_key", "element"). Re-derives the character's own
// currently-open slots server-side rather than trusting the form — the
// disabled/hidden state in the rendered picker is a UI convenience, not the
// real enforcement, same as every other gate on this sheet.
func (s *server) handleElementalAffinityAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slotKey := strings.TrimSpace(r.FormValue("slot_key"))
	element := strings.TrimSpace(r.FormValue("element"))
	if !elementalAffinitySlotKeys[slotKey] {
		http.Error(w, "unknown affinity slot", http.StatusBadRequest)
		return
	}

	var clanSlug sql.NullString
	if err := s.charDB.QueryRow(`SELECT clan_slug FROM characters WHERE id = ?`, id).Scan(&clanSlug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load clan for elemental affinity add:", err)
		return
	}
	level := s.characterLevel(id)
	grantedFeatures, err := s.loadGrantedFeatures(id, clanSlug.String, level)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load granted features for elemental affinity add:", err)
		return
	}
	grantedSlugs := map[string]bool{}
	for _, f := range grantedFeatures {
		grantedSlugs[f.Slug] = true
	}
	hasFeat, err := s.characterHasFeat(id, natureReleaseFeatSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("check nature release feat for elemental affinity add:", err)
		return
	}
	picks, err := charstore.ListElementalAffinities(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load elemental affinity picks for add:", err)
		return
	}

	var matched *elementalAffinitySlot
	for _, slot := range elementalAffinitySlots(clanSlug.String, hasFeat, grantedSlugs, picks) {
		if slot.SlotKey == slotKey {
			s := slot
			matched = &s
			break
		}
	}
	if matched == nil {
		http.Error(w, "this affinity slot isn't open for this character", http.StatusBadRequest)
		return
	}
	if !slices.Contains(matched.Options, element) {
		http.Error(w, "not a valid choice for this slot", http.StatusBadRequest)
		return
	}

	if err := charstore.SetElementalAffinity(s.charDB, id, slotKey, element); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set elemental affinity:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_elemental_affinities")
}
