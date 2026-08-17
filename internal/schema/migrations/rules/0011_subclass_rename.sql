-- "Archetype" is a real, distinct rules term used elsewhere in the
-- sourcebooks (the multiclassing-replacement mechanic, and a class-feat
-- category label the books print literally as "Archetype:") — the books
-- never call subclasses archetypes, so the earlier schema naming was
-- misleading to the dev team. Renaming the subclass system's own tables to
-- match; the unrelated real "Archetype" usages (class_multiclass_rules,
-- feats.category/class_name, the PDF-text match in internal/parse/clans.go)
-- are untouched on purpose.
ALTER TABLE archetype_groups   RENAME TO subclass_groups;
ALTER TABLE archetypes         RENAME TO subclasses;
ALTER TABLE archetype_features RENAME TO subclass_features;

ALTER TABLE subclass_features RENAME COLUMN archetype_slug TO subclass_slug;
ALTER TABLE class_options     RENAME COLUMN archetype_slug TO subclass_slug;

DROP VIEW v_archetype_features;
CREATE VIEW v_subclass_features AS
SELECT slug, subclass_slug, name,
       COALESCE(level_override, level) AS level,
       description, sort_order, detection_status, notes
FROM subclass_features;
