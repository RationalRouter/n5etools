-- Tracks Beguiler's Twisted Casting and Corrupt Thoughts' Psyche Breaker
-- (both 10th level, "select two Genjutsu that you know") — sourced from the
-- character's own already-known jutsu (character_jutsu) rather than a
-- static rules-database catalog, the identical shape
-- 0041_ninjutsu_specialist_picks.sql's character_ninjutsu_jutsu_picks
-- already establishes for Ninjutsu Specialist's Refined Ninjutsu/Ninjutsu
-- Master. character_genjutsu_picks (0035_genjutsu_picks.sql) is the wrong
-- fit here: it stores rules-database class_options slugs, not references
-- into a character's own known-jutsu list (which may include custom_jutsu
-- rows with no slug at all).
--
-- ON DELETE CASCADE on jutsu_id so forgetting/unlearning that Genjutsu
-- automatically drops the Twisted Casting/Psyche Breaker pick that pointed
-- at it, rather than leaving an orphaned pick behind. One table with a
-- category discriminator (same shape character_ninjutsu_jutsu_picks/
-- character_hunter_nin_picks already establish) since both picks point at
-- the same character_jutsu table, just under independent caps.
CREATE TABLE character_genjutsu_jutsu_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('twisted_casting', 'psyche_breaker')),
    jutsu_id     INTEGER NOT NULL REFERENCES character_jutsu(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, jutsu_id)
);

CREATE INDEX idx_character_genjutsu_jutsu_picks_character ON character_genjutsu_jutsu_picks(character_id);
