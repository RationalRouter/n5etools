-- Widens character_companions' kind CHECK to add 'wow-whelp' and
-- 'wow-spectre' — the two auto-added Weapon-of-Wonder companions
-- (ensureWhelpCompanion/wow_whelp.go, ensureAngelEIPSpectreCompanion/
-- science_nin_subclasses.go), previously stored as kind='custom' and
-- identified only by an exact match against their own hardcoded English
-- display Name ("Whelp (Draconic Gauntlet)" / "Spectre (Angel E.I.P)").
-- That match broke once the Name was translated at creation for a non-
-- English locale, and would break just as easily for an English player who
-- simply renamed the companion. Giving each its own kind, the same way
-- 'void-soul' (migration 0085) already carved out a system-generated-only
-- kind with no entry in the player's own kind picker, makes the match
-- locale-independent and rename-proof: Name becomes a purely cosmetic,
-- freely player-editable field for these two, exactly like every other
-- companion kind already treats it.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- create-copy-drop-rename pattern 0064/0070/0085 already use for this exact
-- table. Every column added since 0085's own rebuild must be carried
-- forward, not just 0085's set.
CREATE TABLE character_companions_new (
    id                INTEGER PRIMARY KEY,
    character_id      INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('puppet', 'summon', 'custom', 'nin-dog', 'titan', 'snb', 'void-soul', 'wow-whelp', 'wow-spectre')),
    name              TEXT NOT NULL DEFAULT '',
    summon_tribe_slug TEXT NOT NULL DEFAULT '',

    ac                INTEGER,
    hp_current        INTEGER,
    hp_max            INTEGER,
    speed             INTEGER,
    fly_speed         INTEGER,
    str_score         INTEGER,
    dex_score         INTEGER,
    con_score         INTEGER,
    int_score         INTEGER,
    wis_score         INTEGER,
    cha_score         INTEGER,

    attacks           TEXT NOT NULL DEFAULT '',
    traits            TEXT NOT NULL DEFAULT '',
    notes             TEXT NOT NULL DEFAULT '',

    armor_chassis     TEXT NOT NULL DEFAULT '',
    is_armor_form     INTEGER NOT NULL DEFAULT 0,
    size              TEXT NOT NULL DEFAULT '',

    matryoshka_group_id     INTEGER,
    matryoshka_jutsu_slots  INTEGER NOT NULL DEFAULT 0,

    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now')),

    nin_dog_breed        TEXT NOT NULL DEFAULT '',
    jutsu_slots_current   INTEGER,
    jutsu_slots_max       INTEGER,
    titan_specialization TEXT NOT NULL DEFAULT '',
    barrier_current       INTEGER,
    barrier_max           INTEGER,

    save_proficiencies    TEXT NOT NULL DEFAULT '',
    temp_hp               INTEGER,
    resistances           TEXT NOT NULL DEFAULT '',
    immunities            TEXT NOT NULL DEFAULT '',
    condition_immunities  TEXT NOT NULL DEFAULT '',
    is_demon_foe          INTEGER NOT NULL DEFAULT 0,

    void_soul_is_summoned INTEGER NOT NULL DEFAULT 0
);

INSERT INTO character_companions_new
    (id, character_id, kind, name, summon_tribe_slug,
     ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
     attacks, traits, notes, armor_chassis, is_armor_form, size,
     matryoshka_group_id, matryoshka_jutsu_slots,
     sort_order, created_at, updated_at,
     nin_dog_breed, jutsu_slots_current, jutsu_slots_max,
     titan_specialization, barrier_current, barrier_max,
     save_proficiencies, temp_hp, resistances, immunities, condition_immunities, is_demon_foe,
     void_soul_is_summoned)
SELECT
    id, character_id,
    CASE
        WHEN kind = 'custom' AND name = 'Whelp (Draconic Gauntlet)' THEN 'wow-whelp'
        WHEN kind = 'custom' AND name = 'Spectre (Angel E.I.P)' THEN 'wow-spectre'
        ELSE kind
    END,
    name, summon_tribe_slug,
    ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
    attacks, traits, notes, armor_chassis, is_armor_form, size,
    matryoshka_group_id, matryoshka_jutsu_slots,
    sort_order, created_at, updated_at,
    nin_dog_breed, jutsu_slots_current, jutsu_slots_max,
    titan_specialization, barrier_current, barrier_max,
    save_proficiencies, temp_hp, resistances, immunities, condition_immunities, is_demon_foe,
    void_soul_is_summoned
FROM character_companions;

DROP TABLE character_companions;
ALTER TABLE character_companions_new RENAME TO character_companions;

CREATE INDEX idx_character_companions_character ON character_companions(character_id);
