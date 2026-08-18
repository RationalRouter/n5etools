-- Tracks which of a Weapon Specialist's chosen Weapon Form's "[Form]
-- Styles" (see class_options, subclass_slug set per Form — cmd/n5e/
-- weapon_specialist.go builds the picker) a character has learned. Every
-- Weapon Form's 3rd-level "[Form] Techniques" feature (Battle Techniques,
-- Kenjutsu Technique, ...) auto-grants its own 2-3 bespoke Flurry
-- Techniques for free instead — computed live from granted features, same
-- "derive, don't store" treatment Taijutsu Specialist's Martial Technique
-- auto-grants already get (see 0030_martial_techniques.sql) — they have no
-- class_options catalog row of their own to reference here.
--
-- No FK into rules.db's class_options (separate SQLite file, same
-- cross-DB slug-reference tolerance every other rules.db-keyed table in
-- this schema already uses). UNIQUE on (character_id, option_slug): a
-- character either knows a given Style or doesn't.
CREATE TABLE character_weapon_form_styles (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, option_slug)
);

CREATE INDEX idx_character_weapon_form_styles_character ON character_weapon_form_styles(character_id);
