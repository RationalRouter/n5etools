-- Fixes a real-word typo in Weaved Mail's hand-transcribed description
-- (see 0031_puppet_armor_chassis.sql's own doc comment: this table is
-- transcribed directly from a page-image table with no PDF text layer,
-- so it isn't touched by internal/correct's automated Sweep, which only
-- runs on parsed prose columns): "neigh undetectable" should read "nigh
-- undetectable" ("nigh" = nearly; "neigh" is a horse sound). Confirmed
-- against the full-corpus spelling/grammar audit, 2026-08-05.
UPDATE puppet_armor_chassis
SET description = REPLACE(description, 'make your defense neigh undetectable', 'make your defense nigh undetectable')
WHERE slug = 'puppet-armor-chassis/weaved-mail';
