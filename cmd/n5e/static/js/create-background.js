// Background step's choose-N-from-pool dropdowns: prevents picking the
// same skill/tool option in two sibling dropdowns for the same
// background_proficiencies row. Same technique as create-abilities.js's
// roll-assignment dropdowns (kept as a separate small copy rather than a
// shared import — this app has no build step, and the two call sites
// don't share a DOM shape worth factoring together yet; worth revisiting
// if a third spot needs the same pattern).
(function () {
  const groups = {};
  document.querySelectorAll("select[name^='choice_']").forEach((select) => {
    // Group key is everything before the trailing "_<slot>" — e.g.
    // "choice_2_0" and "choice_2_1" share group "choice_2".
    const group = select.name.replace(/_\d+$/, "");
    (groups[group] ||= []).push(select);
  });

  function refresh(selects) {
    const chosenElsewhere = selects.map((s) => s.value);
    selects.forEach((select, i) => {
      for (const opt of select.options) {
        if (opt.value === "") continue;
        opt.disabled = chosenElsewhere.some((v, j) => j !== i && v === opt.value);
      }
    });
  }

  for (const selects of Object.values(groups)) {
    if (selects.length < 2) continue;
    selects.forEach((select) => select.addEventListener("change", () => refresh(selects)));
    refresh(selects);
  }

  // Tool-kind choices (e.g. background/genius's "One of your choice") get a
  // "View stats" link to the picked toolkit's item page, same idea as
  // create-equipment.js's kit-choice preview — kept in sync on load (so a
  // hydrated revisit shows the link immediately) and on every change.
  document.querySelectorAll("select.background-choice-select").forEach((select) => {
    const preview = select.parentElement.querySelector(".background-choice-preview");
    if (!preview) return;
    function syncPreview() {
      const slug = select.selectedOptions[0]?.dataset.slug || "";
      if (!slug) {
        preview.hidden = true;
        return;
      }
      preview.href = "/items/" + slug;
      preview.dataset.slug = slug;
      preview.hidden = false;
    }
    select.addEventListener("change", () => {
      syncPreview();
      if (!preview.hidden) preview.click();
    });
    syncPreview();
  });
})();
