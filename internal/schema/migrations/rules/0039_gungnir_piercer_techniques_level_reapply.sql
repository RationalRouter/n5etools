-- Migration 0038 set this feature's level to 3 directly, but a one-time
-- content migration is marked "applied" forever and never re-runs, while
-- internal/autoupdate re-ingests the sourcebook (and overwrites this row's
-- parsed columns, including level) whenever its Drive version changes. The
-- fix silently reverted back to a NULL level. The parser itself has now
-- been taught this feature's level (subclasses.go's
-- knownFeatureLevelOverrides), so any FUTURE re-ingest will set level = 3
-- correctly on its own — this migration only repairs an already-shipped
-- install's row in the meantime, the same one-time patch shape as 0038.
UPDATE subclass_features
SET level = 3
WHERE slug = 'class/weapon-specialist/group/weapon-forms/gungnir-piercer-form/feature/gungnir-piercer-techniques-changed';
