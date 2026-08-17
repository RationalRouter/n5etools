-- Armor property reference glossary, mirroring 0012_weapon_properties.sql's
-- shape exactly (a dedicated table + its own many-to-many join, not a
-- polymorphic "equipment_property" concept shared with weapons) — this
-- codebase consistently prefers one dedicated table per real-world concept
-- over a shared/discriminated one (see clan_traits/clan_features/
-- bloodline_latents), and a shared equipment_properties.property_slug column
-- can't cleanly carry a foreign key into two different tables anyway.
--
-- Rules text hand-transcribed from Naruto 5e - Full Document.pdf (book/core
-- v3.11), the "Armor Properties" section, page 25 — not auto-parsed
-- (detection_status 'manual', same convention as 0012's weapon properties).
--
-- Deliberately NOT linked to any of the 32 existing armor rows in
-- `equipment` yet: which named property applies to which specific armor is
-- only printed in the book's Armor table, and that table is a pure image
-- (same already-tracked gap as armor cost/weight — see the Mastersheet-
-- bootstrap/OCR-debt note in project history). equipment_armor_properties
-- ships empty; populating it is the same future OCR pass that resolves the
-- cost/weight gap, not a new debt item.
CREATE TABLE armor_properties (
    slug             TEXT PRIMARY KEY,          -- 'armor-property/bulky'
    name             TEXT NOT NULL,             -- 'Bulky'
    description      TEXT NOT NULL,

    source_book      TEXT,                      -- see 0012's comment: no FK,
    source_version   TEXT,                      -- this is a hand-seeded
    source_page      INTEGER,                   -- migration, not an ingest
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

CREATE TABLE equipment_armor_properties (
    equipment_slug TEXT NOT NULL REFERENCES equipment(slug),
    property_slug  TEXT NOT NULL REFERENCES armor_properties(slug),
    detail         TEXT,                        -- e.g. Heavyweight's Str requirement, Reinforced's DR value
    PRIMARY KEY (equipment_slug, property_slug)
);
CREATE INDEX idx_equipment_armor_properties_equipment ON equipment_armor_properties(equipment_slug);

INSERT INTO armor_properties (slug, name, description, source_book, source_version, source_page) VALUES
('armor-property/bulky', 'Bulky', 'Armor with the Bulky property grants disadvantage on Stealth checks.', 'book/core', '3.11', 25),
('armor-property/bulwark', 'Bulwark', 'Armor with the Bulwark property gains a +2 bonus to saving throws and checks made to resist being moved against your will.', 'book/core', '3.11', 25),
('armor-property/camouflage', 'Camouflage', 'Armor with the Camouflage property grants a +2 bonus to Stealth checks at night.', 'book/core', '3.11', 25),
('armor-property/fashionable', 'Fashionable', 'Armor with the Fashionable property grants a +2 bonus to Persuasion checks against non-hostile creatures.', 'book/core', '3.11', 25),
('armor-property/fortified', 'Fortified', 'Armor with the Fortified property treats the first critical hit against you per rest as a normal hit.', 'book/core', '3.11', 25),
('armor-property/heavyweight', 'Heavyweight', 'Armor with the Heavyweight property has a Strength ability score requirement. The Strength requirement is listed with this property.', 'book/core', '3.11', 25),
('armor-property/high-quality', 'High Quality', 'Armor with the High Quality property gains +1 Seal Slot for Armor Seals.', 'book/core', '3.11', 25),
('armor-property/lightweight', 'Lightweight', 'Armor with the Lightweight property reduces its Bulk by half.', 'book/core', '3.11', 25),
('armor-property/reinforced', 'Reinforced', 'Armor with the Reinforced property gains damage reduction against Bludgeoning, Piercing, and Slashing damage. The DR value is listed with this property.', 'book/core', '3.11', 25),
('armor-property/threatening', 'Threatening', 'Armor with the Threatening property grants a +2 bonus to Intimidation checks.', 'book/core', '3.11', 25);
