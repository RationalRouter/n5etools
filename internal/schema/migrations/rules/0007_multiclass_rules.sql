-- Multiclassing rules (core book, filed under its own "Legacy Rules"
-- chapter — the author has since replaced multiclassing with the Archetype
-- system, but the toolkit supports it as its own feature; see project
-- notes). One row per class; conditions are kept as printed text
-- ("Strength or Dexterity 14 & Constitution 14") rather than a bespoke
-- grammar for an 11-row reference table — a future rules-engine pass can
-- parse further if the multiclass creation flow needs to evaluate them.
CREATE TABLE class_multiclass_rules (
    class_slug           TEXT PRIMARY KEY REFERENCES classes(slug),
    ability_prereq_text  TEXT NOT NULL,  -- ability score minimums to multiclass INTO this class
    proficiencies_text   TEXT NOT NULL,  -- the partial proficiency set gained (not first-level full set)
    jutsu_per_level_text TEXT NOT NULL,  -- e.g. '+1 Every 2 Levels.'

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
