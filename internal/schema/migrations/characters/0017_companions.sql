-- Puppet/Summon companion sheets: a small, separate popup-window sheet a
-- character can open per Puppet Tool or summoned creature they field. Unlike
-- the main sheet, nothing here is derived math — every stat is a plain
-- player-entered field, because a Puppet Master's puppet and a summon
-- tribe's creature each follow their own bespoke stat rules (see
-- summon_tribes.toughness/defensive_ability in rules.db) that this app does
-- not model as a formula. kind fixes which read-only rules-reference panel
-- the companion's own popup shows (see cmd/n5e/companions.go): 'puppet'
-- shows the character's own Puppet Master subclass features gated by their
-- real level, 'summon' shows summon_tribe_slug's rank-gated features gated
-- by the character's Highest Rank Known, 'custom' shows neither.
--
-- One character can field more than one companion (a Puppet Master fields
-- more than one Puppet Tool over a campaign; a character can hold more than
-- one summoning contract), so this is a rows table keyed by its own id, the
-- same shape as character_custom_features, not a single-row-per-character
-- table like character_concentration.
CREATE TABLE character_companions (
    id                INTEGER PRIMARY KEY,
    character_id      INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('puppet', 'summon', 'custom')),
    name              TEXT NOT NULL DEFAULT '',

    -- rules.db summon_tribes.slug; only meaningful (and player-editable)
    -- when kind = 'summon'. Not a foreign key — summon_tribes lives in a
    -- separate SQLite database file, same cross-DB-slug-reference tolerance
    -- character_jutsu already uses for jutsu_slug.
    summon_tribe_slug TEXT NOT NULL DEFAULT '',

    ac                INTEGER,
    hp_current        INTEGER,
    hp_max            INTEGER,
    speed             INTEGER,
    str_score         INTEGER,
    dex_score         INTEGER,
    con_score         INTEGER,
    int_score         INTEGER,
    wis_score         INTEGER,
    cha_score         INTEGER,

    -- Free-text blocks, same "whole field, on blur" autosave as the Bio
    -- tab's five columns (migration 0004) — attacks and traits vary too
    -- much by puppet chassis/summon tribe to model as structured rows.
    attacks           TEXT NOT NULL DEFAULT '',
    traits            TEXT NOT NULL DEFAULT '',
    notes             TEXT NOT NULL DEFAULT '',

    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_character_companions_character ON character_companions(character_id);
