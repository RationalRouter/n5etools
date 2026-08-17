-- Tracks the player-CHOSEN half of class/subclass/clan features whose grant
-- involves a pick this app can't resolve on its own — "choose one skill",
-- Nara's Masterwork Skill (two independent picks, at 7th and again at 18th
-- level), Interrogationist's Perfect Mind (Wisdom or Charisma). Same shape
-- as character_elemental_affinities: one row per independent choice
-- ("slot"), not per character, so a feature offering more than one pick
-- (Nara's two 7th-level skills) gets two slots rather than colliding on one.
--
-- feature_slug is the granting class_features/subclass_features/
-- clan_features row's own slug; choice_index distinguishes multiple
-- independent slots the SAME feature offers (0-based, in the order the book
-- lists them — see internal/features' own choiceSlotDefs for what each
-- index means for a given feature). value is a skill name or a 3-letter
-- ability code, matching character_proficiencies.value's own conventions —
-- internal/features.ResolveChoiceGrants reads it back the same way
-- ResolveProficiencyGrants reads the curated fixed-grant tables.
CREATE TABLE character_feature_choices (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    feature_slug TEXT NOT NULL,
    choice_index INTEGER NOT NULL,
    value        TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (character_id, feature_slug, choice_index)
);
