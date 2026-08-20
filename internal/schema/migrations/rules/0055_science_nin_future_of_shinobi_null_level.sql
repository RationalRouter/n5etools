-- Science-Nin has 5 of its 10 "Future of Shinobi" subclass capstones
-- (Mad Scientist, Ninjaneer, Shinobi-Ware, Spyware, Technobi) shipped with
-- NULL level. The other 5 capstones (Elemental Innovationist, Grenadier,
-- Mech-Crafter, S.N.B Specialist, Storm-Rider) open "At/Finally at 20th
-- level" and were parsed correctly; these 5 open "At Level 20, ..." (a bare
-- number after the word "Level", no ordinal suffix), which
-- internal/parse/subclasses.go's ordinalLevelRe cannot match. LoadGrantedFeatures
-- treats a NULL level as always-granted, so these 5 capstones were live at
-- any level for their subclass instead of gated to 20th.
--
-- internal/parse/subclasses.go's knownFeatureLevelOverrides now fixes this
-- on any future re-ingest, the same fallback mechanism already used for
-- Gungnir Piercer Form and (in migration 0053) Genjutsu Specialist's own
-- NULL-level features.
--
-- This migration repairs the already-shipped rows.
UPDATE subclass_features
SET level = 20
WHERE slug IN (
    'class/science-nin/group/scientific-inquiry/mad-scientist/feature/the-future-of-shinobi-biology',
    'class/science-nin/group/scientific-inquiry/ninjaneer/feature/the-future-of-shinobi-weapons',
    'class/science-nin/group/scientific-inquiry/shinobi-ware/feature/the-future-of-shinobi-shinobi-ware',
    'class/science-nin/group/scientific-inquiry/spyware/feature/the-future-of-shinobi-programs',
    'class/science-nin/group/scientific-inquiry/technobi/feature/the-future-of-shinobi-scrolls'
  );
