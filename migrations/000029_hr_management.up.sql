-- 000029_hr_management.up.sql
-- Native Human Resources suite. Employees intentionally remain a separate
-- domain from admin users; user_id is an optional bridge for future self service.

CREATE TABLE hr_departments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(120) NOT NULL, code VARCHAR(30) NOT NULL,
    description TEXT NULL, translations JSON NULL, manager_employee_id BIGINT UNSIGNED NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_departments_name (name), UNIQUE KEY uq_hr_departments_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_positions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    department_id BIGINT UNSIGNED NULL, title VARCHAR(140) NOT NULL, code VARCHAR(30) NOT NULL,
    description TEXT NULL, translations JSON NULL, grade VARCHAR(30) NULL,
    min_salary DECIMAL(12,2) NULL, max_salary DECIMAL(12,2) NULL, currency CHAR(3) NOT NULL DEFAULT 'MGA',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_positions_code (code), KEY idx_hr_positions_department (department_id),
    CONSTRAINT chk_hr_position_salary CHECK ((min_salary IS NULL OR min_salary >= 0) AND (max_salary IS NULL OR max_salary >= 0) AND (min_salary IS NULL OR max_salary IS NULL OR min_salary <= max_salary)),
    CONSTRAINT fk_hr_positions_department FOREIGN KEY (department_id) REFERENCES hr_departments(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_employees (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, user_id BIGINT UNSIGNED NULL,
    employee_number VARCHAR(40) NOT NULL, first_name VARCHAR(100) NOT NULL, last_name VARCHAR(100) NOT NULL,
    work_email VARCHAR(191) NOT NULL, personal_email VARCHAR(191) NULL, phone VARCHAR(40) NULL,
    date_of_birth DATE NULL, gender VARCHAR(30) NULL, nationality VARCHAR(80) NULL, address TEXT NULL,
    hire_date DATE NOT NULL, end_date DATE NULL, department_id BIGINT UNSIGNED NULL, position_id BIGINT UNSIGNED NULL,
    manager_id BIGINT UNSIGNED NULL, employment_status VARCHAR(30) NOT NULL DEFAULT 'onboarding',
    employment_type VARCHAR(30) NOT NULL DEFAULT 'permanent', work_location VARCHAR(160) NULL,
    photo_url VARCHAR(2048) NULL, notes TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_employee_number (employee_number), UNIQUE KEY uq_hr_employee_work_email (work_email),
    UNIQUE KEY uq_hr_employee_user (user_id), KEY idx_hr_employee_name (last_name, first_name),
    KEY idx_hr_employee_department (department_id), KEY idx_hr_employee_position (position_id), KEY idx_hr_employee_manager (manager_id),
    CONSTRAINT fk_hr_employee_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_employee_department FOREIGN KEY (department_id) REFERENCES hr_departments(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_employee_position FOREIGN KEY (position_id) REFERENCES hr_positions(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_employee_manager FOREIGN KEY (manager_id) REFERENCES hr_employees(id) ON DELETE SET NULL,
    CONSTRAINT chk_hr_employee_dates CHECK (end_date IS NULL OR end_date >= hire_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE hr_departments ADD KEY idx_hr_department_manager (manager_employee_id),
    ADD CONSTRAINT fk_hr_department_manager FOREIGN KEY (manager_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL;

CREATE TABLE hr_emergency_contacts (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL,
    name VARCHAR(160) NOT NULL, relationship VARCHAR(80) NOT NULL, phone VARCHAR(40) NOT NULL,
    alternate_phone VARCHAR(40) NULL, email VARCHAR(191) NULL, is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_contact_employee (employee_id),
    CONSTRAINT fk_hr_contact_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_contracts (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL,
    contract_type VARCHAR(40) NOT NULL, start_date DATE NOT NULL, end_date DATE NULL, probation_end_date DATE NULL,
    salary DECIMAL(12,2) NOT NULL DEFAULT 0, currency CHAR(3) NOT NULL DEFAULT 'MGA', pay_frequency VARCHAR(30) NOT NULL DEFAULT 'monthly',
    status VARCHAR(30) NOT NULL DEFAULT 'draft', document_url VARCHAR(2048) NULL, terms TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_contract_employee (employee_id), KEY idx_hr_contract_dates (start_date, end_date),
    CONSTRAINT chk_hr_contract_values CHECK (salary >= 0 AND (end_date IS NULL OR end_date >= start_date) AND (probation_end_date IS NULL OR probation_end_date >= start_date)),
    CONSTRAINT fk_hr_contract_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_documents (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL,
    document_type VARCHAR(60) NOT NULL, name VARCHAR(180) NOT NULL, file_url VARCHAR(2048) NULL,
    file_key VARCHAR(512) NULL, file_name VARCHAR(255) NULL, mime_type VARCHAR(100) NULL,
    size_bytes BIGINT UNSIGNED NULL, file_data MEDIUMBLOB NULL,
    expires_at DATE NULL, is_confidential BOOLEAN NOT NULL DEFAULT TRUE, notes TEXT NULL,
    uploaded_by BIGINT UNSIGNED NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_document_employee (employee_id), KEY idx_hr_document_expiry (expires_at),
    CONSTRAINT fk_hr_document_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_document_uploader FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_lifecycle_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL,
    event_type VARCHAR(30) NOT NULL, effective_date DATE NOT NULL, status VARCHAR(30) NOT NULL DEFAULT 'scheduled',
    from_department_id BIGINT UNSIGNED NULL, to_department_id BIGINT UNSIGNED NULL,
    from_position_id BIGINT UNSIGNED NULL, to_position_id BIGINT UNSIGNED NULL,
    from_manager_id BIGINT UNSIGNED NULL, to_manager_id BIGINT UNSIGNED NULL,
    probation_end_date DATE NULL, title VARCHAR(180) NULL, notes TEXT NULL,
    created_by BIGINT UNSIGNED NULL, completed_at DATETIME NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_lifecycle_employee (employee_id), KEY idx_hr_lifecycle_effective (effective_date, status),
    CONSTRAINT fk_hr_lifecycle_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_lifecycle_creator FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_lifecycle_from_department FOREIGN KEY (from_department_id) REFERENCES hr_departments(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_lifecycle_to_department FOREIGN KEY (to_department_id) REFERENCES hr_departments(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_lifecycle_from_position FOREIGN KEY (from_position_id) REFERENCES hr_positions(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_lifecycle_to_position FOREIGN KEY (to_position_id) REFERENCES hr_positions(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_lifecycle_from_manager FOREIGN KEY (from_manager_id) REFERENCES hr_employees(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_lifecycle_to_manager FOREIGN KEY (to_manager_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_leave_policies (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, name VARCHAR(120) NOT NULL, code VARCHAR(30) NOT NULL,
    leave_type VARCHAR(50) NOT NULL, description TEXT NULL, translations JSON NULL,
    days_per_year DECIMAL(6,2) NOT NULL DEFAULT 0, accrual_mode VARCHAR(30) NOT NULL DEFAULT 'annual',
    carry_over_days DECIMAL(6,2) NOT NULL DEFAULT 0, minimum_notice_days INT NOT NULL DEFAULT 0,
    max_consecutive_days INT NULL, requires_attachment BOOLEAN NOT NULL DEFAULT FALSE,
    approval_levels JSON NOT NULL, is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_leave_policy_code (code),
    CONSTRAINT chk_hr_leave_policy_values CHECK (days_per_year >= 0 AND carry_over_days >= 0 AND minimum_notice_days >= 0 AND (max_consecutive_days IS NULL OR max_consecutive_days > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_leave_balances (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, policy_id BIGINT UNSIGNED NOT NULL, balance_year SMALLINT NOT NULL,
    allocated_days DECIMAL(7,2) NOT NULL DEFAULT 0, carried_days DECIMAL(7,2) NOT NULL DEFAULT 0,
    adjustment_days DECIMAL(7,2) NOT NULL DEFAULT 0, used_days DECIMAL(7,2) NOT NULL DEFAULT 0, pending_days DECIMAL(7,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_leave_balance (employee_id, policy_id, balance_year), KEY idx_hr_leave_balance_policy (policy_id),
    CONSTRAINT chk_hr_leave_balance_values CHECK (allocated_days >= 0 AND carried_days >= 0 AND used_days >= 0 AND pending_days >= 0 AND allocated_days+carried_days+adjustment_days-used_days-pending_days >= 0),
    CONSTRAINT fk_hr_leave_balance_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_leave_balance_policy FOREIGN KEY (policy_id) REFERENCES hr_leave_policies(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_leave_requests (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, policy_id BIGINT UNSIGNED NOT NULL,
    start_date DATE NOT NULL, end_date DATE NOT NULL, requested_days DECIMAL(7,2) NOT NULL,
    start_portion VARCHAR(20) NOT NULL DEFAULT 'full', end_portion VARCHAR(20) NOT NULL DEFAULT 'full',
    reason TEXT NULL, document_id BIGINT UNSIGNED NULL, attachment_url VARCHAR(2048) NULL, status VARCHAR(30) NOT NULL DEFAULT 'pending',
    current_approval_level INT NOT NULL DEFAULT 1, total_approval_levels INT NOT NULL DEFAULT 1,
    approval_levels_snapshot JSON NOT NULL,
    submitted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, decided_at DATETIME NULL, decided_by BIGINT UNSIGNED NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_leave_request_employee (employee_id), KEY idx_hr_leave_request_status (status, start_date),
    CONSTRAINT chk_hr_leave_request_dates CHECK (end_date >= start_date AND requested_days > 0),
    CONSTRAINT fk_hr_leave_request_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_leave_request_policy FOREIGN KEY (policy_id) REFERENCES hr_leave_policies(id) ON DELETE RESTRICT,
    CONSTRAINT fk_hr_leave_request_document FOREIGN KEY (document_id) REFERENCES hr_documents(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_leave_request_decider FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_leave_approvals (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, request_id BIGINT UNSIGNED NOT NULL, approval_level INT NOT NULL,
    approver_user_id BIGINT UNSIGNED NOT NULL, action VARCHAR(20) NOT NULL, comment TEXT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_leave_approval (request_id, approval_level), KEY idx_hr_leave_approver (approver_user_id),
    CONSTRAINT fk_hr_leave_approval_request FOREIGN KEY (request_id) REFERENCES hr_leave_requests(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_leave_approval_user FOREIGN KEY (approver_user_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_shifts (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, name VARCHAR(100) NOT NULL, code VARCHAR(30) NOT NULL,
    start_time TIME NOT NULL, end_time TIME NOT NULL, break_minutes INT NOT NULL DEFAULT 0, grace_minutes INT NOT NULL DEFAULT 0,
    work_days JSON NOT NULL, is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_shift_code (code),
    CONSTRAINT chk_hr_shift_minutes CHECK (break_minutes >= 0 AND grace_minutes >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_shift_assignments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, shift_id BIGINT UNSIGNED NOT NULL,
    start_date DATE NOT NULL, end_date DATE NULL, status VARCHAR(30) NOT NULL DEFAULT 'active', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_shift_assignment_employee (employee_id, start_date),
    CONSTRAINT chk_hr_shift_assignment_dates CHECK (end_date IS NULL OR end_date >= start_date),
    CONSTRAINT fk_hr_shift_assignment_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_shift_assignment_shift FOREIGN KEY (shift_id) REFERENCES hr_shifts(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_attendance (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, shift_id BIGINT UNSIGNED NULL, attendance_date DATE NOT NULL,
    clock_in DATETIME NULL, clock_out DATETIME NULL, status VARCHAR(30) NOT NULL DEFAULT 'present',
    worked_minutes INT NOT NULL DEFAULT 0, late_minutes INT NOT NULL DEFAULT 0, overtime_minutes INT NOT NULL DEFAULT 0, source VARCHAR(30) NOT NULL DEFAULT 'manual', notes TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_attendance_employee_date (employee_id, attendance_date), KEY idx_hr_attendance_date (attendance_date, status),
    CONSTRAINT chk_hr_attendance_minutes CHECK (worked_minutes >= 0 AND late_minutes >= 0 AND overtime_minutes >= 0),
    CONSTRAINT fk_hr_attendance_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_attendance_shift FOREIGN KEY (shift_id) REFERENCES hr_shifts(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_timesheets (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, week_start DATE NOT NULL,
    regular_hours DECIMAL(7,2) NOT NULL DEFAULT 0, overtime_hours DECIMAL(7,2) NOT NULL DEFAULT 0, entries JSON NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'draft', submitted_at DATETIME NULL, approved_at DATETIME NULL, approved_by BIGINT UNSIGNED NULL, notes TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_timesheet_employee_week (employee_id, week_start),
    CONSTRAINT chk_hr_timesheet_hours CHECK (regular_hours >= 0 AND overtime_hours >= 0),
    CONSTRAINT fk_hr_timesheet_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_timesheet_approver FOREIGN KEY (approved_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_performance_reviews (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, reviewer_employee_id BIGINT UNSIGNED NULL,
    review_period_start DATE NOT NULL, review_period_end DATE NOT NULL, review_type VARCHAR(40) NOT NULL DEFAULT 'annual',
    status VARCHAR(30) NOT NULL DEFAULT 'draft', overall_rating DECIMAL(3,2) NULL, strengths TEXT NULL, improvements TEXT NULL,
    employee_comments TEXT NULL, reviewer_comments TEXT NULL, completed_at DATETIME NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_review_employee (employee_id), KEY idx_hr_review_period (review_period_end, status),
    CONSTRAINT chk_hr_review_values CHECK (review_period_end >= review_period_start AND (overall_rating IS NULL OR (overall_rating >= 0 AND overall_rating <= 5))),
    CONSTRAINT fk_hr_review_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_review_reviewer FOREIGN KEY (reviewer_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_goals (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, review_id BIGINT UNSIGNED NULL,
    title VARCHAR(180) NOT NULL, description TEXT NULL, start_date DATE NULL, due_date DATE NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'not_started', progress_percent INT NOT NULL DEFAULT 0, weight_percent INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_goal_employee (employee_id),
    CONSTRAINT chk_hr_goal_values CHECK (progress_percent BETWEEN 0 AND 100 AND weight_percent BETWEEN 0 AND 100 AND (start_date IS NULL OR due_date IS NULL OR due_date >= start_date)),
    CONSTRAINT fk_hr_goal_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_goal_review FOREIGN KEY (review_id) REFERENCES hr_performance_reviews(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_feedback (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, author_employee_id BIGINT UNSIGNED NULL,
    feedback_type VARCHAR(30) NOT NULL DEFAULT 'manager', visibility VARCHAR(30) NOT NULL DEFAULT 'employee', content TEXT NOT NULL,
    rating DECIMAL(3,2) NULL, feedback_date DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_feedback_employee (employee_id),
    CONSTRAINT chk_hr_feedback_rating CHECK (rating IS NULL OR (rating >= 0 AND rating <= 5)),
    CONSTRAINT fk_hr_feedback_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_feedback_author FOREIGN KEY (author_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_one_to_ones (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, manager_employee_id BIGINT UNSIGNED NULL,
    scheduled_at DATETIME NOT NULL, completed_at DATETIME NULL, status VARCHAR(30) NOT NULL DEFAULT 'scheduled',
    agenda TEXT NULL, notes TEXT NULL, action_items JSON NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_one_to_one_employee (employee_id), KEY idx_hr_one_to_one_schedule (scheduled_at, status),
    CONSTRAINT fk_hr_one_to_one_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_one_to_one_manager FOREIGN KEY (manager_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_vacancies (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, position_id BIGINT UNSIGNED NULL, department_id BIGINT UNSIGNED NULL,
    title VARCHAR(180) NOT NULL, code VARCHAR(40) NOT NULL, description TEXT NULL, employment_type VARCHAR(30) NOT NULL DEFAULT 'permanent',
    location VARCHAR(160) NULL, openings INT NOT NULL DEFAULT 1, status VARCHAR(30) NOT NULL DEFAULT 'draft',
    published_at DATETIME NULL, closes_at DATETIME NULL, hiring_manager_employee_id BIGINT UNSIGNED NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_vacancy_code (code), KEY idx_hr_vacancy_status (status),
    CONSTRAINT chk_hr_vacancy_openings CHECK (openings > 0),
    CONSTRAINT fk_hr_vacancy_position FOREIGN KEY (position_id) REFERENCES hr_positions(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_vacancy_department FOREIGN KEY (department_id) REFERENCES hr_departments(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_vacancy_manager FOREIGN KEY (hiring_manager_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_candidates (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, vacancy_id BIGINT UNSIGNED NOT NULL, first_name VARCHAR(100) NOT NULL, last_name VARCHAR(100) NOT NULL,
    email VARCHAR(191) NOT NULL, phone VARCHAR(40) NULL, source VARCHAR(80) NULL, resume_url VARCHAR(2048) NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'applied', rating DECIMAL(3,2) NULL, notes TEXT NULL, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    hired_employee_id BIGINT UNSIGNED NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_candidate_vacancy_email (vacancy_id, email), KEY idx_hr_candidate_status (vacancy_id, status),
    CONSTRAINT chk_hr_candidate_rating CHECK (rating IS NULL OR (rating >= 0 AND rating <= 5)),
    CONSTRAINT fk_hr_candidate_vacancy FOREIGN KEY (vacancy_id) REFERENCES hr_vacancies(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_candidate_hired FOREIGN KEY (hired_employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_interviews (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, candidate_id BIGINT UNSIGNED NOT NULL, interview_type VARCHAR(40) NOT NULL,
    scheduled_at DATETIME NOT NULL, duration_minutes INT NOT NULL DEFAULT 60, location VARCHAR(255) NULL,
    interviewer_employee_ids JSON NULL, status VARCHAR(30) NOT NULL DEFAULT 'scheduled', score DECIMAL(3,2) NULL, feedback TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_interview_candidate (candidate_id), KEY idx_hr_interview_schedule (scheduled_at, status),
    CONSTRAINT chk_hr_interview_values CHECK (duration_minutes > 0 AND (score IS NULL OR (score >= 0 AND score <= 5))),
    CONSTRAINT fk_hr_interview_candidate FOREIGN KEY (candidate_id) REFERENCES hr_candidates(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_offers (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, candidate_id BIGINT UNSIGNED NOT NULL, position_id BIGINT UNSIGNED NULL,
    offered_salary DECIMAL(12,2) NOT NULL, currency CHAR(3) NOT NULL DEFAULT 'MGA', start_date DATE NOT NULL,
    expires_at DATE NULL, status VARCHAR(30) NOT NULL DEFAULT 'draft', document_url VARCHAR(2048) NULL, terms TEXT NULL,
    sent_at DATETIME NULL, responded_at DATETIME NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_offer_candidate (candidate_id),
    CONSTRAINT chk_hr_offer_values CHECK (offered_salary >= 0 AND (expires_at IS NULL OR expires_at <= start_date)),
    CONSTRAINT fk_hr_offer_candidate FOREIGN KEY (candidate_id) REFERENCES hr_candidates(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_offer_position FOREIGN KEY (position_id) REFERENCES hr_positions(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_expenses (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, expense_date DATE NOT NULL,
    category VARCHAR(80) NOT NULL, description VARCHAR(255) NOT NULL, amount DECIMAL(12,2) NOT NULL, currency CHAR(3) NOT NULL DEFAULT 'MGA',
    receipt_url VARCHAR(2048) NULL, status VARCHAR(30) NOT NULL DEFAULT 'pending', submitted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    approved_at DATETIME NULL, approved_by BIGINT UNSIGNED NULL, reimbursed_at DATETIME NULL, reimbursed_by BIGINT UNSIGNED NULL,
    payment_reference VARCHAR(120) NULL, rejection_reason TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_expense_employee (employee_id), KEY idx_hr_expense_status (status, expense_date),
    CONSTRAINT chk_hr_expense_amount CHECK (amount >= 0),
    CONSTRAINT fk_hr_expense_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_expense_approver FOREIGN KEY (approved_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_expense_reimburser FOREIGN KEY (reimbursed_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_expense_actions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, expense_id BIGINT UNSIGNED NOT NULL, actor_user_id BIGINT UNSIGNED NOT NULL,
    action VARCHAR(30) NOT NULL, comment TEXT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_expense_action (expense_id),
    CONSTRAINT fk_hr_expense_action_expense FOREIGN KEY (expense_id) REFERENCES hr_expenses(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_expense_action_user FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_compensation (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, employee_id BIGINT UNSIGNED NOT NULL, effective_date DATE NOT NULL,
    base_salary DECIMAL(12,2) NOT NULL, currency CHAR(3) NOT NULL DEFAULT 'MGA', pay_frequency VARCHAR(30) NOT NULL DEFAULT 'monthly',
    bonus_target DECIMAL(12,2) NULL, allowances JSON NULL, reason VARCHAR(255) NULL, is_current BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_compensation_employee (employee_id, effective_date),
    CONSTRAINT chk_hr_compensation_values CHECK (base_salary >= 0 AND (bonus_target IS NULL OR bonus_target >= 0)),
    CONSTRAINT fk_hr_compensation_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_benefits (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, name VARCHAR(140) NOT NULL, code VARCHAR(30) NOT NULL,
    benefit_type VARCHAR(50) NOT NULL, description TEXT NULL, translations JSON NULL,
    employer_contribution DECIMAL(12,2) NOT NULL DEFAULT 0, employee_contribution DECIMAL(12,2) NOT NULL DEFAULT 0,
    currency CHAR(3) NOT NULL DEFAULT 'MGA', is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_benefit_code (code),
    CONSTRAINT chk_hr_benefit_values CHECK (employer_contribution >= 0 AND employee_contribution >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_benefit_enrollments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, benefit_id BIGINT UNSIGNED NOT NULL, employee_id BIGINT UNSIGNED NOT NULL,
    start_date DATE NOT NULL, end_date DATE NULL, status VARCHAR(30) NOT NULL DEFAULT 'active', details JSON NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id), UNIQUE KEY uq_hr_benefit_enrollment (benefit_id, employee_id, start_date),
    CONSTRAINT chk_hr_enrollment_dates CHECK (end_date IS NULL OR end_date >= start_date),
    CONSTRAINT fk_hr_enrollment_benefit FOREIGN KEY (benefit_id) REFERENCES hr_benefits(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_enrollment_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_notifications (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, recipient_user_id BIGINT UNSIGNED NULL, employee_id BIGINT UNSIGNED NULL,
    notification_type VARCHAR(60) NOT NULL, title VARCHAR(180) NOT NULL, message TEXT NOT NULL,
    entity_type VARCHAR(60) NULL, entity_id BIGINT UNSIGNED NULL, is_read BOOLEAN NOT NULL DEFAULT FALSE, read_at DATETIME NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_notification_user (recipient_user_id, is_read, created_at), KEY idx_hr_notification_employee (employee_id),
    CONSTRAINT fk_hr_notification_user FOREIGN KEY (recipient_user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_hr_notification_employee FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE hr_workflow_actions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    resource_type VARCHAR(60) NOT NULL, resource_id BIGINT UNSIGNED NOT NULL,
    from_status VARCHAR(30) NOT NULL, to_status VARCHAR(30) NOT NULL, action VARCHAR(30) NOT NULL,
    actor_user_id BIGINT UNSIGNED NOT NULL, note TEXT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id), KEY idx_hr_workflow_resource (resource_type,resource_id,created_at), KEY idx_hr_workflow_actor (actor_user_id),
    CONSTRAINT fk_hr_workflow_actor FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Ready-to-assign personas. `hr` is the full-access umbrella; the other roles
-- demonstrate how domain capabilities compose without coupling HR employees to
-- admin accounts. Sites with an existing role of the same name keep their role.
INSERT IGNORE INTO admin_roles (name, description, permissions, is_system) VALUES
('HR Admin', 'Full access to the native HR suite.', JSON_ARRAY('hr'), TRUE),
('HR Manager', 'Directory, lifecycle, leave, attendance, and performance.', JSON_ARRAY('hr_dashboard','hr_employees','hr_organization','hr_lifecycle','hr_leave','hr_attendance','hr_performance','hr_reports'), TRUE),
('HR Finance', 'Expenses, reimbursement, compensation, benefits, and reports.', JSON_ARRAY('hr_expenses','hr_compensation','hr_reports'), TRUE),
('HR Recruiter', 'Vacancies, candidates, interviews, offers, and hiring.', JSON_ARRAY('hr_recruitment','hr_reports'), TRUE),
('HR Employee', 'Baseline employee self-service access to personal HR records and requests.', JSON_ARRAY('hr_employee'), TRUE);
