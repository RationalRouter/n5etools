-- =============================================================================
-- 0006_level_progression: level becomes a real class level, not a display
-- override.
--
-- Up to now the character sheet's Level box wrote a 'level' row into
-- character_overrides, which changed the DISPLAYED level (and, through it,
-- the proficiency bonus) but deliberately granted nothing else: hit dice
-- still counted character_classes.levels, and Max HP/Max Chakra still summed
-- only the level-1 gain row written at creation. A level 7 character
-- therefore showed 1/1 hit dice and level 1 hit points: level override and
-- progression did not work, and Chakra did not progress either on level up.
--
-- The fix moves the number to where everything already reads it from, so
-- this migration folds any existing override into character_classes.levels
-- and drops the override rows. Characters whose recorded level rises here
-- gain no character_level_gains rows: charsheet.Compute grants the fixed
-- (unrolled) hit-die/chakra-die value for any level that has no row, so the
-- HP and chakra follow automatically and stay correct if Constitution
-- changes later.
--
-- No schema change — this is a data fix only. There is nothing to reverse:
-- a level override and a class level meant the same thing to the player.
-- =============================================================================

UPDATE character_classes
SET levels = (
        SELECT CAST(o.value AS INTEGER)
        FROM character_overrides o
        WHERE o.character_id = character_classes.character_id AND o.field = 'level'
    )
WHERE order_index = 0
  AND EXISTS (
        SELECT 1 FROM character_overrides o
        WHERE o.character_id = character_classes.character_id
          AND o.field = 'level'
          -- The CHECK on character_classes.levels is 1..20; a junk or
          -- out-of-range override is dropped below rather than allowed to
          -- abort the migration.
          AND CAST(o.value AS INTEGER) BETWEEN 1 AND 20
    );

DELETE FROM character_overrides WHERE field = 'level';
