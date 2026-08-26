-- Bijuu Slayer's (Mastercraft Titan Upgrade) "makes all weapons deal an
-- additional two die of damage, but only against Demon type foes" clause —
-- a per-Titan, player-toggled flag (checked before rolling, unlike the
-- combat-round-scoped bonuses this app has no turn-tracking state for) that
-- adds +2 damage dice to every Weapon-keyword Titan Upgrade's own rollable
-- attack row while active. Same "plain boolean column, default off" shape
-- is_armor_form already uses (migration 0018_custom_resources.sql).
ALTER TABLE character_companions ADD COLUMN is_demon_foe INTEGER NOT NULL DEFAULT 0;
