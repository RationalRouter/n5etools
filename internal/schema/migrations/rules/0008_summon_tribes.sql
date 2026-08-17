-- ---------------------------------------------------------------------------
-- SUMMON TRIBES (jutsu compendium's Summoning chapter). A new entity type:
-- creature/tribe stat blocks a summoner fills in at cast time, not
-- characters. Gating is by summon RANK (D/C/B/A/S, tied to jutsu rank), not
-- character level, so features and the stat progression key on rank the
-- same way jutsu do (see jutsu_ranks).
-- ---------------------------------------------------------------------------
CREATE TABLE summon_tribes (
    slug                    TEXT PRIMARY KEY,   -- 'summon/bear'
    name                    TEXT NOT NULL,
    summon_type             TEXT NOT NULL,      -- 'Carnivoran', 'Avian', 'Dragon', ...
    description             TEXT,               -- flavor prose
    toughness               INTEGER,            -- HP formula input: (Toughness + CON mod) * level
    toughness_override      INTEGER,            -- the one field feeding derived math directly
    defensive_ability       TEXT,               -- ability used for AC calc, e.g. 'Constitution'
    saving_throws           TEXT,               -- printed list, comma-separated
    skills                  TEXT,
    senses                  TEXT,
    jutsu_save_dc_text      TEXT,               -- printed formula, e.g. '8 + Strength modifier + ...'
    jutsu_attack_bonus_text TEXT,
    jutsu_specialty_text    TEXT,               -- keyword access list under '<NAME> JUTSU SPECIALTY'

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- A summoner picks one Role per summon (Striker/Caster/Controller/Defender/
-- Lurker/Supporter); tribes list which roles they can take and how each
-- plays out for that tribe specifically. Derived text, rebuilt wholesale.
CREATE TABLE summon_tribe_roles (
    tribe_slug  TEXT NOT NULL REFERENCES summon_tribes(slug),
    role_name   TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (tribe_slug, role_name)
);

-- The 'NATURAL/WEAPONS' section — the tribe's built-in attacks (Claws, Bite,
-- ...). Derived text, rebuilt wholesale.
CREATE TABLE summon_tribe_attacks (
    tribe_slug  TEXT NOT NULL REFERENCES summon_tribes(slug),
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (tribe_slug, name)
);

-- Rank-gated special abilities ('SPECIAL FEATURES: D-RANK/C-RANK/...'). Same
-- override contract as clan_features/class_features.
CREATE TABLE summon_tribe_features (
    slug             TEXT PRIMARY KEY,   -- 'summon/bear/feature/kuma-flex'
    tribe_slug       TEXT NOT NULL REFERENCES summon_tribes(slug),
    name             TEXT NOT NULL,
    rank             TEXT NOT NULL REFERENCES jutsu_ranks(rank),
    description      TEXT NOT NULL,
    sort_order       INTEGER,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

-- The per-rank stat progression table (Rank | Level | Size | ability scores
-- | Jutsu Slots | Jutsu Known | Speed). Ability scores print as literal
-- numbers at D-Rank ("16 10 14 10 12 10") but as an increase formula at
-- every higher rank ("+6 Ability Score Increases up to 20"), and Jutsu
-- Known is free text ("2 D-Rank, 2 C-Rank" vs "B-Rank (or Lower), 1
-- A-Rank."), so everything from Size onward is kept as one printed-text
-- blob rather than forcing a column split the book's own format resists —
-- a future pass can split it further if a query actually needs to. Derived
-- text, rebuilt wholesale.
CREATE TABLE summon_tribe_progression (
    tribe_slug  TEXT NOT NULL REFERENCES summon_tribes(slug),
    rank        TEXT NOT NULL REFERENCES jutsu_ranks(rank),
    level       INTEGER NOT NULL,
    size_text   TEXT,               -- 'M', 'M-L', 'T-S', ... (a range at higher ranks)
    stats_text  TEXT NOT NULL,      -- ability scores/increases, jutsu slots, jutsu known, speed
    PRIMARY KEY (tribe_slug, rank)
);
