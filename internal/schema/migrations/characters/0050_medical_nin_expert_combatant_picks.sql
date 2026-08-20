-- Tracks Combat Medic's Expert Combatant pick (9th level: "select one
-- taijutsu or bukijutsu you know. The selected jutsu gains the Medical
-- Keyword. You may select an additional taijutsu or bukijutsu at 13th and
-- 17th level"; cap 1 -> 2@13th -> 3@17th, hand-curated the same way
-- ninjutsuMasterCap is — no class_level_resources chart exists for it).
--
-- Sources its Available list from the character's OWN known jutsu
-- (character_jutsu) rather than a rules-database catalog, the same shape
-- character_ninjutsu_jutsu_picks (migration 0041) established for Refined
-- Ninjutsu/Ninjutsu Master — jutsu_id references character_jutsu.id (not a
-- rules slug, since a known jutsu may be a player-created custom_jutsu with
-- no slug of its own), ON DELETE CASCADE so forgetting/unlearning that
-- jutsu automatically drops the pick that pointed at it.
--
-- No category discriminator: Expert Combatant is Medical-Nin's only pick of
-- this shape, so this table is single-purpose like
-- character_medical_doctrine_picks rather than category-keyed like
-- character_ninjutsu_jutsu_picks.
--
-- What "gains the Medical Keyword" actually does downstream, and the same
-- feature's separate "attack twice, instead of once" clause, both stay
-- manual/narrated — no attacks-per-action field exists anywhere in this
-- app for any class, and Medical Keyword eligibility isn't itself a
-- trackable pool. Only WHICH jutsu is currently selected becomes tracked,
-- real state — see cmd/n5e/medical_nin.go.
CREATE TABLE character_medical_nin_expert_combatant_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    jutsu_id     INTEGER NOT NULL REFERENCES character_jutsu(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, jutsu_id)
);

CREATE INDEX idx_character_medical_nin_expert_combatant_picks_character ON character_medical_nin_expert_combatant_picks(character_id);
