// Security note: this server binds to 127.0.0.1 only, which already rules
// out any attacker who isn't running code on this same machine. The
// remaining, real threat is a malicious page open in another browser tab
// firing requests at our loopback port — browsers allow that. Two layers
// defend against it, applied to every request:
//
//  1. Origin check: a cross-site request carries an Origin header that
//     won't match ours; reject it. A normal top-level navigation (typing
//     the URL, clicking our own links) doesn't send Origin at all, so
//     those are let through to layer 2.
//  2. Launch token: the browser is opened with a random per-run secret in
//     the URL. The first request that presents it gets a session cookie;
//     every request after that (including the ones with no Origin header,
//     which layer 1 alone can't stop) must carry either the cookie or the
//     token. A malicious page has no way to know the token.
//
// Never add permissive/wildcard CORS headers — there is no legitimate
// cross-origin caller of this server.
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/sergio/n5e/internal/charstore"
)

const tokenCookieName = "n5e_token"

// shutdownGrace bounds how long a graceful shutdown waits for in-flight
// requests (there's only ever a page render or two) before main.go gives up
// and returns anyway.
const shutdownGrace = 5 * time.Second

// heartbeatTimeout is how long the server waits after the last /heartbeat
// ping before deciding the tab was closed and shutting itself down — see
// watchHeartbeat's doc for why this can't just be a page unload/pagehide
// listener in a multi-page app like this one.
//
// This is now only the FALLBACK budget, used while no /alive stream has
// ever connected (see aliveGrace). It stays generous because timer-based
// pings are the unreliable signal:
//
// Real bug (2026-07, first user bug report via the in-app form): this was
// 5 seconds against a 2-second ping interval — only 2.5x headroom. Browsers
// throttle timers in backgrounded tabs (Chrome clamps a hidden tab's
// setInterval to roughly once a minute once "intensive throttling" kicks
// in a few minutes after the tab is hidden; other browsers do something
// similar), so simply switching to another tab for a while was enough to
// blow through a 5-second budget and have the server kill itself out from
// under a tab the user never closed — reported as "N5e tools stop being
// able to load and had to be reopened" after using another tab. Set well
// above realistic worst-case background throttling (see also heartbeat.js's
// visibilitychange listener, which fires an immediate ping the moment the
// tab becomes visible again rather than waiting on a possibly-stale
// throttled interval).
//
// Real bug #2 (2026-07-25): 2 minutes still isn't enough, and no finite
// budget can be. Observed case: the tab was left open while the machine
// slept, and the server killed itself. A suspended or hibernated machine runs
// no timers at all, and a tab that Chrome has frozen outright (Memory
// Saver/tab freezing, which can kick in after a few minutes hidden) runs no
// timers either — in both cases the pings stop dead while the tab is still
// very much open. Raising the number would only trade one wrong answer for
// a slower one, which is why the primary signal is now a held-open
// connection rather than a timer; see watchHeartbeat and handleAlive.
const heartbeatTimeout = 120 * time.Second

// aliveGrace is how long the server waits after the LAST /alive stream
// disconnects before shutting down. It can be short — this is a real
// connection closing, not a timer failing to fire — and only needs to cover
// the gap while an ordinary internal navigation tears down one page's
// stream and the next page opens its own.
const aliveGrace = 8 * time.Second

// aliveKeepalive is how often handleAlive writes an SSE comment down an
// otherwise idle stream. Nothing on the client reads it; it exists so an
// intermediary or an OS that quietly drops idle sockets can't leave the
// server holding a connection the browser no longer has.
const aliveKeepalive = 30 * time.Second

type server struct {
	rulesDB        *sql.DB
	charDB         *sql.DB
	charDBPath     string // absolute path to characters.db, for backups.go
	backupDir      string
	token          string
	expectedOrigin string // e.g. "http://127.0.0.1:54321", filled in once the port is known
	shutdown       chan struct{}
	shutdownOnce   sync.Once

	heartbeatMu   sync.Mutex
	lastHeartbeat time.Time // zero value = no ping received yet (browser hasn't loaded/opened)
	aliveOpen     int       // /alive streams currently held open
	lastAlive     time.Time // when the count last dropped to zero
	aliveSeen     bool      // an /alive stream has connected at least once, so the browser supports it
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating launch token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /characters", s.handleCharacters)
	mux.HandleFunc("GET /characters/new", s.handleNewCharacter)
	mux.HandleFunc("POST /characters", s.handleCreateCharacter)
	mux.HandleFunc("POST /characters/{id}/delete", s.handleDeleteCharacter)
	mux.HandleFunc("POST /characters/{id}/reset-creation", s.handleResetCharacterCreation)
	mux.HandleFunc("GET /characters/{id}/create", s.handleCreationHub)
	mux.HandleFunc("POST /characters/{id}/create/finish", s.handleCreateFinish)
	mux.HandleFunc("GET /characters/{id}/create/clan", s.handleCreateClan)
	mux.HandleFunc("POST /characters/{id}/create/clan", s.handleCreateClan)
	mux.HandleFunc("GET /characters/{id}/create/class", s.handleCreateClass)
	mux.HandleFunc("POST /characters/{id}/create/class", s.handleCreateClass)
	mux.HandleFunc("POST /characters/{id}/create/class/remove", s.handleCreateClassRemove)
	mux.HandleFunc("POST /characters/{id}/create/class/subclass", s.handleCreateClassSubclass)
	mux.HandleFunc("POST /characters/{id}/create/class/level", s.handleCreateClassLevel)
	mux.HandleFunc("GET /characters/{id}/create/abilities", s.handleCreateAbilities)
	mux.HandleFunc("POST /characters/{id}/create/abilities", s.handleCreateAbilities)
	mux.HandleFunc("GET /characters/{id}/create/background", s.handleCreateBackground)
	mux.HandleFunc("POST /characters/{id}/create/background", s.handleCreateBackground)
	mux.HandleFunc("GET /characters/{id}/create/equipment", s.handleCreateEquipment)
	mux.HandleFunc("POST /characters/{id}/create/equipment", s.handleCreateEquipment)
	mux.HandleFunc("GET /characters/{id}/create/jutsu", s.handleCreateJutsu)
	mux.HandleFunc("POST /characters/{id}/create/jutsu", s.handleCreateJutsu)
	mux.HandleFunc("GET /characters/{id}/create/ambitions", s.handleCreateAmbitions)
	mux.HandleFunc("POST /characters/{id}/create/ambitions", s.handleCreateAmbitions)
	mux.HandleFunc("GET /characters/{id}", s.handleCharacterSheet)
	mux.HandleFunc("POST /characters/{id}/sheet/hp", s.handleSheetHP)
	mux.HandleFunc("POST /characters/{id}/sheet/chakra", s.handleSheetChakra)
	mux.HandleFunc("POST /characters/{id}/sheet/base-temp-hp", s.handleSheetBaseTempHP)
	mux.HandleFunc("POST /characters/{id}/sheet/ryo", s.handleSheetRyo)
	mux.HandleFunc("POST /characters/{id}/sheet/speed", s.handleSheetSpeed)
	mux.HandleFunc("POST /characters/{id}/sheet/name", s.handleSheetName)
	mux.HandleFunc("POST /characters/{id}/sheet/rest", s.handleSheetRest)
	mux.HandleFunc("POST /characters/{id}/sheet/resource/{key}", s.handleSheetCustomResource)
	mux.HandleFunc("POST /characters/{id}/sheet/ccd-mending-pct", s.handleSetCCDMendingPct)
	mux.HandleFunc("POST /characters/{id}/sheet/ability", s.handleSheetAbility)
	mux.HandleFunc("POST /characters/{id}/sheet/inventory", s.handleSheetInventoryAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/inventory/{rowID}/update", s.handleSheetInventoryUpdate)
	mux.HandleFunc("POST /characters/{id}/sheet/inventory/{rowID}/unpack", s.handleSheetInventoryUnpack)
	mux.HandleFunc("POST /characters/{id}/sheet/inventory/{rowID}/delete", s.handleSheetInventoryDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/inventory/custom", s.handleSheetInventoryAddCustom)
	mux.HandleFunc("POST /characters/{id}/sheet/inventory/pack-toolkit-choice", s.handleSheetPackToolkitChoice)
	mux.HandleFunc("POST /characters/{id}/sheet/portrait", s.handleSheetPortrait)
	mux.HandleFunc("POST /characters/{id}/sheet/portrait/delete", s.handleSheetPortraitDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/inspiration", s.handleSheetInspiration)
	mux.HandleFunc("POST /characters/{id}/sheet/kujaku-mode", s.handleSheetKujakuMode)
	mux.HandleFunc("POST /characters/{id}/sheet/ambitions", s.handleSheetAmbitions)
	mux.HandleFunc("POST /characters/{id}/sheet/bio", s.handleSheetBio)
	mux.HandleFunc("POST /characters/{id}/sheet/notes", s.handleSheetNotes)
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-primary-target", s.handleSheetHunterPrimaryTarget)
	mux.HandleFunc("POST /characters/{id}/sheet/level", s.handleSheetLevel)
	mux.HandleFunc("POST /characters/{id}/sheet/subclass", s.handleSheetSubclass)
	mux.HandleFunc("GET /characters/{id}/sheet/class", s.handleSheetClass)
	mux.HandleFunc("POST /characters/{id}/sheet/class", s.handleSheetClass)
	mux.HandleFunc("POST /characters/{id}/sheet/class/remove", s.handleSheetClassRemove)
	mux.HandleFunc("POST /characters/{id}/sheet/class/subclass", s.handleSheetClassSubclass)
	mux.HandleFunc("POST /characters/{id}/sheet/class/level", s.handleSheetClassLevel)
	mux.HandleFunc("POST /characters/{id}/sheet/maxima", s.handleSheetMaxima)
	mux.HandleFunc("POST /characters/{id}/sheet/attack-ability", s.handleSheetAttackAbility)
	mux.HandleFunc("POST /characters/{id}/sheet/clash-ability", s.handleSheetClashAbility)
	mux.HandleFunc("POST /characters/{id}/sheet/initiative", s.handleSheetInitiative)
	mux.HandleFunc("POST /characters/{id}/sheet/ac", s.handleSheetAC)
	mux.HandleFunc("GET /characters/{id}/sheet/fragment/{name}", s.handleSheetFragment)
	mux.HandleFunc("POST /characters/{id}/sheet/ui-state", s.handleSheetUIStateSave)
	mux.HandleFunc("POST /characters/{id}/sheet/ui-state/reset", s.handleSheetUIStateReset)
	mux.HandleFunc("POST /characters/{id}/sheet/attacks", s.handleSheetCustomAttack)
	mux.HandleFunc("POST /characters/{id}/sheet/attacks/{rowID}/update", s.handleSheetCustomAttackUpdate)
	mux.HandleFunc("POST /characters/{id}/sheet/attacks/{rowID}/delete", s.handleSheetCustomAttackDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/weapon-attack/{rowID}", s.handleSheetWeaponAttackOptions)
	// The jutsu slug goes in the form body, not the path: slugs contain
	// slashes ("jutsu/hyuga/gentle-fist") and a {slug...} wildcard is only
	// legal as the last segment of a pattern, which rules out a trailing
	// /delete.
	mux.HandleFunc("POST /characters/{id}/sheet/jutsu", s.handleSheetJutsuAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/jutsu/delete", s.handleSheetJutsuDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/jutsu/options", s.handleSheetJutsuOptions)
	mux.HandleFunc("POST /characters/{id}/sheet/jutsu/cast", s.handleSheetJutsuCast)
	mux.HandleFunc("POST /characters/{id}/sheet/concentration/break", s.handleSheetConcentrationBreak)
	mux.HandleFunc("POST /characters/{id}/sheet/feats", s.handleSheetFeatAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/feats/delete", s.handleSheetFeatDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/feats/ability-choice", s.handleSheetFeatAbilityChoice)
	mux.HandleFunc("POST /characters/{id}/sheet/feats/skill-or-tool-choice", s.handleSheetFeatSkillOrToolChoice)
	mux.HandleFunc("GET /characters/{id}/custom-features", s.handleCharacterCustomFeatures)
	mux.HandleFunc("POST /characters/{id}/custom-features/add", s.handleCustomFeatureAdd)
	mux.HandleFunc("POST /characters/{id}/custom-features/{fid}/delete", s.handleCustomFeatureDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/mastery", s.handleSheetMasteryAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/mastery/{name}/delete", s.handleSheetMasteryDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/feature-choice", s.handleSheetFeatureChoice)
	mux.HandleFunc("POST /characters/{id}/sheet/asi", s.handleSheetASI)
	mux.HandleFunc("POST /characters/{id}/sheet/companions", s.handleSheetCompanionAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/companions/{cid}/delete", s.handleSheetCompanionDelete)
	// Void Soul Awakening's own companion-scoped known-jutsu pick
	// (cmd/n5e/void_soul.go) — character-scoped, not companion-scoped (the
	// pick lives in character_scout_nin_picks, keyed by character_id alone),
	// so grouped with the other /sheet/... routes rather than under
	// /companions/{cid}/....
	mux.HandleFunc("POST /characters/{id}/sheet/void-soul-jutsu", s.handleVoidSoulJutsuAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/void-soul-jutsu/delete", s.handleVoidSoulJutsuDelete)
	mux.HandleFunc("GET /characters/{id}/reference", s.handleCharacterReference)
	mux.HandleFunc("GET /characters/{id}/clan-reference", s.handleCharacterClanReference)
	// S.N.B Upgrades / Titan Slots: the first two "subclass tracker popup"
	// pages (see subclass_tracker_popup.go's own header doc for the
	// pattern) — same [data-reference-popup] mechanism as Class/Clan
	// Reference/Custom Features above, each with its own add/delete
	// routes grouped near this file's existing Science-Nin/Titan routes
	// below rather than here, since they reuse those routes' own
	// validation.
	mux.HandleFunc("GET /characters/{id}/snb-upgrades", s.handleSNBUpgradesPopup)
	mux.HandleFunc("GET /characters/{id}/titan-slots", s.handleTitanSlotsPopup)
	// Elemental Innovationist / Grenadier / Mad Scientist / Ninjaneer /
	// Shinobi-Ware: five more "subclass tracker popup" pages, same pattern
	// as S.N.B Upgrades/Titan Slots above — POST add/delete routes grouped
	// near this file's existing Science-Nin routes below, since they reuse
	// those routes' own validation.
	mux.HandleFunc("GET /characters/{id}/science-nin/elemental-innovationist", s.handleElementalInnovationistPopup)
	mux.HandleFunc("GET /characters/{id}/science-nin/grenadier", s.handleGrenadierPopup)
	mux.HandleFunc("GET /characters/{id}/science-nin/mad-scientist", s.handleMadScientistPopup)
	mux.HandleFunc("GET /characters/{id}/science-nin/ninjaneer", s.handleNinjaneerPopup)
	mux.HandleFunc("GET /characters/{id}/science-nin/shinobi-ware", s.handleShinobiWarePopup)
	// Spyware / Storm Rider / Technobi: the last three "subclass tracker
	// popup" pages, same pattern as the six above.
	mux.HandleFunc("GET /characters/{id}/science-nin/spyware", s.handleSpywarePopup)
	mux.HandleFunc("GET /characters/{id}/science-nin/storm-rider", s.handleStormRiderPopup)
	mux.HandleFunc("GET /characters/{id}/science-nin/technobi", s.handleTechnobiPopup)
	// Weapon Form / Stancer / Passionate Flame / Ruin: four more "subclass
	// tracker popup" pages (weapon_form_popup.go, taijutsu_stancer_popup.go,
	// taijutsu_passionate_flame_popup.go, taijutsu_ruin_popup.go), same
	// pattern as the nine Science-Nin popups above — POST add/delete/set
	// routes grouped near this file's existing Weapon Specialist/Taijutsu
	// Specialist Core-sheet routes below, since they reuse those routes' own
	// validation.
	mux.HandleFunc("GET /characters/{id}/weapon-specialist/weapon-form", s.handleWeaponFormPopup)
	mux.HandleFunc("GET /characters/{id}/taijutsu-specialist/stancer", s.handleStancerPopup)
	mux.HandleFunc("GET /characters/{id}/taijutsu-specialist/passionate-flame", s.handlePassionateFlamePopup)
	mux.HandleFunc("GET /characters/{id}/taijutsu-specialist/ruin", s.handleRuinPopup)
	// Hunter's Creed: one more "subclass tracker popup" page (hunter_creed_
	// popup.go), same "one popup, per-subclass gating inside" shape Weapon
	// Form's own 8 Forms above already establish — POST add/delete routes
	// grouped near this file's existing Hunter-Nin routes below, since they
	// reuse those routes' own validation.
	mux.HandleFunc("GET /characters/{id}/hunter-nin/creed", s.handleHunterCreedPopup)
	// Operative Traps / Awakened Scroll / Combat Medic: three more "subclass
	// tracker popup" pages — Intelligence Operative's Tactical Strategist,
	// Ninjutsu Specialist's Scribe Master, and Medical-Nin's Combat Medic —
	// POST add/delete/set routes grouped near this file's existing
	// Intelligence Operative/Ninjutsu Specialist/Medical-Nin Core-sheet
	// routes below, since they reuse those routes' own validation.
	mux.HandleFunc("GET /characters/{id}/intelligence-operative/operative-traps", s.handleOperativeTrapsPopup)
	mux.HandleFunc("GET /characters/{id}/ninjutsu-specialist/awakened-scroll", s.handleAwakenedScrollPopup)
	mux.HandleFunc("GET /characters/{id}/medical-nin/combat-medic", s.handleCombatMedicPopup)
	// Scouting Technique: one more "subclass tracker popup" page (scout_
	// nin_scouting_technique_popup.go), same "one popup, per-subclass
	// gating inside" shape Hunter's Creed above already establishes — POST
	// add/delete/set routes grouped near this file's existing Scout-Nin
	// routes below, since they reuse those routes' own validation.
	mux.HandleFunc("GET /characters/{id}/scout-nin/scouting-technique", s.handleScoutingTechniquePopup)
	// Twisted Casting / Psyche Breaker: two more "subclass tracker popup"
	// pages (genjutsu_twisted_casting_popup.go, genjutsu_psyche_breaker_
	// popup.go), one per Genjutsu Pledge — POST add/delete routes grouped
	// near this file's existing Genjutsu Specialist routes below, since
	// they reuse those routes' own validation.
	mux.HandleFunc("GET /characters/{id}/genjutsu-specialist/twisted-casting", s.handleTwistedCastingPopup)
	mux.HandleFunc("GET /characters/{id}/genjutsu-specialist/psyche-breaker", s.handlePsycheBreakerPopup)
	// Pipe / Expert Combatant / Fast and Furious / Blend Enhancements: four
	// more "subclass tracker popup" pages (cooking_nin_pipe_popup.go,
	// cooking_nin_expert_combatant_popup.go, cooking_nin_fast_and_furious_
	// popup.go, cooking_nin_blend_enhancements_popup.go), one per
	// Cooking-Nin subclass — POST routes grouped near this file's existing
	// Cooking-Nin routes below, since they reuse those routes' own
	// validation.
	mux.HandleFunc("GET /characters/{id}/cooking-nin/pipe", s.handlePipePopup)
	mux.HandleFunc("GET /characters/{id}/cooking-nin/expert-combatant", s.handleBattleCookExpertCombatantPopup)
	mux.HandleFunc("GET /characters/{id}/cooking-nin/fast-and-furious", s.handleFastAndFuriousPopup)
	mux.HandleFunc("GET /characters/{id}/cooking-nin/blend-enhancements", s.handleBlendEnhancementsPopup)
	mux.HandleFunc("GET /characters/{id}/companions/{cid}", s.handleCompanionSheet)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}", s.handleCompanionSave)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/hp", s.handleCompanionHP)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/temp_hp", s.handleCompanionIntField("temp_hp", false))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/ac", s.handleCompanionIntField("ac", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/hp_max", s.handleCompanionIntField("hp_max", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/speed", s.handleCompanionIntField("speed", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/fly_speed", s.handleCompanionIntField("fly_speed", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/str_score", s.handleCompanionIntField("str_score", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/dex_score", s.handleCompanionIntField("dex_score", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/con_score", s.handleCompanionIntField("con_score", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/int_score", s.handleCompanionIntField("int_score", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/wis_score", s.handleCompanionIntField("wis_score", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/cha_score", s.handleCompanionIntField("cha_score", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/size", s.handleCompanionSize)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/matryoshka_jutsu_slots", s.handleCompanionIntField("matryoshka_jutsu_slots", false))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/jutsu_slots_current", s.handleCompanionIntField("jutsu_slots_current", false))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/jutsu_slots_max", s.handleCompanionIntField("jutsu_slots_max", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/barrier_current", s.handleCompanionIntField("barrier_current", false))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/barrier_max", s.handleCompanionIntField("barrier_max", true))
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/nin-dog-feature", s.handleNinDogFeaturePick)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/nin-dog-hijutsu-trait", s.handleNinDogHijutsuTraitPick)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/snb-combat-programming", s.handleSNBCombatProgrammingPick)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/saving-throw", s.handleCompanionSavingThrowToggle)
	// Void Soul Awakening (cmd/n5e/void_soul.go): the summon/dismiss toggle,
	// the Charisma-Modifier-x3 ability-point-buy allocator, and the "one
	// keyword you don't have access to" pick.
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/void-soul-summon", s.handleVoidSoulSummonToggle)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/void-soul-ability-point", s.handleVoidSoulAbilityPoint)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/void-soul-bonus-keyword", s.handleVoidSoulBonusKeyword)
	// Titan (Science-Nin Mech Crafter's Ordnance Training) upgrade picks are
	// character-scoped, not companion-scoped (see titan.go's own doc on why
	// — "You can only have 1 Titan created at a time" means the Creation
	// Points budget and Titan Slots belong to the character, independent of
	// which companion row the live Titan happens to be), so these live
	// under /characters/{id}/sheet/... like every other Science-Nin
	// subclass pick, not under /companions/{cid}/....
	mux.HandleFunc("POST /characters/{id}/sheet/titan-upgrade", s.handleTitanUpgradeAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/titan-upgrade/delete", s.handleTitanPickDelete(charstore.ScienceNinPickTitanUpgrade))
	mux.HandleFunc("POST /characters/{id}/sheet/titan-exosuit-upgrade", s.handleTitanExoSuitUpgradeAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/titan-exosuit-upgrade/delete", s.handleTitanPickDelete(charstore.ScienceNinPickTitanExosuitUpgrade))
	mux.HandleFunc("POST /characters/{id}/sheet/titan-specialist-crafting-keyword", s.handleTitanSpecialistCraftingKeywordSet)
	mux.HandleFunc("GET /titan-upgrades/{slug...}", s.handleTitanUpgradeDetail)
	// Titan Slots popup (titan_slots_popup.go): plain POST-and-redirect
	// routes reusing the sheet routes' own validation (addTitanSlotPick/
	// addTitanExoSuitPick/setTitanSpecialistCraftingKeyword above), not the
	// sheet routes themselves — see subclass_tracker_popup.go's header doc
	// on why a popup needs its own routes rather than repointing these.
	mux.HandleFunc("POST /characters/{id}/titan-slots/add", s.handleTitanSlotsAdd)
	mux.HandleFunc("POST /characters/{id}/titan-slots/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickTitanUpgrade, titanSlotsPopupPath))
	mux.HandleFunc("POST /characters/{id}/titan-slots/exosuit/add", s.handleTitanSlotsExoSuitAdd)
	mux.HandleFunc("POST /characters/{id}/titan-slots/exosuit/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickTitanExosuitUpgrade, titanSlotsPopupPath))
	mux.HandleFunc("POST /characters/{id}/titan-slots/specialist-crafting", s.handleTitanSlotsSpecialistCraftingSet)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/matryoshka-split", s.handlePuppetMatryoshkaSplit)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/matryoshka-merge", s.handlePuppetMatryoshkaMerge)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/upgrades", s.handlePuppetUpgradeAdd)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/upgrades/{uid}/delete", s.handlePuppetUpgradeDelete)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/upgrades/{uid}/choices", s.handlePuppetUpgradeChoiceAdd)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/upgrades/{uid}/choices/{chid}/delete", s.handlePuppetUpgradeChoiceDelete)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/seals", s.handlePuppetSealAdd)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/seals/{sid}/delete", s.handlePuppetSealDelete)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/attacks", s.handlePuppetAttackAdd)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/attacks/{aid}/delete", s.handlePuppetAttackDelete)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/symphony-enhancement-ability", s.handleSheetPuppetSymphonyEnhancementAbility)
	mux.HandleFunc("POST /characters/{id}/companions/{cid}/titan-legion-ability", s.handleSheetTitanLegionAbilityBonus)
	mux.HandleFunc("POST /characters/{id}/sheet/proficiency", s.handleSheetProficiency)
	mux.HandleFunc("POST /characters/{id}/sheet/custom-prof", s.handleSheetCustomProf)
	mux.HandleFunc("POST /characters/{id}/sheet/proficiency-mod", s.handleSheetProficiencyMod)
	mux.HandleFunc("GET /characters/{id}/sheet/chat", s.handleSheetChat)
	mux.HandleFunc("POST /characters/{id}/sheet/chat", s.handleSheetChat)
	mux.HandleFunc("POST /characters/{id}/sheet/chat/clear", s.handleSheetChatClear)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-tactics", s.handlePuppetTacticAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-tactics/delete", s.handlePuppetTacticDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/generalized-skill", s.handleGeneralizedSkillAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/generalized-skill/{skill}/delete", s.handleGeneralizedSkillDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/elemental-affinity", s.handleElementalAffinityAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/full-metal-shinobi-resistance", s.handleFullMetalShinobiResistanceAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/sent-pick", s.handleSENTPick)
	mux.HandleFunc("POST /characters/{id}/sheet/martial-dice", s.handleSheetMartialDice)
	mux.HandleFunc("POST /characters/{id}/sheet/martial-dice/new-turn", s.handleSheetMartialDiceNewTurn)
	mux.HandleFunc("POST /characters/{id}/sheet/martial-techniques", s.handleMartialTechniqueAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/martial-techniques/delete", s.handleMartialTechniqueDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/weapon-focus", s.handleWeaponFocusAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/weapon-focus/delete", s.handleWeaponFocusDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/weapon-form-style", s.handleWeaponFormStyleAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/weapon-form-style/delete", s.handleWeaponFormStyleDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/stalking-predator", s.handleStalkingPredator)
	mux.HandleFunc("POST /characters/{id}/sheet/superior-weapon-flurry", s.handleSuperiorWeaponFlurryAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/superior-weapon-flurry/delete", s.handleSuperiorWeaponFlurryDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/martial-defense-seal", s.handleMartialDefenseSealAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/martial-defense-seal/delete", s.handleMartialDefenseSealDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/fighting-stance", s.handleFightingStance)
	mux.HandleFunc("POST /characters/{id}/sheet/stancer-mixed-martial-arts-stance", s.handleStancerMixedMartialArtsStance)
	mux.HandleFunc("POST /characters/{id}/sheet/stancer-stance-blending-stance", s.handleStancerStanceBlendingStance)
	// Weapon Form popup (weapon_form_popup.go): plain POST-and-redirect
	// routes reusing the sheet routes' own validation (addWeaponFormStyle/
	// setStalkingPredator/addSuperiorWeaponFlurryBenefit above), not the
	// sheet routes themselves — see subclass_tracker_popup.go's header doc
	// on why a popup needs its own routes rather than repointing these.
	mux.HandleFunc("POST /characters/{id}/weapon-specialist/weapon-form/style/add", s.handleWeaponFormStylePopupAdd)
	mux.HandleFunc("POST /characters/{id}/weapon-specialist/weapon-form/style/delete", s.handleWeaponFormStylePopupDelete)
	mux.HandleFunc("POST /characters/{id}/weapon-specialist/weapon-form/stalking-predator", s.handleStalkingPredatorPopup)
	mux.HandleFunc("POST /characters/{id}/weapon-specialist/weapon-form/superior-weapon-flurry/add", s.handleSuperiorWeaponFlurryPopupAdd)
	mux.HandleFunc("POST /characters/{id}/weapon-specialist/weapon-form/superior-weapon-flurry/delete", s.handleSuperiorWeaponFlurryPopupDelete)
	// Stancer / Passionate Flame / Ruin popups (taijutsu_stancer_popup.go,
	// taijutsu_passionate_flame_popup.go, taijutsu_ruin_popup.go): same
	// plain POST-and-redirect convention, reusing setStancerMixed
	// MartialArtsStance/setStancerStanceBlendingStance/setHandWrapsOfPassion/
	// setAntiChakraWavelength above.
	mux.HandleFunc("POST /characters/{id}/taijutsu-specialist/stancer/mixed-martial-arts", s.handleStancerMixedMartialArtsPopup)
	mux.HandleFunc("POST /characters/{id}/taijutsu-specialist/stancer/stance-blending", s.handleStancerStanceBlendingPopup)
	mux.HandleFunc("POST /characters/{id}/taijutsu-specialist/passionate-flame/hand-wraps-of-passion", s.handleHandWrapsOfPassionPopup)
	mux.HandleFunc("POST /characters/{id}/taijutsu-specialist/ruin/anti-chakra-wavelength", s.handleAntiChakraWavelengthPopup)
	mux.HandleFunc("POST /characters/{id}/sheet/weapon-stance", s.handleWeaponStance)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-fighting-stance", s.handlePuppetFightingStance)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-transformer-weapon-type", s.handlePuppetTransformerWeaponType)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-green-technique-jutsu", s.handlePuppetGreenTechniqueJutsu)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-thread-savant-jutsu", s.handlePuppetThreadSavantJutsu)
	mux.HandleFunc("POST /characters/{id}/sheet/puppet-always-prepared-upgrade", s.handlePuppetAlwaysPreparedUpgrade)
	mux.HandleFunc("POST /characters/{id}/sheet/hand-wraps-of-passion", s.handleHandWrapsOfPassion)
	mux.HandleFunc("POST /characters/{id}/sheet/anti-chakra-wavelength", s.handleAntiChakraWavelength)
	mux.HandleFunc("POST /characters/{id}/sheet/food-for-the-soul", s.handleFoodForTheSoul)
	mux.HandleFunc("POST /characters/{id}/sheet/fast-and-furious", s.handleFastAndFurious)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-implement", s.handleCookingToolImplement)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-damage-type", s.handleCookingToolDamageType)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-property-l1", s.handleCookingToolPropertyL1)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-property-l6", s.handleCookingToolPropertyL6)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-property-l11", s.handleCookingToolPropertyL11)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-pipe-implement", s.handleCookingToolPipeImplement)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-pipe-damage-type", s.handleCookingToolPipeDamageType)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-pipe-property-l3", s.handleCookingToolPipePropertyL3)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-pipe-property-l6", s.handleCookingToolPipePropertyL6)
	mux.HandleFunc("POST /characters/{id}/sheet/cooking-tool-pipe-property-l11", s.handleCookingToolPipePropertyL11)
	mux.HandleFunc("POST /characters/{id}/sheet/expert-combatant-weapon", s.handleBattleCookExpertCombatantWeapon)
	mux.HandleFunc("POST /characters/{id}/sheet/blend-enhancement", s.handleCookingNinBlendEnhancementAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/blend-enhancement/delete", s.handleCookingNinBlendEnhancementDelete)
	// Pipe / Expert Combatant / Fast and Furious / Blend Enhancements
	// popups — same underlying set*/add* validation as the Core-sheet
	// routes just above, called via redirect-based wrappers instead of the
	// Core sheet's AJAX fragment response.
	mux.HandleFunc("POST /characters/{id}/cooking-nin/pipe/implement", s.pipePopupSetHandler(s.setCookingToolPipeImplement))
	mux.HandleFunc("POST /characters/{id}/cooking-nin/pipe/damage-type", s.pipePopupSetHandler(s.setCookingToolPipeDamageType))
	mux.HandleFunc("POST /characters/{id}/cooking-nin/pipe/property-l3", s.pipePopupSetHandler(s.setCookingToolPipePropertyL3))
	mux.HandleFunc("POST /characters/{id}/cooking-nin/pipe/property-l6", s.pipePopupSetHandler(s.setCookingToolPipePropertyL6))
	mux.HandleFunc("POST /characters/{id}/cooking-nin/pipe/property-l11", s.pipePopupSetHandler(s.setCookingToolPipePropertyL11))
	mux.HandleFunc("POST /characters/{id}/cooking-nin/expert-combatant/weapon", s.handleBattleCookExpertCombatantPopupSet)
	mux.HandleFunc("POST /characters/{id}/cooking-nin/fast-and-furious/set", s.handleFastAndFuriousPopupSet)
	mux.HandleFunc("POST /characters/{id}/cooking-nin/blend-enhancements/add", s.handleBlendEnhancementsPopupAdd)
	mux.HandleFunc("POST /characters/{id}/cooking-nin/blend-enhancements/delete", s.handleBlendEnhancementsPopupDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/medical-doctrine", s.handleMedicalDoctrineAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/medical-doctrine/delete", s.handleMedicalDoctrineDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/medical-nin-fighting-stance", s.handleMedicalNinFightingStance)
	mux.HandleFunc("POST /characters/{id}/sheet/expert-combatant", s.handleExpertCombatantAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/expert-combatant/delete", s.handleExpertCombatantDelete)
	// Combat Medic popup (Combat Medic only) — same setMedicalNinFightingStance/
	// addExpertCombatantPick used above, called via redirect-based wrappers
	// instead of the Core sheet's AJAX fragment response.
	mux.HandleFunc("POST /characters/{id}/medical-nin/combat-medic/fighting-stance", s.handleMedicalNinFightingStancePopup)
	mux.HandleFunc("POST /characters/{id}/medical-nin/combat-medic/expert-combatant/add", s.handleExpertCombatantPopupAdd)
	mux.HandleFunc("POST /characters/{id}/medical-nin/combat-medic/expert-combatant/delete", s.handleExpertCombatantPopupDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/scout-nin-fighting-stance", s.handleScoutNinFightingStance)
	mux.HandleFunc("POST /characters/{id}/sheet/shinobi-adept", s.handleScoutNinPickAdd(charstore.ScoutNinPickShinobiAdept,
		func(d *scoutNinTabData) int { return d.ShinobiAdeptUsed },
		func(d *scoutNinTabData) int { return d.ShinobiAdeptCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableShinobiAdept }))
	mux.HandleFunc("POST /characters/{id}/sheet/shinobi-adept/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickShinobiAdept))
	mux.HandleFunc("POST /characters/{id}/sheet/jack-of-all", s.handleScoutNinPickAdd(charstore.ScoutNinPickJackOfAll,
		func(d *scoutNinTabData) int { return d.JackOfAllUsed },
		func(d *scoutNinTabData) int { return d.JackOfAllCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableJackOfAll }))
	mux.HandleFunc("POST /characters/{id}/sheet/jack-of-all/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickJackOfAll))
	mux.HandleFunc("POST /characters/{id}/sheet/scout-nin-maneuvers", s.handleScoutNinPickAdd(charstore.ScoutNinPickManeuvers,
		func(d *scoutNinTabData) int { return d.ManeuversUsed },
		func(d *scoutNinTabData) int { return d.ManeuversCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableManeuvers }))
	mux.HandleFunc("POST /characters/{id}/sheet/scout-nin-maneuvers/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickManeuvers))
	mux.HandleFunc("POST /characters/{id}/sheet/signature-technique", s.handleScoutNinPickAdd(charstore.ScoutNinPickSignatureTechnique,
		func(d *scoutNinTabData) int { return d.SignatureTechniqueUsed },
		func(d *scoutNinTabData) int { return d.SignatureTechniqueCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableSignatureTechnique }))
	mux.HandleFunc("POST /characters/{id}/sheet/signature-technique/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickSignatureTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/mobile-savant", s.handleScoutNinPickAdd(charstore.ScoutNinPickMobileSavant,
		func(d *scoutNinTabData) int { return d.MobileSavantUsed },
		func(d *scoutNinTabData) int { return d.MobileSavantCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableMobileSavant }))
	mux.HandleFunc("POST /characters/{id}/sheet/mobile-savant/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickMobileSavant))
	mux.HandleFunc("POST /characters/{id}/sheet/tactical-superiority", s.handleScoutNinPickAdd(charstore.ScoutNinPickTacticalSuperiority,
		func(d *scoutNinTabData) int { return d.TacticalSuperiorityUsed },
		func(d *scoutNinTabData) int { return d.TacticalSuperiorityCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableTacticalSuperiority }))
	mux.HandleFunc("POST /characters/{id}/sheet/tactical-superiority/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickTacticalSuperiority))
	mux.HandleFunc("POST /characters/{id}/sheet/signature-maneuver", s.handleScoutNinPickAdd(charstore.ScoutNinPickSignatureManeuver,
		func(d *scoutNinTabData) int { return d.SignatureManeuverUsed },
		func(d *scoutNinTabData) int { return d.SignatureManeuverCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableSignatureManeuver }))
	mux.HandleFunc("POST /characters/{id}/sheet/signature-maneuver/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickSignatureManeuver))
	mux.HandleFunc("POST /characters/{id}/sheet/supreme-clones", s.handleScoutNinPickAdd(charstore.ScoutNinPickSupremeClones,
		func(d *scoutNinTabData) int { return d.SupremeClonesUsed },
		func(d *scoutNinTabData) int { return d.SupremeClonesCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableSupremeClones }))
	mux.HandleFunc("POST /characters/{id}/sheet/supreme-clones/delete", s.handleScoutNinPickDelete(charstore.ScoutNinPickSupremeClones))
	mux.HandleFunc("POST /characters/{id}/sheet/change-of-heart", s.handleChangeOfHeart)
	mux.HandleFunc("POST /characters/{id}/sheet/paragons-presence", s.handleParagonsPresence)
	mux.HandleFunc("POST /characters/{id}/sheet/signature-jutsu", s.handleSignatureJutsuJutsu)
	mux.HandleFunc("POST /characters/{id}/sheet/signature-jutsu-effect", s.handleSignatureJutsuEffect)
	// Scouting Technique popup (all 9 subclasses) — same
	// setChangeOfHeart/setParagonsPresence/scoutNinPickAddCore used above,
	// called via redirect-based wrappers instead of the Core sheet's AJAX
	// fragment response.
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/maneuvers/add", s.scoutNinTrackerPopupAdd(charstore.ScoutNinPickManeuvers,
		func(d *scoutNinTabData) int { return d.ManeuversUsed },
		func(d *scoutNinTabData) int { return d.ManeuversCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableManeuvers }))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/maneuvers/delete", s.scoutNinTrackerPopupDelete(charstore.ScoutNinPickManeuvers))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/tactical-superiority/add", s.scoutNinTrackerPopupAdd(charstore.ScoutNinPickTacticalSuperiority,
		func(d *scoutNinTabData) int { return d.TacticalSuperiorityUsed },
		func(d *scoutNinTabData) int { return d.TacticalSuperiorityCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableTacticalSuperiority }))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/tactical-superiority/delete", s.scoutNinTrackerPopupDelete(charstore.ScoutNinPickTacticalSuperiority))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/signature-maneuver/add", s.scoutNinTrackerPopupAdd(charstore.ScoutNinPickSignatureManeuver,
		func(d *scoutNinTabData) int { return d.SignatureManeuverUsed },
		func(d *scoutNinTabData) int { return d.SignatureManeuverCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableSignatureManeuver }))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/signature-maneuver/delete", s.scoutNinTrackerPopupDelete(charstore.ScoutNinPickSignatureManeuver))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/supreme-clones/add", s.scoutNinTrackerPopupAdd(charstore.ScoutNinPickSupremeClones,
		func(d *scoutNinTabData) int { return d.SupremeClonesUsed },
		func(d *scoutNinTabData) int { return d.SupremeClonesCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableSupremeClones }))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/supreme-clones/delete", s.scoutNinTrackerPopupDelete(charstore.ScoutNinPickSupremeClones))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/mobile-savant/add", s.scoutNinTrackerPopupAdd(charstore.ScoutNinPickMobileSavant,
		func(d *scoutNinTabData) int { return d.MobileSavantUsed },
		func(d *scoutNinTabData) int { return d.MobileSavantCap },
		func(d *scoutNinTabData) []scoutNinPickOption { return d.AvailableMobileSavant }))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/mobile-savant/delete", s.scoutNinTrackerPopupDelete(charstore.ScoutNinPickMobileSavant))
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/change-of-heart", s.handleChangeOfHeartPopup)
	mux.HandleFunc("POST /characters/{id}/scout-nin/scouting-technique/paragons-presence", s.handleParagonsPresencePopup)
	mux.HandleFunc("POST /characters/{id}/sheet/lethal-precision", s.handleHunterPickAdd(charstore.HunterPickLethalPrecision,
		func(d *hunterTechniquesTabData) int { return d.LethalPrecisionUsed },
		func(d *hunterTechniquesTabData) int { return d.LethalPrecisionCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableLethalPrecision }))
	mux.HandleFunc("POST /characters/{id}/sheet/lethal-precision/delete", s.handleHunterPickDelete(charstore.HunterPickLethalPrecision))
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-patterns", s.handleHunterPickAdd(charstore.HunterPickPattern,
		func(d *hunterTechniquesTabData) int { return d.PatternsUsed },
		func(d *hunterTechniquesTabData) int { return d.PatternsCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailablePatterns }))
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-patterns/delete", s.handleHunterPickDelete(charstore.HunterPickPattern))
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-patterns/choice", s.handleSheetHunterPatternChoice)
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-patterns/practiced-combatant-stance", s.handleHunterPracticedCombatantStance)
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-exploits", s.handleHunterPickAdd(charstore.HunterPickExploit,
		func(d *hunterTechniquesTabData) int { return d.ExploitsUsed },
		func(d *hunterTechniquesTabData) int { return d.ExploitsCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableExploits }))
	mux.HandleFunc("POST /characters/{id}/sheet/hunter-exploits/delete", s.handleHunterPickDelete(charstore.HunterPickExploit))
	mux.HandleFunc("POST /characters/{id}/sheet/defensive-tactics", s.handleHunterPickAdd(charstore.HunterPickDefensiveTactic,
		func(d *hunterTechniquesTabData) int { return d.DefensiveTacticsUsed },
		func(d *hunterTechniquesTabData) int { return d.DefensiveTacticsCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableDefensiveTactics }))
	mux.HandleFunc("POST /characters/{id}/sheet/defensive-tactics/delete", s.handleHunterPickDelete(charstore.HunterPickDefensiveTactic))
	// The 8 Hunter's Creeds' own subclass-exclusive picks — see
	// hunterTechniquesTabData's own doc comment (hunter_nin.go). Arsenal
	// Item's add route is its own handler (handleHunterArsenalItemAdd), not
	// the shared handleHunterPickAdd factory, since 3 of its 14 options are
	// real jutsu; its delete route is still the generic handleHunterPickDelete
	// with no cascade needed, since the jutsu grant itself is computed
	// straight off this pick (hunterNinArsenalItemGrantedJutsu, hunter_nin.go)
	// rather than stored as its own character_jutsu row — there is nothing
	// for the delete route to forget beyond the pick itself.
	mux.HandleFunc("POST /characters/{id}/sheet/warden-weapon", s.handleHunterPickAdd(charstore.HunterPickWardenWeapon,
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponUsed },
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableWardenWeapon }))
	mux.HandleFunc("POST /characters/{id}/sheet/warden-weapon/delete", s.handleHunterPickDelete(charstore.HunterPickWardenWeapon))
	mux.HandleFunc("POST /characters/{id}/sheet/warden-weapon-property", s.handleHunterPickAdd(charstore.HunterPickWardenWeaponProperty,
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponPropertyUsed },
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponPropertyCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableWardenWeaponProperty }))
	mux.HandleFunc("POST /characters/{id}/sheet/warden-weapon-property/delete", s.handleHunterPickDelete(charstore.HunterPickWardenWeaponProperty))
	mux.HandleFunc("POST /characters/{id}/sheet/medical-technique", s.handleHunterPickAdd(charstore.HunterPickMedicalTechnique,
		func(d *hunterTechniquesTabData) int { return d.MedicalTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.MedicalTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableMedicalTechnique }))
	mux.HandleFunc("POST /characters/{id}/sheet/medical-technique/delete", s.handleHunterPickDelete(charstore.HunterPickMedicalTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/shadow-technique", s.handleHunterPickAdd(charstore.HunterPickShadowTechnique,
		func(d *hunterTechniquesTabData) int { return d.ShadowTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.ShadowTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableShadowTechnique }))
	mux.HandleFunc("POST /characters/{id}/sheet/shadow-technique/delete", s.handleHunterPickDelete(charstore.HunterPickShadowTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/arsenal-item", s.handleHunterArsenalItemAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/arsenal-item/delete", s.handleHunterPickDelete(charstore.HunterPickArsenalItem))
	mux.HandleFunc("POST /characters/{id}/sheet/toxic-technique", s.handleHunterPickAdd(charstore.HunterPickToxicTechnique,
		func(d *hunterTechniquesTabData) int { return d.ToxicTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.ToxicTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableToxicTechnique }))
	mux.HandleFunc("POST /characters/{id}/sheet/toxic-technique/delete", s.handleHunterPickDelete(charstore.HunterPickToxicTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/vice-technique", s.handleHunterPickAdd(charstore.HunterPickViceTechnique,
		func(d *hunterTechniquesTabData) int { return d.ViceTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.ViceTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableViceTechnique }))
	mux.HandleFunc("POST /characters/{id}/sheet/vice-technique/delete", s.handleHunterPickDelete(charstore.HunterPickViceTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/void-technique", s.handleHunterPickAdd(charstore.HunterPickVoidTechnique,
		func(d *hunterTechniquesTabData) int { return d.VoidTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.VoidTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableVoidTechnique }))
	mux.HandleFunc("POST /characters/{id}/sheet/void-technique/delete", s.handleHunterPickDelete(charstore.HunterPickVoidTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/prosthetic-attachment", s.handleHunterPickAdd(charstore.HunterPickProstheticAttachment,
		func(d *hunterTechniquesTabData) int { return d.ProstheticAttachmentUsed },
		func(d *hunterTechniquesTabData) int { return d.ProstheticAttachmentCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableProstheticAttachment }))
	mux.HandleFunc("POST /characters/{id}/sheet/prosthetic-attachment/delete", s.handleHunterPickDelete(charstore.HunterPickProstheticAttachment))
	// Wolf Technique's add route is its own handler (handleHunterWolfTechniqueAdd),
	// not the shared handleHunterPickAdd factory, since every option is a
	// real jutsu (same reason Arsenal Item above uses its own handler) —
	// its delete route is still the generic handleHunterPickDelete, same
	// "nothing to cascade" reasoning as Arsenal Item's own comment above.
	mux.HandleFunc("POST /characters/{id}/sheet/wolf-technique", s.handleHunterWolfTechniqueAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/wolf-technique/delete", s.handleHunterPickDelete(charstore.HunterPickWolfTechnique))
	mux.HandleFunc("GET /hunter-techniques/{category}/{slug...}", s.handleHunterPickDetail)
	// Hunter's Creed popup (hunter_creed_popup.go): plain POST-and-redirect
	// routes reusing the Core-sheet routes' own validation (hunterPickAddCore/
	// hunterArsenalItemAddCore/hunterWolfTechniqueAddCore above), not the
	// sheet routes themselves — see subclass_tracker_popup.go's header doc
	// on why a popup needs its own routes rather than repointing these.
	// Delete is fully generic (hunterNinTrackerPopupDelete) for all 10
	// sections, same as every other subclass tracker popup — just its own
	// factory rather than subclassTrackerPopupDelete, since Hunter-Nin's
	// picks table needs its own charstore types (see that factory's own doc
	// comment, hunter_creed_popup.go).
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/warden-weapon/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickWardenWeapon,
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponUsed },
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableWardenWeapon }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/warden-weapon/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickWardenWeapon))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/warden-weapon-property/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickWardenWeaponProperty,
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponPropertyUsed },
		func(d *hunterTechniquesTabData) int { return d.WardenWeaponPropertyCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableWardenWeaponProperty }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/warden-weapon-property/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickWardenWeaponProperty))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/medical-technique/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickMedicalTechnique,
		func(d *hunterTechniquesTabData) int { return d.MedicalTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.MedicalTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableMedicalTechnique }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/medical-technique/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickMedicalTechnique))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/shadow-technique/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickShadowTechnique,
		func(d *hunterTechniquesTabData) int { return d.ShadowTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.ShadowTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableShadowTechnique }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/shadow-technique/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickShadowTechnique))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/arsenal-item/add", s.handleHunterArsenalItemPopupAdd)
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/arsenal-item/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickArsenalItem))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/toxic-technique/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickToxicTechnique,
		func(d *hunterTechniquesTabData) int { return d.ToxicTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.ToxicTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableToxicTechnique }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/toxic-technique/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickToxicTechnique))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/vice-technique/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickViceTechnique,
		func(d *hunterTechniquesTabData) int { return d.ViceTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.ViceTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableViceTechnique }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/vice-technique/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickViceTechnique))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/void-technique/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickVoidTechnique,
		func(d *hunterTechniquesTabData) int { return d.VoidTechniqueUsed },
		func(d *hunterTechniquesTabData) int { return d.VoidTechniqueCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableVoidTechnique }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/void-technique/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickVoidTechnique))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/prosthetic-attachment/add", s.hunterNinTrackerPopupAdd(charstore.HunterPickProstheticAttachment,
		func(d *hunterTechniquesTabData) int { return d.ProstheticAttachmentUsed },
		func(d *hunterTechniquesTabData) int { return d.ProstheticAttachmentCap },
		func(d *hunterTechniquesTabData) []hunterPickOption { return d.AvailableProstheticAttachment }))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/prosthetic-attachment/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickProstheticAttachment))
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/wolf-technique/add", s.handleHunterWolfTechniquePopupAdd)
	mux.HandleFunc("POST /characters/{id}/hunter-nin/creed/wolf-technique/delete", s.hunterNinTrackerPopupDelete(charstore.HunterPickWolfTechnique))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-mirages", s.handleGenjutsuPickAdd(charstore.GenjutsuPickMirage,
		func(d *genjutsuTabData) int { return d.MiragesUsed },
		func(d *genjutsuTabData) int { return d.MiragesCap },
		func(d *genjutsuTabData) []genjutsuPickOption { return d.AvailableMirages }))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-mirages/delete", s.handleGenjutsuPickDelete(charstore.GenjutsuPickMirage))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-inception", s.handleGenjutsuPickAdd(charstore.GenjutsuPickInception,
		func(d *genjutsuTabData) int { return d.InceptionUsed },
		func(d *genjutsuTabData) int { return d.InceptionCap },
		func(d *genjutsuTabData) []genjutsuPickOption { return d.AvailableInception }))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-inception/delete", s.handleGenjutsuPickDelete(charstore.GenjutsuPickInception))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-conversions", s.handleGenjutsuPickAdd(charstore.GenjutsuPickConversion,
		func(d *genjutsuTabData) int { return d.ConversionUsed },
		func(d *genjutsuTabData) int { return d.ConversionCap },
		func(d *genjutsuTabData) []genjutsuPickOption { return d.AvailableConversions }))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-conversions/delete", s.handleGenjutsuPickDelete(charstore.GenjutsuPickConversion))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-illusion-mastery", s.handleGenjutsuPickAdd(charstore.GenjutsuPickIllusionMastery,
		func(d *genjutsuTabData) int { return d.IllusionMasteryUsed },
		func(d *genjutsuTabData) int { return d.IllusionMasteryCap },
		func(d *genjutsuTabData) []genjutsuPickOption { return d.AvailableIllusionMastery }))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-illusion-mastery/delete", s.handleGenjutsuPickDelete(charstore.GenjutsuPickIllusionMastery))
	mux.HandleFunc("GET /genjutsu-picks/{category}/{slug...}", s.handleGenjutsuPickDetail)
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-twisted-casting", s.handleGenjutsuJutsuPickAdd(charstore.GenjutsuPickTwistedCasting,
		func(d *genjutsuTabData) int { return d.TwistedCastingUsed },
		func(d *genjutsuTabData) int { return d.TwistedCastingCap },
		func(d *genjutsuTabData) []knownJutsuOption { return d.AvailableTwistedCasting }))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-twisted-casting/delete", s.handleGenjutsuJutsuPickDelete(charstore.GenjutsuPickTwistedCasting))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-psyche-breaker", s.handleGenjutsuJutsuPickAdd(charstore.GenjutsuPickPsycheBreaker,
		func(d *genjutsuTabData) int { return d.PsycheBreakerUsed },
		func(d *genjutsuTabData) int { return d.PsycheBreakerCap },
		func(d *genjutsuTabData) []knownJutsuOption { return d.AvailablePsycheBreaker }))
	mux.HandleFunc("POST /characters/{id}/sheet/genjutsu-psyche-breaker/delete", s.handleGenjutsuJutsuPickDelete(charstore.GenjutsuPickPsycheBreaker))
	// Twisted Casting / Psyche Breaker popups (genjutsu_twisted_casting_
	// popup.go, genjutsu_psyche_breaker_popup.go) — same underlying
	// addGenjutsuJutsuPick validation as the Core-sheet routes just above,
	// called via redirect-based wrappers instead of the Core sheet's AJAX
	// fragment response.
	mux.HandleFunc("POST /characters/{id}/genjutsu-specialist/twisted-casting/add", s.handleTwistedCastingPopupAdd)
	mux.HandleFunc("POST /characters/{id}/genjutsu-specialist/twisted-casting/delete", s.handleTwistedCastingPopupDelete)
	mux.HandleFunc("POST /characters/{id}/genjutsu-specialist/psyche-breaker/add", s.handlePsycheBreakerPopupAdd)
	mux.HandleFunc("POST /characters/{id}/genjutsu-specialist/psyche-breaker/delete", s.handlePsycheBreakerPopupDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/intelligence-operative-plans", s.handleIntelligenceOperativePickAdd(charstore.IntelligenceOperativePickPlan,
		func(d *intelligenceOperativeTabData) int { return d.PlansUsed },
		func(d *intelligenceOperativeTabData) int { return d.PlansCap },
		func(d *intelligenceOperativeTabData) []intelligenceOperativePickOption { return d.AvailablePlans }))
	mux.HandleFunc("POST /characters/{id}/sheet/intelligence-operative-plans/delete", s.handleIntelligenceOperativePickDelete(charstore.IntelligenceOperativePickPlan))
	mux.HandleFunc("POST /characters/{id}/sheet/operative-traps", s.handleOperativeTrapAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/operative-traps/delete", s.handleIntelligenceOperativePickDelete(charstore.IntelligenceOperativePickOperativeTrap))
	mux.HandleFunc("GET /intelligence-operative-picks/{category}/{slug...}", s.handleIntelligenceOperativePickDetail)
	// Operative Traps popup (Tactical Strategist only) — same
	// addOperativeTrapPick used above, called via a redirect-based wrapper
	// instead of the Core sheet's AJAX fragment response.
	mux.HandleFunc("POST /characters/{id}/intelligence-operative/operative-traps/add", s.handleOperativeTrapPopupAdd)
	mux.HandleFunc("POST /characters/{id}/intelligence-operative/operative-traps/delete", s.handleOperativeTrapPopupDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/ninjutsu-molding", s.handleNinjutsuMoldingAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/ninjutsu-molding/delete", s.handleNinjutsuMoldingDelete)
	mux.HandleFunc("GET /ninjutsu-molding/{slug...}", s.handleNinjutsuMoldingDetail)
	mux.HandleFunc("POST /characters/{id}/sheet/refined-ninjutsu", s.handleNinjutsuJutsuPickAdd(charstore.NinjutsuPickRefined,
		func(d *ninjutsuSpecialistTabData) int { return d.RefinedUsed },
		func(d *ninjutsuSpecialistTabData) int { return d.RefinedCap },
		func(d *ninjutsuSpecialistTabData) []knownJutsuOption { return d.AvailableRefined }))
	mux.HandleFunc("POST /characters/{id}/sheet/refined-ninjutsu/delete", s.handleNinjutsuJutsuPickDelete(charstore.NinjutsuPickRefined))
	mux.HandleFunc("POST /characters/{id}/sheet/ninjutsu-master", s.handleNinjutsuJutsuPickAdd(charstore.NinjutsuPickMaster,
		func(d *ninjutsuSpecialistTabData) int { return d.MasterUsed },
		func(d *ninjutsuSpecialistTabData) int { return d.MasterCap },
		func(d *ninjutsuSpecialistTabData) []knownJutsuOption { return d.AvailableMaster }))
	mux.HandleFunc("POST /characters/{id}/sheet/ninjutsu-master/delete", s.handleNinjutsuJutsuPickDelete(charstore.NinjutsuPickMaster))
	mux.HandleFunc("POST /characters/{id}/sheet/awakened-scroll", s.handleAwakenedScrollAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/awakened-scroll/delete", s.handleNinjutsuJutsuPickDelete(charstore.NinjutsuPickAwakenedScroll))
	// Awakened Scroll popup (Scribe Master only) — same addAwakenedScrollPick
	// used above, called via a redirect-based wrapper instead of the Core
	// sheet's AJAX fragment response.
	mux.HandleFunc("POST /characters/{id}/ninjutsu-specialist/awakened-scroll/add", s.handleAwakenedScrollPopupAdd)
	mux.HandleFunc("POST /characters/{id}/ninjutsu-specialist/awakened-scroll/delete", s.handleAwakenedScrollPopupDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-tools", s.handleScienceNinToolAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-tools/delete", s.handleScienceNinToolDelete)
	mux.HandleFunc("GET /science-nin-tools/{slug...}", s.handleScienceNinToolDetail)
	mux.HandleFunc("POST /characters/{id}/sheet/exoskeleton", s.handleSheetExoskeletonToggle)
	// Infused Genius (11th level, BASE class — unlike every subclass-gated
	// closure below, InfusedGenius applies to any Science-Nin regardless of
	// subclass) reuses the same generic handleScienceNinSubclassPickAdd/
	// Delete factory those closures share.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-infused-tool", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickInfusedTool,
		func(d *scienceNinToolsTabData) int {
			if d.InfusedGenius == nil {
				return 0
			}
			return d.InfusedGenius.Used
		},
		func(d *scienceNinToolsTabData) int {
			if d.InfusedGenius == nil {
				return 0
			}
			return d.InfusedGenius.Cap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.InfusedGenius == nil {
				return nil
			}
			return d.InfusedGenius.Available
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-infused-tool/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickInfusedTool))
	// Every closure below guards its own subclass-data pointer (nil
	// whenever the character lacks that subclass's own granting feature —
	// see loadScienceNinSubclassData) before dereferencing it: a Science-Nin
	// with a different (or no) subclass yet still has a non-nil
	// scienceNinToolsTabData (from the base Scientific Ninja Tools budget
	// alone), so handleScienceNinSubclassPickAdd's own nil check on data
	// itself isn't enough to rule out a nil ElementalInnovationist/
	// Grenadier/MadScientist/Ninjaneer underneath it.
	// E.I.P's own ADD route uses the dedicated handleScienceNinEIPAdd
	// (science_nin_subclasses.go) rather than this generic factory — E.I.Ps
	// spend from the shared Creation Points budget (see that file's header
	// doc), which this factory's own used/cap closures can't check. Its
	// DELETE route is dedicated too (handleScienceNinEIPDelete) — forgetting
	// an E.I.P also needs to clear a dangling Perma Perk designation if that
	// E.I.P was the one designated (see removeEIPPick's own doc); S.N.B
	// Upgrades still uses the generic handleScienceNinSubclassPickDelete
	// below, since it has no equivalent designation to clean up. W.O.W's own
	// ADD and DELETE routes are both dedicated too (handleScienceNinWOWAdd/
	// handleScienceNinWOWDelete, science_nin_elemental_innovationist_popup.go)
	// — a W.o.W pick also grants (and, on delete, revokes) a real equipped
	// weapon; see wow_weapons.go.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-eip", s.handleScienceNinEIPAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-eip/delete", s.handleScienceNinEIPDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-wow", s.handleScienceNinWOWAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-wow/delete", s.handleScienceNinWOWDelete)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-ascended-wow", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickAscendedWoW,
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil || d.ElementalInnovationist.DesignatedWoW == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ElementalInnovationist == nil {
				return nil
			}
			return d.ElementalInnovationist.AvailableDesignatedWoW
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-ascended-wow/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickAscendedWoW))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-perma-perk", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickPermaPerk,
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil || d.ElementalInnovationist.PermaPerk == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ElementalInnovationist == nil {
				return nil
			}
			return d.ElementalInnovationist.AvailablePermaPerk
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-perma-perk/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickPermaPerk))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-bim", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickBIM,
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil {
				return 0
			}
			return d.Grenadier.BIMUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil {
				return 0
			}
			return d.Grenadier.BIMCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Grenadier == nil {
				return nil
			}
			return d.Grenadier.AvailableBIM
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-bim/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickBIM))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-bim-specialist", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickBIMSpecialist,
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil || d.Grenadier.DesignatedBIM == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Grenadier == nil {
				return nil
			}
			return d.Grenadier.AvailableDesignatedBIM
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-bim-specialist/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickBIMSpecialist))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-inversion-serum", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickInversionSerum,
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil {
				return 0
			}
			return d.MadScientist.SerumUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil {
				return 0
			}
			return d.MadScientist.SerumCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.MadScientist == nil {
				return nil
			}
			return d.MadScientist.AvailableSerums
		},
		true))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-inversion-serum/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickInversionSerum))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-sheep-and-shepherd", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickSheepAndShepherdSerum,
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil || d.MadScientist.DesignatedSerum == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.MadScientist == nil {
				return nil
			}
			return d.MadScientist.AvailableDesignatedSerum
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-sheep-and-shepherd/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickSheepAndShepherdSerum))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-arsenal-mod", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickArsenalMod,
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.ArsenalModUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.ArsenalModCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Ninjaneer == nil {
				return nil
			}
			return d.Ninjaneer.AvailableArsenalMods
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-arsenal-mod/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickArsenalMod))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-perfected-weapon", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickPerfectedWeapon,
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.PerfectedWeaponUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.PerfectedWeaponCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Ninjaneer == nil {
				return nil
			}
			return d.Ninjaneer.AvailablePerfectedWeapons
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-perfected-weapon/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickPerfectedWeapon))
	// Ninjaneer's own weapon designations (which owned inventory row is the
	// Enhanced/Legendary/Perfected Weapon) — see ninjaneer.go. Bespoke
	// handlers rather than handleScienceNinSubclassPickAdd/Delete's shared
	// factory, since candidates come from the character's own inventory,
	// not a static class_options catalog.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-enhanced-weapon", s.handleNinjaneerWeaponDesignationAdd(ninjaneerEnhancedWeaponTier))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-enhanced-weapon/delete", s.handleNinjaneerWeaponDesignationDelete(ninjaneerEnhancedWeaponTier))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-legendary-weapon", s.handleNinjaneerWeaponDesignationAdd(ninjaneerLegendaryWeaponTier))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-legendary-weapon/delete", s.handleNinjaneerWeaponDesignationDelete(ninjaneerLegendaryWeaponTier))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-legendary-weapon-active", s.handleSheetNinjaneerLegendaryWeaponToggle)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-perfected-weapon-mark", s.handleNinjaneerWeaponDesignationAdd(ninjaneerPerfectedWeaponMarkTier))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-perfected-weapon-mark/delete", s.handleNinjaneerWeaponDesignationDelete(ninjaneerPerfectedWeaponMarkTier))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-shinobi-ware-upgrade", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickShinobiWareUpgrade,
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.UpgradeUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.UpgradeCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ShinobiWare == nil {
				return nil
			}
			return d.ShinobiWare.AvailableUpgrades
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-shinobi-ware-upgrade/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickShinobiWareUpgrade))
	// In His Image's Shinjutsu Upgrade pick is permanent ("You can not
	// change this later") — no delete route is wired for it, unlike every
	// other pick in this file.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-shinjutsu-upgrade", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickShinjutsuUpgrade,
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil || d.ShinobiWare.ShinjutsuUpgrade == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ShinobiWare == nil {
				return nil
			}
			return d.ShinobiWare.AvailableShinjutsuUpgrade
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-evolved-upgrade", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickEvolvedUpgrade,
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.EvolvedUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.EvolvedCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ShinobiWare == nil {
				return nil
			}
			return d.ShinobiWare.AvailableEvolved
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-evolved-upgrade/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickEvolvedUpgrade))
	// Ever Evolving's own seal pick is a bespoke handler (ever_evolving.go),
	// not handleScienceNinSubclassPickAdd's shared factory — it's gated by
	// the shared Creation Points BUDGET, not a flat slot-count cap, the
	// same reason Titan Upgrades' own routes below bypass that factory too.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-ever-evolving-seal", s.handleEverEvolvingSealAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-ever-evolving-seal/delete", s.handleEverEvolvingSealDelete)
	// Spyware Program's own ADD route uses the dedicated
	// handleScienceNinSpywareProgramAdd (science_nin_subclasses.go) rather
	// than this generic factory — Programs spend from the shared Creation
	// Points budget (see that file's header doc), which this factory's own
	// used/cap closures can't check. Its DELETE route still uses the generic
	// handleScienceNinSubclassPickDelete below, same as S.N.B Upgrades.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-spyware-program", s.handleScienceNinSpywareProgramAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-spyware-program/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickSpywareProgram))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-quick-hack", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickQuickHack,
		func(d *scienceNinToolsTabData) int {
			if d.Spyware == nil || d.Spyware.QuickHack == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.Spyware == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Spyware == nil {
				return nil
			}
			return d.Spyware.AvailableQuickHack
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-quick-hack/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickQuickHack))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-air-treck-enhancement", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickAirTreckEnhancement,
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			return d.StormRider.EnhancementUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			return d.StormRider.EnhancementCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.StormRider == nil {
				return nil
			}
			return d.StormRider.AvailableEnhancements
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-air-treck-enhancement/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickAirTreckEnhancement))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-regalia", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickRegalia,
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			return d.StormRider.RegaliaUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			// 1 normally, 2 once Sky Keeper (20th level) is also granted —
			// see loadScienceNinSubclassData's own RegaliaCap computation.
			return d.StormRider.RegaliaCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.StormRider == nil {
				return nil
			}
			return d.StormRider.AvailableRegalia
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-regalia/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickRegalia))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-technobi-mechanization", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickTechnobiMechanization,
		func(d *scienceNinToolsTabData) int {
			if d.Technobi == nil {
				return 0
			}
			return d.Technobi.MechanizationUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Technobi == nil {
				return 0
			}
			return d.Technobi.MechanizationCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Technobi == nil {
				return nil
			}
			return d.Technobi.AvailableMechanizations
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-technobi-mechanization/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickTechnobiMechanization))
	// Dedicated handler, not the generic factory above — S.N.B Upgrades
	// also needs a Creation-Points-budget check the factory's own signature
	// has no room for. See handleScienceNinSNBUpgradeAdd's own doc.
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-snb-upgrade", s.handleScienceNinSNBUpgradeAdd)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-snb-upgrade/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickSNBUpgrade))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-snb-upgrade-permanent", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickSNBUpgradePermanent,
		func(d *scienceNinToolsTabData) int {
			if d.SNBSpecialist == nil {
				return 0
			}
			return d.SNBSpecialist.PermanentUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.SNBSpecialist == nil {
				return 0
			}
			return d.SNBSpecialist.PermanentCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.SNBSpecialist == nil {
				return nil
			}
			return d.SNBSpecialist.AvailablePermanent
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-snb-upgrade-permanent/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickSNBUpgradePermanent))
	// S.N.B Upgrades popup (snb_upgrades_popup.go): plain POST-and-redirect
	// routes reusing the sheet routes' own validation (addSNBUpgradePick
	// above), not the sheet routes themselves — see subclass_tracker_
	// popup.go's header doc on why a popup needs its own routes rather
	// than repointing these.
	mux.HandleFunc("POST /characters/{id}/snb-upgrades/add", s.handleSNBUpgradesAdd)
	mux.HandleFunc("POST /characters/{id}/snb-upgrades/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickSNBUpgrade, snbUpgradesPopupPath))
	mux.HandleFunc("POST /characters/{id}/snb-upgrades/permanent/add", s.handleSNBUpgradesPermanentAdd)
	mux.HandleFunc("POST /characters/{id}/snb-upgrades/permanent/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickSNBUpgradePermanent, snbUpgradesPopupPath))
	// Elemental Innovationist popup (science_nin_elemental_innovationist_
	// popup.go): plain POST-and-redirect routes reusing
	// handleScienceNinSubclassPickAdd's own core (scienceNinTrackerPopupAdd)
	// via the same used/cap/available closures the Core-sheet routes above
	// use, not those routes themselves — see subclass_tracker_popup.go's
	// header doc on why a popup needs its own routes.
	// E.I.P's own popup ADD route uses the dedicated handleEIPPopupAdd
	// (science_nin_subclasses.go) — see the Core-sheet route's own comment
	// above for why this catalog can't use the generic factory. Its DELETE
	// route is dedicated too (handleEIPPopupDelete) — forgetting an E.I.P
	// also needs to clear a dangling Perma Perk designation if that E.I.P was
	// the one designated (see removeEIPPick's own doc). W.O.W's own ADD and
	// DELETE routes are both dedicated too (handleWoWPopupAdd/
	// handleWoWPopupDelete, science_nin_elemental_innovationist_popup.go) —
	// a W.o.W pick also grants (and, on delete, revokes) a real equipped
	// weapon, see wow_weapons.go.
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/eip/add", s.handleEIPPopupAdd)
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/eip/delete", s.handleEIPPopupDelete)
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/wow/add", s.handleWoWPopupAdd)
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/wow/delete", s.handleWoWPopupDelete)
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/ascended-wow/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickAscendedWoW,
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil || d.ElementalInnovationist.DesignatedWoW == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ElementalInnovationist == nil {
				return nil
			}
			return d.ElementalInnovationist.AvailableDesignatedWoW
		},
		false, elementalInnovationistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/ascended-wow/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickAscendedWoW, elementalInnovationistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/perma-perk/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickPermaPerk,
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil || d.ElementalInnovationist.PermaPerk == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.ElementalInnovationist == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ElementalInnovationist == nil {
				return nil
			}
			return d.ElementalInnovationist.AvailablePermaPerk
		},
		false, elementalInnovationistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/perma-perk/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickPermaPerk, elementalInnovationistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/elemental-innovationist/exoskeleton", s.handleExoskeletonPopupToggle)
	// Grenadier popup (science_nin_grenadier_popup.go).
	mux.HandleFunc("POST /characters/{id}/science-nin/grenadier/bim/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickBIM,
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil {
				return 0
			}
			return d.Grenadier.BIMUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil {
				return 0
			}
			return d.Grenadier.BIMCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Grenadier == nil {
				return nil
			}
			return d.Grenadier.AvailableBIM
		},
		false, grenadierPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/grenadier/bim/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickBIM, grenadierPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/grenadier/bim-specialist/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickBIMSpecialist,
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil || d.Grenadier.DesignatedBIM == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.Grenadier == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Grenadier == nil {
				return nil
			}
			return d.Grenadier.AvailableDesignatedBIM
		},
		false, grenadierPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/grenadier/bim-specialist/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickBIMSpecialist, grenadierPopupPath))
	// Mad Scientist popup (science_nin_mad_scientist_popup.go).
	mux.HandleFunc("POST /characters/{id}/science-nin/mad-scientist/inversion-serum/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickInversionSerum,
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil {
				return 0
			}
			return d.MadScientist.SerumUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil {
				return 0
			}
			return d.MadScientist.SerumCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.MadScientist == nil {
				return nil
			}
			return d.MadScientist.AvailableSerums
		},
		true, madScientistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/mad-scientist/inversion-serum/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickInversionSerum, madScientistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/mad-scientist/sheep-and-shepherd/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickSheepAndShepherdSerum,
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil || d.MadScientist.DesignatedSerum == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.MadScientist == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.MadScientist == nil {
				return nil
			}
			return d.MadScientist.AvailableDesignatedSerum
		},
		false, madScientistPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/mad-scientist/sheep-and-shepherd/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickSheepAndShepherdSerum, madScientistPopupPath))
	// Ninjaneer popup (science_nin_ninjaneer_popup.go).
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/arsenal-mod/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickArsenalMod,
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.ArsenalModUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.ArsenalModCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Ninjaneer == nil {
				return nil
			}
			return d.Ninjaneer.AvailableArsenalMods
		},
		false, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/arsenal-mod/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickArsenalMod, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/perfected-weapon/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickPerfectedWeapon,
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.PerfectedWeaponUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Ninjaneer == nil {
				return 0
			}
			return d.Ninjaneer.PerfectedWeaponCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Ninjaneer == nil {
				return nil
			}
			return d.Ninjaneer.AvailablePerfectedWeapons
		},
		false, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/perfected-weapon/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickPerfectedWeapon, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/enhanced-weapon/add", s.ninjaneerWeaponDesignationPopupAdd(ninjaneerEnhancedWeaponTier, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/enhanced-weapon/delete", s.ninjaneerWeaponDesignationPopupDelete(ninjaneerEnhancedWeaponTier, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/legendary-weapon/add", s.ninjaneerWeaponDesignationPopupAdd(ninjaneerLegendaryWeaponTier, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/legendary-weapon/delete", s.ninjaneerWeaponDesignationPopupDelete(ninjaneerLegendaryWeaponTier, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/legendary-weapon-active", s.handleSheetNinjaneerLegendaryWeaponPopupToggle)
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/perfected-weapon-mark/add", s.ninjaneerWeaponDesignationPopupAdd(ninjaneerPerfectedWeaponMarkTier, ninjaneerPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/ninjaneer/perfected-weapon-mark/delete", s.ninjaneerWeaponDesignationPopupDelete(ninjaneerPerfectedWeaponMarkTier, ninjaneerPopupPath))
	// Shinobi-Ware popup (science_nin_shinobi_ware_popup.go).
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/upgrade/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickShinobiWareUpgrade,
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.UpgradeUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.UpgradeCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ShinobiWare == nil {
				return nil
			}
			return d.ShinobiWare.AvailableUpgrades
		},
		false, shinobiWarePopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/upgrade/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickShinobiWareUpgrade, shinobiWarePopupPath))
	// In His Image's Shinjutsu Upgrade pick is permanent — no delete route,
	// matching the Core-sheet route's own identical omission above.
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/shinjutsu-upgrade/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickShinjutsuUpgrade,
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil || d.ShinobiWare.ShinjutsuUpgrade == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ShinobiWare == nil {
				return nil
			}
			return d.ShinobiWare.AvailableShinjutsuUpgrade
		},
		false, shinobiWarePopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/evolved/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickEvolvedUpgrade,
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.EvolvedUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.ShinobiWare == nil {
				return 0
			}
			return d.ShinobiWare.EvolvedCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.ShinobiWare == nil {
				return nil
			}
			return d.ShinobiWare.AvailableEvolved
		},
		false, shinobiWarePopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/evolved/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickEvolvedUpgrade, shinobiWarePopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/ever-evolving-seal/add", s.handleEverEvolvingSealPopupAdd)
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/ever-evolving-seal/delete", s.handleEverEvolvingSealPopupDelete)
	mux.HandleFunc("POST /characters/{id}/science-nin/shinobi-ware/fms-resistance/add", s.handleFullMetalShinobiResistancePopupAdd)
	// Spyware popup (science_nin_spyware_popup.go). Program's own ADD route
	// uses the dedicated handleSpywareProgramPopupAdd
	// (science_nin_subclasses.go) — see the Core-sheet route's own comment
	// above for why this catalog can't use the generic factory.
	mux.HandleFunc("POST /characters/{id}/science-nin/spyware/program/add", s.handleSpywareProgramPopupAdd)
	mux.HandleFunc("POST /characters/{id}/science-nin/spyware/program/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickSpywareProgram, spywarePopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/spyware/quick-hack/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickQuickHack,
		func(d *scienceNinToolsTabData) int {
			if d.Spyware == nil || d.Spyware.QuickHack == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.Spyware == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Spyware == nil {
				return nil
			}
			return d.Spyware.AvailableQuickHack
		},
		false, spywarePopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/spyware/quick-hack/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickQuickHack, spywarePopupPath))
	// Storm Rider popup (science_nin_storm_rider_popup.go).
	mux.HandleFunc("POST /characters/{id}/science-nin/storm-rider/enhancement/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickAirTreckEnhancement,
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			return d.StormRider.EnhancementUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			return d.StormRider.EnhancementCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.StormRider == nil {
				return nil
			}
			return d.StormRider.AvailableEnhancements
		},
		false, stormRiderPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/storm-rider/enhancement/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickAirTreckEnhancement, stormRiderPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/storm-rider/regalia/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickRegalia,
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			return d.StormRider.RegaliaUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.StormRider == nil {
				return 0
			}
			// 1 normally, 2 once Sky Keeper (20th level) is also granted —
			// see loadScienceNinSubclassData's own RegaliaCap computation.
			return d.StormRider.RegaliaCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.StormRider == nil {
				return nil
			}
			return d.StormRider.AvailableRegalia
		},
		false, stormRiderPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/storm-rider/regalia/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickRegalia, stormRiderPopupPath))
	// Technobi popup (science_nin_technobi_popup.go).
	mux.HandleFunc("POST /characters/{id}/science-nin/technobi/mechanization/add", s.scienceNinTrackerPopupAdd(charstore.ScienceNinPickTechnobiMechanization,
		func(d *scienceNinToolsTabData) int {
			if d.Technobi == nil {
				return 0
			}
			return d.Technobi.MechanizationUsed
		},
		func(d *scienceNinToolsTabData) int {
			if d.Technobi == nil {
				return 0
			}
			return d.Technobi.MechanizationCap
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.Technobi == nil {
				return nil
			}
			return d.Technobi.AvailableMechanizations
		},
		false, technobiPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/technobi/mechanization/delete", s.subclassTrackerPopupDelete(charstore.ScienceNinPickTechnobiMechanization, technobiPopupPath))
	mux.HandleFunc("POST /characters/{id}/science-nin/technobi/sent", s.handleSENTPopupPick)
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-mixed-studies", s.handleScienceNinSubclassPickAdd(charstore.ScienceNinPickMixedStudiesInquiry,
		func(d *scienceNinToolsTabData) int {
			if d.MixedStudies == nil || d.MixedStudies.Picked == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) int {
			if d.MixedStudies == nil {
				return 0
			}
			return 1
		},
		func(d *scienceNinToolsTabData) []scienceNinSubclassOption {
			if d.MixedStudies == nil {
				return nil
			}
			return d.MixedStudies.Available
		},
		false))
	mux.HandleFunc("POST /characters/{id}/sheet/science-nin-mixed-studies/delete", s.handleScienceNinSubclassPickDelete(charstore.ScienceNinPickMixedStudiesInquiry))
	mux.HandleFunc("GET /science-nin-picks/{category}/{slug...}", s.handleScienceNinSubclassPickDetail)
	mux.HandleFunc("GET /clans", s.handleClans)
	mux.HandleFunc("GET /clans/{slug...}", s.handleClanDetail)
	mux.HandleFunc("GET /jutsu", s.handleJutsuList)
	mux.HandleFunc("GET /jutsu/{slug...}", s.handleJutsuDetail)
	mux.HandleFunc("GET /classes", s.handleClasses)
	mux.HandleFunc("GET /classes/{slug...}", s.handleClassDetail)
	mux.HandleFunc("GET /feats", s.handleFeats)
	mux.HandleFunc("GET /feats/{slug...}", s.handleFeatDetail)
	mux.HandleFunc("GET /backgrounds", s.handleBackgrounds)
	mux.HandleFunc("GET /backgrounds/{slug...}", s.handleBackgroundDetail)
	mux.HandleFunc("GET /stances", s.handleStances)
	mux.HandleFunc("GET /stances/{slug...}", s.handleStanceDetail)
	mux.HandleFunc("GET /seals", s.handleSeals)
	mux.HandleFunc("GET /seals/{slug...}", s.handleSealDetail)
	mux.HandleFunc("GET /items", s.handleItems)
	mux.HandleFunc("GET /items/{slug...}", s.handleItemDetail)
	mux.HandleFunc("POST /custom-items/{id}/update", s.handleCustomItemUpdate)
	mux.HandleFunc("GET /traps", s.handleTraps)
	mux.HandleFunc("GET /traps/{slug...}", s.handleTrapDetail)
	mux.HandleFunc("GET /poisons", s.handlePoisons)
	mux.HandleFunc("GET /poisons/{slug...}", s.handlePoisonDetail)
	mux.HandleFunc("GET /properties", s.handleProperties)
	mux.HandleFunc("GET /properties/{slug...}", s.handlePropertyDetail)
	mux.HandleFunc("GET /search/index.json", s.handleSearchIndex)
	mux.HandleFunc("GET /about", s.handleAbout)
	mux.HandleFunc("GET /bugs", s.handleBugs)
	mux.HandleFunc("GET /faq", s.handleFAQ)
	mux.HandleFunc("GET /technical", s.handleTechnical)
	mux.HandleFunc("POST /heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /alive", s.handleAlive)
	mux.HandleFunc("GET /backups", s.handleBackups)
	mux.HandleFunc("POST /backups", s.handleBackupCreate)
	mux.HandleFunc("POST /backups/{filename}/restore", s.handleBackupRestore)
	mux.Handle("GET /static/", noStaleStatic(http.FileServerFS(staticFS)))

	return noBFCache(s.requireSameOrigin(s.requireToken(mux)))
}

// sortKey folds a name for alphabetical comparison: SQLite's default BINARY
// collation sorts by raw codepoint, which puts accented names (Fūshin,
// Hyūga, Shí Hóu) in the wrong place because their diacritics sit at much
// higher codepoints than plain ASCII letters — same problem with the curly
// apostrophe (’, U+2019) the PDF text uses, which sorts after every plain
// ASCII letter and shoves e.g. "Dragon's Wrath" to the end of the list
// instead of next to "Dragons ...". Decomposing to NFD, dropping combining
// marks (Mn) and punctuation, then lowercasing gives an ASCII-ish key that
// sorts the way a reader expects.
func sortKey(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) || unicode.IsPunct(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// requireSameOrigin rejects any request whose Origin header doesn't match
// this server's own origin. Requests with no Origin header (ordinary
// top-level navigation) pass through to the token check.
func (s *server) requireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.expectedOrigin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireToken accepts a request that carries the valid session cookie, or
// the raw launch token as a query parameter (on which it sets the cookie
// for subsequent requests). Anything else is rejected.
func (s *server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(tokenCookieName); err == nil && c.Value == s.token {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Query().Get("token") == s.token {
			http.SetCookie(w, &http.Cookie{
				Name:     tokenCookieName,
				Value:    s.token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, "home.html", map[string]any{"Title": "Home"})
}

type clanRow struct {
	Slug          string
	Name          string
	Epithet       string
	SpeedFeet     sql.NullInt64
	ExtraLanguage sql.NullString
}

func (s *server) handleClans(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, epithet, speed_feet, extra_language FROM v_clans`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query clans:", err)
		return
	}
	defer rows.Close()

	var clans []clanRow
	for rows.Next() {
		var c clanRow
		if err := rows.Scan(&c.Slug, &c.Name, &c.Epithet, &c.SpeedFeet, &c.ExtraLanguage); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan clan:", err)
			return
		}
		clans = append(clans, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("clans rows:", err)
		return
	}
	sort.Slice(clans, func(i, j int) bool { return sortKey(clans[i].Name) < sortKey(clans[j].Name) })

	s.render(w, "clans.html", map[string]any{"Title": "Clans", "Clans": clans})
}

type clanTrait struct {
	Name        string
	Description string
}

type clanFeature struct {
	Name        string
	Level       sql.NullInt64
	Description string
	// Locked is populated only by handleCharacterClanReference (clan_reference.go),
	// against a specific character's level — always false for the standalone
	// /clans/{slug} page and the creation-flow preview, which show a clan's
	// complete catalog with nothing to gate against.
	Locked bool
	// ScalesThrough is the highest ordinal level mentioned within
	// Description itself (see scalinglevel.go), if higher than Level — many
	// clan features are gained at one level but keep escalating in their
	// own text at later ones. 0 if the feature doesn't scale past its own
	// gate level. Computed for every caller of loadClanDetail, not just the
	// character-specific one, since it's a pure function of the text.
	ScalesThrough int
}

type clanJutsuEntry struct {
	Slug        string
	Name        string
	Rank        sql.NullString
	Description string
}

type profGroup struct {
	Kind   string // "Skills", "Tools", "Languages", "Weapons", "Armor"
	Values []string
}

type clanLatent struct {
	Name        string
	Stage       string
	Description string
}

type clanDetail struct {
	Slug                string
	Name                string
	Epithet             string
	Overview            string
	SpeedFeet           sql.NullInt64
	ExtraLanguage       sql.NullString
	AbilityIncreaseText sql.NullString
	Traits              []clanTrait
	Features            []clanFeature
	Jutsu               []clanJutsuEntry
	Proficiencies       []profGroup
	Latents             []clanLatent
}

var profKindLabels = []struct{ kind, label string }{
	{"skill", "Skills"},
	{"tool", "Tools"},
	{"language", "Languages"},
	{"weapon", "Weapons"},
	{"armor", "Armor"},
}

// loadClanDetail is shared by the standalone /clans/{slug} page, the
// character-creation Clan step's two-pane preview, and the AJAX fragment
// swap (see handleClanDetail) — one query set reused three ways, same
// pattern as jutsu.go's loadJutsuDetail.
func loadClanDetail(rulesDB *sql.DB, slug string) (*clanDetail, error) {
	var c clanDetail
	err := rulesDB.QueryRow(`
		SELECT slug, name, epithet, overview, speed_feet, extra_language, ability_increase_text
		FROM v_clans WHERE slug = ?`, slug,
	).Scan(&c.Slug, &c.Name, &c.Epithet, &c.Overview, &c.SpeedFeet, &c.ExtraLanguage, &c.AbilityIncreaseText)
	if err != nil {
		return nil, err
	}

	traitRows, err := rulesDB.Query(`
		SELECT name, description FROM clan_traits WHERE clan_slug = ? ORDER BY sort_order`, slug)
	if err != nil {
		return nil, err
	}
	for traitRows.Next() {
		var t clanTrait
		if err := traitRows.Scan(&t.Name, &t.Description); err != nil {
			traitRows.Close()
			return nil, err
		}
		c.Traits = append(c.Traits, t)
	}
	traitRows.Close()
	if err := traitRows.Err(); err != nil {
		return nil, err
	}

	featureRows, err := rulesDB.Query(`
		SELECT name, level, description FROM v_clan_features WHERE clan_slug = ? ORDER BY sort_order`, slug)
	if err != nil {
		return nil, err
	}
	for featureRows.Next() {
		var f clanFeature
		if err := featureRows.Scan(&f.Name, &f.Level, &f.Description); err != nil {
			featureRows.Close()
			return nil, err
		}
		if scales := highestScalingLevel(f.Description); scales > int(f.Level.Int64) {
			f.ScalesThrough = scales
		}
		c.Features = append(c.Features, f)
	}
	featureRows.Close()
	if err := featureRows.Err(); err != nil {
		return nil, err
	}

	// No ORDER BY here: the join's natural row order doesn't reflect the
	// sourcebook's table layout (unlike clan_traits/clan_features, which
	// carry an explicit sort_order), so there's no document order worth
	// preserving — the Go-side sort.Slice below imposes a real alphabetical
	// order instead. Joins against v_jutsu (not the raw jutsu table) so
	// rank reflects any override, matching the rest of the app's convention
	// of never reading override-bearing tables directly.
	jutsuRows, err := rulesDB.Query(`
		SELECT j.slug, j.name, j.rank, j.description
		FROM clan_jutsu cj JOIN v_jutsu j ON j.slug = cj.jutsu_slug
		WHERE cj.clan_slug = ?`, slug)
	if err != nil {
		return nil, err
	}
	for jutsuRows.Next() {
		var j clanJutsuEntry
		if err := jutsuRows.Scan(&j.Slug, &j.Name, &j.Rank, &j.Description); err != nil {
			jutsuRows.Close()
			return nil, err
		}
		c.Jutsu = append(c.Jutsu, j)
	}
	jutsuRows.Close()
	if err := jutsuRows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(c.Jutsu, func(i, j int) bool { return sortKey(c.Jutsu[i].Name) < sortKey(c.Jutsu[j].Name) })

	profRows, err := rulesDB.Query(`
		SELECT kind, value FROM clan_proficiencies WHERE clan_slug = ?`, slug)
	if err != nil {
		return nil, err
	}
	byKind := map[string][]string{}
	for profRows.Next() {
		var kind, value string
		if err := profRows.Scan(&kind, &value); err != nil {
			profRows.Close()
			return nil, err
		}
		byKind[kind] = append(byKind[kind], value)
	}
	profRows.Close()
	if err := profRows.Err(); err != nil {
		return nil, err
	}
	for _, k := range profKindLabels {
		values := byKind[k.kind]
		if len(values) == 0 {
			continue
		}
		sort.Strings(values)
		c.Proficiencies = append(c.Proficiencies, profGroup{Kind: k.label, Values: values})
	}

	// ORDER BY rowid, not name: bloodline_latents has no explicit sort_order
	// column, but rowid reflects insertion order, which in turn reflects the
	// order latents were parsed off the clan's sourcebook page — named
	// latents grouped together (Dragon Claws I, II, III, ...) followed by
	// the rank-gate unlocks, exactly as printed. Alphabetizing destroys that
	// grouping (it was interleaving "A-Rank Hijutsu" between unrelated named
	// latents purely because of the letter A).
	latentRows, err := rulesDB.Query(`
		SELECT name, stage, description FROM bloodline_latents WHERE clan_slug = ? ORDER BY rowid`, slug)
	if err != nil {
		return nil, err
	}
	for latentRows.Next() {
		var l clanLatent
		if err := latentRows.Scan(&l.Name, &l.Stage, &l.Description); err != nil {
			latentRows.Close()
			return nil, err
		}
		c.Latents = append(c.Latents, l)
	}
	latentRows.Close()
	if err := latentRows.Err(); err != nil {
		return nil, err
	}

	return &c, nil
}

// handleClanDetail serves the standalone /clans/{slug} page and, with
// ?fragment=1, just the inner detail card for the two-pane view's AJAX
// swap — same content-negotiation-by-query-param pattern as jutsu.go's
// handleJutsuDetail.
func (s *server) handleClanDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	c, err := loadClanDetail(s.rulesDB, slug)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query clan detail:", err)
		return
	}

	if r.URL.Query().Get("fragment") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["clan_detail.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render clan fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "clan_detail_card", c); err != nil {
			log.Println("render clan fragment:", err)
		}
		return
	}

	s.render(w, "clan_detail.html", map[string]any{"Title": c.Name, "Clan": c})
}

// handleHeartbeat records that a page is still open — see static/js/
// heartbeat.js, included on every page, which pings this every 2 seconds.
func (s *server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	s.heartbeatMu.Lock()
	s.lastHeartbeat = time.Now()
	s.heartbeatMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// handleAlive is the primary "a tab is still open" signal: an SSE stream the
// browser opens on every page (see alive.js) and holds until the page goes
// away. The handler writes one comment, flushes, and then does nothing but
// block until the request context is cancelled.
//
// A held-open TCP connection answers the question that a timer-based ping
// fundamentally cannot. Closing the tab (or navigating away) makes the
// browser close the socket immediately and unconditionally, so the server
// learns about it in milliseconds, with no timers involved on either side.
// Suspending the machine, hibernating it, or letting the browser freeze the
// tab all leave the socket OPEN — which is exactly right, because the tab is
// still open in every one of those cases. That is the part-6 bug: pings stop
// during sleep, but a connection does not, so the server no longer confuses
// "this computer is asleep" with "this tab is gone".
//
// It deliberately sends no events. The client never needs to hear from the
// server; the connection's existence is the entire payload. The periodic
// comment is only there so a silently dropped socket surfaces as a write
// error rather than as a connection the server holds forever.
func (s *server) handleAlive(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Chrome buffers a streaming response until enough bytes arrive unless
	// told otherwise; the comment line below is small, so flush explicitly.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	s.heartbeatMu.Lock()
	s.aliveOpen++
	s.aliveSeen = true
	s.heartbeatMu.Unlock()
	defer func() {
		s.heartbeatMu.Lock()
		s.aliveOpen--
		s.lastAlive = time.Now()
		s.heartbeatMu.Unlock()
	}()

	ticker := time.NewTicker(aliveKeepalive)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// watchHeartbeat is the app's only quit mechanism (no Quit button — closing
// the browser tab is the whole UX by design). It can't
// be a simple page unload/pagehide listener: this is a plain multi-page
// site, not an SPA, so EVERY internal link click unloads the current page
// exactly the same way closing the tab does — a naive "shut down on
// unload" would kill the server the instant anyone clicked any nav link.
//
// Two signals, checked in order of trustworthiness:
//
//  1. Open /alive streams (handleAlive). While at least one is connected, a
//     tab is definitively open and nothing else matters. Once the last one
//     disconnects, aliveGrace — seconds, not minutes — covers the gap while
//     an ordinary internal navigation opens the next page's stream, and then
//     the server shuts down. This is both the fast path for a real tab close
//     and the reason sleeping the machine no longer kills the server: a
//     suspended machine's sockets stay open, so this signal simply keeps
//     saying "still here".
//
//  2. /heartbeat pings (heartbeat.js), on the long heartbeatTimeout budget,
//     used only while no /alive stream has ever connected. If EventSource is
//     unavailable or blocked, behaviour degrades to exactly what it was
//     before rather than to a server that never exits.
//
// Runs until it decides to shut down (then returns) or the process exits
// some other way (Ctrl+C in main.go) — it does not need its own stop
// signal, since the whole program exits shortly after either path.
func (s *server) watchHeartbeat() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.heartbeatMu.Lock()
		open, seen, lastAlive, lastPing := s.aliveOpen, s.aliveSeen, s.lastAlive, s.lastHeartbeat
		s.heartbeatMu.Unlock()

		if open > 0 {
			continue // a tab is holding a connection open right now
		}
		if seen {
			// lastAlive is only zero if aliveSeen was set by a stream that
			// is somehow still counted open, which the check above already
			// covered; guard anyway rather than shut down on a zero time.
			if !lastAlive.IsZero() && time.Since(lastAlive) > aliveGrace {
				s.shutdownOnce.Do(func() { close(s.shutdown) })
				return
			}
			continue
		}
		if lastPing.IsZero() {
			continue // no ping yet — browser hasn't opened/loaded the page
		}
		if time.Since(lastPing) > heartbeatTimeout {
			s.shutdownOnce.Do(func() { close(s.shutdown) })
			return
		}
	}
}

// assetVersion is a cache-busting token appended to every stylesheet and
// script URL in the layout (see the "assetVersion" template func and
// layout.html).
//
// Files inside an embed.FS all report a zero modification time, so
// http.FileServerFS cannot send a Last-Modified header for them and never
// answers 304. With nothing to validate against, browsers fall back to
// heuristic caching and can keep serving an old app.css or sheet-*.js for
// a long time — which looks exactly like a fix that never shipped. Rebuilt
// binary, same-looking bug. Changing the URL on every run sidesteps the
// question entirely.
//
// It is the process start time rather than a content hash because this
// app is rebuilt and relaunched constantly during development, which is
// precisely when stale assets bite; a hash would be tidier but would have
// to be computed over the whole embedded tree at startup for no practical
// gain here.
var assetVersion = strconv.FormatInt(time.Now().UnixMilli(), 36)

// noBFCache stops the browser from ever serving a page here out of its
// back/forward cache. Every page this app renders reflects live
// characters.db state, and bfcache's whole point is skipping the server
// round-trip a normal navigation would make — which is exactly the
// round-trip that would notice something changed. Reproduced 2026-08-01:
// add a class from the creation flow's Class step (redirects to the
// checklist), then press the browser's Back button — Chrome restored the
// pre-add snapshot of the Class step from bfcache instead of re-fetching
// it, so "Your Classes" silently omitted the class that had, in fact, just
// been added (confirmed still present in characters.db the whole time).
// `Cache-Control: no-store` is what Chrome's own bfcache eligibility check
// treats as a hard exclusion, not just a caching hint — applied to every
// route (wraps the whole mux, outermost) rather than to `render`/
// `renderCard` alone, since several fragment handlers
// (classes.go/items.go/jutsu.go/core.go/characters.go) write their HTML
// response directly and would otherwise need the same header remembered
// individually, forever, on every future one too.
func noBFCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// noStaleStatic tells the browser to revalidate embedded assets instead of
// trusting its own heuristics. Belt and braces with assetVersion: the
// version query string handles the normal case, and this covers anything
// that reaches /static/ without one.
func noStaleStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// wantsFragment reports whether a request came from one of the sheet's own
// fetch() calls, which send X-Requested-With: fetch, rather than from a
// plain browser form submission.
//
// Every mutating sheet endpoint answers with an HTML fragment so the page
// can swap one block in place without reloading. When JavaScript doesn't
// run — disabled, still loading, or broken — the same form submits
// natively, and answering that with a fragment dumps the player onto a
// bare, unstyled scrap of markup with no way back and no visible sign the
// change was even saved. Checking for the header lets those requests get
// an ordinary redirect back to the sheet instead, so the feature degrades
// to a normal page load rather than to a dead end.
func wantsFragment(r *http.Request) bool {
	return r.Header.Get("X-Requested-With") == "fetch"
}

func (s *server) render(w http.ResponseWriter, page string, data any) {
	tmpl, ok := pageTemplates[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render: no template registered for", page)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Println("render", page, ":", err)
	}
}

// renderCard writes one detail-card partial with no layout around it — the
// reply to a ?fragment=1 request, which the browse pages swap into their
// right-hand pane and the character sheet opens in its popup dialog.
//
// page names any template set the card partial is parsed into; every set
// includes every partial, so a list page's own name is the conventional
// choice.
func (s *server) renderCard(w http.ResponseWriter, page, card string, data any) {
	tmpl, ok := pageTemplates[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("renderCard: no template registered for", page)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, card, data); err != nil {
		log.Println("renderCard", card, ":", err)
	}
}
