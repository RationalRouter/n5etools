-- Character portrait, stored inline as a data: URL (e.g.
-- "data:image/png;base64,...") rather than as a path to a file on disk.
--
-- This app ships as a single portable binary that a player copies around
-- alongside characters.db; a portrait kept as a filesystem path would
-- break the moment the .db moved without the image beside it, and there is
-- no other user-writable asset directory in the design. Keeping the bytes
-- in the row means a character is still one self-contained thing. The
-- upload handler caps the accepted file size so a row stays a sane size.
ALTER TABLE characters ADD COLUMN portrait TEXT;
