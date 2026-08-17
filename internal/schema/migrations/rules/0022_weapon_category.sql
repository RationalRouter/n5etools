-- Backfills equipment.weapon_category for every weapon row.
--
-- 0001_init.sql declared the column ("'simple','martial', ...") but nothing
-- ever wrote it: all 74 weapon rows had it NULL, which is why
-- buildAttacks in cmd/n5e/characters.go reads the properties text instead
-- and says so in its doc comment. That was tolerable while nothing needed
-- the category, and it stopped being tolerable once character creation
-- needed to offer a dropdown of simple or martial weapons when a class
-- grants that choice: a dropdown of simple weapons needs to know which
-- weapons are simple.
--
-- Source: Naruto 5e - Full Document.pdf, the same six printed weapon tables
-- migration 0021 read bulk out of, and the grouping is the tables themselves
-- — which weapon sits under which heading:
--   PDF page 31 (printed 29): Simple Melee, Simple Ranged
--   PDF page 32 (printed 30): Martial Melee, Martial Ranged
--   PDF page 33 (printed 31): Exotic Melee, Exotic Ranged
-- Every slug below appears in 0021 under the identical heading; the two
-- migrations partition the same 74 rows the same way, so they can be checked
-- against each other line by line.
--
-- Values are lowercase 'simple' / 'martial' / 'exotic', matching the comment
-- 0001_init.sql wrote next to the column.
--
-- Safe against a future `n5e-ingest sheet` re-run in the same sense 0021 is:
-- internal/store/equipment.go's upsertWeapon does write weapon_category, but
-- only from a Mastersheet column that is empty for every row — which is how
-- these came to be NULL in the first place. Re-running it would restore NULL
-- rather than a competing value, and re-applying this migration restores
-- these.
--
-- weapon/net keeps NULL: it has no row in any of the six printed tables (a
-- Mastersheet-only row, the same reason 0021 left its bulk NULL), so there
-- is no heading to read a category off. weapon/unarmed-strike also keeps
-- NULL — it is not a weapon anyone picks from a list.

-- Simple Melee Weapons (PDF page 31)
UPDATE equipment SET weapon_category = 'simple' WHERE slug IN (
  'weapon/kunai',
  'weapon/hand-axe',
  'weapon/sai',
  'weapon/tanto-shortsword',
  'weapon/kama-hand-scythe',
  'weapon/gunsen',
  'weapon/quarterstaff', 'weapon/quarterstaff-two-hands',
  'weapon/spear', 'weapon/spear-two-hands',
  'weapon/weighted-chain',
  'weapon/kusarigama-chained-hand-scythe',
  'weapon/tekko'
);

-- Simple Ranged Weapons (PDF page 31)
UPDATE equipment SET weapon_category = 'simple' WHERE slug IN (
  'weapon/senbon',
  'weapon/short-bow',
  'weapon/shuriken',
  'weapon/sling',
  'weapon/light-crossbow',
  'weapon/bola'
);

-- Martial Melee Weapons (PDF page 32)
UPDATE equipment SET weapon_category = 'martial' WHERE slug IN (
  'weapon/broadsword',
  'weapon/iron-claw',
  'weapon/taichi', 'weapon/taichi-two-hands',
  'weapon/katana',
  'weapon/odachi-great-sword',
  'weapon/knuckle-blades',
  'weapon/hidden-blade',
  'weapon/chained-spear',
  'weapon/chigiriki',
  'weapon/whip',
  'weapon/battle-wire',
  'weapon/naginata',
  'weapon/sasumata-forked-spear',
  'weapon/great-axe',
  'weapon/scythe',
  'weapon/tinbe-rochin',
  'weapon/yari',
  'weapon/hooked-lance',
  'weapon/tetsubo', 'weapon/tetsubo-two-hands',
  'weapon/tonfa',
  'weapon/war-club',
  'weapon/nunchaku',
  'weapon/combat-bracers',
  'weapon/jitte',
  'weapon/gunbai-fan',
  'weapon/kanabo',
  'weapon/otsuchi-hammer'
);

-- Martial Ranged Weapons (PDF page 32)
UPDATE equipment SET weapon_category = 'martial' WHERE slug IN (
  'weapon/chakram',
  'weapon/monster-chakram',
  'weapon/fuma-shuriken',
  'weapon/monster-shuriken',
  'weapon/torinawa',
  'weapon/boomerang',
  'weapon/monster-boomerang',
  'weapon/longbow',
  'weapon/crossbow-hand',
  'weapon/crossbow-heavy',
  'weapon/blowgun'
);

-- Exotic Melee Weapons (PDF page 33)
UPDATE equipment SET weapon_category = 'exotic' WHERE slug IN (
  'weapon/sansetsukon',
  'weapon/triple-bladed-scythe',
  'weapon/cleaver-sword',
  'weapon/triple-katar',
  'weapon/urumi',
  'weapon/chokuto'
);

-- Exotic Ranged Weapons (PDF page 33)
UPDATE equipment SET weapon_category = 'exotic' WHERE slug IN (
  'weapon/matchlock-pistol',
  'weapon/matchlock-rifle',
  'weapon/hiya-taihou',
  'weapon/combat-scroll',
  'weapon/giant-combat-scroll',
  'weapon/ballistic-kunai-gun',
  'weapon/ballistic-shuriken-rifle'
);
