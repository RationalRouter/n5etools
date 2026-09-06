-- puppet_swarm_stat_block.raw_text is static seed data (not parsed from a
-- sourcebook PDF, so internal/correct's ingest-time Sweep never touches
-- it) that carried the same "a +N bonuses" singular-article/plural-noun
-- error documented in internal/i18n/source_text_grammar_audit.md. Fixed
-- here as a one-time UPDATE, the same way any other post-hoc data
-- correction to already-seeded content is made.
UPDATE puppet_swarm_stat_block
SET raw_text = REPLACE(
    REPLACE(raw_text,
        'gains a +2 bonuses to AC', 'gains a +2 bonus to AC'),
    'giving the attack a +1 to hit and a +3 bonuses to the damage roll',
    'giving the attack a +1 to hit and a +3 bonus to the damage roll')
WHERE class_slug = 'class/puppet-master';
