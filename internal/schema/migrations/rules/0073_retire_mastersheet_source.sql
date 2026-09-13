-- Retires the community Mastersheet as a source of record.
--
-- The class progression charts and the weapon/armor rows that appear in no
-- printed price table are printed only as images, so no parser produces
-- them. They were bootstrapped out of the community Mastersheet xlsx
-- ('sheet/mastersheet'); that file is no longer read at any point. The data
-- now lives in this repo as internal/classcharts' JSON, loaded by
-- `n5e-ingest charts`, and the provenance row is renamed to match.
--
-- Only `equipment` ever carried 'sheet/mastersheet' in a source_book column
-- (verified by scanning every table with an FK to source_books before
-- writing this migration — 89 rows there, zero everywhere else), so the
-- re-point below is the complete set. The insert-then-update-then-delete
-- order matters: equipment.source_book is a real FK to source_books(slug)
-- and PRAGMA foreign_keys is ON during migrations (internal/schema.Apply),
-- so the new parent row has to exist before any child points at it, and the
-- old parent can only be dropped once nothing references it any more.
--
-- file_sha256 is required NOT NULL but has no meaning for a directory of
-- JSON that is version-controlled rather than downloaded; it records what
-- the row actually is instead of a digest of a file that no longer exists.
INSERT INTO source_books (slug, title, version, file_name, file_sha256)
VALUES ('curated/class-charts', 'N5E Toolkit curated charts', '1.0',
        'internal/classcharts', 'n/a — version-controlled in-repo data')
ON CONFLICT(slug) DO NOTHING;

UPDATE equipment
SET source_book = 'curated/class-charts'
WHERE source_book = 'sheet/mastersheet';

DELETE FROM source_books WHERE slug = 'sheet/mastersheet';
