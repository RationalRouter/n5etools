-- Additions discovered while parsing the clan compendium (v3.11).

-- Clan-specific named grants from a TRAITS block that are not one of the
-- standard labels: "Parasitic Technique", "Lightning Literacy",
-- "Unremarkable Legacy [New!]", ... Always-on, granted at clan selection.
-- Rebuilt from the parse on every ingest (no override columns here; clan-
-- level notes/status live on the clans row).
CREATE TABLE clan_traits (
    clan_slug   TEXT NOT NULL REFERENCES clans(slug),
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    sort_order  INTEGER NOT NULL,
    PRIMARY KEY (clan_slug, name)
);

-- The ability-increase line exactly as printed. Fixed increases also land in
-- clan_ability_increases; choice-based ones ("+2 Wis or Int", "+2 to any
-- Ability Score") exist ONLY here — the creation flow reads this text and
-- records the player's actual picks in the character database
-- (character_ability_bonuses), so nothing structured is invented.
ALTER TABLE clans ADD COLUMN ability_increase_text TEXT;

-- Bloodline Latent abilities are bought with Bloodline Points; the tables in
-- the book price each ability.
ALTER TABLE bloodline_latents ADD COLUMN point_cost INTEGER;
