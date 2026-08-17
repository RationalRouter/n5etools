-- Backfills equipment.cost_ryo for the 18 armor + ~65 weapon rows that
-- correspond to a real price-table row in the core book (Naruto 5e - Full
-- Document.pdf, Chapter 5) — the one piece the Mastersheet's WeapArmor tab
-- was never able to carry, since it's a pure image in the PDF with no text
-- layer (see internal/sheet/weaparmor.go's package doc). Digitized via the
-- new internal/ocr package (n5e-ingest ocr words/table) against four
-- rasterized pages, then cross-checked value-by-value against an
-- independent direct visual read of the same four pages — every value
-- below is confirmed by both, not OCR output taken on faith:
--   Armor table:                    PDF page 28 (printed 26)
--   Simple Melee + Ranged Weapons:  PDF page 31 (printed 29)
--   Martial Melee + Ranged Weapons: PDF page 32 (printed 30)
--   Exotic Melee + Ranged Weapons:  PDF page 33 (printed 31)
--
-- Deliberately NOT touching weight_lb: this ruleset replaced traditional
-- item weight with a "Bulk" system (see the core book's "Slots & Bulk"
-- section), and bulk is already populated for every one of these rows
-- from the Mastersheet — weight_lb isn't a mechanic this ruleset uses at
-- all, not a gap to fill.
--
-- Both upsertArmor and upsertWeapon in internal/store/equipment.go only
-- ever SET the columns their own Mastersheet load owns (name/category/
-- ac_bonus/bulk/... for armor, name/damage_dice/damage_type/properties for
-- weapons) — cost_ryo is not among them, confirmed by reading both
-- functions' UPDATE statements directly — so a future `n5e-ingest sheet`
-- re-run can never clobber the values this migration sets. No override-
-- column indirection needed here, unlike class_levels/class_level_resources
-- in migration 0014, whose loader does overwrite every column on reload.
--
-- Three weapons printed in these tables have no corresponding DB row at
-- all — a genuine Mastersheet ingestion gap, not something this migration
-- can fix without inventing a new slug: "Light Crossbow" (Simple Ranged,
-- 20 Ryo, 1d8 Piercing — distinct from the Martial "Crossbow, Hand"/
-- "Crossbow, Heavy" rows the DB does have), "Torinawa" (Martial Ranged,
-- 5 Ryo, 1d4 Bludgeoning, Thrown/Grapple), and "Triple-Bladed Scythe"
-- (Exotic Melee, 100 Ryo, 1d12 Slashing). Flagged for a human decision,
-- not silently added.

UPDATE equipment SET cost_ryo = 100,  detection_status = 'verified' WHERE slug = 'armor/padded-cloth';
UPDATE equipment SET cost_ryo = 250,  detection_status = 'verified' WHERE slug = 'armor/leather-weave';
UPDATE equipment SET cost_ryo = 500,  detection_status = 'verified' WHERE slug = 'armor/armored-cloth';
UPDATE equipment SET cost_ryo = 750,  detection_status = 'verified' WHERE slug = 'armor/reinforced-cloth';
UPDATE equipment SET cost_ryo = 1500, detection_status = 'verified' WHERE slug = 'armor/synthetic-weave';
UPDATE equipment SET cost_ryo = 2500, detection_status = 'verified' WHERE slug = 'armor/shinobi-weave';
UPDATE equipment SET cost_ryo = 100,  detection_status = 'verified' WHERE slug = 'armor/combat-jacket';
UPDATE equipment SET cost_ryo = 250,  detection_status = 'verified' WHERE slug = 'armor/shinobi-jacket';
UPDATE equipment SET cost_ryo = 500,  detection_status = 'verified' WHERE slug = 'armor/shinobi-combat-jacket';
UPDATE equipment SET cost_ryo = 750,  detection_status = 'verified' WHERE slug = 'armor/chunin-jacket';
UPDATE equipment SET cost_ryo = 1500, detection_status = 'verified' WHERE slug = 'armor/battle-coat';
UPDATE equipment SET cost_ryo = 2500, detection_status = 'verified' WHERE slug = 'armor/armored-chunin-coat';
UPDATE equipment SET cost_ryo = 100,  detection_status = 'verified' WHERE slug = 'armor/combat-armor';
UPDATE equipment SET cost_ryo = 250,  detection_status = 'verified' WHERE slug = 'armor/synthetic-armor';
UPDATE equipment SET cost_ryo = 500,  detection_status = 'verified' WHERE slug = 'armor/jonin-armor';
UPDATE equipment SET cost_ryo = 750,  detection_status = 'verified' WHERE slug = 'armor/elite-jonin-armor';
UPDATE equipment SET cost_ryo = 1500, detection_status = 'verified' WHERE slug = 'armor/ronin-armor';
UPDATE equipment SET cost_ryo = 2500, detection_status = 'verified' WHERE slug = 'armor/samurai-armor';

-- Simple Melee Weapons
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/kunai';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/hand-axe';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/sai';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/tanto-shortsword';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/kama-hand-scythe';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/gunsen';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/quarterstaff';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/quarterstaff-two-hands';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/spear';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/spear-two-hands';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/weighted-chain';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/kusarigama-chained-hand-scythe';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/tekko';

-- Simple Ranged Weapons ("Light Crossbow" has no matching row — see note above)
UPDATE equipment SET cost_ryo = 15, detection_status = 'verified' WHERE slug = 'weapon/senbon';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/short-bow';
UPDATE equipment SET cost_ryo = 10, detection_status = 'verified' WHERE slug = 'weapon/shuriken';
UPDATE equipment SET cost_ryo = 5,  detection_status = 'verified' WHERE slug = 'weapon/sling';
UPDATE equipment SET cost_ryo = 5,  detection_status = 'verified' WHERE slug = 'weapon/bola';

-- Martial Melee Weapons ("Taichi" in this DB is the book's "Tachi" — a
-- pre-existing Mastersheet spelling difference, not introduced here;
-- flagged as a known discrepancy, not silently renamed by a cost-backfill migration)
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/broadsword';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/iron-claw';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/taichi';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/taichi-two-hands';
UPDATE equipment SET cost_ryo = 30, detection_status = 'verified' WHERE slug = 'weapon/katana';
UPDATE equipment SET cost_ryo = 80, detection_status = 'verified' WHERE slug = 'weapon/odachi-great-sword';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/knuckle-blades';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/hidden-blade';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/chained-spear';
UPDATE equipment SET cost_ryo = 25, detection_status = 'verified' WHERE slug = 'weapon/chigiriki';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/whip';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/battle-wire';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/naginata';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/sasumata-forked-spear';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/great-axe';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/scythe';
UPDATE equipment SET cost_ryo = 15, detection_status = 'verified' WHERE slug = 'weapon/tinbe-rochin';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/yari';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/hooked-lance';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/tetsubo';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/tetsubo-two-hands';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/tonfa';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/war-club';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/nunchaku';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/combat-bracers';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/jitte';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/gunbai-fan';
UPDATE equipment SET cost_ryo = 30, detection_status = 'verified' WHERE slug = 'weapon/kanabo';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/otsuchi-hammer';

-- Martial Ranged Weapons ("Torinawa" has no matching row — see note above)
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/chakram';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/monster-chakram';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/fuma-shuriken';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/monster-shuriken';
UPDATE equipment SET cost_ryo = 25, detection_status = 'verified' WHERE slug = 'weapon/boomerang';
UPDATE equipment SET cost_ryo = 50, detection_status = 'verified' WHERE slug = 'weapon/monster-boomerang';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/longbow';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/crossbow-hand';
UPDATE equipment SET cost_ryo = 40, detection_status = 'verified' WHERE slug = 'weapon/crossbow-heavy';
UPDATE equipment SET cost_ryo = 20, detection_status = 'verified' WHERE slug = 'weapon/blowgun';

-- Exotic Melee Weapons ("Triple-Bladed Scythe" has no matching row — see note above)
UPDATE equipment SET cost_ryo = 100, detection_status = 'verified' WHERE slug = 'weapon/sansetsukon';
UPDATE equipment SET cost_ryo = 100, detection_status = 'verified' WHERE slug = 'weapon/cleaver-sword';
UPDATE equipment SET cost_ryo = 100, detection_status = 'verified' WHERE slug = 'weapon/triple-katar';
UPDATE equipment SET cost_ryo = 100, detection_status = 'verified' WHERE slug = 'weapon/urumi';
UPDATE equipment SET cost_ryo = 20,  detection_status = 'verified' WHERE slug = 'weapon/chokuto';

-- Exotic Ranged Weapons
UPDATE equipment SET cost_ryo = 200, detection_status = 'verified' WHERE slug = 'weapon/matchlock-pistol';
UPDATE equipment SET cost_ryo = 250, detection_status = 'verified' WHERE slug = 'weapon/matchlock-rifle';
UPDATE equipment SET cost_ryo = 500, detection_status = 'verified' WHERE slug = 'weapon/hiya-taihou';
UPDATE equipment SET cost_ryo = 250, detection_status = 'verified' WHERE slug = 'weapon/combat-scroll';
UPDATE equipment SET cost_ryo = 500, detection_status = 'verified' WHERE slug = 'weapon/giant-combat-scroll';
UPDATE equipment SET cost_ryo = 300, detection_status = 'verified' WHERE slug = 'weapon/ballistic-kunai-gun';
UPDATE equipment SET cost_ryo = 500, detection_status = 'verified' WHERE slug = 'weapon/ballistic-shuriken-rifle';
