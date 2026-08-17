-- Backfills equipment.bulk for every weapon row that has a counterpart in
-- the core book's printed weapon tables.
--
-- Bulk is this ruleset's replacement for 5e's item weight: your inventory
-- capacity is 10 + 2 per point of Strength modifier (core book, "Your
-- Inventory"), and carrying more Bulk than that makes you Encumbered. Armor,
-- gear, tools, toolkits and scrolls all arrived with bulk populated — from
-- the Mastersheet's WeapArmor tab for armor, from the text-layer tables for
-- the rest — but every one of the 74 weapon rows had bulk NULL, so the
-- weapons a character actually carries contributed nothing to the total.
-- Migration 0015's header claims "bulk is already populated for every one of
-- these rows from the Mastersheet"; that is true of the armor half of
-- WeapArmor and false of the weapon half, which never carried a Bulk column
-- at all.
--
-- Source: Naruto 5e - Full Document.pdf, the six weapon tables on
--   PDF page 31 (printed 29): Simple Melee, Simple Ranged
--   PDF page 32 (printed 30): Martial Melee, Martial Ranged
--   PDF page 33 (printed 31): Exotic Melee, Exotic Ranged
-- The Simple and Martial tables are pure images with no text layer (the same
-- reason their cost column needed migration 0015), so their values are a
-- direct visual read of the rendered pages. The Exotic tables DO extract as
-- text, and every Exotic value below was cross-checked against that text
-- extraction as well as the rendered page — the two agree exactly.
--
-- Like 0015, this is safe against a future `n5e-ingest sheet` re-run:
-- internal/store/equipment.go's upsertWeapon sets only name/damage_dice/
-- damage_type/properties/weapon_category, never bulk, so nothing here can be
-- clobbered by reloading the Mastersheet.
--
-- The four "(Two Hands)" rows (Quarterstaff, Spear, Taichi, Tetsubo) are
-- Mastersheet-only versatile-grip duplicates of a single printed weapon, not
-- separate objects — they take the same bulk as the weapon they duplicate,
-- since a character carrying one is carrying one object either way.
--
-- Two weapon rows are deliberately left NULL rather than guessed at:
-- weapon/net has no row in any of the six printed tables (a Mastersheet-only
-- row, same category as the 14 armor rows migration 0020 removed), and
-- weapon/unarmed-strike is set to 0 below because it is not an object at
-- all and cannot occupy inventory space.

-- Simple Melee Weapons (PDF page 31)
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/kunai';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/hand-axe';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/sai';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/tanto-shortsword';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/kama-hand-scythe';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/gunsen';
UPDATE equipment SET bulk = 2 WHERE slug IN ('weapon/quarterstaff', 'weapon/quarterstaff-two-hands');
UPDATE equipment SET bulk = 2 WHERE slug IN ('weapon/spear', 'weapon/spear-two-hands');
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/weighted-chain';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/kusarigama-chained-hand-scythe';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/tekko';

-- Simple Ranged Weapons (PDF page 31)
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/senbon';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/short-bow';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/shuriken';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/sling';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/light-crossbow';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/bola';

-- Martial Melee Weapons (PDF page 32). The book prints "Tachi"; the DB row
-- for it is weapon/taichi, named that way by the Mastersheet load.
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/broadsword';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/iron-claw';
UPDATE equipment SET bulk = 2 WHERE slug IN ('weapon/taichi', 'weapon/taichi-two-hands');
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/katana';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/odachi-great-sword';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/knuckle-blades';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/hidden-blade';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/chained-spear';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/chigiriki';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/whip';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/battle-wire';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/naginata';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/sasumata-forked-spear';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/great-axe';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/scythe';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/tinbe-rochin';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/yari';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/hooked-lance';
UPDATE equipment SET bulk = 3 WHERE slug IN ('weapon/tetsubo', 'weapon/tetsubo-two-hands');
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/tonfa';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/war-club';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/nunchaku';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/combat-bracers';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/jitte';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/gunbai-fan';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/kanabo';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/otsuchi-hammer';

-- Martial Ranged Weapons (PDF page 32)
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/chakram';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/monster-chakram';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/fuma-shuriken';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/monster-shuriken';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/torinawa';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/boomerang';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/monster-boomerang';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/longbow';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/crossbow-hand';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/crossbow-heavy';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/blowgun';

-- Exotic Melee Weapons (PDF page 33)
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/sansetsukon';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/triple-bladed-scythe';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/cleaver-sword';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/triple-katar';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/urumi';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/chokuto';

-- Exotic Ranged Weapons (PDF page 33)
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/matchlock-pistol';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/matchlock-rifle';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/hiya-taihou';
UPDATE equipment SET bulk = 1 WHERE slug = 'weapon/combat-scroll';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/giant-combat-scroll';
UPDATE equipment SET bulk = 2 WHERE slug = 'weapon/ballistic-kunai-gun';
UPDATE equipment SET bulk = 3 WHERE slug = 'weapon/ballistic-shuriken-rifle';

-- Not an object; occupies no inventory space.
UPDATE equipment SET bulk = 0 WHERE slug = 'weapon/unarmed-strike';
