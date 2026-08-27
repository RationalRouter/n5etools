package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file is Trickster Scout's Void Soul Awakening (3rd level,
// class/scout-nin/group/scouting-technique/trickster-scout/feature/
// void-soul-awakening, verbatim text re-confirmed 2026-08-26 against
// rules.db's own subclass_features row). A Void Soul is stored as an
// ordinary kind="void-soul" charstore.Companion, following Titan's own
// "generic companion row + kind-specific formula layer" pattern (titan.go)
// as its base, per this feature's own explicit shape — but with two pieces
// neither Titan nor any other companion kind has any precedent for:
//
//  1. A summon/dismiss (active/inactive) toggle
//     (Companion.VoidSoulIsSummoned, migration 0085) — "This chakra
//     construct can be summoned as a Bonus Action... you can dismiss your
//     Void Soul as a Bonus Action." Every other companion kind, once added,
//     simply exists indefinitely (see titan.go's own header doc on this
//     exact gap). The ONE mechanical effect actually gated on this toggle —
//     internal/charsheet.Compute's Charisma-for-Dexterity AC swap ("While
//     summoned, you can calculate your AC using your Charisma in place of
//     your Dexterity") — is wired there directly (voidSoulSummoned); every
//     other "while summoned" clause (visible to CM-users, occupies your
//     space, re-summons within 120ft) has no automation surface in this app
//     (no battle-map/positioning system) and stays reference text on the
//     companion's own card.
//  2. A companion-scoped known-jutsu cap+catalog pick (cap = the OWNER's
//     own Proficiency Bonus; catalog = whatever jutsu the owner could
//     normally learn — class/clan origin plus elemental affinity, the same
//     jutsuEligibilityContext every other jutsu picker in this app already
//     uses — plus jutsu naming one bonus keyword the owner picks from
//     among the 5 Nature Releases and Medical, which they don't otherwise
//     have access to). Stored as charstore.ScoutNinPickVoidSoulJutsu picks
//     (migration 0086), deliberately NOT written into character_jutsu:
//     that table's own source CHECK enum ('learned', 'clan',
//     'class-feature', 'feat', 'created') has no value meaning
//     "known-but-castable-only-by-my-companion", and the book is explicit
//     the Void Soul casts these, not the player ("These jutsu are added to
//     your known jutsu list, but you cannot cast them, only your void soul
//     can"). "Cannot cast Combination Jutsu" is enforced by excluding any
//     candidate whose keywords name "Combination".
//
// NO BASE STAT CARD: the feature's own text promises "the following
// Statistics found at the end of this class section," but rules.db has no
// such stat block anywhere for the Void Soul — no void_soul-anything table,
// no class_features/subclass_features row beyond this one feature's own
// description (confirmed by direct query of every table in the database,
// and independently corroborated by CLASS_AUDIT.md's own Group-3 entry).
// Unlike Titan (titanBaseAbilityScores/titanBaseTraits, hand-transcribed
// from titan_unit_card.raw_text — an unstructured blob that at least
// EXISTS), there is nothing here to hand-transcribe a baseline from at
// all — so AC/HP/Speed stay plain player-entered fields with no computed
// default, the same "no formula behind this kind" treatment 'summon'/
// 'custom' already get, rather than an invented number with no textual
// basis. If the source PDF's own stat-block page is ever tracked down and
// transcribed, this is the file to extend, mirroring titan.go's own shape.
//
// ABILITY MODIFIERS, NOT SCORES: "Your Void Soul does not have Ability
// scores like a normal creature, but instead only Ability Modifiers. You
// gain a number of points to spend, equal to your Charisma Modifier times
// 3, which raises any ability modifier of your Void Soul by +1... The
// maximum value of any modifier is +6." Stored in the SAME str_score/
// dex_score/.../cha_score columns every other companion kind uses for a
// real ability SCORE, but encoded as score = 10 + 2*modifier — a deliberate
// reuse rather than a fresh set of columns, so companionScoreModifier/
// companionSaves (Saving Throws) keep working completely unmodified: a
// score of exactly 10 (or NULL — companionScoreModifier's own doc) already
// reads as modifier +0, which is also this construct's own un-spent floor,
// so a freshly created companion needs no prefill at all. This encoding is
// never exposed to the player directly — companion_fields.html renders a
// dedicated "Ability Modifiers" block for kind="void-soul" (plain "+N"
// values with +/- buttons spending from the point pool below) instead of
// the generic ability-SCORE row every other kind shows, since surfacing
// "14" where the book only ever prints "+2" would be a confusing semantic
// mismatch with what that row means for every other kind.
//
// Editing (the ability-point buttons, the summon/dismiss toggle, the jutsu
// picker) is Companions-tab-only, matching every other one-time-lock/
// cap+catalog pick already on this companion card (Titan Specialization,
// Legion ability bonus, Breed) — the standalone companion popup
// (companion_sheet.html) shows all three read-only, the same "quick
// reference for combat, editing happens on the tab" boundary
// handleCompanionSheet's own doc already draws for Titan's Known Upgrades.

// voidSoulAwakeningFeatureSlug is Trickster Scout's 3rd-level Void Soul
// Awakening feature. Mirrors internal/charsheet's own unexported
// voidSoulAwakeningFeatureSlug const — that package can't be imported back
// from here, so the slug string itself is duplicated rather than exported
// solely for this, the same "small string, not worth a cross-package API"
// call cmd/n5e/science_nin.go's own scienceNinExoskeletonFeatureSlug
// already makes for an identical shape.
const voidSoulAwakeningFeatureSlug = "class/scout-nin/group/scouting-technique/trickster-scout/feature/void-soul-awakening"

// voidSoulAbilities is the six ability codes in canonical display order —
// charsheet.Abilities directly, named here for readability at each call
// site in this file, mirroring companionSaveAbilities' identical shape in
// companion_saves.go.
var voidSoulAbilities = charsheet.Abilities

// voidSoulMaxModifier is the feature's own stated ceiling: "The maximum
// value of any modifier is +6."
const voidSoulMaxModifier = 6

// voidSoulScoreForModifier encodes a Void Soul ability modifier as the
// fake "score" actually stored in the companion's own str_score/.../
// cha_score column — see this file's own header doc for why.
func voidSoulScoreForModifier(mod int) int64 {
	return int64(10 + 2*mod)
}

// voidSoulBonusKeywordOptions is the "one keyword that you don't have
// access to (Any one nature release or medical keyword)" clause's own
// closed option set — the 5 Nature Releases (elemental_affinity.go's own
// elementNames) plus Medical.
func voidSoulBonusKeywordOptions() []string {
	out := make([]string, 0, len(elementNames)+1)
	out = append(out, elementNames...)
	return append(out, "Medical")
}

// voidSoulAbilityView is one ability's own resolved modifier, for the
// companion card's dedicated "Ability Modifiers" block.
type voidSoulAbilityView struct {
	Ability  string // "str", "dex", ...
	Modifier int
}

// voidSoulJutsuEntry is one jutsu candidate or known pick, for the
// companion card's own Known Jutsu list/picker. Classification/CostText are
// only needed once a pick is KNOWN (voidSoulJutsuAttackRows' own to-hit/
// Save-DC/cost annotation below) — carried on every entry regardless, since
// loadVoidSoulJutsuCatalog's one query already reads every column for both
// known and eligible-to-add candidates alike.
type voidSoulJutsuEntry struct {
	Slug           string
	Name           string
	Rank           string
	Classification string
	CostText       string
	Description    string
}

// voidSoulReference is everything a "void-soul" companion's own card needs
// beyond the generic Companion fields — computed fresh on every render,
// the same "cheap, no Sync button needed" treatment nindog_reference/
// titan_reference already get for their own read-only sections. nil
// whenever the character doesn't currently have Void Soul Awakening
// granted (loadVoidSoulReference's own gate) — a kind="void-soul" companion
// can still exist without it (nothing stops adding one by hand), mirroring
// TitanReference's identical nil-when-unqualified contract.
type voidSoulReference struct {
	Abilities       []voidSoulAbilityView
	PointsTotal     int
	PointsSpent     int
	PointsAvailable int

	BonusKeyword        string
	BonusKeywordOptions []string

	JutsuCap     int
	KnownJutsu   []voidSoulJutsuEntry
	JutsuOptions []voidSoulJutsuEntry
}

// voidSoulReferenceOrZero mirrors titanReferenceOrZero/ninDogReferenceOrZero
// — a nil-safe zero value for template arithmetic that would otherwise
// panic on a nil pointer. Void Soul contributes nothing to the Companions
// tab's own Expected*/AC-HP-Speed-ability-score sums (unlike Titan/Nin-Dog/
// S.N.B, it has no computed stat-block formula at all — see this file's own
// header doc), so nothing currently calls this, but it's kept for the same
// future-proofing reason and symmetry those two helpers already establish.
func voidSoulReferenceOrZero(ref *voidSoulReference) voidSoulReference {
	if ref == nil {
		return voidSoulReference{}
	}
	return *ref
}

// loadVoidSoulJutsuCatalog splits every jutsu the rules database knows
// about into "already known" (knownSlugs, shown regardless of whether it's
// still eligible today — a bonus-keyword change shouldn't silently hide an
// already-learned pick) and "available to add" (currently eligible via the
// owner's own class/clan origin and elemental affinities, widened by
// bonusKeyword, minus anything already known, minus Combination jutsu the
// Void Soul can never cast). Mirrors loadGreenTechniqueJutsuOptions' own
// "compute the eligibility context once, scan the whole v_jutsu view"
// shape (puppets.go) — Void Soul's own catalog has no rank ceiling or
// classification filter the way Master of the Green Technique's does, so
// the scan is otherwise unfiltered.
func (s *server) loadVoidSoulJutsuCatalog(characterID int64, bonusKeyword string, knownSlugs []string) (known, options []voidSoulJutsuEntry, err error) {
	ctx, err := s.loadJutsuEligibilityContext(characterID)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case bonusKeyword == "":
		// no bonus keyword chosen yet — ctx stays exactly the owner's own
		// normal access.
	case bonusKeyword == "Medical":
		// "Any one nature release OR MEDICAL keyword" — granted access is
		// unrestricted by rank, the same treatment characterMedicalRankCap
		// already gives Medical-Nin itself, since the book states no rank
		// ceiling of its own for this bonus access.
		if ctx.medicalRankCap == "" {
			ctx.medicalRankCap = "S"
		}
	default:
		affinities := make(map[string]bool, len(ctx.affinities)+1)
		for k, v := range ctx.affinities {
			affinities[k] = v
		}
		affinities[bonusKeyword] = true
		ctx.affinities = affinities
		ctx.hasAnyAffinity = true
	}

	knownSet := make(map[string]bool, len(knownSlugs))
	for _, slug := range knownSlugs {
		knownSet[slug] = true
	}

	rows, err := s.rulesDB.Query(`SELECT slug, name, rank, classification, cost_text, keywords, description FROM v_jutsu ORDER BY name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug, name, classification, costText, keywords, description string
		var rank sql.NullString
		if err := rows.Scan(&slug, &name, &rank, &classification, &costText, &keywords, &description); err != nil {
			return nil, nil, err
		}
		entry := voidSoulJutsuEntry{
			Slug: slug, Name: name, Rank: rank.String,
			Classification: classification, CostText: costText, Description: description,
		}
		switch {
		case knownSet[slug]:
			known = append(known, entry)
		case strings.Contains(keywords, "Combination"):
			// "Your Void Soul cannot cast Combination Jutsu."
		case ctx.eligible(slug, keywords, rank.String):
			options = append(options, entry)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	sort.Slice(known, func(i, j int) bool { return known[i].Name < known[j].Name })
	return known, options, nil
}

// voidSoulJutsuAttackRows turns each of a Void Soul's own known jutsu into a
// row on the companion's own Attacks list — the same list Bash/Bite/every
// other companion's baseline attack already renders in
// (companion_fields.html's shared "Attacks" section) — so a jutsu pick
// actually shows up somewhere rollable instead of only inside the Known
// Jutsu picker's own description tooltip.
//
// AttackKind/AttackTotal/Save DC reuse jutsuAttackKind/resolveJutsuAttackKind
// and sheet.JutsuAttacks exactly the way the main Jutsu tab's own
// loadCharacterJutsuSheet resolves the PLAYER's own known jutsu — the same
// "uses your [X]" pattern this file's own header doc already establishes for
// Chakra/Speed/saving throws extends naturally to a jutsu's own to-hit and
// Save DC too, since the book never states an independent formula of the
// Void Soul's own for either (only ability-score-based ELIGIBILITY is
// explicitly "its own"). A jutsu with no attack roll (most Genjutsu/
// Fuinjutsu — a saving throw, not a to-hit) renders with NoAttackRoll set,
// exactly like the main Jutsu tab's own AttackKind=="" rows render no roll
// button there either; its Save DC (when its classification matches one of
// charsheet.AttackKinds — Fuinjutsu/Hijutsu have no defined formula at all,
// so get no DC line rather than an invented one) is folded into the row's
// own description text since companionAttackRow has no separate numeric
// slot for one the way the Core tab's own square tile does.
func voidSoulJutsuAttackRows(known []voidSoulJutsuEntry, sheet *charsheet.Sheet) []companionAttackRow {
	byKind := make(map[string]charsheet.JutsuAttack, len(sheet.JutsuAttacks))
	for _, a := range sheet.JutsuAttacks {
		byKind[a.Kind] = a
	}
	out := make([]companionAttackRow, 0, len(known))
	for _, j := range known {
		row := companionAttackRow{CompanionAttack: charstore.CompanionAttack{Name: j.Name}}

		attackKind := jutsuAttackKind(j.Description)
		if attackKind != "" {
			attackKind = resolveJutsuAttackKind(attackKind, j.Classification)
		}
		meta := j.Rank + "-Rank Void Soul Jutsu"
		if j.CostText != "" {
			meta += " · Cost: " + j.CostText
		}
		if a, ok := byKind[attackKind]; attackKind != "" && ok {
			row.AttackTotal = a.Modifier
		} else {
			row.NoAttackRoll = true
			// Only annotate a Save DC for a classification this app actually
			// has a formula for (charsheet.AttackKinds) — Fuinjutsu/Hijutsu
			// jutsu get no DC line at all rather than a fabricated one.
			// classification can be compound ("Hijutsu, Bukijutsu" —
			// confirmed against rules.db), so this matches by substring the
			// same way resolveJutsuAttackKind's own Bukijutsu check does,
			// not by exact equality.
			lowerClassification := strings.ToLower(j.Classification)
			for _, k := range charsheet.AttackKinds {
				if strings.Contains(lowerClassification, strings.ToLower(k.Kind)) {
					if a, ok := byKind[k.Kind]; ok {
						meta += fmt.Sprintf(" · Save DC %d", a.SaveDC)
					}
					break
				}
			}
		}
		row.Description = meta + ". " + j.Description
		out = append(out, row)
	}
	return out
}

// voidSoulBonusKeyword reads the character's own stored "one keyword you
// don't have access to" pick (character_feature_choices, keyed by this
// feature's own slug, choice_index 0 — see internal/charstore's own doc on
// what that table is for) — "" if never chosen yet.
func (s *server) voidSoulBonusKeyword(characterID int64) (string, error) {
	var keyword string
	err := s.charDB.QueryRow(
		`SELECT value FROM character_feature_choices WHERE character_id = ? AND feature_slug = ? AND choice_index = 0`,
		characterID, voidSoulAwakeningFeatureSlug,
	).Scan(&keyword)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return keyword, nil
}

// loadVoidSoulReference builds one companion's own voidSoulReference — nil
// if the character doesn't currently have Void Soul Awakening granted
// (mirroring loadTitanReference's own nil-when-unqualified contract for a
// kind="titan" companion on a character without Mech Crafter).
func (s *server) loadVoidSoulReference(characterID int64, sheet *charsheet.Sheet, companion charstore.Companion) (*voidSoulReference, error) {
	granted, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	if !hasFeature(granted, voidSoulAwakeningFeatureSlug) {
		return nil, nil
	}

	// "Movement speed is equal to yours." Unlike AC/HP/ability scores (no
	// base stat card exists to derive those from at all — this file's own
	// header doc), Speed has an explicit, always-computable formula: the
	// player's own sheet.Speed. Resolve any manual pin first (the same
	// override-before-live-write order loadTitanReference/loadSNBReference
	// already follow) and write whichever value wins on every render — the
	// companions.go call sites re-read the companion row immediately after
	// this call returns, mirroring their existing nin-dog/titan/snb
	// "auto-then-pin, re-read after write" pattern.
	overrides, err := charstore.GetCompanionOverrides(s.charDB, companion.ID)
	if err != nil {
		return nil, err
	}
	speed := int64(sheet.Speed)
	if v, ok := companionOverrideInt(overrides, "speed"); ok {
		speed = v
	}
	if err := charstore.SetVoidSoulSpeedLive(s.charDB, characterID, companion.ID, speed); err != nil {
		return nil, err
	}

	ref := &voidSoulReference{}
	scores := map[string]sql.NullInt64{
		"str": companion.Str, "dex": companion.Dex, "con": companion.Con,
		"int": companion.Int, "wis": companion.Wis, "cha": companion.Cha,
	}
	spent := 0
	for _, a := range voidSoulAbilities {
		mod := companionScoreModifier(scores[a])
		ref.Abilities = append(ref.Abilities, voidSoulAbilityView{Ability: a, Modifier: mod})
		if mod > 0 {
			spent += mod
		}
	}
	pointsTotal := sheet.Abilities["cha"].Modifier * 3
	if pointsTotal < 0 {
		pointsTotal = 0
	}
	ref.PointsTotal = pointsTotal
	ref.PointsSpent = spent
	if ref.PointsAvailable = pointsTotal - spent; ref.PointsAvailable < 0 {
		ref.PointsAvailable = 0
	}

	bonusKeyword, err := s.voidSoulBonusKeyword(characterID)
	if err != nil {
		return nil, err
	}
	ref.BonusKeyword = bonusKeyword
	ref.BonusKeywordOptions = voidSoulBonusKeywordOptions()

	ref.JutsuCap = sheet.ProficiencyBonus
	knownSlugs, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickVoidSoulJutsu)
	if err != nil {
		return nil, err
	}
	known, options, err := s.loadVoidSoulJutsuCatalog(characterID, bonusKeyword, knownSlugs)
	if err != nil {
		return nil, err
	}
	ref.KnownJutsu = known
	ref.JutsuOptions = options

	return ref, nil
}

// handleVoidSoulSummonToggle sets or clears a companion's own summon/
// dismiss toggle (form field "on", "1" or "0") — see
// charstore.SetVoidSoulSummoned's own doc for why this is a dedicated
// single-purpose endpoint rather than folded into the whole-form autosave.
func (s *server) handleVoidSoulSummonToggle(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	on := r.FormValue("on") == "1"
	if err := charstore.SetVoidSoulSummoned(s.charDB, id, cid, on); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set void soul summoned:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// handleVoidSoulBonusKeyword sets or clears the character's own "one
// keyword you don't have access to" pick (form field "keyword", one of
// voidSoulBonusKeywordOptions, or blank to clear).
func (s *server) handleVoidSoulBonusKeyword(w http.ResponseWriter, r *http.Request) {
	id, _, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	if keyword != "" && !slicesContainsString(voidSoulBonusKeywordOptions(), keyword) {
		http.Error(w, "bad keyword", http.StatusBadRequest)
		return
	}
	var err error
	if keyword == "" {
		err = charstore.ClearFeatureChoice(s.charDB, id, voidSoulAwakeningFeatureSlug, 0)
	} else {
		err = charstore.SetFeatureChoice(s.charDB, id, voidSoulAwakeningFeatureSlug, 0, keyword)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set void soul bonus keyword:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// slicesContainsString is a tiny local helper — this file's only use of
// "does this small slice contain this string" doesn't justify importing
// the generic slices package's own Contains just for one call site here
// (elemental_affinity.go/puppets.go both already import "slices" for
// heavier use; this file doesn't otherwise need it).
func slicesContainsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// handleVoidSoulAbilityPoint spends (or refunds) one point from the "Your
// Charisma Modifier times 3" pool on a single ability modifier (form fields
// "ability" one of voidSoulAbilities, "delta" "+1" or "-1") — the point-buy
// allocator this feature's own text describes as "of your choice," so
// unlike Titan/Nin-Dog/S.N.B's pure formula fields, this genuinely needs a
// player-facing picker rather than a computed default (see this file's own
// header doc). Re-derives spent/available from the companion's own stored
// modifiers on every call rather than trusting a client-sent total, the
// same "server re-validates, never trusts the form" boundary every other
// cap-gated pick in this codebase already applies.
func (s *server) handleVoidSoulAbilityPoint(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ability := r.FormValue("ability")
	if !companionSaveAbilitySet[ability] {
		http.Error(w, "bad ability", http.StatusBadRequest)
		return
	}
	delta, err := strconv.Atoi(r.FormValue("delta"))
	if err != nil || (delta != 1 && delta != -1) {
		http.Error(w, "bad delta", http.StatusBadRequest)
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for void soul ability point:", err)
		return
	}
	if companion.Kind != "void-soul" {
		http.Error(w, "not a void soul companion", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for void soul ability point:", err)
		return
	}
	scores := map[string]sql.NullInt64{
		"str": companion.Str, "dex": companion.Dex, "con": companion.Con,
		"int": companion.Int, "wis": companion.Wis, "cha": companion.Cha,
	}
	spent := 0
	for _, a := range voidSoulAbilities {
		if m := companionScoreModifier(scores[a]); m > 0 {
			spent += m
		}
	}
	pointsTotal := sheet.Abilities["cha"].Modifier * 3
	if pointsTotal < 0 {
		pointsTotal = 0
	}
	currentMod := companionScoreModifier(scores[ability])
	newMod := currentMod + delta
	if newMod < 0 || newMod > voidSoulMaxModifier {
		http.Error(w, "modifier out of range", http.StatusBadRequest)
		return
	}
	if delta > 0 && spent+1 > pointsTotal {
		http.Error(w, "no points remaining", http.StatusBadRequest)
		return
	}
	newScore := sql.NullInt64{Int64: voidSoulScoreForModifier(newMod), Valid: true}
	if err := charstore.SetCompanionIntField(s.charDB, id, cid, ability+"_score", newScore); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set void soul ability point:", err)
		return
	}
	s.respondSheet(w, r, id, companionRespondFragment(r))
}

// voidSoulJutsuAddCore validates and stores one Void Soul jutsu pick — the
// shared core handleVoidSoulJutsuAdd's route calls, mirroring
// scoutNinPickAddCore's identical validate-then-store shape (scout_nin.go),
// just against this file's own cap (owner's Proficiency Bonus) and catalog
// (loadVoidSoulJutsuCatalog) instead of scoutNinTabData's.
func (s *server) voidSoulJutsuAddCore(characterID int64, rawSlug string) (int, string) {
	slug := strings.TrimSpace(rawSlug)
	if slug == "" {
		return http.StatusBadRequest, "missing pick"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		log.Println("compute sheet for void soul jutsu add:", err)
		return http.StatusInternalServerError, "database error"
	}
	granted, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		log.Println("load granted features for void soul jutsu add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if !hasFeature(granted, voidSoulAwakeningFeatureSlug) {
		return http.StatusBadRequest, "character does not have Void Soul Awakening"
	}
	bonusKeyword, err := s.voidSoulBonusKeyword(characterID)
	if err != nil {
		log.Println("load void soul bonus keyword for jutsu add:", err)
		return http.StatusInternalServerError, "database error"
	}
	knownSlugs, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickVoidSoulJutsu)
	if err != nil {
		log.Println("list void soul jutsu picks for add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if len(knownSlugs) >= sheet.ProficiencyBonus {
		return http.StatusBadRequest, "known jutsu cap reached"
	}
	_, options, err := s.loadVoidSoulJutsuCatalog(characterID, bonusKeyword, knownSlugs)
	if err != nil {
		log.Println("load void soul jutsu catalog for add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if !slicesContainsVoidSoulSlug(options, slug) {
		return http.StatusBadRequest, "not a valid pick"
	}
	if err := charstore.AddScoutNinPick(s.charDB, characterID, charstore.ScoutNinPickVoidSoulJutsu, slug); err != nil {
		log.Println("add void soul jutsu pick:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

func slicesContainsVoidSoulSlug(options []voidSoulJutsuEntry, slug string) bool {
	for _, o := range options {
		if o.Slug == slug {
			return true
		}
	}
	return false
}

// handleVoidSoulJutsuAdd is voidSoulJutsuAddCore's Core-sheet AJAX route
// (form field "jutsu_slug") — character-scoped, not companion-scoped (the
// pick lives in character_scout_nin_picks, keyed by character_id alone),
// unlike every other handler in this file.
func (s *server) handleVoidSoulJutsuAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if status, msg := s.voidSoulJutsuAddCore(id, r.FormValue("jutsu_slug")); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_summon_tab")
}

// handleVoidSoulJutsuDelete drops one Void Soul jutsu pick, freely, at any
// time — same "trust the player" boundary every other cap+catalog pick in
// this codebase already draws (RemoveScoutNinPick's own doc).
func (s *server) handleVoidSoulJutsuDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("jutsu_slug"))
	if slug == "" {
		http.Error(w, "missing pick", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveScoutNinPick(s.charDB, id, charstore.ScoutNinPickVoidSoulJutsu, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove void soul jutsu pick:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_summon_tab")
}
