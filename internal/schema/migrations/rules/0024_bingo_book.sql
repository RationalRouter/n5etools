-- Schema for the Bingo Book Pack 1 parser (internal/parse/bingobook.go):
-- the adversary-building rules chapter (Ranks, Roles, Role Traits), NOT a
-- bestiary of named creatures — see that file's package comment for the
-- full scope explanation.
--
-- These are real auto-ingested content tables (detection_status/
-- source_book provenance, upsert-by-slug, human overrides survive
-- re-ingest), the same shape as feats/clans/classes — NOT hand-seeded
-- reference data like 0012/0016's weapon/armor properties, since this book
-- gets a real parser rather than a one-time manual transcription.

-- Five named Roles (Striker, Lurker, Defender, Controller, Supporter) an
-- adversary can be built with.
CREATE TABLE adversary_roles (
    slug             TEXT PRIMARY KEY,          -- 'adversary-role/striker'
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- Each Role's catalog of named, ranked traits an adversary with that Role
-- can be given.
CREATE TABLE adversary_role_traits (
    slug             TEXT PRIMARY KEY,          -- 'adversary-role-trait/aggressive'
    name             TEXT NOT NULL,
    role_slug        TEXT NOT NULL REFERENCES adversary_roles(slug),
    rank             TEXT NOT NULL CHECK (rank IN ('D','C','B','A','S')),
    description      TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
CREATE INDEX idx_adversary_role_traits_role ON adversary_role_traits(role_slug);

-- The four Minion/Standard/Elite/Solo rank templates. hp_formula is kept as
-- printed text (see AdversaryRank's doc comment in bingobook.go for why):
-- Minion's is a flat replacement, Standard has none (the ordinary
-- player-style calculation applies unmodified — a real, deliberate row with
-- every numeric column at its zero-bonus default, not a missing one),
-- Elite/Solo's are multipliers layered on top of that same baseline.
CREATE TABLE adversary_ranks (
    slug             TEXT PRIMARY KEY,          -- 'adversary-rank/minion'
    name             TEXT NOT NULL,
    hp_formula       TEXT NOT NULL DEFAULT '',
    ac_bonus         INTEGER NOT NULL DEFAULT 0,
    save_bonus       INTEGER NOT NULL DEFAULT 0,
    save_dc_bonus    INTEGER NOT NULL DEFAULT 0,
    init_bonus       INTEGER NOT NULL DEFAULT 0,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- A rank's Freeform Slots table (how many Freeform Attacks/Jutsu an
-- adversary of that rank and level can use). Pure derived data like
-- jutsu_keywords — no detection_status/overrides, fully replaced on every
-- ingest rather than diffed row-by-row.
CREATE TABLE adversary_freeform_slots (
    rank_slug TEXT NOT NULL REFERENCES adversary_ranks(slug),
    level_min INTEGER NOT NULL,
    slots     INTEGER NOT NULL,
    PRIMARY KEY (rank_slug, level_min)
);
