-- Adds the 3 weapons printed in the core book's weapon tables (Naruto 5e -
-- Full Document.pdf, Chapter 5) that were never in the Mastersheet's
-- WeapArmor tab at all — a genuine Mastersheet ingestion gap flagged (not
-- silently patched) by 0015_equipment_cost.sql's own comment, when that
-- migration backfilled cost_ryo for every OTHER weapon in these same
-- tables but couldn't invent a new row for these three. Confirmed again
-- directly against the book pages here (0015 only recorded name/cost/
-- damage-die/damage-type for these three in passing while its own focus
-- was cost, not full stats) — complete stat lines below, cross-checked
-- against a direct visual read of each page, same standard as 0015/0018:
--   Simple Ranged Weapons:  PDF page 31 (printed 29) — Light Crossbow
--   Martial Ranged Weapons: PDF page 32 (printed 30) — Torinawa
--   Exotic Melee Weapons:   PDF page 33 (printed 31) — Triple-Bladed Scythe
--
-- source_book/source_page/detection_status follow 0017_adventuring_gear.sql's
-- convention for equipment rows inserted directly from the book rather than
-- via a Mastersheet load ('book/core', the PRINTED page number, 'manual').
-- weapon_category and bulk are deliberately left NULL: confirmed by query
-- before writing this that ALL 71 pre-existing weapon rows already have
-- both columns NULL (only the Mastersheet's WeapArmor loader has ever
-- populated them, and it never does for either column in practice) — so
-- leaving them NULL here matches the established data shape, not a new gap.
INSERT INTO equipment (slug, name, kind, cost_ryo, damage_dice, damage_type, properties, source_book, source_version, source_page, detection_status) VALUES
('weapon/light-crossbow', 'Light Crossbow', 'weapon', 20, '1d8', 'Piercing', 'Range (60/120), Two-Handed, Finesse, ammunition', 'book/core', '3.11', 29, 'manual'),
('weapon/torinawa', 'Torinawa', 'weapon', 5, '1d4', 'Bludgeoning', 'Thrown (20/40), Grapple', 'book/core', '3.11', 30, 'manual'),
('weapon/triple-bladed-scythe', 'Triple-Bladed Scythe', 'weapon', 100, '1d12', 'Slashing', 'Reach 2, Deadly 2, Two-Handed, Winding', 'book/core', '3.11', 31, 'manual');
