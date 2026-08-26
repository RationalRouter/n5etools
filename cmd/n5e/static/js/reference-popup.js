// Opens the "Class Reference" (character_reference.html), "Clan Reference"
// (character_clan_reference.html), and every subclass tracker popup (Titan
// Slots, S.N.B Upgrades, each Science-Nin subclass's own tracker —
// subclass_tracker_popup.go) in a real, separate, movable browser window —
// same window.open pattern companion-popup.js already uses for a
// companion's own sheet. The window name is keyed off the link's own href,
// so no two of these popups ever clobber each other's window.
//
// Unlike a companion popup, most of these links have no per-instance id to
// track by (there's one Titan Slots popup per character, not per upgrade) —
// so open handles are tracked by href instead, and only for the subclass
// tracker popups specifically, not the two plain reference popups.
(function () {
  // Titan Slots/SNB Upgrades/every Science-Nin subclass tracker all show a
  // live Creation Points tally that the main sheet's own Scientific Ninja
  // Tools panel can also spend from (sheet-science-nin, see sheet-vitals.js)
  // — a purchase made there has no way to tell an already-open one of these
  // to refresh, so it goes stale until the player closes and reopens it by
  // hand. Class Reference/Clan Reference/Custom Features are plain static
  // rules text with no such live figure — marked by their own -general
  // modifier class — so they're deliberately left untracked here; reloading
  // one would only cost the player their scroll position for no benefit.
  const trackedPopups = {};

  document.addEventListener("click", (e) => {
    const link = e.target.closest("[data-reference-popup]");
    if (!link) return;
    e.preventDefault();
    const handle = window.open(
      link.href,
      "n5e-reference-" + link.href,
      "popup,width=560,height=800,resizable=yes,scrollbars=yes",
    );
    if (handle && !link.classList.contains("sheet-reference-btn-general")) {
      trackedPopups[link.href] = handle;
    }
  });

  // Called by sheet-vitals.js after a data-refresh-popups form save lands —
  // reloads every currently open, still-live tracked popup so its own
  // Creation Points tile (and known-picks list) picks up whatever the main
  // sheet just spent or refunded from the same shared pool.
  window.n5eReloadReferencePopups = function () {
    for (const href in trackedPopups) {
      const handle = trackedPopups[href];
      if (handle && !handle.closed) {
        handle.location.reload();
      } else {
        delete trackedPopups[href];
      }
    }
  };
})();
