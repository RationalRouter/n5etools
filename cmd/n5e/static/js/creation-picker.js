// Shared two-pane list+detail picker for character-creation steps (Clan,
// Class, Background, Jutsu, Equipment) — same fetch-a-fragment-and-swap
// pattern as jutsu-filter.js's/items-filter.js's selectRow, generalized and
// extended to also sync a fixed confirm form's hidden input, since (unlike
// the plain browse pages) picking a row here doesn't apply anything by
// itself — the fragment swap is only a preview, an explicit submit button
// outside the fetched fragment is what actually confirms the choice.
//
// Deliberately scoped to its OWN class/id names (.creation-picker-row,
// #creation-detail-pane, #creation-picker-value) rather than reusing
// jutsu.html's/items.html's #browse-detail-pane/.browse-row directly, even
// though the visual .browse-layout/.browse-row/.browse-detail-pane CSS
// classes ARE shared for identical styling — reusing the same ID/class for
// JS targeting would make this file and jutsu-filter.js/items-filter.js
// both wire up (and race) on each other's pages, since layout.html loads
// every script on every page and this file's own early-return guard can't
// tell "jutsu page" apart from "creation page" any other way.
(function () {
  const detailPane = document.getElementById("creation-detail-pane");
  const rows = document.querySelectorAll(".creation-picker-row");
  const hiddenInput = document.getElementById("creation-picker-value");
  const submitBtn = document.getElementById("creation-picker-submit");
  if (!detailPane || rows.length === 0) return;

  // The Class step's Starting Level picker lives in the page header
  // (create_class.html), outside #creation-detail-pane, so it survives
  // every fragment swap below untouched — but it only applies to a brand
  // first-level class pick (class_picker.html's #class-confirm-form), not
  // an already-held class or a multiclass add, both of which render
  // without that form. Re-synced after every swap, and once up front for
  // whichever class query param loaded the page already focused.
  const levelPicker = document.getElementById("creation-level-picker");
  const levelSelect = document.getElementById("creation-level-select");
  function syncLevelPicker() {
    if (!levelPicker || !levelSelect) return;
    const form = document.getElementById("class-confirm-form");
    if (!form) {
      levelPicker.hidden = true;
      return;
    }
    levelPicker.hidden = false;
    levelSelect.value = form.dataset.currentLevel || "1";
  }

  async function selectRow(row) {
    for (const r of rows) r.classList.remove("active");
    row.classList.add("active");
    if (hiddenInput) hiddenInput.value = row.dataset.slug;
    if (submitBtn) submitBtn.disabled = false;
    try {
      // row.href may already carry a query string (?clan=, ?class=, ...),
      // unlike jutsu.go's/items.go's plain /jutsu/{slug} routes — build the
      // URL properly instead of naively concatenating a second "?".
      const url = new URL(row.href, window.location.href);
      url.searchParams.set("fragment", "1");
      const res = await fetch(url);
      if (!res.ok) return;
      detailPane.innerHTML = await res.text();
      syncLevelPicker();
    } catch (e) {
      console.error("creation picker: fragment fetch failed", e);
    }
  }

  for (const row of rows) {
    row.addEventListener("click", (e) => {
      e.preventDefault();
      selectRow(row);
    });
  }

  syncLevelPicker();
})();
