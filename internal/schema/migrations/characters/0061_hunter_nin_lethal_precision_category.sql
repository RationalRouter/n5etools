-- Widens character_hunter_nin_picks' category CHECK to cover Lethal
-- Precision (class/hunter-nin/feature/lethal-precision, 1st level): "Select
-- one between Taijutsu & Bukijutsu. You can cast the chosen Jutsu type using
-- Dexterity in place of Strength for all calculations. You cannot switch
-- this choice later." Two fixed option slugs ('taijutsu'/'bukijutsu'), not
-- class_options rows — the book states both choices directly in the
-- granting feature's own text, same as defensive_tactic's own hand-curated
-- shape. cap 1, no re-cap: the "cannot switch this choice later" clause is
-- enforced the same way warden_weapon's own no-re-cap pick already is —
-- handleHunterPickAdd rejects any add once the category is at its cap of 1,
-- so once taken the pick can only ever be forgotten and re-picked, never
-- swapped out from under an existing choice mid-add.
--
--   lethal_precision   Hunter-Nin base class, Lethal Precision (1st level):
--                      2-option Taijutsu/Bukijutsu table, cap 1, no re-cap.
--                      The pick itself is tracked; the Dexterity-for-
--                      Strength swap it grants is applied automatically to
--                      the matching JutsuAttacks entry (charsheet.go), same
--                      as the Genjutsu Charisma-swap feats already are.
--
-- SQLite can't ALTER a CHECK constraint in place — rebuild the table, same
-- pattern 0055_hunter_nin_wolf_technique_category.sql already used.
CREATE TABLE character_hunter_nin_picks_new (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN (
                     'pattern', 'exploit', 'defensive_tactic',
                     'warden_weapon', 'warden_weapon_property',
                     'medical_technique', 'shadow_technique', 'arsenal_item',
                     'toxic_technique', 'vice_technique', 'void_technique',
                     'prosthetic_attachment', 'wolf_technique',
                     'lethal_precision')),
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
