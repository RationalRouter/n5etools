-- Attack and damage modifiers become composed rather than typed in whole.
--
-- Rather than a static number typed in, the player chooses proficiency +
-- ability + a custom value, with damage composed as ability + custom value.
-- The reason is the same one 0007_custom_attacks.sql gives for those rows existing
-- at all — N5E is heavily homebrewed and the app cannot derive every attack —
-- but a hand-typed total has a worse problem than being hard to derive: it
-- goes stale silently. Level up and the proficiency bonus changes; raise
-- Strength and every Strength attack changes; neither touches a number the
-- player typed once, and nothing on the sheet says so. Storing the PARTS means
-- the total is recomputed on every render, exactly like every other number on
-- this sheet (see charsheet's package comment: store inputs, never derived
-- math).
--
-- Backwards compatible on purpose. attack_bonus and damage_bonus keep their
-- columns and their values, and simply change meaning from "the whole
-- modifier" to "the flat part of it" — with attack_ability empty and
-- attack_prof 'none', which is what every existing row gets, the composed
-- total is the old typed number unchanged. Nobody's sheet moves on upgrade.
ALTER TABLE character_custom_attacks ADD COLUMN attack_ability TEXT NOT NULL DEFAULT '';
ALTER TABLE character_custom_attacks ADD COLUMN attack_prof    TEXT NOT NULL DEFAULT 'none';
ALTER TABLE character_custom_attacks ADD COLUMN damage_ability TEXT NOT NULL DEFAULT '';

-- Per-weapon overrides for the rows the app DOES derive: an equipped weapon's
-- attack row. Those are computed from the weapon's printed properties
-- (buildAttacks picks Strength or Dexterity off finesse/thrown/ammunition) and
-- always add the full proficiency bonus, which is right until a feature says
-- otherwise — and this app models no weapon proficiency at all, so there was
-- no way to say otherwise.
--
-- A row here is an override, not a record: absent means "keep doing what the
-- weapon's properties imply", and each column is independently optional, so
-- overriding the damage stat does not force you to restate the attack stat.
-- Keyed by inventory row rather than by item slug because two of the same
-- weapon can be carried and tuned differently, and ON DELETE CASCADE from the
-- inventory row means unequipping and dropping a weapon takes its tuning with
-- it rather than leaving an orphan to reattach to whatever reuses the id.
CREATE TABLE character_weapon_attack_options (
    character_id   INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    inventory_id   INTEGER NOT NULL REFERENCES character_inventory(id) ON DELETE CASCADE,
    attack_ability TEXT NOT NULL DEFAULT '',      -- '' = whatever the weapon's properties imply
    attack_prof    TEXT NOT NULL DEFAULT 'full',  -- 'none' | 'half' | 'full'
    attack_bonus   INTEGER NOT NULL DEFAULT 0,
    damage_ability TEXT NOT NULL DEFAULT '',      -- '' = same ability as the attack
    damage_bonus   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (character_id, inventory_id)
);
