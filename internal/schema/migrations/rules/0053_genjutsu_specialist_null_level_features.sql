-- Genjutsu Specialist has 6 class_features rows shipped with NULL level.
-- LoadGrantedFeatures treats a NULL level as always-granted, and the Class
-- Reference popup (cmd/n5e/reference.go's loadCharacterReference,
-- character_reference.html) renders a NULL/invalid level as "Always on"
-- rather than dimmed -- so a 1st-level Genjutsu Specialist saw all 6 of
-- these as already possessed, despite the book gating them behind real
-- level-gated features (Ability Score Improvement/Feat at 4th and 8th,
-- Real World Conversion's 5-option catalog starting 5th level).
--
-- internal/parse/classes.go's knownClassFeatureLevelOverrides now fixes
-- this on any future re-ingest, tagging Ability Score Improvement/Feat to
-- 4th level and the 5 Actualized-* Conversion options to Real World
-- Conversion's own 5th level (none of the 5 states a level anywhere in its
-- own prose, only on the introducing feature) -- the same catalog-option
-- shape as Scout-Nin's Shinobi Adept/Signature Jutsu fix in migration 0051.
--
-- This migration repairs the already-shipped rows the same way.
UPDATE class_features
SET level = 4
WHERE slug = 'class/genjutsu-specialist/feature/ability-score-improvement-feat';

UPDATE class_features
SET level = 5
WHERE class_slug = 'class/genjutsu-specialist'
  AND slug IN (
    'class/genjutsu-specialist/feature/actualized-alteration',
    'class/genjutsu-specialist/feature/actualized-duplicity',
    'class/genjutsu-specialist/feature/actualized-perception',
    'class/genjutsu-specialist/feature/actualized-perfection',
    'class/genjutsu-specialist/feature/actualized-power'
  );
