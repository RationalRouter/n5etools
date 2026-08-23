-- Widens character_ninjutsu_jutsu_picks' category CHECK to add
-- 'awakened_scroll' (Scribe Master's own seal-storage picker,
-- cmd/n5e/ninjutsu_specialist.go's addAwakenedScrollPick/
-- charstore.NinjutsuPickAwakenedScroll) — 0041_ninjutsu_specialist_picks.sql
-- only ever allowed 'refined_ninjutsu'/'ninjutsu_master', so every attempt
-- to add an Awakened Scroll pick has been failing its INSERT's CHECK
-- constraint since the feature was built, regardless of route (Core sheet
-- or popup).
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0056_scout_nin_more_pick_categories.sql already uses.
CREATE TABLE character_ninjutsu_jutsu_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('refined_ninjutsu', 'ninjutsu_master', 'awakened_scroll')),
    jutsu_id     INTEGER NOT NULL REFERENCES character_jutsu(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, jutsu_id)
);

INSERT INTO character_ninjutsu_jutsu_picks_new (id, character_id, category, jutsu_id, created_at)
SELECT id, character_id, category, jutsu_id, created_at
FROM character_ninjutsu_jutsu_picks;

DROP TABLE character_ninjutsu_jutsu_picks;
ALTER TABLE character_ninjutsu_jutsu_picks_new RENAME TO character_ninjutsu_jutsu_picks;

CREATE INDEX idx_character_ninjutsu_jutsu_picks_character ON character_ninjutsu_jutsu_picks(character_id);
