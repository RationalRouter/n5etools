-- Five class_option_entries rows across three Science-Nin upgrade catalogs
-- (Shinobi-Ware Upgrades, Explosive Modifications, Air Treck Enhancements —
-- all bundled multi-item tiers under the Scientific Inquiry group) lost a
-- real, distinct entry into the previous entry's stored description: the
-- sentence immediately before the swallowed entry's own ALL-CAPS header
-- dropped its terminal period during PDF text extraction, so
-- textentries.capsEntryPattern's anchor (which requires punctuation or
-- string-start immediately before a caps run) never fired there and the
-- whole entry was absorbed into its neighbor's body. Same bug shape
-- internal/store/classoptionentries.go's knownMissingPeriodFixes already
-- corrects for ~15 other instances at ingest time — but that fix only
-- reshapes class_option_entries rows freshly derived from a from-scratch
-- ingest; it never touches the class_options parent row it reads from, and
-- it does nothing at all for rows already persisted in a live rules.db.
-- This migration repairs both: the missing period in each class_options
-- parent's own bundled description, and the corresponding
-- class_option_entries split (truncating the entry that swallowed its
-- neighbor, re-numbering sort_order, and inserting the recovered entry).
--
-- Every statement re-checks the exact broken text (or, for the INSERT
-- statements, checks the target row doesn't already exist) before acting,
-- so this migration is a no-op on a database that never had the bug
-- (a fresh install whose ingest already ran with a fixed parser) or one
-- that already applied this migration once.

-- 1. Shinobi-Ware Upgrades / Refined tier: "Synthweave Skin" swallowed into
--    "Chakra-Powered Grappling Hand".
UPDATE class_options
SET description = REPLACE(description,
    'must be recovered SYNTHWEAVE SKIN',
    'must be recovered. SYNTHWEAVE SKIN')
WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/refined'
  AND description LIKE '%must be recovered SYNTHWEAVE SKIN%';

UPDATE class_option_entries
SET description = 'Prerequisite: Grappling Hand Cost: 4 Creation Points Drain: 5 CCD Chakra While your grappling hand is deployed, when you cast a Ninjutsu with a range of touch you can pay the Drain of this upgrade and have your hand-deliver the jutsu as if you had cast it from its location. After this the hand is made inert and must be recovered.'
WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/refined/entry/chakra-powered-grappling-hand'
  AND description = 'Prerequisite: Grappling Hand Cost: 4 Creation Points Drain: 5 CCD Chakra While your grappling hand is deployed, when you cast a Ninjutsu with a range of touch you can pay the Drain of this upgrade and have your hand-deliver the jutsu as if you had cast it from its location. After this the hand is made inert and must be recovered SYNTHWEAVE SKIN Cost: 4 Creation Points Drain: 5 CCD Chakra As an Action, You can spend the drain of this upgrade and for the next minute reduce all bludgeoning, piercing, and slashing damage by your Intelligence Modifier for the next minute.';

UPDATE class_option_entries
SET sort_order = sort_order + 1
WHERE class_option_slug = 'class/science-nin/option/shinobi-ware-upgrades/refined'
  AND sort_order > 0
  AND NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/refined/entry/synthweave-skin');

INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order, source_book, source_version, source_page, detection_status)
SELECT 'class/science-nin/option/shinobi-ware-upgrades/refined/entry/synthweave-skin',
       'class/science-nin/option/shinobi-ware-upgrades/refined',
       'Synthweave Skin',
       'Cost: 4 Creation Points Drain: 5 CCD Chakra As an Action, You can spend the drain of this upgrade and for the next minute reduce all bludgeoning, piercing, and slashing damage by your Intelligence Modifier for the next minute.',
       1, 'book/class-compendium', '2025-05-04 (auto, drive)', 216, 'auto'
WHERE NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/refined/entry/synthweave-skin')
  AND EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/refined/entry/chakra-powered-grappling-hand');

-- 2. Shinobi-Ware Upgrades / Superior tier: "Bijuu Knuckles" swallowed into
--    "Pain Editor".
UPDATE class_options
SET description = REPLACE(description,
    'until the end of your next turn BIJUU KNUCKLES',
    'until the end of your next turn. BIJUU KNUCKLES')
WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/superior'
  AND description LIKE '%until the end of your next turn BIJUU KNUCKLES%';

UPDATE class_option_entries
SET description = 'Cost: 16 Creation Points Drain: 15 CCD Chakra You modify your body with a switch that shuts off your pain receptors dynamically. As a reaction to taking damage you may spend the Drain of this upgrade to delay all damage you take this round until the end of your next turn.'
WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/superior/entry/pain-editor'
  AND description = 'Cost: 16 Creation Points Drain: 15 CCD Chakra You modify your body with a switch that shuts off your pain receptors dynamically. As a reaction to taking damage you may spend the Drain of this upgrade to delay all damage you take this round until the end of your next turn BIJUU KNUCKLES Prerequisite: Power Knuckles Cost: 16 Creation Points Drain: 15 CCD Chakra You further modify your knuckles with increased reinforcement and weight. Your unarmed strike now deals 1d8 Lightning damage.  If you move at least 10 feet in a straight line immediately before making an unarmed attack, you can activate this upgrade to additional 2d6 force damage';

UPDATE class_option_entries
SET sort_order = sort_order + 1
WHERE class_option_slug = 'class/science-nin/option/shinobi-ware-upgrades/superior'
  AND sort_order > 1
  AND NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/superior/entry/bijuu-knuckles');

INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order, source_book, source_version, source_page, detection_status)
SELECT 'class/science-nin/option/shinobi-ware-upgrades/superior/entry/bijuu-knuckles',
       'class/science-nin/option/shinobi-ware-upgrades/superior',
       'Bijuu Knuckles',
       'Prerequisite: Power Knuckles Cost: 16 Creation Points Drain: 15 CCD Chakra You further modify your knuckles with increased reinforcement and weight. Your unarmed strike now deals 1d8 Lightning damage.  If you move at least 10 feet in a straight line immediately before making an unarmed attack, you can activate this upgrade to additional 2d6 force damage',
       2, 'book/class-compendium', '2025-05-04 (auto, drive)', 217, 'auto'
WHERE NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/superior/entry/bijuu-knuckles')
  AND EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/shinobi-ware-upgrades/superior/entry/pain-editor');

-- 3. Grenadier's Explosive Modifications / Refined tier: "Dud B.I.M"
--    swallowed into "Sting B.I.M".
UPDATE class_options
SET description = REPLACE(description,
    'the ranks of Bruised by 1 DUD B.I.M',
    'the ranks of Bruised by 1. DUD B.I.M')
WHERE slug = 'class/science-nin/option/explosive-modifications/refined'
  AND description LIKE '%the ranks of Bruised by 1 DUD B.I.M%';

UPDATE class_option_entries
SET description = 'Cost: 4-6 Creation Points You throw a B.I.M designed to weaken creatures enough that they surrender, or that dispatching them becomes a trivial task. Each creature within 15 feet of the target space must make a Constitution saving throw. On a failed save, a creature takes 4d4 bludgeoning damage and gains 1 rank of Bruised or half as much damage and no additional effects on a successful one. Upgraded: For every 1 Creation Point after the minimum initial cost increase the damage dice by 1 or the ranks of Bruised by 1.'
WHERE slug = 'class/science-nin/option/explosive-modifications/refined/entry/sting-b-i-m'
  AND description = 'Cost: 4-6 Creation Points You throw a B.I.M designed to weaken creatures enough that they surrender, or that dispatching them becomes a trivial task. Each creature within 15 feet of the target space must make a Constitution saving throw. On a failed save, a creature takes 4d4 bludgeoning damage and gains 1 rank of Bruised or half as much damage and no additional effects on a successful one. Upgraded: For every 1 Creation Point after the minimum initial cost increase the damage dice by 1 or the ranks of Bruised by 1 DUD B.I.M Cost: 4 Creation Points You throw a dud B.I.M that looks identical to a real one, making creatures flinch. You throw your B.I.M and hostile creatures within 30 feet must make a Wisdom saving throw. On a fail they spend their reaction ducking, not realizing it’s a fake bomb.';

UPDATE class_option_entries
SET sort_order = sort_order + 1
WHERE class_option_slug = 'class/science-nin/option/explosive-modifications/refined'
  AND sort_order > 3
  AND NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/explosive-modifications/refined/entry/dud-b-i-m');

INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order, source_book, source_version, source_page, detection_status)
SELECT 'class/science-nin/option/explosive-modifications/refined/entry/dud-b-i-m',
       'class/science-nin/option/explosive-modifications/refined',
       'Dud B.I.M',
       'Cost: 4 Creation Points You throw a dud B.I.M that looks identical to a real one, making creatures flinch. You throw your B.I.M and hostile creatures within 30 feet must make a Wisdom saving throw. On a fail they spend their reaction ducking, not realizing it’s a fake bomb.',
       4, 'book/class-compendium', '2025-05-04 (auto, drive)', 232, 'auto'
WHERE NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/explosive-modifications/refined/entry/dud-b-i-m')
  AND EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/explosive-modifications/refined/entry/sting-b-i-m');

-- 4. Storm Rider's Air Treck Enhancements / Minor tier: "Tank Treads"
--    swallowed into "Reversing Wheels".
UPDATE class_options
SET description = REPLACE(description,
    'stand up from being prone TANK TREADS',
    'stand up from being prone. TANK TREADS')
WHERE slug = 'class/science-nin/option/air-treck-enhancements/minor'
  AND description LIKE '%stand up from being prone TANK TREADS%';

UPDATE class_option_entries
SET description = 'Cost: 2 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You outfit your A. Ts with a device that allows you to reverse and build power with much more ease. It only costs 5 feet of movement to stand up from being prone.'
WHERE slug = 'class/science-nin/option/air-treck-enhancements/minor/entry/reversing-wheels'
  AND description = 'Cost: 2 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You outfit your A. Ts with a device that allows you to reverse and build power with much more ease. It only costs 5 feet of movement to stand up from being prone TANK TREADS Cost: 2 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You replace your stock wheels with tank treads that increase power while sacrificing speed. Your movement speed is only increased by half while using A. Ts with this upgrade, but you gain advantage on Strength (Athletics) checks and your A. Ts lose the light property and gain the heavy property. This upgrade is incompatible with Bow Rollers.';

UPDATE class_option_entries
SET sort_order = sort_order + 1
WHERE class_option_slug = 'class/science-nin/option/air-treck-enhancements/minor'
  AND sort_order > 0
  AND NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/air-treck-enhancements/minor/entry/tank-treads');

INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order, source_book, source_version, source_page, detection_status)
SELECT 'class/science-nin/option/air-treck-enhancements/minor/entry/tank-treads',
       'class/science-nin/option/air-treck-enhancements/minor',
       'Tank Treads',
       'Cost: 2 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You replace your stock wheels with tank treads that increase power while sacrificing speed. Your movement speed is only increased by half while using A. Ts with this upgrade, but you gain advantage on Strength (Athletics) checks and your A. Ts lose the light property and gain the heavy property. This upgrade is incompatible with Bow Rollers.',
       1, 'book/class-compendium', '2025-05-04 (auto, drive)', 240, 'auto'
WHERE NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/air-treck-enhancements/minor/entry/tank-treads')
  AND EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/air-treck-enhancements/minor/entry/reversing-wheels');

-- 5. Storm Rider's Air Treck Enhancements / Refined tier: "Shock Absorbers"
--    swallowed into "Reinforced Frame".
UPDATE class_options
SET description = REPLACE(description,
    'gains the Blocking property SHOCK ABSORBERS',
    'gains the Blocking property. SHOCK ABSORBERS')
WHERE slug = 'class/science-nin/option/air-treck-enhancements/refined'
  AND description LIKE '%gains the Blocking property SHOCK ABSORBERS%';

UPDATE class_option_entries
SET description = 'Cost: 4 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You use more durable materials to make your frame, allowing it to take more blows. Your A. T gains the Blocking property.'
WHERE slug = 'class/science-nin/option/air-treck-enhancements/refined/entry/reinforced-frame'
  AND description = 'Cost: 4 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You use more durable materials to make your frame, allowing it to take more blows. Your A. T gains the Blocking property SHOCK ABSORBERS Cost: 4 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You install better shock absorbers into your A. Ts, allowing you to fly much higher within being at risk. You only take falling damage if it is greater than your maximum movement speed. Whenever you take fall damage you remain standing instead of falling prone.';

UPDATE class_option_entries
SET sort_order = sort_order + 1
WHERE class_option_slug = 'class/science-nin/option/air-treck-enhancements/refined'
  AND sort_order > 1
  AND NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/air-treck-enhancements/refined/entry/shock-absorbers');

INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order, source_book, source_version, source_page, detection_status)
SELECT 'class/science-nin/option/air-treck-enhancements/refined/entry/shock-absorbers',
       'class/science-nin/option/air-treck-enhancements/refined',
       'Shock Absorbers',
       'Cost: 4 Creation Points Drain: 5 CCD Chakra Increase the cost of activating your Air Trecks by the CCD Drain of this Enhancement. You install better shock absorbers into your A. Ts, allowing you to fly much higher within being at risk. You only take falling damage if it is greater than your maximum movement speed. Whenever you take fall damage you remain standing instead of falling prone.',
       2, 'book/class-compendium', '2025-05-04 (auto, drive)', 240, 'auto'
WHERE NOT EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/air-treck-enhancements/refined/entry/shock-absorbers')
  AND EXISTS (SELECT 1 FROM class_option_entries WHERE slug = 'class/science-nin/option/air-treck-enhancements/refined/entry/reinforced-frame');
