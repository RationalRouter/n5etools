// Click-to-deselect for the plain "pick one of several tiles" radio
// pickers built from a <ul class="puppet-upgrade-option-list"> of
// <label class="puppet-upgrade-option"> wrapping a real
// <input type="radio" name="option_slug">, styled with no visible radio
// dot (see app.css) — Hunters Patterns/Exploits/Defensive Tactics and
// Martial Techniques all share this exact markup. Native radios have no
// concept of "click the selected one again to clear it" — clicking an
// already-checked radio is normally just a no-op — so that has to be done
// here, same mechanism sheet-puppet-detail.js already uses for the
// Puppets tab's own upgrade_entry_slug radios. Kept separate from that
// file rather than generalizing its name filter: these plain pickers have
// no right-hand preview panel to keep in sync, so they don't need that
// file's more involved change-listener, and the two listeners never
// collide since they match on different radio names.
//
// Delegated from document, not bound per-element: every list here lives
// inside a sheet-fetch-form fragment that gets replaced via outerHTML on
// submit, the same page-reset-bug shape documented project-wide.
(function () {
  // Tracks each option-list's currently selected radio, keyed by the <ul>
  // itself. "change" only fires on a REAL selection change, never on a
  // redundant click of the option that's already selected — exactly the
  // signal needed: this still holds the PREVIOUS selection at the moment
  // the "click" listener below runs (click always fires before change),
  // so a click matching it is provably a re-click of the same tile, not a
  // fresh pick.
  const lastSelectedInList = new WeakMap();

  document.addEventListener("change", (e) => {
    const el = e.target;
    if (!(el instanceof Element) || el.type !== "radio" || el.name !== "option_slug") return;
    const list = el.closest(".puppet-upgrade-option-list");
    if (list) lastSelectedInList.set(list, el);
  });

  document.addEventListener("click", (e) => {
    const label = e.target.closest(".puppet-upgrade-option");
    if (!label) return;
    const input = label.querySelector('input[type="radio"][name="option_slug"]');
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
  });
})();
