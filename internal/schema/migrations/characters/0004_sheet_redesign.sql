-- Character sheet redesign: HP/THP editing, inspiration toggle, the Bio
-- tab's free-text fields, a custom-features panel alongside the auto-seeded
-- class/clan features, and a persisted chat/dice-log.

-- base_temp_hp is the THP fraction's denominator (set only by typing a new
-- value into the dedicated "Base Temp HP" box) — it does NOT move when
-- temp_hp itself drops from absorbing damage, so 10/10 THP taking 10 damage
-- reads 0/10, not 0/0.
ALTER TABLE characters ADD COLUMN base_temp_hp INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN inspiration INTEGER NOT NULL DEFAULT 0 CHECK (inspiration IN (0,1));
ALTER TABLE characters ADD COLUMN allies_organizations TEXT;
ALTER TABLE characters ADD COLUMN additional_features_text TEXT;
ALTER TABLE characters ADD COLUMN treasure TEXT;

-- Core tab's features panel renders real class_features/clan_features rows
-- straight from rules.db alongside these — this table is only the
-- player-added custom/homebrew entries next to them (the "+" button).
CREATE TABLE character_custom_features (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    source_label TEXT NOT NULL,   -- free text, e.g. "Other: Magic Item"
    description  TEXT NOT NULL,
    sort_order   INTEGER NOT NULL
);

-- Persisted chat/dice-log. 'roll' rows store the already-formatted notation
-- line — the dice math itself is computed once, client-side, in
-- dice-roller.js's showResult; this table is just the durable record of it,
-- not a second source of truth.
CREATE TABLE character_chat_log (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('message','roll')),
    text         TEXT NOT NULL,
    crit         TEXT NOT NULL DEFAULT 'none' CHECK (crit IN ('none','nat20','nat1')),
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
