-- The sheet's box layout (sheet-layout.js's 12-col grid, one per tab: core/
-- bio) and its nested tile reorder/orientation (sheet-subgrid.js, one per
-- subgrid: squares/abilities/saves) used to live in the BROWSER's
-- localStorage only, keyed by character id. That silently broke every time
-- the app restarted: cmd/n5e/main.go binds an OS-assigned port
-- (net.Listen("tcp", "127.0.0.1:0")) on every launch, and localStorage is
-- scoped per-ORIGIN — scheme+host+PORT — so a new port is a new origin with
-- empty storage as far as the browser is concerned. A player's saved layout
-- (observed as a tile appearing to keep reverting to a bad default position)
-- was never actually lost, just orphaned under a URL that stops existing the
-- moment the server restarts.
--
-- One generic table for both mechanisms rather than two near-identical
-- ones: both are "a small JSON blob of pure display preference, keyed by
-- character + which instance", nothing here is ever read by game-rules
-- code, and neither needs its own query shape beyond get/set/delete by key.
-- state_key values in use: 'grid:core', 'grid:bio' (sheet-layout.js) and
-- 'subgrid:<name>' for whatever data-subgrid names exist in the template
-- (sheet-subgrid.js) — not enumerated here as a CHECK constraint, since a
-- future subgrid/layout addition should need zero schema change to start
-- using this table.
CREATE TABLE character_sheet_ui_state (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    state_key    TEXT NOT NULL,
    state_json   TEXT NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (character_id, state_key)
);
