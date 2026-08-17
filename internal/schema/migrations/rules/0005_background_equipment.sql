-- Backgrounds print two equipment lines the original table had no home for:
-- "Equipment:" (a fixed item package) and "Equipment Pack:" (a choice of
-- named packs). Kept verbatim; the creation flow presents them as text.
ALTER TABLE backgrounds ADD COLUMN equipment_text TEXT;
ALTER TABLE backgrounds ADD COLUMN equipment_pack_text TEXT;
