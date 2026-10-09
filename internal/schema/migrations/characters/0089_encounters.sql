-- GM-saved adversary encounters, generated from the Bingo Book's Rank/Role/
-- Role Trait/Freeform Slot toolkit (internal/encounter). Lives in
-- characters.db, the app's only writable/persistent file -- rules.db is
-- extracted to a fresh read-only temp file every run, so nothing written
-- there would survive a restart.
--
-- rank_slug/role_slug/trait_slug are cross-database references into
-- rules.db's adversary_ranks/adversary_roles/adversary_role_traits tables
-- (separate sql.DB connections, so SQLite cannot enforce a real FK across
-- them -- same convention already used for custom_items' references into
-- rules.db content). The rest of each row is a snapshot of that content's
-- display data at generation time, not a live join, so a saved encounter
-- stays stable even if the Bingo Book is later re-ingested or edited.
CREATE TABLE encounters (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    party_size   INTEGER NOT NULL,
    party_level  INTEGER NOT NULL,
    difficulty   TEXT NOT NULL CHECK (difficulty IN ('easy','standard','hard','deadly')),
    budget       INTEGER NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE encounter_adversaries (
    id             INTEGER PRIMARY KEY,
    encounter_id   INTEGER NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    name           TEXT NOT NULL,
    rank_slug      TEXT NOT NULL,
    rank_name      TEXT NOT NULL,
    role_slug      TEXT NOT NULL,
    role_name      TEXT NOT NULL,
    level          INTEGER NOT NULL,
    hp_formula     TEXT NOT NULL DEFAULT '',
    ac_bonus       INTEGER NOT NULL DEFAULT 0,
    save_bonus     INTEGER NOT NULL DEFAULT 0,
    save_dc_bonus  INTEGER NOT NULL DEFAULT 0,
    init_bonus     INTEGER NOT NULL DEFAULT 0,
    freeform_slots INTEGER NOT NULL DEFAULT 0,
    budget_cost    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_encounter_adversaries_encounter ON encounter_adversaries(encounter_id);

CREATE TABLE encounter_adversary_traits (
    adversary_id      INTEGER NOT NULL REFERENCES encounter_adversaries(id) ON DELETE CASCADE,
    trait_slug        TEXT NOT NULL,
    trait_name        TEXT NOT NULL,
    trait_rank        TEXT NOT NULL CHECK (trait_rank IN ('D','C','B','A','S')),
    trait_description TEXT NOT NULL DEFAULT '',
    sort_order        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (adversary_id, trait_slug)
);
