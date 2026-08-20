-- Widens character_science_nin_subclass_picks' category CHECK to cover
-- Infused Genius (11th level, base class): "You can select one Scientific
-- Ninja Tool of Creation Point Cost 8 or lower... You can have a number of
-- Infused Tools equal to your Intelligence modifier." Restricted to the
-- character's own already-known Scientific Ninja Tools (character_science_
-- nin_tools), the same "restricted to already-known subset via
-- cross-reference" shape Perma Perk/Quick Hack already use:
--   infused_tool   base class, cap = Intelligence modifier (min 0),
--                   option_slug is a class_option_entries slug shared with
--                   the base Scientific Ninja Tools catalog (see
--                   cmd/n5e/science_nin.go's InfusedGenius field) — not a
--                   new catalog of its own.
--
-- Builds on top of 0050_science_nin_mixed_studies_pick.sql's own CHECK
-- widening (mixed_studies_inquiry) rather than 0046's older list, so this
-- rebuild doesn't silently drop that category back out of the constraint.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046/0050 already use.
CREATE TABLE character_science_nin_subclass_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon',
                     'shinobi_ware_upgrade', 'evolved_upgrade', 'spyware_program', 'quick_hack',
                     'air_treck_enhancement', 'regalia', 'technobi_mechanization', 'snb_upgrade',
                     'mixed_studies_inquiry', 'infused_tool')),
    option_slug  TEXT NOT NULL,
    pool         TEXT NOT NULL DEFAULT '' CHECK (pool IN ('', 'mending', 'maiming')),
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

INSERT INTO character_science_nin_subclass_picks_new
    (id, character_id, category, option_slug, pool, created_at)
SELECT id, character_id, category, option_slug, pool, created_at
FROM character_science_nin_subclass_picks;

DROP TABLE character_science_nin_subclass_picks;
ALTER TABLE character_science_nin_subclass_picks_new RENAME TO character_science_nin_subclass_picks;

CREATE INDEX idx_character_science_nin_subclass_picks_character ON character_science_nin_subclass_picks(character_id);
