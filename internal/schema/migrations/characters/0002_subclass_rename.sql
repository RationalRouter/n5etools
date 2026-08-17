-- Matches rules.db migration 0011_subclass_rename.sql — see its comment for
-- the rationale ("archetype" is a distinct real rules term, not a synonym
-- for subclass).
ALTER TABLE character_archetypes RENAME TO character_subclasses;
ALTER TABLE character_subclasses RENAME COLUMN archetype_slug TO subclass_slug;
