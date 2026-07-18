-- 000028_admin_roles_and_permissions.down.sql
-- Reverse of 000028: drop the users columns (FK first), then the roles table.

ALTER TABLE users
    DROP FOREIGN KEY fk_users_admin_role,
    DROP KEY idx_users_admin_role,
    DROP COLUMN admin_role_id,
    DROP COLUMN is_super_admin;

DROP TABLE IF EXISTS admin_roles;
