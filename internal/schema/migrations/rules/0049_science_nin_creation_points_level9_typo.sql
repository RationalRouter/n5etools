-- Science-Nin's Creation Points chart (class_level_resources) reads "18 ."
-- at 9th level instead of a clean "18" -- a stray Mastersheet cell artifact
-- confirmed against every neighboring level in the same column, each a flat
-- +2 step (16 at 8th, 20 at 10th). strconv.Atoi silently fails on the
-- trailing " .", and cmd/n5e's classLevelResourceInt treats a parse failure
-- as "no value" (returns 0, not an error) rather than surfacing it -- so a
-- 9th-level Science-Nin character's own Creation Points budget (the
-- Scientific Ninja Tools catalog's spend cap) would silently read 0 instead
-- of 18 without this fix. internal/sheet/classcharts.go's
-- knownResourceCellFixes now corrects this on any future re-ingest of the
-- Mastersheet; this migration repairs the already-shipped row.
UPDATE class_level_resources
SET value = '18'
WHERE class_slug = 'class/science-nin'
  AND level = 9
  AND resource_name = 'Creation Points'
  AND value = '18 .';
