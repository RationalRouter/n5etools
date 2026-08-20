-- Migration 0051_scout_nin_null_level_features.sql folded Hidden Technique/
-- Aggressive Technique/Tactical Technique into the same level=2 UPDATE as
-- Shinobi Adept's own 7 catalog options, since all 10 rows shared the same
-- NULL-level bug and happened to be caught by the same sweep. They aren't
-- actually part of Shinobi Adept's catalog, though — they're Signature
-- Technique's own 3 options (class/scout-nin/feature/signature-technique,
-- 10th level: "select one of the following which helps in building out
-- your already broad skillset. Beginning at 18th level, you can instead
-- gain the benefit of two of these options"), and each option's own
-- class_features row (Hidden Technique's ambush-invisibility text,
-- Aggressive Technique's Hit-Die/Chakra-Die-for-HP breathing, Tactical
-- Technique's opportunity-attack immunity) sits immediately after
-- Signature Technique's own introducing row in sort_order/page — fully
-- populated, not missing or truncated data.
--
-- internal/parse/classes.go's knownClassFeatureLevelOverrides["Scout-Nin"]
-- is corrected in the same commit as this migration so any future re-ingest
-- also levels these 3 rows to 10, not 2.
UPDATE class_features
SET level = 10
WHERE class_slug = 'class/scout-nin'
  AND slug IN (
    'class/scout-nin/feature/hidden-technique',
    'class/scout-nin/feature/aggressive-technique',
    'class/scout-nin/feature/tactical-technique'
  )
  AND level = 2;
