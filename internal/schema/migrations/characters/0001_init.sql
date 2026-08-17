-- =============================================================================
-- N5E character database — initial schema.
--
-- Lives as characters.db NEXT TO THE EXECUTABLE (portable-folder model:
-- copy the folder and your characters travel with it). Created on first run.
--
-- This database references rules-DB content by SLUG only (e.g.
-- 'jutsu/chakra-blow'). There are no cross-database foreign keys — the rules
-- DB is a separate embedded file — so slug references are validated in Go,
-- and the validation logic reports any slug that stops existing after a
-- rules update instead of breaking the sheet.
--
-- Design rules:
--  * Store INPUTS and mutable play state; never store derived math.
--    Ability modifiers, AC, save DCs, jutsu-known limits: internal/rules.
--  * Max HP/chakra are the one subtle case: N5E offers fixed-value OR rolled
--    progression, so per-level gains are recorded as rows in
--    character_level_gains and max HP/chakra is their sum + Con adjustments
--    (computed in Go). This keeps rolled dice auditable.
--  * Every ability score bonus is recorded with its source (clan, background,
--    ASI, feat) so the sheet can show WHY a stat is what it is, and undo
--    cleanly when a choice changes.
-- =============================================================================

PRAGMA foreign_keys = ON;

CREATE TABLE characters (
    id              INTEGER PRIMARY KEY,
    name            TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),

    -- Creation choices (slugs into the rules DB)
    clan_slug       TEXT,                       -- 'clan/uzumaki'
    background_slug TEXT,                       -- 'background/trouble-maker'

    -- Base ability scores AS ASSIGNED at creation (standard array / point buy
    -- / rolled), BEFORE clan/background/feat increases. Increases live in
    -- character_ability_bonuses so they stay auditable.
    base_str INTEGER NOT NULL DEFAULT 10,
    base_dex INTEGER NOT NULL DEFAULT 10,
    base_con INTEGER NOT NULL DEFAULT 10,
    base_int INTEGER NOT NULL DEFAULT 10,
    base_wis INTEGER NOT NULL DEFAULT 10,
    base_cha INTEGER NOT NULL DEFAULT 10,

    -- Progression. Character level = sum of class levels (multiclass-ready);
    -- stored XP is progress toward the NEXT level (N5E resets XP each level).
    xp              INTEGER NOT NULL DEFAULT 0,

    -- Mutable play state (the only "derived-ish" numbers we store).
    current_hp      INTEGER NOT NULL DEFAULT 0,
    temp_hp         INTEGER NOT NULL DEFAULT 0,
    current_chakra  INTEGER NOT NULL DEFAULT 0,
    temp_chakra     INTEGER NOT NULL DEFAULT 0,
    hit_dice_spent  INTEGER NOT NULL DEFAULT 0,
    chakra_dice_spent INTEGER NOT NULL DEFAULT 0,
    ryo             REAL NOT NULL DEFAULT 0,

    -- Free-text sheet fields
    appearance      TEXT,
    backstory       TEXT
);

-- One row per class the character has levels in. order_index preserves the
-- order classes were taken (matters for multiclass starting proficiencies).
CREATE TABLE character_classes (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    class_slug   TEXT NOT NULL,
    levels       INTEGER NOT NULL CHECK (levels BETWEEN 1 AND 20),
    order_index  INTEGER NOT NULL,
    PRIMARY KEY (character_id, class_slug)
);

-- Per-level HP/chakra gains: supports both the default fixed values and the
-- DM-optional rolled progression, and keeps every roll auditable.
CREATE TABLE character_level_gains (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    class_slug   TEXT NOT NULL,
    class_level  INTEGER NOT NULL,              -- level IN THAT CLASS (1..20)
    hp_gain      INTEGER NOT NULL,
    chakra_gain  INTEGER NOT NULL,
    method       TEXT NOT NULL CHECK (method IN ('fixed','rolled')),
    PRIMARY KEY (character_id, class_slug, class_level)
);

-- Every ability-score bonus with its source, e.g.
--   ('clan',       'clan/uzumaki',            'con', +2)
--   ('background', 'background/trouble-maker','cha', +1)
--   ('asi',        'level-4',                 'str', +1)
CREATE TABLE character_ability_bonuses (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    source_kind  TEXT NOT NULL CHECK (source_kind IN
                 ('clan','background','asi','feat','bloodline','other')),
    source_ref   TEXT NOT NULL,                 -- slug or label of the source
    ability      TEXT NOT NULL CHECK (ability IN ('str','dex','con','int','wis','cha')),
    amount       INTEGER NOT NULL
);

-- Archetype picks (some classes pick several over a career, e.g. Genjutsu
-- Pledges at levels 2/6/10/14/18).
CREATE TABLE character_archetypes (
    character_id    INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    archetype_slug  TEXT NOT NULL,
    chosen_at_level INTEGER NOT NULL,
    PRIMARY KEY (character_id, archetype_slug)
);

-- The Ambition system: Drive / Goals / Fear (free text, possibly seeded from
-- a background's suggested prompts).
CREATE TABLE character_ambitions (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('drive','goal','fear')),
    text         TEXT NOT NULL
);

-- Jutsu the character knows. EITHER jutsu_slug (published jutsu, rules DB)
-- OR custom_jutsu_id (player-created via the Creating a Jutsu rules) is set.
-- source records how it was learned: class picks count against Jutsu Known,
-- clan/feat grants do not (core book, "Jutsu Known").
CREATE TABLE character_jutsu (
    id              INTEGER PRIMARY KEY,
    character_id    INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    jutsu_slug      TEXT,
    custom_jutsu_id INTEGER REFERENCES custom_jutsu(id),
    learned_at_level INTEGER,
    source          TEXT NOT NULL DEFAULT 'learned' CHECK (source IN
                    ('learned','clan','class-feature','feat','created')),
    CHECK ((jutsu_slug IS NULL) <> (custom_jutsu_id IS NULL))
);

-- Player-created jutsu (custom jutsu creation rules). Mirrors the published
-- jutsu field block so the sheet renders both identically.
CREATE TABLE custom_jutsu (
    id              INTEGER PRIMARY KEY,
    character_id    INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    classification  TEXT NOT NULL,
    rank            TEXT NOT NULL,
    casting_time    TEXT NOT NULL,
    range           TEXT NOT NULL,
    duration        TEXT NOT NULL,
    components      TEXT NOT NULL,
    cost_chakra     INTEGER,
    cost_text       TEXT NOT NULL,
    keywords        TEXT NOT NULL,
    description     TEXT NOT NULL,
    at_higher_ranks TEXT,
    creation_notes  TEXT                        -- which creation options priced it
);

CREATE TABLE character_feats (
    character_id    INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    feat_slug       TEXT NOT NULL,
    chosen_at_level INTEGER,
    source          TEXT NOT NULL DEFAULT 'asi' CHECK (source IN
                    ('asi','background','class-feature','clan','other')),
    PRIMARY KEY (character_id, feat_slug)
);

-- Bloodline Latent selections (schema-ready even though the GUI ships this
-- flow last).
CREATE TABLE character_bloodlines (
    character_id    INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    bloodline_slug  TEXT NOT NULL,
    chosen_at_level INTEGER,
    PRIMARY KEY (character_id, bloodline_slug)
);

-- Inventory: item_slug for published equipment, custom_name for ad-hoc items.
CREATE TABLE character_inventory (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    item_slug    TEXT,
    custom_name  TEXT,
    quantity     INTEGER NOT NULL DEFAULT 1,
    equipped     INTEGER NOT NULL DEFAULT 0,    -- 1 = worn/wielded (feeds AC etc.)
    notes        TEXT,
    CHECK (item_slug IS NOT NULL OR custom_name IS NOT NULL)
);

-- Player-level manual overrides for computed sheet values — same override
-- philosophy as the rules DB. field is a dotted path the sheet engine knows,
-- e.g. 'ac', 'speed', 'max_hp'. Use sparingly; the note says why.
CREATE TABLE character_overrides (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    field        TEXT NOT NULL,
    value        TEXT NOT NULL,
    note         TEXT,
    PRIMARY KEY (character_id, field)
);
