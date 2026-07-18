-- 000028_admin_roles_and_permissions.up.sql
-- Screen-level access control for the admin back office. Until now every admin
-- saw every screen. This introduces reusable *roles* that bundle a set of
-- section permissions (e.g. ["mobility","orders"]); each employee is assigned
-- one role. The original registered admin(s) keep full access via a new
-- is_super_admin bypass flag.
--
-- users.role (customer|admin) is unchanged: employees are still role='admin'
-- (so they clear the coarse admin gate). The fine-grained control is
-- is_super_admin (bypasses all permission checks) + admin_role_id -> the role's
-- permissions JSON. Permission keys are validated app-side against the Go
-- catalog in internal/authz, so no CHECK constraint is needed here.

CREATE TABLE admin_roles (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(100) NOT NULL,
    description VARCHAR(255) DEFAULT NULL,
    permissions JSON NOT NULL,                     -- array of section keys, e.g. ["mobility","orders"]
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,    -- built-in role: cannot be edited or deleted
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_admin_roles_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- is_super_admin: unrestricted access (the seeded owner account).
-- admin_role_id: which role's screens a restricted employee may reach. NULL for
-- customers and for admins with no role assigned yet (they see nothing until
-- assigned). ON DELETE SET NULL so deleting a role never deletes its people.
ALTER TABLE users
    ADD COLUMN is_super_admin BOOLEAN NOT NULL DEFAULT FALSE AFTER role,
    ADD COLUMN admin_role_id  BIGINT UNSIGNED NULL DEFAULT NULL AFTER is_super_admin,
    ADD KEY idx_users_admin_role (admin_role_id),
    ADD CONSTRAINT fk_users_admin_role
        FOREIGN KEY (admin_role_id) REFERENCES admin_roles (id) ON DELETE SET NULL;

-- Backfill: every admin that exists today keeps full access.
UPDATE users SET is_super_admin = TRUE WHERE role = 'admin';
