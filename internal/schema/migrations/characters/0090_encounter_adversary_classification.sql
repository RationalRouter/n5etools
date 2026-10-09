-- Adds the Bingo Book's Tenacity/Optional Classification/Transformation-or-
-- Battlefield-Domain data (internal/encounter's ApplyClassification/
-- WithNarrativeAttachment) to a saved encounter adversary's snapshot.
--
-- transformation_slug/battlefield_domain_slug are cross-database references
-- into rules.db's adversary_transformation_examples/adversary_battlefield_
-- domains tables, same no-real-FK convention as rank_slug/role_slug in
-- 0089_encounters.sql -- name is snapshotted alongside the slug for the same
-- reason. Mutually exclusive per the book (p.21): at most one of the two
-- slug columns is ever non-empty, enforced application-side in
-- WithNarrativeAttachment rather than with a CHECK constraint, since a CHECK
-- can't distinguish "empty because unset" from "empty because not this rank"
-- any more cleanly than the application already does.
ALTER TABLE encounter_adversaries ADD COLUMN tenacity_max INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN classification TEXT NOT NULL DEFAULT '' CHECK (classification IN ('','iconic','epic'));
ALTER TABLE encounter_adversaries ADD COLUMN classification_hp_chakra_modifier INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN classification_ability_points INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN transformation_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE encounter_adversaries ADD COLUMN transformation_name TEXT NOT NULL DEFAULT '';
ALTER TABLE encounter_adversaries ADD COLUMN battlefield_domain_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE encounter_adversaries ADD COLUMN battlefield_domain_name TEXT NOT NULL DEFAULT '';
