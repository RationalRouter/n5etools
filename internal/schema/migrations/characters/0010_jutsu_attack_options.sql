-- Per-jutsu attack and damage overrides, the same idea 0009 gave equipped
-- weapons and custom attacks.
--
-- A known jutsu's row on the sheet was the last one you could not touch. Its
-- to-hit came from whichever of Ninjutsu/Genjutsu/Taijutsu/Bukijutsu the
-- description names (jutsuAttackKind), and its damage column said "see card"
-- and nothing else — because the book prints jutsu damage inside prose, in a
-- dozen different shapes ("2d6 + your Ninjutsu modifier", "1d10 per rank
-- above"), and none of that is stored as dice anywhere. The workaround was to
-- write a whole second row as a custom attack and read past the real one.
--
-- So the real row becomes editable instead. Every column is an override:
-- absent means "keep what the description implies", which for damage means no
-- damage roll at all. That is why the damage columns are nullable rather than
-- defaulted — same distinction 0007_custom_attacks.sql draws, where
-- damage_count = 0 means "no damage roll" and not "zero dice".
--
-- attack_prof defaults to 'full' because the jutsu attack modifiers this
-- overrides already include the full proficiency bonus (see
-- charsheet.AttackKinds), so an override that only changes the ability should
-- not silently halve the number as a side effect.
--
-- Keyed by jutsu slug rather than by a character_jutsu row id: the same jutsu
-- cannot be known twice, and keying by slug means forgetting a jutsu and
-- learning it again keeps whatever the player had tuned. The delete of the
-- override itself is left to the handler that forgets the jutsu, since there
-- is no character_jutsu row id to cascade from.
CREATE TABLE character_jutsu_options (
    character_id   INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    jutsu_slug     TEXT NOT NULL,
    attack_ability TEXT NOT NULL DEFAULT '',      -- '' = the kind the description names
    attack_prof    TEXT NOT NULL DEFAULT 'full',  -- 'none' | 'half' | 'full'
    attack_bonus   INTEGER NOT NULL DEFAULT 0,
    damage_count   INTEGER,                       -- NULL = no damage roll
    damage_sides   INTEGER,
    damage_ability TEXT NOT NULL DEFAULT '',
    damage_bonus   INTEGER NOT NULL DEFAULT 0,
    damage_type    TEXT,
    PRIMARY KEY (character_id, jutsu_slug)
);
