-- Widens character_scout_nin_picks' category CHECK to add
-- 'void_soul_jutsu' — Void Soul Awakening's own "Has a number of Jutsu
-- knowns equal to your Proficiency Bonus" cap+catalog pick (cmd/n5e/
-- void_soul.go). option_slugs here are jutsu slugs, the same shape
-- 'mobile_savant' already established for a jutsu-slug-keyed category
-- rather than a class_features/class_options slug — these are jutsu the
-- Void Soul itself knows and casts, not the player, so they are
-- deliberately NOT written into character_jutsu (see void_soul.go's own
-- header doc for why conflating the two tables would let the player appear
-- able to cast jutsu the book reserves for their Void Soul only).
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0048/0056/0059 already use for this exact table.
CREATE TABLE character_scout_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
        'shinobi_adept', 'jack_of_all', 'maneuvers',
        'signature_technique', 'mobile_savant', 'tactical_superiority', 'signature_maneuver',
        'supreme_clones', 'void_soul_jutsu'
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
