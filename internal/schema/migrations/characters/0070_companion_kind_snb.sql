-- Widens character_companions' kind CHECK to add 'snb' (Science-Nin S.N.B
-- Specialist's Scientific Ninja Beast) alongside the five kinds 0064
-- already added, treated the same bare/generic way as 'custom' (structured
-- Attacks, plain stat fields, no dedicated reference panel or computed
-- stat block -- see companions.go's own doc on companionStructuredAttackKinds
-- for why a bare kind still gets rollable Attacks).
--
-- SQLite can't ALTER a CHECK constraint in place -- rebuild the table, same
-- create-copy-drop-rename pattern 0046/0050/0051/0055/0064 already use.
-- Every column here (including nin_dog_breed/jutsu_slots_*/
-- titan_specialization/barrier_* added afterward by 0065/0066's plain
-- ADD COLUMN statements) must be carried forward, not just 0064's own set.
CREATE TABLE character_companions_new (
    id                INTEGER PRIMARY KEY,
    character_id      INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('puppet', 'summon', 'custom', 'nin-dog', 'titan', 'snb')),
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
    barrier_max           INTEGER
);

INSERT INTO character_companions_new
    (id, character_id, kind, name, summon_tribe_slug,
     ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
     attacks, traits, notes, armor_chassis, is_armor_form, size,
     matryoshka_group_id, matryoshka_jutsu_slots,
     sort_order, created_at, updated_at,
     nin_dog_breed, jutsu_slots_current, jutsu_slots_max,
     titan_specialization, barrier_current, barrier_max)
SELECT
    id, character_id, kind, name, summon_tribe_slug,
    ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
    attacks, traits, notes, armor_chassis, is_armor_form, size,
    matryoshka_group_id, matryoshka_jutsu_slots,
    sort_order, created_at, updated_at,
    nin_dog_breed, jutsu_slots_current, jutsu_slots_max,
    titan_specialization, barrier_current, barrier_max
FROM character_companions;

DROP TABLE character_companions;
ALTER TABLE character_companions_new RENAME TO character_companions;

CREATE INDEX idx_character_companions_character ON character_companions(character_id);
