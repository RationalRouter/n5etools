-- Tracks which of Superior Weapon Flurry's own 4 named benefits (base
-- class, 14th level: select 2; 18th level: select a 3rd — see cmd/n5e/
-- weapon_specialist.go's hardcoded catalog) a Weapon Specialist has
-- selected. No class_options rows exist for these 4 benefits, and the
-- 2-then-3 cap isn't in class_level_resources either — both are hardcoded
-- in Go, the same "small hardcoded catalog+cap" precedent
-- weaponFormTechniqueAutoGrants already sets for this class.
--
-- No FK into rules.db's class_options (separate SQLite file, same
-- cross-DB slug-reference tolerance every other rules.db-keyed table in
-- this schema already uses). UNIQUE on (character_id, option_slug): a
-- character either has a given benefit selected or doesn't.
CREATE TABLE character_superior_weapon_flurry (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, option_slug)
);

CREATE INDEX idx_character_superior_weapon_flurry_character ON character_superior_weapon_flurry(character_id);
