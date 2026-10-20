-- Backfills two enhancement-seal parser fixes onto rules.db copies ingested
-- before them. A fresh ingest now produces the same rows. Every statement is
-- gated on the broken shape, so an already-correct database is untouched.

-- 1. The "S-RANK SEALS" section heading was not recognized, so it was
--    appended to the last A-Rank seal of each section.
UPDATE equipment
SET description = rtrim(substr(description, 1, length(description) - length('S-RANK SEALS')))
WHERE kind = 'enhancement_seal' AND description LIKE '% S-RANK SEALS';

-- Their es/pt translations carry the same heading, translated three ways.
UPDATE content_translations
SET value = substr(value, 1, length(value) - length(' SELLOS DE Nivel S'))
WHERE table_name = 'equipment' AND field = 'description' AND slug LIKE 'seal/%'
  AND value LIKE '% SELLOS DE Nivel S';
UPDATE content_translations
SET value = substr(value, 1, length(value) - length(' SELOS DE NÍVEL S'))
WHERE table_name = 'equipment' AND field = 'description' AND slug LIKE 'seal/%'
  AND value LIKE '% SELOS DE NÍVEL S';
UPDATE content_translations
SET value = substr(value, 1, length(value) - length(' SELOS DE RANGO S'))
WHERE table_name = 'equipment' AND field = 'description' AND slug LIKE 'seal/%'
  AND value LIKE '% SELOS DE RANGO S';

-- 2. The book prints the C-Rank Hunter Seal (650 Ryo, p.76) with the tier
--    word "(SUPERIOR)", the same as the A-Rank Hunter Seal (1800 Ryo, p.78).
--    Both derived the slug seal/weapon/superior/hunter-seal, so the A-Rank
--    entry was dropped and the C-Rank text was stored as an A-Rank seal.
--    Move that row (and its translations, which match its text) to the
--    Refined slug, then restore the A-Rank entry under the Superior slug.
CREATE TEMP TABLE _hunter_refined AS
    SELECT * FROM equipment
    WHERE slug = 'seal/weapon/superior/hunter-seal' AND cost_ryo = 650
      AND NOT EXISTS (SELECT 1 FROM equipment WHERE slug = 'seal/weapon/refined/hunter-seal');
UPDATE _hunter_refined SET
    slug = 'seal/weapon/refined/hunter-seal',
    name = 'Hunter Seal (Refined)',
    seal_rank = 'C';
INSERT INTO equipment SELECT * FROM _hunter_refined;

UPDATE content_translations
SET slug = 'seal/weapon/refined/hunter-seal'
WHERE table_name = 'equipment' AND slug = 'seal/weapon/superior/hunter-seal'
  AND EXISTS (SELECT 1 FROM _hunter_refined);
UPDATE content_translations
SET value = replace(value, '(Superior)', '(refinado)')
WHERE table_name = 'equipment' AND slug = 'seal/weapon/refined/hunter-seal'
  AND locale = 'es' AND field = 'name' AND EXISTS (SELECT 1 FROM _hunter_refined);
UPDATE content_translations
SET value = replace(value, '(Superior)', '(Aperfeiçoado)')
WHERE table_name = 'equipment' AND slug = 'seal/weapon/refined/hunter-seal'
  AND locale = 'pt' AND field = 'name' AND EXISTS (SELECT 1 FROM _hunter_refined);

UPDATE equipment SET
    cost_ryo = 1800,
    source_page = 78,
    description = 'Weapons imbued with this seal deal an additional 2 dice of damage with weapon attacks against specific creature types. Select 2 creature types from the following list to be affected by this seal: Aberrations, Beasts, Celestials, Constructs, Demons, Monstrosities, Mutants, Ōtsutsuki, Plants, Sage Creatures, Undead.'
WHERE slug = 'seal/weapon/superior/hunter-seal' AND EXISTS (SELECT 1 FROM _hunter_refined);

DROP TABLE _hunter_refined;
