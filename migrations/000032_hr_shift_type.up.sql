-- 000032_hr_shift_type.up.sql
--
-- Shifts are now categorized by a fixed `shift_type` (day | night | special)
-- rather than an entirely free-form name. Day and night shifts carry a canonical
-- name ("Day" / "Night"); only `special` shifts keep a user-supplied name.
--
-- Additive and non-breaking: existing rows have arbitrary custom names, so they
-- backfill to `special` (the honest category for a freely named shift). New
-- inserts always send the type explicitly (it is a required app-level field);
-- the column keeps a default only so the ALTER can populate existing rows.

ALTER TABLE hr_shifts
    ADD COLUMN shift_type VARCHAR(20) NOT NULL DEFAULT 'special' AFTER name;
