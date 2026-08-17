-- Removes 14 armor rows loaded from the Mastersheet's WeapArmor tab that
-- were never actually part of the core book's printed Armor table (core
-- book, printed page 26 / PDF page 28) — confirmed by re-reading that page
-- directly and cross-checking every column of the 18 real light/medium/
-- heavy armor rows (which already matched the book exactly, no changes
-- needed there). These 14 used non-standard armor_category values
-- ('ability-mod', 'custom') the book's table never produces, which is what
-- rendered as a broken-looking item card (raw "ability-mod" string, no
-- cost/description/properties) and got reported as "armor data is screwed
-- up".
--
-- 10 of the 14 (Chakra Skin + B/S-Rank variants, Dust Coat + B/S-Rank
-- variants, Geo-Breastplate x2 ability variants, Martial Defense, Skin)
-- could not be traced to any source anywhere else in the DB (searched
-- clan_traits/clan_features/bloodline_latents/class_features/
-- subclass_features/class_options/feats, name and description columns) —
-- dropped outright, not real content.
--
-- The other 4 (Weaved Mail, Steel Fortress, Wooden Suit, Iron Shell) are
-- real content, but as Puppet Master puppet-chassis upgrade names, not
-- armor: they're ALL-CAPS named sub-entries inside the Wood/Platinum Tier
-- option text of class_options 'class/puppet-master/option/
-- armorers-upgrades/{wood,platinum}-tier' (Armorer's Upgrades, Purple
-- Technique - Juggernaut subclass), already parsed and displayed correctly
-- there by cmd/n5e/format.go's capsEntryPattern. No new row needed for
-- them — just dropped from equipment, where they never belonged.
DELETE FROM equipment WHERE slug IN (
    'armor/chakra-skin',
    'armor/chakra-skin-b-rank',
    'armor/chakra-skin-s-rank',
    'armor/dust-coat',
    'armor/dust-coat-b-rank',
    'armor/dust-coat-s-rank',
    'armor/geo-breastplate-constitution',
    'armor/geo-breastplate-intelligence',
    'armor/martial-defense',
    'armor/skin',
    'armor/weaved-mail',
    'armor/steel-fortress',
    'armor/wooden-suit',
    'armor/iron-shell'
);
