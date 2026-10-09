-- Adversary Builder charts (Bingo Book p.6, "Build Your Adversary").
--
-- The Bingo Book's Hit/Chakra Point formula (p.8) depends on two tables the
-- PDF never prints: each Rank's Hit/Chakra Die and the "Base Adversary
-- Statistics by Level". Both, plus the role, clan and automatic-bonus charts
-- the build steps use, come from the community Adversary Builder sheet's
-- ChartsMain tab (Book Maths "V3.11 Bingo Book"). They are seeded here as
-- data so the app never reads the spreadsheet; no sourcebook re-ingest is
-- needed. The sheet's own scratch rows ("Testing God", "Test") are omitted.
--
-- Class-level modifiers (Minion/Standard/Elite/Solo) follow Bingo Book
-- Pack 1 where it disagrees with the sheet; adversary_class_hp_mods holds
-- Pack 1's Hit/Chakra Point Modifier as numbers so adversary_ranks.hp_formula
-- (printed text) never has to be parsed. It is a separate table rather than
-- new adversary_ranks columns because the Pack 1 ingest inserts those rows
-- after migrations run on a fresh database.

-- Base Adversary Statistics by Level, levels -3..30. standard_mods and
-- elite_mods are the six ability modifiers (highest first) the GM assigns to
-- abilities; Minion/Standard use standard_mods, Elite/Solo elite_mods.
-- base_hp/base_cp are only printed for levels <= 0. ac_reference is the
-- sheet's "10 + half/prof" column, kept for reference; the builder's AC
-- formula does not read it.
CREATE TABLE adversary_level_stats (
    level             INTEGER PRIMARY KEY,
    tier              INTEGER NOT NULL,
    ac_reference      INTEGER NOT NULL,
    prof              INTEGER NOT NULL,
    base_hp           INTEGER,
    base_cp           INTEGER,
    attack_bonus      INTEGER NOT NULL,
    damage_per_action INTEGER NOT NULL,
    standard_mods     TEXT NOT NULL,
    elite_mods        TEXT NOT NULL,
    xp                INTEGER NOT NULL
);

-- Number of traits an adversary of each Class has at a level.
CREATE TABLE adversary_trait_counts (
    level    INTEGER PRIMARY KEY,
    minion   INTEGER NOT NULL,
    standard INTEGER NOT NULL,
    elite    INTEGER NOT NULL,
    solo     INTEGER NOT NULL
);

-- Rank (E..S): the Rank Die for Hit and Chakra Points (size and average),
-- and the AC, to-hit, save DC and damage-per-action bonuses the Rank grants.
CREATE TABLE adversary_rank_tiers (
    rank            TEXT PRIMARY KEY CHECK (rank IN ('E','D','C','B','A','S')),
    hit_die         INTEGER NOT NULL,
    hit_avg         INTEGER NOT NULL,
    chakra_die      INTEGER NOT NULL,
    chakra_avg      INTEGER NOT NULL,
    defensive_bonus INTEGER NOT NULL,
    to_hit_bonus    INTEGER NOT NULL,
    save_dc_bonus   INTEGER NOT NULL,
    bonus_damage    INTEGER NOT NULL
);

-- The February 2026 Roles. hp_mod/cp_mod add into the Hit/Chakra Point
-- multiplier, damage_mult scales the level's damage per action.
-- trait_role_slug names the Pack 1 adversary_roles pool whose official
-- traits this role also draws from ('' when Pack 1 has no matching role).
CREATE TABLE adversary_builder_roles (
    slug            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    ac_mod          INTEGER NOT NULL,
    save_mod        INTEGER NOT NULL,
    hp_mod          REAL NOT NULL,
    cp_mod          REAL NOT NULL,
    damage_mult     REAL NOT NULL,
    speed_bonus     INTEGER NOT NULL,
    trait_role_slug TEXT NOT NULL DEFAULT '',
    sort_order      INTEGER NOT NULL
);

-- Per-clan Hit/Chakra Point multiplier additions and jutsu attack/DC
-- bonuses. clan_tag matches adversary_builder_sheet_traits.category_tag.
CREATE TABLE adversary_clan_mods (
    clan_tag     TEXT PRIMARY KEY,
    hp_mod       REAL NOT NULL,
    cp_mod       REAL NOT NULL,
    ninjutsu_atk INTEGER NOT NULL,
    ninjutsu_dc  INTEGER NOT NULL,
    genjutsu_atk INTEGER NOT NULL,
    genjutsu_dc  INTEGER NOT NULL,
    taijutsu_atk INTEGER NOT NULL,
    taijutsu_dc  INTEGER NOT NULL
);

-- Flat bonuses a Role or Class Mod applies automatically, one row per
-- non-zero stat. stat 'note' rows carry a text-only benefit instead.
CREATE TABLE adversary_auto_bonuses (
    tag       TEXT NOT NULL,
    stat      TEXT NOT NULL,
    value     INTEGER NOT NULL,
    note_text TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (tag, stat)
);

-- Pack 1's Hit/Chakra Point Modifier per Class: the multiplier addition is
-- base + per_player x the party size. A Minion ignores the formula and has
-- 10 + its level (minion_flat = 1).
CREATE TABLE adversary_class_hp_mods (
    class_slug  TEXT PRIMARY KEY,
    base        REAL NOT NULL,
    per_player  REAL NOT NULL,
    minion_flat INTEGER NOT NULL DEFAULT 0
);
INSERT INTO adversary_class_hp_mods (class_slug, base, per_player, minion_flat) VALUES
    ('adversary-rank/minion', 0, 0, 1),
    ('adversary-rank/standard', 0, 0, 0),
    ('adversary-rank/elite', 1.25, 0, 0),
    ('adversary-rank/solo', 1.5, 0.5, 0);

-- Jutsu Known by Class, jutsu rank and level (Bingo Book pp.9-11). The count
-- at a level is the row with the highest level_min not above it. A Minion's
-- jutsu are not split by rank (jutsu_rank '*'): one at level 1, plus one per
-- four levels.
CREATE TABLE adversary_jutsu_known (
    class_slug TEXT NOT NULL,
    jutsu_rank TEXT NOT NULL CHECK (jutsu_rank IN ('*','D','C','B','A','S')),
    level_min  INTEGER NOT NULL,
    count      INTEGER NOT NULL,
    PRIMARY KEY (class_slug, jutsu_rank, level_min)
);
INSERT INTO adversary_jutsu_known (class_slug, jutsu_rank, level_min, count) VALUES
    ('adversary-rank/minion', '*', 1, 1),
    ('adversary-rank/minion', '*', 4, 2),
    ('adversary-rank/minion', '*', 8, 3),
    ('adversary-rank/minion', '*', 12, 4),
    ('adversary-rank/minion', '*', 16, 5),
    ('adversary-rank/minion', '*', 20, 6),
    ('adversary-rank/minion', '*', 24, 7),
    ('adversary-rank/minion', '*', 28, 8),
    ('adversary-rank/standard', 'D', 1, 2),
    ('adversary-rank/standard', 'D', 5, 3),
    ('adversary-rank/standard', 'D', 13, 4),
    ('adversary-rank/standard', 'C', 5, 1),
    ('adversary-rank/standard', 'C', 9, 2),
    ('adversary-rank/standard', 'C', 13, 3),
    ('adversary-rank/standard', 'C', 17, 4),
    ('adversary-rank/standard', 'B', 9, 1),
    ('adversary-rank/standard', 'B', 13, 2),
    ('adversary-rank/standard', 'B', 17, 3),
    ('adversary-rank/standard', 'A', 13, 1),
    ('adversary-rank/standard', 'A', 19, 2),
    ('adversary-rank/standard', 'S', 18, 1),
    ('adversary-rank/standard', 'S', 25, 2),
    ('adversary-rank/elite', 'D', 1, 2),
    ('adversary-rank/elite', 'D', 5, 3),
    ('adversary-rank/elite', 'D', 13, 4),
    ('adversary-rank/elite', 'C', 5, 2),
    ('adversary-rank/elite', 'C', 9, 3),
    ('adversary-rank/elite', 'C', 16, 5),
    ('adversary-rank/elite', 'B', 6, 1),
    ('adversary-rank/elite', 'B', 9, 2),
    ('adversary-rank/elite', 'A', 13, 3),
    ('adversary-rank/elite', 'A', 17, 4),
    ('adversary-rank/elite', 'S', 13, 1),
    ('adversary-rank/elite', 'S', 18, 2),
    ('adversary-rank/elite', 'S', 25, 3),
    ('adversary-rank/solo', 'D', 1, 2),
    ('adversary-rank/solo', 'D', 5, 4),
    ('adversary-rank/solo', 'D', 13, 6),
    ('adversary-rank/solo', 'C', 5, 2),
    ('adversary-rank/solo', 'C', 9, 3),
    ('adversary-rank/solo', 'C', 16, 5),
    ('adversary-rank/solo', 'B', 6, 1),
    ('adversary-rank/solo', 'B', 9, 2),
    ('adversary-rank/solo', 'A', 9, 1),
    ('adversary-rank/solo', 'A', 13, 2),
    ('adversary-rank/solo', 'A', 17, 3),
    ('adversary-rank/solo', 'S', 16, 1),
    ('adversary-rank/solo', 'S', 18, 2),
    ('adversary-rank/solo', 'S', 25, 3);

INSERT INTO adversary_level_stats (level, tier, ac_reference, prof, base_hp, base_cp, attack_bonus, damage_per_action, standard_mods, elite_mods, xp) VALUES
    (-3, 0, 11, 1, 1, 1, 1, 1, '0,0,0,-1,-2,-3', '1,0,0,-1,-2,-3', 1),
    (-2, 0, 11, 2, 1, 1, 3, 2, '1,0,0,-1,-2,-3', '1,0,0,-1,-2,-3', 1),
    (-1, 0, 11, 2, 2, 2, 3, 3, '1,0,0,-1,-2,-3', '1,1,0,-1,-2,-3', 1),
    (0, 0, 11, 2, 10, 10, 3, 5, '1,0,0,-1,-2,-3', '2,1,0,-1,-2,-3', 1),
    (1, 1, 11, 3, NULL, NULL, 5, 10, '2,1,1,-1,-2,-3', '2,2,1,-1,-2,-2', 1),
    (2, 1, 11, 3, NULL, NULL, 5, 13, '2,1,1,0,-2,-3', '2,2,1,0,-1,-2', 1),
    (3, 1, 11, 3, NULL, NULL, 5, 16, '2,1,1,0,-1,-3', '3,2,1,0,-1,-2', 1),
    (4, 1, 12, 4, NULL, NULL, 6, 20, '2,1,1,0,-1,-2', '3,3,1,0,-1,-2', 1),
    (5, 2, 12, 4, NULL, NULL, 7, 20, '3,1,2,0,-1,-2', '3,3,1,0,0,-1', 1),
    (6, 2, 12, 4, NULL, NULL, 7, 24, '3,2,2,0,-1,-2', '4,3,1,1,0,-1', 1),
    (7, 2, 12, 5, NULL, NULL, 8, 28, '3,2,2,1,-1,-2', '4,3,2,1,0,-1', 1),
    (8, 2, 12, 5, NULL, NULL, 8, 30, '3,2,2,1,-1,-2', '4,3,2,1,0,-1', 1),
    (9, 3, 12, 5, NULL, NULL, 9, 30, '4,2,3,1,-1,-2', '5,4,2,1,1,0', 1),
    (10, 3, 13, 6, NULL, NULL, 10, 32, '4,3,3,1,0,-2', '5,4,2,1,1,0', 1),
    (11, 3, 13, 6, NULL, NULL, 10, 36, '4,3,3,1,0,-1', '5,4,2,1,1,0', 1),
    (12, 3, 13, 6, NULL, NULL, 10, 40, '4,3,3,2,0,-1', '6,4,2,1,1,0', 1),
    (13, 4, 13, 7, NULL, NULL, 12, 40, '5,3,4,2,0,-1', '6,4,2,1,1,1', 1),
    (14, 4, 13, 7, NULL, NULL, 12, 44, '5,3,4,2,0,-1', '6,5,3,2,1,1', 1),
    (15, 4, 13, 7, NULL, NULL, 12, 48, '5,4,4,2,0,-1', '7,5,3,2,2,1', 1),
    (16, 4, 14, 8, NULL, NULL, 13, 50, '5,4,4,2,0,-1', '7,5,3,2,2,1', 1),
    (17, 5, 14, 8, NULL, NULL, 14, 50, '6,5,4,2,1,-1', '7,5,3,2,2,2', 1),
    (18, 5, 14, 8, NULL, NULL, 14, 54, '6,5,4,2,1,0', '8,5,3,2,2,2', 1),
    (19, 5, 14, 9, NULL, NULL, 15, 58, '6,5,4,2,1,0', '8,6,3,2,2,2', 1),
    (20, 5, 14, 9, NULL, NULL, 15, 60, '6,5,5,3,1,0', '8,6,4,3,2,2', 1),
    (21, 6, 14, 9, NULL, NULL, 16, 60, '7,5,5,3,1,0', '9,6,4,3,3,2', 1),
    (22, 6, 15, 10, NULL, NULL, 17, 64, '7,5,5,3,1,0', '9,6,4,3,3,2', 1),
    (23, 6, 15, 10, NULL, NULL, 17, 68, '7,5,5,3,1,0', '9,6,4,3,3,2', 1),
    (24, 6, 15, 10, NULL, NULL, 17, 70, '7,6,5,3,1,0', '10,7,4,3,3,2', 1),
    (25, 7, 15, 11, NULL, NULL, 19, 70, '8,6,5,3,1,0', '10,7,4,3,3,2', 1),
    (26, 7, 15, 11, NULL, NULL, 19, 74, '8,6,5,3,1,0', '10,7,4,4,3,2', 1),
    (27, 7, 15, 11, NULL, NULL, 19, 78, '8,6,5,3,1,0', '12,7,5,4,3,2', 1),
    (28, 7, 16, 12, NULL, NULL, 20, 80, '8,7,5,3,1,0', '12,7,5,4,3,2', 1),
    (29, 8, 16, 12, NULL, NULL, 21, 80, '9,7,5,3,1,0', '12,8,5,4,3,2', 1),
    (30, 8, 16, 12, NULL, NULL, 21, 90, '9,7,5,3,1,0', '12,8,6,4,3,2', 1);

INSERT INTO adversary_trait_counts (level, minion, standard, elite, solo) VALUES
    (-3, 0, 2, 2, 4),
    (-2, 0, 2, 2, 4),
    (-1, 0, 2, 2, 4),
    (0, 0, 2, 2, 4),
    (1, 1, 2, 4, 6),
    (2, 1, 2, 4, 6),
    (3, 1, 2, 4, 6),
    (4, 1, 2, 4, 6),
    (5, 1, 2, 4, 6),
    (6, 1, 2, 4, 6),
    (7, 1, 2, 4, 6),
    (8, 1, 2, 4, 6),
    (9, 1, 2, 6, 8),
    (10, 1, 2, 6, 8),
    (11, 1, 3, 6, 8),
    (12, 2, 3, 6, 8),
    (13, 2, 3, 6, 8),
    (14, 2, 3, 6, 8),
    (15, 2, 3, 6, 8),
    (16, 2, 3, 6, 8),
    (17, 2, 4, 6, 8),
    (18, 2, 4, 8, 10),
    (19, 2, 4, 8, 10),
    (20, 2, 4, 8, 10),
    (21, 2, 4, 8, 10),
    (22, 2, 4, 8, 10),
    (23, 2, 4, 8, 10),
    (24, 2, 4, 8, 10),
    (25, 2, 4, 8, 10),
    (26, 2, 4, 10, 12),
    (27, 2, 4, 10, 12),
    (28, 2, 4, 10, 12),
    (29, 2, 4, 10, 12),
    (30, 2, 4, 10, 12);

INSERT INTO adversary_rank_tiers (rank, hit_die, hit_avg, chakra_die, chakra_avg, defensive_bonus, to_hit_bonus, save_dc_bonus, bonus_damage) VALUES
    ('E', 4, 3, 4, 3, 0, -2, -3, 0),
    ('D', 6, 4, 6, 4, 0, -1, -2, 0),
    ('C', 8, 5, 8, 5, 0, 0, -1, 0),
    ('B', 10, 6, 10, 6, 1, 0, 0, 5),
    ('A', 12, 7, 12, 7, 2, 1, 1, 10),
    ('S', 20, 11, 20, 11, 3, 2, 2, 15);

INSERT INTO adversary_builder_roles (slug, name, ac_mod, save_mod, hp_mod, cp_mod, damage_mult, speed_bonus, trait_role_slug, sort_order) VALUES
    ('adversary-builder-role/caster-ninjutsu', 'Caster (Ninjutsu)', -1, 1, 0.5, 1.5, 1.2, 0, '', 0),
    ('adversary-builder-role/caster-genjutsu', 'Caster (Genjutsu)', -1, 1, 0.5, 1.5, 1.2, 0, '', 1),
    ('adversary-builder-role/caster-taijutsu', 'Caster (Taijutsu)', -1, 1, 1.25, 0.75, 1.2, 0, '', 2),
    ('adversary-builder-role/controller', 'Controller', 0, 2, 0.8, 1.2, 0.8, 0, 'adversary-role/controller', 3),
    ('adversary-builder-role/defender', 'Defender', -1, 1, 1.4, 0.8, 0.7, 10, 'adversary-role/defender', 4),
    ('adversary-builder-role/lurker', 'Lurker', -2, 1, 0.7, 1.2, 1.3, 0, 'adversary-role/lurker', 5),
    ('adversary-builder-role/generalist', 'Generalist', 1, 1, 1.0, 1.0, 1.0, 0, '', 6),
    ('adversary-builder-role/striker-ninjutsu', 'Striker (Ninjutsu)', 2, 0, 1.3, 0.6, 1.1, 15, 'adversary-role/striker', 7),
    ('adversary-builder-role/striker-genjutsu', 'Striker (Genjutsu)', 2, 0, 1.3, 0.6, 1.1, 15, 'adversary-role/striker', 8),
    ('adversary-builder-role/striker-taijutsu', 'Striker (Taijutsu)', 2, 0, 1.3, 0.6, 1.1, 15, 'adversary-role/striker', 9),
    ('adversary-builder-role/supporter', 'Supporter', 1, 0, 0.8, 0.8, 0.5, 5, 'adversary-role/supporter', 10);

INSERT INTO adversary_clan_mods (clan_tag, hp_mod, cp_mod, ninjutsu_atk, ninjutsu_dc, genjutsu_atk, genjutsu_dc, taijutsu_atk, taijutsu_dc) VALUES
    ('Aburame', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Akimichi', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Bakuton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Beast', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Celestial', 1.0, 1.0, 2, 1, 2, 1, 0, 0),
    ('Construct', 0.5, 0.5, 0, 0, 0, 0, 0, 0),
    ('Demon', 1.0, 1.0, 1, 1, 1, 1, 1, 1),
    ('Fuma', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Futton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hatake', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hebi', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hoshigaki', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hozuki', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hyuga', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Inuzuka', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Jiton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Jugo', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Kaguya', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Kurama', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Kuru', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Monstrosity', 1.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Mutant', 0.5, 0.5, 0, 0, 0, 0, 0, 0),
    ('Namikaze', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Nara', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Non_Clan', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Plant', 0.0, 1.0, 0, 0, 0, 0, 0, 0),
    ('Ranton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Ryu', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Sage Creature', 1.0, 1.0, 0, 0, 0, 0, 0, 0),
    ('Sarutobi', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Senju', 0.5, 0.0, 0, 0, 0, 0, 0, 0),
    ('Shakuton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Shikigami', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Shoton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Tsuchigumo', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Uchiha', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Undead', 0.5, 0.5, 0, 0, 0, 0, 0, 0),
    ('Uzumaki', 0.0, 0.5, 0, 0, 0, 0, 0, 0),
    ('Yamanaka', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Yoton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Yuki', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Otsutsuki', 2.0, 2.0, 2, 1, 2, 1, 2, 1),
    ('Cerealian T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Chinoike', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Fushin', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Genwa', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hakaiton T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hanami', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Hoshi', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Iburi', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Kageyama T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Kato T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Keton', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Konjiki', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Kozu T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Mugen T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Ninneko T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Synthetic Human', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Ouyoku T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Polendina T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Shinigami T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Variant Senju T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Wukong, Low Class T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Yamada', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Gansuringa T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Shi Hou', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Vesper', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Yamada T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Synthetic Human T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0),
    ('Tayuya T7', 0.0, 0.0, 0, 0, 0, 0, 0, 0);

INSERT INTO adversary_auto_bonuses (tag, stat, value, note_text) VALUES
    ('Caster (Ninjutsu)', 'note', 0, '+1 additional die of damage for Ninjutsu'),
    ('Caster (Genjutsu)', 'note', 0, '+1 additional die of damage for Genjutsu'),
    ('Caster (Taijutsu)', 'note', 0, '+1 additional die of damage for Taijutsu'),
    ('Defender', 'dr', 2, ''),
    ('Lurker', 'speed_flat', 25, ''),
    ('Lurker', 'note', 0, 'Crit range for a Lurkers attacks is increased by 1'),
    ('Generalist', 'init', 2, ''),
    ('Generalist', 'all_skills', 2, ''),
    ('Striker (Ninjutsu)', 'ninjutsu_atk', 1, ''),
    ('Striker (Genjutsu)', 'genjutsu_atk', 1, ''),
    ('Striker (Taijutsu)', 'taijutsu_atk', 1, ''),
    ('Supporter', 'note', 0, '+2 on Chakra Control checks');
