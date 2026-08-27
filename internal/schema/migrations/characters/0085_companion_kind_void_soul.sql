-- Widens character_companions' kind CHECK to add 'void-soul' (Trickster
-- Scout's own 3rd-level Void Soul Awakening — class/scout-nin/group/
-- scouting-technique/trickster-scout/feature/void-soul-awakening), and adds
-- the one genuinely new per-companion concept that feature needs and no
-- prior companion kind does: a summon/dismiss (active/inactive) toggle. See
-- cmd/n5e/void_soul.go's own header comment for the full design writeup —
-- in short, a "void-soul" companion reuses the ability-score columns to
-- store the Void Soul's own ABILITY MODIFIERS (score = 10 + 2*modifier, so
-- charstore/charsheet's existing score->modifier math keeps working
-- unmodified) rather than true ability scores, since the book's own text
-- gives this construct modifiers only ("Your Void Soul does not have
-- Ability scores like a normal creature, but instead only Ability
-- Modifiers"), no separate stat card was ever ingested into rules.db for it
-- (confirmed by CLASS_AUDIT.md's own Group-3 entry), and there is no other
-- ingested stat block to compute AC/HP/Speed from — those three stay plain
-- player-entered fields, the same "no formula behind this kind" treatment
-- 'summon'/'custom' already get.
--
-- void_soul_is_summoned: "This chakra construct can be summoned as a Bonus
-- Action... You can dismiss your Void Soul as a Bonus Action on your turn
-- or when you fall unconscious. While summoned, you can calculate your AC
-- using your Charisma in place of your Dexterity for the duration." No
-- other companion kind has ever needed an active/inactive state (see
-- titan.go's own header doc on this exact gap) — this is that hook, a
-- plain player-toggled boolean with no action-economy enforcement of its
-- own, the same "visible toggle, player trusted to time it" shape
-- characters.kujaku_mode_active (migration 0084) already uses for an
-- identical "on for a bonus action, off for a bonus action, no duration
-- tracking" mechanic. internal/charsheet.Compute reads this column directly
-- (voidSoulSummoned) to gate the Charisma-for-Dexterity AC swap — the ONE
-- clause of this feature with an existing formula slot to land in; every
-- other "while summoned" clause (visible to CM-users, occupies your space,
-- re-summons if >120ft away) has no automation surface in this app and
-- stays reference text on the companion's own card.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- create-copy-drop-rename pattern 0064/0070 already use for this exact
-- table. Every column added by 0077/0079(separate table)/0080/0081/0082
-- since 0070's own rebuild must be carried forward, not just 0070's set.
CREATE TABLE character_companions_new (
    id                INTEGER PRIMARY KEY,
    character_id      INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('puppet', 'summon', 'custom', 'nin-dog', 'titan', 'snb', 'void-soul')),
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
     save_proficiencies, temp_hp, resistances, immunities, condition_immunities, is_demon_foe)
SELECT
    id, character_id, kind, name, summon_tribe_slug,
    ac, hp_current, hp_max, speed, fly_speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
    attacks, traits, notes, armor_chassis, is_armor_form, size,
    matryoshka_group_id, matryoshka_jutsu_slots,
    sort_order, created_at, updated_at,
    nin_dog_breed, jutsu_slots_current, jutsu_slots_max,
    titan_specialization, barrier_current, barrier_max,
    save_proficiencies, temp_hp, resistances, immunities, condition_immunities, is_demon_foe
FROM character_companions;

DROP TABLE character_companions;
ALTER TABLE character_companions_new RENAME TO character_companions;

CREATE INDEX idx_character_companions_character ON character_companions(character_id);
