-- Widens character_science_nin_subclass_picks' category CHECK to cover
-- Mixed Studies (18th level, base class): "You gain the 3rd Level features
-- of another Scientific Inquiry. You cannot select the one you chose at
-- 3rd Level." A single-slot, freely re-picked category — the book states no
-- lock, so re-submitting a different Inquiry replaces the pick rather than
-- adding a second one:
--   mixed_studies_inquiry   base class, cap 1, option_slug is the chosen
--                            OTHER subclass's own slug (e.g.
--                            'class/science-nin/group/scientific-inquiry/
--                            grenadier'), not a class_options/
--                            class_option_entries reference like every
--                            other category in this table — see
--                            cmd/n5e/science_nin.go's handleMixedStudiesPickAdd.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046_science_nin_subclass_picks_more_categories.sql already uses.
CREATE TABLE character_science_nin_subclass_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon',
                     'shinobi_ware_upgrade', 'evolved_upgrade', 'spyware_program', 'quick_hack',
                     'air_treck_enhancement', 'regalia', 'technobi_mechanization', 'snb_upgrade',
                     'mixed_studies_inquiry')),
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
