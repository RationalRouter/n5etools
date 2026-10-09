-- Turn-order (initiative) tracker for a saved encounter.
--
-- Entries are ordered at read time by initiative descending, entries with no
-- initiative yet last, ties broken by insertion order -- no sort_order column,
-- so there is no manual reorder to keep consistent.
--
-- adversary_id links an entry back to the roster row it was added from; the
-- name is a snapshot so the entry survives that adversary being rerolled or
-- removed. characters.db does not reliably enforce foreign keys on pooled
-- connections, so the app clears adversary_id and deletes entries itself
-- instead of relying on the ON DELETE clauses below.
--
-- current_turn_entry_id is NULL until the first "Next Turn" (or after a
-- reset), and is cleared/advanced by the app when its entry is removed.
ALTER TABLE encounters ADD COLUMN current_turn_entry_id INTEGER;
ALTER TABLE encounters ADD COLUMN round_number INTEGER NOT NULL DEFAULT 1;

CREATE TABLE encounter_turn_entries (
    id           INTEGER PRIMARY KEY,
    encounter_id INTEGER NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('adversary','player','ally')),
    adversary_id INTEGER REFERENCES encounter_adversaries(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    initiative   INTEGER,
    hp_current   INTEGER,
    hp_max       INTEGER,
    note         TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL
);
CREATE INDEX idx_encounter_turn_entries_encounter ON encounter_turn_entries(encounter_id);
