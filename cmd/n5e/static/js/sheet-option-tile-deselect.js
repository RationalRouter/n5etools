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

  // Only reacts to a click whose event target IS the radio <input> itself
  // — never the wrapping <label> (or any other descendant) it was clicked
  // through. Clicking anywhere inside a <label> fires TWO real "click"
  // events in sequence: one at whatever was actually clicked (bubbling up
  // through the label), then a second, separate one that the label's own
  // native activation behavior forwards straight at the associated
  // <input> — confirmed live by tracing DOM click order with an
  // instrumented .checked setter, not guessed. Reacting to the FIRST of
  // those two loses a real race: queueMicrotask's callback runs at the
  // microtask checkpoint between those two dispatches — i.e. AFTER the
  // first click finishes but BEFORE the label's forwarded click has even
  // started — so this listener's checked=false lands, and then the still-
  // pending forwarded click sees a (now) unchecked radio and re-checks it
  // right back to true as part of ITS OWN native activation, silently
  // undoing the deselect within the same physical click. This is exactly
  // why a synthetic input.click() test (which skips the label entirely, so
  // there's no second forwarded click to race against) passed while real
  // mouse clicks on the label kept failing. Reacting only to the input-
  // targeted click sidesteps the race entirely: that IS the final step of
  // the whole sequence (a radio's own native click never forwards further),
  // so nothing is left afterward to clobber the deferred assignment.
  document.addEventListener("click", (e) => {
    const input = e.target;
    if (!(input instanceof Element) || input.tagName !== "INPUT") return;
    if (input.type !== "radio" || input.name !== "option_slug" || !input.checked) return;
    const list = input.closest(".puppet-upgrade-option-list");
    if (!list || lastSelectedInList.get(list) !== input) return;
    // preventDefault()+checked=false here doesn't stick either, for the
    // same underlying reason: canceling reverts checked back to whatever
    // it was before THIS click, which for an already-checked radio is
    // checked. Deferring to a microtask sidesteps that revert; scoping to
    // only the input-targeted click (above) sidesteps the label-forwarding
    // race.
    queueMicrotask(() => {
      input.checked = false;
      lastSelectedInList.delete(list);
    });
  });
})();
