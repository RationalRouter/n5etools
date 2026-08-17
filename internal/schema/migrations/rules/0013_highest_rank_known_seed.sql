-- Backfills two real gaps in class_levels, both confirmed by directly
-- reading the Orochimaru Compendium's own per-class level tables (an image
-- in the PDF, not extractable text — see the Mastersheet-bootstrap note in
-- internal/sources/books.go): every one of the 11 classes' level tables
-- prints BOTH a "Jutsu Known" and a "Highest Rank Jutsu Known" column, but
-- the community Mastersheet's ClassChartMaster tab never captured a
-- "Highest Rank Known" column at all, and separately has no "Jutsu Known"
-- data whatsoever for 5 of the 11 classes (checked directly against the
-- book, not assumed from the gap).
--
-- Page references (Orochimaru Compendium v3.12, printed page numbers, each
-- class's own level table on its chapter's first page): Genjutsu Specialist
-- p5, Hunter-Nin p22, Intelligence Operative p45, Medical-Nin p65, Ninjutsu
-- Specialist p74, Scout-Nin p89, Taijutsu Specialist p112, Weapon Specialist
-- p126, Puppet Master p140, Cooking-Nin p186, Science-Nin p206.

-- Highest Rank Known follows one universal, level-only formula across ALL
-- 11 classes with zero exceptions (every class's table was read directly to
-- confirm this, not inferred from a subset): levels 1-4 D-Rank, 5-8 C-Rank,
-- 9-12 B-Rank, 13-16 A-Rank, 17-20 S-Rank.
UPDATE class_levels SET highest_rank_known_override = 'D' WHERE level BETWEEN 1 AND 4;
UPDATE class_levels SET highest_rank_known_override = 'C' WHERE level BETWEEN 5 AND 8;
UPDATE class_levels SET highest_rank_known_override = 'B' WHERE level BETWEEN 9 AND 12;
UPDATE class_levels SET highest_rank_known_override = 'A' WHERE level BETWEEN 13 AND 16;
UPDATE class_levels SET highest_rank_known_override = 'S' WHERE level BETWEEN 17 AND 20;

-- Jutsu Known: the Mastersheet has no data at all for these 4 classes
-- (jutsu_known was NULL for every level) even though the book prints real
-- numbers. All 4 follow the exact same progression as every other
-- "standard" class already correctly loaded from the Mastersheet
-- (Genjutsu Specialist, Hunter-Nin, Scout-Nin, Cooking-Nin — cross-checked
-- against their book pages too, values match what's already in the DB).
UPDATE class_levels SET jutsu_known_override = CASE level
    WHEN 1 THEN 6 WHEN 2 THEN 7 WHEN 3 THEN 8 WHEN 4 THEN 8 WHEN 5 THEN 9
    WHEN 6 THEN 10 WHEN 7 THEN 11 WHEN 8 THEN 11 WHEN 9 THEN 12 WHEN 10 THEN 13
    WHEN 11 THEN 14 WHEN 12 THEN 14 WHEN 13 THEN 15 WHEN 14 THEN 16 WHEN 15 THEN 17
    WHEN 16 THEN 17 WHEN 17 THEN 18 WHEN 18 THEN 19 WHEN 19 THEN 20 WHEN 20 THEN 20
    END
WHERE class_slug IN (
    'class/intelligence-operative', 'class/puppet-master',
    'class/taijutsu-specialist', 'class/weapon-specialist'
);

-- Medical-Nin follows a different progression (+1 jutsu known per level,
-- uncapped past 20 — it reaches 25 at level 20).
UPDATE class_levels SET jutsu_known_override = CASE level
    WHEN 1 THEN 6 WHEN 2 THEN 7 WHEN 3 THEN 8 WHEN 4 THEN 9 WHEN 5 THEN 10
    WHEN 6 THEN 11 WHEN 7 THEN 12 WHEN 8 THEN 13 WHEN 9 THEN 14 WHEN 10 THEN 15
    WHEN 11 THEN 16 WHEN 12 THEN 17 WHEN 13 THEN 18 WHEN 14 THEN 19 WHEN 15 THEN 20
    WHEN 16 THEN 21 WHEN 17 THEN 22 WHEN 18 THEN 23 WHEN 19 THEN 24 WHEN 20 THEN 25
    END
WHERE class_slug = 'class/medical-nin';

-- Ninjutsu Specialist's one lone gap at level 17 (every other level was
-- already correctly ingested from the Mastersheet).
UPDATE class_levels SET jutsu_known_override = 22
WHERE class_slug = 'class/ninjutsu-specialist' AND level = 17;
