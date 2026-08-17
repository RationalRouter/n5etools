-- Structured, rollable attacks for a companion — replaces the puppet card's
-- old freeform "Attacks" textarea, same shape as character_custom_attacks
-- (0009) but with no "kind" column: a companion has one unified attack
-- list, not separate Weapons/Jutsu/Other Rollables tables to route between.
-- Also covers a puppet casting jutsu "under certain circumstances" — a row
-- is just a name, optional damage dice, and a description, so a
-- jutsu-shaped entry is added the same way any other custom attack is; real
-- jutsu-known/chakra-casting-economy tracking for companions is out of
-- scope (no chakra-pool model exists for a companion at all today).
CREATE TABLE character_companion_attacks (
    id             INTEGER PRIMARY KEY,
    companion_id   INTEGER NOT NULL REFERENCES character_companions(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',

    -- Ability keys ("str"/"dex"/...) resolve against the COMPANION's own
    -- ability scores, not the player character's — attack_prof resolves
    -- against the OWNING character's proficiency bonus instead (per the
    -- Puppet Tool baseline's own "Proficiency = Puppet Master's
    -- Proficiency" rule). See cmd/n5e/puppets.go's composeCompanionAttacks.
    attack_ability TEXT NOT NULL DEFAULT '',
    attack_prof    TEXT NOT NULL DEFAULT 'none',
    attack_bonus   INTEGER NOT NULL DEFAULT 0,

    damage_count   INTEGER,
    damage_sides   INTEGER,
    damage_ability TEXT NOT NULL DEFAULT '',
    damage_bonus   INTEGER NOT NULL DEFAULT 0,
    damage_type    TEXT NOT NULL DEFAULT '',

    sort_order     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_character_companion_attacks_companion ON character_companion_attacks(companion_id);
