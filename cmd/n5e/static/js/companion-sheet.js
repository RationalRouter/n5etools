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
    if (!(field instanceof Element) || field.name !== "summon_tribe_slug") return;
    if (!field.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        window.location.reload();
      })
      .catch((err) => console.warn("companion autosave failed:", err));
  });
})();
