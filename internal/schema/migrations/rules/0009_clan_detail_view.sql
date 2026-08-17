-- v_clans was missing ability_increase_text (added to the clans table in
-- 0003_clan_traits.sql, but never carried into the view) — needed once the
-- app grew a clan detail page beyond the list card's name/epithet/speed.
DROP VIEW v_clans;

CREATE VIEW v_clans AS
SELECT slug, name, epithet, overview,
       COALESCE(speed_feet_override, speed_feet) AS speed_feet,
       extra_language, ability_increase_text, is_legacy, detection_status, notes
FROM clans;
