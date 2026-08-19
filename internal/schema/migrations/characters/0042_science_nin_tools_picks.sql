-- Tracks Science-Nin's base-class Scientific Ninja Tools pick: a Creation
-- Points budget (class_level_resources, 4 @2nd -> 40 @20th) spent across a
-- 6-tier catalog (Minor/Refined/Greater/Superior/Supreme/Mastercraft, each
-- a class_options row whose own description bundles several individually
-- named tools) rather than a flat per-pick slot count the way every other
-- cap+catalog pick in this schema (Martial Techniques, Hunters Patterns,
-- Efficient Molding, ...) already uses. option_slug points at a
-- class_option_entries row (see internal/store/classoptionentries.go's own
-- split of each tier's bundled description into one row per named tool),
-- not a class_options row directly — the same "entries, not tiers" shape
-- Puppet Master's own Upgrade picks (character_puppet_upgrades) use, cost
-- read live off each entry's own "Cost: N Creation Points" text rather than
-- hand-transcribed (see cmd/n5e/science_nin.go).
--
-- Single-purpose table (no category discriminator) since this is the only
-- class-wide catalog pick built so far — S.N.B Upgrades and every other
-- subclass's own tiered catalog (Shinobi-Ware Upgrades, Arsenal
-- Modifications, ...) are a later stage's own scope, and may reuse this
-- exact shape (their own table) when built. UNIQUE on
-- (character_id, option_slug): a character either owns a given tool or
-- doesn't. No pick-timing enforced (freely learned/forgotten at any time),
-- same "trust the player" boundary every other pick table in this schema
-- draws.
CREATE TABLE character_science_nin_tools (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, option_slug)
);

CREATE INDEX idx_character_science_nin_tools_character ON character_science_nin_tools(character_id);
