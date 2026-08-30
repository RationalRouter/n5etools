-- Same reference-only stat_block_* columns added to class_features/
-- class_options by migration 0067, now added to jutsu as well:
-- tools/sourcebook-audit.rb's independent sweep found 18 jutsu whose
-- Description or At Higher Ranks text has a companion/summon stat card
-- glued into it (summoning and beast-transformation jutsu are exactly the
-- shape of entry likely to sit next to a sidebar stat card in the source
-- PDF) -- the same bug class, a genuinely new table, not a speculative
-- extension. See 0067's own header comment for the full explanation of why
-- these columns exist and what NULL vs. an empty string means.
ALTER TABLE jutsu ADD COLUMN raw_stat_block_text TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_creature_type TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_ac INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_ac_formula_text TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_hp_formula_text TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_speed INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_str INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_dex INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_con INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_int INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_wis INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_cha INTEGER;
ALTER TABLE jutsu ADD COLUMN stat_block_saving_throws_text TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_resistances TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_immunities TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_condition_immunities TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_senses TEXT;
ALTER TABLE jutsu ADD COLUMN stat_block_traits_attacks_text TEXT;
