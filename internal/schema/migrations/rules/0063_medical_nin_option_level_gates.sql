-- Medical Doctrine's 4 named options (Long Life Short Death, Never on the
-- Front Lines, Not Allowed to Die, Until Their Heart Stops) and Preserve/
-- Take Life's 2 named options (Preserve Life, Take Life) all exist as their
-- own class_features rows with no level of their own — the level only
-- appears in the PARENT feature's opening sentence ("Starting at 3rd Level
-- ..." for Medical Doctrine, "Starting at 5th Level ..." for Preserve/Take
-- Life). The Class Reference popup (cmd/n5e/reference.go) locks/unlocks
-- each row independently off its own level column, so all 6 options read as
-- "Always on" and undimmed even for a freshly-created level 1 Medical-Nin,
-- while their own parent rows correctly show as locked.
UPDATE class_features
SET level_override = 3
WHERE slug IN (
  'class/medical-nin/feature/long-life-short-death',
  'class/medical-nin/feature/never-on-the-front-lines',
  'class/medical-nin/feature/not-allowed-to-die',
  'class/medical-nin/feature/until-their-heart-stops'
) AND level IS NULL AND level_override IS NULL;

UPDATE class_features
SET level_override = 5
WHERE slug IN (
  'class/medical-nin/feature/preserve-life',
  'class/medical-nin/feature/take-life'
) AND level IS NULL AND level_override IS NULL;
