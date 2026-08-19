-- Elemental Innovationist's Exoskeleton (Science-Nin subclass feature, 3rd
-- level): "You may don/doff your Exoskeleton over the course of 1 minute.
-- While donned, you may use Intelligence to calculate your armor class..."
-- No equipment row backs this item (it isn't purchasable/equippable through
-- the normal inventory), so its donned/doffed state needs its own boolean,
-- same shape as the existing "inspiration" toggle column.
ALTER TABLE characters ADD COLUMN exoskeleton_donned INTEGER NOT NULL DEFAULT 0 CHECK (exoskeleton_donned IN (0,1));
