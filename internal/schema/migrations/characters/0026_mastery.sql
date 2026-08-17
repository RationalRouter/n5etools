-- Mastery (core book, Chapter 6, page 49): "Some class and clan features,
-- Jutsu or even environmental circumstances will grant you Mastery with a
-- given skill or toolkit... Sources of Mastery: One Source: Rank 1 Mastery
-- (+2), Two Sources: Rank 2 Mastery (+4), Three Sources: Rank 3 Mastery
-- (+6)." The book's own cap is level-gated (1-6: max Rank 1, 7-11: max
-- Rank 2, 12+: max Rank 3) and stacks from an open-ended list of possible
-- SOURCES (specific class/clan features, feats, jutsu, "environmental
-- circumstances") that this app has no way to exhaustively enumerate or
-- auto-detect across every rules table — so, same shape as Generalized
-- Skill/Puppet Tactics, this is a direct player pick (which skill/toolkit,
-- what rank) gated only by the book's own level cap, not an attempt to
-- derive "how many real sources" a player has from their own feature list.
CREATE TABLE character_mastery (
    id            INTEGER PRIMARY KEY,
    character_id  INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    skill_name    TEXT NOT NULL,
    rank          INTEGER NOT NULL CHECK (rank BETWEEN 1 AND 3),
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, skill_name)
);

CREATE INDEX idx_character_mastery_character ON character_mastery(character_id);
