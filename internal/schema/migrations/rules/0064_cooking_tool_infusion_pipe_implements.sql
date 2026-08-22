-- Herbalist's 3rd-level "Bonus Tool Infusion: Pipe" feature (class_features
-- slug 'class/cooking-nin/group/cooking-focus/herbalist/feature/bonus-
-- tool-infusion-pipe') grants "a second Cooking Tool Infusion weapon,
-- following the same structure as your original one" — a cooking tool that
-- "is some kind of smoking implement." Same as the base Cooking Tool
-- Infusion feature (0059_cooking_tool_infusion_implements.sql), the book
-- names the general shape ("some kind of smoking implement") but no closed
-- catalogue of allowed implements, so these rows exist purely to give the
-- sheet's own picker (cmd/n5e/cooking_nin.go's
-- loadCookingToolPipeImplementCatalog) a real catalog to offer instead of
-- free text.
--
-- Slug prefix is 'weapon/cooking-pipe-', deliberately NOT
-- 'weapon/cooking-tool-pipe-' or any other prefix beginning with
-- 'weapon/cooking-tool-' — the base feature's own catalog query and its
-- inventory-sync "remove the stale implement" query both match by
-- LIKE 'weapon/cooking-tool-%', and a Pipe slug nested under that prefix
-- would wrongly appear in the base tool's own picker and get deleted the
-- moment the base implement is re-synced. The two catalogs must stay
-- disjoint under a simple LIKE-prefix test, not just distinctly named.
--
-- damage_dice is left at the flat 1st-4th-level Cooking Die value (1d4) as
-- an inert baseline only, same as every 0059 row — buildAttacks always
-- overrides an equipped Pipe's effective damage dice from the character's
-- own live Cooking Die chart (v_class_level_resources, resource_name
-- 'Cooking Die'), never reads this column for that weapon. damage_type and
-- properties are both left NULL deliberately: the feature's own text has
-- the player choose a damage type (Bludgeoning/Piercing/Slashing) and a
-- weapon property independently of which implement was chosen, at
-- 3rd/6th/11th level — those picks are tracked per-character
-- (character_feature_choices, keyed off the granting feature's own slug)
-- and applied at attack-build time, not baked into the catalog row.
INSERT OR IGNORE INTO source_books (slug, title, version, file_name, file_sha256)
VALUES ('book/class-compendium', 'Class Compendium', '3.12', 'class-compendium.pdf', 'unknown');

INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, damage_dice, description, source_book, source_version, detection_status) VALUES
('weapon/cooking-pipe-tobacco', 'Tobacco Pipe', 'weapon', 4.0, 1.0, '1d4',
 'A one-handed straight-stemmed pipe, its bowl packed with an ordinary shredded leaf blend.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-corn-cob', 'Corn Cob Pipe', 'weapon', 2.0, 1.0, '1d4',
 'A one-handed pipe carved from a dried corn cob, cheap and quick to replace.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-kiseru', 'Kiseru', 'weapon', 6.0, 1.0, '1d4',
 'A one-handed slender metal-and-bamboo pipe with a small bowl, meant for short, sharp draws.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-meerschaum', 'Meerschaum Pipe', 'weapon', 9.0, 1.0, '1d4',
 'A one-handed pipe carved from pale mineral stone, its bowl slowly darkening with use.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-churchwarden', 'Churchwarden Pipe', 'weapon', 7.0, 2.0, '1d4',
 'A two-handed pipe with an unusually long stem, held out at arm''s length while drawn.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-hookah-stem', 'Hookah Stem', 'weapon', 8.0, 2.0, '1d4',
 'A two-handed detached hose-and-mouthpiece length, snapped free of its base and swung like a flail.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-cigar', 'Cigar', 'weapon', 3.0, 1.0, '1d4',
 'A one-handed tightly rolled leaf, thick enough to hold a real infused edge.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-cigarette-holder', 'Cigarette Holder', 'weapon', 3.0, 1.0, '1d4',
 'A one-handed slender holder, light in the hand but weighted enough to jab and strike.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-calabash', 'Calabash Pipe', 'weapon', 5.0, 1.0, '1d4',
 'A one-handed gourd-bowled pipe, curved and hollow, surprisingly sturdy when swung.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-pipe-briar', 'Briar Pipe', 'weapon', 6.0, 1.0, '1d4',
 'A one-handed pipe cut from dense briar root, favored for how well it takes a hard knock.',
 'book/class-compendium', '3.12', 'manual');
