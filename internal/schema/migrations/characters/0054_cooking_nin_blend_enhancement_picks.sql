-- Tracks Gastrochemist's (Cooking-Nin) Nature's Blend Enhancement picks —
-- "choose a Jutsu of your Nature's Blend release, and give it one of the
-- following Enhancements" (2nd level, cap 1->6 across 2nd/3rd/5th/9th/13th/
-- 17th level; see cmd/n5e/cooking_nin.go). Same shape as
-- character_ninjutsu_jutsu_picks (0041_ninjutsu_specialist_picks.sql): the
-- pick references a SPECIFIC known-jutsu row (character_jutsu.id) rather
-- than a rules slug, since a jutsu the character knows may be a
-- player-created custom_jutsu with no slug at all — ON DELETE CASCADE so
-- forgetting/unlearning that jutsu automatically drops its Enhancement pick
-- too, rather than leaving an orphaned pick behind. One extra dimension
-- beyond that precedent: each pick also records WHICH of the 4 Enhancement
-- types (Texture/Kick/Temperature/Aroma) was chosen for that jutsu, via
-- enhancement_type — a jutsu can only be given one Enhancement, hence the
-- UNIQUE (character_id, jutsu_id) rather than a category discriminator
-- allowing repeats the way character_ninjutsu_jutsu_picks' own category
-- column does.
CREATE TABLE character_cooking_nin_blend_enhancement_picks (
    id               INTEGER PRIMARY KEY,
    character_id     INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    jutsu_id         INTEGER NOT NULL REFERENCES character_jutsu(id) ON DELETE CASCADE,
    enhancement_type TEXT NOT NULL CHECK (enhancement_type IN ('texture', 'kick', 'temperature', 'aroma')),
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, jutsu_id)
);

CREATE INDEX idx_character_cooking_nin_blend_enhancement_picks_character ON character_cooking_nin_blend_enhancement_picks(character_id);
