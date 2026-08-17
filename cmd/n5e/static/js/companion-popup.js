// Opens a companion's own sheet (companion_sheet.html) in a real, separate,
// deliberately small browser window rather than the main app tab — this is
// the one link on the sheet that isn't a sheet-popup-trigger inline detail
// card and isn't a same-tab navigation either.
//
// Delegated from the document, not bound per-link at load: the Companions
// box (#sheet-companions) is a sheet-fetch-form fragment, so its rows — and
// this link along with them — get replaced by outerHTML on every
// add/delete. A directly-bound listener would silently die the first time
// that happens, the same failure mode every other sheet control that lives
// inside a swappable fragment has to avoid.
(function () {
  // Tracked by companion id (parsed from the link's own /companions/{id}
  // path) so a save made elsewhere on the main sheet's own Puppets/Summons
  // tab for that same companion (see companion-fields.js's
  // notifyPopupToReload) can find this window again and reload it — the
  // popup has no live-refresh mechanism of its own, a real reload matches
  // its existing reload-on-tribe-change convention.
  const openPopups = {};

  document.addEventListener("click", (e) => {
    const link = e.target.closest("[data-companion-popup]");
    if (!link) return;
    e.preventDefault();
    const handle = window.open(
      link.href,
      "n5e-companion-" + link.href,
      "popup,width=520,height=800,resizable=yes,scrollbars=yes",
    );
    const m = /\/companions\/(\d+)/.exec(link.href);
    if (handle && m) openPopups[m[1]] = handle;
  });

  window.n5eReloadCompanionPopupIfOpen = function (companionID) {
    const handle = openPopups[companionID];
    if (handle && !handle.closed) handle.location.reload();
  };
})();
