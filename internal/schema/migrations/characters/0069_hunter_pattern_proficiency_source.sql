-- Kleptomaniac and Habitual Researcher (Hunters Patterns, class_options —
-- not class_features rows, so they were never wired to grant anything)
-- resolve their own skill/tool proficiency choice as a character_
-- proficiencies row tagged source_kind='hunter_pattern' (internal/charstore/
-- hunter_pattern_proficiency.go's ApplyHunterPatternProficiencies) — a value
-- 0003_proficiencies.sql's original CHECK constraint doesn't allow.
--
-- SQLite can't ALTER a CHECK constraint in place, so this rebuilds the
-- table: new table with the widened constraint, copy every row across, drop
-- the old one, rename. character_proficiencies has gained no columns since
-- 0003 first created it (0011/0028 only reference it, never ALTER it), so
-- the rebuilt table is identical to the original but for the CHECK.
CREATE TABLE character_proficiencies_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN
                 ('skill','tool','language','weapon','armor','saving_throw')),
    value        TEXT NOT NULL,
    source_kind  TEXT NOT NULL CHECK (source_kind IN
                 ('class','clan','background','feat','asi','bloodline','other','hunter_pattern')),
    source_ref   TEXT NOT NULL
);

INSERT INTO character_proficiencies_new (id, character_id, kind, value, source_kind, source_ref)
SELECT id, character_id, kind, value, source_kind, source_ref FROM character_proficiencies;

DROP TABLE character_proficiencies;
ALTER TABLE character_proficiencies_new RENAME TO character_proficiencies;
