-- Widens character_science_nin_subclass_picks' category CHECK to add
-- Shinobi-Ware's own 14th-level Ever Evolving:
--   ever_evolving_seal   "You can spend Creation Points equal to the rank of
--                          an Armor Seal to instantly apply it to your Full
--                          Metal Shinobi armor. You can change this seal
--                          with a Full Turn Action, including choosing no
--                          seal to gain the creation points back and remove
--                          the seal." option_slug is the picked equipment
--                          slug (kind='enhancement_seal', seal_applies_to=
--                          'armor') — the SAME catalog Martial Defense's own
--                          guard slots draw from (see cmd/n5e/
--                          martial_defense.go's loadArmorSealCatalog), but
--                          with no tier-cap filter and no dedicated table of
--                          its own the way Martial Defense's guard-slot
--                          picks get (character_martial_defense_seals,
--                          0037) — reusing this generic table instead since
--                          Ever Evolving is a single-slot (cap 1), Creation-
--                          Points-BUDGET-gated pick shaped like every other
--                          category here, not a flat multi-slot count. Cost
--                          equals the seal's own rank (D=1 ... S=5), spent
--                          from the SAME shared Creation Points pool as
--                          every other pick in this table — see
--                          cmd/n5e/ever_evolving.go.
--
-- Builds on top of 0073_ninjaneer_weapon_designations.sql's own CHECK
-- widening rather than an older list, so this rebuild doesn't silently drop
-- any of its twenty-eight categories back out of the constraint. Also
-- carries forward the quantity column 0071_science_nin_bim_quantity.sql
-- added via a plain ALTER (never part of the CHECK-widening rebuilds before
-- it) — every rebuild after 0071 needs to keep including it explicitly,
-- since a CREATE-TABLE-and-copy rebuild that omitted it would silently drop
-- it.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046/0050/0051/0053/0058/0067/0073 already use.
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
                     'titan_upgrade', 'titan_exosuit_upgrade', 'titan_specialist_crafting_keyword',
                     'enhanced_weapon', 'legendary_weapon', 'perfected_weapon_mark',
                     'ever_evolving_seal')),
    option_slug  TEXT NOT NULL,
    pool         TEXT NOT NULL DEFAULT '' CHECK (pool IN ('', 'mending', 'maiming')),
    quantity     INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

INSERT INTO character_science_nin_subclass_picks_new
    (id, character_id, category, option_slug, pool, quantity, created_at)
SELECT id, character_id, category, option_slug, pool, quantity, created_at
FROM character_science_nin_subclass_picks;

DROP TABLE character_science_nin_subclass_picks;
ALTER TABLE character_science_nin_subclass_picks_new RENAME TO character_science_nin_subclass_picks;

CREATE INDEX idx_character_science_nin_subclass_picks_character ON character_science_nin_subclass_picks(character_id);
