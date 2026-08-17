-- Part 10: "Add custom item" needs to carry the same fields a rules-catalogue
-- item gets from the equipment table (kind, bulk) so a homebrew item can
-- affect the Bulk/carry-capacity readout the same way a real one does and
-- shows a real value in the inventory table's Type/Bulk columns instead of
-- "from creation" / "—". Only meaningful when item_slug IS NULL — a
-- catalogue row still gets kind/bulk from the equipment table join, same as
-- before.
ALTER TABLE character_inventory ADD COLUMN custom_kind TEXT;
ALTER TABLE character_inventory ADD COLUMN custom_bulk REAL;
