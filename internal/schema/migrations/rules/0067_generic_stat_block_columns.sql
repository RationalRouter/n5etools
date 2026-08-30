-- Reference-only columns for a companion/summon stat block split out of a
-- class feature's or class option's description by internal/parse's new
-- generic SplitStatBlock detector (see statblock.go). The flat PDF text
-- extractor has no column/font-size signal, so a sidebar stat card
-- (Armor Class/Hit Points/Speed/six ability scores/Saving Throws/
-- resistances/Senses/traits/attacks) glues verbatim into whichever feature's
-- prose was accumulating at that point in the stream -- this has already
-- happened at least three times (Puppet Master's Puppet Tool, Science-Nin's
-- Titan base card, the S.N.B Specialist's base creature), plus the
-- newly-fixed Draconic Gauntlet/Whelp.
--
-- These columns are reference data for a maintainer hand-wiring a new
-- companion (see cmd/n5e/wow_whelp.go's own established pattern) -- they are
-- NOT rendered to players and NOT auto-wired into gameplay, the same
-- deliberate boundary Class.TitanBaseText's own doc comment already draws
-- ("structuring it now would be speculative"). A NULL value means no stat
-- block was detected for that row; an empty string in a sub-field means the
-- block was detected but that particular section wasn't printed or couldn't
-- be confidently isolated (see StatBlockFields' own doc comment).
ALTER TABLE class_features ADD COLUMN raw_stat_block_text TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_creature_type TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_ac INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_ac_formula_text TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_hp_formula_text TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_speed INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_str INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_dex INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_con INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_int INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_wis INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_cha INTEGER;
ALTER TABLE class_features ADD COLUMN stat_block_saving_throws_text TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_resistances TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_immunities TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_condition_immunities TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_senses TEXT;
ALTER TABLE class_features ADD COLUMN stat_block_traits_attacks_text TEXT;

ALTER TABLE class_options ADD COLUMN raw_stat_block_text TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_creature_type TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_ac INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_ac_formula_text TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_hp_formula_text TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_speed INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_str INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_dex INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_con INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_int INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_wis INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_cha INTEGER;
ALTER TABLE class_options ADD COLUMN stat_block_saving_throws_text TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_resistances TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_immunities TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_condition_immunities TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_senses TEXT;
ALTER TABLE class_options ADD COLUMN stat_block_traits_attacks_text TEXT;
