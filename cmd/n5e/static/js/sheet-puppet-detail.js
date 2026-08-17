// The Puppets tab's right-hand panel: instead of a fixed "Puppet Master
// Reference" list, it shows the full details of whatever the player last
// clicked or is currently considering on the left — a taken upgrade, a
// structured attack, or an Armor Chassis/upgrade option not yet added.
// Entirely client-side, no server round trip: the data is either already
// rendered (a hidden <template> sibling, for something already added) or
// sitting in the selected <option>/<input>'s own data-* attributes (for a
// pending choice).
//
// Both listeners are delegated from document, not bound per-element — same
// reason every other control on a swappable sheet-fetch-form fragment uses
// this pattern (see character-sheet.js's own .rollable handler): the
// Puppets tab's whole companion list can be replaced by outerHTML on any
// add/delete/pick, and a directly-bound listener would silently go dead the
// first time that happens.
(function () {
  const content = document.getElementById("puppet-detail-content");
  const placeholder = document.getElementById("puppet-detail-placeholder");
  if (!content) return; // this panel only exists on the Puppets tab

  function escapeHTML(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function show(name, bodyHTML) {
    content.innerHTML = "<h3>" + escapeHTML(name) + "</h3>" + bodyHTML;
    content.hidden = false;
    if (placeholder) placeholder.hidden = true;
  }

  function clear() {
    content.hidden = true;
    content.innerHTML = "";
    if (placeholder) placeholder.hidden = false;
  }

  // Tracks each upgrade-option-list's currently selected radio, keyed by
  // the <ul> itself — see the click listener below for why.
  const lastSelectedInList = new WeakMap();

  // Click on anything already added (an upgrade pick, an attack row) —
  // ignores clicks that originate inside a nested <form> (delete buttons,
  // the sub-choice add form) so removing or adding something doesn't also
  // flash its parent's own preview.
  document.addEventListener("click", (e) => {
    if (e.target.closest("form")) return;
    const el = e.target.closest("[data-detail-name]");
    if (!el) return;
    const template = el.querySelector(":scope > template.detail-body");
    show(el.dataset.detailName, template ? template.innerHTML : "");
  });

  // Change on a pending (not yet added) selection — the tier-upgrade radio
  // buttons and the Armor Chassis/sub-choice <select>s. Used to read
  // dataset.description (the raw book text, "•"s and all) and wrap it in
  // one plain <p> — tolerable only while the tooltip existed as the one
  // place a long option's own sub-bullets actually rendered properly (via
  // formatDescription, server-side); once the tooltip was removed this was
  // the ONLY preview, and a many-sub-bullet option (e.g. Mech Pilot) showed
  // as one run-on blob. Now reads the same server-rendered
  // formatDescription HTML an already-added pick's own <template
  // class="detail-body"> uses, just addressed differently since the two
  // shapes can't share one lookup: a radio's own <li> can hold a real
  // <template> sibling directly (HTML option content can't, so the
  // <select> cases instead read a hidden <template data-value="..."> from
  // a "detail-body-store" sibling of the <select> itself, matched by the
  // chosen option's own value).
  document.addEventListener("change", (e) => {
    const el = e.target;
    if (!(el instanceof Element)) return;
    let name, template;
    if (el.tagName === "SELECT" && el.dataset.detailSelect !== undefined) {
      const opt = el.selectedOptions[0];
      if (!opt || !opt.value) return;
      name = opt.textContent.trim();
      const store = el.nextElementSibling;
      template = store && store.classList.contains("detail-body-store")
        ? store.querySelector('template[data-value="' + CSS.escape(opt.value) + '"]')
        : null;
    } else if (el.type === "radio" && el.name === "upgrade_entry_slug") {
      name = el.dataset.name || "";
      const li = el.closest("li");
      template = li ? li.querySelector(":scope > template.detail-body") : null;
      lastSelectedInList.set(el.closest(".puppet-upgrade-option-list"), el);
    } else {
      return;
    }
    show(name, template ? template.innerHTML : "");
  });

  // Upgrade-option tiles are still real radio inputs under the hood (native
  // single-selection-per-tier and "required" form validation for free —
  // see character_sheet.html), just styled and clicked as plain tiles with
  // no visible radio dot (see app.css). Native radios have no concept of
  // "click the selected one again to clear it" — clicking an already-
  // checked radio is normally just a no-op — so that has to be done here.
  //
  // "change" only fires on a REAL selection change, never on a redundant
  // click of the option that's already selected, which is exactly the
  // signal needed: lastSelectedInList (updated by the "change" listener
  // above) still holds the PREVIOUS selection at the moment this "click"
  // listener runs (click always fires before change), so a click that
  // matches it is provably a re-click of the same tile, not a fresh pick.
  document.addEventListener("click", (e) => {
    const label = e.target.closest(".puppet-upgrade-option");
    if (!label) return;
    const input = label.querySelector('input[type="radio"][name="upgrade_entry_slug"]');
    if (!input || !input.checked) return;
    const list = input.closest(".puppet-upgrade-option-list");
    if (!list || lastSelectedInList.get(list) !== input) return;
    // Clicking a label's own default action forwards a second, synthetic
    // click straight to its associated control — which for a radio that's
    // (at that point) unchecked just checks it right back, silently
    // undoing the deselect a moment later. preventDefault() here stops
    // that forwarded click from ever being dispatched at all.
    e.preventDefault();
    input.checked = false;
    lastSelectedInList.delete(list);
    clear();
  });
})();
