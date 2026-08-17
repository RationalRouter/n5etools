// Inventory, jutsu, feat and custom-attack edits from anywhere on the
// sheet. Every form here is a real form that works on its own — this only
// stops the browser from navigating away and posts via fetch instead, so
// the player stays where they were on a long page.
//
// Every one of these forms carries data-target (and often
// data-also-refresh): the Go handler answers with a re-rendered fragment
// (see character_sheet.html's "sheet_X" blocks and characters.go's
// renderSheetFragment) and it gets swapped into place, same pattern as
// sheet-vitals.js/sheet-jutsu-fetch.js. The one exception still answering
// with a full-page reload is the custom item "Edit this item" form on an
// item detail popup (item_detail.html, class sheet-inventory-add-form, no
// data-target) — it's a shared local-library entry, not scoped to one
// character's sheet or even necessarily open from the sheet at all, so
// there's no single fragment to swap it into; the handler below falls back
// to the old reload-in-place behavior when data-target is absent.
//
// A plain reload still LOOKS like the page-reset bug (jump to the top of
// the sheet), even though this one is unavoidable rather than a missed
// rewire call: `window.location.reload()` resets scroll to the top
// regardless of which tab reopens. Fixed the same way sheet-tabs.js
// remembers the open tab — sessionStorage, keyed per character, written right before reloading
// and consumed once on the next load. Keep this scroll-restore code as long
// as any form here still reloads — delete it only once every form in
// HANDLED carries a data-target.
//
// Delegated from the document rather than from the Inventory panel, which
// is what it used to be scoped to: these same form classes now appear on
// the Core tab (the condensed inventory, the attack tables' trash cans) and
// in all three library panes. A listener bound to one panel at load time
// would quietly do nothing for every one of them.
(function () {
  const HANDLED = ["sheet-inventory-form", "sheet-inventory-add-form", "sheet-library-form"];

  const chat = document.querySelector(".sheet-chat-panel");
  const scrollKey = "n5e:sheet-scroll:" + (chat ? chat.dataset.characterId : "unknown");

  // Restore once, then forget — a scroll position left over from a stale
  // reload should never apply to an unrelated later page load (e.g. the
  // player navigating away and back via a bookmark).
  try {
    const saved = sessionStorage.getItem(scrollKey);
    if (saved !== null) {
      sessionStorage.removeItem(scrollKey);
      const y = parseInt(saved, 10);
      if (!Number.isNaN(y)) window.scrollTo(0, y);
    }
  } catch (err) {
    // Private-browsing modes can refuse to read/write. Landing at the top
    // is not worth breaking the page over.
  }

  // Quantity boxes save themselves. The <button>Save</button> beside each one
  // is still in the markup — it is what makes the row work with JS off — but
  // a Save button on every row of a long inventory is noise, so the class
  // below hides them and a change on the field submits its own form instead.
  // requestSubmit rather than submit() so the delegated handler above still
  // sees the event and turns it into a fetch.
  document.documentElement.classList.add("js-qty-autosave");
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || !field.classList.contains("sheet-inventory-qty")) return;
    if (field.form) field.form.requestSubmit();
  });

  // The ✎ on a custom attack row reveals that row's edit form, which is
  // rendered underneath it as a normally-hidden second <tr>. Server-rendered
  // rather than built here so it works with JS off too: without this listener
  // the form is simply always visible, which is worse-looking but not broken.
  document.addEventListener("click", (e) => {
    const btn = e.target instanceof Element ? e.target.closest("[data-edit-target]") : null;
    if (!btn) return;
    const row = document.getElementById(btn.dataset.editTarget);
    if (!row) return;
    row.hidden = !row.hidden;
    btn.setAttribute("aria-expanded", String(!row.hidden));
    if (!row.hidden) {
      const first = row.querySelector("input[name=name]");
      if (first) first.focus();
    }
  });

  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (!HANDLED.some((cls) => form.classList.contains(cls))) return;
    // Jutsu learn/forget forms carry sheet-inventory-form/sheet-library-form
    // too (for their shared CSS), but sheet-jutsu-fetch.js owns them —
    // jutsu only ever touches two known, bounded regions (the Known Jutsu
    // list and the Attacks & Jutsu table), so it swaps those in place
    // instead of reloading. Without this exclusion both listeners would
    // fire on the same submit.
    if (form.classList.contains("sheet-jutsu-fetch-form")) return;
    e.preventDefault();

    const body = new URLSearchParams(new FormData(form));
    // A library pane is one <form> holding hundreds of rows, each with its
    // own submit button carrying the slug as its name/value. FormData never
    // includes the button that submitted the form, so without this every
    // drop and every click would post an empty slug.
    if (e.submitter && e.submitter.name) body.append(e.submitter.name, e.submitter.value);

    const targetId = form.dataset.target;

    fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: body.toString(),
    })
      .then((r) => {
        if (!r.ok) {
          return r.text().then((text) => {
            throw new Error(text || "request failed (" + r.status + ")");
          });
        }
        return r.text();
      })
      .then((html) => {
        // Forms with a data-target get the fragment swapped straight in,
        // same as sheet-vitals.js/sheet-jutsu-fetch.js. Forms without one
        // fall back to the old reload, since the response there is a full
        // page reload, not a fragment — see this file's header comment.
        if (!targetId) {
          try {
            sessionStorage.setItem(scrollKey, String(window.scrollY));
          } catch (err) {
            // See the read side above — losing the saved position isn't
            // worth breaking the reload over.
          }
          window.location.reload();
          return;
        }
        const target = document.getElementById(targetId);
        if (!target) return;
        target.outerHTML = html;
        if (window.n5eRewireControls) window.n5eRewireControls();
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks(form.dataset.alsoRefresh);
      })
      .catch((err) => {
        console.warn("sheet update failed:", err);
        window.alert("Could not save that change: " + err.message);
      });
  });
})();
