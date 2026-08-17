-- Links one Ability Score Improvement breakpoint (internal/features.ASISlot's
-- own Ref, "<class-slug>@<level>") to the Feat a player chose INSTEAD of the
-- ability-increase half of that breakpoint — the book's ASI text grants
-- "an Ability Score Improvement OR a Feat you qualify for", and this is the
-- only place that "instead of" relationship is recorded. A dedicated table
-- rather than a nullable column on character_feats: keeps the linkage
-- self-contained (one row per resolved breakpoint, trivially cleaned up from
-- either side of a later switch) rather than mixing ASI-branch bookkeeping
-- into the shared feats table every other feat source also writes to.
--
-- feat_slug is NOT itself the source of truth for "this character has this
-- feat" — character_feats still owns that, and internal/charstore's
-- SetAbilityScoreImprovementFeat writes both rows together. This table only
-- answers "is this breakpoint's pending choice already resolved, and via
-- which feat", which is what internal/features.LoadResolvedASIRefs and the
-- Feats-tab delete handler both need it for.
CREATE TABLE character_asi_feat_choices (
    character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    ref          TEXT NOT NULL,
    feat_slug    TEXT NOT NULL,
    PRIMARY KEY (character_id, ref)
);
