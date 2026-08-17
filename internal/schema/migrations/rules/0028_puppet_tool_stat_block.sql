-- Puppet Master's "Puppet Tool" feature prints its baseline stat card
-- (creature type, HP formula, base ability scores, Speed, Passive
-- Perception, three named traits) as a sidebar box. The flat PDF text
-- extractor has no column/box detection, so that box landed interleaved
-- into the feature's own prose mid-paragraph — class_features's puppet-tool
-- row ends with the real feature text glued directly to the stat card,
-- with no separator, both mistakenly rendered as one prose block on the
-- sheet. internal/parse/classes.go now splits this out at ingest time
-- (puppetToolUnitCardRe); this migration both adds the table that split
-- data lands in AND patches the already-shipped bad row directly, since a
-- migration-only fix would be undone by the next real re-ingest and a
-- parser-only fix wouldn't correct what's already in dist/rules.db and
-- internal/embedded/rules.db.
CREATE TABLE puppet_tool_stat_block (
    class_slug            TEXT PRIMARY KEY REFERENCES classes(slug),
    creature_type         TEXT NOT NULL,
    proficiency_rule_text TEXT NOT NULL,
    hp_base               INTEGER NOT NULL,
    hp_con_bonus_add      INTEGER NOT NULL,
    speed                 INTEGER NOT NULL,
    str_score             INTEGER NOT NULL,
    dex_score             INTEGER NOT NULL,
    con_score             INTEGER NOT NULL,
    int_score             INTEGER NOT NULL,
    wis_score             INTEGER NOT NULL,
    cha_score             INTEGER NOT NULL,
    passive_perception    INTEGER NOT NULL,
    traits_text           TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

-- detection_status='manual' on the class_features row stops a future real
-- re-ingest from silently reintroducing drift if the parser behaves even
-- slightly differently against a later PDF revision (decideStatus in
-- internal/store treats 'manual'/'verified' rows specially).
UPDATE class_features
SET description = 'Beginning at 1st level, you craft a Puppet Tool to carry out your orders and protect you. Your Puppet Tool starts with the statistics below; You learn the Mending E-Rank Ninjutsu, which does not count against your known. A Puppet Tool acts under your command via your chakra threads, thus, if you are unable to make Chakra Threads, you cannot control your Puppet. You can command a Puppet to move on your turn, and you can spend your action or Bonus Action to command a Puppet to take an action and Bonus Action, once per turn. A Puppet has one Bonus Action per round, and it can only be used to activate upgrades or Puppet Master features. You can also use your Reaction, to have your Puppet take a Reaction. Your Puppet has its own attack bonuses and Save DCs, calculated the same way as your own. A Puppet Tool increases one of its ability scores by +2, or two ability scores by +1, when you gain an Ability Score Improvement/Feat from this class. A Puppet Tool can have no more than 20 in an ability score as normal. While connected to a Puppet Tool or in contact with it, as an action, you can cast the Mending Ninjutsu to restore a number of its hit points equal to 2d6 + half your Puppet Master level. This die increases to 2d8 at 9th level and 2d10 at 15th level of this class. Alternatively, you can cast Mending for its entire duration while within 5 feet of the Puppet Tool. If you do, it recovers half of its maximum hit points. After healing the Puppet Tool this way, if it isn''t destroyed after 1 minute, your chakra pulls it back together, regaining its full hit points. On a full rest, you can spend 1 charge of an Armorsmith or Weaponsmith kit to scavenge an existing Puppet Tool to create a new one, or completely replicate a destroyed Puppet. You store your Puppet Tool in a special reusable weapon scroll. You can store your Puppet Tool into this scroll as an action, and you can release it as part of rolling initiative or at any point on your turn. The size of the scroll reflects the puppet''s size. Starting at 6th level when you do not command a Puppet you are connected to on your turn, that puppet takes the dodge action. You also do not need to use your Reaction to command a Puppet to take its Reaction.',
    detection_status = 'manual'
WHERE slug = 'class/puppet-master/feature/puppet-tool';

-- Guarded with WHERE EXISTS rather than a bare INSERT: this migration also
-- runs against a fresh, empty rules DB (every test fixture, and a
-- from-scratch first ingest) where no 'class/puppet-master' row exists yet
-- — an unconditional INSERT would fail puppet_tool_stat_block's own
-- class_slug foreign key in that case. A real classes re-ingest populates
-- this table correctly on its own via upsertPuppetToolStatBlock regardless.
INSERT INTO puppet_tool_stat_block (class_slug, creature_type, proficiency_rule_text,
    hp_base, hp_con_bonus_add, speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
    passive_perception, traits_text, source_book, source_version, source_page, detection_status)
SELECT 'class/puppet-master', 'Medium Construct', 'Puppet Master’s Proficiency',
    4, 5, 30, 15, 13, 13, 5, 5, 5, 7,
    'Bound. A Puppet is bound to its user via chakra threads and can be no more than 500ft. from you. If the strings are not bound to the Puppet, it cannot move or take actions of any kind. If a creature other than its creator uses chakra threads to manipulate the puppet, it cannot use any Upgrades. Hollow Shell. Puppet Tools cannot be affected by Genjutsu. Mechanical Limits. Puppets cannot cast jutsu (or have such jutsu cast through them), or use effects, that increase its ranks of exhaustion or make Clones.',
    'book/class-compendium', '3.12', 142, 'manual'
WHERE EXISTS (SELECT 1 FROM classes WHERE slug = 'class/puppet-master');
