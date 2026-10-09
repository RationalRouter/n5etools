-- Schema for the Bingo Book (book/bingo-book, the 70-page community PDF), NOT
-- the Pack 1 rules chapter (book/bingo-book-pack-1, migrations
-- 0024/0076/0077).
--
-- The source PDF's text layer is an OCR pass over page images and is too
-- corrupted to parse into trustworthy numbers. Every row in these tables is
-- loaded from a hand transcription of the rendered pages
-- (internal/bingobook's embedded JSON), never from extracted text, so rows
-- carry detection_status 'manual'.
--
-- Family A: the NPC/adversary template stat-block library (pp.18-64). Coverage
-- is uneven: not every clan has a Genin/Chunin/Jonin triplet, Inuzuka has
-- Nin-Dog companion blocks, and pp.63-64 are named Iconic NPCs.
CREATE TABLE adversary_template_statblocks (
    slug               TEXT PRIMARY KEY,      -- 'adversary-template/generic-genin'
    name               TEXT NOT NULL,
    clan_slug          TEXT NOT NULL DEFAULT '',  -- '' for non-clan blocks
    clan_name          TEXT NOT NULL DEFAULT '',  -- as printed in the metadata line, e.g. 'Non-Clan'
    rank_tier          TEXT NOT NULL CHECK (rank_tier IN ('D','C','B','A','S')),
    class              TEXT NOT NULL CHECK (class IN ('minion','standard','elite','solo')),
    role               TEXT NOT NULL DEFAULT '',
    affiliation        TEXT NOT NULL DEFAULT '',
    size               TEXT NOT NULL DEFAULT '',
    level              INTEGER NOT NULL DEFAULT 0,
    xp                 INTEGER NOT NULL DEFAULT 0,
    prof_bonus         INTEGER NOT NULL DEFAULT 0,
    tenacity_die       TEXT NOT NULL DEFAULT '',  -- 'd4'; '' when the block has none
    tenacity_count     INTEGER NOT NULL DEFAULT 0,
    ac                 INTEGER NOT NULL DEFAULT 0,
    ac_note            TEXT NOT NULL DEFAULT '',  -- e.g. 'Dust Coat'
    dr                 INTEGER NOT NULL DEFAULT 0,
    initiative_bonus   INTEGER NOT NULL DEFAULT 0,
    speed              TEXT NOT NULL DEFAULT '',
    hp                 INTEGER NOT NULL DEFAULT 0,  -- first (full) phase
    hp_text            TEXT NOT NULL DEFAULT '',    -- as printed, e.g. '360/216/108' for phased blocks
    cp                 INTEGER NOT NULL DEFAULT 0,
    str_score INTEGER NOT NULL DEFAULT 0, str_mod INTEGER NOT NULL DEFAULT 0, str_save INTEGER NOT NULL DEFAULT 0,
    dex_score INTEGER NOT NULL DEFAULT 0, dex_mod INTEGER NOT NULL DEFAULT 0, dex_save INTEGER NOT NULL DEFAULT 0,
    con_score INTEGER NOT NULL DEFAULT 0, con_mod INTEGER NOT NULL DEFAULT 0, con_save INTEGER NOT NULL DEFAULT 0,
    int_score INTEGER NOT NULL DEFAULT 0, int_mod INTEGER NOT NULL DEFAULT 0, int_save INTEGER NOT NULL DEFAULT 0,
    wis_score INTEGER NOT NULL DEFAULT 0, wis_mod INTEGER NOT NULL DEFAULT 0, wis_save INTEGER NOT NULL DEFAULT 0,
    cha_score INTEGER NOT NULL DEFAULT 0, cha_mod INTEGER NOT NULL DEFAULT 0, cha_save INTEGER NOT NULL DEFAULT 0,
    damage_resistances    TEXT NOT NULL DEFAULT '',
    damage_immunities     TEXT NOT NULL DEFAULT '',
    damage_vulnerabilities TEXT NOT NULL DEFAULT '',
    condition_resistances TEXT NOT NULL DEFAULT '',
    condition_immunities  TEXT NOT NULL DEFAULT '',
    senses                TEXT NOT NULL DEFAULT '',  -- excluding passive Perception, e.g. 'Chakra Sight 60 ft.'
    passive_perception    INTEGER NOT NULL DEFAULT 0,

    source_book        TEXT REFERENCES source_books(slug),
    source_version     TEXT,
    source_page        INTEGER,
    detection_status   TEXT NOT NULL DEFAULT 'manual'
                       CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes              TEXT NOT NULL DEFAULT ''
);

CREATE TABLE adversary_template_skills (
    statblock_slug TEXT NOT NULL REFERENCES adversary_template_statblocks(slug) ON DELETE CASCADE,
    skill_name     TEXT NOT NULL,
    modifier       INTEGER NOT NULL DEFAULT 0,
    stars          INTEGER NOT NULL DEFAULT 0,  -- the book's ★ markers (0-3)
    sort_order     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_adversary_template_skills_statblock ON adversary_template_skills(statblock_slug);

-- Every named prose entry in a block — traits, clan/role passives, actions,
-- reactions, the elite action — in printed order. section_title is the
-- heading as printed ('Generic Genin Role Passives'); section_kind is its
-- normalized category. An entry with an empty name is a section's untitled
-- lead-in paragraph.
CREATE TABLE adversary_template_entries (
    statblock_slug TEXT NOT NULL REFERENCES adversary_template_statblocks(slug) ON DELETE CASCADE,
    section_order  INTEGER NOT NULL DEFAULT 0,
    section_kind   TEXT NOT NULL CHECK (section_kind IN
                       ('traits','passives','actions','bonus_actions','reactions','elite','other')),
    section_title  TEXT NOT NULL DEFAULT '',
    sort_order     INTEGER NOT NULL DEFAULT 0,
    name           TEXT NOT NULL DEFAULT '',
    tags           TEXT NOT NULL DEFAULT '',   -- e.g. 'Buff, Defence'
    description    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_adversary_template_entries_statblock ON adversary_template_entries(statblock_slug);

CREATE TABLE adversary_template_jutsu (
    statblock_slug TEXT NOT NULL REFERENCES adversary_template_statblocks(slug) ON DELETE CASCADE,
    discipline     TEXT NOT NULL DEFAULT '',   -- 'Ninjutsu', 'Genjutsu', etc.
    hit_bonus      INTEGER NOT NULL DEFAULT 0,
    save_dc        INTEGER NOT NULL DEFAULT 0,
    rank           TEXT NOT NULL DEFAULT '',   -- 'E'..'S'
    cost           INTEGER NOT NULL DEFAULT 0,
    jutsu_name     TEXT NOT NULL,
    is_new         INTEGER NOT NULL DEFAULT 0, -- printed with a '[New]' marker
    sort_order     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_adversary_template_jutsu_statblock ON adversary_template_jutsu(statblock_slug);

-- Family B: the General Traits glossary (pp.66-70), an alphabetical `Trait
-- | Rank | Keywords | Description` table distinct from both
-- adversary_role_traits (official) and adversary_builder_sheet_traits
-- (community sheet) — a third independent catalog.
CREATE TABLE adversary_beta_general_traits (
    slug             TEXT PRIMARY KEY,     -- 'adversary-beta-general-trait/abandon-a-friend'
    name             TEXT NOT NULL,
    rank             TEXT NOT NULL DEFAULT '' CHECK (rank IN ('','D','C','B','A','S')),
    keywords         TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    sort_order       INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT NOT NULL DEFAULT ''
);

-- Family C: the Specialist Jutsu Lists (p.65) — a short curated jutsu-name
-- list per nature release or discipline.
CREATE TABLE adversary_specialist_jutsu_lists (
    list_name        TEXT NOT NULL,         -- 'Fire', 'Genjutsu', 'Medical', ...
    jutsu_name       TEXT NOT NULL,
    list_order       INTEGER NOT NULL DEFAULT 0,
    sort_order       INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    PRIMARY KEY (list_name, jutsu_name)
);
