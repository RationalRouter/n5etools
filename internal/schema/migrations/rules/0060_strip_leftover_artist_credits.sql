-- A corpus-wide audit turned up 16 rows where the flat PDF text extractor
-- glued an image/artist credit line onto adjacent rules prose, in formats
-- 0037's fix didn't cover: bare "Credit: X on Y" (no "Artist " prefix),
-- "ART CREDIT This picture comes from X on Y" trailing a paragraph instead
-- of sitting on its own heading line, and "Wiki The second picture comes
-- from X on Y". Two of these (shinobi-snacks, bonus-tool-infusion) have the
-- credit line sitting mid-paragraph with real rules text following it, so
-- each fix removes only the exact credit-line span rather than truncating
-- the field.
UPDATE class_features
SET description = REPLACE(description, 'Credit: Antilous Chao on Artstation ', '')
WHERE slug = 'class/cooking-nin/feature/shinobi-snacks';

UPDATE class_features
SET description = REPLACE(description, ' 190 Credit: Jutsinwongart on DeviantArt', '.')
WHERE slug = 'class/cooking-nin/feature/peerless-taste';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Persona 5 Royal', '')
WHERE slug = 'class/scout-nin/group/scouting-technique/trickster-scout/feature/superior-trickster';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Kaeomon#0879 on Discord', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/battle-cook/feature/combat-snacks';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Kidcurious on DeviantArt', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/battle-cook/feature/master-of-dining-and-dicing';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: alchemaniac on DeviantArt', '.')
WHERE slug = 'class/cooking-nin/group/cooking-focus/patissier-chef/feature/gotta-do-the-cooking-by-the-book';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: brunourata.arte on Instagram', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/herbalist/feature/herbal-snacks';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: @-109h on Twitter', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/herbalist/feature/unmatched-botanist';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: @theaaronschmidt on Twitter', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/fry-cooks/feature/fried-snacks';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Anbe Yoshirou, School Girl Strikers', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/fry-cooks/feature/always-ready';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: TofuBlock/Jauni on Twitter', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/gastrochemist/feature/bonus-tool-infusion';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Unknown, World Flipper', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/gastrochemist/feature/in-touch';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Drowtales on DeviantArt', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/show-cook/feature/crowd-pleaser';

UPDATE subclass_features
SET description = REPLACE(description, ' Credit: Artemii Myasnikov on Instagram', '')
WHERE slug = 'class/cooking-nin/group/cooking-focus/sour-taste/feature/poisoned-snacks';

UPDATE jutsu
SET description = REPLACE(description, ' Wiki The second picture comes from Strawberry-senpai on Tumblr', '')
WHERE slug = 'jutsu/hanami/hanami-style-falling-blossom-reprise';

UPDATE feats
SET description = REPLACE(description, '  ART CREDIT This picture comes from KiriSharingan on the Naruto Fanon Wiki', '')
WHERE slug = 'feat/konjiki/superior-steel';
