-- Two gaps found while designing the character-sheet derived-stat engine
-- (internal/charsheet): nothing records which skills/tools/languages/saves
-- a character actually has, and nothing distinguishes a mid-creation draft
-- from a finished character.

-- Every proficiency grant, with its source — same provenance shape as
-- character_ability_bonuses/character_feats. Deliberately NOT deduplicated:
-- if both a class and a background grant Stealth, two rows is correct and
-- auditable (undoing one source doesn't silently remove the proficiency if
-- the other still grants it). internal/charsheet only ever needs "is this
-- granted at all" (SELECT DISTINCT value ... WHERE kind = 'skill'), not a
-- count, so the lack of a UNIQUE constraint costs nothing downstream.
CREATE TABLE character_proficiencies (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN
                 ('skill','tool','language','weapon','armor','saving_throw')),
    value        TEXT NOT NULL,               -- 'Stealth', 'Iwagakure', 'Thieves Tools', 'dex'...
    source_kind  TEXT NOT NULL CHECK (source_kind IN
                 ('class','clan','background','feat','asi','bloodline','other')),
    source_ref   TEXT NOT NULL                -- slug or label of the source
);

-- Distinguishes an in-progress character (the creation flow creates the row
-- immediately, at step 1, and every later step is an ordinary edit against
-- it — see the character creation plan) from a finished, playable one, so
-- the list/sheet pages know whether to offer "Resume Creation" or the
-- normal sheet.
ALTER TABLE characters ADD COLUMN creation_status TEXT NOT NULL DEFAULT 'draft'
    CHECK (creation_status IN ('draft','complete'));
