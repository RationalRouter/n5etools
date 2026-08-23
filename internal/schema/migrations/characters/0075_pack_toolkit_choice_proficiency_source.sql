-- Pack-unpack toolkit picks ("1 Toolkit (pick one)", "3 Toolkits (pick
-- three)", "1 Hackers Kit or Security Kit (pick one)" — see
-- starting_equipment.go's parsePackContents) resolve into a real
-- character_proficiencies row tagged source_kind='pack'
-- (internal/charstore/sheet.go's ResolvePackToolkitChoice) — a value
-- 0003_proficiencies.sql's original CHECK constraint, last widened by 0069
-- for 'hunter_pattern', doesn't allow.
--
-- Same rebuild shape as 0069: SQLite can't ALTER a CHECK constraint in
-- place, so this is a new table with the widened constraint, copy every row
-- across, drop the old one, rename. character_proficiencies has gained no
-- columns since 0069, so the rebuilt table is identical but for the CHECK.
CREATE TABLE character_proficiencies_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN
                 ('skill','tool','language','weapon','armor','saving_throw')),
    value        TEXT NOT NULL,
    source_kind  TEXT NOT NULL CHECK (source_kind IN
                 ('class','clan','background','feat','asi','bloodline','other','hunter_pattern','pack')),
    source_ref   TEXT NOT NULL
);

INSERT INTO character_proficiencies_new (id, character_id, kind, value, source_kind, source_ref)
SELECT id, character_id, kind, value, source_kind, source_ref FROM character_proficiencies;

DROP TABLE character_proficiencies;
ALTER TABLE character_proficiencies_new RENAME TO character_proficiencies;
