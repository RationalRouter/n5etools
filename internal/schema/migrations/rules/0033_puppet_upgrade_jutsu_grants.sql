-- Three Puppet Master upgrades each print a small table granting specific
-- named jutsu at set class levels (Elemental Reactor, Weaponized Jutsu
-- Casting, Entangling Threads) — none of the three tables survived PDF text
-- extraction (same "image-table" data loss as Armor Chassis), so all rows
-- here are hand-transcribed directly from the source PDF pages and
-- hand-verified against the jutsu table (every jutsu below was matched by
-- name, and each upgrade's own rank-per-level progression comes out clean
-- D/C/B/A against highestRankForLevel's bands, which would not happen by
-- coincidence if a name were mistyped or misattributed).
--
-- variant distinguishes a sub-choice-gated upgrade (Elemental Reactor:
-- which element; Weaponized Jutsu Casting: which Puppet Weapon Type) from
-- Entangling Threads' single fixed list (variant NULL). Elemental Reactor's
-- variant values match elementReactorOptions()'s own Slug (the element's
-- plain name); Weaponized Jutsu Casting's match the Puppet Weapon Type
-- class_options row's own leaf slug (drone-weapon/ogre-weapon/
-- sentinel-weapon), since a companion's weapon type is that pick itself,
-- not a separate sub-choice.
-- upgrade_entry_slug/jutsu_slug are plain text, not FKs — same reasoning as
-- puppet_armor_chassis's own source_book column (0031): this is a static
-- hand-seeded migration inserted at schema-apply time, before any real
-- class_option_entries/jutsu content has been ingested into a fresh
-- database, so an FK would fail the very migration that creates it. Every
-- slug below was hand-verified against a live rules.db instead (see this
-- file's own header comment).
CREATE TABLE puppet_upgrade_jutsu_grants (
    upgrade_entry_slug TEXT NOT NULL,
    variant             TEXT,
    level                INTEGER NOT NULL,
    jutsu_slug           TEXT NOT NULL,
    PRIMARY KEY (upgrade_entry_slug, variant, level)
);

INSERT INTO puppet_upgrade_jutsu_grants (upgrade_entry_slug, variant, level, jutsu_slug) VALUES
-- Elemental Reactor (Purple Technique, Wood Tier) — PDF p.163
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Earth', 2,  'jutsu/earth-release-earthen-grasp'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Earth', 6,  'jutsu/earth-release-turning-palm'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Earth', 10, 'jutsu/earth-release-earth-style-wall'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Earth', 14, 'jutsu/earth-release-stone-needle'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Wind', 2,  'jutsu/wind-release-passing-typhoon'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Wind', 6,  'jutsu/wind-release-wall-of-wind'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Wind', 10, 'jutsu/wind-release-10-000-slicing-blades'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Wind', 14, 'jutsu/wind-release-drilling-wind-bullet'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Fire', 2,  'jutsu/fire-release-fox-fire'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Fire', 6,  'jutsu/fire-release-fire-dragon-bullet'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Fire', 10, 'jutsu/fire-release-fire-wall'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Fire', 14, 'jutsu/fire-release-great-fire-absorption'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Water', 2,  'jutsu/water-release-water-whip'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Water', 6,  'jutsu/water-release-wall-of-water'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Water', 10, 'jutsu/water-release-water-fang'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Water', 14, 'jutsu/water-release-falling-rain-needles'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Lightning', 2,  'jutsu/lightning-release-thunder-tempest'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Lightning', 6,  'jutsu/lightning-release-lightning-kings-mantle'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Lightning', 10, 'jutsu/lightning-release-lightning-spear'),
('class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor', 'Lightning', 14, 'jutsu/lightning-release-lightning-shield'),
-- Weaponized Jutsu Casting (Blue Technique, Upgrades of War, Wood Tier) — PDF p.151
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'drone-weapon', 2,  'jutsu/prepared-shot'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'drone-weapon', 6,  'jutsu/sealing-art-mark-of-finding'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'drone-weapon', 10, 'jutsu/kaguras-mind-eye'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'drone-weapon', 14, 'jutsu/medical-release-aura-of-power'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'ogre-weapon', 2,  'jutsu/dempsey-roll'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'ogre-weapon', 6,  'jutsu/beast'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'ogre-weapon', 10, 'jutsu/breaker-fist'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'ogre-weapon', 14, 'jutsu/world-breaker'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'sentinel-weapon', 2,  'jutsu/weapon-deflect'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'sentinel-weapon', 6,  'jutsu/guardian-knight'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'sentinel-weapon', 10, 'jutsu/ichimonji'),
('class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting', 'sentinel-weapon', 14, 'jutsu/shinobi-cross'),
-- Entangling Threads (White Technique, Interwoven Upgrades, Wood Tier) — PDF p.176
('class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads', NULL, 1,  'jutsu/shadow-snake-bite'),
('class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads', NULL, 5,  'jutsu/hair-binding-technique'),
('class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads', NULL, 9,  'jutsu/medical-release-body-pathway-derangement'),
('class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads', NULL, 13, 'jutsu/sealing-art-forcecage');
