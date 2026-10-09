-- Backfills four rules-data fixes onto rules.db copies ingested before the
-- parser/correction changes that now produce the same result on a fresh
-- ingest. Every statement is gated on the broken shape, so a database that
-- is already correct is left untouched.

-- 1. Renamed slugs. CuratedJutsuNameFixes corrects "O Encode Thoughts" and
--    "One Man Front Lotud", which changes their derived slugs; an in-place
--    re-ingest upserted the corrected rows next to the old ones instead of
--    replacing them, leaving each jutsu listed twice. Fold every old row
--    into its corrected slug (creating it first when an older ingest never
--    wrote one), carrying its children and translations across, then drop
--    the old row. Parent before children, children before the old parent,
--    as PRAGMA foreign_keys is ON during migrations.
CREATE TEMP TABLE _renamed_jutsu (old_slug TEXT PRIMARY KEY, new_slug TEXT NOT NULL, new_name TEXT NOT NULL);
INSERT INTO _renamed_jutsu VALUES
    ('jutsu/o-encode-thoughts', 'jutsu/encode-thoughts', 'Encode Thoughts'),
    ('jutsu/one-man-front-lotud', 'jutsu/one-man-front-lotus', 'One Man Front Lotus');

CREATE TEMP TABLE _renamed_jutsu_rows AS
    SELECT j.* FROM jutsu j JOIN _renamed_jutsu r ON r.old_slug = j.slug;
UPDATE _renamed_jutsu_rows SET
    name = (SELECT new_name FROM _renamed_jutsu WHERE old_slug = _renamed_jutsu_rows.slug),
    slug = (SELECT new_slug FROM _renamed_jutsu WHERE old_slug = _renamed_jutsu_rows.slug);
INSERT OR IGNORE INTO jutsu SELECT * FROM _renamed_jutsu_rows;
DROP TABLE _renamed_jutsu_rows;

UPDATE OR IGNORE clan_jutsu SET jutsu_slug = (SELECT new_slug FROM _renamed_jutsu WHERE old_slug = clan_jutsu.jutsu_slug)
    WHERE jutsu_slug IN (SELECT old_slug FROM _renamed_jutsu);
DELETE FROM clan_jutsu WHERE jutsu_slug IN (SELECT old_slug FROM _renamed_jutsu);

UPDATE OR IGNORE jutsu_keywords SET jutsu_slug = (SELECT new_slug FROM _renamed_jutsu WHERE old_slug = jutsu_keywords.jutsu_slug)
    WHERE jutsu_slug IN (SELECT old_slug FROM _renamed_jutsu);
DELETE FROM jutsu_keywords WHERE jutsu_slug IN (SELECT old_slug FROM _renamed_jutsu);

UPDATE OR IGNORE jutsu_upcast_rules SET jutsu_slug = (SELECT new_slug FROM _renamed_jutsu WHERE old_slug = jutsu_upcast_rules.jutsu_slug)
    WHERE jutsu_slug IN (SELECT old_slug FROM _renamed_jutsu);
DELETE FROM jutsu_upcast_rules WHERE jutsu_slug IN (SELECT old_slug FROM _renamed_jutsu);

UPDATE OR IGNORE content_translations SET slug = (SELECT new_slug FROM _renamed_jutsu WHERE old_slug = content_translations.slug)
    WHERE table_name = 'jutsu' AND slug IN (SELECT old_slug FROM _renamed_jutsu);
DELETE FROM content_translations WHERE table_name = 'jutsu' AND slug IN (SELECT old_slug FROM _renamed_jutsu);

DELETE FROM jutsu WHERE slug IN (SELECT old_slug FROM _renamed_jutsu);
DROP TABLE _renamed_jutsu;

-- 2. The clan books label a Hijutsu's keyword line "Keyword:" (singular).
--    The parser only knew "Keywords:", so 37 entries carried the list glued
--    onto Cost ("5 Chakra Keyword: Hijutsu, Taijutsu") with keywords empty.
--    Index the keywords first (read from cost_text), then split the row;
--    SQLite evaluates every SET expression against the pre-update row.
WITH RECURSIVE src(slug, rest) AS (
    SELECT slug, trim(substr(cost_text, instr(cost_text, ' Keyword:') + 9)) || ','
    FROM jutsu
    WHERE instr(cost_text, ' Keyword:') > 0 AND coalesce(keywords, '') = ''
), split(slug, kw, rest) AS (
    SELECT slug, '', rest FROM src
    UNION ALL
    SELECT slug, trim(substr(rest, 1, instr(rest, ',') - 1)), substr(rest, instr(rest, ',') + 1)
    FROM split WHERE rest <> ''
)
INSERT OR IGNORE INTO jutsu_keywords (jutsu_slug, keyword)
SELECT slug, kw FROM split WHERE kw <> '';

UPDATE jutsu SET
    keywords  = trim(substr(cost_text, instr(cost_text, ' Keyword:') + 9)),
    cost_text = trim(substr(cost_text, 1, instr(cost_text, ' Keyword:') - 1))
WHERE instr(cost_text, ' Keyword:') > 0 AND coalesce(keywords, '') = '';

-- 3. Bare chakra costs printed with a stray trailing comma or period
--    ("9 Chakra,", "21 Chakra.", "Special (36 Chakra)."). Same rule as the
--    parser's trimCostPunct.
UPDATE jutsu SET cost_text = substr(cost_text, 1, length(cost_text) - 1)
WHERE (cost_text GLOB '[0-9]* Chakra[.,]'
        AND substr(cost_text, 1, length(cost_text) - 8) NOT GLOB '*[^0-9]*')
   OR (cost_text GLOB 'Special ([0-9]* Chakra)[.,]'
        AND substr(cost_text, 10, length(cost_text) - 18) NOT GLOB '*[^0-9]*');

-- 4. "chakras die" typos, matching internal/correct's curated entries.
UPDATE jutsu SET description = replace(description,
        'Spend a number of chakras die up to your proficiency bonus.',
        'Spend a number of Chakra Dice up to your proficiency bonus.')
    WHERE instr(description, 'Spend a number of chakras die up to your proficiency bonus.') > 0;
UPDATE jutsu SET description = replace(description,
        'can spend up to two chakras die, regaining', 'can spend up to two Chakra Dice, regaining')
    WHERE instr(description, 'can spend up to two chakras die, regaining') > 0;
UPDATE feats SET description = replace(description,
        'spend 3 or more chakras die, you regain 1 chakra die at',
        'spend 3 or more Chakra Dice, you regain 1 Chakra Die at')
    WHERE instr(description, 'spend 3 or more chakras die, you regain 1 chakra die at') > 0;
UPDATE bloodline_latents SET description = replace(description,
        'you spend 2 chakras die and create a keycard', 'you spend 2 Chakra Dice and create a keycard')
    WHERE instr(description, 'you spend 2 chakras die and create a keycard') > 0;
