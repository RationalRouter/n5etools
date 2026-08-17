// The character sheet's three library tabs: filtering for the item and feat
// panes, and drag-and-drop from any library pane onto the "what you have"
// pane beside it.
//
// Drag and drop is layered over controls that already work. Every library
// row carries a real <button type="submit" name="…" value="{slug}"> inside a
// real <form>, so clicking it, tabbing to it, or running with JavaScript off
// all add the row the same way. Dropping a row simply clicks that button.
// That matters beyond principle here: drag and drop is unusable with a
// screen reader and awkward on a touchscreen, so it can't be the only way
// in.
//
// The jutsu pane's filters are not here — that pane reuses the /jutsu
// library's entire toolbar through jutsu-filter.js's factory (see its
// "sheet-jutsu-" call site). This file's filter is the simpler
// search-plus-kind-checkboxes pair the items and feats panes need, which is
// the same shape items-filter.js gives /items.
(function () {
  // ---- Filtering (item and feat panes) ----

  // Adding an item reloads the whole page (see sheet-inventory.js — equipping
  // armor moves AC, equipping a weapon adds an attack row, and nothing short
  // of a re-render keeps those honest). That reload used to rebuild these
  // checkboxes all-on, so filtering down to Armor and clicking + on one piece
  // threw the filter away at the exact moment the player was working through
  // a filtered list. The state is remembered per pane instead, in
  // sessionStorage, the same way sheet-tabs.js remembers the open tab.
  const chatPanel = document.querySelector(".sheet-chat-panel");
  const characterID = chatPanel ? chatPanel.dataset.characterId : "unknown";

  function loadFilterState(key) {
    try {
      const raw = sessionStorage.getItem("n5e:sheet-lib-filter:" + characterID + ":" + key);
      return raw ? JSON.parse(raw) : null;
    } catch (err) {
      // Private-browsing modes refuse to read/write, and a half-written or
      // stale value would throw in JSON.parse. Neither is worth breaking
      // the filters over — fall back to "everything on".
      return null;
    }
  }

  function saveFilterState(key, state) {
    try {
      sessionStorage.setItem("n5e:sheet-lib-filter:" + characterID + ":" + key, JSON.stringify(state));
    } catch (err) {
      /* see loadFilterState */
    }
  }

  document.querySelectorAll(".sheet-library-list[data-search-input]").forEach((list) => {
    const search = document.getElementById(list.dataset.searchInput);
    const kindFilters = document.getElementById(list.dataset.kindFilters);
    const selectAllBtn = document.getElementById(list.dataset.selectAll);
    const deselectAllBtn = document.getElementById(list.dataset.deselectAll);
    const showAll = document.getElementById(list.dataset.showAll);
    const showAllLabel = document.getElementById(list.dataset.showAllLabel);
    const rows = [...list.querySelectorAll(".sheet-lib-row")];
    if (!search || !kindFilters || rows.length === 0) return;

    // Feats the character doesn't qualify for are hidden by default, but the
    // toggle that brings them back only appears on a pane that actually has
    // some — the items pane has no prerequisites to
    // fail, so it would otherwise carry a control that never does anything.
    const hasIneligible = rows.some((r) => r.dataset.eligible === "0");
    if (showAllLabel) showAllLabel.hidden = !hasIneligible;
    if (showAll && hasIneligible) showAll.addEventListener("change", apply);

    const storageKey = list.dataset.filterKey || list.id;
    const kinds = [...new Set(rows.map((r) => r.dataset.kind))];

    // A remembered kind that no longer exists is dropped, and a kind that
    // did not exist when the state was saved defaults to on — so a rules
    // update that adds an item category can't leave it invisibly filtered
    // out with no checkbox ever having been unticked.
    const saved = loadFilterState(storageKey);
    const savedKinds = saved && Array.isArray(saved.kinds) ? new Set(saved.kinds) : null;
    const checked = new Set(savedKinds ? kinds.filter((k) => savedKinds.has(k)) : kinds);
    if (saved && typeof saved.search === "string") search.value = saved.search;
    if (saved && showAll && typeof saved.showAll === "boolean") showAll.checked = saved.showAll;

    const boxes = new Map();
    for (const kind of kinds) {
      const label = document.createElement("label");
      const cb = document.createElement("input");
      cb.type = "checkbox";
      cb.checked = checked.has(kind);
      cb.addEventListener("change", () => {
        if (cb.checked) checked.add(kind);
        else checked.delete(kind);
        apply();
      });
      label.appendChild(cb);
      label.append(" " + kind);
      kindFilters.appendChild(label);
      boxes.set(kind, cb);
    }

    // Simple/Martial/Exotic and Light/Medium/Heavy for the inventory pane.
    // Persisted alongside the kind boxes and for the same reason: adding an
    // item reloads the sheet, and a filter thrown away mid-task is exactly
    // what the sessionStorage above exists to prevent.
    const subfilters = window.n5eEquipmentSubfilters({
      container: document.getElementById(list.dataset.subfilters),
      rows,
      onChange: () => apply(),
      initial: saved ? saved.subfiltersOff : null,
    });

    function setAll(on) {
      checked.clear();
      for (const [kind, cb] of boxes) {
        cb.checked = on;
        if (on) checked.add(kind);
      }
      apply();
    }
    if (selectAllBtn) selectAllBtn.addEventListener("click", () => setAll(true));
    if (deselectAllBtn) deselectAllBtn.addEventListener("click", () => setAll(false));

    function apply() {
      const unlocked = !!(showAll && showAll.checked);
      saveFilterState(storageKey, {
        kinds: [...checked],
        search: search.value,
        showAll: unlocked,
        subfiltersOff: subfilters.state(),
      });
      const q = search.value.trim().toLowerCase();
      for (const row of rows) {
        // Description as well as name, so "poison" finds the antitoxin and
        // "advantage on stealth" finds the feat that grants it — the same
        // name-and-body match the jutsu filter makes.
        const haystack = (row.dataset.name + " " + (row.dataset.search || "")).toLowerCase();
        // Eligibility is folded into this one pass rather than written by a
        // second listener: two things independently setting row.hidden
        // fight, and whichever ran last wins.
        const eligible = unlocked || row.dataset.eligible !== "0";
        row.hidden = !((q === "" || haystack.includes(q)) && checked.has(row.dataset.kind) && eligible
          && subfilters.matches(row));
      }
      for (const group of list.querySelectorAll("[data-lib-group]")) {
        group.hidden = ![...group.querySelectorAll(".sheet-lib-row")].some((r) => !r.hidden);
      }
    }

    search.addEventListener("input", apply);
    // Same fix as jutsu-filter.js's search input: this whole pane (feats
    // and items alike) lives inside one <form> with each row's own "+" as
    // a real submit button (see sheet_library_pane's own doc — "the pane
    // works with no JavaScript at all"), so Enter with no button focused
    // triggers the browser's native "submit via the first submit button"
    // default, silently taking/equipping whatever row happened to be first.
    search.addEventListener("keydown", (e) => {
      if (e.key === "Enter") e.preventDefault();
    });

    // Unconditional: the server renders every row visible, so both a
    // restored filter AND the default hiding of ineligible rows only take
    // effect once this has run at least once.
    apply();
  });

  // ---- Drag and drop (all three panes) ----

  // The row currently being dragged. Kept here rather than read back out of
  // dataTransfer on drop because dataTransfer.getData is unreadable during
  // dragover in most browsers, which is where the drop target has to decide
  // whether to accept — and because the element itself is what the drop
  // handler needs, not just its slug.
  let dragging = null;

  // closest() lives on Element, and a drag event's target can be a
  // non-Element node while the pointer is between boxes. Reaching straight
  // for e.target.closest throws there, which kills the listener for the rest
  // of that drag.
  function closestFrom(node, selector) {
    return node instanceof Element ? node.closest(selector) : null;
  }
  function paneUnder(node) {
    return closestFrom(node, "[data-drop-into]");
  }

  document.addEventListener("dragstart", (e) => {
    const row = closestFrom(e.target, ".sheet-lib-row");
    if (!row) return;
    dragging = row;
    row.classList.add("sheet-lib-row-dragging");
    // Firefox refuses to start a drag at all unless some data is set.
    e.dataTransfer.setData("text/plain", row.dataset.slug || "");
    e.dataTransfer.effectAllowed = "copy";
    // Light up the pane this row can actually be dropped on for the whole
    // drag, not just once the pointer is over it. Previously, dragging a
    // feat into an empty box appeared to do nothing: the feats and jutsu
    // panes are only as tall as their contents, so an empty one is a
    // 3-line strip beside a 60vh-tall library, and a drop aimed at the
    // height you grabbed from lands on nothing. Showing the target's real
    // extent up front is what makes that miss visible instead of silent —
    // the min-height in app.css is the other half of the same fix.
    const add = row.querySelector(".sheet-lib-add");
    for (const pane of document.querySelectorAll("[data-drop-into]")) {
      if (add && add.form && add.form.id === pane.dataset.dropInto) {
        pane.classList.add("sheet-library-owned-armed");
      }
    }
  });

  document.addEventListener("dragend", () => {
    if (dragging) dragging.classList.remove("sheet-lib-row-dragging");
    dragging = null;
    document.querySelectorAll(".sheet-library-owned-over, .sheet-library-owned-armed").forEach((el) => {
      el.classList.remove("sheet-library-owned-over", "sheet-library-owned-armed");
    });
  });

  document.addEventListener("dragover", (e) => {
    if (!dragging) return;
    const target = paneUnder(e.target);
    if (!target) {
      // Swallow the drag even away from a pane. Every library row wraps a
      // real <a href>, so a drag that starts on one IS a link drag as far
      // as the browser is concerned, and dropping a link on the page
      // navigates to it — the sheet vanishing on a missed drop reads as
      // "drag and drop is broken", not as "you missed".
      e.preventDefault();
      e.dataTransfer.dropEffect = "none";
      return;
    }
    // preventDefault on dragover is what marks an element as a valid drop
    // target; without it the browser refuses the drop outright.
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    target.classList.add("sheet-library-owned-over");
  });

  document.addEventListener("dragleave", (e) => {
    const target = paneUnder(e.target);
    // relatedTarget is where the pointer went; still inside the pane means
    // this is a move between its children, not a real exit.
    if (!target || (e.relatedTarget && target.contains(e.relatedTarget))) return;
    target.classList.remove("sheet-library-owned-over");
  });

  document.addEventListener("drop", (e) => {
    if (!dragging) return;
    e.preventDefault();
    const target = paneUnder(e.target);
    if (!target) return;
    target.classList.remove("sheet-library-owned-over");

    // Only accept a row from the form this pane is paired with, so dragging
    // a jutsu onto the inventory can't post a jutsu slug to the item
    // endpoint.
    const form = document.getElementById(target.dataset.dropInto);
    const add = dragging.querySelector(".sheet-lib-add");
    if (!form || !add || add.form !== form) return;
    add.click();
  });
})();
