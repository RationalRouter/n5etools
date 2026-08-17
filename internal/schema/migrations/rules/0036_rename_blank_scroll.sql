-- "Blank Weapon/Item/Jutsu Scroll" read as a concatenated, confusing name —
-- the slash-separated suffix describes three things the same physical
-- scroll can later be prepared to hold, not three separate items. The
-- existing description already explains that; the name no longer needs to.
UPDATE equipment SET name = 'Blank Scroll' WHERE slug = 'scroll/blank-scroll';
