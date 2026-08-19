-- Widens character_scout_nin_picks' category CHECK to add 'maneuvers' —
-- the subclass-scoped Maneuvers Known cap+catalog pick (cmd/n5e/scout_nin.go),
-- landed after 0047_scout_nin_picks.sql shipped with only 'shinobi_adept'
-- and 'jack_of_all'. Unlike those two (base-class-wide, one shared
-- catalog), Maneuvers' own option_slugs are subclass-specific (e.g.
-- 'class/scout-nin/option/arbiter-maneuvers/...') — a character who
-- switches subclass keeps whatever rows are already stored here (the app
-- layer, not this table, decides which of them count against the new
-- subclass's own cap; see loadScoutNinTabData's own comment).
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046_science_nin_subclass_picks_more_categories.sql already uses.
CREATE TABLE character_scout_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('shinobi_adept', 'jack_of_all', 'maneuvers')),
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
