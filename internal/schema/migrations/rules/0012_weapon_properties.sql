-- Weapon property reference glossary + a many-to-many join so a weapon's
-- free-text "properties" column (e.g. "Thrown (30/60), Multiattack,
-- Ammunition") can be normalized into individual rollover-tooltip targets
-- on the new Items page. The join carries a per-weapon "detail" since the
-- same property can carry a different number on different weapons (Thrown
-- (20/60) vs Thrown (30/60); Deadly vs Deadly 2) — that number can't live on
-- the shared property definition.
--
-- Rules text hand-transcribed from Naruto 5e - Full Document.pdf (book/core
-- v3.11), the "Weapon Properties" section, pages 27-28 — not auto-parsed
-- (detection_status 'manual', matching the convention used elsewhere for
-- hand-entered lookup tables like 0002_seed.sql's jutsu_ranks/xp_levels).
CREATE TABLE weapon_properties (
    slug             TEXT PRIMARY KEY,          -- 'property/finesse'
    name             TEXT NOT NULL,             -- 'Finesse'
    description      TEXT NOT NULL,

    -- source_book is plain text, not an FK: unlike every other content
    -- table, this one is a static hand-seeded migration, not something a
    -- LoadX() ingest call ever upserts, so there's no ingest run to have
    -- already inserted a matching source_books row (that table is only ever
    -- populated at ingest time, never by a migration) — same reasoning
    -- 0002_seed.sql's hand-seeded lookup tables don't reference source_book
    -- at all. Kept here anyway, just without the FK, since it's still
    -- useful provenance documentation.
    source_book      TEXT,
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'manual'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual')),
    notes            TEXT
);

CREATE TABLE equipment_properties (
    equipment_slug TEXT NOT NULL REFERENCES equipment(slug),
    property_slug  TEXT NOT NULL REFERENCES weapon_properties(slug),
    detail         TEXT,                        -- '30/60', '2', NULL if the property carries no per-weapon number
    PRIMARY KEY (equipment_slug, property_slug)
);
CREATE INDEX idx_equipment_properties_equipment ON equipment_properties(equipment_slug);

-- Pass-through today (equipment has no override columns yet), but keeps
-- equipment consistent with every other content table's v_* convention —
-- the Items page reads this view, not the raw table, so nothing there has
-- to change later if an override column gets added.
CREATE VIEW v_equipment AS
SELECT slug, name, kind, cost_ryo, weight_lb, description,
       damage_dice, damage_type, properties, ammo_die, weapon_category,
       ac_bonus, armor_category, strength_req, stealth_disadv, bulk,
       armor_ability_1, armor_ability_2, armor_max_mod,
       seal_rank, seal_applies_to, detection_status, notes
FROM equipment;

INSERT INTO weapon_properties (slug, name, description, source_book, source_version, source_page) VALUES
('property/ammunition', 'Ammunition', 'You can use a weapon that has the ammunition property to make a ranged attack only if you have ammunition to fire from the weapon. Each time you attack with the weapon, you roll your ammunition die. If you use a ranged weapon that has the ammunition property to make a melee attack, you treat the weapon as an improvised weapon.', 'book/core', '3.11', 27),
('property/blocking', 'Blocking', 'Weapons with the Blocking property allow you to fight both defensively and offensively. While equipped, increase your AC by +1. This bonus can only be applied once. If this weapon property is applied to a weapon with the Unarmed weapon property, you do not gain this benefit if using another weapon.', 'book/core', '3.11', 27),
('property/critical', 'Critical', 'Weapons with the Critical property gain a +1 bonus to Critical Threat range when making weapon attacks for each Rank of Critical it has when you attack with it (Max. +3). This bonus can only be applied once, even if dual wielding, and thus does not stack.', 'book/core', '3.11', 26),
('property/deadly', 'Deadly', 'Weapons with the Deadly property add +1 additional damage die on a critical hit with a weapon attack, for each Rank of Deadly it has when you attack with it, once per turn (Max. 3).', 'book/core', '3.11', 26),
('property/disarm', 'Disarm', 'Weapons with the Disarm property can, in place of one weapon attack, allow its wielder to attempt to Disarm a target creature. Make a Disarm attempt.', 'book/core', '3.11', 27),
('property/evocation', 'Evocation', 'This property is exclusive to Combat Scrolls. When you first gain a weapon with this property, select one Ninjutsu, Genjutsu, or weapon you have access to that deals damage. You seal an echo of that jutsu or weapon into this weapon, and its damage type becomes the damage type of this weapon, but it does not add your ability modifier to damage rolls and you can make no more than 1 attack with this weapon when you take the Attack action. At character levels 5, 11, and 17, the damage die increases by one step.', 'book/core', '3.11', 27),
('property/finesse', 'Finesse', 'When making a weapon attack with a finesse weapon, you use your choice of your Strength or Dexterity modifier for the attack and damage rolls. You must use the same modifier for both rolls.', 'book/core', '3.11', 27),
('property/flexible', 'Flexible', 'Weapons with the Flexible property have an associated damage type (e.g. Flexible (Bludgeoning)). This weapon can also deal that damage type and qualify for abilities, jutsu, and features that rely on that damage type, but its damage die is 1 step lower, down to a minimum of a d4. You decide on the damage type when you roll damage with this weapon.', 'book/core', '3.11', 28),
('property/grapple', 'Grapple', 'Weapons with the Grapple property can, in place of one weapon attack, allow its wielder to attempt to Grapple a target creature. Make a Grapple attempt.', 'book/core', '3.11', 27),
('property/heavy', 'Heavy', 'A creature must have a Strength score of 16 or higher to use this weapon without penalty. If not, they make attacks at Disadvantage while wielding this weapon. Creatures add half of their Strength modifier to this weapon''s damage rolls.', 'book/core', '3.11', 27),
('property/hidden', 'Hidden', 'You have advantage on Dexterity (Sleight of Hand) checks made to conceal a hidden weapon.', 'book/core', '3.11', 27),
('property/lethal', 'Lethal', 'Weapons with the Lethal property increase their damage die by +1 against a surprised creature or a creature that cannot see its attacker, for each rank of Lethal it has, once per turn. This property can stack on a single weapon no more than 5 times (e.g. Lethal 2 adds +2 damage die).', 'book/core', '3.11', 27),
('property/light', 'Light', 'A light weapon is smaller and easy to handle, making it ideal for use when fighting with two weapons. Once per turn, while wielding two weapons with this property, when you make an attack with a weapon in one hand as part of the Attack action, you can make a second attack using the other weapon with this property, as part of that same attack.', 'book/core', '3.11', 27),
('property/loading', 'Loading', 'A limited number of shots can be made with a weapon that has the Loading property. A character must then reload it using their Use an Object action. You must have one free hand to reload.', 'book/core', '3.11', 27),
('property/multiattack', 'Multiattack', 'A weapon with the Multiattack property can also be used to attack as a bonus action. You cannot add your ability modifier or any bonus to damage rolls using your weapon''s (or Unarmed Damage''s) die, unmodified by jutsu, feats or features.', 'book/core', '3.11', 27),
('property/range', 'Range', 'A weapon that can be used to make a ranged attack has a range shown in parentheses after the ammunition or thrown property. The first number is the weapon''s normal range in feet, the second is its maximum range. Attacking a target beyond normal range gives disadvantage; you can''t attack a target beyond the weapon''s long range.', 'book/core', '3.11', 27),
('property/reach', 'Reach', 'This weapon adds 5 feet to your reach for each Rank of Reach it has when you attack with it (e.g. Reach 1 adds 5 feet of range).', 'book/core', '3.11', 27),
('property/returning', 'Returning', 'If a weapon has the Returning property, after throwing it to make a ranged attack, you can call it back to you if it was thrown less than 30 feet from you. You must have one hand free to catch it.', 'book/core', '3.11', 27),
('property/special', 'Special', 'A property described individually for the specific weapon that has it (e.g. the Battle Wire''s unique trap-setting actions) rather than a generic rule shared across weapons.', 'book/core', '3.11', 28),
('property/tactical', 'Tactical', 'Once per turn, weapons with the Tactical property deal +1 additional damage to a target for each rank of any Physical condition the target has (e.g. a creature with Bruised 3 and Bleed 2 takes +5 additional damage).', 'book/core', '3.11', 28),
('property/thrown', 'Thrown', 'If a weapon has the thrown property, you can throw the weapon to make a ranged attack using your Strength instead of Dexterity for attack and damage rolls.', 'book/core', '3.11', 27),
('property/trip', 'Trip', 'Weapons with the Trip property can, in place of one weapon attack, allow its wielder to attempt to Trip or push a target creature. Make a Trip or Shove attempt; on a success, deal the weapon''s damage.', 'book/core', '3.11', 27),
('property/two-handed', 'Two-Handed', 'This weapon requires two hands to use.', 'book/core', '3.11', 27),
('property/unarmed', 'Unarmed', 'This weapon is equipped to both hands and cannot be disarmed. It doesn''t prevent you from benefiting from a Taijutsu Stance, and you can still use Hand Seals, Chakra Seals, and wield a separate weapon while using it. It uses your Unarmed Damage as its base damage (otherwise its listed damage die). Bonuses or penalties to weapon attacks also apply to Unarmed Attacks made with it, and it can be used to cast both Taijutsu and Bukijutsu.', 'book/core', '3.11', 28),
('property/versatile', 'Versatile', 'This weapon can be used with one or two hands. A damage value in parentheses shows the damage when the weapon is used with two hands to make a melee attack.', 'book/core', '3.11', 28),
('property/volatile', 'Volatile', 'Weapons with the Volatile property are dangerous and unwieldy. You cannot add your ability modifier to damage rolls using these weapons. If your d20 result on a weapon attack is equal to or less than the Volatile rank, the weapon backfires, dealing its damage to you. If this weapon''s damage die is increased in count or size, the Volatile value increases by +4.', 'book/core', '3.11', 28),
('property/winding', 'Winding', 'Weapons with the Winding property can be wound up to store energy before unleashing it in one blow. In place of a weapon attack, you can choose to wind up your weapon; if you do, the next weapon attack you make with this weapon has advantage.', 'book/core', '3.11', 28);
