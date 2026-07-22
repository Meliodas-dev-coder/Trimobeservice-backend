-- 000030_employee_access_and_hr_scopes.up.sql
--
-- Unifies HR employees with back-office identities and introduces the two
-- independent authorization axes used by the admin application:
--   * business navigation: department module boundary + position capability;
--   * HR data access: feature + action + employee scope.
--
-- The migration is deliberately additive. Existing admin_roles remain a
-- compatibility/fallback source while departments and positions are configured.

ALTER TABLE hr_departments
    ADD COLUMN parent_department_id BIGINT UNSIGNED NULL AFTER manager_employee_id,
    ADD KEY idx_hr_department_parent (parent_department_id),
    ADD CONSTRAINT fk_hr_department_parent
        FOREIGN KEY (parent_department_id) REFERENCES hr_departments(id) ON DELETE SET NULL;

CREATE TABLE hr_department_modules (
    department_id BIGINT UNSIGNED NOT NULL,
    module_key VARCHAR(80) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (department_id, module_key),
    KEY idx_hr_department_module_key (module_key),
    CONSTRAINT fk_hr_department_module_department
        FOREIGN KEY (department_id) REFERENCES hr_departments(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_position_capabilities (
    position_id BIGINT UNSIGNED NOT NULL,
    capability_key VARCHAR(120) NOT NULL,
    access_level VARCHAR(20) NOT NULL DEFAULT 'read',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (position_id, capability_key),
    KEY idx_hr_position_capability_key (capability_key),
    CONSTRAINT chk_hr_position_capability_level CHECK (access_level IN ('read','manage')),
    CONSTRAINT fk_hr_position_capability_position
        FOREIGN KEY (position_id) REFERENCES hr_positions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_position_hr_policies (
    position_id BIGINT UNSIGNED NOT NULL,
    feature_key VARCHAR(80) NOT NULL,
    action_key VARCHAR(40) NOT NULL,
    employee_scope VARCHAR(30) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (position_id, feature_key, action_key, employee_scope),
    KEY idx_hr_position_policy_feature (feature_key, action_key),
    CONSTRAINT chk_hr_position_policy_scope CHECK (employee_scope IN ('self','reporting_tree','department_tree','all')),
    CONSTRAINT fk_hr_position_policy_position
        FOREIGN KEY (position_id) REFERENCES hr_positions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Exceptional and temporary access is attached to the employee, never to the
-- login account. permission_key is either a business capability
-- (mobility.bookings) or an HR action key (hr.contracts.view). HR rows carry an
-- employee_scope; business rows leave it NULL.
CREATE TABLE hr_employee_access_assignments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    employee_id BIGINT UNSIGNED NOT NULL,
    permission_key VARCHAR(140) NOT NULL,
    access_level VARCHAR(20) NOT NULL DEFAULT 'read',
    employee_scope VARCHAR(30) NULL,
    effect VARCHAR(10) NOT NULL DEFAULT 'allow',
    starts_at DATE NULL,
    ends_at DATE NULL,
    reason VARCHAR(255) NOT NULL,
    granted_by BIGINT UNSIGNED NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_hr_employee_assignment_active (employee_id, starts_at, ends_at),
    KEY idx_hr_employee_assignment_permission (permission_key),
    CONSTRAINT chk_hr_employee_assignment_level CHECK (access_level IN ('read','manage')),
    CONSTRAINT chk_hr_employee_assignment_scope CHECK (employee_scope IS NULL OR employee_scope IN ('self','reporting_tree','department_tree','all')),
    CONSTRAINT chk_hr_employee_assignment_effect CHECK (effect IN ('allow','deny')),
    CONSTRAINT chk_hr_employee_assignment_dates CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at >= starts_at),
    CONSTRAINT fk_hr_employee_assignment_employee
        FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_employee_assignment_granter
        FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Separate, metadata-only audit for sensitive reads and authorization changes.
-- Content, salaries, terms, document bytes, and personal data never land here.
CREATE TABLE hr_access_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    actor_user_id BIGINT UNSIGNED NULL,
    target_employee_id BIGINT UNSIGNED NULL,
    resource_type VARCHAR(80) NOT NULL,
    resource_id BIGINT UNSIGNED NULL,
    action_key VARCHAR(40) NOT NULL,
    scope_source VARCHAR(80) NULL,
    metadata JSON NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_hr_access_event_actor (actor_user_id, created_at),
    KEY idx_hr_access_event_target (target_employee_id, created_at),
    KEY idx_hr_access_event_resource (resource_type, resource_id, created_at),
    CONSTRAINT fk_hr_access_event_actor
        FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_access_event_target
        FOREIGN KEY (target_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Safe automatic reconciliation: only exact-email admin accounts are linked,
-- and only when neither side is already linked elsewhere. Missing accounts are
-- intentionally left for the password-backed provisioning endpoint; the
-- migration never creates an unusable or predictable-credential account.
UPDATE hr_employees e
JOIN users u
  ON LOWER(u.email) = LOWER(e.work_email)
 AND u.role = 'admin'
LEFT JOIN hr_employees linked
  ON linked.user_id = u.id
 AND linked.id <> e.id
SET e.user_id = u.id
WHERE e.user_id IS NULL
  AND linked.id IS NULL;

-- Legacy non-owner Team Members become HR employees instead of remaining a
-- second person directory. Dates/numbers are deterministic placeholders that
-- HR can refine later; the original account remains the authentication source.
INSERT INTO hr_employees (
    user_id, employee_number, first_name, last_name, work_email, phone,
    hire_date, employment_status, employment_type, notes
)
SELECT
    u.id,
    CONCAT('TM-', LPAD(u.id, 8, '0')),
    CASE WHEN LOCATE(' ', TRIM(u.full_name)) > 0
         THEN SUBSTRING_INDEX(TRIM(u.full_name), ' ', 1)
         ELSE TRIM(u.full_name) END,
    CASE WHEN LOCATE(' ', TRIM(u.full_name)) > 0
         THEN TRIM(SUBSTRING(TRIM(u.full_name), LOCATE(' ', TRIM(u.full_name)) + 1))
         ELSE '-' END,
    u.email,
    u.phone,
    DATE(u.created_at),
    CASE WHEN u.is_active THEN 'active' ELSE 'suspended' END,
    'permanent',
    'Migrated from the legacy Team Members directory.'
FROM users u
LEFT JOIN hr_employees by_user ON by_user.user_id = u.id
LEFT JOIN hr_employees by_email ON LOWER(by_email.work_email) = LOWER(u.email)
WHERE u.role = 'admin'
  AND u.is_super_admin = FALSE
  AND by_user.id IS NULL
  AND by_email.id IS NULL;

-- A linked employee is a back-office identity. Suspended/offboarded records are
-- disabled immediately; otherwise preserve the existing account state so this
-- migration never reactivates a deliberately disabled user. The application
-- also revokes refresh tokens during offboarding.
UPDATE users u
JOIN hr_employees e ON e.user_id = u.id
SET u.role = 'admin',
    u.is_active = CASE
        WHEN e.employment_status IN ('offboarded', 'suspended') THEN FALSE
        ELSE u.is_active
    END
WHERE u.is_super_admin = FALSE;
