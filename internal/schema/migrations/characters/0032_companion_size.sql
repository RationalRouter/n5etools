-- Puppeteer Chassis/Puppet Frameworks/Puppet Roles/Puppet Weapon Types (the
-- 4 mandatory 2nd-level "build your puppet" picks) each set a companion's
-- size — a field this app never tracked before. TEXT NOT NULL DEFAULT '',
-- not nullable, mirroring armor_chassis/attacks/traits/notes' own "blank
-- string means unset" convention rather than fly_speed's NULL-means-unset
-- convention: size is always a plain player-editable label (like Armor
-- Chassis), never involved in NULL-vs-zero arithmetic the way a numeric
-- stat is.
ALTER TABLE character_companions ADD COLUMN size TEXT NOT NULL DEFAULT '';
