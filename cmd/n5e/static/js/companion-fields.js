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
    if (!/\/companions\/\d+(\/(ac|hp|hp_max))?$/.test(form.action)) return;
    e.preventDefault();
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  });

  // Picking a new Armor Chassis: save the pick itself (a <select> firing
  // "change" doesn't reliably also fire "focusout" the way clicking away
  // from a text field does — a mouse-driven selection can leave the select
  // focused with nothing else forcing a blur, so this can't just rely on
  // the generic .companion-field focusout listener above ever running),
  // and recompute AC's real stored value right away — the same "populate
  // then edit" treatment #3/#4 gave AC/HP-max generally, since picking a
  // chassis is itself a deliberate stat-defining action (like creating the
  // puppet was), not a passive re-render. Reads the puppet's currently-
  // DISPLAYED Dex score (not a fresh server round trip) so an unsaved Dex
  // edit still factors in, computes 10 + chassis AC bonus + Dex modifier
  // capped per the chassis's own dex_bonus_mode (mirrors
  // puppetArmorChassisAC server-side exactly), writes it into the AC
  // field, then fires the normal focusout autosave above — the player can
  // still nudge the result afterward, same as any other edit.
  document.addEventListener("change", (e) => {
    const select = e.target;
    if (!(select instanceof Element) || select.name !== "armor_chassis") return;
    select.dispatchEvent(new Event("focusout", { bubbles: true }));

    const opt = select.selectedOptions[0];
    if (!opt || !opt.value || !opt.dataset.acBonus) return; // "— none —" has neither
    const container = select.closest(".companion-card, .companion-sheet");
    if (!container) return;
    const dexField = container.querySelector('[name="dex_score"]');
    const acField = container.querySelector('.companion-delta-field[data-field="ac"]');
    if (!dexField || !acField) return;
    const dexScore = parseInt(dexField.value, 10);
    let dexMod = Number.isFinite(dexScore) ? Math.floor((dexScore - 10) / 2) : 0;
    if (opt.dataset.dexMode === "max2" && dexMod > 2) dexMod = 2;
    else if (opt.dataset.dexMode === "none") dexMod = 0;
    acField.value = String(10 + parseInt(opt.dataset.acBonus, 10) + dexMod);
    acField.dispatchEvent(new Event("focusout", { bubbles: true }));
  });

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
})();
