// Toolkit dropdowns in the creation flow — the equipment step's "N Kits of
// your Choice" bundles (see IsKitChoice in characters.go) and the class
// step's "Select any two Toolkits" proficiency slots (see
// charstore.ToolkitChoiceCount). Both render as .kit-choice-select inside a
// .kit-choice-slot, so both are handled here.
//
// Two jobs: stop the same toolkit being picked in two sibling slots, and
// keep each slot's "View stats" preview link pointing at the current
// selection, reusing creation-picker.js's existing fetch-and-swap click
// handler rather than duplicating that fetch here.
//
// Everything is delegated from the document and re-derived on each change,
// with nothing captured at load time. The class step's dropdowns do not
// exist when this file runs — creation-picker.js fetches the class detail
// fragment and swaps them in later — so the previous version, which
// queried once at load and attached listeners to what it found, was
// entirely inert there. (Same failure subclass-tabs.js had.) The equipment
// step's dropdowns are in the initial HTML, which is why that page kept
// working and this went unnoticed.
(function () {
  // Slots are siblings when their names share everything but the trailing
  // index: kit_0_0_0 / kit_0_0_1, or toolkit_0 / toolkit_1.
  function siblings(select) {
    const group = select.name.replace(/_\d+$/, "");
    return [...document.querySelectorAll("select.kit-choice-select")].filter(
      (other) => other.name.replace(/_\d+$/, "") === group,
    );
  }

  function refreshExclusion(selects) {
    if (selects.length < 2) return;
    const chosenElsewhere = selects.map((s) => s.value);
    selects.forEach((select, i) => {
      for (const opt of select.options) {
        if (opt.value === "") continue;
        opt.disabled = chosenElsewhere.some((v, j) => j !== i && v === opt.value);
      }
    });
  }

  // The option's slug when it carries one (the class step keys options by
  // display name, since that is what a proficiency is stored as), else the
  // value itself (the equipment step keys options by slug already).
  function slugFor(select) {
    const opt = select.selectedOptions[0];
    if (!opt || !opt.value) return "";
    return opt.dataset.slug || opt.value;
  }

  function refreshPreview(select) {
    const preview = select.parentElement && select.parentElement.querySelector(".kit-choice-preview");
    if (!preview) return;
    const slug = slugFor(select);
    if (!slug) {
      preview.hidden = true;
      return;
    }
    preview.href = "/items/" + slug;
    preview.dataset.slug = slug;
    preview.hidden = false;
  }

  document.addEventListener("change", (e) => {
    const select = e.target.closest("select.kit-choice-select");
    if (!select) return;
    refreshExclusion(siblings(select));
    refreshPreview(select);
    const preview = select.parentElement && select.parentElement.querySelector(".kit-choice-preview");
    if (preview && !preview.hidden) preview.click();
  });

  // Initial pass for whatever is already on the page, and again after a
  // fragment swap brings new slots in. Cheap enough to just redo wholesale.
  function refreshAll() {
    const groups = {};
    document.querySelectorAll("select.kit-choice-select").forEach((select) => {
      (groups[select.name.replace(/_\d+$/, "")] ||= []).push(select);
      refreshPreview(select);
    });
    for (const selects of Object.values(groups)) refreshExclusion(selects);
  }

  refreshAll();
  // creation-picker.js swaps fragments into #creation-detail-pane; watching
  // it covers the class step without that file needing to know about this
  // one. No-op on pages that have no such pane.
  const pane = document.getElementById("creation-detail-pane");
  if (pane) new MutationObserver(refreshAll).observe(pane, { childList: true, subtree: true });
})();
