-- Ninjutsu Specialist's Efficient Molding (base class feature, 3rd level)
-- prints its own 17-option catalog directly under its own section, with no
-- subclass gate anywhere in its text ("you learn how to bend your chakra to
-- suit your needs while casting Ninjutsu... You gain two of the following
-- Efficient Molding options of your choice"). The PDF outline bookmarks
-- this catalog right after Tsunami's own subclass section, which
-- internal/parse/subclasses.go's "option list belongs to the subclass
-- printed right before it" heuristic mistook for a real subclass gate —
-- the same PDF-outline-bookmarking artifact already fixed once for
-- Taijutsu Specialist's Martial Techniques (mistagged to Passionate
-- Flame). internal/parse/subclasses.go now special-cases this list on any
-- FUTURE re-ingest; this migration repairs an already-shipped install's
-- 17 rows in the meantime.
UPDATE class_options
SET subclass_slug = NULL
WHERE class_slug = 'class/ninjutsu-specialist'
  AND list_name = 'Efficient Molding'
  AND subclass_slug = 'class/ninjutsu-specialist/group/ninjutsu-focus/tsunami';
