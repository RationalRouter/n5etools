-- The Seal Crafting planner's shared library of planned items. Like
-- custom_items, a crafted item is not owned by a character: inventory rows
-- point at it by its crafted/ slug, so one recipe can be carried by any
-- number of characters, and an edit reaches all of them.
--
-- Only the recipe is stored. Rank, minimum level, slots, cost and downtime
-- are recomputed from rules.db (internal/crafting.Calculate) on every read,
-- so a seal-data correction reaches every saved item.
--
-- crafter_inputs_json is internal/crafting.Crafting (who crafts it and with
-- what); it affects only the cost and downtime shown, never the item.
CREATE TABLE crafted_items (
    id                  INTEGER PRIMARY KEY,
    slug                TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    base_slug           TEXT NOT NULL,
    quality             TEXT NOT NULL DEFAULT 'standard',
    crafter_inputs_json TEXT NOT NULL DEFAULT '{}',
    notes               TEXT NOT NULL DEFAULT '',
    archived            INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

-- choices_json is a JSON array of the seal's picks: damage/creature type
-- IDs, a summon tribe slug, or a jutsu slug (internal/crafting.ChoiceRule).
CREATE TABLE crafted_item_seals (
    id              INTEGER PRIMARY KEY,
    crafted_item_id INTEGER NOT NULL REFERENCES crafted_items(id) ON DELETE CASCADE,
    seal_slug       TEXT NOT NULL,
    choices_json    TEXT NOT NULL DEFAULT '[]',
    sort_order      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX crafted_item_seals_item ON crafted_item_seals (crafted_item_id, sort_order);
