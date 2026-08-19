-- Taijutsu Specialist's Ironclad Technique (20th level) has a PDF-extraction
-- artifact glued into the middle of an otherwise ordinary sentence:
-- "...Once you use this feature, you must spend the same amount of IRON
-- CLAD SHIELD Armor Name AC Bulk Properties Iron Clad Shield +1 1 Bulk
-- Blocking, Light martial die at the beginning of one of your turns to
-- recharge this feature." The intruding text is the Iron Clad Shield stat
-- block table that belongs to the 3rd-level Ironclad feature — that row's
-- own description is separately confirmed truncated, ending at "...This
-- shield has the following statistics;" with the table itself lost during
-- extraction and resurfacing here instead. Stripping it restores the
-- grammatical "...you must spend the same amount of martial die at the
-- beginning of one of your turns to recharge this feature."
-- internal/parse/subclasses.go's knownExtractionSquishes now corrects this
-- on any FUTURE re-ingest; this migration repairs an already-shipped
-- install's row in the meantime, the same one-time patch shape as
-- 0041/0044.
UPDATE subclass_features
SET description = REPLACE(description,
    'IRON CLAD SHIELD Armor Name AC Bulk Properties Iron Clad Shield +1 1 Bulk Blocking, Light martial die',
    'martial die')
WHERE slug = 'class/taijutsu-specialist/group/taijutsu-style/ironclad/feature/ironclad-technique-changed';

-- Also restores the recovered stat block to the 3rd-level Ironclad feature's
-- own truncated description, which otherwise still ends abruptly at "...This
-- shield has the following statistics;" with nothing after it.
UPDATE subclass_features
SET description = description || ' Iron Clad Shield: +1 AC, 1 Bulk, Blocking, Light.'
WHERE slug = 'class/taijutsu-specialist/group/taijutsu-style/ironclad/feature/ironclad'
  AND description LIKE '%This shield has the following statistics;';
