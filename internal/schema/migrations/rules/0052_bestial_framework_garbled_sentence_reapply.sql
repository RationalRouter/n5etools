-- Migration 0041 already repaired Puppet Master's Bestial Framework option
-- once, but a later sourcebook re-ingest (source_books shows
-- book/class-compendium re-ingested 2026-08-18 12:29:32 UTC) ran against a
-- pre-fix binary and re-derived the garbled sentence straight from the PDF,
-- silently overwriting the already-migrated row. The durable parser fix
-- (subclasses.go's knownExtractionSquishes, commit ee9cd48) didn't land
-- until 2026-08-18 12:35:07 UTC — five minutes after that re-ingest — so it
-- missed this pass entirely. schema_migrations still shows 0041 as applied
-- (it ran successfully at the time), so it will never re-run on its own.
-- The parser fix now covers any FUTURE re-ingest correctly; this migration
-- repairs the currently-shipped row in the meantime, the same one-time
-- reapply shape as 0039.
UPDATE class_options
SET description = REPLACE(description,
    'Natural Weapon of to summon you chose.',
    'Natural Weapon of the Sage Creature you chose to summon.')
WHERE slug = 'class/puppet-master/option/puppet-frameworks/bestial-framework';
