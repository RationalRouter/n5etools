-- Tracks four of Science-Nin's per-subclass cap-gated catalog picks, each a
-- slot count (not a spent Creation Points budget the way the base
-- Scientific Ninja Tools catalog works — see character_science_nin_tools):
--   eip              Elemental Innovationist, class_options 'E.I.Ps',
--                     cap 2 -> 3@6th -> 4@9th
--   wow              Elemental Innovationist, class_options
--                     'W.O.W (Weapons of Wonder)', cap 1 -> 2@17th
--   perma_perk       Elemental Innovationist, 14th level: one of the
--                     character's own known 'eip' picks (excluding Wonder
--                     E.I.P) made permanent; option_slug always duplicates
--                     an existing 'eip' row's own slug
--   bim              Grenadier, class_options 'Explosive Modifications',
--                     cap = Proficiency Bonus
--   inversion_serum  Mad Scientist, class_options 'Inversion Serums',
--                     cap = Intelligence modifier; pool records which half
--                     of the character's split Chakra Containment Device
--                     (see characters.ccd_mending_pct) paid for it, since
--                     each serum's own effect depends on that choice
--   arsenal_mod      Ninjaneer, class_options 'Arsenal Modifications',
--                     cap = Proficiency Bonus (Enhanced Arsenal's own
--                     Upgrade Slots)
--   perfected_weapon Ninjaneer, cap = Intelligence modifier (A Weapon to
--                     Surpass); option_slug points at the same Arsenal
--                     Modifications catalog, restricted to its Minor tier,
--                     granted at no Creation Points cost — an independent
--                     cap from arsenal_mod, not drawn from the same pool of
--                     slots
--
-- One generic table with a category discriminator, the same shape
-- character_hunter_nin_picks/character_genjutsu_picks/
-- character_intelligence_operative_picks already establish for their own
-- simultaneous class-level picks. option_slug points at a
-- class_option_entries row (see internal/store/classoptionentries.go's
-- split of each tier's bundled description into one row per named
-- upgrade), not a class_options row directly, matching
-- character_science_nin_tools — except wow, whose 9 rows are already
-- individually named class_options rows with no bundling to split.
--
-- No FK into rules.db's class_options/class_option_entries (same
-- cross-DB slug-reference tolerance every other rules.db-keyed table in
-- this schema already uses). UNIQUE on (character_id, category,
-- option_slug): a character either holds a given pick or doesn't — this
-- means a character cannot hold the same named Inversion Serum twice
-- financed from both CCD pools at once, only one copy per name with pool
-- recording whichever CCD half most recently paid for it.
CREATE TABLE character_science_nin_subclass_picks (
    id           INTEGER PRIMARY KEY,
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    category     TEXT NOT NULL CHECK (category IN ('eip', 'wow', 'perma_perk', 'bim', 'inversion_serum', 'arsenal_mod', 'perfected_weapon')),
    option_slug  TEXT NOT NULL,
    pool         TEXT NOT NULL DEFAULT '' CHECK (pool IN ('', 'mending', 'maiming')),
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (character_id, category, option_slug)
);

CREATE INDEX idx_character_science_nin_subclass_picks_character ON character_science_nin_subclass_picks(character_id);
