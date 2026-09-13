-- "succeed a saving throw" -> "succeed on a saving throw" for the rows
-- internal/correct's Sweep cannot reach.
--
-- "Succeed" is intransitive in this sense: you succeed *on* a save, not *a*
-- save. A full-corpus audit (2026-09-13) found 271 occurrences of the
-- prepositionless form against 52 correct ones — a systemic authoring habit in
-- the sourcebook, not unrelated slips — and the fix now lives permanently in
-- internal/correct/rules.go's succeedon rule, which runs at ingest time on
-- every parsed prose column.
--
-- These 12 rows are the exception: poisons and the Genjutsu Pill are
-- hand-transcribed here in migrations rather than parsed out of a PDF (see
-- 0017_adventuring_gear.sql and 0034's own doc comment for the same case), so
-- no ingest pass ever touches them. Fixing them in SQL is the only place the
-- correction survives a rebuild.
UPDATE poisons
SET description = REPLACE(description, 'succeed a DC', 'succeed on a DC')
WHERE description LIKE '%succeed a DC%';

UPDATE equipment
SET description = REPLACE(description, 'succeed a DC', 'succeed on a DC')
WHERE slug = 'gear/genjutsu-pill';
