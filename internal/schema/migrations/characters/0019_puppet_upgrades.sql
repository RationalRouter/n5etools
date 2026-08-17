-- Tracks which individual Puppet Upgrade entries (rules.db
-- class_option_entries, migration 0029) a companion has actually taken —
-- one row per pick. No FK into class_option_entries: it lives in the
-- separate rules.db SQLite file, same cross-DB slug-reference tolerance
-- character_companions.summon_tribe_slug already uses for jutsu_slug/
-- summon_tribe_slug.
--
-- No UNIQUE constraint on (companion_id, upgrade_entry_slug): several real
-- upgrades are explicitly repeatable per their own printed text ("...can
-- take this upgrade up to 3 times") and others aren't, and that per-upgrade
-- repeat-limit is free prose, not structured data — enforcing it would mean
-- parsing rules text, which this app deliberately doesn't attempt (same
-- restraint already drawn at ArmorChassis/Attacks/Traits/Notes). The app
-- enforces only the slot-count-per-tier cap, computed at request time from
-- rules.db's class_level_resources.
CREATE TABLE character_companion_upgrades (
    id                 INTEGER PRIMARY KEY,
    companion_id       INTEGER NOT NULL REFERENCES character_companions(id) ON DELETE CASCADE,
    class_option_slug  TEXT NOT NULL, -- which tier/list this was drawn from
    upgrade_entry_slug TEXT NOT NULL, -- rules.db class_option_entries.slug
    created_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_character_companion_upgrades_companion ON character_companion_upgrades(companion_id);
