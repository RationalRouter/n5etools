-- Widens character_science_nin_subclass_picks' category CHECK to add
-- Ninjaneer's three single-slot weapon designations — WHICH specific owned
-- weapon (a character_inventory row) currently fills each of Enhanced
-- Arsenal/Warrior of Science/A Weapon to Surpass's own weapon-shaped
-- feature:
--   enhanced_weapon         Enhanced Arsenal (3rd level): the character's
--                            own Enhanced Weapon. option_slug is the
--                            designated character_inventory.id, cast to
--                            text — unlike every category above (which
--                            point at a class_options/class_option_entries
--                            row, or a hand-assigned string for Regalia/the
--                            Titan keyword pick), this is the first
--                            category to reference a character's own
--                            inventory row rather than static rules
--                            content, since two owned copies of the same
--                            weapon must stay distinguishable. Gates
--                            Enhanced Arsenal's own "use Intelligence in
--                            place of Dexterity for Enhanced Finesse
--                            weapons" clause (applied via
--                            character_weapon_attack_options, not tracked
--                            again here) and, once The Future of Shinobi:
--                            Weapons (20th) is also granted, that same
--                            weapon's own flat +Proficiency-Bonus damage
--                            bonus.
--   legendary_weapon         Warrior of Science (9th level): the
--                            character's own Legendary Weapon. Same
--                            option_slug shape as enhanced_weapon. The
--                            temporary 20-CCD-to-activate/10-CCD-upkeep
--                            state itself is a plain boolean
--                            (characters.ninjaneer_legendary_weapon_active,
--                            0072_ninjaneer_legendary_weapon_active.sql),
--                            independent of this designation — WHICH
--                            weapon is the Legendary Weapon and WHETHER
--                            Warrior of Science is currently active are
--                            tracked separately, the same way a Storm
--                            Rider's Air Trecks weapon and its own
--                            attack/damage override are two different
--                            things. Once The Future of Shinobi: Weapons is
--                            granted, this same weapon gets a flat
--                            +2x-Proficiency-Bonus damage bonus.
--   perfected_weapon_mark    A Weapon to Surpass (3rd level): the
--                            character's own (single) Perfected Weapon,
--                            for the sole purpose of The Future of Shinobi:
--                            Weapons' own flat +Intelligence-Modifier
--                            damage bonus. Deliberately a DIFFERENT
--                            category from the existing perfected_weapon
--                            above — that one tracks the count of free
--                            Minor Arsenal Modifications A Weapon to
--                            Surpass grants (capped at Intelligence
--                            Modifier, since the book allows multiple
--                            Perfected Weapons at once), an entirely
--                            unrelated tracked quantity that happens to
--                            share the same granting feature.
--
-- Builds on top of 0067_titan_subclass_picks.sql's own CHECK widening
-- rather than an older list, so this rebuild doesn't silently drop any of
-- its twenty-five categories back out of the constraint. Also carries
-- forward the quantity column 0071_science_nin_bim_quantity.sql added via a
-- plain ALTER (never part of the CHECK-widening rebuilds before it) — every
-- rebuild after 0071 needs to keep including it explicitly, since a
-- CREATE-TABLE-and-copy rebuild that omitted it would silently drop it.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046/0050/0051/0053/0058/0067 already use.
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
                     'enhanced_weapon', 'legendary_weapon', 'perfected_weapon_mark')),
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
