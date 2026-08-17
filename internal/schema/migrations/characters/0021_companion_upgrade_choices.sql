-- Tracks the sub-picks a repeatable upgrade grants ("...you can install up
-- to 3 Poison Tags of any quality into your Puppet Tool" — Black Iron
-- Upgrades / Wood Tier's Poison Mist Hell). One row per companion_upgrade
-- pick (character_companion_upgrades, migration 0019), since the sub-choice
-- only exists once that upgrade itself has been taken. No FK into
-- rules.db's equipment table for choice_slug, same cross-DB slug-reference
-- tolerance the rest of this feature already uses (see 0019's own comment).
--
-- Which upgrades get sub-choices, how many, and from which source list is a
-- small hand-curated Go map (cmd/n5e/puppets.go's
-- puppetUpgradeSubChoiceSpecs) — starting with the one confirmed instance
-- above, not a claim of full catalog coverage.
CREATE TABLE character_companion_upgrade_choices (
    id                   INTEGER PRIMARY KEY,
    companion_upgrade_id INTEGER NOT NULL REFERENCES character_companion_upgrades(id) ON DELETE CASCADE,
    choice_slug          TEXT NOT NULL,
    created_at           TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_character_companion_upgrade_choices_upgrade ON character_companion_upgrade_choices(companion_upgrade_id);
