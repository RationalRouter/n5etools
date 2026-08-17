-- Science-Nin's Titan option list prints a shared base unit card (creature
-- type, HP/AC formula, ability scores, senses, Battery Powered Barrier/Extra
-- Attack/Gradual Expansion/Ninja Tool Integration/Steady Improvement/Titan
-- Specialization traits, and a Bash natural weapon) as a sidebar box, same
-- as Puppet Master's Puppet Tool (see 0028_puppet_tool_stat_block.sql). The
-- flat PDF text extractor has no column/box detection, so this box landed
-- glued onto the end of "Ronin Specialization"'s own bullet list — the ONE
-- option among Legion/Monarch/Ronin Specialization that happened to be
-- current when the extractor crossed it. internal/parse/subclasses.go now
-- splits this out at ingest time (titanUnitCardMarker); this migration both
-- adds the table that split data lands in AND patches the already-shipped
-- bad row directly, same two-part fix shape as 0028.
--
-- Kept as one raw text field, not parsed into columns like
-- puppet_tool_stat_block — nothing in the app renders this yet (Science-Nin
-- has no companion tab), so structuring it now would be speculative; this
-- table exists only to stop the corruption in Ronin Specialization's own
-- description, with the real data preserved for a future Science-Nin pass.
CREATE TABLE titan_unit_card (
    class_slug       TEXT PRIMARY KEY REFERENCES classes(slug),
    raw_text         TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

-- detection_status='manual' stops a future real re-ingest from silently
-- reintroducing drift if the parser behaves even slightly differently
-- against a later PDF revision.
UPDATE class_options
SET description = 'The Ronin Titans, also sometimes referred to as Stryder Titans, focus on mobility, able to move around the battlefield at breakneck speeds and locate what can normally not be seen. These Titans gain the following benefits; • The Ronin Titan has a movement speed of 35 feet. • The Ronin Titan increases its Dexterity score by +4. The maximum for this score also increases by +4. • The Ronin Titan can take the Dash or Disengage actions as a bonus action. When performing either action, the Ronin Titan ignores land-based difficult terrain. • The Ronin Titan has an increased critical threat range of +1 on melee weapon and Taijutsu attacks.',
    detection_status = 'manual'
WHERE slug = 'class/science-nin/option/titan/ronin-specialization';

-- Guarded with WHERE EXISTS for the same reason 0028's INSERT is: this
-- migration also runs against a fresh, empty rules DB (test fixtures, a
-- from-scratch first ingest) where no 'class/science-nin' row exists yet.
INSERT INTO titan_unit_card (class_slug, raw_text, source_book, source_version, source_page, detection_status)
SELECT 'class/science-nin',
    'X Construct, Proficiency bonus = your proficiency bonus, unaligned Hit Points 20+[2*Titan’s Constitution Modifer x Science Nin level] Speed 30 ft. 15 (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3) Senses Darkvision (30 feet), Passive Perception(Yours + Intelligence Modifer) Battery Powered Barrier: All Titans are fitted with a barrier that protects the titan from damage. When a Titan is damaged, it subtracts hit points from its barrier first. The Battery Powered Barrier has a maximum number of hit points equal to twice your Science-Nin level, and on your turn, you can spend increments of 5 chakras from your CCD to replenish 10 of the barrier''s hit points. Extra Attack. Your Titan can attack twice with the attack action. Gradual Expansion. Your Titan starts off as Large, becoming Huge at 14th level. Ninja Tool Integration. The Titan''s attacks are chakra enhanced. Steady Improvement. The Titan gains an additional number of ASI points equal to 1 + your proficiency bonus. You distribute these points when you craft your Titan, and you can redistribute them during a long rest. Titan Specialization. When you craft your Titan, you choose a Titan Specialization for it to following, picking between a Legion Titan, a Monarch Titan, or a Ronin Titan, which grant your Titan additional abilities. Bash. Melee Weapon Attack: reach 10 ft., one target. Hit: 1d6+ Str + Dex in bludgeoning damage. This weapon can be used for the unarmed damage of Taijutsu. Any other upgrades you have with the Weapon Keyword',
    'book/class-compendium', NULL, 236, 'manual'
WHERE EXISTS (SELECT 1 FROM classes WHERE slug = 'class/science-nin');
