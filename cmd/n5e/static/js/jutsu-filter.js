// The jutsu filter UI: text search over name AND description, a
// "Categories" classification/category-group dropdown, a "Filters" dropdown
// for Range/Duration/Components, sourcebook tiles, and Rank/Casting-Action
// checkbox rows — all client-side show/hide over server-rendered rows, so
// there is no per-keystroke round trip.
//
// This started as the /jutsu library page's own IIFE. It is now a factory
// because the character-creation jutsu step needs the same comprehensive
// filters as the jutsu library page, and a second copy of ~400 lines of
// filter logic would drift from this one the first time either changed.
// Everything the two pages disagree about is a config
// field:
//
//   ids            per-page element ids (the creation step prefixes its
//                  own with "create-jutsu-")
//   rowSelector    what counts as a row inside the list
//   itemSelector   OPTIONAL wrapper to hide instead of the row itself —
//                  the creation step's rows sit next to a checkbox inside
//                  a .jutsu-choice-row, and hiding only the link would
//                  leave an orphan checkbox behind
//   groupSelectors OPTIONAL heading wrappers to collapse when they empty
//   fragmentSwap   whether clicking a row fetches its detail card
//   readQueryParam whether a ?q= in the URL pre-fills the search box
//   onApply        OPTIONAL callback after every filter pass
//
// Both call sites are at the bottom of this file; each returns early if
// its own elements aren't on the page, so loading this script everywhere
// (as layout.html does) stays harmless.
(function () {
  function initJutsuFilter(cfg) {
    const ids = cfg.ids;
    const byId = (key) => (ids[key] ? document.getElementById(ids[key]) : null);

    const search = byId("search");
    const toggle = byId("categoryToggle");
    const panel = byId("categoryPanel");
    const detailsToggle = byId("detailsToggle");
    const detailsPanel = byId("detailsPanel");
    const sourceTiles = byId("sourceTiles");
    const rankFilters = byId("rankFilters");
    const actionFilters = byId("actionFilters");
    const originFilters = byId("originFilters");
    const durationFilters = byId("durationFilters");
    const componentFilters = byId("componentFilters");
    const rangeMinInput = byId("rangeMin");
    const rangeMaxInput = byId("rangeMax");
    const rangeFill = byId("rangeFill");
    const rangeMinLabel = byId("rangeMinLabel");
    const rangeMaxLabel = byId("rangeMaxLabel");
    const rangeIncludeSpecial = byId("rangeIncludeSpecial");
    const list = byId("list");
    const detailPane = byId("detailPane");
    if (!search || !toggle || !panel || !list) return;
    const rows = list.querySelectorAll(cfg.rowSelector || ".browse-row");
    if (rows.length === 0) return;

    // Hiding a row means hiding whatever wrapper carries it, so nothing it
    // shares a line with (a checkbox, a "Clan" badge) is left stranded.
    function itemFor(row) {
      if (!cfg.itemSelector) return row;
      return row.closest(cfg.itemSelector) || row;
    }

    rows[0].classList.add("active");

    function keyFor(cls, cat) {
      return cls + " " + cat;
    }

    // classification -> Set(category group), built from the rendered rows
    // rather than duplicating the grouping logic server-side.
    const byClassification = new Map();
    rows.forEach((r) => {
      const cls = r.dataset.classification;
      const cat = r.dataset.category;
      if (!byClassification.has(cls)) byClassification.set(cls, new Set());
      byClassification.get(cls).add(cat);
    });

    const checkedCategory = new Set();
    byClassification.forEach((cats, cls) => {
      cats.forEach((cat) => checkedCategory.add(keyFor(cls, cat)));
    });

    const actionsRow = document.createElement("div");
    actionsRow.className = "dropdown-panel-actions";
    const selectAllBtn = document.createElement("button");
    selectAllBtn.type = "button";
    selectAllBtn.textContent = "Select all";
    const deselectAllBtn = document.createElement("button");
    deselectAllBtn.type = "button";
    deselectAllBtn.textContent = "Deselect all";
    actionsRow.append(selectAllBtn, deselectAllBtn);
    panel.appendChild(actionsRow);

    const allGroups = [];

    byClassification.forEach((cats, cls) => {
      const catList = [...cats].sort();
      const isFlat = catList.length === 1 && catList[0] === cls;

      const clsRow = document.createElement("label");
      clsRow.className = "dropdown-panel-heading";
      const clsCb = document.createElement("input");
      clsCb.type = "checkbox";
      clsCb.checked = true;
      clsRow.appendChild(clsCb);
      clsRow.append(" " + cls);
      panel.appendChild(clsRow);

      if (isFlat) {
        clsCb.addEventListener("change", () => {
          if (clsCb.checked) checkedCategory.add(keyFor(cls, cls));
          else checkedCategory.delete(keyFor(cls, cls));
          apply();
        });
        allGroups.push({ clsCb, catCbs: [clsCb], cls, catList: [cls] });
        return;
      }

      const catCbs = [];
      catList.forEach((cat) => {
        const row = document.createElement("label");
        row.className = "dropdown-panel-item";
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = true;
        cb.addEventListener("change", () => {
          if (cb.checked) checkedCategory.add(keyFor(cls, cat));
          else checkedCategory.delete(keyFor(cls, cat));
          syncClsCheckbox();
          apply();
        });
        row.appendChild(cb);
        row.append(" " + cat);
        panel.appendChild(row);
        catCbs.push(cb);
      });

      function syncClsCheckbox() {
        const allChecked = catCbs.every((cb) => cb.checked);
        const noneChecked = catCbs.every((cb) => !cb.checked);
        clsCb.checked = allChecked;
        clsCb.indeterminate = !allChecked && !noneChecked;
      }

      clsCb.addEventListener("change", () => {
        clsCb.indeterminate = false;
        catCbs.forEach((cb) => {
          cb.checked = clsCb.checked;
        });
        catList.forEach((cat) => {
          if (clsCb.checked) checkedCategory.add(keyFor(cls, cat));
          else checkedCategory.delete(keyFor(cls, cat));
        });
        apply();
      });

      allGroups.push({ clsCb, catCbs, cls, catList });
    });

    function setAll(isChecked) {
      allGroups.forEach(({ clsCb, catCbs, cls, catList }) => {
        clsCb.checked = isChecked;
        clsCb.indeterminate = false;
        catCbs.forEach((cb) => {
          cb.checked = isChecked;
        });
        catList.forEach((cat) => {
          if (isChecked) checkedCategory.add(keyFor(cls, cat));
          else checkedCategory.delete(keyFor(cls, cat));
        });
      });
      apply();
    }

    selectAllBtn.addEventListener("click", () => setAll(true));
    deselectAllBtn.addEventListener("click", () => setAll(false));

    // Sourcebook tiles: click toggles that book in/out of the filter,
    // defaulting to every book active (matches the plan's reference image —
    // tiles start "on", every book shown). Exclusive single-select from
    // there on, not independent toggles: clicking a tile shows ONLY that
    // book, deselecting whatever was selected before. Clicking the sole
    // active tile again resets back to "every book" rather than leaving
    // zero selected (a dead-end state with nothing visible and no obvious
    // way out).
    const checkedSource = new Set([...rows].map((r) => r.dataset.source));
    if (sourceTiles) {
      const allTiles = [...sourceTiles.querySelectorAll(".source-tile")];
      allTiles.forEach((tile) => {
        tile.addEventListener("click", () => {
          const src = tile.dataset.source;
          const wasSoleActive = checkedSource.size === 1 && checkedSource.has(src);
          checkedSource.clear();
          allTiles.forEach((t) => t.classList.remove("active"));
          if (wasSoleActive) {
            allTiles.forEach((t) => {
              checkedSource.add(t.dataset.source);
              t.classList.add("active");
            });
          } else {
            checkedSource.add(src);
            tile.classList.add("active");
          }
          apply();
        });
      });
    }

    // Rank checkboxes, same pattern as items-filter.js's kind row.
    const ranks = [...new Set([...rows].map((r) => r.dataset.rank).filter(Boolean))];
    const rankOrder = { E: 0, D: 1, C: 2, B: 3, A: 4, S: 5 };
    ranks.sort((a, b) => (rankOrder[a] ?? 99) - (rankOrder[b] ?? 99));
    const checkedRank = new Set(ranks);
    if (rankFilters) {
      for (const rank of ranks) {
        const label = document.createElement("label");
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = true;
        cb.addEventListener("change", () => {
          if (cb.checked) checkedRank.add(rank);
          else checkedRank.delete(rank);
          apply();
        });
        label.appendChild(cb);
        label.append(" " + rank + "-Rank");
        rankFilters.appendChild(label);
      }
    }

    // Casting Action checkboxes: same compact always-visible row as Rank,
    // since there are only 4 buckets (see castingActionBucket in
    // jutsu_filters.go). Fixed display order rather than alphabetical/
    // as-observed, so it always reads "Action, Bonus Action, Reaction,
    // Special" regardless of which buckets happen to appear first in the DOM.
    const actionOrder = ["Action", "Bonus Action", "Reaction", "Special"];
    const actions = actionOrder.filter((a) => [...rows].some((r) => r.dataset.castingAction === a));
    const checkedAction = new Set(actions);
    if (actionFilters) {
      for (const action of actions) {
        const label = document.createElement("label");
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = true;
        cb.addEventListener("change", () => {
          if (cb.checked) checkedAction.add(action);
          else checkedAction.delete(action);
          apply();
        });
        label.appendChild(cb);
        label.append(" " + action);
        actionFilters.appendChild(label);
      }
    }

    // Origin checkboxes (Class / Clan), only on the pages whose rows are
    // tagged with data-jutsu-source — the creation step and the sheet's
    // jutsu library, where the list is a union of what the class casts and
    // what the clan teaches. /jutsu shows every jutsu in the book with no
    // such split, so it has no container for these and gets none.
    //
    // Deliberately not folded into the sourcebook tiles: those answer "which
    // book is this printed in", which is a different question from "is this
    // one of my clan's".
    const originLabels = { class: "Class", clan: "Clan", other: "Other" };
    const origins = ["class", "clan", "other"].filter((o) => [...rows].some((r) => r.dataset.jutsuSource === o));
    const checkedOrigin = new Set(origins);
    if (originFilters && origins.length > 1) {
      for (const origin of origins) {
        const label = document.createElement("label");
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = true;
        cb.addEventListener("change", () => {
          if (cb.checked) checkedOrigin.add(origin);
          else checkedOrigin.delete(origin);
          apply();
        });
        label.appendChild(cb);
        label.append(" " + (originLabels[origin] || origin));
        originFilters.appendChild(label);
      }
    }

    // Duration checkboxes, in the "Filters" dropdown since there are more of
    // these than fit in an always-visible row (see durationOrder in
    // jutsu_filters.go for the fixed sort order encoded as data-duration-order
    // on each row).
    const durationEntries = new Map(); // label -> order
    rows.forEach((r) => {
      if (r.dataset.duration) durationEntries.set(r.dataset.duration, Number(r.dataset.durationOrder) || 999);
    });
    const durations = [...durationEntries.keys()].sort((a, b) => durationEntries.get(a) - durationEntries.get(b));
    const checkedDuration = new Set(durations);
    const durationCbs = [];
    if (durationFilters) {
      for (const dur of durations) {
        const label = document.createElement("label");
        label.className = "dropdown-panel-item";
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = true;
        cb.addEventListener("change", () => {
          if (cb.checked) checkedDuration.add(dur);
          else checkedDuration.delete(dur);
          apply();
        });
        label.appendChild(cb);
        label.append(" " + dur);
        durationFilters.appendChild(label);
        durationCbs.push({ dur, cb });
      }
    }

    function setAllDuration(isChecked) {
      durationCbs.forEach(({ dur, cb }) => {
        cb.checked = isChecked;
        if (isChecked) checkedDuration.add(dur);
        else checkedDuration.delete(dur);
      });
      apply();
    }
    if (ids.durationSelectAll) {
      document.getElementById(ids.durationSelectAll)?.addEventListener("click", () => setAllDuration(true));
    }
    if (ids.durationDeselectAll) {
      document.getElementById(ids.durationDeselectAll)?.addEventListener("click", () => setAllDuration(false));
    }

    // Component checkboxes, in the "Filters" dropdown, fixed order + full
    // names matching the legend defined by componentNames in
    // jutsu_filters.go — important for Bukijutsu especially: narrowing to
    // just "W — Weapon" isolates every jutsu whose casting requires a
    // weapon in hand, regardless of what else it also requires.
    const componentLegend = {
      HS: "Hand Seals", CM: "Chakra Molding", CS: "Chakra Seals",
      M: "Mobility", W: "Weapon", NT: "Ninja Tools",
    };
    const componentOrder = ["HS", "CM", "CS", "M", "W", "NT"];
    const presentComponents = new Set();
    rows.forEach((r) => {
      (r.dataset.components || "").trim().split(/\s+/).filter(Boolean).forEach((c) => presentComponents.add(c));
    });
    const components = componentOrder.filter((c) => presentComponents.has(c));
    const checkedComponent = new Set(components);
    if (componentFilters) {
      for (const code of components) {
        const label = document.createElement("label");
        label.className = "dropdown-panel-item";
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = true;
        cb.addEventListener("change", () => {
          if (cb.checked) checkedComponent.add(code);
          else checkedComponent.delete(code);
          apply();
        });
        label.appendChild(cb);
        label.append(" " + code + " — " + (componentLegend[code] || code));
        componentFilters.appendChild(label);
      }
    }

    // Range slider: a stepped dual-thumb slider (two overlapping native
    // <input type=range>, index-based) over the sorted set of distinct
    // "primary range in feet" values actually present in the data, rather
    // than a raw linear feet scale — the real values cluster hard under
    // 300ft with a few outliers out to "10 Miles" (52800ft), so a linear
    // scale would squash every jutsu most players care about into a few
    // pixels at the left edge. Rows with a non-numeric range (Weapon Range /
    // Movement Speed / Special — see parseJutsuRange) sit outside this scale
    // entirely and are governed by the separate "Include..." checkbox instead
    // of the slider.
    const rangeStops = [...new Set(
      [...rows].filter((r) => r.dataset.rangeNumeric === "true").map((r) => Number(r.dataset.rangeFeet))
    )].sort((a, b) => a - b);

    function rangeLabel(feet) {
      if (feet >= 5280 && feet % 5280 === 0) {
        const miles = feet / 5280;
        return miles + (miles === 1 ? " Mile" : " Miles");
      }
      return feet + " ft";
    }

    let includeSpecial = true;
    if (rangeStops.length > 0 && rangeMinInput && rangeMaxInput) {
      rangeMinInput.max = rangeMaxInput.max = String(rangeStops.length - 1);
      rangeMinInput.value = "0";
      rangeMaxInput.value = String(rangeStops.length - 1);

      const updateRangeDisplay = () => {
        const minIdx = Number(rangeMinInput.value);
        const maxIdx = Number(rangeMaxInput.value);
        if (rangeMinLabel) rangeMinLabel.textContent = rangeLabel(rangeStops[minIdx]);
        if (rangeMaxLabel) rangeMaxLabel.textContent = rangeLabel(rangeStops[maxIdx]);
        if (rangeFill) {
          const span = rangeStops.length - 1 || 1;
          rangeFill.style.left = (minIdx / span) * 100 + "%";
          rangeFill.style.right = 100 - (maxIdx / span) * 100 + "%";
        }
      };

      rangeMinInput.addEventListener("input", () => {
        if (Number(rangeMinInput.value) > Number(rangeMaxInput.value)) rangeMinInput.value = rangeMaxInput.value;
        updateRangeDisplay();
        apply();
      });
      rangeMaxInput.addEventListener("input", () => {
        if (Number(rangeMaxInput.value) < Number(rangeMinInput.value)) rangeMaxInput.value = rangeMinInput.value;
        updateRangeDisplay();
        apply();
      });
      updateRangeDisplay();
    }
    if (rangeIncludeSpecial) {
      rangeIncludeSpecial.addEventListener("change", () => {
        includeSpecial = rangeIncludeSpecial.checked;
        apply();
      });
    }

    function matchesRange(r) {
      if (r.dataset.rangeNumeric !== "true") return includeSpecial;
      if (rangeStops.length === 0) return true;
      const feet = Number(r.dataset.rangeFeet);
      const minFeet = rangeStops[Number(rangeMinInput.value)];
      const maxFeet = rangeStops[Number(rangeMaxInput.value)];
      return feet >= minFeet && feet <= maxFeet;
    }

    // Set by the persistence block at the bottom, once the saved filters
    // have been restored — see cfg.persistKey.
    let persistState = null;

    function apply() {
      const q = search.value.trim().toLowerCase();
      rows.forEach((r) => {
        const matchesText = q === "" || r.dataset.name.toLowerCase().includes(q) ||
          (r.dataset.description || "").toLowerCase().includes(q);
        const matchesCategory = checkedCategory.has(keyFor(r.dataset.classification, r.dataset.category));
        const matchesSource = checkedSource.has(r.dataset.source);
        const matchesRank = !r.dataset.rank || checkedRank.has(r.dataset.rank);
        const matchesAction = checkedAction.has(r.dataset.castingAction);
        const matchesOrigin = !r.dataset.jutsuSource || checkedOrigin.has(r.dataset.jutsuSource);
        const matchesDuration = checkedDuration.has(r.dataset.duration);
        const comps = (r.dataset.components || "").trim().split(/\s+/).filter(Boolean);
        const matchesComponents = comps.length === 0 || comps.some((c) => checkedComponent.has(c));
        itemFor(r).hidden = !(matchesText && matchesCategory && matchesSource && matchesRank &&
          matchesAction && matchesOrigin && matchesDuration && matchesComponents && matchesRange(r));
      });
      // Subgroup and classification headings live in the same wrapper as
      // their rows (data-jutsu-subgroup/data-jutsu-group), so hiding the
      // whole wrapper hides its heading along with it. Scoped to this
      // filter's own list so two filters on one page can't fight.
      (cfg.groupSelectors || []).forEach((selector) => {
        list.querySelectorAll(selector).forEach((grp) => {
          grp.hidden = ![...grp.querySelectorAll(cfg.rowSelector || ".browse-row")].some((r) => !itemFor(r).hidden);
        });
      });
      if (cfg.onApply) cfg.onApply();
      if (persistState) persistState();
    }

    async function selectRow(row) {
      for (const r of rows) r.classList.remove("active");
      row.classList.add("active");
      if (!cfg.fragmentSwap || !detailPane) return;
      try {
        const url = new URL(row.href, window.location.href);
        url.searchParams.set("fragment", "1");
        const res = await fetch(url);
        if (!res.ok) return;
        detailPane.innerHTML = await res.text();
      } catch (e) {
        console.error("jutsu: fragment fetch failed", e);
      }
    }

    for (const row of rows) {
      row.addEventListener("click", (e) => {
        e.preventDefault();
        selectRow(row);
      });
    }

    search.addEventListener("input", apply);
    // The sheet's whole library toolbar+list lives inside one <form> (each
    // row's own "+" is a submit button targeting that same form, keyed by
    // its own slug) — pressing Enter in a text <input> with no button
    // explicitly focused triggers the browser's native "submit via the
    // first submit button in the form" behavior, which silently added
    // whatever jutsu happened to be first in DOM order. This input's job is
    // filtering, never submitting, so Enter here is always a no-op.
    search.addEventListener("keydown", (e) => {
      if (e.key === "Enter") e.preventDefault();
    });

    toggle.addEventListener("click", () => {
      const opening = panel.hidden;
      panel.hidden = !opening;
      toggle.setAttribute("aria-expanded", String(opening));
    });
    document.addEventListener("click", (e) => {
      if (!e.target.closest("#" + ids.categoryFilter)) {
        panel.hidden = true;
        toggle.setAttribute("aria-expanded", "false");
      }
    });

    if (detailsToggle && detailsPanel) {
      detailsToggle.addEventListener("click", () => {
        const opening = detailsPanel.hidden;
        detailsPanel.hidden = !opening;
        detailsToggle.setAttribute("aria-expanded", String(opening));
      });
      document.addEventListener("click", (e) => {
        if (!e.target.closest("#" + ids.detailsFilter)) {
          detailsPanel.hidden = true;
          detailsToggle.setAttribute("aria-expanded", "false");
        }
      });
    }

    if (cfg.readQueryParam) {
      const q = new URLSearchParams(location.search).get("q");
      if (q) {
        search.value = q;
        apply();
        const visible = [...rows].filter((r) => !itemFor(r).hidden);
        if (visible.length === 1) selectRow(visible[0]);
      }
    }

    // ---- Remembering the filters across reloads (cfg.persistKey) ----
    //
    // Only the character sheet passes a key. Learning a jutsu from its
    // library reloads the whole page (sheet-inventory.js explains why), and
    // rebuilding these controls from scratch threw the player's filters away
    // at the exact moment they were working down a filtered list — the item
    // library had the same bug, and this pane is the same bug with a bigger
    // toolbar. sessionStorage rather than
    // localStorage, matching sheet-tabs.js: remembered for the session, not
    // pinned to the browser forever.
    //
    // Checkboxes are keyed by container id plus label text rather than by
    // index, so a rules update that adds a category or a rank shifts nothing
    // — an unrecognised key simply isn't in the saved map, and anything not
    // in the map restores to "on" rather than to invisibly filtered out.
    if (cfg.persistKey) {
      const storeKey = "n5e:jutsu-filter:" + cfg.persistKey;
      const boxContainers = [panel, rankFilters, actionFilters, originFilters, durationFilters, componentFilters]
        .filter(Boolean);

      const eachBox = (fn) => {
        boxContainers.forEach((container) => {
          container.querySelectorAll("input[type=checkbox]").forEach((cb) => {
            const label = cb.closest("label");
            fn(cb, container.id + "|" + (label ? label.textContent.trim() : ""));
          });
        });
      };

      const allTiles = sourceTiles ? [...sourceTiles.querySelectorAll(".source-tile")] : [];

      let saved = null;
      try {
        const raw = sessionStorage.getItem(storeKey);
        saved = raw ? JSON.parse(raw) : null;
      } catch (err) {
        // Private browsing refuses to read, and a stale or truncated value
        // throws in JSON.parse. Neither is worth breaking the filters over.
        saved = null;
      }

      if (saved) {
        if (typeof saved.search === "string") search.value = saved.search;
        eachBox((cb, key) => {
          const want = saved.boxes && key in saved.boxes ? saved.boxes[key] : true;
          if (cb.checked !== want) {
            cb.checked = want;
            cb.dispatchEvent(new Event("change", { bubbles: true }));
          }
        });
        if (rangeIncludeSpecial && typeof saved.special === "boolean" && rangeIncludeSpecial.checked !== saved.special) {
          rangeIncludeSpecial.checked = saved.special;
          rangeIncludeSpecial.dispatchEvent(new Event("change", { bubbles: true }));
        }
        [[rangeMinInput, saved.rangeMin], [rangeMaxInput, saved.rangeMax]].forEach(([input, value]) => {
          if (input && typeof value === "string" && input.value !== value) {
            input.value = value;
            input.dispatchEvent(new Event("input", { bubbles: true }));
          }
        });
        // The tiles are an exclusive select (see their click handler), so
        // the only reachable states are "every book" and "exactly one" —
        // one click reproduces either.
        if (saved.sources && saved.sources.length === 1) {
          const tile = allTiles.find((t) => t.dataset.source === saved.sources[0]);
          if (tile) tile.click();
        }
        apply();
      }

      // Armed only now, so restoring above doesn't write half-restored
      // states back over the saved one on the way through.
      persistState = () => {
        const boxes = {};
        eachBox((cb, key) => {
          boxes[key] = cb.checked;
        });
        try {
          sessionStorage.setItem(storeKey, JSON.stringify({
            search: search.value,
            boxes,
            special: rangeIncludeSpecial ? rangeIncludeSpecial.checked : true,
            rangeMin: rangeMinInput ? rangeMinInput.value : null,
            rangeMax: rangeMaxInput ? rangeMaxInput.value : null,
            sources: allTiles.filter((t) => t.classList.contains("active")).map((t) => t.dataset.source),
          }));
        } catch (err) {
          /* see the read above */
        }
      };
    }

    return { apply };
  }

  // Element ids differ only by prefix between the two pages, so both
  // configs are built from one template rather than written out twice —
  // adding a filter control means adding one id here, not two.
  function idsWithPrefix(prefix, listId, detailPaneId) {
    return {
      search: prefix + "search",
      categoryFilter: prefix + "category-filter",
      categoryToggle: prefix + "category-toggle",
      categoryPanel: prefix + "category-panel",
      detailsFilter: prefix + "details-filter",
      detailsToggle: prefix + "details-toggle",
      detailsPanel: prefix + "details-panel",
      sourceTiles: prefix + "source-tiles",
      rankFilters: prefix + "rank-filters",
      actionFilters: prefix + "action-filters",
      originFilters: prefix + "origin-filters",
      durationFilters: prefix + "duration-filters",
      durationSelectAll: prefix + "duration-select-all",
      durationDeselectAll: prefix + "duration-deselect-all",
      componentFilters: prefix + "component-filters",
      rangeMin: prefix + "range-min",
      rangeMax: prefix + "range-max",
      rangeFill: prefix + "range-fill",
      rangeMinLabel: prefix + "range-min-label",
      rangeMaxLabel: prefix + "range-max-label",
      rangeIncludeSpecial: prefix + "range-include-special",
      list: listId,
      detailPane: detailPaneId,
    };
  }

  // /jutsu — rows are plain links, clicking one swaps the detail pane.
  initJutsuFilter({
    ids: idsWithPrefix("jutsu-", "browse-list", "browse-detail-pane"),
    groupSelectors: ["[data-jutsu-subgroup]", "[data-jutsu-group]"],
    fragmentSwap: true,
    readQueryParam: true,
  });

  // The creation step — same filters, but each row is a checkbox plus a
  // link inside a .jutsu-choice-row, so the wrapper is what gets hidden,
  // and the running "N / M selected" counter has to be recomputed after
  // every filter pass (create-jutsu.js owns that counter and exposes the
  // hook, since it also owns the Select all/Deselect all buttons that only
  // act on currently-visible rows).
  window.n5eCreateJutsuFilter = initJutsuFilter({
    ids: idsWithPrefix("create-jutsu-", "create-jutsu-list", "creation-detail-pane"),
    itemSelector: ".jutsu-choice-row",
    groupSelectors: ["[data-jutsu-group]"],
    fragmentSwap: true,
    onApply: () => {
      if (window.n5eCreateJutsuCounts) window.n5eCreateJutsuCounts();
    },
  });

  // The character sheet's Jutsu tab. Same filters again, over a pane whose
  // rows are a link plus an "add" button inside a .sheet-lib-row wrapper —
  // so the wrapper is what gets hidden, as on the creation step.
  //
  // No fragment swap: this pane has no detail column. Clicking a row opens
  // the jutsu's card in the sheet's popup dialog instead (sheet-popup.js,
  // delegated from the document, so the preventDefault below doesn't stop
  // it).
  const sheetChat = document.querySelector(".sheet-chat-panel");
  initJutsuFilter({
    ids: idsWithPrefix("sheet-jutsu-", "sheet-jutsu-library-list", null),
    itemSelector: ".sheet-lib-row",
    groupSelectors: ["[data-jutsu-subgroup]", "[data-jutsu-group]"],
    fragmentSwap: false,
    // Per character, because learning a jutsu reloads the page and would
    // otherwise reset the toolbar mid-task. /jutsu and the creation step
    // pass no key: neither reloads under the player, and a browse page that
    // silently reopens filtered is worse than one that starts clean.
    persistKey: "sheet:" + (sheetChat ? sheetChat.dataset.characterId : "unknown"),
  });
})();
