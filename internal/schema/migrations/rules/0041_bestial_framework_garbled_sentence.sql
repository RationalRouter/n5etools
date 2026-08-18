-- Puppet Master's Green Technique Bestial Framework option prints a
-- garbled, ungrammatical sentence: "...The Bestial Puppet gains one Natural
-- Weapon of to summon you chose." Read against the surrounding text (the
-- Puppet's chosen "Sage Creature", referenced by the bullets immediately
-- before and after this one), the intended sentence is "...gains one
-- Natural Weapon of the Sage Creature you chose to summon." This is a
-- class_option, not a class_feature, and buildOption did not previously
-- call fixKnownExtractionSquish at all (now fixed in subclasses.go) — so no
-- class_option anywhere benefited from this replacer table before. That
-- parser fix corrects any FUTURE re-ingest on its own; this migration
-- repairs an already-shipped install's row in the meantime, the same
-- one-time patch shape as 0038.
UPDATE class_options
SET description = REPLACE(description,
    'Natural Weapon of to summon you chose.',
    'Natural Weapon of the Sage Creature you chose to summon.')
WHERE slug = 'class/puppet-master/option/puppet-frameworks/bestial-framework';
