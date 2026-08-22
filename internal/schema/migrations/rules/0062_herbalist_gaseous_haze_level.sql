-- Herbalist's identity feature Gaseous Haze ("You may use Charisma in place
-- of Wisdom for calculating your Genjutsu attack bonus and DC") never states
-- an ordinal level in its own printed text, unlike every other Cooking-Nin
-- Focus's own sort_order-0 identity feature (Expert Combatant, Fast and
-- Furious, Water and Oil Do Mix, Eye of the Storm, If You Can't Handle the
-- Heat, Sweet Smell, Give Them a Show, I Expect You to Die — all explicitly
-- 2nd level), so it parsed as always-on (level NULL) instead of 2nd level.
-- The parser override was fixed in the same commit as this migration
-- (internal/parse/subclasses.go's knownFeatureLevelOverrides); this repairs
-- the already-shipped NULL row directly.
UPDATE subclass_features
SET level = 2
WHERE slug = 'class/cooking-nin/group/cooking-focus/herbalist/feature/gaseous-haze'
  AND level IS NULL;
