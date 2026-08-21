-- Titan (Science-Nin Mech Crafter's Ordnance Training) companion-specific
-- fields. Plain ADD COLUMN statements, mirroring 0065_nindog_fields.sql's
-- shape.
--
-- titan_specialization is a one-time pick ("When you craft your Titan, you
-- choose a Titan Specialization for it to following, picking between a
-- Legion Titan, a Monarch Titan, or a Ronin Titan" -- the source text never
-- mentions re-selecting one later) stored as a plain column and locked once
-- set, the same pattern nin_dog_breed/armor_chassis already use for their
-- own one-time crafting picks.
--
-- barrier_current/barrier_max track the Battery Powered Barrier's own
-- separate hit-point pool ("When a Titan is damaged, it subtracts hit
-- points from its barrier first") -- delta-editable via the same
-- +/-/blank shape hp_current/hp_max already use (handleCompanionIntField),
-- kept as its own pair of columns since the Barrier is a resource layered
-- ON TOP of the Titan's own hit points, not a replacement for them.
ALTER TABLE character_companions ADD COLUMN titan_specialization TEXT NOT NULL DEFAULT '';
ALTER TABLE character_companions ADD COLUMN barrier_current INTEGER;
ALTER TABLE character_companions ADD COLUMN barrier_max INTEGER;
