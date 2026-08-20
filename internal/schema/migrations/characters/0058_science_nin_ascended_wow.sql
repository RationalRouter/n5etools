-- Widens character_science_nin_subclass_picks' category CHECK to add one
-- more subclass-capstone-driven pick, the same "restricted to already-known
-- subset via cross-reference" shape bim_specialist/sheep_and_shepherd_serum
-- already use:
--   ascended_wow   Elemental Innovationist, Elemental Innovation (17th
--                  level): one designated Ascended W.o.W, cap 1, restricted
--                  to the character's own known 'wow' picks (any known
--                  W.o.W qualifies, no exclusion clause — same
--                  unrestricted shape bim_specialist uses) — see
--                  cmd/n5e/science_nin_subclasses.go's
--                  scienceNinElementalInnovationistData.DesignatedWoW.
--
-- Builds on top of 0053_science_nin_more_pick_categories.sql's own CHECK
-- widening rather than an older list, so this rebuild doesn't silently drop
-- any of its five categories back out of the constraint.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046/0050/0051/0053 already use.
CREATE TABLE character_science_nin_subclass_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon',
                     'shinobi_ware_upgrade', 'evolved_upgrade', 'spyware_program', 'quick_hack',
                     'air_treck_enhancement', 'regalia', 'technobi_mechanization', 'snb_upgrade',
                     'mixed_studies_inquiry', 'infused_tool',
                     'bim_specialist', 'sheep_and_shepherd_serum', 'shinjutsu_upgrade', 'snb_upgrade_permanent',
                     'ascended_wow')),
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
