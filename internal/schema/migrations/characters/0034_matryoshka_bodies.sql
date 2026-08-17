-- Matryoshka Framework (Puppet Framework, class/puppet-master/option/
-- puppet-frameworks/matryoshka-framework) lets its own Puppet Tool split
-- into 1-3 separate bodies on a rest, each a fully independent creature
-- for command purposes, re-merging on a later rest. A "body" is just
-- another character_companions row (kind='puppet') sharing this group's
-- own matryoshka_group_id — reusing the existing multi-companion mechanism
-- rather than inventing a new kind, the same "no new subsystem where an
-- existing one already fits" call this pass makes elsewhere.
--
-- matryoshka_group_id is nullable: NULL means "not currently split" (the
-- ordinary, single-body state every puppet starts and ends in). A non-NULL
-- value is the id of whichever body is the group's own "primary" row (the
-- one that survives a re-merge) — not a separate groups table, since
-- nothing about a group needs its own row beyond that one id.
ALTER TABLE character_companions ADD COLUMN matryoshka_group_id INTEGER;

-- matryoshka_jutsu_slots is a plain player-editable counter ("up to 9
-- jutsu total, 3 per body if split") — companions have no real known-jutsu
-- list anywhere in this app (see Shade/Spellblade Framework's own jutsu
-- picks, which are stored as reference-only sub-choices for the same
-- reason), so this is reference tracking, not a real casting list,
-- editable the same delta-in/bare-number-sets/blank-clears way AC and
-- HP-max already are (see cmd/n5e/companions.go's handleCompanionIntField).
ALTER TABLE character_companions ADD COLUMN matryoshka_jutsu_slots INTEGER NOT NULL DEFAULT 0;
