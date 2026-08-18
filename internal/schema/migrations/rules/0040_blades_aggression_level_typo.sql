-- Hunter-Nin's Blade Warden Blade's Aggression prints "Beginning at 1oth
-- level, ..." — a lowercase letter "o" swapped in for the digit "0" by the
-- sourcebook's PDF text extraction. That typo breaks ordinalLevelRe's
-- \d{1,2} match entirely, so this feature parsed with a NULL level instead
-- of the intended 10th-level gate. subclasses.go's knownExtractionSquishes
-- now corrects "1oth level" to "10th level" before the level regex runs, so
-- any FUTURE re-ingest will set level = 10 (and print the corrected text)
-- on its own. This migration repairs an already-shipped install's row in
-- the meantime, the same one-time patch shape as 0038.
UPDATE subclass_features
SET description = REPLACE(description, '1oth level', '10th level'),
    level = 10
WHERE slug = 'class/hunter-nin/group/hunters-creeds/blade-warden/feature/blades-aggression';
