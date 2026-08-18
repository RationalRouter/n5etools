-- Weapon Flurry (2nd level) introduces "the following Flurry Techniques" as
-- a named list: Enhanced Deflection, Chained Reaction [New], Chakra Strike,
-- Perceptive Augmentation, and Focused Efficiency — confirmed by their
-- shared sort_order block sitting directly between Weapon Flurry and Weapon
-- Stance, and by Superior Weapon Flurry's own text naming two of them by
-- name. None of the four without a table entry states its own level
-- anywhere in its printed text, so they parsed with a NULL level (showing
-- as always-on from character level 1) instead of gated to 2nd level like
-- their parent. Chakra Strike doesn't state its own gating level either —
-- its only ordinal is an internal damage-escalation clause ("...+2 Flurry
-- Die. This increases to +3 Flurry Die at 11th level."), which the level
-- regex matched instead, parsing it as an 11th-level feature instead of a
-- 2nd-level one whose damage bonus merely increases at 11th.
-- internal/parse/classes.go's knownClassFeatureLevelOverrides now corrects
-- all five, so any FUTURE re-ingest will set level = 2 on its own. This
-- migration repairs an already-shipped install's rows in the meantime, the
-- same one-time patch shape as 0038/0040.
UPDATE class_features
SET level = 2
WHERE slug IN (
	'class/weapon-specialist/feature/enhanced-deflection',
	'class/weapon-specialist/feature/chained-reaction-new',
	'class/weapon-specialist/feature/chakra-strike',
	'class/weapon-specialist/feature/perceptive-augmentation',
	'class/weapon-specialist/feature/focused-efficiency'
);
