// Shared companion field-autosave logic — the core of what used to be
// companion-sheet.js's whole file, factored out so both the companion
// popup (companion_sheet.html, one companion per page) and the main
// sheet's Puppets/Summons tabs (character_sheet.html, several companions
// per page, inside swappable sheet-fetch-form fragments) drive their
// fields identically instead of maintaining two copies of the same "+3/-2
// adjusts, bare number sets, blank clears" HP logic.
//
// Every listener here is delegated from document, not bound to individual
// elements — this is what makes it safe to reuse on the main sheet, where
// a companion card can be replaced wholesale by a fragment swap (adding/
// removing a puppet, adding/removing an upgrade) at any time: a
// document-level listener keeps matching new elements automatically, with
// no rewire pass needed the way a directly-bound listener would.
(function () {
  function postForm(form) {
    return fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: new URLSearchParams(new FormData(form)).toString(),
    });
  }
  window.n5eCompanionPostForm = postForm;

  // Only truthy inside the companion popup (window.open'd from the main
  // sheet's Core tab) — the main sheet itself has no window.opener. A save
  // made in the popup has no other way to reach the main sheet's own
  // Puppets/Summons tabs, since they live in a completely separate browser
  // window/tab with their own already-rendered DOM; this is the one place
  // that gap gets closed. Guarded in a try/catch since window.opener can be
  // closed, cross-origin, or have navigated away by the time this fires —
  // none of those are failures worth surfacing, the popup's own save has
  // already succeeded either way.
  function notifyOpenerToRefresh() {
    try {
      if (window.opener && !window.opener.closed && window.opener.n5eRefreshBlocks) {
        window.opener.n5eRefreshBlocks("sheet-puppet-tab sheet-summon-tab");
      }
    } catch (err) {
      console.warn("could not refresh the main sheet tab:", err);
    }
  }

  // A companion popup (window.open'd from the main sheet, see above) loads
  // dice-roller.js and character-sheet.js — its structured Attacks/Puppet
  // Skills rows are .rollable — but not sheet-chat.js, and it has no
  // .sheet-chat-panel of its own: it is a separate document, so a roll made
  // there had nowhere to go, silently never reaching the dice log the
  // player actually has open on the main sheet. Forwarded here the same way
  // a field save already is: window.opener is only truthy inside the popup,
  // so this is a no-op on the main sheet itself, where sheet-chat.js's own
  // document-level "n5e:roll-result" listener already handles the roll
  // directly.
  document.addEventListener("n5e:roll-result", (e) => {
    try {
      if (window.opener && !window.opener.closed && window.opener.n5eLogRoll) {
        window.opener.n5eLogRoll(e.detail);
      }
    } catch (err) {
      console.warn("could not log this roll to the main sheet's dice log:", err);
    }
  });

  // The other direction: a save made on the MAIN sheet's own Puppets/
  // Summons tab for a companion whose popup is still open elsewhere. Only
  // defined there (companion-popup.js, which the popup page itself never
  // loads — there is nothing for the popup to open a popup of), so this is
  // a no-op inside the popup itself. window.n5eReloadCompanionPopupIfOpen
  // looks up any tracked handle by companion id and reloads it — a real
  // reload, not a fragment refresh, matching the popup's own existing
  // reload-on-tribe-change convention (small, mostly-static page).
  function notifyPopupToReload(form) {
    if (!window.n5eReloadCompanionPopupIfOpen) return;
    const m = /\/companions\/(\d+)/.exec(form.action);
    if (m) window.n5eReloadCompanionPopupIfOpen(m[1]);
  }

  // Every field except HP-current is a .companion-field associated with
  // its companion's form via the HTML `form="..."` attribute (not DOM
  // nesting — see templates/partials/companion_fields.html's own header
  // comment for why), and blurring any one of them resubmits the form's
  // ENTIRE field set, the same "whole field, on blur" shape sheet-bio.js
  // uses for the main sheet's Bio/Notes fields.
  document.addEventListener("focusout", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || !field.classList.contains("companion-field")) return;
    if (!field.form) return;
    postForm(field.form).then((r) => {
      if (!r.ok) { console.warn("companion autosave rejected:", r.status); return; }
      notifyOpenerToRefresh();
      notifyPopupToReload(field.form);
    }).catch((err) => {
      console.warn("companion autosave failed:", err);
    });
  });

  // HP-current, AC, and Max HP are each deliberately NOT a .companion-field
  // — every one has its own tiny <form> and its own endpoint
  // (handleCompanionHP/handleCompanionIntField), because the typed text is
  // not the value to store: a leading "+"/"-" is a delta, a bare number
  // sets outright, blank clears, exactly like the main sheet's HP/Ryo
  // boxes. The response is the new resolved value, written back into the
  // input here — if a typed "+3" stayed displayed after being applied, a
  // second focusout on the same field would resubmit "+3" again and
  // silently double the delta. AC/Max HP replaced the old "Use computed"
  // hint button with this same delta-editable shape: a fresh (or
  // backfilled) puppet's fields already start at the computed baseline, so
  // typing "+1" after an upgrade is enough — no separate button needed.
  document.addEventListener("focusout", (e) => {
    const field = e.target;
    if (!(field instanceof Element)) return;
    if (!field.classList.contains("companion-hp-field") && !field.classList.contains("companion-delta-field")) return;
    if (!field.form) return;
    postForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        return r.text();
      })
      .then((text) => { field.value = text; notifyOpenerToRefresh(); notifyPopupToReload(field.form); })
      .catch((err) => console.warn("companion delta-field autosave failed:", err));
  });

  // Every companion field lives in a real <form> (HP-current/AC/Max HP each
  // have their own tiny one; everything else shares FormID via the `form=`
  // attribute) posting to a real URL — so pressing Enter while typing (the
  // natural way to confirm a value, not just tabbing/clicking away) submits
  // that form for real instead of firing a "focusout". With no submit
  // handler to stop it, the browser actually navigates to the endpoint URL,
  // which answers with nothing but the bare new value as plain text — the
  // player lands on a blank page showing e.g. "17", with no way back short
  // of relaunching the app, even though the save itself went through fine.
  // Stopping the real submission and blurring the focused field instead
  // reuses the two focusout handlers above, so Enter behaves exactly like
  // clicking away — the same fix shape as sheet-vitals.js's own edit boxes,
  // which handle Enter explicitly for the same reason.
  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (!/\/companions\/\d+(\/(ac|hp|hp_max|jutsu_slots_current|jutsu_slots_max|matryoshka_jutsu_slots|barrier_current|barrier_max))?$/.test(form.action)) return;
    e.preventDefault();
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  });

  // Explicit-commit selects (.companion-commit-select — Nin-Dog Breed today,
  // any future locked-once-chosen picker later) are deliberately NOT a
  // .companion-field: merely selecting (or, worse, just blurring) one of
  // these must never itself commit the pick, since the value locks
  // permanently server-side the instant it's saved non-empty (see
  // charstore.SetCompanionFields' own CASE WHEN guard). Its own sibling
  // .companion-commit-btn (inside the same .companion-block-label) is the
  // only thing that actually saves it — this listener just keeps that
  // button's disabled state in sync with whether a real option is
  // currently highlighted, so "click Set" is only ever actionable once
  // there is something to set. The commit itself (posting the form, then
  // reloading the popup or refreshing the tab fragment) is wired per-view
  // (companion-sheet.js, sheet-puppets.js) since those two need different
  // post-commit refresh behavior — the same split the breed/tribe picker's
  // own "change" listener already used before this fix.
  document.addEventListener("change", (e) => {
    const select = e.target;
    if (!(select instanceof Element) || !select.classList.contains("companion-commit-select")) return;
    const label = select.closest(".companion-block-label");
    const btn = label && label.querySelector(".companion-commit-btn");
    if (btn) btn.disabled = select.value === "";
  });

  // Picking a new Armor Chassis only PREVIEWS its AC in the AC field, the
  // same "highlighting an option must never itself commit anything" rule
  // .companion-commit-select enforces for the select itself now (armor_chassis
  // used to be a plain .companion-field, which — combined with this same
  // listener unconditionally dispatching "focusout" on both the select AND
  // the AC field — saved BOTH the chassis pick and its derived AC the
  // instant an option was merely highlighted, with no confirmation; see
  // companion_fields.html's own doc comment on this select). The actual
  // save of both fields is deferred to the "Set Armor Chassis" button click
  // (window.n5eCommitArmorChassisAC below, called from the per-view commit
  // handlers in companion-sheet.js/sheet-puppets.js after the chassis pick
  // itself lands), which reuses this same computation. Reads the puppet's
  // currently-DISPLAYED Dex score (not a fresh server round trip) so an
  // unsaved Dex edit still factors in, computing 10 + chassis AC bonus +
  // Dex modifier capped per the chassis's own dex_bonus_mode (mirrors
  // puppetArmorChassisAC server-side exactly).
  function computeArmorChassisAC(select) {
    const opt = select.selectedOptions[0];
    if (!opt || !opt.value || !opt.dataset.acBonus) return null; // "— none —" has neither
    const container = select.closest(".companion-card, .companion-sheet");
    const dexField = container && container.querySelector('[name="dex_score"]');
    const acField = container && container.querySelector('.companion-delta-field[data-field="ac"]');
    if (!dexField || !acField) return null;
    const dexScore = parseInt(dexField.value, 10);
    let dexMod = Number.isFinite(dexScore) ? Math.floor((dexScore - 10) / 2) : 0;
    if (opt.dataset.dexMode === "max2" && dexMod > 2) dexMod = 2;
    else if (opt.dataset.dexMode === "none") dexMod = 0;
    return { acField, value: String(10 + parseInt(opt.dataset.acBonus, 10) + dexMod) };
  }

  document.addEventListener("change", (e) => {
    const select = e.target;
    if (!(select instanceof Element) || select.name !== "armor_chassis") return;
    const result = computeArmorChassisAC(select);
    if (result) result.acField.value = result.value;
  });

  // Called by the "Set Armor Chassis" commit-button handlers (companion-
  // sheet.js, sheet-puppets.js) right after the chassis pick itself is
  // successfully posted — recomputes and actually saves the derived AC,
  // same formula as the live preview above, so AC and the newly-committed
  // chassis land together instead of AC only updating on some later,
  // unrelated edit. Returns a Promise (resolving even when there's nothing
  // to save) so callers can refresh their own fragment only once this save
  // has actually landed, rather than racing it.
  window.n5eCommitArmorChassisAC = function (select) {
    const result = computeArmorChassisAC(select);
    if (!result || !result.acField.form) return Promise.resolve();
    result.acField.value = result.value;
    return postForm(result.acField.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        return r.text();
      })
      .then((text) => { result.acField.value = text; })
      .catch((err) => console.warn("armor chassis AC save failed:", err));
  };

  // The Puppet Upgrade "Sync AC to N"/"Sync Max HP to N"/"Sync Speed to
  // N"/"Sync Fly to N" buttons — a plain <button type="button">, not a
  // <form> posting to the same /ac or /hp_max URL the field's own tiny
  // form already targets. Two different forms both able to submit to that
  // one endpoint invites exactly the kind of race the Enter-key guard
  // further up this file exists to prevent, so this reuses the field's OWN
  // already-working autosave instead of adding a second path to the same
  // place — same "set the value, dispatch focusout" shape the armor_chassis
  // handler just above already uses for the identical reason.
  //
  // Two different kinds of field can be synced, which is why the lookup
  // tries both: AC and Max HP are .companion-delta-field inputs with their
  // own per-field <form> (they accept "+3" deltas as well as absolute
  // values), while Speed and Fly are plain .companion-field inputs on the
  // card's shared whole-record form. Both autosave on focusout, so the
  // same dispatch drives either one.
  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".companion-sync-btn");
    if (!btn) return;
    const container = btn.closest(".companion-card, .companion-sheet");
    if (!container) return;
    const name = btn.dataset.field;
    const field =
      container.querySelector('.companion-delta-field[data-field="' + name + '"]') ||
      container.querySelector('.companion-field[name="' + name + '"]');
    if (!field) return;
    field.value = btn.dataset.value;
    field.dispatchEvent(new Event("focusout", { bubbles: true }));
  });

  // The Sync-buttons popout (.companion-sync-menu, companion_fields.html) —
  // same .dropdown-filter/.dropdown-toggle/.dropdown-panel shape the Jutsu
  // page's Categories menu uses (jutsu-filter.js), but delegated from
  // document instead of bound directly to one toggle/panel pair: a
  // companion card, and the popout inside it, can be replaced wholesale by
  // a fragment swap at any time (see this file's own header comment), and
  // unlike the Jutsu page there can be several of these panels open at once
  // across different companion cards on the same Companions tab. Clicking a
  // .companion-sync-btn inside an open panel does NOT close it — the panel
  // stays open so several fields (e.g. every ability score after a level
  // up) can be synced in one pass instead of reopening the menu each time.
  function closeAllCompanionSyncPanels() {
    document.querySelectorAll(".companion-sync-panel").forEach((panel) => {
      panel.hidden = true;
      const toggle = panel.parentElement && panel.parentElement.querySelector(".companion-sync-toggle");
      if (toggle) toggle.setAttribute("aria-expanded", "false");
    });
  }
  document.addEventListener("click", (e) => {
    const toggle = e.target.closest(".companion-sync-toggle");
    if (toggle) {
      const panel = toggle.parentElement && toggle.parentElement.querySelector(".companion-sync-panel");
      if (!panel) return;
      const opening = panel.hidden;
      closeAllCompanionSyncPanels();
      panel.hidden = !opening;
      toggle.setAttribute("aria-expanded", String(opening));
      return;
    }
    if (!e.target.closest(".companion-sync-panel")) closeAllCompanionSyncPanels();
  });

  // The Nin-Dog "Cast" button next to each auto-known Inuzuka Hijutsu
  // (nindog_reference's own Known Hijutsu list, companion_fields.html) —
  // same "set the value, dispatch focusout" reuse of jutsu_slots_current's
  // own already-working delta-field autosave as .companion-sync-btn just
  // above, just spending a fixed -1 delta instead of syncing to a computed
  // value. AddCompanionIntField floors at 0 server-side, so casting with no
  // Jutsu Slots left is a harmless no-op rather than going negative.
  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".companion-jutsu-cast-btn");
    if (!btn) return;
    const container = btn.closest(".companion-card, .companion-sheet");
    const field = container && container.querySelector('.companion-delta-field[data-field="jutsu_slots_current"]');
    if (!field) return;
    field.value = "-1";
    field.dispatchEvent(new Event("focusout", { bubbles: true }));
  });
})();
