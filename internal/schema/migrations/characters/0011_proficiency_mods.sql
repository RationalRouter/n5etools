-- Part 9's rework of the Tool Proficiencies & Custom Skills box: every line
-- in it becomes rollable, with the same ability + proficiency + flat-bonus
-- tweak menu Initiative and every attack already has (charsheet.ComposeModifier).
--
-- Keyed by (character_id, kind, value) rather than a character_proficiencies
-- row id: that table is deliberately not deduplicated (a class AND a
-- background can each grant the same tool, as two rows) and the sheet only
-- ever displays DISTINCT values, so the identity a modifier hangs off is the
-- displayed name, not any one grant. This also means a tool re-granted by a
-- different source (or the same custom skill re-typed after being deleted)
-- keeps whatever the player tuned. 'language' is deliberately not a valid
-- kind here — languages have no roll and nothing to adjust.
CREATE TABLE character_proficiency_mods (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('tool','skill')),
    value        TEXT NOT NULL,
    ability      TEXT NOT NULL DEFAULT '',      -- '' = no ability term
    prof_mode    TEXT NOT NULL DEFAULT 'none' CHECK (prof_mode IN ('full','half','none')),
    bonus        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (character_id, kind, value)
);
