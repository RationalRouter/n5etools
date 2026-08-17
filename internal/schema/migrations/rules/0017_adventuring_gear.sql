-- Fills the equipment chapter's "Adventuring Gear" section (Naruto 5e -
-- Full Document.pdf, book/core v3.11, printed pages 32-48) -- previously
-- equipment only had Weapons/Armor/Enhancement Seals (251 rows); Gear,
-- Tools, Toolkits and Scrolls were 100% empty. Hand-transcribed directly
-- from the PDF text (both `pdftotext -layout` and plain reading-order
-- dumps cross-checked against each other, since this chapter's two-column
-- layout interleaves badly in either single mode), not auto-parsed --
-- same 'manual' detection_status convention as 0016's armor properties.
--
-- Kind assignment, using only the CHECK constraint's EXISTING allowed
-- values (see 0001_init.sql) rather than widening it: SQLite can't alter an
-- existing column's CHECK without recreating the table, and this
-- migration runner executes each file as a single transaction where the
-- PRAGMA needed to safely recreate a table referenced by other tables'
-- foreign keys (legacy_alter_table) is a documented no-op mid-transaction
-- (confirmed empirically before writing this file) -- not worth the
-- shared-infrastructure risk for what's ultimately a cosmetic grouping
-- label. 'tool' was sitting completely unused (0 rows); repurposed here
-- for the book's "Ninja Tools"-style consumable gadgets (communication
-- devices, explosive tags/bombs) so they don't all flatten into one giant
-- undifferentiated "Gear" section on the Items page:
--   gear    -- clothing, equipment packs, mundane equipment, medical/
--             consumables, utility kits, storage tools
--   tool    -- communication/electronics, explosive tags & bombs
--   toolkit -- the 12 real toolkits (Alchemist through Weaponsmith)
--   scroll  -- every scroll type (item/weapon/jutsu/recording/data/server)
--
-- Poisons and Trap templates are deliberately NOT `equipment` rows at all:
-- they're crafted via a Toolkit during downtime, never bought off a price
-- list the way everything above is, and their stat shape (poison rank/
-- craft DC, trap build/save/notice DC) doesn't fit equipment's columns --
-- same "one dedicated table per real concept" call this codebase already
-- made for weapon vs. armor properties in 0012/0016.
--
-- Traps' underlying DC-scaling/DM-facing mechanics (Ninja-Net security
-- tiers, generic ability-check DC tables, lock quality's own DC/duration/
-- HP-multiplier columns) are pure rules reference, not purchasable or
-- craftable templates -- deliberately excluded, same boundary as the
-- already-excluded jutsu-creation cost rules.

ALTER TABLE equipment ADD COLUMN charges INTEGER;      -- toolkit/kit charge count
ALTER TABLE equipment ADD COLUMN craft_dc INTEGER;      -- kit "Swift Craft DC" (Armorsmith/Weaponsmith)
ALTER TABLE equipment ADD COLUMN heal_amount TEXT;      -- dice notation for HP/Chakra/Temp regained
ALTER TABLE equipment ADD COLUMN regain_type TEXT CHECK (regain_type IN ('hp','chakra','temp_hp_chakra'));
ALTER TABLE equipment ADD COLUMN uses INTEGER;          -- charges/syringes/doses before replacement
ALTER TABLE equipment ADD COLUMN bonus_effect TEXT;     -- tag/bomb "Bonus Effect" column, kit tier perks
ALTER TABLE equipment ADD COLUMN save_dc INTEGER;       -- explosive tags/bombs' own Save DC

-- v_equipment (0012_weapon_properties.sql) needs to project the 7 new
-- columns too, same as 0014's class_levels view fix -- a plain DROP/CREATE
-- VIEW is safe (unlike the table-recreate problem discussed above), views
-- have no foreign-key-rewrite hazard since they're just re-resolved by name.
DROP VIEW v_equipment;
CREATE VIEW v_equipment AS
SELECT slug, name, kind, cost_ryo, weight_lb, description,
       damage_dice, damage_type, properties, ammo_die, weapon_category,
       ac_bonus, armor_category, strength_req, stealth_disadv, bulk,
       armor_ability_1, armor_ability_2, armor_max_mod,
       seal_rank, seal_applies_to, detection_status, notes,
       charges, craft_dc, heal_amount, regain_type, uses, bonus_effect, save_dc
FROM equipment;

-- equipment.source_book carries a real FK to source_books (0001_init.sql) --
-- unlike weapon_properties/armor_properties, which deliberately dropped it
-- (see 0012's comment) for this exact reason. source_books is normally
-- populated by n5e-ingest's own upsert (internal/store/store.go,
-- upsertSourceBook, a real ON CONFLICT DO UPDATE), not by any migration, so
-- a from-empty schema apply (this migration is the first to INSERT new
-- equipment rows rather than only UPDATE pre-ingested ones) has no
-- 'book/core' row yet to satisfy that FK. INSERT OR IGNORE seeds a minimal
-- placeholder row here purely so this migration works standalone (schema
-- tests, a fresh dev clone before ever running n5e-ingest); a real ingest
-- run afterward overwrites every column via its ON CONFLICT clause, so
-- this placeholder can never linger as stale data.
INSERT OR IGNORE INTO source_books (slug, title, version, file_name, file_sha256)
VALUES ('book/core', 'Naruto 5e - Full Document', '3.11', 'Naruto 5e - Full Document.pdf', 'unknown');

-- ---------------------------------------------------------------------------
-- CLOTHING (p.32) -- 1 Bulk each
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('gear/ordinary-clothing', 'Ordinary Clothing', 'gear', 5, 1, 'Common clothing made through basic tailoring. Counts as unarmored for the purpose of calculating AC.', 'book/core', '3.11', 32, 'manual'),
('gear/fine-clothing', 'Fine Clothing', 'gear', 200, 1, 'Grants a +1d4 bonus to Charisma checks made against someone living a Comfortable lifestyle or lower.', 'book/core', '3.11', 32, 'manual'),
('gear/high-fashion-clothing', 'High-Fashion Clothing', 'gear', 500, 1, 'Grants a +1d8 bonus to Charisma checks made against someone living a Wealthy or Unprecedented lifestyle.', 'book/core', '3.11', 32, 'manual'),
('gear/winter-clothing', 'Winter Clothing', 'gear', 100, 1, 'Grants advantage on all checks or saving throws made as a result of a cold environment.', 'book/core', '3.11', 32, 'manual');

-- ---------------------------------------------------------------------------
-- EQUIPMENT PACKS (p.32) -- all contents sealed in an Item Scroll, 1 Bulk
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('gear/captains-pack', 'Captain''s Pack', 'gear', 400, 1, 'Contents: 1 Toolkit (pick one), Commoner''s clothes, an empty book, writing utensils, 7 days of field rations, a compass, a Shinobi Leg Pouch, and a thermos.', 'book/core', '3.11', 32, 'manual'),
('gear/explorers-pack', 'Explorer''s Pack', 'gear', 400, 1, 'Contents: 1 bedroll, 5 glow rods, 5 heating pads, 1 Radio Link, Commoner''s clothes, 7 days of field rations, a Shinobi Backpack, and 50 feet of rope sealed in a scroll.', 'book/core', '3.11', 32, 'manual'),
('gear/infiltrators-pack', 'Infiltrator''s Pack', 'gear', 700, 1, 'Contents: 1 Hackers Kit or Security Kit (pick one), 1 blank Keycard, 1 blank Data Scroll, and a Shinobi Waist Bag.', 'book/core', '3.11', 32, 'manual'),
('gear/crafters-pack', 'Crafter''s Pack', 'gear', 500, 1, 'Contents: 3 Toolkits (pick three).', 'book/core', '3.11', 32, 'manual'),
('gear/travelers-pack', 'Traveler''s Pack', 'gear', 200, 1, 'Contents: Commoner''s clothes, a Shinobi Waist Bag, a Shinobi Belt Pouch, a Shinobi Leg Pouch, 1 bedroll, 2 glow rods, and 1 blank map scroll.', 'book/core', '3.11', 32, 'manual');

-- ---------------------------------------------------------------------------
-- MUNDANE EQUIPMENT (p.32-33) -- non-standard items default to 100 Ryo
-- unless otherwise stated; Lock's 6 quality tiers come from its own DC/
-- duration/failed-attempts/HP-multiplier table (folded into description,
-- not worth 4 new columns for one 6-row item).
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('gear/compass', 'Compass', 'gear', 100, 0, 'Grants a +2 bonus to Survival checks made to navigate, so long as the user is not underground or at an extreme elevation.', 'book/core', '3.11', 32, 'manual'),
('gear/grappling-hook', 'Grappling Hook', 'gear', 100, 1, 'Thrown with an attached rope to ease a climb. Anchoring requires a Strength or Dexterity (Survival) check vs. the anchor point''s DC; failing by 5 or more means the hook seems to hold but gives way partway up.', 'book/core', '3.11', 32, 'manual'),
('gear/bedroll', 'Bedroll', 'gear', 100, 2, 'Lets a creature gain the benefits of a short or long rest while not indoors or in a comfortable environment.', 'book/core', '3.11', 32, 'manual'),
('gear/caltrops', 'Caltrops', 'gear', 100, 1, 'Scattered in an adjacent space as a bonus action. The first creature to move into that square must succeed on a DC 15 Acrobatics check or take 1d4 piercing damage and gain 1 rank of bleed, plus a 5-foot Speed penalty until the caltrops are plucked free.', 'book/core', '3.11', 32, 'manual'),
('gear/heating-pad', 'Heating Pad', 'gear', 100, 0, 'While active (up to 1 hour), grants resistance to cold damage and immunity to the Chilled condition from a hostile environment. Single use.', 'book/core', '3.11', 32, 'manual'),
('gear/lock-poor', 'Lock (Poor Quality)', 'gear', 1, NULL, 'AC 20, HP x1. Lockpick DC 10, 1 Round to pick, jams after 5 failed attempts. Requires a Security Kit to pick.', 'book/core', '3.11', 33, 'manual'),
('gear/lock-simple', 'Lock (Simple Quality)', 'gear', 10, NULL, 'AC 20, HP x2. Lockpick DC 15, 1d4 Rounds to pick, jams after 5 failed attempts. Requires a Security Kit to pick.', 'book/core', '3.11', 33, 'manual'),
('gear/lock-average', 'Lock (Average Quality)', 'gear', 50, NULL, 'AC 20, HP x4. Lockpick DC 20, 1 Minute to pick, jams after 3 failed attempts. Requires a Security Kit to pick.', 'book/core', '3.11', 33, 'manual'),
('gear/lock-greater', 'Lock (Greater Quality)', 'gear', 200, NULL, 'AC 20, HP x6. Lockpick DC 25, 5 Minutes to pick, jams after 3 failed attempts. Requires a Security Kit to pick.', 'book/core', '3.11', 33, 'manual'),
('gear/lock-superior', 'Lock (Superior Quality)', 'gear', 1000, NULL, 'AC 20, HP x10. Lockpick DC 30, 10 Minutes to pick, jams after 2 failed attempts. Requires a Security Kit to pick.', 'book/core', '3.11', 33, 'manual'),
('gear/lock-supreme', 'Lock (Supreme Quality)', 'gear', 5000, NULL, 'AC 20, HP x15. Lockpick DC 35, 1 Hour to pick, jams after 1 failed attempt. Requires a Security Kit to pick.', 'book/core', '3.11', 33, 'manual');

-- ---------------------------------------------------------------------------
-- COMMUNICATION & DATA RECORDING TOOLS (p.34-35) -- kind='tool', the
-- previously-unused kind (see header comment)
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('tool/radio-link', 'Radio Link', 'tool', 50, 0, 'A standard handheld communication device with a range of up to 1 mile, reduced by up to half in dense urban areas or areas of high interference.', 'book/core', '3.11', 34, 'manual'),
('tool/radio-link-worn', 'Radio Link (Worn)', 'tool', 150, 0, 'A Radio Link installed into clothing or armor, or worn independently, functioning hands-free.', 'book/core', '3.11', 35, 'manual'),
('tool/radio-jammer', 'Radio Jammer', 'tool', 500, 1, 'Scrambles communications, blocking transmissions from communication devices in a 1-mile radius.', 'book/core', '3.11', 34, 'manual'),
('tool/radio-tracing-device', 'Radio Tracing Device', 'tool', 1500, 2, 'A worn gadget used to trace a radio transmission back to its source, vibrating more strongly the closer it gets.', 'book/core', '3.11', 34, 'manual'),
('tool/video-chat-device', 'Video Chat Device', 'tool', 350, 1, 'A communications unit that sends and receives live video over the Ninja-Net. Usually requires a Village or City''s infrastructure to be reliable.', 'book/core', '3.11', 34, 'manual'),
('tool/keycards', 'Keycards', 'tool', 100, 1, 'Small plastic cards containing coded access information, assigned to a specific Data Server. A blank keycard plus a Hackers Kit can potentially spoof access credentials.', 'book/core', '3.11', 35, 'manual');

-- ---------------------------------------------------------------------------
-- SCROLLS (p.34-35, 47) -- kind='scroll'
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('scroll/recording-scroll', 'Recording Scroll', 'scroll', 250, 1, 'Captures and stores audio/visual information fed to it within 15 feet, transcribed as plain text -- up to 2000 words or 5 minutes of audio.', 'book/core', '3.11', 35, 'manual'),
('scroll/data-scroll', 'Data Scroll', 'scroll', 500, 2, 'A scroll with an input port that works like a flash drive: stores and transmits data, and can display stored images or plain text.', 'book/core', '3.11', 35, 'manual'),
('scroll/data-server-scroll', 'Data Server Scroll', 'scroll', NULL, 3, 'A Large scroll, roughly 5 feet long and 3 feet thick, that stores massive amounts of information and acts as a server/database -- the backbone of most modern cities'' Ninja-Net infrastructure. No fixed price is printed.', 'book/core', '3.11', 35, 'manual'),
('scroll/blank-scroll', 'Blank Weapon/Item/Jutsu Scroll', 'scroll', 50, 1, 'A blank scroll usable to seal a single weapon (up to 5 Bulk), up to 5 Bulk of items, or a known Ninjutsu/Genjutsu jutsu, depending on how it''s prepared.', 'book/core', '3.11', 47, 'manual'),
('scroll/e-rank-jutsu-scroll', 'E-Rank Jutsu Scroll', 'scroll', 25, 1, 'A pre-sealed scroll storing an E-Rank jutsu, with a predefined Save DC and attack bonus based on rank.', 'book/core', '3.11', 47, 'manual'),
('scroll/d-rank-jutsu-scroll', 'D-Rank Jutsu Scroll', 'scroll', 100, 1, 'A pre-sealed scroll storing a D-Rank jutsu, with a predefined Save DC and attack bonus based on rank.', 'book/core', '3.11', 47, 'manual'),
('scroll/c-rank-jutsu-scroll', 'C-Rank Jutsu Scroll', 'scroll', 250, 1, 'A pre-sealed scroll storing a C-Rank jutsu, with a predefined Save DC and attack bonus based on rank.', 'book/core', '3.11', 47, 'manual'),
('scroll/b-rank-jutsu-scroll', 'B-Rank Jutsu Scroll', 'scroll', 1000, 1, 'A pre-sealed scroll storing a B-Rank jutsu, with a predefined Save DC and attack bonus based on rank.', 'book/core', '3.11', 47, 'manual'),
('scroll/a-rank-jutsu-scroll', 'A-Rank Jutsu Scroll', 'scroll', 2500, 1, 'A pre-sealed scroll storing an A-Rank jutsu, with a predefined Save DC and attack bonus based on rank.', 'book/core', '3.11', 47, 'manual'),
('scroll/s-rank-jutsu-scroll', 'S-Rank Jutsu Scroll', 'scroll', 5000, 1, 'A pre-sealed scroll storing an S-Rank jutsu, with a predefined Save DC and attack bonus based on rank.', 'book/core', '3.11', 47, 'manual');

-- ---------------------------------------------------------------------------
-- EXPLOSIVE TOOLS (p.36-38) -- kind='tool', thrown/set consumables. Range is
-- 30 feet (+ Str mod x5 for the "thrown" variants) unless noted; damage_dice
-- carries the full compound string for dual-damage-type items (Breaching
-- Tag) since damage_type can't cleanly hold two types at once.
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, damage_dice, damage_type, save_dc, bonus_effect, description, source_book, source_version, source_page, detection_status) VALUES
('tool/paper-bombs', 'Paper Bombs', 'tool', 25, 1, '5d4', 'Fire', 13, NULL, 'A thrown explosive tag, range 30 feet. Each creature within 10 feet makes a Dexterity save taking the listed damage, or half on success. Can be affixed to a Kunai (detonates on the Kunai''s next hit) or secretly planted via a Sleight of Hand check.', 'book/core', '3.11', 37, 'manual'),
('tool/greater-paper-bombs', 'Greater Paper Bombs', 'tool', 150, 1, '8d4', 'Fire', 16, NULL, 'See Paper Bombs.', 'book/core', '3.11', 37, 'manual'),
('tool/superior-paper-bombs', 'Superior Paper Bombs', 'tool', 500, 1, '11d4', 'Fire', 19, NULL, 'See Paper Bombs.', 'book/core', '3.11', 37, 'manual'),
('tool/supreme-paper-bombs', 'Supreme Paper Bombs', 'tool', 750, 1, '14d4', 'Fire', 22, NULL, 'See Paper Bombs.', 'book/core', '3.11', 37, 'manual'),
('tool/breaching-tag', 'Breaching Tag', 'tool', 100, 1, '3d6 Fire/3d6 Bludgeoning', NULL, 15, NULL, 'Used to blow holes in structures. Takes 1 minute to install; detonates on a 6-second timer or by remote detonator within 250 feet. Damages up to a 10x10x5-foot section of wall; each creature within 20 feet makes a Dexterity save (constructs at disadvantage) taking the listed damage, or half on success. Constructs and structures take triple damage.', 'book/core', '3.11', 37, 'manual'),
('tool/greater-breaching-tag', 'Greater Breaching Tag', 'tool', 250, 1, '5d6 Fire/5d6 Bludgeoning', NULL, 17, NULL, 'See Breaching Tag.', 'book/core', '3.11', 37, 'manual'),
('tool/superior-breaching-tag', 'Superior Breaching Tag', 'tool', 500, 1, '7d6 Fire/7d6 Bludgeoning', NULL, 19, NULL, 'See Breaching Tag.', 'book/core', '3.11', 37, 'manual'),
('tool/supreme-breaching-tag', 'Supreme Breaching Tag', 'tool', 1000, 1, '9d6 Fire/9d6 Bludgeoning', NULL, 21, NULL, 'See Breaching Tag.', 'book/core', '3.11', 37, 'manual'),
('tool/chili-pepper-bomb', 'Chili Pepper Bomb', 'tool', NULL, 1, NULL, NULL, NULL, NULL, 'A golf-ball-sized sphere that bursts into a thick red mist (10 ft high, 5 ft wide) on impact. Creatures beginning their turn in the mist may remake a saving throw to resist or end a Genjutsu at advantage, up to twice per use. No fixed price is printed -- craftable via an Alchemist Kit.', 'book/core', '3.11', 37, 'manual'),
('tool/explosive-tag-ball', 'Explosive Tag Ball', 'tool', 75, 1, '3d6', 'Fire', 13, NULL, 'A sphere-shaped Paper Bomb, thrown up to 30 feet + Str mod x5. Each creature within 10 feet makes a Dexterity save taking the listed damage, or half on success.', 'book/core', '3.11', 37, 'manual'),
('tool/greater-explosive-tag-ball', 'Greater Explosive Tag Ball', 'tool', 150, 1, '6d6', 'Fire', 16, NULL, 'See Explosive Tag Ball.', 'book/core', '3.11', 37, 'manual'),
('tool/superior-explosive-tag-ball', 'Superior Explosive Tag Ball', 'tool', 450, 1, '9d6', 'Fire', 19, NULL, 'See Explosive Tag Ball.', 'book/core', '3.11', 37, 'manual'),
('tool/supreme-explosive-tag-ball', 'Supreme Explosive Tag Ball', 'tool', 600, 1, '11d6', 'Fire', 22, NULL, 'See Explosive Tag Ball.', 'book/core', '3.11', 38, 'manual'),
('tool/fire-bomb', 'Fire Bomb', 'tool', 100, 1, '5d6', 'Fire', 15, NULL, 'A palm-sized thrown explosive, range 30 feet + Str mod x5. Each creature within 10 feet makes a Dexterity save taking the listed damage, or half on success; a failed save also knocks the creature prone, burns them, and ignites nearby flammable items.', 'book/core', '3.11', 38, 'manual'),
('tool/greater-fire-bomb', 'Greater Fire Bomb', 'tool', 250, 1, '8d6', 'Fire', 18, NULL, 'See Fire Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/superior-fire-bomb', 'Superior Fire Bomb', 'tool', 750, 1, '11d6', 'Fire', 21, NULL, 'See Fire Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/supreme-fire-bomb', 'Supreme Fire Bomb', 'tool', 900, 1, '14d6', 'Fire', 24, NULL, 'See Fire Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/ice-bomb', 'Ice Bomb', 'tool', 100, 1, '4d6', 'Cold', 15, NULL, 'A palm-sized thrown explosive, range 30 feet + Str mod x5. Each creature within 10 feet makes a Constitution save taking the listed Cold damage and gaining 1 rank of Chilled until the end of their next turn, or half damage and no effect on success.', 'book/core', '3.11', 38, 'manual'),
('tool/greater-ice-bomb', 'Greater Ice Bomb', 'tool', 250, 1, '7d6', 'Cold', 18, NULL, 'See Ice Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/superior-ice-bomb', 'Superior Ice Bomb', 'tool', 750, 1, '10d6', 'Cold', 21, NULL, 'See Ice Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/supreme-ice-bomb', 'Supreme Ice Bomb', 'tool', 900, 1, '13d6', 'Cold', 24, NULL, 'See Ice Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/flash-tag', 'Flash Tag', 'tool', 100, 1, NULL, NULL, 14, NULL, 'A repurposed Paper Bomb creating a blinding flash, range 30 feet. Each creature within 10 feet makes a Wisdom save or is blinded until the end of their turn. Can be affixed to a Kunai like a Paper Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/greater-flash-tag', 'Greater Flash Tag', 'tool', 250, 1, NULL, NULL, 17, '1 Rank of Dazzled', 'See Flash Tag.', 'book/core', '3.11', 38, 'manual'),
('tool/superior-flash-tag', 'Superior Flash Tag', 'tool', 400, 1, NULL, NULL, 20, '3 Ranks of Dazzled', 'See Flash Tag.', 'book/core', '3.11', 38, 'manual'),
('tool/supreme-flash-tag', 'Supreme Flash Tag', 'tool', 675, 1, NULL, NULL, 23, 'Blinded for 1d4 rounds', 'See Flash Tag.', 'book/core', '3.11', 38, 'manual'),
('tool/poison-tag', 'Poison Tag', 'tool', 100, 1, NULL, NULL, 13, NULL, 'Explodes into a 15-ft radius sphere of heavily-obscuring green fog lasting 1 minute (or until dispersed by wind). A creature entering or starting its turn in the fog makes a Constitution save, taking 1d8 poison damage and becoming Envenomed on a failure, or half damage with no effect on success. Constructs and appropriately-protected creatures are unaffected.', 'book/core', '3.11', 38, 'manual'),
('tool/greater-poison-tag', 'Greater Poison Tag', 'tool', 250, 1, NULL, NULL, 16, '1 Rank of Envenomed', 'See Poison Tag.', 'book/core', '3.11', 38, 'manual'),
('tool/superior-poison-tag', 'Superior Poison Tag', 'tool', 475, 1, NULL, NULL, 19, '3 Ranks of Envenomed', 'See Poison Tag.', 'book/core', '3.11', 38, 'manual'),
('tool/supreme-poison-tag', 'Supreme Poison Tag', 'tool', 650, 1, NULL, NULL, 21, '5 Ranks of Envenomed', 'See Poison Tag.', 'book/core', '3.11', 38, 'manual'),
('tool/shock-bomb', 'Shock Bomb', 'tool', 100, 1, '4d6', 'Lightning', 15, NULL, 'Thrown, range 30 feet + Str mod x5. Each creature within 10 feet makes a Dexterity save taking the listed Lightning damage and being shocked until the end of its next turn on a failure, or half damage on success.', 'book/core', '3.11', 38, 'manual'),
('tool/greater-shock-bomb', 'Greater Shock Bomb', 'tool', 250, 1, '5d6', 'Lightning', 18, NULL, 'See Shock Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/superior-shock-bomb', 'Superior Shock Bomb', 'tool', 750, 1, '8d6', 'Lightning', 21, NULL, 'See Shock Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/supreme-shock-bomb', 'Supreme Shock Bomb', 'tool', 900, 1, '11d6', 'Lightning', 24, NULL, 'See Shock Bomb.', 'book/core', '3.11', 38, 'manual'),
('tool/smoke-bomb', 'Smoke Bomb', 'tool', 25, 1, NULL, NULL, NULL, NULL, 'A gumball-sized sphere thrown up to 30 feet + Str mod x5. Explodes into smoke that heavily obscures a 25-foot radius; dispersed in 4 rounds by moderate wind or 1 round by strong wind.', 'book/core', '3.11', 39, 'manual');

-- ---------------------------------------------------------------------------
-- MEDICAL / LIFE SUPPORT & CONSUMABLES (p.38-40) -- kind='gear'
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, heal_amount, regain_type, uses, description, source_book, source_version, source_page, detection_status) VALUES
('gear/antidote-kit-standard', 'Antidote Kit', 'gear', 100, 1, NULL, NULL, 2, 'Contains 2 syringes. As an action, administer a syringe to cure a target of one D-Rank poison affecting them, or grant advantage on saves against Poison/Envenomed of up to 1 rank higher for 1 hour.', 'book/core', '3.11', 39, 'manual'),
('gear/antidote-kit-enhanced', 'Enhanced Antidote Kit', 'gear', 250, 1, NULL, NULL, 2, 'As Antidote Kit, but cures up to C-Rank poisons.', 'book/core', '3.11', 39, 'manual'),
('gear/antidote-kit-greater', 'Greater Antidote Kit', 'gear', 500, 1, NULL, NULL, 2, 'As Antidote Kit, but cures up to B-Rank poisons.', 'book/core', '3.11', 39, 'manual'),
('gear/antidote-kit-superior', 'Superior Antidote Kit', 'gear', 750, 1, NULL, NULL, 2, 'As Antidote Kit, but cures up to A-Rank poisons.', 'book/core', '3.11', 39, 'manual'),
('gear/antidote-kit-supreme', 'Supreme Antidote Kit', 'gear', 1000, 1, NULL, NULL, 2, 'As Antidote Kit, but cures up to S-Rank poisons.', 'book/core', '3.11', 39, 'manual'),
('gear/genjutsu-pill', 'Genjutsu Pill', 'gear', 100, 1, NULL, NULL, 1, 'An edible pill placing the user in a deep hypnotic state for up to 1 hour. The consumer must succeed a DC 15 Wisdom save or be Stunned for the duration; can be woken early by damage or being shaken.', 'book/core', '3.11', 39, 'manual'),
('gear/blood-pill', 'Blood Pill', 'gear', 50, 1, '2d6+5', 'hp', 1, 'An edible pill that regains Hit Points equal to its quality when consumed as a bonus action.', 'book/core', '3.11', 39, 'manual'),
('gear/greater-blood-pill', 'Greater Blood Pill', 'gear', 150, 1, '3d6+10', 'hp', 1, 'See Blood Pill.', 'book/core', '3.11', 39, 'manual'),
('gear/superior-blood-pill', 'Superior Blood Pill', 'gear', 500, 1, '5d6+15', 'hp', 1, 'See Blood Pill.', 'book/core', '3.11', 39, 'manual'),
('gear/supreme-blood-pill', 'Supreme Blood Pill', 'gear', 1000, 1, '7d6+20', 'hp', 1, 'See Blood Pill.', 'book/core', '3.11', 39, 'manual'),
('gear/chakra-pill', 'Chakra Pill', 'gear', 50, 1, '2d6+2', 'chakra', 1, 'An edible pill that regains Chakra Points equal to its quality when consumed as a bonus action. Usable up to 5 times per long rest before costing 1 chakra die instead.', 'book/core', '3.11', 39, 'manual'),
('gear/greater-chakra-pill', 'Greater Chakra Pill', 'gear', 200, 1, '3d6+5', 'chakra', 1, 'See Chakra Pill.', 'book/core', '3.11', 39, 'manual'),
('gear/superior-chakra-pill', 'Superior Chakra Pill', 'gear', 500, 1, '5d6+10', 'chakra', 1, 'See Chakra Pill.', 'book/core', '3.11', 39, 'manual'),
('gear/supreme-chakra-pill', 'Supreme Chakra Pill', 'gear', 1000, 1, '7d6+15', 'chakra', 1, 'See Chakra Pill.', 'book/core', '3.11', 39, 'manual'),
('gear/military-ration-pill', 'Military Ration Pill', 'gear', 150, 1, '2d10+5', 'temp_hp_chakra', 1, 'An edible pill granting Temporary Hit and Chakra Points equal to its quality for 1 hour when consumed as a bonus action. A new dose replaces rather than stacks with the old; usable at most twice per 24 hours.', 'book/core', '3.11', 40, 'manual'),
('gear/greater-military-ration-pill', 'Greater Military Ration Pill', 'gear', 450, 1, '4d10+5', 'temp_hp_chakra', 1, 'See Military Ration Pill.', 'book/core', '3.11', 40, 'manual'),
('gear/superior-military-ration-pill', 'Superior Military Ration Pill', 'gear', 850, 1, '6d10+5', 'temp_hp_chakra', 1, 'See Military Ration Pill.', 'book/core', '3.11', 40, 'manual'),
('gear/supreme-military-ration-pill', 'Supreme Military Ration Pill', 'gear', 1200, 1, '8d10+5', 'temp_hp_chakra', 1, 'See Military Ration Pill.', 'book/core', '3.11', 40, 'manual'),
('gear/first-aid-kit', 'First Aid Kit', 'gear', 100, 2, NULL, NULL, 5, 'Spend 2 uses as an action to stabilize a creature at 0 HP without a Medicine check, removing one failed death save. Alternatively, spend 1 use per creature over a short rest to grant 2d4 HP, or over a long rest to grant 3d6 HP.', 'book/core', '3.11', 40, 'manual'),
('gear/greater-first-aid-kit', 'Greater First Aid Kit', 'gear', 250, 2, NULL, NULL, 5, 'As First Aid Kit, but grants 4d4 HP over a short rest or 5d6 HP over a long rest.', 'book/core', '3.11', 40, 'manual'),
('gear/superior-first-aid-kit', 'Superior First Aid Kit', 'gear', 750, 2, NULL, NULL, 5, 'As First Aid Kit, but grants 6d4 HP over a short rest or 7d6 HP over a long rest.', 'book/core', '3.11', 40, 'manual'),
('gear/supreme-first-aid-kit', 'Supreme First Aid Kit', 'gear', 1500, 2, NULL, NULL, 5, 'As First Aid Kit, but grants 8d4 HP over a short rest or 9d6 HP over a long rest.', 'book/core', '3.11', 40, 'manual'),
('gear/aquatic-rebreather', 'Aquatic Rebreather', 'gear', 250, 1, NULL, NULL, 1, 'A worn breath mask letting the wearer breathe water for up to 1 hour before its filter needs replacing.', 'book/core', '3.11', 38, 'manual'),
('gear/respirator', 'Respirator', 'gear', 150, 1, NULL, NULL, NULL, 'A portable breath mask allowing an oxygen-breather to survive in low-oxygen atmospheres.', 'book/core', '3.11', 38, 'manual');

-- ---------------------------------------------------------------------------
-- TOOLKITS (p.39-46) -- kind='toolkit'. "Regardless of purchase, all
-- toolkits have a Bulk of 2." Poison Kit has no upgrade tiers (its
-- progression is which poisons it can craft -- see the poisons table below
-- -- not a bonus-effect table like the other 11).
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, charges, bonus_effect, description, source_book, source_version, source_page, detection_status) VALUES
('toolkit/alchemist-kit', 'Alchemist Kit', 'toolkit', 200, 2, 5, 'Creates up to 2 items per batch.', 'Craft chemical solutions and compounds. Grants proficiency bonus to checks made to test chemical properties, create compounds/concoctions, or craft Chemical Bombs (Smoke/Ice/Shock/Chili Pepper Bombs), Military Ration Pills, or Chakra Pills.', 'book/core', '3.11', 41, 'manual'),
('toolkit/greater-alchemist-kit', 'Greater Alchemist Kit', 'toolkit', 500, 2, 7, 'Creates up to 4 items per batch.', 'See Alchemist Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/superior-alchemist-kit', 'Superior Alchemist Kit', 'toolkit', 750, 2, 9, 'Creates up to 6 items per batch.', 'See Alchemist Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/supreme-alchemist-kit', 'Supreme Alchemist Kit', 'toolkit', 1200, 2, 12, 'Creates up to 8 items per batch.', 'See Alchemist Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/armorsmith-kit', 'Armorsmith Kit', 'toolkit', 200, 2, 5, NULL, 'Craft armor and Armor Seals up to C-Rank. Swift Craft DC 24. Grants proficiency bonus to checks involving this kit.', 'book/core', '3.11', 40, 'manual'),
('toolkit/greater-armorsmith-kit', 'Greater Armorsmith Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to all checks made to create Armor or Armor Seals.', 'Swift Craft DC 22. See Armorsmith Kit.', 'book/core', '3.11', 40, 'manual'),
('toolkit/superior-armorsmith-kit', 'Superior Armorsmith Kit', 'toolkit', 750, 2, 9, 'Gain a +1d4 bonus to all checks made to create Armor or Armor Seals, and increase the per-week Market Value contribution on items crafted with this kit by 100 Ryo.', 'Swift Craft DC 20. See Armorsmith Kit.', 'book/core', '3.11', 40, 'manual'),
('toolkit/supreme-armorsmith-kit', 'Supreme Armorsmith Kit', 'toolkit', 1000, 2, 12, 'As Superior, plus the per-week Market Value contribution increases by 150 Ryo, and a Swift Craft check''s Ryo bonus increases to 200 for every +3 over the listed DC.', 'Swift Craft DC 18. See Armorsmith Kit.', 'book/core', '3.11', 40, 'manual'),
('toolkit/cooking-kit', 'Cooking Kit', 'toolkit', 200, 2, 5, NULL, 'Prepares food for up to six people. Grants proficiency bonus to checks to identify/create food. Can cook a reinvigorating meal (temp HP = kit quality: 15/20/30/45), create Cooked Food Rations (5 per batch), or create Military Ration Pills.', 'book/core', '3.11', 41, 'manual'),
('toolkit/greater-cooking-kit', 'Greater Cooking Kit', 'toolkit', 500, 2, 7, 'Consuming 3 Cooked Food Rations at once, as an action, can remove 1 rank of Dazed, Envenomed, Exhaustion, or Weakened (up to 2 ranks) once per long rest.', 'See Cooking Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/superior-cooking-kit', 'Superior Cooking Kit', 'toolkit', 750, 2, 9, 'As Greater, but consuming 5 rations removes up to 3 ranks of the condition.', 'See Cooking Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/supreme-cooking-kit', 'Supreme Cooking Kit', 'toolkit', 1200, 2, 12, 'As Greater, but consuming 7 rations removes up to 5 ranks of the condition.', 'See Cooking Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/demolitions-kit', 'Demolitions Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to set or disarm explosives. Crafts Breaching Tags, Paper Bombs, Explosive Tag Balls, and Fire Bombs (up to charges spent, quality of this kit). Disarming a set explosive costs 1 charge as an action: Intelligence or Wisdom (Demolitions Kit) vs. DC 15 + the explosive''s quality bonus (+2/+4/+6/+8).', 'book/core', '3.11', 41, 'manual'),
('toolkit/greater-demolitions-kit', 'Greater Demolitions Kit', 'toolkit', 450, 2, 7, 'Reduce the DC to disarm explosives by -2.', 'See Demolitions Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/superior-demolitions-kit', 'Superior Demolitions Kit', 'toolkit', 750, 2, 9, 'As Greater, and this check can no longer be made at disadvantage.', 'See Demolitions Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/supreme-demolitions-kit', 'Supreme Demolitions Kit', 'toolkit', 1000, 2, 12, 'As Superior, and you can remake a failed check once per attempt without the explosive detonating.', 'See Demolitions Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/disguise-kit', 'Disguise Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to create a disguise. Plain (1 charge, 10 min), Elaborate (2 charges, 1 hour, insight checks against it at disadvantage), or Exquisite (3 charges, 4+ hours, insight checks against it at disadvantage) disguises. A creature can attempt a Wisdom (Insight) check vs. the disguiser''s Intelligence/Charisma (Disguise Kit) to see through it.', 'book/core', '3.11', 41, 'manual'),
('toolkit/greater-disguise-kit', 'Greater Disguise Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to Charisma checks while disguised.', 'See Disguise Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/superior-disguise-kit', 'Superior Disguise Kit', 'toolkit', 750, 2, 9, 'As Greater, and reduces the time to don a disguise to 1 minute.', 'See Disguise Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/supreme-disguise-kit', 'Supreme Disguise Kit', 'toolkit', 1000, 2, 12, 'As Superior, and halves the time needed to create a disguise.', 'See Disguise Kit.', 'book/core', '3.11', 41, 'manual'),
('toolkit/forensics-kit', 'Forensics Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to identify DNA, fingerprints, drugs, chemicals, poisons, or diseases. Analyzing evidence is a two-step process: a DC 15 Investigation check to collect it (1 hour), then an Intelligence (Forensics) check (DC 10 + 1 per day since it was left, capped at 30) to analyze it (1 hour).', 'book/core', '3.11', 42, 'manual'),
('toolkit/greater-forensics-kit', 'Greater Forensics Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to Intelligence checks made with this kit.', 'See Forensics Kit.', 'book/core', '3.11', 42, 'manual'),
('toolkit/superior-forensics-kit', 'Superior Forensics Kit', 'toolkit', 750, 2, 9, 'As Greater, and by spending an additional charge you can reduce the check''s DC by 1 per charge spent.', 'See Forensics Kit.', 'book/core', '3.11', 42, 'manual'),
('toolkit/supreme-forensics-kit', 'Supreme Forensics Kit', 'toolkit', 1000, 2, 12, 'As Superior, and on a success you also learn the gender, size, shape, and other physical properties of the evidence''s source.', 'See Forensics Kit.', 'book/core', '3.11', 42, 'manual'),
('toolkit/forgery-kit', 'Forgery Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to forge documents, badges, IDs, and signatures. Simple (1 charge, 10 min), Complicated (2 charges, 1 hour), or Exquisite (3 charges, 8+ hours) forgeries.', 'book/core', '3.11', 42, 'manual'),
('toolkit/greater-forgery-kit', 'Greater Forgery Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to Intelligence checks made with this kit.', 'See Forgery Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/superior-forgery-kit', 'Superior Forgery Kit', 'toolkit', 750, 2, 9, 'As Greater, and by spending an additional charge you can reduce the check''s DC by 1 per charge spent.', 'See Forgery Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/supreme-forgery-kit', 'Supreme Forgery Kit', 'toolkit', 1000, 2, 12, 'As Superior.', 'See Forgery Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/hackers-kit', 'Hackers Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to hack computers/networks or set cyber traps. Quick Hack (1+ charge, full-turn action) or Full Hack (3+ charges, 1 hour) vs. a system''s Security DC; a Cyber Trap (1 charge) adds a d6 to a hostile entity''s Counter Hack DC.', 'book/core', '3.11', 43, 'manual'),
('toolkit/greater-hackers-kit', 'Greater Hackers Kit', 'toolkit', 500, 2, 7, 'Gain a +1d4 bonus to Intelligence checks made with this kit.', 'See Hackers Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/superior-hackers-kit', 'Superior Hackers Kit', 'toolkit', 750, 2, 9, 'As Greater, and by spending an additional charge you can reduce the check''s DC by 2 per charge spent.', 'See Hackers Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/supreme-hackers-kit', 'Supreme Hackers Kit', 'toolkit', 1000, 2, 12, 'As Superior, and reduces the charges needed to negate a Cyber Trap to 1.', 'See Hackers Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/medicine-kit', 'Medicine Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to medical checks. Medical Check Up (1 charge, 10 min, 6 creatures), Light Patch Up (X charges, 2d4 HP each), Full Fix Up (2 charges, removes 1 failed death save), Stabilization (3 charges, stabilizes + removes up to 2 failed death saves), Condition Treatment (3 charges), or Blood Pill Creation (2 charges per pill). Cures up to C-Rank conditions with Light Patch Up.', 'book/core', '3.11', 43, 'manual'),
('toolkit/greater-medicine-kit', 'Greater Medicine Kit', 'toolkit', 575, 2, 7, 'Can use this kit to gain the benefit of an equal-quality Antidote Kit.', 'Light Patch Up still 2d4; cures up to B-Rank conditions. See Medicine Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/superior-medicine-kit', 'Superior Medicine Kit', 'toolkit', 850, 2, 9, 'As Greater, and also grants the benefit of an equal-quality First Aid Kit.', 'Light Patch Up 2d6; cures up to A-Rank conditions. See Medicine Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/supreme-medicine-kit', 'Supreme Medicine Kit', 'toolkit', 1250, 2, 12, 'As Superior, and can spend 3 charges to gain the benefits of a short rest as a full-turn action, once per long rest.', 'Light Patch Up 2d8; cures up to S-Rank conditions. See Medicine Kit.', 'book/core', '3.11', 43, 'manual'),
('toolkit/poison-kit', 'Poison Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to craft or use poisons. Crafting a poison (over a short rest) costs charges equal to the poison''s Effective Rank and requires an Intelligence or Wisdom (Poison Kit) check vs. its Craft DC. See the Poisons reference for what can be crafted with this kit.', 'book/core', '3.11', 45, 'manual'),
('toolkit/security-kit', 'Security Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency to pick locks and disarm traps. Picking a lock (1 charge) requires five Dexterity/Intelligence/Wisdom (Security Kit) checks vs. the Lock DC. Disarming a trap (2 charges) is a single check vs. its Disarm DC; failure triggers the trap on you.', 'book/core', '3.11', 46, 'manual'),
('toolkit/greater-security-kit', 'Greater Security Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to one check made to pick a lock or disarm a trap.', 'See Security Kit.', 'book/core', '3.11', 46, 'manual'),
('toolkit/superior-security-kit', 'Superior Security Kit', 'toolkit', 750, 2, 9, 'Gain a +1d4 bonus to two lockpicking checks or one disarm check, and reduce the DC of disarming traps by 2.', 'See Security Kit.', 'book/core', '3.11', 46, 'manual'),
('toolkit/supreme-security-kit', 'Supreme Security Kit', 'toolkit', 1000, 2, 12, 'As Superior, and reduces the time to pick a lock by 1d10 minutes (minimum 1 round).', 'See Security Kit.', 'book/core', '3.11', 46, 'manual'),
('toolkit/trappers-kit', 'Trappers Kit', 'toolkit', 200, 2, 5, NULL, 'Grants proficiency bonus to checks to create any trap. Required to create Traps (1 charge each); can enhance a trap for +2 charges, increasing its Save DC by +1, Notice DC by +2, or damage by +1 die.', 'book/core', '3.11', 46, 'manual'),
('toolkit/greater-trappers-kit', 'Greater Trappers Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to all checks made to create a trap.', 'See Trappers Kit.', 'book/core', '3.11', 45, 'manual'),
('toolkit/superior-trappers-kit', 'Superior Trappers Kit', 'toolkit', 750, 2, 9, 'As Greater, and increases the trap''s Notice DC by +2.', 'See Trappers Kit.', 'book/core', '3.11', 45, 'manual'),
('toolkit/supreme-trappers-kit', 'Supreme Trappers Kit', 'toolkit', 1000, 2, 12, 'As Superior, and increases the trap''s Disarm DC by 1d4.', 'See Trappers Kit.', 'book/core', '3.11', 45, 'manual'),
('toolkit/weaponsmith-kit', 'Weaponsmith Kit', 'toolkit', 200, 2, 5, NULL, 'Craft mundane weapons and Chakra-Enhanced Weapons of any type. Swift Craft DC 24. Grants proficiency bonus to checks involving this kit.', 'book/core', '3.11', 46, 'manual'),
('toolkit/greater-weaponsmith-kit', 'Greater Weaponsmith Kit', 'toolkit', 450, 2, 7, 'Gain a +1d4 bonus to all checks made to create Weapons or Weapon Seals.', 'Swift Craft DC 22. See Weaponsmith Kit.', 'book/core', '3.11', 46, 'manual'),
('toolkit/superior-weaponsmith-kit', 'Superior Weaponsmith Kit', 'toolkit', 750, 2, 9, 'Gain a +1d4 bonus to all checks made to create Weapons or Weapon Seals, and increase the per-week Market Value contribution on items crafted with this kit by 100 Ryo.', 'Swift Craft DC 20. See Weaponsmith Kit.', 'book/core', '3.11', 46, 'manual'),
('toolkit/supreme-weaponsmith-kit', 'Supreme Weaponsmith Kit', 'toolkit', 1000, 2, 12, 'As Superior, plus the per-week Market Value contribution increases by 150 Ryo, and a Swift Craft check''s Ryo bonus increases to 200 for every +3 over the listed DC.', 'Swift Craft DC 18. See Weaponsmith Kit.', 'book/core', '3.11', 46, 'manual');

UPDATE equipment SET craft_dc = 24 WHERE slug = 'toolkit/armorsmith-kit';
UPDATE equipment SET craft_dc = 22 WHERE slug = 'toolkit/greater-armorsmith-kit';
UPDATE equipment SET craft_dc = 20 WHERE slug = 'toolkit/superior-armorsmith-kit';
UPDATE equipment SET craft_dc = 18 WHERE slug = 'toolkit/supreme-armorsmith-kit';
UPDATE equipment SET craft_dc = 24 WHERE slug = 'toolkit/weaponsmith-kit';
UPDATE equipment SET craft_dc = 22 WHERE slug = 'toolkit/greater-weaponsmith-kit';
UPDATE equipment SET craft_dc = 20 WHERE slug = 'toolkit/superior-weaponsmith-kit';
UPDATE equipment SET craft_dc = 18 WHERE slug = 'toolkit/supreme-weaponsmith-kit';

-- ---------------------------------------------------------------------------
-- UTILITY KITS & STORAGE TOOLS (p.47-48) -- kind='gear'. Bulk here is the
-- *bonus* a storage item grants (Shinobi pouches/backpack), not its own
-- weight -- matches the book's own "Bulk Bonus" column heading.
-- ---------------------------------------------------------------------------
INSERT INTO equipment (slug, name, kind, cost_ryo, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('gear/camping-kit', 'Camping Kit', 'gear', 75, 5, 'Contains a Two-person Tent, a Bedroll, and a Blanket.', 'book/core', '3.11', 47, 'manual'),
('gear/mess-kit', 'Mess Kit', 'gear', 50, 3, 'Contains 7 days of Field Rations plus cooking/eating utensils.', 'book/core', '3.11', 47, 'manual'),
('gear/pocket-watch', 'Pocket Watch', 'gear', 50, 0, 'A portable timepiece.', 'book/core', '3.11', 47, 'manual'),
('gear/rope-50ft', 'Rope (50 ft)', 'gear', 20, 0, '50 feet of rope.', 'book/core', '3.11', 47, 'manual'),
('gear/field-rations-1-day', 'Field Rations (1 Day)', 'gear', 5, 0, 'One day''s worth of field rations.', 'book/core', '3.11', 47, 'manual'),
('gear/glow-rod', 'Glow Rod', 'gear', 5, 0, 'A small stick that snaps to give off soft neon-green light: 20 feet of bright light plus 10 feet of dim light.', 'book/core', '3.11', 32, 'manual'),
('gear/heat-generator', 'Heat Generator', 'gear', 100, 0, 'A portable heat source.', 'book/core', '3.11', 47, 'manual'),
('gear/binoculars', 'Binoculars', 'gear', 25, 0, 'A handheld optical device for viewing distant objects.', 'book/core', '3.11', 47, 'manual'),
('gear/two-person-tent', 'Two-person Tent', 'gear', 50, 0, 'A tent sized for two occupants.', 'book/core', '3.11', 47, 'manual'),
('gear/shinobi-backpack', 'Shinobi Backpack', 'gear', 250, 10, 'Grants +10 Bulk of carrying capacity. Only one Shinobi storage tool of a given type benefits you at once.', 'book/core', '3.11', 48, 'manual'),
('gear/shinobi-waist-bag', 'Shinobi Waist Bag', 'gear', 75, 5, 'Grants +5 Bulk of carrying capacity. Only one Shinobi storage tool of a given type benefits you at once.', 'book/core', '3.11', 48, 'manual'),
('gear/shinobi-belt-pouch', 'Shinobi Belt Pouch', 'gear', 50, 3, 'Grants +3 Bulk of carrying capacity. Only one Shinobi storage tool of a given type benefits you at once.', 'book/core', '3.11', 48, 'manual'),
('gear/shinobi-leg-pouch', 'Shinobi Leg Pouch', 'gear', 25, 2, 'Grants +2 Bulk of carrying capacity. Only one Shinobi storage tool of a given type benefits you at once.', 'book/core', '3.11', 48, 'manual'),
('gear/thermos', 'Thermos', 'gear', 5, 0, 'Keeps liquid contents hot or cold.', 'book/core', '3.11', 48, 'manual'),
('gear/wallet', 'Wallet', 'gear', 5, 0, 'Holds Ryo and small documents.', 'book/core', '3.11', 48, 'manual'),
('gear/ration-case', 'Ration Case', 'gear', 5, 0, 'A weatherproof case for storing field rations.', 'book/core', '3.11', 48, 'manual');

-- ---------------------------------------------------------------------------
-- POISONS (p.45-46) -- a dedicated table, not `equipment`: crafted via a
-- Poison Kit during downtime rather than bought off a shelf ("cannot be
-- purchased by normal means... must be created or purchased from black
-- market salesmen"), and its stat shape (potency rank, craft DC) doesn't
-- map onto equipment's weapon/armor-oriented columns. Poison Tag (the
-- thrown delivery item) is a separate, already-ingested `equipment` row
-- under kind='tool' -- this table is the poison itself, applied via
-- ingestion or a weapon coating.
-- ---------------------------------------------------------------------------
CREATE TABLE poisons (
    slug              TEXT PRIMARY KEY,       -- 'poison/assassins-blood'
    name              TEXT NOT NULL,
    poison_rank       TEXT REFERENCES jutsu_ranks(rank),
    craft_dc          INTEGER NOT NULL,
    uses              INTEGER,                -- doses per craft, per the book's own "Uses" column
    bulk              REAL,
    cost_ryo          REAL,
    description       TEXT NOT NULL,

    source_book       TEXT,                     -- plain text, no FK (see 0012's comment: source_books is populated by n5e-ingest, not by migrations)
    source_version    TEXT,
    source_page       INTEGER,
    detection_status  TEXT NOT NULL DEFAULT 'manual'
                      CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes             TEXT
);

INSERT INTO poisons (slug, name, poison_rank, craft_dc, uses, bulk, cost_ryo, description, source_book, source_version, source_page) VALUES
('poison/assassins-blood', 'Assassin''s Blood', 'D', 14, 2, 2, 250, 'A creature subjected to this poison must succeed a DC 12 Constitution save, taking 2d6 poison damage and becoming Envenomed for 24 hours on a failure, or half damage and no effect on a success.', 'book/core', '3.11', 45),
('poison/serpent-venom', 'Serpent Venom', 'D', 16, 2, 2, 275, 'Extracted from a dead or incapacitated poisonous snake. A creature subjected to this poison must succeed a DC 13 Constitution save, taking 4d6 poison damage and becoming Envenomed for 24 hours on a failure.', 'book/core', '3.11', 45),
('poison/midnight-tears', 'Midnight Tears', 'C', 18, 2, 2, 350, 'Causes no effect until the stroke of midnight. If not neutralized by then, the creature must succeed a DC 15 Constitution save, taking 6d6 poison damage on a failure or half as much on a success.', 'book/core', '3.11', 45),
('poison/ether', 'Ether', 'C', 18, 2, 2, 375, 'Used in espionage and political manipulation rather than combat. A creature subjected to this poison must succeed a DC 16 Constitution save or become Charmed by the first creature they see who issues them a command, for 24 hours.', 'book/core', '3.11', 45),
('poison/wolfs-bane', 'Wolf''s Bane', 'C', 20, 2, 2, 400, 'Best ingested, though it can be applied to a weapon for less potency. Ingested: DC 17 Constitution save or Envenomed for 72 hours, taking 10d4 poison damage and slowed for the duration on a failure (half on success). Via weapon: DC 16 Constitution save, 5d6 poison damage on a failure, half on success.', 'book/core', '3.11', 46),
('poison/devils-kiss', 'Devil''s Kiss', 'B', 22, 2, 2, 750, 'Best ingested, though it can be applied to a weapon for less potency. Ingested: DC 20 Constitution save, taking 8d8 fire damage (ignores resistance) on a failure or half on success, then a further DC 18 Constitution save each round taking 3d8 fire damage until the creature succeeds.', 'book/core', '3.11', 46),
('poison/kamizuru-venom', 'Kamizuru Venom', 'B', 24, 2, 2, 950, 'Extracted from the Kamizuru''s Bee Forest. A creature subjected to this poison must succeed a DC 19 Constitution save, taking 6d8 poison damage and becoming Envenomed for 1 hour on a failure, or half damage with no further effect on a success.', 'book/core', '3.11', 46),
('poison/moulding-mushroom', 'Moulding Mushroom', 'B', 26, 2, 2, 1250, 'Extracted from the Molding Fungi Forest; can only be ingested. A creature subjected to this poison must succeed a DC 19 Constitution save or be Stunned for 1 hour.', 'book/core', '3.11', 46),
('poison/angels-breath', 'Angel''s Breath', 'A', 28, 2, 2, 2100, 'A mixed concoction of Devil''s Kiss, Moulding Mushroom, and Assassin''s Blood. A creature subjected to this poison must succeed a DC 20 Constitution save or become Unconscious for 96 hours (4 days) on a failure.', 'book/core', '3.11', 46),
('poison/zetsubo-petals', 'Zetsubo Petals', 'A', 28, 2, 2, 3500, 'Developed from an extremely rare flower found only in the Land of Iron. A creature subjected to this poison must succeed a DC 21 Constitution save or gain 5 ranks of Envenomed and the Berserk condition for 1 hour on a failure, or become Envenomed for 1 hour on a success.', 'book/core', '3.11', 46),
('poison/torpor', 'Torpor', 'S', 30, 2, 2, 5000, 'One of the top three most potent poisons in the shinobi world. A creature subjected to this poison must succeed a DC 22 Constitution save or become Envenomed for 1 week, regaining only 1/4 of any hit points it would otherwise regain and taking 10d8 poison damage every 24 hours; can only be neutralized by an effect of A-Rank or higher.', 'book/core', '3.11', 46),
('poison/black-lily', 'Black Lily', 'S', 32, 2, 2, 7500, 'Crafted from the mysterious Black Lotus Lily, so poisonous that its blooming kills wildlife within a mile. A creature subjected to this poison must succeed a DC 23 Constitution save, taking 15d8 poison damage every hour for 5 hours and gaining the Envenomed condition; nearby creatures sharing air can be affected too. Can only be neutralized by an effect of S-Rank.', 'book/core', '3.11', 46),
('poison/malice', 'Malice', 'S', 35, 2, 2, 10000, 'Created by the shinobi Sasori and banned by every village. A creature subjected to this poison must succeed a DC 25 Constitution save (ignoring resistance or immunity) or fall Unconscious for 72 hours, dying at the end of that duration.', 'book/core', '3.11', 46);

-- ---------------------------------------------------------------------------
-- TRAP TEMPLATES (p.33-34) -- a dedicated table, not `equipment`: traps are
-- built with a Trappers/Demolitions/Poison/Hackers/Cooking Kit during
-- downtime, never bought off a price list. All traps begin at D-Rank;
-- every rank increase adds +4 Build DC, +2 Save/Notice DC, and +1 damage
-- die (a generic scaling rule, not repeated per row).
-- ---------------------------------------------------------------------------
CREATE TABLE trap_templates (
    slug              TEXT PRIMARY KEY,       -- 'trap/alarming-trap'
    name              TEXT NOT NULL,
    build_dc          INTEGER NOT NULL,
    save_dc           INTEGER,
    notice_disable_dc INTEGER,
    vs_ability        TEXT,                   -- 'Dexterity' | 'Constitution' | 'Strength'
    time_to_build     TEXT,
    toolkit_required  TEXT NOT NULL,
    description       TEXT NOT NULL,

    source_book       TEXT,                     -- plain text, no FK (see 0012's comment: source_books is populated by n5e-ingest, not by migrations)
    source_version    TEXT,
    source_page       INTEGER,
    detection_status  TEXT NOT NULL DEFAULT 'manual'
                      CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes             TEXT
);

INSERT INTO trap_templates (slug, name, build_dc, save_dc, notice_disable_dc, vs_ability, time_to_build, toolkit_required, description, source_book, source_version, source_page) VALUES
('trap/alarming-trap', 'Alarming Trap', 15, 12, 14, 'Dexterity', '10 Minutes', 'Trappers Kit', 'An alarm snare rigged from noisy objects to a trip wire, pressure plate, or manual trigger, designated to be heard between 100 and 500 feet away. Triggers when a Small or larger creature enters the square.', 'book/core', '3.11', 33),
('trap/deadfall-trap', 'Deadfall Trap', 15, 14, 16, 'Dexterity', '10 Minutes', 'Trappers Kit', 'A weight-sensitive plate or wire drops a large object over a 15-foot cube trigger space, affecting a 15-foot-wide, 60-foot line. Creatures in the line take 2d10 + (5 x rank) bludgeoning damage on a failed save, half on success.', 'book/core', '3.11', 33),
('trap/drowning-pit-trap', 'Drowning Pit Trap', 15, 15, 17, 'Dexterity', '10 Hours', 'Trappers Kit', 'A trapdoor over a 10-foot-square, 30-foot-deep pit with 5 feet of water at the bottom and four spouts. A creature walking onto it falls, taking falling damage (save to avoid falling in); the trap then fills with water via its own initiative (+10), losing its bonus action/action as spouts are disabled.', 'book/core', '3.11', 34),
('trap/explosive-trap', 'Explosive Trap', 15, 12, 14, 'Dexterity', '10 Minutes', 'Demolitions Kit + 2 Paper Bombs', 'Rigged Paper Bombs detonate when a creature enters the trap''s 5-foot space or it''s manually triggered. Creatures within 10 feet take 8d4 fire damage on a failed save (double on a fail by 5+), half on success.', 'book/core', '3.11', 33),
('trap/flashing-trap', 'Flashing Trap', 15, 13, 15, 'Constitution', '1 Minutes', 'Demolitions Kit + 2 Flash Tags', 'Rigged Flash Tags detonate when a creature enters the trap''s 5-foot space or it''s manually triggered. Creatures within 10 feet are blinded for 2d4 rounds on a failed save (blinded and dazed for 1 minute on a fail by 5+), half duration on success.', 'book/core', '3.11', 34),
('trap/hidden-pit-trap', 'Hidden Pit Trap', 15, 15, 17, 'Dexterity', '1 Hour', 'Trappers Kit', 'A trapdoor over a 10-foot-square, 30-foot-deep pit with 5 feet of water at the bottom. A creature walking onto it falls and takes falling damage (save to avoid). Can be reset by reengaging the door.', 'book/core', '3.11', 34),
('trap/poisonous-lock', 'Poisonous Lock', 15, 13, 15, 'Constitution', '1 Hour', 'Poison Kit', 'A spring-loaded lock with an Envenomed spine near the keyhole; breaking the lock does not disengage it. Attempting to unlock/pick it without the correct key triggers a stab, forcing a save to avoid becoming Envenomed for 1 hour (3 ranks of Envenomed on a fail by 5+).', 'book/core', '3.11', 34),
('trap/poisonous-trap', 'Poisonous Trap', 15, 12, 14, 'Dexterity', '10 Minutes', 'Poison Kit + Poison Gas Tags', 'Rigged Poison Tags detonate when a creature enters the trap''s 5-foot space or it''s manually triggered. Creatures within 10 feet become Envenomed on a failed save, half effect on success.', 'book/core', '3.11', 34),
('trap/restraining-trap', 'Restraining Trap', 15, 14, 16, 'Strength', '10 Minutes', 'Trappers Kit + Battle Wire', 'Built to capture creatures entering its 5-foot space. Creatures within 10 feet become grappled and restrained on a failed save (stunned until the end of their next turn, then grappled/restrained for 1 minute, on a fail by 5+); affected creatures remake the save at the end of each of their turns.', 'book/core', '3.11', 34),
('trap/shocking-trap', 'Shocking Trap', 15, 14, 16, 'Constitution', '10 Minutes', 'Hackers Kit + 2 Shock Bombs', 'Rigged Shock Bombs detonate when a creature enters the trap''s 5-foot space or it''s manually triggered. Creatures within 10 feet take 8d6 lightning damage on a failed save (double on a fail by 5+), half on success.', 'book/core', '3.11', 34),
('trap/weapon-trap', 'Weapon Trap', 15, 14, 16, 'Dexterity', '1 Minutes', 'Trappers Kit + 1 Weapon Scroll', 'Small weapons (Kunai, Shuriken, Senbon, etc.) fire across the trap''s 5-foot space when triggered. Creatures within 15 feet take 9d4 Slashing or Piercing damage (attacker''s choice) on a failed save (double damage plus 2 ranks of bleeding on a fail by 5+), half on success.', 'book/core', '3.11', 34),
('trap/yellow-mold', 'Yellow Mold', 15, 18, 20, 'Constitution', '10 Hours', 'Cooking Kit', 'A cultivated poisonous mold spore, triggered when a creature moves into its space, damages it, or fails to disable it. Exposed creatures within 10 feet must save each turn for 1 minute, gaining 1 rank of Envenomed (lasting 24 hours) per failure.', 'book/core', '3.11', 34);
