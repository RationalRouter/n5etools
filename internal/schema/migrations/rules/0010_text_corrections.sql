-- Audit trail for n5e-ingest's automatic prose correction (misspell +
-- LanguageTool + a list-context apostrophe heuristic). NOT a review queue:
-- corrections are applied automatically, no detection_status column on
-- purpose, so this table never appears in the validation report (which only
-- discovers tables that have one). Exists purely so a maintainer can diff
-- what changed if something in the shipped text looks wrong. Append-only.
CREATE TABLE text_corrections (
    id             INTEGER PRIMARY KEY,
    source_book    TEXT NOT NULL REFERENCES source_books(slug),
    source_version TEXT,
    entity_type    TEXT NOT NULL,          -- Go struct name, e.g. 'ClanTrait'
    entity_path    TEXT NOT NULL,          -- audit-only label, e.g. 'hebi-clan/weapon-proficiencies'
    field          TEXT NOT NULL,          -- struct field name, e.g. 'Description'
    tool           TEXT NOT NULL CHECK (tool IN ('misspell','languagetool','apostrophe')),
    rule_id        TEXT,                   -- LanguageTool rule id; NULL for misspell/apostrophe
    original       TEXT NOT NULL,
    corrected      TEXT NOT NULL,
    applied_at     TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (source_book, entity_type, entity_path, field, original, corrected)
);
CREATE INDEX idx_text_corrections_book ON text_corrections(source_book);
