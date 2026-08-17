-- Sibling of character_feature_choices (0028), scoped one level deeper: a
-- player pick that a granted CHARACTER feature offers, but that applies to
-- one specific COMPANION rather than the character itself. Puppet Master's
-- Symphony of Puppetry Enhancement branch ("The two Puppets you started
-- with... increase one of their ability scores by +2, EACH") is the first
-- feature that needs this — a single character-level feature grant, but a
-- genuinely independent pick per companion (each of the two Puppets can
-- have a different ability boosted).
--
-- feature_slug/choice_index mean the same thing character_feature_choices'
-- own columns do; companion_id narrows the pick to one specific
-- character_companions row. value is a 3-letter ability code, matching
-- character_feature_choices' own value conventions for an ability pick.
CREATE TABLE character_feature_companion_choices (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    feature_slug TEXT NOT NULL,
    companion_id INTEGER NOT NULL REFERENCES character_companions(id) ON DELETE CASCADE,
    choice_index INTEGER NOT NULL,
    value        TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (character_id, feature_slug, companion_id, choice_index)
);
