-- Traits from the community "N5e Adversary Builder" Google Sheet (exported
-- as BingoBook.xlsx) — a separate, non-official companion tool credited on
-- its own Front Page and linked from the Kage Guide, distinct from the
-- official Bingo Book PDF chapter already covered by 0024/0076/0077. This
-- table holds its TraitsMain tab: clan-, affiliation-, class-mod-, and
-- role-flavored traits a GM can hand-pick when reskinning a generated
-- adversary, plus a small "Iconic" named-ability catalog (category =
-- 'iconic') referenced from the Classification feature.
--
-- Deliberately kept separate from adversary_role_traits (the official,
-- procedurally-sampled 150-trait pool) rather than merged into it: this
-- sheet's own "Generic"/role-split content is a different, non-official
-- categorization of a similar idea, and blending unverified fan content
-- into the random-generation pool would silently change already-verified
-- generator behavior. Everything here is GM-reference only, never
-- auto-assigned — same non-goal already established for Transformations/
-- Battlefield Domains and the book's own "Elite Abilities" examples.
CREATE TABLE adversary_builder_sheet_traits (
    slug             TEXT PRIMARY KEY,
    category         TEXT NOT NULL CHECK (category IN
                       ('general','role','clan','affiliation','class_mod','passive','iconic')),
    category_tag     TEXT NOT NULL, -- the sheet's own tag, e.g. 'Aburame', 'Cursed Seal', 'Controller', 'Iconic'
    name             TEXT NOT NULL,
    rank             TEXT NOT NULL DEFAULT '' CHECK (rank IN ('','D','C','B','A','S')),
    description      TEXT NOT NULL,
    keywords         TEXT NOT NULL DEFAULT '',
    sort_order       INTEGER NOT NULL,
    source_book      TEXT NOT NULL,
    source_version   TEXT NOT NULL DEFAULT '',
    detection_status TEXT NOT NULL DEFAULT 'auto',
    notes            TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_adversary_builder_sheet_traits_category ON adversary_builder_sheet_traits(category, category_tag);
