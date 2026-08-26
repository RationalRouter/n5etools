package main

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// subclass_tracker_popup.go: the "subclass tracker popup" pattern — a real,
// separate window.open() popup (reference-popup.js, [data-reference-popup],
// the same mechanism character_reference.html/character_clan_reference.html/
// character_custom_features.html already use) dedicated to ONE subclass's
// own cap+catalog pick tracker, pulled out of the Core sheet/Companions tab
// so it (a) reads in this project's blue "subclass-specific" convention
// (.subclass-scope, app.css's own header doc) instead of blending into the
// general-content gold, and (b) is reachable from every tab — the sidebar
// button lives in .sheet-sidebar, outside every tab panel, the same as
// Class/Clan Reference and Custom Features already are — rather than only
// from whichever one tab happened to host the tracker inline.
//
// First two users: character_snb_upgrades.html (S.N.B Specialist's own
// S.N.B Upgrades + Future of Shinobi: S.N.B, formerly inline in
// sheet_science_nin — see snb_upgrades_popup.go) and character_titan_slots.
// html (Mech Crafter's own Titan Slots + Exo-Suit + Specialist Crafting,
// formerly inline in titan_reference/companion_fields.html — see
// titan_slots_popup.go).
//
// Building a THIRD subclass's own popup means, per this same recipe:
//  1. A GET handler that loads that subclass's own data and reduces it to
//     []subclassTrackerSection (see snbUpgradesSection/titanSlotsSection
//     for the shape to copy), then calls renderSubclassTrackerPopup.
//  2. A dedicated POST handler per add-action, extracting its core
//     validate+mutate logic into its own small method (addSNBUpgradePick/
//     addTitanSlotPick are the models) so the Core-sheet/Companions-tab
//     AJAX route and the new popup route share one validation path and
//     cannot drift apart — budget/cap checks differ too much per catalog
//     to generalize into one factory the way the flat-slot-only catalogs
//     already share handleScienceNinSubclassPickAdd.
//  3. subclassTrackerPopupDelete (below) for each delete-action — this
//     part IS fully generic, since removing a pick is always just
//     RemoveScienceNinSubclassPick regardless of catalog.
//  4. A new template reusing partials/subclass_tracker_popup.html's shared
//     subclass_tracker_popup_header/subclass_tracker_section blocks for
//     everything that fits the generic Known/Available shape, plus
//     whatever bespoke markup that subclass's tracker needs beyond it
//     (character_titan_slots.html's own Specialist Crafting <select> is
//     the existing example — it isn't a catalog picker at all, so it isn't
//     part of the shared partial).
//  5. A templates.go registration (layout_bare.html + partials/*.html,
//     same as this pattern's other four users).
//  6. A sidebar button in character_sheet.html, gated on that subclass
//     actually being chosen (see .ScienceNin.SNBSpecialist / .
//     ShowTitanSlotsPopup for the two existing examples of that gate).
//
// Every mutation route in this pattern is a plain POST-and-redirect back
// to the popup's own GET URL — NOT the main sheet's fetch-and-swap
// (sheet-fetch-form/data-target) convention — matching character_custom_
// features.html's own established convention for popup-scoped forms (see
// that file's header doc). This is a deliberate downgrade from the
// Core-sheet's live AJAX swap: a real separate window.open() window has no
// DOM access back into the opener without new cross-window plumbing this
// codebase doesn't have, so every add/remove here reloads the popup window
// itself instead of updating in place. The Core-sheet/Companions-tab
// routes these mutations used to go through exclusively
// (handleScienceNinSNBUpgradeAdd, handleTitanUpgradeAdd, ...) are left
// completely in place, unrenamed and unredirected — they still back the
// shared Creation Points budget's own cross-check tests
// (science_nin_creation_points_test.go), and nothing about this pattern
// repoints them; the popup routes here are new, separate routes reusing
// the same underlying charstore mutations.

// subclassTrackerOption is one radio-pickable "available to add" catalog
// entry, reduced to the generic display shape every subclass tracker
// popup's picker form needs. Epithet is pre-formatted by the caller (e.g.
// "Rare · Cost 5" or "Rare · MECH · Cost 5 · Drain 1") so partials/
// subclass_tracker_popup.html never needs to know which subclass's own
// catalog fields (Tier/Cost/Keyword/Drain/...) it came from.
type subclassTrackerOption struct {
	Slug        string
	Name        string
	Epithet     string
	Description string
	// DescriptionHTML, when set, is shown in the tooltip instead of running
	// Description through the template's own formatDescription call — the
	// escape hatch a catalog needs when its raw text wants different
	// formatting than every other subclass tracker's plain-prose fields
	// (W.O.W's own base/Ascension split — see formatWoWDescription — is the
	// first and, so far, only user).
	DescriptionHTML template.HTML
	// Disabled marks an option the character cannot currently afford —
	// evaluated by whichever section builder populates a Creation-Points-
	// spending catalog's own Available list (everEvolvingSection,
	// snbUpgradesSection, spywareProgramSection, eipSection,
	// titanSlotsSection, titanExoSuitSection — the six catalogs that
	// actually draw from the shared Creation Points pool; see
	// science_nin_subclasses.go's own header doc on why every OTHER
	// Science-Nin catalog in that file stays Cost-informational-only) as
	// CreationPointsUsed + this option's own effective cost exceeding
	// CreationPointsCap (subclassTrackerPopupData's own two fields, already
	// in scope wherever a section builder runs). Deliberately named so its
	// zero value (false) means "pickable": a catalog with no Cost concept
	// at all (Hunter Creed traps, Scout-Nin maneuvers, every flat-slot-only
	// Puppet-style pick this pattern also serves) never touches this field
	// and stays fully selectable exactly as before it existed. partials/
	// subclass_tracker_popup.html's own subclass_tracker_section renders
	// this as a disabled radio input plus a dimming
	// .puppet-upgrade-option-unaffordable wrapper (app.css) — a UI
	// convenience only, the same "a disabled control isn't the real
	// enforcement" boundary character_sheet.html's own Puppet Upgrade
	// picker already draws for its PrereqMet/AlreadyTaken gate (see that
	// template's own comment). Each add*Pick validator this option
	// ultimately POSTs to (addEverEvolvingSealPick, addSNBUpgradePick,
	// addSpywareProgramPick, addEIPPick, addTitanSlotPick,
	// addTitanExoSuitPick) re-runs the identical budget check server-side
	// and stays the actual enforcement — Disabled only prevents a normal
	// player from submitting a pick their own sheet already shows they
	// can't afford, it doesn't (and isn't meant to) stop a hand-crafted
	// POST.
	Disabled bool
}

// subclassTrackerKnownPick is one already-installed pick's display row.
// DetailHref "" renders the name as plain text instead of a
// sheet-popup-trigger link — every current catalog has a static detail
// route, but the field stays optional for a future one that might not.
type subclassTrackerKnownPick struct {
	Slug       string
	Name       string
	Epithet    string
	DetailHref string
}

// subclassTrackerSection is one titled Known/Available block inside a
// subclass tracker popup — see partials/subclass_tracker_popup.html's own
// "subclass_tracker_section" template for how this renders. AddAction ""
// hides the picker form entirely (the section is full, or has nothing left
// to offer), matching the {{if lt Used Cap}} gate every inline Core-sheet
// picker this pattern replaces already applied.
type subclassTrackerSection struct {
	Title        string
	Hint         string
	Known        []subclassTrackerKnownPick
	DeleteAction string
	AddAction    string
	AddLabel     string
	Available    []subclassTrackerOption
}

// subclassTrackerPopupData is the full page-level data every subclass
// tracker popup's own GET handler builds and hands to
// renderSubclassTrackerPopup. ShowCreationPoints/CreationPointsUsed/
// CreationPointsCap render the same shared science_nin_creation_points_
// tile partial the Core-tab Scientific Ninja Tools box shows — every
// current user of this pattern spends from that same pool, but a future
// non-Science-Nin subclass tracker (if this pattern is ever reused outside
// Science-Nin) would simply leave ShowCreationPoints false.
type subclassTrackerPopupData struct {
	Title              string
	CharacterID        int64
	CharacterName      string
	ShowCreationPoints bool
	CreationPointsUsed int
	CreationPointsCap  int
	Sections           []subclassTrackerSection
	// EmptyHint shows in place of Sections when the character doesn't (or
	// no longer) actually have this subclass — reachable only by visiting
	// the popup's URL directly after a respec, since the sidebar button
	// that links here is itself gated on the same condition.
	EmptyHint string
	// RefreshOpenerBlocks: space-separated character_sheet.html element ids
	// (the same id list a sheet-fetch-form's own data-also-refresh already
	// takes — see sheet-refresh.js) to refresh on the OPENER window once
	// this popup page loads. Every mutation in this pattern is a plain
	// POST-and-redirect back to this same GET URL (this file's own header
	// doc), so "this page just (re)loaded" already covers "a pick
	// was just added or removed" — nothing further needs to trigger this
	// beyond the page load itself. Sent to subclass-tracker-popup.js via a
	// data attribute on subclass_tracker_popup_header's own <h1>
	// (partials/subclass_tracker_popup.html), which calls the opener's own
	// window.opener.n5eRefreshBlocks — the exact mechanism companion-fields.
	// js's notifyOpenerToRefresh already uses for a companion popup's own
	// field saves, just triggered by a page load here instead of a fetch,
	// since this pattern has no fetch of its own to hook (a real popup
	// window has no other way back into the opener's DOM — see
	// subclass_tracker_popup.go's own header doc on why every mutation here
	// is redirect-based rather than fetch-and-swap).
	//
	// Set by each popup's own GET handler to whatever Core-sheet fragment(s)
	// that subclass's picks are actually rendered inside — normally the
	// single sheet_X fragment its own (pre-existing) Core-sheet/Companions-
	// tab AJAX route already responds with (e.g. "sheet-science-nin" for
	// most Science-Nin subclasses), but wider when a pick's effect is
	// ALSO visible somewhere else: Titan Slots/SNB Upgrades additionally
	// need "sheet-summon-tab" (the companion card showing Known Upgrades),
	// W.o.W/Ascended-W.o.W additionally need "sheet-weapon-attacks
	// sheet-inventory sheet-inventory-full" (the granted weapon itself —
	// see wow_weapons.go), and Hunter Creed's Warden Weapon additionally
	// needs "sheet-weapon-attacks" (hunterNinWardenWeaponBonuses feeds
	// buildAttacks directly, characters.go). Refreshing an unaffected block
	// is harmless — n5eRefreshBlocks already no-ops on any id not present
	// on the current tab — so erring wide here costs nothing.
	RefreshOpenerBlocks string
}

// renderSubclassTrackerPopup renders any subclass tracker popup page — the
// shared page-level chrome (back link, <h1>, optional Creation Points
// tile) stays visually identical no matter which subclass's own tracker is
// being shown. page is the specific template name (e.g.
// "character_snb_upgrades.html"). extra merges in any additional top-level
// template fields a specific popup needs beyond the generic
// subclassTrackerPopupData shape (character_titan_slots.html's own
// Specialist Crafting <select> is the existing example — it isn't a
// catalog picker, so it isn't part of subclassTrackerSection at all); nil
// for a popup that needs nothing beyond the shared shape.
func (s *server) renderSubclassTrackerPopup(w http.ResponseWriter, page string, data subclassTrackerPopupData, extra map[string]any) {
	fields := map[string]any{
		"Title": data.CharacterName + " — " + data.Title,
		"Data":  data,
	}
	for k, v := range extra {
		fields[k] = v
	}
	s.render(w, page, fields)
}

// subclassTrackerPopupDelete builds a "forget a pick" POST route for a
// subclass tracker popup — the same underlying RemoveScienceNinSubclassPick
// mutation handleScienceNinSubclassPickDelete/handleTitanPickDelete already
// use for their own Core-sheet/Companions-tab routes, redirecting back to
// the popup's own URL afterward instead of swapping a sheet fragment (see
// this file's header doc on why). Fully generic — unlike the add side,
// removing a pick never needs a budget/cap check, so one factory covers
// every category this pattern will ever need.
func (s *server) subclassTrackerPopupDelete(category charstore.ScienceNinSubclassPickCategory, popupPath func(id int64) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove subclass tracker pick:", err)
			return
		}
		if category == charstore.ScienceNinPickAscendedWoW {
			logWhelpSyncErr(s.syncDraconicGauntletWhelp(id))
		}
		http.Redirect(w, r, popupPath(id), http.StatusSeeOther)
	}
}
