-- Audit trail for the OCR cross-validation pass: on every real re-ingest,
-- each parsed page is independently re-read via internal/ocr and compared
-- against internal/extract's text-layer reading of the same page. A low
-- similarity score means the two readings disagree more than normal OCR/
-- line-break noise accounts for, which can mean the text-layer parser
-- silently misread something on that page. Advisory only, same as
-- text_corrections: nothing here is applied automatically, and this table
-- never feeds back into any load — a maintainer reviews it by hand.
CREATE TABLE ocr_anomalies (
    id             INTEGER PRIMARY KEY,
    source_book    TEXT NOT NULL REFERENCES source_books(slug),
    source_version TEXT,
    page           INTEGER NOT NULL,
    text_excerpt   TEXT NOT NULL,
    ocr_excerpt    TEXT NOT NULL,
    similarity     REAL NOT NULL,
    detected_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (source_book, page, text_excerpt, ocr_excerpt)
);
CREATE INDEX idx_ocr_anomalies_book ON ocr_anomalies(source_book);
