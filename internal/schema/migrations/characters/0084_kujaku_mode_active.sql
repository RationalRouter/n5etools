-- Hoshi Clan's Kujaku Mode (clan/hoshi/feature/kujaku-mode, 3rd level):
-- "as a bonus action you can activate this unique chakra mode... Kujaku
-- Mode lasts for 1 min or until you dismiss it (no action)." No resource
-- consumption or duration tracking backs this on the sheet (the mode's own
-- limited-uses-per-long-rest pool is already tracked separately as the
-- "kujaku_mode_uses" custom resource) — this is just the player's own
-- on/off toggle for whether the mode is currently active, same shape as
-- the existing "inspiration" and "exoskeleton_donned" columns.
ALTER TABLE characters ADD COLUMN kujaku_mode_active INTEGER NOT NULL DEFAULT 0 CHECK (kujaku_mode_active IN (0,1));
