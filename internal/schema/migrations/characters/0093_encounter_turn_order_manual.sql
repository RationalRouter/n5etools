-- The turn-order tracker's order becomes stored rather than computed, so the
-- GM can drag entries into any order: breaking initiative ties, or letting
-- the party send a heavy hitter first. Initiative still decides where a new
-- entry lands, and editing one entry's initiative re-places just that entry;
-- nothing else moves on its own.
--
-- Backfill: existing entries keep the order 0092 computed (initiative
-- descending, blank last, ties by insertion).
ALTER TABLE encounter_turn_entries ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE encounter_turn_entries
SET sort_order = (
    SELECT rn FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY encounter_id
            ORDER BY initiative IS NULL, initiative DESC, id
        ) - 1 AS rn
        FROM encounter_turn_entries
    ) ranked
    WHERE ranked.id = encounter_turn_entries.id
);
