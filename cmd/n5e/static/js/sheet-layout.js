// Gridstack.js-backed dashboard editing for the sheet's box layouts (Core,
// Bio, Puppets, Companions — every [data-sheet-layout] section). Replaces the
// old hand-rolled drag/resize/occupancy engine (see sheet-grid.js's own
// header comment) with vendor/gridstack-all.js, for the smoother, grid-snap
// drag/resize feel a real dashboard library provides.
//
// Existing template markup is untouched: every .sheet-box[data-box-id] stays
// exactly where the Go template renders it. This file wraps each one in a
// Gridstack-required <div class="grid-stack-item"><!-- .sheet-box moves in
// here, tagged .grid-stack-item-content --></div> at init time instead, so
// no template needed touching across Core/Bio/Puppets/Companions.
//
// The layout is a per-character display preference, persisted server-side in
// character_sheet_ui_state under keys "grid:core"/"grid:bio"/"grid:puppets"/
// "grid:summons" — see sheet-grid.js for the wire format and its backward
// compatibility with layouts saved under the pre-Gridstack engine.
(function () {
  const ROW_HEIGHT = window.N5eGrid.ROW_HEIGHT; // px, == Gridstack cellHeight
  const ROW_GAP = window.N5eGrid.ROW_GAP; // px, split across marginTop+marginBottom below
  const COL_GAP_PX = 28; // px, == app.css's old 1.75rem column-gap, split across marginLeft+marginRight
  const CM_TO_PX = 96 / 2.54; // 96px/in ÷ 2.54cm/in, same conversion the browser itself uses for a `cm` length
  const UNDO_LIMIT = 20;

  const idPanel = document.querySelector("[data-character-id]");
  const characterID = idPanel ? idPanel.dataset.characterId : "unknown";
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
    return fetch("/characters/" + characterID + "/sheet/ui-state", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
      body: body.toString(),
    });
  }

  // Temporarily lifts a box's Gridstack-managed absolute positioning/height
  // clamp and overflow clipping so it can size itself to its own content for
  // one synchronous measurement, then puts both back exactly as they were.
  // Needed anywhere this file wants a box's real (unclamped) content height —
  // computeDefaultState measures BEFORE any box is wrapped as a Gridstack
  // item, so it never needs this; every measurement AFTER init does.
  function measureNaturalHeight(contentEl) {
    const prevPosition = contentEl.style.position;
    const prevHeight = contentEl.style.height;
    const prevOverflowY = contentEl.style.overflowY;
    contentEl.style.position = "static";
    contentEl.style.height = "auto";
    contentEl.style.overflowY = "visible";
    const height = contentEl.getBoundingClientRect().height;
    contentEl.style.position = prevPosition;
    contentEl.style.height = prevHeight;
    contentEl.style.overflowY = prevOverflowY;
    return height;
  }

  function rowsForPx(px) {
    return Math.max(1, Math.ceil((px + ROW_GAP) / (ROW_HEIGHT + ROW_GAP)));
  }

  function rectOverlaps(a, b) {
    return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
  }
  function anyOverlap(rects) {
    for (let i = 0; i < rects.length; i++) {
      for (let j = i + 1; j < rects.length; j++) {
        if (rectOverlaps(rects[i], rects[j])) return true;
      }
    }
    return false;
  }

  // See growCallbacks' own doc below (one shared map, not per-layout, since
  // a caller outside this file only knows a box's id, not which named
  // layout it belongs to).
  const growCallbacks = new Map();
  window.n5eGrowSheetBox = function (boxId) {
    const grow = growCallbacks.get(boxId);
    if (grow) grow();
  };

  document.querySelectorAll("[data-sheet-layout]").forEach((layout) => {
    initLayout(layout);
  });

  function initLayout(layout) {
    const gridRoot = layout.querySelector(":scope > form, :scope > .sheet-core-grid") || layout;
    const columnEls = Array.from(gridRoot.querySelectorAll(":scope > .sheet-col[data-column]"));
    if (columnEls.length === 0) return;

    const stateKey = "grid:" + layout.dataset.sheetLayout;
    const oldStorageKey = "n5e:sheet-layout:grid:" + layout.dataset.sheetLayout + ":" + characterID;
    const boxes = () => Array.from(gridRoot.querySelectorAll(".sheet-box[data-box-id]"));

    let editing = false;
    let undoStack = [];
    let suppressToggleHandler = false;
    let suppressSave = false;
    let saveTimer = null;
    let preActionSnapshot = null;

    function snapshotDetailsOpen() {
      const m = new Map();
      boxes().forEach((b) => {
        if (b.tagName === "DETAILS") m.set(b.dataset.boxId, b.open);
      });
      return m;
    }
    function updateUndoButton() {
      if (undoBtn) undoBtn.disabled = undoStack.length === 0;
    }

    // Every currently-collapsed <details> box's own PRE-collapse (real,
    // open) row height — see the toggle listener below and saveLayout's own
    // doc for why a collapsed-only height must never reach the server: a
    // <details> box's `open` attribute is always hardcoded open server-side
    // (no "stays collapsed across reload" concept exists), so a saved
    // collapsed height would come back on next load OPEN (full content) but
    // pinned to the CLOSED size.
    const preCollapseHeight = new Map();

    // The default layout a fresh page (or a box the saved data doesn't know
    // about) should get: each .sheet-col becomes an even band of the grid
    // (12 / number-of-bands, any remainder folded into the last band), boxes
    // stack top-to-bottom within their band in template order — exactly
    // reproducing today's visual grouping. Runs BEFORE any box is wrapped as
    // a Gridstack item, while boxes are still plain flex children of their
    // .sheet-col — the same measurement context the pre-Gridstack engine
    // used, unaffected by this rewrite.
    function computeDefaultState() {
      // Only one tab's <section> is visible at a time; a display:none
      // ancestor measures 0x0, which would floor every box's height here at
      // 1 row regardless of actual content. Temporarily un-hiding for this
      // synchronous pass is invisible to the user — nothing yields back to
      // the browser's paint loop between the two attribute writes.
      const wasHidden = layout.hidden;
      if (wasHidden) layout.hidden = false;

      const totalCols = window.N5eGrid.TOTAL_COLS;
      const bandWidth = Math.floor(totalCols / columnEls.length);
      const gridWidthPx = gridRoot.getBoundingClientRect().width;
      const colWidthPx = gridWidthPx / totalCols;
      const next = new Map();

      columnEls.forEach((col, colIndex) => {
        const colStart = colIndex * bandWidth; // 0-based
        const bandColSpan = colIndex === columnEls.length - 1 ? totalCols - colStart : bandWidth;
        let rowCursor = 0; // 0-based
        Array.from(col.querySelectorAll(":scope > .sheet-box[data-box-id]")).forEach((box) => {
          const boxId = box.dataset.boxId;
          const defaultWCm = box.dataset.defaultWCm ? parseFloat(box.dataset.defaultWCm) : null;
          const defaultHCm = box.dataset.defaultHCm ? parseFloat(box.dataset.defaultHCm) : null;
          const minColSpan = box.dataset.minW ? parseInt(box.dataset.minW, 10) : 1;

          const w = defaultWCm && colWidthPx
            ? Math.max(1, Math.round((defaultWCm * CM_TO_PX) / colWidthPx))
            : Math.min(totalCols, Math.max(bandColSpan, minColSpan));

          let h;
          if (defaultHCm) {
            h = rowsForPx(defaultHCm * CM_TO_PX);
          } else {
            // Measure at the box's real assigned width, not its current
            // (possibly narrower band) width — a box widened past its band
            // (e.g. minColSpan) would otherwise measure content wrapped at
            // the narrower width and get an inflated row count.
            let restoreWidth = null;
            if (colWidthPx && w !== bandColSpan) {
              restoreWidth = box.style.width;
              box.style.width = (w * colWidthPx) + "px";
            }
            const naturalHeight = box.getBoundingClientRect().height;
            if (restoreWidth !== null) box.style.width = restoreWidth;
            h = rowsForPx(naturalHeight);
          }

          // A widened box's x still comes from its own band's start, which
          // can run past the grid's right edge (or into the next band's
          // boxes — that overlap is resolved by loadOrComputeState's own
          // post-init compact() below, not here). This only keeps it
          // on-grid.
          let x = colStart;
          if (x + w > totalCols) x = Math.max(0, totalCols - w);

          next.set(boxId, { x: x, y: rowCursor, w: w, h: h });
          rowCursor += h;
        });
      });

      if (wasHidden) layout.hidden = true;
      return next;
    }

    // Merges saved state with fresh defaults for any box the saved data
    // doesn't mention (a box added to the template after the layout was last
    // saved). Detecting whether the result actually needs Gridstack's own
    // compact() to resolve an overlap (see below) is a plain O(n^2)
    // rectangle check — small box counts (a dozen or so per tab) make this
    // trivial, no occupancy engine needed for it.
    function loadOrComputeState() {
      const defaults = computeDefaultState();
      let saved = null;
      try {
        saved = window.N5eGrid.deserialize(uiState[stateKey]);
      } catch (err) {
        saved = null;
      }
      // One-time migration: nothing saved server-side yet, but the OLD
      // localStorage key (from whichever port this browser last saved
      // under) might still have this exact origin's most recent layout —
      // see sheet-grid.js's own doc on why this remains readable.
      if (!saved) {
        let legacy = null;
        try {
          legacy = window.N5eGrid.deserialize(localStorage.getItem(oldStorageKey));
        } catch (err) {
          legacy = null;
        }
        if (legacy) {
          saved = legacy;
          postUIState(stateKey, window.N5eGrid.serialize(legacy)).catch((err) => {
            console.warn("could not migrate saved layout to the server:", err);
          });
          try {
            localStorage.removeItem(oldStorageKey);
          } catch (err) {
            // Nothing to clean up if this fails — worst case the orphaned
            // key sits unread forever, same as it already was.
          }
        }
      }

      const savedById = new Map();
      if (saved) saved.forEach((n) => savedById.set(n.id, n));

      const merged = new Map();
      let missingCount = 0;
      boxes().forEach((box) => {
        const boxId = box.dataset.boxId;
        const rect = savedById.get(boxId);
        if (rect) {
          merged.set(boxId, rect);
        } else {
          merged.set(boxId, defaults.get(boxId));
          missingCount++;
        }
      });

      // needsHealing covers every case Gridstack's own compact() (called
      // once, right after init — see below) needs to resolve: no saved
      // layout at all (a widened default box can overlap its neighbour
      // band), a box merged in fresh from defaults (can overlap whatever's
      // already at that spot in the saved layout), or a genuinely corrupted
      // saved layout with real overlaps. A clean, complete saved layout
      // (the common case) skips this — resettling a player's own careful
      // arrangement on every load for no reason is exactly what this
      // avoids.
      const needsHealing = !saved || missingCount > 0 || anyOverlap(Array.from(merged.values()));
      return { state: merged, needsHealing: needsHealing };
    }

    // Wraps one box in the <div class="grid-stack-item"> Gridstack expects,
    // carrying its initial gs-x/gs-y/gs-w/gs-h/gs-id as HTML attributes —
    // GridStack.init's own default auto-detection picks these straight up,
    // no separate load() call needed for the first paint.
    function wrapAsGridItem(box, rect) {
      const item = document.createElement("div");
      item.className = "grid-stack-item";
      item.setAttribute("gs-id", box.dataset.boxId);
      item.setAttribute("gs-x", String(rect.x));
      item.setAttribute("gs-y", String(rect.y));
      item.setAttribute("gs-w", String(rect.w));
      item.setAttribute("gs-h", String(rect.h));
      if (box.dataset.minW) item.setAttribute("gs-min-w", box.dataset.minW);
      if (box.dataset.maxW) item.setAttribute("gs-max-w", box.dataset.maxW);
      box.classList.add("grid-stack-item-content");
      box.parentNode.insertBefore(item, box);
      item.appendChild(box);
      return item;
    }

    // Reparents every box out of its .sheet-col into the grid root, wrapped
    // as a Gridstack item, in the row-then-column order `state` implies (so
    // DOM/tab order stays a reasonable reading order) — then removes the
    // now-empty .sheet-col wrappers, same flattening this file has always
    // done, just producing Gridstack markup instead of inline grid-column/
    // grid-row styles.
    function flattenAndWrap(state) {
      const ids = Array.from(state.keys()).sort((a, b) => {
        const ra = state.get(a);
        const rb = state.get(b);
        return ra.y - rb.y || ra.x - rb.x;
      });
      const itemsById = new Map();
      ids.forEach((boxId) => {
        const box = gridRoot.querySelector('.sheet-box[data-box-id="' + CSS.escape(boxId) + '"]');
        if (!box) return;
        const item = wrapAsGridItem(box, state.get(boxId));
        gridRoot.appendChild(item);
        itemsById.set(boxId, item);
      });
      columnEls.forEach((col) => col.remove());
      return itemsById;
    }

    const loaded = loadOrComputeState();
    const itemsById = flattenAndWrap(loaded.state);

    gridRoot.classList.add("grid-stack");
    const grid = GridStack.init({
      column: window.N5eGrid.TOTAL_COLS,
      cellHeight: ROW_HEIGHT,
      marginTop: ROW_GAP / 2,
      marginBottom: ROW_GAP / 2,
      marginLeft: COL_GAP_PX / 2,
      marginRight: COL_GAP_PX / 2,
      float: false,
      // Starts locked — matches the toolbar's initial "Edit Layout" (off)
      // state; setEditing() below is the only thing that ever flips this.
      staticGrid: true,
      // Only the box's own .sheet-box-handle can start a drag, same as
      // before — the default handle is the whole box, which would fight
      // clicks/text-selection everywhere else in it. The handle IS a
      // <button>, and the drag plugin's default `cancel` list includes
      // "button" (meant to stop a drag starting on some OTHER button deeper
      // in the content) — left as default, that would also cancel dragging
      // from our own handle. Dropped from the cancel list here; the handle
      // restriction above already keeps every other in-box button from
      // ever starting a drag.
      draggable: { handle: ".sheet-box-handle", cancel: "input,textarea,select,option" },
      resizable: { handles: "se" },
      // oneColumnSize measures the .grid-stack element's OWN width, which is
      // narrower than the browser window by however much nav/sidebar chrome
      // eats into it (~450px on this app's layout) — using it directly
      // dropped to one column on an ordinary 1280px-wide desktop window,
      // well before the old CSS's own viewport-width breakpoint ever would
      // have. columnOpts + breakpointForWindow compares against
      // window.innerWidth instead, matching the pre-Gridstack engine's own
      // `@media (max-width:860px)` behavior.
      columnOpts: { breakpointForWindow: true, breakpoints: [{ w: 860, c: 1 }] },
      animate: true,
    }, gridRoot);

    if (loaded.needsHealing) grid.compact();

    // Edit-mode snap-grid dots (app.css's own .sheet-layout-editing.grid-stack
    // rule). Verified directly against rendered .grid-stack-item boxes
    // (getBoundingClientRect + computed margin, both 0 on the item itself):
    // Gridstack positions items edge-to-edge at exactly x*cellWidth()/
    // y*cellHeight, with NO added margin/gap in that position math — the
    // visible gap between boxes comes from padding inside .sheet-box, not
    // from item spacing — so the dot grid needs no offset, just the raw
    // pitch. Row pitch is a fixed, known-at-build-time px value (cellHeight,
    // set above); column pitch depends on the grid's own rendered width,
    // which changes with the window, a sidebar toggle, or the oneColumnMode
    // breakpoint above — so it's set as a CSS var, recomputed via Gridstack's
    // own cellWidth() (container width ÷ column count, the same slot width
    // it positions items against) whenever that width actually changes,
    // rather than assumed once at init.
    gridRoot.style.setProperty("--n5e-grid-row-pitch", ROW_HEIGHT + "px");
    function updateGridDotColPitch() {
      gridRoot.style.setProperty("--n5e-grid-col-pitch", grid.cellWidth() + "px");
    }
    updateGridDotColPitch();
    // Kept as a property on gridRoot itself, not just a bare local — an
    // observer with nothing else referencing it is a known footgun (some
    // engines are free to garbage-collect it once nothing holds a strong
    // reference, even mid-observation); tying it to the still-attached DOM
    // node it's observing keeps it alive for exactly as long as it needs to
    // be. Confirmed live: without this, a real window resize changed the
    // grid's actual rendered width (and grid.cellWidth()) but the dot
    // background silently kept using the stale pitch from init.
    gridRoot._n5eDotObserver = new ResizeObserver(updateGridDotColPitch);
    gridRoot._n5eDotObserver.observe(gridRoot);
    // Belt-and-suspenders: a plain window resize also recomputes this
    // directly, rather than relying solely on the ResizeObserver above.
    // Found live during testing that ResizeObserver notifications can go
    // missing for a resize that demonstrably did change gridRoot's own
    // rendered width (grid.cellWidth() reflected the new size correctly
    // when queried directly; the observer callback just never ran) — window
    // resize is the one thing that's certain to fire for the most common
    // cause of this pitch changing.
    window.addEventListener("resize", updateGridDotColPitch);

    // Undo capture: snapshotted right before a drag/resize actually starts,
    // but only pushed onto the stack once 'change' confirms something
    // really moved (see below) — a click-then-release with no movement
    // shouldn't leave a no-op entry on the stack.
    grid.on("dragstart resizestart", () => {
      preActionSnapshot = { nodes: grid.save(false), detailsOpen: snapshotDetailsOpen() };
    });

    grid.on("change", () => {
      if (suppressSave) return;
      if (preActionSnapshot) {
        undoStack.push(preActionSnapshot);
        if (undoStack.length > UNDO_LIMIT) undoStack.shift();
        updateUndoButton();
        preActionSnapshot = null;
      }
      clearTimeout(saveTimer);
      saveTimer = setTimeout(saveLayout, 200);
    });

    itemsById.forEach((item, boxId) => {
      const box = item.querySelector(":scope > .grid-stack-item-content");

      // The Features & Traits panel is itself a <details> — collapsing it
      // natively hides everything but the <summary>, but Gridstack doesn't
      // know that happened on its own, so without this it'd just leave a
      // tall empty item in place. Every other box's height only ever
      // changes via an explicit resize-grip drag; this is the one case
      // where content itself changes height, so it gets its own
      // recompute-on-toggle.
      if (box.tagName === "DETAILS") {
        box.addEventListener("toggle", () => {
          // Setting .open programmatically (Undo's restore, below) fires
          // this same native "toggle" event — without this guard, restoring
          // a past open/closed state would immediately trigger a fresh
          // measure-and-resize fighting the very state Undo just put back.
          if (suppressToggleHandler) return;
          const node = item.gridstackNode;
          if (!node) return;

          // Collapsing measures the now-summary-only content to shrink to;
          // reopening restores whatever height was in effect right before
          // this box collapsed (which may be a player's own manual resize,
          // not just the natural content height) instead of remeasuring —
          // remeasuring here would silently discard that resize every time
          // the box got collapsed and reopened.
          const h = box.open && preCollapseHeight.has(boxId)
            ? preCollapseHeight.get(boxId)
            : rowsForPx(measureNaturalHeight(box));
          if (node.h === h) return;

          // Captured here, not via the generic dragstart/resizestart path
          // above: the "toggle" event fires AFTER box.open has already
          // flipped, but the grid node's own height is still the
          // PRE-toggle value at this point (grid.update hasn't run yet
          // below) — exactly the snapshot Undo needs.
          const detailsOpen = snapshotDetailsOpen();
          detailsOpen.set(boxId, !box.open);
          undoStack.push({ nodes: grid.save(false), detailsOpen: detailsOpen });
          if (undoStack.length > UNDO_LIMIT) undoStack.shift();
          updateUndoButton();

          // Remember the real (open) height — which may itself be a manual
          // resize, not just the natural content height — before shrinking
          // to the collapsed one, so saveLayout can substitute it back in no
          // matter what triggers the NEXT save (see preCollapseHeight's own
          // doc) and so reopening above can restore it exactly. Reopening
          // clears the override: the node's own value is the real open
          // height again and needs no substitution.
          if (!box.open) {
            if (!preCollapseHeight.has(boxId)) preCollapseHeight.set(boxId, node.h);
          } else {
            preCollapseHeight.delete(boxId);
          }

          // Deliberately suppressed — see preCollapseHeight's own doc for
          // why persisting a collapsed-only height is exactly the bug this
          // whole mechanism exists to prevent. compact() still runs so any
          // OTHER box pulls up into the space this one just gave back
          // (no void left mid-layout), it just doesn't reach the server
          // for THIS box's own change.
          suppressSave = true;
          grid.update(item, { h: h });
          grid.compact();
          suppressSave = false;
        });
      }

      // See growCallbacks' own doc above. Grow-only: shrinking a box the
      // player deliberately sized smaller than a moment's content would
      // undo their own choice, and there's no way from here to tell "stale
      // default" apart from "player's own resize" — growing only ever
      // recovers space the box's own content already needs.
      growCallbacks.set(boxId, () => {
        const node = item.gridstackNode;
        if (!node) return;
        const wasHidden = layout.hidden;
        if (wasHidden) layout.hidden = false;
        const naturalHeight = measureNaturalHeight(box);
        if (wasHidden) layout.hidden = true;
        const currentPx = node.h * ROW_HEIGHT + (node.h - 1) * ROW_GAP;
        if (naturalHeight <= currentPx) return;
        grid.update(item, { h: rowsForPx(naturalHeight) });
        grid.compact();
      });
    });

    // Builds the node list actually sent to the server: identical to
    // grid.save() except any currently-collapsed box's height is swapped
    // back for its own real (open) height — see preCollapseHeight's own
    // doc. Every save goes through this, since any OTHER box's drag/resize
    // could commit while this one happens to be collapsed.
    function nodesForSave() {
      const nodes = grid.save(false);
      if (preCollapseHeight.size === 0) return nodes;
      return nodes.map((n) => {
        if (!preCollapseHeight.has(n.id)) return n;
        return Object.assign({}, n, { h: preCollapseHeight.get(n.id) });
      });
    }
    function saveLayout() {
      postUIState(stateKey, window.N5eGrid.serialize(nodesForSave())).catch((err) => {
        console.warn("could not save sheet layout:", err);
      });
    }
    if (loaded.needsHealing) saveLayout();

    function setEditing(on) {
      editing = on;
      layout.classList.toggle("sheet-layout-editing", editing);
      const toggle = layout.querySelector(":scope > .sheet-layout-toolbar > .sheet-layout-edit-toggle");
      if (toggle) {
        toggle.setAttribute("aria-pressed", String(editing));
        toggle.textContent = editing ? "Done Editing" : "Edit Layout";
      }
      grid.setStatic(!editing);
    }

    const editToggle = layout.querySelector(":scope > .sheet-layout-toolbar > .sheet-layout-edit-toggle");
    if (editToggle) {
      editToggle.addEventListener("click", () => setEditing(!editing));
    }

    const resetBtn = layout.querySelector(":scope > .sheet-layout-toolbar .sheet-layout-reset");
    if (resetBtn) {
      resetBtn.addEventListener("click", () => {
        const body = new URLSearchParams();
        body.set("key", stateKey);
        // Reload only after the reset lands server-side — reloading first
        // would re-fetch the page before the delete commits and land right
        // back on the layout Reset was meant to clear.
        fetch("/characters/" + characterID + "/sheet/ui-state/reset", {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "fetch" },
          body: body.toString(),
        })
          .catch((err) => console.warn("could not reset sheet layout:", err))
          .finally(() => window.location.reload());
      });
    }

    const undoBtn = layout.querySelector(":scope > .sheet-layout-toolbar .sheet-layout-undo");
    if (undoBtn) {
      undoBtn.addEventListener("click", () => {
        if (!undoStack.length) return;
        const entry = undoStack.pop();
        // Don't let this restore itself get captured as a new undo entry —
        // grid.load() below still fires 'change' (which still persists the
        // restored layout, deliberately: Undo is itself a real, wanted
        // layout change), it just shouldn't feed back into the stack it's
        // popping from.
        preActionSnapshot = null;
        suppressToggleHandler = true;
        entry.detailsOpen.forEach((open, boxId) => {
          const item = itemsById.get(boxId);
          const box = item && item.querySelector(":scope > .grid-stack-item-content");
          if (box) box.open = open;
        });
        grid.load(entry.nodes);
        // Per spec, setting .open queues a "toggle" event task rather than
        // dispatching it synchronously — resetting the flag right here
        // would close the suppression window before that queued event
        // arrives, letting it slip through unguarded. A macrotask reset
        // runs after whatever toggle task(s) got queued just above, since
        // those were queued first.
        setTimeout(() => { suppressToggleHandler = false; }, 0);
        updateUndoButton();
      });
    }

    // One frame after everything above has settled: a box whose natural
    // content height doesn't quite fit the row count it was given (a
    // measurement taken in a different layout context than Gridstack's own
    // absolute-position rendering, or simply stale saved data) shows a
    // scrollbar it doesn't need. This is a generic "grow anything that's
    // still clipped" pass — not gated to freshly-defaulted boxes, since a
    // saved layout can hit the exact same mismatch.
    requestAnimationFrame(() => {
      let grew = false;
      itemsById.forEach((item, boxId) => {
        const box = item.querySelector(":scope > .grid-stack-item-content");
        if (box.scrollHeight <= box.clientHeight) return;
        const node = item.gridstackNode;
        if (!node) return;
        const naturalHeight = measureNaturalHeight(box);
        const currentPx = node.h * ROW_HEIGHT + (node.h - 1) * ROW_GAP;
        if (naturalHeight <= currentPx) return;
        grid.update(item, { h: rowsForPx(naturalHeight) });
        grew = true;
      });
      if (grew) grid.compact();
    });
  }
})();
