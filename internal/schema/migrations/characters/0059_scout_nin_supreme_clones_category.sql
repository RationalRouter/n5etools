-- Widens character_scout_nin_picks' category CHECK to add 'supreme_clones' —
-- Cloning Scout's own flat 20th-level pick of an already-known Maneuver
-- (cmd/n5e/scout_nin.go's supremeClonesFeatureSlug), landed after
-- 0054_scout_nin_more_pick_categories.sql shipped the previous 7. Like
-- 'signature_maneuver', this category's own option_slugs always overlap
-- with a subset of that character's own 'maneuvers' picks, but with no
-- keyword restriction and no escalating cap.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0054_scout_nin_more_pick_categories.sql already used.
CREATE TABLE character_scout_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
        'shinobi_adept', 'jack_of_all', 'maneuvers',
        'signature_technique', 'mobile_savant', 'tactical_superiority', 'signature_maneuver',
        'supreme_clones'
    )),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

INSERT INTO character_scout_nin_picks_new (id, character_id, category, option_slug, created_at)
SELECT id, character_id, category, option_slug, created_at
FROM character_scout_nin_picks;

DROP TABLE character_scout_nin_picks;
ALTER TABLE character_scout_nin_picks_new RENAME TO character_scout_nin_picks;

CREATE INDEX idx_character_scout_nin_picks_character ON character_scout_nin_picks(character_id);
