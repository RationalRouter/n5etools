// Jutsu-learning creation step: the selection counters and the Select all /
// Deselect all buttons.
//
// Filtering is NOT done here any more — jutsu-filter.js drives this page's
// list through the same factory the /jutsu library page uses, so the step
// gets the full Categories/Rank/Action/Duration/Components/Range filter set
// instead of the name-and-rank text match this file used to do. Two
// independent filters writing `hidden` on the same rows would fight; this
// file only reads `hidden` (so "select all" means "select all of what is
// currently visible") and exposes window.n5eCreateJutsuCounts for the filter
// to call after each pass.
//
// Two counters, not one. The book's clan-jutsu rule is that a clan's jutsu
// are picked "instead of selecting jutsu from the Normal jutsu list(s)" —
// one allowance covering both — so the total is what's capped, and the clan
// count is shown beside it to keep clan picks tracked separately.
(function () {
  const list = document.getElementById("create-jutsu-list");
  const selectAllBtn = document.getElementById("create-jutsu-select-all");
  const deselectAllBtn = document.getElementById("create-jutsu-deselect-all");
  const countEl = document.getElementById("create-jutsu-count");
  const clanCountEl = document.getElementById("create-jutsu-clan-count");
  const capHint = document.getElementById("create-jutsu-cap-hint");
  if (!list || !countEl) return;

  const rows = Array.from(list.querySelectorAll(".jutsu-choice-row"));
  const checkboxes = rows.map((row) => row.querySelector("input[type=checkbox]"));

  const known = parseInt(countEl.dataset.known, 10);

  function updateCount() {
    const n = checkboxes.filter((cb) => cb.checked).length;
    countEl.textContent = known >= 0 ? n + " / " + known + " selected" : n + " selected";
    countEl.classList.toggle("jutsu-selected-count-over", known >= 0 && n > known);

    // At the cap, every unpicked jutsu goes disabled rather than staying
    // clickable and being rejected on submit. Unchecking one re-enables the
    // rest immediately, so the allowance reads as a budget you spend rather
    // than as a rule the form tells you about after the fact. The server
    // still enforces the same cap in handleCreateJutsu — this is the
    // affordance, not the guard.
    if (known >= 0) {
      const atCap = n >= known;
      checkboxes.forEach((cb) => {
        cb.disabled = atCap && !cb.checked;
      });
      if (capHint) capHint.hidden = !atCap;
    }

    if (clanCountEl) {
      const clan = rows.filter((row, i) => row.dataset.source === "clan" && checkboxes[i].checked).length;
      clanCountEl.textContent = clan + " clan jutsu";
      clanCountEl.hidden = false;
    }
  }

  // Called by jutsu-filter.js after every filter pass. The counters don't
  // actually change when rows are hidden — a hidden pick is still a pick —
  // but the hook keeps the two files' contract explicit rather than
  // relying on this file happening not to need it.
  window.n5eCreateJutsuCounts = updateCount;

  checkboxes.forEach((cb) => cb.addEventListener("change", updateCount));

  if (selectAllBtn) {
    // "Select all" means "all of what I can currently see", but it stops at
    // the allowance rather than blowing straight past it — with a cap in
    // force, a button that hands you an invalid selection in one click is
    // just a slower way to get the error page.
    selectAllBtn.addEventListener("click", () => {
      let n = checkboxes.filter((cb) => cb.checked).length;
      rows.forEach((row, i) => {
        if (row.hidden || checkboxes[i].checked) return;
        if (known >= 0 && n >= known) return;
        checkboxes[i].checked = true;
        n++;
      });
      updateCount();
    });
  }
  if (deselectAllBtn) {
    deselectAllBtn.addEventListener("click", () => {
      rows.forEach((row, i) => {
        if (!row.hidden) checkboxes[i].checked = false;
      });
      updateCount();
    });
  }

  updateCount();
})();
