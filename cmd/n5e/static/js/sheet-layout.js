// Security Onion-style dashboard editing for the sheet's box layouts (part
// 11's addition of width-resize on top of part 10's reorder + height-resize).
//
// Two independent layouts share this file: the Core tab (.sheet-core-layout)
// and the Bio tab (.sheet-bio-layout), each marked with
// data-sheet-layout="core"/"bio" and each with its own "Edit Layout"/"Reset
// layout" toolbar. Every box only starts a drag from its own
// .sheet-box-handle, and only resizes from its own .sheet-box-resize-handle
// (added here, not in the template, so no box can be missed) — and neither
// control does anything at all until that layout's "Edit Layout" toggle is
// on, same as before.
//
// Boxes are now real items of a shared 12-column CSS grid (see app.css and
// sheet-grid.js's N5eGrid, the pure occupancy-grid math this file drives).
// The template's .sheet-col wrappers are still what a fresh page's default
// grouping/order comes from (see computeDefaultState below), but at init
// this file flattens every box out of its .sheet-col and removes the
// now-empty wrapper — an empty .sheet-col left behind would still claim an
// implicit grid cell N5eGrid doesn't know about and cause a real overlap.
// After that, .sheet-col plays no further role; position comes entirely
// from each box's own inline grid-column/grid-row, computed from N5eGrid
// state.
//
// Reordering is still the same native HTML5 drag-and-drop the three library
// panes rely on (see sheet-library.js) — desktop-mouse-only. Resizing drags
// the same bottom-right corner grip as before, now changing width AND
// height together rather than height alone; both axes snap to the grid (12
// columns, 20px rows) via N5eGrid.clampRect, and N5eGrid.resolvePush pushes
// anything the moved/resized box now overlaps out of the way, with
// N5eGrid.compact closing any gaps left behind once the drag/resize
// commits. Only the commit (dragend / mouseup) is compacted and saved — the
// live preview during a drag only pushes, so already-settled boxes don't
// visibly jump around under the cursor mid-drag.
//
// The layout is a per-character display preference, persisted server-side
// in character_sheet_ui_state (see 0015_sheet_ui_state.sql) under keys
// "grid:core"/"grid:bio" — NOT localStorage, as of 2026-07-27. It used to
// be localStorage-only, keyed by character id, and that silently lost every
// saved layout on every app restart: cmd/n5e/main.go binds a fresh
// OS-assigned port each launch, and localStorage is scoped per-origin
// (port included), so a new port is a browser-side blank slate. The initial
// page render embeds whatever's saved into #sheet-ui-state-data (one JSON
// blob keyed the same way the table is); readUIState() below reads it once.
// A one-time migration reads the OLD localStorage key too (see
// loadOrComputeState) so a layout fixed in the currently-running session
// isn't silently discarded the moment this ships.
(function () {
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

  // One grow-if-needed callback per box id, populated by every initLayout()
  // call below — a single shared map (not per-layout) since a caller
  // outside this file only knows a box's id, not which named layout
  // (core/bio/summons/...) it belongs to. A box's content can grow after
  // its own layout was already measured-and-saved (a companion attached, a
  // Tactic/Mastery pick added — see loadOrComputeState's own comment on why
  // nothing here ever re-checks an already-saved layout on its own), and
  // the fragment swap that grows it only replaces the box's INNER markup,
  // never the box element itself — so nothing here notices unless asked.
  // window.n5eGrowSheetBox is that ask: exposed so a swap elsewhere
  // (sheet-vitals.js's swap(), sheet-refresh.js's n5eRefreshBlocks) can
  // request one specific box re-measure itself once its content has
  // changed. Deliberately grow-only (see the registration below) and
  // deliberately opt-in per box id rather than run automatically after
  // every swap — most boxes rely on their own overflow:auto scroll on
  // purpose (a capped Skills list, e.g.), and force-growing them on every
  // unrelated save would fight that.
  const growCallbacks = new Map();
  window.n5eGrowSheetBox = function (boxId) {
    const grow = growCallbacks.get(boxId);
    if (grow) grow();
  };

  document.querySelectorAll("[data-sheet-layout]").forEach((layout) => {
    initLayout(layout);
  });

  function initLayout(layout) {
    // Bio's grid container is the <form> that wraps its two .sheet-col
    // bands; Core's is the sibling .sheet-core-grid wrapper that plays the
    // same role. Either way the toolbar stays a sibling of the grid root,
    // never a child of it — a direct-child toolbar with no explicit grid
    // placement would itself become an implicit grid item and get
    // auto-placed wherever the browser found a free cell, nowhere near the
    // top once every box carries explicit grid-column/grid-row. Resolving
    // this from DOM shape (rather than checking
    // layout.dataset.sheetLayout === "bio") keeps this function identical
    // for both tabs.
    const gridRoot = layout.querySelector(":scope > form, :scope > .sheet-core-grid") || layout;
    const columnEls = Array.from(gridRoot.querySelectorAll(":scope > .sheet-col[data-column]"));
    if (columnEls.length === 0) return;

    const stateKey = "grid:" + layout.dataset.sheetLayout;
    // The old localStorage key, read exactly once as a migration fallback —
    // see loadOrComputeState. Never written to again after this file stops
    // being the localStorage version.
    const oldStorageKey = "n5e:sheet-layout:grid:" + layout.dataset.sheetLayout + ":" + characterID;
    const boxes = () => Array.from(gridRoot.querySelectorAll(".sheet-box[data-box-id]"));

    let editing = false;
    let dragging = null;
    let dragGrabOffsetX = 0;
    let dragGrabOffsetY = 0;
    let dragProposed = null;
    let state = new Map();
    // Edge auto-scroll while drag-reordering a box: native HTML5 drag-and-
    // drop does NOT auto-scroll the page on its own (unlike a native OS file
    // drag), so without this a box could never be dragged to a row below
    // whatever is currently in the viewport — the grid can pack boxes down
    // past row 168 (see n5e-sheet-grid-engine notes), well past one screen.
    // autoScrollSpeed is signed px/frame (negative = scrolling up); the rAF
    // loop is only ever (re)started when going from stopped to moving, never
    // stacked, and always torn down on dragend as a safety net in case the
    // pointer leaves the grid (and stops firing dragover) without crossing
    // back out of the edge zone first.
    let autoScrollSpeed = 0;
    let autoScrollHandle = null;
    const AUTO_SCROLL_EDGE_PX = 60;
    const AUTO_SCROLL_MAX_PX = 18;

    function autoScrollStep() {
      if (!autoScrollSpeed) {
        autoScrollHandle = null;
        return;
      }
      window.scrollBy(0, autoScrollSpeed);
      autoScrollHandle = requestAnimationFrame(autoScrollStep);
    }

    // Computes and (re)starts the scroll speed for the current pointer Y —
    // called from dragover, where e.clientY is already viewport-relative, so
    // this works regardless of how far down the (possibly very tall) grid
    // root the pointer actually is.
    function updateAutoScroll(clientY) {
      let speed = 0;
      if (clientY < AUTO_SCROLL_EDGE_PX) {
        speed = -AUTO_SCROLL_MAX_PX * (1 - clientY / AUTO_SCROLL_EDGE_PX);
      } else if (clientY > window.innerHeight - AUTO_SCROLL_EDGE_PX) {
        speed = AUTO_SCROLL_MAX_PX * (1 - (window.innerHeight - clientY) / AUTO_SCROLL_EDGE_PX);
      }
      autoScrollSpeed = speed;
      if (speed && autoScrollHandle === null) autoScrollHandle = requestAnimationFrame(autoScrollStep);
    }

    function stopAutoScroll() {
      autoScrollSpeed = 0;
      if (autoScrollHandle !== null) {
        cancelAnimationFrame(autoScrollHandle);
        autoScrollHandle = null;
      }
    }

    // A bounded undo stack for the one thing Reset Layout was previously
    // the only way back from: a single bad drag or resize. Each entry is a
    // full-state Map snapshot taken right before a commit overwrites
    // `state` — resolvePush/compact always build a NEW Map rather than
    // mutating the old one in place, so keeping the pre-commit Map's
    // reference here is enough, no deep clone needed. Capped rather than
    // unbounded since this is a per-tab in-memory stack, not something
    // persisted beyond the page's lifetime.
    //
    // Alongside `state`, each entry also snapshots every <details> box's
    // open/closed DOM state. Undoing past a Features & Traits toggle would
    // otherwise restore the smaller (or larger) grid-row size that toggle
    // committed WITHOUT restoring the open attribute that size was
    // measured for, leaving the panel's real (native, browser-owned) open
    // state and its now-reverted box size disagreeing — a closed-sized box
    // showing open content clipped, or vice versa.
    let undoStack = [];
    const UNDO_LIMIT = 20;
    let suppressToggleHandler = false;

    // Every <details> box's own `open` HTML attribute is unconditionally
    // hardcoded server-side (character_sheet.html never conditionally
    // renders it) — there is no persisted "stays collapsed across a
    // reload" concept anywhere. That means a box's collapsed-toggle
    // recompute (below) shrinks state's own rowSpan for a size that is
    // ONLY ever valid while this box happens to be collapsed IN THIS
    // SESSION — every future page load renders it open again regardless.
    // If that shrunk value were ever written to the server (via this
    // toggle's own save or ANY unrelated save firing while this box
    // happens to be collapsed — state is one shared map, so a totally
    // different box's drag-resize would pick it up too), the box comes
    // back on next load OPEN (full content) but pinned to the CLOSED
    // size — exactly the overlapping-scrollbar bug this map exists to
    // prevent. Tracks each currently-collapsed box's own PRE-collapse
    // (i.e. real open) rowSpan, so saveLayout can always substitute the
    // correct value back in before serializing, no matter which action
    // triggered the save.
    let preCollapseRowSpan = new Map();
    function snapshotDetailsOpen() {
      const m = new Map();
      boxes().forEach((b) => {
        if (b.tagName === "DETAILS") m.set(b.dataset.boxId, b.open);
      });
      return m;
    }
    function pushUndo() {
      undoStack.push({ state: state, detailsOpen: snapshotDetailsOpen() });
      if (undoStack.length > UNDO_LIMIT) undoStack.shift();
      updateUndoButton();
    }
    function updateUndoButton() {
      if (undoBtn) undoBtn.disabled = undoStack.length === 0;
    }

    // Every box gets a resize grip added here rather than in the template —
    // that is what guarantees every box (present now or added later) has
    // one, instead of relying on each template location to remember it.
    boxes().forEach((box) => {
      if (box.querySelector(":scope > .sheet-box-resize-handle")) return;
      const grip = document.createElement("div");
      grip.className = "sheet-box-resize-handle";
      grip.setAttribute("aria-hidden", "true");
      grip.title = "Drag to resize";
      box.appendChild(grip);
    });

    function constraintsFor(boxId) {
      const box = gridRoot.querySelector('.sheet-box[data-box-id="' + CSS.escape(boxId) + '"]');
      return {
        minColSpan: box && box.dataset.minW ? parseInt(box.dataset.minW, 10) : window.N5eGrid.MIN_COL_SPAN,
        maxColSpan: box && box.dataset.maxW ? parseInt(box.dataset.maxW, 10) : window.N5eGrid.MAX_COL_SPAN,
        minRowSpan: window.N5eGrid.MIN_ROW_SPAN,
      };
    }

    // 96px/in ÷ 2.54cm/in — the standard CSS px-per-cm conversion, same one
    // the browser itself uses for a literal `cm` length in a stylesheet.
    const CM_TO_PX = 96 / 2.54;

    // The default layout a fresh page (or a box the saved data doesn't know
    // about) should get: each .sheet-col becomes an even band of the
    // 12-column grid (bandWidth = 12 / number-of-bands — 4 for Core's 3
    // bands, 6 for Bio's 2 — with any remainder folded into the last band),
    // and boxes stack top-to-bottom within their band in template order,
    // exactly reproducing today's visual layout when nothing has been
    // dragged yet.
    function computeDefaultState() {
      // Both tabs init unconditionally on page load (see the
      // querySelectorAll(...).forEach(initLayout) call below), but only one
      // tab's <section> is ever visible at a time — the other carries the
      // [hidden] attribute, which app.css maps to display:none !important.
      // A display:none box measures 0x0 via getBoundingClientRect, so
      // measuring the inactive tab's boxes here would floor EVERY one of
      // them at MIN_ROW_SPAN regardless of actual content (this is exactly
      // what was silently crushing every Bio box — including Character
      // Backstory's 24-row textarea — down to a 4-row box on any load where
      // Bio wasn't the remembered tab). Temporarily un-hiding for this
      // synchronous measurement pass is invisible to the user: nothing
      // yields back to the browser's paint loop between the two attribute
      // writes, so there is no frame where the section flashes visible.
      const wasHidden = layout.hidden;
      if (wasHidden) layout.hidden = false;

      const totalCols = window.N5eGrid.TOTAL_COLS;
      const bandWidth = Math.floor(totalCols / columnEls.length);
      const gridWidthPx = gridRoot.getBoundingClientRect().width;
      const colWidthPx = gridWidthPx / totalCols;
      let next = new Map();
      const widened = [];
      columnEls.forEach((col, colIndex) => {
        const colStart = 1 + colIndex * bandWidth;
        const bandColSpan = colIndex === columnEls.length - 1 ? totalCols - colStart + 1 : bandWidth;
        let rowCursor = 1;
        Array.from(col.querySelectorAll(":scope > .sheet-box[data-box-id]")).forEach((box) => {
          const boxId = box.dataset.boxId;
          // A box can declare an exact default size in cm (data-default-w-cm/
          // data-default-h-cm — Calculator's own explicit "7cm x 7cm" ask)
          // rather than the generic band-width + measured-content default
          // every other box gets. Converted through the SAME colWidthPx this
          // function already measures the real grid at, so it lands on the
          // right column count at whatever width the grid actually renders,
          // not a guess baked in for one screen size.
          const defaultWCm = box.dataset.defaultWCm ? parseFloat(box.dataset.defaultWCm) : null;
          const defaultHCm = box.dataset.defaultHCm ? parseFloat(box.dataset.defaultHCm) : null;

          // A box can also declare a minimum column span wider than its
          // column's even band via data-min-w. No box currently sets one
          // (2026-07-27: the last one, sheet-squares' data-min-w="5", was
          // removed in favor of letting it wrap+correctly re-measure height
          // at a narrower width, same as Attacks & Jutsu's own data-min-w
          // was removed earlier in favor of wrap+center — this stays as a
          // real escape hatch for a future box with no graceful-degrade
          // option). Measuring it at the narrower band width, the way every
          // other box is measured, both
          // under-sizes its default width AND — because the content then
          // wraps inside that narrow measurement — inflates the measured
          // height with wrapped lines that won't exist once it's actually
          // widened. Bumping colSpan up front and measuring at THAT width
          // fixes both: Bukijutsu's attack circle no longer wraps onto its
          // own (left-aligned, not centered) row by default, and the
          // default rowSpan reflects the box's real, unwrapped height.
          const minColSpan = box.dataset.minW ? parseInt(box.dataset.minW, 10) : window.N5eGrid.MIN_COL_SPAN;
          const colSpan = defaultWCm && colWidthPx
            ? Math.max(window.N5eGrid.MIN_COL_SPAN, Math.round((defaultWCm * CM_TO_PX) / colWidthPx))
            : Math.min(totalCols, Math.max(bandColSpan, minColSpan));

          let rowSpan;
          if (defaultHCm) {
            // An explicit height needs no content measurement at all — skip
            // straight to solving N*ROW_HEIGHT + (N-1)*ROW_GAP >= target for
            // the smallest integer N, same formula the measured path below
            // uses, just against a fixed target instead of naturalHeight.
            rowSpan = Math.max(
              window.N5eGrid.MIN_ROW_SPAN,
              Math.ceil((defaultHCm * CM_TO_PX + window.N5eGrid.ROW_GAP) / (window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP))
            );
          } else {
            let restoreWidth = null;
            if (colWidthPx && colSpan !== bandColSpan) {
              restoreWidth = box.style.width;
              box.style.width = (colSpan * colWidthPx) + "px";
            }
            const naturalHeight = box.getBoundingClientRect().height;
            if (restoreWidth !== null) box.style.width = restoreWidth;

            // A box spanning N rows renders at N*ROW_HEIGHT + (N-1)*ROW_GAP px
            // (grid-auto-rows: minmax(20px, auto), with row-gap filling the
            // space between every track a box spans) — and every grid item
            // stretches to fill its full assigned area by default, so
            // dividing naturalHeight by ROW_HEIGHT alone (ignoring the gaps
            // that get folded into that area) systematically overshoots the
            // row count, leaving genuine dead space at the bottom of the box.
            // This solves N*ROW_HEIGHT + (N-1)*ROW_GAP >= naturalHeight for
            // the smallest integer N.
            rowSpan = Math.max(
              window.N5eGrid.MIN_ROW_SPAN,
              Math.ceil((naturalHeight + window.N5eGrid.ROW_GAP) / (window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP))
            );
          }
          // clampRect's own job — shifting colStart left, never shrinking
          // the span — is what keeps a bumped colSpan from running past
          // column 12 (Calculator's band starts at column 9; bumped to a
          // wider span would otherwise reach past it).
          const rect = window.N5eGrid.clampRect(
            { colStart: colStart, colSpan: colSpan, rowStart: rowCursor, rowSpan: rowSpan },
            { minColSpan: minColSpan }
          );
          next.set(boxId, rect);
          rowCursor += rowSpan;
          if (colSpan !== bandColSpan) widened.push(boxId);
        });
      });

      if (wasHidden) layout.hidden = true;

      // A widened box's colStart still comes from its old band, so it can
      // now run into the next column's boxes (Attacks & Jutsu's 6 columns
      // starting at Mid's colStart=5 reaches into Right's 9-12 band).
      // Same resolvePush+compact already used below to fold a saved
      // layout's missing boxes in — pushing down (never sideways) is
      // exactly the resolution a first paint should use here too.
      widened.forEach((boxId) => {
        next = window.N5eGrid.resolvePush(next, boxId, next.get(boxId));
      });
      if (widened.length) next = window.N5eGrid.compact(next);
      return next;
    }

    // Merges saved state with fresh defaults for any box the saved data
    // doesn't mention (a box added to the template after the layout was
    // last saved), pushing/compacting once to resolve whatever overlap that
    // merge might introduce — rather than special-casing "new box" logic.
    // Returns which box ids came from computeDefaultState (as opposed to a
    // user's own saved rect) alongside the merged state — see
    // correctFreshDefaults below for why that distinction matters.
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
      // under) might still have this exact origin's most recent layout.
      // Adopt it and persist it server-side immediately so it survives the
      // next restart too, then clear the old key — it's been promoted, not
      // just read.
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
      if (!saved) return { state: defaults, freshIds: new Set(defaults.keys()) };

      let merged = new Map(saved);
      const missingIds = [];
      defaults.forEach((rect, boxId) => {
        if (!merged.has(boxId)) {
          merged.set(boxId, rect);
          missingIds.push(boxId);
        }
      });
      // Drop entries for boxes no longer present in the DOM (e.g. removed
      // from a later template revision) — stale ids would otherwise sit in
      // localStorage forever, contributing nothing but confusion.
      Array.from(merged.keys()).forEach((boxId) => {
        if (!defaults.has(boxId)) merged.delete(boxId);
      });
      // A saved layout can still contain genuine overlaps that predate this
      // load — e.g. a box's rowSpan grown (a Tactic/Mastery pick added, a
      // Companion attached) without the push/compact cycle that normally
      // clears its new footprint, since today only an explicit drag/resize/
      // toggle commit runs that cycle, not every content change that can
      // grow a box's natural height. Left unhealed, two boxes render
      // stacked on top of each other — overlapping text, an unrelated box's
      // border cutting across this one's — every load, forever, since
      // nothing else here ever re-checks an already-fully-saved layout.
      // compact() finds each box's smallest free rowStart in its own column
      // range, so it's already the standard "settle" step used after every
      // OTHER state-changing action in this file (drag/resize commit,
      // toggle, undo, correctFreshDefaults below) — reusing it here, gated
      // on overlap actually being present, self-heals a corrupted saved
      // layout without touching one that's already fine (no missing boxes,
      // no overlap: compact() would just confirm each box is already at its
      // own smallest free slot).
      if (missingIds.length === 0) {
        const rects = Array.from(merged.values());
        let hasOverlap = false;
        for (let i = 0; i < rects.length && !hasOverlap; i++) {
          for (let j = i + 1; j < rects.length; j++) {
            if (window.N5eGrid.overlaps(rects[i], rects[j])) { hasOverlap = true; break; }
          }
        }
        if (hasOverlap) merged = window.N5eGrid.compact(merged);
        return { state: merged, freshIds: new Set(), repaired: hasOverlap };
      }

      missingIds.forEach((boxId) => {
        merged = window.N5eGrid.resolvePush(merged, boxId, merged.get(boxId));
      });
      return { state: window.N5eGrid.compact(merged), freshIds: new Set(missingIds), repaired: false };
    }

    // computeDefaultState measures every box in the transitional pre-flatten
    // .sheet-col (flex) context, not the grid context it actually renders in
    // once flattened — for most boxes the two agree closely, but anything
    // whose height is sensitive to exactly where its content wraps (a
    // flex-wrap row of pills, an attack-mods row) can come out a little
    // short once real grid track widths are involved, silently clipping
    // content the box was never actually too small for. This runs once,
    // right after the real DOM is live, and only touches ids in `freshIds`
    // — boxes using a genuinely fresh default, never one the player saved
    // themselves. Growing a box the player deliberately shrank smaller than
    // its content (the whole point of today's fix letting that work) would
    // be exactly the bug this is patching in reverse.
    function correctFreshDefaults(freshIds) {
      if (!freshIds.size) return;
      let changed = false;
      boxes().forEach((box) => {
        const boxId = box.dataset.boxId;
        if (!freshIds.has(boxId)) return;
        const rect = state.get(boxId);
        if (!rect) return;
        const currentPx = rect.rowSpan * window.N5eGrid.ROW_HEIGHT + (rect.rowSpan - 1) * window.N5eGrid.ROW_GAP;
        // Measure with the box's own height constraint (and the
        // overflow:auto scrollbar a too-short constraint can trigger)
        // lifted first, same align-self:start trick the Features & Traits
        // toggle listener above already uses for the identical reason: a
        // stretched grid item's box.scrollHeight reflects its CURRENT
        // (possibly wrong) height, not its natural content height. Here it
        // compounds — .sheet-box's overflow:auto steals ~15px of inline
        // width the instant it needs to scroll, which narrows a flex-wrap
        // row (sheet-squares' four tiles) just enough to wrap an extra tile
        // and inflate scrollHeight further, a self-reinforcing loop that
        // measuring the constrained box directly can never see past (found
        // 2026-07-27 investigating Prof Bonus's row wrapping and clipping
        // under a scrollbar on saved characters, never on a truly fresh
        // one).
        const prevAlignSelf = box.style.alignSelf;
        const prevOverflow = box.style.overflow;
        box.style.alignSelf = "start";
        box.style.overflow = "visible";
        const naturalHeight = box.getBoundingClientRect().height;
        box.style.alignSelf = prevAlignSelf;
        box.style.overflow = prevOverflow;
        if (naturalHeight <= currentPx) return;
        const rowSpan = Math.ceil(
          (naturalHeight + window.N5eGrid.ROW_GAP) / (window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP)
        );
        state.set(boxId, Object.assign({}, rect, { rowSpan: rowSpan }));
        changed = true;
      });
      if (!changed) return;
      // compact() re-derives every rowStart from a top-to-bottom, sorted
      // scan rather than needing resolvePush's push-cascade — safe here
      // since every rect it's compacting already has its FINAL rowSpan.
      state = window.N5eGrid.compact(state);
      applyGridStyles(state);
      saveLayout();
    }

    // A box can end up showing a scrollbar it doesn't actually need: at a
    // narrow enough colSpan, a flex-wrap row (sheet-squares' four tiles is
    // the one that surfaced this) sits close enough to its box's full width
    // that .sheet-box's own overflow:auto — triggered by SOME unrelated,
    // momentary overflow elsewhere on the page during initial layout — steals
    // ~15px of the row's available inline width the instant it shows a
    // scrollbar, which is just enough to wrap one more tile, which makes the
    // box genuinely too short for two rows, which keeps the scrollbar there
    // for good. None of this is width the row actually needed; it's a
    // self-reinforcing loop bootstrapped once and never a real sizing
    // problem.
    // Observed: the row wrapped and clipped its fourth tile on every
    // character but one, with byte-identical saved geometry across all of
    // them — the giveaway that this is a rendering glitch, not a stale
    // number correctFreshDefaults above should be growing.
    //
    // correctFreshDefaults runs first and can itself grow OTHER boxes,
    // which changes total page height and can flip the browser's own page
    // scrollbar on/off — which shrinks every grid column's available width
    // by the same amount, all over again. Fixing a box's own scrollbar
    // in-place, mid-pass, before that settles doesn't stick — checked and
    // confirmed live 2026-07-27: a box fixed correctly this way could be
    // right back in the wrapped state by the time the whole page finished
    // laying out. So this runs as its own separate LAST step, over every
    // box on the page (not just correctFreshDefaults' freshIds — a box
    // the player actually resized can hit this same glitch too), scheduled
    // for the frame after everything else this file does has committed.
    //
    // Safe to run unconditionally on every box currently showing a
    // scrollbar, deliberate ones included: lifting align-self/overflow and
    // putting them straight back doesn't change any final CSS declaration,
    // so a box whose content genuinely doesn't fit its current height (an
    // intentional, permanent scroll, e.g. a capped Skills list) reflows
    // right back into the same scrolling state once reverted — there's
    // nothing here to distinguish deliberate from spurious ahead of time,
    // the reflow itself does that safely on its own.
    function unstickSpuriousScrollbars() {
      boxes().forEach((box) => {
        if (box.scrollHeight <= box.clientHeight) return;
        const prevAlignSelf = box.style.alignSelf;
        const prevOverflow = box.style.overflow;
        box.style.alignSelf = "start";
        box.style.overflow = "visible";
        void box.getBoundingClientRect(); // force the un-clamped reflow
        box.style.alignSelf = prevAlignSelf;
        box.style.overflow = prevOverflow;
      });
    }

    function applyGridStyles(s) {
      boxes().forEach((box) => {
        const rect = s.get(box.dataset.boxId);
        if (!rect) return;
        box.style.gridColumn = rect.colStart + " / span " + rect.colSpan;
        box.style.gridRow = rect.rowStart + " / span " + rect.rowSpan;
      });
    }

    // Reparents every box out of its .sheet-col into the shared grid
    // container, in the row-then-column order `state` implies (so DOM/tab
    // order stays a reasonable reading order), then removes the now-empty
    // .sheet-col wrappers — an empty one left behind would still claim an
    // implicit grid cell our own occupancy math never accounts for.
    function flattenColumns(s) {
      const ids = Array.from(s.keys()).sort((a, b) => {
        const ra = s.get(a);
        const rb = s.get(b);
        return ra.rowStart - rb.rowStart || ra.colStart - rb.colStart;
      });
      ids.forEach((boxId) => {
        const box = gridRoot.querySelector('.sheet-box[data-box-id="' + CSS.escape(boxId) + '"]');
        if (box) gridRoot.appendChild(box);
      });
      columnEls.forEach((col) => col.remove());
    }

    function setEditing(on) {
      editing = on;
      layout.classList.toggle("sheet-layout-editing", editing);
      const toggle = layout.querySelector(":scope > .sheet-layout-toolbar > .sheet-layout-edit-toggle");
      if (toggle) {
        toggle.setAttribute("aria-pressed", String(editing));
        toggle.textContent = editing ? "Done Editing" : "Edit Layout";
      }
      if (!editing) disarmAll();
    }

    function armDrag(box) {
      if (editing) box.draggable = true;
    }
    function disarmAll() {
      boxes().forEach((b) => { b.draggable = false; });
    }

    boxes().forEach((box) => {
      // A plain descendant query, not ":scope > .sheet-box-handle": every
      // other box's handle is a direct child, but the Features & Traits
      // panel's lives one level deeper, inside its <summary> (see below) —
      // a direct-child-only query silently found nothing for it, so its
      // handle never got a mousedown listener at all and the box could
      // never be armed for dragging. Safe to widen for every box: a
      // .sheet-box never contains another .sheet-box, so this can't
      // accidentally pick up a different box's handle.
      const handle = box.querySelector(".sheet-box-handle");
      if (handle) {
        handle.addEventListener("mousedown", () => armDrag(box));
        // The Features & Traits panel's handle lives inside its <summary>
        // (it has to be visible whether the panel is open or closed) —
        // without this, grabbing it would also toggle the disclosure
        // open/closed via the browser's default <summary> click behaviour.
        handle.addEventListener("click", (e) => {
          if (!editing) return;
          e.preventDefault();
          e.stopPropagation();
        });
      }

      box.addEventListener("dragstart", (e) => {
        if (!editing) { e.preventDefault(); return; }
        dragging = box;
        const boxRect = box.getBoundingClientRect();
        dragGrabOffsetX = e.clientX - boxRect.left;
        dragGrabOffsetY = e.clientY - boxRect.top;
        dragProposed = state.get(box.dataset.boxId);
        box.classList.add("sheet-box-dragging");
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", box.dataset.boxId);
      });
      box.addEventListener("dragend", () => {
        box.classList.remove("sheet-box-dragging");
        box.draggable = false;
        if (dragging && dragProposed) {
          pushUndo();
          state = window.N5eGrid.compact(window.N5eGrid.resolvePush(state, dragging.dataset.boxId, dragProposed));
          applyGridStyles(state);
          saveLayout();
        }
        dragging = null;
        dragProposed = null;
        stopAutoScroll();
      });

      // The Features & Traits panel is itself a <details> — collapsing it
      // natively hides everything but the <summary>, but its .sheet-box
      // grid area (grid-row's rowSpan) doesn't know that happened, so it
      // used to just leave a huge empty box in place, with nothing below it
      // moving up: collapsing the panel just made its text disappear without
      // reclaiming the space. Every other box's height only ever changes via an
      // explicit resize-grip drag; this is the one case where content
      // itself changes height, so it gets its own recompute-on-toggle
      // rather than trying to route it through wireResize.
      if (box.tagName === "DETAILS") {
        box.addEventListener("toggle", () => {
          // Setting .open programmatically (as Undo's restore does, below)
          // fires this same native "toggle" event — without this guard,
          // restoring a past open/closed state would immediately trigger a
          // fresh measure-and-resize that fights the very state Undo is
          // trying to put back, and pushes a spurious extra entry onto the
          // undo stack in the middle of popping one off it.
          if (suppressToggleHandler) return;
          // Neither getBoundingClientRect nor scrollHeight gets this right
          // on its own: the box's rendered height is still pinned to its
          // CURRENT (pre-toggle) grid-row span via grid stretch, and
          // scrollHeight can never report SMALLER than that stretched
          // clientHeight — closing the panel shrinks its actual content
          // (down to just <summary>) well below the still-large span, but
          // scrollHeight would just echo the stale span back, same failure
          // mode measuring a box pinned taller than its content as
          // computeDefaultState hitting one pinned SHORTER (see
          // correctFreshDefaults above). align-self: start (temporarily
          // overriding the grid's default stretch) lets the box size
          // itself to its own content for one synchronous measurement,
          // exactly like .sheet-col does permanently during the pre-flatten
          // pass — then it's removed so normal stretch resumes.
          box.style.alignSelf = "start";
          const naturalHeight = box.getBoundingClientRect().height;
          box.style.alignSelf = "";
          const rowSpan = Math.max(
            window.N5eGrid.MIN_ROW_SPAN,
            Math.ceil((naturalHeight + window.N5eGrid.ROW_GAP) / (window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP))
          );
          const current = state.get(box.dataset.boxId);
          if (!current || current.rowSpan === rowSpan) return;
          // Not pushUndo() here: the "toggle" event fires AFTER box.open has
          // already flipped, so a generic snapshot taken now would record
          // THIS box's open state as the new value, not the one being
          // undone away from — undo would then restore the old (correct)
          // grid size but pair it with the new (wrong) open state, leaving
          // the two out of sync. Since a toggle is always exactly one flip,
          // the pre-toggle value is simply the opposite of the current one.
          const detailsOpen = snapshotDetailsOpen();
          detailsOpen.set(box.dataset.boxId, !box.open);
          undoStack.push({ state: state, detailsOpen: detailsOpen });
          if (undoStack.length > UNDO_LIMIT) undoStack.shift();
          updateUndoButton();
          // Remember the real (open) size before shrinking to the
          // collapsed one, so saveLayout can substitute it back in no
          // matter what triggers the next save — see preCollapseRowSpan's
          // own doc. Reopening clears the override: this same recompute
          // just measured the box's real open content, so state's own
          // value is correct again and needs no substitution.
          if (!box.open) {
            if (!preCollapseRowSpan.has(box.dataset.boxId)) {
              preCollapseRowSpan.set(box.dataset.boxId, current.rowSpan);
            }
          } else {
            preCollapseRowSpan.delete(box.dataset.boxId);
          }
          const proposed = Object.assign({}, current, { rowSpan: rowSpan });
          state = window.N5eGrid.compact(window.N5eGrid.resolvePush(state, box.dataset.boxId, proposed));
          applyGridStyles(state);
          // Deliberately NOT saveLayout() here — see preCollapseRowSpan's
          // own doc for why persisting a collapsed-only size is exactly
          // the bug this whole mechanism exists to prevent. The visual
          // reflow (state + applyGridStyles above) still happens for this
          // session; nothing about THIS toggle needs to reach the server.
        });
      }

      // See growCallbacks' own doc above. Grow-only, same reasoning as
      // correctFreshDefaults: shrinking a box the player deliberately sized
      // smaller than a moment's content would undo their own choice, and
      // there's no way from here to tell "stale auto-default" apart from
      // "player's own resize" — growing is the safe direction in both
      // cases, since it only ever recovers space the box's own content
      // already needs.
      growCallbacks.set(box.dataset.boxId, () => {
        const current = state.get(box.dataset.boxId);
        if (!current) return;
        // Same "temporarily unhide the inactive tab to measure" trick
        // computeDefaultState uses — a display:none ancestor (the OTHER
        // tab being open) would otherwise measure 0 and never grow
        // anything. Invisible to the user: nothing yields back to the
        // browser's paint loop between the two attribute writes.
        const wasHidden = layout.hidden;
        if (wasHidden) layout.hidden = false;
        const prevAlignSelf = box.style.alignSelf;
        const prevOverflow = box.style.overflow;
        box.style.alignSelf = "start";
        box.style.overflow = "visible";
        const naturalHeight = box.getBoundingClientRect().height;
        box.style.alignSelf = prevAlignSelf;
        box.style.overflow = prevOverflow;
        if (wasHidden) layout.hidden = true;
        const currentPx = current.rowSpan * window.N5eGrid.ROW_HEIGHT + (current.rowSpan - 1) * window.N5eGrid.ROW_GAP;
        if (naturalHeight <= currentPx) return;
        const rowSpan = Math.ceil((naturalHeight + window.N5eGrid.ROW_GAP) / (window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP));
        const proposed = Object.assign({}, current, { rowSpan: rowSpan });
        state = window.N5eGrid.compact(window.N5eGrid.resolvePush(state, box.dataset.boxId, proposed));
        applyGridStyles(state);
        saveLayout();
      });

      wireResize(box);
    });

    // Any mouseup at all disarms every box, not just the one whose handle
    // was pressed — the mouse can come up outside the handle (or outside
    // the window) without ever firing a drag, and a box left draggable=true
    // would then hijack the next plain click or text selection inside it.
    document.addEventListener("mouseup", disarmAll);

    // One listener on the grid root (not one per column, since columns no
    // longer exist after init) computes a proposed grid cell from the
    // pointer position and shows a live push-only preview — compaction is
    // deliberately deferred to dragend so other boxes don't visibly jump
    // around while the drag is still in progress.
    gridRoot.addEventListener("dragover", (e) => {
      if (!dragging) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = "move";
      updateAutoScroll(e.clientY);

      const gridRect = gridRoot.getBoundingClientRect();
      const colWidthPx = gridRect.width / window.N5eGrid.TOTAL_COLS;
      if (!colWidthPx) return;

      const boxId = dragging.dataset.boxId;
      const current = state.get(boxId);
      const colStart = Math.round((e.clientX - gridRect.left - dragGrabOffsetX) / colWidthPx) + 1;
      // Row N's top offset from the grid is (N-1) row-heights PLUS (N-1)
      // row-gaps, same reasoning as computeDefaultState's rowSpan fix.
      const rowStep = window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP;
      const rowStart = Math.round((e.clientY - gridRect.top - dragGrabOffsetY) / rowStep) + 1;
      const proposed = window.N5eGrid.clampRect(
        { colStart: colStart, colSpan: current.colSpan, rowStart: rowStart, rowSpan: current.rowSpan },
        constraintsFor(boxId)
      );
      dragProposed = proposed;
      applyGridStyles(window.N5eGrid.resolvePush(state, boxId, proposed));
    });
    gridRoot.addEventListener("drop", (e) => e.preventDefault());
    // Without this, dragging the pointer off gridRoot entirely (past its top
    // into the nav bar, or past its bottom into whatever follows the grid)
    // stops dragover from firing — which stops updateAutoScroll from ever
    // being called again — but the rAF loop it started keeps running at
    // whatever speed it was last given, scrolling indefinitely until
    // dragend. dragover firing again on re-entry naturally restarts it.
    gridRoot.addEventListener("dragleave", stopAutoScroll);

    // Drag the bottom-right grip to change a box's width AND height
    // together, snapped to the grid (columns horizontally, ROW_HEIGHT
    // vertically). Same live-preview-then-commit split as drag-reorder
    // above.
    function wireResize(box) {
      const grip = box.querySelector(":scope > .sheet-box-resize-handle");
      if (!grip) return;

      let startX = 0;
      let startY = 0;
      let startRect = null;
      let colWidthPx = 0;
      let lastProposed = null;

      function onMove(e) {
        const dCols = Math.round((e.clientX - startX) / colWidthPx);
        // Same ROW_HEIGHT+ROW_GAP step as computeDefaultState: growing a box
        // by one row on screen takes that much pixel movement, not just
        // ROW_HEIGHT — otherwise the grip visually lags a row behind the
        // cursor while dragging.
        const dRows = Math.round((e.clientY - startY) / (window.N5eGrid.ROW_HEIGHT + window.N5eGrid.ROW_GAP));
        const proposed = window.N5eGrid.clampRect(
          {
            colStart: startRect.colStart,
            colSpan: startRect.colSpan + dCols,
            rowStart: startRect.rowStart,
            rowSpan: startRect.rowSpan + dRows,
          },
          constraintsFor(box.dataset.boxId)
        );
        lastProposed = proposed;
        applyGridStyles(window.N5eGrid.resolvePush(state, box.dataset.boxId, proposed));
      }
      function onUp() {
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
        box.classList.remove("sheet-box-resizing");
        document.body.classList.remove("n5e-resizing");
        if (lastProposed) {
          pushUndo();
          state = window.N5eGrid.compact(window.N5eGrid.resolvePush(state, box.dataset.boxId, lastProposed));
          applyGridStyles(state);
          saveLayout();
        }
      }

      grip.addEventListener("mousedown", (e) => {
        if (!editing) return;
        e.preventDefault();
        e.stopPropagation(); // don't also arm a drag via a handle underneath
        const gridRect = gridRoot.getBoundingClientRect();
        colWidthPx = gridRect.width / window.N5eGrid.TOTAL_COLS;
        if (!colWidthPx) return;
        startX = e.clientX;
        startY = e.clientY;
        startRect = state.get(box.dataset.boxId);
        lastProposed = null;
        box.classList.add("sheet-box-resizing");
        document.body.classList.add("n5e-resizing");
        document.addEventListener("mousemove", onMove);
        document.addEventListener("mouseup", onUp);
      });
    }

    // Builds the map actually sent to the server: identical to `state`
    // except any currently-collapsed box's rowSpan is swapped back for
    // its own real (open) size — see preCollapseRowSpan's own doc. Every
    // saveLayout() call site (drag/resize included, not just the toggle
    // handler) goes through this, since `state` is one shared map and any
    // of them could fire while some OTHER box happens to be collapsed.
    function stateForSave() {
      if (preCollapseRowSpan.size === 0) return state;
      const out = new Map(state);
      preCollapseRowSpan.forEach((rowSpan, boxId) => {
        const rect = out.get(boxId);
        if (rect) out.set(boxId, Object.assign({}, rect, { rowSpan: rowSpan }));
      });
      return out;
    }

    function saveLayout() {
      postUIState(stateKey, window.N5eGrid.serialize(stateForSave())).catch((err) => {
        console.warn("could not save sheet layout:", err);
      });
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
        state = entry.state;
        // entry.state/entry.detailsOpen were captured together, so
        // whatever rowSpan state now holds already matches the open/closed
        // pairing being restored below — any preCollapseRowSpan override
        // left over from a toggle that happened AFTER this snapshot no
        // longer applies to the state being restored (it might not even
        // be the same box's collapse anymore), so it's cleared rather than
        // carried forward stale.
        preCollapseRowSpan.clear();
        suppressToggleHandler = true;
        entry.detailsOpen.forEach((open, boxId) => {
          const box = gridRoot.querySelector('.sheet-box[data-box-id="' + CSS.escape(boxId) + '"]');
          if (box) box.open = open;
        });
        // Per spec, setting .open queues a "toggle" event task rather than
        // dispatching it synchronously — resetting the flag right here
        // would close the suppression window before that queued event
        // actually arrives, letting it slip through unguarded (which is
        // exactly what happened: an unsuppressed re-fire re-measured the
        // box, silently overwrote the size undo had just restored, and left
        // a phantom entry on the stack). A macrotask reset runs after
        // whatever toggle task(s) got queued just above, since those were
        // queued first.
        setTimeout(() => { suppressToggleHandler = false; }, 0);
        applyGridStyles(state);
        saveLayout();
        updateUndoButton();
      });
    }

    const loaded = loadOrComputeState();
    state = loaded.state;
    flattenColumns(state);
    applyGridStyles(state);
    // Persist the de-overlapped layout so a corrupted saved state only ever
    // needs healing once, not on every subsequent load.
    if (loaded.repaired) saveLayout();
    correctFreshDefaults(loaded.freshIds);
    // One frame later, not right here: correctFreshDefaults above (and the
    // OTHER tab's own initLayout call, and sheet-subgrid.js's own top-level
    // pass, both of which run after this file finishes) can still change
    // total page height and flip the page's own scrollbar on the way to
    // settling — see unstickSpuriousScrollbars' own comment for why fixing
    // a box mid-pass, before that settles, doesn't stick.
    requestAnimationFrame(unstickSpuriousScrollbars);
  }
})();
