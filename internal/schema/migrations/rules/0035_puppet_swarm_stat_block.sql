-- Red Technique's "Master of the Red Technique" (L20) feature prints the
-- Puppet Swarm's own sidebar stat card (creature type, AC/HP/Speed formula,
-- ability score placeholders, saves/skills, resistances/immunities, senses,
-- and its three named traits plus the Puppet Performance Special Actions)
-- immediately after its own prose. The flat PDF text extractor has no
-- column/box detection, so this box landed glued onto the end of Master of
-- the Red Technique's own description — the ONE feature that happened to be
-- current when the extractor crossed the sidebar, same as Puppet Master's
-- own Puppet Tool card (0028_puppet_tool_stat_block.sql) and Science-Nin's
-- Titan card (0030_titan_unit_card.sql). "Performance of 10 Puppets" (L10),
-- the feature that actually grants use of this statblock, only references
-- it ("...command the Puppet Swarm statblock, listed on the following
-- page.") and was never itself corrupted.
--
-- Kept as one raw text field, not parsed into columns like
-- puppet_tool_stat_block: AC/HP/Speed/ability-scores are computed live from
-- the character's own Puppet Tool companions (cmd/n5e/puppets.go's
-- puppetSwarmStats), not read back out of this prose, so there is no
-- structured data to extract here beyond what's already computed elsewhere.
-- This table exists to hold the remaining reference-only text (the fixed
-- traits and the three Commands-spending Special Actions) that the new
-- Puppet Swarm panel renders as-is, and to stop it from being repeated
-- inside Master of the Red Technique's own feature description.
--
-- Migration-only, like titan_unit_card: no ingest-time parser split was
-- added for this box (internal/parse/subclasses.go), since nothing else in
-- this table's own row shape would change from a future re-ingest picking
-- the sidebar back up — a re-ingest that reglues this text onto
-- master-of-the-red-technique's own description would only affect that one
-- subclass_features row, which detection_status='manual' below still
-- correctly flags 'needs_review' rather than silently reverting (decideStatus
-- in internal/store demotes on any content change), even though it does not
-- literally block the overwrite. This table's own row, populated only by
-- this migration's INSERT (same as titan_unit_card, no ongoing upsert
-- function), stays untouched by any such re-ingest either way.
CREATE TABLE puppet_swarm_stat_block (
    class_slug       TEXT PRIMARY KEY REFERENCES classes(slug),
    raw_text         TEXT NOT NULL,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

UPDATE subclass_features
SET description = 'Starting at 20th level, you achieve the pinnacle of the Red Technique''s teachings, gaining a small army''s worth of power at your fingertips, capable of conquering an entire nation, all by itself. First created by the legendary Puppet Master, Sasori of the Red Sand. When you would use your Performance of 10 Puppets feature, you can choose to summon the Performance of 100 Puppets instead. When you do, you gain access to additional features as noted on the Puppet Swarm statblock.',
    detection_status = 'manual'
WHERE slug = 'class/puppet-master/group/puppet-techniques/red-technique-performer/feature/master-of-the-red-technique';

-- Guarded with WHERE EXISTS for the same reason 0028's/0030's INSERTs are:
-- this migration also runs against a fresh, empty rules DB (test fixtures,
-- a from-scratch first ingest) where no 'class/puppet-master' row exists
-- yet.
INSERT INTO puppet_swarm_stat_block (class_slug, raw_text, source_book, source_version, source_page, detection_status)
SELECT 'class/puppet-master',
    'Huge Swarm of Puppets, unaligned Armor Class (The Swarm has an AC equal to your Puppet Tool with the highest AC + 2) Hit Points (The Sum of all your Puppet’s Hit Points) x 1.5 Speed 60ft. STR DEX CON INT WIS CHA X (+X) X (+X) X (+X) X (+X) X (+X) X (+X) Saving Throws Proficient in All (Treat negative modifiers as +0) Skills Same proficiencies as your Puppets Damage Resistances Acid, Chakra, Necrotic Damage Immunities Psychic, Poison Condition Immunities All Mental, Bleeding, Poisoned Senses Same Visual Sights as your Puppets, Passive Perception = Twice the highest Passive Perception amongst your Puppets. One of Many. The Puppet Swarm counts as a single Puppet in regards to jutsu, attacks, and other effects that would target it, however, it counts as being multiple Puppets for the purposes of interactions with Red Technique class features. As a Bonus Action, you can command a single Puppet to take an action and move. The Puppet can only move up to half its movement speed away from the Swarm, as by the end of the turn it returns to the Swarm. Puppet Tool. The Swarm has access to the same features as the Puppet Tool statblock. Any upgrades that provide passive effects are still active as normal, however, upgrades that require attack rolls or impose saving throws, can only be used with the Attack Special Action (Unless an upgrade specifies otherwise). Puppet Swarm. The Puppet Swarm is made up of copies of your existing puppets, until you have 10/100 Puppets total. When determining the Puppet Swarm''s stats, choose the highest stat for each ability score from each of your Puppets. Each copied Puppet shares the same statistics and Puppet Upgrades (Any upgrades with use limits or that require ammunition, have their uses/ammunition shared with the entire swarm and are NOT replenished when the swarm is activated). While the Puppet Swarm is active, you gain access to every Puppet Role, and the first time you summon the Puppet Swarm per full rest, you regain half of any expended uses of Upgrades. If you summon the Performance of 100 Puppets, regain all uses. Performance of 100 Puppets. When the performance of 100 Puppets is active, the Puppet Swarm becomes Gargantuan in size, gains a +2 bonuses to AC, deals +1 extra die of damage with weapons and upgrades, and increases the multiplier to its maximum hit points to 2 from 1.5. You also gain improvements to your Special Actions and more Commands, as noted below. Puppet Performance. When you command your Puppet Tool using your Action, you instead command the Puppet Swarm to take the following Special Actions; These Special Actions take from a pool of Commands. While the Performance of 10 Puppets is active, you have 5 Commands to spend. While the Performance of 100 Puppets is active, you have 10 Commands to spend. You regain spent commands at the start of your next turn. Attack (X Commands). Select one weapon one of your Puppet''s possesses. Make an attack with this weapon against a creature within range. For every command you spend to use this Special Action, you send Puppets to aid your initial Puppet Tool, giving the attack a +1 to hit and a +3 bonuses to the damage roll. For every 2 Commands, impose a -1 penalty to all affected creature’s first saving throw (Max. -3). If you use an upgrade that makes an attack roll or imposes a saving throw, you must spend an additional command to utilize it with this special action. If the Performance of 100 Puppets is active, and you spend at least two Commands, your Puppets gain advantage on the attack. Defend (2 Commands). You spend two commands to have Puppets circle around an allied creature within range and protect them. Until the start of your next turn, all attacks that target this creature are made with a -1d4 penalty, and the creature gains a +1d4 bonus to the first saving throw it makes per turn. If the Performance of 100 Puppets is active, increase the 1d4s to 1d6s. Swarm (3 Commands). You spend 3 commands to swarm hostile creatures in a 20-foot radius within 60 feet of the swarm. Creature must make a Strength or Dexterity saving throw (Your choice, for all creatures), suffering the listed effects; If the Performance of 100 Puppets is active, creatures have disadvantage on their saving throw.',
    'book/class-compendium', '3.12', 169, 'manual'
WHERE EXISTS (SELECT 1 FROM classes WHERE slug = 'class/puppet-master');
