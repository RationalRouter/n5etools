// Popup-page-specific companion behavior (companion_sheet.html). The
// shared field-autosave core (whole-form blur, HP box) lives in
// companion-fields.js, loaded alongside this file — see that file's own
// header comment for why the two are split.
//
// The Summon Tribe picker changes what reference content shows below it —
// save immediately on selection rather than waiting for a blur that a
// mouse-driven <select> choice may not actually trigger, then reload so
// the (currently absent) reference panel below it populates. The reload is
// a real page load, not a fetch-and-swap — this popup is a small, mostly-
// static page, not the main sheet's box grid, so there's no scroll-
// position/tab-state to preserve the way the main sheet's own reload sites
// have to (the main sheet's own Summons tab uses a fragment refresh
// instead for exactly that reason — see sheet-puppets.js).
(function () {
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof Element)) return;
    // summon_tribe_slug is not a one-way-locked field the way nin_dog_breed
    // is (SetCompanionFields has no CASE WHEN guard on it), so there is
    // nothing to preview — it still commits, and reveals its own reference
    // content, the instant it's picked. nin_dog_breed used to be special-
    // cased here too, committing on the same "change" event; it no longer
    // is — see the .companion-commit-btn click listener below, which is
    // now the only thing that writes a breed pick, on an explicit click
    // rather than the moment an option is merely highlighted.
    if (field.name !== "summon_tribe_slug") return;
    if (!field.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        window.location.reload();
      })
      .catch((err) => console.warn("companion autosave failed:", err));
  });

  // Demon Foe (is_demon_foe) changes each affected attack row's OWN
  // displayed damage dice (titanApplyDemonFoeBonus, titan.go) — server-
  // computed HTML, not something this page recomputes client-side — so,
  // same reasoning as summon_tribe_slug just above, waiting on a checkbox's
  // own "focusout" (companion-fields.js's generic whole-form autosave)
  // isn't good enough: clicking a checkbox with a mouse checks it and
  // leaves it focused, with no further blur ever coming unless the player
  // happens to click something else afterward — which reads as "checking
  // the box does nothing" the moment the player just looks at the Attacks
  // list instead. Saves on "change" (which DOES fire the instant a
  // checkbox is toggled, mouse or keyboard) and reloads the same way
  // summon_tribe_slug does. companion-fields.js's own "change" listener has
  // already mirrored this checkbox's new state onto every other
  // is_demon_foe checkbox on the card by the time this fires, so the
  // form's own FormData already reflects one consistent value.
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || field.name !== "is_demon_foe") return;
    if (!field.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        window.location.reload();
      })
      .catch((err) => console.warn("companion autosave failed:", err));
  });

  // Nin-Dog Breed's explicit "Set Breed" commit button (companion_fields.
  // html's own doc comment, and companion-fields.js's .companion-commit-
  // select handling, cover the reasoning in full): selecting a breed only
  // PREVIEWS it — sheet-option-tooltip.js's own "change" listener updates
  // the tooltip text above the select with no network call at all — so the
  // actual permanent, one-way-locked write only happens here, on an
  // explicit click, and only once the button is no longer disabled (which
  // companion-fields.js keeps in sync with whether a real option is
  // currently chosen). Reuses the same "post, then reload" shape as
  // summon_tribe_slug just above, since committing a breed likewise
  // reveals reference content — the breed's own trait list, its bonus
  // special-feature pick slots — not yet in the DOM.
  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".companion-commit-btn");
    if (!btn || btn.disabled) return;
    const label = btn.closest(".companion-block-label");
    const select = label && label.querySelector(".companion-commit-select");
    if (!select || !select.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(select.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        const acSaved = select.name === "armor_chassis" && window.n5eCommitArmorChassisAC
          ? window.n5eCommitArmorChassisAC(select)
          : Promise.resolve();
        return acSaved.then(() => window.location.reload());
      })
      .catch((err) => console.warn("companion commit failed:", err));
  });

  // Nin-Dog's own feature-pick/Hijutsu-trait-pick forms (nindog_reference,
  // templates/partials/companion_fields.html) reuse the main sheet's
  // "sheet-fetch-form" class so the Companions TAB (sheet-vitals.js) can
  // already swap them in place without any popup-specific code — but this
  // standalone popup never loads sheet-vitals.js at all (see this file's
  // own header comment on staying genuinely small), so without this
  // listener the same class here falls through to a real, unhandled
  // browser form submission. Reuses the "post, then reload" shape the
  // breed/tribe change listener just above already uses, since a pick here
  // also changes what's shown below it (the picked feature's own name, the
  // next slot opening up).
  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement) || !form.classList.contains("sheet-fetch-form")) return;
    if (!/\/(nin-dog-feature|nin-dog-hijutsu-trait)$/.test(form.action)) return;
    e.preventDefault();
    if (!window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        window.location.reload();
      })
      .catch((err) => console.warn("companion autosave failed:", err));
  });
})();
