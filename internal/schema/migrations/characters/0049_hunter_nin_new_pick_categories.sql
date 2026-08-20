-- Widens character_hunter_nin_picks' category CHECK to cover 6 more
-- Hunter's Creeds' own 3rd-level (cap 1, ->2@10th unless noted) catalog
-- picks, previously entirely untracked. Every one of these tables is
-- mistagged in rules.db onto an unrelated 7th/17th-level feature's own
-- description row rather than the granting feature's own row — see
-- cmd/n5e/hunter_nin.go's own option-catalog doc comments for exactly
-- which row each was transcribed from:
--   warden_weapon             Blade Warden, Warden's Proficiency (3rd
--                              level): a single simple, non-Two-Handed/
--                              non-Heavy weapon type (dynamic, sourced
--                              from the equipment table, not hand-
--                              curated), cap 1, no re-cap
--   warden_weapon_property     Blade Warden, same feature: Flurry Attack/
--                              Mortal/Fatal, cap 1->2@10th
--   medical_technique          Necrotic Hand, Medical Proficiency (3rd):
--                              9-option Medical Assassination Technique
--                              table, cap 1->2@10th
--   shadow_technique            Grave Stalker, Stalkers Proficiency (3rd):
--                              8-option Shadow Assassination Technique
--                              table, cap 1->2@10th
--   arsenal_item                Arsenalist, Arsenal's Proficiency (3rd):
--                              14-option Arsenal catalog (3 of which grant
--                              a free jutsu on pick), cap 4->6@10th
--   toxic_technique             Undertaker, Toxic Proficiency (3rd):
--                              8-option Toxic Assassination Technique
--                              table, cap 1->2@10th
--   vice_technique               Vice Agent, Sin's Proficiency (3rd):
--                              6-option Vice Assassination Technique
--                              table, cap 1->2@10th
--   void_technique               Void Walker, Stalker Proficiency (3rd):
--                              5-option Void Assassination Technique
--                              table, cap 1->2@10th
--   prosthetic_attachment       Wolves Legacy, Wolf's Proficiency (3rd):
--                              7-option Prosthetic Attachments table, cap
--                              2->4@10th, plus its own Proficiency-Bonus-
--                              per-rest use pool (customResourceGrants,
--                              key 'prosthetic_attachments')
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0046_science_nin_subclass_picks_more_categories.sql/
-- 0048_scout_nin_maneuvers_category.sql already use.
CREATE TABLE character_hunter_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'pattern', 'exploit', 'defensive_tactic',
                     'warden_weapon', 'warden_weapon_property',
                     'medical_technique', 'shadow_technique', 'arsenal_item',
                     'toxic_technique', 'vice_technique', 'void_technique',
                     'prosthetic_attachment')),
    option_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

INSERT INTO character_hunter_nin_picks_new (id, character_id, category, option_slug, created_at)
SELECT id, character_id, category, option_slug, created_at
FROM character_hunter_nin_picks;

DROP TABLE character_hunter_nin_picks;
ALTER TABLE character_hunter_nin_picks_new RENAME TO character_hunter_nin_picks;

CREATE INDEX idx_character_hunter_nin_picks_character ON character_hunter_nin_picks(character_id);
