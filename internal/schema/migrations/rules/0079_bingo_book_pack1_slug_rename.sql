-- Renames the official adversary-building rules chapter's provenance slug
-- from 'book/bingo-book' to 'book/bingo-book-pack-1'.
--
-- The Bingo Book is the community's actual bestiary/encounter-building
-- resource; the chapter previously ingested under 'book/bingo-book'
-- (Ranks, Roles, Role Traits, Freeform tables, the General Traits-adjacent
-- glossaries — migrations 0024/0076/0077) is only its latest official
-- update layer, not the book itself. This frees the 'book/bingo-book' slug
-- for the actual 70-page community PDF (internal/sources.OfficialBooks),
-- which from here on is treated as the base document and always wins the
-- naming slot; the chapter keeps its data unchanged under its new slug.
--
-- Same insert-then-update-then-delete order as 0073_retire_mastersheet_
-- source.sql and for the same reason: PRAGMA foreign_keys is ON during
-- migrations (internal/schema.Apply), so the new parent row must exist
-- before any child is repointed at it, and the old parent can only be
-- dropped once nothing references it any more. On a fresh install that has
-- never ingested this chapter, source_books has no 'book/bingo-book' row
-- yet and every statement below is a no-op — the next ingest simply writes
-- 'book/bingo-book-pack-1' directly (internal/ingest.BingoBookPack1).
INSERT INTO source_books (slug, title, version, version_date, file_name, file_sha256, ingested_at)
SELECT 'book/bingo-book-pack-1', title, version, version_date, file_name, file_sha256, ingested_at
FROM source_books WHERE slug = 'book/bingo-book';

-- FK-backed children (source_books(slug) REFERENCES). Includes the two
-- generic cross-book tables every parser writes into (ocr_anomalies,
-- text_corrections), not just the Bingo-Book-specific ones — found by
-- scanning every table with a source_book column against a real ingested
-- rules.db before trusting this list, the same audit discipline
-- 0073_retire_mastersheet_source.sql's own comment describes.
UPDATE adversary_roles              SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE adversary_role_traits        SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE adversary_ranks              SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE adversary_clarifications     SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE adversary_transformation_buffs SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE ocr_anomalies                SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE text_corrections             SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';

-- Hand-curated tables (0077) that carry the same slug as plain text, no FK.
UPDATE adversary_rule_breaks           SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE adversary_transformation_examples SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';
UPDATE adversary_battlefield_domains   SET source_book = 'book/bingo-book-pack-1' WHERE source_book = 'book/bingo-book';

DELETE FROM source_books WHERE slug = 'book/bingo-book';
