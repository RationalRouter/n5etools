-- Adversary Pack 1 enrichment, first auto-parseable slice: the tables and
-- glossaries whose printed layout is clean enough (single line per row, or
-- a small closed vocabulary of names) to parse safely. See
-- internal/parse/bingobook_freeform.go's package comment for why several
-- other tables in this same chapter (Freeform DR, Damage Penalty, Jutsu
-- Shapes, Effect & Conditions) are deliberately NOT here — their printed
-- column layout wraps irregularly and is hand-transcribed instead.

-- Damage dice for a Freeform Strike, by the adversary's level and which
-- column its class uses (Low for Minions, Medium for Standards, High for
-- Elites/Solos). Pure derived data, no detection_status — same convention
-- as adversary_freeform_slots.
CREATE TABLE adversary_freeform_strike_table (
    level_min INTEGER PRIMARY KEY,
    low       TEXT NOT NULL,
    medium    TEXT NOT NULL,
    high      TEXT NOT NULL DEFAULT ''  -- '' at levels 1-2, where the book prints an em-dash
);

-- Damage dice for a Freeform Jutsu, by tier (the book prints one table for
-- Minion/Standard adversaries and a separate one for Elite/Solo), the
-- adversary's level, and which of the five Rank-derived columns applies
-- (D-Rank -> Very Low ... S-Rank -> Very High). Pure derived data.
CREATE TABLE adversary_freeform_jutsu_damage_table (
    tier      TEXT NOT NULL CHECK (tier IN ('minion_standard', 'elite_solo')),
    level_min INTEGER NOT NULL,
    very_low  TEXT NOT NULL DEFAULT '',  -- '' where the book prints an em-dash (Elite/Solo, levels 1-2)
    low       TEXT NOT NULL DEFAULT '',
    medium    TEXT NOT NULL DEFAULT '',
    high      TEXT NOT NULL DEFAULT '',
    very_high TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (tier, level_min)
);

-- The flat stat bumps an Iconic or Epic Optional Classification adds on top
-- of an already-built adversary. Pure derived data (exactly one row per
-- kind, fully replaced on every ingest).
CREATE TABLE adversary_classification_modifiers (
    kind               TEXT PRIMARY KEY CHECK (kind IN ('iconic', 'epic')),
    hp_chakra_modifier INTEGER NOT NULL,
    ac_bonus           INTEGER NOT NULL,
    attack_save_bonus  INTEGER NOT NULL,
    ability_points     INTEGER NOT NULL,
    initiative_bonus   INTEGER NOT NULL
);

-- The book's own CLARIFICATIONS glossary (Auras/Ambush/Barrier/Intercept/
-- Mark/Strike/Saving Throws) defining how certain Role Trait wording should
-- be read. A closed, fixed 7-term vocabulary, same auto-ingested/
-- correctable shape as adversary_role_traits.
CREATE TABLE adversary_clarifications (
    slug             TEXT PRIMARY KEY,          -- 'adversary-clarification/auras'
    term             TEXT NOT NULL,
    description      TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- The fixed menu of enhancements a GM picks 2-3 from when building a Solo
-- Adversary's Transformation (More Powerful, Expanded Senses, Tougher
-- Defenses, Expanded Size, New Resistances or Immunities, New Movement,
-- Summon/Manifest, Rule Break). Same auto-ingested/correctable shape as
-- adversary_role_traits.
CREATE TABLE adversary_transformation_buffs (
    slug             TEXT PRIMARY KEY,          -- 'adversary-transformation-buff/more-powerful'
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    sort_order       INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
