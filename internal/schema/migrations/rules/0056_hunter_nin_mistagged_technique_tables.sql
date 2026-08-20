-- Hunter-Nin has 7 of its 8 Hunters Creed subclasses' option tables (a
-- technique/property/attachment list, printed with its own ALL-CAPS
-- heading) glued by the flat PDF text extractor onto a LATER, unrelated
-- sibling feature instead of the 3rd-level "Proficiency" feature that
-- actually grants the choice ("Select one of the following ..."). Only
-- Arsenalist, whose option list is a plain bullet list rather than an
-- ALL-CAPS-headed table, was unaffected. Confirmed against dist/rules.db:
--
--   Blade Warden   WARDEN WEAPON PROPERTY TABLE      superior-offense (17th)  -> wardens-proficiency (3rd)
--   Necrotic Hand  MEDICAL ASSASSINATION TECH TABLE  necrotic-touch (7th)     -> medical-proficiency (3rd)
--   Grave Stalker  SHADOW ASSASSINATION TECH TABLE   master-ambusher (7th)    -> stalkers-proficiency (3rd)
--   Undertaker     TOXIC ASSASSINATION TECH TABLE    false-faces (7th)        -> toxic-proficiency (3rd)
--   Vice Agent     VICE ASSASSINATION TECH TABLE     arrogances-influence (7th) -> sins-proficiency (3rd)
--   Void Walker    VOID ASSASSINATION TECH TABLE     vorpal-strike (7th)      -> stalker-proficiency (3rd)
--   Wolves Legacy  PROSTHETIC ATTACHMENTS TABLE      eyes-of-a-shinobi (7th)  -> wolfs-proficiency (3rd)
--
-- Grave Stalker's case is the one exception to a clean end-of-description
-- append: the table was spliced into the MIDDLE of Master Ambusher's own
-- closing sentence ("You SHADOW ASSASSINATION TECHNIQUE TABLE ... reduced
-- by 2. can attempt to interject socially in this way, once every 10
-- minutes."), so its fix also stitches "You" and "can attempt to interject
-- ..." back into one sentence once the table is lifted out.
--
-- internal/parse/subclasses.go's redistributeMistaggedTables now performs
-- this same redistribution on any future re-ingest, keyed off each table's
-- own printed ALL-CAPS heading. This migration repairs the already-shipped
-- rows. Every UPDATE is guarded by an INSTR() check so re-running this
-- migration (or applying it to a database already re-ingested with the
-- parser fix) against already-correct rows is a no-op, not a corruption --
-- confirmed by running it twice against a scratch copy and diffing.

-- Blade Warden
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'WARDEN WEAPON PROPERTY TABLE')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/blade-warden/feature/wardens-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/blade-warden/feature/superior-offense'
  AND INSTR(from_feat.description, 'WARDEN WEAPON PROPERTY TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'WARDEN WEAPON PROPERTY TABLE') - 1))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/blade-warden/feature/superior-offense'
  AND INSTR(description, 'WARDEN WEAPON PROPERTY TABLE') > 0;

-- Necrotic Hand
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'MEDICAL ASSASSINATION TECHNIQUE TABLE')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/medical-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/necrotic-touch'
  AND INSTR(from_feat.description, 'MEDICAL ASSASSINATION TECHNIQUE TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'MEDICAL ASSASSINATION TECHNIQUE TABLE') - 1))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/necrotic-hand/feature/necrotic-touch'
  AND INSTR(description, 'MEDICAL ASSASSINATION TECHNIQUE TABLE') > 0;

-- Grave Stalker (table spliced mid-sentence into Master Ambusher)
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(
      from_feat.description,
      INSTR(from_feat.description, 'SHADOW ASSASSINATION TECHNIQUE TABLE'),
      INSTR(SUBSTR(from_feat.description, INSTR(from_feat.description, 'SHADOW ASSASSINATION TECHNIQUE TABLE')), 'can attempt to interject socially in this way, once every 10 minutes.') - 1
    ))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/grave-stalker/feature/stalkers-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/grave-stalker/feature/master-ambusher'
  AND INSTR(from_feat.description, 'SHADOW ASSASSINATION TECHNIQUE TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'SHADOW ASSASSINATION TECHNIQUE TABLE') - 1))
    || ' ' ||
    TRIM(SUBSTR(description, INSTR(description, 'can attempt to interject socially in this way, once every 10 minutes.')))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/grave-stalker/feature/master-ambusher'
  AND INSTR(description, 'SHADOW ASSASSINATION TECHNIQUE TABLE') > 0;

-- Undertaker
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'TOXIC ASSASSINATION TECHNIQUE TABLE')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/undertaker/feature/toxic-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/undertaker/feature/false-faces'
  AND INSTR(from_feat.description, 'TOXIC ASSASSINATION TECHNIQUE TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'TOXIC ASSASSINATION TECHNIQUE TABLE') - 1))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/undertaker/feature/false-faces'
  AND INSTR(description, 'TOXIC ASSASSINATION TECHNIQUE TABLE') > 0;

-- Vice Agent
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'VICE ASSASSINATION TECHNIQUE TABLE')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/vice-agent/feature/sins-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/vice-agent/feature/arrogances-influence'
  AND INSTR(from_feat.description, 'VICE ASSASSINATION TECHNIQUE TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'VICE ASSASSINATION TECHNIQUE TABLE') - 1))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/vice-agent/feature/arrogances-influence'
  AND INSTR(description, 'VICE ASSASSINATION TECHNIQUE TABLE') > 0;

-- Void Walker
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'VOID ASSASSINATION TECHNIQUE TABLE')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/void-walker/feature/stalker-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/void-walker/feature/vorpal-strike'
  AND INSTR(from_feat.description, 'VOID ASSASSINATION TECHNIQUE TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'VOID ASSASSINATION TECHNIQUE TABLE') - 1))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/void-walker/feature/vorpal-strike'
  AND INSTR(description, 'VOID ASSASSINATION TECHNIQUE TABLE') > 0;

-- Wolves Legacy
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'PROSTHETIC ATTACHMENTS TABLE')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/wolfs-proficiency'
  AND from_feat.slug = 'class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/eyes-of-a-shinobi'
  AND INSTR(from_feat.description, 'PROSTHETIC ATTACHMENTS TABLE') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'PROSTHETIC ATTACHMENTS TABLE') - 1))
WHERE slug = 'class/hunter-nin/group/hunters-creeds/wolves-legacy/feature/eyes-of-a-shinobi'
  AND INSTR(description, 'PROSTHETIC ATTACHMENTS TABLE') > 0;
