-- Custom items marked "Other rollable" at creation (see custom_items.
-- rollable_kind, 0013_custom_items.sql) get a character_custom_attacks row
-- of their own, same flat attack/damage-bonus mechanism a hand-written
-- custom attack already uses — no new override table needed. That table's
-- kind CHECK only allowed 'weapon'/'jutsu' (the two existing sections of
-- the Attacks & Jutsu box); 'item' is a third section, "Other Rollables",
-- for rollables that are neither.
--
-- SQLite can't ALTER a CHECK constraint in place, so this rebuilds the
-- table: new table with the widened constraint, copy every row across,
-- drop the old one, rename. The rebuilt table carries every column the
-- live table actually has today — the original CREATE TABLE from
-- 0007_custom_attacks.sql plus attack_ability/attack_prof/damage_ability,
-- added later by 0009_composed_attack_modifiers.sql's ALTER TABLE — not
-- just what 0007 first created.
CREATE TABLE character_custom_attacks_new (
    id             INTEGER PRIMARY KEY,
    character_id   INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('weapon', 'jutsu', 'item')),
    name           TEXT NOT NULL,
    attack_ability TEXT NOT NULL DEFAULT '',
    attack_prof    TEXT NOT NULL DEFAULT 'none',
    attack_bonus   INTEGER NOT NULL DEFAULT 0,
    damage_count   INTEGER,
    damage_sides   INTEGER,
    damage_ability TEXT NOT NULL DEFAULT '',
    damage_bonus   INTEGER NOT NULL DEFAULT 0,
    damage_type    TEXT,
    notes          TEXT,
    sort_order     INTEGER NOT NULL DEFAULT 0
);

INSERT INTO character_custom_attacks_new
    (id, character_id, kind, name, attack_ability, attack_prof, attack_bonus,
     damage_count, damage_sides, damage_ability, damage_bonus, damage_type, notes, sort_order)
SELECT id, character_id, kind, name, attack_ability, attack_prof, attack_bonus,
       damage_count, damage_sides, damage_ability, damage_bonus, damage_type, notes, sort_order
FROM character_custom_attacks;

DROP TABLE character_custom_attacks;
ALTER TABLE character_custom_attacks_new RENAME TO character_custom_attacks;

CREATE INDEX idx_custom_attacks_character ON character_custom_attacks(character_id, kind, sort_order);
