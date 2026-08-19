-- Rejuvenating Rest's own description reads "Also, at Level 1 you use your
-- medical skills to revitalize wounded allies during a short rest... This
-- amount of extra healing increases to 2d6 at 7th level, 3d6 at 11th, and
-- 4d6 at 17th Level" — the base 1d6 bonus applies from 1st level; the level
-- regex instead matched the mid-sentence "7th level" describing the first
-- escalation tier, parsing this as a 7th-level feature. internal/parse/
-- classes.go's knownClassFeatureLevelOverrides now corrects this for any
-- FUTURE re-ingest. This migration repairs an already-shipped install's
-- row in the meantime, the same one-time patch shape as 0043.
UPDATE class_features
SET level = 1
WHERE slug = 'class/medical-nin/feature/rejuvenating-rest';
