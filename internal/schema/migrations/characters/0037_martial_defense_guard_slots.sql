-- Tracks a Taijutsu Specialist's Martial Defense guard-slot picks (class
-- feature "Martial Defense", 5th level: 2 guard slots, each infused with a
-- taijutsu technique meant to mimic a minor armor seal; a 3rd slot and
-- access to refined seals at 9th, a 4th and greater seals at 13th, a 5th
-- and Superior seals at 17th). Structurally the same shape as Purple
-- Technique's Battle Ready Armor (character_companion_upgrade_choices'
-- seal sub-picks) and Weapon Specialist's Weapon Focus
-- (character_weapon_focus) — a flat per-character pick list, not a
-- companion-scoped one, since this feature belongs to the character
-- directly, not a puppet.
--
-- Stores equipment.slug (an enhancement_seal row, seal_applies_to =
-- 'armor') from rules.db — no FK, same cross-DB slug-reference tolerance
-- every other rules.db-keyed table in this schema already uses. Infusing a
-- seal is gated to "over the course of a full rest" in the book; freely
-- re-editable here instead, same "trust the player" boundary Weapon
-- Focus/Mastery/Puppet Tactics/Martial Techniques already draw. UNIQUE on
-- (character_id, seal_slug): the same seal can't be infused into two guard
-- slots at once.
CREATE TABLE character_martial_defense_seals (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    seal_slug    TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, seal_slug)
);

CREATE INDEX idx_character_martial_defense_seals_character ON character_martial_defense_seals(character_id);
