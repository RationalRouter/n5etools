-- The flat PDF text extractor glued an image caption onto the end of
-- Weapon Specialist's Superior Weapon Flurry feature text. The parser now
-- strips this pattern on ingest (internal/parse's artistCreditRe), but this
-- one row was already ingested before that fix, so it needs a one-time
-- content correction too.
UPDATE class_features
SET description = 'Beginning at 14th level, you gain an advanced level of proficiency with one of your Flurry Techniques. Select two of the following benefits. You gain an additional benefit at 18th level. • Your Enhanced Deflection flurry technique now lasts until the beginning of your next turn. • Your Enhanced Deflection allows you to choose to add a third of your maximum Flurry Die to your AC instead. • Your Perceptive Augmentation now lasts until the end of your next turn. • Your Perceptive Augmentation now grants you the effects of the Dash action.'
WHERE slug = 'class/weapon-specialist/feature/superior-weapon-flurry';
