-- Tracks the skills a Puppet Master has chosen for "Generalized Skill"
-- (class_features, class/puppet-master/feature/generalized-skill, granted at
-- 5th level): "select a number of skills equal to 2 + your Intelligence
-- Modifier. Your Puppet Tools may use your skill bonuses for these chosen
-- skills." Character-level, not per-companion — the feature's own text
-- applies the same chosen list to every Puppet Tool the character has (see
-- cmd/n5e/puppet_skills.go for how each companion's own effective bonus is
-- then resolved from this list plus any upgrade-granted additions).
--
-- No FK into rules.db (separate SQLite file, same cross-DB slug-reference
-- tolerance every other rules.db-keyed table in this schema already uses —
-- though here the "slug" is just a skill name, not a rules.db row at all).
-- UNIQUE on (character_id, skill_name): picking the same skill twice is a
-- no-op, not a second copy.
CREATE TABLE character_generalized_skills (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    skill_name   TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, skill_name)
);

CREATE INDEX idx_character_generalized_skills_character ON character_generalized_skills(character_id);
