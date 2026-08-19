-- Scout-Nin has 16 non-ASI class_features rows shipped with NULL level.
-- LoadGrantedFeatures treats a NULL level as always-granted, so every
-- Scout-Nin character at any level (even 1st) was shown as already
-- possessing all 16 of these catalog-option features at once, despite the
-- book gating them behind real level-gated picks (Deft Explorer's
-- 1st/6th/11th sub-tiers, Shinobi Adept's 2-of-10 pick starting 2nd level,
-- Signature Jutsu's 1-of-3 pick starting 7th level).
--
-- internal/parse/classes.go now fixes this on any future re-ingest (writing
-- the class_features.level column directly, the same column upsertLeveledFeature
-- always writes to -- level_override is a separate human-only pin that ingest
-- never populates, so this migration matches that column, not level_override):
--   - levelSuffixRe/stripLevelSuffix recognizes the "(<n>th Level)"
--     parenthetical glued into Deft Explorer's Canny/Mobile/Tireless
--     sub-tier NAMES (a PDF-extraction artifact) and strips it into the
--     feature's Level. Stripping the suffix also changes the stored NAME,
--     which changes the row's slug (slug is derived from name at ingest
--     time) -- this migration renames the slug to match what the next
--     re-ingest will produce, so the two converge on the same row instead
--     of the old slug being orphaned and a duplicate new one created.
--     internal/features/choices.go's existing Canny skill-choice entry is
--     updated to the new slug in the same commit as this migration.
--   - knownClassFeatureLevelOverrides tags Shinobi Adept's 10 catalog
--     options to Shinobi Adept's own 2nd level, and Signature Jutsu's 3
--     effect options to Signature Jutsu's own 7th level -- neither catalog
--     states a level anywhere in its own option text, only on the
--     introducing feature.
--
-- This migration repairs the already-shipped rows the same way.
UPDATE class_features
SET slug = 'class/scout-nin/feature/canny', name = 'Canny', level = 1
WHERE slug = 'class/scout-nin/feature/canny-1-st-level';

UPDATE class_features
SET slug = 'class/scout-nin/feature/mobile', name = 'Mobile', level = 6
WHERE slug = 'class/scout-nin/feature/mobile-6th-level';

UPDATE class_features
SET slug = 'class/scout-nin/feature/tireless', name = 'Tireless', level = 11
WHERE slug = 'class/scout-nin/feature/tireless-11-th-level';

UPDATE class_features
SET level = 2
WHERE class_slug = 'class/scout-nin'
  AND slug IN (
    'class/scout-nin/feature/shinobis-tactics',
    'class/scout-nin/feature/shinobis-general-literacy',
    'class/scout-nin/feature/shinobis-tool-competency',
    'class/scout-nin/feature/shinobis-precision',
    'class/scout-nin/feature/shinobis-edge',
    'class/scout-nin/feature/shinobis-drive',
    'class/scout-nin/feature/shinobis-focus',
    'class/scout-nin/feature/hidden-technique',
    'class/scout-nin/feature/aggressive-technique',
    'class/scout-nin/feature/tactical-technique'
  );

UPDATE class_features
SET level = 7
WHERE class_slug = 'class/scout-nin'
  AND slug IN (
    'class/scout-nin/feature/signature-power',
    'class/scout-nin/feature/signature-ramping',
    'class/scout-nin/feature/signature-control'
  );

-- Separate mistagging bug, same fix shape: Jack of All, Master of None's 5
-- named generalizations (Combat/Control/Mobility/Skill/Support) were each
-- tagged level=11 in class_features, even though the granting feature itself
-- unlocks at 5th level -- each option's own prose states only a LATER
-- numeric tier bump ("...increases to +2 at 11th level"), which the level
-- regex matched instead of the feature's real 5th-level unlock.
UPDATE class_features
SET level = 5
WHERE class_slug = 'class/scout-nin'
  AND slug IN (
    'class/scout-nin/feature/combat',
    'class/scout-nin/feature/control',
    'class/scout-nin/feature/mobility',
    'class/scout-nin/feature/skill',
    'class/scout-nin/feature/support'
  )
  AND level = 11;
