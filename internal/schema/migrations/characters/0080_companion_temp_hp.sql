-- A companion's own temporary hit points — the same "extra pool absorbed
-- before real HP, set outright rather than added to" mechanic
-- charstore.SetHP/SetBaseTempHP already give the player character on the
-- Core sheet (see internal/charstore/sheet.go), extended to companions.
-- Nullable like every other companion stat pool (hp_current, barrier_current,
-- ...): NULL means "never touched", not "zero temp HP".
ALTER TABLE character_companions ADD COLUMN temp_hp INTEGER;
