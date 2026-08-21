-- Cooking-Nin's 1st-level Cooking Tool Infusion feature (class_features
-- slug 'class/cooking-nin/feature/cooking-tool-infusion') grants "a Cooking
-- Tool: a frying pan, or other item of your description that fits your
-- personal way of cooking" — the book names one example and explicitly
-- leaves the rest open-ended, printing no closed catalogue of allowed
-- implement types anywhere. No equipment row for any cooking implement
-- exists in this schema before this migration (toolkit/cooking-kit and its
-- 3 upgrade tiers are the separate, non-weapon Cooking Tool Proficiency
-- item — see that row's own description).
--
-- These rows exist so the sheet's own implement picker
-- (cmd/n5e/cooking_nin.go's loadCookingToolImplementCatalog) has a real
-- catalog to offer instead of free text, following this table's own
-- existing (kind-prefix)/(specific-description) slug convention (e.g.
-- scroll/a-rank-jutsu-scroll, weapon/kunai) rather than inventing a new
-- shape. damage_dice is left at the flat 1st-4th-level Cooking Die value
-- (1d4) as an inert baseline only — cmd/n5e/characters.go's buildAttacks
-- always overrides an equipped cooking-tool weapon's effective damage dice
-- from the character's own live Cooking Die chart
-- (v_class_level_resources, resource_name 'Cooking Die'), never reads this
-- column for that weapon. damage_type and properties are both left NULL
-- deliberately: the feature's own text has the player choose a damage type
-- (Bludgeoning/Piercing/Slashing) and weapon properties independently of
-- which implement was chosen, at 1st/6th/11th level — those picks are
-- tracked per-character (character_feature_choices, keyed off the granting
-- feature's own slug) and applied at attack-build time, not baked into the
-- catalog row.
INSERT OR IGNORE INTO source_books (slug, title, version, file_name, file_sha256)
VALUES ('book/class-compendium', 'Class Compendium', '3.12', 'class-compendium.pdf', 'unknown');

INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, damage_dice, description, source_book, source_version, detection_status) VALUES
('weapon/cooking-tool-frying-pan', 'Frying Pan', 'weapon', 5.0, 1.0, '1d4',
 'A one-handed skillet, well-balanced for a quick swing — the Cooking Tool Infusion feature''s own named example.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-chefs-knife', 'Chef''s Knife', 'weapon', 8.0, 1.0, '1d4',
 'A one-handed all-purpose kitchen knife, kept honed for both prep work and combat.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-paring-knife', 'Paring Knife', 'weapon', 3.0, 1.0, '1d4',
 'A small, one-handed blade meant for delicate cuts, easy to conceal in a sleeve or apron pocket.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-cleaver', 'Cleaver', 'weapon', 10.0, 1.0, '1d4',
 'A heavy, one-handed rectangular blade built for chopping straight through bone and joint.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-bread-knife', 'Bread Knife', 'weapon', 6.0, 1.0, '1d4',
 'A one-handed serrated blade, its teeth tearing as much as they cut.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-filleting-knife', 'Filleting Knife', 'weapon', 7.0, 1.0, '1d4',
 'A thin, flexible one-handed blade meant to slip along bone with minimal resistance.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-boning-knife', 'Boning Knife', 'weapon', 7.0, 1.0, '1d4',
 'A stiff, narrow one-handed blade for working meat free of bone and cartilage.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-ladle', 'Ladle', 'weapon', 4.0, 1.0, '1d4',
 'A one-handed long-handled serving spoon, deep-bowled enough to double as a bludgeon.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-whisk', 'Whisk', 'weapon', 3.0, 1.0, '1d4',
 'A one-handed bundle of looped wire, more sting than heft but infused all the same.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-wooden-spoon', 'Wooden Spoon', 'weapon', 1.0, 1.0, '1d4',
 'A humble one-handed stirring spoon, the kind every household kitchen already keeps within reach.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-meat-tenderizer', 'Meat Tenderizer', 'weapon', 6.0, 1.0, '1d4',
 'A one-handed studded mallet, meant for pounding cuts of meat flat.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-tongs', 'Tongs', 'weapon', 4.0, 1.0, '1d4',
 'A one-handed pair of long-armed pincers, as good for gripping a foe as a hot coal.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-spatula', 'Spatula', 'weapon', 3.0, 1.0, '1d4',
 'A one-handed flat-bladed turner, thin at the edge and surprisingly sharp when infused.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-skewer', 'Skewer', 'weapon', 2.0, 1.0, '1d4',
 'A one-handed metal spit, straight and sharpened to a fine point.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-ice-pick', 'Ice Pick', 'weapon', 5.0, 1.0, '1d4',
 'A one-handed spike-tipped tool for breaking down block ice, its point kept needle-fine.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-carving-fork', 'Carving Fork', 'weapon', 5.0, 1.0, '1d4',
 'A one-handed two-tined fork used to steady a roast while it''s carved.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-kitchen-shears', 'Kitchen Shears', 'weapon', 6.0, 1.0, '1d4',
 'A one-handed heavy-duty pair of shears, strong enough to cut through poultry joints.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-rolling-pin', 'Rolling Pin', 'weapon', 4.0, 2.0, '1d4',
 'A two-handed cylinder of hardwood, swung with both arms to flatten dough — or an opponent''s guard.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-wok', 'Wok', 'weapon', 9.0, 2.0, '1d4',
 'A two-handed wide-bottomed cooking pan, heavy enough to need both hands to swing properly.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-stockpot-lid', 'Stockpot Lid', 'weapon', 6.0, 2.0, '1d4',
 'A one-handed metal lid, broad enough to double as an improvised buckler.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-mortar-and-pestle', 'Mortar and Pestle', 'weapon', 4.0, 1.0, '1d4',
 'A one-handed stone grinding pestle, kept alongside its bowl for crushing spice and bone alike.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-garlic-press', 'Garlic Press', 'weapon', 2.0, 1.0, '1d4',
 'A small one-handed hinged press, its crushing jaw infused into something far more dangerous.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-pizza-cutter', 'Pizza Cutter', 'weapon', 3.0, 1.0, '1d4',
 'A one-handed rolling wheel blade, its edge kept honed for a clean slice.',
 'book/class-compendium', '3.12', 'manual'),
('weapon/cooking-tool-basting-brush', 'Basting Brush', 'weapon', 1.0, 1.0, '1d4',
 'A one-handed long-handled brush, its bristle end reinforced to take a real infused strike.',
 'book/class-compendium', '3.12', 'manual');
