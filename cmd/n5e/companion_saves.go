package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file is a companion's own Saving Throws — computed, rollable, and
// (on the Companions tab card, not the read-only popup) editable via a
// per-ability proficiency toggle, mirroring the interaction SHAPE of the
// main sheet's own Ability Score box (a rollable tile whose number is
// itself the editable control) even though what's actually stored per
// companion is proficiency, not a raw score — a saving throw's number is
// always DERIVED (ability modifier + proficiency bonus if proficient),
// never a free value, so proficiency is the only thing that can honestly be
// "the editable part" here. See this project's own research notes on why
// there was no existing "edit a saving throw" pattern to copy verbatim.
//
// Scoped to every companion kind except "puppet" — a Puppet Tool's Saving
// Throws are a fixed class-text readout (companion_fields.html's own
// companion-base-traits block, sourced from puppetToolStatBlock), not a
// per-companion pick, so this box is never rendered for that kind at all
// (see companion_fields.html's own {{if ne .Companion.Kind "puppet"}}
// guard) rather than being computed-but-hidden.

// companionSaveRow is one ability's own computed saving-throw line.
type companionSaveRow struct {
	Ability    string
	Modifier   int
	Proficient bool
}

// companionSavesView is everything a companion's Saving Throws box needs:
// the six computed rows in charsheet.Abilities order, plus S.N.B's own
// "Proficient in 2 of your choice (3 at 14th level)" cap — Cap is 0 for
// every other kind, meaning "no cap enforced, no X/Y epithet shown", not
// "zero allowed".
type companionSavesView struct {
	Rows []companionSaveRow
	Cap  int
	Used int
}

// companionScoreModifier is charsheet.AbilityModifier applied to a
// companion's own nullable score field directly — distinct from puppets.go's
// own companionAbilityModifier, which resolves a three-letter ability KEY
// ("str", "dex", ...) against a whole charstore.Companion first; this one
// already has the raw score in hand (companionSaves resolves the ability key
// itself, once, into a small local map, rather than re-resolving it per
// call). Companions have no charsheet.Compute-equivalent pipeline of their
// own (see this project's own research notes), so this is the one place
// this formula gets reused for one. A NULL score (never set, e.g. most
// "summon"/"custom" companions have no computed baseline to prefill from)
// reads as a modifier of 0, the same as an ability score of exactly 10,
// rather than erroring or always reading as the worst case.
func companionScoreModifier(v sql.NullInt64) int {
	if !v.Valid {
		return 0
	}
	return charsheet.AbilityModifier(int(v.Int64))
}

// companionSaveAbilities lists the six ability codes a saving throw can be
// proficient in, in canonical display order — charsheet.Abilities directly,
// named here only for readability at each call site below.
var companionSaveAbilities = charsheet.Abilities

// companionSaveAbilitySet is companionSaveAbilities as a lookup set, for
// validating a raw "ability" form value before it ever reaches SQL.
var companionSaveAbilitySet = func() map[string]bool {
	set := make(map[string]bool, len(companionSaveAbilities))
	for _, a := range companionSaveAbilities {
		set[a] = true
	}
	return set
}()

// parseSaveProficiencies turns a companion's stored comma-separated
// save_proficiencies column back into a lookup set, silently dropping any
// token that isn't one of the six known ability codes (defensive against a
// hand-edited or stale row — the same "never trust stored free text as
// already-valid" caution SetCompanionFields' own CASE WHEN guards apply
// elsewhere in this package).
func parseSaveProficiencies(raw string) map[string]bool {
	set := map[string]bool{}
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if companionSaveAbilitySet[tok] {
			set[tok] = true
		}
	}
	return set
}

// joinSaveProficiencies is parseSaveProficiencies' inverse — always in
// companionSaveAbilities' own fixed order regardless of insertion order, so
// the stored column stays stable/diffable across saves instead of reflecting
// whatever order a Go map happened to range in.
func joinSaveProficiencies(set map[string]bool) string {
	var out []string
	for _, a := range companionSaveAbilities {
		if set[a] {
			out = append(out, a)
		}
	}
	return strings.Join(out, ",")
}

// companionSaveCap returns S.N.B Specialist's own "Proficient in 2 of your
// choice (3 at 14th level)" cap, or 0 for every other kind (no cap
// enforced). Nin-Dog's "Proficient in all" and Titan's fixed STR/DEX/CON
// (mental saves "still use yours") are both stated outright in their own
// reference text (nindog_reference/titan_reference, companion_fields.html)
// rather than enforced here — unlike S.N.B's free "2 of your choice", they
// name a fixed set rather than a count budget, so there's nothing for a cap
// check to gate; the player is trusted to toggle them to match, the same way
// a fresh companion's ability scores start blank and are trusted to be
// filled in to match the book instead of every field being force-computed.
// Void Soul is a THIRD shape ("Uses your saving throw proficiencies" — no
// choice or fixed set at all, a strict mirror of the player's own list) and
// gets neither treatment: companionSaves below computes its proficiencies
// directly from sheet.Saves rather than trusting a manual toggle to match,
// since unlike the other two there's nothing here for a player to
// legitimately choose differently from the mirrored source.
func companionSaveCap(kind string, level int) int {
	if kind != "snb" {
		return 0
	}
	if level >= 14 {
		return 3
	}
	return 2
}

// companionSaves computes a companion's whole Saving Throws box: one row per
// ability in canonical order, each ability's own modifier being its ability
// score's modifier (0 for an unset score) plus the character's own
// proficiency bonus if this companion is proficient in that save. S.N.B
// Specialist's own base traits state "negative modifiers are treated as +0"
// for this companion kind specifically (companion_fields.html's snb_reference
// dt/dd text) — read as the ability modifier itself being floored at 0
// before proficiency is added, not the final total, so a proficient S.N.B
// with a below-10 score still gets the full proficiency bonus rather than
// proficiency "wasted" topping up a modifier that's already been floored to
// match a total of 0.
//
// Takes the whole sheet (every call site already had sheet.ProficiencyBonus/
// sheet.Level in hand) rather than those two ints separately, so a
// kind="void-soul" companion can mirror "Uses your saving throw
// proficiencies" directly off sheet.Saves instead of its own stored
// save_proficiencies column — not a player choice, so there is nothing
// legitimate for a manual toggle to diverge to (see companionSaveCap's own
// doc for how this differs from Nin-Dog/Titan's own fixed-but-unenforced
// shapes).
func companionSaves(c charstore.Companion, sheet *charsheet.Sheet) companionSavesView {
	profBonus, level := sheet.ProficiencyBonus, sheet.Level
	var profs map[string]bool
	if c.Kind == "void-soul" {
		profs = make(map[string]bool, len(sheet.Saves))
		for _, s := range sheet.Saves {
			if s.Proficient {
				profs[s.Ability] = true
			}
		}
	} else {
		profs = parseSaveProficiencies(c.SaveProficiencies)
	}
	scores := map[string]sql.NullInt64{
		"str": c.Str, "dex": c.Dex, "con": c.Con,
		"int": c.Int, "wis": c.Wis, "cha": c.Cha,
	}
	view := companionSavesView{Cap: companionSaveCap(c.Kind, level)}
	for _, ability := range companionSaveAbilities {
		mod := companionScoreModifier(scores[ability])
		if c.Kind == "snb" && mod < 0 {
			mod = 0
		}
		proficient := profs[ability]
		total := mod
		if proficient {
			total += profBonus
			view.Used++
		}
		view.Rows = append(view.Rows, companionSaveRow{Ability: ability, Modifier: total, Proficient: proficient})
	}
	return view
}

// handleCompanionSavingThrowToggle flips one companion's proficiency in one
// saving throw (form fields "ability", "on") — the Companions tab card's own
// .sheet-toggle-form (see sheet-toggles.js, already document-delegated and
// so safe inside a fragment that can be swapped wholesale by an unrelated
// action elsewhere on the same tab), never reachable from the read-only
// popup (companion_fields.html only renders the toggle form when
// ShowSaveToggles is set, which only the Companions tab card passes).
//
// S.N.B Specialist's own cap ("2 of your choice, 3 at 14th level") is
// enforced here, not just hinted at in the UI — same "a disabled control is
// a convenience, not the real enforcement" rule every other pick cap in this
// package follows. Every other kind has no cap at all (companionSaveCap
// returns 0), so this never blocks them.
func (s *server) handleCompanionSavingThrowToggle(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for saving throw toggle:", err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	// "Uses your saving throw proficiencies" is not a player choice —
	// companionSaves already ignores this companion's own save_proficiencies
	// column for kind="void-soul" and mirrors sheet.Saves instead
	// (companion_fields.html's own template hides the toggle button for the
	// same reason), so a direct POST here must be rejected server-side too,
	// the same "a disabled control is a convenience, not the real
	// enforcement" rule this handler's own S.N.B cap check already follows.
	if companion.Kind == "void-soul" {
		http.Error(w, "void soul saving throw proficiencies always match yours", http.StatusBadRequest)
		return
	}
	ability := strings.TrimSpace(r.FormValue("ability"))
	if !companionSaveAbilitySet[ability] {
		http.Error(w, "bad ability", http.StatusBadRequest)
		return
	}
	turningOn := r.FormValue("on") == "1"

	profs := parseSaveProficiencies(companion.SaveProficiencies)
	if turningOn {
		if !profs[ability] && companion.Kind == "snb" {
			sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("compute sheet for saving throw cap check:", err)
				return
			}
			saveCap := companionSaveCap(companion.Kind, sheet.Level)
			used := 0
			for _, on := range profs {
				if on {
					used++
				}
			}
			if used >= saveCap {
				http.Error(w, "saving throw proficiency cap reached", http.StatusBadRequest)
				return
			}
		}
		profs[ability] = true
	} else {
		delete(profs, ability)
	}

	if err := charstore.SetCompanionSaveProficiencies(s.charDB, id, cid, joinSaveProficiencies(profs)); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set companion saving throw proficiency:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_summon_tab")
}
