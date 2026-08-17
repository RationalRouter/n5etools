// Drag-reorder + horizontal/vertical flip for the small tile groups nested
// inside a panel — Prof Bonus/Initiative/Speed/Hit Dice, Ability Scores,
// Saving Throws (part 11: "let the boxes inside the panel be reordered and
// reoriented", with Ability Scores flipping to a vertical Roll20-style
// stack as the named example).
//
// This is a much smaller mechanism than sheet-layout.js's box-level grid
// engine (sheet-grid.js/N5eGrid): these tiles only ever reorder within
// their own single row/column (no free 2D placement, no per-tile resize)
// and only ever flip between a horizontal row and a vertical column, so
// plain DOM order plus a CSS class is enough — no occupancy grid needed.
//
// Fully event-delegated on `document` rather than wired per-element at
// load. #sheet-squares is one of the fragments the sheet swaps wholesale
// via outerHTML after an HP/rest/speed edit (see sheet-refresh.js), and
// listeners attached directly to nodes at load go silently dead the moment
// that swap happens — see sheet-rest.js's history of exactly this bug.
// Delegation sidesteps it entirely; a MutationObserver below only has to
// re-apply saved order/orientation to whatever fresh markup lands, not
// re-wire any events.
//
// "Is Edit Layout on" is read straight off the ancestor
// `.sheet-layout-editing` class sheet-layout.js already maintains on the
// tab's `[data-sheet-layout]` element (that element itself is never
// swapped, only things inside it) — not tracked separately here — so this
// file has no load-order dependency on sheet-layout.js either.
//
// Order/orientation is persisted server-side in character_sheet_ui_state
// (see 0015_sheet_ui_state.sql), under keys "subgrid:<name>" — NOT
// localStorage, as of 2026-07-27, and for the identical reason
// sheet-layout.js's own grid state moved: a fresh OS-assigned port on every
// app restart makes localStorage (scoped per-origin, port included) forget
// everything. See that file's header comment for the full story; this file
// mirrors its readUIState/postUIState/one-time-migration shape exactly
// rather than sharing code with it, consistent with this codebase's
// small-self-contained-file convention elsewhere (e.g. both files already
// compute `characterID` independently the same way).
(function () {
  const idPanel = document.querySelector("[data-character-id]");
  const characterID = () => (idPanel ? idPanel.dataset.characterId : "unknown");

  const uiState = readUIState();
  function readUIState() {
    const el = document.getElementById("sheet-ui-state-data");
    if (!el) return {};
    try {
      return JSON.parse(el.textContent) || {};
    } catch (err) {
      return {};
    }
  }

  function postUIState(key, data) {
    const body = new URLSearchParams();
    body.set("key", key);
    body.set("data", JSON.stringify(data));
    return fetch("/characters/" + characterID() + "/sheet/ui-state", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: body.toString(),
    });
  }

  function isEditing(container) {
    const layout = container.closest("[data-sheet-layout]");
    return !!layout && layout.classList.contains("sheet-layout-editing");
  }

  function items(container) {
    return Array.from(container.querySelectorAll(":scope > [data-subitem]"));
  }

  function toggleBtn(container) {
    return container.querySelector(":scope > .sheet-subgrid-orient-toggle");
  }

  function stateKey(container) {
    return "subgrid:" + container.dataset.subgrid;
  }

  // Read exactly once per container as a migration fallback — the OLD
  // localStorage key, from whichever port this browser last saved under.
  function oldStorageKey(container) {
    return "n5e:sheet-subgrid:" + container.dataset.subgrid + ":" + characterID();
  }

  function applyState(container) {
    const key = stateKey(container);
    let saved = uiState[key] || null;

    if (!saved) {
      let legacy = null;
      try {
        legacy = JSON.parse(localStorage.getItem(oldStorageKey(container)));
      } catch (err) {
        legacy = null;
      }
      if (legacy) {
        saved = legacy;
        // Adopt into the in-memory cache too, so a later MutationObserver
        // re-application of this same container (e.g. after an unrelated
        // fragment swap) doesn't need to re-read localStorage a second time.
        uiState[key] = legacy;
        postUIState(key, legacy).catch((err) => {
          console.warn("could not migrate saved subgrid layout to the server:", err);
        });
        try {
          localStorage.removeItem(oldStorageKey(container));
        } catch (err) {
          // Nothing to clean up if this fails — worst case the orphaned key
          // sits unread forever, same as it already was.
        }
      }
    }

    if (!saved) return;
    container.classList.toggle("sheet-subgrid-vertical", !!saved.vertical);
    if (!Array.isArray(saved.order)) return;
    const btn = toggleBtn(container);
    saved.order.forEach((id) => {
      const item = container.querySelector(':scope > [data-subitem="' + CSS.escape(id) + '"]');
      // Always re-insert right before the toggle button, so the toggle
      // (appended last in the template) stays the last child no matter
      // what order the tiles end up in.
      if (item) container.insertBefore(item, btn || null);
    });
  }

  function saveState(container) {
    postUIState(stateKey(container), {
      order: items(container).map((i) => i.dataset.subitem),
      vertical: container.classList.contains("sheet-subgrid-vertical"),
    }).catch((err) => {
      console.warn("could not save sheet subgrid layout:", err);
    });
  }

  document.querySelectorAll("[data-subgrid]").forEach(applyState);

  // Any later fragment swap (#sheet-squares after an HP/rest/speed edit)
  // re-renders the container from scratch, in template order — put back
  // whatever the player last dragged/flipped.
  new MutationObserver((mutations) => {
    mutations.forEach((m) => {
      m.addedNodes.forEach((node) => {
        if (node.nodeType !== 1) return;
        if (node.matches && node.matches("[data-subgrid]")) applyState(node);
        if (node.querySelectorAll) node.querySelectorAll("[data-subgrid]").forEach(applyState);
      });
    });
  }).observe(document.body, { childList: true, subtree: true });

  let dragging = null;

  document.addEventListener("mousedown", (e) => {
    const handle = e.target.closest(".sheet-subitem-handle");
    if (!handle) return;
    const item = handle.closest("[data-subitem]");
    const container = item && item.closest("[data-subgrid]");
    if (!container || !isEditing(container)) return;
    item.draggable = true;
  });
  // Every subitem handle sits inside a .rollable tile (character-sheet.js
  // delegates click-to-roll off `document` too, and registers first —
  // script order alone can't make a later bubble-phase listener here win).
  // Capture phase runs before ANY bubble-phase listener anywhere, target
  // included, so stopping propagation here reliably keeps a bare click on
  // the handle from also rolling the tile underneath it — same problem the
  // ability-score edit button already solves, just via a phase that
  // doesn't depend on registration order instead of a listener on the
  // button itself.
  document.addEventListener("click", (e) => {
    const handle = e.target.closest(".sheet-subitem-handle");
    if (!handle) return;
    const item = handle.closest("[data-subitem]");
    const container = item && item.closest("[data-subgrid]");
    if (!container || !isEditing(container)) return;
    e.stopPropagation();
  }, { capture: true });
  // Same "any mouseup disarms everything" reasoning as sheet-layout.js's
  // box-level handles: the gesture can end outside the handle, and a tile
  // left draggable=true would hijack its own next click-to-roll.
  document.addEventListener("mouseup", () => {
    document.querySelectorAll('[data-subitem][draggable="true"]').forEach((i) => { i.draggable = false; });
  });

  document.addEventListener("dragstart", (e) => {
    const item = e.target.closest("[data-subitem]");
    if (!item || !item.draggable) return;
    const container = item.closest("[data-subgrid]");
    if (!container || !isEditing(container)) { e.preventDefault(); return; }
    dragging = item;
    item.classList.add("sheet-subitem-dragging");
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", "");
  });
  document.addEventListener("dragend", (e) => {
    const item = e.target.closest("[data-subitem]");
    if (!item) return;
    item.classList.remove("sheet-subitem-dragging");
    item.draggable = false;
    const container = item.closest("[data-subgrid]");
    if (dragging && container) saveState(container);
    dragging = null;
  });
  document.addEventListener("dragover", (e) => {
    if (!dragging) return;
    const container = e.target.closest("[data-subgrid]");
    if (!container || container !== dragging.closest("[data-subgrid]")) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";

    const vertical = container.classList.contains("sheet-subgrid-vertical");
    const after = items(container)
      .filter((i) => i !== dragging)
      .find((sib) => {
        const rect = sib.getBoundingClientRect();
        const mid = vertical ? rect.top + rect.height / 2 : rect.left + rect.width / 2;
        const pos = vertical ? e.clientY : e.clientX;
        return pos < mid;
      });
    container.insertBefore(dragging, after || toggleBtn(container) || null);
  });
  document.addEventListener("drop", (e) => {
    if (e.target.closest("[data-subgrid]")) e.preventDefault();
  });

  document.addEventListener("click", (e) => {
    const btn = e.target.closest(".sheet-subgrid-orient-toggle");
    if (!btn) return;
    const container = btn.closest("[data-subgrid]");
    if (!container || !isEditing(container)) return;
    container.classList.toggle("sheet-subgrid-vertical");
    saveState(container);
  });
})();
