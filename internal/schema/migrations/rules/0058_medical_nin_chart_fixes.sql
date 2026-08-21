-- Medical-Nin's 6 Tenets of Medicine subclasses each print a "<Subclass>
-- Chart" table at the end of their own 17th-level feature, naming the 4
-- jutsu (5th/9th/13th/17th) that subclass learns via its own "Adept
-- Medicine"/"Black Medicine"/etc. feature. The flat PDF text extractor
-- mishandled two of the six charts, same class of bug as
-- 0056_hunter_nin_mistagged_technique_tables.sql:
--
--   1. Combat Medic's own COMBAT MEDIC CHART was glued onto the END of
--      Black Medicine's 17th-level feature (Venomous Sting) instead of
--      Combat Medic's own 17th-level feature (Yin Seal: Release) — a
--      cross-subclass misattribution, confirmed against dist/rules.db:
--      venomous-sting's description ran straight from Venomous Sting's own
--      text into "COMBAT MEDIC CHART Level Jutsu Learned Jutsu Feature 5th
--      Pressure Point Barrage ..." with nothing separating the two
--      subclasses' text, while yin-seal-release's own description held only
--      its own feature text and no chart at all.
--
--   2. Three charts (Natural Medicine, Shaman, Transmuter) lost the leading
--      digit off each "5th"/"9th"/"13th" level-column entry, reading as a
--      bare "th" — Natural Medicine lost all four (its "17th" row read "th"
--      too), Shaman and Transmuter each kept their own "17th" row intact
--      and lost only "5th"/"9th"/"13th". Same information-loss shape as
--      knownExtractionSquishes' "1oth level" entry (internal/parse/
--      subclasses.go) — a lost character at a text-run boundary, not
--      something a general regex can safely recover, so each instance is
--      hand-verified against dist/rules.db and fixed by exact substring
--      match rather than a blind "insert a digit before every bare th"
--      rule (which would also corrupt any genuine "the" -> "th e" split or
--      similar elsewhere in the corpus).
--
-- internal/parse/subclasses.go's medicalNinChartFixes (redistributeMedicalNinCharts)
-- now performs both fixes on any future re-ingest. This migration repairs
-- an already-shipped install's rows. Every UPDATE is guarded so re-running
-- it (or applying it to a database already re-ingested with the parser fix)
-- against already-correct rows is a no-op — the combat-medic move is guarded
-- by an INSTR() check, and the three digit fixes use REPLACE() against
-- exact corrupted substrings that no longer exist once fixed.

-- 1. Move the Combat Medic Chart from Black Medicine's Venomous Sting to
-- Combat Medic's own Yin Seal: Release.
UPDATE subclass_features AS to_feat
SET description = TRIM(to_feat.description) || ' ' || TRIM(SUBSTR(from_feat.description, INSTR(from_feat.description, 'COMBAT MEDIC CHART')))
FROM subclass_features AS from_feat
WHERE to_feat.slug = 'class/medical-nin/group/tenets-of-medicine/combat-medic/feature/yin-seal-release'
  AND from_feat.slug = 'class/medical-nin/group/tenets-of-medicine/black-medicine/feature/venomous-sting'
  AND INSTR(from_feat.description, 'COMBAT MEDIC CHART') > 0;

UPDATE subclass_features
SET description = TRIM(SUBSTR(description, 1, INSTR(description, 'COMBAT MEDIC CHART') - 1))
WHERE slug = 'class/medical-nin/group/tenets-of-medicine/black-medicine/feature/venomous-sting'
  AND INSTR(description, 'COMBAT MEDIC CHART') > 0;

-- 2. Restore the missing level digits in Natural Medicine's chart (all 4
-- rows corrupted).
UPDATE subclass_features
SET description = REPLACE(REPLACE(REPLACE(REPLACE(description,
      'Feature th Chakra Transfer', 'Feature 5th Chakra Transfer'),
      'target gains. th Gift of the Apex', 'target gains. 9th Gift of the Apex'),
      'first selection. th Bestial Art Predator', 'first selection. 13th Bestial Art Predator'),
      'to yourself. th Supreme Water Lion', 'to yourself. 17th Supreme Water Lion')
WHERE slug = 'class/medical-nin/group/tenets-of-medicine/natural-medicine/feature/natures-avatar';

-- 3. Restore the missing level digits in Shaman's chart (5th/9th/13th
-- rows corrupted; 17th already intact).
UPDATE subclass_features
SET description = REPLACE(REPLACE(REPLACE(description,
      'Feature th Vampiric Touch', 'Feature 5th Vampiric Touch'),
      'of you. th Phantasmal killer', 'of you. 9th Phantasmal killer'),
      'affected creature. th Aura of Power', 'affected creature. 13th Aura of Power')
WHERE slug = 'class/medical-nin/group/tenets-of-medicine/shaman/feature/master-of-hexes';

-- 4. Restore the missing level digits in Transmuter's chart (5th/9th/13th
-- rows corrupted; 17th already intact).
UPDATE subclass_features
SET description = REPLACE(REPLACE(REPLACE(description,
      'Feature th Restorative', 'Feature 5th Restorative'),
      'Class level. th Curse of Prey', 'Class level. 9th Curse of Prey'),
      'Penalty to -6. th Reconstructive Hand', 'Penalty to -6. 13th Reconstructive Hand')
WHERE slug = 'class/medical-nin/group/tenets-of-medicine/transmuter/feature/transmogrified-biology';
