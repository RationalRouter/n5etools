-- Intelligence Operative's Plans (base class feature, Master Planner, 2nd
-- level) prints its own 21-option catalog directly under its own section,
-- with no subclass gate anywhere in its text ("You learn two plans of your
-- choice, which are detailed in the plans Section at the end of this
-- class... Base Plan: ..."). The PDF outline bookmarks this catalog right
-- after Tactical Strategist's own subclass section, which
-- internal/parse/subclasses.go's "option list belongs to the subclass
-- printed right before it" heuristic mistook for a real subclass gate --
-- the same PDF-outline-bookmarking artifact already fixed for Taijutsu
-- Specialist's Martial Techniques (mistagged to Passionate Flame) and
-- Ninjutsu Specialist's Efficient Molding (mistagged to Tsunami).
-- internal/parse/subclasses.go now special-cases this list on any FUTURE
-- re-ingest; this migration repairs an already-shipped install's 21 rows in
-- the meantime.
UPDATE class_options
SET subclass_slug = NULL
WHERE class_slug = 'class/intelligence-operative'
  AND list_name = 'Plans'
  AND subclass_slug = 'class/intelligence-operative/group/master-strategies/tactical-strategist';
