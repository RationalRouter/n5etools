-- Links the 10 armor properties (seeded empty in 0016_armor_properties.sql,
-- deliberately not linked to any armor row at the time — "which named
-- property applies to which specific armor is only printed in the book's
-- Armor table, and that table is a pure image") to the 18 armor rows that
-- correspond to a real book price-table row (same 18 rows migration 0015
-- backfilled cost_ryo for). Read directly off the rendered PDF page image
-- (Naruto 5e - Full Document.pdf, PDF page 28 / printed page 26 — the
-- Armor Properties + Armor table page), the same page 0015 already
-- identified and cross-checked for cost — this time capturing its
-- "Armor Property" column too, which that earlier pass didn't need.
--
-- Also backfills equipment.strength_req for the 5 armors carrying
-- Heavyweight: the property's own rules text says "Armor with the
-- Heavyweight property has a Strength Ability score requirement. The
-- Strength requirement is listed with this property" — so the number in
-- Heavyweight's detail *is* strength_req, a real pre-existing NULL gap on
-- these rows (confirmed via a direct query before writing this migration),
-- not a new column.
--
-- The other 14 armor rows in `equipment` (Chakra Skin and its (B-Rank)/
-- (S-Rank) variants, Dust Coat, etc.) are Mastersheet-sourced clan/class-
-- specific armors with no corresponding row in this printed table at all —
-- left unlinked, same as their cost_ryo/weight_lb gap already is.
--
-- equipment_armor_properties.equipment_slug carried a real FK to
-- equipment(slug) (0016_armor_properties.sql) — harmless while the table
-- shipped empty, but this migration is the first to actually INSERT rows
-- into it, and (same class of problem as 0017's source_books fix)
-- equipment is populated by n5e-ingest's own sheet/weapon/armor parsers,
-- never by a migration, so a from-empty schema apply (schema_test.go) has
-- no armor rows yet to satisfy that FK. Dropping and recreating the table
-- without that one FK is safe here specifically because nothing else in
-- the schema references equipment_armor_properties by name (unlike the
-- equipment table itself, recreating which would risk the FK-rewrite
-- hazard documented in 0017) — confirmed empirically before writing this.
DROP TABLE equipment_armor_properties;
CREATE TABLE equipment_armor_properties (
    equipment_slug TEXT NOT NULL,
    property_slug  TEXT NOT NULL REFERENCES armor_properties(slug),
    detail         TEXT,
    PRIMARY KEY (equipment_slug, property_slug)
);
CREATE INDEX idx_equipment_armor_properties_equipment ON equipment_armor_properties(equipment_slug);

INSERT INTO equipment_armor_properties (equipment_slug, property_slug, detail) VALUES
-- Light Armor
('armor/leather-weave', 'armor-property/camouflage', NULL),
('armor/armored-cloth', 'armor-property/fortified', NULL),
('armor/reinforced-cloth', 'armor-property/reinforced', '2'),
('armor/synthetic-weave', 'armor-property/fashionable', NULL),
('armor/shinobi-weave', 'armor-property/high-quality', NULL),
-- Medium Armor
('armor/combat-jacket', 'armor-property/bulky', NULL),
('armor/shinobi-jacket', 'armor-property/fashionable', NULL),
('armor/shinobi-jacket', 'armor-property/reinforced', '2'),
('armor/shinobi-combat-jacket', 'armor-property/camouflage', NULL),
('armor/shinobi-combat-jacket', 'armor-property/reinforced', '3'),
('armor/chunin-jacket', 'armor-property/fortified', NULL),
('armor/chunin-jacket', 'armor-property/reinforced', '3'),
('armor/battle-coat', 'armor-property/high-quality', NULL),
('armor/battle-coat', 'armor-property/heavyweight', '12'),
('armor/battle-coat', 'armor-property/reinforced', '4'),
('armor/armored-chunin-coat', 'armor-property/fortified', NULL),
('armor/armored-chunin-coat', 'armor-property/heavyweight', '14'),
('armor/armored-chunin-coat', 'armor-property/reinforced', '4'),
-- Heavy Armor
('armor/combat-armor', 'armor-property/bulky', NULL),
('armor/combat-armor', 'armor-property/reinforced', '4'),
('armor/synthetic-armor', 'armor-property/high-quality', NULL),
('armor/synthetic-armor', 'armor-property/reinforced', '4'),
('armor/jonin-armor', 'armor-property/threatening', NULL),
('armor/jonin-armor', 'armor-property/heavyweight', '14'),
('armor/jonin-armor', 'armor-property/reinforced', '6'),
('armor/elite-jonin-armor', 'armor-property/threatening', NULL),
('armor/elite-jonin-armor', 'armor-property/high-quality', NULL),
('armor/elite-jonin-armor', 'armor-property/reinforced', '6'),
('armor/ronin-armor', 'armor-property/lightweight', NULL),
('armor/ronin-armor', 'armor-property/fortified', NULL),
('armor/ronin-armor', 'armor-property/heavyweight', '16'),
('armor/ronin-armor', 'armor-property/reinforced', '8'),
('armor/samurai-armor', 'armor-property/high-quality', NULL),
('armor/samurai-armor', 'armor-property/fortified', NULL),
('armor/samurai-armor', 'armor-property/heavyweight', '18'),
('armor/samurai-armor', 'armor-property/reinforced', '8');
-- Padded Cloth has no property at all (book lists "-") — no row needed.

UPDATE equipment SET strength_req = 12, detection_status = 'verified' WHERE slug = 'armor/battle-coat';
UPDATE equipment SET strength_req = 14, detection_status = 'verified' WHERE slug = 'armor/armored-chunin-coat';
UPDATE equipment SET strength_req = 14, detection_status = 'verified' WHERE slug = 'armor/jonin-armor';
UPDATE equipment SET strength_req = 16, detection_status = 'verified' WHERE slug = 'armor/ronin-armor';
UPDATE equipment SET strength_req = 18, detection_status = 'verified' WHERE slug = 'armor/samurai-armor';
