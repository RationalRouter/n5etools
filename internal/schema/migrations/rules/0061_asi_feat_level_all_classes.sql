-- Every class's own Ability Score Improvement/Feat feature prints the
-- identical prose ("When you reach 4th and again at 8th, 12th, 16th, and
-- 19th, level, you can increase..."), which never puts an ordinal
-- immediately before the word "level" the way the parser's ordinal-level
-- regex requires -- so the row parsed as always-on (level NULL) instead of
-- 4th level. class_features has no per-level rows for the later
-- breakpoints either (8th/12th/16th/19th); ResolveASISlots (internal/
-- features/asi.go) walks ASIBreakpoints itself off this single row's own
-- level, so setting it to 4 is enough to unlock every later breakpoint too.
--
-- internal/parse/classes.go's knownClassFeatureLevelOverrides only carried
-- this fix for Genjutsu Specialist, the one class where the bug was first
-- found -- confirmed live: a Cooking-Nin character reached level 5 with no
-- Pending Choices prompt at all, because the feature itself was never
-- granted (LoadGrantedFeatures never matches a NULL-level row against any
-- class level). The override is now added for all 11 classes in the same
-- commit as this migration, so any future re-ingest also levels these
-- correctly; this repairs the 10 already-shipped NULL rows directly.
UPDATE class_features
SET level = 4
WHERE slug LIKE 'class/%/feature/ability-score-improvement-feat'
  AND level IS NULL;
