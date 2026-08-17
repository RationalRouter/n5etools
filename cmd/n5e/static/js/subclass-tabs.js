// Multi-select subclass tiles. Selecting a subclass reveals its
// description, its features (interleaved into the level-ordered "Class
// Features" list alongside every other selected subclass and the
// always-visible general features), and its option lists. Deselecting
// hides them again. A level-feature-group left with nothing visible (every
// subclass hidden, no general feature at that level) collapses too.
//
// Delegated from the document, with nothing captured at load time. This
// markup appears in two places: the /classes browse page, where it is in
// the HTML from the start, and the character-creation class picker, where
// creation-picker.js fetches the same class_detail fragment and swaps it
// into #creation-detail-pane after the page has already loaded. The
// previous version queried for .subclass-tiles once and returned early
// when it found none, so on the creation page it switched itself off
// before the tiles existed — the tiles rendered, and clicking them did
// nothing whatsoever.
//
// Which subclasses are selected is read back out of the DOM (.active on
// the tiles) rather than held in a Set here, so a freshly swapped-in
// fragment starts from its own state instead of inheriting selections
// that belonged to a different class.
//
// Purely progressive enhancement — with JS disabled, subclass-tagged
// content just stays hidden server-side and the page still shows general
// features. Harmless no-op on pages with no .subclass-tiles.
(function () {
  function apply(root) {
    const selected = new Set();
    root.querySelectorAll(".subclass-tile.active").forEach((btn) => {
      selected.add(btn.dataset.subclass);
    });
    root.querySelectorAll(".subclass-scope[data-subclass]").forEach((el) => {
      el.hidden = !selected.has(el.dataset.subclass);
    });
    root.querySelectorAll(".level-feature-group").forEach((group) => {
      const anyVisible = [...group.querySelectorAll(".entry")].some((e) => !e.hidden);
      group.hidden = !anyVisible;
    });
  }

  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".subclass-tile");
    if (!btn) return;
    const active = !btn.classList.contains("active");
    btn.classList.toggle("active", active);
    btn.setAttribute("aria-pressed", active ? "true" : "false");

    // Scoped to the detail pane the tile lives in when there is one, so a
    // creation-page swap can't reach outside itself; the whole document
    // otherwise, which is the /classes page's single class.
    apply(document.getElementById("creation-detail-pane") || document);
  });

  // Fresh markup always arrives with every subclass hidden and no tile
  // active, which is already the right starting state — so this only runs
  // for the initial page load, to collapse level groups that have nothing
  // visible in them.
  apply(document);
})();
