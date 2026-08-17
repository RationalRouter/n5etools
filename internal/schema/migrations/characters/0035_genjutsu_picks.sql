-- Tracks Genjutsu Specialist's four cap-gated catalog picks: Malleable
-- Mirages (class_options, list_name='Malleable Mirages', 79 rows), Genjutsu
-- Inception (class_options, list_name='Genjutsu Inception', 6 rows, cap
-- always 1), Real World Conversion (5 named class_features rows, blank
-- level, read live off class_features rather than hand-curated), and
-- Master of Illusion (4 named options embedded in the granting feature's
-- own text, no rows of their own anywhere, hand-curated the same way
-- Hunter-Nin's Defensive Tactics is — see cmd/n5e/genjutsu.go's
-- masterOfIllusionOptions). One generic table with a category discriminator
-- rather than four near-identical tables, the same generalization
-- character_hunter_nin_picks already makes for its own three simultaneous
-- picks — here there are four, an even stronger case.
--
-- No FK into rules.db's class_options (same cross-DB slug-reference
-- tolerance every other rules.db-keyed table in this schema already uses).
-- UNIQUE on (character_id, category, option_slug): a character either
-- knows a given pick or doesn't.
CREATE TABLE character_genjutsu_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('mirage', 'inception', 'conversion', 'illusion_mastery')),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

CREATE INDEX idx_character_genjutsu_picks_character ON character_genjutsu_picks(character_id);
