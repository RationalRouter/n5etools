-- Orochimaru's Observation Compendium's W.O.W (Weapons of Wonder) catalog
-- (Science-Nin, Elemental Innovationist) lost an entire 10th entry --
-- "Primeval Bow" -- during PDF text extraction: pages 247/250 print two
-- columns per page, and a full-width or sidebar table sitting below/beside
-- the columns got glued onto whichever entry's own text ran immediately
-- before it in raw extraction order, instead of the entry it actually
-- belongs to. Same bug shape already fixed elsewhere in this catalog's own
-- ingest (see internal/store's PDF-column-bleed fixups) -- this migration
-- is the one-off content correction for these specific rows, verified
-- directly against the source PDF (Orochimaru's Observation Compendium,
-- printed pages 245 and 248-249; render checked visually, not just
-- pdftotext output, since -layout's column heuristics get confused by
-- hanging-indent bullets in this exact spot).
--
-- 1. Alchemical Staff / Blunderbuss (printed page 245): the full-width
--    "ALCHEMICAL STAFFS" table (5 elemental Charged Shot variants) prints
--    below BOTH columns, but belongs to Alchemical Staff alone -- its own
--    intro text says "Select one of the following Alchemical Staffs
--    below." It was appended to Blunderbuss's row instead (the entry whose
--    own column happened to finish right before the footer table in
--    extraction order). Moved back to Alchemical Staff; stripped from
--    Blunderbuss.
--
-- 2. Terra Spikes / Ray Gun (printed pages 248-249): Primeval Bow's own
--    base description + Ascension paragraph bled onto the end of Terra
--    Spikes' row, and its "PRIMEVAL BOW AWAKENINGS" Ascension-effect choice
--    table (4 Awakenings: Ancient Earth, Eternal Storm, Lost Void, Royal
--    Family) got spliced into the MIDDLE of Ray Gun's own Ascension
--    section, between Ray Gun's Industrial and Otherworldly bullets --
--    Primeval Bow's own sidebar table runs down the right column of both
--    pages, parallel to Terra Spikes/Ray Gun's left-column text, so it
--    shares a page-turn boundary with each without belonging to either.
--    Both fragments are stripped from Terra Spikes/Ray Gun and combined
--    into a new Primeval Bow row (sort_order 6, between Terra Spikes and
--    Ray Gun, matching its position at the top of Ray Gun's own printed
--    page). Existing rows from sort_order 6 on shift up by one. The
--    bled-in "PRIMEVAL BOW" entry-name header immediately before "Clip
--    Size:" is dropped from the recovered row, matching every other W.o.W
--    entry's own convention of never repeating its own name inline (the
--    name column already carries it) -- confirmed by checking all 9
--    existing rows have no such prefix.
--
--    (The trailing sentence right after the Royal Family Awakening on the
--    printed page turn -- "At the start of each of their turns creatures
--    may remake the saving throw... turned into ash." -- reads like it
--    could continue Primeval Bow's table, but it is confirmed to be
--    Sinister Skull's own Vaporizing Skull bullet by cross-checking
--    Sinister Skull's own row, which already contains that exact sentence
--    correctly in place. Left untouched.)
--
-- All five rows get detection_status='manual' -- the underlying PDF
-- extraction bug in the ingest parser is NOT fixed by this migration, only
-- this catalog's already-ingested data, so a future from-scratch re-ingest
-- would reproduce the exact same bleed. 'manual' stops that future
-- re-ingest from silently reverting this fix (decideStatus in
-- internal/store demotes on any content mismatch instead), same
-- convention as 0030/0035's own stat-block recoveries.
--
-- Every statement re-checks the exact broken text before acting, so this
-- migration is a no-op on a database that already had it applied.
--
-- The Primeval Bow INSERT is additionally guarded by "AND EXISTS (... the
-- elemental-innovationist subclass row ...)", same reasoning as 0048's own
-- guard: without it, re-running migrations against an empty schema-only
-- database (as internal/store's own tests do) would try to insert a
-- class_options row referencing a class_slug/subclass_slug that doesn't
-- exist yet and fail its FOREIGN KEY constraint.

-- 1a. Move the ALCHEMICAL STAFFS table onto Alchemical Staff's own row.
UPDATE class_options
SET description = description || ' ALCHEMICAL STAFFS Alchemical Staff Damage Type Charged Shot Staff of Earth Earth Damage On a hit, the ground beneath the target hit in a 5-foot radius cracks and spews mud. Targets within range must succeed a Strength saving throw or become Restrained and Bruised until the end of their next turn as they are buried in mud. Staff of Wind Wind Damage You create a 20-foot tall, 10-foot wide tornado upon hitting a target. Each creature within range must succeed a Dexterity saving throw or gain 1 rank of Bleeding and be knocked up 45 feet into the air. At the end of each turn, the tornado moves 10 feet in any direction of your choice, prompting the same saving throw. The tornado disappears at the start of your next turn. A creature cannot be affected by this tornado more than once for the duration of its existence. Staff of Fire Fire Damage You fire a ball of condensed magma which spews across the ground on a hit. Record the damage dealt with the Alchemical Staff’s [Weapon’s Damage Die]. All creatures within a 15-foot radius must make a Dexterity saving throw, taking half the damage recorded as fire damage and gain 1 rank of Burned on a failure, or half as much damage and no further effects on a success. This magma lasts until the end of your next turn, and counts as difficult terrain. A creature that attempts to move in this magma takes half the damage recorded as fire damage. Damage from this Charged Shot pierces DR. Staff of Water Cold Damage On a hit, the water fired crystalizes and explodes, creating a small blizzard. All creatures within a 30-foot cube must make a Strength saving throw, gaining 1 rank of Chilled and taking 3d8 cold damage on a failure, or half as much damage and no chilled on a success. Creatures reduced to 0 hit points by this effect, instead fall to 1 hit point and become Petrified in ice. A cloud of ice particles covers the affected area for the next minute as with a Smoke Bomb. Staff of Lightning Lightning Damage You fire a large ball of electricity which rapidly jolts to creatures near it. On a hit, the target must succeed a Constitution saving throw or gain 1 rank of Shocked and Dazzled. Regardless, a large 10 by 10-foot sphere is created next to the target, causing all hostile creatures who start their turn within 30 feet of it to be targeted by a new ranged Ninjutsu attack, taking 5d4 lightning damage and gaining 1 rank of Shocked on a hit. This sphere lasts until the start of your next turn or until destroyed (the sphere has 10 AC and 1 HP)',
    detection_status = 'manual'
WHERE slug = 'class/science-nin/option/w-o-w-weapons-of-wonder/alchemical-staff'
  AND description NOT GLOB '*ALCHEMICAL STAFFS Alchemical Staff Damage Type*';

-- 1b. Strip it back off of Blunderbuss.
UPDATE class_options
SET description = REPLACE(description, ' ALCHEMICAL STAFFS Alchemical Staff Damage Type Charged Shot Staff of Earth Earth Damage On a hit, the ground beneath the target hit in a 5-foot radius cracks and spews mud. Targets within range must succeed a Strength saving throw or become Restrained and Bruised until the end of their next turn as they are buried in mud. Staff of Wind Wind Damage You create a 20-foot tall, 10-foot wide tornado upon hitting a target. Each creature within range must succeed a Dexterity saving throw or gain 1 rank of Bleeding and be knocked up 45 feet into the air. At the end of each turn, the tornado moves 10 feet in any direction of your choice, prompting the same saving throw. The tornado disappears at the start of your next turn. A creature cannot be affected by this tornado more than once for the duration of its existence. Staff of Fire Fire Damage You fire a ball of condensed magma which spews across the ground on a hit. Record the damage dealt with the Alchemical Staff’s [Weapon’s Damage Die]. All creatures within a 15-foot radius must make a Dexterity saving throw, taking half the damage recorded as fire damage and gain 1 rank of Burned on a failure, or half as much damage and no further effects on a success. This magma lasts until the end of your next turn, and counts as difficult terrain. A creature that attempts to move in this magma takes half the damage recorded as fire damage. Damage from this Charged Shot pierces DR. Staff of Water Cold Damage On a hit, the water fired crystalizes and explodes, creating a small blizzard. All creatures within a 30-foot cube must make a Strength saving throw, gaining 1 rank of Chilled and taking 3d8 cold damage on a failure, or half as much damage and no chilled on a success. Creatures reduced to 0 hit points by this effect, instead fall to 1 hit point and become Petrified in ice. A cloud of ice particles covers the affected area for the next minute as with a Smoke Bomb. Staff of Lightning Lightning Damage You fire a large ball of electricity which rapidly jolts to creatures near it. On a hit, the target must succeed a Constitution saving throw or gain 1 rank of Shocked and Dazzled. Regardless, a large 10 by 10-foot sphere is created next to the target, causing all hostile creatures who start their turn within 30 feet of it to be targeted by a new ranged Ninjutsu attack, taking 5d4 lightning damage and gaining 1 rank of Shocked on a hit. This sphere lasts until the start of your next turn or until destroyed (the sphere has 10 AC and 1 HP)', ''),
    detection_status = 'manual'
WHERE slug = 'class/science-nin/option/w-o-w-weapons-of-wonder/blunderbuss'
  AND description GLOB '*ALCHEMICAL STAFFS Alchemical Staff Damage Type*';

-- 2a. Strip Primeval Bow's bled base description + Ascension paragraph off
--     of Terra Spikes.
UPDATE class_options
SET description = REPLACE(description, ' PRIMEVAL BOW Clip Size: 7 [14] You infuse your element into multiple stones that come together and form a strange, primeval bow, coupled with a drawstring made of pure chakra. Those who wield this bow feel various emotions emanating from it, as if the emotions and stories buried within the earth from eons past manifest within this weapon. This weapon possesses Range (200/500), Tactical, and Two-Handed properties and deals 1d10 piercing/fire damage. This weapon counts as a Bow, and can be used as a component in Bukijutsu. If you do, you can double the cost of the Bukijutsu and round up to the nearest interval of 5 and pay it with your CCD. If you do, you may use Intelligence as your Taijutsu casting modifier for the duration of the casting Each attack with the Bow expends 1 Ammo. Once per turn, when you make an attack with the Primeval Bow, you can spend 7 chakras from your CCD to increase the Bow’s potency before you attack. When you do, your next attack before the end of the turn has advantage, does not break stealth, gains a +1 bonus to critical threat range, and deals an extra 4d4 damage on a hit. ASCENSION: PRIMEVAL BOW If the Primeval Bow is Ascended, its [Weapon’s Damage] becomes 2d10, and the stories buried deep within its stones manifest. Gain one of the following Awakenings above.', ''),
    detection_status = 'manual'
WHERE slug = 'class/science-nin/option/w-o-w-weapons-of-wonder/terra-spikes'
  AND description GLOB '*come together and form a strange, primeval bow*';

-- 2b. Strip Primeval Bow's bled Awakenings table out of the middle of Ray
--     Gun's own Ascension section (reconnects Industrial directly to
--     Otherworldly).
UPDATE class_options
SET description = REPLACE(description, ' PRIMEVAL BOW AWAKENINGS Awakening Effects Primeval Bow of the Ancient Earth The rocks of your Primeval Bow crack and burst, and the magma of the ancient world courses through. • Increase your Bow’s [Weapon’s Damage] by +2. • When you would increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, the target hit by your attack must succeed a Strength saving throw or become Restrained in a stalactite of rock. At the start of each of their turns, a volcanic eruption occurs within the rock formation, causing the creature to take 4d10+4 fire damage and gain +1 ranks of Burned and Weakened. They take this damage at the start of each of their turns. At the end of each of their turns they may remake the Strength saving throw to escape. Primeval Bow of the Eternal Storm The rocks of your Primeval Bow crack and burst, and everlasting winds and lightning now swirl around it. • This bow now deals piercing and lightning damage. When you make an attack with this Bow, that does not hit a creature, you may spend 5 chakras from your CCD to leave behind a floating orb of lightning until the start of your next turn. This orb of lightning deals 15 lightning damage to any creature that walks through it, once per turn. The orb emits 15 feet of bright light and 30 feet of dim light. • When you would increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, a small, electrified storm forms where your attack lands (10-foot wide, 30-foot tall cylinder). The storm travels up to 20 feet in any direction of your choice in a straight line. Any creature that comes in contact with the storm must make a Dexterity saving throw, taking 3d10 wind and 3d10 lightning damage and gaining 1 rank of Shocked on a failure, or half as much damage and no effects on a success. The storm disappears at the end of the turn. Primeval Bow of the Lost Void The rocks of your Primeval Bow crack and burst, and a misty doorway to another realm seeps through it. • This bow now deals piercing and psychic damage. When you make an attack with your bow, you may spend 10 chakras from your CCD to open a small rift in time and space, forcing all creatures within 5 feet of where your attack lands to succeed a Charisma saving throw or be teleported to different spaces within 20 feet that can hold them (you decide for each creature). On a critical hit, the target hit makes this saving throw at disadvantage. A creature can only be targeted by a rift once per turn. • When you increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, you open a large rift and 1d4+1 “Otherworldly Aberrations” appear and begin attacking targets of your choice within 60 feet. These creatures float to their targets and cannot be damaged. At the start of a targeted creature’s turn, they must succeed a Wisdom saving throw or take Xd12 psychic damage and gain a number of ranks of Demoralized ranks against you equal to half of X (Min. 1), where X equals the number of skulls targeting them +1. The skulls then disappear. Primeval Bow of the Royal Family The rocks of your Primeval Bow crack and burst as royal chakra and deep sorrows now emanate from its form. When you lay hands on this bow, the mournful roar of a lion echoes through your mind. • This bow now deals piercing and force damage. This Bow gains the Splash Damage effect of the Ray Gun Weapon of Wonder, once per turn. • When you would increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, the spirit of a lion manifests as you fire, causing you to target all creatures within a 30-foot-long, 10-foot wide line with your attack roll. All affected creatures must also make a Dexterity saving throw, being knocked back 30 feet and falling prone on a failed save. If you score a critical hit with this attack, only the first target affected suffers the effects of the critical hit and makes their saving throw at disadvantage.', ''),
    detection_status = 'manual'
WHERE slug = 'class/science-nin/option/w-o-w-weapons-of-wonder/ray-gun'
  AND description GLOB '*PRIMEVAL BOW AWAKENINGS Awakening Effects*';

-- 2c. Make room at sort_order 6 for the recovered Primeval Bow row.
UPDATE class_options
SET sort_order = sort_order + 1
WHERE list_name = 'W.O.W (Weapons of Wonder)'
  AND sort_order >= 6
  AND NOT EXISTS (
      SELECT 1 FROM class_options
      WHERE slug = 'class/science-nin/option/w-o-w-weapons-of-wonder/primeval-bow'
  );

-- 2d. Insert the recovered Primeval Bow entry.
INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order, source_book, source_version, source_page, detection_status, notes)
SELECT
    'class/science-nin/option/w-o-w-weapons-of-wonder/primeval-bow',
    'class/science-nin',
    'class/science-nin/group/scientific-inquiry/elemental-innovationist',
    'W.O.W (Weapons of Wonder)',
    'Primeval Bow',
    'Clip Size: 7 [14] You infuse your element into multiple stones that come together and form a strange, primeval bow, coupled with a drawstring made of pure chakra. Those who wield this bow feel various emotions emanating from it, as if the emotions and stories buried within the earth from eons past manifest within this weapon. This weapon possesses Range (200/500), Tactical, and Two-Handed properties and deals 1d10 piercing/fire damage. This weapon counts as a Bow, and can be used as a component in Bukijutsu. If you do, you can double the cost of the Bukijutsu and round up to the nearest interval of 5 and pay it with your CCD. If you do, you may use Intelligence as your Taijutsu casting modifier for the duration of the casting Each attack with the Bow expends 1 Ammo. Once per turn, when you make an attack with the Primeval Bow, you can spend 7 chakras from your CCD to increase the Bow’s potency before you attack. When you do, your next attack before the end of the turn has advantage, does not break stealth, gains a +1 bonus to critical threat range, and deals an extra 4d4 damage on a hit. ASCENSION: PRIMEVAL BOW If the Primeval Bow is Ascended, its [Weapon’s Damage] becomes 2d10, and the stories buried deep within its stones manifest. Gain one of the following Awakenings above. PRIMEVAL BOW AWAKENINGS Awakening Effects Primeval Bow of the Ancient Earth The rocks of your Primeval Bow crack and burst, and the magma of the ancient world courses through. • Increase your Bow’s [Weapon’s Damage] by +2. • When you would increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, the target hit by your attack must succeed a Strength saving throw or become Restrained in a stalactite of rock. At the start of each of their turns, a volcanic eruption occurs within the rock formation, causing the creature to take 4d10+4 fire damage and gain +1 ranks of Burned and Weakened. They take this damage at the start of each of their turns. At the end of each of their turns they may remake the Strength saving throw to escape. Primeval Bow of the Eternal Storm The rocks of your Primeval Bow crack and burst, and everlasting winds and lightning now swirl around it. • This bow now deals piercing and lightning damage. When you make an attack with this Bow, that does not hit a creature, you may spend 5 chakras from your CCD to leave behind a floating orb of lightning until the start of your next turn. This orb of lightning deals 15 lightning damage to any creature that walks through it, once per turn. The orb emits 15 feet of bright light and 30 feet of dim light. • When you would increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, a small, electrified storm forms where your attack lands (10-foot wide, 30-foot tall cylinder). The storm travels up to 20 feet in any direction of your choice in a straight line. Any creature that comes in contact with the storm must make a Dexterity saving throw, taking 3d10 wind and 3d10 lightning damage and gaining 1 rank of Shocked on a failure, or half as much damage and no effects on a success. The storm disappears at the end of the turn. Primeval Bow of the Lost Void The rocks of your Primeval Bow crack and burst, and a misty doorway to another realm seeps through it. • This bow now deals piercing and psychic damage. When you make an attack with your bow, you may spend 10 chakras from your CCD to open a small rift in time and space, forcing all creatures within 5 feet of where your attack lands to succeed a Charisma saving throw or be teleported to different spaces within 20 feet that can hold them (you decide for each creature). On a critical hit, the target hit makes this saving throw at disadvantage. A creature can only be targeted by a rift once per turn. • When you increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, you open a large rift and 1d4+1 “Otherworldly Aberrations” appear and begin attacking targets of your choice within 60 feet. These creatures float to their targets and cannot be damaged. At the start of a targeted creature’s turn, they must succeed a Wisdom saving throw or take Xd12 psychic damage and gain a number of ranks of Demoralized ranks against you equal to half of X (Min. 1), where X equals the number of skulls targeting them +1. The skulls then disappear. Primeval Bow of the Royal Family The rocks of your Primeval Bow crack and burst as royal chakra and deep sorrows now emanate from its form. When you lay hands on this bow, the mournful roar of a lion echoes through your mind. • This bow now deals piercing and force damage. This Bow gains the Splash Damage effect of the Ray Gun Weapon of Wonder, once per turn. • When you would increase your Bow’s Potency, you may spend 1 additional Ammo. If you do, the spirit of a lion manifests as you fire, causing you to target all creatures within a 30-foot-long, 10-foot wide line with your attack roll. All affected creatures must also make a Dexterity saving throw, being knocked back 30 feet and falling prone on a failed save. If you score a critical hit with this attack, only the first target affected suffers the effects of the critical hit and makes their saving throw at disadvantage.',
    6,
    'book/class-compendium',
    '3.12',
    250,
    'manual',
    'Recovered from a PDF-column-bleed bug -- see this migration''s own header comment. Wired into cmd/n5e/wow_weapons.go''s wowWeaponSpecs for mechanical attack-line modeling; it is otherwise a fully normal, pickable W.O.W catalog entry via the generic class_options loader.'
WHERE NOT EXISTS (
    SELECT 1 FROM class_options
    WHERE slug = 'class/science-nin/option/w-o-w-weapons-of-wonder/primeval-bow'
) AND EXISTS (
    SELECT 1 FROM subclasses
    WHERE slug = 'class/science-nin/group/scientific-inquiry/elemental-innovationist'
);
