-- Custom items ("+ Add custom item" on the Inventory tab) used to live
-- entirely inline on character_inventory (custom_name/custom_kind/
-- custom_bulk, item_slug NULL) and were invisible to every catalogue-item
-- mechanism: no detail page, no reuse across characters/campaigns, no
-- weapon/toolkit rollable wiring. This table makes a custom item a real,
-- slug-addressable thing, same shape as a rules.db equipment row but living
-- in characters.db (the only writable, persistent file this app has —
-- rules.db is extracted to a fresh read-only temp file every run, see
-- cmd/n5e/rulesdb.go).
--
-- Not scoped by character_id: this is a personal, local item library shared
-- across every character/campaign in this characters.db, mirroring how
-- rules.db's equipment table is one shared catalogue every character reads
-- from. kind is free text (the player-typed "Type" column, e.g. "trinket",
-- "container") and deliberately NOT CHECK-constrained, so the backfill below
-- can carry over any existing custom_kind value without risking a failed
-- migration on an unexpected string. rollable_kind is the separate,
-- enum-like flag that actually gates the weapon/toolkit/other rollable
-- wiring — kept apart from kind so a free-text Type of "weapon" can never
-- accidentally make an item rollable, and so a rollable item can have any
-- Type at all.
CREATE TABLE custom_items (
    id            INTEGER PRIMARY KEY,
    slug          TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL DEFAULT '',
    rollable_kind TEXT CHECK (rollable_kind IN ('weapon', 'toolkit', 'other')),
    damage_dice   TEXT,
    damage_type   TEXT,
    properties    TEXT,   -- weapon rollables only: same free-text properties
                           -- column rules.db equipment has (finesse/thrown/
                           -- ammunition drive buildAttacks' ability pick)
    bulk          REAL,
    description   TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Backfill: every existing inline custom row becomes its own library entry.
-- Slugged off the inventory row's own id, which is already unique, so no
-- random suffix is needed and no two old rows can collide even if they
-- share a name. Each old row gets its own entry rather than merging
-- same-named rows across characters, since merging risks silently
-- conflating two unrelated homebrew items that just happen to share a name.
INSERT INTO custom_items (slug, name, kind, bulk, description, created_at)
SELECT
    'custom/' || lower(
        replace(replace(replace(replace(trim(custom_name), ' ', '-'), '''', ''), '"', ''), '/', '-')
    ) || '-' || id,
    custom_name,
    coalesce(custom_kind, ''),
    custom_bulk,
    notes,
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
