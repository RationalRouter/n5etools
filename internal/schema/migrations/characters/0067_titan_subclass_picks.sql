-- Widens character_science_nin_subclass_picks' category CHECK to add three
-- Mech Crafter (Titan) categories:
--   titan_upgrade                  a Mech/Weapon-keyword upgrade installed
--                                  into one of the Titan's own Titan Slots
--                                  (Ordnance Training, cap = Proficiency
--                                  Bonus). option_slug is the picked Titan
--                                  Upgrades class_option_entries.slug (or
--                                  the Mastercraft tier's own class_options
--                                  slug for Bijuu Slayer, which has no
--                                  entries row). Spends from the SAME
--                                  Creation Points budget as every other
--                                  category and the base Scientific Ninja
--                                  Tools catalog -- see
--                                  cmd/n5e/titan.go's
--                                  titanEffectiveUpgradeCost.
--   titan_exosuit_upgrade          Endless Work's own separate 1 (2 at
--                                  14th level) Mech-keyword-only slot,
--                                  restricted to upgrades of Cost 8 or
--                                  lower ("Greater or lower") -- see
--                                  cmd/n5e/titan.go's own header doc on the
--                                  Refined/Greater tier merge this
--                                  restriction is read against.
--   titan_specialist_crafting_keyword  Specialist Crafting's (14th level)
--                                  own single-slot "Mech or Weapon" keyword
--                                  designation -- option_slug is the
--                                  literal string "mech" or "weapon", cap
--                                  1, freely re-picked (delete the current
--                                  one, then add the other), same "trust
--                                  the player" boundary Mixed Studies'
--                                  own single-slot Inquiry pick already
--                                  draws.
--
-- Builds on top of 0058_science_nin_ascended_wow.sql's own CHECK widening
-- rather than an older list, so this rebuild doesn't silently drop any of
-- its twenty-two categories back out of the constraint.
--
-- SQLite can't ALTER a CHECK constraint in place -- rebuild the table, same
-- pattern 0046/0050/0051/0053/0058 already use.
CREATE TABLE character_science_nin_subclass_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon',
                     'shinobi_ware_upgrade', 'evolved_upgrade', 'spyware_program', 'quick_hack',
                     'air_treck_enhancement', 'regalia', 'technobi_mechanization', 'snb_upgrade',
                     'mixed_studies_inquiry', 'infused_tool',
                     'bim_specialist', 'sheep_and_shepherd_serum', 'shinjutsu_upgrade', 'snb_upgrade_permanent',
                     'ascended_wow',
                     'titan_upgrade', 'titan_exosuit_upgrade', 'titan_specialist_crafting_keyword')),
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
