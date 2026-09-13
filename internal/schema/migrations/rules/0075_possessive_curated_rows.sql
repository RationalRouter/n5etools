-- Dropped possessive apostrophe in a hand-transcribed row, for the same
-- reason 0074 exists: the seal catalog is written out in migrations rather
-- than parsed out of a PDF, so internal/correct's Sweep never sees it and the
-- possessive rules added 2026-09-13 cannot reach it.
--
-- The parsed corpus carries 228 further instances of this sweep's findings,
-- all corrected at ingest time by internal/correct/rules.go's possessiveDetRe,
-- possessiveNounRe, abbrevSplitRe and pluralVerbRe.
UPDATE equipment
SET description = REPLACE(description, 'vs the targets Wisdom', 'vs the target’s Wisdom')
WHERE slug = 'seal/weapon/refined/fearsome-seal';
