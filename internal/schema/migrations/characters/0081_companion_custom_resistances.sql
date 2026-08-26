-- A kind="custom" companion's own manually-typed Damage Resistances/Damage
-- Immunities/Condition Immunities — the free-text, player-editable
-- counterpart to the SAME three fields Titan/S.N.B/Nin-Dog/Summon all
-- compute automatically from their own upgrade/feature catalogs (see
-- titanReference/snbReference in titan.go/snb.go, ninDogReference in
-- nindog.go, summonTribeReference in companions.go). A "custom" companion
-- has no upgrade catalog or tribe table behind it at all — every one of its
-- stat-block fields is already plain player-entered text (Attacks/Traits/
-- Notes, this table's own attacks/traits/notes columns) rather than a
-- computed formula — so these three follow that exact same shape instead of
-- a computed one, with no formula anywhere to conflict with.
--
-- Reuses the generic "resistances"/"immunities"/"condition_immunities"
-- column names rather than a "custom_"-prefixed variant, the same
-- "meaningful only for one particular kind" precedent nin_dog_breed/
-- titan_specialization/armor_chassis already set in this table — each of
-- those is likewise a plain column that only one specific kind's own popup
-- ever reads or writes, left blank and inert for every other kind.
--
-- Defaults to '' (nothing entered) for both a brand-new companion and every
-- pre-existing one, the same "no per-kind backfill guess" reasoning
-- migration 0077's own save_proficiencies column documents.
ALTER TABLE character_companions ADD COLUMN resistances TEXT NOT NULL DEFAULT '';
ALTER TABLE character_companions ADD COLUMN immunities TEXT NOT NULL DEFAULT '';
ALTER TABLE character_companions ADD COLUMN condition_immunities TEXT NOT NULL DEFAULT '';
