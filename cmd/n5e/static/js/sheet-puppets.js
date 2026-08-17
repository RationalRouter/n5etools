// Main-sheet-specific behavior for the Puppets and Summons tabs. Field
// autosave (the whole-form blur, the HP box, "Use computed" hint buttons)
// is shared with the companion popup via companion-fields.js; companion
// add/delete and Puppet Upgrade pick/remove all use the app's existing
// generic form.sheet-fetch-form + data-target/data-also-refresh mechanism
// (sheet-vitals.js/sheet-refresh.js), so none of that needs any code here.
//
// The one thing that DOES need a dedicated listener: a Summons-tab card's
// Summon Tribe <select> changes what reference content that SAME card
// shows below it — the popup's own companion-sheet.js does a full page
// reload for this (fine for a small static page), but a full reload on the
// main sheet would lose scroll position/the open tab, exactly the
// page-reset bug class documented elsewhere in this codebase. A fragment
// refresh (re-rendering sheet_summon_tab after the save lands) gets the
// same result — every card's reference re-populated from the DB — without
// a real navigation.
(function () {
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || field.name !== "summon_tribe_slug") return;
    if (!field.closest("#sheet-summon-tab")) return; // only this tab's own picker, not the popup's
    if (!field.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks("sheet-summon-tab");
      })
      .catch((err) => console.warn("summon tribe save failed:", err));
  });
})();
