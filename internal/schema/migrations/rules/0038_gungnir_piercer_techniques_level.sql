-- Every other Weapon Form's own "Techniques[changed]" feature opens with
-- "Starting at 3rd level, ..." in its description, which is where the
-- parser reads the level from. Gungnir Piercer Techniques' printed text
-- omits that sentence and launches straight into its named abilities, so
-- it parsed with a NULL level — showing as an always-on feature instead of
-- gated to 3rd level like its 7 sibling Weapon Forms. Corrected directly;
-- the level 3 gate matches every other Weapon Form's identical feature.
UPDATE subclass_features
SET level = 3
WHERE slug = 'class/weapon-specialist/group/weapon-forms/gungnir-piercer-form/feature/gungnir-piercer-techniques-changed';
