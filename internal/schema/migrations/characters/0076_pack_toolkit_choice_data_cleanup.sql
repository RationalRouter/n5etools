-- Backfills three live characters whose inventory still carries the effects
-- of gaps in pack unpacking that 0075's schema widening and
-- cmd/n5e/pack_toolkit_choice.go's resolver now fix going forward
-- (Crafter's/Captain's/Infiltrator's Pack toolkit picks used to leave inert
-- free-text lines with no way to resolve them into a real item or a tool
-- proficiency). This migration does not change any code path — it only
-- moves three characters' already-existing rows into the shape the fixed
-- mechanism would have produced had the pack been unpacked correctly the
-- first time, plus one unrelated pre-existing data gap (an unsplit
-- background equipment sentence) found in the same audit.
--
-- Every statement is guarded by the exact broken content it targets (slug,
-- name, or quantity), so this is a no-op on a database that never had the
-- affected rows (a fresh install) or one where this migration already ran.
--
-- A fourth known-broken character (id 10, "Konnichiwa") had a Crafter's
-- Pack's "3 Toolkits (pick three)" placeholder with zero signal anywhere in
-- her data for which three toolkits she'd have picked — guessing three
-- specific toolkits for her would be inventing player intent, not fixing a
-- bug. Instead her placeholder is retagged so it becomes a live, resolvable
-- "Pick a Toolkit (3 remaining)" picker on her sheet (the exact UI
-- buildPendingPackToolkitChoiceRows now renders for any row carrying this
-- tag) — she resolves her own three picks next time she opens the sheet,
-- same as if she'd unpacked the pack today. See step 4 below.
--
-- A fifth known-broken character (id 1, "Sakura") has two similarly-shaped
-- unresolved custom rows ("1 Martial Weapon", "2 Kits of your Choice") —
-- but those come from class/hunter-nin's class_equipment_options step, not
-- a pack, predating even the creation-time dropdown fix for that path (see
-- the investigation this migration is drawn from). No pending-choice UI
-- exists yet for a post-creation class-equipment pick the way it now does
-- for pack toolkits, and there is likewise no signal for which weapon or
-- kits she'd choose, so those two rows are deliberately left untouched by
-- this migration — flagged here rather than guessed at.

-- 1. Character 5 ("Hon"): Infiltrator's Pack granted "1 Hackers Kit or
--    Security Kit (pick one)" as inert free text. She already renamed the
--    custom item's own display name to "Hackers Kit" (character_inventory
--    row 68 / custom_items row 13) — the closest thing to an explicit,
--    already-recorded pick that exists short of asking her directly — but
--    never got a real item or the matching tool proficiency for it. Point
--    the existing row at the real catalogue item (merging is unnecessary:
--    she holds no other Hackers Kit) and grant the proficiency the pack
--    was always supposed to hand out. The custom_items row this leaves
--    behind (slug 'custom/1-hackers-kit-or-security-kit-(pick-one)-13') is
--    orphaned, same as ResolvePackToolkitChoice's own real resolutions
--    never clean up the placeholder's custom_items row either.
UPDATE character_inventory
SET item_slug = 'toolkit/hackers-kit'
WHERE character_id = 5
  AND item_slug = 'custom/1-hackers-kit-or-security-kit-(pick-one)-13'
  AND quantity = 1;

INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
SELECT 5, 'tool', 'Hackers Kit', 'pack', 'toolkit/hackers-kit'
WHERE EXISTS (
    SELECT 1 FROM character_inventory
    WHERE character_id = 5 AND item_slug = 'toolkit/hackers-kit'
)
AND NOT EXISTS (
    SELECT 1 FROM character_proficiencies
    WHERE character_id = 5 AND kind = 'tool' AND value = 'Hackers Kit'
);

-- 2. Character 2 ("Sasuke"): also unpacked an Infiltrator's Pack, and
--    worked around the same gap a different way — he added a real
--    toolkit/hackers-kit row (character_inventory row 46) via the item
--    library himself rather than renaming the leftover custom row (which
--    he appears to have deleted; no placeholder row remains for him). The
--    item half is already correct; only the proficiency half was ever
--    missing.
INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
SELECT 2, 'tool', 'Hackers Kit', 'pack', 'toolkit/hackers-kit'
WHERE EXISTS (
    SELECT 1 FROM character_inventory
    WHERE character_id = 2 AND item_slug = 'toolkit/hackers-kit'
)
AND NOT EXISTS (
    SELECT 1 FROM character_proficiencies
    WHERE character_id = 2 AND kind = 'tool' AND value = 'Hackers Kit'
);

-- 3. Character 2 ("Sasuke"): background/urchin's whole equipment sentence
--    ("A Map of the Village you grew up in, A Token of intimate value to
--    you, Set of Basic Clothing, Wallet containing 100 Ryo") was stored as
--    ONE unsplit free-text row (character_inventory row 13 / custom_items
--    row 3) instead of the four lines parseStartingEquipment splits it
--    into today — two genuine flavor lines with no equipment-table match,
--    plus Ordinary Clothing and a Wallet, both real equippable/rollable
--    items neither of which he actually holds. His Ryo (100.0) was already
--    credited correctly by a separate code path, so only the item half
--    needs splitting.
--
--    Step 3a repurposes the existing custom_items row in place to become
--    just the first flavor line, rather than deleting and recreating it —
--    its slug keeps the original combined text (cosmetic only; the same
--    slug/display-name mismatch already exists elsewhere in this data,
--    e.g. character 5's Hackers Kit row above, and this app treats it as
--    normal).
UPDATE custom_items
SET name = 'A Map of the Village you grew up in'
WHERE slug = 'custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13'
  AND name = 'A Map of the Village you grew up in, A Token of intimate value to you, Set of Basic Clothing, Wallet containing 100 Ryo';

--    Step 3b creates the second flavor line as its own custom item, the
--    same insert-then-stamp-slug two-step SetEquipment/UnpackInventoryItem
--    use at runtime (a slug needs the row's own just-assigned id, so it
--    can't be written in one INSERT). Gated on character 2's original
--    unsplit row still existing, same as every other step of this fix —
--    not just on "no custom item with this name exists yet" — so this
--    can't fire (and, on a database with no character id 2 at all, trip
--    the character_inventory foreign key below) on a database that never
--    had the bug.
INSERT INTO custom_items (slug, name)
SELECT '', 'A Token of intimate value to you'
WHERE EXISTS (
    SELECT 1 FROM character_inventory
    WHERE character_id = 2
      AND item_slug = 'custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13'
)
AND NOT EXISTS (SELECT 1 FROM custom_items WHERE name = 'A Token of intimate value to you');

UPDATE custom_items
SET slug = 'custom/a-token-of-intimate-value-to-you-' || id
WHERE slug = '' AND name = 'A Token of intimate value to you';

INSERT INTO character_inventory (character_id, item_slug, quantity, notes)
SELECT 2, ci.slug, 1, 'creation-equipment'
FROM custom_items ci
WHERE ci.name = 'A Token of intimate value to you'
  AND ci.slug LIKE 'custom/a-token-of-intimate-value-to-you-%'
  AND EXISTS (
      SELECT 1 FROM character_inventory
      WHERE character_id = 2
        AND item_slug = 'custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13'
  )
  AND NOT EXISTS (
      SELECT 1 FROM character_inventory
      WHERE character_id = 2 AND item_slug = ci.slug
  );

--    Step 3c/3d land the two real items. Both guarded on row 13 still
--    being the original unsplit line (identifies this as the same
--    character/bug without re-deriving the split logic in SQL) and on the
--    character not already holding one (he doesn't, checked against the
--    live database before writing this migration).
INSERT INTO character_inventory (character_id, item_slug, quantity, notes)
SELECT 2, 'gear/ordinary-clothing', 1, 'creation-equipment'
WHERE EXISTS (
    SELECT 1 FROM character_inventory
    WHERE character_id = 2
      AND item_slug = 'custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13'
)
AND NOT EXISTS (
    SELECT 1 FROM character_inventory WHERE character_id = 2 AND item_slug = 'gear/ordinary-clothing'
);

INSERT INTO character_inventory (character_id, item_slug, quantity, notes)
SELECT 2, 'gear/wallet', 1, 'creation-equipment'
WHERE EXISTS (
    SELECT 1 FROM character_inventory
    WHERE character_id = 2
      AND item_slug = 'custom/a-map-of-the-village-you-grew-up-in,-a-token-of-intimate-value-to-you,-set-of-basic-clothing,-wallet-containing-100-ryo-13'
)
AND NOT EXISTS (
    SELECT 1 FROM character_inventory WHERE character_id = 2 AND item_slug = 'gear/wallet'
);

-- 4. Character 10 ("Konnichiwa"): Crafter's Pack's "3 Toolkits (pick
--    three)" (character_inventory row 146 / custom_items row 29) has no
--    resolvable signal for which three toolkits to grant, so rather than
--    guess, retag the row with the exact notes value
--    charstore.PackToolkitChoiceNotesTag ("pack-toolkit-choice")
--    UnpackInventoryItem now writes for this same segment — this is the
--    one column character_inventory has for "this row is a still-open
--    request", so setting it is what turns an inert, permanently-stuck
--    free-text line into the same live "Pick a Toolkit (3 remaining)"
--    picker a freshly unpacked Crafter's Pack renders today. Quantity (3)
--    is untouched, so all three picks remain available to her.
UPDATE character_inventory
SET notes = 'pack-toolkit-choice'
WHERE character_id = 10
  AND item_slug = 'custom/3-toolkits-(pick-three)-29'
  AND quantity = 3
  AND notes IS NULL;
