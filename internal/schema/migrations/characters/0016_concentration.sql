-- First real slice of concentration tracking (the jutsu-casting economy's
-- second-to-last unbuilt piece, after chakra-spend casting and upcasting).
--
-- Only one concentration slot exists in the base rules and there's no
-- evidence of any class feature granting a second one, so character_id is
-- the primary key: exactly one active concentration per character, enforced
-- at the DB level rather than left to app-code discipline. Casting a second
-- concentration jutsu is meant to silently replace the first (the book's own
-- single-slot rule) — an upsert against this key is what makes that fall out
-- for free instead of needing an explicit "delete the old row first" step.
--
-- No denormalized jutsu name, matching character_jutsu's own convention of
-- storing only the slug and looking the name up from rules.db at render
-- time — same "skip/fall back rather than break" tolerance for a slug that
-- goes stale after a rules update.
CREATE TABLE character_concentration (
    character_id  INTEGER PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
    jutsu_slug    TEXT NOT NULL,
    cast_at_rank  TEXT NOT NULL,
    started_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
