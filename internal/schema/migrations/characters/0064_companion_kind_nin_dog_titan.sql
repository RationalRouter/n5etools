-- Widens character_companions' kind CHECK to add 'nin-dog' and 'titan'
-- alongside the original 'puppet'/'summon'/'custom' three, so the Core
-- tab's "+ Add Companion" dropdown can offer them as real, storable kinds.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- create-copy-drop-rename pattern 0046/0050/0051/0055 already use.
CREATE TABLE character_companions_new (
    id                INTEGER PRIMARY KEY,
    character_id      INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('puppet', 'summon', 'custom', 'nin-dog', 'titan')),
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
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO character_companions_new
    (id, character_id, kind, name, summon_tribe_slug,
     ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
     attacks, traits, notes, armor_chassis, is_armor_form, size,
     matryoshka_group_id, matryoshka_jutsu_slots,
     sort_order, created_at, updated_at)
SELECT
    id, character_id, kind, name, summon_tribe_slug,
    ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
    attacks, traits, notes, armor_chassis, is_armor_form, size,
    matryoshka_group_id, matryoshka_jutsu_slots,
    sort_order, created_at, updated_at
FROM character_companions;

DROP TABLE character_companions;
ALTER TABLE character_companions_new RENAME TO character_companions;

CREATE INDEX idx_character_companions_character ON character_companions(character_id);
