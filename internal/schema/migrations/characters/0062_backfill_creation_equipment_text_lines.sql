-- Fixes a regression from 0013_custom_items.sql: that migration moved every
-- character_inventory row to a real item_slug (backfilling old inline
-- custom_name rows into the new custom_items library), but SetEquipment
-- (internal/charstore/charstore.go, character creation's equipment step) was
-- never updated to match — its free-text branch (background equipment-pack
-- prose the ingest couldn't resolve to a real item, e.g. "a Book full of
-- teachings from your master") kept inserting bare custom_name rows with no
-- item_slug. loadCharacterInventory only ever selects item_slug now, so
-- these rows render as a blank, unbulked line with nothing to click —
-- reported live on a level-20 Genjutsu Specialist's sheet. The write path
-- itself is fixed in the same commit as this migration; this backfills
-- whatever such rows already exist.
--
-- Same insert-then-relink shape 0013 used, keyed off the inventory row's own
-- id (already unique) rather than the new custom_items row's id, so no
-- rowid-correlation trick is needed. Deliberately does NOT carry `notes`
-- into `description` the way 0013 did for its own backfill: notes on these
-- rows holds the 'creation-equipment' tag (see SetEquipment), not a real
-- item description, and copying it would just relabel every affected row
-- "creation-equipment" in its detail view.
INSERT INTO custom_items (slug, name, kind, bulk, created_at)
SELECT
    'custom/' || lower(
        replace(replace(replace(replace(trim(custom_name), ' ', '-'), '''', ''), '"', ''), '/', '-')
    ) || '-' || id,
    custom_name,
    coalesce(custom_kind, ''),
    custom_bulk,
    datetime('now')
FROM character_inventory
WHERE item_slug IS NULL;

UPDATE character_inventory
SET item_slug = (
    SELECT ci.slug FROM custom_items ci
    WHERE ci.name = character_inventory.custom_name
      AND ci.slug = 'custom/' || lower(
          replace(replace(replace(replace(trim(character_inventory.custom_name), ' ', '-'), '''', ''), '"', ''), '/', '-')
      ) || '-' || character_inventory.id
)
WHERE item_slug IS NULL;
