-- Widens character_scout_nin_picks' category CHECK to add 4 more
-- categories closing further Scout-Nin Group-1 gaps:
--   signature_technique    Signature Technique (base class, 10th/18th
--                          level), a 3-option catalog (Hidden/Aggressive/
--                          Tactical Technique) mistakenly folded into
--                          Shinobi Adept's own catalog until this pass —
--                          see cmd/n5e/scout_nin.go's signatureTechniqueSlugs.
--   mobile_savant          Mobile Savant (Pathfinder Scout, 3rd/6th/9th/
--                          14th level), a 4-jutsu catalog — option_slugs
--                          here are jutsu slugs, not class_features slugs.
--   tactical_superiority   Tactical Superiority (Tactical Scout, 6th/14th/
--                          20th level), a cross-subclass Maneuver pick
--                          drawing from several OTHER subclasses' own
--                          Maneuvers catalogs.
--   signature_maneuver     Signature Maneuver (Tactical Scout, 3rd/9th
--                          level), a pick of an already-known Tactical-
--                          keyword Maneuver.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0048_scout_nin_maneuvers_category.sql already uses.
CREATE TABLE character_scout_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
        'shinobi_adept', 'jack_of_all', 'maneuvers',
        'signature_technique', 'mobile_savant', 'tactical_superiority', 'signature_maneuver'
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
