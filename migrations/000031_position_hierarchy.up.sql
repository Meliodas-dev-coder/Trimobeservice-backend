-- 000031_position_hierarchy.up.sql
--
-- Adds a base position hierarchy to each department: a self-referencing parent
-- pointer plus an ordering rank on hr_positions. This is the template the org
-- uses to (a) seed a new hire's manager at onboarding and (b) drive leave
-- approval steps that walk up the position chain (`position_hierarchy` approver).
--
-- Deliberately additive: existing positions default to no parent and rank 0, so
-- nothing changes until a department's ladder is configured. The column is named
-- `hierarchy_rank` (not `rank`) because RANK is a reserved word in MySQL 8 and the
-- HR resource engine builds unquoted column lists.
--
-- Same-department parent and cycle prevention are enforced in the service layer
-- (they reference other rows, which a single-row CHECK cannot express), mirroring
-- how hr_departments.parent_department_id is validated.

ALTER TABLE hr_positions
    ADD COLUMN parent_position_id BIGINT UNSIGNED NULL AFTER department_id,
    ADD COLUMN hierarchy_rank INT NOT NULL DEFAULT 0 AFTER parent_position_id,
    ADD KEY idx_hr_positions_parent (parent_position_id),
    ADD CONSTRAINT fk_hr_positions_parent
        FOREIGN KEY (parent_position_id) REFERENCES hr_positions(id) ON DELETE SET NULL;
