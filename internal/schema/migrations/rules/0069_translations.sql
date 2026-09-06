-- Generic EAV-style translation storage for game content (jutsu, feats,
-- clans, class_features, etc.), added once as localization work begins
-- (Spanish first, Portuguese planned next). One table for every content
-- table rather than a per-table jutsu_i18n/feats_i18n/clans_i18n/... because:
-- content tables gain translatable columns over time (see e.g. 0068 above),
-- and a per-table translations table would need a paired migration+Go
-- struct change every time that happens; a generic table needs none. All
-- content-table primary keys are stable, human-independent TEXT slugs
-- (e.g. 'jutsu/chakra-blow') that survive re-ingest by design (see
-- internal/store/store.go), so they're a safe long-term key here.
--
-- Never touched by cmd/n5e-ingest's upsert logic (which only knows about
-- the specific content tables it parses into), so this table survives
-- re-ingestion for free, the same way *_override columns and notes already
-- do. 'en' is never a valid locale value here -- English lives in the base
-- content tables' own columns, not in this table.
--
-- Which fields of a given table are actually translatable is tracked in Go
-- (internal/i18n.TranslatableFields), not enforced by this schema -- e.g.
-- jutsu.components is deliberately excluded there because it's a
-- machine-parsed field (cmd/n5e/jutsu_filters.go), not display prose.
CREATE TABLE content_translations (
    table_name  TEXT NOT NULL,
    slug        TEXT NOT NULL,
    locale      TEXT NOT NULL,
    field       TEXT NOT NULL,
    value       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'machine'
                CHECK (status IN ('machine', 'reviewed')),
    en_hash     TEXT NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (table_name, slug, locale, field)
) WITHOUT ROWID;

CREATE INDEX idx_content_translations_lookup
    ON content_translations(locale, table_name, slug);
