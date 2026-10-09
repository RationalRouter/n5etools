-- Stores a saved adversary's Adversary Builder choices (Bingo Book p.6,
-- internal/encounter.BuildChoices). The stat block itself is not stored:
-- it is recomputed from these choices and rules.db's builder charts (rules
-- migration 0082) every time the roster is read, so a chart correction
-- reaches every saved adversary.
--
-- adv_rank is the book's Rank (E..S). The existing rank_slug column holds
-- the book's Class (adversary_ranks: Minion/Standard/Elite/Solo) and keeps
-- its name. str_mod..cha_mod are NULL when the ability is assigned
-- automatically from the level's modifier array. save_profs is a
-- comma-separated list of ability keys; empty means the two highest.
ALTER TABLE encounter_adversaries ADD COLUMN adv_rank TEXT NOT NULL DEFAULT 'D' CHECK (adv_rank IN ('E','D','C','B','A','S'));
ALTER TABLE encounter_adversaries ADD COLUMN clan_tag TEXT NOT NULL DEFAULT 'Non_Clan';
ALTER TABLE encounter_adversaries ADD COLUMN affiliation_tag TEXT NOT NULL DEFAULT 'Factionless';
ALTER TABLE encounter_adversaries ADD COLUMN class_mod_tag TEXT NOT NULL DEFAULT '';
ALTER TABLE encounter_adversaries ADD COLUMN armor TEXT NOT NULL DEFAULT 'light';
ALTER TABLE encounter_adversaries ADD COLUMN str_mod INTEGER;
ALTER TABLE encounter_adversaries ADD COLUMN dex_mod INTEGER;
ALTER TABLE encounter_adversaries ADD COLUMN con_mod INTEGER;
ALTER TABLE encounter_adversaries ADD COLUMN int_mod INTEGER;
ALTER TABLE encounter_adversaries ADD COLUMN wis_mod INTEGER;
ALTER TABLE encounter_adversaries ADD COLUMN cha_mod INTEGER;
ALTER TABLE encounter_adversaries ADD COLUMN save_profs TEXT NOT NULL DEFAULT '';
ALTER TABLE encounter_adversaries ADD COLUMN adj_hp INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN adj_cp INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN adj_ac INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN adj_init INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN adj_speed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE encounter_adversaries ADD COLUMN adj_dr INTEGER NOT NULL DEFAULT 0;

-- Existing adversaries get the Rank their level maps to
-- (encounter.RankForLevel).
UPDATE encounter_adversaries SET adv_rank = CASE
    WHEN level <= 4 THEN 'D'
    WHEN level <= 8 THEN 'C'
    WHEN level <= 12 THEN 'B'
    WHEN level <= 16 THEN 'A'
    ELSE 'S' END;

-- Pack 1's five roles map onto the builder's eleven (a Pack 1 Striker is a
-- weapon fighter, so Striker (Taijutsu)), with the armor the generator
-- gives that role (encounter.DefaultArmor).
UPDATE encounter_adversaries SET role_slug = 'adversary-builder-role/striker-taijutsu', role_name = 'Striker (Taijutsu)', armor = 'medium'
    WHERE role_slug = 'adversary-role/striker';
UPDATE encounter_adversaries SET role_slug = 'adversary-builder-role/controller', role_name = 'Controller', armor = 'chakra-wis'
    WHERE role_slug = 'adversary-role/controller';
UPDATE encounter_adversaries SET role_slug = 'adversary-builder-role/defender', role_name = 'Defender', armor = 'heavy'
    WHERE role_slug = 'adversary-role/defender';
UPDATE encounter_adversaries SET role_slug = 'adversary-builder-role/lurker', role_name = 'Lurker', armor = 'light'
    WHERE role_slug = 'adversary-role/lurker';
UPDATE encounter_adversaries SET role_slug = 'adversary-builder-role/supporter', role_name = 'Supporter', armor = 'chakra-wis'
    WHERE role_slug = 'adversary-role/supporter';

-- Traits from the community Adversary Builder sheet can be unranked, so the
-- rank CHECK now allows ''. trait_source says which catalog the slug
-- belongs to (rules.db adversary_role_traits or
-- adversary_builder_sheet_traits); trait_category is the sheet's category
-- (general, role, clan, affiliation, class_mod, iconic), or 'role' for an
-- official trait.
CREATE TABLE encounter_adversary_traits_new (
    adversary_id      INTEGER NOT NULL REFERENCES encounter_adversaries(id) ON DELETE CASCADE,
    trait_slug        TEXT NOT NULL,
    trait_name        TEXT NOT NULL,
    trait_rank        TEXT NOT NULL CHECK (trait_rank IN ('','D','C','B','A','S')),
    trait_description TEXT NOT NULL DEFAULT '',
    sort_order        INTEGER NOT NULL DEFAULT 0,
    trait_source      TEXT NOT NULL DEFAULT 'official' CHECK (trait_source IN ('official','sheet')),
    trait_category    TEXT NOT NULL DEFAULT 'role',
    PRIMARY KEY (adversary_id, trait_slug)
);
INSERT INTO encounter_adversary_traits_new
    (adversary_id, trait_slug, trait_name, trait_rank, trait_description, sort_order)
    SELECT adversary_id, trait_slug, trait_name, trait_rank, trait_description, sort_order
    FROM encounter_adversary_traits;
DROP TABLE encounter_adversary_traits;
ALTER TABLE encounter_adversary_traits_new RENAME TO encounter_adversary_traits;

-- Step 9's known jutsu, snapshotted like traits. jutsu_slug points into
-- rules.db's jutsu table.
CREATE TABLE encounter_adversary_jutsu (
    adversary_id   INTEGER NOT NULL REFERENCES encounter_adversaries(id) ON DELETE CASCADE,
    jutsu_slug     TEXT NOT NULL,
    jutsu_name     TEXT NOT NULL,
    jutsu_rank     TEXT NOT NULL DEFAULT '',
    classification TEXT NOT NULL DEFAULT '',
    cost_text      TEXT NOT NULL DEFAULT '',
    sort_order     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (adversary_id, jutsu_slug)
);
