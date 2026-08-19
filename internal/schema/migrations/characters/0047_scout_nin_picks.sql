-- Tracks two of Scout-Nin's base-class cap-gated catalog picks:
--   shinobi_adept  Shinobi Adept (2nd level), a hand-curated 10-option
--                  catalog with no class_options rows of its own — the book
--                  states the 10 named generalizations directly as their
--                  own class_features rows (Shinobi's Tactics/General
--                  Literacy/Tool Competency/Precision/Edge/Drive/Focus,
--                  Hidden Technique, Aggressive Technique, Tactical
--                  Technique) rather than a catalog list, same shape
--                  cmd/n5e/medical_nin.go's medicalDoctrineSlugs already
--                  reads live from class_features. Cap 2 -> 4@13th.
--   jack_of_all    Jack of All, Master of None (5th level), the same
--                  shape over a 5-option catalog (Combat, Control,
--                  Mobility, Skill, Support) — cap 1, extended to 2@9th
--                  and 3@20th for Tactical Scout only (Expert of All, Jack
--                  of None / Unmatched Tactics).
--
-- One generic table with a category discriminator, the same pattern
-- character_hunter_nin_picks/character_genjutsu_picks/
-- character_science_nin_subclass_picks already establish for a class's own
-- simultaneous cap-gated picks, rather than two near-identical tables.
--
-- No FK into rules.db's class_features (same cross-DB slug-reference
-- tolerance every other rules.db-keyed table in this schema already uses).
-- UNIQUE on (character_id, category, option_slug): a character either
-- holds a given pick or doesn't.
CREATE TABLE character_scout_nin_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('shinobi_adept', 'jack_of_all')),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

CREATE INDEX idx_character_scout_nin_picks_character ON character_scout_nin_picks(character_id);
