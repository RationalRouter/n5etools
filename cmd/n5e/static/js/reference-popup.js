// Opens the "Class Reference" (character_reference.html) and "Clan
// Reference" (character_clan_reference.html) popups in a real, separate,
// movable browser window — same window.open pattern companion-popup.js
// already uses for a companion's own sheet, just for a fixed link per popup
// instead of a set of per-companion ones, so it doesn't need that file's
// per-companion-id tracking or reload-on-save hook. The window name is
// keyed off the link's own href, so the two popups never clobber each
// other's window.
(function () {
  document.addEventListener("click", (e) => {
    const link = e.target.closest("[data-reference-popup]");
    if (!link) return;
    e.preventDefault();
    window.open(
      link.href,
      "n5e-reference-" + link.href,
      "popup,width=560,height=800,resizable=yes,scrollbars=yes",
    );
  });
})();
