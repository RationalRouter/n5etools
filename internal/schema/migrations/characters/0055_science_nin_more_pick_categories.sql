-- Widens character_science_nin_subclass_picks' category CHECK to cover four
-- more subclass-capstone-driven picks, all restricted to a subset of a
-- character's own already-known picks in a sibling category (the same
-- "restricted to already-known subset via cross-reference" shape Perma
-- Perk/Quick Hack/Infused Genius already use), plus one permanent pick:
--   bim_specialist            Grenadier, B.I.M Specialist (9th level): one
--                              designated B.I.M type, cap 1, restricted to
--                              the character's own known 'bim' picks — see
--                              cmd/n5e/science_nin_subclasses.go's
--                              scienceNinGrenadierData.DesignatedBIM.
--   sheep_and_shepherd_serum  Mad Scientist, The Sheep and the Shepherd
--                              (17th level): one designated dual-effect
--                              Inversion Serum, cap 1, restricted to the
--                              character's own known 'inversion_serum'
--                              picks whose own catalog entry has no fixed
--                              CCD pool — see scienceNinMadScientistData.
--                              DesignatedSerum.
--   shinjutsu_upgrade         Shinobi-Ware, In His Image (9th level): one
--                              Shinjutsu-tier Shinobi-Ware Upgrade, cap 1,
--                              PERMANENT (the book states "You can not
--                              change this later" — no delete route is
--                              wired for this category, unlike every other
--                              category in this table) — see
--                              scienceNinShinobiWareData.ShinjutsuUpgrade.
--   snb_upgrade_permanent     S.N.B Specialist, Future of Shinobi: S.N.B
--                              (20th level): 3 slot-free, Creation-Point-
--                              free S.N.B Upgrades, cap 3, restricted to
--                              the character's own known 'snb_upgrade'
--                              picks — see scienceNinSNBSpecialistData.
--                              PermanentCap/KnownPermanent.
--
-- Builds on top of 0051_science_nin_infused_genius.sql's own CHECK widening
-- (mixed_studies_inquiry, infused_tool) rather than an older list, so this
-- rebuild doesn't silently drop either category back out of the constraint.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046/0050/0051 already use.
CREATE TABLE character_science_nin_subclass_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon',
                     'shinobi_ware_upgrade', 'evolved_upgrade', 'spyware_program', 'quick_hack',
                     'air_treck_enhancement', 'regalia', 'technobi_mechanization', 'snb_upgrade',
                     'mixed_studies_inquiry', 'infused_tool',
                     'bim_specialist', 'sheep_and_shepherd_serum', 'shinjutsu_upgrade', 'snb_upgrade_permanent')),
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
