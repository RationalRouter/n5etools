-- =============================================================================
-- Hand-checked seed data from the core book (Naruto 5e Full Document v3.11).
-- These are tiny fixed tables transcribed by a human, not parsed — if the
-- book changes them in a future version, update here and bump a migration.
-- =============================================================================

-- Jutsu ranks and their stock-5e spell level equivalents (core book, Ch. 9).
INSERT INTO jutsu_ranks (rank, sort_order, spell_level_equivalent) VALUES
    ('E', 0, 'Cantrips'),
    ('D', 1, '1st-2nd Level Spells'),
    ('C', 2, '3rd-4th Level Spells'),
    ('B', 3, '5th-6th Level Spells'),
    ('A', 4, '7th-8th Level Spells'),
    ('S', 5, '9th Level Spells');

-- Character Advancement table (core book p.14). Remember: XP resets to 0 at
-- each level-up in N5E, so xp_required is the cost of the NEXT level, not a
-- lifetime total.
INSERT INTO xp_levels (level, xp_required, proficiency_bonus) VALUES
    ( 1,    0, 3),
    ( 2,   50, 3),
    ( 3,   75, 3),
    ( 4,  100, 4),
    ( 5,  150, 4),
    ( 6,  200, 4),
    ( 7,  350, 5),
    ( 8,  475, 5),
    ( 9,  600, 5),
    (10,  725, 6),
    (11,  850, 6),
    (12, 1000, 6),
    (13, 1200, 7),
    (14, 1400, 7),
    (15, 1600, 7),
    (16, 1800, 8),
    (17, 2100, 8),
    (18, 2400, 8),
    (19, 2700, 9),
    (20, 3000, 9);

-- 30-point-buy costs (core book p.10, "Ability Score Point Cost").
INSERT INTO ability_point_costs (score, cost) VALUES
    ( 8, 0),
    ( 9, 1),
    (10, 2),
    (11, 3),
    (12, 4),
    (13, 5),
    (14, 7),
    (15, 9);

-- Elemental Superiority circle (core book, "What's Different" → Combat):
-- Fire > Wind > Lightning > Earth > Water > Fire.
INSERT INTO elemental_advantage (superior, inferior) VALUES
    ('Fire',      'Wind'),
    ('Wind',      'Lightning'),
    ('Lightning', 'Earth'),
    ('Earth',     'Water'),
    ('Water',     'Fire');
