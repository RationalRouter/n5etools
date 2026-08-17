-- Equipment columns for the Mastersheet's armor model. N5E measures carry
-- capacity in BULK (its own inventory unit), not pounds, and armor AC is
-- computed from a calculation type + up to two ability modifiers + a cap:
--   AC = 10 + armor bonus + min(ability mods, max mod) (+ half prof, per
--   proficiency rules). armor_category extends beyond light/medium/heavy
-- with 'ability-mod' (clan/class special wearables like the Dust Coat) and
-- 'custom' (the unarmored Skin baseline).
ALTER TABLE equipment ADD COLUMN bulk REAL;
ALTER TABLE equipment ADD COLUMN armor_ability_1 TEXT;  -- 'DEX', 'INT', ...
ALTER TABLE equipment ADD COLUMN armor_ability_2 TEXT;  -- second mod: 'PROF', 'INT', ...
ALTER TABLE equipment ADD COLUMN armor_max_mod REAL;    -- cap on the ability mod; NULL = uncapped
