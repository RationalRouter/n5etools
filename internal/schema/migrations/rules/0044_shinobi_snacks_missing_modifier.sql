-- Cooking-Nin's Shinobi Snacks (base class feature, 1st level) prints "you
-- can create a number of them equal to your proficiency bonus plus your
-- Intelligence." — the word "modifier" is dropped after "Intelligence",
-- confirmed a PDF-extraction dropped word rather than a deliberate
-- use-full-score rule: this same feature's own War and Food (7th level)
-- explicitly reads "equal to your intelligence modifier" for the identical
-- recreate-Snacks effect. internal/parse/subclasses.go's
-- knownExtractionSquishes now corrects this on any FUTURE re-ingest (wired
-- into classes.go's flushFeature); this migration repairs an already-shipped
-- install's row in the meantime, the same one-time patch shape as 0041/0043.
UPDATE class_features
SET description = REPLACE(description,
    'equal to your proficiency bonus plus your Intelligence.',
    'equal to your proficiency bonus plus your Intelligence modifier.')
WHERE slug = 'class/cooking-nin/feature/shinobi-snacks';
