-- Tracks which of Taijutsu Specialist's 20 class-wide Martial Techniques
-- (see class_options, list_name='Martial Techniques' — Martial Adept grants
-- these, cmd/n5e/taijutsu.go builds the picker) a character has learned.
-- Only player-picked techniques get a row here: the ~16 bespoke techniques
-- subclasses auto-grant for free (e.g. Passionate Flame's Enhanced Flurry
-- granting "Chakra Enhanced Blows"/"Dynamic Set-Up") are computed live from
-- granted features instead, the same "derive, don't store" treatment free
-- jutsu grants already get (see internal/features and jutsu_grants.go) —
-- they have no class_options catalog row of their own to reference here.
--
-- No FK into rules.db's class_options (separate SQLite file, same
-- cross-DB slug-reference tolerance every other rules.db-keyed table in
-- this schema already uses). UNIQUE on (character_id, option_slug): a
-- character either knows a given technique or doesn't.
CREATE TABLE character_martial_techniques (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, option_slug)
);

CREATE INDEX idx_character_martial_techniques_character ON character_martial_techniques(character_id);
