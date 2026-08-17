// Pure occupancy-grid math for the sheet's Core/Bio box layouts
// (sheet-layout.js does all the DOM/event work; this file never touches the
// DOM). A small hand-rolled stand-in for what a library like Gridstack.js
// would otherwise provide — box positions are 1-based {colStart, colSpan,
// rowStart, rowSpan} rects on a fixed TOTAL_COLS-wide grid, in a
// Map<boxId, rect>. Every function here returns a NEW Map; none of them
// mutate the state passed in, so callers can always compare "current state"
// against "proposed state" (e.g. for a live drag preview) without extra
// bookkeeping.
window.N5eGrid = (function () {
  const TOTAL_COLS = 12;
  const ROW_HEIGHT = 20; // px — same unit as the old height-resize snap step
  const ROW_GAP = 8; // px — app.css's .sheet-core-grid/.sheet-bio-form row-gap (0.5rem)
  // Both floors dropped to 1 (2026-07-27, removing hard per-box
  // width/height minimums program-wide) — 1 is the true grid minimum (a box
  // can't span less than one column/row), not an arbitrary "looks
  // reasonable" number like the old 2/4. A per-box data-min-w can still
  // raise minColSpan above this via constraintsFor (sheet-layout.js) for a
  // future box that genuinely has no graceful-degrade option; none
  // currently does — reach for a CSS-only graceful degrade before adding a
  // new data-min-w.
  const MIN_COL_SPAN = 1;
  const MAX_COL_SPAN = TOTAL_COLS;
  const MIN_ROW_SPAN = 1; // 1 * 20px = 20px

  function cloneState(state) {
    const next = new Map();
    state.forEach((rect, id) => next.set(id, Object.assign({}, rect)));
    return next;
  }

  // Clamps a proposed rect's span to [min,max], then shifts (never shrinks)
  // colStart left if the span would run past the right edge of the grid.
  function clampRect(rect, constraints) {
    const c = constraints || {};
    const minColSpan = c.minColSpan || MIN_COL_SPAN;
    const maxColSpan = Math.min(c.maxColSpan || MAX_COL_SPAN, TOTAL_COLS);
    const minRowSpan = c.minRowSpan || MIN_ROW_SPAN;

    let colSpan = Math.max(minColSpan, Math.min(maxColSpan, rect.colSpan));
    let rowSpan = Math.max(minRowSpan, rect.rowSpan);
    let colStart = Math.max(1, rect.colStart);
    if (colStart + colSpan - 1 > TOTAL_COLS) colStart = TOTAL_COLS - colSpan + 1;
    let rowStart = Math.max(1, rect.rowStart);

    return { colStart, colSpan, rowStart, rowSpan };
  }

  function overlaps(a, b) {
    return (
      a.colStart < b.colStart + b.colSpan &&
      b.colStart < a.colStart + a.colSpan &&
      a.rowStart < b.rowStart + b.rowSpan &&
      b.rowStart < a.rowStart + a.rowSpan
    );
  }

  // Fixes `movedId` at `proposedRect` (already clamped by the caller), then
  // pushes anything it now overlaps straight down below its new bottom edge.
  // A pushed box is re-checked against every other box in turn, so a chain
  // of overlaps cascades correctly. Rows only ever increase during a push,
  // which guarantees this terminates — the iteration cap is a defensive
  // backstop, not something normal use should ever hit.
  function resolvePush(state, movedId, proposedRect) {
    const next = cloneState(state);
    if (!next.has(movedId)) return next;
    next.set(movedId, Object.assign({}, proposedRect));

    const queue = [movedId];
    const maxIterations = next.size * 50;
    let iterations = 0;

    while (queue.length && iterations < maxIterations) {
      iterations++;
      const currentId = queue.shift();
      const current = next.get(currentId);
      next.forEach((rect, id) => {
        if (id === currentId) return;
        if (!overlaps(current, rect)) return;
        const pushedRowStart = current.rowStart + current.rowSpan;
        if (rect.rowStart < pushedRowStart) {
          next.set(id, Object.assign({}, rect, { rowStart: pushedRowStart }));
          queue.push(id);
        }
      });
    }
    return next;
  }

  // Vertical-only compaction: never touches colStart/colSpan. Walks boxes
  // top-to-bottom (ties broken by colStart) and settles each one at the
  // smallest free rowStart in its own column range, closing any gap left
  // behind by a box that moved away. Horizontal position only ever changes
  // via an explicit user drag/resize — this never repacks a box sideways.
  function compact(state) {
    const next = cloneState(state);
    const ids = Array.from(next.keys()).sort((a, b) => {
      const ra = next.get(a);
      const rb = next.get(b);
      return ra.rowStart - rb.rowStart || ra.colStart - rb.colStart;
    });

    const placed = [];
    ids.forEach((id) => {
      const rect = next.get(id);
      let rowStart = 1;
      while (placed.some((p) => overlaps({ colStart: rect.colStart, colSpan: rect.colSpan, rowStart: rowStart, rowSpan: rect.rowSpan }, p))) {
        rowStart++;
      }
      const settled = Object.assign({}, rect, { rowStart });
      next.set(id, settled);
      placed.push(settled);
    });
    return next;
  }

  // How many grid tracks a pixel offset covers, given the grid container's
  // current rendered width — rounds to the nearest track, never fractional.
  function pxToCol(px, containerWidthPx) {
    const colWidthPx = containerWidthPx / TOTAL_COLS;
    if (!colWidthPx) return 0;
    return Math.round(px / colWidthPx);
  }

  function serialize(state) {
    const boxes = {};
    state.forEach((rect, id) => {
      boxes[id] = { x: rect.colStart - 1, y: rect.rowStart - 1, w: rect.colSpan, h: rect.rowSpan };
    });
    return { version: 2, cols: TOTAL_COLS, boxes: boxes };
  }

  // Returns null for anything absent/malformed/wrong-version rather than
  // throwing — callers treat null exactly like "no saved layout" and fall
  // back to computing fresh defaults.
  //
  // raw accepts either a JSON string (the shape a fresh JSON.parse or a
  // localStorage read gives you) or an already-parsed object (what the
  // server now embeds directly via a <script type="application/json">
  // block — see sheet-layout.js's readUIState) — one function serving both
  // the old client-only storage shape and the new server-persisted one
  // rather than making every caller know which it has.
  function deserialize(raw) {
    if (!raw) return null;
    let json = raw;
    if (typeof raw === "string") {
      try {
        json = JSON.parse(raw);
      } catch (err) {
        return null;
      }
    }
    if (!json || json.version !== 2 || !json.boxes || typeof json.boxes !== "object") return null;

    const state = new Map();
    Object.keys(json.boxes).forEach((id) => {
      const b = json.boxes[id];
      if (
        typeof b.x !== "number" || typeof b.y !== "number" ||
        typeof b.w !== "number" || typeof b.h !== "number"
      ) return;
      state.set(id, { colStart: b.x + 1, colSpan: b.w, rowStart: b.y + 1, rowSpan: b.h });
    });
    return state.size ? state : null;
  }

  return {
    TOTAL_COLS: TOTAL_COLS,
    ROW_HEIGHT: ROW_HEIGHT,
    ROW_GAP: ROW_GAP,
    MIN_COL_SPAN: MIN_COL_SPAN,
    MAX_COL_SPAN: MAX_COL_SPAN,
    MIN_ROW_SPAN: MIN_ROW_SPAN,
    clampRect: clampRect,
    overlaps: overlaps,
    resolvePush: resolvePush,
    compact: compact,
    pxToCol: pxToCol,
    serialize: serialize,
    deserialize: deserialize,
  };
})();
