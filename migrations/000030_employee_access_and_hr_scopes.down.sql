-- 000030_employee_access_and_hr_scopes.down.sql
-- Account links and role/status reconciliation are data migrations and are not
-- undone: unlinking them would orphan valid employee login identities.

DROP TABLE IF EXISTS hr_access_events;
DROP TABLE IF EXISTS hr_employee_access_assignments;
DROP TABLE IF EXISTS hr_position_hr_policies;
DROP TABLE IF EXISTS hr_position_capabilities;
DROP TABLE IF EXISTS hr_department_modules;

ALTER TABLE hr_departments
    DROP FOREIGN KEY fk_hr_department_parent,
    DROP KEY idx_hr_department_parent,
    DROP COLUMN parent_department_id;
