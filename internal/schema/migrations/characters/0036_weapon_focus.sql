-- Tracks a Weapon Specialist's chosen Weapon Focus weapon type(s) (class
-- feature "Weapon Focus", level 1: 1 type; a 2nd at 9th level; a 3rd at
-- 15th). "Type" in the book's own text means an equipment catalog entry
-- (e.g. "Katana") — every weapon sharing that entry's slug counts, not one
-- specific inventory item — so this stores equipment.slug, not an
-- inventory_id, and buildAttacks (cmd/n5e/characters.go) matches any
-- equipped weapon whose slug is in the set.
--
-- Only the flat Attack & Damage bonus this feature grants is modeled here
-- (see cmd/n5e/weapon_specialist.go's doc comment for why the bonus trait
-- chart / Dex-for-Str substitution nuances are left as reference text —
-- this app doesn't mechanically simulate weapon properties anywhere else
-- either, so there's no field for a granted trait to reach).
--
-- No FK into rules.db's equipment table (separate SQLite file, same
-- cross-DB slug-reference tolerance every other rules.db-keyed table in
-- this schema already uses). UNIQUE on (character_id, weapon_slug): the
-- same type can't be picked twice. RAW gates re-picking to "you cannot go
-- back and change your selection" — freely re-editable here instead, same
-- "trust the player" boundary Mastery/Puppet Tactics/Martial Techniques
-- already draw.
CREATE TABLE character_weapon_focus (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    weapon_slug  TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, weapon_slug)
);

CREATE INDEX idx_character_weapon_focus_character ON character_weapon_focus(character_id);
