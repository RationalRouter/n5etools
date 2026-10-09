-- Two jutsu slugs changed when their misspelled book names were corrected
-- (rules migration 0081 folds the old rows into the new ones):
--   jutsu/o-encode-thoughts   -> jutsu/encode-thoughts
--   jutsu/one-man-front-lotud -> jutsu/one-man-front-lotus
-- Repoint every character reference so a learned copy keeps resolving.
-- character_jutsu_options is keyed by (character_id, jutsu_slug); when a
-- character already holds options under the new slug, those win and the
-- old row is dropped.
UPDATE character_jutsu SET jutsu_slug = 'jutsu/encode-thoughts' WHERE jutsu_slug = 'jutsu/o-encode-thoughts';
UPDATE character_jutsu SET jutsu_slug = 'jutsu/one-man-front-lotus' WHERE jutsu_slug = 'jutsu/one-man-front-lotud';

UPDATE OR IGNORE character_jutsu_options SET jutsu_slug = 'jutsu/encode-thoughts' WHERE jutsu_slug = 'jutsu/o-encode-thoughts';
UPDATE OR IGNORE character_jutsu_options SET jutsu_slug = 'jutsu/one-man-front-lotus' WHERE jutsu_slug = 'jutsu/one-man-front-lotud';
DELETE FROM character_jutsu_options WHERE jutsu_slug IN ('jutsu/o-encode-thoughts', 'jutsu/one-man-front-lotud');

UPDATE character_concentration SET jutsu_slug = 'jutsu/encode-thoughts' WHERE jutsu_slug = 'jutsu/o-encode-thoughts';
UPDATE character_concentration SET jutsu_slug = 'jutsu/one-man-front-lotus' WHERE jutsu_slug = 'jutsu/one-man-front-lotud';
