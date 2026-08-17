-- Puppet Master's Purple Technique (Juggernaut) subclass feature "Armor
-- Chassis" reads "Select one of the following Armor Chassis, listed on the
-- following page" — that page (Orochimaru's Observation Compendium p.162)
-- is a bordered image table (Armor Name/Description/Armor Type/AC/Dex
-- Bonus/Bulk/Armor Properties), never extractable as PDF text and so never
-- ingested anywhere in this database. Hand-transcribed directly from the
-- rendered page image, same precedent as internal/charsheet/
-- skilldescriptions.go's page 51-54 skill blurbs — the PDFs remain the
-- permanent source of truth even where the text layer can't reach them.
--
-- AC interpretation confirmed from the Armor Type column matching the core
-- book's own light/medium/heavy conventions: total AC = 10 + ac_bonus +
-- Dexterity modifier, capped per dex_bonus_mode ('full' = light armor's
-- uncapped Dex, 'max2' = medium armor's +2 cap, 'none' = heavy armor's flat
-- AC). Weaved Mail's own Armor Type column is blank ("-") in the book —
-- it behaves like no armor worn at all.
CREATE TABLE puppet_armor_chassis (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    armor_type       TEXT NOT NULL,             -- '', 'Light', 'Medium', 'Heavy'
    ac_bonus         INTEGER NOT NULL,
    dex_bonus_mode   TEXT NOT NULL CHECK (dex_bonus_mode IN ('full','max2','none')),
    bulk             INTEGER NOT NULL,
    properties_text  TEXT NOT NULL,              -- as printed, e.g. 'Fashionable, Reinforced (4), Sturdy'
    sort_order       INTEGER NOT NULL,

    -- source_book is plain text, not an FK — same reasoning as
    -- 0012_weapon_properties.sql's own source_book column: this is a
    -- static hand-seeded migration, not something a LoadX() ingest call
    -- ever upserts, so there's no ingest run guaranteed to have already
    -- inserted a matching source_books row first.
    source_book      TEXT,
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

INSERT INTO puppet_armor_chassis (slug, name, description, armor_type, ac_bonus, dex_bonus_mode, bulk, properties_text, sort_order, source_book, source_version, source_page) VALUES
('puppet-armor-chassis/weaved-mail', 'Weaved Mail',
 'A unique chassis that takes the appearance of normal clothing. Like a set of wearable chakra strings, this armor is able to conduct your chakra to aid your survivability and make your defense neigh undetectable.',
 '', 0, 'full', 0, 'Smart, Mobile', 1, 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis/wooden-suit', 'Wooden Suit',
 'A simple yet elegantly designed set of smooth wooden armor with various supports to aid in maneuverability as well as defense.',
 'Light', 3, 'full', 3, 'Athletic, Mobile', 2, 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis/iron-shell', 'Iron Shell',
 'This armor is formed of multiple segments of iron, which grants you access to an armor set that is both sturdy and dependable, but not egregiously heavy.',
 'Medium', 5, 'max2', 6, 'Fashionable, Reinforced (4), Sturdy', 3, 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis/steel-fortress', 'Steel Fortress',
 'A favorite of Sasori of the Red Sand. This Armor chassis is extremely large and durable, providing the most protection thanks to its Steel lining.',
 'Heavy', 8, 'none', 8, 'Bulky, Powerful Build, Reinforced (7), Threatening', 4, 'book/class-compendium', '3.12', 162);

-- The 5 "Unique Armor Properties" from the same page — puppet-specific
-- definitions, distinct from (and not a duplicate of) the core book's own
-- Chapter 5 armor_properties catalog (Bulky/Fashionable/Reinforced/
-- Threatening above already resolve there; only these 5 are new).
CREATE TABLE puppet_armor_chassis_property (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,

    source_book      TEXT,
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

INSERT INTO puppet_armor_chassis_property (slug, name, description, source_book, source_version, source_page) VALUES
('puppet-armor-chassis-property/athletic', 'Athletic',
 'Armor with the Athletic property grants a +1d4 bonus to Acrobatics.',
 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis-property/mobile', 'Mobile',
 'Armor with the Mobile property grants a +5 bonus to movement speed and a +1 bonus to Dexterity skill checks and saving throws.',
 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis-property/powerful-build', 'Powerful Build',
 'Armor with the Powerful Build property causes the wearer to count as one size larger when determining the carrying capacity and weight they can push, drag, or lift. Increase your maximum bulk by +10. Lastly, increase your Strength Score by +2, up to the maximum of 22.',
 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis-property/smart', 'Smart',
 'Armor with the Smart property enables the wearer to calculate their AC using the Smart AC calculation: Smart AC: 10 + your Dexterity Modifier (Min. 1) + your Intelligence Modifier (Min. 1) + half your Proficiency Bonus (rounded down).',
 'book/class-compendium', '3.12', 162),
('puppet-armor-chassis-property/sturdy-renamed', 'Sturdy (Renamed)',
 'Armor with the Sturdy property enhances Reactions that would provide its wearer with damage reduction or temporary hit points. With such Reactions, add twice this armor''s Reinforced DR value to the damage reduced or temporary hit points gained. This property does not take effect if the armor''s Reinforced DR value would already reduce the damage being received.',
 'book/class-compendium', '3.12', 162);
