-- Tracks Hunter-Nin's three cap-gated catalog picks: Hunters Patterns
-- (class_options, list_name='Hunters Patterns'), Hunters Exploits
-- (class_options, list_name='Hunters Exploits'), and Defensive Tactics (a
-- hand-curated 4-option catalog with no class_options rows of its own —
-- the book embeds its options directly in the granting feature's text, see
-- cmd/n5e/hunter_nin.go's defensiveTacticsOptions). One generic table with
-- a category discriminator rather than three near-identical tables (the
-- shape character_martial_techniques established) — these three catalogs
-- are simultaneous, equally-shaped uses of the same "known set, capped by
-- level, freely re-editable" pattern, not three independent one-off needs.
--
-- No FK into rules.db's class_options (same cross-DB slug-reference
-- tolerance every other rules.db-keyed table in this schema already
-- uses). UNIQUE on (character_id, category, option_slug): a character
-- either knows a given pick or doesn't.
CREATE TABLE character_hunter_nin_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('pattern', 'exploit', 'defensive_tactic')),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

CREATE INDEX idx_character_hunter_nin_picks_character ON character_hunter_nin_picks(character_id);
