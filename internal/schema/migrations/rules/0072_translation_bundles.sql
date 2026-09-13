-- Tracks, per locale, the last centrally-translated bundle this install has
-- applied (see internal/autoupdate's translation-bundle fetch/apply logic
-- and internal/i18n's cmd/n5e-i18n translate-book pipeline). Deliberately
-- separate from source_books.drive_last_modified/ingest_schema_version:
-- those track when a book's CONTENT was last ingested, not when its
-- TRANSLATIONS were last regenerated, and the two can legitimately drift —
-- English content can be re-ingested well before a maintainer gets around to
-- running a fresh translation pass over it.
CREATE TABLE translation_bundles (
    locale                TEXT PRIMARY KEY,
    ingest_schema_version TEXT NOT NULL,
    etag                  TEXT NOT NULL,
    applied_at            TEXT NOT NULL DEFAULT (datetime('now'))
);
