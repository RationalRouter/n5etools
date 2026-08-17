-- The book's per-class level table prints a "Features" column with the
-- exact text for that level (including repeat-choice markers like "(2)",
-- "(3)" for recurring picks such as Genjutsu Pledge, and the universal
-- "Ability Score Improvement/Feat" at levels 4/8/12/16/19) — this is NOT
-- reproducible from class_features (checked directly: that table collapses
-- each repeating choice into a single row at its first level, and doesn't
-- carry the universal ASI/Feat entry at all, since it's identical across
-- every class). Stored here as plain verbatim text, hand-transcribed from
-- each class's own level-table page (Orochimaru Compendium v3.12 — Genjutsu
-- Specialist p5, Hunter-Nin p22, Intelligence Operative p45, Medical-Nin
-- p65, Ninjutsu Specialist p74, Scout-Nin p89, Taijutsu Specialist p112,
-- Weapon Specialist p126, Puppet Master p140, Cooking-Nin p186, Science-Nin
-- p206), not derived, since no parser produces this shape today.
ALTER TABLE class_levels ADD COLUMN features_text TEXT;

-- class_level_resources gains override support, same pattern as every other
-- content table — the column-completeness gaps below are real Mastersheet
-- gaps (confirmed against the book directly, not assumed), and this way a
-- future re-ingest of the (still-incomplete) Mastersheet won't silently
-- wipe these hand-added rows without a human noticing (decideStatus-style
-- protection would need matching changes in internal/store/classlevels.go
-- to fully honor 'verified'/'manual' status on the delete-then-reinsert
-- path — not done in this migration, tracked as a follow-up since it's a
-- real Go-code change, not just schema).
ALTER TABLE class_level_resources ADD COLUMN value_override TEXT;
ALTER TABLE class_level_resources ADD COLUMN detection_status TEXT NOT NULL DEFAULT 'auto'
    CHECK (detection_status IN ('auto','needs_review','verified','manual'));

CREATE VIEW v_class_level_resources AS
SELECT class_slug, level, resource_name,
       COALESCE(value_override, value) AS value,
       detection_status
FROM class_level_resources;

-- v_class_levels (from an earlier migration) predates features_text and
-- doesn't project it; recreate it here so callers can keep reading through
-- the view instead of the raw table.
DROP VIEW v_class_levels;
CREATE VIEW v_class_levels AS
SELECT class_slug, level, proficiency_bonus, features_text,
       COALESCE(jutsu_known_override, jutsu_known) AS jutsu_known,
       COALESCE(highest_rank_known_override, highest_rank_known) AS highest_rank_known,
       detection_status, notes
FROM class_levels;

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Chakra Disruption, Actualization'
    WHEN 2  THEN 'Genjutsu Pledge, Malleable Mirages'
    WHEN 3  THEN 'Genjutsu Inception'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Real World Conversion, Keen Awareness'
    WHEN 6  THEN 'Genjutsu Pledge (2)'
    WHEN 7  THEN 'Genjutsu Inception (2)'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Real World Conversion (2)'
    WHEN 10 THEN 'Genjutsu Pledge (3)'
    WHEN 11 THEN 'Keen Awareness (2), The Turn'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'The Turn, Master of Illusion'
    WHEN 14 THEN 'Genjutsu Pledge (4)'
    WHEN 15 THEN 'Real World Conversion (3), The Turn'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN NULL
    WHEN 18 THEN 'Genjutsu Pledge (5)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'The Prestige, The Turn (2), Master of Illusion (2)'
    END
WHERE class_slug = 'class/genjutsu-specialist';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Swift Response, Lethal Precision, Lethal Attack'
    WHEN 2  THEN 'Cunning Action, Hunters Patterns, Primary Target'
    WHEN 3  THEN 'Hunter Creed, Hunter Exploits'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Uncanny Dodge, Expertise'
    WHEN 6  THEN 'Hunted Target, Defensive Tactics'
    WHEN 7  THEN 'Hunter Creed (2)'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Hunters Patterns (2)'
    WHEN 10 THEN 'Hunter Creed (3), Hunters Exploits (2)'
    WHEN 11 THEN 'Defensive Tactics (2), Hunted Target (2)'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Elusive'
    WHEN 14 THEN 'Hunter Creed (4), Hunters Strike (3)'
    WHEN 15 THEN 'Hunters Patterns (3), Expertise (2)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Hunter Creed (5), Defensive Tactics (3)'
    WHEN 18 THEN 'Hunters Exploits (3)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Hunters Strike (4), Assassinate'
    END
WHERE class_slug = 'class/hunter-nin';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Strategic Timing, Exploit Weakness'
    WHEN 2  THEN 'Master Planner, Expertise'
    WHEN 3  THEN 'Master Strategist'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Tactical Scheme, Helpful Operative'
    WHEN 6  THEN 'Master Strategist (2)'
    WHEN 7  THEN 'Sabaki'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Master Strategist (3), Expertise (2)'
    WHEN 10 THEN 'Tactical Scheme (2)'
    WHEN 11 THEN 'Declaration of War'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Master Strategist (4)'
    WHEN 14 THEN 'Tsume'
    WHEN 15 THEN 'Tactical Scheme (3)'
    WHEN 16 THEN 'Ability Score Improvement/Feat, Expertise (3)'
    WHEN 17 THEN 'Master Strategist (5)'
    WHEN 18 THEN 'Declaration of War (2)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Checkmate (2)'
    END
WHERE class_slug = 'class/intelligence-operative';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Medical Ninjutsu, Rejuvenating Rest'
    WHEN 2  THEN 'Channeled Healing, Tenets of Medicine'
    WHEN 3  THEN 'Chakra Scalpel, Medical Doctrine'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Tenets of Medicine (2), Preserve/Take Life'
    WHEN 6  THEN 'Advanced Medical Research'
    WHEN 7  THEN 'Chakra Scalpel (2)'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Tenets of Medicine (3)'
    WHEN 10 THEN 'Gifted Healer'
    WHEN 11 THEN 'Chakra Scalpel (3)'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Medical Doctrine (2), Tenets of Medicine (4)'
    WHEN 14 THEN 'Gifted Healer (2)'
    WHEN 15 THEN NULL
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Tenets of Medicine (4)'
    WHEN 18 THEN 'Chakra Scalpel (4)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Supreme Healer'
    END
WHERE class_slug = 'class/medical-nin';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Chakra Recovery, Refined Ninjutsu'
    WHEN 2  THEN 'Ninjutsu Tradition'
    WHEN 3  THEN 'Efficient Molding'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Jutsu Breaker, Efficient Molding (2)'
    WHEN 6  THEN 'Ninjutsu Tradition (2), Chakra Recovery (2)'
    WHEN 7  THEN NULL
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Refined Ninjutsu (2), Efficient Molding (3)'
    WHEN 10 THEN 'Ninjutsu Tradition (3)'
    WHEN 11 THEN 'Chakra Recovery (3), Jutsu Breaker (2)'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Efficient Molding (4)'
    WHEN 14 THEN 'Ninjutsu Tradition (4)'
    WHEN 15 THEN 'Efficient Molding (5)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Chakra Recovery (4), Refined Ninjutsu (3), Jutsu Breaker (3)'
    WHEN 18 THEN 'Ninjutsu Tradition (5), Efficient Molding (6)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Ninjutsu Master'
    END
WHERE class_slug = 'class/ninjutsu-specialist';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Fighting Style, Deft Explorer'
    WHEN 2  THEN 'Shinobi Adept'
    WHEN 3  THEN 'Scouting Technique'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Extra Attack, Jack of all, Master of None'
    WHEN 6  THEN 'Scouting Techniques (2)'
    WHEN 7  THEN 'Signature Jutsu'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Scouting Techniques (3)'
    WHEN 10 THEN 'Signature Technique'
    WHEN 11 THEN NULL
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Shinobi Adept (2)'
    WHEN 14 THEN 'Scouting Techniques (4)'
    WHEN 15 THEN 'Signature Jutsu (2)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Scouting Techniques (5)'
    WHEN 18 THEN NULL
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Scouting Techniques (6)'
    END
WHERE class_slug = 'class/scout-nin';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Martial Defense, Unarmed Technique, Taijutsu Stance'
    WHEN 2  THEN 'Martial Adept, Enhanced Movement'
    WHEN 3  THEN 'Taijutsu Style'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Martial Defense (2), Extra Attack'
    WHEN 6  THEN 'Unarmed Technique (2), Taijutsu Style (2)'
    WHEN 7  THEN 'Evasion, Chakra Enhanced Strikes'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Martial Defense (3), Taijutsu Style (3)'
    WHEN 10 THEN 'Unbreakable Will'
    WHEN 11 THEN 'Unarmed Technique (3), Flow of Battle'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Martial Defense (4), Master of Persistence'
    WHEN 14 THEN 'Taijutsu Style (4)'
    WHEN 15 THEN 'Perfect Body'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Martial Defense (5), Taijutsu Style (5)'
    WHEN 18 THEN 'Perfect Mind'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Taijutsu Style (6)'
    END
WHERE class_slug = 'class/taijutsu-specialist';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Weapon Focus'
    WHEN 2  THEN 'Weapon Flurry, Weapon Stance'
    WHEN 3  THEN 'Weapon Form'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Extra Attack, Enhanced Property'
    WHEN 6  THEN 'Weapon Form (2)'
    WHEN 7  THEN 'Critical Focus, Enhanced Chakra Strike'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Weapon Focus (2)'
    WHEN 10 THEN 'Battle Readiness'
    WHEN 11 THEN 'Superior Attack, Critical Focus (2)'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Weapon Form (3)'
    WHEN 14 THEN 'Superior Weapon Flurry'
    WHEN 15 THEN 'Weapon Focus (3)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Critical Focus (3)'
    WHEN 18 THEN 'Superior Weapon Flurry (2)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Weapon Form (4)'
    END
WHERE class_slug = 'class/weapon-specialist';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Chakra Threads, Puppet Tool'
    WHEN 2  THEN 'Tactics of the Craft, Puppet Technique, Puppet Upgrades'
    WHEN 3  THEN 'Chakra Enhanced Retrofit'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Extra Attack, Generalized Skill'
    WHEN 6  THEN 'Puppet Technique (2), Puppet Tool (2)'
    WHEN 7  THEN 'Tool Expertise'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Chakra Threads (2), Puppet Tool (3)'
    WHEN 10 THEN 'Puppet Technique (3)'
    WHEN 11 THEN 'Tactics of the Craft (2)'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Chakra Enhanced Retrofit (2)'
    WHEN 14 THEN 'Puppet Technique (4)'
    WHEN 15 THEN 'Always Prepared, Puppet Tool (4)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Puppet Technique (5)'
    WHEN 18 THEN 'Always Prepared (2)'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Puppet Technique (6)'
    END
WHERE class_slug = 'class/puppet-master';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Cooking Tool Infusion, Shinobi Snacks'
    WHEN 2  THEN 'Cooking Focus, Food for the Soul'
    WHEN 3  THEN 'Tool Expertise'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Cooking Focus (2)'
    WHEN 6  THEN 'Cooking Tool (2), Wandering Aroma'
    WHEN 7  THEN 'War and Food'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Cooking Focus (3)'
    WHEN 10 THEN 'Iron Stomach'
    WHEN 11 THEN 'Cooking Tool (3)'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Cooking Focus (4)'
    WHEN 14 THEN 'Iron Stomach (2), Wandering Aroma (2)'
    WHEN 15 THEN 'Food for the Soul (2), War and Food (2)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Cooking Focus (5)'
    WHEN 18 THEN NULL
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Peerless Taste'
    END
WHERE class_slug = 'class/cooking-nin';

UPDATE class_levels SET features_text = CASE level
    WHEN 1  THEN 'Shinobi of Science, Chakra Cell Enhancement'
    WHEN 2  THEN 'Chakra containment Device, Scientific Ninja Tools'
    WHEN 3  THEN 'Scientific Inquiry'
    WHEN 4  THEN 'Ability Score Improvement/Feat'
    WHEN 5  THEN 'Extra Attack, The Right Tool'
    WHEN 6  THEN 'Scientific Inquiry (2)'
    WHEN 7  THEN 'Yhprum''s Law'
    WHEN 8  THEN 'Ability Score Improvement/Feat'
    WHEN 9  THEN 'Scientific Inquiry (3)'
    WHEN 10 THEN 'Calculated Response'
    WHEN 11 THEN 'Infused Genius'
    WHEN 12 THEN 'Ability Score Improvement/Feat'
    WHEN 13 THEN 'Chakra Containment Device (2)'
    WHEN 14 THEN 'Scientific Inquiry (4)'
    WHEN 15 THEN 'Calculated Response (2)'
    WHEN 16 THEN 'Ability Score Improvement/Feat'
    WHEN 17 THEN 'Scientific Inquiry (5)'
    WHEN 18 THEN 'Mixed Studies'
    WHEN 19 THEN 'Ability Score Improvement/Feat'
    WHEN 20 THEN 'Scientific Inquiry (6)'
    END
WHERE class_slug = 'class/science-nin';

-- Puppet Master's Wood/Bronze/Silver/Gold/Platinum Tier columns are
-- completely absent from the Mastersheet (zero class_level_resources rows
-- existed for this class before this migration — confirmed against
-- internal/sheet ingestion output, not assumed). Dashes in the book (no
-- tier yet at that level) are simply omitted rows, same convention already
-- used for every other class's early-level dashes.
--
-- INSERT...SELECT joined against class_levels (rather than a plain INSERT
-- VALUES) so this is a no-op on a freshly-migrated, not-yet-ingested
-- database (schema tests apply every migration against an empty DB) —
-- class_level_resources' FK requires the (class_slug, level) row to already
-- exist in class_levels, which only ingestion populates.
INSERT INTO class_level_resources (class_slug, level, resource_name, value, detection_status)
SELECT cl.class_slug, cl.level, v.resource_name, v.value, 'manual'
FROM class_levels cl
JOIN (
    SELECT 2 AS level, 'Wood Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 3 AS level, 'Wood Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 4 AS level, 'Wood Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 5 AS level, 'Wood Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 6 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 6 AS level, 'Bronze Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 7 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 7 AS level, 'Bronze Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 8 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 8 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 9 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 9 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 10 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 10 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 10 AS level, 'Silver Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 11 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 11 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 11 AS level, 'Silver Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 12 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 12 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 12 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 13 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 13 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 13 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 14 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 14 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 14 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 14 AS level, 'Gold Tier' AS resource_name, '1' AS value
    UNION ALL
    SELECT 15 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 15 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 15 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 15 AS level, 'Gold Tier' AS resource_name, '1' AS value
    UNION ALL
    SELECT 16 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 16 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 16 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 16 AS level, 'Gold Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 17 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 17 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 17 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 17 AS level, 'Gold Tier' AS resource_name, '2' AS value
    UNION ALL
    SELECT 18 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 18 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 18 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 18 AS level, 'Gold Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 18 AS level, 'Platinum Tier' AS resource_name, '1' AS value
    UNION ALL
    SELECT 19 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 19 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 19 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 19 AS level, 'Gold Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 19 AS level, 'Platinum Tier' AS resource_name, '1' AS value
    UNION ALL
    SELECT 20 AS level, 'Wood Tier' AS resource_name, '4' AS value
    UNION ALL
    SELECT 20 AS level, 'Bronze Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 20 AS level, 'Silver Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 20 AS level, 'Gold Tier' AS resource_name, '3' AS value
    UNION ALL
    SELECT 20 AS level, 'Platinum Tier' AS resource_name, '2' AS value
) AS v ON v.level = cl.level
WHERE cl.class_slug = 'class/puppet-master';

-- Ninjutsu Specialist's one lone gap at level 17 (matches the same gap
-- already known and fixed for jutsu_known in migration 0013). Same
-- INSERT...SELECT FK-safety pattern as above.
INSERT INTO class_level_resources (class_slug, level, resource_name, value, detection_status)
SELECT cl.class_slug, cl.level, v.resource_name, v.value, 'manual'
FROM class_levels cl
JOIN (
    SELECT 'Refined Ninjutsu' AS resource_name, '7' AS value
    UNION ALL
    SELECT 'Efficient Moldings', '5'
) AS v ON 1=1
WHERE cl.class_slug = 'class/ninjutsu-specialist' AND cl.level = 17;
