-- Custom resource trackers: the second spendable point pool several
-- classes/clans layer on top of the shared HP/Chakra pair (Science-Nin's
-- Chakra Containment Device, Hatake's White Chakra, Hoshi's Star Chakra,
-- and others — see cmd/n5e/custom_resources.go's customResourceGrants for
-- the full curated list). One character can carry more than one of these
-- at once (e.g. a Hatake Science-Nin has both White Chakra and CCD), so
-- this is a rows-per-character-per-resource table, same shape as
-- character_companions, rather than character_concentration's
-- single-row-per-character shape.
CREATE TABLE character_custom_resources (
    character_id  INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    resource_key  TEXT NOT NULL,
    current       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (character_id, resource_key)
);

-- Puppet Master's Purple Technique lets a Puppet Tool transform into
-- Juggernaut Armor built around a specific Armor Chassis. The chassis stat
-- blocks themselves live in prose ("Select one of the following Armor
-- Chassis, listed on the following page") that was never ingested as
-- structured rules-database rows, so this is a free-text field, same
-- "customization over automation" boundary the companion sheet already
-- uses for Attacks/Traits/Notes. is_armor_form reflects whether the puppet
-- is currently worn as armor (it stops being a commandable creature while
-- so worn).
ALTER TABLE character_companions ADD COLUMN armor_chassis TEXT NOT NULL DEFAULT '';
ALTER TABLE character_companions ADD COLUMN is_armor_form INTEGER NOT NULL DEFAULT 0;
