-- Tracks the player-CHOSEN half of elemental (Nature Release) affinities —
-- the flat, no-choice clan grants (e.g. Uchiha always has Fire) need no
-- storage at all, computed fresh every time from a curated table (see
-- cmd/n5e/elemental_affinity.go). This table only exists for the affinities
-- that are a genuine choice: a combo clan's 1st-level "pick one of two"
-- trait (Bakuton, Futton, Jiton, Keton, Namikaze, Ranton, Senju, Shakuton,
-- Yoton, Yuki all share this shape — the OTHER element of the pair is
-- granted automatically at 7th level, needing no storage either), the
-- Nature Release feat's own pick, and Ninjutsu Specialist's Professor
-- subclass (Versatile Release / Twin Cast / Soshikage, three independent
-- picks at levels 2/6/14).
--
-- One row per independent choice ("slot"), not per character — slot_key
-- identifies WHICH choice this is ('clan', 'nature-release-feat',
-- 'versatile-release', 'twin-cast', 'soshikage'), so a character with both
-- a combo clan AND the Professor subclass can hold multiple independent
-- picks without them colliding. PRIMARY KEY enforces exactly one element
-- chosen per slot; changing a pick is a plain upsert, not add-then-remove.
CREATE TABLE character_elemental_affinities (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    slot_key     TEXT NOT NULL,
    element      TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (character_id, slot_key)
);
