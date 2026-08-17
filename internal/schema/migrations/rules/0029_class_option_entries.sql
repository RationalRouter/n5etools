-- Puppet Master's Puppet Upgrade tiers (class_options rows named "Wood
-- Tier"/"Bronze Tier"/etc., both the class-wide "Puppet Master Upgrades"
-- list and each subclass's own exclusive list) each bundle several
-- individually-named upgrades into one prose field, e.g. Black Iron/Wood
-- Tier's "CHAKRA DISRUPTION BLADE Techniques: Black, Perfect ... HIDDEN
-- BLADES Techniques: Black, Perfect ...". This table gives each of those
-- bundled names its own real row (internal/store/classoptionentries.go),
-- so a Puppets sheet tab can offer them as individually pickable upgrades
-- instead of one opaque block of text per tier.
CREATE TABLE class_option_entries (
    slug              TEXT PRIMARY KEY,   -- '<class_options.slug>/entry/<slugified-name>'
    class_option_slug TEXT NOT NULL REFERENCES class_options(slug),
    name              TEXT NOT NULL,
    description       TEXT NOT NULL,
    sort_order        INTEGER,

    source_book      TEXT REFERENCES source_books(slug),
    source_version   TEXT,
    source_page      INTEGER,
    detection_status TEXT NOT NULL DEFAULT 'auto'
                     CHECK (detection_status IN ('auto','needs_review','verified','manual'))
);

CREATE INDEX idx_class_option_entries_option ON class_option_entries(class_option_slug);
