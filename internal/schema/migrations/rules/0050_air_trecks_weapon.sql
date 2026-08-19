-- Storm Rider's 3rd-level Air Trecks feature crafts a weapon that has no
-- equipment row anywhere in this schema — its full stat line is printed
-- inline inside the feature's own subclass_features description ("AIR
-- TRECKS Cost Damage Damage Type Weapon Properties Bulk Ryo / 1d6 Slashing
-- Unarmed, Finesse, Light, Multiattack 1"), not in the book's own weapon
-- tables that internal/ingest's Mastersheet-driven weapon load already
-- covers. Same shape as 0019_missing_weapons.sql's three book-table
-- weapons the Mastersheet never had rows for either — a genuine weapon
-- printed in the sourcebook, hand-added because no parser pass reaches
-- this text. No Ryo cost is printed (the Trecks are built, not bought),
-- so cost_ryo is left NULL rather than guessed at.
--
-- equipment.source_book carries a real FK to source_books, and (same gap
-- 0017_adventuring_gear.sql's own comment already documents) source_books
-- is normally populated by n5e-ingest's own upsert, not by any migration —
-- a from-empty schema apply (every schema test, a fresh dev clone before
-- ever running n5e-ingest) has no 'book/class-compendium' row yet to
-- satisfy that FK. INSERT OR IGNORE seeds a minimal placeholder here, same
-- pattern and same reasoning as 0017's own 'book/core' placeholder; a real
-- ingest run afterward overwrites every column via its own ON CONFLICT
-- clause, so this placeholder can never linger as stale data.
INSERT OR IGNORE INTO source_books (slug, title, version, file_name, file_sha256)
VALUES ('book/class-compendium', 'Class Compendium', '3.12', 'class-compendium.pdf', 'unknown');

INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, properties, bulk, description, source_book, source_version, source_page, detection_status) VALUES
('weapon/air-trecks', 'Air Trecks', 'weapon', '1d6', 'Slashing', 'Unarmed, Finesse, Light, Multiattack', 1,
 'A pair of built (not bought) skates granted by Storm Rider''s Air Trecks feature. While activated they count as a simple melee weapon, and Storm Rider''s own text overrides the normal Finesse rule: attack and damage rolls use Intelligence, not the better of Strength/Dexterity.',
 'book/class-compendium', '3.12', 239, 'manual');
