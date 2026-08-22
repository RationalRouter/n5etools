-- Tracks Shinobi-Ware's Full-Metal Shinobi damage-resistance picks: "At
-- Level 6 you choose to gain resistance to either Bludgeoning, Piercing, or
-- Slashing damage. At Level 9 you can choose another. At Level 14 you gain
-- the last that you did not choose." Same one-row-per-independent-slot shape
-- as character_elemental_affinities (0024_elemental_affinities.sql), sized
-- to this feature's own two player picks ('sixth-level', 'ninth-level') — the
-- third, automatic-at-14th damage type needs no storage, computed fresh as
-- "whichever of the three isn't already in these two rows."
CREATE TABLE character_full_metal_shinobi_resistances (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    slot_key     TEXT NOT NULL,
    damage_type  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (character_id, slot_key)
);
