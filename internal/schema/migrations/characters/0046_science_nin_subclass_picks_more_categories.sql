-- Widens character_science_nin_subclass_picks' category CHECK to cover the
-- final 4 Science-Nin subclasses' own cap+catalog picks, same table shape
-- 0044_science_nin_subclass_picks.sql already established for the first
-- four (eip/wow/perma_perk/bim/inversion_serum/arsenal_mod/
-- perfected_weapon):
--   shinobi_ware_upgrade    Shinobi-Ware, class_options 'Shinobi-Ware
--                            Upgrades', cap = Proficiency Bonus
--   evolved_upgrade          Shinobi-Ware, Glorious Evolution (6th level):
--                            up to 2 slot-free Evolved Upgrades, tier
--                            gated by character level (Refined at 6th,
--                            also Greater at 9th, also Superior at 14th);
--                            option_slug points at a Shinobi-Ware Upgrades
--                            entry the same way perma_perk points at an
--                            eip entry
--   spyware_program          Spyware, class_options 'Spyware Programs',
--                            cap = Proficiency Bonus
--   quick_hack                Spyware, Netrunner (14th level): one
--                            slot-free Program made always-prepared;
--                            option_slug points at a spyware_program entry
--   air_treck_enhancement    Storm Rider, class_options 'Air Treck
--                            Enhancements' (Minor through Mastercraft
--                            tiers only — Regalia is its own category
--                            below, not part of this cap), cap =
--                            Proficiency Bonus
--   regalia                   Storm Rider, King's Road (14th level): a
--                            single-pick hand-curated 8-option catalog
--                            (the 'Air Treck Enhancements' list's own
--                            Regalia row bundles all 8 with no per-entry
--                            'Cost:' stat line, so it was never split into
--                            class_option_entries by LoadClassOptionEntries
--                            — option_slug is a hand-assigned slug, not a
--                            class_option_entries reference)
--   technobi_mechanization    Technobi, class_options 'Technobi
--                            Mechanizations', cap = Proficiency Bonus
--   snb_upgrade                S.N.B Specialist, class_options 'S.N.B
--                            Upgrades' (re-scoped off the former phantom
--                            'S.N.B Upgrades' subclass by
--                            0048_snb_upgrades_phantom_subclass.sql,
--                            rules.db), cap = Proficiency Bonus
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0014_custom_attack_item_kind.sql already uses.
CREATE TABLE character_science_nin_subclass_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon',
                     'shinobi_ware_upgrade', 'evolved_upgrade', 'spyware_program', 'quick_hack',
                     'air_treck_enhancement', 'regalia', 'technobi_mechanization', 'snb_upgrade')),
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
