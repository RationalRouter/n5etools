-- Tracks Intelligence Operative's two cap-gated catalog picks: Plans
-- (class_options, list_name='Plans', 21 rows, base class, cap 2@2nd ->
-- 8@20th) and Operative Traps (class_options, list_name='Operative Traps',
-- 8 rows, Tactical Strategist subclass only, cap 2@3rd -> 4@9th). One
-- generic table with a category discriminator, the same shape
-- character_hunter_nin_picks/character_genjutsu_picks already establish for
-- their own simultaneous class-level picks.
--
-- No FK into rules.db's class_options (same cross-DB slug-reference
-- tolerance every other rules.db-keyed table in this schema already uses).
-- UNIQUE on (character_id, category, option_slug): a character either knows
-- a given pick or doesn't.
CREATE TABLE character_intelligence_operative_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('plan', 'operative_trap')),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

CREATE INDEX idx_character_intelligence_operative_picks_character ON character_intelligence_operative_picks(character_id);
