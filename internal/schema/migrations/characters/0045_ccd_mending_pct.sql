-- Mad Scientist's Biotic Mastery (Science-Nin subclass feature, 3rd level):
-- "you split your CCD into two, each leading into an outlet on the palms of
-- your hand. One hand contains your Mending Device, the other contains your
-- Maiming Device. your CCD is split into two pools. You can change the
-- ratio of the two Devices during a long rest in intervals of 5." The two
-- resulting pools' own current/max readouts reuse the existing
-- character_custom_resources machinery (keys "ccd_mending"/"ccd_maiming",
-- see cmd/n5e/custom_resources.go) — this column holds only the ratio
-- itself (Mending's percentage share of the base Chakra Containment
-- Device's own total, Maiming getting the remainder), since neither the
-- base "ccd" grant's Max function nor character_custom_resources' own
-- (character_id, resource_key) shape has anywhere to carry a player-chosen
-- split point. Same "no natural home, so a plain toggle-shaped column"
-- reasoning migration 0043 already used for exoskeleton_donned, just an int
-- instead of a bool.
ALTER TABLE characters ADD COLUMN ccd_mending_pct INTEGER NOT NULL DEFAULT 50
    CHECK (ccd_mending_pct >= 0 AND ccd_mending_pct <= 100 AND ccd_mending_pct % 5 = 0);
