-- Chakra Enhanced Retrofit (Class: Puppet Master/3rd Level) — "Your Puppet
-- Tool gains a number of seal slots equal to 2 + your Proficiency Bonus.
-- You can place both Armor and Weapon seals on your puppet." A companion's
-- own equipped seals, one row per seal (kind='enhancement_seal' in
-- rules.db's equipment table — same cross-DB slug-reference tolerance
-- character_companion_upgrade_choices already uses for choice_slug), not
-- tied to any Puppet Upgrade pick — this is a fixed class feature every
-- Puppet Master gets at level 3, unlike Battle Ready Armor's own Silver
-- Tier upgrade sub-choice (which stays armor-only and upgrade-gated,
-- unrelated to this table).
CREATE TABLE character_companion_seals (
    id            INTEGER PRIMARY KEY,
    companion_id  INTEGER NOT NULL REFERENCES character_companions(id) ON DELETE CASCADE,
    seal_slug     TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_character_companion_seals_companion ON character_companion_seals(companion_id);
