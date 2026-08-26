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
    if (!(field instanceof Element)) return;
    // summon_tribe_slug is not a one-way-locked field, so it still commits
    // and reveals its own reference content the instant it's picked — see
    // companion-sheet.js's own identical comment for the popup half of
    // this picker. nin_dog_breed used to be special-cased here too; it no
    // longer is — see the .companion-commit-btn click listener below.
    if (field.name !== "summon_tribe_slug") return;
    if (!field.closest("#sheet-summon-tab")) return; // only this tab's own picker, not the popup's
    if (!field.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks("sheet-summon-tab");
      })
      .catch((err) => console.warn("summon tribe save failed:", err));
  });

  // Demon Foe (is_demon_foe) — this tab's own half of the identical fix as
  // companion-sheet.js's listener for the popup (see that file's own
  // comment for why a checkbox's "change" event, not "focusout", is what
  // has to trigger the save): a Titan's own Attacks cards live only on the
  // Companions tab (#sheet-summon-tab), so a fragment refresh there is
  // enough to show every affected row's new dice count immediately, the
  // same "no full reload" reasoning summon_tribe_slug's own listener above
  // already uses for this tab.
  document.addEventListener("change", (e) => {
    const field = e.target;
    if (!(field instanceof Element) || field.name !== "is_demon_foe") return;
    if (!field.closest("#sheet-summon-tab")) return; // only this tab's own checkbox, not the popup's
    if (!field.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(field.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        if (window.n5eRefreshBlocks) window.n5eRefreshBlocks("sheet-summon-tab");
      })
      .catch((err) => console.warn("demon foe save failed:", err));
  });

  // Every locked-once-chosen picker's explicit "Set ..." commit button
  // (Nin-Dog Breed and Titan Specialization on the Summons tab, Armor
  // Chassis on the Puppets tab), this tab's own half of the same fix as
  // companion-sheet.js's identical listener for the popup — see that
  // file's own comment for why the commit is deferred to an explicit click
  // rather than firing on "change"/blur. Delegated from document (not
  // bound to any one button) so it keeps working after any fragment swap
  // that replaces either tab's own markup — adding/deleting a companion,
  // another card's own save — with no rewire pass needed.
  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".companion-commit-btn");
    if (!btn || btn.disabled) return;
    const tab = btn.closest("#sheet-summon-tab, #sheet-puppet-tab"); // only these tabs' own pickers, not the popup's
    if (!tab) return;
    const label = btn.closest(".companion-block-label");
    const select = label && label.querySelector(".companion-commit-select");
    if (!select || !select.form || !window.n5eCompanionPostForm) return;
    window.n5eCompanionPostForm(select.form)
      .then((r) => {
        if (!r.ok) throw new Error("server rejected the request (" + r.status + ")");
        const acSaved = select.name === "armor_chassis" && window.n5eCommitArmorChassisAC
          ? window.n5eCommitArmorChassisAC(select)
          : Promise.resolve();
        return acSaved.then(() => {
          if (window.n5eRefreshBlocks) window.n5eRefreshBlocks(tab.id);
        });
      })
      .catch((err) => console.warn("commit failed:", err));
  });
})();
