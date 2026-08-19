-- Tracks Ninjutsu Specialist's three cap-gated picks: Efficient Molding
-- (base class, level 3, class_options list_name "Efficient Molding", 17
-- rows, cap 2->6 per class_level_resources; see cmd/n5e/ninjutsu_specialist.go),
-- and Refined Ninjutsu (level 1, cap 2->8) / Ninjutsu Master (level 20,
-- cap 1) — both of which pick from the character's OWN already-known
-- jutsu (character_jutsu) rather than a static rules-database catalog, a
-- different shape from every other cap+catalog pick in this schema so far.
--
-- character_ninjutsu_molding_picks mirrors character_medical_doctrine_picks
-- exactly (single-purpose, option_slug into class_options, no category
-- discriminator needed since Efficient Molding is this class's only
-- rules-catalog-backed pick).
--
-- character_ninjutsu_jutsu_picks instead references a SPECIFIC known-jutsu
-- row (character_jutsu.id) rather than a rules slug, since a jutsu the
-- character knows may be a custom_jutsu with no slug at all — ON DELETE
-- CASCADE so forgetting/unlearning that jutsu automatically drops the
-- Refined Ninjutsu/Ninjutsu Master pick that pointed at it, rather than
-- leaving an orphaned pick behind. One table with a category discriminator
-- (same shape character_hunter_nin_picks/character_intelligence_operative_picks
-- already establish for their own simultaneous same-character picks) since
-- Refined Ninjutsu and Ninjutsu Master both point at the same
-- character_jutsu table, just under independent caps.
CREATE TABLE character_ninjutsu_molding_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, option_slug)
);

CREATE INDEX idx_character_ninjutsu_molding_picks_character ON character_ninjutsu_molding_picks(character_id);

CREATE TABLE character_ninjutsu_jutsu_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('refined_ninjutsu', 'ninjutsu_master')),
    jutsu_id     INTEGER NOT NULL REFERENCES character_jutsu(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, jutsu_id)
);

CREATE INDEX idx_character_ninjutsu_jutsu_picks_character ON character_ninjutsu_jutsu_picks(character_id);
