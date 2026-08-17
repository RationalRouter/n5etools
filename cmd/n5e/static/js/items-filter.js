// Items page: a filterable list on the left, a live detail pane on the
// right. Rows are real <a href="/items/{slug}"> links (so middle-click /
// open-in-new-tab / no-JS all still work — progressive enhancement, same
// approach as the rest of this app's no-build-step JS), intercepted here to
// fetch just the detail card's HTML fragment and swap it in instead of a
// full page reload. All rows are server-rendered up front (a few hundred at
// most), so text/kind filtering is plain DOM show/hide — same in-memory
// approach as static/js/search.js.
(function () {
  const search = document.getElementById("items-search");
  const kindFilters = document.getElementById("items-kind-filters");
  const list = document.getElementById("browse-list");
  const detailPane = document.getElementById("browse-detail-pane");
  const rows = document.querySelectorAll(".browse-row");
  if (!search || !kindFilters || !list || !detailPane || rows.length === 0) return;

  // The server pre-selects the first group's first item into the detail
  // pane (see handleItems) — mark the matching row active to match, since
  // DOM order here is the same sort order that produced that selection.
  rows[0].classList.add("active");

  const kinds = [...new Set([...rows].map((r) => r.dataset.kind))];
  const checked = new Set(kinds);

  for (const kind of kinds) {
    const label = document.createElement("label");
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.checked = true;
    cb.addEventListener("change", () => {
      if (cb.checked) checked.add(kind);
      else checked.delete(kind);
      apply();
    });
    label.appendChild(cb);
    label.append(" " + kind.charAt(0).toUpperCase() + kind.slice(1));
    kindFilters.appendChild(label);
  }

  // Simple/Martial/Exotic and Light/Medium/Heavy, folded into the one apply()
  // pass below rather than hiding rows on their own — see the module's header.
  const subfilters = window.n5eEquipmentSubfilters({
    container: document.getElementById("items-subfilters"),
    rows,
    onChange: () => apply(),
  });

  function apply() {
    const q = search.value.trim().toLowerCase();
    for (const row of rows) {
      const matchesText = q === "" || row.dataset.name.toLowerCase().includes(q);
      const matchesKind = checked.has(row.dataset.kind);
      row.hidden = !(matchesText && matchesKind && subfilters.matches(row));
    }
    for (const heading of list.querySelectorAll("[data-kind-heading]")) {
      let anyVisible = false;
      for (let el = heading.nextElementSibling; el && !el.hasAttribute("data-kind-heading"); el = el.nextElementSibling) {
        if (!el.hidden) anyVisible = true;
      }
      heading.hidden = !anyVisible;
    }
  }

  async function selectRow(row) {
    for (const r of rows) r.classList.remove("active");
    row.classList.add("active");
    try {
      const res = await fetch(row.href + "?fragment=1");
      if (!res.ok) return;
      detailPane.innerHTML = await res.text();
    } catch (e) {
      // Fetch failed (offline asset, etc.) — leave the previous detail pane
      // showing rather than clearing it; the row's href still works as a
      // normal link if the user clicks it again or reloads.
      console.error("items: fragment fetch failed", e);
    }
  }

  for (const row of rows) {
    row.addEventListener("click", (e) => {
      e.preventDefault();
      selectRow(row);
    });
  }

  search.addEventListener("input", apply);

  // Deep link from site search (/items/{slug}) lands on the standalone
  // detail page directly, not here — but ?q= is kept as a fallback filter
  // entry point for anyone linking straight to the list with a name.
  const q = new URLSearchParams(location.search).get("q");
  if (q) {
    search.value = q;
    apply();
    const visible = [...rows].filter((r) => !r.hidden);
    if (visible.length === 1) selectRow(visible[0]);
  }
})();
