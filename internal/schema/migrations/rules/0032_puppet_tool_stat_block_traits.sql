-- The Puppet Tool's sidebar stat card (see migration 0028) prints several
-- more lines below its ability scores: Armor Class, Saving Throws, Damage
-- Resistance, Damage Immunity, Condition Immunities, and Weapon
-- Proficiencies (confirmed against the book's page 141 image directly —
-- "Armor Class 13 + your Proficiency Bonus (Natural Armor)", "Saving
-- Throws Proficient in All (Treat negative modifiers as +0)", "Damage
-- Immunity Psychic, Poison", "Damage Resistance Acid, Chakra, Necrotic",
-- "Condition Immunities All Mental, Bleeding, Exhaustion, Poisoned",
-- "Weapon Proficiencies Always the same as yours."). Confirmed these never
-- reached migration 0028's row at all — unlike the rest of the card, this
-- text is not present anywhere in the PDF's own linear text stream (checked
-- directly with pdftotext), so it isn't a parser gap internal/parse/classes.go
-- can fix with a better regex; there is nothing in the extracted text for a
-- regex to match. These columns are therefore populated ONLY by this
-- migration, never by internal/store/classes.go's upsertPuppetToolStatBlock
-- (which has no fields for them and will never touch them) — a real
-- re-ingest cannot reproduce or overwrite this data either way.
ALTER TABLE puppet_tool_stat_block ADD COLUMN ac_base INTEGER;
ALTER TABLE puppet_tool_stat_block ADD COLUMN saving_throws_text TEXT;
ALTER TABLE puppet_tool_stat_block ADD COLUMN damage_resistance_text TEXT;
ALTER TABLE puppet_tool_stat_block ADD COLUMN damage_immunity_text TEXT;
ALTER TABLE puppet_tool_stat_block ADD COLUMN condition_immunity_text TEXT;
ALTER TABLE puppet_tool_stat_block ADD COLUMN weapon_proficiency_text TEXT;

UPDATE puppet_tool_stat_block
SET ac_base = 13,
    saving_throws_text = 'Proficient in All (Treat negative modifiers as +0)',
    damage_resistance_text = 'Acid, Chakra, Necrotic',
    damage_immunity_text = 'Psychic, Poison',
    condition_immunity_text = 'All Mental, Bleeding, Exhaustion, Poisoned',
    weapon_proficiency_text = 'Always the same as yours.'
WHERE class_slug = 'class/puppet-master';
