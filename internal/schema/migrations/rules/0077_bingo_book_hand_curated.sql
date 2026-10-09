-- Adversary Pack 1 enrichment, second slice: the content whose printed
-- layout is either genuinely irregular to parse (small lookup tables that
-- wrap their rows across multiple lines depending on cell length) or is
-- narrative GM-reference material with no closed vocabulary a parser could
-- key off of. All hand-transcribed once here, never re-ingested — same
-- convention as 0012_weapon_properties.sql and 0002_seed.sql's tiny
-- hand-checked lookup tables.
--
-- Deliberately NOT covered by this migration, confirmed by reading the
-- actual PDF text before writing any schema (see
-- internal/parse/bingobook_freeform.go's package comment for the sibling
-- auto-parsed tables):
--   * Elite Actions — a fixed 4-option rules mechanic (Attack/Cast/
--     Command/Trait) described entirely in prose, not enumerable data.
--   * Elite Abilities — the book explicitly defers its actual ability list
--     to the "Adversary Attacks, Abilities & Jutsu" section, which is not
--     part of this Pack 1 PDF at all (a different, not-yet-ingested book).
--     Nothing to transcribe here.
--   * Health Gates — a fixed universal rule (60%/30% max-HP thresholds,
--     30% Chakra recovery, a Healing Surge letting a player spend up to 2
--     hit dice), not per-rank or per-adversary data. The Iconic/Epic
--     modifier numbers it references are already captured in
--     adversary_classification_modifiers (migration 0076). Best modeled as
--     fixed generator logic in a later stage, not a table.

-- FREEFORM DR TABLE (book p.15): Damage Reduction granted by a Freeform
-- Defensive Jutsu's "DR" focus, by the adversary's Class and the jutsu's
-- Rank. Minion has no DR option above C-Rank, printed as an em-dash.
CREATE TABLE adversary_freeform_dr_table (
    class  TEXT PRIMARY KEY CHECK (class IN ('minion', 'standard', 'elite', 'solo')),
    d_rank TEXT NOT NULL,
    c_rank TEXT NOT NULL,
    b_rank TEXT,  -- NULL where the book prints an em-dash (Minion)
    a_rank TEXT,
    s_rank TEXT
);
INSERT INTO adversary_freeform_dr_table (class, d_rank, c_rank, b_rank, a_rank, s_rank) VALUES
    ('minion',   '+2',    '+4',    NULL,    NULL,     NULL),
    ('standard', '+2',    '+4',    '+6',    '+8',     '+10'),
    ('elite',    '1d4+2', '1d6+3', '1d8+4', '1d10+5', '1d12+6'),
    ('solo',     '1d6+2', '1d8+3', '1d10+4','1d12+5', '2d6+6');

-- FREEFORM DAMAGE PENALTY TABLE (book p.13): the level penalty or bonus a
-- Freeform Jutsu's effective level takes for each optional feature stacked
-- onto it (see FREEFORM RANK DAMAGE COLUMNS, same page, for how the
-- resulting effective level maps to a Very Low..Very High damage column).
CREATE TABLE adversary_freeform_damage_penalty_table (
    source          TEXT PRIMARY KEY,
    level_modifier  TEXT NOT NULL,
    sort_order      INTEGER NOT NULL
);
INSERT INTO adversary_freeform_damage_penalty_table (source, level_modifier, sort_order) VALUES
    ('Area of Effect (AOE)',              '-1', 1),
    ('Tier 1 Effect',                     '-1', 2),
    ('Tier 2 Effect',                     '-2', 3),
    ('No Damage on Save',                 '+1', 4),
    ('Use Lowest Attack Bonus',           '+1', 5),
    ('Preestablished Effect: Tier 1',     '+1', 6),
    ('Preestablished Effect: Tier 2',     '+2', 7);

-- FREEFORM JUTSU SHAPES (book p.13): fixed area-of-effect shapes an
-- Offensive Freeform Jutsu can pick when it targets an area instead of a
-- single creature.
CREATE TABLE adversary_freeform_jutsu_shapes (
    shape      TEXT PRIMARY KEY,
    size_text  TEXT NOT NULL,
    sort_order INTEGER NOT NULL
);
INSERT INTO adversary_freeform_jutsu_shapes (shape, size_text, sort_order) VALUES
    ('Line',     '5ft Wide, 60ft Long',              1),
    ('Sphere',   '20ft-Radius, centered on a Point',  2),
    ('Cone',     '30ft Cone',                          3),
    ('Cylinder', '15-Foot Wide, 30-Feet Tall',         4),
    ('Cube',     '30-Foot Cube',                       5);

-- EFFECT & CONDITIONS TABLE (book p.14): the menu a Freeform Jutsu or Trait
-- draws a secondary effect from, each tagged with the Tier that determines
-- its FREEFORM DAMAGE PENALTY TABLE cost. "Bleed" is printed twice in the
-- book's own tier-1 condition row — transcribed as printed, not corrected,
-- since it's ambiguous whether that's a genuine intentional duplicate (two
-- independently-stacking Bleed effects) or an upstream typo.
CREATE TABLE adversary_conditions_table (
    tier        TEXT NOT NULL,  -- '1', '2', or '1 or 2' for the open-ended "make your own" row
    effects     TEXT NOT NULL,
    effect_type TEXT NOT NULL CHECK (effect_type IN ('Condition', 'Non-Condition', 'Variable')),
    sort_order  INTEGER NOT NULL PRIMARY KEY
);
INSERT INTO adversary_conditions_table (sort_order, tier, effects, effect_type) VALUES
    (1,  '1',      'Bruised/Bleed/Bleed/Shaken/Weakened',      'Condition'),
    (2,  '1',      'Elemental Afflictions',                    'Condition'),
    (3,  '1',      'Envenomed',                                'Condition'),
    (4,  '1',      'Concussed/Demoralized/Sealed',             'Condition'),
    (5,  '1',      'Dazzled/Deafened',                         'Condition'),
    (6,  '1',      'Grappled/Prone',                           'Condition'),
    (7,  '1',      'AC/DR/Speed Penalty',                      'Non-Condition'),
    (8,  '1',      'Penalty on Attacks/Saves/Checks/DCs',      'Non-Condition'),
    (9,  '1',      'Forced Movement (Push/Pull)',              'Non-Condition'),
    (10, '2',      'Dispel or Interrupt Effects',              'Non-Condition'),
    (11, '2',      'Disadvantage on Attacks/Saves/Checks',     'Non-Condition'),
    (12, '2',      'Compelled (Berserk/Charm/Confused)',       'Condition'),
    (13, '2',      'Blinded/Slowed/Stunned/Exhaustion',        'Condition'),
    (14, '2',      'Dazed, Staggered, Restrained',             'Condition'),
    (15, '2',      'Action/Bonus Action/Reaction Denial',      'Non-Condition'),
    (16, '1 or 2', 'Make your own!',                           'Variable');

-- RULE BREAKS, TRANSFORMATION EXAMPLES, and BATTLEFIELD DOMAIN EXAMPLES
-- (book pp.19-23): GM-facing reference write-ups, not mechanical data an
-- adversary's stat block reads from at generation time. Same
-- provenance-without-FK shape as weapon_properties: a one-time migration
-- seed, not something internal/ingest ever upserts, so there's no
-- guarantee a matching source_books row exists yet when this runs.
CREATE TABLE adversary_rule_breaks (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    sort_order       INTEGER NOT NULL,
    source_book      TEXT,
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
INSERT INTO adversary_rule_breaks (slug, name, sort_order, source_book, source_version, source_page, description) VALUES
('adversary-rule-break/stamina-die', 'Stamina Die', 1, 'book/bingo-book', '2026-07-27', 19,
 'When the Adversary Transforms, select either a d8 (Elite) or d12 (Solo), and set it to face its highest value. This value is the Adversary''s Stamina. The Adversary can spend Stamina to perform any activity that normally requires Action economy (Actions, Bonus Actions, Reactions, etc.) so long as it has Stamina remaining — Actions cost 2 Stamina, Bonus Actions cost 1, Reactions cost 1. The Adversary regains 2 Stamina on each of its turns in initiative, and can spend Stamina during its turns or Elite Actions until it has none. If it ever has 0 Stamina, it is Staggered and Incapacitated. If a player creature deals damage the Adversary is vulnerable to, or scores a critical hit against it, it loses 1 Stamina.'),
('adversary-rule-break/stability-posture', 'Stability/Posture', 2, 'book/bingo-book', '2026-07-27', 19,
 'When the Adversary Transforms, every creature in the fight — players, player-allied creatures, and the Adversary itself — begins tracking Stability, a measure of composure, footing, and focus under the overwhelming pressure of this transformed state. At Initiative Count 99, every player and player-allied creature rolls 1d6 and adds the result to their current Stability; players begin at 0 when the Transformation first triggers, gaining their first roll as part of that moment. The Adversary begins with 20 Stability when the Transformation triggers. Stability cannot exceed 10 for players and player-allied creatures, or 20 for the Adversary. Each time any creature would lose hit points (or Chakra Points, for chakra damage), it loses 1 Stability — a single action dealing multiple instances of damage reduces Stability by 1 for each separate instance. At the start of each of the Adversary''s turns in initiative, it regains 2 Stability, up to its maximum of 20. Players and player-allied creatures can recover Stability between Initiative Count 99 refreshes: receiving any amount of healing, gaining Temporary Hit Points, or having any condition removed by any means each grants +1 Stability (up to the maximum of 10), and each instance counts separately. If any creature''s Stability reaches 0, it is immediately Stunned until it takes any instance of damage, regains any Hit Points, is targeted by a jutsu or ability that removes a condition (even with no conditions to remove), or reaches the beginning of its next turn in initiative — whichever comes first.'),
('adversary-rule-break/mobile-battle', 'Mobile Battle', 3, 'book/bingo-book', '2026-07-27', 19,
 'When the Adversary Transforms, the fight becomes a chase — the Adversary is in constant motion, and the distance between it and every other creature on the battlefield determines everything. The Adversary projects three zones centered on itself that move with it at all times. The Outer Zone (beyond 30 feet) grants the Adversary advantage on all attacks and jutsu targeting creatures there, and those creatures cannot deal damage to the Adversary. The Strike Zone (10 to 30 feet) is the only range at which the Adversary can be damaged; creatures there act normally. The Inner Zone (within 10 feet) immediately Staggers and Dazes any creature entering it or starting its turn there, until the start of their next turn. The Adversary must move at least half its speed on every turn and every Elite Action it takes, and cannot voluntarily end its movement where it started — if a condition or effect would prevent it from moving, the Inner Zone immediately expands to 20 feet for the duration, representing the dangerous buildup of momentum with nowhere to go. At Initiative Count 99, the Adversary rolls 1d6; on a 4 or higher, the Outer Zone and Inner Zone swap their effects until the next Initiative Count 99 — the Strike Zone never changes. This Adversary shouldn''t have speed that makes it impossible to keep up with for more than half of the party; physical obstacles or speed penalties are encouraged to ramp up tension.'),
('adversary-rule-break/mirror-phase', 'Mirror Phase', 4, 'book/bingo-book', '2026-07-27', 19,
 'For this Rule Break, collect the primary 2 damage types and conditions each player or player-allied creature can deal, and record them ahead of time. When the Adversary Transforms, it gains the ability to Mirror a player creature. At Initiative Count 99, the Adversary randomly selects one player or player-allied creature it can see in initiative; until the next Initiative Count 99, it mirrors that creature, duplicating their AC (if the Adversary''s is lower), duplicating their highest saving throw bonus (if the Adversary''s is lower), and gaining Immunity to that creature''s primary damage types and the conditions it inflicts. The mirrored creature and its allies can identify who is being Mirrored and what the Adversary is immune to with a successful Read the Enemy, Sense Motive, or Identify check against the Adversary''s highest Jutsu Save DC.');

CREATE TABLE adversary_transformation_examples (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    sort_order       INTEGER NOT NULL,
    source_book      TEXT,
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
INSERT INTO adversary_transformation_examples (slug, name, sort_order, source_book, source_version, source_page, description) VALUES
('adversary-transformation-example/tailed-beast-chakra-cloak', 'Tailed Beast Chakra Cloak', 1, 'book/bingo-book', '2026-07-27', 20,
 'Concept: the Jinchūriki has partially lost control — dense, burning red chakra erupts from their body in the shape of a beast, sharpening their instincts beyond natural limits and turning their skin into something that punishes touch.

Buffs (3 selected): More Powerful (raw Tailed Beast chakra amplifies every strike and technique); New Resistances (Resistance to Fire and Bludgeoning, Vulnerability to the Sealed condition); Rule Break — Stability/Posture (the overwhelming presence of the Tailed Beast chakra destabilizes every creature on the battlefield; the Adversary begins with 20 Stability, representing the Beast''s will fighting back against suppression).

Exclusive Freeform Ability — Chakra Shroud (Cost 2 Discipline): once per turn on any creature''s turn, when a creature enters or starts its turn within 10 feet of this Adversary, the Adversary may spend 2 Tenacity to deal Very Low fire damage and inflict 1 rank of Burned.

Optional Counter: a player who successfully applies the Sealed condition suppresses the Chakra Shroud and both damage Resistances until the start of the Adversary''s next turn at Initiative Count 99, representing the seal temporarily reasserting control.'),
('adversary-transformation-example/eight-inner-gates', 'Eight Inner Gates', 2, 'book/bingo-book', '2026-07-27', 20,
 'Concept: the Adversary has trained their body beyond its natural limits; when the fight demands it, they open their inner gates.

Buffs (3 selected): More Powerful (every physical technique and strike operates at a higher tier the moment the gates open); New Movement (bursts of speed so fast the Adversary appears to teleport — if it moves 30 feet or fewer, it instead teleports into the space it ends its movement in); Tougher Defenses (the surge of released chakra hardens the body and accelerates reflexes, improving both AC and DR despite the self-damage occurring underneath).

Exclusive Freeform Strike — Impact Hammer (Cost 2 Discipline): once per turn on this Adversary''s turn, by spending 2 Tenacity, it makes a single Freeform Strike dealing High damage, forcing the target to make a Constitution saving throw against the Adversary''s highest Save DC or be launched 20 feet in a direction of the Adversary''s choice, falling Prone and Staggered; the Adversary then takes Very Low damage as the gate release tears at its own body.

Optional Counter — Countdown: the body is consuming itself. The countdown begins at the result of 1d6, falling by 1 at the end of each of the Adversary''s turns; when it reaches 0, the Adversary loses 2d6 × its level in HP, loses all Transformation Buffs for one round, and the countdown die is rolled again on the next Initiative Count 99 as the Adversary forces the gates back open through sheer will. Any time the Adversary takes damage equal to five times its level from a single source, the countdown advances by 1.'),
('adversary-transformation-example/curse-mark-stage-1-2', 'Curse Mark Stage 1/2', 3, 'book/bingo-book', '2026-07-27', 20,
 'Concept: the Curse Mark spreads across the body, warping it and granting additional appendages — wings, extra limbs, hardened bone — grotesque, but undeniably powerful.

Buffs (2 selected): More Powerful (cursed chakra flowing through the Adversary enhances its strength); New Resistances (Resistance to Chakra and Bludgeoning, Vulnerability to the Staggered condition); New Movement (gains Flight).

Exclusive Freeform Ability — Cursed Regeneration (Cost 2 Discipline): once per turn on this Adversary''s turn, by spending 2 Tenacity, it regenerates Hit Points equal to the Medium column of its Freeform Jutsu table at its level, and reduces the ranks of one Physical condition on itself by 2 (this cannot restore HP lost while it was Staggered).

Optional Counter — Suppressed Regen: while this creature has two or more ranks of Weakened, the regeneration and both Resistances are suppressed until the next Initiative Count 99.'),
('adversary-transformation-example/earth-grudge-fear', 'Earth Grudge Fear', 4, 'book/bingo-book', '2026-07-27', 20,
 'Concept: the Adversary''s body is held together by dark threads of chakra that writhe across the battlefield as extensions of its will.

Buffs (2 selected): New Resistances (while any Heart remains active on the battlefield, the Adversary has Resistance to all Physical damage); Summon/Manifest (the Adversary possesses 5 Hearts total; when it Transforms, it immediately detaches 2 Hearts onto the battlefield as independent creatures, each acting on one of its Elite Actions — the remaining Hearts stay inside its body, each granting Immunity to one specific elemental damage type chosen at build time. While a Heart is detached and active, its associated Immunity is suppressed; if a detached Heart is destroyed, that Immunity is lost permanently. When a detached Heart is destroyed, the Adversary can spend 1 Tenacity on its next turn to detach a new one from its remaining pool, up to a maximum of 2 active at any time. A Heart''s level is 4 lower than the Adversary''s own, with HP rounded to the nearest 10); Expanded Senses (the thread network grants the Adversary 30 feet of Tremor Sense, and each Heart grants a further 15 feet of Tremor Sense the Adversary benefits from as well).

Exclusive Freeform Ability — Body Hijack (Cost 3 Discipline): as an Action, by spending 3 Tenacity, the Adversary launches threads toward one creature within 60 feet, forcing a Strength saving throw against its highest Save DC. On a failure, the target is Restrained and Grappled, pulled 10 feet closer at the start of each of its turns, and cannot benefit from Reactions for the duration; a creature that escapes the Grapple ends all effects. This ability cannot be used again until the start of the Adversary''s next turn at Initiative Count 99.

Optional Counter — Elemental Vulnerability: each Heart is vulnerable to the elemental damage type it is inferior to.');

CREATE TABLE adversary_battlefield_domains (
    slug             TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL,
    sort_order       INTEGER NOT NULL,
    source_book      TEXT,
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);
INSERT INTO adversary_battlefield_domains (slug, name, sort_order, source_book, source_version, source_page, description) VALUES
('adversary-battlefield-domain/soul-flame', 'Soul Flame', 1, 'book/bingo-book', '2026-07-27', 21,
 'Concept: the Adversary has bound itself to a dark ritual that feeds on isolation and pride; the battlefield pulses with its energy, and the players will quickly learn that standing alone is the worst thing they can do.

First Battlefield Turn (Initiative 99): the Battlefield randomly selects two players and marks each with a Soulfire Brand — a visible aura of crackling black flame with no immediate effect.

Second Battlefield Turn: the Brands detonate, with the result depending on where the Branded players are standing relative to every other creature. If at least one creature is within 10 feet of a Branded player, the Brand detonates outward, dealing Medium fire damage to every creature within 10 feet (the Branded player is unharmed); if no creature is within 10 feet, the Brand collapses inward, dealing Very High fire damage to the Branded player and Staggering them, ignoring resistance and immunity. After resolving, the Brands are dispelled. There is no outcome that harms no one — clustering the Branded players with allies spreads manageable damage, while isolating them concentrates severe damage onto the marked players but protects everyone else.

Counter: two chakra pylons exist on the battlefield at opposite ends, placed by the GM before combat, each with HP equal to 10% of the Adversary''s maximum HP and a specific random vulnerability. If both pylons are destroyed before the second Battlefield Turn in a round, both Brands are extinguished without detonating and the Battlefield Aura drops, with the Adversary losing its Resistance and condition immunity until the next Initiative Count 99. The pylons reform at the end of the following round. A successful Understanding the Battlefield check can reveal that the Brands exist and who carries them, the detonation rules, that the pylons exist and what destroying them does, or a pylon''s vulnerability.'),
('adversary-battlefield-domain/tsukuyomis-domain', 'Tsukuyomi''s Domain', 2, 'book/bingo-book', '2026-07-27', 22,
 'Concept: the Adversary has dragged the players into the Tsukuyomi, a pocket dimension of absolute darkness where time moves at the Adversary''s will and the rules of the outside world no longer apply.

First Battlefield Turn (Initiative 99): the GM rolls 1d6 to determine the current Moon State — a Full Moon (1-2), a Crescent Moon (3-4), or a New Moon (5-6) — narrated through environmental description. Three Pools of Darkness manifest at random points chosen by the GM, each a 10-foot-radius sphere that Blinds any creature standing within it; all prior Pools dissolve. Any player who casts Release Genjutsu, or any Genjutsu that removes Mental conditions, immediately creates a new Pool of Darkness centered on themselves, lasting until the next Initiative Count 99.

Second Battlefield Turn: depending on the Moon phase — Full Moon deals Very High psychic damage to every creature not standing in a Pool of Darkness (creatures inside are unaffected); Crescent Moon inflicts 5 ranks of Dazzled, until the end of their next turn, on every creature standing inside a Pool of Darkness (creatures outside are unaffected); New Moon deals Medium psychic damage and inflicts 2 ranks of Dazzled on every player and player-allied creature, unless all of them are occupying the same single Pool of Darkness simultaneously when the turn fires.

Counter: the Tsukuyomi is sustained by the Adversary''s Genjutsu concentration. If a player successfully applies the Concussed or Dazzled condition to the Adversary before the second Battlefield Turn in a given cycle, the Moon''s phase doesn''t change, the second turn''s effects are nullified, and the Battlefield Aura is suppressed (losing Resistance and condition immunity) until the next Initiative Count 99. A successful Understanding the Battlefield check can reveal what each Moon State''s effect is and how it works, that certain Genjutsu creates a new Pool of Darkness, or that disrupting the Adversary''s concentration before the second turn prevents the effect and which conditions trigger the Counter.'),
('adversary-battlefield-domain/chains-of-binding', 'Chains of Binding', 3, 'book/bingo-book', '2026-07-27', 22,
 'Concept: the Adversary weaves an invisible sealing network across the battlefield, binding players to each other through threads of chakra they cannot see until the bonds are tested.

First Battlefield Turn (Initiative 99): the Battlefield randomly pairs every player and player-allied creature (one is left unpaired and immune, if the count is odd). Each pair is bound by a Seal Chain, visible as faintly glowing script between them. While chained, they share each other''s chakra — when either casts a jutsu, the GM records the cost but doesn''t subtract it as normal; instead, at the end of the round, each takes chakra damage equal to the amount their linked ally spent.

Second Battlefield Turn (an intentional exception: this resolves at the end of the round rather than mid-round): each pair resolves based on their current distance from each other — 10 feet or closer, the Chain pulses inward, dealing Low fire damage to both but leaving the Chain intact and both unaffected by further penalty; 15 to 30 feet, the Chain holds but strains, inflicting 2 ranks of Weakened on both; further than 30 feet, the Chain snaps, dealing Very High fire damage and 4 ranks of Weakened to both. At the next Initiative Count 99, the Battlefield dissolves all existing Chains and redraws entirely new pairs, so no pairing lasts more than one round.

Counter: the sealing network is anchored to a central glyph visible somewhere on the Adversary''s body or the environment, chosen by the GM. Any jutsu with the Fuinjutsu keyword can target this anchor (even on the Adversary''s body); casting it triggers a contested Chakra Control or Ninshou check against the Adversary''s highest Jutsu Save DC, and on a success the Chains dissolve and the Battlefield Aura drops until the next Initiative Count 99. A successful Understanding the Battlefield check can reveal who is chained to whom, what each distance outcome produces, that the pairs redraw every round, or what triggers the Counter and where the anchor is — creatures with Chakra Sight or Chakra Sense can see the anchor, the chains, and who is tied to whom without any check.'),
('adversary-battlefield-domain/echoes-of-disaster', 'Echoes of Disaster', 4, 'book/bingo-book', '2026-07-27', 23,
 'Concept: the Adversary imprints fragments of its chakra onto players — echoes of its own destructive or draining energy that linger and grow if left unaddressed.

Setup Round — First Battlefield Turn (Initiative 99): the Battlefield selects one player and marks them with a Painful Echo, an invisible pulsing red aura visible only to creatures with Chakra Sight or Chakra Sense; a marked player can transfer their Echo to an adjacent willing ally as a Bonus Action, once per turn.

Setup Round — Second Battlefield Turn: the Battlefield selects a different player and marks them with a Draining Echo (an invisible blue shimmer, same visibility and transfer rule as the Painful Echo).

Trigger Round — Battlefield Turn (Initiative 99): any player still carrying a Painful Echo when this turn fires takes Very High psychic damage.

Trigger Round — Second Battlefield Turn: any player still carrying a Draining Echo suffers 5 ranks of Weakened, and the Adversary''s effective level for damage increases by +2 until the end of the next round. All Echoes are then dispelled, and the next Initiative Count 99 becomes a new Setup Round.

Counter: both Echoes can be forcibly dispelled before they detonate by targeting the Adversary with any jutsu that removes a Mental or Elemental condition of at least B-Rank, forcing a saving throw (GM''s choice, decided during encounter design) against the Jutsu Save DC of the casting player — on a failure, all Echoes are dispelled and the Battlefield Aura is suppressed until the next Initiative Count 99. A successful Understanding the Battlefield check can reveal that the Echoes exist, what each does when it detonates, that Echoes can be transferred and how, or what jutsu triggers the Counter — creatures with Chakra Sight or Chakra Sense can see the Echoes and who carries them without any check.'),
('adversary-battlefield-domain/phantom-legion', 'Phantom Legion', 5, 'book/bingo-book', '2026-07-27', 23,
 'Concept: the Adversary fractures pieces of its own chakra into autonomous battle constructs — pale imitations of itself that fight independently and feed their creator if allowed to persist. Each Phantom uses the Adversary''s level, lowest attack bonus, and Save DC, but has HP equal to 10 plus the Adversary''s level and uses the Minion/Standard damage table; Phantoms act immediately on the Elite Action following their spawn turn.

Setup Round — First Battlefield Turn (Initiative 99): the Battlefield spawns 1d4 Red Phantoms — aggressive constructs that only attack on each Elite Action they are granted.

Setup Round — Second Battlefield Turn: the Battlefield spawns 1d4 Blue Phantoms — passive constructs that orbit the Adversary, absorbing incoming damage directed at them, but can use their Reaction to cast a save-based effect against an attacking player.

Trigger Round — Battlefield Turn (Initiative 99): every surviving Red Phantom is consumed; for each one consumed this way, the Adversary recovers HP as if casting a Hit Point Burst.

Trigger Round — Second Battlefield Turn: every surviving Blue Phantom is consumed; for each one consumed this way, one player chosen randomly by the Battlefield gains 2 ranks of a condition they often inflict on hostile creatures (the GM decides this for each player before combat begins). The next Initiative Count 99 becomes a new Setup Round, spawning Red Phantoms again.

Counter: if all active Phantoms of both colors are destroyed before the second Battlefield Turn of a round, the Adversary is left without constructs to consume, and the Battlefield Aura drops until the next Initiative Count 99 — the round cycle then resets to a Setup Round. A successful Understanding the Battlefield check can reveal which Phantoms spawn when, what surviving Red or Blue Phantoms do when consumed, how each Phantom group behaves in combat, or that destroying all Phantoms in a single round triggers the Counter.');
