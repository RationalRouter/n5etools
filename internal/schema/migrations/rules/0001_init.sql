-- =============================================================================
-- N5E rules database — initial schema.
--
-- This database holds GAME CONTENT ONLY (clans, classes, jutsu, feats, gear).
-- It is produced by `n5e-ingest` from the sourcebook PDFs and embedded
-- read-only into the player app. Player characters live in a separate
-- database (see migrations/characters).
--
-- Conventions used throughout:
--
--  * Primary keys are human-readable SLUGS assigned once at ingestion, e.g.
--    'jutsu/chakra-blow', 'clan/aburame', 'class/genjutsu-specialist'.
--    Every cross-reference points at a slug. Nothing is ever matched by
--    re-typing a name — that whole category of spreadsheet work is gone.
--
--  * Provenance columns on every content table say exactly where a row came
--    from: source_book (slug), source_version (book version string, the four
--    books ship at DIFFERENT versions), source_page (page in that PDF).
--
--  * detection_status says how much a human should trust the row:
--      'auto'         parsed cleanly, no anomalies
--      'needs_review' parser or OCR was unsure — shows up in the validation
--                     report until a human resolves it
--      'verified'     a human checked it against the book
--      'manual'       a human entered it by hand (no parser involved)
--
--  * Overrides are first-class: wherever a parsed value might need a human
--    correction there is a matching *_override column. The app always reads
--    COALESCE(x_override, x) — see the v_* views at the bottom. Re-running
--    ingestion rewrites parsed columns and NEVER touches overrides or notes.
--
--  * Derived values (ability modifiers, AC, max HP/chakra, save DCs) are NOT
--    stored here. They are computed in Go by internal/rules. If you are
--    looking for a number the old spreadsheets stored, check that package.
-- =============================================================================

PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- Sourcebooks: one row per ingested PDF, so every entity can say which file
-- and which version of the rules it came from.
-- ---------------------------------------------------------------------------
CREATE TABLE source_books (
    slug          TEXT PRIMARY KEY,             -- e.g. 'book/jutsu-compendium'
    title         TEXT NOT NULL,                -- "Jiraiya's Jutsu Compendium"
    version       TEXT NOT NULL,                -- "3.1" (books differ!)
    version_date  TEXT,                         -- as printed, e.g. "9/24/2024"
    file_name     TEXT NOT NULL,                -- the PDF this was read from
    file_sha256   TEXT NOT NULL,                -- so re-ingests can detect change
    ingested_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ---------------------------------------------------------------------------
-- Jutsu ranks: tiny enum table so rank ordering lives in one place.
-- E is weakest, S is strongest. sort_order makes "highest rank known"
-- comparisons trivial in SQL.
-- ---------------------------------------------------------------------------
CREATE TABLE jutsu_ranks (
    rank        TEXT PRIMARY KEY,               -- 'E','D','C','B','A','S'
    sort_order  INTEGER NOT NULL UNIQUE,        -- E=0 .. S=5
    spell_level_equivalent TEXT NOT NULL        -- e.g. 'Cantrips', '1st-2nd'
);

-- ---------------------------------------------------------------------------
-- CLANS (Tsunade's Studies compendium; ~50 of them incl. Non-Clan)
-- ---------------------------------------------------------------------------
CREATE TABLE clans (
    slug             TEXT PRIMARY KEY,          -- 'clan/aburame'
    name             TEXT NOT NULL,             -- 'Aburame Clan'
    epithet          TEXT,                      -- section title, e.g. 'Creepy Crawly'
    overview         TEXT,                      -- descriptive text under the epithet
    speed_feet       INTEGER,                   -- base walking speed from Traits
    speed_feet_override INTEGER,                -- human correction ("missing clan
                                                -- speeds" was a known manual chore)
    extra_language   TEXT,                      -- e.g. 'Insect-Speak'
    is_legacy        INTEGER NOT NULL DEFAULT 0,-- 1 = Legacy Content section

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- Fixed ability score increases granted by a clan (core book "Ability Score
-- Summary": e.g. Uzumaki = +2 Con, +1 Cha). One row per ability.
CREATE TABLE clan_ability_increases (
    clan_slug  TEXT NOT NULL REFERENCES clans(slug),
    ability    TEXT NOT NULL CHECK (ability IN ('str','dex','con','int','wis','cha')),
    amount     INTEGER NOT NULL,
    PRIMARY KEY (clan_slug, ability)
);

-- Skill/tool/language/weapon proficiencies granted by a clan's Traits block.
CREATE TABLE clan_proficiencies (
    clan_slug  TEXT NOT NULL REFERENCES clans(slug),
    kind       TEXT NOT NULL CHECK (kind IN ('skill','tool','language','weapon','armor')),
    value      TEXT NOT NULL,                   -- 'Nature', 'Animal Handling', ...
    PRIMARY KEY (clan_slug, kind, value)
);

-- Named, often level-gated clan features ("Bug Host: Beginning at 1st
-- level ..."). level is the level the feature is FIRST gained; improvements
-- at later levels stay inside description — the sheet engine re-parses those
-- ordinals when needed and the validation report flags unusual ones.
CREATE TABLE clan_features (
    slug             TEXT PRIMARY KEY,          -- 'clan/aburame/feature/bug-host'
    clan_slug        TEXT NOT NULL REFERENCES clans(slug),
    name             TEXT NOT NULL,
    level            INTEGER,                   -- parsed "gained at" level (NULL = always on)
    level_override   INTEGER,                   -- human correction ("features at
                                                -- unusual levels" manual chore)
    description      TEXT NOT NULL,
    sort_order       INTEGER,                   -- order of appearance in the book

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- Which jutsu belong to a clan's exclusive list ("Aburame Clan Jutsu ...
-- You can add these Jutsu to your jutsu list"). The jutsu themselves live in
-- the jutsu table; this is pure list membership.
CREATE TABLE clan_jutsu (
    clan_slug  TEXT NOT NULL REFERENCES clans(slug),
    jutsu_slug TEXT NOT NULL REFERENCES jutsu(slug),
    PRIMARY KEY (clan_slug, jutsu_slug)
);

-- ---------------------------------------------------------------------------
-- CLASSES (Orochimaru's Observations compendium; 11 classes)
-- ---------------------------------------------------------------------------
CREATE TABLE classes (
    slug             TEXT PRIMARY KEY,          -- 'class/genjutsu-specialist'
    name             TEXT NOT NULL,
    hit_die          INTEGER,                   -- 6 for d6, 8 for d8, ...
    chakra_die       INTEGER,                   -- chakra runs parallel to HP
    jutsu_tier       TEXT CHECK (jutsu_tier IN ('high','medium','low')),
    description      TEXT,                      -- flavor + Creating a <class> text
    quick_build      TEXT,
    is_legacy        INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- Which ability powers each casting discipline for this class
-- (e.g. Genjutsu Specialist: ninjutsu→int, genjutsu→wis).
CREATE TABLE class_casting (
    class_slug  TEXT NOT NULL REFERENCES classes(slug),
    discipline  TEXT NOT NULL CHECK (discipline IN ('ninjutsu','genjutsu','taijutsu','bukijutsu')),
    ability     TEXT NOT NULL CHECK (ability IN ('str','dex','con','int','wis','cha')),
    PRIMARY KEY (class_slug, discipline, ability)
);

-- Armor/weapon/tool/save/skill proficiencies from the class Proficiencies
-- block. Skill choices ("Choose three from ...") use kind='skill_choice' with
-- choose_n set and one row per option.
CREATE TABLE class_proficiencies (
    class_slug TEXT NOT NULL REFERENCES classes(slug),
    kind       TEXT NOT NULL CHECK (kind IN
               ('armor','weapon','tool','saving_throw','skill','skill_choice')),
    value      TEXT NOT NULL,
    choose_n   INTEGER,                         -- only for *_choice kinds
    PRIMARY KEY (class_slug, kind, value)
);

-- Starting equipment options ("(a) One Kunai stack or (b) One Shuriken
-- stack"). group_idx groups mutually-exclusive choices; choice_idx orders
-- them; item_slug links to equipment when the text maps cleanly.
CREATE TABLE class_equipment_options (
    class_slug  TEXT NOT NULL REFERENCES classes(slug),
    group_idx   INTEGER NOT NULL,
    choice_idx  INTEGER NOT NULL,
    description TEXT NOT NULL,
    item_slug   TEXT REFERENCES equipment(slug),
    quantity    INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (class_slug, group_idx, choice_idx)
);

-- One row per class per level 1..20 — the progression table that exists only
-- as an IMAGE in the PDF. Populated from reviewed OCR output in
-- seed/class_tables/, hence detection_status starts at 'needs_review'.
CREATE TABLE class_levels (
    class_slug           TEXT NOT NULL REFERENCES classes(slug),
    level                INTEGER NOT NULL CHECK (level BETWEEN 1 AND 20),
    proficiency_bonus    INTEGER,
    jutsu_known          INTEGER,
    jutsu_known_override INTEGER,
    highest_rank_known   TEXT REFERENCES jutsu_ranks(rank),
    highest_rank_known_override TEXT REFERENCES jutsu_ranks(rank),

    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT,
    PRIMARY KEY (class_slug, level)
);

-- Class-specific per-level resources from the same table image, e.g.
-- Genjutsu Specialist 'Malleable Mirages' = 2 at level 2. Name/value pairs so
-- eleven different classes don't need eleven different columns.
CREATE TABLE class_level_resources (
    class_slug    TEXT NOT NULL REFERENCES classes(slug),
    level         INTEGER NOT NULL,
    resource_name TEXT NOT NULL,
    value         TEXT NOT NULL,                -- TEXT: some resources are dice ('1d4')
    PRIMARY KEY (class_slug, level, resource_name),
    FOREIGN KEY (class_slug, level) REFERENCES class_levels(class_slug, level)
);

-- Level-gated class features parsed from prose ("Starting at 1st Level ...").
CREATE TABLE class_features (
    slug             TEXT PRIMARY KEY,          -- 'class/genjutsu-specialist/feature/actualization'
    class_slug       TEXT NOT NULL REFERENCES classes(slug),
    name             TEXT NOT NULL,
    level            INTEGER,
    level_override   INTEGER,
    description      TEXT NOT NULL,
    sort_order       INTEGER,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- ARCHETYPES — each class has its own differently-named subclass system
-- (Genjutsu Pledges, Hunter's Creeds, Weapon Forms, Puppet Techniques, ...).
-- One generic structure + a per-class display name covers all of them.
-- NOTE: unlike stock 5e, some classes pick MULTIPLE archetypes over their
-- career (Genjutsu Pledge at levels 2/6/10/14/18) — selection_levels records
-- the levels at which a new pick happens.
-- ---------------------------------------------------------------------------
CREATE TABLE archetype_groups (
    slug             TEXT PRIMARY KEY,          -- 'class/genjutsu-specialist/group/genjutsu-pledge'
    class_slug       TEXT NOT NULL REFERENCES classes(slug),
    display_name     TEXT NOT NULL,             -- 'Genjutsu Pledge'
    selection_levels TEXT,                      -- comma list, e.g. '2,6,10,14,18'
    description      TEXT,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

CREATE TABLE archetypes (
    slug             TEXT PRIMARY KEY,          -- '.../genjutsu-pledge/beguiler'
    group_slug       TEXT NOT NULL REFERENCES archetype_groups(slug),
    name             TEXT NOT NULL,
    description      TEXT,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

CREATE TABLE archetype_features (
    slug             TEXT PRIMARY KEY,
    archetype_slug   TEXT NOT NULL REFERENCES archetypes(slug),
    name             TEXT NOT NULL,
    level            INTEGER,                   -- class level gate, when stated
    level_override   INTEGER,
    description      TEXT NOT NULL,
    sort_order       INTEGER,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- JUTSU (Jiraiya's compendium + clan-exclusive jutsu from the clan book)
-- Field names mirror the book's rigid entry block one-to-one.
-- ---------------------------------------------------------------------------
CREATE TABLE jutsu (
    slug             TEXT PRIMARY KEY,          -- 'jutsu/chakra-blow'
    name             TEXT NOT NULL,
    classification   TEXT NOT NULL,             -- 'Ninjutsu' / 'Genjutsu' / ... as printed
    rank             TEXT NOT NULL REFERENCES jutsu_ranks(rank),
    rank_override    TEXT REFERENCES jutsu_ranks(rank),
    casting_time     TEXT NOT NULL,             -- '1 Action', '1 Bonus Action', ...
    range            TEXT NOT NULL,             -- '30 feet', 'Self', 'Weapons Range', ...
    duration         TEXT NOT NULL,             -- 'Instant', 'Concentration, up to 1 minute'
    components       TEXT NOT NULL,             -- 'HS, CM, W(any)' as printed
    cost_chakra      INTEGER,                   -- numeric part of 'Cost: 2 Chakra'
    cost_text        TEXT NOT NULL,             -- full cost line (some costs are odd)
    keywords         TEXT NOT NULL,             -- 'Ninjutsu, Fuinjutsu' as printed
    description      TEXT NOT NULL,
    at_higher_ranks  TEXT,                      -- the 'At Higher Ranks/Levels' block
    category_group   TEXT,                      -- book section: 'Non-Elemental',
                                                -- 'Fire Release', 'Summoning: Toad',
                                                -- 'Taijutsu', 'Bukijutsu', ...
    clan_slug        TEXT REFERENCES clans(slug), -- set for clan-exclusive jutsu

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- One row per keyword so 'find every Fuinjutsu' is an index lookup, while the
-- printed keyword line above stays untouched for display.
CREATE TABLE jutsu_keywords (
    jutsu_slug TEXT NOT NULL REFERENCES jutsu(slug),
    keyword    TEXT NOT NULL,                   -- 'Ninjutsu', 'Fuinjutsu', 'Sensory', ...
    PRIMARY KEY (jutsu_slug, keyword)
);

-- ---------------------------------------------------------------------------
-- FEATS — core book Ch. 13 categories plus clan feats (clan book) and
-- class/archetype feats (class book). clan_slug / class_slug scope a feat to
-- its owner when it is not generally available.
-- ---------------------------------------------------------------------------
CREATE TABLE feats (
    slug             TEXT PRIMARY KEY,          -- 'feat/action-surge'
    name             TEXT NOT NULL,
    category         TEXT NOT NULL,             -- 'general','skill','chakra','ninjutsu',
                                                -- 'taijutsu','genjutsu','critical',
                                                -- 'clan','archetype-class','caster-class',
                                                -- 'martial-class', ...
    prerequisites    TEXT,                      -- as printed; structured checks live in Go
    description      TEXT NOT NULL,
    clan_slug        TEXT REFERENCES clans(slug),
    class_slug       TEXT REFERENCES classes(slug),
    is_legacy        INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- BLOODLINE LATENTS (clan compendium, own section). Modeled minimally until
-- the parser meets the real structure; extended in a later migration if the
-- section turns out to have more shape (per-clan links, tiers, ...).
-- ---------------------------------------------------------------------------
CREATE TABLE bloodline_latents (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    stage            TEXT CHECK (stage IN ('latent','realized')),
    clan_slug        TEXT REFERENCES clans(slug),
    prerequisites    TEXT,
    description      TEXT NOT NULL,
    is_legacy        INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- BACKGROUNDS & AMBITIONS (core book Ch. 3). Each background grants
-- proficiencies plus an Ability Score increase or Free Feat; personality
-- traits are replaced by the Ambition system (Drive / Goals / Fear prompts).
-- ---------------------------------------------------------------------------
CREATE TABLE backgrounds (
    slug             TEXT PRIMARY KEY,          -- 'background/trouble-maker'
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    feature_name     TEXT,                      -- the background's special feature
    feature_text     TEXT,
    asi_text         TEXT,                      -- printed ASI / Free Feat grant

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

CREATE TABLE background_proficiencies (
    background_slug TEXT NOT NULL REFERENCES backgrounds(slug),
    kind            TEXT NOT NULL CHECK (kind IN ('skill','tool','language')),
    value           TEXT NOT NULL,
    PRIMARY KEY (background_slug, kind, value)
);

-- Suggested Ambition prompts (d-table entries under each background, plus any
-- general lists). kind mirrors the book's Drive / Goal / Fear split.
CREATE TABLE ambition_prompts (
    id              INTEGER PRIMARY KEY,
    background_slug TEXT REFERENCES backgrounds(slug),  -- NULL = general list
    kind            TEXT NOT NULL CHECK (kind IN ('drive','goal','fear')),
    text            TEXT NOT NULL
);

-- ---------------------------------------------------------------------------
-- FIGHTING STANCES (core book Ch. 13: Taijutsu / Bukijutsu stances)
-- ---------------------------------------------------------------------------
CREATE TABLE fighting_stances (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    stance_type      TEXT CHECK (stance_type IN ('taijutsu','bukijutsu')),
    prerequisites    TEXT,
    description      TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- EQUIPMENT (core book Ch. 5). One table, kind-discriminated; the per-kind
-- columns are nullable and only meaningful for their kind. Prices in Ryo
-- (1 gp ≈ 1 Ryo).
-- ---------------------------------------------------------------------------
CREATE TABLE equipment (
    slug             TEXT PRIMARY KEY,          -- 'weapon/kunai', 'armor/padded-cloth'
    name             TEXT NOT NULL,
    kind             TEXT NOT NULL CHECK (kind IN
                     ('weapon','armor','gear','tool','toolkit','scroll','enhancement_seal')),
    cost_ryo         REAL,
    weight_lb        REAL,
    description      TEXT,

    -- weapon-only
    damage_dice      TEXT,                      -- '1d4'
    damage_type      TEXT,                      -- 'Piercing' (incl. new Wind/Earth types)
    properties       TEXT,                      -- 'Finesse, Thrown (20/60)' as printed
    ammo_die         TEXT,                      -- the ammunition-die mechanic
    weapon_category  TEXT,                      -- 'simple','martial', ...

    -- armor-only
    ac_bonus         INTEGER,                   -- AC = 10 + Dex + ½Prof + this
    armor_category   TEXT,                      -- 'light','medium','heavy'
    strength_req     INTEGER,
    stealth_disadv   INTEGER,                   -- 1 = disadvantage on Stealth

    -- enhancement-seal-only
    seal_rank        TEXT REFERENCES jutsu_ranks(rank),
    seal_applies_to  TEXT CHECK (seal_applies_to IN ('weapon','armor')),

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- RULE TABLES — small fixed lookup tables the whole system leans on.
-- Seeded in 0002_seed.sql from the core book (hand-checked, not parsed).
-- ---------------------------------------------------------------------------

-- Character Advancement (core book p.14): XP needed and proficiency bonus per
-- level. NOTE: in N5E, XP RESETS to 0 on level-up (leftover carries).
CREATE TABLE xp_levels (
    level             INTEGER PRIMARY KEY CHECK (level BETWEEN 1 AND 20),
    xp_required       INTEGER NOT NULL,
    proficiency_bonus INTEGER NOT NULL
);

-- 30-point-buy costs (core book "Ability Score Point Cost").
CREATE TABLE ability_point_costs (
    score INTEGER PRIMARY KEY CHECK (score BETWEEN 8 AND 15),
    cost  INTEGER NOT NULL
);

-- Elemental Superiority circle: Fire > Wind > Lightning > Earth > Water > Fire.
-- superior attacks inferior at advantage (Elemental Advantage mechanic).
CREATE TABLE elemental_advantage (
    superior TEXT NOT NULL,
    inferior TEXT NOT NULL,
    PRIMARY KEY (superior, inferior)
);

-- ---------------------------------------------------------------------------
-- CUSTOM JUTSU CREATION (core book "Creating a Jutsu", Ch. 7): the priced
-- building blocks (components, keywords, effects per discipline) that the app
-- uses to cost player-built jutsu. Generic name/effect/cost rows; refined in
-- a later migration once the parser meets the full structure.
-- ---------------------------------------------------------------------------
CREATE TABLE jutsu_creation_options (
    slug             TEXT PRIMARY KEY,
    discipline       TEXT NOT NULL CHECK (discipline IN
                     ('ninjutsu','genjutsu','taijutsu','bukijutsu','any')),
    category         TEXT NOT NULL,             -- 'component','type','required-feature', ...
    name             TEXT NOT NULL,
    effect_text      TEXT NOT NULL,
    cost_text        TEXT,                      -- as printed; math lives in internal/rules

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- ---------------------------------------------------------------------------
-- EFFECTIVE-VALUE VIEWS — the app reads these, never the raw override pairs.
-- Pattern: COALESCE(override, parsed) AS the plain column name.
-- ---------------------------------------------------------------------------

CREATE VIEW v_clan_features AS
SELECT slug, clan_slug, name,
       COALESCE(level_override, level) AS level,
       description, sort_order, detection_status, notes
FROM clan_features;

CREATE VIEW v_class_features AS
SELECT slug, class_slug, name,
       COALESCE(level_override, level) AS level,
       description, sort_order, detection_status, notes
FROM class_features;

CREATE VIEW v_archetype_features AS
SELECT slug, archetype_slug, name,
       COALESCE(level_override, level) AS level,
       description, sort_order, detection_status, notes
FROM archetype_features;

CREATE VIEW v_class_levels AS
SELECT class_slug, level, proficiency_bonus,
       COALESCE(jutsu_known_override, jutsu_known) AS jutsu_known,
       COALESCE(highest_rank_known_override, highest_rank_known) AS highest_rank_known,
       detection_status, notes
FROM class_levels;

CREATE VIEW v_jutsu AS
SELECT slug, name, classification,
       COALESCE(rank_override, rank) AS rank,
       casting_time, range, duration, components, cost_chakra, cost_text,
       keywords, description, at_higher_ranks, category_group, clan_slug,
       detection_status, notes
FROM jutsu;

CREATE VIEW v_clans AS
SELECT slug, name, epithet, overview,
       COALESCE(speed_feet_override, speed_feet) AS speed_feet,
       extra_language, is_legacy, detection_status, notes
FROM clans;
