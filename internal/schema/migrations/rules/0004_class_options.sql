-- Class option lists: named pick-lists printed inside the class chapters —
-- Malleable Mirages, Arbiter Maneuvers, S.N.B Upgrades, Genjutsu Inception…
-- Some belong to one archetype (archetype_slug set), others to the whole
-- class. The creation flow presents them as selectable entries; characters
-- record picks by slug.
CREATE TABLE class_options (
    slug             TEXT PRIMARY KEY,          -- 'class/scout-nin/option/arbiter-maneuvers/assisted-accuracy'
    class_slug       TEXT NOT NULL REFERENCES classes(slug),
    archetype_slug   TEXT REFERENCES archetypes(slug),  -- NULL = class-wide list
    list_name        TEXT NOT NULL,             -- 'Arbiter Maneuvers'
    name             TEXT NOT NULL,
    prerequisites    TEXT,                      -- 'Prerequisite: …' line, when printed
    description      TEXT NOT NULL,
    sort_order       INTEGER,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
