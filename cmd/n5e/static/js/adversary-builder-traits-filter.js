// /encounters/adversary-traits: a plain text filter over a single
// server-rendered tooltip-list page (906+ rows across 7 category groups).
// No detail pane/per-entity route — this catalog is GM-reference material
// only, read in place via the same hover-tooltip pattern used everywhere
// else in the app, not a browsable entity with its own URL.
(function () {
  const search = document.getElementById("adversary-traits-search");
  const rows = document.querySelectorAll(".adversary-trait-row");
  if (!search || rows.length === 0) return;

  function apply() {
    const q = search.value.trim().toLowerCase();
    for (const row of rows) {
      row.hidden = q !== "" && !row.dataset.filter.includes(q);
    }
    for (const section of document.querySelectorAll("[data-category-section]")) {
      section.hidden = section.querySelector(".adversary-trait-row:not([hidden])") === null;
    }
  }

  search.addEventListener("input", apply);
})();
