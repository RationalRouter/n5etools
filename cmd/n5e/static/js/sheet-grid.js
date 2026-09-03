// Thin translation layer between the sheet layout's persisted JSON shape
// (character_sheet_ui_state, key "grid:<layout>") and Gridstack.js's own
// {id,x,y,w,h} node array. sheet-layout.js drives Gridstack.js directly now
// (see that file) and no longer needs any occupancy-grid math of its own —
// this file used to BE that math (a hand-rolled stand-in for what a library
// like Gridstack.js provides), kept as its own small file purely so the wire
// format's own history stays documented in one place rather than folded into
// sheet-layout.js's much larger init logic.
window.N5eGrid = (function () {
  const TOTAL_COLS = 12;
  const ROW_HEIGHT = 20; // px — Gridstack's own cellHeight, see sheet-layout.js's init options
  const ROW_GAP = 8; // px — combined top+bottom margin between two stacked rows, see sheet-layout.js

  // Builds the wire shape saved to character_sheet_ui_state. version 3 is
  // Gridstack's own node shape; the OLD version:2 shape (the pre-Gridstack
  // hand-rolled engine's own {colStart,...} rects, already serialized as
  // 0-based x/y before being written) turns out to already be
  // field-identical to it — see deserialize below — so bumping the version
  // number here is purely a marker for future compatibility checks, not a
  // format change.
  function serialize(nodes) {
    const boxes = {};
    nodes.forEach((n) => {
      boxes[n.id] = { x: n.x, y: n.y, w: n.w, h: n.h };
    });
    return { version: 3, cols: TOTAL_COLS, boxes: boxes };
  }

  // Returns a GridStackWidget[] (the shape grid.load()/init's own
  // gs-x/gs-y/gs-w/gs-h auto-detection both accept), or null for anything
  // absent/malformed — callers treat null exactly like "no saved layout" and
  // fall back to computed defaults.
  //
  // Accepts BOTH the old version:2 shape and the new version:3 shape: both
  // already store the same 0-based x/y, column/row units (version 2's own
  // serialize() computed `x: rect.colStart - 1` etc. — the exact same
  // 0-based coordinate Gridstack itself uses), so this is a straight field
  // mapping, not a lossy conversion. A layout a player already saved under
  // the old engine loads correctly here, unchanged, the first time they open
  // a tab after this file's own migration to Gridstack — nothing about that
  // upgrade is destructive.
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
    if (!json || (json.version !== 2 && json.version !== 3) || !json.boxes || typeof json.boxes !== "object") return null;

    const nodes = [];
    Object.keys(json.boxes).forEach((id) => {
      const b = json.boxes[id];
      if (
        typeof b.x !== "number" || typeof b.y !== "number" ||
        typeof b.w !== "number" || typeof b.h !== "number"
      ) return;
      nodes.push({ id: id, x: b.x, y: b.y, w: b.w, h: b.h });
    });
    return nodes.length ? nodes : null;
  }

  return {
    TOTAL_COLS: TOTAL_COLS,
    ROW_HEIGHT: ROW_HEIGHT,
    ROW_GAP: ROW_GAP,
    serialize: serialize,
    deserialize: deserialize,
  };
})();
