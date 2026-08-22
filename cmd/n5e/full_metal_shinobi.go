package main

import (
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// Shinobi-Ware's Full-Metal Shinobi (3rd level): "At Level 6 you choose to
// gain resistance to either Bludgeoning, Piercing, or Slashing damage. At
// Level 9 you can choose another. At Level 14 you gain the last that you did
// not choose." Two independent player picks (6th/9th level), the third
// damage type is automatic once 14th level is reached — same
// "affinityFeatureSlot, staged across levels" shape elemental_affinity.go's
// professorAffinitySlots already models, just for a fixed 3-option catalog
// (Bludgeoning/Piercing/Slashing) instead of the 5 elements, with a real
// auto-complete clause at the final level instead of a third open pick.

// fullMetalShinobiDamageTypes is every damage type Full-Metal Shinobi can
// grant resistance to, in the order the book lists them.
var fullMetalShinobiDamageTypes = []string{"Bludgeoning", "Piercing", "Slashing"}

// fullMetalShinobiSlotDef is one of the feature's two player-choice slots.
type fullMetalShinobiSlotDef struct {
	SlotKey  string
	Label    string
	MinLevel int
}

// fullMetalShinobiSlotDefs: the 6th and 9th level picks. The 14th-level
// "last one" needs no slot of its own — it's computed automatically by
// fullMetalShinobiResistances once both slots are open (or level allows).
var fullMetalShinobiSlotDefs = []fullMetalShinobiSlotDef{
	{"sixth-level", "6th Level", 6},
	{"ninth-level", "9th Level", 9},
}

// fullMetalShinobiSlot is one open pick slot the sheet's picker should show,
// same "list every open slot, empty means not shown" shape
// elementalAffinitySlot already establishes.
type fullMetalShinobiSlot struct {
	SlotKey string
	Label   string
	Options []string
	Current string
}

// fullMetalShinobiResolvedPicks returns every valid damage type actually
// stored across the character's own two slots, ignoring a slot whose stored
// value isn't (or is no longer) one of the three valid damage types.
func fullMetalShinobiResolvedPicks(picks map[string]string) []string {
	var out []string
	for _, def := range fullMetalShinobiSlotDefs {
		if v := picks[def.SlotKey]; slices.Contains(fullMetalShinobiDamageTypes, v) {
			out = append(out, v)
		}
	}
	return out
}

// fullMetalShinobiResistances resolves every damage type the character
// currently has resistance to via Full-Metal Shinobi, gating each slot's
// own pick on the character having actually reached that slot's level, and
// filling in the remaining, not-yet-picked damage type(s) once level 14 is
// reached ("you gain the last that you did not choose") — deterministically
// in fullMetalShinobiDamageTypes order, so a character who never made an
// earlier pick still ends up with all three at 14th, matching the book's
// guaranteed end state regardless of pick order.
func fullMetalShinobiResistances(level int, picks map[string]string) []string {
	var out []string
	for _, def := range fullMetalShinobiSlotDefs {
		if level < def.MinLevel {
			continue
		}
		if v := picks[def.SlotKey]; slices.Contains(fullMetalShinobiDamageTypes, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	if level >= 14 {
		for _, dt := range fullMetalShinobiDamageTypes {
			if !slices.Contains(out, dt) {
				out = append(out, dt)
			}
		}
	}
	return out
}

// fullMetalShinobiSlots lists every 6th/9th level slot the character has
// reached, each offering the damage types not already resolved elsewhere
// (excluding its own current pick, so it stays visible in its own slot).
func fullMetalShinobiSlots(level int, picks map[string]string) []fullMetalShinobiSlot {
	resolved := fullMetalShinobiResolvedPicks(picks)
	var out []fullMetalShinobiSlot
	for _, def := range fullMetalShinobiSlotDefs {
		if level < def.MinLevel {
			continue
		}
		current := picks[def.SlotKey]
		var options []string
		for _, dt := range fullMetalShinobiDamageTypes {
			if dt == current || !slices.Contains(resolved, dt) {
				options = append(options, dt)
			}
		}
		out = append(out, fullMetalShinobiSlot{SlotKey: def.SlotKey, Label: def.Label, Options: options, Current: current})
	}
	return out
}

// fullMetalShinobiSlotKeys is every valid slot key the picker's own add
// handler may write to — a whitelist, since slot_key comes straight off a
// form field, same boundary elementalAffinitySlotKeys already draws.
var fullMetalShinobiSlotKeys = func() map[string]bool {
	set := map[string]bool{}
	for _, def := range fullMetalShinobiSlotDefs {
		set[def.SlotKey] = true
	}
	return set
}()

// fullMetalShinobiPassiveRows returns synthetic grantedFeatureRow entries —
// one per damage type the character currently holds resistance to via
// Full-Metal Shinobi — for computePassiveTraits to fold in, the same
// "append, don't merge into the stored list" treatment
// hunterNinPatternPassiveRows already gives Hunters Patterns. Each synthetic
// slug has its own passiveTraitGrants entry (passive_traits.go); the real
// feature slug (scienceNinFullMetalShinobiFeatureSlug) intentionally has no
// entry of its own any more, since a flat "granted at 3rd level" row could
// only ever represent an unconditional trait, not this feature's own
// level/pick-staged one.
//
// grantedFeatures gates this on the character actually holding
// scienceNinFullMetalShinobiFeatureSlug (Shinobi-Ware, 3rd level) — unlike
// hunterNinPatternPassiveRows, fullMetalShinobiResistances' own 14th-level
// auto-complete clause produces output from an EMPTY picks map, so without
// this gate any character of any class simply reaching character level 14
// would silently gain all three damage resistances.
func (s *server) fullMetalShinobiPassiveRows(characterID int64, level int, grantedFeatures []grantedFeatureRow) ([]grantedFeatureRow, error) {
	if !hasGrantedFeature(grantedFeatures, scienceNinFullMetalShinobiFeatureSlug) {
		return nil, nil
	}
	picks, err := charstore.ListFullMetalShinobiResistances(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	names := map[string]string{
		"Bludgeoning": "Full-Metal Shinobi (Bludgeoning)",
		"Piercing":    "Full-Metal Shinobi (Piercing)",
		"Slashing":    "Full-Metal Shinobi (Slashing)",
	}
	var rows []grantedFeatureRow
	for _, dt := range fullMetalShinobiResistances(level, picks) {
		rows = append(rows, grantedFeatureRow{
			Slug:        fullMetalShinobiPassiveTraitSlug(dt),
			Name:        names[dt],
			SourceLabel: "Full-Metal Shinobi",
		})
	}
	return rows, nil
}

// fullMetalShinobiPassiveTraitSlug is the synthetic passiveTraitGrants key
// for one damage type's own resistance — never a real rules-database slug,
// scoped under the real feature's own slug so it reads clearly in the
// Sources tooltip and can never collide with an actual feature row.
func fullMetalShinobiPassiveTraitSlug(damageType string) string {
	return scienceNinFullMetalShinobiFeatureSlug + "/resistance/" + strings.ToLower(damageType)
}

// handleFullMetalShinobiResistanceAdd sets (or changes) one resistance
// slot's pick (form fields "slot_key", "damage_type"). Re-derives the
// character's own currently-open slots server-side rather than trusting the
// form, same boundary handleElementalAffinityAdd already draws.
func (s *server) handleFullMetalShinobiResistanceAdd(w http.ResponseWriter, r *http.Request) {
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
	damageType := strings.TrimSpace(r.FormValue("damage_type"))
	if !fullMetalShinobiSlotKeys[slotKey] {
		http.Error(w, "unknown resistance slot", http.StatusBadRequest)
		return
	}

	level := s.characterLevel(id)
	picks, err := charstore.ListFullMetalShinobiResistances(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load full-metal shinobi picks for add:", err)
		return
	}

	var matched *fullMetalShinobiSlot
	for _, slot := range fullMetalShinobiSlots(level, picks) {
		if slot.SlotKey == slotKey {
			s := slot
			matched = &s
			break
		}
	}
	if matched == nil {
		http.Error(w, "this resistance slot isn't open for this character", http.StatusBadRequest)
		return
	}
	if !slices.Contains(matched.Options, damageType) {
		http.Error(w, "not a valid choice for this slot", http.StatusBadRequest)
		return
	}

	if err := charstore.SetFullMetalShinobiResistance(s.charDB, id, slotKey, damageType); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set full-metal shinobi resistance:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}
