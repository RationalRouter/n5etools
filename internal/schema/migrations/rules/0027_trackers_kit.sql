-- Hunter-Nin's Multiclassing Proficiencies row (core book, page 179) grants
-- "trackers kit" on top of light armor and a skill choice — but no such
-- toolkit exists anywhere in the equipment catalog under that name (checked:
-- not "Trappers Kit", a real but unrelated trap-setting toolkit Taijutsu
-- Specialist/Weapon Specialist already have). This is a genuine content gap,
-- not a naming mismatch to resolve against an existing row, so it gets a
-- real new catalog entry rather than a guessed substitution. Priced/shaped
-- like every other starting-tier toolkit (Disguise/Forensics/Forgery Kit:
-- 200 ryo, no recorded weight — same gap those already ship with).
INSERT INTO equipment (slug, name, kind, cost_ryo, description, source_book, source_version, source_page, detection_status, notes) VALUES
('toolkit/trackers-kit', 'Trackers Kit', 'toolkit', 200,
 'Grants proficiency bonus to checks made to track a creature or find and follow a trail.',
 'book/core', '3.11', 179, 'manual',
 'Added for the Hunter-Nin multiclassing proficiency grant, which names this kit but no catalog entry existed for it.');
