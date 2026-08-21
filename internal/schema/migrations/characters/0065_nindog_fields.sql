-- Nin-Dog (Inuzuka clan's Beast Master clan feature) companion-specific
-- fields. Plain ADD COLUMN statements, not a rebuild -- unlike 0064, none
-- of these touch the kind CHECK constraint.
--
-- nin_dog_breed is a one-time pick (Young Inuit / Young Kugsha / Young
-- Tamaskan -- "Choose between one of the following breeds", the source
-- text never mentions re-selecting one later) stored as a plain column and
-- locked once set, the exact same pattern armor_chassis already uses for a
-- puppet's own one-time Armor Chassis pick (see SetCompanionFields' CASE
-- guard).
--
-- jutsu_slots_current/jutsu_slots_max track a Nin-Dog's own Jutsu Slots
-- resource ("Your Nin-Dog regains 1 Jutsu Slot on a Short Rest, and all of
-- their spent Jutsu Slots on a Long Rest... If your Nin-Dog spends all of
-- its Jutsu Slots, it does not vanish or become unsummoned") -- delta-
-- editable via the same +/-/blank shape AC and Max HP already use
-- (handleCompanionIntField), kept as its own pair of columns rather than
-- folded into hp_current/hp_max since a Nin-Dog's Jutsu Slots are a
-- completely separate resource from its hit points.
ALTER TABLE character_companions ADD COLUMN nin_dog_breed TEXT NOT NULL DEFAULT '';
ALTER TABLE character_companions ADD COLUMN jutsu_slots_current INTEGER;
ALTER TABLE character_companions ADD COLUMN jutsu_slots_max INTEGER;
