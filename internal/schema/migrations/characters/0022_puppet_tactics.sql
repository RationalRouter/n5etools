-- Tracks which of Puppet Master's 5 named Tactics (Agile/Defensive/Helpful/
-- Offensive/Resourceful — see class_features under class/puppet-master, and
-- cmd/n5e/puppet_tactics.go for the picker built on top) a character has
-- chosen. Character-level, not companion-level: Tactics are the player's own
-- backup plan for when their Puppet Tool is destroyed, not something the
-- puppet itself has.
--
-- No FK into rules.db's class_features (separate SQLite file, same
-- cross-DB slug-reference tolerance every other rules.db-keyed table in
-- this schema already uses). UNIQUE on (character_id, tactic_slug): unlike
-- Puppet Upgrades, nothing about a Tactic is repeatable — a character either
-- has Agile Tactics or doesn't.
CREATE TABLE character_puppet_tactics (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    tactic_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, tactic_slug)
);

CREATE INDEX idx_character_puppet_tactics_character ON character_puppet_tactics(character_id);
