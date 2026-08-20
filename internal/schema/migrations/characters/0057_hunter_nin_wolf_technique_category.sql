-- Widens character_hunter_nin_picks' category CHECK to cover a 7th
-- Hunter's Creed catalog pick: Wolves Legacy's own Wolf Techniques (10th
-- level, "Select 1 of the following jutsu... The jutsu selected are now
-- counted as your Wolf Techniques... added to your Jutsu known and also
-- gain their listed effects"). Six of the book's eight named jutsu resolve
-- to real jutsu rows (Weapon Deflect/Weapon Break/Earth Breaker/Triple
-- Windmill Blades/Ichimonji/Judgement Cut); Kunai Assault and Reapers Swing
-- have no matching jutsu row anywhere in this database under any name or
-- spelling variant, so they're excluded from the catalog — a data-
-- ingestion gap, not a picker bug.
--
--   wolf_technique    Wolves Legacy, Wolf Techniques (10th level): 6-option
--                     named-jutsu table, cap 1, no re-cap. Picking one both
--                     records the pick and learns the matching jutsu (same
--                     "learn the matching jutsu on pick" side effect
--                     arsenal_item's own Manipulated Tools entries already
--                     use). The alternate casting cost this feature grants
--                     ("spending two uses of your Prosthetic Attachment
--                     instead of spending chakra... always cast as if it
--                     were B-Rank, ignoring its component requirement")
--                     has no analog anywhere in the jutsu-casting-cost
--                     pipeline and stays manual — only which jutsu is known
--                     as a Wolf Technique is tracked.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0049_hunter_nin_new_pick_categories.sql already used.
CREATE TABLE character_hunter_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'pattern', 'exploit', 'defensive_tactic',
                     'warden_weapon', 'warden_weapon_property',
                     'medical_technique', 'shadow_technique', 'arsenal_item',
                     'toxic_technique', 'vice_technique', 'void_technique',
                     'prosthetic_attachment', 'wolf_technique')),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

INSERT INTO character_hunter_nin_picks_new (id, character_id, category, option_slug, created_at)
SELECT id, character_id, category, option_slug, created_at
FROM character_hunter_nin_picks;

DROP TABLE character_hunter_nin_picks;
ALTER TABLE character_hunter_nin_picks_new RENAME TO character_hunter_nin_picks;

CREATE INDEX idx_character_hunter_nin_picks_character ON character_hunter_nin_picks(character_id);
