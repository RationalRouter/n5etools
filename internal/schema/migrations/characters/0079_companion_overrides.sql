-- Per-companion manual pins for otherwise auto-computed stat fields (AC,
-- Max HP, Speed, Fly Speed, the six ability scores, Jutsu Slots Max,
-- Barrier Max, Size) — the companion-scoped equivalent of character_overrides
-- (0001_init.sql), which the main sheet's own Max HP/Max Chakra boxes use
-- for the identical "auto by default, freely overridable" contract.
--
-- A separate table rather than reusing character_overrides directly: a
-- companion's own raw character_companions columns (ac, hp_max, speed, ...)
-- must always hold the CURRENT EFFECTIVE value, not just the auto value,
-- because two other code paths read those raw columns directly and cannot
-- reach the formula that would otherwise resolve them —
-- internal/charsheet.puppetWornAsArmorAC (a puppet worn as Juggernaut Armor
-- feeds its own AC into the wearer's AC calc, but internal/charsheet cannot
-- import cmd/n5e, where that AC formula actually lives) and
-- cmd/n5e/companion_saves.go's companionSaves (reads Str/Dex/Con/Int/Wis/Cha
-- straight off the loaded struct). So this table stores only the PIN itself
-- (present = a player has manually set this field and it should stop
-- following the computed default); the per-render loaders
-- (loadPuppetsTabData/loadNinDogReference/loadTitanReference/
-- loadSNBReference) resolve auto-vs-pinned and write the EFFECTIVE value
-- into character_companions on every render, the same "always overwrite
-- with the resolved value" contract charstore.SetCompanionStatDefaults
-- already documents for Puppet Tools.
CREATE TABLE character_companion_overrides (
    companion_id INTEGER NOT NULL REFERENCES character_companions(id) ON DELETE CASCADE,
    field        TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (companion_id, field)
);
