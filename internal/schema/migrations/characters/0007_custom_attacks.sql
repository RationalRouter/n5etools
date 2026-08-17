-- Player-defined attack rows for the sheet's Attacks & Jutsu block.
--
-- Everything that block shows today is derived: an equipped weapon becomes
-- an attack, a known jutsu whose text calls for an attack roll gets a to-hit
-- button. That covers the printed cases and nothing else, and N5E is a
-- heavily homebrewed system: the game is highly customizable, and it is
-- impossible to fully automate attack and jutsu rolls the way one could with
-- standard D&D 5e. A class feature that changes a
-- weapon's damage type, a summon's attack, a GM-granted technique, a jutsu
-- cast at a higher rank: none of those can be inferred from the rules
-- database, and the sheet had nowhere to put them.
--
-- Kind is 'weapon' or 'jutsu' purely so the row renders in the matching
-- table; both kinds carry exactly the same numbers, because a jutsu attack
-- and a weapon attack roll identically (d20 + to-hit, then damage dice +
-- bonus). Splitting them into two tables would duplicate every column.
--
-- Damage is stored as its three parts rather than as a "2d6+3" string: the
-- roll buttons on this sheet are driven by data-sides/data-count/
-- data-modifier attributes that window.n5eRoll reads, so a string would only
-- have to be re-parsed on the way out. damage_count = 0 means the row has no
-- damage roll at all (a shove, a grapple, a control jutsu), which is why the
-- columns are nullable rather than defaulted.
CREATE TABLE character_custom_attacks (
    id            INTEGER PRIMARY KEY,
    character_id  INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL CHECK (kind IN ('weapon', 'jutsu')),
    name          TEXT NOT NULL,
    attack_bonus  INTEGER NOT NULL DEFAULT 0,
    damage_count  INTEGER,
    damage_sides  INTEGER,
    damage_bonus  INTEGER NOT NULL DEFAULT 0,
    damage_type   TEXT,
    notes         TEXT,
    sort_order    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_custom_attacks_character ON character_custom_attacks(character_id, kind, sort_order);
