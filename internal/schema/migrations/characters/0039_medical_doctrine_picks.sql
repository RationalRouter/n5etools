-- Tracks Medical Doctrine's 4-option catalog pick (cap 1 @3rd -> 2 @13th).
-- The 4 options (Long Life Short Death / Never on the Front Lines / Not
-- Allowed to Die / Until Their Heart Stops) are read live off rules.db's
-- own class_features rows (NULL level, sort_order 6-9, sitting between
-- Chakra Scalpel and ASI/Feat) rather than hand-transcribed — see
-- cmd/n5e/medical_nin.go's loadMedicalDoctrineCatalog. A single-purpose
-- table (no category discriminator) since this is Medical-Nin's only
-- cap-gated catalog pick, the same shape character_martial_techniques
-- already established for Taijutsu Specialist's own single catalog.
--
-- No FK into rules.db's class_features (same cross-DB slug-reference
-- tolerance every other rules.db-keyed table in this schema already
-- uses). UNIQUE on (character_id, option_slug): a character either has
-- chosen a given doctrine or hasn't. RAW says this choice can't be changed
-- once made, but this app doesn't enforce pick-timing anywhere else either
-- (Hunters Exploits, Malleable Mirages, Martial Techniques all allow free
-- re-picks) — see cmd/n5e/medical_nin.go.
CREATE TABLE character_medical_doctrine_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, option_slug)
);

CREATE INDEX idx_character_medical_doctrine_picks_character ON character_medical_doctrine_picks(character_id);
